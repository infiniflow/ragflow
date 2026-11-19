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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
)

func novaCodeTestConfig(t *testing.T, protocol, effort string) *APIConfig {
	t.Helper()
	raw, err := json.Marshal(novaCodeCredentials{APIKey: "test-secret", WireAPI: protocol, ReasoningEffort: effort})
	if err != nil {
		t.Fatal(err)
	}
	key := string(raw)
	return &APIConfig{ApiKey: &key}
}

func TestNovaCodeProviderRegistration(t *testing.T) {
	dir, restore := setupProviderTestDir(t, "novacode.json")
	defer restore()
	if err := InitProviderManager(dir); err != nil {
		t.Fatal(err)
	}
	provider := GetProviderManager().FindProvider("NovaCode")
	if provider == nil {
		t.Fatal("NovaCode provider not found")
	}
	driver, ok := provider.ModelDriver.(*NovaCodeModel)
	if !ok {
		t.Fatalf("driver=%T", provider.ModelDriver)
	}
	if driver.baseModel.URLSuffix.Responses != "responses" {
		t.Fatalf("Responses suffix missing")
	}
	if driver.Name() != "NovaCode" {
		t.Fatalf("Name=%s", driver.Name())
	}
	if _, ok := driver.NewInstance(map[string]string{"default": "http://localhost"}).(*NovaCodeModel); !ok {
		t.Fatal("NewInstance lost driver type")
	}
}

func TestNovaCodeChatProtocols(t *testing.T) {
	withSSRFBypass(t)
	for _, protocol := range []string{"responses", "chat_completions"} {
		t.Run(protocol, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-secret" {
					t.Errorf("incorrect authorization")
				}
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if _, ok := body["thinking"]; ok {
					t.Error("must not send thinking")
				}
				if body["model"] != "gpt-6.1-sol" {
					t.Errorf("model=%v", body["model"])
				}
				if protocol == "responses" {
					if r.URL.Path != "/v1/responses" {
						t.Errorf("path=%s", r.URL.Path)
					}
					reasoning := body["reasoning"].(map[string]any)
					if reasoning["effort"] != "medium" {
						t.Errorf("reasoning=%v", reasoning)
					}
					if _, ok := body["messages"]; ok {
						t.Error("Responses contains messages")
					}
					if _, ok := body["reasoning_effort"]; ok {
						t.Error("Responses contains reasoning_effort")
					}
					fmt.Fprint(w, `{"id":"resp-1","status":"completed","output":[{"type":"reasoning","summary":[{"text":"considered"}]},{"type":"message","content":[{"type":"output_text","text":"pong"}]}],"usage":{"input_tokens":4,"output_tokens":3,"total_tokens":7}}`)
				} else {
					if r.URL.Path != "/v1/chat/completions" {
						t.Errorf("path=%s", r.URL.Path)
					}
					if body["reasoning_effort"] != "medium" {
						t.Errorf("effort=%v", body["reasoning_effort"])
					}
					if _, ok := body["input"]; ok {
						t.Error("chat contains Responses input")
					}
					fmt.Fprint(w, `{"id":"chat-1","choices":[{"message":{"content":"pong"}}],"usage":{"prompt_tokens":4,"completion_tokens":3,"total_tokens":7}}`)
				}
			}))
			defer server.Close()
			driver := NewNovaCodeModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{})
			name := "gpt-6.1-sol"
			cfg := &ChatConfig{}
			usage := &common.ModelUsage{}
			// Exercise the same ChatModel wrapper as connection verification: its
			// default Thinking=false must not disable the configured reasoning effort.
			result, err := NewChatModel(driver, &name, novaCodeTestConfig(t, protocol, "")).ChatWithMessages(t.Context(), []Message{{Role: "user", Content: "Hi"}}, cfg, usage)
			if err != nil {
				t.Fatal(err)
			}
			if result.Answer == nil || *result.Answer != "pong" {
				t.Fatalf("answer=%v", result.Answer)
			}
			if cfg.UsageResult == nil || cfg.UsageResult.TotalTokens != 7 || usage.TotalTokens != 7 {
				t.Fatalf("usage=%+v context=%+v", cfg.UsageResult, usage)
			}
		})
	}
}

