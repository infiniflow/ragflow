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
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

// mockChunkService implements service.Retriever for testing.
// It captures the last request received so tests can verify field mapping.
type mockChunkService struct {
	retrievalTestFn func(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error)
	LastReq         *service.RetrievalTestRequest
	LastUserID      string
}

func (m *mockChunkService) RetrievalTest(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error) {
	m.LastReq = req
	m.LastUserID = userID
	if m.retrievalTestFn != nil {
		return m.retrievalTestFn(req, userID)
	}
	return &service.RetrievalTestResponse{
		Chunks: []map[string]interface{}{{"docnm_kwd": "test", "content_with_weight": "content"}},
	}, nil
}

func setupSearchbotsTest(userID string) (*SearchBotHandler, *mockChunkService, *gin.Engine) {
	mockSvc := &mockChunkService{}
	h := &SearchBotHandler{
		chunkSvc: mockSvc,
	}
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: userID})
	})
	r.POST("/api/v1/searchbots/retrieval_test", h.RetrievalTest)
	return h, mockSvc, r
}

func TestSearchBotsRetrieval_Basic(t *testing.T) {
	_, mockSvc, r := setupSearchbotsTest("user1")

	body := `{"kb_ids": ["kb1"], "question": "test question"}`
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
	if msg, _ := resp["message"].(string); msg != "success" {
		t.Errorf("expected message 'success', got %q", msg)
	}
	// Verify field mapping: handler → service request
	if mockSvc.LastReq == nil {
		t.Fatal("RetrievalTest was not called")
	}
	if len(mockSvc.LastReq.Datasets) != 1 || mockSvc.LastReq.Datasets[0] != "kb1" {
		t.Errorf("Datasets = %v, want [\"kb1\"]", mockSvc.LastReq.Datasets)
	}
	if mockSvc.LastReq.Question != "test question" {
		t.Errorf("Question = %q, want \"test question\"", mockSvc.LastReq.Question)
	}
	if mockSvc.LastUserID != "user1" {
		t.Errorf("userID = %q, want \"user1\"", mockSvc.LastUserID)
	}
}

func TestSearchBotsRetrieval_MissingKbID(t *testing.T) {
	_, _, r := setupSearchbotsTest("user1")
	body := `{"question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	msg, _ := resp["message"].(string)
	if msg == "" || msg == "success" {
		t.Errorf("expected validation error message, got %q", msg)
	}
	if !strings.Contains(msg, "KbIDs") || !strings.Contains(msg, "required") {
		t.Errorf("expected message to mention 'KbIDs' and 'required', got %q", msg)
	}
}

func TestSearchBotsRetrieval_MissingQuestion(t *testing.T) {
	_, _, r := setupSearchbotsTest("user1")
	body := `{"kb_ids": ["kb1"]}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	msg, _ := resp["message"].(string)
	if msg == "" || msg == "success" {
		t.Errorf("expected validation error message, got %q", msg)
	}
	if !strings.Contains(msg, "Question") || !strings.Contains(msg, "required") {
		t.Errorf("expected message to mention 'Question' and 'required', got %q", msg)
	}
}

func TestSearchBotsRetrieval_NoAuth(t *testing.T) {
	h := NewSearchBotHandler(nil, nil, nil, &mockChunkService{})
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/v1/searchbots/retrieval_test", h.RetrievalTest)
	w := httptest.NewRecorder()
	body := `{"kb_ids": ["kb1"], "question": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestSearchBotsRetrieval_ServiceError(t *testing.T) {
	h, _, r := setupSearchbotsTest("user1")
	h.chunkSvc = &mockChunkService{
		retrievalTestFn: func(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error) {
			return nil, errors.New("db error")
		},
	}
	w := httptest.NewRecorder()
	body := `{"kb_ids": ["kb1"], "question": "test"}`
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	code, _ := resp["code"].(float64)
	if code == 0 {
		t.Errorf("expected non-zero error code, got %v", code)
	}
	msg, _ := resp["message"].(string)
	if msg == "" || msg == "success" {
		t.Errorf("expected error message, got %q", msg)
	}
}

func TestSearchBotsRetrieval_KbIDSingleString(t *testing.T) {
	// Verify "kb1" (string) is accepted and converted to []string{"kb1"}
	_, mockSvc, r := setupSearchbotsTest("user1")

	body := `{"kb_ids": "kb1", "question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if mockSvc.LastReq == nil {
		t.Fatal("RetrievalTest was not called")
	}
	if len(mockSvc.LastReq.Datasets) != 1 || mockSvc.LastReq.Datasets[0] != "kb1" {
		t.Errorf("Datasets = %v, want [\"kb1\"]", mockSvc.LastReq.Datasets)
	}
}

