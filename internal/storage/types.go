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
	"errors"
	"time"
)

var (
	// ErrNotFound is returned when an object is not found
	ErrNotFound = errors.New("object not found")
	// ErrBucketNotFound is returned when a bucket is not found
	ErrBucketNotFound = errors.New("bucket not found")
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
	// Health checks the storage service availability
	Health() bool

	// Put uploads an object to storage
	// bucket: the bucket/container name
	// fnm: the file/object name (key)
	// binary: the data to upload
	// tenantID: optional tenant identifier
	Put(bucket, fnm string, binary []byte, tenantID ...string) error

	// Get retrieves an object from storage
	// Returns the data or nil if not found
	Get(bucket, fnm string, tenantID ...string) ([]byte, error)

	// Remove removes an object from storage
	Remove(bucket, fnm string, tenantID ...string) error

	// ObjExist checks if an object exists
	ObjExist(bucket, fnm string, tenantID ...string) bool

	// GetPresignedURL generates a presigned URL for accessing an object
	// expires: duration until the URL expires
	GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error)

	// BucketExists checks if a bucket exists
	BucketExists(bucket string) bool

	// RemoveBucket removes a bucket and all its objects
	RemoveBucket(bucket string) error

	// Copy copies an object from source to destination
	Copy(srcBucket, srcPath, destBucket, destPath string) bool

	// Move moves an object from source to destination
	Move(srcBucket, srcPath, destBucket, destPath string) bool
}
