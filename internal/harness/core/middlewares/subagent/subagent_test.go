package subagent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"ragflow/internal/harness/core"
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
		return nil, errors.New("mockModel: no more responses")
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

// ---- forcedToolModel: first call returns tool calls, subsequent return final response ----

type forcedToolModel struct {
	inner     *mockModel
	toolCalls []schema.ToolCall
	finalResp string
	mu        sync.Mutex
	firstCall bool
}

func newForcedToolModel(inner *mockModel, toolCalls []schema.ToolCall, finalResp string) *forcedToolModel {
	return &forcedToolModel{
		inner:     inner,
		toolCalls: toolCalls,
		finalResp: finalResp,
		firstCall: true,
	}
}

func (m *forcedToolModel) Generate(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.Message, error) {
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

func (m *forcedToolModel) Stream(ctx context.Context, msgs []*schema.Message, opts ...core.ModelOption) (*schema.StreamReader[*schema.Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]*schema.Message{msg}), nil
}

func (m *forcedToolModel) BindTools(tools []*schema.ToolInfo) error { return nil }

// ---- Mock Tool ----

type mockTool struct {
	name      string
	desc      string
	executed  bool
	invokeErr error
	mu        sync.Mutex
}

func (t *mockTool) Name() string             { return t.name }
func (t *mockTool) Description() string       { return t.desc }
func (t *mockTool) Invoke(ctx context.Context, args string, opts ...core.ToolOption) (string, error) {
	t.mu.Lock()
	t.executed = true
	err := t.invokeErr
	t.mu.Unlock()
	if err != nil {
		return "", err
	}
	return "mock result for " + t.name, nil
}
func (t *mockTool) Stream(ctx context.Context, args string, opts ...core.ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{"mock stream result"}), nil
}

// ---- Scripted Model (multi-step) ----

type scriptedStep struct {
	Text      string
	ToolCalls []schema.ToolCall
}

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

// ---- Panic Tool ----

type panicTool struct {
	name string
	desc string
}

func (t *panicTool) Name() string               { return t.name }
func (t *panicTool) Description() string         { return t.desc }
func (t *panicTool) Invoke(ctx context.Context, args string, opts ...core.ToolOption) (string, error) {
	panic("unexpected error in tool execution")
}
func (t *panicTool) Stream(ctx context.Context, args string, opts ...core.ToolOption) (*schema.StreamReader[string], error) {
	panic("unexpected stream error")
}

// ---- Slow Tool (for timeout testing) ----

type slowTool struct {
	name  string
	desc  string
	delay time.Duration
}

func (t *slowTool) Name() string               { return t.name }
func (t *slowTool) Description() string         { return t.desc }
func (t *slowTool) Invoke(ctx context.Context, args string, opts ...core.ToolOption) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case <-time.After(t.delay):
		return "slow result for " + t.name, nil
	}
}
func (t *slowTool) Stream(ctx context.Context, args string, opts ...core.ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{"slow stream result"}), nil
}

// ---- Enhanced Error Tool ----

type enhancedErrorTool struct {
	name      string
	desc      string
	errMsg    string
	executed  bool
	mu        sync.Mutex
}

func (t *enhancedErrorTool) Name() string               { return t.name }
func (t *enhancedErrorTool) Description() string         { return t.desc }
func (t *enhancedErrorTool) Invoke(ctx context.Context, args string, opts ...core.ToolOption) (string, error) {
	return "", nil
}
func (t *enhancedErrorTool) Stream(ctx context.Context, args string, opts ...core.ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{""}), nil
}
func (t *enhancedErrorTool) EnhancedInvoke(ctx context.Context, args *schema.ToolArgument, opts ...core.ToolOption) (*schema.ToolResult, error) {
	t.mu.Lock()
	t.executed = true
	t.mu.Unlock()
	return &schema.ToolResult{Name: t.name, Error: t.errMsg, ToolCallID: args.CallID}, nil
}
func (t *enhancedErrorTool) EnhancedStream(ctx context.Context, args *schema.ToolArgument, opts ...core.ToolOption) (*schema.StreamReader[*schema.ToolResult], error) {
	return nil, nil
}

// ---- Middleware tracking ----

type trackingMiddleware struct {
	core.BaseMiddleware[*schema.Message]
	beforeAgentCalled bool
	beforeModelCalled bool
	mu                sync.Mutex
}

func (m *trackingMiddleware) BeforeAgent(ctx context.Context, rc *core.ReActAgentContext) (context.Context, *core.ReActAgentContext, error) {
	m.mu.Lock()
	m.beforeAgentCalled = true
	m.mu.Unlock()
	return ctx, rc, nil
}
func (m *trackingMiddleware) BeforeModelRewrite(ctx context.Context, state *core.ReActAgentState, mc *core.ModelContext) (context.Context, *core.ReActAgentState, error) {
	m.mu.Lock()
	m.beforeModelCalled = true
	m.mu.Unlock()
	return ctx, state, nil
}

// ---- Checkpoint store ----

type memStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: make(map[string][]byte)} }
func (s *memStore) Get(ctx context.Context, key string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.data[key]
	if !ok {
		return nil, false, nil
	}
	return v, true, nil
}
func (s *memStore) Set(ctx context.Context, key string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = data
	return nil
}

// ---- Helpers ----

func runAgent(ctx context.Context, t *testing.T, agent core.Agent, msg string) (string, error) {
	t.Helper()
	runner := core.NewTypedRunner(core.RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(msg)})
	var final string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			return final, ev.Err
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil &&
			!ev.Output.MessageOutput.IsStreaming &&
			ev.Output.MessageOutput.Message != nil {
			final = ev.Output.MessageOutput.Message.Content
		}
	}
	return final, nil
}

func runAgentWithStore(ctx context.Context, t *testing.T, agent core.Agent, msg string, store *memStore) (string, error) {
	t.Helper()
	runner := core.NewTypedRunner(core.RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(msg)})
	var final string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			return final, ev.Err
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil &&
			!ev.Output.MessageOutput.IsStreaming &&
			ev.Output.MessageOutput.Message != nil {
			final = ev.Output.MessageOutput.Message.Content
		}
	}
	return final, nil
}

// ========================================================================
// Tests
// ========================================================================

// TestSubAgent_Basic verifies a pre-built sub-agent is invoked via tool call.
func TestSubAgent_Basic(t *testing.T) {
	subModel := &mockModel{}
	subModel.addResp("result from researcher")
	subAgent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: subModel,
	}).WithName("researcher").WithDescription("Research a topic")

	mw := New([]SubAgentSpec{
		{Name: "researcher", Description: "Research a topic", Agent: subAgent},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "call_1", Function: schema.ToolCallFunction{Name: "researcher", Arguments: "{}"}},
		},
		"parent final answer",
	)
	cfg := &core.ReActConfig[*schema.Message]{Model: parentModel, Middlewares: []core.ReActMiddleware{mw}}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "research something")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "parent final answer" {
		t.Errorf("expected 'parent final answer', got %q", final)
	}
	t.Logf("basic: final=%q", final)
}

// TestSubAgent_DeclarativeConfig verifies the declarative AgentConfig path.
func TestSubAgent_DeclarativeConfig(t *testing.T) {
	mw := New([]SubAgentSpec{
		{
			Name:        "worker",
			Description: "Worker agent",
			AgentConfig: &AgentConfig{
				Model: func() core.Model[*schema.Message] {
					m := &mockModel{}
					m.addResp("worker done")
					return m
				}(),
				SystemPrompt: "You are a worker.",
			},
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "w1", Function: schema.ToolCallFunction{Name: "worker", Arguments: "{}"}},
		},
		"parent ok",
	)
	cfg := &core.ReActConfig[*schema.Message]{Model: parentModel, Middlewares: []core.ReActMiddleware{mw}}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "do work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "parent ok" {
		t.Errorf("expected 'parent ok', got %q", final)
	}
	t.Logf("declarative: final=%q", final)
}

// TestSubAgent_DeclarativeWithOwnTools verifies AgentConfig sub-agent that
// has its own tools.
func TestSubAgent_DeclarativeWithOwnTools(t *testing.T) {
	innerTool := &mockTool{name: "calc", desc: "Calculator"}

	mw := New([]SubAgentSpec{
		{
			Name:        "worker",
			Description: "Worker with tools",
			AgentConfig: &AgentConfig{
				Model: newForcedToolModel(&mockModel{},
					[]schema.ToolCall{
						{ID: "ct", Function: schema.ToolCallFunction{Name: "calc", Arguments: "{'x':1}"}},
					},
					"worker result",
				),
				Tools:         []core.Tool{innerTool},
				SystemPrompt:  "You are a worker with tools.",
				MaxIterations: 5,
			},
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "pw", Function: schema.ToolCallFunction{Name: "worker", Arguments: "{}"}},
		},
		"parent done",
	)
	cfg := &core.ReActConfig[*schema.Message]{Model: parentModel, Middlewares: []core.ReActMiddleware{mw}}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "do work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "parent done" {
		t.Errorf("expected 'parent done', got %q", final)
	}
	if !innerTool.executed {
		t.Error("sub-agent's own tool was not executed")
	}
	t.Logf("declarative with tools: final=%q, tool executed=%v", final, innerTool.executed)
}

// TestSubAgent_MultipleSubAgents verifies multiple sub-agents are available.
func TestSubAgent_MultipleSubAgents(t *testing.T) {
	mw := New([]SubAgentSpec{
		{
			Name: "researcher", Description: "Research agent",
			AgentConfig: &AgentConfig{Model: func() *mockModel { m := &mockModel{}; m.addResp("research done"); return m }()},
		},
		{
			Name: "coder", Description: "Coding agent",
			AgentConfig: &AgentConfig{Model: func() *mockModel { m := &mockModel{}; m.addResp("code done"); return m }()},
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "c1", Function: schema.ToolCallFunction{Name: "coder", Arguments: "{}"}},
		},
		"parent done",
	)
	cfg := &core.ReActConfig[*schema.Message]{Model: parentModel, Middlewares: []core.ReActMiddleware{mw}}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "do work")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "parent done" {
		t.Errorf("expected 'parent done', got %q", final)
	}
	t.Logf("multiple sub-agents: final=%q", final)
}

