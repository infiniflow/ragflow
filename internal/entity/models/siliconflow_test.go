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
	"testing"
)

func TestSiliconflowToolCalls(t *testing.T) {
	withSSRFBypass(t)
	newDriver := func(baseURL string) ModelDriver {
		return NewSiliconflowModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
	}
	t.Run("non-streaming", func(t *testing.T) {
		testNonStreamingToolCall(t, "Pro/deepseek-ai/DeepSeek-V4-Pro", "/chat/completions", newDriver)
	})
	t.Run("streaming", func(t *testing.T) {
		testStreamingToolCall(t, "Pro/deepseek-ai/DeepSeek-V4-Pro", "/chat/completions", newDriver)
	})
}

func TestSiliconflowChatRejectsMissingContent(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[{"message":{}}]}`))
	}))
	defer server.Close()

	apiKey := "test-key"
	model := NewSiliconflowModel(map[string]string{"default": server.URL}, URLSuffix{Chat: "chat/completions"})
	ctx := t.Context()
	if _, err := model.ChatWithMessages(ctx, "model", []Message{{Role: "user", Content: "hi"}}, &APIConfig{ApiKey: &apiKey}, nil, nil); err == nil {
		t.Fatal("expected missing content error")
	}
}

func TestSiliconflowChatWithMessagesExtractsResponseAndUsage(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-siliconflow",
			"object":  "chat.completion",
			"created": 1,
			"model":   "Qwen/Qwen3-8B",
			"choices": []map[string]any{{
				"index": 0,
				"message": map[string]any{
					"role":              "assistant",
					"content":           "answer",
					"reasoning_content": "\nreasoning",
				},
				"finish_reason": "stop",
			}},
			"usage": map[string]any{
				"prompt_tokens":     3,
				"completion_tokens": 5,
				"total_tokens":      8,
				"completion_tokens_details": map[string]any{
					"reasoning_tokens": 2,
				},
				"prompt_tokens_details": map[string]any{
					"cached_tokens": 1,
				},
			},
		})
	}))
	defer server.Close()

	apiKey := "test-key"
	thinking := true
	response, err := NewSiliconflowModel(
		map[string]string{"default": server.URL},
		URLSuffix{Chat: "chat/completions"},
	).ChatWithMessages(
		t.Context(),
		"Qwen/Qwen3-8B",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if response.Answer == nil || *response.Answer != "answer" {
		t.Fatalf("Answer=%#v, want answer", response.Answer)
	}
	if response.ReasonContent == nil || *response.ReasonContent != "reasoning" {
		t.Fatalf("ReasonContent=%#v, want reasoning", response.ReasonContent)
	}
	if response.Usage == nil || response.Usage.PromptTokens != 3 || response.Usage.CompletionTokens != 5 || response.Usage.TotalTokens != 8 {
		t.Fatalf("Usage=%#v, want prompt=3 completion=5 total=8", response.Usage)
	}
}

func TestSiliconflowChatStreamlyWithSenderCollectsUsage(t *testing.T) {
	withSSRFBypass(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var requestBody map[string]any
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		if requestBody["enable_thinking"] != true {
			t.Errorf("enable_thinking=%#v, want true", requestBody["enable_thinking"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":5,\"total_tokens\":8}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	apiKey := "test-key"
	thinking := true
	config := &ChatConfig{Thinking: &thinking}
	err := NewSiliconflowModel(
		map[string]string{"default": server.URL},
		URLSuffix{Chat: "chat/completions"},
	).ChatStreamlyWithSender(
		t.Context(),
		"Qwen/Qwen3-8B",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		config,
		nil,
		func(content *string, reasoning *string) error {
			if reasoning != nil {
				t.Errorf("reasoning=%q", *reasoning)
			}
			if content != nil && *content != "answer" && *content != "[DONE]" {
				t.Errorf("content=%q", *content)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if config.UsageResult == nil || config.UsageResult.PromptTokens != 3 || config.UsageResult.CompletionTokens != 5 || config.UsageResult.TotalTokens != 8 {
		t.Fatalf("UsageResult=%#v, want prompt=3 completion=5 total=8", config.UsageResult)
	}
}
