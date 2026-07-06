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
	addChunkFn      func(req *service.AddChunkRequest, userID string) (*service.AddChunkResponse, error)
	listFn          func(req *service.ListChunksRequest, userID string) (*service.ListChunksResponse, error)
	switchChunksFn  func(userID, datasetID, documentID string, availableInt int, chunkIDs []string) error
	updateChunkFn   func(req *service.UpdateChunkRequest, userID string) error
	stopParsingFn   func(userID, datasetID string, req service.StopParsingRequest) (*service.StopParsingResponse, common.ErrorCode, error)
}

type codedTestError struct {
	code common.ErrorCode
	msg  string
}

func (e codedTestError) Error() string {
	return e.msg
}

func (e codedTestError) Code() common.ErrorCode {
	return e.code
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
func (m *mockChunkSvc) StopParsing(userID, datasetID string, req service.StopParsingRequest) (*service.StopParsingResponse, common.ErrorCode, error) {
	if m.stopParsingFn != nil {
		return m.stopParsingFn(userID, datasetID, req)
	}
	panic("not implemented")
}
func (m *mockChunkSvc) Parse(string, string, *service.ParseFileRequest) (map[string]interface{}, common.ErrorCode, error) {
	panic("not implemented")
}
func (m *mockChunkSvc) AddChunk(req *service.AddChunkRequest, userID string) (*service.AddChunkResponse, error) {
	if m.addChunkFn != nil {
		return m.addChunkFn(req, userID)
	}
	return &service.AddChunkResponse{Chunk: map[string]interface{}{"id": "chunk-1"}}, nil
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

func setupChunkStopParsingTest(userID string) (*gin.Engine, *mockChunkSvc) {
	mock := &mockChunkSvc{}
	h := &ChunkHandler{chunkService: mock}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: userID})
	})
	r.DELETE("/api/v1/datasets/:dataset_id/chunks", h.StopParsing)
	return r, mock
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

func TestChunkHandlerListChunksMapsAvailableFalse(t *testing.T) {
	mock := &mockChunkSvc{}
	r, h := setupChunkHandlerWithUser("user-1", mock)
	r.GET("/api/v1/datasets/:dataset_id/documents/:document_id/chunks", h.ListChunks)

	mock.listFn = func(req *service.ListChunksRequest, userID string) (*service.ListChunksResponse, error) {
		if userID != "user-1" {
			t.Fatalf("userID = %q, want user-1", userID)
		}
		if req.AvailableInt == nil || *req.AvailableInt != 0 {
			t.Fatalf("available_int = %v, want 0", req.AvailableInt)
		}
		return &service.ListChunksResponse{
			Total:  0,
			Chunks: []map[string]interface{}{},
			Doc:    map[string]interface{}{"id": "doc-1"},
		}, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/datasets/kb-1/documents/doc-1/chunks?available=false", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
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

func TestChunkHandlerUpdateChunkValidationErrorIsBadRequest(t *testing.T) {
	mock := &mockChunkSvc{}
	r, h := setupChunkHandlerWithUser("user-1", mock)
	r.PATCH("/api/v1/datasets/:dataset_id/documents/:document_id/chunks/:chunk_id", h.UpdateChunk)

	mock.updateChunkFn = func(req *service.UpdateChunkRequest, userID string) error {
		return codedTestError{code: common.CodeArgumentError, msg: "`tag_feas` values must be finite numbers greater than 0"}
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/datasets/kb-1/documents/doc-1/chunks/chunk-1", strings.NewReader(`{"tag_feas":{"tag":0}}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
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

func TestChunkStopParsing_Success(t *testing.T) {
	r, mock := setupChunkStopParsingTest("user1")
	mock.stopParsingFn = func(userID, datasetID string, req service.StopParsingRequest) (*service.StopParsingResponse, common.ErrorCode, error) {
		if userID != "user1" {
			t.Fatalf("expected user1, got %q", userID)
		}
		if datasetID != "kb1" {
			t.Fatalf("expected kb1, got %q", datasetID)
		}
		if len(req.DocumentIDs) != 2 || req.DocumentIDs[0] != "doc1" || req.DocumentIDs[1] != "doc2" {
			t.Fatalf("unexpected document IDs: %#v", req.DocumentIDs)
		}
		return nil, common.CodeSuccess, nil
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/datasets/kb1/chunks", strings.NewReader(`{"document_ids":["doc1","doc2"]}`))
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
		t.Fatalf("expected code 0, got %v: %s", resp["code"], w.Body.String())
	}
	if resp["message"] != "success" {
		t.Fatalf("expected success message, got %v", resp["message"])
	}
}

func TestChunkStopParsingRouteRequiresDocumentIDs(t *testing.T) {
	r, mock := setupChunkStopParsingTest("user1")
	mock.stopParsingFn = func(userID, datasetID string, req service.StopParsingRequest) (*service.StopParsingResponse, common.ErrorCode, error) {
		t.Fatal("service should not be called when document_ids is missing")
		return nil, common.CodeSuccess, nil
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/datasets/kb1/chunks", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected data error, got %v: %s", resp["code"], w.Body.String())
	}
	if resp["message"] != "`document_ids` is required" {
		t.Fatalf("unexpected message: %v", resp["message"])
	}
}

func TestChunkStopParsing_InvalidStateIncludesPythonErrorCode(t *testing.T) {
	r, mock := setupChunkStopParsingTest("user1")
	mock.stopParsingFn = func(userID, datasetID string, req service.StopParsingRequest) (*service.StopParsingResponse, common.ErrorCode, error) {
		return &service.StopParsingResponse{
			Data: map[string]interface{}{"error_code": "DOC_STOP_PARSING_INVALID_STATE"},
		}, common.CodeDataError, errors.New("Can't stop parsing document that has not started or already completed")
	}

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/api/v1/datasets/kb1/chunks", strings.NewReader(`{"document_ids":["doc1"]}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected data error, got %v: %s", resp["code"], w.Body.String())
	}
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	if data["error_code"] != "DOC_STOP_PARSING_INVALID_STATE" {
		t.Fatalf("unexpected error_code: %v", data["error_code"])
	}
	if resp["message"] != "Can't stop parsing document that has not started or already completed" {
		t.Fatalf("unexpected message: %v", resp["message"])
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

type addChunkTestError struct {
	code common.ErrorCode
	msg  string
}

func (e addChunkTestError) Error() string          { return e.msg }
func (e addChunkTestError) Code() common.ErrorCode { return e.code }

func TestChunkHandlerAddChunkSuccess(t *testing.T) {
	mock := &mockChunkSvc{
		addChunkFn: func(req *service.AddChunkRequest, userID string) (*service.AddChunkResponse, error) {
			if userID != "user1" {
				t.Fatalf("userID = %q, want user1", userID)
			}
			if req.DatasetID != "kb1" || req.DocumentID != "doc1" || req.Content != "chunk body" {
				t.Fatalf("unexpected request: %#v", req)
			}
			return &service.AddChunkResponse{Chunk: map[string]interface{}{"id": "chunk-1", "content": req.Content}}, nil
		},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user1"})
	})
	h := &ChunkHandler{chunkService: mock}
	r.POST("/api/v1/datasets/:dataset_id/documents/:document_id/chunks", h.AddChunk)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/kb1/documents/doc1/chunks", strings.NewReader(`{"content":"chunk body"}`))
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
		t.Fatalf("expected success code, got %v", resp["code"])
	}
}

func TestChunkHandlerAddChunkPathIDsOverrideBody(t *testing.T) {
	mock := &mockChunkSvc{
		addChunkFn: func(req *service.AddChunkRequest, userID string) (*service.AddChunkResponse, error) {
			if req.DatasetID != "kb1" || req.DocumentID != "doc1" {
				t.Fatalf("path IDs were not preserved: %#v", req)
			}
			if req.Content != "chunk body" {
				t.Fatalf("unexpected content: %#v", req)
			}
			return &service.AddChunkResponse{Chunk: map[string]interface{}{"id": "chunk-1"}}, nil
		},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user1"})
	})
	h := &ChunkHandler{chunkService: mock}
	r.POST("/api/v1/datasets/:dataset_id/documents/:document_id/chunks", h.AddChunk)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/kb1/documents/doc1/chunks", strings.NewReader(`{"dataset_id":"evil-kb","document_id":"evil-doc","content":"chunk body"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestChunkHandlerAddChunkCodedError(t *testing.T) {
	mock := &mockChunkSvc{
		addChunkFn: func(req *service.AddChunkRequest, userID string) (*service.AddChunkResponse, error) {
			return nil, addChunkTestError{code: common.CodeDataError, msg: "`content` is required"}
		},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user1"})
	})
	h := &ChunkHandler{chunkService: mock}
	r.POST("/api/v1/datasets/:dataset_id/documents/:document_id/chunks", h.AddChunk)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/kb1/documents/doc1/chunks", strings.NewReader(`{"content":" "}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeDataError) {
		t.Fatalf("expected data error code, got %v", resp["code"])
	}
}

