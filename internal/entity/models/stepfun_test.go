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

package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newStepFunServer(t *testing.T, expectedPath string, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
		if r.Method == http.MethodPost {
			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
				t.Errorf("expected Content-Type to start with application/json, got %q", got)
				return
			}
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			var body map[string]interface{}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal: %v\nraw=%s", err, string(raw))
				return
			}
			handler(t, r, body, w)
			return
		}
		handler(t, r, nil, w)
	}))
}

func newStepFunForTest(baseURL string) *StepFunModel {
	return NewStepFunModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v1/chat/completions", Models: "v1/models"},
	)
}

func newStepFunSSEServer(t *testing.T, expectedPath, ssePayload string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			return
		}
		if r.URL.Path != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("expected Content-Type to start with application/json, got %q", got)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, ssePayload)
	}))
}

func TestStepFunName(t *testing.T) {
	if got := newStepFunForTest("http://unused").Name(); got != "stepfun" {
		t.Errorf("Name()=%q, want %q", got, "stepfun")
	}
}

func TestStepFunNewInstancePreservesConfig(t *testing.T) {
	model := NewStepFunModel(
		map[string]string{"default": "http://old.example"},
		URLSuffix{Chat: "chat", Models: "models"},
	)

	instance, ok := model.NewInstance(map[string]string{"default": "http://new.example"}).(*StepFunModel)
	if !ok {
		t.Fatalf("NewInstance type=%T, want *StepFunModel", instance)
	}
	if instance.baseModel.BaseURL["default"] != "http://new.example" {
		t.Errorf("BaseURL=%q", instance.baseModel.BaseURL["default"])
	}
	if instance.baseModel.URLSuffix.Chat != "chat" || instance.baseModel.URLSuffix.Models != "models" {
		t.Errorf("URLSuffix=%+v", instance.baseModel.URLSuffix)
	}
}

func TestStepFunChatParsesUsage(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-abc123",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "step-3.7-flash",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hello!",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     20,
				"completion_tokens": 15,
				"total_tokens":      35,
			},
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("Usage must be non-nil")
	}
	if resp.Usage.PromptTokens != 20 {
		t.Errorf("PromptTokens=%d, want 20", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 15 {
		t.Errorf("CompletionTokens=%d, want 15", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 35 {
		t.Errorf("TotalTokens=%d, want 35", resp.Usage.TotalTokens)
	}
}

func TestStepFunChatParsesExtendedUsage(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-ext",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "step-3.7-flash",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hello!",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     83,
				"completion_tokens": 176,
				"total_tokens":      259,
				"prompt_tokens_details": map[string]interface{}{
					"cached_tokens": 10,
				},
				"completion_tokens_details": map[string]interface{}{
					"reasoning_tokens": 50,
				},
			},
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("Usage must be non-nil")
	}
	if resp.Usage.PromptTokens != 83 {
		t.Errorf("PromptTokens=%d, want 83", resp.Usage.PromptTokens)
	}
	if resp.Usage.CompletionTokens != 176 {
		t.Errorf("CompletionTokens=%d, want 176", resp.Usage.CompletionTokens)
	}
	if resp.Usage.TotalTokens != 259 {
		t.Errorf("TotalTokens=%d, want 259", resp.Usage.TotalTokens)
	}
}

func TestStepFunChatFallsBackTotalTokens(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-nototal",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "step-3.7-flash",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hi",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 5,
			},
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Usage == nil {
		t.Fatal("Usage must be non-nil")
	}
	if resp.Usage.TotalTokens != 15 {
		t.Errorf("TotalTokens=%d, want 15 (prompt+completion)", resp.Usage.TotalTokens)
	}
}

func TestStepFunChatNilUsageWhenAllZero(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-zero",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "step-3.7-flash",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "Hello!",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     0,
				"completion_tokens": 0,
				"total_tokens":      0,
			},
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Usage == nil {
		t.Error("Usage must be non-nil")
	}
}

