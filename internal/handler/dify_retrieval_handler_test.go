//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package handler

import (
	"context"
	"encoding/json"
	"errors"
		"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service/nlp"
	"ragflow/internal/engine/types"

	"github.com/gin-gonic/gin"
)

// --- Mock implementations ---

type mockKBService struct {
	KBServiceIface
	getByIDFn    func(kbID string) (*entity.Knowledgebase, error)
	accessibleFn func(kbID, userID string) bool
}

func (m *mockKBService) GetByID(kbID string) (*entity.Knowledgebase, error) {
	if m.getByIDFn != nil {
		return m.getByIDFn(kbID)
	}
	return &entity.Knowledgebase{
		ID: kbID, TenantID: "tenant1", EmbdID: "text-embedding",
	}, nil
}

func (m *mockKBService) Accessible(kbID, userID string) bool {
	if m.accessibleFn != nil {
		return m.accessibleFn(kbID, userID)
	}
	return true
}

type mockModelService struct {
	ModelServiceIface
	getEmbeddingFn func(tenantID, embdID string) (*modelModule.EmbeddingModel, error)
	getChatModelFn func(tenantID, llmID string) (*modelModule.ChatModel, error)
}

func (m *mockModelService) GetEmbeddingModel(tenantID, embdID string) (*modelModule.EmbeddingModel, error) {
	if m.getEmbeddingFn != nil {
		return m.getEmbeddingFn(tenantID, embdID)
	}
	return &modelModule.EmbeddingModel{}, nil
}

func (m *mockModelService) GetChatModel(tenantID, llmID string) (*modelModule.ChatModel, error) {
	if m.getChatModelFn != nil {
		return m.getChatModelFn(tenantID, llmID)
	}
	return &modelModule.ChatModel{}, nil
}

type mockMetadataService struct {
	MetadataServiceIface
	getFlattedMetaFn func(kbIDs []string) (common.MetaData, error)
	labelQuestionFn  func(question string, kbs []*entity.Knowledgebase) map[string]float64
}

func (m *mockMetadataService) GetFlattedMetaByKBs(kbIDs []string) (common.MetaData, error) {
	if m.getFlattedMetaFn != nil {
		return m.getFlattedMetaFn(kbIDs)
	}
	return common.MetaData{}, nil
}

func (m *mockMetadataService) LabelQuestion(question string, kbs []*entity.Knowledgebase) map[string]float64 {
	if m.labelQuestionFn != nil {
		return m.labelQuestionFn(question, kbs)
	}
	return nil
}

type mockRetrievalService struct {
	RetrievalServiceIface
	retrievalFn func(ctx context.Context, req *nlp.RetrievalRequest) (*nlp.RetrievalResult, error)
}

func (m *mockRetrievalService) Retrieval(ctx context.Context, req *nlp.RetrievalRequest) (*nlp.RetrievalResult, error) {
	if m.retrievalFn != nil {
		return m.retrievalFn(ctx, req)
	}
	return &nlp.RetrievalResult{
		Chunks: []map[string]interface{}{
			{"doc_id": "doc1", "docnm_kwd": "Test Doc", "content_with_weight": "test content", "similarity": 0.85},
		},
	}, nil
}

type mockDocDAO struct {
	DocumentDAOIface
	getByIDsFn func(ids []string) ([]*entity.Document, error)
}

func (m *mockDocDAO) GetByIDs(ids []string) ([]*entity.Document, error) {
	if m.getByIDsFn != nil {
		return m.getByIDsFn(ids)
	}
	return []*entity.Document{
		{ID: "doc1", Name: strPtr("Test Doc"), MetaFields: &entity.JSONMap{"author": "Zhang San"}},
	}, nil
}

// mockDocEngine stubs the DocEngine interface (embed = panic on unimplemented).
type mockDocEngine struct {
	engine.DocEngine
}

func (m *mockDocEngine) Close() error                          { return nil }
func (m *mockDocEngine) Ping(ctx context.Context) error         { return nil }
func (m *mockDocEngine) GetType() string { return "mock" }
	func (m *mockDocEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
		return &types.SearchResult{}, nil
	}
func (m *mockDocEngine) GetChunk(ctx context.Context, _, _ string, _ []string) (interface{}, error) {
	return map[string]interface{}{}, nil
}

// --- Helper ---

func setupDifyTest(userID string) (*DifyRetrievalHandler, *gin.Engine) {
	h := &DifyRetrievalHandler{
		kbSvc:        &mockKBService{},
		modelSvc:     &mockModelService{},
		metadataSvc:  &mockMetadataService{},
		retrievalSvc: &mockRetrievalService{},
		docDAO:       &mockDocDAO{},
		docEngine:    &mockDocEngine{},
	}

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: userID})
	})
	r.POST("/api/v1/dify/retrieval", h.Retrieval)
	r.GET("/api/v1/dify/retrieval", h.Retrieval)
	r.GET("/api/v1/dify/retrieval/health", h.HealthCheck)
	return h, r
}

