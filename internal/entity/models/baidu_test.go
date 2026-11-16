package models

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// newBaiduChatServer stands up an httptest server that records the JSON
// chat request body and answers with a single OpenAI-style choice.
func newBaiduChatServer(t *testing.T, captured *map[string]interface{}) (*BaiduModel, *APIConfig) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("failed to read body: %v", err)
			return
		}
		var body map[string]interface{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Errorf("invalid JSON body: %v\n%s", err, string(raw))
			return
		}
		*captured = body
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"choices":[{"message":{"role":"assistant","content":"ok"}}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(server.Close)
	apiKey := "test-key"
	baseURL := server.URL
	model := NewBaiduModel(map[string]string{"default": server.URL}, URLSuffix{Chat: "chat/completions"})
	return model, &APIConfig{ApiKey: &apiKey, BaseURL: &baseURL}
}

func TestBaiduChatMovesSystemPromptToTopLevelField(t *testing.T) {
	withSSRFBypass(t)
	var captured map[string]interface{}
	model, apiConfig := newBaiduChatServer(t, &captured)

	messages := []Message{
		{Role: "system", Content: "You are a helpful assistant."},
		{Role: "user", Content: "hi"},
	}
	if _, err := model.ChatWithMessages(context.Background(), "ernie-4.5", messages, apiConfig, nil, nil); err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}

	if got, ok := captured["system"].(string); !ok || got != "You are a helpful assistant." {
		t.Fatalf("expected top-level system prompt %q, got %v", "You are a helpful assistant.", captured["system"])
	}
	raw, err := json.Marshal(captured["messages"])
	if err != nil {
		t.Fatalf("marshal messages: %v", err)
	}
	var sent []map[string]interface{}
	if err := json.Unmarshal(raw, &sent); err != nil {
		t.Fatalf("unmarshal messages: %v", err)
	}
	if len(sent) != 1 || sent[0]["role"] != "user" || sent[0]["content"] != "hi" {
		t.Fatalf("expected only the user message in messages, got %s", raw)
	}
}

func TestBaiduChatWithoutSystemPromptOmitsSystemField(t *testing.T) {
	withSSRFBypass(t)
	var captured map[string]interface{}
	model, apiConfig := newBaiduChatServer(t, &captured)

	messages := []Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "bye"},
	}
	if _, err := model.ChatWithMessages(context.Background(), "ernie-4.5", messages, apiConfig, nil, nil); err != nil {
		t.Fatalf("ChatWithMessages: %v", err)
	}

	if _, ok := captured["system"]; ok {
		t.Fatalf("expected no system field without a system message, got %v", captured["system"])
	}
	if stream, ok := captured["stream"].(bool); !ok || stream {
		t.Fatalf("expected stream=false on the non-streaming path, got %v", captured["stream"])
	}
}

func TestSplitBaiduSystemMessages(t *testing.T) {
	cases := []struct {
		name          string
		messages      []Message
		wantSystem    string
		wantRemaining []Message
	}{
		{
			name: "joins leading system messages",
			messages: []Message{
				{Role: "system", Content: "rule one"},
				{Role: "system", Content: "rule two"},
				{Role: "user", Content: "hi"},
			},
			wantSystem:    "rule one\n\nrule two",
			wantRemaining: []Message{{Role: "user", Content: "hi"}},
		},
		{
			name: "drops non-leading system roles",
			messages: []Message{
				{Role: "user", Content: "hi"},
				{Role: "system", Content: "mid-conversation"},
				{Role: "assistant", Content: "hello"},
			},
			wantSystem:    "",
			wantRemaining: []Message{{Role: "user", Content: "hi"}, {Role: "assistant", Content: "hello"}},
		},
		{
			name: "ignores blank system content",
			messages: []Message{
				{Role: "system", Content: "   "},
				{Role: "user", Content: "hi"},
			},
			wantSystem:    "",
			wantRemaining: []Message{{Role: "user", Content: "hi"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			system, rest := splitBaiduSystemMessages(tc.messages)
			if system != tc.wantSystem {
				t.Fatalf("system prompt: got %q, want %q", system, tc.wantSystem)
			}
			if len(rest) != len(tc.wantRemaining) {
				t.Fatalf("remaining messages: got %d, want %d", len(rest), len(tc.wantRemaining))
			}
			for i := range rest {
				if rest[i].Role != tc.wantRemaining[i].Role || rest[i].Content != tc.wantRemaining[i].Content {
					t.Fatalf("remaining[%d]: got %v, want %v", i, rest[i], tc.wantRemaining[i])
				}
			}
		})
	}
}

func TestBuildBaiduChatBodyRejectsSystemOnlyMessages(t *testing.T) {
	if _, err := buildBaiduChatBody(nil, "ernie-4.5", []Message{{Role: "system", Content: "only system"}}, false); err == nil {
		t.Fatal("expected an error when only system messages remain")
	}
}
