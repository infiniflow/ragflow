package agentsmd

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ---- Test Backends ----

type testBackend struct {
	content string
	exists  bool
	readErr error
}

func (b *testBackend) Read(path string) (string, error) {
	if b.readErr != nil { return "", b.readErr }
	return b.content, nil
}
func (b *testBackend) Exists(path string) bool { return b.exists }

type importBackend struct {
	files map[string]string
}

func (b *importBackend) Read(path string) (string, error) {
	if c, ok := b.files[path]; ok { return c, nil }
	return "", errors.New("file not found: " + path)
}
func (b *importBackend) Exists(path string) bool {
	_, ok := b.files[path]
	return ok
}

// ---- Tests ----

func TestBeforeAgent_ContentInjection(t *testing.T) {
	backend := &testBackend{content: "# Available Agents\n- coder\n- reviewer", exists: true}
	mw := NewTyped[*schema.Message](&TypedConfig[*schema.Message]{
		Backend: backend,
		Files:   []string{"AGENTS.md"},
	})

	rc := &core.ReActAgentContext{Instruction: "Help me."}
	_, newRc, err := mw.BeforeAgent(context.Background(), rc)
	if err != nil { t.Fatalf("BeforeAgent: %v", err) }
	if !strings.Contains(newRc.Instruction, "Available Agents") {
		t.Error("content not injected into instruction")
	}
}

func TestBeforeAgent_EmptyBackend(t *testing.T) {
	mw := NewTyped[*schema.Message](&TypedConfig[*schema.Message]{})
	rc := &core.ReActAgentContext{Instruction: "Base"}
	_, newRc, _ := mw.BeforeAgent(context.Background(), rc)
	if newRc.Instruction != "Base" {
		t.Error("empty backend should not modify instruction")
	}
}

func TestBeforeAgent_NilConfig(t *testing.T) {
	mw := NewTyped[*schema.Message](nil)
	if mw == nil {
		t.Fatal("nil middleware")
	}
}

func contains(s, sub string) bool {
	return strings.Contains(s, sub)
}