func TestChunkHandlerAddChunkValidatesListFields(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{
			name:    "important keywords type",
			body:    `{"content":"chunk body","important_keywords":{}}`,
			wantMsg: "`important_keywords` is required to be a list",
		},
		{
			name:    "tag kwd element type",
			body:    `{"content":"chunk body","tag_kwd":[1]}`,
			wantMsg: "`tag_kwd` must be a list of strings",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockChunkSvc{
				addChunkFn: func(req *service.AddChunkRequest, userID string) (*service.AddChunkResponse, error) {
					t.Fatal("service should not be called for invalid request")
					return nil, nil
				},
			}
			gin.SetMode(gin.TestMode)
			r := gin.New()
			r.Use(func(c *gin.Context) {
				c.Set("user", &entity.User{ID: "user1"})
			})
			h := &ChunkHandler{chunkService: mock}
			r.POST("/api/v1/datasets/:dataset_id/documents/:document_id/chunks", h.AddChunk)

			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/v1/datasets/kb1/documents/doc1/chunks", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)

			var resp map[string]interface{}
			if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp["message"] != tt.wantMsg {
				t.Fatalf("message = %v, want %q", resp["message"], tt.wantMsg)
			}
		})
	}
}

func TestChunkHandlerAddChunkHidesServerErrorDetails(t *testing.T) {
	mock := &mockChunkSvc{
		addChunkFn: func(req *service.AddChunkRequest, userID string) (*service.AddChunkResponse, error) {
			return nil, addChunkTestError{code: common.CodeServerError, msg: "encode chunk embedding: provider secret"}
		},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "user1"})
	})
	h := &ChunkHandler{chunkService: mock}
	r.POST("/api/v1/datasets/:dataset_id/documents/:document_id/chunks", h.AddChunk)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/datasets/kb1/documents/doc1/chunks", strings.NewReader(`{"content":"chunk body"}`))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["message"] != "Failed to add chunk" {
		t.Fatalf("message = %v, want generic failure", resp["message"])
	}
	if strings.Contains(w.Body.String(), "provider secret") {
		t.Fatalf("server error details leaked: %s", w.Body.String())
	}
}
