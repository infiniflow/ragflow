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

// newCheapaimodelForTest creates a CheapaimodelModel instance for testing.
func newCheapaimodelForTest(baseURL string) *CheapaimodelModel {
	return NewCheapaimodelModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestCheapaimodelName tests the driver name.
func TestCheapaimodelName(t *testing.T) {
	if got := newCheapaimodelForTest("http://unused").Name(); got != "cheapaimodel" {
		t.Errorf("Name()=%q", got)
	}
}

// TestCheapaimodelFactory tests creating the driver from factory.
func TestCheapaimodelFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("cheapaimodel", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*CheapaimodelModel); !ok {
		t.Fatalf("driver type=%T, want *CheapaimodelModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*CheapaimodelModel); !ok {
		t.Fatal("NewInstance did not return *CheapaimodelModel")
	}
}

// The shipped config must resolve to the cheapaimodel driver.
// CreateModelDriver dispatches on strings.ToLower(name), so the
// "cheapaimodel" provider name has to match the "cheapaimodel" case;
// anything else silently falls through to DummyModel.
func TestCheapaimodelProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "cheapaimodel.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("cheapaimodel")
	if provider == nil {
		t.Fatal("cheapaimodel provider not found")
	}
	if _, ok := provider.ModelDriver.(*CheapaimodelModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.CheapaimodelModel", provider.ModelDriver)
	}
	if got := provider.URL["default"]; got != "https://api.cheapaimodel.ai/v1" {
		t.Errorf("default URL=%q", got)
	}
	if provider.URLSuffix.Chat != "chat/completions" || provider.URLSuffix.Models != "models" {
		t.Errorf("URLSuffix=%+v", provider.URLSuffix)
	}
}

// TestCheapaimodelChatSendsAPIKey tests chat completion authentication and
// that the model id reaches the API unchanged.
func TestCheapaimodelChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "deepseek-v4-1-flash" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-cheapaimodel",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newCheapaimodelForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"deepseek-v4-1-flash",
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

// TestCheapaimodelListModelsAndCheckConnection tests model listing and
// connectivity check. The cheapaimodel model list requires the API key.
func TestCheapaimodelListModelsAndCheckConnection(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/models" {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer sk-gw-test" {
			t.Errorf("Authorization=%q", got)
		}
		// The load balancer in front of the API rejects a GET that carries a body.
		if r.ContentLength != 0 || len(r.TransferEncoding) > 0 {
			t.Errorf("GET /models carried a body (ContentLength=%d, TransferEncoding=%v)", r.ContentLength, r.TransferEncoding)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "deepseek-v4-1-flash"},
				{"id": "glm-5.3"},
			},
		})
	}))
	defer srv.Close()

	model := newCheapaimodelForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "deepseek-v4-1-flash,glm-5.3" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

// Known cheapaimodel models must report their catalogued types, so the vision
// models are not downgraded to chat-only by name inference. Unknown ids keep
// the name-based fallback, and ids the chat endpoint cannot serve (embedding,
// rerank) are not listed.
func TestCheapaimodelListModelsUsesCatalogTypes(t *testing.T) {
	withSSRFBypass(t)
	dir, restore := setupProviderTestDir(t, "cheapaimodel.json")
	defer restore()
	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "deepseek-v4-1-flash"},
				{"id": "glm-5.3"},
				{"id": "glm-5.3-flash"},
				{"id": "kimi-k3"},
				{"id": "grok-4.7"},
				{"id": "some-new-vl-model"},
				{"id": "text-embedding-v2"},
				{"id": "bge-reranker-v2"},
			},
		})
	}))
	defer srv.Close()

	list, err := newCheapaimodelForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	want := map[string]string{
		"deepseek-v4-1-flash": "chat,vision",
		"glm-5.3":             "chat",
		"glm-5.3-flash":       "chat,vision",
		"kimi-k3":             "chat,vision",
		"grok-4.7":            "chat,vision",
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

// The endpoint returns ids only, and most cheapaimodel ids have no global
// catalog entry, so the context length must come from the provider catalog.
// A context length the response does carry wins over the catalog.
func TestCheapaimodelListModelsUsesCatalogContextLength(t *testing.T) {
	withSSRFBypass(t)
	dir, restore := setupProviderTestDir(t, "cheapaimodel.json")
	defer restore()
	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "glm-5.3"},
				{"id": "grok-4.7", "context_length": 131072},
				{"id": "some-new-model"},
			},
		})
	}))
	defer srv.Close()

	list, err := newCheapaimodelForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	catalog := GetProviderManager().FindProvider("cheapaimodel")
	glm := GetProviderManager().FindModel(catalog, "glm-5.3")
	if glm == nil || glm.ContextLength == nil {
		t.Fatal("glm-5.3 has no catalogued context length")
	}
	want := map[string]*int{"glm-5.3": glm.ContextLength, "grok-4.7": intPtr(131072), "some-new-model": nil}
	if len(list) != len(want) {
		t.Fatalf("models=%v", list)
	}
	for _, model := range list {
		got, exp := model.ContextLength, want[model.Name]
		if (got == nil) != (exp == nil) || (got != nil && *got != *exp) {
			t.Errorf("%s context_length=%v, want %v", model.Name, deref(got), deref(exp))
		}
	}
}

func intPtr(v int) *int { return &v }

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}
