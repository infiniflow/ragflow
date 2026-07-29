package supervisor

import (
	"context"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Name != "supervisor" {
		t.Errorf("default name = %s", cfg.Name)
	}
}

func TestNew_RequiresModel(t *testing.T) {
	ctx := context.Background()
	_, err := New(ctx, &Config{})
	if err == nil {
		t.Error("expected error when Model is nil")
	}
}

func TestNew_WithModelAndAgents(t *testing.T) {
	ctx := context.Background()
	model := &mockSupervisorModel{}

	subAgent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model:       model,
		Instruction: "You are a coder.",
	}).WithName("coder")

	flow, err := New(ctx, &Config{
		Model:  model,
		Name:   "my_supervisor",
		Agents: []AgentSpec{{Name: "coder", Description: "Writes code", Agent: subAgent}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if flow == nil {
		t.Fatal("nil flow agent")
	}
}

func TestBuildAgentDescriptions(t *testing.T) {
	descs := buildAgentDescriptions([]AgentSpec{
		{Name: "researcher", Description: "Searches the web"},
		{Name: "writer", Description: "Writes articles"},
	})
	if !contains(descs, "researcher") || !contains(descs, "writer") {
		t.Errorf("bad descriptions: %s", descs)
	}
}

func TestBuildAgentDescriptions_Empty(t *testing.T) {
	descs := buildAgentDescriptions(nil)
	if descs != "" {
		t.Error("nil agents should produce empty description")
	}
}

func TestSystemPrompt(t *testing.T) {
	if systemPrompt == "" {
		t.Error("systemPrompt empty")
	}
	if !contains(systemPrompt, "supervisor") {
		t.Error("missing 'supervisor' in prompt")
	}
}

func TestNewWithRouter(t *testing.T) {
	ctx := context.Background()
	model := &mockSupervisorModel{}
	subAgent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{Model: model}).WithName("sub")

	flow, err := NewWithRouter(ctx, model, []AgentSpec{{Name: "sub", Description: "Sub agent", Agent: subAgent}})
	if err != nil {
		t.Fatalf("NewWithRouter: %v", err)
	}
	if flow == nil {
		t.Fatal("nil from NewWithRouter")
	}
}

func TestDeterministicTransfer(t *testing.T) {
	ctx := context.Background()
	model := &mockSupervisorModel{}
	subAgent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{Model: model}).WithName("coder")

	// Verify that sub-agents get wrapped with DeterministicTransfer
	flow, err := New(ctx, &Config{
		Model:  model,
		Name:   "my_supervisor",
		Agents: []AgentSpec{{Name: "coder", Description: "Writes code", Agent: subAgent}},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if flow == nil {
		t.Fatal("nil flow agent")
	}
	// The flow should run without error — deterministic transfer wrapping
	// is internal and should not break normal operation
	input := &core.AgentInput{Messages: []*schema.Message{
		{Role: schema.RoleUser, Content: "write code"},
	}}
	iter := flow.Run(ctx, input)
	var events []*core.AgentEvent
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		events = append(events, ev)
	}
	if len(events) == 0 {
		t.Error("expected at least one event")
	}
}

func TestGetType(t *testing.T) {
	model := &mockSupervisorModel{}

	// The supervisor's ReActAgent should have GetType == "ReActAgent"
	sup := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model:       model,
		Instruction: "test",
	}).WithName("supervisor")

	if sup.GetType() != "ReActAgent" {
		t.Errorf("expected GetType() = ReActAgent, got %s", sup.GetType())
	}
}

type mockSupervisorModel struct{}

func (m *mockSupervisorModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	return &schema.Message{Role: schema.RoleAssistant, Content: "routed to coder"}, nil
}
func (m *mockSupervisorModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{{Content: "routed"}}), nil
}
func (m *mockSupervisorModel) BindTools(tools []*schema.ToolInfo) error { return nil }

func contains(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDeterministicTransferConstraint(t *testing.T) {
	ctx := context.Background()
	model := &mockSupervisorModel{}
	subAgent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model:       model,
		Instruction: "You are a coder.",
	}).WithName("coder")

	// Verify that the sub-agent can be wrapped with deterministic transfer
	wrapped := core.AgentWithDeterministicTransfer(ctx, &core.DeterministicTransferConfig{
		Agent:        subAgent,
		ToAgentNames: []string{"supervisor"},
	})
	if wrapped == nil {
		t.Fatal("nil wrapped agent")
	}
	if wrapped.Name(ctx) != "coder" {
		t.Errorf("expected name 'coder', got %q", wrapped.Name(ctx))
	}
}
