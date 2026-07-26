package evals

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"ragflow/internal/harness/core"
	"ragflow/internal/harness/core/schema"
)

// ---- Mock model ----

type mockEvalModel struct {
	responses []string
	mu        sync.Mutex
}

func (m *mockEvalModel) addResp(r string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses = append(m.responses, r)
}

func (m *mockEvalModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.responses) == 0 {
		return &schema.Message{Role: schema.RoleAssistant, Content: "done"}, nil
	}
	resp := m.responses[0]
	m.responses = m.responses[1:]
	return &schema.Message{Role: schema.RoleAssistant, Content: resp}, nil
}

func (m *mockEvalModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}
func (m *mockEvalModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// ---- Test helpers ----

func newMockAgent(resp string) core.Agent {
	m := &mockEvalModel{}
	m.addResp(resp)
	return core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: m,
	}).WithName("mock_agent")
}

type mockT struct {
	testing.T
	logs   []string
	errors []string
	fatal  bool
	mu     sync.Mutex
}

func (m *mockT) Logf(format string, args ...any) {
	m.mu.Lock()
	m.logs = append(m.logs, format)
	m.mu.Unlock()
}
func (m *mockT) Errorf(format string, args ...any) {
	m.mu.Lock()
	m.errors = append(m.errors, format)
	m.mu.Unlock()
}
func (m *mockT) Fatal(args ...any) {
	m.mu.Lock()
	m.fatal = true
	m.mu.Unlock()
}

// ---- Tests ----

// TestFinalTextContains verifies the FinalTextContains scorer.
func TestFinalTextContains(t *testing.T) {
	scorer := FinalTextContains("hello")
	err := scorer(context.Background(), &EvalResult{
		Messages: []*schema.Message{
			{Role: schema.RoleAssistant, Content: "Hello, World!"},
		},
	})
	if err != nil {
		t.Errorf("expected pass, got: %v", err)
	}

	// Should fail.
	err = scorer(context.Background(), &EvalResult{
		Messages: []*schema.Message{
			{Role: schema.RoleAssistant, Content: "Goodbye"},
		},
	})
	if err == nil {
		t.Error("expected failure for missing text")
	}
}

// TestFinalTextExcludes verifies the FinalTextExcludes scorer.
func TestFinalTextExcludes(t *testing.T) {
	scorer := FinalTextExcludes("forbidden")
	err := scorer(context.Background(), &EvalResult{
		Messages: []*schema.Message{
			{Role: schema.RoleAssistant, Content: "clean output"},
		},
	})
	if err != nil {
		t.Errorf("expected pass, got: %v", err)
	}

	err = scorer(context.Background(), &EvalResult{
		Messages: []*schema.Message{
			{Role: schema.RoleAssistant, Content: "forbidden content"},
		},
	})
	if err == nil {
		t.Error("expected failure for forbidden text")
	}
}

// TestToolCalled verifies the ToolCalled scorer.
func TestToolCalled(t *testing.T) {
	scorer := ToolCalled("web_search")
	err := scorer(context.Background(), &EvalResult{
		Messages: []*schema.Message{
			{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{
				{Function: schema.ToolCallFunction{Name: "read_file"}},
			}},
			{Role: schema.RoleAssistant, ToolCalls: []schema.ToolCall{
				{Function: schema.ToolCallFunction{Name: "web_search"}},
			}},
		},
	})
	if err != nil {
		t.Errorf("expected pass, got: %v", err)
	}

	err = scorer(context.Background(), &EvalResult{
		Messages: []*schema.Message{
			{Role: schema.RoleAssistant, Content: "no tools called"},
		},
	})
	if err == nil {
		t.Error("expected failure when tool not called")
	}
}

// TestSteps verifies the Steps scorer.
func TestSteps(t *testing.T) {
	scorer := Steps(3)
	err := scorer(context.Background(), &EvalResult{
		Messages: []*schema.Message{
			{Role: schema.RoleAssistant},
			{Role: schema.RoleTool},
			{Role: schema.RoleAssistant},
			{Role: schema.RoleTool},
			{Role: schema.RoleAssistant},
		},
	})
	if err != nil {
		t.Errorf("expected pass, got: %v", err)
	}

	err = scorer(context.Background(), &EvalResult{
		Messages: []*schema.Message{
			{Role: schema.RoleAssistant},
		},
	})
	if err == nil {
		t.Error("expected failure for too few steps")
	}
}

// TestFileContentEquals verifies the FileContentEquals scorer.
func TestFileContentEquals(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world"), 0644)

	scorer := FileContentEquals(path, "hello world")
	err := scorer(context.Background(), &EvalResult{})
	if err != nil {
		t.Errorf("expected pass, got: %v", err)
	}

	scorer = FileContentEquals(path, "wrong content")
	err = scorer(context.Background(), &EvalResult{})
	if err == nil {
		t.Error("expected failure for content mismatch")
	}
}

// TestFileContains verifies the FileContains scorer.
func TestFileContains(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	os.WriteFile(path, []byte("hello world foo bar"), 0644)

	scorer := FileContains(path, "foo")
	err := scorer(context.Background(), &EvalResult{})
	if err != nil {
		t.Errorf("expected pass, got: %v", err)
	}

	scorer = FileContains(path, "notfound")
	err = scorer(context.Background(), &EvalResult{})
	if err == nil {
		t.Error("expected failure for missing content")
	}
}

// TestAgentError verifies the AgentError scorer.
func TestAgentError(t *testing.T) {
	scorer := AgentError()
	err := scorer(context.Background(), &EvalResult{Err: nil})
	if err != nil {
		t.Errorf("expected pass for no error, got: %v", err)
	}

	err = scorer(context.Background(), &EvalResult{Err: core.ErrCancelTimeout})
	if err == nil {
		t.Error("expected failure when error present")
	}
}