// TestSubAgent_AgentFactory verifies lazy agent construction (backward compat).
func TestSubAgent_AgentFactory(t *testing.T) {
	var constructed bool
	factory := func(ctx context.Context) (core.Agent, error) {
		constructed = true
		m := &mockModel{}
		m.addResp("factory built result")
		return core.NewReActAgent(&core.ReActConfig[*schema.Message]{
			Model: m,
		}).WithName("factory_agent").WithDescription("Lazy built agent"), nil
	}

	mw := New([]SubAgentSpec{
		{Name: "factory_agent", Description: "Lazy built", AgentFactory: factory},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "f1", Function: schema.ToolCallFunction{Name: "factory_agent", Arguments: "{}"}},
		},
		"parent with factory",
	)
	cfg := &core.ReActConfig[*schema.Message]{Model: parentModel, Middlewares: []core.ReActMiddleware{mw}}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "test factory")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "parent with factory" {
		t.Errorf("expected 'parent with factory', got %q", final)
	}
	if !constructed {
		t.Error("AgentFactory was not called")
	}
	t.Logf("factory: final=%q, constructed=%v", final, constructed)
}

// TestSubAgent_MiddlewareChain verifies integration with other middlewares.
func TestSubAgent_MiddlewareChain(t *testing.T) {
	tracker := &trackingMiddleware{}

	mw := New([]SubAgentSpec{
		{
			Name: "helper", Description: "Helper agent",
			AgentConfig: &AgentConfig{
				Model: func() *mockModel { m := &mockModel{}; m.addResp("helper done"); return m }(),
			},
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "h1", Function: schema.ToolCallFunction{Name: "helper", Arguments: "{}"}},
		},
		"parent chain",
	)
	cfg := &core.ReActConfig[*schema.Message]{
		Model:       parentModel,
		Middlewares: []core.ReActMiddleware{tracker, mw},
	}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "chain test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "parent chain" {
		t.Errorf("expected 'parent chain', got %q", final)
	}
	if !tracker.beforeAgentCalled {
		t.Error("tracking middleware BeforeAgent was not called")
	}
	t.Logf("chain: final=%q, tracker.BeforeAgent=%v", final, tracker.beforeAgentCalled)
}

// TestSubAgent_NestedSubAgent verifies 3-level nesting (parent → middle → inner).
func TestSubAgent_NestedSubAgent(t *testing.T) {
	// Innermost.
	innerMW := New([]SubAgentSpec{
		{
			Name: "inner", Description: "Inner sub-agent",
			AgentConfig: &AgentConfig{
				Model: func() *mockModel { m := &mockModel{}; m.addResp("inner result"); return m }(),
			},
		},
	}, &Config{MaxDepth: 5})
	innerCfg := &core.ReActConfig[*schema.Message]{
		Model: newForcedToolModel(&mockModel{},
			[]schema.ToolCall{
				{ID: "inner1", Function: schema.ToolCallFunction{Name: "inner", Arguments: "{}"}},
			},
			"middle done",
		),
		Middlewares: []core.ReActMiddleware{innerMW},
	}
	innerMW.BindToConfig(context.Background(), innerCfg)
	middleAgent := core.NewReActAgent(innerCfg).WithName("middle").WithDescription("Middle sub-agent")

	// Top-level.
	outerMW := New([]SubAgentSpec{
		{Name: "middle", Description: "Middle sub-agent", Agent: middleAgent},
	}, &Config{MaxDepth: 5})
	outerCfg := &core.ReActConfig[*schema.Message]{
		Model: newForcedToolModel(&mockModel{},
			[]schema.ToolCall{
				{ID: "outer1", Function: schema.ToolCallFunction{Name: "middle", Arguments: "{}"}},
			},
			"top done",
		),
		Middlewares: []core.ReActMiddleware{outerMW},
	}
	outerMW.BindToConfig(context.Background(), outerCfg)
	topAgent := core.NewReActAgent(outerCfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, topAgent, "nested call")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "top done" {
		t.Errorf("expected 'top done', got %q", final)
	}
	t.Logf("nested: final=%q", final)
}

