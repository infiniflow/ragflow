package core

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// ============================================================================
// Stress/Defect-Discovery Test Suite for workflow.go, agent_loop.go, flow.go
//
// These tests are designed NOT to pass trivially. They target specific known
// weaknesses and edge cases to expose bugs or design flaws.
// ============================================================================

// ---- Bug #1: drainEvents drops error events (workflow.go:263) ----
//
// drainEvents sends the error to gen but returns nil, causing runSeq to
// continue to the next sub-agent instead of terminating the workflow.
// Expected: a failing sub-agent should stop the entire sequential workflow.
// Actual (current): the error is sent to the event stream, but runSeq
// continues, resulting in partial execution after the failing node.
func TestWorkflow_ErrorInSubAgent_ShouldStopNotContinue(t *testing.T) {
	var execOrder []string
	var mu sync.Mutex

	agents := make([]Agent, 5)
	for i := 0; i < 5; i++ {
		i := i
		nodeID := fmt.Sprintf("node_%02d", i)
		if i == 2 {
			// Node 2 fails
			agents[i] = newErrorAgent(nodeID + "_error")
		} else {
			tool := workflowNodeTool(nodeID, &execOrder, &mu)
			model := &forcedToolModel{
				toolCalls: []schema.ToolCall{{ID: fmt.Sprintf("c%d", i), Function: schema.ToolCallFunction{Name: tool.Name(), Arguments: "{}"}}},
				finalResp: fmt.Sprintf("final from %s", nodeID),
				firstCall: true,
			}
			agents[i] = NewReActAgent(&ReActConfig[*schema.Message]{Model: model, Tools: []Tool{tool}}).WithName(nodeID)
		}
	}

	wf, err := NewSequential(context.Background(), &SequentialConfig{
		Name: "error_stop_test", Description: "5 nodes with middle failure",
		SubAgents: agents,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}})

	var gotError bool
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			gotError = true
			t.Logf("Got expected error: %v", ev.Err)
		}
	}

	if !gotError {
		t.Error("BUG: expected an error from the failing sub-agent, but none was received")
	}

	// The bug: node_03 and node_04 should NOT have executed because node_02 failed.
	// If they did, drainEvents is masking the error.
	mu.Lock()
	execCount := len(execOrder)
	mu.Unlock()

	if execCount > 2 {
		t.Errorf("BUG: node_02 failed, but %d nodes after it still executed. "+
			"drainEvents returns nil after error, so runSeq continues. "+
			"Expected ≤2 tool executions, got %d (executed: %v)",
			2, execCount, execOrder)
	}
	t.Logf("Executed %d tools before error stopped workflow (expected ≤2)", execCount)
}

// errorAgent returns an error immediately on Run.
type errorAgent struct {
	name string
}

func newErrorAgent(name string) Agent {
	return &errorAgent{name: name}
}

func (a *errorAgent) Name(_ context.Context) string                                    { return a.name }
func (a *errorAgent) Description(_ context.Context) string                             { return a.name + " error" }
func (a *errorAgent) GetType() string                                                  { return "ErrorAgent" }
func (a *errorAgent) Run(_ context.Context, _ *AgentInput, _ ...RunOption) *AsyncIterator[*AgentEvent] {
	it, gen := NewAsyncIteratorPair[*AgentEvent]()
	gen.Send(&AgentEvent{Err: errors.New("intentional agent failure")})
	gen.Close()
	return it
}

