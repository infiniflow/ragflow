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
