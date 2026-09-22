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

// newApiRouteForTest creates an ApiRouteModel instance for testing.
func newApiRouteForTest(baseURL string) *ApiRouteModel {
	return NewApiRouteModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings"},
	)
}

// TestApiRouteName tests the driver name.
func TestApiRouteName(t *testing.T) {
	if got := newApiRouteForTest("http://unused").Name(); got != "apiroute" {
		t.Errorf("Name()=%q", got)
	}
}

// TestApiRouteFactory tests creating the driver from factory.
func TestApiRouteFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("api-route", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*ApiRouteModel); !ok {
		t.Fatalf("driver type=%T, want *ApiRouteModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*ApiRouteModel); !ok {
		t.Fatal("NewInstance did not return *ApiRouteModel")
	}
}

// The shipped config must resolve to the API-Route driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "API-Route" provider name only
// matches a "api-route" case; a "apiroute" case silently falls through to
// DummyModel.
func TestApiRouteProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "apiroute.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("API-Route")
	if provider == nil {
		t.Fatal("API-Route provider not found")
	}
	if _, ok := Underlying(provider.ModelDriver).(*ApiRouteModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.ApiRouteModel", provider.ModelDriver)
	}
}

// TestApiRouteChatSendsAPIKey tests chat completion authentication.
func TestApiRouteChatSendsAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-apiroute",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newApiRouteForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"claude-3-7-sonnet-20250219",
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

// TestApiRouteEmbed tests embedding requests.
func TestApiRouteEmbed(t *testing.T) {
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

	modelName := "text-embedding-3-small"
	embeddings, err := newApiRouteForTest(srv.URL).Embed(
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

// TestApiRouteListModelsAndCheckConnection tests model listing and connectivity check.
func TestApiRouteListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "claude-3-7-sonnet-20250219"},
				{"id": "text-embedding-3-small"},
			},
		})
	}))
	defer srv.Close()

	model := newApiRouteForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "claude-3-7-sonnet-20250219,text-embedding-3-small" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
