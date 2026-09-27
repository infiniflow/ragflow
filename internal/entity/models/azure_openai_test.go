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

func newAzureForTest(baseURL string) *AzureOpenAIModel {
	return NewAzureOpenAIModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "deployments", Embedding: "embeddings"},
	)
}

func newAzureServer(t *testing.T, deployment, op string, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()
	expectedPath := "/deployments/" + deployment + "/" + op + "?api-version=2024-10-21"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.String())
			return
		}
		if got := r.Header.Get("api-key"); got != "test-key" {
			t.Errorf("expected api-key=test-key, got %q", got)
			return
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("expected no Authorization header, got %q", got)
			return
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("expected Content-Type to start with application/json, got %q", got)
			return
		}
		if r.Method == http.MethodGet {
			handler(t, nil, w)
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
		handler(t, body, w)
	}))
}

func newAzureSSEServer(t *testing.T, deployment, op, ssePayload string) *httptest.Server {
	t.Helper()
	expectedPath := "/deployments/" + deployment + "/" + op + "?api-version=2024-10-21"
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
			return
		}
		if r.URL.String() != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.String())
			return
		}
		if got := r.Header.Get("api-key"); got != "test-key" {
			t.Errorf("expected api-key=test-key, got %q", got)
			return
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("expected no Authorization header, got %q", got)
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

func TestAzureName(t *testing.T) {
	if got := newAzureForTest("http://unused").Name(); got != "azure-openai" {
		t.Errorf("Name()=%q, want azure-openai", got)
	}
}

func TestAzureChatHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newAzureServer(t, "gpt-4o", "chat/completions", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		messages, ok := body["messages"].([]interface{})
		if !ok || len(messages) != 1 {
			t.Errorf("messages=%v", body["messages"])
		}
		if body["stream"] != false {
			t.Errorf("stream=%v want false", body["stream"])
		}
		if body["temperature"] != float64(1) {
			t.Errorf("temperature=%v want 1", body["temperature"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{"content": "pong"},
			}},
			"usage": map[string]interface{}{
				"prompt_tokens":     3,
				"completion_tokens": 5,
				"total_tokens":      8,
			},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	resp, err := newAzureForTest(srv.URL).ChatWithMessages(
		ctx,
		"gpt-4o",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
	)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q, want pong", *resp.Answer)
	}
}

func TestAzureChatRequiresDeployment(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	_, err := newAzureForTest("http://unused").ChatWithMessages(
		ctx,
		"",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "deployment name is required") {
		t.Fatalf("expected deployment name error, got %v", err)
	}
}

func TestAzureChatRequiresMessages(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	_, err := newAzureForTest("http://unused").ChatWithMessages(
		ctx,
		"gpt-4o",
		nil,
		&APIConfig{ApiKey: &apiKey}, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "messages is empty") {
		t.Fatalf("expected messages error, got %v", err)
	}
}

func TestAzureChatRejectsHTTPError(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newAzureServer(t, "gpt-4o", "chat/completions", func(t *testing.T, _ map[string]interface{}, w http.ResponseWriter) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	})
	defer srv.Close()

	apiKey := "test-key"
	_, err := newAzureForTest(srv.URL).ChatWithMessages(
		ctx,
		"gpt-4o",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAzureChatStreamHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newAzureSSEServer(t, "gpt-4o", "chat/completions",
		`data: {"choices":[{"index":0,"delta":{"role":"assistant","content":"hello"}}]}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{"content":" world"}}]}`+"\n"+
			`data: {"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`+"\n"+
			`data: {"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`+"\n"+
			`data: [DONE]`+"\n",
	)
	defer srv.Close()

	apiKey := "test-key"
	var content []string
	var sawDone bool
	err := newAzureForTest(srv.URL).ChatStreamlyWithSender(
		ctx,
		"gpt-4o",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
		func(c *string, _ *string) error {
			if c != nil && *c == "[DONE]" {
				sawDone = true
			}
			if c != nil && *c != "[DONE]" {
				content = append(content, *c)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("stream: %v", err)
	}
	if got := strings.Join(content, ""); got != "hello world" {
		t.Errorf("content=%q, want 'hello world'", got)
	}
	if !sawDone {
		t.Error("expected [DONE] sentinel")
	}
}

func TestAzureChatRequiresAPIKey(t *testing.T) {
	ctx := t.Context()
	_, err := newAzureForTest("http://unused").ChatWithMessages(
		ctx,
		"gpt-4o",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{}, nil, nil,
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAzureChatStreamRejectsExplicitFalse(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	stream := false
	err := newAzureForTest("http://unused").ChatStreamlyWithSender(
		ctx,
		"gpt-4o",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, &ChatConfig{Stream: &stream}, nil,
		func(c *string, _ *string) error { return nil },
	)
	if err == nil || !strings.Contains(err.Error(), "stream must be true") {
		t.Fatalf("expected stream error, got %v", err)
	}
}

func TestAzureStreamRequiresSender(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	apiKey := "test-key"
	err := newAzureForTest("http://unused").ChatStreamlyWithSender(
		ctx,
		"gpt-4o",
		[]Message{{Role: "user", Content: "x"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "sender is required") {
		t.Fatalf("expected sender error, got %v", err)
	}
}

func TestAzureListModelsHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	expectedPath := "/deployments?api-version=2024-10-21"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.String() != expectedPath {
			t.Errorf("expected path=%s, got %s", expectedPath, r.URL.String())
			return
		}
		if got := r.Header.Get("api-key"); got != "test-key" {
			t.Errorf("expected api-key=test-key, got %q", got)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"id": "gpt-4o"},
				{"id": "gpt-4-turbo"},
			},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	models, err := newAzureForTest(srv.URL).ListModels(ctx, &APIConfig{ApiKey: &apiKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if models[0].Name != "gpt-4o" || models[1].Name != "gpt-4-turbo" {
		t.Errorf("models=%v", models)
	}
}

func TestAzureEmbedHappyPath(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newAzureServer(t, "text-embedding-3-small", "embeddings", func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		input, ok := body["input"].([]interface{})
		if !ok || len(input) != 1 || input[0] != "hello" {
			t.Errorf("input=%v", body["input"])
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]interface{}{
				{"index": 0, "embedding": []float64{0.1, 0.2, 0.3}},
			},
		})
	})
	defer srv.Close()

	modelName := "text-embedding-3-small"
	apiKey := "test-key"
	embeddings, err := newAzureForTest(srv.URL).Embed(
		ctx,
		&modelName,
		EmbedRequest{Texts: []string{"hello"}},
		&APIConfig{ApiKey: &apiKey}, nil, nil,
	)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 1 {
		t.Fatalf("got %d embeddings, want 1", len(embeddings))
	}
	if len(embeddings[0].Embedding) != 3 || embeddings[0].Index != 0 {
		t.Errorf("embedding=%#v", embeddings[0])
	}
}