func TestStepFunChatExtractsReasoning(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-reasoning",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "step-3.7-flash",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":              "assistant",
					"content":           "The answer is 42.",
					"reasoning_content": "I need to think about this...",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 20,
				"total_tokens":      30,
			},
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "what is the meaning of life"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.ReasonContent == nil {
		t.Fatal("ReasonContent must be non-nil")
	}
	if *resp.ReasonContent != "I need to think about this..." {
		t.Errorf("ReasonContent=%q, want 'I need to think about this...'", *resp.ReasonContent)
	}
}

func TestStepFunChatFallsBackToReasoningContent(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-dscompat",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "step-3.7-flash",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":              "assistant",
					"content":           "Done.",
					"reasoning_content": "DeepSeek-compatible reasoning field.",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     5,
				"completion_tokens": 10,
				"total_tokens":      15,
			},
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.ReasonContent == nil {
		t.Fatal("ReasonContent must be non-nil")
	}
	if *resp.ReasonContent != "DeepSeek-compatible reasoning field." {
		t.Errorf("ReasonContent=%q, want 'DeepSeek-compatible reasoning field.'", *resp.ReasonContent)
	}
}

func TestStepFunChatAcceptsReasoningOnlyResponse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     "cmpl_reasoning_only",
			"object": "chat.completion",
			"model":  "step-3.7-flash",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":              "assistant",
					"content":           nil,
					"reasoning_content": "The answer is 4.",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     5,
				"completion_tokens": 10,
				"total_tokens":      15,
			},
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "what is 2+2"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v (should not error on reasoning-only response)", err)
	}
	if resp.Answer == nil {
		t.Error("Answer must be non-nil")
	} else if *resp.Answer != "" {
		t.Errorf("Answer=%q, want empty string", *resp.Answer)
	}
	if resp.ReasonContent == nil {
		t.Error("ReasonContent must be non-nil")
	} else if *resp.ReasonContent != "The answer is 4." {
		t.Errorf("ReasonContent=%q, want 'The answer is 4.'", *resp.ReasonContent)
	}
}

func TestStepFunStreamParsesUsage(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"content":"hi"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12}}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	chatConfig := &ChatConfig{}
	err := m.ChatStreamlyWithSender(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, chatConfig, nil,
		func(*string, *string) error { return nil })
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if chatConfig.UsageResult == nil {
		t.Fatal("UsageResult must be non-nil after stream with usage event")
	}
	if chatConfig.UsageResult.PromptTokens != 10 || chatConfig.UsageResult.CompletionTokens != 2 || chatConfig.UsageResult.TotalTokens != 12 {
		t.Errorf("UsageResult=%#v, want prompt=10 completion=2 total=12", chatConfig.UsageResult)
	}
}

func TestStepFunStreamNullUsage(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	// StepFun may send usage: null on intermediate chunks.
	srv := newStepFunSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"content":"hi"}}],"usage":null}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":8,"completion_tokens":2,"total_tokens":10}}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	chatConfig := &ChatConfig{}
	err := m.ChatStreamlyWithSender(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, chatConfig, nil,
		func(*string, *string) error { return nil })
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if chatConfig.UsageResult == nil {
		t.Fatal("UsageResult must be non-nil")
	}
	if chatConfig.UsageResult.TotalTokens != 10 {
		t.Errorf("UsageResult=%#v, want total=10", chatConfig.UsageResult)
	}
}

