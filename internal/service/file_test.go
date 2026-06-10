package service

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"ragflow/internal/storage"
)

// fakeStorage mocks storage.Storage for testing DownloadAgentFile.
type fakeStorage struct {
	lastBucket string
	lastFnm    string
	blob       []byte
	err        error
}

func (f *fakeStorage) Health() bool {
	return true
}

func (f *fakeStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	panic("not implemented in fakeStorage")
}

func (f *fakeStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	f.lastBucket = bucket
	f.lastFnm = fnm
	return f.blob, f.err
}

func (f *fakeStorage) Remove(bucket, fnm string, tenantID ...string) error {
	panic("not implemented in fakeStorage")
}

func (f *fakeStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	panic("not implemented in fakeStorage")
}

func (f *fakeStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	panic("not implemented in fakeStorage")
}

func (f *fakeStorage) BucketExists(bucket string) bool {
	panic("not implemented in fakeStorage")
}

func (f *fakeStorage) RemoveBucket(bucket string) error {
	panic("not implemented in fakeStorage")
}

func (f *fakeStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	panic("not implemented in fakeStorage")
}

func (f *fakeStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	panic("not implemented in fakeStorage")
}

func TestFileService_DownloadAgentFile_Success(t *testing.T) {
	// Setup mock storage
	expectedBlob := []byte("fake file content")
	mockStorage := &fakeStorage{
		blob: expectedBlob,
		err:  nil,
	}

	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(mockStorage)
	t.Cleanup(func() {
		factory.SetStorage(originalStorage)
	})

	svc := NewFileService()
	tenantID := "tenant123"
	location := "file-abc.txt"

	blob, err := svc.DownloadAgentFile(tenantID, location)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if mockStorage.lastBucket != "tenant123-downloads" {
		t.Errorf("expected bucket 'tenant123-downloads', got %q", mockStorage.lastBucket)
	}
	if mockStorage.lastFnm != location {
		t.Errorf("expected fnm %q, got %q", location, mockStorage.lastFnm)
	}
	if !bytes.Equal(blob, expectedBlob) {
		t.Errorf("expected blob %v, got %v", expectedBlob, blob)
	}
}

func TestFileService_DownloadAgentFile_Error(t *testing.T) {
	// Setup mock storage
	expectedErr := errors.New("not found")
	mockStorage := &fakeStorage{
		blob: nil,
		err:  expectedErr,
	}

	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(mockStorage)
	t.Cleanup(func() {
		factory.SetStorage(originalStorage)
	})

	svc := NewFileService()
	tenantID := "tenant123"
	location := "file-abc.txt"

	blob, err := svc.DownloadAgentFile(tenantID, location)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	if blob != nil {
		t.Errorf("expected nil blob, got %v", blob)
	}
}
