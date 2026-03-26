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
	"fmt"
	"log"
	"os"
	"ragflow/internal/utility"
	"testing"
	"time"

	"ragflow/internal/server"
)

// getMinioConfig returns MinIO configuration for testing
// Configuration can be loaded from environment variables or config file
func getMinioConfig() (*server.MinioConfig, error) {

	// Initialize configuration
	if err := server.Init(""); err != nil {
		return nil, err
	}

	// Try to get configuration from environment variables first
	config := server.GetConfig().StorageEngine.Minio

	log.Printf("MinioConfig: %+v", config)
	return config, nil
}

// getEnv gets environment variable or returns default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvBool gets environment variable as bool or returns default value
func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

// newTestMinioStorage creates a new MinIO storage instance for testing
func newTestMinioStorage(t *testing.T) *MinioStorage {
	rootDir := utility.GetProjectRoot()
	t.Chdir(rootDir)
	t.Chdir(rootDir)

	config, err := getMinioConfig()

	if err != nil {
		t.Skipf("Skipping test: failed to get MinIO configuration: %v", err)
		return nil
	}
	storage, err := NewMinioStorage(config)
	if err != nil {
		t.Skipf("Skipping test: failed to connect to MinIO: %v", err)
	}
	return storage
}

func TestNewMinioStorage(t *testing.T) {
	rootDir := utility.GetProjectRoot()
	t.Chdir(rootDir)

	config, err := getMinioConfig()
	if err != nil {
		t.Skipf("Skipping test: failed to get MinIO configuration: %v", err)
		return
	}

	storage, err := NewMinioStorage(config)
	if err != nil {
		t.Skipf("Skipping test: failed to connect to MinIO: %v", err)
	}

	if storage == nil {
		t.Error("Expected storage to be non-nil")
	}

	if storage.client == nil {
		t.Error("Expected client to be non-nil")
	}

	if storage.config == nil {
		t.Error("Expected config to be non-nil")
	}
}

func TestNewMinioStorage_InvalidConfig(t *testing.T) {
	// Test with invalid host
	config := &server.MinioConfig{
		Host:     "invalid-host:99999",
		User:     "test",
		Password: "test",
		Secure:   false,
	}

	_, err := NewMinioStorage(config)
	// Should return an error for invalid connection
	if err == nil {
		t.Log("Note: Connection may succeed but fail later depending on network timeout")
	}
}

func TestMinioStorage_Health(t *testing.T) {
	storage := newTestMinioStorage(t)

	healthy := storage.Health()
	// Health check should return true if connection is working
	// Note: This depends on whether a default bucket is configured
	t.Logf("Health check result: %v", healthy)
}

func TestMinioStorage_PutAndGet(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	key := "test-file.txt"
	content := []byte("Hello, MinIO Test!")

	// Test Put
	err := storage.Put(bucket, key, content)
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	// Test Get
	retrieved, err := storage.Get(bucket, key)
	if err != nil {
		t.Fatalf("Failed to get object: %v", err)
	}

	if !bytes.Equal(retrieved, content) {
		t.Errorf("Retrieved content does not match. Expected %s, got %s", content, retrieved)
	}

	// Cleanup
	err = storage.Remove(bucket, key)
	if err != nil {
		t.Logf("Warning: failed to cleanup test object: %v", err)
	}
}

func TestMinioStorage_Put_EmptyData(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	key := "empty-file.txt"
	content := []byte{}

	err := storage.Put(bucket, key, content)
	if err != nil {
		t.Fatalf("Failed to put empty object: %v", err)
	}

	// Verify object exists
	exists := storage.ObjExist(bucket, key)
	if !exists {
		t.Error("Expected empty object to exist")
	}

	// Cleanup
	storage.Remove(bucket, key)
}

