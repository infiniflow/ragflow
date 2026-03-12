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

	"cloud.google.com/go/storage"
	"go.uber.org/zap"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// GCSConfig holds Google Cloud Storage configuration
type GCSConfig struct {
	Bucket          string `mapstructure:"bucket"`           // GCS bucket name
	CredentialsFile string `mapstructure:"credentials_file"` // Path to service account credentials JSON file (optional)
	ProjectID       string `mapstructure:"project_id"`       // GCP Project ID (optional)
}

// GCSStorage implements Storage interface for Google Cloud Storage
type GCSStorage struct {
	client     *storage.Client
	bucket     string
	config     *GCSConfig
}

// NewGCSStorage creates a new GCS storage instance
func NewGCSStorage(config *GCSConfig) (*GCSStorage, error) {
	storage := &GCSStorage{
		bucket: config.Bucket,
		config: config,
	}

	if err := storage.connect(); err != nil {
		return nil, err
	}

	return storage, nil
}

func (g *GCSStorage) connect() error {
	ctx := context.Background()

	var opts []option.ClientOption

	// Use credentials file if provided
	if g.config.CredentialsFile != "" {
		opts = append(opts, option.WithCredentialsFile(g.config.CredentialsFile))
	}

	client, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return fmt.Errorf("failed to create GCS client: %w", err)
	}

	g.client = client
	return nil
}

func (g *GCSStorage) reconnect() {
	if err := g.connect(); err != nil {
		zap.L().Error("Failed to reconnect to GCS", zap.Error(err))
	}
}

func (g *GCSStorage) getBlobPath(folder, filename string) string {
	if folder == "" {
		return filename
	}
	return fmt.Sprintf("%s/%s", folder, filename)
}

// Health checks GCS service availability
func (g *GCSStorage) Health() bool {
	ctx := context.Background()

	// Check if bucket exists
	bucketHandle := g.client.Bucket(g.bucket)
	_, err := bucketHandle.Attrs(ctx)
	if err != nil {
		zap.L().Error("Health check failed - bucket does not exist", zap.String("bucket", g.bucket), zap.Error(err))
		return false
	}

	// Try to upload a test object
	testData := []byte("_t@@@1")
	blobPath := g.getBlobPath("ragflow-health", "health_check")

	w := bucketHandle.Object(blobPath).NewWriter(ctx)
	if _, err := w.Write(testData); err != nil {
		zap.L().Error("Health check failed - write error", zap.Error(err))
		w.Close()
		return false
	}
	if err := w.Close(); err != nil {
		zap.L().Error("Health check failed - close error", zap.Error(err))
		return false
	}

	return true
}

