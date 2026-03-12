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
	"os"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azdatalake"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azdatalake/file"
	"go.uber.org/zap"
)

// AzureSPNConfig holds Azure Data Lake Storage SPN configuration
type AzureSPNConfig struct {
	AccountURL    string `mapstructure:"account_url"`    // Azure Data Lake account URL (e.g., "https://<account>.dfs.core.windows.net")
	ClientID      string `mapstructure:"client_id"`      // Service Principal Client ID
	ClientSecret  string `mapstructure:"secret"`         // Service Principal Client Secret
	TenantID      string `mapstructure:"tenant_id"`      // Azure AD Tenant ID
	ContainerName string `mapstructure:"container_name"` // File system/container name
	AuthorityHost string `mapstructure:"authority_host"` // Azure authority host (default: Azure China)
}

// AzureSPNStorage implements Storage interface for Azure Data Lake Storage using Service Principal
type AzureSPNStorage struct {
	client        *azdatalake.Client
	containerName string
	config        *AzureSPNConfig
}

// NewAzureSPNStorage creates a new Azure SPN storage instance
func NewAzureSPNStorage(config *AzureSPNConfig) (*AzureSPNStorage, error) {
	// Allow override via environment variables
	accountURL := os.Getenv("ACCOUNT_URL")
	if accountURL == "" {
		accountURL = config.AccountURL
	}

	clientID := os.Getenv("CLIENT_ID")
	if clientID == "" {
		clientID = config.ClientID
	}

	clientSecret := os.Getenv("SECRET")
	if clientSecret == "" {
		clientSecret = config.ClientSecret
	}

	tenantID := os.Getenv("TENANT_ID")
	if tenantID == "" {
		tenantID = config.TenantID
	}

	containerName := os.Getenv("CONTAINER_NAME")
	if containerName == "" {
		containerName = config.ContainerName
	}

	storage := &AzureSPNStorage{
		containerName: containerName,
		config: &AzureSPNConfig{
			AccountURL:    accountURL,
			ClientID:      clientID,
			ClientSecret:  clientSecret,
			TenantID:      tenantID,
			ContainerName: containerName,
			AuthorityHost: config.AuthorityHost,
		},
	}

	if err := storage.connect(); err != nil {
		return nil, err
	}

	return storage, nil
}

func (a *AzureSPNStorage) connect() error {
	// Create client secret credential
	credOptions := &azidentity.ClientSecretCredentialOptions{}

	// Set authority host (default to Azure China)
	if a.config.AuthorityHost == "" {
		credOptions.AuthorityHost = azidentity.AzureChina
	} else {
		switch a.config.AuthorityHost {
		case "AzurePublicCloud":
			credOptions.AuthorityHost = azidentity.AzurePublicCloud
		case "AzureGovernment":
			credOptions.AuthorityHost = azidentity.AzureGovernment
		case "AzureChina":
			credOptions.AuthorityHost = azidentity.AzureChina
		}
	}

	cred, err := azidentity.NewClientSecretCredential(
		a.config.TenantID,
		a.config.ClientID,
		a.config.ClientSecret,
		credOptions,
	)
	if err != nil {
		return fmt.Errorf("failed to create Azure credential: %w", err)
	}

	// Create Data Lake client
	client, err := azdatalake.NewClient(a.config.AccountURL, cred, nil)
	if err != nil {
		return fmt.Errorf("failed to create Azure Data Lake client: %w", err)
	}

	a.client = client
	return nil
}

func (a *AzureSPNStorage) reconnect() {
	if err := a.connect(); err != nil {
		zap.L().Error("Failed to reconnect to Azure SPN storage", zap.Error(err))
	}
}

func (a *AzureSPNStorage) getFileClient(fnm string) *file.Client {
	fsClient := a.client.NewFileSystemClient(a.containerName)
	return fsClient.NewFileClient(fnm)
}

// Health checks Azure SPN storage availability
func (a *AzureSPNStorage) Health() bool {
	ctx := context.Background()

	testData := []byte("_t@@@1")

	fileClient := a.getFileClient("health_check")

	// Create file
	_, err := fileClient.Create(ctx, nil)
	if err != nil {
		zap.L().Error("Health check failed - create file", zap.Error(err))
		return false
	}

	// Append data
	reader := bytes.NewReader(testData)
	_, err = fileClient.AppendData(ctx, 0, reader, nil)
	if err != nil {
		zap.L().Error("Health check failed - append data", zap.Error(err))
		return false
	}

	// Flush data
	_, err = fileClient.FlushData(ctx, int64(len(testData)), nil)
	if err != nil {
		zap.L().Error("Health check failed - flush data", zap.Error(err))
		return false
	}

	return true
}

