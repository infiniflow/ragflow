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
	"fmt"
	"os"
	"sync"

	"github.com/spf13/viper"
	"go.uber.org/zap"
)

// StorageFactory creates storage instances based on configuration
type StorageFactory struct {
	storageType StorageType
	storage     Storage
	config      *StorageConfig
	mu          sync.RWMutex
}

// StorageConfig holds all storage-related configurations
type StorageConfig struct {
	StorageType string       `mapstructure:"storage_type"`
	Minio       *MinioConfig `mapstructure:"minio"`
	S3          *S3Config    `mapstructure:"s3"`
	OSS         *OSSConfig   `mapstructure:"oss"`
}

// AzureConfig holds Azure-specific configurations
type AzureConfig struct {
	ContainerURL  string `mapstructure:"container_url"`
	SASToken      string `mapstructure:"sas_token"`
	AccountURL    string `mapstructure:"account_url"`
	ClientID      string `mapstructure:"client_id"`
	Secret        string `mapstructure:"secret"`
	TenantID      string `mapstructure:"tenant_id"`
	ContainerName string `mapstructure:"container_name"`
	AuthorityHost string `mapstructure:"authority_host"`
}

var (
	globalFactory *StorageFactory
	once          sync.Once
)

// GetStorageFactory returns the singleton storage factory instance
func GetStorageFactory() *StorageFactory {
	once.Do(func() {
		globalFactory = &StorageFactory{}
	})
	return globalFactory
}

// InitStorageFactory initializes the storage factory with configuration
func InitStorageFactory(v *viper.Viper) error {
	factory := GetStorageFactory()

	// Get storage type from environment or config
	storageType := os.Getenv("STORAGE_IMPL")
	if storageType == "" {
		storageType = v.GetString("storage_type")
	}
	if storageType == "" {
		storageType = "MINIO" // Default storage type
	}

	storageConfig := &StorageConfig{}
	if err := v.UnmarshalKey("storage", storageConfig); err != nil {
		return fmt.Errorf("failed to unmarshal storage config: %w", err)
	}
	storageConfig.StorageType = storageType

	factory.config = storageConfig

	// Initialize storage based on type
	if err := factory.initStorage(storageType, v); err != nil {
		return err
	}

	zap.L().Info("Storage factory initialized",
		zap.String("storage_type", storageType),
	)

	return nil
}

// initStorage initializes the specific storage implementation
func (f *StorageFactory) initStorage(storageType string, v *viper.Viper) error {
	switch storageType {
	case "MINIO":
		return f.initMinio(v)
	case "AWS_S3":
		return f.initS3(v)
	case "OSS":
		return f.initOSS(v)
	default:
		return fmt.Errorf("unsupported storage type: %s", storageType)
	}
}