func TestSearchBotsRetrieval_KbIDArray(t *testing.T) {
	// Verify ["a","b"] (array) still works
	_, mockSvc, r := setupSearchbotsTest("user1")

	body := `{"kb_ids": ["a","b"], "question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if mockSvc.LastReq == nil {
		t.Fatal("RetrievalTest was not called")
	}
	if len(mockSvc.LastReq.Datasets) != 2 || mockSvc.LastReq.Datasets[0] != "a" || mockSvc.LastReq.Datasets[1] != "b" {
		t.Errorf("Datasets = %v, want [\"a\",\"b\"]", mockSvc.LastReq.Datasets)
	}
}

func TestSearchBotsRetrieval_InvalidJSON(t *testing.T) {
	_, _, r := setupSearchbotsTest("user1")
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader("{invalid}"))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSearchBotsRetrieval_EmptyStringKbID(t *testing.T) {
	_, _, r := setupSearchbotsTest("user1")
	body := `{"kb_ids": "", "question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if msg, _ := resp["message"].(string); msg != "kb_id and question are required" {
		t.Errorf("expected message 'kb_id and question are required', got %q", msg)
	}
}

func TestSearchBotsRetrieval_WhitespaceOnlyKbID(t *testing.T) {
	_, _, r := setupSearchbotsTest("user1")
	body := `{"kb_ids": "  ", "question": "test"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if msg, _ := resp["message"].(string); msg != "kb_id and question are required" {
		t.Errorf("expected message 'kb_id and question are required', got %q", msg)
	}
}

func TestSearchBotsRetrieval_DefaultsApplied(t *testing.T) {
	// Verify that when optional fields are omitted, the handler applies
	// defaults matching Python bot_api.py retrieval_test endpoint.
	_, mockSvc, r := setupSearchbotsTest("user1")

	body := `{"kb_ids": ["kb1"], "question": "does this default?"}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if mockSvc.LastReq == nil {
		t.Fatal("RetrievalTest was not called")
	}

	svcReq := mockSvc.LastReq
	if svcReq.Page == nil || *svcReq.Page != 1 {
		t.Errorf("Page = %v, want 1", nullableInt(svcReq.Page))
	}
	if svcReq.Size == nil || *svcReq.Size != 30 {
		t.Errorf("Size = %v, want 30", nullableInt(svcReq.Size))
	}
	if svcReq.TopK == nil || *svcReq.TopK != 1024 {
		t.Errorf("TopK = %v, want 1024", nullableInt(svcReq.TopK))
	}
	if svcReq.UseKG == nil || *svcReq.UseKG != false {
		t.Errorf("UseKG = %v, want false", nullableBool(svcReq.UseKG))
	}
	if svcReq.Keyword == nil || *svcReq.Keyword != false {
		t.Errorf("Keyword = %v, want false", nullableBool(svcReq.Keyword))
	}
	if svcReq.SimilarityThreshold == nil || *svcReq.SimilarityThreshold != 0.0 {
		t.Errorf("SimilarityThreshold = %v, want 0.0", nullableFloat(svcReq.SimilarityThreshold))
	}
	if svcReq.VectorSimilarityWeight == nil || *svcReq.VectorSimilarityWeight != 0.3 {
		t.Errorf("VectorSimilarityWeight = %v, want 0.3", nullableFloat(svcReq.VectorSimilarityWeight))
	}
}

func TestSearchBotsRetrieval_TopKZero(t *testing.T) {
	_, _, r := setupSearchbotsTest("user1")
	body := `{"kb_ids": ["kb1"], "question": "test", "top_k": 0}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	if msg, _ := resp["message"].(string); msg != "top_k must be greater than 0" {
		t.Errorf("expected message 'top_k must be greater than 0', got %q", msg)
	}
}

func TestSearchBotsRetrieval_TopKNegative(t *testing.T) {
	_, _, r := setupSearchbotsTest("user1")
	body := `{"kb_ids": ["kb1"], "question": "test", "top_k": -1}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if msg := jsonDecodeMessage(t, w.Body.Bytes()); msg != "top_k must be greater than 0" {
		t.Errorf("expected message 'top_k must be greater than 0', got %q", msg)
	}
}

