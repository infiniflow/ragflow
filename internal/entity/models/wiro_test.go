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

// newWiroForTest creates a WiroModel instance for testing.
func newWiroForTest(baseURL string) *WiroModel {
	return NewWiroModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "chat/completions", Models: "models"},
	)
}

// TestWiroName tests the driver name.
func TestWiroName(t *testing.T) {
	if got := newWiroForTest("http://unused").Name(); got != "wiro" {
		t.Errorf("Name()=%q", got)
	}
}

// TestWiroFactory tests creating the driver from factory.
func TestWiroFactory(t *testing.T) {
	driver, err := NewModelFactory().CreateModelDriver("Wiro AI", map[string]string{"default": "http://unused"}, URLSuffix{})
	if err != nil {
		t.Fatalf("CreateModelDriver: %v", err)
	}
	if _, ok := driver.(*WiroModel); !ok {
		t.Fatalf("driver type=%T, want *WiroModel", driver)
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://other"}).(*WiroModel); !ok {
		t.Fatal("NewInstance did not return *WiroModel")
	}
}

// The shipped config must resolve to the Wiro AI driver. CreateModelDriver
// dispatches on strings.ToLower(name), so the "Wiro AI" provider name has to
// match the "wiro ai" case; anything else silently falls through to
// DummyModel. Every catalogued model is a chat model with both limits set.
func TestWiroProviderConfig(t *testing.T) {
	saved := GetProviderManager()
	defer setProviderManager(saved)
	dir, restore := setupProviderTestDir(t, "wiro.json")
	defer restore()

	if err := InitProviderManager(dir); err != nil {
		t.Fatalf("InitProviderManager: %v", err)
	}

	provider := GetProviderManager().FindProvider("Wiro AI")
	if provider == nil {
		t.Fatal("Wiro AI provider not found")
	}
	if _, ok := provider.ModelDriver.(*WiroModel); !ok {
		t.Fatalf("ModelDriver=%T, want *models.WiroModel", provider.ModelDriver)
	}
	if got := provider.URL["default"]; got != "https://llm.wiro.ai/v1" {
		t.Errorf("default URL=%q", got)
	}
	if provider.URLSuffix.Chat != "chat/completions" || provider.URLSuffix.Models != "models" {
		t.Errorf("URLSuffix=%+v", provider.URLSuffix)
	}
	if len(provider.Models) == 0 {
		t.Fatal("Wiro AI catalog is empty")
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
	}
}

// TestWiroChatSendsAPIKey tests chat completion authentication and that the
// "owner/model" id reaches the API unchanged.
func TestWiroChatSendsAPIKey(t *testing.T) {
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
		if body.Model != "openai/gpt-5-4-nano" {
			t.Errorf("model=%q", body.Model)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-wiro",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	usage := &common.ModelUsage{}
	resp, err := newWiroForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"openai/gpt-5-4-nano",
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
// through, while the thinking switch, do_sample and the vLLM chat-template
// payload that the embedded driver adds for qwen3 ids are not sent. stop is
// sent only when it holds a non-empty sequence.
func TestWiroChatSendsChatCompletionsFields(t *testing.T) {
	withSSRFBypass(t)
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody = map[string]any{}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chat-wiro",
			"choices": []map[string]any{{"message": map[string]any{"content": "pong"}}},
			"usage":   map[string]any{"prompt_tokens": 3, "completion_tokens": 5, "total_tokens": 8},
		})
	}))
	defer srv.Close()

	thinking := true
	doSample := true
	temperature := 0.2
	maxTokens := 64
	chat := func(stop []string) {
		t.Helper()
		_, err := newWiroForTest(srv.URL).ChatWithMessages(
			t.Context(),
			"qwen/qwen3-32b",
			[]Message{{Role: "user", Content: "ping"}},
			&APIConfig{ApiKey: &testAPIKey},
			&ChatConfig{Thinking: &thinking, DoSample: &doSample, Temperature: &temperature, MaxTokens: &maxTokens, Stop: &stop},
			&common.ModelUsage{},
		)
		if err != nil {
			t.Fatalf("ChatWithMessages: %v", err)
		}
	}

	chat([]string{})
	for _, key := range []string{"thinking", "chat_template_kwargs", "enable_thinking", "do_sample", "stop"} {
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

	chat([]string{"", "END"})
	stop, ok := gotBody["stop"].([]any)
	if !ok || len(stop) != 1 || stop[0] != "END" {
		t.Errorf("stop=%#v, want [END]", gotBody["stop"])
	}
}

