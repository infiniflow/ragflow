//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
	"go.uber.org/zap"
)

// S3Storage implements Storage interface for AWS S3
type S3Storage struct {
	client     *s3.Client
	bucket     string
	prefixPath string
	config     *S3Config
}

// NewS3Storage creates a new S3 storage instance
func NewS3Storage(config *S3Config) (*S3Storage, error) {
	storage := &S3Storage{
		bucket:     config.Bucket,
		prefixPath: config.PrefixPath,
		config:     config,
	}

	if err := storage.connect(); err != nil {
		return nil, err
	}

	return storage, nil
}

func (s *S3Storage) connect() error {
	ctx := context.Background()

	var opts []func(*config.LoadOptions) error

	// Configure region
	if s.config.Region != "" {
		opts = append(opts, config.WithRegion(s.config.Region))
	}

	// Configure credentials if provided
	if s.config.AccessKeyID != "" && s.config.SecretAccessKey != "" {
		creds := credentials.NewStaticCredentialsProvider(
			s.config.AccessKeyID,
			s.config.SecretAccessKey,
			s.config.SessionToken,
		)
		opts = append(opts, config.WithCredentialsProvider(creds))
	}

	// Load configuration
	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with custom endpoint if provided
	clientOpts := []func(*s3.Options){}
	if s.config.EndpointURL != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(s.config.EndpointURL)
		})
	}

	s.client = s3.NewFromConfig(cfg, clientOpts...)
	return nil
}

func (s *S3Storage) reconnect() {
	if err := s.connect(); err != nil {
		zap.L().Error("Failed to reconnect to S3", zap.Error(err))
	}
}

func (s *S3Storage) resolveBucketAndPath(bucket, fnm string) (string, string) {
	actualBucket := bucket
	if s.bucket != "" {
		actualBucket = s.bucket
	}

	actualPath := fnm
	if s.prefixPath != "" {
		actualPath = fmt.Sprintf("%s/%s/%s", s.prefixPath, bucket, fnm)
	}

	return actualBucket, actualPath
}

// Health checks S3 service availability
func (s *S3Storage) Health() bool {
	bucket := s.bucket
	if bucket == "" {
		bucket = "health-check-bucket"
	}

	fnm := "txtxtxtxt1"
	if s.prefixPath != "" {
		fnm = fmt.Sprintf("%s/%s", s.prefixPath, fnm)
	}
	binary := []byte("_t@@@1")

	ctx := context.Background()

	// Ensure bucket exists
	if !s.BucketExists(bucket) {
		_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			zap.L().Error("Failed to create bucket for health check", zap.String("bucket", bucket), zap.Error(err))
			return false
		}
	}

	// Try to upload a test object
	reader := bytes.NewReader(binary)
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
		Body:   reader,
	})

	if err != nil {
		zap.L().Error("Health check failed", zap.Error(err))
		return false
	}

	return true
}

// Put uploads an object to S3
func (s *S3Storage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	bucket, fnm = s.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	for i := 0; i < 2; i++ {
		// Ensure bucket exists
		if !s.BucketExists(bucket) {
			_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(bucket),
			})
			if err != nil {
				zap.L().Error("Failed to create bucket", zap.String("bucket", bucket), zap.Error(err))
				s.reconnect()
				time.Sleep(time.Second)
				continue
			}
			zap.L().Info("Created bucket", zap.String("bucket", bucket))
		}

		reader := bytes.NewReader(binary)
		_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
			Body:   reader,
		})
		if err != nil {
			zap.L().Error("Failed to put object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			s.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to put object after retries")
}

