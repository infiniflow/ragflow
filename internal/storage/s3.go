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
	"ragflow/internal/common"
	"ragflow/internal/server/config"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3Config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"go.uber.org/zap"
)

// S3Storage implements Storage interface for AWS S3
type S3Storage struct {
	client     *s3.Client
	bucket     string
	prefixPath string
	config     config.S3Config
}

// NewS3Storage creates a new S3 storage instance
func NewS3Storage(ctx context.Context, config config.S3Config) (*S3Storage, error) {
	storage := &S3Storage{
		bucket:     config.Bucket,
		prefixPath: config.PrefixPath,
		config:     config,
	}

	if err := storage.connect(ctx); err != nil {
		return nil, err
	}

	return storage, nil
}

func (s *S3Storage) connect(ctx context.Context) error {

	var opts []func(*s3Config.LoadOptions) error

	// Configure region
	if s.config.Region != "" {
		opts = append(opts, s3Config.WithRegion(s.config.Region))
	}

	// Configure credentials if provided
	if s.config.AccessKey != "" && s.config.SecretKey != "" {
		creds := credentials.NewStaticCredentialsProvider(
			s.config.AccessKey,
			s.config.SecretKey,
			s.config.SessionToken,
		)
		opts = append(opts, s3Config.WithCredentialsProvider(creds))
	}

	// Load configuration
	cfg, err := s3Config.LoadDefaultConfig(ctx, opts...)
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

func (s *S3Storage) reconnect(ctx context.Context) {
	if err := s.connect(ctx); err != nil {
		common.Error("Failed to reconnect to S3", err, zap.Error(err))
	}
}

func (s *S3Storage) resolveBucketAndPath(bucket, fnm string) (string, string) {
	actualBucket := bucket
	if s.bucket != "" {
		actualBucket = s.bucket
	}

	actualPath := fnm
	if s.bucket != "" {
		prefix := s.prefixPath
		if prefix != "" {
			prefix += "/"
		}
		actualPath = fmt.Sprintf("%s%s/%s", prefix, bucket, fnm)
	} else if s.prefixPath != "" {
		actualPath = fmt.Sprintf("%s/%s/%s", s.prefixPath, bucket, fnm)
	}

	return actualBucket, actualPath
}

func (s *S3Storage) Type() string { return "s3" }

// Health checks S3 service availability
func (s *S3Storage) Health(ctx context.Context) bool {
	bucket := s.bucket
	if bucket == "" {
		bucket = "health-check-bucket"
	}

	fnm := "txtxtxtxt1"
	if s.prefixPath != "" {
		fnm = fmt.Sprintf("%s/%s", s.prefixPath, fnm)
	}
	binary := []byte("_t@@@1")

	// Ensure bucket exists
	if !s.BucketExists(ctx, bucket) {
		_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			common.Error("Failed to create bucket for health check", err, zap.String("bucket", bucket), zap.Error(err))
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
		common.Error("Health check failed", err, zap.Error(err))
		return false
	}

	return true
}

// Put uploads an object to S3
func (s *S3Storage) Put(ctx context.Context, bucket, fnm string, binary []byte, tenantID ...string) error {
	bucket, fnm = s.resolveBucketAndPath(bucket, fnm)

	for i := 0; i < 2; i++ {
		// Ensure bucket exists
		if !s.BucketExists(ctx, bucket) {
			_, err := s.client.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(bucket),
			})
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				common.Error("Failed to create bucket", err, zap.String("bucket", bucket), zap.Error(err))
				s.reconnect(ctx)
				if err = sleepOrAbort(ctx, time.Second); err != nil {
					return err
				}
				continue
			}
			common.Info("Created bucket", zap.String("bucket", bucket))
		}

		reader := bytes.NewReader(binary)
		_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
			Body:   reader,
		})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			common.Error("Failed to put object", err, zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			s.reconnect(ctx)
			if err = sleepOrAbort(ctx, time.Second); err != nil {
				return err
			}
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to put object after retries")
}

// Get retrieves an object from S3
func (s *S3Storage) Get(ctx context.Context, bucket, fnm string, tenantID ...string) ([]byte, error) {
	bucket, fnm = s.resolveBucketAndPath(bucket, fnm)

	for i := 0; i < 2; i++ {
		result, err := s.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
		})
		if err != nil {
			common.Error("Failed to get object", err, zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			s.reconnect(ctx)
			time.Sleep(time.Second)
			continue
		}
		buf := new(bytes.Buffer)

		readErr := func() error {
			defer result.Body.Close()
			_, err = buf.ReadFrom(result.Body)
			return err
		}()
		if readErr != nil {
			common.Error("Failed to read object data", readErr, zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(readErr))
			s.reconnect(ctx)
			time.Sleep(time.Second)
			continue
		}

		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("failed to get object after retries")
}

// Remove removes an object from S3
func (s *S3Storage) Remove(ctx context.Context, bucket, fnm string, tenantID ...string) error {
	bucket, fnm = s.resolveBucketAndPath(bucket, fnm)

	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
	})
	if err != nil {
		common.Error("Failed to remove object", err, zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return err
	}

	return nil
}

