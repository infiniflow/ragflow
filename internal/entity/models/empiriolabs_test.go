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

// newEmpirioLabsForTest creates an EmpirioLabsModel instance for testing.
func newEmpirioLabsForTest(baseURL string) *EmpirioLabsModel {
	return NewEmpirioLabsModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings", Rerank: "reranks"},
	)
}

// TestEmpirioLabsName tests the driver name.
func TestEmpirioLabsName(t *testing.T) {
	if got := newEmpirioLabsForTest("http://unused").Name(); got != "empiriolabs" {
		t.Errorf("Name()=%q", got)
	}
}

// TestEmpirioLabsFactory tests creating the driver from factory.
func TestEmpirioLabsFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("EmpirioLabs AI", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*EmpirioLabsModel); !ok {
		t.Fatalf("driver type=%T, want *EmpirioLabsModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*EmpirioLabsModel); !ok {
		t.Fatal("NewInstance did not return *EmpirioLabsModel")
	}
}

// The shipped config must resolve to the EmpirioLabs AI driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "EmpirioLabs AI" provider name has
// to match the "empiriolabs ai" case; anything else silently falls through to
// DummyModel. The rerank endpoint is "reranks" (plural) and the embeddings
// endpoint takes at most 10 inputs per request, so both are pinned here.
func TestEmpirioLabsProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "empiriolabs.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("EmpirioLabs AI")
	if provider == nil {
		t.Fatal("EmpirioLabs AI provider not found")
	}
	if _, ok := provider.ModelDriver.(*EmpirioLabsModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.EmpirioLabsModel", provider.ModelDriver)
	}
	if provider.URLSuffix.Rerank != "reranks" {
		t.Errorf("rerank url_suffix=%q, want reranks", provider.URLSuffix.Rerank)
	}
	for _, model := range provider.Models {
		if model.ModelTypeMap["embedding"] && (model.MaxBatchSize == nil || *model.MaxBatchSize > 10) {
			t.Errorf("embedding model %s must declare max_batch_size <= 10", model.Name)
		}
	}
}

// TestEmpirioLabsChatSendsAPIKey tests chat completion authentication.
func TestEmpirioLabsChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-empiriolabs",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newEmpirioLabsForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"qwen3-8-flash",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &testAPIKey},
		nil,
		usage,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if *resp.Answer != "pong" {
		t.Errorf("Answer=%q", *resp.Answer)
	}
	assertModelUsage(t, usage, 3, 5, 8)
}

// TestEmpirioLabsEmbed tests embedding requests.
func TestEmpirioLabsEmbed(t *testing.T) {
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

	modelName := "text-embedding-v4"
	embeddings, err := newEmpirioLabsForTest(srv.URL).Embed(
		t.Context(),
		&modelName,
		EmbedRequest{Texts: []string{"document"}},
		&APIConfig{ApiKey: &testAPIKey},
		nil,
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(embeddings) != 1 || len(embeddings[0].Embedding) != 2 {
		t.Fatalf("embeddings=%#v", embeddings)
	}
}

// TestEmpirioLabsRerank tests rerank requests against the plural reranks endpoint.
func TestEmpirioLabsRerank(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/reranks" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		var body struct {
			Model     string   `json:"model"`
			Query     string   `json:"query"`
			Documents []string `json:"documents"`
			TopN      int      `json:"top_n"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "qwen3-rerank" || body.Query != "query" || len(body.Documents) != 2 || body.TopN != 2 {
			t.Errorf("body=%+v", body)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"results": []map[string]any{
				{"index": 1, "relevance_score": 0.9},
				{"index": 0, "relevance_score": 0.2},
			},
		})
	}))
	defer srv.Close()

	modelName := "qwen3-rerank"
	resp, err := newEmpirioLabsForTest(srv.URL).Rerank(
		t.Context(),
		&modelName,
		RerankRequest{Query: "query", Documents: []string{"first", "second"}},
		&APIConfig{ApiKey: &testAPIKey},
		&RerankConfig{},
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("Rerank: %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[0].Index != 1 || resp.Data[0].RelevanceScore != 0.9 {
		t.Fatalf("results=%#v", resp.Data)
	}
}

// TestEmpirioLabsListModelsAndCheckConnection tests model listing and connectivity check.
func TestEmpirioLabsListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "qwen3-8-flash"},
				{"id": "text-embedding-v4"},
			},
		})
	}))
	defer srv.Close()

	model := newEmpirioLabsForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "qwen3-8-flash,text-embedding-v4" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
