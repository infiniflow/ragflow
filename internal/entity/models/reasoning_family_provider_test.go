package models

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newReasoningFamilyChatServer(t *testing.T, handler func(t *testing.T, body map[string]interface{}, w http.ResponseWriter)) *httptest.Server {
	t.Helper()

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s, want POST", r.Method)
			return
		}
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path=%s, want /chat/completions", r.URL.Path)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-key" {
			t.Errorf("Authorization=%q, want Bearer test-key", got)
			return
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Errorf("Content-Type=%q, want application/json", got)
			return
		}

		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
			return
		}

		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("unmarshal body: %v\nraw=%s", err, string(raw))
			return
		}

		handler(t, body, w)
	}))
}

func TestGiteeChatExtractsQwenThinkingFromInlineContent(t *testing.T) {
	srv := newReasoningFamilyChatServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "qwen3-8b" {
			t.Errorf("model=%v, want qwen3-8b", body["model"])
		}
		if !assertThinkingEnabled(t, body) {
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": "<think>\nreasoning</think>\nanswer",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	thinking := true
	modelClass := "qwen3-8b"
	resp, err := NewGiteeModel(
		map[string]string{"default": srv.URL},
		URLSuffix{Chat: "chat/completions"},
	).ChatWithMessages(
		"qwen3-8b",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking, ModelClass: &modelClass},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	assertThinkingResponse(t, resp)
}

func TestSiliconflowChatExtractsProviderPrefixedQwenThinkingFromInlineContent(t *testing.T) {
	srv := newReasoningFamilyChatServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "qwen/qwen3-8b" {
			t.Errorf("model=%v, want qwen/qwen3-8b", body["model"])
		}
		if !assertSiliconflowThinkingEnabled(t, body) {
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{
				"message": map[string]interface{}{
					"content": "<think>\nreasoning</think>\nanswer",
				},
			}},
		})
	})
	defer srv.Close()

	apiKey := "test-key"
	thinking := true
	modelClass := "qwen/qwen3-8b"
	resp, err := NewSiliconflowModel(
		map[string]string{"default": srv.URL},
		URLSuffix{Chat: "chat/completions"},
	).ChatWithMessages(
		"qwen/qwen3-8b",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking, ModelClass: &modelClass},
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	assertThinkingResponse(t, resp)
}

// SiliconFlow's wire format uses a boolean `enable_thinking` field rather than
// the DeepSeek-style `thinking: {type: "enabled"}` object. See siliconflow.go
// for the rationale.
func assertSiliconflowThinkingEnabled(t *testing.T, body map[string]interface{}) bool {
	t.Helper()

	et, ok := body["enable_thinking"].(bool)
	if !ok {
		t.Errorf("enable_thinking=%#v, want true", body["enable_thinking"])
		return false
	}
	if !et {
		t.Errorf("enable_thinking=%v, want true", et)
		return false
	}
	return true
}

func assertThinkingEnabled(t *testing.T, body map[string]interface{}) bool {
	t.Helper()

	thinking, ok := body["thinking"].(map[string]interface{})
	if !ok {
		t.Errorf("thinking=%#v, want object", body["thinking"])
		return false
	}
	if thinking["type"] != "enabled" {
		t.Errorf("thinking.type=%v, want enabled", thinking["type"])
		return false
	}
	return true
}

func assertThinkingResponse(t *testing.T, resp *ChatResponse) {
	t.Helper()

	if resp == nil {
		t.Fatal("response is nil")
	}
	if resp.Answer == nil || *resp.Answer != "answer" {
		t.Fatalf("Answer=%v, want answer", resp.Answer)
	}
	if resp.ReasonContent == nil || *resp.ReasonContent != "reasoning" {
		t.Fatalf("ReasonContent=%v, want reasoning", resp.ReasonContent)
	}
}