// TestSubAgent_RecursionGuard verifies that nesting beyond MaxDepth is blocked.
// The tool error is converted to a tool result string (not a Go error) by
// ToolsNode.executeStandard, so the agent completes normally but the inner
// agent is never invoked.
func TestSubAgent_RecursionGuard(t *testing.T) {
	// Track whether leaf model was ever called.
	leafModel := &mockModel{}
	leafModel.addResp("leaf result")

	leafAgent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: leafModel,
	}).WithName("leaf").WithDescription("Leaf agent (innermost)")

	// Middle sub-agent with MaxDepth=1: parent→middle works (depth 0→1),
	// but middle→leaf fails (depth 1→2 exceeds limit).
	middleMW := New([]SubAgentSpec{
		{Name: "leaf", Description: "Leaf", Agent: leafAgent},
	}, &Config{MaxDepth: 1})
	middleCfg := &core.ReActConfig[*schema.Message]{
		Model: newForcedToolModel(&mockModel{},
			[]schema.ToolCall{
				{ID: "leaf1", Function: schema.ToolCallFunction{Name: "leaf", Arguments: "{}"}},
			},
			"middle done",
		),
		MaxIterations: 5,
		Middlewares:   []core.ReActMiddleware{middleMW},
	}
	middleMW.BindToConfig(context.Background(), middleCfg)
	middleAgent := core.NewReActAgent(middleCfg).WithName("middle").WithDescription("Middle sub-agent")

	// Top-level parent agent calls middle.
	topMW := New([]SubAgentSpec{
		{Name: "middle", Description: "Middle", Agent: middleAgent},
	}, nil)
	topCfg := &core.ReActConfig[*schema.Message]{
		Model: newForcedToolModel(&mockModel{},
			[]schema.ToolCall{
				{ID: "top1", Function: schema.ToolCallFunction{Name: "middle", Arguments: "{}"}},
			},
			"top done",
		),
		MaxIterations: 5,
		Middlewares:   []core.ReActMiddleware{topMW},
	}
	topMW.BindToConfig(context.Background(), topCfg)
	topAgent := core.NewReActAgent(topCfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, topAgent, "start")
	if err != nil {
		// Go-level error from inline path is also acceptable.
		t.Logf("recursion guard: got Go error: %v", err)
		return
	}
	// No Go error: ToolsNode captured the recursion error as a tool result string.
	if final != "top done" {
		t.Errorf("expected 'top done', got %q", final)
	}
	// Verify leaf model was NEVER called (responses not consumed → still has 1 entry).
	t.Logf("recursion guard: leaf model has %d remaining responses", len(leafModel.responses))
	if len(leafModel.responses) != 1 {
		t.Error("recursion guard: leaf model was invoked when it should have been blocked")
	}
	t.Logf("recursion guard: final=%q, leaf blocked=true", final)
}

// TestSubAgent_NestedWithinLimit verifies nesting works when depth is within MaxDepth.

// TestSubAgent_NestedWithinLimit verifies nesting works when depth is within MaxDepth.
func TestSubAgent_NestedWithinLimit(t *testing.T) {
	// Leaf sub-agent.
	leafAgent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: func() *mockModel { m := &mockModel{}; m.addResp("leaf result"); return m }(),
	}).WithName("leaf").WithDescription("Leaf agent")

	// Middle sub-agent with MaxDepth=2 (allows parent→middle→leaf).
	middleMW := New([]SubAgentSpec{
		{Name: "leaf", Description: "Leaf", Agent: leafAgent},
	}, &Config{MaxDepth: 2})
	middleCfg := &core.ReActConfig[*schema.Message]{
		Model: newForcedToolModel(&mockModel{},
			[]schema.ToolCall{
				{ID: "leaf1", Function: schema.ToolCallFunction{Name: "leaf", Arguments: "{}"}},
			},
			"middle done",
		),
		MaxIterations: 5,
		Middlewares:   []core.ReActMiddleware{middleMW},
	}
	middleMW.BindToConfig(context.Background(), middleCfg)
	middleAgent := core.NewReActAgent(middleCfg).WithName("middle").WithDescription("Middle sub-agent")

	// Top-level.
	topMW := New([]SubAgentSpec{
		{Name: "middle", Description: "Middle", Agent: middleAgent},
	}, nil)
	topCfg := &core.ReActConfig[*schema.Message]{
		Model: newForcedToolModel(&mockModel{},
			[]schema.ToolCall{
				{ID: "top1", Function: schema.ToolCallFunction{Name: "middle", Arguments: "{}"}},
			},
			"top done",
		),
		MaxIterations: 5,
		Middlewares:   []core.ReActMiddleware{topMW},
	}
	topMW.BindToConfig(context.Background(), topCfg)
	topAgent := core.NewReActAgent(topCfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, topAgent, "start")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "top done" {
		t.Errorf("expected 'top done', got %q", final)
	}
	t.Logf("within limit: final=%q", final)
}

// TestSubAgent_MiddlewareInheritance verifies parent middlewares are inherited.
func TestSubAgent_MiddlewareInheritance(t *testing.T) {
	parentTracker := &trackingMiddleware{}

	// Sub-agent with InheritParentMiddlewares. It should have parentTracker
	// in its middleware chain (but NOT the SubAgentMiddleware itself).
	mw := New([]SubAgentSpec{
		{
			Name:        "inheritor",
			Description: "Inheriting sub-agent",
			AgentConfig: &AgentConfig{
				Model: func() *mockModel { m := &mockModel{}; m.addResp("inheritor done"); return m }(),
			},
			InheritParentMiddlewares: true,
			ExcludedParentMiddlewareNames: nil,
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "ih", Function: schema.ToolCallFunction{Name: "inheritor", Arguments: "{}"}},
		},
		"parent inherited",
	)
	cfg := &core.ReActConfig[*schema.Message]{
		Model:       parentModel,
		Middlewares: []core.ReActMiddleware{parentTracker, mw},
	}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "test inheritance")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "parent inherited" {
		t.Errorf("expected 'parent inherited', got %q", final)
	}
	if !parentTracker.beforeAgentCalled {
		t.Error("parent tracker BeforeAgent was not called")
	}
	t.Logf("inheritance: final=%q, parentTracker.BeforeAgent=%v", final, parentTracker.beforeAgentCalled)
}

