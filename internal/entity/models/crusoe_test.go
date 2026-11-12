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
	"strings"
	"testing"
)

// newCrusoeForTest creates a CrusoeModel instance for testing.
func newCrusoeForTest(baseURL string) *CrusoeModel {
	return NewCrusoeModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestCrusoeName tests the driver name.
func TestCrusoeName(t *testing.T) {
	if got := newCrusoeForTest("http://unused").Name(); got != "crusoe" {
		t.Errorf("Name()=%q", got)
	}
}

// TestCrusoeFactory tests creating the driver from factory.
func TestCrusoeFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Crusoe", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*CrusoeModel); !ok {
		t.Fatalf("driver type=%T, want *CrusoeModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*CrusoeModel); !ok {
		t.Fatal("NewInstance did not return *CrusoeModel")
	}
}

// The shipped config must resolve to the Crusoe driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "Crusoe" provider name has to
// match the "crusoe" case; anything else silently falls through to
// DummyModel. Every catalogued model is a chat model with both limits set and
// states whether its reasoning can be switched off.
func TestCrusoeProviderConfig(t *testing.T) {
	saved := GetProviderManager()
	defer setProviderManager(saved)
	dir, restore := setupProviderTestDir(t, "crusoe.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Crusoe")
	if provider == nil {
		t.Fatal("Crusoe provider not found")
	}
	if _, ok := provider.ModelDriver.(*CrusoeModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.CrusoeModel", provider.ModelDriver)
	}
	if got := provider.URL["default"]; got != "https://api.inference.crusoecloud.com/v1" {
		t.Errorf("default URL=%q", got)
	}
	if provider.URLSuffix.Chat != "chat/completions" || provider.URLSuffix.Models != "models" {
		t.Errorf("URLSuffix=%+v", provider.URLSuffix)
	}
	if len(provider.Models) == 0 {
		t.Fatal("Crusoe catalog is empty")
	}
	for _, model := range provider.Models {
		if !containsModelType(model.ModelTypes, "chat") {
			t.Errorf("%s types=%v, want chat", model.Name, model.ModelTypes)
		}
		if model.ContextLength == nil || model.MaxOutput == nil {
			t.Errorf("%s is missing context_length or max_output", model.Name)
			continue
		}
		if *model.MaxOutput > *model.ContextLength {
			t.Errorf("%s max_output %d exceeds context_length %d", model.Name, *model.MaxOutput, *model.ContextLength)
		}
		if model.Thinking == nil {
			t.Errorf("%s is missing its thinking config", model.Name)
			continue
		}
		_, canSwitchOff := crusoeReasoningOffEffort(model.Name)
		if strings.Contains(model.Name, "gpt-oss") {
			canSwitchOff = false
		}
		if model.Thinking.ClearThinking != canSwitchOff {
			t.Errorf("%s clear_thinking=%v, but the driver switches reasoning off: %v", model.Name, model.Thinking.ClearThinking, canSwitchOff)
		}
	}
}

// TestCrusoeChatSendsAPIKey tests chat completion authentication and that the
// "owner/model" id reaches the API unchanged.
func TestCrusoeChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "openai/gpt-oss-120b" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-crusoe",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newCrusoeForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"openai/gpt-oss-120b",
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
func TestCrusoeChatSendsChatCompletionsFields(t *testing.T) {
	withSSRFBypass(t)
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-crusoe",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	thinking := true
	doSample := true
	temperature := 0.2
	maxTokens := 64
	_, err := newCrusoeForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"qwen/Qwen3.8-27B",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &testAPIKey},
		&ChatConfig{Thinking: &thinking, DoSample: &doSample, Temperature: &temperature, MaxTokens: &maxTokens},
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
	if gotBody["reasoning_effort"] != "medium" {
		t.Errorf("reasoning_effort=%v, want medium", gotBody["reasoning_effort"])
	}
}