func TestNovaCodeEffort(t *testing.T) {
	driver := NewNovaCodeModel(nil, URLSuffix{})
	for _, effort := range []string{"low", "medium", "high", "xhigh", "max"} {
		t.Run(effort, func(t *testing.T) {
			for _, wire := range []string{"responses", "chat_completions"} {
				cfg := &ChatConfig{Effort: &effort}
				_, _, body, responses, err := driver.prepare(novaCodeTestConfig(t, wire, "low"), cfg, "gpt-6.1-sol", []Message{{Role: "user", Content: "Hi"}}, false)
				if err != nil {
					t.Fatal(err)
				}
				got := body["reasoning_effort"]
				if responses {
					got = body["reasoning"].(map[string]any)["effort"]
				}
				if got != effort {
					t.Fatalf("effort=%v want=%s", got, effort)
				}
			}
		})
	}
	none := "none"
	if _, _, _, _, err := driver.prepare(novaCodeTestConfig(t, "responses", "medium"), &ChatConfig{Effort: &none}, "m", []Message{{Role: "user", Content: "Hi"}}, false); err == nil {
		t.Fatal("none must not be sent to NovaCode")
	}
}

func TestNovaCodeResponsesToolsAndVision(t *testing.T) {
	maxTokens, verbosity, choice := 200, "low", "auto"
	body, err := buildNovaCodeResponsesRequest(&ChatConfig{MaxTokens: &maxTokens, Verbosity: &verbosity, Tools: []map[string]any{{"type": "function", "function": map[string]any{"name": "weather", "parameters": map[string]any{"type": "object"}}}}, ToolChoice: &choice}, "m", []Message{
		{Role: "user", Content: []map[string]any{{"type": "text", "text": "describe"}, {"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64,AA==", "detail": "low"}}}},
		{Role: "assistant", ToolCalls: []map[string]any{{"id": "call-1", "type": "function", "function": map[string]any{"name": "weather", "arguments": "{}"}}}},
		{Role: "tool", ToolCallID: "call-1", Content: "sunny"},
	}, false, "high")
	if err != nil {
		t.Fatal(err)
	}
	input := body["input"].([]any)
	content := input[0].(map[string]any)["content"].([]map[string]any)
	if content[0]["type"] != "input_text" || content[1]["type"] != "input_image" || content[1]["image_url"] != "data:image/png;base64,AA==" {
		t.Fatalf("content=%v", content)
	}
	call := input[1].(map[string]any)
	output := input[2].(map[string]any)
	if call["type"] != "function_call" || call["name"] != "weather" || output["type"] != "function_call_output" || output["call_id"] != "call-1" {
		t.Fatalf("input=%v", input)
	}
	tools := body["tools"].([]map[string]any)
	if tools[0]["name"] != "weather" || tools[0]["function"] != nil {
		t.Fatalf("tools=%v", tools)
	}
	if body["max_output_tokens"] != 200 || body["max_tokens"] != nil {
		t.Fatalf("tokens=%v", body)
	}
	if body["text"].(map[string]any)["verbosity"] != "low" || body["tool_choice"] != "auto" {
		t.Fatal(body)
	}
	named := &ChatConfig{ToolChoiceValue: map[string]any{"type": "function", "function": map[string]any{"name": "weather"}}}
	body, err = buildNovaCodeResponsesRequest(named, "m", []Message{{Role: "user", Content: "Hi"}}, false, "medium")
	if err != nil {
		t.Fatal(err)
	}
	if body["tool_choice"].(map[string]any)["name"] != "weather" {
		t.Fatal(body)
	}
}