func TestStepFunStreamExtractsReasoning(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"think. "}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"reasoning_content":"done."}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"final answer"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	var content, reasoning []string
	err := m.ChatStreamlyWithSender(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(c *string, r *string) error {
			if c != nil && r != nil {
				t.Errorf("sender called with both args non-nil")
			}
			if r != nil && *r != "" {
				reasoning = append(reasoning, *r)
			}
			if c != nil && *c != "" && *c != "[DONE]" {
				content = append(content, *c)
			}
			return nil
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if got := strings.Join(reasoning, ""); got != "think. done." {
		t.Errorf("reasoning=%q", got)
	}
	if got := strings.Join(content, ""); got != "final answer" {
		t.Errorf("content=%q", got)
	}
}

func TestStepFunStreamFallsBackToReasoningContent(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"reasoning_content":"ds-style reasoning"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"ok"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	var reasoning []string
	err := m.ChatStreamlyWithSender(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(c *string, r *string) error {
			if r != nil && *r != "" {
				reasoning = append(reasoning, *r)
			}
			_ = c
			return nil
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if got := strings.Join(reasoning, ""); got != "ds-style reasoning" {
		t.Errorf("reasoning=%q", got)
	}
}

func TestStepFunStreamHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":"Hello"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":" world"},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	var chunks []string
	var sawDone bool
	err := m.ChatStreamlyWithSender(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(c *string, _ *string) error {
			if c == nil {
				return nil
			}
			if *c == "[DONE]" {
				sawDone = true
				return nil
			}
			chunks = append(chunks, *c)
			return nil
		})
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if strings.Join(chunks, "") != "Hello world" {
		t.Errorf("content=%v", chunks)
	}
	if !sawDone {
		t.Error("expected [DONE] sentinel")
	}
}

func TestStepFunStreamRequiresSender(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newStepFunForTest("http://unused")
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender-required error, got %v", err)
	}
}

