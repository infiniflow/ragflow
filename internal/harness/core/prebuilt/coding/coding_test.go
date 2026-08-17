package coding

import (
	"context"
	"strings"
	"sync"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/middlewares/subagent"
	"ragflow/internal/harness/core/profile"
	"ragflow/internal/harness/core/schema"
)

// ---- Mock Model ----

type mockModel struct {
	responses []string
	mu        sync.Mutex
}

func (m *mockModel) addResp(r string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, r)
}

func (m *mockModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.responses) == 0 {
		return &schema.Message{Role: schema.RoleAssistant, Content: "ok"}, nil
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return &schema.Message{Role: schema.RoleAssistant, Content: resp}, nil
}

func (m *mockModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *mockModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// ---- Tests ----

// TestNewCodingAgent_Basic verifies a coding agent is created and can run.
func TestNewCodingAgent_Basic(t *testing.T) {
	model := &mockModel{}
	model.addResp("I will read the file first.")

	agent := New(&Config{
		Model: model,
		Name:  "test_coder",
	})
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}

	ctx := context.Background()
	runner := core.NewTypedRunner(core.RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("fix this code")})

	var final string
	var gotErr error
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			gotErr = ev.Err
			break
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil &&
			!ev.Output.MessageOutput.IsStreaming &&
			ev.Output.MessageOutput.Message != nil {
			final = ev.Output.MessageOutput.Message.Content
		}
	}
	if gotErr != nil {
		t.Fatalf("unexpected error: %v", gotErr)
	}
	if final != "I will read the file first." {
		t.Errorf("expected 'I will read the file first.', got %q", final)
	}
	t.Logf("coding agent basic: final=%q", final)
}

// TestNewCodingAgent_WithShell verifies shell-enabled coding agent.
func TestNewCodingAgent_WithShell(t *testing.T) {
	model := &mockModel{}
	model.addResp("running build")

	agent := New(&Config{
		Model:       model,
		Name:        "shell_coder",
		EnableShell: true,
	})
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}

	ctx := context.Background()
	runner := core.NewTypedRunner(core.RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("build")})

	var final string
	var gotErr error
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			gotErr = ev.Err
			break
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil &&
			!ev.Output.MessageOutput.IsStreaming &&
			ev.Output.MessageOutput.Message != nil {
			final = ev.Output.MessageOutput.Message.Content
		}
	}
	if gotErr != nil {
		t.Fatalf("unexpected error: %v", gotErr)
	}
	if final != "running build" {
		t.Errorf("expected 'running build', got %q", final)
	}
	t.Logf("coding agent with shell: final=%q", final)
}

// TestNewCodingAgent_WithSubAgents verifies sub-agent support.
func TestNewCodingAgent_WithSubAgents(t *testing.T) {
	subModel := &mockModel{}
	subModel.addResp("sub-agent result")
	subSpec := subagent.SubAgentSpec{
		Name:        "researcher",
		Description: "Research topics",
		AgentConfig: &subagent.AgentConfig{
			Model: subModel,
		},
	}

	parentModel := &mockModel{}
	parentModel.addResp("parent answer")

	agent := New(&Config{
		Model:         parentModel,
		Name:          "agentic_coder",
		SubAgentSpecs: []subagent.SubAgentSpec{subSpec},
	})
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}

	ctx := context.Background()
	runner := core.NewTypedRunner(core.RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("research and code")})

	var final string
	var gotErr error
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			gotErr = ev.Err
			break
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil &&
			!ev.Output.MessageOutput.IsStreaming &&
			ev.Output.MessageOutput.Message != nil {
			final = ev.Output.MessageOutput.Message.Content
		}
	}
	if gotErr != nil {
		t.Fatalf("unexpected error: %v", gotErr)
	}
	if final != "parent answer" {
		t.Errorf("expected 'parent answer', got %q", final)
	}
	t.Logf("coding agent with sub-agents: final=%q", final)
}

