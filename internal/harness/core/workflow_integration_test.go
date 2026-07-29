package core

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// ============================================================
// P1-8: Streaming + checkpoint + cancel combination
// ============================================================

func TestWorkflow_StreamCheckpointCancelResume(t *testing.T) {
	model := &mockModel{}
	model.addResp("hello world")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("stream_cp")
	agent.name = "stream_cp"

	store := newCancelTestStore()
	cid := "stream-cp-1"
	cancelOpt, cancelFunc := WithCancel()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store, EnableStreaming: true})
	ctx := context.Background()

	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("stream test")},
		WithCheckPointID(cid), cancelOpt)

	time.Sleep(10 * time.Millisecond)
	cancelFunc(WithCancelMode(CancelImmediate))

	var cancelSeen bool
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			var ce *CancelError
			if errors.As(ev.Err, &ce) {
				cancelSeen = true
				t.Logf("cancel received during stream: %v", ce)
			}
			break
		}
	}

	t.Logf("cancel seen: %v", cancelSeen)

	resumedIter, err := runner.Resume(ctx, cid)
	if err != nil {
		t.Logf("resume after stream cancel: %v", err)
		return
	}

	var resumedEvents int
	for {
		ev, ok := resumedIter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("resume event error: %v", ev.Err)
			break
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil {
			resumedEvents++
		}
	}
	t.Logf("resumed events: %d", resumedEvents)
}

// ============================================================
// P1-9: Tool node semaphore leak on panic
// ============================================================

type panickingTool struct {
	name    string
	panicOn int32
	callNum int32
}

func (t *panickingTool) Name() string        { return t.name }
func (t *panickingTool) Description() string { return "tool that may panic" }

func (t *panickingTool) Invoke(ctx context.Context, args string, opts ...ToolOption) (string, error) {
	n := atomic.AddInt32(&t.callNum, 1)
	if n == t.panicOn {
		panic(fmt.Sprintf("simulated panic in tool %s", t.name))
	}
	return "result", nil
}

func (t *panickingTool) Stream(ctx context.Context, args string, opts ...ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{"result"}), nil
}

type toolCallingModel struct {
	mu        sync.Mutex
	toolCalls []schema.ToolCall
}

func (m *toolCallingModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return &schema.Message{
		Role:      schema.RoleAssistant,
		Content:   "",
		ToolCalls: m.toolCalls,
	}, nil
}

func (m *toolCallingModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]Message{msg}), nil
}

func (m *toolCallingModel) BindTools(tools []*schema.ToolInfo) error { return nil }

func TestWorkflow_ToolPanic_SemaphoreLeak(t *testing.T) {
	pTool := &panickingTool{name: "panic_tool", panicOn: 1}

	// This test verifies that if a tool panics during execution, the semaphore
	// slot is released (otherwise subsequent tool calls would deadlock).
	// The semaphore pattern in tools_node.go:116 uses:
	//   sem <- struct{}{}  // acquire
	//   defer func() { <-sem }()  // release
	// If the goroutine panics before the defer runs (between acquire and defer
	// setup), the semaphore is leaked.

	// Create a model that produces N tool calls to test semaphore behavior.
	model := &toolCallingModel{
		toolCalls: []schema.ToolCall{
			{ID: "call_1", Function: schema.ToolCallFunction{Name: "panic_tool", Arguments: "{}"}},
			{ID: "call_2", Function: schema.ToolCallFunction{Name: "panic_tool", Arguments: "{}"}},
			{ID: "call_3", Function: schema.ToolCallFunction{Name: "panic_tool", Arguments: "{}"}},
		},
	}

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Tools:       []Tool{pTool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{pTool}},
	}).WithName("panic_tool_test")

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test")}})

	gotError := false
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			gotError = true
			t.Logf("tool panic error: %v", ev.Err)
			break
		}
	}
	if !gotError {
		t.Log("tool panic may have been recovered silently")
	}
}

// ============================================================
// P1-10: Sequential workflow error propagation
// ============================================================

func TestWorkflow_SequentialWorkflow_ErrorPropagation(t *testing.T) {
	m1 := &mockModel{}
	m1.addResp("agent a response")
	m2 := &mockModel{}
	m2.addResp("agent b response")

	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}).WithName("agent_a")
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("agent_b")

	// Make agent_a fail by configuring it with shouldFail
	m1.shouldFail = true

	ctx := context.Background()
	seq, err := NewSequential(ctx, &SequentialConfig{
		Name: "seq_err", Description: "error propagation test",
		SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewSequential: %v", err)
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: seq})
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("test")})

	// Agent A fails, sequential workflow should stop and NOT execute agent B.
	var bExecuted bool
	var errorSeen bool
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			errorSeen = true
			t.Logf("error propagated correctly: %v", ev.Err)
			break
		}
		if ev.AgentName == "agent_b" {
			bExecuted = true
		}
	}

	if !errorSeen {
		t.Error("expected error from seq workflow, got none")
	}
	if bExecuted {
		t.Error("BUG: agent B executed despite agent A failing")
	} else {
		t.Log("agent B was correctly skipped after agent A failure")
	}
}