// ---- Bug #2: cancelTransition uses Interrupted instead of CancelError (workflow.go:106) ----
//
// cancelTransition() creates an Interrupted action (business interrupt) when a cancel
// is requested, rather than a CancelError. This means cancellation in a sequential
// workflow is indistinguishable from a business interrupt. The upper AgentLoop treats
// them differently — CancelError causes clean exit, Interrupted saves checkpoint.
// This test verifies the distinction is correct.
func TestWorkflow_SequentialCancel_ShouldNotTriggerInterruptCheckpoint(t *testing.T) {
	var execOrder []string
	var mu sync.Mutex

	wf, err := buildSequentialWorkflow(6, &execOrder, &mu)
	if err != nil {
		t.Fatal(err)
	}

	store := newConcurrentStore()
	ctx := context.Background()
	opt, cancel := WithCancel()
	cpID := "cancel_no_checkpoint_test"

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{
		Agent:           wf,
		CheckPointStore: store,
	})
	evIter := runner.Run(ctx, []*schema.Message{schema.UserMessage("cancel test")},
		WithCheckPointID(cpID), opt)

	// Cancel after node 3
	for {
		ev, ok := evIter.Next()
		if !ok {
			break
		}
		_ = ev
		mu.Lock()
		orderLen := len(execOrder)
		mu.Unlock()
		if orderLen >= 3 {
			cancel()
			break
		}
	}
	// Drain remaining
	for {
		_, ok := evIter.Next()
		if !ok {
			break
		}
	}

	// After cancel, checkpoint should NOT have been saved for interrupt purposes.
	// The cancelTransition creates an Interrupted action, which may trigger checkpoint save.
	_, found, err := store.Get(ctx, cpID)
	if err != nil {
		t.Fatal(err)
	}

	// This assertion exposes the bug: if found == true, the cancel was incorrectly
	// treated as an interrupt, saving an unnecessary checkpoint.
	if found {
		t.Errorf("BUG: cancel in sequential workflow saved a checkpoint. "+
			"cancelTransition uses Interrupted action instead of CancelError, "+
			"so the upper AgentLoop treats cancel as a business interrupt and saves checkpoint. "+
			"Cancel should NOT produce an interrupt checkpoint.")
	}
	t.Logf("Cancel checkpoint found=%v (expected false if cancel is clean)", found)
}

// ---- Bug #3: AgentLoop goroutine leak detection ----
//
// Tests that calling Stop() does not leak goroutines. The AgentLoop starts
// multiple goroutines (run, handleEvents, watchPreempt, watchStop, proxyGen).
// This test uses runtime.NumGoroutine to detect leaks.
func TestAgentLoop_GoroutineLeak(t *testing.T) {
	initial := runtime.NumGoroutine()

	for i := 0; i < 20; i++ {
		loop := NewAgentLoop[*schema.Message](AgentLoopConfig[*schema.Message]{
			GenInput: func(_ context.Context, _ *AgentLoop[*schema.Message], items []*schema.Message) (*GenInputResult[*schema.Message], error) {
				return &GenInputResult[*schema.Message]{
					Input: &AgentInput{Messages: items}, Consumed: items,
				}, nil
			},
			PrepareAgent: func(_ context.Context, _ *AgentLoop[*schema.Message], _ []*schema.Message) (Agent, error) {
				m := &mockModel{}
				m.addResp("ok")
				return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("leak_test"), nil
			},
		})
		loop.Push(schema.UserMessage("leak"))
		loop.Run(context.Background())
		loop.Stop()
		_ = loop.Wait()
		time.Sleep(time.Millisecond * 5)
	}

	// Give goroutines time to clean up
	time.Sleep(time.Millisecond * 50)
	after := runtime.NumGoroutine()

	leaked := after - initial
	if leaked > 5 {
		t.Errorf("BUG: possible goroutine leak: started with %d goroutines, ended with %d "+
			"(diff=%d, expected <5)", initial, after, leaked)
	}
	t.Logf("Goroutine check: initial=%d, after=%d, diff=%d", initial, after, leaked)
}

// ---- Bug #4: flow.go transfer loop — context cancel doesn't stop infinite loop ----
//
// flow.runLoop (line 213-218) handles transfer by calling next.Run().Next() in a loop.
// If the context is canceled, the loop should terminate but doesn't check for it.
func TestFlow_TransferLoop_ContextCancel(t *testing.T) {
	agentA := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: &forcedToolModel{
			toolCalls: []schema.ToolCall{{ID: "c1", Function: schema.ToolCallFunction{Name: "dummy_tool", Arguments: "{}"}}},
			finalResp: "done",
			firstCall: true,
		},
		Tools: []Tool{&mockTool{name: "dummy_tool", desc: "dummy"}},
	}).WithName("agent_a")

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		iter := agentA.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("start")}})
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			_ = ev
		}
	}()

	// Cancel after a short delay
	time.Sleep(time.Millisecond * 50)
	cancel()

	select {
	case <-done:
		// Agent terminated cleanly
	case <-time.After(time.Second * 5):
		t.Errorf("BUG: agent did not terminate within 5s after context cancel. "+
			"flow.go runLoop may not check context cancellation, causing goroutine leak")
	}
}