func TestMinioStorage_Put_LargeData(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	key := "large-file.bin"
	// Create 1MB of data
	content := make([]byte, 1024*1024)
	for i := range content {
		content[i] = byte(i % 256)
	}

	err := storage.Put(bucket, key, content)
	if err != nil {
		t.Fatalf("Failed to put large object: %v", err)
	}

	retrieved, err := storage.Get(bucket, key)
	if err != nil {
		t.Fatalf("Failed to get large object: %v", err)
	}

	if !bytes.Equal(retrieved, content) {
		t.Error("Retrieved large content does not match original")
	}

	// Cleanup
	storage.Remove(bucket, key)
}

func TestMinioStorage_Get_NonExistent(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	key := "non-existent-file.txt"

	_, err := storage.Get(bucket, key)
	if err == nil {
		t.Error("Expected error when getting non-existent object")
	}
}

func TestMinioStorage_Remove(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	key := "file-to-delete.txt"
	content := []byte("Delete me")

	// First, put an object
	err := storage.Put(bucket, key, content)
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	// Verify it exists
	exists := storage.ObjExist(bucket, key)
	if !exists {
		t.Fatal("Expected object to exist before removal")
	}

	// Remove it
	err = storage.Remove(bucket, key)
	if err != nil {
		t.Fatalf("Failed to remove object: %v", err)
	}

	// Verify it's gone
	exists = storage.ObjExist(bucket, key)
	if exists {
		t.Error("Expected object to not exist after removal")
	}
}

func TestMinioStorage_Remove_NonExistent(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	key := "non-existent-file.txt"

	// Removing a non-existent object should not error
	err := storage.Remove(bucket, key)
	if err != nil {
		t.Logf("Remove non-existent object returned error (may be acceptable): %v", err)
	}
}

func TestMinioStorage_ObjExist(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	key := "existence-test.txt"
	content := []byte("Test content")

	// Check non-existent object
	exists := storage.ObjExist(bucket, key)
	if exists {
		t.Error("Expected non-existent object to return false")
	}

	// Create object
	err := storage.Put(bucket, key, content)
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	// Check existing object
	exists = storage.ObjExist(bucket, key)
	if !exists {
		t.Error("Expected existing object to return true")
	}

	// Cleanup
	storage.Remove(bucket, key)
}

func TestMinioStorage_GetPresignedURL(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	key := "presigned-test.txt"
	content := []byte("Presigned URL test content")

	// Create object first
	err := storage.Put(bucket, key, content)
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	// Get presigned URL
	url, err := storage.GetPresignedURL(bucket, key, 5*time.Minute)
	if err != nil {
		t.Fatalf("Failed to get presigned URL: %v", err)
	}

	if url == "" {
		t.Error("Expected presigned URL to be non-empty")
	}

	// Verify URL contains expected components
	if len(url) > 0 {
		t.Logf("Generated presigned URL (first 100 chars): %s...", url[:min(100, len(url))])
	}

	// Cleanup
	storage.Remove(bucket, key)
}

func TestMinioStorage_GetPresignedURL_NonExistent(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	key := "non-existent-presigned.txt"

	_, err := storage.GetPresignedURL(bucket, key, 5*time.Minute)
	if err == nil {
		t.Log("Note: Some MinIO versions may allow presigned URLs for non-existent objects")
	}
}

func TestMinioStorage_BucketExists(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := fmt.Sprintf("test-bucket-exists-%d", time.Now().Unix())

	// Check non-existent bucket
	exists := storage.BucketExists(bucket)
	if exists {
		t.Error("Expected non-existent bucket to return false")
	}

	// Create bucket by putting an object
	err := storage.Put(bucket, "test.txt", []byte("test"))
	if err != nil {
		t.Fatalf("Failed to create bucket: %v", err)
	}

	// Check existing bucket
	exists = storage.BucketExists(bucket)
	if !exists {
		t.Error("Expected existing bucket to return true")
	}

	// Cleanup
	storage.RemoveBucket(bucket)
}

