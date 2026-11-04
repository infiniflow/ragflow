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

// newUberLLMForTest creates a UberLLMModel instance for testing.
func newUberLLMForTest(baseURL string) *UberLLMModel {
	return NewUberLLMModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestUberLLMName tests the driver name.
func TestUberLLMName(t *testing.T) {
	if got := newUberLLMForTest("http://unused").Name(); got != "uberllm" {
		t.Errorf("Name()=%q", got)
	}
}

// TestUberLLMFactory tests creating the driver from factory.
func TestUberLLMFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("uberllm", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*UberLLMModel); !ok {
		t.Fatalf("driver type=%T, want *UberLLMModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*UberLLMModel); !ok {
		t.Fatal("NewInstance did not return *UberLLMModel")
	}
}

// The shipped config must resolve to the UberLLM driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "UberLLM" provider name has
// to match the "uberllm" case; anything else silently falls through to
// DummyModel.
func TestUberLLMProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "uberllm.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("UberLLM")
	if provider == nil {
		t.Fatal("UberLLM provider not found")
	}
	if _, ok := provider.ModelDriver.(*UberLLMModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.UberLLMModel", provider.ModelDriver)
	}
}

// TestUberLLMChatSendsAPIKey tests chat completion authentication and that
// the model id reaches the API unchanged.
func TestUberLLMChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "deepseek/deepseek-v4.1-flash" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-uberllm",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newUberLLMForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"deepseek/deepseek-v4.1-flash",
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

// TestUberLLMListModelsAndCheckConnection tests model listing and connectivity check.
func TestUberLLMListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "deepseek/deepseek-v4.1-flash"},
				{"id": "z-ai/glm-5.2"},
			},
		})
	}))
	defer srv.Close()

	model := newUberLLMForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "deepseek/deepseek-v4.1-flash,z-ai/glm-5.2" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
