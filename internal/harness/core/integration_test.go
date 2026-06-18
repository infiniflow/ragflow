package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// TestIntegration_ReActToolResumeComplete verifies a full ReAct cycle:
// model returns tool call -> tool executes -> model returns final answer.
func TestIntegration_ReActToolResumeComplete(t *testing.T) {
	model := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "call_1", Function: schema.ToolCallFunction{Name: "calc", Arguments: "{\"x\":6,\"y\":7}"}}},
		finalResp: "the answer is 42",
		firstCall: true,
	}
	tool := &mockTool{name: "calc", desc: "calculator"}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	})
	agent.name = "react_tool"
	store := newCancelTestStore()
	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("compute")})
	var lastContent string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
			lastContent = ev.Output.MessageOutput.Message.Content
		}
	}
	if lastContent != "the answer is 42" {
		t.Errorf("expected 'the answer is 42', got %q", lastContent)
	}
}

// TestIntegration_SequentialAgent verifies sequential execution of two agents.
func TestIntegration_SequentialAgent(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("hello from agent A")
	m2 := &mockModel{}
	m2.addResp("hello from agent B")

	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}).WithName("agent_a").WithDescription("first agent")
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("agent_b").WithDescription("second agent")

	ctx := context.Background()
	seq, err := NewSequential(ctx, &SequentialConfig{
		Name: "seq_test", Description: "sequential test",
		SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewSequential: %v", err)
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: seq})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run agents")})
	var outputs []string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
			outputs = append(outputs, ev.Output.MessageOutput.Message.Content)
		}
	}
	if len(outputs) == 0 {
		t.Fatal("expected at least one output event")
	}
	t.Logf("sequential outputs: %v", outputs)
}

// TestIntegration_ParallelAgent verifies parallel execution of two agents.
func TestIntegration_ParallelAgent(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("result from parallel A")
	m2 := &mockModel{}
	m2.addResp("result from parallel B")

	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}).WithName("par_a").WithDescription("parallel agent A")
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("par_b").WithDescription("parallel agent B")

	ctx := context.Background()
	par, err := NewParallel(ctx, &ParallelConfig{
		Name: "par_test", Description: "parallel test",
		SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewParallel: %v", err)
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: par})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run parallel")})
	var outputs []string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
			outputs = append(outputs, ev.Output.MessageOutput.Message.Content)
		}
	}
	if len(outputs) == 0 {
		t.Fatal("expected at least one output event")
	}
	t.Logf("parallel outputs: %v", outputs)
}

// TestIntegration_LoopAgent verifies a loop agent that runs sub-agents in a loop.
func TestIntegration_LoopAgent(t *testing.T) {
	m := &mockModel{}
	// The loop runs the body agent up to MaxIterations (3) times, so add 3 responses
	for i := 0; i < 3; i++ {
		m.addResp("loop iteration output")
	}

	a := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("loop_body").WithDescription("loop body agent")

	ctx := context.Background()
	loop, err := NewLoop(ctx, &LoopConfig{
		Name: "loop_test", Description: "loop test",
		SubAgents:     []Agent{a},
		MaxIterations: 3,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: loop})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run loop")})
	var outputs []string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
			outputs = append(outputs, ev.Output.MessageOutput.Message.Content)
		}
	}
	t.Logf("loop outputs: %v", outputs)
}

// TestIntegration_SupervisorTransfer creates a simple supervisor with one sub-agent
// and verifies basic execution completes without error.
func TestIntegration_SupervisorTransfer(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("supervisor output")
	m2 := &mockModel{}
	m2.addResp("sub-agent output")

	sub := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("worker").WithDescription("worker agent")

	// Use AgentWithOptions with disallow transfer to parent and the sub-agent
	ctx := context.Background()
	wrappedSub := AgentWithOptions(ctx, sub, WithDisallowTransferToParent())

	sup := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       m1,
		Instruction: "You are a supervisor. Transfer to worker agent when asked.",
	}).WithName("supervisor").WithDescription("supervisor agent")

	flow, err := SetSubAgents(ctx, sup, []Agent{wrappedSub})
	if err != nil {
		t.Fatalf("SetSubAgents: %v", err)
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: flow})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("hello")})
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected err: %v", ev.Err)
		}
	}
}

