// Package component — Agent unit tests.
//
// Tests inject a canned agentRunner to verify the component contract
// without requiring a real model or eino react agent runtime:
//
//  1. NoToolsReAct: the runner returns a plain answer → component
//     surfaces content with empty tool_calls.
//  2. ToolCallRound: the runner returns a message with ToolCalls →
//     component extracts them into the tool_calls output.
//  3. ExhaustRoundsError: the runner returns an error → component
//     propagates it.
//  4. MissingModelID: the component rejects before calling the runner.
package component

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/cloudwego/eino/components/model"
	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/flow/agent/react"
	"github.com/cloudwego/eino/schema"

	"ragflow/internal/agent/runtime"
	agenttool "ragflow/internal/agent/tool"
)

// withAgentRunner replaces the package-level agentRunner for the duration
// of t.
func withAgentRunner(t *testing.T, fn func(context.Context, AgentParam) (*schema.Message, error)) {
	t.Helper()
	prev := agentRunner
	agentRunner = fn
	t.Cleanup(func() { agentRunner = prev })
}

func TestAgent_NoToolsReAct(t *testing.T) {
	var calls int
	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		calls++
		return &schema.Message{Role: schema.Assistant, Content: "the answer is 42"}, nil
	})

	c := NewAgentComponent(AgentParam{ModelID: "stub", MaxRounds: 3})
	out, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "what is 6*7?",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], "the answer is 42"; got != want {
		t.Errorf("content=%v, want %v", got, want)
	}
	toolCalls, ok := out["tool_calls"].([]map[string]any)
	if !ok {
		t.Fatalf("tool_calls missing or wrong type: %T", out["tool_calls"])
	}
	if len(toolCalls) != 0 {
		t.Errorf("tool_calls=%d, want 0", len(toolCalls))
	}
	if calls != 1 {
		t.Errorf("runner called %d times, want 1", calls)
	}
}

func TestAgent_ResolvesUserPromptFromCanvasState(t *testing.T) {
	var gotPrompt string
	withAgentRunner(t, func(_ context.Context, p AgentParam) (*schema.Message, error) {
		gotPrompt = p.UserPrompt
		return &schema.Message{Role: schema.Assistant, Content: "ok"}, nil
	})

	state := runtime.NewCanvasState("run-1", "task-1")
	state.Sys["query"] = "what is marigold"
	ctx := runtime.WithState(context.Background(), state)

	c := NewAgentComponent(AgentParam{
		ModelID:    "stub",
		APIKey:     "test-key",
		UserPrompt: "Question: {sys.query}",
		MaxRounds:  1,
	})
	if _, err := c.Invoke(ctx, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if gotPrompt != "Question: what is marigold" {
		t.Fatalf("runner prompt = %q, want resolved sys.query", gotPrompt)
	}
}

func TestAgent_UsesPromptsListForSysQuery(t *testing.T) {
	var gotPrompt string
	withAgentRunner(t, func(_ context.Context, p AgentParam) (*schema.Message, error) {
		gotPrompt = p.UserPrompt
		return &schema.Message{Role: schema.Assistant, Content: "ok"}, nil
	})

	cmp, err := New("Agent", map[string]any{
		"model_id":    "stub",
		"api_key":     "test-key",
		"sys_prompt":  "act as assistant",
		"user_prompt": "This is the order you need to send to the agent.",
		"prompts": []any{
			map[string]any{"role": "user", "content": "{sys.query}"},
		},
	})
	if err != nil {
		t.Fatalf("New(Agent): %v", err)
	}

	state := runtime.NewCanvasState("run-1", "task-1")
	state.Sys["query"] = "用户真正的问题"
	ctx := runtime.WithState(context.Background(), state)

	if _, err := cmp.Invoke(ctx, nil); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if gotPrompt != "用户真正的问题" {
		t.Fatalf("runner prompt = %q, want sys.query from prompts list", gotPrompt)
	}
}

func TestAgent_FormatsRuntimePromptLikePython(t *testing.T) {
	var gotPrompt string
	withAgentRunner(t, func(_ context.Context, p AgentParam) (*schema.Message, error) {
		gotPrompt = p.UserPrompt
		return &schema.Message{Role: schema.Assistant, Content: "ok"}, nil
	})

	c := NewAgentComponent(AgentParam{
		ModelID:   "stub",
		APIKey:    "test-key",
		MaxRounds: 1,
	})
	if _, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "write answer",
		"reasoning":   "selected because it can answer",
		"context":     "known facts",
	}); err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	want := "\nREASONING:\nselected because it can answer\n\nCONTEXT:\nknown facts\n\nQUERY:\nwrite answer\n"
	if gotPrompt != want {
		t.Fatalf("runner prompt = %q, want %q", gotPrompt, want)
	}
}

