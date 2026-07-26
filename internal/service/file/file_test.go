package file

import (
	"bytes"
	"context"

	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
)

// fakeStorage mocks storage.Storage for testing DownloadAgentFile.
type fakeStorage struct {
	lastBucket string
	lastFnm    string
	blob       []byte
	err        error
	exists     bool
	getCalls   int
}

// sptr returns a pointer to the given string.
func sptr(s string) *string { return &s }

// testFilePerm controls the permission check returned by testFileService.
// Tests that need to simulate denied access can set it to a function that
// returns false.
var testFilePerm CheckFilePermFunc = func(_ context.Context, _ *dao.FileDAO, _ *entity.File, _ string) bool { return true }

func testFileService() *FileService {
	return &FileService{
		fileDAO:          dao.NewFileDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		documentService:  nil,
		checkFilePerm:    testFilePerm,
	}
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
	f.getCalls++
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

func (f *fakeStorage) ListObjects(bucket string, tenantID ...string) ([]string, error) {
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

func (f *fakeStorage) Close() error { return nil }

func TestFileService_GetFileContents_NotAccessible(t *testing.T) {
	memory := storage.NewMemoryStorage()
	if err := memory.Put("other-user-downloads", "loc-1", []byte("secret")); err != nil {
		t.Fatalf("put: %v", err)
	}
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(memory)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	svc := testFileService()
	texts, images, err := svc.GetFileContents(t.Context(), "user-1", []map[string]interface{}{{
		"id":         "loc-1",
		"name":       "secret.txt",
		"mime_type":  "text/plain",
		"created_by": "other-user",
	}}, false)
	if err == nil {
		t.Fatal("expected authorization error")
	}
	if err.Error() != "No authorization." {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(texts) != 0 || len(images) != 0 {
		t.Fatalf("expected no content, got texts=%v images=%v", texts, images)
	}
}

func TestFileService_GetFileContents_Accessible(t *testing.T) {
	memory := storage.NewMemoryStorage()
	if err := memory.Put("user-1-downloads", "loc-1", []byte("allowed content")); err != nil {
		t.Fatalf("put: %v", err)
	}
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(memory)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	svc := testFileService()
	texts, images, err := svc.GetFileContents(t.Context(), "user-1", []map[string]interface{}{{
		"id":         "loc-1",
		"name":       "doc.txt",
		"mime_type":  "text/plain",
		"created_by": "user-1",
	}}, false)
	if err != nil {
		t.Fatalf("GetFileContents failed: %v", err)
	}
	if len(images) != 0 {
		t.Fatalf("expected no images, got %v", images)
	}
	if len(texts) != 1 || !strings.Contains(texts[0], "allowed content") {
		t.Fatalf("unexpected texts: %v", texts)
	}
}

func TestFileService_ParseAgentUploads_TextAndImageInRequestOrder(t *testing.T) {
	ctx := t.Context()
	memory := storage.NewMemoryStorage()
	if err := memory.Put("user-1-downloads", "text-id", []byte("uploaded text")); err != nil {
		t.Fatalf("put text: %v", err)
	}
	if err := memory.Put("user-1-downloads", "image-id", []byte("png")); err != nil {
		t.Fatalf("put image: %v", err)
	}
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(memory)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	contents, err := testFileService().ParseAgentUploads(ctx, "user-1", []map[string]interface{}{
		{"id": "text-id", "name": "notes.txt", "mime_type": "text/plain", "created_by": "user-1"},
		{"id": "image-id", "name": "photo.bin", "mime_type": "image/png", "created_by": "user-1"},
	}, "Plain Text")
	if err != nil {
		t.Fatalf("ParseAgentUploads: %v", err)
	}
	if len(contents) != 2 {
		t.Fatalf("contents length = %d, want 2", len(contents))
	}
	if !strings.Contains(contents[0], "File: notes.txt") || !strings.Contains(contents[0], "uploaded text") {
		t.Fatalf("unexpected text content: %q", contents[0])
	}
	if contents[1] != "data:image/png;base64,cG5n" {
		t.Fatalf("unexpected image content: %q", contents[1])
	}
}

func TestFileService_ParseAgentUploads_RejectsForeignOwner(t *testing.T) {
	ctx := t.Context()
	memory := storage.NewMemoryStorage()
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(memory)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	_, err := testFileService().ParseAgentUploads(ctx, "user-1", []map[string]interface{}{
		{"id": "file-id", "name": "secret.txt", "mime_type": "text/plain", "created_by": "user-2"},
	}, "")
	if err == nil || !strings.Contains(err.Error(), "created_by does not match") {
		t.Fatalf("error = %v, want created_by mismatch", err)
	}
}

func TestFileService_ParseAgentUploads_MissingObjectFails(t *testing.T) {
	ctx := t.Context()
	memory := storage.NewMemoryStorage()
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(memory)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	_, err := testFileService().ParseAgentUploads(ctx, "user-1", []map[string]interface{}{
		{"id": "missing", "name": "missing.txt", "mime_type": "text/plain", "created_by": "user-1"},
	}, "")
	if err == nil || !strings.Contains(err.Error(), "read upload") {
		t.Fatalf("error = %v, want storage read failure", err)
	}
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

	svc := testFileService()
	tenantID := "tenant123"
	location := "file-abc.txt"

	ctx := t.Context()
	blob, err := svc.DownloadAgentFile(ctx, tenantID, location)
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

	svc := testFileService()
	tenantID := "tenant123"
	location := "file-abc.txt"

	ctx := t.Context()
	blob, err := svc.DownloadAgentFile(ctx, tenantID, location)
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

	origAssert := utility.AssertURLSafe
	origPinned := utility.PinnedHTTPClient
	utility.AssertURLSafe = func(rawURL string) (string, string, error) {
		return "127.0.0.1", "127.0.0.1", nil
	}
	utility.PinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		utility.AssertURLSafe = origAssert
		utility.PinnedHTTPClient = origPinned
	})

	mockStorage := &fakeStorage{}
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(mockStorage)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	ctx := t.Context()
	svc := testFileService()
	resp, err := svc.UploadFromURL(ctx, "tenant123", server.URL+"/report")
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

	origAssert := utility.AssertURLSafe
	origPinned := utility.PinnedHTTPClient
	utility.AssertURLSafe = func(rawURL string) (string, string, error) {
		return "127.0.0.1", "127.0.0.1", nil
	}
	utility.PinnedHTTPClient = func(hostname, resolvedIP string, timeout time.Duration) *http.Client {
		return server.Client()
	}
	t.Cleanup(func() {
		utility.AssertURLSafe = origAssert
		utility.PinnedHTTPClient = origPinned
	})

	mockStorage := &fakeStorage{}
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(mockStorage)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	ctx := t.Context()
	svc := testFileService()
	resp, err := svc.UploadFromURL(ctx, "tenant123", server.URL+"/page")
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
	filename, contentType, data := utility.NormalizeUploadInfoContent(
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

// zeroReader is an io.Reader that returns an infinite stream of zero bytes.
type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
