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
	"ragflow/internal/server/config"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
)

// GCSStorage implements Storage interface for GCS.
//
// With config.Bucket set it runs in single-bucket mode: every logical bucket
// (a dataset or folder id) is a prefix inside that one bucket, so an object
// lands at gs://<Bucket>/[<PrefixPath>/]<logical bucket>/<name>. Without it,
// each logical bucket is a real GCS bucket and PrefixPath is not used.
type GCSStorage struct {
	client *storage.Client
	config config.GCSConfig
}

// singleBucket reports whether every logical bucket lives in config.Bucket.
func (m *GCSStorage) singleBucket() bool { return m.config.Bucket != "" }

// resolve maps a logical bucket and object name to the GCS bucket and object.
func (m *GCSStorage) resolve(bucket, fnm string) (string, string) {
	if m.singleBucket() {
		return m.config.Bucket, m.logicalPrefix(bucket) + fnm
	}
	return bucket, fnm
}

// logicalPrefix is the object-name prefix holding a logical bucket in
// single-bucket mode, with its trailing slash.
func (m *GCSStorage) logicalPrefix(bucket string) string {
	if prefix := strings.Trim(m.config.PrefixPath, "/"); prefix != "" {
		return prefix + "/" + bucket + "/"
	}
	return bucket + "/"
}

// gcsBucket is the GCS bucket that holds a logical bucket.
func (m *GCSStorage) gcsBucket(bucket string) string {
	if m.singleBucket() {
		return m.config.Bucket
	}
	return bucket
}

// NewGCSStorage creates a new GCS storage instance
func NewGCSStorage(ctx context.Context, config config.GCSConfig) (*GCSStorage, error) {
	gcsStorage := &GCSStorage{
		config: config,
	}

	if err := gcsStorage.connect(ctx); err != nil {
		return nil, err
	}

	return gcsStorage, nil
}

func (m *GCSStorage) connect(ctx context.Context) error {

	client, err := storage.NewClient(ctx)
	if err != nil {
		common.Fatal(fmt.Sprintf("Failed to create client: %s", err.Error()))
	}

	m.client = client
	return nil
}

func (m *GCSStorage) reconnect(ctx context.Context) {
	if err := m.connect(ctx); err != nil {
		common.Fatal(fmt.Sprintf("Failed to reconnect to GCS, %s", err.Error()))
	}
}

func (m *GCSStorage) Type() string { return "gcs" }

// Health checks GCS service availability
func (m *GCSStorage) Health(ctx context.Context) bool {
	return m.BucketExists(ctx, m.config.Bucket)
}

// Put uploads an object to GCS
func (m *GCSStorage) Put(ctx context.Context, bucket, fnm string, binary []byte, tenantID ...string) error {
	uploadCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	bucket, fnm = m.resolve(bucket, fnm)
	obj := m.client.Bucket(bucket).Object(fnm)
	w := obj.NewWriter(uploadCtx)

	if _, err := w.Write(binary); err != nil {
		return err
	}
	return w.Close()
}

// Get retrieves an object from GCS
func (m *GCSStorage) Get(ctx context.Context, bucket, fnm string, tenantID ...string) ([]byte, error) {
	bucket, fnm = m.resolve(bucket, fnm)
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
func (m *GCSStorage) Remove(ctx context.Context, bucketName, objectName string, tenantID ...string) error {
	bucketName, objectName = m.resolve(bucketName, objectName)
	obj := m.client.Bucket(bucketName).Object(objectName)
	if err := obj.Delete(ctx); err != nil {
		if errors.Is(err, storage.ErrObjectNotExist) || errors.Is(err, storage.ErrBucketNotExist) {
			return nil
		}
		return fmt.Errorf("fail to delete object: %w", err)
	}

	return nil
}

// ObjExist checks if an object exists in GCS
func (m *GCSStorage) ObjExist(ctx context.Context, bucketName, objectName string, tenantID ...string) bool {
	bucketName, objectName = m.resolve(bucketName, objectName)
	obj := m.client.Bucket(bucketName).Object(objectName)

	_, err := obj.Attrs(ctx)
	if err != nil {
		return false
	}

	return true
}

func (m *GCSStorage) ObjectExists(ctx context.Context, bucketName, objectName string) (bool, error) {
	bucketName, objectName = m.resolve(bucketName, objectName)
	_, err := m.client.Bucket(bucketName).Object(objectName).Attrs(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, storage.ErrObjectNotExist) || errors.Is(err, storage.ErrBucketNotExist) {
		return false, nil
	}
	return false, err
}

// ListObjects lists the object names of a logical bucket.
func (m *GCSStorage) ListObjects(ctx context.Context, bucket string, tenantID ...string) ([]string, error) {
	var query *storage.Query
	prefix := ""
	if m.singleBucket() {
		prefix = m.logicalPrefix(bucket)
		query = &storage.Query{Prefix: prefix}
	}
	it := m.client.Bucket(m.gcsBucket(bucket)).Objects(ctx, query)

	var objects []string
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return nil, err
		}

		objects = append(objects, strings.TrimPrefix(attrs.Name, prefix))
	}

	return objects, nil
}

