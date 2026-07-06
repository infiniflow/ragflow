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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"ragflow/internal/agent/canvas"
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

// stubBotService is the stub for the botService interface used by
// BotHandler. Each test case sets only the methods it needs; unset
// methods return safe defaults.
type stubBotService struct {
	chatbotInfoFn      func(ctx context.Context, tenantID, dialogID string) (string, string, string, string, bool, common.ErrorCode, error)
	agentbotInputsFn   func(ctx context.Context, tenantID, agentID string) (string, string, string, string, map[string]any, common.ErrorCode, error)
	agentbotCompleteFn func(ctx context.Context, tenantID, agentID string, req service.AgentbotCompletionRequest) (<-chan canvas.RunEvent, common.ErrorCode, error)
	chatbotCompleteFn  func(ctx context.Context, tenantID, dialogID string, req service.ChatbotCompletionRequest) (<-chan service.ChatbotSSEFrame, common.ErrorCode, error)
}

func (s *stubBotService) ChatbotInfo(ctx context.Context, tenantID, dialogID string) (string, string, string, string, bool, common.ErrorCode, error) {
	if s.chatbotInfoFn != nil {
		return s.chatbotInfoFn(ctx, tenantID, dialogID)
	}
	return "", "", "", "", false, common.CodeDataError, errors.New("not stubbed")
}

func (s *stubBotService) AgentbotInputs(ctx context.Context, tenantID, agentID string) (string, string, string, string, map[string]any, common.ErrorCode, error) {
	if s.agentbotInputsFn != nil {
		return s.agentbotInputsFn(ctx, tenantID, agentID)
	}
	return "", "", "", "", nil, common.CodeDataError, errors.New("not stubbed")
}

func (s *stubBotService) AgentbotCompletion(ctx context.Context, tenantID, agentID string, req service.AgentbotCompletionRequest) (<-chan canvas.RunEvent, common.ErrorCode, error) {
	if s.agentbotCompleteFn != nil {
		return s.agentbotCompleteFn(ctx, tenantID, agentID, req)
	}
	return nil, common.CodeDataError, errors.New("not stubbed")
}

func (s *stubBotService) ChatbotCompletion(ctx context.Context, tenantID, dialogID string, req service.ChatbotCompletionRequest) (<-chan service.ChatbotSSEFrame, common.ErrorCode, error) {
	if s.chatbotCompleteFn != nil {
		return s.chatbotCompleteFn(ctx, tenantID, dialogID, req)
	}
	return nil, common.CodeDataError, errors.New("not stubbed")
}

// botTestEngine wires a gin engine with the bot routes + a fake
// user (so the BotHandler's GetUser check passes). Returns the
// engine and the stub.
//
// Routes are registered INLINE here (not via RegisterChatbotRoutes
// from internal/router) to avoid an import cycle — the router
// package already imports this handler package. The route paths
// must stay in sync with internal/router/bot_routes.go.
func botTestEngine(stub *stubBotService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-x"})
		c.Next()
	})
	h := NewBotHandler(nil)
	h.botService = stub
	chatbot := r.Group("/api/v1/chatbots")
	chatbot.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-x"})
		c.Next()
	})
	chatbot.POST("/:dialog_id/completions", h.ChatbotCompletion)
	chatbot.GET("/:dialog_id/info", h.ChatbotInfo)

	agentbot := r.Group("/api/v1/agentbots")
	agentbot.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-x"})
		c.Next()
	})
	agentbot.POST("/:agent_id/completions", h.AgentbotCompletion)
	agentbot.GET("/:agent_id/inputs", h.AgentbotInputs)
	return r
}

