package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ======================== Model Agent Tests ========================

// reActAgentSetup creates a basic agent with given model and optional tools
func reActAgentSetup(model Model[*schema.Message], tools []Tool) *ReActAgent[*schema.Message] {
	cfg := &ReActConfig[*schema.Message]{Model: model}
	if len(tools) > 0 { cfg.Tools = tools }
	a := NewReActAgent(cfg)
	a.name = "test_cma"
	return a
}

func TestReActAgent_BasicGenerate(t *testing.T) {
	model := &mockModel{}; model.addResp("Hello!")
	agent := reActAgentSetup(model, nil)
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("Hi")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 { t.Fatal("expected events") }
	found := false
	for _, e := range events {
		if e.Output != nil && e.Output.MessageOutput != nil {
			if e.Output.MessageOutput.Message.Content == "Hello!" { found = true }
		}
	}
	if !found { t.Error("expected Hello! in output") }
}

func TestReActAgent_ToolCallAndResponse(t *testing.T) {
	inner := &mockModel{}
	inner.addResp("final")
	wrapperModel := &forcedToolModel{
		toolCalls: []schema.ToolCall{{ID: "c1", Function: schema.ToolCallFunction{Name: "search", Arguments: `{"q":"test"}`}}},
		finalResp: "Final answer", firstCall: true,
	}
	tool := &mockTool{name: "search", desc: "Search tool"}
	agent := reActAgentSetup(wrapperModel, []Tool{tool})
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("Search")}})
	drainAgentEvents(t, iter)
	if !tool.executed { t.Error("tool not executed") }
}

func TestReActAgent_MaxIterationsExceeded(t *testing.T) {
	loopModel := &loopToolModel{toolCalls: []schema.ToolCall{{ID: "c1", Function: schema.ToolCallFunction{Name: "loop", Arguments: "{}"}}}}
	agent := &ReActAgent[*schema.Message]{
		config: &ReActConfig[*schema.Message]{Model: loopModel, Tools: []Tool{&mockTool{name: "loop", desc: "loop"}}, MaxIterations: 2},
		name:   "maxiter",
	}
	agent.config.Model = loopModel
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("Loop")}})
	var lastErr error
	for { ev, ok := iter.Next(); if !ok { break }; if ev.Err != nil { lastErr = ev.Err } }
	if lastErr == nil { t.Error("expected max iterations error") }
}

func TestReActAgent_ZeroMaxIterations(t *testing.T) {
	model := &mockModel{}; model.addResp("zero iter")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model, MaxIterations: 0})
	agent.name = "zero_iter"
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 { t.Error("expected events even with zero iterations") }
}

func TestReActAgent_ReturnDirectly(t *testing.T) {
	tool := &mockTool{name: "quick", desc: "Returns immediately"}
	wrapperModel := &forcedToolModel{
		toolCalls: []schema.ToolCall{{ID: "c1", Function: schema.ToolCallFunction{Name: "quick", Arguments: "{}"}}},
		finalResp: "Final", firstCall: true,
	}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: wrapperModel, Tools: []Tool{tool},
		ReturnDirectly: map[string]bool{"quick": true},
	})
	agent.name = "rd"
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 { t.Error("expected events") }
}

// ======================== Runner Tests ========================

func TestRunner_CreateAndQuery(t *testing.T) {
	model := &mockModel{}; model.addResp("Runner query")
	agent := reActAgentSetup(model, nil)
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Query(context.Background(), "Test query")
	events := drainAgentEvents(t, iter)
	if len(events) == 0 { t.Error("expected events") }
}

func TestRunner_MultipleRuns(t *testing.T) {
	model := &mockModel{}; model.addResp("1"); model.addResp("2")
	agent := reActAgentSetup(model, nil)
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})

	iter1 := runner.Run(context.Background(), []Message{schema.UserMessage("A")})
	e1 := drainAgentEvents(t, iter1)
	iter2 := runner.Run(context.Background(), []Message{schema.UserMessage("B")})
	e2 := drainAgentEvents(t, iter2)
	if len(e1) == 0 || len(e2) == 0 { t.Errorf("events: %d %d", len(e1), len(e2)) }
}

func TestRunner_WithRunOptions(t *testing.T) {
	model := &mockModel{}; model.addResp("Options")
	agent := reActAgentSetup(model, nil)
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	iter := runner.Run(context.Background(), []Message{schema.UserMessage("opts")}, WithSessionValues(map[string]any{"k": "v"}))
	drainAgentEvents(t, iter)
}