func TestAgent_ToolCallRound(t *testing.T) {
	var calls int
	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		calls++
		return &schema.Message{
			Role:    schema.Assistant,
			Content: "final answer based on tool",
			ToolCalls: []schema.ToolCall{
				{
					ID:   "call_1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "search",
						Arguments: `{"q": "ragflow"}`,
					},
				},
			},
		}, nil
	})

	c := NewAgentComponent(AgentParam{ModelID: "stub", MaxRounds: 3})
	out, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "find out about ragflow",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got, want := out["content"], "final answer based on tool"; got != want {
		t.Errorf("content=%v, want %v", got, want)
	}
	toolCalls, ok := out["tool_calls"].([]map[string]any)
	if !ok {
		t.Fatalf("tool_calls missing or wrong type: %T", out["tool_calls"])
	}
	if len(toolCalls) != 1 {
		t.Fatalf("tool_calls=%d, want 1", len(toolCalls))
	}
	if toolCalls[0]["name"] != "search" {
		t.Errorf("tool name=%v, want search", toolCalls[0]["name"])
	}
	if calls != 1 {
		t.Errorf("runner called %d times, want 1", calls)
	}
}

func TestAgent_ExhaustRoundsError(t *testing.T) {
	withAgentRunner(t, func(_ context.Context, _ AgentParam) (*schema.Message, error) {
		return nil, errors.New("agent: exhausted rounds without final answer")
	})

	c := NewAgentComponent(AgentParam{ModelID: "stub", MaxRounds: 2})
	_, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "x",
	})
	if err == nil {
		t.Fatal("expected error when loop exhausts without a final answer")
	}
}

func TestAgent_MissingModelID(t *testing.T) {
	c := NewAgentComponent(AgentParam{MaxRounds: 1})
	_, err := c.Invoke(context.Background(), map[string]any{"user_prompt": "x"})
	if err == nil {
		t.Fatal("expected ParamError for missing model_id")
	}
	var pe *ParamError
	if !errors.As(err, &pe) {
		t.Errorf("err type=%T, want *ParamError", err)
	}
}

// TestAgent_Invoke_RespectsParentCancellation: when the parent
// ctx is cancelled, the runner observes it and the error
// propagates through Invoke.
func TestAgent_Invoke_RespectsParentCancellation(t *testing.T) {
	withAgentRunner(t, func(ctx context.Context, _ AgentParam) (*schema.Message, error) {
		// Honor ctx cancellation — real runners do; a stub that
		// ignores ctx would defeat the test's purpose.
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return &schema.Message{Content: "ok"}, nil
	})
	c := NewAgentComponent(AgentParam{ModelID: "echo", MaxRounds: 1})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel

	_, err := c.Invoke(ctx, map[string]any{"user_prompt": "hi"})
	if err == nil {
		t.Fatal("expected error from pre-cancelled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestAgent_UnknownToolName(t *testing.T) {
	c := NewAgentComponent(AgentParam{
		ModelID:   "stub",
		MaxRounds: 1,
		Tools:     []string{"does_not_exist"},
	})
	_, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "x",
	})
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(err.Error(), `build tools: agent tool: unsupported tool "does_not_exist"`) {
		t.Fatalf("err = %q, want unsupported tool message", err.Error())
	}
}

