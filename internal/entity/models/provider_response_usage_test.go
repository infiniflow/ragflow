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

func TestProviderLocalChatResponsesExposeUsage(t *testing.T) {
	withSSRFBypass(t)
	type providerCase struct {
		name string
		path string
		new  func(string) ModelDriver
	}

	cases := []providerCase{
		{
			name: "jina",
			path: "/chat/completions",
			new: func(baseURL string) ModelDriver {
				return NewJinaModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
		},
		{
			name: "gitee",
			path: "/chat/completions",
			new: func(baseURL string) ModelDriver {
				return NewGiteeModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
		},
		{
			name: "openrouter",
			path: "/chat/completions",
			new: func(baseURL string) ModelDriver {
				return NewOpenRouterModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
		},
		{
			name: "jiekouai",
			path: "/openai/v1/chat/completions",
			new: func(baseURL string) ModelDriver {
				return NewJieKouAIModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "openai/v1/chat/completions"})
			},
		},
		{
			name: "hunyuan",
			path: "/chat/completions",
			new: func(baseURL string) ModelDriver {
				return NewHunyuanModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tc.path {
					t.Errorf("path=%q, want %q", r.URL.Path, tc.path)
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"id": "chat-request",
					"choices": []map[string]any{{
						"message": map[string]any{"content": "answer"},
					}},
					"usage": map[string]any{
						"prompt_tokens":     3,
						"completion_tokens": 5,
						"total_tokens":      8,
					},
				})
			}))
			defer server.Close()

			apiKey := "test-key"
			response, err := tc.new(server.URL).ChatWithMessages(
				t.Context(),
				"model",
				[]Message{{Role: "user", Content: "hello"}},
				&APIConfig{ApiKey: &apiKey},
				nil,
				nil,
			)
			if err != nil {
				t.Fatalf("ChatWithMessages: %v", err)
			}
			if response.Usage == nil || response.Usage.PromptTokens != 3 || response.Usage.CompletionTokens != 5 || response.Usage.TotalTokens != 8 {
				t.Fatalf("Usage=%#v, want prompt=3 completion=5 total=8", response.Usage)
			}
		})
	}
}

// TestApplyStreamUsageAlignsWithRecordResponseUsage verifies that
// applyStreamUsage mirrors recordResponseUsage's nil-modelUsage handling:
// a nil *ModelUsage (the common case from the model_chat / generator service
// layer) must not silently drop the usage, and a populated one must get
// Type="chat" and the token counts written through to analytics. Before the
// alignment the function returned early on a nil modelUsage, so streaming
// callers (anthropic, cohere, google, bedrock, novita) never reached the
// stats driver while the shared HandleStreamingResponse path did.
func TestApplyStreamUsageAlignsWithRecordResponseUsage(t *testing.T) {
	t.Run("populated modelUsage", func(t *testing.T) {
		usage := &TokenUsage{PromptTokens: 3, CompletionTokens: 5, TotalTokens: 8}
		chatConfig := &ChatConfig{}
		modelUsage := &common.ModelUsage{}

		applyStreamUsage(chatConfig, modelUsage, usage)

		if chatConfig.UsageResult != usage {
			t.Fatalf("chatConfig.UsageResult=%#v, want the applied usage", chatConfig.UsageResult)
		}
		if modelUsage.Type != "chat" {
			t.Fatalf("modelUsage.Type=%q, want chat", modelUsage.Type)
		}
		if modelUsage.InputTokens != 3 || modelUsage.OutputTokens != 5 || modelUsage.TotalTokens != 8 {
			t.Fatalf("modelUsage tokens=(%d,%d,%d), want (3,5,8)", modelUsage.InputTokens, modelUsage.OutputTokens, modelUsage.TotalTokens)
		}
	})

	t.Run("nil modelUsage still surfaces usage", func(t *testing.T) {
		usage := &TokenUsage{PromptTokens: 1, CompletionTokens: 2, TotalTokens: 3}
		chatConfig := &ChatConfig{}

		// Must not panic and must still expose the usage to the caller via
		// chatConfig, matching recordResponseUsage's synthetic path.
		applyStreamUsage(chatConfig, nil, usage)

		if chatConfig.UsageResult != usage {
			t.Fatalf("chatConfig.UsageResult=%#v, want the applied usage", chatConfig.UsageResult)
		}
	})

	t.Run("nil usage is a no-op", func(t *testing.T) {
		chatConfig := &ChatConfig{}
		modelUsage := &common.ModelUsage{}
		applyStreamUsage(chatConfig, modelUsage, nil)
		if chatConfig.UsageResult != nil {
			t.Fatalf("chatConfig.UsageResult=%#v, want nil", chatConfig.UsageResult)
		}
		if modelUsage.Type != "" || modelUsage.InputTokens != 0 {
			t.Fatalf("modelUsage mutated by nil usage: %#v", modelUsage)
		}
	})
}
