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

package cloud

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/server/config"
	"ragflow/internal/storage"
)

// Init explicitly installs the configured cloud backend into the shared factory.
// Importing storage for local parsing does not import or initialize cloud SDKs.
func Init(ctx context.Context, config *config.Config) error {
	if config == nil {
		return errors.New("storage configuration required")
	}
	var backend storage.Storage
	var err error
	var name string
	switch config.StorageEngineType() {
	case "minio":
		name = "MinIO"
		backend, err = NewMinioStorage(config.GetMinioConfig())
	case "s3":
		name = "S3"
		backend, err = NewS3Storage(ctx, config.GetS3Config())
	case "oss":
		name = "OSS"
		backend, err = NewOSSStorage(ctx, config.GetOSSConfig())
	case "gcs":
		name = "GCS"
		backend, err = NewGCSStorage(ctx, config.GetGCSConfig())
	default:
		return fmt.Errorf("unsupported storage type: %s", config.StorageEngineType())
	}
	if err != nil {
		return fmt.Errorf("failed to create %s storage: %w", name, err)
	}
	storage.GetStorageFactory().SetStorage(backend)
	common.Info(fmt.Sprintf("Storage initialized: %s", config.StorageEngineType()))
	return nil
}
