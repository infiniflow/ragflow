package patchtoolcalls

import (
	"context"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

func TestBeforeModelRewrite_InsertsPlaceholders(t *testing.T) {
	mw := New[*schema.Message](nil)
	msgs := []*schema.Message{
		schema.UserMessage("Hello"),
		{
			Role:    schema.RoleAssistant,
			Content: "",
			ToolCalls: []schema.ToolCall{
				{ID: "call_1", Type: "function", Function: schema.ToolCallFunction{Name: "search", Arguments: `{"q":"test"}`}},
			},
		},
		// No corresponding tool message for call_1
		schema.UserMessage("Tell me more"),
	}
	state := core.NewReActAgentState(msgs, nil, 10)
	_, newState, err := mw.BeforeModelRewrite(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewrite: %v", err)
	}

	// Should have inserted a placeholder for the missing tool result
	foundPlaceholder := false
	for _, m := range newState.Messages {
		if m.Role == schema.RoleTool && m.Name == "call_1" {
			foundPlaceholder = true
			break
		}
	}
	if !foundPlaceholder {
		t.Error("no placeholder inserted for missing tool call 'call_1'")
	}
}

func TestBeforeModelRewrite_CompleteToolCall(t *testing.T) {
	mw := New[*schema.Message](nil)
	msgs := []*schema.Message{
		schema.UserMessage("Hello"),
		{
			Role:    schema.RoleAssistant,
			Content: "",
			ToolCalls: []schema.ToolCall{
				{ID: "call_1", Type: "function", Function: schema.ToolCallFunction{Name: "search", Arguments: `{"q":"test"}`}},
			},
		},
		schema.ToolMessage("Search result", "call_1"),
	}
	state := core.NewReActAgentState(msgs, nil, 10)
	_, newState, err := mw.BeforeModelRewrite(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewrite: %v", err)
	}

	// Should NOT insert a placeholder since the tool result exists
	placeholderCount := 0
	for _, m := range newState.Messages {
		if m.Role == schema.RoleTool && m.Name == "call_1" {
			placeholderCount++
		}
	}
	if placeholderCount > 1 {
		t.Errorf("expected 1 tool message for call_1, got %d", placeholderCount)
	}
}

func TestBeforeModelRewrite_NoToolCalls(t *testing.T) {
	mw := New[*schema.Message](nil)
	msgs := []*schema.Message{
		schema.UserMessage("No tools here"),
		{Role: schema.RoleAssistant, Content: "Just a response"},
	}
	state := core.NewReActAgentState(msgs, nil, 10)
	_, newState, err := mw.BeforeModelRewrite(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewrite: %v", err)
	}
	if len(newState.Messages) != 2 {
		t.Errorf("expected no changes, got %d messages", len(newState.Messages))
	}
}

func TestBeforeModelRewrite_MultipleMissingCalls(t *testing.T) {
	mw := New[*schema.Message](nil)
	msgs := []*schema.Message{
		schema.UserMessage("Hello"),
		{
			Role:    schema.RoleAssistant,
			Content: "",
			ToolCalls: []schema.ToolCall{
				{ID: "call_a", Function: schema.ToolCallFunction{Name: "tool1", Arguments: "{}"}},
				{ID: "call_b", Function: schema.ToolCallFunction{Name: "tool2", Arguments: "{}"}},
			},
		},
		// No tool result after assistant message - both calls are missing
		schema.UserMessage("User follow-up"),
	}
	state := core.NewReActAgentState(msgs, nil, 10)
	_, newState, err := mw.BeforeModelRewrite(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewrite: %v", err)
	}

	// Should have inserted placeholders for both missing calls
	foundA := false
	foundB := false
	for _, m := range newState.Messages {
		if m.Role == schema.RoleTool {
			if m.Name == "call_a" {
				foundA = true
			}
			if m.Name == "call_b" {
				foundB = true
			}
		}
	}
	if !foundA || !foundB {
		t.Errorf("missing placeholders: call_a=%v call_b=%v", foundA, foundB)
	}
}

func TestBeforeModelRewrite_EmptyState(t *testing.T) {
	mw := New[*schema.Message](nil)
	state := core.NewReActAgentState[*schema.Message](nil, nil, 10)
	_, newState, err := mw.BeforeModelRewrite(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewrite: %v", err)
	}
	_ = newState
}