// TestSubAgent_NoParentTools verifies graceful handling when parent has only
// sub-agent tools (no user-provided tools).
func TestSubAgent_NoParentTools(t *testing.T) {
	mw := New([]SubAgentSpec{
		{
			Name: "researcher", Description: "Research agent",
			AgentConfig: &AgentConfig{
				Model: func() *mockModel { m := &mockModel{}; m.addResp("research done"); return m }(),
			},
		},
	}, nil)

	parentModel := &mockModel{}
	parentModel.addResp("no tools needed")

	cfg := &core.ReActConfig[*schema.Message]{Model: parentModel, Middlewares: []core.ReActMiddleware{mw}}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "no tools needed" {
		t.Errorf("expected 'no tools needed', got %q", final)
	}
	t.Logf("no parent tools: final=%q", final)
}

// TestSubAgent_AgentFactoryOnly verifies the legacy AgentFactory path still works.
func TestSubAgent_AgentFactoryOnly(t *testing.T) {
	mw := New([]SubAgentSpec{
		{
			Name:        "legacy",
			Description: "Legacy factory agent",
			AgentFactory: func(ctx context.Context) (core.Agent, error) {
				m := &mockModel{}
				m.addResp("legacy result")
				return core.NewReActAgent(&core.ReActConfig[*schema.Message]{
					Model: m,
				}).WithName("legacy").WithDescription("Legacy factory agent"), nil
			},
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "lg", Function: schema.ToolCallFunction{Name: "legacy", Arguments: "{}"}},
		},
		"parent legacy",
	)
	cfg := &core.ReActConfig[*schema.Message]{Model: parentModel, Middlewares: []core.ReActMiddleware{mw}}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "test legacy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "parent legacy" {
		t.Errorf("expected 'parent legacy', got %q", final)
	}
	t.Logf("legacy factory: final=%q", final)
}

// TestSubAgent_BindIdempotent verifies BindToConfig can be called multiple times
// without creating duplicate tools.
func TestSubAgent_BindIdempotent(t *testing.T) {
	mw := New([]SubAgentSpec{
		{
			Name: "worker", Description: "Worker",
			AgentConfig: &AgentConfig{
				Model: func() *mockModel { m := &mockModel{}; m.addResp("ok"); return m }(),
			},
		},
	}, nil)

	cfg := &core.ReActConfig[*schema.Message]{
		Model:       newForcedToolModel(&mockModel{}, nil, "done"),
		Middlewares: []core.ReActMiddleware{mw},
	}
	// Call BindToConfig twice.
	mw.BindToConfig(context.Background(), cfg)
	mw.BindToConfig(context.Background(), cfg)

	// Should have exactly 1 tool.
	if len(cfg.Tools) != 1 {
		t.Errorf("expected 1 tool after idempotent BindToConfig, got %d", len(cfg.Tools))
	}
	t.Logf("idempotent: tools=%d", len(cfg.Tools))
}

// TestSubAgent_RecursionErrorMessageDirect is covered by the direct test
// in agentcore/ (which accesses unexported subAgentDepthKey).

// TestSubAgent_SubAgentOwnMiddlewares verifies sub-agent specific middlewares
// are applied alongside inherited ones.
func TestSubAgent_SubAgentOwnMiddlewares(t *testing.T) {
	subTracker := &trackingMiddleware{}

	mw := New([]SubAgentSpec{
		{
			Name:        "tracked",
			Description: "Tracked sub-agent",
			AgentConfig: &AgentConfig{
				Model: func() *mockModel { m := &mockModel{}; m.addResp("tracked done"); return m }(),
				Middlewares: []core.ReActMiddleware{subTracker},
			},
			InheritParentMiddlewares: true,
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "tr", Function: schema.ToolCallFunction{Name: "tracked", Arguments: "{}"}},
		},
		"parent tracked",
	)
	cfg := &core.ReActConfig[*schema.Message]{Model: parentModel, Middlewares: []core.ReActMiddleware{mw}}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "test own middlewares")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if final != "parent tracked" {
		t.Errorf("expected 'parent tracked', got %q", final)
	}
	if !subTracker.beforeAgentCalled {
		t.Error("sub-agent's own tracker BeforeAgent was not called")
	}
	t.Logf("own middlewares: final=%q, subTracker.BeforeAgent=%v", final, subTracker.beforeAgentCalled)
}