func setupDifyTestNoAuth() (*DifyRetrievalHandler, *gin.Engine) {
	h := &DifyRetrievalHandler{
		kbSvc:        &mockKBService{},
		modelSvc:     &mockModelService{},
		metadataSvc:  &mockMetadataService{},
		retrievalSvc: &mockRetrievalService{},
		docDAO:       &mockDocDAO{},
		docEngine:    &mockDocEngine{},
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/dify/retrieval", h.Retrieval)
	return h, r
}

// --- Tests ---

func TestDifyRetrieval_HealthCheck(t *testing.T) {
	_, r := setupDifyTest("user1")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/dify/retrieval/health", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["data"] != true {
		t.Errorf("expected data=true, got %v", resp["data"])
	}
}

func TestDifyRetrieval_Basic(t *testing.T) {
	_, r := setupDifyTest("user1")
	body := `{"knowledge_id": "kb1", "query": "test question"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	records, ok := resp["records"].([]interface{})
	if !ok || len(records) == 0 {
		t.Errorf("expected non-empty records, got %v", resp["records"])
	}
}

func TestDifyRetrieval_GET(t *testing.T) {
	_, r := setupDifyTest("user1")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/v1/dify/retrieval?knowledge_id=kb1&query=test", nil)
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDifyRetrieval_MissingArgs(t *testing.T) {
	_, r := setupDifyTest("user1")
	tests := []struct {
		name string
		body string
	}{
		{"no knowledge_id", `{"query": "test"}`},
		{"no query", `{"knowledge_id": "kb1"}`},
		{"empty body", `{}`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			r.ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", w.Code)
			}
		})
	}
}

func TestDifyRetrieval_KBNotFound(t *testing.T) {
	h, r := setupDifyTest("user1")
	h.kbSvc = &mockKBService{
		getByIDFn: func(kbID string) (*entity.Knowledgebase, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	w := httptest.NewRecorder()
	body := `{"knowledge_id": "nonexistent", "query": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDifyRetrieval_NoAuth(t *testing.T) {
	_, r := setupDifyTestNoAuth()
	w := httptest.NewRecorder()
	body := `{"knowledge_id": "kb1", "query": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDifyRetrieval_Unauthorized(t *testing.T) {
	h, r := setupDifyTest("user1")
	h.kbSvc = &mockKBService{
		accessibleFn: func(kbID, userID string) bool { return false },
	}
	w := httptest.NewRecorder()
	body := `{"knowledge_id": "kb1", "query": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestDifyRetrieval_WithMetadataFilter(t *testing.T) {
	h, r := setupDifyTest("user1")
	h.metadataSvc = &mockMetadataService{
		getFlattedMetaFn: func(kbIDs []string) (common.MetaData, error) {
			return common.MetaData{}, nil
		},
	}
	body := `{"knowledge_id":"kb1","query":"test","metadata_condition":{"conditions":[{"name":"author","comparison_operator":"eq","value":"Zhang San"}],"logic":"and"}}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestDifyRetrieval_InvalidJSON(t *testing.T) {
	_, r := setupDifyTest("user1")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestDifyRetrieval_UseKG(t *testing.T) {
	h, r := setupDifyTest("user1")
	h.metadataSvc = &mockMetadataService{
		labelQuestionFn: func(question string, kbs []*entity.Knowledgebase) map[string]float64 {
			return map[string]float64{"tag_1": 0.8}
		},
	}
	body := `{"knowledge_id":"kb1","query":"test","use_kg":true}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func strPtr(s string) *string { return &s }

func TestDifyRetrieval_KBDBError(t *testing.T) {
	h, r := setupDifyTest("user1")
	h.kbSvc = &mockKBService{
		getByIDFn: func(kbID string) (*entity.Knowledgebase, error) {
			return nil, errors.New("connection refused")
		},
	}
	w := httptest.NewRecorder()
	body := `{"knowledge_id": "kb1", "query": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DB error, got %d", w.Code)
	}
}

func TestDifyRetrieval_DocLoadError(t *testing.T) {
	h, r := setupDifyTest("user1")
	h.docDAO = &mockDocDAO{
		getByIDsFn: func(ids []string) ([]*entity.Document, error) {
			return nil, errors.New("db unavailable")
		},
	}
	w := httptest.NewRecorder()
	body := `{"knowledge_id": "kb1", "query": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for doc load error, got %d", w.Code)
	}
}

func TestDifyRetrieval_RetrievalNotFound(t *testing.T) {
	h, r := setupDifyTest("user1")
	h.retrievalSvc = &mockRetrievalService{
		retrievalFn: func(ctx context.Context, req *nlp.RetrievalRequest) (*nlp.RetrievalResult, error) {
			return nil, errors.New("no chunk found: not_found")
		},
	}
	w := httptest.NewRecorder()
	body := `{"knowledge_id": "kb1", "query": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/dify/retrieval", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for not_found, got %d", w.Code)
	}
}