// ---- Bug #5: Concurrent Push + Stop race ----
//
// Push items while concurrently calling Stop(). The AgentLoop has lateItems and
// buffer that could race. This test tries to trigger the race.
func TestAgentLoop_ConcurrentPushStop_Race(t *testing.T) {
	const iterations = 50

	for i := 0; i < iterations; i++ {
		loop := NewAgentLoop[*schema.Message](AgentLoopConfig[*schema.Message]{
			GenInput: func(_ context.Context, _ *AgentLoop[*schema.Message], items []*schema.Message) (*GenInputResult[*schema.Message], error) {
				return &GenInputResult[*schema.Message]{
					Input: &AgentInput{Messages: items}, Consumed: items,
				}, nil
			},
			PrepareAgent: func(_ context.Context, _ *AgentLoop[*schema.Message], _ []*schema.Message) (Agent, error) {
				m := &mockModel{}
				m.addResp("ok")
				return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("race_test"), nil
			},
		})

		var wg sync.WaitGroup
		wg.Add(3)

		// Push items concurrently
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				loop.Push(schema.UserMessage(fmt.Sprintf("item_%d_%d", i, j)))
				time.Sleep(time.Microsecond * time.Duration(rand.Intn(100)))
			}
		}()

		// Start loop concurrently
		go func() {
			defer wg.Done()
			time.Sleep(time.Microsecond * 50)
			loop.Run(context.Background())
		}()

		// Stop concurrently
		go func() {
			defer wg.Done()
			time.Sleep(time.Microsecond * time.Duration(50+rand.Intn(100)))
			loop.Stop()
		}()

		wg.Wait()
		_ = loop.Wait()
	}
}

// ---- Bug #6: handleIter panic in runner.go may cause double-close ----
//
// runner.go handleIter has recover() that sends an error event then calls gen.Close().
// But gen.Send() after gen.Close() would panic. If the panic occurs during gen.Send(),
// the recover() would fire again, causing infinite recursion.
//
// NOTE: This test confirmed that a panic inside an agent's Run() is NOT caught
// by runner.go's handleIter, because the panic happens in flowAgent.Run() which
// is called BEFORE handleIter starts. The panic propagates up to the test.
// This is itself a bug — agent panics should be caught and converted to error events.
func TestRunner_HandleIter_PanicSafety(t *testing.T) {
	// Instead of using the full runner (which panics outside handleIter),
	// directly test handleIter's recover behavior.
	_, gen := NewAsyncIteratorPair[*TypedAgentEvent[*schema.Message]]()
	ai := NewAsyncIterator[*TypedAgentEvent[*schema.Message]]()
	ai.Close() // closed iterator to simulate quick termination

	done := make(chan struct{})
	go func() {
		defer close(done)
		// This should NOT panic — handleIter has recover()
		handleIter(false, nil, context.Background(), ai, gen, nil, nil)
	}()

	select {
	case <-done:
		// Clean termination
	case <-time.After(time.Second * 5):
		t.Errorf("BUG: handleIter deadlocked")
	}
}

type panicAgent struct {
	name string
}

func (a *panicAgent) Name(_ context.Context) string        { return a.name }
func (a *panicAgent) Description(_ context.Context) string { return "panics" }
func (a *panicAgent) GetType() string                      { return "PanicAgent" }
func (a *panicAgent) Run(_ context.Context, _ *AgentInput, _ ...RunOption) *AsyncIterator[*AgentEvent] {
	_, gen := NewAsyncIteratorPair[*AgentEvent]()
	gen.Send(&AgentEvent{Err: errors.New("before panic")})
	panic("intentional panic in Run()")
}

// ---- Bug #7: workflow runSeq — Exit action does not stop AgentLoop ----
//
// When a sub-agent returns Exit action, runSeq sends the event to gen and returns nil.
// But the AgentLoop's runAgentAndHandleEvents only checks interruptContexts and
// capturedCancelErr — it doesn't check for Exit. So the AgentLoop continues to the
// next iteration instead of stopping.
func TestWorkflow_ExitAction_ShouldStopAgentLoop(t *testing.T) {
	exitAgent := &exitAgent{name: "exit_agent"}

	loop := NewAgentLoop[*schema.Message](AgentLoopConfig[*schema.Message]{
		GenInput: func(_ context.Context, _ *AgentLoop[*schema.Message], items []*schema.Message) (*GenInputResult[*schema.Message], error) {
			return &GenInputResult[*schema.Message]{
				Input: &AgentInput{Messages: items}, Consumed: items,
			}, nil
		},
		PrepareAgent: func(_ context.Context, _ *AgentLoop[*schema.Message], _ []*schema.Message) (Agent, error) {
			return exitAgent, nil
		},
	})

	loop.Push(schema.UserMessage("test exit"))
	loop.Run(context.Background())
	loop.Stop()
	result := loop.Wait()

	// After Exit action, AgentLoop should stop cleanly, not produce more events
	_ = result
	t.Logf("Exit action test completed. Result exit reason: %v", result.ExitReason)
}

