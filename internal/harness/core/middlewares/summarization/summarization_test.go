package summarization

import (
	"context"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// Test helpers

type mockBackend struct {
	responses []string
	callCount int
}

func (m *mockBackend) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	if m.callCount >= len(m.responses) {
		return nil, nil
	}
	resp := m.responses[m.callCount]
	m.callCount++
	return &schema.Message{Role: schema.RoleAssistant, Content: resp}, nil
}

func (m *mockBackend) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *mockBackend) BindTools(tools []*schema.ToolInfo) error { return nil }

// ---- Tests ----

func TestNew_NilConfig(t *testing.T) {
	mw := NewTyped[*schema.Message](nil)
	if mw == nil {
		t.Fatal("nil middleware returned for nil config")
	}
}

func TestBeforeModelRewrite_NoTrigger(t *testing.T) {
	mw := NewTyped[*schema.Message](&TypedConfig[*schema.Message]{
		Trigger: &TriggerCondition{MaxMessages: 100},
	})

	msgs := []*schema.Message{
		schema.UserMessage("Hello"),
		schema.SystemMessage("System prompt"),
	}
	state := core.NewReActAgentState(msgs, nil, 10)
	_, newState, err := mw.BeforeModelRewrite(context.Background(), state, nil)
	if err != nil {
		t.Fatalf("BeforeModelRewrite: %v", err)
	}
	if len(newState.Messages) != 2 {
		t.Errorf("expected 2 messages (no trigger), got %d", len(newState.Messages))
	}
}

func TestExtractText(t *testing.T) {
	tests := []struct {
		name string
		msg  *schema.Message
		want string
	}{
		{"content", schema.UserMessage("Hello world"), "Hello world"},
		{"empty content", &schema.Message{Role: schema.RoleAssistant, Content: ""}, ""},
		{"multi-line", schema.UserMessage("Line1\nLine2"), "Line1\nLine2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.msg.Content
			if got != tt.want {
				t.Errorf("content = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetSummaryInstruction_Language(t *testing.T) {
	cfg := &TypedConfig[*schema.Message]{
		Model:       &mockBackend{},
		SummaryLang: "Chinese",
	}
	mw := NewTyped[*schema.Message](cfg)
	if mw == nil {
		t.Fatal("nil middleware")
	}
}

func TestSummarization_NilModelConfig(t *testing.T) {
	cfg := &TypedConfig[*schema.Message]{
		Model:   nil,
		Trigger: &TriggerCondition{MaxMessages: 1},
	}
	mw := NewTyped[*schema.Message](cfg)
	if mw == nil {
		t.Fatal("nil middleware")
	}
}
