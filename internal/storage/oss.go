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

// OSSStorage implements Storage interface for Aliyun OSS
// OSS uses S3-compatible API
type OSSStorage struct {
	client     *s3.Client
	bucket     string
	prefixPath string
	config     *OSSConfig
}

// NewOSSStorage creates a new OSS storage instance
func NewOSSStorage(config *OSSConfig) (*OSSStorage, error) {
	storage := &OSSStorage{
		bucket:     config.Bucket,
		prefixPath: config.PrefixPath,
		config:     config,
	}

	if err := storage.connect(); err != nil {
		return nil, err
	}

	return storage, nil
}

func (o *OSSStorage) connect() error {
	ctx := context.Background()

	// Create static credentials
	creds := credentials.NewStaticCredentialsProvider(
		o.config.AccessKeyID,
		o.config.SecretAccessKey,
		"",
	)

	// Load configuration
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(o.config.Region),
		config.WithCredentialsProvider(creds),
	)
	if err != nil {
		return fmt.Errorf("failed to load OSS config: %w", err)
	}

	// Create S3 client with OSS endpoint
	o.client = s3.NewFromConfig(cfg, func(opts *s3.Options) {
		opts.BaseEndpoint = aws.String(o.config.EndpointURL)
	})

	return nil
}

func (o *OSSStorage) reconnect() {
	if err := o.connect(); err != nil {
		zap.L().Error("Failed to reconnect to OSS", zap.Error(err))
	}
}

func (o *OSSStorage) resolveBucketAndPath(bucket, fnm string) (string, string) {
	actualBucket := bucket
	if o.bucket != "" {
		actualBucket = o.bucket
	}

	actualPath := fnm
	if o.prefixPath != "" {
		actualPath = fmt.Sprintf("%s/%s", o.prefixPath, fnm)
	}

	return actualBucket, actualPath
}

// Health checks OSS service availability
func (o *OSSStorage) Health() bool {
	bucket := o.bucket
	if bucket == "" {
		bucket = "health-check-bucket"
	}

	fnm := "txtxtxtxt1"
	if o.prefixPath != "" {
		fnm = fmt.Sprintf("%s/%s", o.prefixPath, fnm)
	}
	binary := []byte("_t@@@1")

	ctx := context.Background()

	// Ensure bucket exists
	if !o.BucketExists(bucket) {
		_, err := o.client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			zap.L().Error("Failed to create bucket for health check", zap.String("bucket", bucket), zap.Error(err))
			return false
		}
	}

	// Try to upload a test object
	reader := bytes.NewReader(binary)
	_, err := o.client.PutObject(ctx, &s3.PutObjectInput{
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

// Put uploads an object to OSS
func (o *OSSStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	bucket, fnm = o.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	for i := 0; i < 2; i++ {
		// Ensure bucket exists
		if !o.BucketExists(bucket) {
			_, err := o.client.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(bucket),
			})
			if err != nil {
				zap.L().Error("Failed to create bucket", zap.String("bucket", bucket), zap.Error(err))
				o.reconnect()
				time.Sleep(time.Second)
				continue
			}
			zap.L().Info("Created bucket", zap.String("bucket", bucket))
		}

		reader := bytes.NewReader(binary)
		_, err := o.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
			Body:   reader,
		})
		if err != nil {
			zap.L().Error("Failed to put object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			o.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to put object after retries")
}

// Get retrieves an object from OSS
func (o *OSSStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	bucket, fnm = o.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	for i := 0; i < 2; i++ {
		result, err := o.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
		})
		if err != nil {
			zap.L().Error("Failed to get object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			o.reconnect()
			time.Sleep(time.Second)
			continue
		}
		defer result.Body.Close()

		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(result.Body); err != nil {
			zap.L().Error("Failed to read object data", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			o.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("failed to get object after retries")
}

// Rm removes an object from OSS
func (o *OSSStorage) Rm(bucket, fnm string, tenantID ...string) error {
	bucket, fnm = o.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	_, err := o.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
	})
	if err != nil {
		zap.L().Error("Failed to remove object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return err
	}

	return nil
}

// ObjExist checks if an object exists in OSS
func (o *OSSStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	bucket, fnm = o.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	_, err := o.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
	})
	if err != nil {
		if isOSSNotFound(err) {
			return false
		}
		return false
	}

	return true
}

// GetPresignedURL generates a presigned URL for accessing an object
func (o *OSSStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	bucket, fnm = o.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	presignClient := s3.NewPresignClient(o.client)

	for i := 0; i < 10; i++ {
		req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
		}, s3.WithPresignExpires(expires))
		if err != nil {
			zap.L().Error("Failed to generate presigned URL", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			o.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return req.URL, nil
	}

	return "", fmt.Errorf("failed to generate presigned URL after 10 retries")
}

// BucketExists checks if a bucket exists
func (o *OSSStorage) BucketExists(bucket string) bool {
	actualBucket := bucket
	if o.bucket != "" {
		actualBucket = o.bucket
	}

	ctx := context.Background()

	_, err := o.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(actualBucket),
	})
	if err != nil {
		zap.L().Debug("Bucket does not exist or error", zap.String("bucket", actualBucket), zap.Error(err))
		return false
	}

	return true
}

// RemoveBucket removes a bucket and all its objects
func (o *OSSStorage) RemoveBucket(bucket string) error {
	actualBucket := bucket
	if o.bucket != "" {
		actualBucket = o.bucket
	}

	ctx := context.Background()

	// Check if bucket exists
	if !o.BucketExists(actualBucket) {
		return nil
	}

	// List and delete all objects
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(actualBucket),
	}

	for {
		result, err := o.client.ListObjectsV2(ctx, listInput)
		if err != nil {
			zap.L().Error("Failed to list objects", zap.String("bucket", actualBucket), zap.Error(err))
			return err
		}

		for _, obj := range result.Contents {
			_, err := o.client.DeleteObject(ctx, &s3.DeleteObjectInput{
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
	_, err := o.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(actualBucket),
	})
	if err != nil {
		zap.L().Error("Failed to delete bucket", zap.String("bucket", actualBucket), zap.Error(err))
		return err
	}

	return nil
}

// Copy copies an object from source to destination
func (o *OSSStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	srcBucket, srcPath = o.resolveBucketAndPath(srcBucket, srcPath)
	destBucket, destPath = o.resolveBucketAndPath(destBucket, destPath)

	ctx := context.Background()

	copySource := fmt.Sprintf("%s/%s", srcBucket, srcPath)

	_, err := o.client.CopyObject(ctx, &s3.CopyObjectInput{
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
func (o *OSSStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if o.Copy(srcBucket, srcPath, destBucket, destPath) {
		if err := o.Rm(srcBucket, srcPath); err != nil {
			zap.L().Error("Failed to remove source object after copy", zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
			return false
		}
		return true
	}
	return false
}

// Helper functions
func isOSSNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "404" || apiErr.ErrorCode() == "NoSuchKey"
	}
	return false
}