func TestNovaCodeResponsesNonStreamingToolCall(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"status":"completed","output":[{"type":"function_call","id":"fc-1","call_id":"call-1","name":"weather","arguments":"{\"city\":\"Singapore\"}"}]}`)
	}))
	defer server.Close()
	driver := NewNovaCodeModel(map[string]string{"default": server.URL}, URLSuffix{})
	result, err := driver.ChatWithMessages(t.Context(), "gpt-6.1-sol", []Message{{Role: "user", Content: "weather?"}}, novaCodeTestConfig(t, "responses", "medium"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Answer == nil || len(result.ToolCalls) != 1 || result.ToolCalls[0]["id"] != "call-1" {
		t.Fatalf("result=%+v", result)
	}
}

func TestNovaCodeResponsesStreaming(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path=%s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		if body["stream"] != true || body["stream_options"] != nil {
			t.Errorf("body=%v", body)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, `event: response.output_item.added
data: {"type":"response.output_item.added","output_index":1,"item":{"type":"function_call","id":"fc-1","call_id":"call-1","name":"weather","arguments":""}}

data: {"type":"response.function_call_arguments.delta","output_index":1,"delta":"{\"city\":"}

data: {"type":"response.function_call_arguments.delta","output_index":1,"delta":"\"Singapore\"}"}

data: {"type":"response.function_call_arguments.done","output_index":1,"arguments":"{\"city\":\"Singapore\"}"}

data: {"type":"response.reasoning_summary_text.delta","delta":"thinking"}

data: {"type":"response.output_text.delta","delta":"hello"}

data: {"type":"response.completed","response":{"id":"resp-1","status":"completed","usage":{"input_tokens":2,"output_tokens":3,"total_tokens":5}}}

