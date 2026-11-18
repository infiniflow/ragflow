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

var worldRouterTestAPIKey = "sk-worldrouter-test"

func newWorldRouterForTest(baseURL string) *WorldRouterModel {
	return NewWorldRouterModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestWorldRouterFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("worldrouter", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if driver.Name() != "worldrouter" {
		t.Fatalf("Name()=%q", driver.Name())
	}
	if _, ok := driver.(*WorldRouterModel); !ok {
		t.Fatalf("driver type=%T, want *WorldRouterModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*WorldRouterModel); !ok {
		t.Fatal("NewInstance did not return *WorldRouterModel")
	}
}

func TestWorldRouterProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "worldrouter.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}
	provider := GetProviderManager().FindProvider("WorldRouter")
	if provider == nil {
		t.Fatal("WorldRouter provider not found")
	}
	if _, ok := provider.ModelDriver.(*WorldRouterModel); !ok {
		t.Fatalf("ModelDriver=%T, want *WorldRouterModel", provider.ModelDriver)
	}
	if got := provider.URL["default"]; got != "https://inference-api.worldrouter.ai/v1" {
		t.Errorf("default URL=%q", got)
	}
	if provider.URLSuffix.Chat != "chat/completions" || provider.URLSuffix.Models != "models" {
		t.Errorf("URLSuffix=%+v", provider.URLSuffix)
	}
	if len(provider.Models) != 0 {
		t.Errorf("models=%v, want dynamic discovery", provider.Models)
	}
}

func TestWorldRouterChatSendsTenantAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-worldrouter-test" {
			t.Errorf("Authorization=%q", got)
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "gpt-6-luna" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-worldrouter",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newWorldRouterForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"gpt-6-luna",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &worldRouterTestAPIKey},
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

func TestWorldRouterListModelsFiltersUnsupportedModels(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-worldrouter-test" {
			t.Errorf("Authorization=%q", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "gpt-6-luna"},
				{"id": "new-chat-model"},
				{"id": "mock-test"},
				{"id": "gpt-image-2"},
				{"id": "pixverse-c1"},
				{"id": "seedance-2.5"},
				{"id": "bge-m3"},
			},
		})
	}))
	defer srv.Close()

	list, err := newWorldRouterForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &worldRouterTestAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if got := joinModelNames(list, ","); got != "gpt-6-luna,new-chat-model" {
		t.Errorf("models=%q", got)
	}
}