// Get retrieves an object from S3
func (s *S3Storage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	bucket, fnm = s.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	for i := 0; i < 2; i++ {
		result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
		})
		if err != nil {
			zap.L().Error("Failed to get object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			s.reconnect()
			time.Sleep(time.Second)
			continue
		}
		defer result.Body.Close()

		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(result.Body); err != nil {
			zap.L().Error("Failed to read object data", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			s.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("failed to get object after retries")
}

// Rm removes an object from S3
func (s *S3Storage) Rm(bucket, fnm string, tenantID ...string) error {
	bucket, fnm = s.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
	})
	if err != nil {
		zap.L().Error("Failed to remove object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return err
	}

	return nil
}

// ObjExist checks if an object exists in S3
func (s *S3Storage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	bucket, fnm = s.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
	})
	if err != nil {
		if isS3NotFound(err) {
			return false
		}
		return false
	}

	return true
}

// GetPresignedURL generates a presigned URL for accessing an object
func (s *S3Storage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	bucket, fnm = s.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	presignClient := s3.NewPresignClient(s.client)

	for i := 0; i < 10; i++ {
		req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
		}, s3.WithPresignExpires(expires))
		if err != nil {
			zap.L().Error("Failed to generate presigned URL", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			s.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return req.URL, nil
	}

	return "", fmt.Errorf("failed to generate presigned URL after 10 retries")
}

// BucketExists checks if a bucket exists
func (s *S3Storage) BucketExists(bucket string) bool {
	actualBucket := bucket
	if s.bucket != "" {
		actualBucket = s.bucket
	}

	ctx := context.Background()

	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(actualBucket),
	})
	if err != nil {
		zap.L().Debug("Bucket does not exist or error", zap.String("bucket", actualBucket), zap.Error(err))
		return false
	}

	return true
}

// RemoveBucket removes a bucket and all its objects
func (s *S3Storage) RemoveBucket(bucket string) error {
	actualBucket := bucket
	if s.bucket != "" {
		actualBucket = s.bucket
	}

	ctx := context.Background()

	// Check if bucket exists
	if !s.BucketExists(actualBucket) {
		return nil
	}

	// List and delete all objects
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(actualBucket),
	}

	for {
		result, err := s.client.ListObjectsV2(ctx, listInput)
		if err != nil {
			zap.L().Error("Failed to list objects", zap.String("bucket", actualBucket), zap.Error(err))
			return err
		}

		for _, obj := range result.Contents {
			_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(actualBucket),
				Key:    obj.Key,
			})
			if err != nil {
				zap.L().Error("Failed to delete object", zap.String("bucket", actualBucket), zap.Error(err))
			}
		}

		if result.IsTruncated == nil || !*result.IsTruncated {
			break
		}
		listInput.ContinuationToken = result.NextContinuationToken
	}

	// Delete bucket
	_, err := s.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(actualBucket),
	})
	if err != nil {
		zap.L().Error("Failed to delete bucket", zap.String("bucket", actualBucket), zap.Error(err))
		return err
	}

	return nil
}

// Copy copies an object from source to destination
func (s *S3Storage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	srcBucket, srcPath = s.resolveBucketAndPath(srcBucket, srcPath)
	destBucket, destPath = s.resolveBucketAndPath(destBucket, destPath)

	ctx := context.Background()

	copySource := fmt.Sprintf("%s/%s", srcBucket, srcPath)

	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(destBucket),
		Key:        aws.String(destPath),
		CopySource: aws.String(copySource),
	})
	if err != nil {
		zap.L().Error("Failed to copy object", zap.String("src", copySource), zap.String("dest", fmt.Sprintf("%s/%s", destBucket, destPath)), zap.Error(err))
		return false
	}

	return true
}

// Move moves an object from source to destination
func (s *S3Storage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if s.Copy(srcBucket, srcPath, destBucket, destPath) {
		if err := s.Rm(srcBucket, srcPath); err != nil {
			zap.L().Error("Failed to remove source object after copy", zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
			return false
		}
		return true
	}
	return false
}

// isNotFound checks if the error is a not found error
func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "404" || apiErr.ErrorCode() == "NoSuchKey"
	}
	return false
}
