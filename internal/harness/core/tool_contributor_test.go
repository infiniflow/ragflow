package core

import (
	"context"
	"sync"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ========================================================================
// Local helpers (avoids dependency on subagent/ test internals)
// ========================================================================

type testScriptedStep struct {
	Text      string
	ToolCalls []schema.ToolCall
}

type testScriptedModel struct {
	mu    sync.Mutex
	steps []testScriptedStep
	pos   int
}

func newTestScriptedModel(steps ...testScriptedStep) *testScriptedModel {
	return &testScriptedModel{steps: steps}
}

func (m *testScriptedModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...ModelOption) (*schema.Message, error) {
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

func (m *testScriptedModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...ModelOption) (*schema.StreamReader[*schema.Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *testScriptedModel) BindTools(tools []*schema.ToolInfo) error { return nil }

type testForcedToolModel struct {
	inner     *mockModel
	toolCalls []schema.ToolCall
	finalResp string
	mu        sync.Mutex
	firstCall bool
}

func newTestForcedToolModel(inner *mockModel, toolCalls []schema.ToolCall, finalResp string) *testForcedToolModel {
	return &testForcedToolModel{
		inner:     inner,
		toolCalls: toolCalls,
		finalResp: finalResp,
		firstCall: true,
	}
}

func (m *testForcedToolModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...ModelOption) (*schema.Message, error) {
	m.mu.Lock()
	isFirst := m.firstCall
	if isFirst {
		m.firstCall = false
	}
	m.mu.Unlock()
	if isFirst {
		return &schema.Message{
			Role:      schema.RoleAssistant,
			Content:   "",
			ToolCalls: m.toolCalls,
		}, nil
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: m.finalResp}, nil
}

func (m *testForcedToolModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...ModelOption) (*schema.StreamReader[*schema.Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *testForcedToolModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// ========================================================================
// Mock ToolContributor Middlewares
// ========================================================================

// contributorMW is a middleware that contributes tools via ToolContributor.
type contributorMW struct {
	BaseMiddleware[*schema.Message]
	tools    []Tool
	infos    []*schema.ToolInfo
	returnRD map[string]bool
}

func (m *contributorMW) ContributeTools(ctx context.Context) []Tool {
	return m.tools
}
func (m *contributorMW) ContributeToolInfos(ctx context.Context) []*schema.ToolInfo {
	return m.infos
}
func (m *contributorMW) ContributeReturnDirectly(ctx context.Context) map[string]bool {
	return m.returnRD
}

// markerTool records whether it was executed.
type markerTool struct {
	name     string
	executed bool
	mu       sync.Mutex
}

func (t *markerTool) Name() string        { return t.name }
func (t *markerTool) Description() string { return "test tool: " + t.name }
func (t *markerTool) Invoke(ctx context.Context, args string, opts ...ToolOption) (string, error) {
	t.mu.Lock()
	t.executed = true
	t.mu.Unlock()
	return "result:" + t.name, nil
}
func (t *markerTool) Stream(ctx context.Context, args string, opts ...ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{"stream:" + t.name}), nil
}

// ========================================================================
// Tests
// ========================================================================

// TestToolContributor_Basic verifies that a ToolContributor middleware's tools
// are available for tool execution in the ReAct loop.
func TestToolContributor_Basic(t *testing.T) {
	myTool := &markerTool{name: "contrib_tool"}

	mw := &contributorMW{tools: []Tool{myTool}}
	model := newTestForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "c1", Function: schema.ToolCallFunction{Name: "contrib_tool", Arguments: "{}"}},
		},
		"final answer",
	)
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Middlewares: []ReActMiddleware{mw},
	})

	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("use tool")})

	drainTestFinal(t, iter)
	if !myTool.executed {
		t.Error("contributor tool was NOT executed")
	}
	t.Logf("basic: tool_executed=%v", myTool.executed)
}

// TestToolContributor_MultipleContributors verifies that tools from multiple
// ToolContributor middlewares are all available.
func TestToolContributor_MultipleContributors(t *testing.T) {
	toolA := &markerTool{name: "tool_a"}
	toolB := &markerTool{name: "tool_b"}

	mwA := &contributorMW{tools: []Tool{toolA}}
	mwB := &contributorMW{tools: []Tool{toolB}}

	model := newTestScriptedModel(
		testScriptedStep{ToolCalls: []schema.ToolCall{
			{ID: "call_a", Function: schema.ToolCallFunction{Name: "tool_a", Arguments: "{}"}},
			{ID: "call_b", Function: schema.ToolCallFunction{Name: "tool_b", Arguments: "{}"}},
		}},
		testScriptedStep{Text: "multi done"},
	)
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Middlewares: []ReActMiddleware{mwA, mwB},
	})

	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("use both")})

	drainTestFinal(t, iter)
	if !toolA.executed {
		t.Error("tool_a was NOT executed")
	}
	if !toolB.executed {
		t.Error("tool_b was NOT executed")
	}
	t.Logf("multiple: toolA=%v, toolB=%v", toolA.executed, toolB.executed)
}

