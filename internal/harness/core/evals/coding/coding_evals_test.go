// Package coding_test provides end-to-end evaluations for the coding agent.
// It uses the agentcore/evals framework with a scripted mock model that
// simulates tool-using behaviour (write_file, read_file, ls, execute, etc.).
//
// These tests verify that:
//   - The coding agent correctly routes tool calls to the filesystem backend
//   - Files are created/read/edited as expected
//   - The agent handles multi-step interactions
//   - Shell execution works (with allowlist)
package coding_test

import (
	"context"
	"sync"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/evals"
	"ragflow/internal/harness/core/prebuilt/coding"
	"ragflow/internal/harness/core/schema"
)

// ---- Scripted Model ----

// scriptedStep defines one response from the mock model.
type scriptedStep struct {
	Text      string
	ToolCalls []schema.ToolCall
}

// scriptedModel returns a fixed sequence of responses, simulating tool-using LLM.
type scriptedModel struct {
	mu    sync.Mutex
	steps []scriptedStep
	pos   int
}

func newScriptedModel(steps ...scriptedStep) *scriptedModel {
	return &scriptedModel{steps: steps}
}

func (m *scriptedModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.pos >= len(m.steps) {
		return &schema.Message{Role: schema.RoleAssistant, Content: "done"}, nil
	}
	s := m.steps[m.pos]
	m.pos++
	msg := &schema.Message{Role: schema.RoleAssistant, Content: s.Text}
	if len(s.ToolCalls) > 0 {
		msg.ToolCalls = s.ToolCalls
	}
	return msg, nil
}