// ObjExist checks if an object exists in S3
func (s *S3Storage) ObjExist(ctx context.Context, bucket, fnm string, tenantID ...string) bool {
	bucket, fnm = s.resolveBucketAndPath(bucket, fnm)

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

func (s *S3Storage) ListObjects(ctx context.Context, bucket string, tenantID ...string) ([]string, error) {

	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	result, err := s.client.ListObjectsV2(ctx, listInput)
	if err != nil {
		return nil, err
	}

	var objects []string

	for _, obj := range result.Contents {
		objects = append(objects, *obj.Key)
	}

	return objects, nil
}

// GetPresignedURL generates a presigned URL for accessing an object
func (s *S3Storage) GetPresignedURL(ctx context.Context, bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	bucket, fnm = s.resolveBucketAndPath(bucket, fnm)

	presignClient := s3.NewPresignClient(s.client)

	for i := 0; i < 10; i++ {
		req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
		}, s3.WithPresignExpires(expires))
		if err != nil {
			common.Error("Failed to generate presigned URL", err, zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			s.reconnect(ctx)
			time.Sleep(time.Second)
			continue
		}

		return req.URL, nil
	}

	return "", fmt.Errorf("failed to generate presigned URL after 10 retries")
}

// BucketExists checks if a bucket exists
func (s *S3Storage) BucketExists(ctx context.Context, bucket string) bool {
	actualBucket := bucket
	if s.bucket != "" {
		actualBucket = s.bucket
	}

	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(actualBucket),
	})
	if err != nil {
		common.Debug("Bucket does not exist or error", zap.String("bucket", actualBucket), zap.Error(err))
		return false
	}

	return true
}

// RemoveBucket removes a bucket and all its objects
func (s *S3Storage) RemoveBucket(ctx context.Context, bucket string) error {
	actualBucket, prefix := s.resolveBucketAndPrefix(bucket)
	exists, err := s.bucketExistsForRemoval(ctx, actualBucket)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}

	if err := s.removeObjects(ctx, actualBucket, prefix); err != nil {
		return err
	}
	if s.bucket != "" {
		return nil
	}

	_, err = s.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(actualBucket),
	})
	if err != nil {
		common.Error("Failed to delete bucket", err, zap.String("bucket", bucket), zap.Error(err))
		return err
	}

	return nil
}

func (s *S3Storage) resolveBucketAndPrefix(bucket string) (string, string) {
	if s.bucket == "" {
		return bucket, ""
	}
	prefix := s.prefixPath
	if prefix != "" {
		prefix += "/"
	}
	return s.bucket, prefix + bucket + "/"
}

func (s *S3Storage) bucketExistsForRemoval(ctx context.Context, bucket string) (bool, error) {
	_, err := s.client.HeadBucket(ctx, &s3.HeadBucketInput{Bucket: aws.String(bucket)})
	if err == nil {
		return true, nil
	}
	if isS3BucketNotFound(err) {
		return false, nil
	}
	return false, fmt.Errorf("head S3 bucket %q: %w", bucket, err)
}

func (s *S3Storage) removeObjects(ctx context.Context, bucket, prefix string) error {
	versions := s3.NewListObjectVersionsPaginator(s.client, &s3.ListObjectVersionsInput{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for versions.HasMorePages() {
		page, err := versions.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list S3 object versions in %q: %w", bucket, err)
		}
		toDelete := make([]types.ObjectIdentifier, 0, len(page.Versions)+len(page.DeleteMarkers))
		for _, version := range page.Versions {
			toDelete = append(toDelete, types.ObjectIdentifier{Key: version.Key, VersionId: version.VersionId})
		}
		for _, marker := range page.DeleteMarkers {
			toDelete = append(toDelete, types.ObjectIdentifier{Key: marker.Key, VersionId: marker.VersionId})
		}
		if err := s.deleteObjects(ctx, bucket, toDelete); err != nil {
			return err
		}
	}
	return nil
}

func (s *S3Storage) deleteObjects(ctx context.Context, bucket string, objects []types.ObjectIdentifier) error {
	for len(objects) > 0 {
		count := min(len(objects), 1000)
		output, err := s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{Objects: objects[:count], Quiet: aws.Bool(true)},
		})
		if err != nil {
			return fmt.Errorf("delete S3 objects in %q: %w", bucket, err)
		}
		if len(output.Errors) > 0 {
			failed := output.Errors[0]
			return fmt.Errorf("delete S3 object %q (version %q): %s: %s", aws.ToString(failed.Key), aws.ToString(failed.VersionId), aws.ToString(failed.Code), aws.ToString(failed.Message))
		}
		objects = objects[count:]
	}
	return nil
}

// Copy copies an object from source to destination
func (s *S3Storage) Copy(ctx context.Context, srcBucket, srcPath, destBucket, destPath string) bool {
	srcBucket, srcPath = s.resolveBucketAndPath(srcBucket, srcPath)
	destBucket, destPath = s.resolveBucketAndPath(destBucket, destPath)

	copySource := fmt.Sprintf("%s/%s", srcBucket, srcPath)

	_, err := s.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(destBucket),
		Key:        aws.String(destPath),
		CopySource: aws.String(copySource),
	})
	if err != nil {
		common.Error("Failed to copy object", err, zap.String("src", copySource), zap.String("dest", fmt.Sprintf("%s/%s", destBucket, destPath)), zap.Error(err))
		return false
	}

	return true
}

// Move moves an object from source to destination
func (s *S3Storage) Move(ctx context.Context, srcBucket, srcPath, destBucket, destPath string) bool {
	if s.Copy(ctx, srcBucket, srcPath, destBucket, destPath) {
		if err := s.Remove(ctx, srcBucket, srcPath); err != nil {
			common.Error("Failed to remove source object after copy", err, zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
			rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			err = s.Remove(rollbackCtx, destBucket, destPath)
			if err != nil {
				common.Warn("Failed to roll back copied destination object", zap.String("bucket", destBucket), zap.String("key", destPath), zap.Error(err))
			}
			return false
		}
		return true
	}
	return false
}

func (s *S3Storage) Close() error { return nil }

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

func isS3BucketNotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "404" || apiErr.ErrorCode() == "NoSuchBucket"
	}
	return false
}
