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
	"ragflow/internal/common"
	"ragflow/internal/server"
	"sync"
)

var (
	globalFactory *StorageFactory
	once          sync.Once
)

// StorageFactory creates storage instances based on configuration
type StorageFactory struct {
	storage Storage
	mu      sync.RWMutex
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
	// Initialize storage based on type
	if err := factory.initStorage(); err != nil {
		return err
	}

	common.Info(fmt.Sprintf("Storage initialized: %s", globalConfig.StorageEngineType()))

	return nil
}

// CloseStorage closes the storage connection
func CloseStorage() error {
	factory := GetStorageFactory()
	factory.mu.Lock()
	defer factory.mu.Unlock()
	return factory.storage.Close()
}

func (f *StorageFactory) initStorage() error {
	globalConfig := server.GetConfig()
	switch globalConfig.StorageEngineType() {
	case "minio":
		return f.initMinio()
	case "s3":
		return f.initS3()
	case "oss":
		return f.initOSS()
	case "gcs":
		return f.initGCS()
	default:
		return fmt.Errorf("unsupported storage type: %s", globalConfig.StorageEngineType())
	}
}

func (f *StorageFactory) initMinio() error {
	globalConfig := server.GetConfig()
	storage, err := NewMinioStorage(globalConfig.GetMinioConfig())
	if err != nil {
		return fmt.Errorf("failed to create MinIO storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage

	return nil
}

func (f *StorageFactory) initS3() error {
	globalConfig := server.GetConfig()
	storage, err := NewS3Storage(globalConfig.GetS3Config())
	if err != nil {
		return fmt.Errorf("failed to create S3 storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage

	return nil
}

func (f *StorageFactory) initOSS() error {
	globalConfig := server.GetConfig()
	storage, err := NewOSSStorage(globalConfig.GetOSSConfig())
	if err != nil {
		return fmt.Errorf("failed to create OSS storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage

	return nil
}

func (f *StorageFactory) initGCS() error {
	globalConfig := server.GetConfig()
	storage, err := NewGCSStorage(globalConfig.GetGCSConfig())
	if err != nil {
		return fmt.Errorf("failed to create GCS storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage

	return nil
}

// GetStorage returns the current storage instance
func (f *StorageFactory) GetStorage() Storage {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.storage
}

// Create creates a new storage instance based on the storage type
// This is the factory method equivalent to Python's StorageFactory.create()
//func (f *StorageFactory) Create(storageType StorageType) (Storage, error) {
//	var storage Storage
//	var err error
//
//	switch storageType {
//	case StorageMinio:
//		storage, err = NewMinioStorage(f.config.Minio)
//		if err != nil {
//			return nil, fmt.Errorf("MinIO config not available: %w, %v", err, f.config.Minio)
//		}
//	case StorageAWSS3:
//		storage, err = NewS3Storage(f.config.S3)
//		if err != nil {
//			return nil, fmt.Errorf("S3 config not available: %w, %v", err, f.config.S3)
//		}
//	case StorageOSS:
//		storage, err = NewOSSStorage(f.config.OSS)
//		if err != nil {
//			return nil, fmt.Errorf("OSS config not available: %w, %v", err, f.config.OSS)
//		}
//	default:
//		return nil, fmt.Errorf("unsupported storage type: %v", storageType)
//	}
//
//	return storage, nil
//}

// SetStorage sets the storage instance (useful for testing)
func (f *StorageFactory) SetStorage(storage Storage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage
}