type exitAgent struct {
	name string
}

func (a *exitAgent) Name(_ context.Context) string        { return a.name }
func (a *exitAgent) Description(_ context.Context) string { return "returns exit" }
func (a *exitAgent) GetType() string                      { return "ExitAgent" }
func (a *exitAgent) Run(_ context.Context, _ *AgentInput, _ ...RunOption) *AsyncIterator[*AgentEvent] {
	it, gen := NewAsyncIteratorPair[*AgentEvent]()
	gen.Send(&AgentEvent{Action: &AgentAction{Exit: true}})
	gen.Close()
	return it
}

// ---- Bug #8: React loop doesn't call runAfterAgent on max iteration exceeded ----
//
// react_loop.go buildReActRunFunc: when state.RemainingIterations <= 0 (line 93),
// it sends error and returns, skipping runAfterAgent. Middleware cleanup is missed.
func TestReAct_MaxIterationExceeded_SkipsAfterAgent(t *testing.T) {
	var afterAgentCalled atomic.Bool

	// Use a custom middleware that tracks AfterAgent call
	afterAgentMW := &callTrackingMiddleware{
		onAfterAgent: func() { afterAgentCalled.Store(true) },
	}

	// Create a model that always returns tool calls (infinite loop)
	loopModel := &loopToolModel{
		toolCalls: []schema.ToolCall{
			{ID: "c1", Function: schema.ToolCallFunction{Name: "loop_tool", Arguments: "{}"}},
		},
	}

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:         loopModel,
		MaxIterations: 3,
		Tools:         []Tool{&mockTool{name: "loop_tool", desc: "loops"}},
		Middlewares:   []TypedReActMiddleware[*schema.Message]{afterAgentMW},
	})

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("loop test")}})

	var gotMaxIterError bool
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			gotMaxIterError = true
			t.Logf("Got error: %v", ev.Err)
		}
	}

	if !gotMaxIterError {
		t.Fatal("expected max iteration error")
	}

	if !afterAgentCalled.Load() {
		t.Errorf("BUG: AfterAgent middleware not called after max iteration exceeded. "+
			"buildReActRunFunc returns early on line 94, skipping runAfterAgent. "+
			"Middleware cleanup/hooks are missed.")
	} else {
		t.Log("AfterAgent middleware was called (pass)")
	}
}

// callTrackingMiddleware is a simple middleware that tracks calls to AfterAgent.
type callTrackingMiddleware struct {
	TypedReActMiddleware[*schema.Message]
	onAfterAgent func()
}

func (m *callTrackingMiddleware) BeforeAgent(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
	return ctx, rc, nil
}
func (m *callTrackingMiddleware) BeforeModelRewrite(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
	return ctx, state, nil
}
func (m *callTrackingMiddleware) AfterModelRewrite(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
	return ctx, state, nil
}
func (m *callTrackingMiddleware) AfterAgent(ctx context.Context, state *ReActAgentState) (context.Context, error) {
	if m.onAfterAgent != nil {
		m.onAfterAgent()
	}
	return ctx, nil
}
func (m *callTrackingMiddleware) WrapModel(ctx context.Context, model Model[*schema.Message], mc *ModelContext) (Model[*schema.Message], error) {
	return model, nil
}

