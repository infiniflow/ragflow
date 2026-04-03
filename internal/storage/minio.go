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
	"crypto/tls"
	"fmt"
	"net/http"
	"ragflow/internal/server"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

// MinioStorage implements Storage interface for MinIO
type MinioStorage struct {
	client     *minio.Client
	bucket     string
	prefixPath string
	config     *server.MinioConfig
}

// NewMinioStorage creates a new MinIO storage instance
func NewMinioStorage(config *server.MinioConfig) (*MinioStorage, error) {
	storage := &MinioStorage{
		bucket:     config.Bucket,
		prefixPath: config.PrefixPath,
		config:     config,
	}

	if err := storage.connect(); err != nil {
		return nil, err
	}

	return storage, nil
}

func (m *MinioStorage) connect() error {
	var transport http.RoundTripper

	// Configure transport for SSL/TLS verification
	if m.config.Secure {
		verify := m.config.Verify
		transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: !verify,
			},
		}
	}

	client, err := minio.New(m.config.Host, &minio.Options{
		Creds:     credentials.NewStaticV4(m.config.User, m.config.Password, ""),
		Secure:    m.config.Secure,
		Transport: transport,
	})
	if err != nil {
		return fmt.Errorf("failed to connect to MinIO: %w", err)
	}

	m.client = client
	return nil
}

func (m *MinioStorage) reconnect() {
	if err := m.connect(); err != nil {
		zap.L().Error("Failed to reconnect to MinIO", zap.Error(err))
	}
}

func (m *MinioStorage) resolveBucketAndPath(bucket, fnm string) (string, string) {
	actualBucket := bucket
	if m.bucket != "" {
		actualBucket = m.bucket
	}

	actualPath := fnm
	if m.bucket != "" {
		if m.prefixPath != "" {
			actualPath = fmt.Sprintf("%s/%s/%s", m.prefixPath, bucket, fnm)
		} else {
			actualPath = fmt.Sprintf("%s/%s", bucket, fnm)
		}
	} else if m.prefixPath != "" {
		actualPath = fmt.Sprintf("%s/%s", m.prefixPath, fnm)
	}

	return actualBucket, actualPath
}

// Health checks MinIO service availability
func (m *MinioStorage) Health() bool {
	ctx := context.Background()

	if m.bucket != "" {
		exists, err := m.client.BucketExists(ctx, m.bucket)
		if err != nil {
			zap.L().Warn("MinIO health check failed", zap.Error(err))
			return false
		}
		return exists
	}

	_, err := m.client.ListBuckets(ctx)
	if err != nil {
		zap.L().Warn("MinIO health check failed", zap.Error(err))
		return false
	}
	return true
}

