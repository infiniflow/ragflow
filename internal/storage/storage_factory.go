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
	"ragflow/internal/logger"
	"ragflow/internal/server"
	"sync"
)

var (
	globalFactory *StorageFactory
	once          sync.Once
)

// StorageFactory creates storage instances based on configuration
type StorageFactory struct {
	storageType StorageType
	storage     Storage
	config      *server.StorageConfig
	mu          sync.RWMutex
}

// GetStorageFactory returns the singleton storage factory instance
func GetStorageFactory() *StorageFactory {
	once.Do(func() {
		globalFactory = &StorageFactory{}
	})
	return globalFactory
}

// InitStorageFactory initializes the storage factory with configuration
func InitStorageFactory() error {
	factory := GetStorageFactory()

	globalConfig := server.GetConfig()

	factory.config = &globalConfig.StorageEngine

	// Initialize storage based on type
	if err := factory.initStorage(); err != nil {
		return err
	}

	logger.Info(fmt.Sprintf("Storage initialized: %s", factory.config.Type))

	return nil
}

// initStorage initializes the specific storage implementation
func (f *StorageFactory) initStorage() error {
	switch f.config.Type {
	case "minio":
		return f.initMinio(f.config.Minio)
	case "s3":
		return f.initS3(f.config.S3)
	case "oss":
		return f.initOSS(f.config.OSS)
	default:
		return fmt.Errorf("unsupported storage type: %s", f.config.Type)
	}
}

func (f *StorageFactory) initMinio(minioConfig *server.MinioConfig) error {
	storage, err := NewMinioStorage(minioConfig)
	if err != nil {
		return fmt.Errorf("failed to create MinIO storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageMinio
	f.storage = storage
	f.config.Minio = minioConfig

	return nil
}

func (f *StorageFactory) initS3(s3Config *server.S3Config) error {
	storage, err := NewS3Storage(s3Config)
	if err != nil {
		return fmt.Errorf("failed to create S3 storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageAWSS3
	f.storage = storage
	f.config.S3 = s3Config

	return nil
}

func (f *StorageFactory) initOSS(ossConfig *server.OSSConfig) error {

	storage, err := NewOSSStorage(ossConfig)
	if err != nil {
		return fmt.Errorf("failed to create OSS storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storageType = StorageOSS
	f.storage = storage
	f.config.OSS = ossConfig

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
var StorageTypeMapping = map[StorageType]func(*server.StorageConfig) (Storage, error){
	StorageMinio: func(config *server.StorageConfig) (Storage, error) {
		if config.Minio == nil {
			return nil, fmt.Errorf("MinIO config not available")
		}
		return NewMinioStorage(config.Minio)
	},
	StorageAWSS3: func(config *server.StorageConfig) (Storage, error) {
		if config.S3 == nil {
			return nil, fmt.Errorf("S3 config not available")
		}
		return NewS3Storage(config.S3)
	},
	StorageOSS: func(config *server.StorageConfig) (Storage, error) {
		if config.OSS == nil {
			return nil, fmt.Errorf("OSS config not available")
		}
		return NewOSSStorage(config.OSS)
	},
}