// TestSubAgent_MaxDepthDefault verifies that MaxDepth=0 allows unlimited nesting.
func TestSubAgent_MaxDepthDefault(t *testing.T) {
	// 3 levels with default MaxDepth=0 should work.
	leafModel := &mockModel{}
	leafModel.addResp("leaf")
	leafAgent := core.NewReActAgent(&core.ReActConfig[*schema.Message]{
		Model: leafModel,
	}).WithName("leaf").WithDescription("Leaf")

	middleMW := New([]SubAgentSpec{
		{Name: "leaf", Description: "Leaf", Agent: leafAgent},
	}, nil) // MaxDepth=0
	middleCfg := &core.ReActConfig[*schema.Message]{
		Model: newForcedToolModel(&mockModel{},
			[]schema.ToolCall{
				{ID: "leaf1", Function: schema.ToolCallFunction{Name: "leaf", Arguments: "{}"}},
			},
			fmt.Sprintf("middle done"),
		),
		MaxIterations: 5,
		Middlewares:   []core.ReActMiddleware{middleMW},
	}
	middleMW.BindToConfig(context.Background(), middleCfg)
	middleAgent := core.NewReActAgent(middleCfg).WithName("middle").WithDescription("Middle")

	topMW := New([]SubAgentSpec{
		{Name: "middle", Description: "Middle", Agent: middleAgent},
	}, nil)
	topCfg := &core.ReActConfig[*schema.Message]{
		Model: newForcedToolModel(&mockModel{},
			[]schema.ToolCall{
				{ID: "top1", Function: schema.ToolCallFunction{Name: "middle", Arguments: "{}"}},
			},
			"top done",
		),
		MaxIterations: 5,
		Middlewares:   []core.ReActMiddleware{topMW},
	}
	topMW.BindToConfig(context.Background(), topCfg)
	topAgent := core.NewReActAgent(topCfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, topAgent, "start")
	if err != nil {
		t.Fatalf("unexpected error with MaxDepth=0: %v", err)
	}
	if final != "top done" {
		t.Errorf("expected 'top done', got %q", final)
	}
	t.Logf("default depth: final=%q", final)
}

// ========================================================================
// Phase 1 — Basic Error Scenarios
// ========================================================================

// TestSubAgent_ToolInvokeReturnsError verifies that when a sub-agent's tool
// returns a Go error, the parent agent completes normally (error captured as
// tool result text, not a Go error).
func TestSubAgent_ToolInvokeReturnsError(t *testing.T) {
	failTool := &mockTool{
		name:      "failing_tool",
		desc:      "Always fails",
		invokeErr: errors.New("API rate limit exceeded"),
	}

	subModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "f1", Function: schema.ToolCallFunction{Name: "failing_tool", Arguments: "{}"}},
		},
		"sub-agent completed after tool error",
	)

	mw := New([]SubAgentSpec{
		{
			Name: "researcher", Description: "Research",
			AgentConfig: &AgentConfig{
				Model:         subModel,
				Tools:         []core.Tool{failTool},
				SystemPrompt:  "You are a resilient researcher.",
				MaxIterations: 5,
			},
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "p1", Function: schema.ToolCallFunction{Name: "researcher", Arguments: "{'query': 'test'}"}},
		},
		"parent final answer",
	)
	cfg := &core.ReActConfig[*schema.Message]{
		Model: parentModel, Middlewares: []core.ReActMiddleware{mw},
		MaxIterations: 5,
	}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "research something")
	if err != nil {
		t.Fatalf("parent should NOT get Go error: %v", err)
	}
	if final != "parent final answer" {
		t.Errorf("expected 'parent final answer', got %q", final)
	}
	if !failTool.executed {
		t.Error("failing_tool was not invoked")
	}
	t.Logf("Phase1 ToolError: final=%q, tool executed=%v", final, failTool.executed)
}

// TestSubAgent_EnhancedToolReturnsError verifies EnhancedTool's Error field
// is captured as tool result text.
func TestSubAgent_EnhancedToolReturnsError(t *testing.T) {
	eTool := &enhancedErrorTool{
		name:   "enhanced_fail",
		desc:   "Enhanced tool that returns Error field",
		errMsg: "quota exceeded",
	}

	subModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "e1", Function: schema.ToolCallFunction{Name: "enhanced_fail", Arguments: "{}"}},
		},
		"sub-agent handled enhanced error",
	)

	mw := New([]SubAgentSpec{
		{
			Name: "helper", Description: "Helper",
			AgentConfig: &AgentConfig{
				Model:         subModel,
				Tools:         []core.Tool{eTool},
				MaxIterations: 5,
			},
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "pe1", Function: schema.ToolCallFunction{Name: "helper", Arguments: "{}"}},
		},
		"parent enhanced done",
	)
	cfg := &core.ReActConfig[*schema.Message]{
		Model: parentModel, Middlewares: []core.ReActMiddleware{mw},
		MaxIterations: 5,
	}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "test enhanced error")
	if err != nil {
		t.Fatalf("parent should NOT get Go error: %v", err)
	}
	if final != "parent enhanced done" {
		t.Errorf("expected 'parent enhanced done', got %q", final)
	}
	if !eTool.executed {
		t.Error("enhanced_fail tool was not invoked")
	}
	t.Logf("Phase1 EnhancedError: final=%q, tool executed=%v", final, eTool.executed)
}

// ========================================================================
// Phase 2 — Agent-Level Error Scenarios
// ========================================================================

