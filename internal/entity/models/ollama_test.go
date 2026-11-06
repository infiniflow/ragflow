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
	"ragflow/internal/common"
	"reflect"
	"testing"
)

func newOllamaForListModelsTest(baseURL string) *OllamaModel {
	return NewOllamaModel(map[string]string{"default": baseURL}, URLSuffix{Models: "api/tags"})
}

func newOllamaForChatTest(baseURL string) *OllamaModel {
	return NewOllamaModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "api/chat", AsyncChat: "api/chat"})
}

func ollamaMultimodalTestMessages() []Message {
	return []Message{{Role: "user", Content: []interface{}{
		map[string]interface{}{"type": "text", "text": "describe"},
		map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "data:image/png;base64,aGVsbG8="}},
		map[string]interface{}{"type": "text", "text": "carefully"},
		map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "cmF3LWltYWdl"}},
		map[string]interface{}{"type": "image_url", "image_url": map[string]interface{}{"url": "https://example.com/cat.png"}},
	}}}
}

func assertOllamaMultimodalRequest(t *testing.T, body map[string]interface{}, stream bool) {
	t.Helper()
	if body["stream"] != stream {
		t.Errorf("stream=%v, want %v", body["stream"], stream)
	}
	messages, ok := body["messages"].([]interface{})
	if !ok || len(messages) != 1 {
		t.Errorf("messages=%v, want one message", body["messages"])
		return
	}
	message, ok := messages[0].(map[string]interface{})
	if !ok {
		t.Errorf("message=%v, want object", messages[0])
		return
	}
	if message["content"] != "describe\ncarefully" {
		t.Errorf("content=%v, want joined text parts", message["content"])
	}
	images, ok := message["images"].([]interface{})
	if !ok {
		t.Errorf("images=%v, want array", message["images"])
		return
	}
	want := []string{"aGVsbG8=", "cmF3LWltYWdl", "https://example.com/cat.png"}
	if len(images) != len(want) {
		t.Errorf("images=%v, want %v", images, want)
		return
	}
	for i, expected := range want {
		if images[i] != expected {
			t.Errorf("images[%d]=%v, want %q", i, images[i], expected)
		}
	}
}

func TestOllamaChatMapsMultimodalImages(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		assertOllamaMultimodalRequest(t, body, false)
		_, _ = io.WriteString(w, `{"message":{"content":"ok","thinking":""}}`)
	}))
	defer srv.Close()

	response, err := newOllamaForChatTest(srv.URL).ChatWithMessages(
		t.Context(),
		"llava",
		ollamaMultimodalTestMessages(),
		&APIConfig{},
		&ChatConfig{},
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if response.Answer == nil || *response.Answer != "ok" {
		t.Fatalf("answer=%v, want ok", response.Answer)
	}
}

func TestOllamaStreamingChatMapsMultimodalImages(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		assertOllamaMultimodalRequest(t, body, true)
		_, _ = io.WriteString(w, "{\"message\":{\"content\":\"ok\"},\"done\":false}\n{\"done\":true}\n")
	}))
	defer srv.Close()

	err := newOllamaForChatTest(srv.URL).ChatStreamlyWithSender(
		t.Context(),
		"llava",
		ollamaMultimodalTestMessages(),
		&APIConfig{},
		&ChatConfig{},
		&common.ModelUsage{},
		func(*string, *string) error { return nil },
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
}

func TestOllamaChatSendsToolCallArgumentsAsObject(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		assertOllamaToolCallArgumentsAreObjects(t, body)
		_, _ = io.WriteString(w, `{"message":{"content":"ok","thinking":""}}`)
	}))
	defer srv.Close()

	response, err := newOllamaForChatTest(srv.URL).ChatWithMessages(
		t.Context(),
		"llama3",
		ollamaToolCallTestMessages(),
		&APIConfig{},
		&ChatConfig{},
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if response.Answer == nil || *response.Answer != "ok" {
		t.Fatalf("answer=%v, want ok", response.Answer)
	}
}

func TestOllamaStreamingChatSendsToolCallArgumentsAsObject(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}
		assertOllamaToolCallArgumentsAreObjects(t, body)
		_, _ = io.WriteString(w, "{\"message\":{\"content\":\"ok\"},\"done\":false}\n{\"done\":true}\n")
	}))
	defer srv.Close()

	err := newOllamaForChatTest(srv.URL).ChatStreamlyWithSender(
		t.Context(),
		"llama3",
		ollamaToolCallTestMessages(),
		&APIConfig{},
		&ChatConfig{},
		&common.ModelUsage{},
		func(*string, *string) error { return nil },
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
}

