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
	"fmt"
	"time"

	"ragflow/internal/common"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go/encoding/httpbinding"
	"go.uber.org/zap"
)

// s3Base is the shared implementation for S3-compatible backends built on
// aws-sdk-go-v2 (AWS S3 and Aliyun OSS). A backend embeds it and injects the
// few pieces that genuinely differ between vendors:
//
//   - client:     a getter returning the CURRENT client, so reconnect swaps
//     are visible to shared methods without stale handles;
//   - resolve:    the bucket/key folding semantics (see doc.go for the
//     deliberately non-uniform per-vendor rules);
//   - makeBucket: bucket creation (AWS pins a LocationConstraint per region,
//     OSS does not);
//   - reconnect:  rebuild the client after a failed attempt;
//   - typeName:   the Type() identifier.
//
// Methods that differ structurally between vendors — connect, RemoveBucket,
// RemoveEmptyBucket and the versioned-object removal helpers — stay on the
// concrete backend and override anything the base would otherwise provide.
type s3Base struct {
	client     func() *s3.Client
	bucket     string
	prefixPath string
	typeName   string
	resolve    func(bucket, fnm string) (string, string)
	makeBucket func(ctx context.Context, bucket string) error
	reconnect  func(ctx context.Context)
}

func (b *s3Base) Type() string { return b.typeName }

// Close is a no-op: the aws-sdk client holds no resources to release.
func (b *s3Base) Close() error { return nil }

// Health checks service availability by uploading a probe object.
func (b *s3Base) Health(ctx context.Context) bool {
	bucket := b.bucket
	if bucket == "" {
		bucket = "health-check-bucket"
	}

	fnm := "txtxtxtxt1"
	if b.prefixPath != "" {
		fnm = fmt.Sprintf("%s/%s", b.prefixPath, fnm)
	}
	binary := []byte("_t@@@1")

	// Ensure bucket exists
	if !b.BucketExists(ctx, bucket) {
		if err := b.makeBucket(ctx, bucket); err != nil {
			common.Error("Failed to create bucket for health check", err, zap.String("bucket", bucket), zap.Error(err))
			return false
		}
	}

	// Try to upload a test object
	reader := bytes.NewReader(binary)
	if _, err := b.client().PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
		Body:   reader,
	}); err != nil {
		common.Error("Health check failed", err, zap.Error(err))
		return false
	}

	return true
}

// Put uploads an object, creating the bucket on first write.
func (b *s3Base) Put(ctx context.Context, bucket, fnm string, binary []byte, tenantID ...string) error {
	bucket, fnm = b.resolve(bucket, fnm)

	return retryObjectOp(ctx, 2, func() { b.reconnect(ctx) }, func() error {
		// Ensure bucket exists
		if !b.BucketExists(ctx, bucket) {
			if err := b.makeBucket(ctx, bucket); err != nil {
				common.Error("Failed to create bucket", err, zap.String("bucket", bucket), zap.Error(err))
				return err
			}
			common.Info("Created bucket", zap.String("bucket", bucket))
		}

		reader := bytes.NewReader(binary)
		if _, err := b.client().PutObject(ctx, &s3.PutObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
			Body:   reader,
		}); err != nil {
			common.Error("Failed to put object", err, zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			return err
		}

		return nil
	})
}

// Get retrieves an object. Cancellation surfaces promptly per the contract
// pinned by s3_cancel_test.go (retryObjectOp checks ctx before each attempt
// and after each op error, winning over the backoff sleep).
func (b *s3Base) Get(ctx context.Context, bucket, fnm string, tenantID ...string) ([]byte, error) {
	bucket, fnm = b.resolve(bucket, fnm)

	var data []byte
	err := retryObjectOp(ctx, 2, func() { b.reconnect(ctx) }, func() error {
		result, err := b.client().GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
		})
		if err != nil {
			common.Error("Failed to get object", err, zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			return err
		}
		buf := new(bytes.Buffer)

		readErr := func() error {
			defer result.Body.Close()
			_, err = buf.ReadFrom(result.Body)
			return err
		}()
		if readErr != nil {
			common.Error("Failed to read object data", readErr, zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(readErr))
			return readErr
		}
		data = buf.Bytes()

		return nil
	})
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Remove removes an object; a missing object/bucket is not an error.
func (b *s3Base) Remove(ctx context.Context, bucket, fnm string, tenantID ...string) error {
	bucket, fnm = b.resolve(bucket, fnm)

	if _, err := b.client().DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
	}); err != nil {
		if isS3NotFound(err) {
			return nil
		}
		common.Error("Failed to remove object", err, zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return err
	}

	return nil
}

