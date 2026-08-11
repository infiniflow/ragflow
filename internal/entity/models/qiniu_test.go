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
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQiniuToolCalls(t *testing.T) {
	newDriver := func(baseURL string) ModelDriver {
		return NewQiniuModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
	}
	t.Run("non-streaming", func(t *testing.T) {
		testNonStreamingToolCall(t, "deepseek/deepseek-v4-pro", "/chat/completions", newDriver)
	})
	t.Run("streaming", func(t *testing.T) {
		testStreamingToolCall(t, "deepseek/deepseek-v4-pro", "/chat/completions", newDriver)
	})
}

func TestQiniuChatStreamRejectsTruncatedResponse(t *testing.T) {
	ctx := t.Context()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"partial"}}]}` + "\n\n"))
	}))
	defer server.Close()

	apiKey := "test-key"
	model := NewQiniuModel(map[string]string{"default": server.URL}, URLSuffix{Chat: "chat/completions"})
	err := model.ChatStreamlyWithSender(ctx, "model", []Message{{Role: "user", Content: "hi"}}, &APIConfig{ApiKey: &apiKey}, &ChatConfig{}, nil, func(_, _ *string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "stream ended before [DONE] or finish_reason") {
		t.Fatalf("error = %v", err)
	}
}

func TestQiniuCheckConnectionUsesListModels(t *testing.T) {
	ctx := t.Context()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/models" {
			t.Errorf("path = %s, want /models", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer test-key")
		}
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"deepseek/deepseek-v4-flash"}]}`))
	}))
	defer server.Close()

	apiKey := "test-key"
	model := NewQiniuModel(map[string]string{"default": server.URL}, URLSuffix{Models: "models"})
	if err := model.CheckConnection(ctx, &APIConfig{ApiKey: &apiKey}); err != nil {
		t.Fatalf("CheckConnection() error = %v", err)
	}
}

func TestQiniuCheckConnectionPropagatesListModelsError(t *testing.T) {
	ctx := t.Context()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"invalid api key"}`, http.StatusUnauthorized)
	}))
	defer server.Close()

	apiKey := "invalid-key"
	model := NewQiniuModel(map[string]string{"default": server.URL}, URLSuffix{Models: "models"})
	err := model.CheckConnection(ctx, &APIConfig{ApiKey: &apiKey})
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Fatalf("CheckConnection() error = %v, want status 401", err)
	}
}
