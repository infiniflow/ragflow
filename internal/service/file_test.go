package service

import (
	"bytes"
	"errors"
	"net"
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

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"report.pdf", "report.pdf"},
		{"../../../etc/passwd", "passwd"},
		{"file:with:colons.txt", "file_with_colons.txt"},
		{"", "download"},
		{".", "download"},
		{"..", "download"},
		{"CON.txt", "download"},
		{"NUL", "download"},
		{"  trimmed  ", "trimmed"},
		{"..hidden", "hidden"},
		{strings.Repeat("a", 260), strings.Repeat("a", 255)},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := sanitizeFilename(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsPublicIP(t *testing.T) {
	public := []string{
		"8.8.8.8",
		"1.1.1.1",
		"93.184.216.34",
	}
	private := []string{
		"127.0.0.1",
		"10.0.0.1",
		"192.168.1.1",
		"172.16.0.1",
		"::1",
		"169.254.1.1",
		"100.64.0.1",
	}

	for _, addr := range public {
		if !isPublicIP(net.ParseIP(addr)) {
			t.Errorf("expected %s to be public", addr)
		}
	}
	for _, addr := range private {
		if isPublicIP(net.ParseIP(addr)) {
			t.Errorf("expected %s to be non-public", addr)
		}
	}
}

func TestUploadFromURL_InvalidURL(t *testing.T) {
	svc := NewFileService()

	tests := []struct {
		name    string
		rawURL  string
		wantErr string
	}{
		{"empty", "", "url is required"},
		{"no scheme", "notaurl", "invalid or unsafe URL"},
		{"ftp scheme", "ftp://example.com/file.txt", "invalid or unsafe URL"},
		{"no host", "http:///path", "invalid or unsafe URL"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.UploadFromURL("tenant-1", tt.rawURL)
			if err == nil {
				t.Fatalf("expected error for %q, got nil", tt.rawURL)
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error=%q want=%q", err.Error(), tt.wantErr)
			}
		})
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