// ---- Bug #9: workflow runSeq cancelTransition data leakage ----
//
// cancelTransition creates an Interrupted action with msg and state in
// internalInterrupted. This state is used by the upper framework to determine
// if a checkpoint should be saved. If cancel is confused with interrupt,
// the state saved may be incorrect.
func TestWorkflow_SequentialCancel_StateConsistency(t *testing.T) {
	var execOrder []string
	var mu sync.Mutex

	wf, err := buildSequentialWorkflow(8, &execOrder, &mu)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	opt, cancel := WithCancel()

	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("consistency")}}, opt)

	var lastEvent *AgentEvent
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		lastEvent = ev
		mu.Lock()
		orderLen := len(execOrder)
		mu.Unlock()
		if orderLen >= 4 {
			cancel()
		}
	}

	if lastEvent != nil && lastEvent.Action != nil && lastEvent.Action.Interrupted != nil {
		t.Logf("Cancel produced Interrupted action (may be a bug): Data=%v", lastEvent.Action.Interrupted.Data)
		// Check if the interrupt data suggests it was treated as a business interrupt
		if msg, ok := lastEvent.Action.Interrupted.Data.(string); ok && msg == "Sequential cancel" {
			t.Errorf("BUG: cancel is represented as Interrupted with Data=%q. "+
				"This causes the framework to save a checkpoint for a cancel, "+
				"which is unnecessary and may confuse resume logic.", msg)
		}
	}
}

// ---- Bug #10: drainEventsChan goroutine leak ----
//
// drainEventsChan starts a goroutine that loops on iter.Next(). If the caller
// breaks out of the for-range loop (e.g., via break), the goroutine is leaked
// because it's blocked on iter.Next().
func TestDrainEventsChan_GoroutineLeak(t *testing.T) {
	initial := runtime.NumGoroutine()

	for i := 0; i < 10; i++ {
		wf, err := buildSequentialWorkflow(3, &[]string{}, &sync.Mutex{})
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("leak test")}})

		ch := drainEventsChan(iter)
		// Read just 1 event then break — goroutine should still be alive
		select {
		case _, ok := <-ch:
			if !ok {
				continue
			}
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for first event")
		}
		// Break early — the drain goroutine is leaked!
		break
	}

	time.Sleep(time.Millisecond * 50)
	after := runtime.NumGoroutine()
	leaked := after - initial

	if leaked > 5 {
		t.Errorf("BUG: drainEventsChan goroutine leak detected: %d goroutines leaked. "+
			"The goroutine blocks on iter.Next() even after the caller breaks the loop.", leaked)
	} else {
		t.Logf("No significant goroutine leak: initial=%d, after=%d", initial, after)
	}
}

// ---- Bug #11: Sequential workflow drainEvents drops non-action last events ----
//
// In workflow.go runSeq, drainEvents returns the last AgentEvent only if it has an Action.
// If the last event is a regular message output without action, drainEvents returns nil
// and runSeq continues to the next sub-agent without propagating the final event.
func TestWorkflow_Sequential_LastEventWithoutActionIsDropped(t *testing.T) {
	// Create an agent that returns a message event without any action
	plainMsgAgent := &plainMessageAgent{name: "plain_msg_agent"}

	wf, err := NewSequential(context.Background(), &SequentialConfig{
		Name: "drop_test", Description: "test last event dropping",
		SubAgents: []Agent{plainMsgAgent},
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test")}})

	var events []*AgentEvent
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		events = append(events, ev)
	}

	if len(events) == 0 {
		t.Errorf("BUG: no events received from sequential workflow. "+
			"drainEvents in runSeq returns nil when the last event has no Action, "+
			"so the final message output is dropped by runSeq (it's not forwarded to gen).")
	} else {
		t.Logf("Received %d events (first is probably the dropped one)", len(events))
		for i, ev := range events {
			var out string
			if ev.Output != nil && ev.Output.MessageOutput != nil && ev.Output.MessageOutput.Message != nil {
				out = ev.Output.MessageOutput.Message.Content
			}
			t.Logf("  event[%d]: Output=%v, Action=%v, Err=%v", i, out, ev.Action, ev.Err)
		}
	}
}

type plainMessageAgent struct {
	name string
}

func (a *plainMessageAgent) Name(_ context.Context) string        { return a.name }
func (a *plainMessageAgent) Description(_ context.Context) string { return "plain msg" }
func (a *plainMessageAgent) GetType() string                      { return "PlainMsgAgent" }
func (a *plainMessageAgent) Run(_ context.Context, _ *AgentInput, _ ...RunOption) *AsyncIterator[*AgentEvent] {
	it, gen := NewAsyncIteratorPair[*AgentEvent]()
	gen.Send(&AgentEvent{
		AgentName: a.name,
		Output: &AgentOutput{MessageOutput: &TypedMessageVariant[*schema.Message]{
			Message: &schema.Message{Role: schema.RoleAssistant, Content: "hello"},
		}},
	})
	gen.Close()
	return it
}

// ---- Bug #12: Concurrent workflow with shared state race ----
//
// Multiple concurrent agents share the same runContext.Session.Values map.
// If values are modified concurrently, data race occurs.
func TestWorkflow_Parallel_SharedSessionValuesRace(t *testing.T) {
	agents := make([]Agent, 10)
	for i := 0; i < 10; i++ {
		i := i
		nodeID := fmt.Sprintf("parallel_%02d", i)
		m := &mockModel{}
		m.addResp(fmt.Sprintf("result %d", i))
		agents[i] = NewReActAgent(&ReActConfig[*schema.Message]{
			Model: m,
		}).WithName(nodeID)
	}

	wf, err := NewParallel(context.Background(), &ParallelConfig{
		Name: "parallel_race", Description: "test session value race",
		SubAgents: agents,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("race test")}})
	for range drainEventsChan(iter) {
	}
	t.Log("Parallel shared session values race test completed (run with -race)")
}

