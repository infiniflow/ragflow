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

// newHostingerRouterForTest creates a HostingerRouterModel instance for testing.
func newHostingerRouterForTest(baseURL string) *HostingerRouterModel {
	return NewHostingerRouterModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings"},
	)
}

// TestHostingerRouterName tests the driver name.
func TestHostingerRouterName(t *testing.T) {
	if got := newHostingerRouterForTest("http://unused").Name(); got != "hostinger_router" {
		t.Errorf("Name()=%q", got)
	}
}

// TestHostingerRouterFactory tests creating the driver from factory.
func TestHostingerRouterFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Hostinger Router", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*HostingerRouterModel); !ok {
		t.Fatalf("driver type=%T, want *HostingerRouterModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*HostingerRouterModel); !ok {
		t.Fatal("NewInstance did not return *HostingerRouterModel")
	}
}

// The shipped config must resolve to the Hostinger Router driver.
// CreateModelDriver dispatches on strings.ToLower(name), so the
// "Hostinger Router" provider name has to match the "hostinger router" case;
// anything else silently falls through to DummyModel.
func TestHostingerRouterProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "hostinger_router.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Hostinger Router")
	if provider == nil {
		t.Fatal("Hostinger Router provider not found")
	}
	if _, ok := provider.ModelDriver.(*HostingerRouterModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.HostingerRouterModel", provider.ModelDriver)
	}
	for _, model := range provider.Models {
		if model.ContextLength != nil && model.MaxOutput != nil && *model.MaxOutput > *model.ContextLength {
			t.Errorf("%s max_output %d exceeds context_length %d", model.Name, *model.MaxOutput, *model.ContextLength)
		}
	}
}

// TestHostingerRouterChatSendsAPIKey tests chat completion authentication and
// that the unprefixed regional model id reaches the API unchanged.
func TestHostingerRouterChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "gpt-5-mini-eu" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-hostinger-router",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newHostingerRouterForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"gpt-5-mini-eu",
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

// TestHostingerRouterEmbed tests embedding requests.
func TestHostingerRouterEmbed(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embeddings" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "text-embedding-3-small" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":  []map[string]any{{"embedding": []float64{0.1, 0.2}, "index": 0}},
			"usage": map[string]any{"prompt_tokens": 7, "total_tokens": 7},
		})
	}))
	defer srv.Close()

	modelName := "text-embedding-3-small"
	embeddings, err := newHostingerRouterForTest(srv.URL).Embed(
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

// TestHostingerRouterListModelsAndCheckConnection tests model listing and connectivity check.
func TestHostingerRouterListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-sonnet-5"},
				{"id": "text-embedding-3-small"},
			},
		})
	}))
	defer srv.Close()

	model := newHostingerRouterForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "claude-sonnet-5,text-embedding-3-small" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

// TestHostingerRouterListModelsKeepsCatalogueTypes tests that listed models in
// the Hostinger Router catalogue keep its types, so vision models are not
// reported as chat-only, while other models keep name-inferred types.
func TestHostingerRouterListModelsKeepsCatalogueTypes(t *testing.T) {
	withSSRFBypass(t)
	prev := providerManager
	defer func() { providerManager = prev }()
	dir, restore := setupProviderTestDir(t, "hostinger_router.json")
	defer restore()
	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-sonnet-5"},
				{"id": "gpt-5-mini-eu"},
				{"id": "text-embedding-3-small"},
				{"id": "glm-5.2"},
			},
		})
	}))
	defer srv.Close()

	list, err := newHostingerRouterForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	got := map[string][]string{}
	for _, m := range list {
		got[m.Name] = m.ModelTypes
	}
	for _, name := range []string{"claude-sonnet-5", "gpt-5-mini-eu"} {
		if !containsModelType(got[name], "vision") || !containsModelType(got[name], "chat") {
			t.Errorf("%s types=%v, want chat and vision", name, got[name])
		}
	}
	if !containsModelType(got["text-embedding-3-small"], "embedding") {
		t.Errorf("text-embedding-3-small types=%v, want embedding", got["text-embedding-3-small"])
	}
	if !containsModelType(got["glm-5.2"], "chat") {
		t.Errorf("glm-5.2 types=%v, want name-inferred chat", got["glm-5.2"])
	}
}