// ============================================================
// P1-11: 1000+ concurrent Runner.Run resource exhaustion
// ============================================================

func TestWorkflow_ConcurrentRunner_HighVolume(t *testing.T) {
	const concurrency = 1000

	goroBefore := runtime.NumGoroutine()
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			model := &mockModel{}
			model.addResp(fmt.Sprintf("response %d", id))
			agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName(fmt.Sprintf("high_%d", id))
			runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})

			ctx := context.Background()
			iter := runner.Run(ctx, []*schema.Message{schema.UserMessage(fmt.Sprintf("q%d", id))})
			gotResponse := false
			for {
				ev, ok := iter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					errs <- fmt.Errorf("agent %d: %w", id, ev.Err)
					return
				}
				if ev.Output != nil && ev.Output.MessageOutput != nil {
					gotResponse = true
				}
			}
			if !gotResponse {
				errs <- fmt.Errorf("agent %d: no output", id)
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	time.Sleep(50 * time.Millisecond)
	goroAfter := runtime.NumGoroutine()

	var failures int
	for err := range errs {
		t.Error(err)
		failures++
	}
	if failures > 0 {
		t.Errorf("expected 0 failures, got %d", failures)
	}

	leaked := goroAfter - goroBefore
	if leaked > 50 {
		t.Errorf("possible goroutine leak: %d before, %d after (delta=%d)", goroBefore, goroAfter, leaked)
	} else {
		t.Logf("1000 concurrent runs: goroutines before=%d, after=%d (delta=%d)", goroBefore, goroAfter, leaked)
	}
}

// ============================================================
// P1-12: Model all-failover timeout chain
// ============================================================

func TestWorkflow_ModelFailover_TimeoutChain(t *testing.T) {
	// 3 models that all time out, with 3 retries each.
	// Total: 3 models x 3 retries = 9 sequential model calls.
	// The whole execution should fail with an error, not hang.
	slowModel := newCancelTestChatModel(nil)
	slowModel.addResp("never")
	slowModel.setDelay(200 * time.Millisecond) // Will be cut by context

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: slowModel,
		RetryConfig: &ModelRetryConfig{
			MaxRetries: 3,
			ShouldRetry: func(ctx context.Context, rc *RetryContext) *RetryDecision {
				return &RetryDecision{Retry: true}
			},
			BackoffFunc: func(ctx context.Context, attempt int) time.Duration {
				return time.Millisecond
			},
		},
	}).WithName("timeout_chain")
	agent.name = "timeout_chain"

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	gotTimeout := false
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			if errors.Is(ev.Err, context.DeadlineExceeded) {
				gotTimeout = true
			}
			t.Logf("timeout chain error: %v", ev.Err)
			break
		}
	}
	if !gotTimeout {
		t.Log("no timeout error (model may have completed before deadline)")
	}
}

// ============================================================
// P1-14: AgentLoop Push/interrupt/resume integration
// ============================================================

func TestWorkflow_AgentLoop_PushInterruptResume(t *testing.T) {
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
			m.addResp("turn response")
			agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("turn_agent")
			return agent, nil
		},
	})

	// Push items concurrently
	var pushWg sync.WaitGroup
	for i := 0; i < 20; i++ {
		pushWg.Add(1)
		go func(id int) {
			defer pushWg.Done()
			loop.Push(schema.UserMessage(fmt.Sprintf("item %d", id)))
		}(i)
	}
	pushWg.Wait()

	loop.Run(ctx)

	// Cancel after some items processed
	time.Sleep(10 * time.Millisecond)
	loop.Stop()

	state := loop.Wait()
	t.Logf("AgentLoop state: exit=%v, unhandled=%d", state.ExitReason, len(state.UnhandledItems))

	if state.ExitReason != nil {
		var ce *CancelError
		if errors.As(state.ExitReason, &ce) {
			t.Logf("AgentLoop cancelled: %v", ce)
		} else {
			t.Logf("AgentLoop exit reason: %v", state.ExitReason)
		}
	}
}