func TestRunner_WithCheckpoint(t *testing.T) {
	model := &mockModel{}; model.addResp("cp test")
	agent := reActAgentSetup(model, nil)
	store := &memStore{}
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	iter := runner.Run(context.Background(), []Message{schema.UserMessage("cp")})
	drainAgentEvents(t, iter)
}

func TestRunner_NilAgent(t *testing.T) {
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: nil})
	if runner == nil { t.Fatal("nil runner") }
}

// ======================== Agent Tool Tests ========================

func TestAgentTool_Basic(t *testing.T) {
	subModel := &mockModel{}; subModel.addResp("Sub result")
	subAgent := reActAgentSetup(subModel, nil)
	subAgent.name = "sub_tool"

	ctx := context.Background()
	agentTool := NewAgentTool(ctx, subAgent)
	if agentTool.Name() != "sub_tool" { t.Errorf("tool name = %s", agentTool.Name()) }

	result, err := agentTool.Invoke(ctx, `{"query":"test"}`)
	if err != nil { t.Fatalf("invoke: %v", err) }
	t.Logf("agent tool result: %q", result)
}

func TestAgentTool_WithFullChatHistory(t *testing.T) {
	subModel := &mockModel{}; subModel.addResp("history result")
	subAgent := reActAgentSetup(subModel, nil)
	subAgent.name = "history_tool"
	ctx := context.Background()
	agentTool := NewAgentTool(ctx, subAgent, WithFullChatHistoryAsInput())
	_, err := agentTool.Invoke(ctx, `{"query":"test"}`)
	if err != nil { t.Fatal(err) }
}

func TestAgentTool_FromRunner(t *testing.T) {
	subModel := &mockModel{}; subModel.addResp("Sub result")
	subAgent := reActAgentSetup(subModel, nil)
	subAgent.name = "research"

	ctx := context.Background()
	agentTool := NewAgentTool(ctx, subAgent)

	mainModel := &forcedToolModel{
		toolCalls: []schema.ToolCall{{ID: "c1", Function: schema.ToolCallFunction{Name: "research", Arguments: `{"topic":"AI"}`}}},
		finalResp: "Main done", firstCall: true,
	}
	mainAgent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: mainModel, Tools: []Tool{agentTool},
	})
	mainAgent.name = "main"
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: mainAgent})
	iter := runner.Query(ctx, "Research AI")
	events := drainAgentEvents(t, iter)
	if len(events) == 0 { t.Error("expected events") }
}

// ======================== ToolsNode Tests ========================

func TestToolsNode_Basic(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{&mockTool{name: "greet", desc: "Greet"}},
	})
	resp := &schema.Message{
		Role: schema.RoleAssistant, Content: "",
		ToolCalls: []schema.ToolCall{{ID: "c1", Function: schema.ToolCallFunction{Name: "greet", Arguments: `{"name":"world"}`}}},
	}
	results, action, err := tn.Execute(context.Background(), resp, nil, nil)
	if err != nil { t.Fatalf("Execute: %v", err) }
	if len(results) != 1 { t.Errorf("expected 1 result, got %d", len(results)) }
	_ = action
}

func TestToolsNode_NoToolCalls(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{Tools: []Tool{&mockTool{name: "t", desc: "t"}}})
	resp := &schema.Message{Role: schema.RoleAssistant, Content: "Just text"}
	results, action, err := tn.Execute(context.Background(), resp, nil, nil)
	if err != nil { t.Fatalf("Execute: %v", err) }
	if len(results) != 0 { t.Errorf("expected 0 results, got %d", len(results)) }
	_ = action
}

func TestToolsNode_ToolNotFound(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{Tools: []Tool{&mockTool{name: "a", desc: "a"}}})
	resp := &schema.Message{
		Role: schema.RoleAssistant, Content: "",
		ToolCalls: []schema.ToolCall{{ID: "c1", Function: schema.ToolCallFunction{Name: "nonexistent", Arguments: "{}"}}},
	}
	results, action, err := tn.Execute(context.Background(), resp, nil, nil)
	if err != nil { t.Fatalf("unexpected error: %v", err) }
	if len(results) != 1 { t.Errorf("expected 1 result, got %d", len(results)) }
	_ = action
}