func (f *StorageFactory) initMinio(v *viper.Viper) error {
	config := &MinioConfig{}

	// Try to load from minio section first
	if v.IsSet("minio") {
		minioConfig := v.Sub("minio")
		if minioConfig != nil {
			config.Host = minioConfig.GetString("host")
			config.User = minioConfig.GetString("user")
			config.Password = minioConfig.GetString("password")
			config.Secure = minioConfig.GetBool("secure")
			config.Verify = minioConfig.GetBool("verify")
			config.Bucket = minioConfig.GetString("bucket")
			config.PrefixPath = minioConfig.GetString("prefix_path")
		}
	}

	// Apply defaults
	if config.Host == "" {
		config.Host = "localhost:9000"
	}
	if config.User == "" {
		config.User = "minioadmin"
	}
	if config.Password == "" {
		config.Password = "minioadmin"
	}

	storage, err := NewMinioStorage(config)
	if err != nil {
		return fmt.Errorf("failed to create MinIO storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageMinio
	f.storage = storage
	f.config.Minio = config

	return nil
}

func (f *StorageFactory) initS3(v *viper.Viper) error {
	config := &S3Config{}

	if v.IsSet("s3") {
		s3Config := v.Sub("s3")
		if s3Config != nil {
			config.AccessKeyID = s3Config.GetString("access_key")
			config.SecretAccessKey = s3Config.GetString("secret_key")
			config.SessionToken = s3Config.GetString("session_token")
			config.Region = s3Config.GetString("region_name")
			config.EndpointURL = s3Config.GetString("endpoint_url")
			config.SignatureVersion = s3Config.GetString("signature_version")
			config.AddressingStyle = s3Config.GetString("addressing_style")
			config.Bucket = s3Config.GetString("bucket")
			config.PrefixPath = s3Config.GetString("prefix_path")
		}
	}

	storage, err := NewS3Storage(config)
	if err != nil {
		return fmt.Errorf("failed to create S3 storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageAWSS3
	f.storage = storage
	f.config.S3 = config

	return nil
}

func (f *StorageFactory) initOSS(v *viper.Viper) error {
	config := &OSSConfig{}

	if v.IsSet("oss") {
		ossConfig := v.Sub("oss")
		if ossConfig != nil {
			config.AccessKeyID = ossConfig.GetString("access_key")
			config.SecretAccessKey = ossConfig.GetString("secret_key")
			config.EndpointURL = ossConfig.GetString("endpoint_url")
			config.Region = ossConfig.GetString("region")
			config.Bucket = ossConfig.GetString("bucket")
			config.PrefixPath = ossConfig.GetString("prefix_path")
			config.SignatureVersion = ossConfig.GetString("signature_version")
			config.AddressingStyle = ossConfig.GetString("addressing_style")
		}
	}

	storage, err := NewOSSStorage(config)
	if err != nil {
		return fmt.Errorf("failed to create OSS storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageOSS
	f.storage = storage
	f.config.OSS = config

	return nil
}

// GetStorage returns the current storage instance
func (f *StorageFactory) GetStorage() Storage {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.storage
}

// GetStorageType returns the current storage type
func (f *StorageFactory) GetStorageType() StorageType {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.storageType
}

// Create creates a new storage instance based on the storage type
// This is the factory method equivalent to Python's StorageFactory.create()
func (f *StorageFactory) Create(storageType StorageType) (Storage, error) {
	var storage Storage
	var err error

	switch storageType {
	case StorageMinio:
		if f.config.Minio != nil {
			storage, err = NewMinioStorage(f.config.Minio)
		} else {
			return nil, fmt.Errorf("MinIO config not available")
		}
	case StorageAWSS3:
		if f.config.S3 != nil {
			storage, err = NewS3Storage(f.config.S3)
		} else {
			return nil, fmt.Errorf("S3 config not available")
		}
	case StorageOSS:
		if f.config.OSS != nil {
			storage, err = NewOSSStorage(f.config.OSS)
		} else {
			return nil, fmt.Errorf("OSS config not available")
		}
	default:
		return nil, fmt.Errorf("unsupported storage type: %v", storageType)
	}

	if err != nil {
		return nil, err
	}

	return storage, nil
}

// SetStorage sets the storage instance (useful for testing)
func (f *StorageFactory) SetStorage(storage Storage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage
}

// StorageTypeMapping returns the storage type mapping (equivalent to Python's storage_mapping)
var StorageTypeMapping = map[StorageType]func(*StorageConfig) (Storage, error){
	StorageMinio: func(config *StorageConfig) (Storage, error) {
		if config.Minio == nil {
			return nil, fmt.Errorf("MinIO config not available")
		}
		return NewMinioStorage(config.Minio)
	},
	StorageAWSS3: func(config *StorageConfig) (Storage, error) {
		if config.S3 == nil {
			return nil, fmt.Errorf("S3 config not available")
		}
		return NewS3Storage(config.S3)
	},
	StorageOSS: func(config *StorageConfig) (Storage, error) {
		if config.OSS == nil {
			return nil, fmt.Errorf("OSS config not available")
		}
		return NewOSSStorage(config.OSS)
	},
}
