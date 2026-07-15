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

func newRAGconServer(t *testing.T, handler func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("expected Authorization=Bearer test-key, got %q", got)
			return
		}
		// The RAGcon client sets Content-Type: application/json on every
		// request, including the GET to /v1/models (see ragcon.go ListModels),
		// so this assertion intentionally applies even when there's no body.
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("expected Content-Type to start with application/json, got %q", got)
			return
		}
		var body map[string]interface{}
		if r.Method == http.MethodPost {
			raw, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read body: %v", err)
				return
			}
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("unmarshal: %v\nraw=%s", err, string(raw))
				return
			}
		}
		handler(t, r, body, w)
	}))
}

func newRAGconForTest(baseURL string) *RAGconModel {
	return NewRAGconModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v1/chat/completions", Models: "v1/models"},
	)
}

func TestRAGconName(t *testing.T) {
	if got := newRAGconForTest("http://unused").Name(); got != "ragcon" {
		t.Errorf("Name()=%q want %q", got, "ragcon")
	}
}

func TestRAGconFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("RAGcon", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*RAGconModel); !ok {
		t.Fatalf("driver type=%T, want *RAGconModel", driver)
	}
}

func TestRAGconChatHappyPath(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["model"] != "gpt-4o" {
			t.Errorf("model=%v", body["model"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v want false", body["stream"])
		}
		if body["max_tokens"] != float64(32) {
			t.Errorf("max_tokens=%v", body["max_tokens"])
		}
		messages, ok := body["messages"].([]interface{})
		if !ok || len(messages) != 1 {
			t.Fatalf("messages=%#v", body["messages"])
		}
		first, _ := messages[0].(map[string]interface{})
		if first["role"] != "user" || first["content"] != "ping" {
			t.Errorf("message=%#v", first)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content":           "pong",
					"reasoning_content": "thinking",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	mt := 32
	resp, err := newRAGconForTest(srv.URL).ChatWithMessages(
		"gpt-4o",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{MaxTokens: &mt},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	if *resp.ReasonContent != "thinking" {
		t.Errorf("ReasonContent=%q", *resp.ReasonContent)
	}
}

func TestRAGconChatRequiresApiKey(t *testing.T) {
	_, err := newRAGconForTest("http://unused").ChatWithMessages("gpt-4o", []Message{{Role: "user", Content: "x"}}, &APIConfig{}, nil)
	if err == nil || !strings.Contains(err.Error(), "api key is required") {
		t.Errorf("expected api-key error, got %v", err)
	}
}

func TestRAGconChatRequiresModelName(t *testing.T) {
	apiKey := "test-key"
	_, err := newRAGconForTest("http://unused").ChatWithMessages("", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "model name is required") {
		t.Errorf("expected model-name error, got %v", err)
	}
}

func TestRAGconChatRequiresMessages(t *testing.T) {
	apiKey := "test-key"
	_, err := newRAGconForTest("http://unused").ChatWithMessages("gpt-4o", nil, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Errorf("expected messages error, got %v", err)
	}
}

func TestRAGconChatSurfacesHTTPError(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		http.Error(w, "bad key", http.StatusUnauthorized)
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newRAGconForTest(srv.URL).ChatWithMessages("gpt-4o", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "status 401") {
		t.Errorf("expected HTTP status error, got %v", err)
	}
}

func TestRAGconChatRejectsProviderError(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]interface{}{"message": "invalid model"},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newRAGconForTest(srv.URL).ChatWithMessages("gpt-4o", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil)
	if err == nil || !strings.Contains(err.Error(), "upstream error") {
		t.Errorf("expected upstream error, got %v", err)
	}
}

func TestRAGconStreamHappyPath(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if body["stream"] != true {
			t.Errorf("stream=%v want true", body["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"choices":[{"delta":{"reasoning_content":"think"}}]}`+"\n\n"+
				`data: {"choices":[{"delta":{"content":"pong"}}]}`+"\n\n"+
				`data: [DONE]`+"\n\n",
		)
	})
	defer srv.Close()

	var contentParts []string
	var reasonParts []string
	apiKey := "test-key"
	err := newRAGconForTest(srv.URL).ChatStreamlyWithSender(
		"gpt-4o",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		func(content *string, reason *string) error {
			if content != nil {
				contentParts = append(contentParts, *content)
			}
			if reason != nil {
				reasonParts = append(reasonParts, *reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(reasonParts, "") != "think" {
		t.Errorf("reasonParts=%v", reasonParts)
	}
	if contentParts[0] != "pong" {
		t.Errorf("contentParts[0]=%q", contentParts[0])
	}
	if contentParts[len(contentParts)-1] != "[DONE]" {
		t.Errorf("missing trailing [DONE] sentinel, got %v", contentParts)
	}
}

func TestRAGconStreamRejectsNilSender(t *testing.T) {
	apiKey := "test-key"
	err := newRAGconForTest("http://unused").ChatStreamlyWithSender("gpt-4o", []Message{{Role: "user", Content: "x"}}, &APIConfig{ApiKey: &apiKey}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Errorf("expected sender error, got %v", err)
	}
}

func TestRAGconListModelsEnvelope(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/models" {
			t.Errorf("method/path=%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "gpt-4o"},
				{"id": "claude-3-7-sonnet"},
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newRAGconForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := []string{"gpt-4o", "claude-3-7-sonnet"}
	if strings.Join(models, ",") != strings.Join(want, ",") {
		t.Errorf("models=%v want %v", models, want)
	}
}

func TestRAGconListModelsBareList(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{"id": "llama-3.1-8b"},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	models, err := newRAGconForTest(srv.URL).ListModels(&APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 1 || models[0] != "llama-3.1-8b" {
		t.Errorf("models=%v", models)
	}
}

func TestRAGconCheckConnection(t *testing.T) {
	srv := newRAGconServer(t, func(t *testing.T, r *http.Request, body map[string]interface{}, w http.ResponseWriter) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"data": []map[string]interface{}{}})
	})
	defer srv.Close()

	apiKey := "test-key"
	if err := newRAGconForTest(srv.URL).CheckConnection(&APIConfig{ApiKey: &apiKey}); err != nil {
		t.Errorf("CheckConnection: %v", err)
	}
}

func TestRAGconUnsupportedMethodsReturnNoSuchMethod(t *testing.T) {
	m := newRAGconForTest("http://unused")
	apiKey := "test-key"
	cfg := &APIConfig{ApiKey: &apiKey}
	if _, err := m.Embed(nil, nil, cfg, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Embed=%v", err)
	}
	if _, err := m.Rerank(nil, "", nil, cfg, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Rerank=%v", err)
	}
	if _, err := m.Balance(cfg); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("Balance=%v", err)
	}
	if _, err := m.AudioSpeech(nil, nil, cfg, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("AudioSpeech=%v", err)
	}
	if _, err := m.OCRFile(nil, nil, nil, cfg, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("OCRFile=%v", err)
	}
	if _, err := m.ParseFile(nil, nil, nil, cfg, nil); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ParseFile=%v", err)
	}
	if _, err := m.ListTasks(cfg); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ListTasks=%v", err)
	}
	if _, err := m.ShowTask("", cfg); err == nil || !strings.Contains(err.Error(), "no such method") {
		t.Errorf("ShowTask=%v", err)
	}
}