// ollamaToolCallTestMessages builds a multi-turn history whose assistant
// message carries tool_calls with OpenAI-style string arguments, the shape
// appendToolResults leaves in history for the next request.
func ollamaToolCallTestMessages() []Message {
	return []Message{
		{Role: "user", Content: "What is the weather in Paris?"},
		{
			Role:    "assistant",
			Content: nil,
			ToolCalls: []map[string]any{
				{
					"id":   "call_123",
					"type": "function",
					"function": map[string]any{
						"name":      "get_weather",
						"arguments": `{"location":"Paris"}`,
					},
				},
			},
		},
		{Role: "tool", Content: "Sunny, 18C", ToolCallID: "call_123"},
		{Role: "user", Content: "Thanks"},
	}
}

// assertOllamaToolCallArgumentsAreObjects verifies that every assistant
// tool_call in the request payload carries function.arguments as a JSON
// object (Ollama format), not a JSON string (OpenAI format).
func assertOllamaToolCallArgumentsAreObjects(t *testing.T, body map[string]interface{}) {
	t.Helper()
	messages, ok := body["messages"].([]interface{})
	if !ok {
		t.Fatalf("messages=%v, want array", body["messages"])
	}
	var found bool
	for _, m := range messages {
		msg, ok := m.(map[string]interface{})
		if !ok || msg["role"] != "assistant" {
			continue
		}
		calls, ok := msg["tool_calls"].([]interface{})
		if !ok || len(calls) == 0 {
			continue
		}
		found = true
		for _, c := range calls {
			call, ok := c.(map[string]interface{})
			if !ok {
				t.Fatalf("tool_call=%v, want object", c)
			}
			fn, ok := call["function"].(map[string]interface{})
			if !ok {
				t.Fatalf("function=%v, want object", call["function"])
			}
			args, ok := fn["arguments"].(map[string]interface{})
			if !ok {
				t.Fatalf("arguments=%v (%T), want JSON object", fn["arguments"], fn["arguments"])
			}
			if args["location"] != "Paris" {
				t.Errorf("arguments[location]=%v, want Paris", args["location"])
			}
		}
	}
	if !found {
		t.Fatal("no assistant message with tool_calls in request payload")
	}
}

// TestOllamaBuildMessagesDoesNotMutateToolCallArguments asserts that
// buildOllamaMessages leaves the caller's messages slice untouched: the
// assistant tool_calls arguments must still be a JSON string (the form
// appendToolResults and OpenAI-compatible providers expect on the wire) after
// the Ollama request path has converted its own copy to objects. This is the
// load-bearing part of the implementation, so it is covered as a property
// test rather than only in prose.
func TestOllamaBuildMessagesDoesNotMutateToolCallArguments(t *testing.T) {
	withSSRFBypass(t)
	messages := ollamaToolCallTestMessages()
	original := messages[1].ToolCalls[0]["function"].(map[string]any)["arguments"].(string)

	_ = buildOllamaMessages(messages)

	got, ok := messages[1].ToolCalls[0]["function"].(map[string]any)["arguments"].(string)
	if !ok {
		t.Fatalf("input arguments type=%T, want string (must not be mutated)",
			messages[1].ToolCalls[0]["function"].(map[string]any)["arguments"])
	}
	if got != original {
		t.Fatalf("input arguments=%q, want %q (must not be mutated)", got, original)
	}
}

// TestOllamaNormalizesMalformedToolCallArguments verifies that empty,
// whitespace-only, and unparseable string arguments are normalised to an empty
// JSON object rather than left as strings. Ollama rejects the whole /api/chat
// request when arguments is not an object, so leaving these as strings would
// reproduce the 400 the request-path conversion was meant to close.
func TestOllamaNormalizesMalformedToolCallArguments(t *testing.T) {
	withSSRFBypass(t)
	messages := []Message{
		{Role: "user", Content: "run the tools"},
		{
			Role:    "assistant",
			Content: nil,
			ToolCalls: []map[string]any{
				{"id": "call_empty", "type": "function", "function": map[string]any{"name": "a", "arguments": ""}},
				{"id": "call_ws", "type": "function", "function": map[string]any{"name": "b", "arguments": "   "}},
				{"id": "call_bad", "type": "function", "function": map[string]any{"name": "c", "arguments": "{not json"}},
				{"id": "call_arr", "type": "function", "function": map[string]any{"name": "d", "arguments": "[1,2]"}},
				{"id": "call_scalar", "type": "function", "function": map[string]any{"name": "e", "arguments": "123"}},
				{"id": "call_obj", "type": "function", "function": map[string]any{"name": "f", "arguments": `{"x":1}`}},
			},
		},
	}

	apiMessages := buildOllamaMessages(messages)
	assistant, ok := apiMessages[1].(map[string]any)
	if !ok {
		t.Fatalf("apiMessages[1]=%T, want map", apiMessages[1])
	}
	calls, ok := assistant["tool_calls"].([]map[string]any)
	if !ok {
		t.Fatalf("tool_calls=%T, want []map[string]any", assistant["tool_calls"])
	}
	want := map[string]any{}
	for i, call := range calls {
		fn, ok := call["function"].(map[string]any)
		if !ok {
			t.Fatalf("call[%d].function=%T, want map", i, call["function"])
		}
		args, ok := fn["arguments"].(map[string]any)
		if !ok {
			t.Errorf("call[%d] arguments=%v (%T), want JSON object", i, fn["arguments"], fn["arguments"])
			continue
		}
		if call["id"] == "call_obj" {
			if args["x"] != float64(1) {
				t.Errorf("call[%d] arguments=%v, want {\"x\":1}", i, args)
			}
			continue
		}
		if !reflect.DeepEqual(args, want) {
			t.Errorf("call[%d] (id=%v) arguments=%v, want empty object {}", i, call["id"], args)
		}
	}
}

