package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
	"ragflow/internal/storage"
)

type fakeHandlerArtifactStorage struct {
	data []byte
}

func (s *fakeHandlerArtifactStorage) Health() bool { return true }

func (s *fakeHandlerArtifactStorage) Put(bucket, fnm string, binary []byte, tenantID ...string) error {
	return nil
}

func (s *fakeHandlerArtifactStorage) Get(bucket, fnm string, tenantID ...string) ([]byte, error) {
	return s.data, nil
}

func (s *fakeHandlerArtifactStorage) Remove(bucket, fnm string, tenantID ...string) error { return nil }

func (s *fakeHandlerArtifactStorage) ObjExist(bucket, fnm string, tenantID ...string) bool {
	return true
}

func (s *fakeHandlerArtifactStorage) GetPresignedURL(bucket, fnm string, expires time.Duration, tenantID ...string) (string, error) {
	return "", nil
}

func (s *fakeHandlerArtifactStorage) BucketExists(bucket string) bool { return true }

func (s *fakeHandlerArtifactStorage) RemoveBucket(bucket string) error { return nil }

func (s *fakeHandlerArtifactStorage) Copy(srcBucket, srcPath, destBucket, destPath string) bool {
	return true
}

func (s *fakeHandlerArtifactStorage) Move(srcBucket, srcPath, destBucket, destPath string) bool {
	return true
}

func setupArtifactHandlerTest(t *testing.T, data []byte) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	factory := storage.GetStorageFactory()
	prev := factory.GetStorage()
	factory.SetStorage(&fakeHandlerArtifactStorage{data: data})
	t.Cleanup(func() {
		factory.SetStorage(prev)
	})

	h := &DocumentHandler{documentService: &service.DocumentService{}}
	r := gin.New()
	r.GET("/api/v1/documents/artifact/:filename", h.GetDocumentArtifact)
	return r
}

func TestGetDocumentArtifactReturnsInlineFile(t *testing.T) {
	r := setupArtifactHandlerTest(t, []byte("png bytes"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/artifact/chart.png", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}
	if w.Body.String() != "png bytes" {
		t.Fatalf("unexpected response body %q", w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "image/png" {
		t.Fatalf("expected image/png content type, got %q", got)
	}
	if got := w.Header().Get("Content-Disposition"); got != `inline; filename="chart.png"` {
		t.Fatalf("expected inline content disposition, got %q", got)
	}
}

func TestGetDocumentArtifactForcesAttachmentForHTML(t *testing.T) {
	r := setupArtifactHandlerTest(t, []byte("<html></html>"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/artifact/page.html", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "text/html" {
		t.Fatalf("expected text/html content type, got %q", got)
	}
	if got := w.Header().Get("Content-Disposition"); got != "attachment" {
		t.Fatalf("expected attachment content disposition, got %q", got)
	}
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("expected nosniff header, got %q", got)
	}
}

func TestGetDocumentArtifactDataErrorsMatchPythonEnvelope(t *testing.T) {
	r := setupArtifactHandlerTest(t, []byte("data"))

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/documents/artifact/script.sh", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected code %d, got %#v", common.CodeDataError, body["code"])
	}
	if body["message"] != "Invalid file type." {
		t.Fatalf("expected invalid file type message, got %#v", body["message"])
	}
	if _, ok := body["data"]; ok {
		t.Fatalf("python get_data_error_result does not include data, got %#v", body["data"])
	}
}
