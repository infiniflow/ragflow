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
	"net/url"
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/container"
	"go.uber.org/zap"
)

// AzureSASConfig holds Azure Blob Storage SAS configuration
type AzureSASConfig struct {
	ContainerURL string `mapstructure:"container_url"` // Azure container URL
	SASToken     string `mapstructure:"sas_token"`     // SAS token
}

// AzureSASStorage implements Storage interface for Azure Blob Storage using SAS token
type AzureSASStorage struct {
	client       *container.Client
	containerURL string
	sasToken     string
	config       *AzureSASConfig
}

// NewAzureSASStorage creates a new Azure SAS storage instance
func NewAzureSASStorage(config *AzureSASConfig) (*AzureSASStorage, error) {
	// Allow override via environment variables
	containerURL := os.Getenv("CONTAINER_URL")
	if containerURL == "" {
		containerURL = config.ContainerURL
	}

	sasToken := os.Getenv("SAS_TOKEN")
	if sasToken == "" {
		sasToken = config.SASToken
	}

	storage := &AzureSASStorage{
		containerURL: containerURL,
		sasToken:     sasToken,
		config:       config,
	}

	if err := storage.connect(); err != nil {
		return nil, err
	}

	return storage, nil
}

func (a *AzureSASStorage) connect() error {
	// Construct full URL with SAS token
	fullURL := fmt.Sprintf("%s?%s", a.containerURL, a.sasToken)

	// Parse URL to extract SAS token
	parsedURL, err := url.Parse(fullURL)
	if err != nil {
		return fmt.Errorf("failed to parse container URL: %w", err)
	}

	// Create container client with SAS credential
	cred := azblob.NewNoCredential()
	client, err := container.NewClient(fullURL, cred, nil)
	if err != nil {
		return fmt.Errorf("failed to create Azure container client: %w", err)
	}

	a.client = client
	return nil
}

func (a *AzureSASStorage) reconnect() {
	if err := a.connect(); err != nil {
		zap.L().Error("Failed to reconnect to Azure SAS storage", zap.Error(err))
	}
}

// Health checks Azure SAS storage availability
func (a *AzureSASStorage) Health() bool {
	ctx := context.Background()

	testData := []byte("_t@@@1")
	reader := bytes.NewReader(testData)

	_, err := a.client.NewBlockBlobClient("health_check").UploadStream(ctx, reader, nil)
	if err != nil {
		zap.L().Error("Health check failed", zap.Error(err))
		return false
	}

	return true
}

// Put uploads an object to Azure Blob Storage
func (a *AzureSASStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		reader := bytes.NewReader(binary)
		_, err := a.client.NewBlockBlobClient(fnm).UploadStream(ctx, reader, nil)
		if err != nil {
			zap.L().Error("Failed to put object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			a.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to put object after 3 retries")
}

// Get retrieves an object from Azure Blob Storage
func (a *AzureSASStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		resp, err := a.client.NewBlockBlobClient(fnm).DownloadStream(ctx, nil)
		if err != nil {
			zap.L().Error("Failed to get object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			a.reconnect()
			time.Sleep(time.Second)
			continue
		}
		defer resp.Body.Close()

		buf := new(bytes.Buffer)
		if _, err := buf.ReadFrom(resp.Body); err != nil {
			zap.L().Error("Failed to read object data", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			a.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return buf.Bytes(), nil
	}

	return nil, fmt.Errorf("failed to get object after retries")
}

// Rm removes an object from Azure Blob Storage
func (a *AzureSASStorage) Rm(bucket, fnm string, tenantID ...string) error {
	ctx := context.Background()

	_, err := a.client.NewBlockBlobClient(fnm).Delete(ctx, nil)
	if err != nil {
		zap.L().Error("Failed to remove object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return err
	}

	return nil
}

// ObjExist checks if an object exists in Azure Blob Storage
func (a *AzureSASStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	ctx := context.Background()

	_, err := a.client.NewBlockBlobClient(fnm).GetProperties(ctx, nil)
	if err != nil {
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			if respErr.StatusCode == 404 {
				return false
			}
		}
		zap.L().Error("Failed to check object existence", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return false
	}

	return true
}

// GetPresignedURL generates a presigned URL for accessing an object
// Note: With SAS token, the container URL is already a presigned URL
// This method generates a blob-level SAS URL if possible, otherwise returns empty
func (a *AzureSASStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	// With SAS authentication, the container URL is already pre-authorized
	// We can't generate new SAS URLs without account key
	// Return the direct blob URL which should work with the container SAS
	blobURL := fmt.Sprintf("%s/%s?%s", a.containerURL, fnm, a.sasToken)
	return blobURL, nil
}

// BucketExists checks if a container exists
func (a *AzureSASStorage) BucketExists(bucket string) bool {
	ctx := context.Background()

	_, err := a.client.GetProperties(ctx, nil)
	if err != nil {
		zap.L().Debug("Container does not exist or error", zap.Error(err))
		return false
	}

	return true
}

// RemoveBucket removes a container and all its blobs
// Note: With SAS token, we can only delete blobs, not the container itself
func (a *AzureSASStorage) RemoveBucket(bucket string) error {
	ctx := context.Background()

	// List and delete all blobs
	pager := a.client.NewListBlobsFlatPager(nil)
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			zap.L().Error("Failed to list blobs", zap.Error(err))
			return err
		}

		for _, blob := range resp.Segment.BlobItems {
			_, err := a.client.NewBlockBlobClient(*blob.Name).Delete(ctx, nil)
			if err != nil {
				zap.L().Error("Failed to delete blob", zap.String("name", *blob.Name), zap.Error(err))
			}
		}
	}

	return nil
}

// Copy copies an object from source to destination
func (a *AzureSASStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	ctx := context.Background()

	// Source URL with SAS token
	srcURL := fmt.Sprintf("%s/%s?%s", a.containerURL, srcPath, a.sasToken)

	// Copy blob
	copyOptions := &azblob.CopyFromURLOptions{}
	_, err := a.client.NewBlockBlobClient(destPath).CopyFromURL(ctx, srcURL, copyOptions)
	if err != nil {
		zap.L().Error("Failed to copy object", zap.String("src", srcPath), zap.String("dest", destPath), zap.Error(err))
		return false
	}

	return true
}

// Move moves an object from source to destination
func (a *AzureSASStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if a.Copy(srcBucket, srcPath, destBucket, destPath) {
		if err := a.Rm(srcBucket, srcPath); err != nil {
			zap.L().Error("Failed to remove source object after copy", zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
			return false
		}
		return true
	}
	return false
}
