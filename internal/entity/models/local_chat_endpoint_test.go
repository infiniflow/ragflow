package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ragflow/internal/common"
)

// newLocalChatPathServer serves only the expected chat endpoint and records
// the path of every request. Requests to any other path — including the bare
// base URL the local drivers used to produce when async_chat was not
// configured — receive a realistic 404 so a misrouted call fails loudly.
func newLocalChatPathServer(t *testing.T, paths chan<- string, ollama bool) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths <- r.URL.Path
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") && !strings.HasSuffix(r.URL.Path, "/api/chat") {
			http.Error(w, "404 page not found", http.StatusNotFound)
			return
		}
		if ollama {
			var body map[string]interface{}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["stream"] == true {
				w.Header().Set("Content-Type", "application/x-ndjson")
				_, _ = io.WriteString(w, "{\"message\":{\"content\":\"ok\"},\"done\":false}\n{\"done\":true}\n")
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"message":{"content":"ok"},"done":true,"prompt_eval_count":10,"eval_count":20}`)
			return
		}
		var body map[string]interface{}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["stream"] == true {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\n")
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`)
	}))
}

// TestLocalDriversRouteQwenGlmToChatEndpoint reproduces #19004: the LM-Studio,
// Ollama and VLLM provider configs (conf/models/{lmstudio,ollama,vllm}.json)
// define no async_chat suffix, yet the drivers rewrote the chat URL for
// qwen/glm-named models to that empty suffix, sending every request to the
// bare base URL — so every chat call to the local model failed (agent could
// not answer, auto-keyword extraction aborted document indexing). Those models
// must keep using the regular chat endpoint.
func TestLocalDriversRouteQwenGlmToChatEndpoint(t *testing.T) {
	withSSRFBypass(t)

	cases := []struct {
		name    string
		new     func(baseURL string) ModelDriver
		ollama  bool
		model   string
		stream  bool
		wantURL string
	}{
		{
			name: "lm-studio non-stream glm",
			new: func(baseURL string) ModelDriver {
				return NewLmStudioModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			model:   "glm-4-flash",
			wantURL: "/chat/completions",
		},
		{
			name: "lm-studio stream qwen",
			new: func(baseURL string) ModelDriver {
				return NewLmStudioModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			model:   "qwen-max",
			stream:  true,
			wantURL: "/chat/completions",
		},
		{
			name: "ollama non-stream qwen underscore tag",
			new: func(baseURL string) ModelDriver {
				return NewOllamaModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "api/chat"})
			},
			ollama:  true,
			model:   "qwen_7b",
			wantURL: "/api/chat",
		},
		{
			name: "ollama stream glm",
			new: func(baseURL string) ModelDriver {
				return NewOllamaModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "api/chat"})
			},
			ollama:  true,
			model:   "glm-4-flash",
			stream:  true,
			wantURL: "/api/chat",
		},
		{
			name: "vllm non-stream qwen",
			new: func(baseURL string) ModelDriver {
				return NewVllmModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			model:   "qwen-max",
			wantURL: "/chat/completions",
		},
		{
			name: "vllm stream glm",
			new: func(baseURL string) ModelDriver {
				return NewVllmModel(map[string]string{"default": baseURL}, URLSuffix{Chat: "chat/completions"})
			},
			model:   "glm-4-flash",
			stream:  true,
			wantURL: "/chat/completions",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			paths := make(chan string, 1)
			server := newLocalChatPathServer(t, paths, testCase.ollama)
			defer server.Close()

			messages := []Message{{Role: "user", Content: "hello"}}
			apiConfig := &APIConfig{}
			usage := &common.ModelUsage{}
			chatConfig := &ChatConfig{}
			if testCase.stream {
				if err := testCase.new(server.URL).ChatStreamlyWithSender(t.Context(), testCase.model, messages, apiConfig, chatConfig, usage, func(*string, *string) error { return nil }); err != nil {
					t.Fatalf("ChatStreamlyWithSender: %v", err)
				}
			} else {
				if _, err := testCase.new(server.URL).ChatWithMessages(t.Context(), testCase.model, messages, apiConfig, nil, usage); err != nil {
					t.Fatalf("ChatWithMessages: %v", err)
				}
			}

			select {
			case got := <-paths:
				if !strings.HasSuffix(got, testCase.wantURL) {
					t.Fatalf("request path=%s, want suffix %s", got, testCase.wantURL)
				}
			default:
				t.Fatal("no request reached the server")
			}
		})
	}
}