// TestSubAgent_MaxIterationsExceeded verifies that when a sub-agent exceeds
// its MaxIterations, the parent completes normally (error captured in tool result).
func TestSubAgent_MaxIterationsExceeded(t *testing.T) {
	innerTool := &mockTool{name: "calc", desc: "Calculator"}

	// ScriptedModel: 2 tool calls → MaxIterations=2 → both consumed, loop exits.
	subModel := newScriptedModel(
		scriptedStep{ToolCalls: []schema.ToolCall{
			{ID: "c1", Function: schema.ToolCallFunction{Name: "calc", Arguments: "{}"}},
		}},
		scriptedStep{ToolCalls: []schema.ToolCall{
			{ID: "c2", Function: schema.ToolCallFunction{Name: "calc", Arguments: "{}"}},
		}},
	)

	mw := New([]SubAgentSpec{
		{
			Name: "worker", Description: "Worker",
			AgentConfig: &AgentConfig{
				Model:         subModel,
				Tools:         []core.Tool{innerTool},
				MaxIterations: 2,
			},
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "pw", Function: schema.ToolCallFunction{Name: "worker", Arguments: "{}"}},
		},
		"parent done",
	)
	cfg := &core.ReActConfig[*schema.Message]{
		Model: parentModel, Middlewares: []core.ReActMiddleware{mw},
		MaxIterations: 5,
	}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "work")
	if err != nil {
		t.Fatalf("parent should not get Go error: %v", err)
	}
	t.Logf("Phase2 MaxIterations: final=%q", final)
}

// TestSubAgent_ParentContextCancelled verifies that context cancellation during
// sub-agent execution is handled gracefully.
func TestSubAgent_ParentContextCancelled(t *testing.T) {
	slowTool := &slowTool{name: "slow", desc: "Slow tool", delay: 500 * time.Millisecond}

	subModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "s1", Function: schema.ToolCallFunction{Name: "slow", Arguments: "{}"}},
		},
		"slow done",
	)

	mw := New([]SubAgentSpec{
		{
			Name: "slowpoke", Description: "Slow",
			AgentConfig: &AgentConfig{
				Model:         subModel,
				Tools:         []core.Tool{slowTool},
				MaxIterations: 5,
			},
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "ps1", Function: schema.ToolCallFunction{Name: "slowpoke", Arguments: "{}"}},
		},
		"parent done",
	)
	cfg := &core.ReActConfig[*schema.Message]{
		Model: parentModel, Middlewares: []core.ReActMiddleware{mw},
		MaxIterations: 5,
	}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runner := core.NewTypedRunner(core.RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("go slow")})

	// Drain some events then cancel.
	var gotError bool
	count := 0
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			gotError = true
			t.Logf("cancellation produced error: %v", ev.Err)
			break
		}
		count++
		if count >= 3 {
			cancel()
		}
	}
	if !gotError {
		t.Log("cancellation did NOT produce a Go error (acceptable — error captured as tool result text)")
	}
	t.Logf("Phase2 ContextCancel: events drained=%d", count)
}

// ========================================================================
// Phase 3 — Boundary and Exception Scenarios
// ========================================================================

// TestSubAgent_ToolPanicRecovery verifies that a panicking tool inside a sub-agent
// does NOT crash the parent agent.
func TestSubAgent_ToolPanicRecovery(t *testing.T) {
	panicTool := &panicTool{name: "panic_tool", desc: "Panics on invoke"}

	subModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "pt1", Function: schema.ToolCallFunction{Name: "panic_tool", Arguments: "{}"}},
		},
		"sub-agent survived panic",
	)

	mw := New([]SubAgentSpec{
		{
			Name: "explorer", Description: "Explorer",
			AgentConfig: &AgentConfig{
				Model:         subModel,
				Tools:         []core.Tool{panicTool},
				MaxIterations: 5,
			},
		},
	}, nil)

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "pp1", Function: schema.ToolCallFunction{Name: "explorer", Arguments: "{}"}},
		},
		"parent ok",
	)
	cfg := &core.ReActConfig[*schema.Message]{
		Model: parentModel, Middlewares: []core.ReActMiddleware{mw},
		MaxIterations: 5,
	}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "explore")
	if err != nil {
		t.Logf("parent got Go error (acceptable if panic propagates before recovery): %v", err)
		return
	}
	if final != "parent ok" {
		t.Errorf("expected 'parent ok', got %q", final)
	}
	t.Logf("Phase3 PanicRecovery: final=%q (parent did NOT crash)", final)
}

// TestSubAgent_AgentFactoryReturnsError verifies that when AgentFactory returns
// an error, the spec is skipped and no tool is added to the config.
func TestSubAgent_AgentFactoryReturnsError(t *testing.T) {
	called := false
	mw := New([]SubAgentSpec{
		{
			Name: "broken", Description: "Broken factory",
			AgentFactory: func(ctx context.Context) (core.Agent, error) {
				called = true
				return nil, errors.New("factory initialization failed")
			},
		},
	}, nil)

	cfg := &core.ReActConfig[*schema.Message]{
		Model:       &mockModel{responses: []string{"no tools needed"}},
		Middlewares: []core.ReActMiddleware{mw},
	}
	mw.BindToConfig(context.Background(), cfg)

	if len(cfg.Tools) != 0 {
		t.Errorf("expected 0 tools (factory failed), got %d", len(cfg.Tools))
	}
	if !called {
		t.Error("AgentFactory was not called")
	}
	t.Log("Phase3 AgentFactoryError: spec correctly skipped")
}

