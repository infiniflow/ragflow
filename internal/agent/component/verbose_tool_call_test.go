package component

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"
)

func TestFormatVerboseToolCall(t *testing.T) {
	pending := map[string]schema.ToolCall{
		"call_1": {
			ID: "call_1",
			Function: schema.FunctionCall{
				Name:      "search_0",
				Arguments: `{"q":"ragflow"}`,
			},
		},
	}
	msg := &schema.Message{
		Role:       schema.Tool,
		ToolCallID: "call_1",
		Content:    "ok",
	}
	got := formatVerboseToolCall(msg, pending)
	if !strings.HasPrefix(got, "<tool_call>") || !strings.HasSuffix(got, "</tool_call>") {
		t.Fatalf("missing tool_call wrappers: %q", got)
	}
	var payload map[string]any
	body := strings.TrimSuffix(strings.TrimPrefix(got, "<tool_call>"), "</tool_call>")
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("payload json: %v\n%s", err, body)
	}
	if payload["name"] != "search_0" {
		t.Errorf("name=%v, want search_0", payload["name"])
	}
	if payload["result"] != "ok" {
		t.Errorf("result=%v, want ok", payload["result"])
	}
	args, ok := payload["args"].(map[string]any)
	if !ok || args["q"] != "ragflow" {
		t.Errorf("args=%v, want {q:ragflow}", payload["args"])
	}
}

func TestFormatVerboseToolCall_MissingPending(t *testing.T) {
	got := formatVerboseToolCall(&schema.Message{Role: schema.Tool, Content: "x"}, nil)
	if !strings.Contains(got, `"name": "tool"`) {
		t.Fatalf("expected fallback name, got %q", got)
	}
}
