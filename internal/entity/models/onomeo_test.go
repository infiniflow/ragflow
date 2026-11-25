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
	"io"
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"slices"
	"strings"
	"testing"
)

// newOnomeoForTest creates a OnomeoModel instance for testing.
func newOnomeoForTest(baseURL string) *OnomeoModel {
	return NewOnomeoModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestOnomeoName tests the driver name.
func TestOnomeoName(t *testing.T) {
	if got := newOnomeoForTest("http://unused").Name(); got != "onomeo" {
		t.Errorf("Name()=%q", got)
	}
}

// TestOnomeoFactory tests creating the driver from factory.
func TestOnomeoFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("onomeo", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*OnomeoModel); !ok {
		t.Fatalf("driver type=%T, want *OnomeoModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*OnomeoModel); !ok {
		t.Fatal("NewInstance did not return *OnomeoModel")
	}
}

// The shipped config must resolve to the onomeo driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "onomeo" provider name has
// to match the "onomeo" case; anything else silently falls through to
// DummyModel. Every catalogued model is a chat model with both limits set.
func TestOnomeoProviderConfig(t *testing.T) {
	saved := GetProviderManager()
	defer setProviderManager(saved)
	dir, restore := setupProviderTestDir(t, "onomeo.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("onomeo")
	if provider == nil {
		t.Fatal("onomeo provider not found")
	}
	if _, ok := provider.ModelDriver.(*OnomeoModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.OnomeoModel", provider.ModelDriver)
	}
	if got := provider.URL["default"]; got != "https://onomeo.com/v1" {
		t.Errorf("default URL=%q", got)
	}
	if provider.URLSuffix.Chat != "chat/completions" || provider.URLSuffix.Models != "models" {
		t.Errorf("URLSuffix=%+v", provider.URLSuffix)
	}
	if len(provider.Models) == 0 {
		t.Fatal("onomeo catalog is empty")
	}
	for _, model := range provider.Models {
		if !slices.ContainsFunc(model.ModelTypes, func(t string) bool { return strings.EqualFold(t, "chat") }) {
			t.Errorf("%s types=%v, want chat", model.Name, model.ModelTypes)
		}
		if model.ContextLength == nil || model.MaxOutput == nil {
			t.Errorf("%s is missing context_length or max_output", model.Name)
			continue
		}
		if *model.MaxOutput > *model.ContextLength {
			t.Errorf("%s max_output %d exceeds context_length %d", model.Name, *model.MaxOutput, *model.ContextLength)
		}
	}
}

// TestOnomeoChatSendsAPIKey tests chat completion authentication and that
// the model id reaches the API unchanged.
func TestOnomeoChatSendsAPIKey(t *testing.T) {
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
			"id":      "chat-onomeo",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newOnomeoForTest(srv.URL).ChatWithMessages(
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

// Chat requests carry Chat Completions fields. The sampling fields pass
// through, while do_sample and the thinking and vLLM chat-template payloads
// that the embedded driver adds are not sent: reasoning_effort is the only
// reasoning field.
func TestOnomeoChatSendsChatCompletionsFields(t *testing.T) {
	withSSRFBypass(t)
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-onomeo",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	thinking := true
	doSample := true
	temperature := 0.2
	maxTokens := 64
	effort := "High"
	_, err := newOnomeoForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"qwen3.8-27b",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &testAPIKey},
		&ChatConfig{Thinking: &thinking, Effort: &effort, DoSample: &doSample, Temperature: &temperature, MaxTokens: &maxTokens},
		&common.ModelUsage{},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}

	for _, key := range []string{"thinking", "chat_template_kwargs", "enable_thinking", "do_sample", "stream_options"} {
		if value, ok := gotBody[key]; ok {
			t.Errorf("request sends %q=%v", key, value)
		}
	}
	if gotBody["stream"] != false {
		t.Errorf("stream=%v, want false", gotBody["stream"])
	}
	if gotBody["temperature"] != 0.2 || gotBody["max_tokens"] != float64(64) {
		t.Errorf("temperature=%v max_tokens=%v", gotBody["temperature"], gotBody["max_tokens"])
	}
	if gotBody["reasoning_effort"] != "high" {
		t.Errorf("reasoning_effort=%v, want high", gotBody["reasoning_effort"])
	}
}

// The effort reaches the gateway as reasoning_effort when it is low, medium
// or high; any other effort is dropped. The thinking switch alone sends
// nothing.
func TestOnomeoReasoningEffort(t *testing.T) {
	on, off := true, false
	effort := func(value string) *string { return &value }
	cases := []struct {
		name   string
		config *ChatConfig
		want   any
	}{
		{name: "no config", config: nil, want: nil},
		{name: "empty config", config: &ChatConfig{}, want: nil},
		{name: "switch on alone", config: &ChatConfig{Thinking: &on}, want: nil},
		{name: "switch off alone", config: &ChatConfig{Thinking: &off}, want: nil},
		{name: "effort alone", config: &ChatConfig{Effort: effort(" low ")}, want: "low"},
		{name: "effort with switch on", config: &ChatConfig{Thinking: &on, Effort: effort("Medium")}, want: "medium"},
		{name: "unsupported effort", config: &ChatConfig{Effort: effort("max")}, want: nil},
		{name: "none effort", config: &ChatConfig{Effort: effort("none")}, want: nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := buildOnomeoRequestBody(tc.config, "deepseek-v4.1-flash", []Message{{Role: "user", Content: "ping"}}, false)
			got, sent := reqBody["reasoning_effort"]
			if tc.want == nil {
				if sent {
					t.Errorf("reasoning_effort=%v, want it unset", got)
				}
				return
			}
			if got != tc.want {
				t.Errorf("reasoning_effort=%v, want %v", got, tc.want)
			}
		})
	}
}

// TestOnomeoChatStreamReadsReasoning tests the streaming request and that the
// reasoning text reaches the sender whether the upstream streams it as
// delta.reasoning_content or as delta.reasoning, along with the answer and
// the usage of the final chunk.
func TestOnomeoChatStreamReadsReasoning(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
		}
		if body["stream"] != true {
			t.Errorf("stream=%v, want true", body["stream"])
		}
		streamOptions, ok := body["stream_options"].(map[string]any)
		if !ok || streamOptions["include_usage"] != true {
			t.Errorf("stream_options=%#v, want include_usage=true", body["stream_options"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}],\"usage\":null}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"17*3\"},\"finish_reason\":null}],\"usage\":null}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning\":\"=51\"},\"finish_reason\":null}],\"usage\":null}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"51\"},\"finish_reason\":null}],\"usage\":null}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":null}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":5,\"total_tokens\":8}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	config := &ChatConfig{}
	var answer, reasoning strings.Builder
	err := newOnomeoForTest(srv.URL).ChatStreamlyWithSender(
		t.Context(),
		"deepseek-v4.1-flash",
		[]Message{{Role: "user", Content: "What is 17 times 3?"}},
		&APIConfig{ApiKey: &testAPIKey},
		config,
		nil,
		func(content, reason *string) error {
			if content != nil && *content != "[DONE]" {
				answer.WriteString(*content)
			}
			if reason != nil {
				reasoning.WriteString(*reason)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if answer.String() != "51" {
		t.Errorf("answer=%q, want %q", answer.String(), "51")
	}
	if reasoning.String() != "17*3=51" {
		t.Errorf("reasoning=%q, want %q", reasoning.String(), "17*3=51")
	}
	if config.UsageResult == nil || config.UsageResult.PromptTokens != 3 || config.UsageResult.CompletionTokens != 5 || config.UsageResult.TotalTokens != 8 {
		t.Errorf("UsageResult=%#v, want prompt=3 completion=5 total=8", config.UsageResult)
	}
}

// Every call needs the API key. A missing or blank key fails before a
// request is sent, for chat, streaming, the model list and the connection
// check.
func TestOnomeoRequiresAPIKey(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("request sent without an API key: %s %s", r.Method, r.URL.Path)
	}))
	defer srv.Close()

	blank := "  "
	configs := map[string]*APIConfig{
		"nil config": nil,
		"nil key":    {},
		"blank key":  {ApiKey: &blank},
	}
	model := newOnomeoForTest(srv.URL)
	messages := []Message{{Role: "user", Content: "ping"}}
	for name, apiConfig := range configs {
		t.Run(name, func(t *testing.T) {
			_, chatErr := model.ChatWithMessages(t.Context(), "deepseek-v4.1-flash", messages, apiConfig, nil, nil)
			streamErr := model.ChatStreamlyWithSender(t.Context(), "deepseek-v4.1-flash", messages, apiConfig, nil, nil, func(*string, *string) error { return nil })
			_, listErr := model.ListModels(t.Context(), apiConfig)
			checkErr := model.CheckConnection(t.Context(), apiConfig)
			for call, err := range map[string]error{
				"ChatWithMessages":       chatErr,
				"ChatStreamlyWithSender": streamErr,
				"ListModels":             listErr,
				"CheckConnection":        checkErr,
			} {
				if err == nil || !strings.Contains(err.Error(), "api key is required") {
					t.Errorf("%s: err=%v, want api key is required", call, err)
				}
			}
		})
	}
}

// TestOnomeoListModelsReportsAPIError tests that a rejected key surfaces as
// an error from both the model list and the connectivity check. The body is
// the gateway's answer to an invalid key.
func TestOnomeoListModelsReportsAPIError(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"code":"bad_key","message":"Invalid API key, or the key has been revoked."}}`)
	}))
	defer srv.Close()

	model := newOnomeoForTest(srv.URL)
	if _, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("ListModels err=%v, want status 401", err)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err == nil {
		t.Error("CheckConnection succeeded with a rejected key")
	}
}

// TestOnomeoListModelsAndCheckConnection tests model listing and connectivity
// check. The model list requires the API key.
func TestOnomeoListModelsAndCheckConnection(t *testing.T) {
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
				{"id": "glm-5.3"},
			},
		})
	}))
	defer srv.Close()

	model := newOnomeoForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "deepseek-v4.1-flash,glm-5.3" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}