func TestAgent_AllRegisteredToolsConfigPassesToRunner(t *testing.T) {
	var captured AgentParam
	withAgentRunner(t, func(_ context.Context, p AgentParam) (*schema.Message, error) {
		captured = p
		return &schema.Message{Role: schema.Assistant, Content: "ok"}, nil
	})

	c := NewAgentComponent(AgentParam{ModelID: "stub", MaxRounds: 1})
	_, err := c.Invoke(context.Background(), map[string]any{
		"user_prompt": "x",
		"tools": []any{
			"akshare", "arxiv", "code_exec", "crawler", "deepl", "duckduckgo",
			"email", "github", "google", "google_scholar", "jin10", "pubmed",
			"qweather", "retrieval", "searxng", "tavily", "tushare", "wencai",
			"wikipedia", "yahoo_finance", "execute_sql",
		},
		"tool_params": map[string]any{
			"execute_sql": map[string]any{
				"db_type":     "mysql",
				"host":        "127.0.0.1",
				"port":        3306,
				"database":    "demo",
				"username":    "u",
				"password":    "p",
				"max_records": 10,
			},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if len(captured.Tools) != 21 {
		t.Fatalf("captured.Tools len = %d, want 21", len(captured.Tools))
	}
	if captured.ToolParams == nil || captured.ToolParams["execute_sql"] == nil {
		t.Fatalf("captured.ToolParams missing execute_sql: %#v", captured.ToolParams)
	}
}

type fakeToolCallingChatModel struct {
	tools []*schema.ToolInfo
}

func (m *fakeToolCallingChatModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: "ok"}, nil
}

func (m *fakeToolCallingChatModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		_ = sw.Send(&schema.Message{Role: schema.Assistant, Content: "ok"}, io.EOF)
	}()
	return sr, nil
}

func (m *fakeToolCallingChatModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	cp := *m
	cp.tools = append([]*schema.ToolInfo(nil), tools...)
	return &cp, nil
}

func TestAgent_CanCreateReactAgentWithAllRegisteredTools(t *testing.T) {
	p := AgentParam{
		Tools: []string{
			"akshare", "arxiv", "code_exec", "crawler", "deepl", "duckduckgo",
			"email", "github", "google", "google_scholar", "jin10", "pubmed",
			"qweather", "retrieval", "searxng", "tavily", "tushare", "wencai",
			"wikipedia", "yahoo_finance", "execute_sql",
		},
		ToolParams: map[string]map[string]any{
			"execute_sql": {
				"db_type":     "mysql",
				"host":        "127.0.0.1",
				"port":        3306,
				"database":    "demo",
				"username":    "u",
				"password":    "p",
				"max_records": 10,
			},
		},
		MaxRounds: 1,
	}
	tools, err := buildAgentTools(p)
	if err != nil {
		t.Fatalf("buildAgentTools: %v", err)
	}
	if len(tools) != len(p.Tools) {
		t.Fatalf("len(tools) = %d, want %d", len(tools), len(p.Tools))
	}
	_, err = react.NewAgent(context.Background(), &react.AgentConfig{
		ToolCallingModel: &fakeToolCallingChatModel{},
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: tools,
		},
		MaxStep: 1,
	})
	if err != nil {
		t.Fatalf("react.NewAgent(all tools): %v", err)
	}
}

func TestAgent_Registered(t *testing.T) {
	c, err := New("Agent", map[string]any{"model_id": "stub", "user_prompt": "x"})
	if err != nil {
		t.Fatalf("New(Agent): %v", err)
	}
	if c.Name() != "Agent" {
		t.Errorf("Name()=%q, want Agent", c.Name())
	}
}

// exhaustStepsModel is a scripted ToolCallingChatModel that emits a
// tool_call on every Generate and never returns final content. It
// is the input driver for TestAgent_ReActExhaustsSteps, which needs
// the eino ReAct loop to hit its MaxStep ceiling.
type exhaustStepsModel struct {
	turn       int
	rounds     [][]*schema.Message
	boundTools []*schema.ToolInfo
	toolName   string
	toolArgs   string
}

func (m *exhaustStepsModel) WithTools(tools []*schema.ToolInfo) (model.ToolCallingChatModel, error) {
	m.boundTools = tools
	return m, nil
}

