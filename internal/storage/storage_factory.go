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
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/server"
	"sync"
	"time"
)

var (
	globalFactory *Factory
	once          sync.Once
)

// Factory creates storage instances based on configuration
type Factory struct {
	storage Storage
	mu      sync.RWMutex
}

// GetStorageFactory returns the singleton storage factory instance
func GetStorageFactory() *Factory {
	once.Do(func() {
		globalFactory = &Factory{}
	})
	return globalFactory
}

// Init initializes the storage factory with configuration
func Init(ctx context.Context) error {
	factory := GetStorageFactory()

	globalConfig := server.GetConfig()
	// Initialize storage based on type
	if err := factory.initStorage(ctx); err != nil {
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

func (f *Factory) initStorage(ctx context.Context) error {
	globalConfig := server.GetConfig()
	switch globalConfig.StorageEngineType() {
	case "minio":
		return f.initMinio()
	case "s3":
		return f.initS3(ctx)
	case "oss":
		return f.initOSS(ctx)
	case "gcs":
		return f.initGCS(ctx)
	default:
		return fmt.Errorf("unsupported storage type: %s", globalConfig.StorageEngineType())
	}
}

func (f *Factory) initMinio() error {
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

func (f *Factory) initS3(ctx context.Context) error {
	globalConfig := server.GetConfig()
	storage, err := NewS3Storage(ctx, globalConfig.GetS3Config())
	if err != nil {
		return fmt.Errorf("failed to create S3 storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage

	return nil
}

func (f *Factory) initOSS(ctx context.Context) error {
	globalConfig := server.GetConfig()
	storage, err := NewOSSStorage(ctx, globalConfig.GetOSSConfig())
	if err != nil {
		return fmt.Errorf("failed to create OSS storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage

	return nil
}

func (f *Factory) initGCS(ctx context.Context) error {
	globalConfig := server.GetConfig()
	storage, err := NewGCSStorage(ctx, globalConfig.GetGCSConfig())
	if err != nil {
		return fmt.Errorf("failed to create GCS storage: %w", err)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage

	return nil
}

// GetStorage returns the current storage instance
func (f *Factory) GetStorage() Storage {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.storage
}

// SetStorage sets the storage instance (useful for testing)
func (f *Factory) SetStorage(storage Storage) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.storage = storage
}

func sleepOrAbort(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
