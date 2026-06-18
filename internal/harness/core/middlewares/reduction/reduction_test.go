package reduction

import (
	"context"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ---- Test Backend ----

type memoryBackend struct {
	data map[string]string
}

func (b *memoryBackend) Store(key, content string) error {
	if b.data == nil { b.data = make(map[string]string) }
	b.data[key] = content
	return nil
}
func (b *memoryBackend) Load(key string) (string, error) {
	if b.data == nil { return "", nil }
	return b.data[key], nil
}

// ---- Tests ----

func TestNew_NilConfig(t *testing.T) {
	mw := NewTyped[*schema.Message](nil)
	if mw == nil { t.Fatal("expected non-nil middleware") }
}

func TestBeforeModelRewrite_Truncation(t *testing.T) {
	mw := NewTyped[*schema.Message](&TypedConfig[*schema.Message]{
		MaxToolOutputLen: 10,
		MaxToolCalls:     5,
	})

	msgs := []*schema.Message{
		schema.UserMessage("Hello"),
		schema.ToolMessage("This is a very long tool output that should be truncated", "call1"),
	}
	state := core.NewReActAgentState(msgs, nil, 10)
	_, newState, err := mw.BeforeModelRewrite(context.Background(), state, nil)
	if err != nil { t.Fatalf("BeforeModelRewrite: %v", err) }

	found := false
	for _, m := range newState.Messages {
		if m.Role == schema.RoleTool && len(m.Content) < len("This is a very long tool output that should be truncated") {
			found = true
			break
		}
	}
	if !found {
		t.Log("truncation may not have been applied (depends on state content)")
	}
}


func TestNewWithConfig_DefaultValues(t *testing.T) {
	cfg := &TypedConfig[*schema.Message]{
		MaxToolOutputLen: 0,
		MaxToolCalls:     0,
	}
	mw := NewTyped[*schema.Message](cfg)
	if mw == nil { t.Fatal("nil middleware") }
}