// Put uploads an object to GCS
func (g *GCSStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	ctx := context.Background()

	bucketHandle := g.client.Bucket(g.bucket)
	blobPath := g.getBlobPath(bucket, fnm)

	for i := 0; i < 3; i++ {
		w := bucketHandle.Object(blobPath).NewWriter(ctx)
		if _, err := w.Write(binary); err != nil {
			zap.L().Error("Failed to write object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			w.Close()
			g.reconnect()
			time.Sleep(time.Second)
			continue
		}
		if err := w.Close(); err != nil {
			zap.L().Error("Failed to close writer", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			g.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to put object after 3 retries")
}

// Get retrieves an object from GCS
func (g *GCSStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	ctx := context.Background()

	bucketHandle := g.client.Bucket(g.bucket)
	blobPath := g.getBlobPath(bucket, fnm)

	for i := 0; i < 2; i++ {
		r, err := bucketHandle.Object(blobPath).NewReader(ctx)
		if err != nil {
			if err == storage.ErrObjectNotExist {
				return nil, nil
			}
			zap.L().Error("Failed to get object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			g.reconnect()
			time.Sleep(time.Second)
			continue
		}
		defer r.Close()

		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(r); err != nil {
			zap.L().Error("Failed to read object data", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			g.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("failed to get object after retries")
}

// Rm removes an object from GCS
func (g *GCSStorage) Rm(bucket, fnm string, tenantID ...string) error {
	ctx := context.Background()

	bucketHandle := g.client.Bucket(g.bucket)
	blobPath := g.getBlobPath(bucket, fnm)

	if err := bucketHandle.Object(blobPath).Delete(ctx); err != nil {
		if err == storage.ErrObjectNotExist {
			return nil
		}
		zap.L().Error("Failed to remove object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return err
	}

	return nil
}

// ObjExist checks if an object exists in GCS
func (g *GCSStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	ctx := context.Background()

	bucketHandle := g.client.Bucket(g.bucket)
	blobPath := g.getBlobPath(bucket, fnm)

	_, err := bucketHandle.Object(blobPath).Attrs(ctx)
	if err != nil {
		if err == storage.ErrObjectNotExist {
			return false
		}
		zap.L().Error("Failed to get object attributes", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return false
	}

	return true
}

// GetPresignedURL generates a presigned URL for accessing an object
func (g *GCSStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	ctx := context.Background()

	bucketHandle := g.client.Bucket(g.bucket)
	blobPath := g.getBlobPath(bucket, fnm)

	for i := 0; i < 10; i++ {
		opts := &storage.SignedURLOptions{
			Method:  "GET",
			Expires: time.Now().Add(expires),
		}

		url, err := bucketHandle.SignedURL(blobPath, opts)
		if err != nil {
			zap.L().Error("Failed to generate presigned URL", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			g.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return url, nil
	}

	return "", fmt.Errorf("failed to generate presigned URL after 10 retries")
}

// BucketExists checks if a bucket exists
func (g *GCSStorage) BucketExists(bucket string) bool {
	ctx := context.Background()

	bucketHandle := g.client.Bucket(g.bucket)
	_, err := bucketHandle.Attrs(ctx)
	if err != nil {
		zap.L().Debug("Bucket does not exist or error", zap.String("bucket", g.bucket), zap.Error(err))
		return false
	}

	return true
}

// RemoveBucket removes a virtual bucket (folder) and all its objects
func (g *GCSStorage) RemoveBucket(bucket string) error {
	ctx := context.Background()

	bucketHandle := g.client.Bucket(g.bucket)
	prefix := g.getBlobPath(bucket, "")

	// List and delete all objects with prefix
	it := bucketHandle.Objects(ctx, &storage.Query{
		Prefix: prefix,
	})

	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			zap.L().Error("Failed to list objects", zap.Error(err))
			return err
		}

		if err := bucketHandle.Object(attrs.Name).Delete(ctx); err != nil {
			zap.L().Error("Failed to delete object", zap.String("name", attrs.Name), zap.Error(err))
		}
	}

	return nil
}

// Copy copies an object from source to destination
func (g *GCSStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	ctx := context.Background()

	srcBlobPath := g.getBlobPath(srcBucket, srcPath)
	destBlobPath := g.getBlobPath(destBucket, destPath)

	src := g.client.Bucket(g.bucket).Object(srcBlobPath)
	dest := g.client.Bucket(g.bucket).Object(destBlobPath)

	// Check if source object exists
	_, err := src.Attrs(ctx)
	if err != nil {
		zap.L().Error("Source object not found", zap.String("path", srcBlobPath), zap.Error(err))
		return false
	}

	// Copy object
	if _, err := dest.CopierFrom(src).Run(ctx); err != nil {
		zap.L().Error("Failed to copy object", zap.String("src", srcBlobPath), zap.String("dest", destBlobPath), zap.Error(err))
		return false
	}

	return true
}

// Move moves an object from source to destination
func (g *GCSStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if g.Copy(srcBucket, srcPath, destBucket, destPath) {
		if err := g.Rm(srcBucket, srcPath); err != nil {
			zap.L().Error("Failed to remove source object after copy", zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
			return false
		}
		return true
	}
	return false
}