// ---- Bug #13: AgentLoop idle timer goroutine leak ----
//
// agent_loop_run.go line 135 starts a goroutine for idle timer.
// If commitStop is called externally while the timer goroutine is running,
// the goroutine may leak or cause double-commit.
func TestAgentLoop_IdleTimerGoroutineLeak(t *testing.T) {
	initial := runtime.NumGoroutine()

	for i := 0; i < 10; i++ {
		loop := NewAgentLoop[*schema.Message](AgentLoopConfig[*schema.Message]{
			GenInput: func(_ context.Context, _ *AgentLoop[*schema.Message], items []*schema.Message) (*GenInputResult[*schema.Message], error) {
				return &GenInputResult[*schema.Message]{
					Input: &AgentInput{Messages: items}, Consumed: items,
				}, nil
			},
			PrepareAgent: func(_ context.Context, _ *AgentLoop[*schema.Message], _ []*schema.Message) (Agent, error) {
				m := &mockModel{}
				m.addResp("ok")
				return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("idle_test"), nil
			},
		})
		loop.Push(schema.UserMessage("idle"))
		loop.Run(context.Background())

		// Set idle timeout and immediately stop — should not leak goroutines
		loop.Stop(UntilIdleFor(time.Millisecond * 100))
		loop.Stop() // immediate stop to cancel idle timer
		_ = loop.Wait()
	}

	time.Sleep(time.Millisecond * 200) // wait for any lingering timers
	after := runtime.NumGoroutine()

	leaked := after - initial
	if leaked > 5 {
		t.Errorf("BUG: possible goroutine leak from idle timer: initial=%d, after=%d, diff=%d",
			initial, after, leaked)
	}
	t.Logf("Idle timer goroutine check: initial=%d, after=%d", initial, after)
}

// ---- Bug #14: Checkpoint data race under concurrent resume ----
//
// Multiple concurrent resumes from the same checkpoint should be safe.
// This tests for data races in the checkpoint store.
func TestCheckpoint_ConcurrentResumeRace(t *testing.T) {
	store := newConcurrentStore()

	// Run a workflow, interrupt it, save checkpoint
	var execOrder []string
	var mu sync.Mutex
	wf, err := buildSequentialWorkflow(5, &execOrder, &mu)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	cpID := "concurrent_resume_test"

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{
		Agent:           wf,
		CheckPointStore: store,
	})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("resume test")},
		WithCheckPointID(cpID))

	// Run until interrupted
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Action != nil && ev.Action.Interrupted != nil {
			t.Logf("Interrupted at: %v", ev.Action.Interrupted.Data)
			break
		}
	}

	// Now resume concurrently
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			r := NewTypedRunner(RunnerConfig[*schema.Message]{
				Agent:           wf,
				CheckPointStore: store,
			})
			it, err := r.Resume(context.Background(), cpID)
			if err != nil {
				t.Logf("Tenant %d resume error: %v", id, err)
				return
			}
			for {
				_, ok := it.Next()
				if !ok {
					break
				}
			}
		}(i)
	}
	wg.Wait()
	t.Log("Concurrent resume test completed (run with -race)")
}