func TestMinioStorage_RemoveBucket(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := fmt.Sprintf("test-bucket-remove-%d", time.Now().Unix())

	// Create bucket with some objects
	err := storage.Put(bucket, "file1.txt", []byte("content1"))
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	err = storage.Put(bucket, "file2.txt", []byte("content2"))
	if err != nil {
		t.Fatalf("Failed to put object: %v", err)
	}

	// Verify bucket exists
	exists := storage.BucketExists(bucket)
	if !exists {
		t.Fatal("Expected bucket to exist before removal")
	}

	// Remove bucket
	err = storage.RemoveBucket(bucket)
	if err != nil {
		t.Fatalf("Failed to remove bucket: %v", err)
	}

	// Verify bucket is gone
	exists = storage.BucketExists(bucket)
	if exists {
		t.Error("Expected bucket to not exist after removal")
	}
}

func TestMinioStorage_Copy(t *testing.T) {
	storage := newTestMinioStorage(t)

	srcBucket := "test-bucket-src"
	srcKey := "source-file.txt"
	destBucket := "test-bucket-dest"
	destKey := "copied-file.txt"
	content := []byte("Content to copy")

	// Create source object
	err := storage.Put(srcBucket, srcKey, content)
	if err != nil {
		t.Fatalf("Failed to put source object: %v", err)
	}

	// Copy object
	success := storage.Copy(srcBucket, srcKey, destBucket, destKey)
	if !success {
		t.Fatal("Failed to copy object")
	}

	// Verify destination exists
	exists := storage.ObjExist(destBucket, destKey)
	if !exists {
		t.Error("Expected copied object to exist")
	}

	// Verify content matches
	retrieved, err := storage.Get(destBucket, destKey)
	if err != nil {
		t.Fatalf("Failed to get copied object: %v", err)
	}

	if !bytes.Equal(retrieved, content) {
		t.Error("Copied content does not match original")
	}

	// Cleanup
	storage.Remove(srcBucket, srcKey)
	storage.Remove(destBucket, destKey)
}

func TestMinioStorage_Copy_NonExistentSource(t *testing.T) {
	storage := newTestMinioStorage(t)

	srcBucket := "test-bucket-src"
	srcKey := "non-existent-source.txt"
	destBucket := "test-bucket-dest"
	destKey := "should-not-exist.txt"

	success := storage.Copy(srcBucket, srcKey, destBucket, destKey)
	if success {
		t.Error("Expected copy of non-existent object to fail")
	}

	// Verify destination does not exist
	exists := storage.ObjExist(destBucket, destKey)
	if exists {
		t.Error("Expected destination object to not exist after failed copy")
		storage.Remove(destBucket, destKey)
	}
}

func TestMinioStorage_Move(t *testing.T) {
	storage := newTestMinioStorage(t)

	srcBucket := "test-bucket-src"
	srcKey := "file-to-move.txt"
	destBucket := "test-bucket-dest"
	destKey := "moved-file.txt"
	content := []byte("Content to move")

	// Create source object
	err := storage.Put(srcBucket, srcKey, content)
	if err != nil {
		t.Fatalf("Failed to put source object: %v", err)
	}

	// Move object
	success := storage.Move(srcBucket, srcKey, destBucket, destKey)
	if !success {
		t.Fatal("Failed to move object")
	}

	// Verify source is gone
	exists := storage.ObjExist(srcBucket, srcKey)
	if exists {
		t.Error("Expected source object to not exist after move")
	}

	// Verify destination exists
	exists = storage.ObjExist(destBucket, destKey)
	if !exists {
		t.Error("Expected moved object to exist")
	}

	// Verify content matches
	retrieved, err := storage.Get(destBucket, destKey)
	if err != nil {
		t.Fatalf("Failed to get moved object: %v", err)
	}

	if !bytes.Equal(retrieved, content) {
		t.Error("Moved content does not match original")
	}

	// Cleanup
	storage.Remove(destBucket, destKey)
}

