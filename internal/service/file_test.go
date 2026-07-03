package service

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
)

// fakeStorage mocks storage.Storage for testing DownloadAgentFile.
type fakeStorage struct {
	lastBucket string
	lastFnm    string
	blob       []byte
	err        error
	exists     bool
}

func testFileService() *FileService {
	return &FileService{
		fileDAO:          dao.NewFileDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		documentService:  &DocumentService{},
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

	svc := testFileService()
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

	svc := testFileService()
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

	svc := testFileService()
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

	svc := testFileService()
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

func TestFileService_DeleteSingleFile_RemovesLinkedDocumentThroughDocumentService(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	mockStorage := newFakeUploadStorage()
	factory := storage.GetStorageFactory()
	originalStorage := factory.GetStorage()
	factory.SetStorage(mockStorage)
	t.Cleanup(func() { factory.SetStorage(originalStorage) })

	insertTestKB(t, "kb-file-delete", "tenant-1", 1, 30, 10)
	insertTestDoc(t, "doc-file-delete", "kb-file-delete", 30, 10)
	insertTestTask(t, "task-file-delete", "doc-file-delete")

	location := "doc.pdf"
	insertTestFile(t, "file-delete", "folder-delete", "doc.pdf", &location)
	insertTestFile2Document(t, "f2d-file-delete", "file-delete", "doc-file-delete")
	if err := mockStorage.Put("folder-delete", location, []byte("blob")); err != nil {
		t.Fatalf("put test blob: %v", err)
	}

	file, err := dao.NewFileDAO().GetByID("file-delete")
	if err != nil {
		t.Fatalf("get file: %v", err)
	}
	docSvc := testDocumentService(t)
	docEngine := &rerunDeleteDocEngine{}
	docSvc.docEngine = docEngine
	svc := &FileService{
		fileDAO:          dao.NewFileDAO(),
		file2DocumentDAO: dao.NewFile2DocumentDAO(),
		documentService:  docSvc,
	}

	if err := svc.deleteSingleFile(context.Background(), file); err != nil {
		t.Fatalf("deleteSingleFile failed: %v", err)
	}

	if mockStorage.ObjExist("folder-delete", location) {
		t.Fatal("expected storage object to be removed")
	}
	if _, err := dao.NewDocumentDAO().GetByID("doc-file-delete"); err == nil {
		t.Fatal("expected linked document to be deleted")
	}
	var taskCount int64
	if err := dao.DB.Model(&entity.Task{}).Where("doc_id = ?", "doc-file-delete").Count(&taskCount).Error; err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	if taskCount != 0 {
		t.Fatalf("expected linked tasks to be deleted, got %d", taskCount)
	}
	mappings, err := dao.NewFile2DocumentDAO().GetByFileID("file-delete")
	if err != nil {
		t.Fatalf("get file2document mappings: %v", err)
	}
	if len(mappings) != 0 {
		t.Fatalf("expected file2document mappings to be deleted, got %d", len(mappings))
	}
	files, err := dao.NewFileDAO().GetByIDs([]string{"file-delete"})
	if err != nil {
		t.Fatalf("get deleted file: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected file record to be deleted, got %d", len(files))
	}
	if docEngine.deleteCalls != 1 {
		t.Fatalf("expected document engine cleanup to be called once, got %d", docEngine.deleteCalls)
	}
	if docEngine.indexName != "ragflow_tenant-1" {
		t.Fatalf("expected document engine index ragflow_tenant-1, got %q", docEngine.indexName)
	}
	if docEngine.datasetID != "kb-file-delete" {
		t.Fatalf("expected document engine dataset kb-file-delete, got %q", docEngine.datasetID)
	}
	if docEngine.condition["doc_id"] != "doc-file-delete" {
		t.Fatalf("expected document engine doc_id condition doc-file-delete, got %v", docEngine.condition["doc_id"])
	}
	kb, err := dao.NewKnowledgebaseDAO().GetByID("kb-file-delete")
	if err != nil {
		t.Fatalf("get kb: %v", err)
	}
	if kb.DocNum != 0 || kb.TokenNum != 0 || kb.ChunkNum != 0 {
		t.Fatalf("expected KB counters to be decremented to zero, got doc=%d token=%d chunk=%d", kb.DocNum, kb.TokenNum, kb.ChunkNum)
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}
