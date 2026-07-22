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
	"testing"
)

func testNonStreamingToolCall(t *testing.T, modelName, path string, newDriver func(string) ModelDriver) {
	ctx := t.Context()
	t.Helper()
	var requestBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("path = %q, want %q", r.URL.Path, path)
		}
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":null,"tool_calls":[{"id":"call-2","type":"function","function":{"name":"retrieval","arguments":"{}"}}]}}]}`))
	}))
	defer server.Close()

	apiKey, toolChoice := "test-key", "required"
	response, err := newDriver(server.URL).ChatWithMessages(
		ctx,
		modelName,
		[]Message{
			{Role: "assistant", ToolCalls: []map[string]interface{}{{"id": "call-1"}}},
			{Role: "tool", Content: "result", ToolCallID: "call-1"},
		},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Tools: []map[string]interface{}{{"type": "function"}}, ToolChoice: &toolChoice},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if requestBody["tool_choice"] != toolChoice || requestBody["tools"] == nil {
		t.Errorf("tool config = %#v", requestBody)
	}
	messages, _ := requestBody["messages"].([]interface{})
	toolMessage, _ := messages[1].(map[string]interface{})
	if toolMessage["tool_call_id"] != "call-1" {
		t.Errorf("tool message = %#v", toolMessage)
	}
	if len(response.ToolCalls) != 1 || response.ToolCalls[0]["id"] != "call-2" {
		t.Errorf("tool calls = %#v", response.ToolCalls)
	}
}

func testStreamingToolCall(t *testing.T, modelName, path string, newDriver func(string) ModelDriver) {
	ctx := t.Context()
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			t.Errorf("path = %q, want %q", r.URL.Path, path)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"retrieval","arguments":"{\"query\":\""}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ragflow\"}"}}]},"finish_reason":"tool_calls"}]}

`))
	}))
	defer server.Close()

	apiKey, toolChoice := "test-key", "auto"
	config := &ChatConfig{
		Tools:           []map[string]interface{}{{"type": "function"}},
		ToolChoice:      &toolChoice,
		ToolCallsResult: &[]map[string]interface{}{{"id": "stale"}},
	}
	err := newDriver(server.URL).ChatStreamlyWithSender(
		ctx,
		modelName,
		[]Message{{Role: "user", Content: "find ragflow"}},
		&APIConfig{ApiKey: &apiKey},
		config,
		nil,
		func(_, _ *string) error { return nil },
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if config.ToolCallsResult == nil || len(*config.ToolCallsResult) != 1 {
		t.Fatalf("tool calls = %#v", config.ToolCallsResult)
	}
	function, _ := (*config.ToolCallsResult)[0]["function"].(map[string]interface{})
	if function["arguments"] != `{"query":"ragflow"}` {
		t.Errorf("function = %#v", function)
	}
}
