package models

import (
	"net/http"
	"net/http/httptest"
	"ragflow/internal/common"
	"testing"
	"time"
)

func TestProviderChatUsage(t *testing.T) {
	withSSRFBypass(t)
	type providerCase struct {
		name string
		new  func(string) ModelDriver
	}

	cases := []providerCase{
		{
			name: "deepseek",
			new: func(baseURL string) ModelDriver {
				return NewDeepSeekModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
		},
		{
			name: "hunyuan",
			new: func(baseURL string) ModelDriver {
				return NewHunyuanModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
		},
		{
			name: "jina",
			new: func(baseURL string) ModelDriver {
				return NewJinaModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
		},
		{
			name: "gitee",
			new: func(baseURL string) ModelDriver {
				return NewGiteeModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
		},
		{
			name: "openrouter",
			new: func(baseURL string) ModelDriver {
				return NewOpenRouterModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
		},
		{
			name: "jiekouai",
			new: func(baseURL string) ModelDriver {
				return NewJieKouAIModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`{"id":"chat-request","choices":[{"message":{"content":"answer"}}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`))
			}))
			defer server.Close()

			apiKey := "test-key"
			usage := &common.ModelUsage{StartAt: time.Now()}
			response, err := tc.new(server.URL).ChatWithMessages(
				t.Context(),
				"test-model",
				[]Message{{Role: "user", Content: "hello"}},
				&APIConfig{ApiKey: &apiKey},
				nil,
				usage,
			)
			if err != nil {
				t.Fatalf("ChatWithMessages: %v", err)
			}
			if response.Usage == nil || response.Usage.PromptTokens != 3 || response.Usage.CompletionTokens != 5 || response.Usage.TotalTokens != 8 {
				t.Fatalf("response usage=%#v", response.Usage)
			}
			assertModelUsage(t, usage, 3, 5, 8)
			if usage.Type != "chat" {
				t.Fatalf("usage type=%q, want chat", usage.Type)
			}
		})
	}
}
