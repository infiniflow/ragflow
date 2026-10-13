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
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
)

func newLiteLLMForTest(baseURL string) *LiteLLMModel {
	return NewLiteLLMModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings", Rerank: "rerank"},
	)
}

func TestLiteLLMName(t *testing.T) {
	if got := newLiteLLMForTest("http://unused").Name(); got != "litellm" {
		t.Errorf("Name()=%q", got)
	}
}

func TestLiteLLMFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("litellm", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*LiteLLMModel); !ok {
		t.Fatalf("driver type=%T, want *LiteLLMModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*LiteLLMModel); !ok {
		t.Fatal("NewInstance did not return *LiteLLMModel")
	}
}

// The shipped config must resolve to the LiteLLM driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "LiteLLM" provider name has to
// match the "litellm" case; anything else silently falls through to DummyModel.
func TestLiteLLMProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "litellm.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("LiteLLM")
	if provider == nil {
		t.Fatal("LiteLLM provider not found")
	}
	if _, ok := provider.ModelDriver.(*LiteLLMModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.LiteLLMModel", provider.ModelDriver)
	}
	if provider.Class != "local" {
		t.Errorf("Class=%q, want local", provider.Class)
	}
	if len(provider.Models) != 0 {
		t.Errorf("Models=%v, want no built-in catalog", provider.Models)
	}
}

// A proxy can run without auth, so calls must work with an empty APIConfig.
func TestLiteLLMChatWithoutAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Errorf("Authorization=%q, want empty", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-litellm",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newLiteLLMForTest(srv.URL).ChatWithMessages(
		t.Context(), "gpt-6-luna", []Message{{Role: "user", Content: "ping"}}, &APIConfig{}, nil, usage,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	assertModelUsage(t, usage, 3, 5, 8)
}

// A virtual key must reach the proxy as a Bearer token, and the model alias
// must pass through unchanged.
func TestLiteLLMChatSendsVirtualKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-litellm-test" {
			t.Errorf("Authorization=%q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body["model"] != "gpt-6-luna" {
			t.Errorf("model=%v", body["model"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-litellm",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	apiKey := "sk-litellm-test"
	resp, err := newLiteLLMForTest(srv.URL).ChatWithMessages(
		t.Context(), "gpt-6-luna", []Message{{Role: "user", Content: "ping"}}, &APIConfig{ApiKey: &apiKey}, nil, &common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
}

func TestLiteLLMEmbed(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  []map[string]any{{"embedding": []float64{0.1, 0.2}, "index": 0}},
			"usage": map[string]any{"prompt_tokens": 7, "total_tokens": 7},
		})
	}))
	defer srv.Close()

	modelName := "text-embedding-3-large"
	embeddings, err := newLiteLLMForTest(srv.URL).Embed(
		t.Context(), &modelName, EmbedRequest{Texts: []string{"document"}}, &APIConfig{}, nil, &common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 1 || len(embeddings[0].Embedding) != 2 {
		t.Fatalf("embeddings=%#v", embeddings)
	}
}

func TestLiteLLMRerank(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/rerank" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		var body struct {
			Model     string   `json:"model"`
			Query     string   `json:"query"`
			Documents []string `json:"documents"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode body: %v", err)
		}
		if body.Model != "rerank-default" || body.Query != "q" || len(body.Documents) != 2 {
			t.Errorf("body=%+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"index": 1, "relevance_score": 0.9},
				{"index": 0, "relevance_score": 0.1},
			},
		})
	}))
	defer srv.Close()

	modelName := "rerank-default"
	resp, err := newLiteLLMForTest(srv.URL).Rerank(
		t.Context(), &modelName, RerankRequest{Query: "q", Documents: []string{"a", "b"}}, &APIConfig{}, &RerankConfig{}, nil,
	)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[0].Index != 1 || resp.Data[0].RelevanceScore != 0.9 {
		t.Fatalf("rerank data=%#v", resp.Data)
	}
}

func TestLiteLLMListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{{"id": "gpt-6-luna"}, {"id": "text-embedding-3-large"}},
		})
	}))
	defer srv.Close()

	model := newLiteLLMForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "gpt-6-luna,text-embedding-3-large" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
