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
	"io"
	"ragflow/internal/common"
	"ragflow/internal/server"
	"time"

	"cloud.google.com/go/storage"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// GCSStorage implements Storage interface for GCS
type GCSStorage struct {
	client *storage.Client
	config *server.GCSConfig
}

// NewGCSStorage creates a new GCS storage instance
func NewGCSStorage(config *server.GCSConfig) (*GCSStorage, error) {
	gcsStorage := &GCSStorage{
		config: config,
	}

	if err := gcsStorage.connect(); err != nil {
		return nil, err
	}

	return gcsStorage, nil
}

func (m *GCSStorage) connect() error {

	ctx := context.Background()

	client, err := storage.NewClient(ctx)
	if err != nil {
		common.Fatal(fmt.Sprintf("Failed to create client: %s", err.Error()))
	}

	m.client = client
	return nil
}

func (m *GCSStorage) reconnect() {
	if err := m.connect(); err != nil {
		common.Fatal(fmt.Sprintf("Failed to reconnect to GCS, %s", err.Error()))
	}
}

// Health checks GCS service availability
func (m *GCSStorage) Health() bool {
	return m.BucketExists(m.config.Bucket)
}

// Put uploads an object to GCS
func (m *GCSStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	ctx := context.Background()

	obj := m.client.Bucket(bucket).Object(fnm)
	w := obj.NewWriter(ctx)

	if _, err := w.Write(binary); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}

	return nil
}

// Get retrieves an object from GCS
func (m *GCSStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	ctx := context.Background()

	r, err := m.client.Bucket(bucket).Object(fnm).NewReader(ctx)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return data, nil
}

// Remove removes an object from GCS
func (m *GCSStorage) Remove(bucketName, objectName string, tenantID ...string) error {
	ctx := context.Background()

	obj := m.client.Bucket(bucketName).Object(objectName)
	if err := obj.Delete(ctx); err != nil {
		return fmt.Errorf("fail to delete object: %v", err)
	}

	return nil
}

// ObjExist checks if an object exists in GCS
func (m *GCSStorage) ObjExist(bucketName, objectName string, tenantID ...string) bool {
	ctx := context.Background()

	obj := m.client.Bucket(bucketName).Object(objectName)

	_, err := obj.Attrs(ctx)
	if err != nil {
		return false
	}

	return true
}

func (m *GCSStorage) ListObjects(bucket string, tenantID ...string) ([]string, error) {
	ctx := context.Background()

	bucketObject := m.client.Bucket(bucket)
	it := bucketObject.Objects(ctx, nil)

	var objects []string
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}

		objects = append(objects, attrs.Name)
	}

	return objects, nil
}

// GetPresignedURL generates a presigned URL for accessing an object
func (m *GCSStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {

	bucketObject := m.client.Bucket(bucket)
	objectPath := fmt.Sprintf("%s/%s", bucket, fnm)
	opts := &storage.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(expires * time.Second),
	}
	url, err := bucketObject.SignedURL(objectPath, opts)
	if err != nil {
		return "", err
	}
	return url, nil
}

// BucketExists checks if a bucket exists
func (m *GCSStorage) BucketExists(bucket string) bool {
	actualBucket := bucket
	if m.config.Bucket != "" {
		actualBucket = m.config.Bucket
	}

	ctx := context.Background()

	_, err := m.client.Bucket(actualBucket).Attrs(ctx)
	if err != nil {
		return false
	}

	return true
}

// RemoveBucket removes a bucket and all its objects
func (m *GCSStorage) RemoveBucket(bucketName string) error {
	if bucketName == "" {
		return fmt.Errorf("attempt to delete bucket without name")
	}

	ctx := context.Background()

	bucket := m.client.Bucket(bucketName)

	it := bucket.Objects(ctx, nil)
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}

		if err = bucket.Object(attrs.Name).Delete(ctx); err != nil {
			return err
		}
	}

	if err := bucket.Delete(ctx); err != nil {
		return err
	}

	return nil
}

// Copy copies an object from source to destination
func (m *GCSStorage) Copy(srcBucket, srcObject, destBucket, destObject string) bool {
	ctx := context.Background()
	src := m.client.Bucket(srcBucket).Object(srcObject)
	dst := m.client.Bucket(destBucket).Object(destObject)
	copier := dst.CopierFrom(src)
	_, err := copier.Run(ctx)
	if err != nil {
		return false
	}

	return true
}

// Move moves an object from source to destination
func (m *GCSStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if m.Copy(srcBucket, srcPath, destBucket, destPath) {
		if err := m.Remove(srcBucket, srcPath); err != nil {
			common.Warn("Failed to remove source object after copy", zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
			return false
		}
		return true
	}
	return false
}

func (m *GCSStorage) Close() error {
	common.Info("Closing GCS client")
	return m.client.Close()
}
