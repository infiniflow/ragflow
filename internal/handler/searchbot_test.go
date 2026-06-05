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

// mockChunkService implements ChunkServiceIface for testing.
type mockChunkService struct {
	retrievalTestFn func(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error)
}

func (m *mockChunkService) RetrievalTest(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error) {
	if m.retrievalTestFn != nil {
		return m.retrievalTestFn(req, userID)
	}
	return &service.RetrievalTestResponse{
		Chunks: []map[string]interface{}{{"docnm_kwd": "test", "content_with_weight": "content"}},
	}, nil
}

func setupSearchbotsTest(userID string) (*SearchbotHandler, *gin.Engine) {
	h := &SearchbotHandler{
		chunkSvc: &mockChunkService{},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: userID})
	})
	r.POST("/api/v1/searchbots/retrieval_test", h.RetrievalTest)
	return h, r
}

func TestSearchbotsRetrieval_Basic(t *testing.T) {
	_, r := setupSearchbotsTest("user1")
	body := `{"kb_id": ["kb1"], "question": "test question"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != float64(common.CodeSuccess) {
		t.Errorf("expected code 0, got %v", resp["code"])
	}
}

func TestSearchbotsRetrieval_MissingKbID(t *testing.T) {
	_, r := setupSearchbotsTest("user1")
	body := `{"question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSearchbotsRetrieval_MissingQuestion(t *testing.T) {
	_, r := setupSearchbotsTest("user1")
	body := `{"kb_id": ["kb1"]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSearchbotsRetrieval_NoAuth(t *testing.T) {
	h := &SearchbotHandler{chunkSvc: &mockChunkService{}}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/searchbots/retrieval_test", h.RetrievalTest)
	w := httptest.NewRecorder()
	body := `{"kb_id": ["kb1"], "question": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSearchbotsRetrieval_ServiceError(t *testing.T) {
	h, r := setupSearchbotsTest("user1")
	h.chunkSvc = &mockChunkService{
		retrievalTestFn: func(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error) {
			return nil, errors.New("db error")
		},
	}
	w := httptest.NewRecorder()
	body := `{"kb_id": ["kb1"], "question": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSearchbotsRetrieval_NotFound(t *testing.T) {
	h, r := setupSearchbotsTest("user1")
	h.chunkSvc = &mockChunkService{
		retrievalTestFn: func(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error) {
			return nil, errors.New("no chunk found: not_found")
		},
	}
	w := httptest.NewRecorder()
	body := `{"kb_id": ["kb1"], "question": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSearchbotsRetrieval_InvalidJSON(t *testing.T) {
	_, r := setupSearchbotsTest("user1")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}