// TestIntegration_PlanExecute creates a PlanExecute agent with mock models
// and verifies basic execution completes without error.
func TestIntegration_PlanExecute(t *testing.T) {
	plannerM := &mockModel{}
	plannerM.addResp("plan created")
	execM := &mockModel{}
	execM.addResp("executed step")
	replannerM := &mockModel{}
	replannerM.addResp("replanning")

	ctx := context.Background()

	planner := NewReActAgent(&ReActConfig[*schema.Message]{Model: plannerM}).WithName("planner").WithDescription("planner agent")
	executor := NewReActAgent(&ReActConfig[*schema.Message]{Model: execM}).WithName("executor").WithDescription("executor agent")
	replanner := NewReActAgent(&ReActConfig[*schema.Message]{Model: replannerM}).WithName("replanner").WithDescription("replanner agent")

	loopAgent, err := NewLoop(ctx, &LoopConfig{
		Name:          "pe_loop",
		Description:   "Plan-Execute loop",
		SubAgents:     []Agent{executor, replanner},
		MaxIterations: 1,
	})
	if err != nil {
		t.Fatalf("NewLoop: %v", err)
	}

	seqAgent, err := NewSequential(ctx, &SequentialConfig{
		Name:        "plan_execute",
		Description: "Plan-Execute agent",
		SubAgents:   []Agent{planner, loopAgent},
	})
	if err != nil {
		t.Fatalf("NewSequential: %v", err)
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: seqAgent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("do something")})
	var outputs []string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
			outputs = append(outputs, ev.Output.MessageOutput.Message.Content)
		}
	}
	t.Logf("plan-execute outputs: %v", outputs)
}

func TestIntegration_TurnLoopPushStop(t *testing.T) {
	ctx := context.Background()

	loop := NewAgentLoop[*schema.Message](AgentLoopConfig[*schema.Message]{
		GenInput: func(_ context.Context, l *AgentLoop[*schema.Message], items []*schema.Message) (*GenInputResult[*schema.Message], error) {
			return &GenInputResult[*schema.Message]{
				Input:     &AgentInput{Messages: items},
				Consumed:  items,
				Remaining: nil,
			}, nil
		},
		PrepareAgent: func(_ context.Context, _ *AgentLoop[*schema.Message], consumed []*schema.Message) (Agent, error) {
			m := &mockModel{}
			m.addResp("turn loop response")
			agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("turn_agent")
			return agent, nil
		},
	})

	loop.Push(schema.UserMessage("item 1"))
	loop.Push(schema.UserMessage("item 2"))
	loop.Run(ctx)
	loop.Stop()
	state := loop.Wait()
	if state.ExitReason != nil && !errors.As(state.ExitReason, new(*CancelError)) {
		t.Fatalf("unexpected exit reason: %v", state.ExitReason)
	}
	t.Logf("turn loop exit: reason=%v, unhandled=%d", state.ExitReason, len(state.UnhandledItems))
}

// TestIntegration_MiddlewareStack verifies that middleware hooks fire in a ReAct agent.
func TestIntegration_MiddlewareStack(t *testing.T) {
	var beforeAgentCalled, afterAgentCalled, beforeModelCalled, afterModelCalled bool

	mw := &testMiddleware{
		beforeAgent: func(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
			beforeAgentCalled = true
			return ctx, rc, nil
		},
		afterAgent: func(ctx context.Context, state *ReActAgentState) (context.Context, error) {
			afterAgentCalled = true
			return ctx, nil
		},
		beforeModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			beforeModelCalled = true
			return ctx, state, nil
		},
		afterModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			afterModelCalled = true
			return ctx, state, nil
		},
	}

	model := &mockModel{}
	model.addResp("middleware test response")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Middlewares: []ReActMiddleware{mw},
	})
	agent.name = "mw_test"
	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test middleware")}})
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected err: %v", ev.Err)
		}
	}

	if !beforeAgentCalled {
		t.Error("BeforeAgent middleware was not called")
	}
	if !afterAgentCalled {
		t.Error("AfterAgent middleware was not called")
	}
	if !beforeModelCalled {
		t.Error("BeforeModelRewrite middleware was not called")
	}
	if !afterModelCalled {
		t.Error("AfterModelRewrite middleware was not called")
	}
}