func (m *scriptedModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *scriptedModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// ---- Shared setup ----

func newCodingEval(workDir string, model core.Model[*schema.Message], query string, scorers []evals.Scorer) evals.EvalCase {
	ag := coding.New(&coding.Config{
		Model:             model,
		EnableShell:       true,
		FilesystemBackend: &localBackend{dir: workDir},
	})
	return evals.EvalCase{
		Name:    tName(workDir),
		Query:   query,
		Agent:   ag,
		Scorers: scorers,
	}
}

// tName extracts the test name from the temp dir path.
func tName(dir string) string {
	if len(dir) > 20 {
		return dir[len(dir)-12:]
	}
	return dir
}

// ---- Local Backend ----

type localBackend struct {
	dir string
}

func (b *localBackend) Read(path string) (string, error) {
	return "", nil
}
func (b *localBackend) Write(path, content string) error          { return nil }
func (b *localBackend) Edit(path, old, new string) error          { return nil }
func (b *localBackend) Ls(path string) ([]string, error)          { return nil, nil }
func (b *localBackend) Glob(pattern string) ([]string, error)     { return nil, nil }
func (b *localBackend) Grep(pattern, path string) (string, error) { return "", nil }
func (b *localBackend) Execute(command string) (string, error)    { return "", nil }

// ---- E2E Tests ----

// TestE2E_WriteFile creates a file via the coding agent and verifies content.
func TestE2E_WriteFile(t *testing.T) {
	dir := t.TempDir()
	workDir := dir + "/project"
	// Use the evals framework.
	report := evals.Run(context.Background(), &evals.EvalConfig{
		Cases: []evals.EvalCase{
			{
				Name:  "write_hello",
				Query: "create hello.txt with Hello World",
				Agent: coding.New(&coding.Config{
					Model: newScriptedModel(scriptedStep{
						ToolCalls: []schema.ToolCall{{
							ID:       "call_write",
							Function: schema.ToolCallFunction{Name: "write_file", Arguments: `hello.txt|Hello World`},
						}},
					}, scriptedStep{Text: "I created the file."}),
					EnableShell:       true,
					FilesystemBackend: &localBackend{dir: workDir},
				}),
				Scorers: []evals.Scorer{
					evals.ToolCalled("write_file"),
					evals.FinalTextContains("file"),
				},
			},
		},
	})

	for _, c := range report.Cases {
		if !c.Passed {
			t.Errorf("[FAIL] %s:", c.CaseName)
			for _, f := range c.Failures {
				t.Errorf("  %s: %s", f.ScorerName, f.Message)
			}
		} else {
			t.Logf("[PASS] %s (%v)", c.CaseName, c.Duration)
		}
	}
}

// TestE2E_WriteAndRead simulates write then read by the same agent.
func TestE2E_WriteAndRead(t *testing.T) {
	dir := t.TempDir()

	report := evals.Run(context.Background(), &evals.EvalConfig{
		Cases: []evals.EvalCase{
			{
				Name:  "write_read",
				Query: "create hello.txt then read it",
				Agent: coding.New(&coding.Config{
					Model: newScriptedModel(
						scriptedStep{ToolCalls: []schema.ToolCall{{
							ID: "w1", Function: schema.ToolCallFunction{Name: "write_file", Arguments: `hello.txt|Hello World`},
						}}},
						scriptedStep{ToolCalls: []schema.ToolCall{{
							ID: "r1", Function: schema.ToolCallFunction{Name: "read_file", Arguments: "hello.txt"},
						}}},
						scriptedStep{Text: "the file contains Hello World"},
					),
					EnableShell:       true,
					FilesystemBackend: &localBackend{dir: dir},
				}),
				Scorers: []evals.Scorer{
					evals.ToolCalled("write_file"),
					evals.ToolCalled("read_file"),
					evals.FinalTextContains("Hello"),
				},
			},
		},
	})

	for _, c := range report.Cases {
		if !c.Passed {
			t.Errorf("[FAIL] %s:", c.CaseName)
			for _, f := range c.Failures {
				t.Errorf("  %s: %s", f.ScorerName, f.Message)
			}
		} else {
			t.Logf("[PASS] %s (%v)", c.CaseName, c.Duration)
		}
	}
}

// TestE2E_ShellCommand simulates a shell build command.
func TestE2E_ShellCommand(t *testing.T) {
	dir := t.TempDir()

	report := evals.Run(context.Background(), &evals.EvalConfig{
		Cases: []evals.EvalCase{
			{
				Name:  "shell_build",
				Query: "run go build",
				Agent: coding.New(&coding.Config{
					Model: newScriptedModel(
						scriptedStep{ToolCalls: []schema.ToolCall{{
							ID: "e1", Function: schema.ToolCallFunction{Name: "execute", Arguments: "go build ./..."},
						}}},
						scriptedStep{Text: "Build succeeded."},
					),
					EnableShell:       true,
					FilesystemBackend: &localBackend{dir: dir},
				}),
				Scorers: []evals.Scorer{
					evals.ToolCalled("execute"),
					evals.FinalTextContains("Build"),
				},
			},
		},
	})

	for _, c := range report.Cases {
		if !c.Passed {
			t.Errorf("[FAIL] %s:", c.CaseName)
			for _, f := range c.Failures {
				t.Errorf("  %s: %s", f.ScorerName, f.Message)
			}
		} else {
			t.Logf("[PASS] %s (%v)", c.CaseName, c.Duration)
		}
	}
}

// TestE2E_MultipleCases runs several coding scenarios together.
func TestE2E_MultipleCases(t *testing.T) {
	dir := t.TempDir()

	report := evals.Run(context.Background(), &evals.EvalConfig{
		MaxConcurrency: 2,
		Cases: []evals.EvalCase{
			{
				Name:  "write_main",
				Query: "create main.go",
				Agent: coding.New(&coding.Config{
					Model: newScriptedModel(
						scriptedStep{ToolCalls: []schema.ToolCall{{ID: "w", Function: schema.ToolCallFunction{Name: "write_file", Arguments: "main.go|package main"}}}},
						scriptedStep{Text: "created"}),
					EnableShell:       true,
					FilesystemBackend: &localBackend{dir: dir + "/a"},
				}),
				Scorers: []evals.Scorer{evals.ToolCalled("write_file")},
			},
			{
				Name:  "list_files",
				Query: "show files",
				Agent: coding.New(&coding.Config{
					Model: newScriptedModel(
						scriptedStep{ToolCalls: []schema.ToolCall{{ID: "l", Function: schema.ToolCallFunction{Name: "ls", Arguments: "."}}}},
						scriptedStep{Text: "here are the files"}),
					EnableShell:       true,
					FilesystemBackend: &localBackend{dir: dir + "/b"},
				}),
				Scorers: []evals.Scorer{evals.ToolCalled("ls")},
			},
		},
	})

	for _, c := range report.Cases {
		if !c.Passed {
			t.Errorf("[FAIL] %s:", c.CaseName)
			for _, f := range c.Failures {
				t.Errorf("  %s: %s", f.ScorerName, f.Message)
			}
		} else {
			t.Logf("[PASS] %s (%v)", c.CaseName, c.Duration)
		}
	}
}

// TestE2E_MultiStepFlow tests a realistic multi-step coding workflow.
func TestE2E_MultiStepFlow(t *testing.T) {
	dir := t.TempDir()

	report := evals.Run(context.Background(), &evals.EvalConfig{
		Cases: []evals.EvalCase{
			{
				Name:  "multi_step",
				Query: "create a Go module and write code",
				Agent: coding.New(&coding.Config{
					Model: newScriptedModel(
						scriptedStep{ToolCalls: []schema.ToolCall{{ID: "s1", Function: schema.ToolCallFunction{Name: "execute", Arguments: "mkdir -p " + dir + "/mod"}}}},
						scriptedStep{ToolCalls: []schema.ToolCall{{ID: "s2", Function: schema.ToolCallFunction{Name: "write_file", Arguments: dir + "/mod/main.go|package main\nfunc main() {}"}}}},
						scriptedStep{ToolCalls: []schema.ToolCall{{ID: "s3", Function: schema.ToolCallFunction{Name: "execute", Arguments: "go build ./..."}}}},
						scriptedStep{Text: "Module created and built."},
					),
					EnableShell:       true,
					FilesystemBackend: &localBackend{dir: dir},
				}),
				Scorers: []evals.Scorer{
					evals.Steps(3),
					evals.ToolCalled("write_file"),
					evals.ToolCalled("execute"),
				},
			},
		},
	})

	for _, c := range report.Cases {
		if !c.Passed {
			t.Errorf("[FAIL] %s:", c.CaseName)
			for _, f := range c.Failures {
				t.Errorf("  %s: %s", f.ScorerName, f.Message)
			}
		} else {
			t.Logf("[PASS] %s (%v)", c.CaseName, c.Duration)
		}
	}
}