// TestWiroChatStreamReadsReasoning tests the streaming request and that the
// reasoning text, which the gateway streams as delta.reasoning, reaches the
// sender along with the answer and the usage of the final chunk.
func TestWiroChatStreamReadsReasoning(t *testing.T) {
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
		if value, ok := body["thinking"]; ok {
			t.Errorf("request sends thinking=%v", value)
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
	config := &ChatConfig{Thinking: &thinking}
	var answer, reasoning strings.Builder
	err := newWiroForTest(srv.URL).ChatStreamlyWithSender(
		t.Context(),
		"deepseek/v4-flash",
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

// TestWiroListModelsAndCheckConnection tests model listing and connectivity
// check. The model list requires the API key.
func TestWiroListModelsAndCheckConnection(t *testing.T) {
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
				{"id": "openai/gpt-5-4-nano"},
				{"id": "deepseek/v4-flash"},
			},
		})
	}))
	defer srv.Close()

	model := newWiroForTest(srv.URL)
	list, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if joinModelNames(list, ",") != "openai/gpt-5-4-nano,deepseek/v4-flash" {
		t.Errorf("models=%v", list)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err != nil {
		t.Fatalf("CheckConnection: %v", err)
	}
}

// Every call needs the API key. A missing or blank key fails before a
// request is sent, for chat, streaming, the model list and the connection
// check.
func TestWiroRequiresAPIKey(t *testing.T) {
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
	model := newWiroForTest(srv.URL)
	messages := []Message{{Role: "user", Content: "ping"}}
	for name, apiConfig := range configs {
		t.Run(name, func(t *testing.T) {
			_, chatErr := model.ChatWithMessages(t.Context(), "openai/gpt-5-4-nano", messages, apiConfig, nil, nil)
			streamErr := model.ChatStreamlyWithSender(t.Context(), "openai/gpt-5-4-nano", messages, apiConfig, nil, nil, func(*string, *string) error { return nil })
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

// Listed models take their limits and image input from the gateway's
// metadata: context_length is the context window, max_tokens the output
// limit, and "image" in capabilities.input_modalities marks a vision model.
// A null limit stays unset, and a model that names its endpoints without
// "chat" is not listed.
func TestWiroListModelsReadsGatewayMetadata(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"object":"list","data":[
			{"id":"openai/gpt-5-4-nano","context_length":400000,"max_tokens":65536,"capabilities":{"endpoints":["anthropic","chat","responses"],"input_modalities":["text","image","document"]}},
			{"id":"deepseek/v4-flash","context_length":1048576,"max_tokens":384000,"capabilities":{"endpoints":["anthropic","chat","responses"],"input_modalities":["text"]}},
			{"id":"example/limit-unset","context_length":null,"max_tokens":128000,"capabilities":{"endpoints":["chat"],"input_modalities":["text"]}},
			{"id":"example/responses-only","context_length":8192,"max_tokens":4096,"capabilities":{"endpoints":["responses"],"input_modalities":["text"]}},
			{"id":"example/id-only"},
			{"id":"  "}
		]}`)
	}))
	defer srv.Close()

	list, err := newWiroForTest(srv.URL).ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey})
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}

	type listed struct {
		types         string
		contextLength int
		maxOutput     int
	}
	want := map[string]listed{
		"openai/gpt-5-4-nano": {types: "chat,vision", contextLength: 400000, maxOutput: 65536},
		"deepseek/v4-flash":   {types: "chat", contextLength: 1048576, maxOutput: 384000},
		"example/limit-unset": {types: "chat", maxOutput: 128000},
		"example/id-only":     {types: "chat"},
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

// TestWiroListModelsReportsAPIError tests that a rejected key surfaces as an
// error from both the model list and the connectivity check.
func TestWiroListModelsReportsAPIError(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"The supplied API credentials are invalid.","type":"authentication_error","param":null,"code":"invalid_api_key"}}`)
	}))
	defer srv.Close()

	model := newWiroForTest(srv.URL)
	if _, err := model.ListModels(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("ListModels err=%v, want status 401", err)
	}
	if err := model.CheckConnection(t.Context(), &APIConfig{ApiKey: &testAPIKey}); err == nil {
		t.Error("CheckConnection succeeded with a rejected key")
	}
}