// GetPresignedURL generates a presigned URL for accessing an object
func (m *GCSStorage) GetPresignedURL(ctx context.Context, bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	bucket, fnm = m.resolve(bucket, fnm)
	opts := &storage.SignedURLOptions{
		Method:  "GET",
		Expires: time.Now().Add(expires),
	}
	url, err := m.client.Bucket(bucket).SignedURL(fnm, opts)
	if err != nil {
		return "", err
	}
	return url, nil
}

// BucketExists checks if a bucket exists
func (m *GCSStorage) BucketExists(ctx context.Context, bucket string) bool {
	_, err := m.client.Bucket(m.gcsBucket(bucket)).Attrs(ctx)
	if err != nil {
		return false
	}

	return true
}

func (m *GCSStorage) BucketExistsWithError(ctx context.Context, bucket string) (bool, error) {
	_, err := m.client.Bucket(m.gcsBucket(bucket)).Attrs(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, storage.ErrBucketNotExist) {
		return false, nil
	}
	return false, err
}

// RemoveBucket removes a logical bucket and every generation of its objects,
// so nothing stays behind as a noncurrent version when the bucket has object
// versioning. In single-bucket mode it deletes the bucket's prefix and leaves
// the GCS bucket in place.
func (m *GCSStorage) RemoveBucket(ctx context.Context, bucketName string) error {
	if bucketName == "" {
		return fmt.Errorf("attempt to delete bucket without name")
	}

	bucket := m.client.Bucket(m.gcsBucket(bucketName))
	query := &storage.Query{Versions: true}
	if m.singleBucket() {
		query.Prefix = m.logicalPrefix(bucketName)
	}

	it := bucket.Objects(ctx, query)
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return err
		}

		if err = bucket.Object(attrs.Name).Generation(attrs.Generation).Delete(ctx); err != nil && !errors.Is(err, storage.ErrObjectNotExist) {
			return err
		}
	}

	if m.singleBucket() {
		return nil
	}
	if err := bucket.Delete(ctx); err != nil {
		return err
	}

	return nil
}

// RemoveEmptyBucket removes a bucket only if GCS finds it empty. In
// single-bucket mode it removes nothing and fails while any generation of an
// object, current or noncurrent, remains under the bucket's prefix.
func (m *GCSStorage) RemoveEmptyBucket(ctx context.Context, bucketName string) error {
	if bucketName == "" {
		return fmt.Errorf("attempt to delete bucket without name")
	}
	if m.singleBucket() {
		it := m.client.Bucket(m.config.Bucket).Objects(ctx, &storage.Query{Prefix: m.logicalPrefix(bucketName), Versions: true})
		_, err := it.Next()
		if errors.Is(err, iterator.Done) {
			return nil
		}
		if err != nil {
			return err
		}
		return fmt.Errorf("bucket %s is not empty", bucketName)
	}
	err := m.client.Bucket(bucketName).Delete(ctx)
	if errors.Is(err, storage.ErrBucketNotExist) {
		return nil
	}
	return err
}

// Copy copies an object from source to destination
func (m *GCSStorage) Copy(ctx context.Context, srcBucket, srcObject, destBucket, destObject string) bool {
	srcBucket, srcObject = m.resolve(srcBucket, srcObject)
	destBucket, destObject = m.resolve(destBucket, destObject)
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
func (m *GCSStorage) Move(ctx context.Context, srcBucket, srcPath, destBucket, destPath string) bool {
	if m.Copy(ctx, srcBucket, srcPath, destBucket, destPath) {
		if err := m.Remove(ctx, srcBucket, srcPath); err != nil {
			common.Warn("Failed to remove source object after copy", zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
			rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 30*time.Second)
			defer cancel()
			err = m.Remove(rollbackCtx, destBucket, destPath)
			if err != nil {
				common.Warn("Failed to roll back copied destination object", zap.String("bucket", destBucket), zap.String("key", destPath), zap.Error(err))
			}
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