// Put uploads an object to Azure Data Lake Storage
func (a *AzureSPNStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		fileClient := a.getFileClient(fnm)

		// Create file
		_, err := fileClient.Create(ctx, nil)
		if err != nil {
			zap.L().Error("Failed to create file", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			a.reconnect()
			time.Sleep(time.Second)
			continue
		}

		// Append data
		reader := bytes.NewReader(binary)
		_, err = fileClient.AppendData(ctx, 0, reader, nil)
		if err != nil {
			zap.L().Error("Failed to append data", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			a.reconnect()
			time.Sleep(time.Second)
			continue
		}

		// Flush data
		_, err = fileClient.FlushData(ctx, int64(len(binary)), nil)
		if err != nil {
			zap.L().Error("Failed to flush data", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
			a.reconnect()
			time.Sleep(time.Second)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to put object after 3 retries")
}

// Get retrieves an object from Azure Data Lake Storage
func (a *AzureSPNStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	ctx := context.Background()

	for i := 0; i < 2; i++ {
		fileClient := a.getFileClient(fnm)

		resp, err := fileClient.Read(ctx, nil)
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

// Rm removes an object from Azure Data Lake Storage
func (a *AzureSPNStorage) Rm(bucket, fnm string, tenantID ...string) error {
	ctx := context.Background()

	fileClient := a.getFileClient(fnm)

	_, err := fileClient.Delete(ctx, nil)
	if err != nil {
		zap.L().Error("Failed to remove object", zap.String("bucket", bucket), zap.String("key", fnm), zap.Error(err))
		return err
	}

	return nil
}

// ObjExist checks if an object exists in Azure Data Lake Storage
func (a *AzureSPNStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	ctx := context.Background()

	fileClient := a.getFileClient(fnm)

	_, err := fileClient.GetProperties(ctx, nil)
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
// Note: Azure Data Lake doesn't support presigned URLs like S3
// This method returns empty string
func (a *AzureSPNStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	// Azure Data Lake doesn't support presigned URLs like S3
	// Users can use Azure Blob Storage with SAS tokens for this purpose
	return "", fmt.Errorf("presigned URLs not supported for Azure Data Lake")
}

// BucketExists checks if a file system exists
func (a *AzureSPNStorage) BucketExists(bucket string) bool {
	ctx := context.Background()

	fsClient := a.client.NewFileSystemClient(bucket)

	_, err := fsClient.GetProperties(ctx, nil)
	if err != nil {
		zap.L().Debug("File system does not exist or error", zap.Error(err))
		return false
	}

	return true
}

// RemoveBucket removes a file system and all its files
func (a *AzureSPNStorage) RemoveBucket(bucket string) error {
	ctx := context.Background()

	fsClient := a.client.NewFileSystemClient(bucket)

	// List and delete all paths
	pager := fsClient.NewListPathsPager(true, nil)
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			zap.L().Error("Failed to list paths", zap.Error(err))
			return err
		}

		for _, path := range resp.Paths {
			if path.Name != nil {
				_, err := fsClient.NewFileClient(*path.Name).Delete(ctx, nil)
				if err != nil {
					zap.L().Error("Failed to delete file", zap.String("name", *path.Name), zap.Error(err))
				}
			}
		}
	}

	// Delete file system
	_, err := fsClient.Delete(ctx, nil)
	if err != nil {
		zap.L().Error("Failed to delete file system", zap.String("container", bucket), zap.Error(err))
		return err
	}

	return nil
}

// Copy copies an object from source to destination
func (a *AzureSPNStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	ctx := context.Background()

	srcFsClient := a.client.NewFileSystemClient(srcBucket)
	destFsClient := a.client.NewFileSystemClient(destBucket)

	srcFileClient := srcFsClient.NewFileClient(srcPath)
	destFileClient := destFsClient.NewFileClient(destPath)

	// Get source file properties to get content length
	resp, err := srcFileClient.GetProperties(ctx, nil)
	if err != nil {
		zap.L().Error("Failed to get source file properties", zap.String("src", srcPath), zap.Error(err))
		return false
	}

	contentLength := int64(0)
	if resp.ContentLength != nil {
		contentLength = *resp.ContentLength
	}

	// Create destination file
	_, err = destFileClient.Create(ctx, nil)
	if err != nil {
		zap.L().Error("Failed to create destination file", zap.String("dest", destPath), zap.Error(err))
		return false
	}

	// Copy data in chunks
	chunkSize := int64(4 * 1024 * 1024) // 4MB chunks
	for offset := int64(0); offset < contentLength; offset += chunkSize {
		end := offset + chunkSize
		if end > contentLength {
			end = contentLength
		}
		length := end - offset

		_, err = destFileClient.AppendDataFromURL(ctx, srcFileClient.URL(), offset, offset, length, nil)
		if err != nil {
			zap.L().Error("Failed to append data from URL", zap.Error(err))
			return false
		}
	}

	// Flush data
	_, err = destFileClient.FlushData(ctx, contentLength, nil)
	if err != nil {
		zap.L().Error("Failed to flush data", zap.Error(err))
		return false
	}

	return true
}

// Move moves an object from source to destination
func (a *AzureSPNStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if a.Copy(srcBucket, srcPath, destBucket, destPath) {
		if err := a.Rm(srcBucket, srcPath); err != nil {
			zap.L().Error("Failed to remove source object after copy", zap.String("bucket", srcBucket), zap.String("key", srcPath), zap.Error(err))
			return false
		}
		return true
	}
	return false
}
