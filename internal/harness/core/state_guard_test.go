package core

import (
	"reflect"
	"testing"

	"ragflow/internal/harness/core/schema"
)

func TestCopyMessagePreservesToolName(t *testing.T) {
	original := &schema.Message{
		Role:     schema.RoleTool,
		Content:  "result",
		Name:     "call-1",
		ToolName: "lookup",
	}
	if copied := copyMessage(original); !reflect.DeepEqual(copied, original) {
		t.Fatalf("copied message = %+v, want %+v", copied, original)
	}
}

func TestCopyMessageAgenticIsolatesContentBlocks(t *testing.T) {
	call := &schema.ToolCallInfo{Name: "lookup", Arguments: `{}`}
	nested := map[string]any{"value": "original", "items": []any{"original"}}
	result := &schema.ToolResult{
		Content: "result",
		Extra:   map[string]any{"nested": nested},
	}
	original := &schema.AgenticMessage{
		Role:    schema.AgenticRoleAssistant,
		Content: "response",
		ContentBlocks: []schema.ContentBlock{
			{Type: "text", Text: "original"},
			{Type: "tool_use", ToolCall: call},
			{Type: "tool_result", ToolResult: result},
		},
	}
	copied := copyMessage(original)
	if !reflect.DeepEqual(copied, original) {
		t.Fatal("copy changed agentic message fields")
	}

	copied.ContentBlocks[0].Text = "changed"
	copied.ContentBlocks[1].ToolCall.Arguments = "changed"
	copied.ContentBlocks[2].ToolResult.Content = "changed"
	extra := copied.ContentBlocks[2].ToolResult.Extra
	extra["new"] = "changed"
	copiedNested := extra["nested"].(map[string]any)
	copiedNested["value"] = "changed"
	copiedNested["items"].([]any)[0] = "changed"

	if original.ContentBlocks[0].Text != "original" {
		t.Error("copy shares content blocks")
	}
	if call.Arguments != `{}` {
		t.Error("copy shares tool call")
	}
	if result.Content != "result" {
		t.Error("copy shares tool result")
	}
	if len(result.Extra) != 1 {
		t.Error("copy shares tool result extra map")
	}
	if nested["value"] != "original" || nested["items"].([]any)[0] != "original" {
		t.Error("copy shares nested tool result extra values")
	}
}
