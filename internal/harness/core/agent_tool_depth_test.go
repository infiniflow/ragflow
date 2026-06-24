package core

import (
	"context"
	"testing"

	"ragflow/internal/harness/core/schema"
)

func TestAgentTool_DepthErrorMessage(t *testing.T) {
	m := &mockModel{}
	m.addResp("ok")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("inner").WithDescription("Inner")

	tool := NewAgentTool(context.Background(), agent, WithMaxDepth(1))

	// Create parent context with depth=1 (simulating one level of nesting).
	ctx := context.WithValue(context.Background(), subAgentDepthKey{}, 1)

	_, err := tool.Invoke(ctx, "{}")
	if err == nil {
		t.Fatal("expected recursion limit error")
	}
	errMsg := err.Error()
	if !containsStr(errMsg, "recursion limit") && !containsStr(errMsg, "max depth") {
		t.Errorf("error should mention recursion limit or max depth, got: %s", errMsg)
	}
	t.Logf("depth error message: %s", errMsg)
}

func containsStr(s, substr string) bool {
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