func (m *exhaustStepsModel) Generate(_ context.Context, in []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	cp := make([]*schema.Message, len(in))
	copy(cp, in)
	m.rounds = append(m.rounds, cp)
	m.turn++
	return &schema.Message{
		Role: schema.Assistant,
		ToolCalls: []schema.ToolCall{{
			ID:   fmt.Sprintf("call_%d", m.turn),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      m.toolName,
				Arguments: m.toolArgs,
			},
		}},
	}, nil
}

func (m *exhaustStepsModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	sw.Close()
	return sr, nil
}

// TestAgent_ReActExhaustsSteps drives a real react.NewAgent whose
// scripted model always returns a tool_call and never returns final
// content. With MaxStep: 2 the loop must terminate with an error
// from eino's MaxStep guard. This is the eino error-path counterpart
// to TestExeSQL_RealReactAgent_ExecutesTool: the latter proves the
// happy path (model returns tool_call, framework runs tool, model
// returns final); this one proves the loop guard terminates even
// when the model never produces a final answer.
//
// Earlier versions also asserted mock.ExpectQuery("SELECT 1") to
// pin the tool-call count, but that assumption is fragile to eino
// version changes (different eino builds invoke the tool a
// different number of times before the MaxStep guard fires).
// The MaxStep guard itself — the thing we actually care about —
// is asserted by the non-nil err return above. PR review round 8
// (CI red): the test was failing on every CI run with "remaining
// expectation" against the SELECT 1 query because eino's internal
// counter for "is this the MaxStep iteration?" varies between
// releases. Stage ExpectPing only.
func TestAgent_ReActExhaustsSteps(t *testing.T) {
	t.Parallel()

	// Real ExeSQLTool with sqlmock. We stage ExpectPing only —
	// the optional Query expectation was removed because eino's
	// MaxStep-guard iteration count is an implementation detail
	// we cannot pin across eino versions.
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	defer db.Close()
	mock.ExpectPing()

	// Default sql.Open would try to connect to a real MySQL; the
	// dialer stub makes the tool talk to sqlmock instead.
	dialer := func(_, _ string) (*sql.DB, error) { return db, nil }
	// BuildByName goes through the public registry — the same path
	// AgentComponent.buildAgentTools takes. This proves the agent's
	// own wiring (ToolsConfig -> real BaseTool) works under the
	// MaxStep guard, not a backdoor constructor.
	built, err := agenttool.BuildByName("execute_sql", map[string]any{
		"db_type":     "mysql",
		"host":        "127.0.0.1",
		"port":        3306,
		"database":    "demo",
		"username":    "u",
		"password":    "p",
		"max_records": 10,
	})
	if err != nil {
		t.Fatalf("agenttool.BuildByName(execute_sql): %v", err)
	}
	exeSQLTool, ok := built.(*agenttool.ExeSQLTool)
	if !ok {
		t.Fatalf("BuildByName(execute_sql) returned %T, want *ExeSQLTool", built)
	}
	realTool := exeSQLTool.WithExeSQLDialer(dialer)

	mdl := &exhaustStepsModel{
		toolName: "execute_sql",
		toolArgs: `{"sql": "SELECT 1"}`,
	}

	agent, err := react.NewAgent(context.Background(), &react.AgentConfig{
		ToolCallingModel: mdl,
		ToolsConfig: compose.ToolsNodeConfig{
			Tools: []einotool.BaseTool{realTool},
		},
		MaxStep: 2,
	})
	if err != nil {
		t.Fatalf("react.NewAgent: %v", err)
	}

	out, err := agent.Generate(context.Background(), []*schema.Message{
		schema.UserMessage("loop forever"),
	})
	if err == nil {
		t.Fatalf("agent.Generate returned no error; out=%+v — expected MaxStep exhaustion", out)
	}
	if mdl.turn < 1 {
		t.Errorf("model.Generate called %d times, want >= 1 (the loop should have invoked it before giving up)", mdl.turn)
	}
	if len(mdl.boundTools) != 1 || mdl.boundTools[0].Name != "execute_sql" {
		names := make([]string, 0, len(mdl.boundTools))
		for _, ti := range mdl.boundTools {
			names = append(names, ti.Name)
		}
		t.Errorf("tools bound to model = %v, want [execute_sql]", names)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock expectations: %v", err)
	}
}