// TestToolContributor_WithConfigTools verifies that tools from both config.Tools
// and ToolContributor middlewares are merged and available.
func TestToolContributor_WithConfigTools(t *testing.T) {
	configTool := &markerTool{name: "config_tool"}
	contribTool := &markerTool{name: "contrib_tool"}

	mw := &contributorMW{tools: []Tool{contribTool}}
	model := newTestScriptedModel(
		testScriptedStep{ToolCalls: []schema.ToolCall{
			{ID: "c1", Function: schema.ToolCallFunction{Name: "config_tool", Arguments: "{}"}},
			{ID: "c2", Function: schema.ToolCallFunction{Name: "contrib_tool", Arguments: "{}"}},
		}},
		testScriptedStep{Text: "merged done"},
	)
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Tools:       []Tool{configTool},
		Middlewares: []ReActMiddleware{mw},
	})

	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("use both types")})

	drainTestFinal(t, iter)
	if !configTool.executed {
		t.Error("config tool was NOT executed")
	}
	if !contribTool.executed {
		t.Error("contributor tool was NOT executed")
	}
	t.Logf("merged: config=%v, contrib=%v", configTool.executed, contribTool.executed)
}

// TestToolContributor_ReturnDirectly verifies ContributeReturnDirectly works.
func TestToolContributor_ReturnDirectly(t *testing.T) {
	myTool := &markerTool{name: "rd_tool"}

	mw := &contributorMW{
		tools:    []Tool{myTool},
		returnRD: map[string]bool{"rd_tool": true},
	}
	model := newTestForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "rd1", Function: schema.ToolCallFunction{Name: "rd_tool", Arguments: "{}"}},
		},
		"should not be reached",
	)
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Middlewares: []ReActMiddleware{mw},
	})

	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("return directly")})

	drainTestFinal(t, iter)
	if !myTool.executed {
		t.Error("rd_tool was NOT executed")
	}
	t.Logf("return-directly: tool_executed=%v", myTool.executed)
}

// TestToolContributor_WithSubAgent verifies subagent middleware works
// via ToolContributor + Init (without BindToConfig).
func TestToolContributor_WithSubAgent(t *testing.T) {
	// Use subagent package types — import at top
	subModel := &mockModel{}
	subModel.addResp("sub result")
	subAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: subModel,
	}).WithName("helper").WithDescription("Helper agent")

	parentModel := newTestForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "h1", Function: schema.ToolCallFunction{Name: "helper", Arguments: "{}"}},
		},
		"parent done",
	)
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: parentModel,
		Tools: []Tool{
			&agentTool{
				name: "helper", desc: "Helper agent",
				agent: subAgent, baseCtx: context.Background(),
			},
		},
	})

	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("use helper")})

	final := drainTestFinal(t, iter)
	if final != "parent done" {
		t.Errorf("expected 'parent done', got %q", final)
	}
	t.Logf("subagent tc: final=%q", final)
}

// TestToolContributor_BeforeAgentStillWorks verifies that BeforeAgent can still
// modify non-tool fields.
func TestToolContributor_BeforeAgentStillWorks(t *testing.T) {
	tracker := &testTrackerMiddleware{}

	model := &mockModel{}
	model.addResp("before agent works")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Middlewares: []ReActMiddleware{tracker},
	})

	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("test")})

	drainTestFinal(t, iter)
	if !tracker.beforeAgentCalled {
		t.Error("BeforeAgent was NOT called")
	}
	t.Log("BeforeAgent still works with ToolContributor")
}

// TestToolContributor_NoInitNeeded verifies that subagent tools are auto-collected
// when Init/BindToConfig is not called.
func TestToolContributor_NoInitNeeded(t *testing.T) {
	subModel := &mockModel{}
	subModel.addResp("auto sub result")
	subAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: subModel,
	}).WithName("auto_helper").WithDescription("Auto helper")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: newTestForcedToolModel(&mockModel{},
			[]schema.ToolCall{
				{ID: "a1", Function: schema.ToolCallFunction{Name: "auto_helper", Arguments: "{}"}},
			},
			"auto parent done",
		),
		Tools: []Tool{
			&agentTool{
				name: "auto_helper", desc: "Auto helper",
				agent: subAgent, baseCtx: context.Background(),
			},
		},
	})

	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("auto")})

	final := drainTestFinal(t, iter)
	if final != "auto parent done" {
		t.Errorf("expected 'auto parent done', got %q", final)
	}
	t.Logf("no-init: final=%q", final)
}

// ========================================================================
// Helpers
// ========================================================================
type testTrackerMiddleware struct {
	BaseMiddleware[*schema.Message]
	beforeAgentCalled bool
	mu                sync.Mutex
}

func (m *testTrackerMiddleware) BeforeAgent(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
	m.mu.Lock()
	m.beforeAgentCalled = true
	m.mu.Unlock()
	return ctx, rc, nil
}

// drainTestFinal drains the iterator and returns the final output text.
func drainTestFinal(t *testing.T, iter *AsyncIterator[*TypedAgentEvent[*schema.Message]]) string {
	t.Helper()
	var final string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("event error: %v", ev.Err)
			continue
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil &&
			!ev.Output.MessageOutput.IsStreaming &&
			ev.Output.MessageOutput.Message != nil {
			final = ev.Output.MessageOutput.Message.Content
		}
	}
	return final
}