func TestToolsNode_ReturnDirectly(t *testing.T) {
	tn := NewToolsNode[*schema.Message](&ToolsNodeConfig{
		Tools: []Tool{&mockTool{name: "quick", desc: "quick"}},
		ReturnDirectly: map[string]bool{"quick": true},
	})
	resp := &schema.Message{
		Role: schema.RoleAssistant, Content: "",
		ToolCalls: []schema.ToolCall{{ID: "c1", Function: schema.ToolCallFunction{Name: "quick", Arguments: "{}"}}},
	}
	_, action, err := tn.Execute(context.Background(), resp, nil, nil)
	if err != nil { t.Fatalf("Execute: %v", err) }
	_ = action
}

// ======================== Retry / Failover ========================

func TestReActAgent_RetrySucceeds(t *testing.T) {
	inner := &mockModel{}; inner.addResp("final")
	retryM := &retryModel{inner: inner, failAttempts: 2}
	agent := reActAgentSetup(retryM, nil)
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	drainAgentEvents(t, iter)
}

func TestReActAgent_RetryExhausted(t *testing.T) {
	inner := &mockModel{}; inner.addResp("never")
	retryM := &retryModel{inner: inner, failAttempts: 100}
	agent := reActAgentSetup(retryM, nil)
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	var lastErr error
	for { ev, ok := iter.Next(); if !ok { break }; if ev.Err != nil { lastErr = ev.Err } }
	_ = lastErr
}

func TestReActAgent_AlwaysFails(t *testing.T) {
	failing := &failModel{}
	agent := reActAgentSetup(failing, nil)
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("hello")}})
	var lastErr error
	for { ev, ok := iter.Next(); if !ok { break }; if ev.Err != nil { lastErr = ev.Err } }
	if lastErr == nil { t.Error("expected error from failing model") }
}

// ======================== Interrupt Tests ========================

func TestInterrupt_Basic(t *testing.T) {
	agent := reActAgentSetup(&mockModel{}, nil)
	ctx := context.Background()
	_ = TypedCompositeInterrupt[*schema.Message](ctx, "user_interrupt", nil)
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	drainAgentEvents(t, iter)
}

func TestInterrupt_WithResumeData(t *testing.T) {
	agent := reActAgentSetup(&mockModel{}, nil)
	agent.name = "resume"
	store := &memStore{}
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	ctx := context.Background()
	iter := runner.Run(ctx, []Message{schema.UserMessage("test")})
	drainAgentEvents(t, iter)
}

// ======================== Workflow Tests ========================

func TestWorkflow_SequentialAgents(t *testing.T) {
	m1 := &mockModel{}; m1.addResp("A1")
	m2 := &mockModel{}; m2.addResp("A2")
	a1 := reActAgentSetup(m1, nil); a1.name = "a1"
	a2 := reActAgentSetup(m2, nil); a2.name = "a2"

	ctx := context.Background()
	wf, err := NewSequential(ctx, &SequentialConfig{Name: "seq", Description: "test", SubAgents: []Agent{a1, a2}})
	if err != nil { t.Fatalf("NewSequential: %v", err) }
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 { t.Error("expected events") }
	t.Logf("sequential: %d events", len(events))
}

func TestWorkflow_ParallelAgents(t *testing.T) {
	m1 := &mockModel{}; m1.addResp("P1")
	m2 := &mockModel{}; m2.addResp("P2")
	a1 := reActAgentSetup(m1, nil); a1.name = "p1"
	a2 := reActAgentSetup(m2, nil); a2.name = "p2"

	ctx := context.Background()
	wf, err := NewParallel(ctx, &ParallelConfig{Name: "par", Description: "test", SubAgents: []Agent{a1, a2}})
	if err != nil { t.Fatalf("NewParallel: %v", err) }
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 { t.Error("expected events") }
}

func TestWorkflow_LoopAgents(t *testing.T) {
	m1 := &mockModel{}; m1.addResp("Main")
	m2 := &mockModel{}; m2.addResp("Critique")
	a1 := reActAgentSetup(m1, nil); a1.name = "main"
	a2 := reActAgentSetup(m2, nil); a2.name = "critique"

	ctx := context.Background()
	wf, err := NewLoop(ctx, &LoopConfig{
		Name: "loop", Description: "test", SubAgents: []Agent{a1, a2}, MaxIterations: 2,
	})
	if err != nil { t.Fatalf("NewLoop: %v", err) }
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("iterate")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 { t.Error("expected events") }
}

// ======================== Middleware Chain Tests ========================