// Put uploads an object to MinIO
func (m *MinioStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	bucket, fnm = m.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	for i := 0; i < 3; i++ {
		// Ensure bucket exists
		if m.bucket == "" {
			exists, err := m.client.BucketExists(ctx, bucket)
			if err != nil {
				zap.L().Error("Failed to check bucket existence", zap.String("bucket", bucket), zap.Error(err))
				m.reconnect()
				time.Sleep(time.Second)
				continue
			}
			if !exists {
				if err := m.client.MakeBucket(ctx, bucket, minio.MakeBucketOptions{}); err != nil {
					zap.L().Error("Failed to create bucket", zap.String("bucket", bucket), zap.Error(err))
					m.reconnect()
					time.Sleep(time.Second)
					continue
				}
			}
		}

		reader := bytes.NewReader(binary)
		_, err := m.client.PutObject(ctx, bucket, fnm, reader, int64(len(binary)), minio.PutObjectOptions{})
		if err != nil {
			zap.L().Error("Failed to put object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			m.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to put object after 3 retries")
}

// Get retrieves an object from MinIO
func (m *MinioStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	bucket, fnm = m.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	for i := 0; i < 2; i++ {
		obj, err := m.client.GetObject(ctx, bucket, fnm, minio.GetObjectOptions{})
		if err != nil {
			zap.L().Error("Failed to get object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			m.reconnect()
			time.Sleep(time.Second)
			continue
		}
		defer obj.Close()

		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(obj); err != nil {
			zap.L().Error("Failed to read object data", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			m.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("failed to get object after retries")
}

// Rm removes an object from MinIO
func (m *MinioStorage) Rm(bucket, fnm string, tenantID ...string) error {
	bucket, fnm = m.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	if err := m.client.RemoveObject(ctx, bucket, fnm, minio.RemoveObjectOptions{}); err != nil {
		zap.L().Error("Failed to remove object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return err
	}

	return nil
}

// ObjExist checks if an object exists in MinIO
func (m *MinioStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	bucket, fnm = m.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	exists, err := m.client.BucketExists(ctx, bucket)
	if err != nil || !exists {
		return false
	}

	_, err = m.client.StatObject(ctx, bucket, fnm, minio.StatObjectOptions{})
	if err != nil {
		errResponse := minio.ToErrorResponse(err)
		if errResponse.Code == "NoSuchKey" || errResponse.Code == "NoSuchBucket" {
			return false
		}
		zap.L().Error("Failed to stat object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return false
	}

	return true
}

// GetPresignedURL generates a presigned URL for accessing an object
func (m *MinioStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	bucket, fnm = m.resolveBucketAndPath(bucket, fnm)

	ctx := context.Background()

	for i := 0; i < 10; i++ {
		url, err := m.client.PresignedGetObject(ctx, bucket, fnm, expires, nil)
		if err != nil {
			zap.L().Error("Failed to get presigned URL", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			m.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return url.String(), nil
	}

	return "", fmt.Errorf("failed to get presigned URL after 10 retries")
}

// BucketExists checks if a bucket exists
func (m *MinioStorage) BucketExists(bucket string) bool {
	actualBucket := bucket
	if m.bucket != "" {
		actualBucket = m.bucket
	}

	ctx := context.Background()

	exists, err := m.client.BucketExists(ctx, actualBucket)
	if err != nil {
		zap.L().Error("Failed to check bucket existence", zap.String("bucket", actualBucket), zap.Error(err))
		return false
	}

	return exists
}

// RemoveBucket removes a bucket and all its objects
func (m *MinioStorage) RemoveBucket(bucket string) error {
	actualBucket := bucket
	origBucket := bucket

	if m.bucket != "" {
		actualBucket = m.bucket
	}

	ctx := context.Background()

	// Build prefix for single-bucket mode
	prefix := ""
	if m.bucket != "" {
		if m.prefixPath != "" {
			prefix = fmt.Sprintf("%s/", m.prefixPath)
		}
		prefix += fmt.Sprintf("%s/", origBucket)
	}

	// List and delete objects with prefix
	objectsCh := make(chan minio.ObjectInfo)

	go func() {
		defer close(objectsCh)
		for obj := range m.client.ListObjects(ctx, actualBucket, minio.ListObjectsOptions{
			Prefix:    prefix,
			Recursive: true,
		}) {
			if obj.Err != nil {
				zap.L().Error("Error listing objects", zap.Error(obj.Err))
				return
			}
			objectsCh <- obj
		}
	}()

	for err := range m.client.RemoveObjects(ctx, actualBucket, objectsCh, minio.RemoveObjectsOptions{}) {
		zap.L().Error("Failed to remove object", zap.String("key", err.ObjectName), zap.Error(err.Err))
	}

	// Only remove the actual bucket if not in single-bucket mode
	if m.bucket == "" {
		if err := m.client.RemoveBucket(ctx, actualBucket); err != nil {
			zap.L().Error("Failed to remove bucket", zap.String("bucket", actualBucket), zap.Error(err))
			return err
		}
	}

	return nil
}

// Copy copies an object from source to destination
func (m *MinioStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	srcBucket, srcPath = m.resolveBucketAndPath(srcBucket, srcPath)
	destBucket, destPath = m.resolveBucketAndPath(destBucket, destPath)

	ctx := context.Background()

	// Ensure destination bucket exists
	if m.bucket == "" {
		exists, err := m.client.BucketExists(ctx, destBucket)
		if err != nil {
			zap.L().Error("Failed to check bucket existence", zap.String("bucket", destBucket), zap.Error(err))
			return false
		}
		if !exists {
			if err := m.client.MakeBucket(ctx, destBucket, minio.MakeBucketOptions{}); err != nil {
				zap.L().Error("Failed to create bucket", zap.String("bucket", destBucket), zap.Error(err))
				return false
			}
		}
	}

	// Check if source object exists
	_, err := m.client.StatObject(ctx, srcBucket, srcPath, minio.StatObjectOptions{})
	if err != nil {
		zap.L().Error("Source object not found", zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
		return false
	}

	// Copy object
	srcOpts := minio.CopySrcOptions{
		Bucket: srcBucket,
		Object: srcPath,
	}
	destOpts := minio.CopyDestOptions{
		Bucket: destBucket,
		Object: destPath,
	}

	_, err = m.client.CopyObject(ctx, destOpts, srcOpts)
	if err != nil {
		zap.L().Error("Failed to copy object", zap.String("src", fmt.Sprintf("%s/%s", srcBucket, srcPath)), zap.String("dest", fmt.Sprintf("%s/%s", destBucket, destPath)), zap.Error(err))
		return false
	}

	return true
}

// Move moves an object from source to destination
func (m *MinioStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if m.Copy(srcBucket, srcPath, destBucket, destPath) {
		if err := m.Rm(srcBucket, srcPath); err != nil {
			zap.L().Error("Failed to remove source object after copy", zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
			return false
		}
		return true
	}
	return false
}
