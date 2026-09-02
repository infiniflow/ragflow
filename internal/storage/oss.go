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
	"github.com/aws/smithy-go"
	"go.uber.org/zap"
)

// OSSStorage implements Storage interface for Aliyun OSS
// OSS uses S3-compatible API
type OSSStorage struct {
	client     *s3.Client
	bucket     string
	prefixPath string
	config     config.OSSConfig
}

// NewOSSStorage creates a new OSS storage instance
func NewOSSStorage(ctx context.Context, config config.OSSConfig) (*OSSStorage, error) {
	storage := &OSSStorage{
		bucket:     config.Bucket,
		prefixPath: config.PrefixPath,
		config:     config,
	}

	if err := storage.connect(ctx); err != nil {
		return nil, err
	}

	return storage, nil
}

func (o *OSSStorage) connect(ctx context.Context) error {

	// Create static credentials
	creds := credentials.NewStaticCredentialsProvider(
		o.config.AccessKey,
		o.config.SecretKey,
		"",
	)

	// Load configuration
	cfg, err := s3Config.LoadDefaultConfig(ctx,
		s3Config.WithRegion(o.config.Region),
		s3Config.WithCredentialsProvider(creds),
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

func (o *OSSStorage) reconnect(ctx context.Context) {
	if err := o.connect(ctx); err != nil {
		common.Error("Failed to reconnect to OSS", err)
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

func (o *OSSStorage) Type() string { return "oss" }

// Health checks OSS service availability
func (o *OSSStorage) Health(ctx context.Context) bool {
	bucket := o.bucket
	if bucket == "" {
		bucket = "health-check-bucket"
	}

	fnm := "txtxtxtxt1"
	if o.prefixPath != "" {
		fnm = fmt.Sprintf("%s/%s", o.prefixPath, fnm)
	}
	binary := []byte("_t@@@1")

	// Ensure bucket exists
	if !o.BucketExists(ctx, bucket) {
		_, err := o.client.CreateBucket(ctx, &s3.CreateBucketInput{
			Bucket: aws.String(bucket),
		})
		if err != nil {
			common.Error("Failed to create bucket for health check", err, zap.String("bucket", bucket))
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
		common.Error("Health check failed", err)
		return false
	}

	return true
}

// Put uploads an object to OSS
func (o *OSSStorage) Put(ctx context.Context, bucket, fnm string, binary []byte, tenantID ...string) error {
	bucket, fnm = o.resolveBucketAndPath(bucket, fnm)

	for i := 0; i < 2; i++ {
		// Ensure bucket exists
		if !o.BucketExists(ctx, bucket) {
			_, err := o.client.CreateBucket(ctx, &s3.CreateBucketInput{
				Bucket: aws.String(bucket),
			})
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					return ctxErr
				}
				common.Error("Failed to create bucket", err, zap.String("bucket", bucket))
				o.reconnect(ctx)
				if err = sleepOrAbort(ctx, time.Second); err != nil {
					return err
				}
				continue
			}
			common.Info("Created bucket", zap.String("bucket", bucket))
		}

		reader := bytes.NewReader(binary)
		_, err := o.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
			Body:   reader,
		})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return ctxErr
			}
			common.Error("Failed to put object", err, zap.String("bucket", bucket), zap.String("key", fnm))
			o.reconnect(ctx)
			if err = sleepOrAbort(ctx, time.Second); err != nil {
				return err
			}
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to put object after retries")
}

// Get retrieves an object from OSS
func (o *OSSStorage) Get(ctx context.Context, bucket, fnm string, tenantID ...string) ([]byte, error) {
	bucket, fnm = o.resolveBucketAndPath(bucket, fnm)

	for i := 0; i < 2; i++ {
		result, err := o.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
		})
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			common.Error("Failed to get object", err, zap.String("bucket", bucket), zap.String("key", fnm))
			o.reconnect(ctx)
			if err = sleepOrAbort(ctx, time.Second); err != nil {
				return nil, err
			}
			continue
		}
		buf := new(bytes.Buffer)

		readErr := func() error {
			defer result.Body.Close()
			_, err = buf.ReadFrom(result.Body)
			return err
		}()
		if readErr != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, ctxErr
			}
			common.Error("Failed to read object data", readErr, zap.String("bucket", bucket), zap.String("key", fnm))
			o.reconnect(ctx)
			if err = sleepOrAbort(ctx, time.Second); err != nil {
				return nil, err
			}
			continue
		}

		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("failed to get object after retries")
}

