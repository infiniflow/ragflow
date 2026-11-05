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
	"strings"
	"testing"
)

// newCloudGPUForTest creates a CloudGPUModel instance for testing.
func newCloudGPUForTest(baseURL string) *CloudGPUModel {
	return NewCloudGPUModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestCloudGPUName tests the driver name.
func TestCloudGPUName(t *testing.T) {
	if got := newCloudGPUForTest("http://unused").Name(); got != "cloudgpu" {
		t.Errorf("Name()=%q", got)
	}
}

// TestCloudGPUFactory tests creating the driver from factory.
func TestCloudGPUFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("cloudgpu", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*CloudGPUModel); !ok {
		t.Fatalf("driver type=%T, want *CloudGPUModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*CloudGPUModel); !ok {
		t.Fatal("NewInstance did not return *CloudGPUModel")
	}
}

// The shipped config must resolve to the CloudGPU driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "CloudGPU" provider name has
// to match the "cloudgpu" case; anything else silently falls through to
// DummyModel.
func TestCloudGPUProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "cloudgpu.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("CloudGPU")
	if provider == nil {
		t.Fatal("CloudGPU provider not found")
	}
	if _, ok := provider.ModelDriver.(*CloudGPUModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.CloudGPUModel", provider.ModelDriver)
	}
}

// TestCloudGPUChatSendsAPIKey tests chat completion authentication and that
// the model id reaches the API unchanged.
func TestCloudGPUChatSendsAPIKey(t *testing.T) {
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
			"id":      "chat-cloudgpu",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newCloudGPUForTest(srv.URL).ChatWithMessages(
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

// TestCloudGPUListModelsAndCheckConnection tests model listing and connectivity check.
func TestCloudGPUListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "deepseek-v4.1-flash"},
				{"id": "kimi-k2.6"},
			},
		})
	}))
	defer srv.Close()

	model := newCloudGPUForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "deepseek-v4.1-flash,kimi-k2.6" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

// Known CloudGPU models must report their catalogued types, so the vision
// models are not downgraded to chat-only by name inference. Unknown ids keep
// the name-based fallback.
func TestCloudGPUListModelsUsesCatalogTypes(t *testing.T) {
	withSSRFBypass(t)
	dir, restore := setupProviderTestDir(t, "cloudgpu.json")
	defer restore()
	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "deepseek-v4.1-flash"},
				{"id": "kimi-k2.6"},
				{"id": "glm-5.2"},
				{"id": "some-new-vl-model"},
			},
		})
	}))
	defer srv.Close()

	list, err := newCloudGPUForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := map[string]string{
		"deepseek-v4.1-flash": "chat,vision",
		"kimi-k2.6":           "chat,vision",
		"glm-5.2":             "chat",
		"some-new-vl-model":   "chat,vision",
	}
	if len(list) != len(want) {
		t.Fatalf("models=%v", list)
	}
	for _, model := range list {
		if got := strings.Join(model.ModelTypes, ","); got != want[model.Name] {
			t.Errorf("%s types=%q, want %q", model.Name, got, want[model.Name])
		}
	}
}
