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

func TestOpenAICompatibleProvidersExtractStreamingUsage(t *testing.T) {
	withSSRFBypass(t)
	providers := []struct {
		name string
		new  func(string) ModelDriver
		path string
	}{
		{
			name: "siliconflow",
			new: func(baseURL string) ModelDriver {
				return NewSiliconflowModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "aliyun",
			new: func(baseURL string) ModelDriver {
				return NewAliyunModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "qiniu",
			new: func(baseURL string) ModelDriver {
				return NewQiniuModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "volcengine",
			new: func(baseURL string) ModelDriver {
				return NewVolcEngine(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "gitee",
			new: func(baseURL string) ModelDriver {
				return NewGiteeModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "openrouter",
			new: func(baseURL string) ModelDriver {
				return NewOpenRouterModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
		{
			name: "jiekouai",
			new: func(baseURL string) ModelDriver {
				return NewJieKouAIModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "openai/v1/chat/completions"})
			},
			path: "/openai/v1/chat/completions",
		},
		{
			name: "hunyuan",
			new: func(baseURL string) ModelDriver {
				return NewHunyuanModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			path: "/chat/completions",
		},
	}

	for _, provider := range providers {
		provider := provider
		t.Run(provider.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost {
					t.Errorf("method=%s, want POST", r.Method)
				}
				if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
					t.Errorf("Authorization=%q, want Bearer test-key", got)
				}
				if r.URL.Path != provider.path {
					t.Errorf("path=%q, want %q", r.URL.Path, provider.path)
				}
				var requestBody map[string]any
				if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
					t.Fatalf("decode request: %v", err)
				}
				streamOptions, ok := requestBody["stream_options"].(map[string]any)
				if !ok || streamOptions["include_usage"] != true {
					t.Errorf("stream_options=%#v, want include_usage=true", requestBody["stream_options"])
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"answer\"},\"finish_reason\":null}]}\n\n")
				_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":5,\"total_tokens\":8}}\n\n")
				_, _ = io.WriteString(w, "data: [DONE]\n\n")
			}))
			defer server.Close()

			apiKey := "test-key"
			config := &ChatConfig{}
			err := provider.new(server.URL).ChatStreamlyWithSender(
				t.Context(),
				"model",
				[]Message{{Role: "user", Content: "hello"}},
				&APIConfig{ApiKey: &apiKey},
				config,
				nil,
				func(*string, *string) error { return nil },
			)
			if err != nil {
				t.Fatalf("ChatStreamlyWithSender: %v", err)
			}
			if config.UsageResult == nil || config.UsageResult.PromptTokens != 3 || config.UsageResult.CompletionTokens != 5 || config.UsageResult.TotalTokens != 8 {
				t.Fatalf("UsageResult=%#v, want prompt=3 completion=5 total=8", config.UsageResult)
			}
		})
	}
}