func TestOllamaThinkingPayloadTriState(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name string
		cfg  *ChatConfig
		want any
	}{
		{name: "nil omitted"},
		{name: "false", cfg: &ChatConfig{Thinking: boolPointer(false)}, want: false},
		{name: "true", cfg: &ChatConfig{Thinking: boolPointer(true)}, want: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]interface{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				_, _ = io.WriteString(w, `{"message":{"content":"ok","thinking":""}}`)
			}))
			defer srv.Close()

			if _, err := newOllamaForChatTest(srv.URL).ChatWithMessages(t.Context(), "llava", []Message{{Role: "user", Content: "hello"}}, &APIConfig{}, tc.cfg, nil); err != nil {
				t.Fatalf("ChatWithMessages: %v", err)
			}
			got, exists := body["think"]
			if tc.want == nil {
				if exists {
					t.Fatalf("think=%v, want omitted", got)
				}
				return
			}
			if !exists || got != tc.want {
				t.Fatalf("think=%v, want %v", got, tc.want)
			}
			if options, exists := body["options"]; exists {
				if optionsMap, ok := options.(map[string]interface{}); ok {
					if _, nested := optionsMap["think"]; nested {
						t.Fatal("think must not be nested in options")
					}
				}
			}
		})
	}
}

func TestOllamaThinkingOverridesEffortOnlyForGPTOSS(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name, model string
		want        any
	}{
		{name: "gpt-oss uses effort", model: "gpt-oss:20b", want: "high"},
		{name: "other model keeps thinking", model: "llava", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var body map[string]interface{}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Errorf("decode request: %v", err)
				}
				_, _ = io.WriteString(w, `{"message":{"content":"ok","thinking":""}}`)
			}))
			defer srv.Close()
			thinking := false
			effort := "high"
			if _, err := newOllamaForChatTest(srv.URL).ChatWithMessages(t.Context(), tc.model, []Message{{Role: "user", Content: "hello"}}, &APIConfig{}, &ChatConfig{Thinking: &thinking, Effort: &effort}, nil); err != nil {
				t.Fatalf("ChatWithMessages: %v", err)
			}
			if got := body["think"]; got != tc.want {
				t.Fatalf("think=%v, want %v", got, tc.want)
			}
		})
	}
}

func TestOllamaListModels(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method=%s, want GET", r.Method)
		}
		if r.URL.Path != "/api/tags" {
			t.Errorf("path=%s, want /api/tags", r.URL.Path)
		}
		// Ollama's /api/tags response shape (name + model fields).
		_, _ = io.WriteString(w, `{"models":[{"name":"llama3:latest","model":"llama3:latest"},{"name":"qwen3:8b","model":"qwen3:8b"}]}`)
	}))
	defer srv.Close()

	models, err := newOllamaForListModelsTest(srv.URL).ListModels(ctx, &APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("len(models)=%d, want 2", len(models))
	}
	if models[0].Name != "llama3:latest" || models[1].Name != "qwen3:8b" {
		t.Fatalf("names=%v, want [llama3:latest qwen3:8b]", []string{models[0].Name, models[1].Name})
	}
}

func TestOllamaListModelsFallsBackToModelField(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Some entries may carry only the "model" field; it should be used as the name.
		_, _ = io.WriteString(w, `{"models":[{"model":"phi3:mini"},{"name":""},{"name":"  "}]}`)
	}))
	defer srv.Close()

	models, err := newOllamaForListModelsTest(srv.URL).ListModels(ctx, &APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 {
		t.Fatalf("len(models)=%d, want 1 (blank names skipped)", len(models))
	}
	if models[0].Name != "phi3:mini" {
		t.Fatalf("Name=%q, want phi3:mini", models[0].Name)
	}
}

func TestOllamaListModelsRejectsHTTPError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `boom`)
	}))
	defer srv.Close()

	if _, err := newOllamaForListModelsTest(srv.URL).ListModels(ctx, &APIConfig{}); err == nil {
		t.Fatal("ListModels: expected error for HTTP 500, got nil")
	}
}

func TestOllamaListModelsRequiresBaseURL(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	m := NewOllamaModel(map[string]string{}, URLSuffix{Models: "api/tags"})
	if _, err := m.ListModels(ctx, &APIConfig{}); err == nil {
		t.Fatal("ListModels: expected error for missing base URL, got nil")
	}
}
