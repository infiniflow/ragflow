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
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/server/config"

	"github.com/aws/aws-sdk-go-v2/aws"
	s3Config "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

// OSSStorage implements Storage for Aliyun OSS. The shared S3-compatible
// surface lives in s3Base; this file holds only what differs for OSS: the
// connection setup, the path-folding semantics (no logical-bucket segment —
// see doc.go), and the refusal to remove logical buckets in shared-bucket
// mode.
type OSSStorage struct {
	s3Base
	client *s3.Client // owned by connect/reconnect; the base reads it via its getter
	config config.OSSConfig
}

// NewOSSStorage creates a new OSS storage instance
func NewOSSStorage(ctx context.Context, config config.OSSConfig) (*OSSStorage, error) {
	storage := &OSSStorage{
		config: config,
	}

	if err := storage.connect(ctx); err != nil {
		return nil, err
	}

	storage.s3Base = s3Base{
		client:     func() *s3.Client { return storage.client },
		bucket:     config.Bucket,
		prefixPath: config.PrefixPath,
		typeName:   "oss",
		resolve:    storage.resolveBucketAndPath,
		makeBucket: storage.createBucket,
		reconnect:  storage.reconnect,
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

func (o *OSSStorage) createBucket(ctx context.Context, bucket string) error {
	_, err := o.client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})
	return err
}

// RemoveBucket removes a bucket and all its objects. In shared-bucket mode a
// logical bucket has no physical isolation, so removal is refused rather than
// silently clearing the shared prefix.
func (o *OSSStorage) RemoveBucket(ctx context.Context, bucket string) error {
	if o.bucket != "" {
		return fmt.Errorf("cannot remove logical OSS bucket %s: shared bucket mode does not isolate objects by logical bucket", bucket)
	}

	actualBucket := bucket

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
			if _, err = o.client.DeleteObject(ctx, &s3.DeleteObjectInput{
				Bucket: aws.String(actualBucket),
				Key:    obj.Key,
			}); err != nil {
				common.Error("Failed to delete object", err, zap.String("bucket", actualBucket))
			}
		}

		if result.IsTruncated == nil || !*result.IsTruncated {
			break
		}
		listInput.ContinuationToken = result.NextContinuationToken
	}

	// Delete bucket
	if _, err := o.client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(actualBucket),
	}); err != nil {
		common.Error("Failed to delete bucket", err, zap.String("bucket", actualBucket))
		return err
	}

	return nil
}

// RemoveEmptyBucket removes a physical bucket only when it is empty. In
// shared-bucket mode a logical bucket cannot be removed at all.
func (o *OSSStorage) RemoveEmptyBucket(ctx context.Context, bucket string) error {
	if o.bucket != "" {
		return fmt.Errorf("cannot remove logical OSS bucket %s: shared bucket mode does not isolate objects by logical bucket", bucket)
	}
	_, err := o.client.DeleteBucket(ctx, &s3.DeleteBucketInput{Bucket: aws.String(bucket)})
	if isS3NotFound(err) {
		return nil
	}
	return err
}