func jsonDecodeMessage(t *testing.T, body []byte) string {
	t.Helper()
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}
	msg, _ := resp["message"].(string)
	return msg
}

func nullableInt(p *int) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%d", *p)
}
func nullableBool(p *bool) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%v", *p)
}
func nullableFloat(p *float64) string {
	if p == nil {
		return "nil"
	}
	return fmt.Sprintf("%v", *p)
}
func TestSearchBotsRetrieval_EmptyQuestion(t *testing.T) {
	// Send kb_id but empty question — caught by binding:"required" on the DTO.
	_, _, r := setupSearchbotsTest("user1")
	body := `{"kb_ids": ["kb1"], "question": ""}`
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/api/v1/searchbots/retrieval_test", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
	msg := jsonDecodeMessage(t, w.Body.Bytes())
	if !strings.Contains(msg, "Question") || !strings.Contains(msg, "required") {
		t.Errorf("expected validation error mentioning Question and required, got %q", msg)
	}
}

func setupSearchBotRequest(body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/v1/searchbots/related_questions",
		strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "user-1"})
	c.Set("user_id", "user-1")
	return c, w
}

// TestSearchBotHandler_MissingQuestion verifies validation.
func TestSearchBotHandler_MissingQuestion(t *testing.T) {
	h := NewSearchBotHandler(nil, nil, nil, nil)

	c, w := setupSearchBotRequest(`{}`)
	h.Handle(c)

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	code, _ := resp["code"].(float64)
	if code == 0 {
		t.Errorf("expected error code, got 0")
	}
}

// ---- Ask handler tests ----

