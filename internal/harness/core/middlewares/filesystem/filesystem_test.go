package filesystem

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ---- Test Backend ----

type testBackend struct {
	readResult string
	readErr    error
	written    map[string]string
	grepResult string
	lsResult   []string
}

func (b *testBackend) Read(path string) (string, error) { return b.readResult, b.readErr }
func (b *testBackend) Write(path, content string) error {
	if b.written == nil {
		b.written = make(map[string]string)
	}
	b.written[path] = content
	return nil
}
func (b *testBackend) Edit(path, old, new string) error {
	if b.written == nil {
		b.written = make(map[string]string)
	}
	b.written[path+"_edit"] = new
	return nil
}
func (b *testBackend) Ls(path string) ([]string, error)      { return b.lsResult, nil }
func (b *testBackend) Glob(pattern string) ([]string, error) { return []string{"a.txt", "b.go"}, nil }
func (b *testBackend) Grep(pattern, path string) (string, error) {
	if b.grepResult != "" {
		return b.grepResult, nil
	}
	return "match1\nmatch2", nil
}
func (b *testBackend) Execute(command string) (string, error) { return "done", nil }

// ---- Tests ----

// getTools is a helper that calls ContributeTools and returns the tool list.
func getTools(mw *middleware[*schema.Message]) []core.Tool {
	return mw.ContributeTools(context.Background())
}

func TestNew_NilBackend(t *testing.T) {
	mw := New(nil)
	tools := getTools(mw)
	if len(tools) != 0 {
		t.Error("nil backend should not add tools")
	}
}

func TestNew_AddsAllTools(t *testing.T) {
	mw := New(&Config{Backend: &testBackend{readResult: "hello"}})
	tools := getTools(mw)
	if len(tools) != 7 {
		t.Errorf("expected 7 tools, got %d", len(tools))
	}
}

func TestTool_Read_Function(t *testing.T) {
	mw := New(&Config{Backend: &testBackend{readResult: "file content"}})
	for _, tool := range getTools(mw) {
		if tool.Name() == "read_file" {
			result, err := tool.Invoke(context.Background(), "test.txt")
			if err != nil {
				t.Fatalf("read_file: %v", err)
			}
			if !strings.Contains(result, "file content") {
				t.Errorf("got %q", result)
			}
			return
		}
	}
	t.Error("read_file tool not found")
}

func TestTool_Write_Function(t *testing.T) {
	backend := &testBackend{}
	mw := New(&Config{Backend: backend})
	for _, tool := range getTools(mw) {
		if tool.Name() == "write_file" {
			result, err := tool.Invoke(context.Background(), "file.txt|Hello World")
			if err != nil {
				t.Fatalf("write_file: %v", err)
			}
			t.Logf("write result: %q", result)
			return
		}
	}
	t.Error("write_file tool not found")
}

func TestTool_Edit_Function(t *testing.T) {
	backend := &testBackend{}
	mw := New(&Config{Backend: backend})
	for _, tool := range getTools(mw) {
		if tool.Name() == "edit_file" {
			result, err := tool.Invoke(context.Background(), "file.txt|old text|new text")
			if err != nil {
				t.Fatalf("edit_file: %v", err)
			}
			t.Logf("edit result: %q", result)
			return
		}
	}
	t.Error("edit_file tool not found")
}

func TestTool_Ls_Function(t *testing.T) {
	backend := &testBackend{lsResult: []string{"a.txt", "b.txt"}}
	mw := New(&Config{Backend: backend})
	for _, tool := range getTools(mw) {
		if tool.Name() == "ls" {
			result, err := tool.Invoke(context.Background(), ".")
			if err != nil {
				t.Fatalf("ls: %v", err)
			}
			if !strings.Contains(result, "a.txt") {
				t.Errorf("got %q", result)
			}
			return
		}
	}
	t.Error("ls tool not found")
}

func TestTool_Glob_Function(t *testing.T) {
	mw := New(&Config{Backend: &testBackend{}})
	for _, tool := range getTools(mw) {
		if tool.Name() == "glob" {
			result, err := tool.Invoke(context.Background(), "*.txt")
			if err != nil {
				t.Fatalf("glob: %v", err)
			}
			if !strings.Contains(result, "a.txt") {
				t.Errorf("got %q", result)
			}
			return
		}
	}
	t.Error("glob tool not found")
}

func TestTool_Grep_Function(t *testing.T) {
	mw := New(&Config{Backend: &testBackend{}})
	for _, tool := range getTools(mw) {
		if tool.Name() == "grep" {
			result, err := tool.Invoke(context.Background(), "pattern|.")
			if err != nil {
				t.Fatalf("grep: %v", err)
			}
			if !strings.Contains(result, "match1") {
				t.Errorf("got %q", result)
			}
			return
		}
	}
	t.Error("grep tool not found")
}

func TestTool_Execute_Function(t *testing.T) {
	mw := New(&Config{Backend: &testBackend{}})
	for _, tool := range getTools(mw) {
		if tool.Name() == "execute" {
			result, err := tool.Invoke(context.Background(), "ls -la")
			if err != nil {
				t.Fatalf("execute: %v", err)
			}
			if result != "done" {
				t.Errorf("got %q", result)
			}
			return
		}
	}
	t.Error("execute tool not found")
}

func TestTool_ReadError(t *testing.T) {
	mw := New(&Config{Backend: &testBackend{readErr: errors.New("permission denied")}})
	for _, tool := range getTools(mw) {
		if tool.Name() == "read_file" {
			_, err := tool.Invoke(context.Background(), "secret.txt")
			if err != nil {
				t.Logf("read error propagated: %v", err)
			}
			return
		}
	}
}

func TestTool_Config_DisableTool(t *testing.T) {
	cfg := &Config{
		Backend: &testBackend{readResult: "hello"},
		ToolConfig: map[string]*ToolConfig{
			"execute": {Disabled: true},
		},
	}
	mw := New(cfg)
	for _, tool := range getTools(mw) {
		if tool.Name() == "execute" {
			t.Error("execute tool should be disabled")
		}
	}
}

func TestTool_ReadBytesLimit(t *testing.T) {
	cfg := &Config{
		Backend:   &testBackend{readResult: "short file"},
		ReadBytes: 100,
	}
	mw := New(cfg)
	for _, tool := range getTools(mw) {
		if tool.Name() == "read_file" {
			result, err := tool.Invoke(context.Background(), "short.txt")
			if err != nil {
				t.Fatalf("read_file: %v", err)
			}
			if !strings.Contains(result, "short file") {
				t.Errorf("unexpected result: %q", result)
			}
			return
		}
	}
}
