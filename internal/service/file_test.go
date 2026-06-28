package service

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
	exists     bool
}

func (f *fakeStorage) Health() bool {
	return true
}

func (f *fakeStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	f.lastBucket = bucket
	f.lastFnm = fnm
	f.blob = binary
	f.exists = true
	return f.err
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
	return f.exists && f.lastBucket == bucket && f.lastFnm == fnm
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

func TestFileService_UploadFromURL_PDFAddsExtensionAndStoresToDownloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/pdf")
		_, _ = w.Write([]byte("%PDF-1.7 fake pdf"))
	}))
	defer server.Close()

	origAssert := assertURLSafe
	origPinned := pinnedHTTPClient
	assertURLSafe = func(rawURL string) (string, string, error) {
		return "127.0.0.1", "127.0.0.1", nil
	}
	pinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		assertURLSafe = origAssert
		pinnedHTTPClient = origPinned
	})

	mockStorage := &fakeStorage{}
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(mockStorage)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	svc := NewFileService()
	resp, err := svc.UploadFromURL("tenant123", server.URL+"/report")
	if err != nil {
		t.Fatalf("UploadFromURL failed: %v", err)
	}

	if mockStorage.lastBucket != "tenant123-downloads" {
		t.Fatalf("bucket = %q", mockStorage.lastBucket)
	}
	if resp["name"] != "report.pdf" {
		t.Fatalf("name = %#v, want report.pdf", resp["name"])
	}
	if resp["mime_type"] != "application/pdf" {
		t.Fatalf("mime_type = %#v", resp["mime_type"])
	}
	if resp["id"] != mockStorage.lastFnm {
		t.Fatalf("id = %#v, stored key = %q", resp["id"], mockStorage.lastFnm)
	}
}

func TestFileService_UploadFromURL_HTMLNormalizesReadableContent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><script>bad()</script></head><body><div>Hello</div><p>World</p></body></html>`))
	}))
	defer server.Close()

	origAssert := assertURLSafe
	origPinned := pinnedHTTPClient
	assertURLSafe = func(rawURL string) (string, string, error) {
		return "127.0.0.1", "127.0.0.1", nil
	}
	pinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		assertURLSafe = origAssert
		pinnedHTTPClient = origPinned
	})

	mockStorage := &fakeStorage{}
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(mockStorage)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	svc := NewFileService()
	resp, err := svc.UploadFromURL("tenant123", server.URL+"/page")
	if err != nil {
		t.Fatalf("UploadFromURL failed: %v", err)
	}

	stored := string(mockStorage.blob)
	if strings.Contains(strings.ToLower(stored), "<html") || strings.Contains(strings.ToLower(stored), "<script") {
		t.Fatalf("stored html was not normalized: %q", stored)
	}
	if !strings.Contains(stored, "Hello") || !strings.Contains(stored, "World") {
		t.Fatalf("stored normalized text missing content: %q", stored)
	}
	if resp["mime_type"] != "text/html" {
		t.Fatalf("mime_type = %#v", resp["mime_type"])
	}
}

func TestNormalizeUploadInfoContent_PDFTakesPrecedenceOverHTML(t *testing.T) {
	filename, contentType, data := normalizeUploadInfoContent(
		"report",
		"text/html",
		[]byte("%PDF-1.7 fake pdf"),
	)
	if filename != "report.pdf" {
		t.Fatalf("filename = %q, want report.pdf", filename)
	}
	if contentType != "application/pdf" {
		t.Fatalf("contentType = %q, want application/pdf", contentType)
	}
	if !bytes.Equal(data, []byte("%PDF-1.7 fake pdf")) {
		t.Fatalf("pdf bytes were unexpectedly transformed: %q", string(data))
	}
}

func TestReadUploadInfoData_RejectsOversizedInput(t *testing.T) {
	reader := io.LimitReader(zeroReader{}, maxRemoteFileSize+1)
	_, err := readUploadInfoData(reader)
	if err == nil {
		t.Fatal("expected oversized input to be rejected")
	}
	if !strings.Contains(err.Error(), "file size exceeds") {
		t.Fatalf("err = %v, want size limit message", err)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
