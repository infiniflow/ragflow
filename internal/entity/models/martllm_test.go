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
	"slices"
	"testing"
)

// newMartLLMForTest creates a MartLLMModel instance for testing.
func newMartLLMForTest(baseURL string) *MartLLMModel {
	return NewMartLLMModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestMartLLMName tests the driver name.
func TestMartLLMName(t *testing.T) {
	if got := newMartLLMForTest("http://unused").Name(); got != "martllm" {
		t.Errorf("Name()=%q", got)
	}
}

// TestMartLLMFactory tests creating the driver from factory.
func TestMartLLMFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("martllm", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*MartLLMModel); !ok {
		t.Fatalf("driver type=%T, want *MartLLMModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*MartLLMModel); !ok {
		t.Fatal("NewInstance did not return *MartLLMModel")
	}
}

// The shipped config must resolve to the MartLLM driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "MartLLM" provider name has
// to match the "martllm" case; anything else silently falls through to
// DummyModel.
func TestMartLLMProviderConfig(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "martllm.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("MartLLM")
	if provider == nil {
		t.Fatal("MartLLM provider not found")
	}
	if _, ok := provider.ModelDriver.(*MartLLMModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.MartLLMModel", provider.ModelDriver)
	}
}

// TestMartLLMChatSendsAPIKey tests chat completion authentication and that
// the model id reaches the API unchanged.
func TestMartLLMChatSendsAPIKey(t *testing.T) {
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
			"id":      "chat-martllm",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newMartLLMForTest(srv.URL).ChatWithMessages(
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

// TestMartLLMListModelsAndCheckConnection tests model listing and connectivity check.
func TestMartLLMListModelsAndCheckConnection(t *testing.T) {
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

	model := newMartLLMForTest(srv.URL)
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

// TestMartLLMListModelsKeepsCatalogueTypes tests that listed models in the
// MartLLM catalogue keep its types, so vision models are not reported as
// chat-only, while other models keep name-inferred types.
func TestMartLLMListModelsKeepsCatalogueTypes(t *testing.T) {
	withSSRFBypass(t)
	saved := GetProviderManager()
	defer setProviderManager(saved)
	dir, restore := setupProviderTestDir(t, "martllm.json")
	defer restore()
	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "google/gemini-3.5-flash"},
				{"id": "openai/gpt-5.4-mini"},
				{"id": "z-ai/glm-5.2"},
				{"id": "qwen/qwen3-max"},
			},
		})
	}))
	defer srv.Close()

	list, err := newMartLLMForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	got := map[string][]string{}
	for _, m := range list {
		got[m.Name] = m.ModelTypes
	}
	for _, name := range []string{"google/gemini-3.5-flash", "openai/gpt-5.4-mini"} {
		if !slices.Contains(got[name], "vision") || !slices.Contains(got[name], "chat") {
			t.Errorf("%s types=%v, want chat and vision", name, got[name])
		}
	}
	if slices.Contains(got["z-ai/glm-5.2"], "vision") || !slices.Contains(got["z-ai/glm-5.2"], "chat") {
		t.Errorf("z-ai/glm-5.2 types=%v, want chat only", got["z-ai/glm-5.2"])
	}
	if !slices.Contains(got["qwen/qwen3-max"], "chat") {
		t.Errorf("qwen/qwen3-max types=%v, want name-inferred chat", got["qwen/qwen3-max"])
	}
}

// TestMartLLMListModelsReadsListedCapabilities tests that models outside the
// catalogue take their types from the listed endpoints and input modalities,
// and their limits from context_length and max_output_tokens.
func TestMartLLMListModelsReadsListedCapabilities(t *testing.T) {
	withSSRFBypass(t)
	chatEndpoints := []string{"/v1/chat/completions", "/v1/messages", "/v1/responses"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": []map[string]any{
				{"id": "anthropic/claude-opus-4.6", "endpoints": chatEndpoints, "input_modalities": []string{"text", "image"}, "context_length": 1000000, "max_output_tokens": 128000},
				{"id": "deepseek/deepseek-v4-pro", "endpoints": chatEndpoints, "input_modalities": []string{"text"}},
				{"id": "acme/image-model", "endpoints": []string{"/v1/images/generations"}, "input_modalities": []string{"text"}},
			},
		})
	}))
	defer srv.Close()

	list, err := newMartLLMForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	got := map[string]ListModelResponse{}
	for _, m := range list {
		got[m.Name] = m
	}
	opus, ok := got["anthropic/claude-opus-4.6"]
	if !ok || !slices.Contains(opus.ModelTypes, "vision") || !slices.Contains(opus.ModelTypes, "chat") {
		t.Errorf("claude-opus-4.6 types=%v, want chat and vision", opus.ModelTypes)
	}
	if opus.ContextLength == nil || *opus.ContextLength != 1000000 || opus.MaxOutput == nil || *opus.MaxOutput != 128000 {
		t.Errorf("claude-opus-4.6 limits=%v/%v, want 1000000/128000", opus.ContextLength, opus.MaxOutput)
	}
	ds := got["deepseek/deepseek-v4-pro"]
	if slices.Contains(ds.ModelTypes, "vision") || !slices.Contains(ds.ModelTypes, "chat") {
		t.Errorf("deepseek-v4-pro types=%v, want chat only", ds.ModelTypes)
	}
	if _, ok := got["acme/image-model"]; ok {
		t.Errorf("acme/image-model has no chat endpoint and must not be listed")
	}
}