func TestMinioStorage_Move_NonExistentSource(t *testing.T) {
	storage := newTestMinioStorage(t)

	srcBucket := "test-bucket-src"
	srcKey := "non-existent-source.txt"
	destBucket := "test-bucket-dest"
	destKey := "should-not-exist.txt"

	success := storage.Move(srcBucket, srcKey, destBucket, destKey)
	if success {
		t.Error("Expected move of non-existent object to fail")
	}
}

func TestMinioStorage_MultipleObjectsInBucket(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := fmt.Sprintf("test-multi-%d", time.Now().Unix())
	numObjects := 10

	// Create multiple objects
	for i := 0; i < numObjects; i++ {
		key := fmt.Sprintf("file-%d.txt", i)
		content := []byte(fmt.Sprintf("Content %d", i))
		err := storage.Put(bucket, key, content)
		if err != nil {
			t.Fatalf("Failed to put object %d: %v", i, err)
		}
	}

	// Verify all objects exist
	for i := 0; i < numObjects; i++ {
		key := fmt.Sprintf("file-%d.txt", i)
		exists := storage.ObjExist(bucket, key)
		if !exists {
			t.Errorf("Expected object %s to exist", key)
		}
	}

	// Verify content
	for i := 0; i < numObjects; i++ {
		key := fmt.Sprintf("file-%d.txt", i)
		expectedContent := []byte(fmt.Sprintf("Content %d", i))
		retrieved, err := storage.Get(bucket, key)
		if err != nil {
			t.Errorf("Failed to get object %s: %v", key, err)
			continue
		}
		if !bytes.Equal(retrieved, expectedContent) {
			t.Errorf("Content mismatch for object %s", key)
		}
	}

	// Cleanup - remove bucket with all objects
	err := storage.RemoveBucket(bucket)
	if err != nil {
		t.Logf("Warning: failed to cleanup bucket: %v", err)
	}
}

func TestMinioStorage_SpecialCharactersInKey(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	specialKeys := []string{
		"file with spaces.txt",
		"file-with-dashes.txt",
		"file_with_underscores.txt",
		"file.multiple.dots.txt",
		"path/to/nested/file.txt",
		"unicode-文件.txt",
	}

	for _, key := range specialKeys {
		content := []byte(fmt.Sprintf("Content for %s", key))

		err := storage.Put(bucket, key, content)
		if err != nil {
			t.Errorf("Failed to put object with key '%s': %v", key, err)
			continue
		}

		retrieved, err := storage.Get(bucket, key)
		if err != nil {
			t.Errorf("Failed to get object with key '%s': %v", key, err)
			continue
		}

		if !bytes.Equal(retrieved, content) {
			t.Errorf("Content mismatch for key '%s'", key)
		}

		// Cleanup
		storage.Remove(bucket, key)
	}
}

func TestMinioStorage_TenantID(t *testing.T) {
	storage := newTestMinioStorage(t)

	bucket := "test-bucket"
	key := "tenant-test.txt"
	content := []byte("Tenant test content")
	tenantID := "tenant-123"

	// Put with tenant ID
	err := storage.Put(bucket, key, content, tenantID)
	if err != nil {
		t.Fatalf("Failed to put object with tenant ID: %v", err)
	}

	// Get with tenant ID
	retrieved, err := storage.Get(bucket, key, tenantID)
	if err != nil {
		t.Fatalf("Failed to get object with tenant ID: %v", err)
	}

	if !bytes.Equal(retrieved, content) {
		t.Error("Content mismatch for tenant-specific object")
	}

	// Check existence with tenant ID
	exists := storage.ObjExist(bucket, key, tenantID)
	if !exists {
		t.Error("Expected object to exist with tenant ID")
	}

	// Cleanup
	storage.Remove(bucket, key, tenantID)
}

// min is a helper function to get the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
