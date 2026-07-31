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

func newCohereForTest(baseURL string) *CoHereModel {
	return NewCoHereModel(
		map[string]string{"default": baseURL},
		URLSuffix{Chat: "v2/chat", Embedding: "v2/embed", Rerank: "v2/rerank"},
	)
}

func TestCohereChatAllowsToolCallOnlyResponse(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/chat" {
			t.Errorf("path=%s", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "cohere-chat-tool",
			"message": map[string]any{
				"role": "assistant",
				"tool_calls": []map[string]any{
					{
						"id":   "tool-1",
						"type": "function",
						"function": map[string]any{
							"name":      "lookup",
							"arguments": `{"q":"ragflow"}`,
						},
					},
				},
			},
			"usage": map[string]any{
				"tokens": map[string]any{
					"input_tokens":  4,
					"output_tokens": 2,
				},
			},
		})
	}))
	defer srv.Close()

	apiKey := "test-key"
	usage := &common.ModelUsage{}
	resp, err := newCohereForTest(srv.URL).ChatWithMessages(
		t.Context(),
		"command-a",
		[]Message{{Role: "user", Content: "use a tool"}},
		&APIConfig{ApiKey: &apiKey},
		nil,
		usage,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	if resp.Answer == nil || *resp.Answer != "" {
		t.Fatalf("Answer=%v, want empty content", resp.Answer)
	}
	if len(resp.ToolCalls) != 1 || resp.ToolCalls[0]["id"] != "tool-1" {
		t.Fatalf("ToolCalls=%#v, want tool-1", resp.ToolCalls)
	}
	assertModelUsage(t, usage, 4, 2, 6)
}

func TestCohereStreamRecordsDeltaUsage(t *testing.T) {
	withSSRFBypass(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/chat" {
			t.Errorf("path=%s", r.URL.Path)
		}
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatalf("unmarshal body: %v", err)
		}
		if body["stream"] != true {
			t.Fatalf("stream=%v, want true", body["stream"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w,
			`data: {"type":"content-delta","delta":{"message":{"content":{"text":"hello"}}}}`+"\n"+
				`data: {"type":"message-end","delta":{"usage":{"tokens":{"input_tokens":4,"output_tokens":2}}}}`+"\n",
		)
	}))
	defer srv.Close()

	apiKey := "test-key"
	cfg := &ChatConfig{}
	usage := &common.ModelUsage{}
	var chunks []string
	err := newCohereForTest(srv.URL).ChatStreamlyWithSender(
		t.Context(),
		"command-a",
		[]Message{{Role: "user", Content: "hi"}},
		&APIConfig{ApiKey: &apiKey},
		cfg,
		usage,
		func(content *string, reasoning *string) error {
			if content != nil {
				chunks = append(chunks, *content)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatalf("ChatStreamlyWithSender: %v", err)
	}
	if strings.Join(chunks, "") != "hello[DONE]" {
		t.Fatalf("chunks=%q, want hello[DONE]", strings.Join(chunks, ""))
	}
	if cfg.UsageResult == nil || cfg.UsageResult.PromptTokens != 4 || cfg.UsageResult.CompletionTokens != 2 || cfg.UsageResult.TotalTokens != 6 {
		t.Fatalf("UsageResult=%#v, want prompt=4 completion=2 total=6", cfg.UsageResult)
	}
	if usage.InputTokens != 4 || usage.OutputTokens != 2 || usage.TotalTokens != 6 {
		t.Fatalf("model usage=(%d,%d,%d), want (4,2,6)", usage.InputTokens, usage.OutputTokens, usage.TotalTokens)
	}
}