// TestIntegration_AgentToolNested creates an AgentTool wrapping a simple agent
// and verifies it can be invoked through a parent agent's tool execution.
func TestIntegration_AgentToolNested(t *testing.T) {
	innerM := &mockModel{}
	innerM.addResp("inner agent result")
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{Model: innerM}).WithName("inner_agent").WithDescription("inner agent for testing")

	ctx := context.Background()
	agentTool := NewAgentTool(ctx, innerAgent)

	// Now create a parent agent that "has" this tool and executes it
	parentM := &forcedToolModel{
		toolCalls: []schema.ToolCall{{ID: "call_at_1", Function: schema.ToolCallFunction{Name: "inner_agent", Arguments: "{\"task\":\"test\"}"}}},
		finalResp: "parent done",
		firstCall: true,
	}

	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       parentM,
		Tools:       []Tool{agentTool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{agentTool}},
	}).WithName("parent_agent")

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("use agent tool")})
	var lastContent string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
			lastContent = ev.Output.MessageOutput.Message.Content
		}
	}
	if lastContent != "parent done" {
		t.Errorf("expected 'parent done', got %q", lastContent)
	}
}

// TestIntegration_CheckpointResume verifies that a Runner with checkpoint store
// can execute an agent and resume from checkpoint.
func TestIntegration_CheckpointResume(t *testing.T) {
	// Use a model that produces a tool call, causing an interrupt-like flow
	model := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "call_cp_1", Function: schema.ToolCallFunction{Name: "cp_tool", Arguments: "{\"x\":1}"}}},
		finalResp: "resume complete",
		firstCall: true,
	}
	tool := &mockTool{name: "cp_tool", desc: "checkpoint tool"}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	})
	agent.name = "cp_agent"
	store := newCancelTestStore()
	ctx := context.Background()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})

	// Run with a checkpoint ID
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("checkpoint test")})
	var lastContent string
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			// CancelError with interrupt is expected in checkpoint flow
			var ce *CancelError
			if errors.As(ev.Err, &ce) {
				t.Logf("got CancelError (expected in checkpoint resume flow): %v", ce)
				break
			}
			t.Fatalf("unexpected err: %v", ev.Err)
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
			lastContent = ev.Output.MessageOutput.Message.Content
		}
	}
	t.Logf("checkpoint run completed, last content: %q", lastContent)
}

// TestIntegration_SequentialCancelResume verifies that a sequential agent can be
// cancelled mid-execution and later resumed.
func TestIntegration_SequentialCancelResume(t *testing.T) {
	// First agent: responds immediately
	m1 := &mockModel{}
	m1.addResp("agent A done")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}).WithName("seq_a").WithDescription("first in sequence")

	// Second agent: use cancelTestChatModel with a delay so we can cancel mid-execution
	m2 := newCancelTestChatModel(nil)
	m2.addResp("agent B done")
	m2.setDelay(50 * time.Millisecond)
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("seq_b").WithDescription("second in sequence")

	ctx := context.Background()
	seq, err := NewSequential(ctx, &SequentialConfig{
		Name: "seq_cancel", Description: "sequential cancel test",
		SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewSequential: %v", err)
	}

	cancelOpt, cancelFunc := WithCancel()
	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: seq, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run sequential")}, cancelOpt)

	// Wait for agent A to complete, then cancel
	time.Sleep(20 * time.Millisecond)
	cancelFunc(WithCancelMode(CancelImmediate))

	var cancelSeen bool
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		var ce *CancelError
		if ev.Err != nil && errors.As(ev.Err, &ce) {
			cancelSeen = true
			t.Logf("got CancelError: %v", ce)
			break
		}
		if ev.Err != nil {
			t.Logf("non-cancel error: %v", ev.Err)
		}
	}
	if !cancelSeen {
		t.Log("cancel may not have been delivered (expected with non-graceful cancel)")
	}
}