func TestAskHandler_MissingQuestion(t *testing.T) {
	ret := &fakeChunkRetriever{result: &service.RetrievalTestResponse{}}
	h := NewSearchBotHandler(nil, nil, nil, ret)
	c, w := cw()
	c.Request = httptest.NewRequest("POST", "/api/v1/searchbots/ask",
		strings.NewReader(`{"kb_ids": ["kb1"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "user-1"})
	h.Ask(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing question, got %d", w.Code)
	}
}

func TestAskHandler_MissingKbIDs(t *testing.T) {
	ret := &fakeChunkRetriever{result: &service.RetrievalTestResponse{}}
	h := NewSearchBotHandler(nil, nil, nil, ret)
	c, w := cw()
	c.Request = httptest.NewRequest("POST", "/api/v1/searchbots/ask",
		strings.NewReader(`{"question": "test"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "user-1"})
	h.Ask(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing kb_ids, got %d", w.Code)
	}
}

type fakeChunkRetriever struct {
	result *service.RetrievalTestResponse
	err    error
}

func (f *fakeChunkRetriever) RetrievalTest(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.result, nil
}

func cw() (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	return c, w
}

// bufferSSEWriter collects SSE output into a strings.Builder for test assertions.
type bufferSSEWriter struct {
	buf strings.Builder
}

func (w *bufferSSEWriter) Write(_ *gin.Context, data string) {
	w.buf.WriteString(data)
}

func (w *bufferSSEWriter) String() string { return w.buf.String() }

// ---- Ask handler tests ----

func TestAskHandler_EmptyQuestion(t *testing.T) {

	ret := &fakeChunkRetriever{result: &service.RetrievalTestResponse{
		Chunks: []map[string]interface{}{{"id": "c1", "content_with_weight": "test"}},
	}}
	h := NewSearchBotHandler(nil, nil, nil, ret)
	c, w := cw()
	c.Request = httptest.NewRequest("POST", "/api/v1/searchbots/ask",
		strings.NewReader(`{"question": "  ", "kb_ids": ["kb1"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "user-1"})
	h.Ask(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for whitespace question, got %d", w.Code)
	}
}

func TestAskHandler_EmptyKbIDs(t *testing.T) {
	ret := &fakeChunkRetriever{result: &service.RetrievalTestResponse{}}
	h := NewSearchBotHandler(nil, nil, nil, ret)
	c, w := cw()
	c.Request = httptest.NewRequest("POST", "/api/v1/searchbots/ask",
		strings.NewReader(`{"question": "test", "kb_ids": []}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "user-1"})
	h.Ask(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty kb_ids, got %d", w.Code)
	}
}

func TestAskHandler_NoChatModel(t *testing.T) {
	buf := &bufferSSEWriter{}
	ret := &fakeChunkRetriever{result: &service.RetrievalTestResponse{
		Chunks: []map[string]interface{}{{"id": "c1", "content_with_weight": "test"}},
	}}
	h := NewSearchBotHandler(nil, nil, nil, ret)
	h.sseWriter = buf
	c, _ := cw()
	c.Request = httptest.NewRequest("POST", "/api/v1/searchbots/ask",
		strings.NewReader(`{"question": "test", "kb_ids": ["kb1"]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "user-1"})
	h.Ask(c)
	body := buf.String()
	if !strings.Contains(body, "chat model not configured") {
		t.Errorf("expected 'chat model not configured', got: %q", body)
	}
}

func TestAskHandler_InvalidJSON(t *testing.T) {
	ret := &fakeChunkRetriever{result: &service.RetrievalTestResponse{}}
	h := NewSearchBotHandler(nil, nil, nil, ret)
	c, w := cw()
	c.Request = httptest.NewRequest("POST", "/api/v1/searchbots/ask",
		strings.NewReader(`not json`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "user-1"})
	h.Ask(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", w.Code)
	}
}

func TestAskHandler_WhitespaceKbIDFiltered(t *testing.T) {
	ret := &fakeChunkRetriever{result: &service.RetrievalTestResponse{}}
	h := NewSearchBotHandler(nil, nil, nil, ret)
	c, w := cw()
	c.Request = httptest.NewRequest("POST", "/api/v1/searchbots/ask",
		strings.NewReader(`{"question": "test", "kb_ids": ["  ", ""]}`))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user", &entity.User{ID: "user-1"})
	h.Ask(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for all-whitespace kb_ids, got %d", w.Code)
	}
}

func TestParseMindMapMarkdown_ListUnderHeading(t *testing.T) {
	got := parseMindMapMarkdown("# Product\n- Features\n  - Search")
	if got.ID != "Product" {
		t.Fatalf("root = %q, want Product", got.ID)
	}
	if len(got.Children) != 1 || got.Children[0].ID != "Features" {
		t.Fatalf("children = %+v, want Features under Product", got.Children)
	}
	if len(got.Children[0].Children) != 1 || got.Children[0].Children[0].ID != "Search" {
		t.Fatalf("nested children = %+v, want Search under Features", got.Children[0].Children)
	}
}

// ---- SSE helper direct tests ----

func TestSseAnswer_Final(t *testing.T) {
	s := sseAnswer("hello", map[string]interface{}{"chunks": []int{}}, true)
	if !strings.Contains(s, `"answer":"hello"`) {
		t.Errorf("missing answer: %s", s)
	}
	if !strings.Contains(s, `"final":true`) {
		t.Errorf("missing final=true: %s", s)
	}
	if !strings.Contains(s, "data: ") {
		t.Errorf("missing SSE prefix: %s", s)
	}
}

func TestSseAnswer_NilRefs(t *testing.T) {
	s := sseAnswer("hello", nil, false)
	if !strings.Contains(s, `"reference":{}`) {
		t.Errorf("nil refs should produce {}: %s", s)
	}
}

func TestSseMarker_ThinkOpen(t *testing.T) {
	s := sseMarker("<think>")
	if !strings.Contains(s, `"start_to_think":true`) {
		t.Errorf("missing start_to_think: %s", s)
	}
	if strings.Contains(s, "end_to_think") {
		t.Error("should NOT contain end_to_think for <think> marker")
	}
}

func TestSseMarker_ThinkClose(t *testing.T) {
	s := sseMarker("</think>")
	if !strings.Contains(s, `"end_to_think":true`) {
		t.Errorf("missing end_to_think: %s", s)
	}
	if strings.Contains(s, "start_to_think") {
		t.Error("should NOT contain start_to_think for </think> marker")
	}
}

func TestSseError_Format(t *testing.T) {
	s := sseError("something broke")
	if !strings.Contains(s, `"code":500`) {
		t.Errorf("missing error code: %s", s)
	}
	if !strings.Contains(s, `**ERROR**: something broke`) {
		t.Errorf("missing error prefix: %s", s)
	}
	if !strings.Contains(s, `"reference":[]`) {
		t.Errorf("missing empty reference array: %s", s)
	}
}