func TestStepFunStreamFailsWithoutTerminal(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"delta":{"content":"half"}}]}`+"\n",
	)
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream ended before") {
		t.Errorf("expected truncation error, got %v", err)
	}
}

func TestStepFunChatRequiresAPIKey(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newStepFunForTest("http://unused")
	_, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestStepFunChatRequiresMessages(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newStepFunForTest("http://unused")
	apiKey := "test-key"
	_, err := m.ChatWithMessages(ctx, "step-3.7-flash", nil, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages-empty error, got %v", err)
	}
}

func TestStepFunChatRejectsHTTPError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	_, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 propagated, got %v", err)
	}
}

func TestStepFunChatHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "step-3.7-flash" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v want false", body["stream"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-ok",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "step-3.7-flash",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": "pong",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     5,
				"completion_tokens": 3,
				"total_tokens":      8,
			},
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	resp, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if resp.Answer == nil || resp.ReasonContent == nil {
		t.Fatalf("Answer/ReasonContent must be non-nil pointers, got Answer=%v ReasonContent=%v", resp.Answer, resp.ReasonContent)
	}
	if *resp.Answer != "pong" {
		t.Errorf("answer=%q want pong", *resp.Answer)
	}
	if *resp.ReasonContent != "" {
		t.Errorf("ReasonContent=%q want empty", *resp.ReasonContent)
	}
}

func TestStepFunChatSupportsToolCalls(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	var requestBody map[string]interface{}
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		requestBody = body
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-tools",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "step-3.7-flash",
			"choices": []map[string]interface{}{{
				"index": 0,
				"message": map[string]interface{}{
					"role":    "assistant",
					"content": nil,
					"tool_calls": []map[string]interface{}{{
						"id":   "call-1",
						"type": "function",
						"function": map[string]interface{}{
							"name":      "get_weather",
							"arguments": `{"city":"Beijing"}`,
						},
					}},
				},
				"finish_reason": "tool_calls",
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	toolChoice := "auto"
	resp, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "what's the weather in Beijing"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{
			Tools: []map[string]interface{}{
				{
					"type": "function",
					"function": map[string]interface{}{
						"name":        "get_weather",
						"description": "Get current weather",
					},
				},
			},
			ToolChoice: &toolChoice,
		},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if requestBody["tool_choice"] != "auto" {
		t.Fatalf("tool_choice=%#v, want auto", requestBody["tool_choice"])
	}
	if _, ok := requestBody["tools"].([]interface{}); !ok {
		t.Fatalf("tools missing or wrong type: %#v", requestBody["tools"])
	}
	if resp.Answer == nil || *resp.Answer != "" {
		t.Fatalf("Answer=%#v, want empty string for tool-call response", resp.Answer)
	}
	if len(resp.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len=%d, want 1", len(resp.ToolCalls))
	}
	fn, _ := resp.ToolCalls[0]["function"].(map[string]interface{})
	if fn["name"] != "get_weather" {
		t.Fatalf("tool call function=%#v, want get_weather", fn["name"])
	}
	if resp.Usage == nil || resp.Usage.TotalTokens != 15 {
		t.Fatalf("Usage=%#v, want total tokens 15", resp.Usage)
	}
}

func TestStepFunStreamDoesNotSendDoneAfterScannerError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunSSEServer(t, "/v1/chat/completions",
		`data: `+strings.Repeat("x", 1024*1024+1)+"\n",
	)
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	var sawDone bool
	err := m.ChatStreamlyWithSender(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(answer, _ *string) error {
			if answer != nil && *answer == "[DONE]" {
				sawDone = true
			}
			return nil
		})
	if err == nil {
		t.Fatal("expected scanner error")
	}
	if sawDone {
		t.Fatal("sender received [DONE] after scanner error")
	}
}

func TestStepFunChatRejectsMalformedResponse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/chat/completions", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"id":      "chatcmpl-bad",
			"object":  "chat.completion",
			"created": 1700000000,
			"model":   "step-3.7-flash",
			// No choices at all.
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	_, err := m.ChatWithMessages(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Errorf("expected no-choices error, got %v", err)
	}
}

func TestStepFunStreamRejectsMalformedFrame(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunSSEServer(t, "/v1/chat/completions",
		`data: {"choices":[{"index":0,"delta":{"content":"ok"}}]}`+"\n"+
			`data: {this is not valid json}`+"\n",
	)
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	err := m.ChatStreamlyWithSender(ctx, "step-3.7-flash",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(*string, *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "invalid SSE event") {
		t.Errorf("expected invalid-SSE error, got %v", err)
	}
}

func TestStepFunListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/models", func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if body != nil {
			t.Errorf("GET /models should not send a JSON body: %v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data": []map[string]interface{}{
				{"id": "step-3.7-flash"},
				{"id": "step-2"},
			},
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"

	models, err := m.ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Errorf("ListModels returned %d models, want 2", len(models))
	}

	if err := m.CheckConnection(ctx, &APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
}

func TestStepFunListModelsRejectsInvalidResponses(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newStepFunServer(t, "/v1/models", func(t *testing.T, _ *http.Request, _ map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			// Missing "data" field.
		})
	})
	defer srv.Close()

	m := newStepFunForTest(srv.URL)
	apiKey := "test-key"
	_, err := m.ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "invalid models list format") {
		t.Errorf("expected invalid-models error, got %v", err)
	}
}

func TestStepFunListModelsRequiresAPIKey(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newStepFunForTest("http://unused")
	_, err := m.ListModels(ctx, &APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestStepFunEmbedReturnsNotImplemented(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newStepFunForTest("http://unused")
	model := "x"
	_, err := m.Embed(ctx, &model, []string{"a"}, &APIConfig{}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Errorf("Embed: want 'not implemented', got %v", err)
	}
}

func TestStepFunRerankReturnsNoSuchMethod(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newStepFunForTest("http://unused")
	model := "x"
	_, err := m.Rerank(ctx, &model, "q", []string{"a"}, &APIConfig{}, &RerankConfig{TopN: 1}, nil)
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank: want 'no such method', got %v", err)
	}
}

func TestStepFunBalanceReturnsNoSuchMethod(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newStepFunForTest("http://unused")
	_, err := m.Balance(ctx, &APIConfig{})
	if err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance: want 'no such method', got %v", err)
	}
}

func TestStepFunAudioOCRReturnNoSuchMethod(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := newStepFunForTest("http://unused")
	model := "x"
	if _, err := m.TranscribeAudio(ctx, &model, &model, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("TranscribeAudio: want 'no such method', got %v", err)
	}
	if _, err := m.OCRFile(ctx, &model, nil, &model, &APIConfig{}, nil, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile: want 'no such method', got %v", err)
	}
}