`)
	}))
	defer server.Close()
	driver := NewNovaCodeModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{})
	cfg := &ChatConfig{}
	var content, reason strings.Builder
	err := driver.ChatStreamlyWithSender(t.Context(), "gpt-6.1-sol", []Message{{Role: "user", Content: "Hi"}}, novaCodeTestConfig(t, "responses", "medium"), cfg, nil, func(c, r *string) error {
		if c != nil {
			content.WriteString(*c)
		}
		if r != nil {
			reason.WriteString(*r)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if content.String() != "hello[DONE]" || reason.String() != "thinking" {
		t.Fatalf("content=%s reason=%s", content.String(), reason.String())
	}
	if cfg.UsageResult == nil || cfg.UsageResult.TotalTokens != 5 || cfg.ToolCallsResult == nil || len(*cfg.ToolCallsResult) != 1 {
		t.Fatalf("config=%+v", cfg)
	}
	call := (*cfg.ToolCallsResult)[0]
	if call["id"] != "call-1" || call["function"].(map[string]any)["arguments"] != `{"city":"Singapore"}` {
		t.Fatalf("call=%v", call)
	}
}

func TestNovaCodeResponsesStreamFailures(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"truncated", `data: {"type":"response.output_text.delta","delta":"Hi"}` + "\n\n", "before response.completed"},
		{"done alone", "data: [DONE]\n\n", "before response.completed"},
		{"malformed", "data: {broken}\n\n", "NovaCode stream"},
		{"error", `data: {"type":"error","message":"denied"}` + "\n\n", "denied"},
		{"failed", `data: {"type":"response.failed","response":{"status":"failed","error":{"message":"upstream failure"}}}` + "\n\n", "upstream failure"},
		{"incomplete", `data: {"type":"response.incomplete","response":{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}}` + "\n\n", "max_output_tokens"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := handleNovaCodeResponseStream(strings.NewReader(tc.body), nil, nil, func(*string, *string) error { return nil })
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%v want %s", err, tc.want)
			}
		})
	}
	sentinel := errors.New("sender stopped")
	err := handleNovaCodeResponseStream(strings.NewReader("data: {\"type\":\"response.output_text.delta\",\"delta\":\"Hi\"}\n\n"), nil, nil, func(*string, *string) error { return sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("sender error=%v", err)
	}
}

func TestNovaCodeCancellation(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.created\"}\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	driver := NewNovaCodeModel(map[string]string{"default": server.URL}, URLSuffix{})
	err := driver.ChatStreamlyWithSender(ctx, "m", []Message{{Role: "user", Content: "Hi"}}, novaCodeTestConfig(t, "responses", "medium"), nil, nil, func(*string, *string) error { return nil })
	if err == nil {
		t.Fatal("cancelled stream succeeded")
	}
}

func TestNovaCodeModelListCredentials(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/public/models" {
			if r.Header.Get("Authorization") != "" {
				t.Error("public metadata request leaked credentials")
			}
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/v1/models" || r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Errorf("request=%s authorization correct=%v", r.URL.Path, r.Header.Get("Authorization") == "Bearer test-secret")
		}
		fmt.Fprint(w, `{"data":[{"id":"gpt-6.1-sol"},{"id":"gpt-image-2"},{"id":"text-embedding-3-large"}]}`)
	}))
	defer server.Close()
	driver := NewNovaCodeModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{})
	models, err := driver.ListModels(t.Context(), novaCodeTestConfig(t, "responses", "high"))
	if err != nil {
		t.Fatal(err)
	}
	if len(models) != 1 || models[0].Name != "gpt-6.1-sol" {
		t.Fatalf("models=%v", models)
	}
}

func TestNovaCodeResponseErrors(t *testing.T) {
	withSSRFBypass(t)
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"http", `{"error":"denied"}`, "status 401", 401},
		{"upstream", `{"error":{"message":"invalid effort"}}`, "invalid effort", 200},
		{"incomplete", `{"status":"incomplete","incomplete_details":{"reason":"max_output_tokens"}}`, "max_output_tokens", 200},
		{"empty", `{"status":"completed","output":[]}`, "no text or tool calls", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tc.status); fmt.Fprint(w, tc.body) }))
			defer server.Close()
			driver := NewNovaCodeModel(map[string]string{"default": server.URL}, URLSuffix{})
			_, err := driver.ChatWithMessages(t.Context(), "m", []Message{{Role: "user", Content: "Hi"}}, novaCodeTestConfig(t, "responses", "medium"), nil, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error=%v want=%s", err, tc.want)
			}
		})
	}
}

func TestNovaCodeResponsesReplaysEncryptedReasoning(t *testing.T) {
	withSSRFBypass(t)
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		include, _ := body["include"].([]any)
		if len(include) != 1 || include[0] != "reasoning.encrypted_content" {
			t.Errorf("include=%v", include)
		}
		if calls == 1 {
			fmt.Fprint(w, `{"status":"completed","output":[{"id":"rs-1","type":"reasoning","encrypted_content":"opaque-encrypted-state","summary":[]},{"id":"fc-1","type":"function_call","call_id":"call-1","name":"weather","arguments":"{}"}]}`)
			return
		}
		input := body["input"].([]any)
		if len(input) != 4 {
			t.Errorf("input item count=%d want 4", len(input))
			return
		}
		reasoning := input[1].(map[string]any)
		function := input[2].(map[string]any)
		output := input[3].(map[string]any)
		if reasoning["id"] != "rs-1" || reasoning["encrypted_content"] != "opaque-encrypted-state" || function["id"] != "fc-1" || function["call_id"] != "call-1" || output["type"] != "function_call_output" {
			t.Errorf("input=%v", input)
		}
		fmt.Fprint(w, `{"status":"completed","output":[{"type":"message","content":[{"type":"output_text","text":"sunny"}]}]}`)
	}))
	defer server.Close()
	driver := NewNovaCodeModel(map[string]string{"default": server.URL}, URLSuffix{})
	history := []Message{{Role: "user", Content: "weather?"}}
	config := novaCodeTestConfig(t, "responses", "medium")
	response, err := driver.ChatWithMessages(t.Context(), "gpt-6.1-sol", history, config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Replay through the same Message fields used by tool-loop history, after
	// serialization as happens in stored conversation state.
	history = append(history, Message{Role: "assistant", ToolCalls: response.ToolCalls, ProviderState: response.ProviderState}, Message{Role: "tool", ToolCallID: "call-1", Content: "sunny"})
	raw, err := json.Marshal(history)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, &history); err != nil {
		t.Fatal(err)
	}
	response, err = driver.ChatWithMessages(t.Context(), "gpt-6.1-sol", history, config, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if response.Answer == nil || *response.Answer != "sunny" {
		t.Fatal("tool continuation did not return answer")
	}
}

func TestNovaCodeStreamStopsAtCompleted(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"type\":\"response.output_text.delta\",\"delta\":\"OK\"}\n\n")
		fmt.Fprint(w, "data: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\"}}\n\n")
		w.(http.Flusher).Flush()
		// Responses terminates at response.completed, without waiting for a
		// gateway to close the connection or emit Chat Completions' [DONE].
		<-r.Context().Done()
	}))
	defer server.Close()
	driver := NewNovaCodeModel(map[string]string{"default": server.URL}, URLSuffix{})
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	err := driver.ChatStreamlyWithSender(ctx, "m", []Message{{Role: "user", Content: "Hi"}}, novaCodeTestConfig(t, "responses", "medium"), nil, nil, func(*string, *string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
}

// The shared ChatModel tool loops must carry state returned by the actual
// NovaCode driver, including when a streamed tool round has no text deltas.
func TestNovaCodeChatModelToolLoopState(t *testing.T) {
	withSSRFBypass(t)
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprintf("stream=%v", stream), func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					w.WriteHeader(400)
					return
				}
				output := []any{}
				if calls == 1 {
					output = []any{
						map[string]any{"type": "reasoning", "id": "rs-loop", "encrypted_content": "loop-cipher", "summary": []any{}},
						map[string]any{"type": "function_call", "id": "fc-loop", "call_id": "call-loop", "name": "lookup", "arguments": "{}", "status": "completed"},
					}
				} else {
					input, ok := body["input"].([]any)
					if !ok || len(input) != 4 {
						t.Errorf("continuation input=%v", body["input"])
						w.WriteHeader(400)
						return
					}
					reasoning := input[1].(map[string]any)
					function := input[2].(map[string]any)
					result := input[3].(map[string]any)
					if reasoning["encrypted_content"] != "loop-cipher" || function["id"] != "fc-loop" || result["call_id"] != "call-loop" || result["output"] != "found" {
						t.Errorf("state not preserved: %v", input)
					}
					output = []any{map[string]any{"type": "message", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "done"}}}}
				}
				response := map[string]any{"id": "resp-loop", "status": "completed", "output": output, "usage": map[string]any{"input_tokens": 2, "output_tokens": 1, "total_tokens": 3}}
				if !stream {
					_ = json.NewEncoder(w).Encode(response)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				if calls > 1 {
					writeNovaCodeStateEvent(w, map[string]any{"type": "response.output_text.delta", "delta": "done"})
				}
				writeNovaCodeStateEvent(w, map[string]any{"type": "response.completed", "response": response})
			}))
			defer server.Close()
			driver := NewNovaCodeModel(map[string]string{"default": server.URL + "/v1"}, URLSuffix{})
			model := NewChatModel(driver, new("gpt-6.1-sol"), novaCodeTestConfig(t, "responses", "medium"))
			model.BindTools(&stubSession{results: map[string]string{"lookup": "found"}}, []map[string]any{{"type": "function", "function": map[string]any{"name": "lookup", "parameters": map[string]any{"type": "object"}}}})
			model.ToolConfig.MaxRetries = 1
			history := []Message{{Role: "user", Content: "look it up"}}
			var answer string
			var tokens int
			var err error
			if stream {
				tokens, err = model.ChatStreamlyWithTools(t.Context(), "", history, &ChatConfig{}, func(text, reason *string) error {
					if text != nil && *text != "[DONE]" {
						answer += *text
					}
					return nil
				})
			} else {
				answer, tokens, err = model.ChatWithTools(t.Context(), "", history, &ChatConfig{})
			}
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || answer != "done" || tokens != 6 {
				t.Fatalf("calls=%d answer=%s tokens=%d", calls, answer, tokens)
			}
		})
	}
}
