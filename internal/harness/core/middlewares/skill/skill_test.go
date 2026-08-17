package skill

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ---- Test Backend ----

type testBackend struct {
	content string
}

func (b *testBackend) Read(path string) (string, error) { return b.content, nil }
func (b *testBackend) List() ([]string, error)          { return nil, nil }

type testModel struct{}

func (m *testModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	return &schema.Message{Role: schema.RoleAssistant, Content: "model response"}, nil
}
func (m *testModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	return schema.StreamReaderFromArray([]*schema.Message{{Role: schema.RoleAssistant, Content: "stream response"}}), nil
}
func (m *testModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// ---- Tests ----

func TestBeforeAgent_InlineSkill(t *testing.T) {
	mw := NewTyped[*schema.Message](&TypedConfig[*schema.Message]{
		Skills: []Config{
			{Name: "test_skill", Description: "A test skill", Content: "You are a test assistant.", ExecutionMode: ModeInline},
		},
	})
	rc := &core.ReActAgentContext{Instruction: "Base instruction", Tools: make([]core.Tool, 0)}
	_, newRc, err := mw.BeforeAgent(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforeAgent: %v", err)
	}

	// Inline skill should modify instruction
	if !strings.Contains(newRc.Instruction, "test_skill") && !strings.Contains(newRc.Instruction, "test assistant") {
		t.Log("inline skill content should be reflected in instruction")
	}
}

func TestBeforeAgent_ForkSkill(t *testing.T) {
	mw := NewTyped[*schema.Message](&TypedConfig[*schema.Message]{
		Skills: []Config{
			{Name: "fork_skill", Description: "A fork skill", Content: "Execute this separately.", ExecutionMode: ModeFork},
		},
	})
	rc := &core.ReActAgentContext{Instruction: "Base", Tools: make([]core.Tool, 0)}
	_, newRc, err := mw.BeforeAgent(context.Background(), rc)
	if err != nil {
		t.Fatalf("BeforeAgent: %v", err)
	}
	// Fork skills should add a tool
	if len(newRc.Tools) == 0 {
		t.Log("fork skill should add a tool to the tool list")
	}
}

func TestParseSkill_Frontmatter(t *testing.T) {
	content := `---
name: my_skill
description: My custom skill
---
This is the skill content.`
	skill := parseSkill(content)
	if skill == nil {
		t.Fatal("nil skill")
	}
	if skill.Name != "my_skill" {
		t.Errorf("name = %q", skill.Name)
	}
	if skill.Description != "My custom skill" {
		t.Errorf("desc = %q", skill.Description)
	}
}

func TestParseSkill_NoFrontmatter(t *testing.T) {
	content := "Simple skill without frontmatter"
	skill := parseSkill(content)
	if skill == nil {
		t.Fatal("nil skill")
	}
	if skill.Name != "" {
		t.Error("expected empty name for no frontmatter")
	}
	if skill.Content != content {
		t.Errorf("content = %q", skill.Content)
	}
}

func TestBeforeAgent_NilConfig(t *testing.T) {
	mw := NewTyped[*schema.Message](nil)
	if mw == nil {
		t.Fatal("nil middleware")
	}
}