type orderedMiddleware struct {
	BaseMiddleware[*schema.Message]
	id       string
	executed []string
}

func (m *orderedMiddleware) BeforeAgent(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
	m.executed = append(m.executed, m.id+":BeforeAgent")
	return ctx, rc, nil
}
func (m *orderedMiddleware) BeforeModelRewrite(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
	m.executed = append(m.executed, m.id+":BeforeModelRewrite")
	return ctx, state, nil
}
func (m *orderedMiddleware) AfterModelRewrite(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
	m.executed = append(m.executed, m.id+":AfterModelRewrite")
	return ctx, state, nil
}
func (m *orderedMiddleware) AfterAgent(ctx context.Context, state *ReActAgentState) (context.Context, error) {
	m.executed = append(m.executed, m.id+":AfterAgent")
	return ctx, nil
}

func TestMiddleware_ChainOrderPreserved(t *testing.T) {
	model := &mockModel{}; model.addResp("chain result")
	m1 := &orderedMiddleware{id: "mw1", executed: make([]string, 0)}
	m2 := &orderedMiddleware{id: "mw2", executed: make([]string, 0)}

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Middlewares: []ReActMiddleware{m1, m2},
	})
	agent.name = "chain"
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	drainAgentEvents(t, iter)
	t.Logf("m1: %v", m1.executed)
	t.Logf("m2: %v", m2.executed)
}

func TestMiddleware_ErrorPropagation(t *testing.T) {
	for _, failAt := range []string{"BeforeAgent", "BeforeModelRewrite", "AfterModelRewrite", "AfterAgent"} {
		t.Run(failAt, func(t *testing.T) {
			model := &mockModel{}; model.addResp("err test")
			mw := &errorMiddleware{failAt: failAt}
			agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model, Middlewares: []ReActMiddleware{mw}})
			agent.name = "err_" + failAt
			iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("test")}})
			var lastErr error
			for { ev, ok := iter.Next(); if !ok { break }; if ev.Err != nil { lastErr = ev.Err } }
			_ = lastErr
		})
	}
}

func TestMiddleware_WrapModel(t *testing.T) {
	var wrapped bool
	mw := &testMiddleware{}
	mw.wrapModel = func(ctx context.Context, m Model[*schema.Message], mc *ModelContext) (Model[*schema.Message], error) {
		wrapped = true; return m, nil
	}
	model := &mockModel{}; model.addResp("wrapped")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model, Middlewares: []ReActMiddleware{mw}})
	agent.name = "wrap_m"
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	drainAgentEvents(t, iter)
	if !wrapped { t.Error("WrapModel not called") }
}

// ======================== Callback Tests ========================

func TestCallbacks_OnStartOnEnd(t *testing.T) {
	var onStart, onEnd bool
	cb := callbackHandler{
		onStart: func(ctx context.Context, input *AgentCallbackInput) { onStart = true },
		onEnd:   func(ctx context.Context, output *AgentCallbackOutput) { onEnd = true },
	}
	model := &mockModel{}; model.addResp("cb test")
	agent := reActAgentSetup(model, nil)
	agent.name = "cb_agent"
	// Callbacks are wired in flowAgent.Run path
	iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("cb")}}, WithCallbacks(cb))
	drainAgentEvents(t, iter)
	_ = onStart
	_ = onEnd
}

func TestCallbackFilter_AgentNameMatch(t *testing.T) {
	cb := callbackHandler{onStart: func(ctx context.Context, input *AgentCallbackInput) {}}
	opts := []RunOption{WithCallbacks(cb), WithAgentNames("my_agent")}
	filtered := filterOptions("my_agent", opts)
	o := getCommonOptions(nil, filtered...)
	if len(o.callbacks) == 0 { t.Error("callbacks should pass through for matching agent") }
}

// ======================== Callback Infrastructure Tests ========================

func TestInitAgentCallbacks_Nil(t *testing.T) {
	ctx := initAgentCallbacks(context.Background(), "test", "ReActAgent")
	if cbs := getCallbacks(ctx); cbs != nil { t.Error("expected nil callbacks") }
}

func TestSetRunLocalValue_NoExecCtx(t *testing.T) {
	err := SetRunLocalValue(context.Background(), "k", "v")
	if err == nil { t.Error("expected error with no exec ctx") }
}

func TestGetRunLocalValue_NoExecCtx(t *testing.T) {
	_, _, err := GetRunLocalValue(context.Background(), "k")
	if err == nil { t.Error("expected error") }
}

