package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
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
	listFn          func(req *service.ListChunksRequest, userID string) (*service.ListChunksResponse, error)
	switchChunksFn  func(userID, datasetID, documentID string, availableInt int, chunkIDs []string) error
	updateChunkFn   func(req *service.UpdateChunkRequest, userID string) error
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
func (m *mockChunkSvc) List(req *service.ListChunksRequest, userID string) (*service.ListChunksResponse, error) {
	if m.listFn != nil {
		return m.listFn(req, userID)
	}
	panic("not implemented")
}
func (m *mockChunkSvc) SwitchChunks(userID, datasetID, documentID string, availableInt int, chunkIDs []string) error {
	if m.switchChunksFn != nil {
		return m.switchChunksFn(userID, datasetID, documentID, availableInt, chunkIDs)
	}
	panic("not implemented")
}
func (m *mockChunkSvc) UpdateChunk(req *service.UpdateChunkRequest, userID string) error {
	if m.updateChunkFn != nil {
		return m.updateChunkFn(req, userID)
	}
	panic("not implemented")
}
func (m *mockChunkSvc) RemoveChunks(*service.RemoveChunksRequest, string) (int64, error) {
	panic("not implemented")
}
func (m *mockChunkSvc) StopParsing(string, string, service.StopParsingRequest) (*service.StopParsingResponse, common.ErrorCode, error) {
	panic("not implemented")
}
func (m *mockChunkSvc) Parse(string, string, *service.ParseFileRequest) (map[string]interface{}, common.ErrorCode, error) {
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

func setupChunkHandlerWithUser(userID string, mock *mockChunkSvc) (*gin.Engine, *ChunkHandler) {
	h := &ChunkHandler{chunkService: mock}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: userID})
	})
	return r, h
}

func TestChunkHandlerListChunksMapsPathAndQuery(t *testing.T) {
	mock := &mockChunkSvc{}
	r, h := setupChunkHandlerWithUser("user-1", mock)
	r.GET("/api/v1/datasets/:dataset_id/documents/:document_id/chunks", h.ListChunks)

	mock.listFn = func(req *service.ListChunksRequest, userID string) (*service.ListChunksResponse, error) {
		if userID != "user-1" {
			t.Fatalf("userID = %q, want user-1", userID)
		}
		if req.DatasetID != "kb-1" || req.DocID != "doc-1" {
			t.Fatalf("req ids = %q/%q, want kb-1/doc-1", req.DatasetID, req.DocID)
		}
		if req.Page == nil || *req.Page != 2 {
			t.Fatalf("page = %v, want 2", req.Page)
		}
		if req.Size == nil || *req.Size != 5 {
			t.Fatalf("size = %v, want 5", req.Size)
		}
		if req.Keywords != "AI" {
			t.Fatalf("keywords = %q, want AI", req.Keywords)
		}
		if req.AvailableInt == nil || *req.AvailableInt != 1 {
			t.Fatalf("available_int = %v, want 1", req.AvailableInt)
		}
		return &service.ListChunksResponse{
			Total: 1,
			Chunks: []map[string]interface{}{
				{"id": "chunk-1"},
			},
			Doc: map[string]interface{}{"id": "doc-1"},
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/kb-1/documents/doc-1/chunks?page=2&page_size=5&keywords=AI&available=true", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if body["message"] != "success" {
		t.Fatalf("message = %v, want success", body["message"])
	}
}

func TestChunkHandlerSwitchChunksCallsService(t *testing.T) {
	mock := &mockChunkSvc{}
	r, h := setupChunkHandlerWithUser("user-1", mock)
	r.PATCH("/api/v1/datasets/:dataset_id/documents/:document_id/chunks", h.SwitchChunks)

	mock.switchChunksFn = func(userID, datasetID, documentID string, availableInt int, chunkIDs []string) error {
		if userID != "user-1" || datasetID != "kb-1" || documentID != "doc-1" {
			t.Fatalf("ids = %q/%q/%q, want user-1/kb-1/doc-1", userID, datasetID, documentID)
		}
		if availableInt != 0 {
			t.Fatalf("availableInt = %d, want 0", availableInt)
		}
		if !reflect.DeepEqual(chunkIDs, []string{"chunk-1", "chunk-2"}) {
			t.Fatalf("chunkIDs = %#v", chunkIDs)
		}
		return nil
	}

	body := `{"chunk_ids":["chunk-1","chunk-2"],"available":false}`
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/datasets/kb-1/documents/doc-1/chunks", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var res map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("invalid JSON response: %v", err)
	}
	if res["data"] != true {
		t.Fatalf("data = %v, want true", res["data"])
	}
}

func TestChunkHandlerSwitchChunksRejectsMissingChunkIDs(t *testing.T) {
	mock := &mockChunkSvc{}
	r, h := setupChunkHandlerWithUser("user-1", mock)
	r.PATCH("/api/v1/datasets/:dataset_id/documents/:document_id/chunks", h.SwitchChunks)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/datasets/kb-1/documents/doc-1/chunks", strings.NewReader(`{"available":true}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
}

func TestChunkHandlerUpdateChunkUsesPathIDs(t *testing.T) {
	mock := &mockChunkSvc{}
	r, h := setupChunkHandlerWithUser("user-1", mock)
	r.PATCH("/api/v1/datasets/:dataset_id/documents/:document_id/chunks/:chunk_id", h.UpdateChunk)

	mock.updateChunkFn = func(req *service.UpdateChunkRequest, userID string) error {
		if userID != "user-1" {
			t.Fatalf("userID = %q, want user-1", userID)
		}
		if req.DatasetID != "kb-1" || req.DocumentID != "doc-1" || req.ChunkID != "chunk-1" {
			t.Fatalf("ids = %q/%q/%q, want kb-1/doc-1/chunk-1", req.DatasetID, req.DocumentID, req.ChunkID)
		}
		if req.Content == nil || *req.Content != "updated" {
			t.Fatalf("content = %v, want updated", req.Content)
		}
		return nil
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/datasets/kb-1/documents/doc-1/chunks/chunk-1", strings.NewReader(`{"content":"updated"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
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