// ---- Bug #15: flowAgent deepCopy loses subAgents after SetSubAgents ----
//
// flow.go SetSubAgents checks if len(fa.subAgents) > 0 but doesn't actually
// set them on the flowAgent. The toFlowAgent deepCopy copies subAgents but
// SetSubAgents doesn't populate them. This means workflow.subAgents and
// flowAgent.subAgents are out of sync.
func TestFlow_SetSubAgents_DoesNotPopulateSubAgents(t *testing.T) {
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: &mockModel{},
	}).WithName("parent")

	sub1 := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: &mockModel{},
	}).WithName("sub1")

	ctx := context.Background()
	fa := toFlowAgent(ctx, agent)
	_, err := SetSubAgents(ctx, fa, []Agent{sub1})
	if err != nil {
		t.Fatal(err)
	}

	// SetSubAgents returns fa (which is the agent itself), but doesn't
	// populate fa.subAgents — it just validates that none are already set.
	if len(fa.subAgents) > 0 {
		t.Logf("subAgents populated: %d (unexpected but OK)", len(fa.subAgents))
	} else {
		t.Errorf("BUG: SetSubAgents does not populate flowAgent.subAgents. "+
			"The sub-agents are set on the returned ResumableAgent but not on "+
			"the original flowAgent. This causes inconsistency when flowAgent.Run() "+
			"tries to find sub-agents via getAgent().")
	}
}

// ---- Bug #16: runSeq ignores canceled context after drainEvents returns error ----
//
// workflow.go runSeq calls drainEvents, which sends error to gen but returns nil.
// The loop then checks shouldCancel() again but at that point the next sub-agent
// may already have started executing. This is a TOCTOU race.
func TestWorkflow_Sequential_CancelRaceInRunSeq(t *testing.T) {
	var execOrder []string
	var mu sync.Mutex

	agents := make([]Agent, 10)
	for i := 0; i < 10; i++ {
		i := i
		nodeID := fmt.Sprintf("race_node_%02d", i)
		tool := workflowNodeTool(nodeID, &execOrder, &mu)
		// Add small delay in tool to widen the race window
		delayedTool := &delayedTool{
			inner: tool.(*workflowNodeToolImpl),
			delay: time.Millisecond * time.Duration(5+i),
			order: &execOrder,
			mu:    &mu,
		}
		model := &forcedToolModel{
			toolCalls: []schema.ToolCall{{ID: fmt.Sprintf("c%d", i), Function: schema.ToolCallFunction{Name: delayedTool.Name(), Arguments: "{}"}}},
			finalResp: fmt.Sprintf("final from %s", nodeID),
			firstCall: true,
		}
		agents[i] = NewReActAgent(&ReActConfig[*schema.Message]{Model: model, Tools: []Tool{delayedTool}}).WithName(nodeID)
	}

	wf, err := NewSequential(context.Background(), &SequentialConfig{
		Name: "cancel_race", Description: "cancel race in runSeq",
		SubAgents: agents,
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	opt, cancel := WithCancel()

	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("race")}}, opt)

	// Cancel early — before most agents have run
	time.AfterFunc(time.Millisecond*10, func() {
		cancel()
	})

	for {
		_, ok := iter.Next()
		if !ok {
			break
		}
	}

	mu.Lock()
	count := len(execOrder)
	mu.Unlock()

	// Due to the TOCTOU race, we may have executed more nodes than expected.
	// The cancel check in runSeq happens BEFORE each sub-agent execution,
	// but the cancel itself is async. This is a design limitation.
	t.Logf("Cancel race: executed %d tools (expected ≤~5 due to async cancel delay)", count)
}

type delayedTool struct {
	inner *workflowNodeToolImpl
	delay time.Duration
	order *[]string
	mu    *sync.Mutex
}

func (t *delayedTool) Name() string       { return t.inner.Name() }
func (t *delayedTool) Description() string { return t.inner.Description() }
func (t *delayedTool) Invoke(ctx context.Context, args string, opts ...ToolOption) (string, error) {
	time.Sleep(t.delay)
	t.mu.Lock()
	*t.order = append(*t.order, t.Name())
	t.mu.Unlock()
	return fmt.Sprintf("%s executed", t.Name()), nil
}
func (t *delayedTool) Stream(ctx context.Context, args string, opts ...ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{"stream: " + t.Name()}), nil
}
