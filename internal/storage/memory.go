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
	"fmt"
	"sync"
	"time"
)

// ErrMemoryNotFound is returned when a key does not exist in the in-memory backend.
var ErrMemoryNotFound = errors.New("memory storage: object not found")

// MemoryEntry describes a single stored object, used by Inspect for tests and diagnostics.
type MemoryEntry struct {
	Bucket string
	Key    string
	Size   int
}

// MemoryStorage is an in-process implementation of the Storage interface,
// intended for unit tests and ephemeral tooling. All operations are safe
// for concurrent use.
type MemoryStorage struct {
	mu      sync.RWMutex
	objects map[string]map[string][]byte
}

// NewMemoryStorage returns a fresh, empty in-memory storage backend.
func NewMemoryStorage() Storage {
	return &MemoryStorage{objects: make(map[string]map[string][]byte)}
}

// Health always reports healthy for the in-memory backend.
func (m *MemoryStorage) Health() bool {
	return true
}

// Put uploads an object to the in-memory backend, creating the bucket
// on demand if it does not yet exist. The stored bytes are a defensive copy.
func (m *MemoryStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	if bucket == "" {
		return fmt.Errorf("memory storage: bucket is required")
	}
	if fnm == "" {
		return fmt.Errorf("memory storage: key is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	bucketMap, ok := m.objects[bucket]
	if !ok {
		bucketMap = make(map[string][]byte)
		m.objects[bucket] = bucketMap
	}

	cp := make([]byte, len(binary))
	copy(cp, binary)
	bucketMap[fnm] = cp
	return nil
}

// Get retrieves an object from the in-memory backend. Returns
// ErrMemoryNotFound when the bucket or key is missing.
func (m *MemoryStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bucketMap, ok := m.objects[bucket]
	if !ok {
		return nil, fmt.Errorf("memory storage: bucket %q: %w", bucket, ErrMemoryNotFound)
	}
	data, ok := bucketMap[fnm]
	if !ok {
		return nil, fmt.Errorf("memory storage: object %q in bucket %q: %w", fnm, bucket, ErrMemoryNotFound)
	}

	out := make([]byte, len(data))
	copy(out, data)
	return out, nil
}

// Remove deletes an object from the in-memory backend. Removing a
// non-existent key is a no-op and returns nil.
func (m *MemoryStorage) Remove(bucket, fnm string, tenantID ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	bucketMap, ok := m.objects[bucket]
	if !ok {
		return nil
	}
	delete(bucketMap, fnm)
	return nil
}

// ObjExist reports whether the given bucket and key are present.
func (m *MemoryStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	bucketMap, ok := m.objects[bucket]
	if !ok {
		return false
	}
	_, ok = bucketMap[fnm]
	return ok
}

// GetPresignedURL returns a deterministic, non-network URL string for tests.
// Format: memory://<bucket>/<key>?exp=<unix-seconds>
func (m *MemoryStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if _, ok := m.objects[bucket]; !ok {
		return "", fmt.Errorf("memory storage: bucket %q: %w", bucket, ErrMemoryNotFound)
	}
	bucketMap := m.objects[bucket]
	if _, ok := bucketMap[fnm]; !ok {
		return "", fmt.Errorf("memory storage: object %q in bucket %q: %w", fnm, bucket, ErrMemoryNotFound)
	}

	exp := time.Now().Add(expires).Unix()
	return fmt.Sprintf("memory://%s/%s?exp=%d", bucket, fnm, exp), nil
}

// BucketExists reports whether the named bucket has been created.
func (m *MemoryStorage) BucketExists(bucket string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	_, ok := m.objects[bucket]
	return ok
}

// RemoveBucket deletes a bucket and all of its keys. Removing a
// non-existent bucket is a no-op and returns nil.
func (m *MemoryStorage) RemoveBucket(bucket string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.objects, bucket)
	return nil
}

// Copy duplicates an object from srcBucket/srcKey to destBucket/destKey.
// The source is left untouched. Returns false if the source does not exist
// or if the destination bucket creation fails.
func (m *MemoryStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	m.mu.RLock()
	srcBucketMap, ok := m.objects[srcBucket]
	if !ok {
		m.mu.RUnlock()
		return false
	}
	data, ok := srcBucketMap[srcPath]
	if !ok {
		m.mu.RUnlock()
		return false
	}
	cp := make([]byte, len(data))
	copy(cp, data)
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()
	destBucketMap, ok := m.objects[destBucket]
	if !ok {
		destBucketMap = make(map[string][]byte)
		m.objects[destBucket] = destBucketMap
	}
	destBucketMap[destPath] = cp
	return true
}

// Move transfers an object to a new location, deleting the source on success.
// Returns false if the source does not exist or the copy step fails.
func (m *MemoryStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	if !m.Copy(srcBucket, srcPath, destBucket, destPath) {
		return false
	}
	if err := m.Remove(srcBucket, srcPath); err != nil {
		return false
	}
	return true
}

// Inspect returns a stable snapshot of all (bucket, key, size) entries
// currently held by the backend. Intended for test diagnostics and
// cleanup assertions. The slice is freshly allocated and safe to mutate.
func (m *MemoryStorage) Inspect() []MemoryEntry {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]MemoryEntry, 0)
	for bucket, bucketMap := range m.objects {
		for key, data := range bucketMap {
			out = append(out, MemoryEntry{Bucket: bucket, Key: key, Size: len(data)})
		}
	}
	return out
}
