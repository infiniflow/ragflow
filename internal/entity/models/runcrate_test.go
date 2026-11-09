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

// newRuncrateForTest creates a RuncrateModel instance for testing.
func newRuncrateForTest(baseURL string) *RuncrateModel {
	return NewRuncrateModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models", Embedding: "embeddings"},
	)
}

// TestRuncrateName tests the driver name.
func TestRuncrateName(t *testing.T) {
	if got := newRuncrateForTest("http://unused").Name(); got != "runcrate" {
		t.Errorf("Name()=%q", got)
	}
}

// TestRuncrateFactory tests creating the driver from factory.
func TestRuncrateFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("runcrate", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*RuncrateModel); !ok {
		t.Fatalf("driver type=%T, want *RuncrateModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*RuncrateModel); !ok {
		t.Fatal("NewInstance did not return *RuncrateModel")
	}
}

// The shipped config must resolve to the Runcrate driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "Runcrate" provider name has
// to match the "runcrate" case; anything else silently falls through to
// DummyModel.
func TestRuncrateProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "runcrate.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Runcrate")
	if provider == nil {
		t.Fatal("Runcrate provider not found")
	}
	if _, ok := provider.ModelDriver.(*RuncrateModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.RuncrateModel", provider.ModelDriver)
	}
}

// TestRuncrateChatSendsAPIKey tests chat completion authentication and that
// the organization-prefixed model id reaches the API unchanged.
func TestRuncrateChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "deepseek-ai/DeepSeek-V4.1-Flash" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-runcrate",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newRuncrateForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"deepseek-ai/DeepSeek-V4.1-Flash",
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

// TestRuncrateEmbed tests embedding requests.
func TestRuncrateEmbed(t *testing.T) {
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

	modelName := "Qwen/Qwen3-Embedding-8B"
	embeddings, err := newRuncrateForTest(srv.URL).Embed(
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

// TestRuncrateListModelsAndCheckConnection tests model listing and connectivity check.
func TestRuncrateListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "deepseek-ai/DeepSeek-V4.1-Flash"},
				{"id": "Qwen/Qwen3-Embedding-8B"},
			},
		})
	}))
	defer srv.Close()

	model := newRuncrateForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "deepseek-ai/DeepSeek-V4.1-Flash,Qwen/Qwen3-Embedding-8B" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
