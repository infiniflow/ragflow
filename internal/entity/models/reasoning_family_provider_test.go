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
	withSSRFBypass(t)
	ctx := t.Context()
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
		ctx,
		"qwen3-8b",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking, ModelClass: &modelClass},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	assertThinkingResponse(t, resp)
}

func TestSiliconflowChatExtractsProviderPrefixedQwenThinkingFromInlineContent(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
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
		ctx,
		"qwen/qwen3-8b",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking, ModelClass: &modelClass},
		nil,
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

// TestMoonshotChatSendsThinkingDisabled verifies that disabling thinking
// produces `thinking: {type: "disabled"}` on the wire and applies the
// kimi-k2.6 parameter policy (temperature dropped, top_p/penalties pinned),
// mirroring the Python model-family policy.
func TestMoonshotChatSendsThinkingDisabled(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newReasoningFamilyChatServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if body["model"] != "kimi-k2.6" {
			t.Errorf("model=%v, want kimi-k2.6", body["model"])
		}
		thinking, ok := body["thinking"].(map[string]interface{})
		if !ok || thinking["type"] != "disabled" {
			t.Errorf("thinking=%#v, want {type: disabled}", body["thinking"])
		}
		if _, present := body["temperature"]; present {
			t.Errorf("temperature=%v, want absent", body["temperature"])
		}
		if body["top_p"] != 0.95 {
			t.Errorf("top_p=%v, want 0.95", body["top_p"])
		}
		if body["n"] != float64(1) {
			t.Errorf("n=%v, want 1", body["n"])
		}
		if body["presence_penalty"] != float64(0) || body["frequency_penalty"] != float64(0) {
			t.Errorf("penalties=%v/%v, want 0/0", body["presence_penalty"], body["frequency_penalty"])
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
	thinking := false
	temperature := 0.0
	topP := 0.0
	resp, err := NewMoonshotModel(
		map[string]string{"default": srv.URL},
		URLSuffix{Chat: "chat/completions"},
	).ChatWithMessages(
		ctx,
		"kimi-k2.6",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{Thinking: &thinking, Temperature: &temperature, TopP: &topP},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	assertThinkingResponse(t, resp)
}

// TestMoonshotChatOmitsThinkingWhenUnset verifies no thinking directive is
// sent when the config leaves it unset (the UI "default" option).
func TestMoonshotChatOmitsThinkingWhenUnset(t *testing.T) {
	withSSRFBypass(t)
	ctx := t.Context()
	srv := newReasoningFamilyChatServer(t, func(t *testing.T, body map[string]interface{}, w http.ResponseWriter) {
		if _, present := body["thinking"]; present {
			t.Errorf("thinking=%#v, want absent", body["thinking"])
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
	resp, err := NewMoonshotModel(
		map[string]string{"default": srv.URL},
		URLSuffix{Chat: "chat/completions"},
	).ChatWithMessages(
		ctx,
		"moonshot-v1-8k",
		[]Message{{Role: "user", Content: "ping"}},
		&APIConfig{ApiKey: &apiKey},
		&ChatConfig{},
		nil,
	)
	if err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}
	assertThinkingResponse(t, resp)
}
