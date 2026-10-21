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

// newAIStupidLevelForTest creates an AIStupidLevelModel instance for testing.
func newAIStupidLevelForTest(baseURL string) *AIStupidLevelModel {
	return NewAIStupidLevelModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestAIStupidLevelName tests the driver name.
func TestAIStupidLevelName(t *testing.T) {
	if got := newAIStupidLevelForTest("http://unused").Name(); got != "aistupidlevel" {
		t.Errorf("Name()=%q", got)
	}
}

// TestAIStupidLevelFactory tests creating the driver from the factory by the provider name.
func TestAIStupidLevelFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("AI Stupid Level", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*AIStupidLevelModel); !ok {
		t.Fatalf("driver type=%T, want *AIStupidLevelModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*AIStupidLevelModel); !ok {
		t.Fatal("NewInstance did not return *AIStupidLevelModel")
	}
}

// The shipped config must resolve to the AI Stupid Level driver: CreateModelDriver
// dispatches on strings.ToLower(name), so "AI Stupid Level" has to match the
// "ai stupid level" case; anything else silently falls through to DummyModel.
func TestAIStupidLevelProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "aistupidlevel.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("AI Stupid Level")
	if provider == nil {
		t.Fatal("AI Stupid Level provider not found")
	}
	if _, ok := provider.ModelDriver.(*AIStupidLevelModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.AIStupidLevelModel", provider.ModelDriver)
	}
}

// TestAIStupidLevelChatSendsAPIKey tests chat authentication and that the routing
// mode reaches the API unchanged as the model id.
func TestAIStupidLevelChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "auto-coding" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-aistupidlevel",
			"model":   "claude-opus-5-5",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newAIStupidLevelForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"auto-coding",
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

// TestAIStupidLevelListModelsAndCheckConnection tests model listing and the connectivity check.
func TestAIStupidLevelListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "auto-coding"},
				{"id": "claude-opus-5-5"},
			},
		})
	}))
	defer srv.Close()

	model := newAIStupidLevelForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "auto-coding,claude-opus-5-5" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