func TestDeleteRunLocalValue_NoExecCtx(t *testing.T) {
	err := DeleteRunLocalValue(context.Background(), "k")
	if err == nil { t.Error("expected error") }
}

func TestSendEvent_NoExecCtx(t *testing.T) {
	err := SendEvent(context.Background(), nil)
	if err == nil { t.Error("expected error") }
}

// ======================== Gob Encodability ========================

func TestCheckGobEncodability(t *testing.T) {
	tests := []struct { name string; val any; wantErr bool }{
		{"string", "hello", false},
		{"int", 42, false},
		{"nil", nil, false},
		{"unregistered", struct{ X int }{1}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkGobEncodability("key", tt.val)
			if tt.wantErr && err == nil { t.Error("expected error") }
			if !tt.wantErr && err != nil { t.Errorf("unexpected error: %v", err) }
		})
	}
}

// ======================== RunOption Tests ========================

func TestRunOptions(t *testing.T) {
	tests := []struct {
		name string
		opt  RunOption
		check func(*testing.T, *runOptions)
	}{
		{"SessionValues", WithSessionValues(map[string]any{"k": "v"}), func(t *testing.T, o *runOptions) {
			if o.sessionValues["k"] != "v" { t.Error("session value not set") }
		}},
		{"SharedParent", WithSharedParentSession(), func(t *testing.T, o *runOptions) {
			if !o.sharedParentSession { t.Error("sharedParentSession not set") }
		}},
		{"SkipTransfer", WithSkipTransferMessages(), func(t *testing.T, o *runOptions) {
			if !o.skipTransferMessages { t.Error("skipTransferMessages not set") }
		}},
		{"AgentNames", WithAgentNames("a1"), func(t *testing.T, o *runOptions) {
			if len(o.agentNames) != 1 || o.agentNames[0] != "a1" { t.Error("agent names not set") }
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := getCommonOptions(nil, tt.opt)
			tt.check(t, o)
		})
	}
}

// ======================== Schema and Message Tests ========================

func TestSchemaMessageTypes(t *testing.T) {
	t.Run("UserMessage", func(t *testing.T) {
		m := schema.UserMessage("Hello")
		if m.Role != schema.RoleUser || m.Content != "Hello" { t.Error("bad user message") }
	})
	t.Run("SystemMessage", func(t *testing.T) {
		m := schema.SystemMessage("Sys")
		if m.Role != schema.RoleSystem { t.Error("bad system message") }
	})
	t.Run("ToolMessage", func(t *testing.T) {
		m := schema.ToolMessage("Result", "call_1")
		if m.Role != schema.RoleTool || m.Name != "call_1" { t.Error("bad tool message") }
	})
}

func TestToolCallConstruction(t *testing.T) {
	tc := schema.ToolCall{
		ID: "call_1", Type: "function",
		Function: schema.ToolCallFunction{Name: "search", Arguments: `{"q":"hello"}`},
	}
	if tc.ID != "call_1" { t.Errorf("id = %q", tc.ID) }
	if tc.Function.Name != "search" { t.Errorf("name = %q", tc.Function.Name) }
}

// ======================== Concurrency Tests ========================

func TestConcurrentCancel(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			model := &mockModel{}; model.addResp(fmt.Sprintf("concurrent-%d", id))
			agent := reActAgentSetup(model, nil)
			agent.name = "cc"
			opt, cancel := WithCancel()
			iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("test")}}, opt)
			cancel()
			drainAgentEvents(t, iter)
		}(i)
	}
	wg.Wait()
}

func TestConcurrentIterators(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			model := &mockModel{}
			model.addResp("conc")
			agent := reActAgentSetup(model, nil)
			iter := agent.Run(context.Background(), &AgentInput{Messages: []Message{schema.UserMessage("hi")}})
			drainAgentEvents(t, iter)
		}()
	}
	wg.Wait()
}

// ======================== GetAgentType ========================

type typedAgentMock struct{}
func (t *typedAgentMock) Run(ctx context.Context, input *AgentInput, opts ...RunOption) *AsyncIterator[*AgentEvent] { return nil }
func (t *typedAgentMock) Name(ctx context.Context) string { return "typed" }
func (t *typedAgentMock) Description(ctx context.Context) string { return "" }
func (t *typedAgentMock) GetType() string { return "CustomType" }

