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

func newObscurifyForTest(baseURL string) *ObscurifyModel {
	return NewObscurifyModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

func TestObscurifyName(t *testing.T) {
	if got := newObscurifyForTest("http://unused").Name(); got != "obscurify" {
		t.Errorf("Name()=%q", got)
	}
}

func TestObscurifyFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Obscurify.ai", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*ObscurifyModel); !ok {
		t.Fatalf("driver type=%T, want *ObscurifyModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*ObscurifyModel); !ok {
		t.Fatal("NewInstance did not return *ObscurifyModel")
	}
}

// The provider name in conf/models/obscurify.json contains a dot; it must
// still lower-case to the "obscurify.ai" factory case rather than falling
// through to DummyModel.
func TestObscurifyProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "obscurify.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Obscurify.ai")
	if provider == nil {
		t.Fatal("Obscurify.ai provider not found")
	}
	if _, ok := provider.ModelDriver.(*ObscurifyModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.ObscurifyModel", provider.ModelDriver)
	}
	if GetProviderManager().FindModel(provider, "deepseek/deepseek-v4-flash") == nil {
		t.Error("deepseek/deepseek-v4-flash missing from the shipped catalog")
	}
}

// Chat requests carry the bearer key and keep the vendor-prefixed model id.
func TestObscurifyChatSendsKeyAndModelID(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer "+testAPIKey {
			t.Errorf("Authorization=%q", got)
		}
		var body struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body.Model != "moonshotai/kimi-k2-0905" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-local",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 4, "completion_tokens": 2, "total_tokens": 6},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newObscurifyForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"moonshotai/kimi-k2-0905",
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
	assertModelUsage(t, usage, 4, 2, 6)
}

func TestObscurifyListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "deepseek/deepseek-v4-flash"},
				{"id": "z-ai/glm-5.3"},
			},
		})
	}))
	defer srv.Close()

	model := newObscurifyForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "deepseek/deepseek-v4-flash,z-ai/glm-5.3" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