// Remove removes an object from OSS
func (o *OSSStorage) Remove(ctx context.Context, bucket, fnm string, tenantID ...string) error {
	bucket, fnm = o.resolveBucketAndPath(bucket, fnm)

	_, err := o.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
	})
	if err != nil {
		common.Error("Failed to remove object", err, zap.String("bucket", bucket), zap.String("key", fnm))
		return err
	}

	return nil
}

// ObjExist checks if an object exists in OSS
func (o *OSSStorage) ObjExist(ctx context.Context, bucket, fnm string, tenantID ...string) bool {
	bucket, fnm = o.resolveBucketAndPath(bucket, fnm)

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

func (o *OSSStorage) ListObjects(ctx context.Context, bucket string, tenantID ...string) ([]string, error) {

	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	}

	result, err := o.client.ListObjectsV2(ctx, listInput)
	if err != nil {
		return nil, err
	}

	var objects []string

	for _, obj := range result.Contents {
		objects = append(objects, *obj.Key)
	}

	return objects, nil
}

func (o *OSSStorage) GetPresignedURL(ctx context.Context, bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	bucket, fnm = o.resolveBucketAndPath(bucket, fnm)

	presignClient := s3.NewPresignClient(o.client)

	for i := 0; i < 10; i++ {
		req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
		}, s3.WithPresignExpires(expires))
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return "", ctxErr
			}
			common.Error("Failed to generate presigned URL", err, zap.String("bucket", bucket), zap.String("key", fnm))
			o.reconnect(ctx)
			if err = sleepOrAbort(ctx, time.Second); err != nil {
				return "", err
			}
			continue
		}

		return req.URL, nil
	}

	return "", fmt.Errorf("failed to generate presigned URL after 10 retries")
}

// BucketExists checks if a bucket exists
func (o *OSSStorage) BucketExists(ctx context.Context, bucket string) bool {
	actualBucket := bucket
	if o.bucket != "" {
		actualBucket = o.bucket
	}

	_, err := o.client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(actualBucket),
	})
	if err != nil {
		common.Error("Bucket does not exist or error", err, zap.String("bucket", actualBucket))
		return false
	}

	return true
}

// RemoveBucket removes a bucket and all its objects
func (o *OSSStorage) RemoveBucket(ctx context.Context, bucket string) error {
	actualBucket := bucket
	if o.bucket != "" {
		actualBucket = o.bucket
	}

	// Check if bucket exists
	if !o.BucketExists(ctx, actualBucket) {
		return nil
	}

	// List and delete all objects
	listInput := &s3.ListObjectsV2Input{
		Bucket: aws.String(actualBucket),
	}

	for {
		result, err := o.client.ListObjectsV2(ctx, listInput)
		if err != nil {
			common.Error("Failed to list objects", err, zap.String("bucket", actualBucket))
			return err
		}

		for _, obj := range result.Contents {
			_, err = o.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(actualBucket),
				Key:    obj.Key,
			})
			if err != nil {
				common.Error("Failed to delete object", err, zap.String("bucket", actualBucket))
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
		common.Error("Failed to delete bucket", err, zap.String("bucket", actualBucket))
		return err
	}

	return nil
}

// Copy copies an object from source to destination
func (o *OSSStorage) Copy(ctx context.Context, srcBucket, srcPath, destBucket, destPath string) bool {
	srcBucket, srcPath = o.resolveBucketAndPath(srcBucket, srcPath)
	destBucket, destPath = o.resolveBucketAndPath(destBucket, destPath)

	copySource := fmt.Sprintf("%s/%s", srcBucket, srcPath)

	_, err := o.client.CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(destBucket),
		Key:        aws.String(destPath),
		CopySource: aws.String(copySource),
	})
	if err != nil {
		common.Error("Failed to copy object", err, zap.String("src", copySource), zap.String("dest", fmt.Sprintf("%s/%s", destBucket, destPath)))
		return false
	}

	return true
}

// Move moves an object from source to destination
func (o *OSSStorage) Move(ctx context.Context, srcBucket, srcPath, destBucket, destPath string) bool {
	if o.Copy(ctx, srcBucket, srcPath, destBucket, destPath) {
		if err := o.Remove(ctx, srcBucket, srcPath); err != nil {
			common.Error("Failed to remove source object after copy", err, zap.String("bucket", srcBucket), zap.String("key", srcPath))
			rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			err = o.Remove(rollbackCtx, destBucket, destPath)
			if err != nil {
				common.Warn("Failed to roll back copied destination object", zap.String("bucket", destBucket), zap.String("key", destPath), zap.Error(err))
			}
			return false
		}
		return true
	}
	return false
}

func (o *OSSStorage) Close() error { return nil }

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