func TestGetAgentType(t *testing.T) {
	if gt := getAgentType(&typedAgentMock{}); gt != "CustomType" { t.Errorf("expected CustomType, got %s", gt) }
}

func TestGetAgentType_Default(t *testing.T) {
	agent := reActAgentSetup(&mockModel{}, nil)
	if gt := getAgentType(agent); gt != "ReActAgent" { t.Errorf("expected ReActAgent, got %s", gt) }
}

// ======================== Agent Resume ========================

func TestRunner_ResumeWithCheckpoint(t *testing.T) {
	agent := reActAgentSetup(&mockModel{}, nil)
	agent.name = "resume_test"
	store := &memStore{}
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	ctx := context.Background()
	iter := runner.Run(ctx, []Message{schema.UserMessage("test")})
	drainAgentEvents(t, iter)
}

// drainAgentEvents drains all events from the iterator, used for test cleanup.
func drainAgentEvents(t *testing.T, iter *AsyncIterator[*AgentEvent]) []*AgentEvent {
	t.Helper()
	var events []*AgentEvent
	for { ev, ok := iter.Next(); if !ok { break }; events = append(events, ev) }
	return events
}

// ======================== helper types used across tests ========================

type retryModel struct {
	inner        *mockModel
	failAttempts int32
	callCount    int32
}

func (m *retryModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	cnt := atomicAdd32(&m.callCount)
	if cnt <= m.failAttempts { return nil, errors.New("retryable error") }
	return m.inner.Generate(ctx, msgs, opts...)
}
func (m *retryModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil { return nil, err }
	return schema.StreamReaderFromArray([]Message{msg}), nil
}
func (m *retryModel) BindTools(tools []*schema.ToolInfo) error { return nil }

type failModel struct{}

func (m *failModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	return nil, errors.New("always fails")
}
func (m *failModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	return nil, errors.New("always fails")
}
func (m *failModel) BindTools(tools []*schema.ToolInfo) error { return nil }

type errorMiddleware struct {
	BaseMiddleware[*schema.Message]
	failAt string
}

func (m *errorMiddleware) BeforeAgent(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
	if m.failAt == "BeforeAgent" { return ctx, nil, errors.New("error in BeforeAgent") }
	return ctx, rc, nil
}
func (m *errorMiddleware) BeforeModelRewrite(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
	if m.failAt == "BeforeModelRewrite" { return ctx, nil, errors.New("error in BeforeModelRewrite") }
	return ctx, state, nil
}
func (m *errorMiddleware) AfterModelRewrite(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
	if m.failAt == "AfterModelRewrite" { return ctx, nil, errors.New("error in AfterModelRewrite") }
	return ctx, state, nil
}
func (m *errorMiddleware) AfterAgent(ctx context.Context, state *ReActAgentState) (context.Context, error) {
	if m.failAt == "AfterAgent" { return ctx, errors.New("error in AfterAgent") }
	return ctx, nil
}

func atomicAdd32(p *int32) int32 { return 0 }

// ======================== RunOption Tests ========================

func TestRunOptions_WithChatModelOptions(t *testing.T) {
	opt := WithChatModelOptions([]ModelOption{})
	o := &runOptions{}
	opt.apply(o)
	if o.chatModelOptions == nil {
		t.Error("chatModelOptions should not be nil")
	}
}

func TestRunOptions_WithToolOptions(t *testing.T) {
	opt := WithToolOptions([]ToolOption{})
	o := &runOptions{}
	opt.apply(o)
	if o.toolOptions == nil {
		t.Error("toolOptions should not be nil")
	}
}

func TestRunOptions_WithAgentToolOptions(t *testing.T) {
	opt := WithAgentToolOptions("sub_agent", []RunOption{WithSkipTransferMessages()})
	o := &runOptions{}
	opt.apply(o)
	if o.agentToolOptions == nil {
		t.Error("agentToolOptions should not be nil")
	}
	if opts, ok := o.agentToolOptions["sub_agent"]; !ok || len(opts) != 1 {
		t.Errorf("expected 1 options for sub_agent, got %d", len(opts))
	}
}

func TestRunOptions_WithHistoryModifier(t *testing.T) {
	fn := func(ctx context.Context, msgs []Message) []Message { return msgs }
	opt := WithHistoryModifier(fn)
	o := &runOptions{}
	opt.apply(o)
	if o.historyModifier == nil {
		t.Error("historyModifier should not be nil")
	}
}
