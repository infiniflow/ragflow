package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// mockChunkSvc implements chunkSvcIface for testing ChunkHandler.
// Only the methods actually called by the test are set; others panic.
type mockChunkSvc struct {
	retrievalTestFn func(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error)
}

func (m *mockChunkSvc) RetrievalTest(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error) {
	if m.retrievalTestFn != nil {
		return m.retrievalTestFn(req, userID)
	}
	return &service.RetrievalTestResponse{
		Chunks: []map[string]interface{}{{"docnm_kwd": "test", "content_with_weight": "content"}},
		Total:  1,
	}, nil
}
func (m *mockChunkSvc) Get(*service.GetChunkRequest, string) (*service.GetChunkResponse, error) {
	panic("not implemented")
}
func (m *mockChunkSvc) List(*service.ListChunksRequest, string) (*service.ListChunksResponse, error) {
	panic("not implemented")
}
func (m *mockChunkSvc) UpdateChunk(*service.UpdateChunkRequest, string) error {
	panic("not implemented")
}
func (m *mockChunkSvc) RemoveChunks(*service.RemoveChunksRequest, string) (int64, error) {
	panic("not implemented")
}

func setupChunkRetrievalTest(userID string) (*gin.Engine, *mockChunkSvc) {
	mock := &mockChunkSvc{}
	h := &ChunkHandler{chunkService: mock}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: userID})
	})
	r.POST("/api/v1/datasets/search", h.RetrievalTest)
	return r, mock
}

func setupChunkRetrievalTestNoAuth() *gin.Engine {
	// Returns a router without the user middleware — used for error-path
	// tests that don't call the service.
	h := &ChunkHandler{}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/datasets/search", h.RetrievalTest)
	return r
}

func TestChunkRetrieval_EmptyQuestion(t *testing.T) {
	r, _ := setupChunkRetrievalTest("user1")

	body := `{"dataset_ids": ["kb1"], "question": ""}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected code 0, got %v: %q", resp["code"], resp["message"])
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data to be object, got %T", resp["data"])
	}
	chunks, _ := data["chunks"].([]interface{})
	if chunks == nil || len(chunks) != 0 {
		t.Errorf("expected empty chunks array, got %v", chunks)
	}
	if total, _ := data["total"].(float64); total != 0 {
		t.Errorf("expected total 0, got %v", total)
	}
}

func TestChunkRetrieval_WhitespaceQuestion(t *testing.T) {
	r, _ := setupChunkRetrievalTest("user1")

	body := `{"dataset_ids": ["kb1"], "question": "   "}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected code 0, got %v", resp["code"])
	}
}

func TestChunkRetrieval_TopKZero(t *testing.T) {
	r, _ := setupChunkRetrievalTest("user1")

	body := `{"dataset_ids": ["kb1"], "question": "test", "top_k": 0}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if msg, _ := resp["message"].(string); msg != "top_k must be greater than 0" {
		t.Errorf("expected 'top_k must be greater than 0', got %q", msg)
	}
}

func TestChunkRetrieval_MissingDatasetIDs(t *testing.T) {
	r, _ := setupChunkRetrievalTest("user1")

	body := `{"question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChunkRetrieval_EmptyDatasetIDs(t *testing.T) {
	r, _ := setupChunkRetrievalTest("user1")

	body := `{"dataset_ids": [], "question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if msg, _ := resp["message"].(string); msg != "kb_id array cannot be empty" {
		t.Errorf("expected 'kb_id array cannot be empty', got %q", msg)
	}
}

func TestChunkRetrieval_NoAuth(t *testing.T) {
	r := setupChunkRetrievalTestNoAuth()

	body := `{"dataset_ids": ["kb1"], "question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	// jsonError returns HTTP 200 with error code in body
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] == float64(common.CodeSuccess) {
		t.Errorf("expected error code, got %v", resp["code"])
	}
}

func TestChunkRetrieval_InvalidJSON(t *testing.T) {
	r, _ := setupChunkRetrievalTest("user1")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/search", strings.NewReader("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChunkRetrieval_Success(t *testing.T) {
	_, mock := setupChunkRetrievalTest("user1")
	mock.retrievalTestFn = func(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error) {
		if userID != "user1" {
			t.Errorf("expected userID 'user1', got %q", userID)
		}
		return &service.RetrievalTestResponse{
			Chunks:  []map[string]interface{}{{"docnm_kwd": "result"}},
			DocAggs: []map[string]interface{}{{"doc_id": "1", "count": float64(1)}},
			Total:   1,
		}, nil
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user1"})
	})
	h := &ChunkHandler{chunkService: mock}
	r.POST("/api/v1/datasets/search", h.RetrievalTest)

	body := `{"dataset_ids": ["kb1"], "question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Fatalf("expected code 0, got %v: %q", resp["code"], resp["message"])
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	if total, _ := data["total"].(float64); total != 1 {
		t.Errorf("expected total 1, got %v", total)
	}
}

func TestChunkRetrieval_ServiceError(t *testing.T) {
	_, mock := setupChunkRetrievalTest("user1")
	mock.retrievalTestFn = func(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error) {
		return nil, errors.New("db connection refused")
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user1"})
	})
	h := &ChunkHandler{chunkService: mock}
	r.POST("/api/v1/datasets/search", h.RetrievalTest)

	body := `{"dataset_ids": ["kb1"], "question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/search", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	msg, _ := resp["message"].(string)
	if msg != "dataset search failed" {
		t.Errorf("expected generic error message, got %q", msg)
	}
	if strings.Contains(msg, "db connection refused") {
		t.Errorf("internal error details leaked to response: %q", msg)
	}
}