// The thinking switch and the effort reach the endpoint as reasoning_effort.
// A disabled switch sends "none" to the families that stop reasoning with it,
// "low" to gpt-oss, which rejects "none", and nothing to other models. An
// effort passes through as low, medium or high; any other effort is dropped.
func TestCrusoeReasoningEffort(t *testing.T) {
	on, off := true, false
	effort := func(value string) *string { return &value }
	cases := []struct {
		name   string
		model  string
		config *ChatConfig
		want   any
	}{
		{name: "no config", model: "openai/gpt-oss-120b", config: nil, want: nil},
		{name: "empty config", model: "openai/gpt-oss-120b", config: &ChatConfig{}, want: nil},
		{name: "off on nemotron", model: "nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B", config: &ChatConfig{Thinking: &off}, want: "none"},
		{name: "off on gemma", model: "google/gemma-4-31b-it", config: &ChatConfig{Thinking: &off}, want: "none"},
		{name: "off on glm", model: "zai-org/GLM-5.3", config: &ChatConfig{Thinking: &off}, want: "none"},
		{name: "off on gpt-oss", model: "openai/gpt-oss-120b", config: &ChatConfig{Thinking: &off}, want: "low"},
		{name: "off on a model without a switch", model: "deepseek-ai/DeepSeek-V4.1-Flash", config: &ChatConfig{Thinking: &off}, want: nil},
		{name: "off wins over effort", model: "google/gemma-4-31b-it", config: &ChatConfig{Thinking: &off, Effort: effort("high")}, want: "none"},
		{name: "on without effort", model: "google/gemma-4-31b-it", config: &ChatConfig{Thinking: &on}, want: "medium"},
		{name: "on with effort", model: "openai/gpt-oss-120b", config: &ChatConfig{Thinking: &on, Effort: effort("High")}, want: "high"},
		{name: "effort alone", model: "openai/gpt-oss-120b", config: &ChatConfig{Effort: effort(" low ")}, want: "low"},
		{name: "unsupported effort alone", model: "openai/gpt-oss-120b", config: &ChatConfig{Effort: effort("max")}, want: nil},
		{name: "unsupported effort with switch on", model: "openai/gpt-oss-120b", config: &ChatConfig{Thinking: &on, Effort: effort("none")}, want: "medium"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			reqBody := buildCrusoeRequestBody(tc.config, tc.model, []Message{{Role: "user", Content: "ping"}}, false)
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

// TestCrusoeChatStreamReadsReasoning tests the streaming request and that the
// reasoning text, which the endpoint streams as delta.reasoning, reaches the
// sender along with the answer and the usage of the final chunk.
func TestCrusoeChatStreamReadsReasoning(t *testing.T) {
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
		if body["reasoning_effort"] != "high" {
			t.Errorf("reasoning_effort=%v, want high", body["reasoning_effort"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"role\":\"assistant\"},\"finish_reason\":null}],\"usage\":null}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"reasoning\":\"17*3=51\"},\"finish_reason\":null}],\"usage\":null}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"51\"},\"finish_reason\":null}],\"usage\":null}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{},\"finish_reason\":\"stop\"}],\"usage\":null}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":5,\"total_tokens\":8}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()

	thinking := true
	effort := "high"
	config := &ChatConfig{Thinking: &thinking, Effort: &effort}
	var answer, reasoning strings.Builder
	err := newCrusoeForTest(srv.URL).ChatStreamlyWithSender(
		t.Context(),
		"openai/gpt-oss-120b",
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

// TestCrusoeListModelsAndCheckConnection tests model listing and connectivity
// check. The model list requires the API key.
func TestCrusoeListModelsAndCheckConnection(t *testing.T) {
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
				{"id": "openai/gpt-oss-120b"},
				{"id": "google/gemma-4-31b-it"},
			},
		})
	}))
	defer srv.Close()

	model := newCrusoeForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "openai/gpt-oss-120b,google/gemma-4-31b-it" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

// Every call needs the API key. A missing or blank key fails before a
// request is sent, for chat, streaming, the model list and the connection
// check.
func TestCrusoeRequiresAPIKey(t *testing.T) {
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
	model := newCrusoeForTest(srv.URL)
	messages := []Message{{Role: "user", Content: "ping"}}
	for name, apiConfig := range configs {
		t.Run(name, func(t *testing.T) {
			_, chatErr := model.ChatWithMessages(t.Context(), "openai/gpt-oss-120b", messages, apiConfig, nil, nil)
			streamErr := model.ChatStreamlyWithSender(t.Context(), "openai/gpt-oss-120b", messages, apiConfig, nil, nil, func(*string, *string) error { return nil })
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

// Listed models take their limits and image input from the endpoint's
// metadata: context_length is the context window,
// top_provider.max_completion_tokens the output limit, and a "multimodal"
// architecture.modality marks a vision model whatever the type field says. A
// zero limit stays unset. Private deployments and embedding deployments are
// not listed. The rows mirror GET /v1/models as captured on 2026-09-12.
func TestCrusoeListModelsReadsEndpointMetadata(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"object":"list","data":[
			{"id":"openai/gpt-oss-120b","is_public":true,"type":"chat","context_length":131072,"architecture":{"modality":"text"},"top_provider":{"context_length":131072,"max_completion_tokens":131072}},
			{"id":"google/gemma-4-31b-it","is_public":true,"type":"multimodal","context_length":262144,"architecture":{"modality":"multimodal"},"top_provider":{"context_length":262144,"max_completion_tokens":262141}},
			{"id":"nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B","is_public":true,"type":"chat","context_length":262144,"architecture":{"modality":"multimodal"},"top_provider":{"max_completion_tokens":262144}},
			{"id":"nvidia/Nemotron-3.5-Lightning-30B-A3B","is_public":true,"type":"","context_length":262144,"architecture":{"modality":"text"},"top_provider":{"max_completion_tokens":262143}},
			{"id":"zai-org/GLM-5.3","is_public":true,"type":"chat","context_length":1048576,"architecture":{"modality":"text"},"top_provider":{"max_completion_tokens":0}},
			{"id":"example/private-deployment","is_public":false,"type":"","context_length":262144,"architecture":{"modality":""},"top_provider":{"max_completion_tokens":0}},
			{"id":"example/embedding-model","is_public":true,"type":"embedding","context_length":8192,"architecture":{"modality":"embedding"},"top_provider":{"max_completion_tokens":0}},
			{"id":"example/id-only"},
			{"id":"  "}
		]}`)
	}))
	defer srv.Close()

	list, err := newCrusoeForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	type listed struct {
		types         string
		contextLength int
		maxOutput     int
	}
	want := map[string]listed{
		"openai/gpt-oss-120b":                           {types: "chat", contextLength: 131072, maxOutput: 131072},
		"google/gemma-4-31b-it":                         {types: "chat,vision", contextLength: 262144, maxOutput: 262141},
		"nvidia/Nemotron-3-Nano-Omni-Reasoning-30B-A3B": {types: "chat,vision", contextLength: 262144, maxOutput: 262144},
		"nvidia/Nemotron-3.5-Lightning-30B-A3B":         {types: "chat", contextLength: 262144, maxOutput: 262143},
		"zai-org/GLM-5.3":                               {types: "chat", contextLength: 1048576},
		"example/id-only":                               {types: "chat"},
	}
	if len(list) != len(want) {
		t.Fatalf("models=%v, want %d models", joinModelNames(list, ","), len(want))
	}
	valueOrZero := func(p *int) int {
		if p == nil {
			return 0
		}
		return *p
	}
	for _, model := range list {
		exp, ok := want[model.Name]
		if !ok {
			t.Errorf("unexpected model %q", model.Name)
			continue
		}
		got := listed{
			types:         strings.Join(model.ModelTypes, ","),
			contextLength: valueOrZero(model.ContextLength),
			maxOutput:     valueOrZero(model.MaxOutput),
		}
		if got != exp {
			t.Errorf("%s=%+v, want %+v", model.Name, got, exp)
		}
	}
}

// TestCrusoeListModelsReportsAPIError tests that a rejected key surfaces as
// an error from both the model list and the connectivity check.
func TestCrusoeListModelsReportsAPIError(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"Invalid API key","type":"authentication_error","code":401}}`)
	}))
	defer srv.Close()

	model := newCrusoeForTest(srv.URL)
	if _, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("ListModels err=%v, want status 401", err)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err == nil {
		t.Error("CheckConnection succeeded with a rejected key")
	}
}