// TestShellAllowList verifies the shell allow-list middleware.
func TestShellAllowList(t *testing.T) {
	tests := []struct {
		name    string
		command string
		allowed bool
	}{
		{"git allowed", "git status", true},
		{"go build allowed", "go build ./...", true},
		{"npm install allowed", "npm install", true},
		{"ls allowed", "ls -la", true},
		{"rm blocked", "rm -rf /tmp", false},
		{"chmod blocked", "chmod -R 777 /etc", false},
		{"dd blocked", "dd if=/dev/zero of=/dev/sda", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mw := NewShellAllowList(&ShellAllowListConfig{
				AllowedCommands: DefaultShellAllowList(),
				BlockedCommands: DefaultBlockedCommands(),
			})

			var called bool
			ep := func(ctx context.Context, args string, opts ...core.ToolOption) (string, error) {
				called = true
				return "ok", nil
			}

			wrapped, err := mw.WrapToolInvoke(context.Background(), ep, &core.ToolContext{Name: "execute"})
			if err != nil {
				t.Fatalf("WrapToolInvoke error: %v", err)
			}

			result, err := wrapped(context.Background(), tt.command)
			if err != nil {
				t.Fatalf("invoke error: %v", err)
			}

			if tt.allowed && !called {
				t.Errorf("expected tool to be called for %q, but it was blocked: %s", tt.command, result)
			}
			if !tt.allowed && called {
				t.Errorf("expected tool to be blocked for %q, but it was called", tt.command)
			}
		})
	}
}

// TestShellAllowList_Passthrough verifies passthrough mode allows everything.
func TestShellAllowList_Passthrough(t *testing.T) {
	mw := NewShellAllowList(&ShellAllowListConfig{Passthrough: true})

	var called bool
	ep := func(ctx context.Context, args string, opts ...core.ToolOption) (string, error) {
		called = true
		return "ok", nil
	}

	wrapped, err := mw.WrapToolInvoke(context.Background(), ep, &core.ToolContext{Name: "execute"})
	if err != nil {
		t.Fatalf("WrapToolInvoke error: %v", err)
	}

	_, err = wrapped(context.Background(), "rm -rf /")
	if err != nil {
		t.Fatalf("invoke error: %v", err)
	}
	if !called {
		t.Error("expected tool to be called in passthrough mode")
	}
}

// TestHarnessProfile verifies the coding harness profile registration.
func TestHarnessProfile(t *testing.T) {
	profile.ClearHarnesses()
	RegisterHarnessProfile()

	h := profile.LookupHarness("coding-agent")
	if h == nil {
		t.Fatal("expected coding-agent harness profile to be registered")
	}
	if h.BaseSystemPrompt == nil || !strings.Contains(*h.BaseSystemPrompt, "software engineer") {
		t.Errorf("expected system prompt about 'software engineer', got %v", h.BaseSystemPrompt)
	}
	if h.MaxIterations != 30 {
		t.Errorf("expected MaxIterations=30, got %d", h.MaxIterations)
	}
	if h.RecursionDepth != 5 {
		t.Errorf("expected RecursionDepth=5, got %d", h.RecursionDepth)
	}
}

// TestLocalShellBackend verifies basic local shell operations.
func TestLocalShellBackend(t *testing.T) {
	b := &localShellBackend{}

	// Write a temp file.
	tmpDir := t.TempDir()
	tmpFile := tmpDir + "/test.txt"
	err := b.Write(tmpFile, "hello world")
	if err != nil {
		t.Fatalf("Write error: %v", err)
	}

	// Read it back.
	content, err := b.Read(tmpFile)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if content != "hello world" {
		t.Errorf("expected 'hello world', got %q", content)
	}

	// Edit.
	err = b.Edit(tmpFile, "hello", "goodbye")
	if err != nil {
		t.Fatalf("Edit error: %v", err)
	}
	content, _ = b.Read(tmpFile)
	if content != "goodbye world" {
		t.Errorf("expected 'goodbye world', got %q", content)
	}

	// Ls.
	entries, err := b.Ls(tmpDir)
	if err != nil {
		t.Fatalf("Ls error: %v", err)
	}
	if len(entries) != 1 || entries[0] != "test.txt" {
		t.Errorf("expected [test.txt], got %v", entries)
	}

	// Execute.
	out, err := b.Execute("echo 'hello from shell'")
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}
	if !strings.Contains(out, "hello from shell") {
		t.Errorf("expected output containing 'hello from shell', got %q", out)
	}
	t.Logf("local shell backend: all operations passed")
}
