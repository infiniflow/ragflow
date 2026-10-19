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
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/server/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	s3Config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"go.uber.org/zap"
)

// S3Storage implements Storage for AWS S3. The shared S3-compatible surface
// lives in s3Base; this file holds only what differs for AWS: connection
// options (signature version, addressing style), the region-aware bucket
// factory, the path-folding semantics, and the versioned bucket removal.
type S3Storage struct {
	s3Base
	client *s3.Client // owned by connect/reconnect; the base reads it via its getter
	config config.S3Config
}

// NewS3Storage creates a new S3 storage instance
func NewS3Storage(ctx context.Context, config config.S3Config) (*S3Storage, error) {
	storage := &S3Storage{
		config: config,
	}

	if err := storage.connect(ctx); err != nil {
		return nil, err
	}

	storage.s3Base = s3Base{
		client:     func() *s3.Client { return storage.client },
		bucket:     config.Bucket,
		prefixPath: config.PrefixPath,
		typeName:   "s3",
		resolve:    storage.resolveBucketAndPath,
		makeBucket: storage.createBucket,
		reconnect:  storage.reconnect,
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

	s.client = s3.NewFromConfig(cfg, func(o *s3.Options) {
		if s.config.EndpointURL != "" {
			o.BaseEndpoint = aws.String(s.config.EndpointURL)
		}
		o.UsePathStyle = s.config.AddressingStyle == "path"
		if s.config.SignatureVersion == "s3" {
			o.RequestChecksumCalculation = aws.RequestChecksumCalculationWhenRequired
			o.APIOptions = append(o.APIOptions, v4.SwapComputePayloadSHA256ForUnsignedPayloadMiddleware)
		}
	})
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

func (s *S3Storage) createBucket(ctx context.Context, bucket string) error {
	input := &s3.CreateBucketInput{Bucket: aws.String(bucket)}
	region := s.client.Options().Region
	if region != "" && region != "us-east-1" && region != "auto" {
		input.CreateBucketConfiguration = &types.CreateBucketConfiguration{
			LocationConstraint: types.BucketLocationConstraint(region),
		}
	}
	_, err := s.client.CreateBucket(ctx, input)
	return err
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

// RemoveEmptyBucket removes a bucket only when it contains no object versions.
func (s *S3Storage) RemoveEmptyBucket(ctx context.Context, bucket string) error {
	actualBucket, prefix := s.resolveBucketAndPrefix(bucket)
	exists, err := s.bucketExistsForRemoval(ctx, actualBucket)
	if err != nil || !exists {
		return err
	}
	if s.bucket != "" {
		versions, err := s.client.ListObjectVersions(ctx, &s3.ListObjectVersionsInput{
			Bucket: aws.String(actualBucket), Prefix: aws.String(prefix), MaxKeys: aws.Int32(1),
		})
		if err != nil {
			return err
		}
		if len(versions.Versions) > 0 || len(versions.DeleteMarkers) > 0 || (versions.IsTruncated != nil && *versions.IsTruncated) {
			return fmt.Errorf("bucket %s is not empty", bucket)
		}
		return nil
	}
	_, err = s.client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(actualBucket)})
	return err
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

// isNotFound checks if the error is a not found error
func isS3NotFound(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		return apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "404" || apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NoSuchBucket"
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