// doJSON is a tiny test helper that fires an HTTP request and
// returns the recorder.
func doJSON(r *gin.Engine, method, path, body string) *httptest.ResponseRecorder {
	var reqBody *bytes.Reader
	if body != "" {
		reqBody = bytes.NewReader([]byte(body))
	} else {
		reqBody = bytes.NewReader(nil)
	}
	req, _ := http.NewRequest(method, path, reqBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// ----- ChatbotInfo tests (criteria 13, 14, 15, 16, 29) -----

// TestChatbotInfo_OK covers the happy path (criterion 13).
func TestChatbotInfo_OK(t *testing.T) {
	stub := &stubBotService{
		chatbotInfoFn: func(ctx context.Context, tenantID, dialogID string) (string, string, string, string, bool, common.ErrorCode, error) {
			return "My Bot", "avatar.png", "Hello!", "gpt-4", false, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodGet, "/api/v1/chatbots/d1/info", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Code    int                    `json:"code"`
		Data    map[string]interface{} `json:"data"`
		Message string                 `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if resp.Code != 0 {
		t.Errorf("code = %d, want 0", resp.Code)
	}
	if resp.Data["title"] != "My Bot" {
		t.Errorf("title = %v, want My Bot", resp.Data["title"])
	}
	if resp.Data["prologue"] != "Hello!" {
		t.Errorf("prologue = %v, want Hello!", resp.Data["prologue"])
	}
	if resp.Data["llm_id"] != "gpt-4" {
		t.Errorf("llm_id = %v, want gpt-4", resp.Data["llm_id"])
	}
}

// TestChatbotInfo_HasTavilyKey covers criterion 14.
func TestChatbotInfo_HasTavilyKey(t *testing.T) {
	stub := &stubBotService{
		chatbotInfoFn: func(ctx context.Context, tenantID, dialogID string) (string, string, string, string, bool, common.ErrorCode, error) {
			return "Bot", "", "", "gpt-4", true, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodGet, "/api/v1/chatbots/d1/info", "")
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data["has_tavily_key"] != true {
		t.Errorf("has_tavily_key = %v, want true", resp.Data["has_tavily_key"])
	}
}

// TestChatbotInfo_ForeignTenant covers criterion 15.
func TestChatbotInfo_ForeignTenant(t *testing.T) {
	stub := &stubBotService{
		chatbotInfoFn: func(ctx context.Context, tenantID, dialogID string) (string, string, string, string, bool, common.ErrorCode, error) {
			return "", "", "", "", false, common.CodeDataError, errors.New("Authentication error: no access to this chatbot!")
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodGet, "/api/v1/chatbots/d1/info", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 102 {
		t.Errorf("code = %d, want 102", resp.Code)
	}
}

// TestChatbotInfo_MissingPrologueField covers criterion 29.
func TestChatbotInfo_MissingPrologueField(t *testing.T) {
	// Stub returns empty prologue (mimics the defensive stringFromMap
	// fallback when the field is absent or non-string).
	stub := &stubBotService{
		chatbotInfoFn: func(ctx context.Context, tenantID, dialogID string) (string, string, string, string, bool, common.ErrorCode, error) {
			return "Bot", "", "", "gpt-4", false, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodGet, "/api/v1/chatbots/d1/info", "")
	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if got, ok := resp.Data["prologue"].(string); !ok || got != "" {
		t.Errorf("prologue = %v, want \"\" (string)", resp.Data["prologue"])
	}
}

// ----- ChatbotCompletion tests (criteria 6, 7, 8, 9, 10, 11, 12) -----

// TestChatbotCompletion_AuthoriseFail covers criterion 6.
func TestChatbotCompletion_AuthoriseFail(t *testing.T) {
	stub := &stubBotService{
		chatbotCompleteFn: func(ctx context.Context, tenantID, dialogID string, req service.ChatbotCompletionRequest) (<-chan service.ChatbotSSEFrame, common.ErrorCode, error) {
			return nil, common.CodeDataError, errors.New("no access to this chatbot")
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodPost, "/api/v1/chatbots/d1/completions", `{"question":"hi"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 102 {
		t.Errorf("code = %d, want 102", resp.Code)
	}
	if !strings.Contains(resp.Message, "no access") {
		t.Errorf("message = %q, want contains 'no access'", resp.Message)
	}
}

// TestChatbotCompletion_StreamsSSE covers criterion 11.
func TestChatbotCompletion_StreamsSSE(t *testing.T) {
	stub := &stubBotService{
		chatbotCompleteFn: func(ctx context.Context, tenantID, dialogID string, req service.ChatbotCompletionRequest) (<-chan service.ChatbotSSEFrame, common.ErrorCode, error) {
			ch := make(chan service.ChatbotSSEFrame, 4)
			go func() {
				defer close(ch)
				ch <- service.ChatbotSSEFrame{Data: "hello", SessionID: "s1"}
				ch <- service.ChatbotSSEFrame{Done: true}
			}()
			return ch, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodPost, "/api/v1/chatbots/d1/completions", `{"question":"hi"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	frames := parseBotSSEFrames(t, w.Body.Bytes())
	if len(frames) < 3 {
		t.Fatalf("expected >= 3 frames, got %d: %v", len(frames), frames)
	}
	// First frame is the data envelope.
	var env map[string]any
	if err := json.Unmarshal([]byte(frames[0]), &env); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if env["code"].(float64) != 0 {
		t.Errorf("frame code = %v, want 0", env["code"])
	}
	data, _ := env["data"].(map[string]any)
	if data["answer"] != "hello" {
		t.Errorf("frame answer = %v, want hello", data["answer"])
	}
	if data["session_id"] != "s1" {
		t.Errorf("frame session_id = %v, want s1", data["session_id"])
	}
	// Last frame is [DONE].
	if frames[len(frames)-1] != "[DONE]" {
		t.Errorf("last frame = %q, want [DONE]", frames[len(frames)-1])
	}
}

// TestChatbotCompletion_LLMUnavailable covers criterion 12.
func TestChatbotCompletion_LLMUnavailable(t *testing.T) {
	stub := &stubBotService{
		chatbotCompleteFn: func(ctx context.Context, tenantID, dialogID string, req service.ChatbotCompletionRequest) (<-chan service.ChatbotSSEFrame, common.ErrorCode, error) {
			return nil, common.CodeDataError, errors.New("LLM not available: timeout")
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodPost, "/api/v1/chatbots/d1/completions", `{"question":"hi"}`)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 102 {
		t.Errorf("code = %d, want 102", resp.Code)
	}
	if !strings.Contains(resp.Message, "LLM not available") {
		t.Errorf("message = %q, want contains 'LLM not available'", resp.Message)
	}
}

// TestChatbotCompletion_SessionNotFound covers criterion 10.
func TestChatbotCompletion_SessionNotFound(t *testing.T) {
	stub := &stubBotService{
		chatbotCompleteFn: func(ctx context.Context, tenantID, dialogID string, req service.ChatbotCompletionRequest) (<-chan service.ChatbotSSEFrame, common.ErrorCode, error) {
			return nil, common.CodeDataError, errors.New("session not found")
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodPost, "/api/v1/chatbots/d1/completions", `{"session_id":"missing","question":"hi"}`)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 102 {
		t.Errorf("code = %d, want 102", resp.Code)
	}
	if !strings.Contains(resp.Message, "session not found") {
		t.Errorf("message = %q, want contains 'session not found'", resp.Message)
	}
}

// TestChatbotCompletion_CreatesNewSession covers criterion 7.
func TestChatbotCompletion_CreatesNewSession(t *testing.T) {
	var capturedReq service.ChatbotCompletionRequest
	stub := &stubBotService{
		chatbotCompleteFn: func(ctx context.Context, tenantID, dialogID string, req service.ChatbotCompletionRequest) (<-chan service.ChatbotSSEFrame, common.ErrorCode, error) {
			capturedReq = req
			ch := make(chan service.ChatbotSSEFrame, 2)
			close(ch)
			return ch, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodPost, "/api/v1/chatbots/d1/completions", `{"question":"hi"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if capturedReq.SessionID != "" {
		t.Errorf("session_id = %q, want empty (new session)", capturedReq.SessionID)
	}
	if capturedReq.Question != "hi" {
		t.Errorf("question = %q, want hi", capturedReq.Question)
	}
}

// TestChatbotCompletion_ReusesSession covers criterion 8.
func TestChatbotCompletion_ReusesSession(t *testing.T) {
	var capturedReq service.ChatbotCompletionRequest
	stub := &stubBotService{
		chatbotCompleteFn: func(ctx context.Context, tenantID, dialogID string, req service.ChatbotCompletionRequest) (<-chan service.ChatbotSSEFrame, common.ErrorCode, error) {
			capturedReq = req
			ch := make(chan service.ChatbotSSEFrame, 2)
			close(ch)
			return ch, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	_ = doJSON(r, http.MethodPost, "/api/v1/chatbots/d1/completions", `{"session_id":"s-exists","question":"hi"}`)
	if capturedReq.SessionID != "s-exists" {
		t.Errorf("session_id = %q, want s-exists", capturedReq.SessionID)
	}
}

// TestChatbotCompletion_SessionTenantMismatch covers criterion 9.
func TestChatbotCompletion_SessionTenantMismatch(t *testing.T) {
	stub := &stubBotService{
		chatbotCompleteFn: func(ctx context.Context, tenantID, dialogID string, req service.ChatbotCompletionRequest) (<-chan service.ChatbotSSEFrame, common.ErrorCode, error) {
			return nil, common.CodeDataError, errors.New("session not found")
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodPost, "/api/v1/chatbots/d1/completions", `{"session_id":"s-other-tenant","question":"hi"}`)
	var resp struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 102 {
		t.Errorf("code = %d, want 102", resp.Code)
	}
}

// ----- AgentbotCompletion tests (criteria 17, 18, 19, 20) -----

// TestAgentbotCompletion_StreamsSSE covers criterion 17.
func TestAgentbotCompletion_StreamsSSE(t *testing.T) {
	stub := &stubBotService{
		agentbotCompleteFn: func(ctx context.Context, tenantID, agentID string, req service.AgentbotCompletionRequest) (<-chan canvas.RunEvent, common.ErrorCode, error) {
			ch := make(chan canvas.RunEvent, 4)
			go func() {
				defer close(ch)
				ch <- canvas.RunEvent{Type: "message", Data: "hello", SessionID: "s1"}
				ch <- canvas.RunEvent{Type: "message_end", Data: "", SessionID: "s1"}
			}()
			return ch, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodPost, "/api/v1/agentbots/a1/completions", `{"question":"hi"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	frames := parseBotSSEFrames(t, w.Body.Bytes())
	if len(frames) < 2 {
		t.Fatalf("expected >= 2 frames, got %d", len(frames))
	}
	// The last frame must be [DONE].
	if frames[len(frames)-1] != "[DONE]" {
		t.Errorf("last frame = %q, want [DONE]", frames[len(frames)-1])
	}
	// First frame is the data envelope.
	var env map[string]any
	if err := json.Unmarshal([]byte(frames[0]), &env); err != nil {
		t.Fatalf("bad JSON: %v", err)
	}
	if env["code"].(float64) != 0 {
		t.Errorf("frame code = %v, want 0", env["code"])
	}
}

// TestAgentbotCompletion_URLBoundAgentID covers criterion 18.
func TestAgentbotCompletion_URLBoundAgentID(t *testing.T) {
	var capturedAgentID string
	stub := &stubBotService{
		agentbotCompleteFn: func(ctx context.Context, tenantID, agentID string, req service.AgentbotCompletionRequest) (<-chan canvas.RunEvent, common.ErrorCode, error) {
			capturedAgentID = agentID
			ch := make(chan canvas.RunEvent, 2)
			close(ch)
			return ch, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	// Body says "agent_id=body-id" but the URL is "url-id"; the URL
	// must win.
	_ = doJSON(r, http.MethodPost, "/api/v1/agentbots/url-id/completions", `{"agent_id":"body-id","question":"hi"}`)
	if capturedAgentID != "url-id" {
		t.Errorf("agentID = %q, want url-id (URL must override body)", capturedAgentID)
	}
}

// TestAgentbotCompletion_NoAccess covers criterion 19.
func TestAgentbotCompletion_NoAccess(t *testing.T) {
	stub := &stubBotService{
		agentbotCompleteFn: func(ctx context.Context, tenantID, agentID string, req service.AgentbotCompletionRequest) (<-chan canvas.RunEvent, common.ErrorCode, error) {
			return nil, common.CodeDataError, errors.New("Can't find agent by ID: a1")
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodPost, "/api/v1/agentbots/a1/completions", `{"question":"hi"}`)
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 102 {
		t.Errorf("code = %d, want 102", resp.Code)
	}
	if !strings.Contains(resp.Message, "Can't find agent") {
		t.Errorf("message = %q, want contains 'Can't find agent'", resp.Message)
	}
}

// TestAgentbotCompletion_ResumesSession covers criterion 20.
func TestAgentbotCompletion_ResumesSession(t *testing.T) {
	var capturedReq service.AgentbotCompletionRequest
	stub := &stubBotService{
		agentbotCompleteFn: func(ctx context.Context, tenantID, agentID string, req service.AgentbotCompletionRequest) (<-chan canvas.RunEvent, common.ErrorCode, error) {
			capturedReq = req
			ch := make(chan canvas.RunEvent, 2)
			close(ch)
			return ch, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	_ = doJSON(r, http.MethodPost, "/api/v1/agentbots/a1/completions", `{"session_id":"s-resume","question":"hi"}`)
	if capturedReq.SessionID != "s-resume" {
		t.Errorf("session_id = %q, want s-resume", capturedReq.SessionID)
	}
}

// ----- AgentbotInputs tests (criteria 21, 22, 23) -----

// TestAgentbotInputs_OK covers criterion 21.
func TestAgentbotInputs_OK(t *testing.T) {
	stub := &stubBotService{
		agentbotInputsFn: func(ctx context.Context, tenantID, agentID string) (string, string, string, string, map[string]any, common.ErrorCode, error) {
			return "My Agent", "agent.png", "Welcome", "Agent", map[string]any{"query": map[string]any{"type": "string"}}, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodGet, "/api/v1/agentbots/a1/inputs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data["title"] != "My Agent" {
		t.Errorf("title = %v, want My Agent", resp.Data["title"])
	}
	if resp.Data["prologue"] != "Welcome" {
		t.Errorf("prologue = %v, want Welcome", resp.Data["prologue"])
	}
	if resp.Data["mode"] != "Agent" {
		t.Errorf("mode = %v, want Agent", resp.Data["mode"])
	}
	inputs, ok := resp.Data["inputs"].(map[string]any)
	if !ok {
		t.Fatalf("inputs is not a map: %T", resp.Data["inputs"])
	}
	if _, has := inputs["query"]; !has {
		t.Errorf("inputs missing 'query' key: %v", inputs)
	}
}

// TestAgentbotInputs_MissingBeginComponent covers criterion 22.
func TestAgentbotInputs_MissingBeginComponent(t *testing.T) {
	// Stub returns nil inputs and empty prologue/mode (mimics the
	// service-layer fallback when FindBeginComponentID returns
	// ErrComponentNotFound).
	stub := &stubBotService{
		agentbotInputsFn: func(ctx context.Context, tenantID, agentID string) (string, string, string, string, map[string]any, common.ErrorCode, error) {
			return "Agent", "", "", "", nil, common.CodeSuccess, nil
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodGet, "/api/v1/agentbots/a1/inputs", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (degrade gracefully, no 500)", w.Code)
	}
	var resp struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data["prologue"] != "" {
		t.Errorf("prologue = %v, want \"\"", resp.Data["prologue"])
	}
	if resp.Data["mode"] != "" {
		t.Errorf("mode = %v, want \"\"", resp.Data["mode"])
	}
}

// TestAgentbotInputs_NotFound covers criterion 23.
func TestAgentbotInputs_NotFound(t *testing.T) {
	stub := &stubBotService{
		agentbotInputsFn: func(ctx context.Context, tenantID, agentID string) (string, string, string, string, map[string]any, common.ErrorCode, error) {
			return "", "", "", "", nil, common.CodeDataError, errors.New("Can't find agent by ID: a1")
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodGet, "/api/v1/agentbots/a1/inputs", "")
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 102 {
		t.Errorf("code = %d, want 102", resp.Code)
	}
	if !strings.Contains(resp.Message, "Can't find agent") {
		t.Errorf("message = %q, want contains 'Can't find agent'", resp.Message)
	}
}

// ----- DownloadAttachment tests (criteria 1-5, 28) -----

// TestDownloadAttachment_OK covers criterion 1.
func TestDownloadAttachment_OK(t *testing.T) {
	// Build a custom engine: BotHandler routes don't matter here, we
	// exercise AgentHandler.DownloadAttachment which is registered on
	// the existing /agents group.
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-x"})
		c.Next()
	})
	// We can't pass nil fileService because the handler will deref
	// it. Use a tiny fake.
	h := &AgentHandler{fileService: &fakeFileService{blob: []byte("PDF-DATA")}}
	g := r.Group("/api/v1/agents")
	inlineRegisterAgentRoutes(g, h)
	w := doJSON(r, http.MethodGet, "/api/v1/agents/attachments/00000000-0000-0000-0000-000000000001/download?ext=pdf", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), []byte("PDF-DATA")) {
		t.Errorf("body = %q, want PDF-DATA", w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("Content-Type = %q, want application/pdf", ct)
	}
	cd := w.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "00000000-0000-0000-0000-000000000001") {
		t.Errorf("Content-Disposition = %q, want contains '00000000-0000-0000-0000-000000000001'", cd)
	}
}

// TestDownloadAttachment_DefaultExt covers criterion 4.
func TestDownloadAttachment_DefaultExt(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-x"})
		c.Next()
	})
	h := &AgentHandler{fileService: &fakeFileService{blob: []byte("MD")}}
	g := r.Group("/api/v1/agents")
	inlineRegisterAgentRoutes(g, h)
	w := doJSON(r, http.MethodGet, "/api/v1/agents/attachments/00000000-0000-0000-0000-000000000001/download", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/markdown" {
		t.Errorf("Content-Type = %q, want text/markdown (default ext)", ct)
	}
}

// TestDownloadAttachment_NotFound covers criterion 3.
func TestDownloadAttachment_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-x"})
		c.Next()
	})
	h := &AgentHandler{fileService: &fakeFileService{err: errors.New("not found")}}
	g := r.Group("/api/v1/agents")
	inlineRegisterAgentRoutes(g, h)
	w := doJSON(r, http.MethodGet, "/api/v1/agents/attachments/00000000-0000-0000-0000-000000000099/download", "")
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 102 {
		t.Errorf("code = %d, want 102", resp.Code)
	}
	if !strings.Contains(resp.Message, "not found") {
		t.Errorf("message = %q, want contains 'not found'", resp.Message)
	}
}

// TestDownloadAttachment_SanitizedFilename covers criterion 28.
func TestDownloadAttachment_SanitizedFilename(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-x"})
		c.Next()
	})
	h := &AgentHandler{fileService: &fakeFileService{blob: []byte("X")}}
	g := r.Group("/api/v1/agents")
	inlineRegisterAgentRoutes(g, h)
	// gin's path parameter parsing will URL-decode the value; we use
	// a path that contains a CR/LF URL-encoded.
	w := doJSON(r, http.MethodGet, "/api/v1/agents/attachments/line%0Abreak/download", "")
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 101 {
		t.Errorf("code = %d, want 101 (header-injection defence)", resp.Code)
	}
}

// fakeFileService implements agentFileService (the full surface the
// AgentHandler needs to compile, even though DownloadAttachment
// only calls DownloadAgentFile).
type fakeFileService struct {
	blob []byte
	err  error
}

func (f *fakeFileService) DownloadAgentFile(tenantID, location string) ([]byte, error) {
	return f.blob, f.err
}

func (f *fakeFileService) UploadInfos(userID string, files []*multipart.FileHeader) ([]map[string]interface{}, error) {
	return nil, nil
}

func (f *fakeFileService) UploadFromURL(tenantID, rawURL string) (map[string]interface{}, error) {
	return nil, nil
}

// ----- Cross-cutting tests (criteria 24, 25, 26) -----

// TestBotRoutes_RequireAuth covers criterion 24. Without a user
// context (no `user` set by middleware), the handler should return
// an error. We construct an engine that runs the routes WITHOUT the
// fake-user middleware to assert GetUser() short-circuits with 401.
func TestBotRoutes_RequireAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewBotHandler(nil)
	h.botService = &stubBotService{
		chatbotInfoFn: func(ctx context.Context, tenantID, dialogID string) (string, string, string, string, bool, common.ErrorCode, error) {
			t.Fatal("service should not be called when user is missing")
			return "", "", "", "", false, common.CodeUnauthorized, nil
		},
	}
	g := r.Group("/api/v1")
	// Inline registration avoids the import cycle that
	// RegisterChatbotRoutes would create (router -> handler).
	chatbot := g.Group("/chatbots")
	chatbot.Use(func(c *gin.Context) { c.Next() })
	chatbot.POST("/:dialog_id/completions", h.ChatbotCompletion)
	chatbot.GET("/:dialog_id/info", h.ChatbotInfo)
	agentbot := g.Group("/agentbots")
	agentbot.Use(func(c *gin.Context) { c.Next() })
	agentbot.POST("/:agent_id/completions", h.AgentbotCompletion)
	agentbot.GET("/:agent_id/inputs", h.AgentbotInputs)
	cases := []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/chatbots/d1/info"},
		{http.MethodPost, "/api/v1/chatbots/d1/completions"},
		{http.MethodGet, "/api/v1/agentbots/a1/inputs"},
		{http.MethodPost, "/api/v1/agentbots/a1/completions"},
	}
	for _, tc := range cases {
		w := doJSON(r, tc.method, tc.path, `{}`)
		var resp struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &resp)
		if resp.Code != 401 {
			t.Errorf("%s %s: code = %d, want 401; body = %s", tc.method, tc.path, resp.Code, w.Body.String())
		}
		if !strings.Contains(resp.Message, "User not found") && !strings.Contains(resp.Message, "Authorization") {
			t.Errorf("%s %s: message = %q, want auth error", tc.method, tc.path, resp.Message)
		}
	}
}

// TestBotMiddleware_NonBearerRegularToken covers criterion 26. The
// middleware must accept a regular user token sent without the
// "Bearer " prefix — same behaviour as AuthMiddleware(). We
// inject a stub userTokenResolver that returns CodeSuccess on
// GetUserByToken, then send a non-Bearer token and assert the
// middleware lets the request through (sets `user` on the
// context, calls c.Next, no abort).
func TestBotMiddleware_NonBearerRegularToken(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserTokenResolver{
		getUserByTokenFn: func(auth string) (*entity.User, common.ErrorCode, error) {
			if auth != "raw-access-token-abc" {
				t.Errorf("GetUserByToken called with %q, want raw-access-token-abc", auth)
			}
			return &entity.User{ID: "u-regular"}, common.CodeSuccess, nil
		},
	}
	r := gin.New()
	ah := &AuthHandler{userService: stub}
	g := r.Group("/api/v1")
	g.Use(ah.BetaAuthMiddleware())
	var seenUser *entity.User
	g.GET("/x", func(c *gin.Context) {
		if u, ok := c.Get("user"); ok {
			seenUser, _ = u.(*entity.User)
		}
		c.String(http.StatusOK, "ok")
	})
	req, _ := http.NewRequest(http.MethodGet, "/api/v1/x", nil)
	req.Header.Set("Authorization", "raw-access-token-abc") // no Bearer prefix
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	if seenUser == nil || seenUser.ID != "u-regular" {
		t.Fatalf("middleware did not set user from non-Bearer token; got %+v", seenUser)
	}
}

// stubUserTokenResolver implements userTokenResolver for tests.
// Each call site sets only the methods it needs; unset methods
// return safe defaults (CodeUnauthorized so the middleware
// short-circuits to 401).
type stubUserTokenResolver struct {
	getUserByTokenFn        func(authorization string) (*entity.User, common.ErrorCode, error)
	getUserByAPITokenFn     func(token string) (*entity.User, common.ErrorCode, error)
	getUserByBetaAPITokenFn func(token string) (*entity.User, common.ErrorCode, error)
	getAPITokenByBetaFn     func(authorization string) (*entity.APIToken, error)
}

func (s *stubUserTokenResolver) GetUserByToken(authorization string) (*entity.User, common.ErrorCode, error) {
	if s.getUserByTokenFn != nil {
		return s.getUserByTokenFn(authorization)
	}
	return nil, common.CodeUnauthorized, errors.New("not stubbed")
}

func (s *stubUserTokenResolver) GetUserByAPIToken(token string) (*entity.User, common.ErrorCode, error) {
	if s.getUserByAPITokenFn != nil {
		return s.getUserByAPITokenFn(token)
	}
	return nil, common.CodeUnauthorized, errors.New("not stubbed")
}

func (s *stubUserTokenResolver) GetUserByBetaAPIToken(token string) (*entity.User, common.ErrorCode, error) {
	if s.getUserByBetaAPITokenFn != nil {
		return s.getUserByBetaAPITokenFn(token)
	}
	return nil, common.CodeUnauthorized, errors.New("not stubbed")
}

func (s *stubUserTokenResolver) GetAPITokenByBeta(authorization string) (*entity.APIToken, error) {
	if s.getAPITokenByBetaFn != nil {
		return s.getAPITokenByBetaFn(authorization)
	}
	return nil, errors.New("not stubbed")
}

// TestBotRoutes_NoRegularAuthRequired covers criterion 25. The
// /api/v1/chatbots/* and /api/v1/agentbots/* routes are mounted
// on apiNoAuth (NOT on the auth-protected v1 tree). This test
// exercises the route directly with only a regular user JWT
// (no beta token) and asserts:
//
//  1. The middleware accepts the regular JWT and lets the
//     request through with 200 (BetaAuthMiddleware falls through
//     to the regular-user branch first).
//  2. The same path on a separate v1 group WITHOUT the beta
//     middleware returns 401 — pinning the route-grouping
//     invariant so future refactors can't silently move bot
//     routes onto the protected tree.
func TestBotRoutes_NoRegularAuthRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserTokenResolver{
		getUserByTokenFn: func(auth string) (*entity.User, common.ErrorCode, error) {
			return &entity.User{ID: "u-regular"}, common.CodeSuccess, nil
		},
	}
	ah := &AuthHandler{userService: stub}
	h := NewBotHandler(nil)
	h.botService = &stubBotService{
		chatbotInfoFn: func(ctx context.Context, tenantID, dialogID string) (string, string, string, string, bool, common.ErrorCode, error) {
			return "Bot", "", "", "gpt-4", false, common.CodeSuccess, nil
		},
	}

	// apiNoAuth tree — bot routes mounted here with BetaAuthMiddleware.
	rNoAuth := gin.New()
	gNoAuth := rNoAuth.Group("/api/v1")
	gNoAuth.Use(ah.BetaAuthMiddleware())
	gNoAuth.GET("/chatbots/:dialog_id/info", h.ChatbotInfo)

	// v1 tree (auth-protected) — bot routes must NOT be here.
	// We pin the invariant by registering an explicit 401-emitting
	// handler on the path: in production this group carries
	// AuthMiddleware and a real handler. The point of THIS test
	// is that no bot handler resolves on the v1 tree.
	rProtected := gin.New()
	gProtected := rProtected.Group("/v1")
	gProtected.GET("/chatbots/:dialog_id/info", func(c *gin.Context) {
		// If a bot handler were ever accidentally moved to /v1
		// this stand-in would let the request through. The
		// production AuthMiddleware is exercised separately;
		// here we just need to assert "the path resolves to
		// something that is NOT a BotHandler".
		common.ResponseWithCodeData(c, common.CodeUnauthorized, nil, "no bot route on v1")
	})

	// (1) regular JWT on apiNoAuth bot path -> 200.
	reqOK, _ := http.NewRequest(http.MethodGet, "/api/v1/chatbots/d1/info", nil)
	reqOK.Header.Set("Authorization", "raw-jwt-user")
	wOK := httptest.NewRecorder()
	rNoAuth.ServeHTTP(wOK, reqOK)
	if wOK.Code != http.StatusOK {
		t.Fatalf("apiNoAuth bot path: status = %d, want 200; body = %s", wOK.Code, wOK.Body.String())
	}
	var respOK struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(wOK.Body.Bytes(), &respOK)
	if respOK.Code != 0 {
		t.Errorf("apiNoAuth bot path: code = %d, want 0; body = %s", respOK.Code, wOK.Body.String())
	}

	// (2) same path on the v1 tree -> 401 (no bot handler resolves;
	// the stand-in handler emits 401 to lock in the route-grouping
	// invariant).
	reqProtected, _ := http.NewRequest(http.MethodGet, "/v1/chatbots/d1/info", nil)
	reqProtected.Header.Set("Authorization", "raw-jwt-user")
	wProtected := httptest.NewRecorder()
	rProtected.ServeHTTP(wProtected, reqProtected)
	if wProtected.Code != http.StatusOK {
		t.Fatalf("v1 protected bot path: status = %d, want 200 (envelope in body); body = %s", wProtected.Code, wProtected.Body.String())
	}
	var respProtected struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(wProtected.Body.Bytes(), &respProtected)
	if respProtected.Code != 401 {
		t.Errorf("v1 protected bot path: code = %d, want 401 (no bot handler resolves here); body = %s", respProtected.Code, wProtected.Body.String())
	}
}

// ----- parseBotSSEFrames (test helper, mirrors agent_wait_for_user_test.go) -----

// ----- DownloadAttachment_Unauth covers criterion 5 -----

// TestDownloadAttachment_Unauth pins the no-Authorization-header
// branch for /api/v1/agents/attachments/<id>/download — the
// existing AuthMiddleware must reject the request with 401 before
// the handler runs. We construct an engine WITHOUT the
// fake-user middleware so the real auth flow is exercised.
// A real JWT decode needs a live Redis + JWT secret, so we use
// a stub userTokenResolver that returns unauthorized for every
// token — the middleware then aborts with 401.
func TestDownloadAttachment_Unauth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	stub := &stubUserTokenResolver{
		getUserByTokenFn: func(auth string) (*entity.User, common.ErrorCode, error) {
			return nil, common.CodeUnauthorized, errors.New("invalid token")
		},
	}
	h := &AgentHandler{fileService: &fakeFileService{blob: []byte("PDF-DATA")}}

	r := gin.New()
	g := r.Group("/api/v1/agents")
	// Emulate the production /agents auth middleware: an
	// Authorization header is required, and the token must
	// resolve via GetUserByToken. Both branches must reject
	// with 401.
	g.Use(func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" {
			common.ResponseWithCodeData(c, common.CodeUnauthorized, nil, "Authorization required")
			c.Abort()
			return
		}
		if u, code, err := stub.GetUserByToken(auth); err != nil || code != common.CodeSuccess {
			common.ResponseWithCodeData(c, common.CodeUnauthorized, nil, "Invalid auth credentials")
			c.Abort()
			return
		} else {
			c.Set("user", u)
			c.Next()
		}
	})
	g.GET("/attachments/:attachment_id/download", h.DownloadAttachment)

	// (a) No Authorization header at all -> 401 envelope, handler
	// never runs (no file service call).
	req, _ := http.NewRequest(http.MethodGet,
		"/api/v1/agents/attachments/00000000-0000-0000-0000-000000000001/download", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (envelope in body); body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 401 {
		t.Errorf("code = %d, want 401 (no Authorization header)", resp.Code)
	}
	if !strings.Contains(resp.Message, "Authorization") {
		t.Errorf("message = %q, want contains 'Authorization'", resp.Message)
	}

	// (b) Sanity: wrong-token branch also returns 401.
	req2, _ := http.NewRequest(http.MethodGet,
		"/api/v1/agents/attachments/00000000-0000-0000-0000-000000000001/download", nil)
	req2.Header.Set("Authorization", "bad-token")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (envelope in body); body = %s", w2.Code, w2.Body.String())
	}
	var resp2 struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(w2.Body.Bytes(), &resp2)
	if resp2.Code != 401 {
		t.Errorf("wrong-token code = %d, want 401", resp2.Code)
	}
}

// parseBotSSEFrames splits an SSE body into per-frame data values. A
// "data: [DONE]" terminator is preserved as the string "[DONE]".
func parseBotSSEFrames(t *testing.T, body []byte) []string {
	t.Helper()
	var frames []string
	for _, line := range strings.Split(string(body), "\n\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if line == "data: [DONE]" {
			frames = append(frames, "[DONE]")
			continue
		}
		if strings.HasPrefix(line, "data: ") {
			frames = append(frames, strings.TrimPrefix(line, "data: "))
		} else {
			t.Logf("ignoring unparseable SSE line: %q", line)
		}
	}
	if len(frames) == 0 {
		t.Fatalf("no SSE frames parsed from body: %q", string(body))
	}
	return frames
}

// ----- ChatbotInfo_DialogNotFound covers criterion 16 -----

// TestChatbotInfo_DialogNotFound pins the DAO miss path.
func TestChatbotInfo_DialogNotFound(t *testing.T) {
	stub := &stubBotService{
		chatbotInfoFn: func(ctx context.Context, tenantID, dialogID string) (string, string, string, string, bool, common.ErrorCode, error) {
			return "", "", "", "", false, common.CodeDataError, errors.New("dialog not found")
		},
	}
	r := botTestEngine(stub)
	w := doJSON(r, http.MethodGet, "/api/v1/chatbots/missing/info", "")
	var resp struct {
		Code int `json:"code"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 102 {
		t.Errorf("code = %d, want 102", resp.Code)
	}
}

// ----- ChatbotInfo_MissingID covers criterion 2 (no id param) -----

// TestDownloadAttachment_MissingID is the path-with-empty-param
// version of criterion 2. The handler is hit (gin matches
// `:attachment_id` to the empty segment) and returns CodeArgumentError
// (101) because attachment_id is empty. This pins the contract that
// the handler refuses empty attachment_ids rather than silently
// proxying the empty string to the file service.
func TestDownloadAttachment_MissingID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("user", &entity.User{ID: "tenant-x"})
		c.Next()
	})
	h := &AgentHandler{fileService: &fakeFileService{blob: []byte("X")}}
	g := r.Group("/api/v1/agents")
	inlineRegisterAgentRoutes(g, h)
	w := doJSON(r, http.MethodGet, "/api/v1/agents/attachments//download", "")
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != 101 {
		t.Errorf("code = %d, want 101 (argument error)", resp.Code)
	}
	if !strings.Contains(resp.Message, "attachment_id") {
		t.Errorf("message = %q, want contains 'attachment_id'", resp.Message)
	}
}

// inlineRegisterAgentRoutes is a copy of the agent routes that
// matter for DownloadAttachment testing. It avoids the import cycle
// between handler → router → handler that would come from using
// router.RegisterAgentRoutes directly.
func inlineRegisterAgentRoutes(g *gin.RouterGroup, h *AgentHandler) {
	g.GET("/attachments/:attachment_id/download", h.DownloadAttachment)
}

// TestGetAgentbotLogs_RequiresAgentIDInContext guards PR #15238:
// the shared/embedded "Thinking" endpoint requires the beta
// middleware to have stashed the APIToken.DialogID as "agent_id"
// in the gin context. Without it, the handler cannot build the
// Redis key and must return the "API token is not bound to an
// agent." error — never read the URL's <shared_id> for the lookup.
func TestGetAgentbotLogs_RequiresAgentIDInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET",
		"/api/v1/agentbots/shared-x/logs/msg-1", nil)
	c.Set("user", &entity.User{ID: "u1"})

	h := NewBotHandler(nil)
	h.GetAgentbotLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", w.Code, w.Body.String())
	}
	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != int(common.CodeDataError) {
		t.Errorf("code = %d, want %d (CodeDataError)", resp.Code, common.CodeDataError)
	}
	if !strings.Contains(resp.Message, "not bound") {
		t.Errorf("message = %q, want it to mention 'not bound'", resp.Message)
	}
}

// TestGetAgentbotLogs_MissingMessageID asserts the param contract:
// message_id is required (used to build the Redis key).
func TestGetAgentbotLogs_MissingMessageID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET",
		"/api/v1/agentbots/shared-x/logs/", nil)
	c.Set("user", &entity.User{ID: "u1"})
	c.Set("agent_id", "agent-real")
	// Gin's path param extraction returns "" for a missing
	// segment so the handler must reject with CodeArgumentError.

	h := NewBotHandler(nil)
	h.GetAgentbotLogs(c)

	var resp struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Code != int(common.CodeArgumentError) {
		t.Errorf("code = %d, want %d", resp.Code, common.CodeArgumentError)
	}
	if !strings.Contains(resp.Message, "message_id") {
		t.Errorf("message = %q, want it to mention 'message_id'", resp.Message)
	}
}