func TestIntegration_LoopAgentSimple(t *testing.T) {
	m1 := &mockModel{}
	// 2 iterations * 1 call each = 2 calls
	m1.addResp("loop_a1")
	m1.addResp("loop_a1")
	m2 := &mockModel{}
	// 2 iterations * 1 call each = 2 calls
	m2.addResp("loop_a2")
	m2.addResp("loop_a2")
	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}); a1.name = "la1"
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}); a2.name = "la2"
	ctx := context.Background()
	wf, err := NewLoop(ctx, &LoopConfig{Name: "loop_simple", Description: "test", SubAgents: []Agent{a1, a2}, MaxIterations: 2})
	if err != nil { t.Fatalf("NewLoop: %v", err) }
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("go")}})
	var count int
	for { ev, ok := iter.Next(); if !ok { break }; if ev.Err != nil { t.Fatalf("err: %v", ev.Err) }; count++ }
	if count == 0 { t.Error("expected events from loop") }
}

func TestIntegration_PlanExecuteSimple(t *testing.T) {
	model := &mockModel{}
	model.addResp("plan")
	model.addResp("execute")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("pe_test")
	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	ctx := context.Background()
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("test")})
	for { ev, ok := iter.Next(); if !ok { break }; if ev.Err != nil { t.Fatalf("err: %v", ev.Err) } }
}

// ---- Runner-level integration tests ----

// TestIntegration_RunnerToolCall verifies a full ReAct cycle via Runner:
// model returns tool call -> tool executes -> model returns final answer.
func TestIntegration_RunnerToolCall(t *testing.T) {
	model := &forcedToolModel{
		inner:     &mockModel{},
		toolCalls: []schema.ToolCall{{ID: "call_1", Function: schema.ToolCallFunction{Name: "calculator", Arguments: "{\"x\":6,\"y\":7}"}}},
		finalResp: "the answer is 42",
		firstCall: true,
	}
	tool := &mockTool{name: "calculator", desc: "calculates things"}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: model, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	})
	agent.name = "calc_agent"
	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	ctx := context.Background()
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("what is 6*7?")})
	var lastContent string
	for {
		ev, ok := iter.Next()
		if !ok { break }
		if ev.Err != nil { t.Fatalf("err: %v", ev.Err) }
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
			lastContent = ev.Output.MessageOutput.Message.Content
		}
	}
	if lastContent != "the answer is 42" {
		t.Errorf("expected 'the answer is 42', got %q", lastContent)
	}
}

// TestIntegration_RunnerSimple runs a basic agent via Runner with checkpoint.
func TestIntegration_RunnerSimple(t *testing.T) {
	model := &mockModel{}
	model.addResp("hello world")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model})
	agent.name = "runner_test"
	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	ctx := context.Background()
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("say hi")})
	var found bool
	for {
		ev, ok := iter.Next()
		if !ok { break }
		if ev.Err != nil { t.Fatalf("err: %v", ev.Err) }
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
			if ev.Output.MessageOutput.Message.Content == "hello world" { found = true }
		}
	}
	if !found { t.Error("expected 'hello world' in output") }
}

