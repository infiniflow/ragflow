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

// newSynapForTest creates a SynapModel instance for testing.
func newSynapForTest(baseURL string) *SynapModel {
	return NewSynapModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestSynapName tests the driver name.
func TestSynapName(t *testing.T) {
	if got := newSynapForTest("http://unused").Name(); got != "synap" {
		t.Errorf("Name()=%q", got)
	}
}

// TestSynapFactory tests creating the driver from factory.
func TestSynapFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("synap", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*SynapModel); !ok {
		t.Fatalf("driver type=%T, want *SynapModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*SynapModel); !ok {
		t.Fatal("NewInstance did not return *SynapModel")
	}
}

// The shipped config must resolve to the Synap driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "Synap" provider name has
// to match the "synap" case; anything else silently falls through to
// DummyModel.
func TestSynapProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "synap.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Synap")
	if provider == nil {
		t.Fatal("Synap provider not found")
	}
	if _, ok := provider.ModelDriver.(*SynapModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.SynapModel", provider.ModelDriver)
	}
}

// TestSynapChatSendsAPIKey tests chat completion authentication and that
// the model id reaches the API unchanged.
func TestSynapChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "qwen3-coder-30b" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-synap",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newSynapForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"qwen3-coder-30b",
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

// TestSynapListModelsAndCheckConnection tests model listing and connectivity check.
func TestSynapListModelsAndCheckConnection(t *testing.T) {
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
				{"id": "qwen3-coder-30b"},
				{"id": "gpt-oss-20b"},
			},
		})
	}))
	defer srv.Close()

	model := newSynapForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "qwen3-coder-30b,gpt-oss-20b" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