// ObjExist checks if an object exists.
func (b *s3Base) ObjExist(ctx context.Context, bucket, fnm string, tenantID ...string) bool {
	bucket, fnm = b.resolve(bucket, fnm)

	if _, err := b.client().HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
	}); err != nil {
		return false
	}

	return true
}

// ObjectExists distinguishes a missing object from a storage error.
func (b *s3Base) ObjectExists(ctx context.Context, bucket, fnm string) (bool, error) {
	bucket, fnm = b.resolve(bucket, fnm)

	if _, err := b.client().HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(fnm),
	}); err != nil {
		if isS3NotFound(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// ListObjects lists the keys in a bucket.
func (b *s3Base) ListObjects(ctx context.Context, bucket string, tenantID ...string) ([]string, error) {
	result, err := b.client().ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
	})
	if err != nil {
		return nil, err
	}

	var objects []string
	for _, obj := range result.Contents {
		objects = append(objects, *obj.Key)
	}

	return objects, nil
}

// GetPresignedURL generates a presigned URL for accessing an object.
func (b *s3Base) GetPresignedURL(ctx context.Context, bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	bucket, fnm = b.resolve(bucket, fnm)

	presignClient := s3.NewPresignClient(b.client())

	var url string
	err := retryObjectOp(ctx, 10, func() { b.reconnect(ctx) }, func() error {
		req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(bucket),
			Key:    aws.String(fnm),
		}, s3.WithPresignExpires(expires))
		if err != nil {
			common.Error("Failed to generate presigned URL", err, zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			return err
		}
		url = req.URL

		return nil
	})
	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL after 10 retries: %w", err)
	}

	return url, nil
}

// BucketExists checks if a bucket exists.
func (b *s3Base) BucketExists(ctx context.Context, bucket string) bool {
	if b.bucket != "" {
		bucket = b.bucket
	}

	if _, err := b.client().HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}); err != nil {
		common.Debug("Bucket does not exist or error", zap.String("bucket", bucket), zap.Error(err))
		return false
	}

	return true
}

// BucketExistsWithError distinguishes a missing bucket from a storage error.
func (b *s3Base) BucketExistsWithError(ctx context.Context, bucket string) (bool, error) {
	if b.bucket != "" {
		bucket = b.bucket
	}

	if _, err := b.client().HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	}); err != nil {
		if isS3BucketNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("head S3 bucket %q: %w", bucket, err)
	}

	return true, nil
}

// Copy copies an object from source to destination.
func (b *s3Base) Copy(ctx context.Context, srcBucket, srcPath, destBucket, destPath string) bool {
	srcBucket, srcPath = b.resolve(srcBucket, srcPath)
	destBucket, destPath = b.resolve(destBucket, destPath)

	copySource := httpbinding.EscapePath(fmt.Sprintf("%s/%s", srcBucket, srcPath), false)

	if _, err := b.client().CopyObject(ctx, &s3.CopyObjectInput{
		Bucket:     aws.String(destBucket),
		Key:        aws.String(destPath),
		CopySource: aws.String(copySource),
	}); err != nil {
		common.Error("Failed to copy object", err, zap.String("src", copySource), zap.String("dest", fmt.Sprintf("%s/%s", destBucket, destPath)), zap.Error(err))
		return false
	}

	return true
}

// Move moves an object from source to destination, rolling back the
// destination if the source removal fails.
func (b *s3Base) Move(ctx context.Context, srcBucket, srcPath, destBucket, destPath string) bool {
	return moveWithRollback(ctx, srcBucket, srcPath, destBucket, destPath,
		func() bool { return b.Copy(ctx, srcBucket, srcPath, destBucket, destPath) },
		func(rctx context.Context) error { return b.Remove(rctx, srcBucket, srcPath) },
		func(rctx context.Context) error { return b.Remove(rctx, destBucket, destPath) },
	)
}
