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

// newWallyForTest creates a WallyModel instance for testing.
func newWallyForTest(baseURL string) *WallyModel {
	return NewWallyModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestWallyName tests the driver name.
func TestWallyName(t *testing.T) {
	if got := newWallyForTest("http://unused").Name(); got != "wally" {
		t.Errorf("Name()=%q", got)
	}
}

// TestWallyFactory tests creating the driver from factory.
func TestWallyFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("wally", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*WallyModel); !ok {
		t.Fatalf("driver type=%T, want *WallyModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*WallyModel); !ok {
		t.Fatal("NewInstance did not return *WallyModel")
	}
}

// The shipped config must resolve to the Wally driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "Wally" provider name has
// to match the "wally" case; anything else silently falls through to
// DummyModel.
func TestWallyProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "wally.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Wally")
	if provider == nil {
		t.Fatal("Wally provider not found")
	}
	if _, ok := provider.ModelDriver.(*WallyModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.WallyModel", provider.ModelDriver)
	}
}

// TestWallyChatSendsAPIKey tests chat completion authentication and that
// the model id reaches the API unchanged.
func TestWallyChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "deepseek-v4.1-flash" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-wally",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newWallyForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"deepseek-v4.1-flash",
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

// TestWallyListModelsAndCheckConnection tests model listing and connectivity check.
func TestWallyListModelsAndCheckConnection(t *testing.T) {
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
				{"id": "deepseek-v4.1-flash"},
				{"id": "glm-5.3-flash"},
			},
		})
	}))
	defer srv.Close()

	model := newWallyForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "deepseek-v4.1-flash,glm-5.3-flash" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