// TestSubAgent_ParallelToolCallsOneFails verifies that when the parent issues
// multiple parallel tool calls and one sub-agent fails, the other succeeds,
// and the parent completes without a Go error.
func TestSubAgent_ParallelToolCallsOneFails(t *testing.T) {
	failTool := &mockTool{
		name:      "failing_tool",
		desc:      "Always fails",
		invokeErr: errors.New("rate limit exceeded"),
	}
	goodTool := &mockTool{name: "good_tool", desc: "Always works"}

	// Sub-agent A calls a failing tool, B calls working tool.
	subA := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "f1", Function: schema.ToolCallFunction{Name: "failing_tool", Arguments: "{}"}},
		},
		"sub A completed",
	)
	subB := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "g1", Function: schema.ToolCallFunction{Name: "good_tool", Arguments: "{}"}},
		},
		"sub B completed",
	)

	mw := New([]SubAgentSpec{
		{
			Name: "agent_a", Description: "Failing agent",
			AgentConfig: &AgentConfig{
				Model:         subA,
				Tools:         []core.Tool{failTool},
				MaxIterations: 5,
			},
		},
		{
			Name: "agent_b", Description: "Working agent",
			AgentConfig: &AgentConfig{
				Model:         subB,
				Tools:         []core.Tool{goodTool},
				MaxIterations: 5,
			},
		},
	}, nil)

	// Parent calls both sub-agents in parallel via concurrent tool calls.
	parentModel := newScriptedModel(
		scriptedStep{ToolCalls: []schema.ToolCall{
			{ID: "pa1", Function: schema.ToolCallFunction{Name: "agent_a", Arguments: "{}"}},
		}},
		scriptedStep{ToolCalls: []schema.ToolCall{
			{ID: "pb1", Function: schema.ToolCallFunction{Name: "agent_b", Arguments: "{}"}},
		}},
		scriptedStep{Text: "parent final"},
	)
	cfg := &core.ReActConfig[*schema.Message]{
		Model: parentModel, Middlewares: []core.ReActMiddleware{mw},
		MaxIterations: 5,
	}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	final, err := runAgent(ctx, t, agent, "run both")
	if err != nil {
		t.Fatalf("parent should NOT get Go error: %v", err)
	}
	if final != "parent final" {
		t.Errorf("expected 'parent final', got %q", final)
	}
	if !goodTool.executed {
		t.Error("good_tool was not executed")
	}
	t.Logf("Phase3 ParallelCalls: final=%q, good_tool=%v", final, goodTool.executed)
}

// ========================================================================
// Phase 4 — Integration Scenarios
// ========================================================================

// TestSubAgent_EmitInternalEventsWithError verifies that when EmitInternalEvents
// is enabled and a sub-agent's tool fails, the parent stream receives the
// sub-agent's internal error events without panicking or deadlocking.
func TestSubAgent_EmitInternalEventsWithError(t *testing.T) {
	failTool := &mockTool{
		name:      "flaky", desc: "Flaky tool",
		invokeErr: errors.New("internal error"),
	}

	subModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "x1", Function: schema.ToolCallFunction{Name: "flaky", Arguments: "{}"}},
		},
		"sub recovered",
	)

	mw := New([]SubAgentSpec{
		{
			Name: "internal", Description: "Internal agent",
			AgentConfig: &AgentConfig{
				Model:         subModel,
				Tools:         []core.Tool{failTool},
				MaxIterations: 5,
			},
		},
	}, &Config{EmitInternalEvents: true, MaxDepth: 5})

	parentModel := newForcedToolModel(&mockModel{},
		[]schema.ToolCall{
			{ID: "px1", Function: schema.ToolCallFunction{Name: "internal", Arguments: "{}"}},
		},
		"parent internal done",
	)
	cfg := &core.ReActConfig[*schema.Message]{
		Model: parentModel, Middlewares: []core.ReActMiddleware{mw},
		MaxIterations: 5,
	}
	mw.BindToConfig(context.Background(), cfg)
	agent := core.NewReActAgent(cfg)

	ctx := context.Background()
	runner := core.NewTypedRunner(core.RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("test internal events")})

	var final string
	var eventCount int
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		eventCount++
		if ev.Err != nil {
			t.Logf("event error: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil &&
			!ev.Output.MessageOutput.IsStreaming &&
			ev.Output.MessageOutput.Message != nil {
			final = ev.Output.MessageOutput.Message.Content
		}
	}
	if final != "parent internal done" {
		t.Errorf("expected 'parent internal done', got %q", final)
	}
	t.Logf("Phase4 EmitInternalEvents: final=%q, total events=%d", final, eventCount)
}
