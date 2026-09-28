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
	"time"
)

// StorageType represents the type of storage backend
type StorageType int

const (
	StorageMinio    StorageType = 1
	StorageAzureSpn StorageType = 2
	StorageAzureSas StorageType = 3
	StorageAWSS3    StorageType = 4
	StorageOSS      StorageType = 5
	StorageOpenDAL  StorageType = 6
	StorageGCS      StorageType = 7
)

func (s StorageType) String() string {
	switch s {
	case StorageMinio:
		return "MINIO"
	case StorageAzureSpn:
		return "AZURE_SPN"
	case StorageAzureSas:
		return "AZURE_SAS"
	case StorageAWSS3:
		return "AWS_S3"
	case StorageOSS:
		return "OSS"
	case StorageOpenDAL:
		return "OPENDAL"
	case StorageGCS:
		return "GCS"
	default:
		return "UNKNOWN"
	}
}

// Storage defines the interface for storage operations
type Storage interface {
	Type() string

	// Health checks the storage service availability
	Health(ctx context.Context) bool

	// Put uploads an object to storage
	// bucket: the bucket/container name
	// fnm: the file/object name (key)
	// binary: the data to upload
	// tenantID: optional tenant identifier
	Put(ctx context.Context, bucket, fnm string, binary []byte, tenantID ...string) error

	// Get retrieves an object from storage
	// Returns the data or nil if not found
	Get(ctx context.Context, bucket, fnm string, tenantID ...string) ([]byte, error)

	// Remove removes an object from storage
	Remove(ctx context.Context, bucket, fnm string, tenantID ...string) error

	// ObjExist checks if an object exists
	ObjExist(ctx context.Context, bucket, fnm string, tenantID ...string) bool
	// ObjectExists distinguishes a missing object from a storage error
	ObjectExists(ctx context.Context, bucket, fnm string) (bool, error)

	// ListObjects list all objects of the bucket
	ListObjects(ctx context.Context, bucket string, tenantID ...string) ([]string, error)

	// GetPresignedURL generates a presigned URL for accessing an object
	// expires: duration until the URL expires
	GetPresignedURL(ctx context.Context, bucket, fnm string, expires time.Duration, tenantID ...string) (string, error)

	// BucketExists checks if a bucket exists
	BucketExists(ctx context.Context, bucket string) bool
	// BucketExistsWithError distinguishes a missing bucket from a storage error
	BucketExistsWithError(ctx context.Context, bucket string) (bool, error)

	// RemoveBucket removes a bucket and all its objects
	RemoveBucket(ctx context.Context, bucket string) error

	// RemoveEmptyBucket removes a bucket only if it has no objects
	RemoveEmptyBucket(ctx context.Context, bucket string) error

	// Copy copies an object from source to destination
	Copy(ctx context.Context, srcBucket, srcPath, destBucket, destPath string) bool

	// Move moves an object from source to destination
	Move(ctx context.Context, srcBucket, srcPath, destBucket, destPath string) bool

	// Close closes the storage connection
	Close() error
}
