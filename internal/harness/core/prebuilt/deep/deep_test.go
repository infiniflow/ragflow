package deep

import (
	"context"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

type mockModel struct{}

func (m *mockModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	return &schema.Message{Role: schema.RoleAssistant, Content: "deep result"}, nil
}
func (m *mockModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.RoleAssistant, Content: "deep stream"}}), nil
}
func (m *mockModel) BindTools(tools []*schema.ToolInfo) error { return nil }

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Name != "deep_agent" {
		t.Errorf("expected name 'deep_agent', got %q", cfg.Name)
	}
	if cfg.MaxIterations != 20 {
		t.Errorf("expected MaxIterations 20, got %d", cfg.MaxIterations)
	}
}

func TestNewTyped_NilConfig(t *testing.T) {
	agent := NewTyped(nil)
	if agent == nil {
		t.Fatal("nil agent for nil config")
	}
}

func TestNewTyped_WithModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model = &mockModel{}
	agent := NewTyped(cfg)
	if agent == nil {
		t.Fatal("nil agent")
	}
	name := agent.Name(context.Background())
	if name != "deep_agent" {
		t.Errorf("name = %q", name)
	}
}

func TestNew(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model = &mockModel{}
	agent := New(cfg)
	if agent == nil {
		t.Fatal("nil agent")
	}
	_ = agent
}

func TestPrompt(t *testing.T) {
	prompt := Prompt()
	if prompt == "" {
		t.Error("empty prompt")
	}
}

func TestSelectPrompt(t *testing.T) {
	prompt := SelectPrompt("en")
	if prompt == "" {
		t.Error("empty select prompt")
	}
}

func TestDefaultConfig_Enhanced(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.SubAgents != nil {
		t.Error("expected nil SubAgents by default")
	}
	if cfg.FailoverModel != nil {
		t.Error("expected nil FailoverModel by default")
	}
	if cfg.OutputKey != "" {
		t.Errorf("expected empty OutputKey, got %q", cfg.OutputKey)
	}
}

func TestNewWithSubAgents_NilConfig(t *testing.T) {
	ctx := context.Background()
	flow, err := NewWithSubAgents(ctx, nil)
	if err != nil {
		t.Fatalf("NewWithSubAgents(nil): %v", err)
	}
	if flow == nil {
		t.Fatal("nil flow agent")
	}
}

func TestNewWithSubAgents_NoSubs(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.Model = &mockModel{}
	flow, err := NewWithSubAgents(ctx, cfg)
	if err != nil {
		t.Fatalf("NewWithSubAgents: %v", err)
	}
	if flow == nil {
		t.Fatal("nil flow agent")
	}
}

func TestNewWithSubAgents_WithSubs(t *testing.T) {
	ctx := context.Background()
	cfg := DefaultConfig()
	cfg.Model = &mockModel{}
	sub := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: &mockModel{}, Instruction: "You are a helper.",
	}).WithName("helper")

	cfg.SubAgents = []SubAgentSpec{
		{Name: "helper", Description: "A helper agent", Agent: sub},
	}

	flow, err := NewWithSubAgents(ctx, cfg)
	if err != nil {
		t.Fatalf("NewWithSubAgents with subs: %v", err)
	}
	if flow == nil {
		t.Fatal("nil flow agent")
	}
}

func TestWithFailoverModel(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Model = &mockModel{}
	cfg.FailoverModel = &mockModel{}

	agent := NewTyped(cfg)
	if agent == nil {
		t.Fatal("nil agent")
	}
}

func TestNewWithSubAgents_BasicCreation(t *testing.T) {
	subAgent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model:       &mockModel{},
		Instruction: "You handle data processing.",
	}).WithName("data_processor")

	cfg := &Config{
		Model: &mockModel{},
		SubAgents: []SubAgentSpec{
			{Name: "data_processor", Description: "Handles data", Agent: subAgent},
		},
	}
	// Test the agent creation (not the sub-agent flow - that needs actual execution)
	agent := NewTyped(cfg)
	if agent == nil {
		t.Fatal("nil agent")
	}
}