// TestAgentErrorContains verifies the AgentErrorContains scorer.
func TestAgentErrorContains(t *testing.T) {
	scorer := AgentErrorContains("cancel")
	err := scorer(context.Background(), &EvalResult{
		Err: core.ErrCancelTimeout,
	})
	if err != nil {
		t.Errorf("expected pass, got: %v", err)
	}

	err = scorer(context.Background(), &EvalResult{Err: nil})
	if err == nil {
		t.Error("expected failure when no error expected")
	}

	// Should fail for non-matching substring.
	err = scorer(context.Background(), &EvalResult{Err: core.ErrExecutionEnded})
	if err == nil {
		t.Error("expected failure for non-matching substring")
	}
}

// TestRunT_Pass verifies RunT with a passing case.
func TestRunT_Pass(t *testing.T) {
	mt := &mockT{}
	model := &mockEvalModel{}
	model.addResp("the answer is 42")

	agent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: model,
	}).WithName("eval_under_test")

	RunT(mt, &EvalConfig{
		Model: model,
		Agent: agent,
		Cases: []EvalCase{
			{
				Name:  "test_pass",
				Query: "what is the answer?",
				Scorers: []Scorer{
					FinalTextContains("42"),
				},
			},
		},
	})

	if mt.fatal {
		t.Error("unexpected fatal")
	}
	if len(mt.errors) > 0 {
		t.Errorf("expected no errors, got: %v", mt.errors)
	}
}

// TestRunT_Fail verifies RunT with a failing case.
func TestRunT_Fail(t *testing.T) {
	mt := &mockT{}
	model := &mockEvalModel{}
	model.addResp("I don't know")

	agent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: model,
	}).WithName("eval_under_test")

	RunT(mt, &EvalConfig{
		Model: model,
		Agent: agent,
		Cases: []EvalCase{
			{
				Name:  "test_fail",
				Query: "what is 42?",
				Scorers: []Scorer{
					FinalTextContains("42"),
				},
			},
		},
	})

	if len(mt.errors) == 0 {
		t.Error("expected errors from failing case")
	}
}

// TestRun_MultipleCases verifies Run with multiple cases.
func TestRun_MultipleCases(t *testing.T) {
	model := &mockEvalModel{}
	model.addResp("alpha")
	model.addResp("beta")
	model.addResp("gamma")

	agent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: model,
	}).WithName("multi_eval")

	report := Run(context.Background(), &EvalConfig{
		Model: model,
		Agent: agent,
		Cases: []EvalCase{
			{Name: "case_a", Query: "a", Scorers: []Scorer{FinalTextContains("alpha")}},
			{Name: "case_b", Query: "b", Scorers: []Scorer{FinalTextContains("beta")}},
			{Name: "case_c", Query: "c", Scorers: []Scorer{FinalTextContains("gamma")}},
		},
	})

	if report.Total != 3 {
		t.Errorf("expected 3 total, got %d", report.Total)
	}
	if report.Passed != 3 {
		t.Errorf("expected 3 passed, got %d", report.Passed)
	}
}

// TestRun_Parallel verifies parallel execution with independent per-case agents.
func TestRun_Parallel(t *testing.T) {
	report := Run(context.Background(), &EvalConfig{
		MaxConcurrency: 4,
		Cases: []EvalCase{
			{
				Name: "p1", Query: "1",
				Agent:   newMockAgent("x"),
				Scorers: []Scorer{FinalTextContains("x")},
			},
			{
				Name: "p2", Query: "2",
				Agent:   newMockAgent("y"),
				Scorers: []Scorer{FinalTextContains("y")},
			},
			{
				Name: "p3", Query: "3",
				Agent:   newMockAgent("z"),
				Scorers: []Scorer{FinalTextContains("z")},
			},
		},
	})

	if report.Total != 3 {
		t.Errorf("expected 3 total, got %d", report.Total)
	}
	if report.Passed != 3 {
		t.Errorf("expected 3 passed, got %d: %+v", report.Passed, report.Cases)
	}
}

// TestReportOutput verifies report file generation.
func TestReportOutput(t *testing.T) {
	dir := t.TempDir()
	model := &mockEvalModel{}
	model.addResp("report test")

	agent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: model,
	}).WithName("report_eval")

	RunT(&mockT{}, &EvalConfig{
		Model:     model,
		Agent:     agent,
		ReportDir: dir,
		Cases: []EvalCase{
			{Name: "report_case", Query: "test", Scorers: []Scorer{FinalTextContains("report")}},
		},
	})

	// Check report files exist.
	if _, err := os.Stat(filepath.Join(dir, "summary.json")); os.IsNotExist(err) {
		t.Error("summary.json not written")
	}
	if _, err := os.Stat(filepath.Join(dir, "results.csv")); os.IsNotExist(err) {
		t.Error("results.csv not written")
	}
}

// TestMultipleScorers verifies a case with multiple scorers.
func TestMultipleScorers(t *testing.T) {
	model := &mockEvalModel{}
	model.addResp("the file contains hello world")

	agent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: model,
	}).WithName("multi_scorer")

	mt := &mockT{}
	RunT(mt, &EvalConfig{
		Model: model,
		Agent: agent,
		Cases: []EvalCase{
			{
				Name:  "multi_check",
				Query: "write hello",
				Scorers: []Scorer{
					FinalTextContains("hello"),
					FinalTextContains("world"),
					Steps(1),
				},
			},
		},
	})

	if mt.fatal {
		t.Error("unexpected fatal")
	}
	if len(mt.errors) > 0 {
		t.Errorf("expected no errors, got: %v", mt.errors)
	}
}