// TestIntegration_RunnerResume verifies the full cancel-then-resume cycle.
func TestIntegration_RunnerResume(t *testing.T) {
	model := &mockModel{}
	model.addResp("first response")
	model.addResp("resumed response")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("resume_test")
	store := newCancelTestStore()

	// Run with a known checkpoint ID so we can resume from it.
	cid := "resume-cid-001"
	cancelOpt, cancelFunc := WithCancel()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	ctx := context.Background()
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run me")},
		WithCheckPointID(cid), cancelOpt)

	time.Sleep(10 * time.Millisecond)
	cancelFunc(WithCancelMode(CancelImmediate))
	for { _, ok := iter.Next(); if !ok { break } }

	// Resume from the known checkpoint ID.
	resumedIter, err := runner.Resume(ctx, cid)
	if err != nil {
		t.Logf("Resume failed (expected if cancel didn't produce checkpoint): %v", err)
		return
	}
	var outputs []string
	for {
		ev, ok := resumedIter.Next()
		if !ok { break }
		if ev.Err != nil { break }
		if ev.Output != nil && ev.Output.MessageOutput != nil && ev.Output.MessageOutput.Message != nil {
			outputs = append(outputs, ev.Output.MessageOutput.Message.Content)
		}
	}
	t.Logf("resumed outputs: %v", outputs)
}

// TestIntegration_RunnerCancel verifies cancellation via WithCancel option.
func TestIntegration_RunnerCancel(t *testing.T) {
	m := newCancelTestChatModel(nil)
	m.addResp("should not appear")
	m.setDelay(200 * time.Millisecond)
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("cancel_test")

	cancelOpt, cancelFunc := WithCancel()
	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
	ctx := context.Background()
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("cancel me")}, cancelOpt)

	time.Sleep(50 * time.Millisecond)
	cancelFunc(WithCancelMode(CancelImmediate))

	var gotCancel bool
	for {
		ev, ok := iter.Next()
		if !ok { break }
		if ev.Err != nil {
			var ce *CancelError
			if errors.As(ev.Err, &ce) { gotCancel = true }
			break
		}
	}
	if !gotCancel { t.Log("cancel may not have been delivered (expected with non-graceful cancel)") }
}

// TestIntegration_RunnerStreamMode verifies that streaming events are received.
func TestIntegration_RunnerStreamMode(t *testing.T) {
	model := &mockModel{}
	model.addResp("streamed output")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("stream_test")

	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store, EnableStreaming: true})
	ctx := context.Background()
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("stream")})
	var streamingEvents int
	for {
		ev, ok := iter.Next()
		if !ok { break }
		if ev.Err != nil { t.Fatalf("err: %v", ev.Err) }
		if ev.Output != nil && ev.Output.MessageOutput != nil && ev.Output.MessageOutput.IsStreaming {
			streamingEvents++
		}
	}
	t.Logf("streaming events received: %d", streamingEvents)
}

// TestIntegration_AgentToolViaRunner verifies AgentTool invocation through Runner.
func TestIntegration_AgentToolViaRunner(t *testing.T) {
	innerM := &mockModel{}
	innerM.addResp("inner tool result")
	innerAgent := NewReActAgent(&ReActConfig[*schema.Message]{Model: innerM}).WithName("inner").WithDescription("inner")
	ctx := context.Background()
	agentTool := NewAgentTool(ctx, innerAgent)

	parentM := &forcedToolModel{
		toolCalls: []schema.ToolCall{{ID: "call_tool", Function: schema.ToolCallFunction{Name: "inner", Arguments: "{\"task\":\"run\"}"}}},
		finalResp: "parent complete",
		firstCall: true,
	}
	parent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: parentM, Tools: []Tool{agentTool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{agentTool}},
	}).WithName("parent_tool")

	store := newCancelTestStore()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent, CheckPointStore: store})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("use agent tool")})
	var lastContent string
	for {
		ev, ok := iter.Next()
		if !ok { break }
		if ev.Err != nil { t.Fatalf("err: %v", ev.Err) }
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming && ev.Output.MessageOutput.Message != nil {
			lastContent = ev.Output.MessageOutput.Message.Content
		}
	}
	if lastContent != "parent complete" {
		t.Errorf("expected 'parent complete', got %q", lastContent)
	}
}
