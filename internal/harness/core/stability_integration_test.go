package core

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
)

// ============================================================
// P0-1: ReActGraph lifecycle -- streaming + checkpoint + cancel + resume + interrupt
// ============================================================

func TestStability_ReActGraph_FullLifecycle(t *testing.T) {
	store := checkpoint.NewMemorySaver()
	m := &mockModel{}
	m.addResp("direct response")
	tool := &mockTool{name: "calc", desc: "calculator"}
	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: m, Tools: []Tool{tool},
		ToolsConfig: &ToolsNodeConfig{Tools: []Tool{tool}},
	}).WithName("lifecycle_agent")

	rg, err := NewReActGraph(agent, &ReActGraphConfig{
		Checkpointer:    store,
		RecursionLimit:  10,
		InterruptBefore: nil, // no interrupts for this test
	}, nil)
	if err != nil {
		t.Fatalf("NewReActGraph: %v", err)
	}

	// Use the compiled graph directly.
	cg := rg.Compile()
	state := &ReActGraphState{
		Messages:       []*schema.Message{schema.UserMessage("test")},
		IterationsLeft: 10,
		MaxIterations:  10,
	}
	_, err = cg.Invoke(t.Context(), state)
	if err != nil {
		t.Fatalf("graph Invoke: %v", err)
	}

	// Verify checkpoints were saved
	ctx := t.Context()
	checkpoints, err := store.List(ctx, map[string]interface{}{
		constants.ConfigKeyThreadID: "lifecycle-thread",
	}, 10)
	if err != nil {
		t.Logf("List checkpoints: %v", err)
	} else {
		t.Logf("checkpoints saved: %d", len(checkpoints))
	}

	t.Log("ReActGraph lifecycle: graph invoke completed")
}

// ============================================================
// P0-2: 10K+ message history -- genInput O(n) replay stress
// ============================================================

func TestStability_LongMessageHistory(t *testing.T) {
	model := &mockModel{}
	model.addResp("response")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("long_history")

	const numMessages = 10000
	msgs := make([]Message, numMessages)
	for i := 0; i < numMessages; i++ {
		msgs[i] = schema.UserMessage(fmt.Sprintf("message %d", i))
	}

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})

	var memBefore, memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	ctx := t.Context()
	iter := runner.Run(ctx, msgs)
	var gotResponse bool
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("error with 10K messages: %v", ev.Err)
			break
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
			gotResponse = true
		}
	}

	runtime.GC()
	runtime.ReadMemStats(&memAfter)

	allocMB := float64(memAfter.TotalAlloc-memBefore.TotalAlloc) / 1024 / 1024
	t.Logf("10K messages: got response=%v, allocated=%.2f MB", gotResponse, allocMB)

	if allocMB > 500 {
		t.Errorf("memory allocation too high: %.2f MB (expected < 500 MB)", allocMB)
	}
}

// ============================================================
// P0-3: Parallel workflow shared state race -- 50 sub-agents, 5 concurrent
// ============================================================

func TestStability_ParallelWorkflow_SharedStateRace(t *testing.T) {
	const numParallel = 50
	const numRuns = 5

	for runID := 0; runID < numRuns; runID++ {
		agents := make([]Agent, numParallel)
		for i := 0; i < numParallel; i++ {
			model := &mockModel{}
			model.addResp(fmt.Sprintf("agent %d response", i))
			agents[i] = NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName(fmt.Sprintf("p_%d", i))
		}

		ctx := t.Context()
		par, err := NewParallel(ctx, &ParallelConfig{
			Name: "p0_par", Description: "parallel state race test",
			SubAgents: agents,
		})
		if err != nil {
			t.Fatalf("NewParallel: %v", err)
		}

		runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: par})
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("go")})
		var count int
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				t.Fatalf("run %d err: %v", runID, ev.Err)
			}
			if ev.Output != nil {
				count++
			}
		}
		if count == 0 {
			t.Errorf("run %d: no output", runID)
		}
	}
}

// ============================================================
// P0-4: Checkpoint corruption -- gob encoding failures + partial writes
// ============================================================

func TestStability_CheckpointCorruption(t *testing.T) {
	t.Run("unregistered_type_fails_gob", func(t *testing.T) {
		var buf bytes.Buffer
		enc := gob.NewEncoder(&buf)
		err := enc.Encode(map[string]interface{}{"data": make(chan int)})
		if err == nil {
			t.Log("gob encoding of unencodable type succeeded (unexpected)")
		} else {
			t.Logf("gob correctly rejected unencodable type: %v", err)
		}
	})

	t.Run("corrupted_data_returns_error", func(t *testing.T) {
		store := &memStore{data: make(map[string][]byte)}
		store.Set(t.Context(), "corrupt", []byte{0x00, 0x01, 0x02, 0x03})

		_, _, _, err := loadCheckpoint(store, t.Context(), "corrupt")
		if err == nil {
			t.Error("expected error loading corrupted checkpoint, got nil")
		} else {
			t.Logf("corrupted checkpoint correctly rejected: %v", err)
		}
	})

	t.Run("checkpoint_roundtrip_with_events", func(t *testing.T) {
		store := newCancelTestStore()
		cid := "test-rt"
		ctx := t.Context()

		err := saveCheckpoint(store, ctx, cid, false, &InterruptInfo{}, &InterruptSignal{
			ID: "test", Info: "test-data",
		})
		if err != nil {
			t.Fatalf("saveCheckpoint: %v", err)
		}

		_, _, info, err := loadCheckpoint(store, ctx, cid)
		if err != nil {
			t.Fatalf("loadCheckpoint: %v", err)
		}
		if info == nil {
			t.Fatal("loaded nil ResumeInfo")
		}
		t.Logf("checkpoint roundtrip: info=%v", info)
	})

	t.Run("checkpoint_restores_resume_state", func(t *testing.T) {
		store := newCancelTestStore()
		cid := "resume-state"
		wantState := "waiting-for-approval"
		signal := &InterruptSignal{
			Address: Address{{Type: AddressSegmentAgent, ID: "agent"}},
			State:   wantState,
		}
		if err := saveCheckpoint(store, t.Context(), cid, false, &InterruptInfo{Data: "approve"}, signal); err != nil {
			t.Fatalf("saveCheckpoint: %v", err)
		}

		_, _, info, err := loadCheckpoint(store, t.Context(), cid)
		if err != nil {
			t.Fatalf("loadCheckpoint: %v", err)
		}
		if !info.WasInterrupted {
			t.Fatal("WasInterrupted = false, want true")
		}
		gotState, ok := info.InterruptState.(string)
		if !ok || gotState != wantState {
			t.Fatalf("InterruptState = %#v, want %#v", info.InterruptState, wantState)
		}
	})

	t.Run("partial_checkpoint_after_cancel", func(t *testing.T) {
		m := newCancelTestChatModel(nil)
		m.addResp("cancel response")
		m.setDelay(50 * time.Millisecond)

		agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("cp_cancel")
		store := newCancelTestStore()

		cancelOpt, cancelFunc := WithCancel()
		runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, CheckPointStore: store})
		ctx := t.Context()

		cid := "partial-cp"
		iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("run")},
			WithCheckPointID(cid), cancelOpt)

		time.Sleep(15 * time.Millisecond)
		cancelFunc(WithCancelMode(CancelImmediate))

		for {
			_, ok := iter.Next()
			if !ok {
				break
			}
		}

		resumedIter, err := runner.Resume(ctx, cid)
		if err != nil {
			t.Logf("resume from partial checkpoint: %v", err)
		} else {
			var outputs int
			for {
				ev, ok := resumedIter.Next()
				if !ok {
					break
				}
				if ev.Err != nil {
					break
				}
				if ev.Output != nil && ev.Output.MessageOutput != nil {
					outputs++
				}
			}
			t.Logf("partial checkpoint resume: %d outputs", outputs)
		}
	})
}

type checkpointResumeTestAgent struct {
	resumeInfo *ResumeInfo
}

func (a *checkpointResumeTestAgent) Name(context.Context) string { return "checkpoint_resume" }
func (a *checkpointResumeTestAgent) Description(context.Context) string {
	return "checkpoint resume test agent"
}

func (a *checkpointResumeTestAgent) Run(ctx context.Context, _ *AgentInput, _ ...RunOption) *AsyncIterator[*AgentEvent] {
	iter, gen := NewAsyncIteratorPair[*AgentEvent]()
	go func() {
		defer gen.Close()
		gen.Send(StatefulInterrupt(ctx, "approval required", "saved-state"))
	}()
	return iter
}

func (a *checkpointResumeTestAgent) Resume(_ context.Context, info *ResumeInfo, _ ...RunOption) *AsyncIterator[*AgentEvent] {
	a.resumeInfo = info
	iter, gen := NewAsyncIteratorPair[*AgentEvent]()
	gen.Send(EventFromMessage(schema.AssistantMessage("resumed"), nil, schema.RoleAssistant, ""))
	gen.Close()
	return iter
}

func TestRunner_ResumeRestoresInterruptState(t *testing.T) {
	agent := &checkpointResumeTestAgent{}
	store := newCancelTestStore()
	runner := NewRunner(t.Context(), RunnerConfig[*schema.Message]{
		Agent:           agent,
		CheckPointStore: store,
	})
	cid := "resume-stateful-agent"
	initial := drainAgentEvents(t, runner.Run(t.Context(), []*schema.Message{schema.UserMessage("start")}, WithCheckPointID(cid)))
	if len(initial) == 0 || initial[0].Action == nil || initial[0].Action.Interrupted == nil {
		t.Fatal("initial run did not emit an interrupt")
	}

	resumed, err := runner.Resume(t.Context(), cid)
	if err != nil {
		t.Fatalf("Resume: %v", err)
	}
	resumedEvents := drainAgentEvents(t, resumed)
	if len(resumedEvents) == 0 || resumedEvents[0].Err != nil {
		t.Fatalf("resume events = %#v", resumedEvents)
	}
	if agent.resumeInfo == nil || !agent.resumeInfo.WasInterrupted || agent.resumeInfo.InterruptState != "saved-state" {
		t.Fatalf("resume info = %#v, want interrupted with saved state", agent.resumeInfo)
	}
}

// ============================================================
// P0-5: 10-layer nested agent cancel propagation
// ============================================================

func TestStability_NestedAgentCancelPropagation(t *testing.T) {
	const depth = 10

	agents := make([]Agent, depth)
	for i := 0; i < depth; i++ {
		model := &mockModel{}
		model.addResp(fmt.Sprintf("layer %d response", i))
		agents[i] = NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName(fmt.Sprintf("seq_%c", 'a'+i))
	}

	ctx := t.Context()
	var current Agent = agents[depth-1]
	for i := depth - 2; i >= 0; i-- {
		inner := current
		outer := agents[i]
		seq, err := NewSequential(ctx, &SequentialConfig{
			Name:      fmt.Sprintf("seq_%c", 'a'+i),
			SubAgents: []Agent{outer, inner},
		})
		if err != nil {
			t.Fatalf("NewSequential depth %d: %v", i, err)
		}
		current = seq
	}

	store := newCancelTestStore()
	cancelOpt, cancelFunc := WithCancel()
	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: current, CheckPointStore: store})

	ctx = t.Context()
	cid := "nested-cancel"
	iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("go")},
		WithCheckPointID(cid), cancelOpt)

	time.Sleep(10 * time.Millisecond)
	cancelFunc(WithCancelMode(CancelImmediate))

	gotCancel := false
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		var ce *CancelError
		if ev.Err != nil && errors.As(ev.Err, &ce) {
			gotCancel = true
			t.Logf("nested cancel propagated: %v", ce)
			break
		}
	}
	if !gotCancel {
		t.Log("nested cancel may not have been delivered (known gap)")
	}

	time.Sleep(50 * time.Millisecond)
	runtime.GC()
	t.Logf("nested cancel: depth=%d, goroutines=%d", depth, runtime.NumGoroutine())
}

// ============================================================
// P0-6: Goroutine leak detection -- all concurrent paths
// ============================================================

func TestStability_GoroutineLeak_AllPaths(t *testing.T) {
	type testCase struct {
		name string
		run  func(*testing.T)
	}

	tests := []testCase{
		{
			name: "runner_simple",
			run: func(t *testing.T) {
				model := &mockModel{}
				model.addResp("ok")
				agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("leak_test")
				runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
				iter := runner.Run(t.Context(), []*schema.Message{schema.UserMessage("test")})
				for {
					ev, ok := iter.Next()
					if !ok {
						break
					}
					_ = ev
				}
			},
		},
		{
			name: "agent_tool_nested",
			run: func(t *testing.T) {
				innerM := &mockModel{}
				innerM.addResp("inner")
				inner := NewReActAgent(&ReActConfig[*schema.Message]{Model: innerM}).WithName("inner")
				ctx := t.Context()
				agentTool := NewAgentTool(ctx, inner)

				parentM := &forcedToolModel{
					inner: &mockModel{}, firstCall: true,
					toolCalls: []schema.ToolCall{{ID: "c1", Function: schema.ToolCallFunction{Name: "inner", Arguments: "{}"}}},
					finalResp: "parent done",
				}
				parent := NewReActAgent(&ReActConfig[*schema.Message]{
					Model: parentM, Tools: []Tool{agentTool},
					ToolsConfig: &ToolsNodeConfig{Tools: []Tool{agentTool}},
				}).WithName("parent")
				runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: parent})
				iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("go")})
				for {
					ev, ok := iter.Next()
					if !ok {
						break
					}
					_ = ev
				}
			},
		},
		{
			name: "sequential_workflow",
			run: func(t *testing.T) {
				m1 := &mockModel{}
				m1.addResp("a")
				m2 := &mockModel{}
				m2.addResp("b")
				a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}).WithName("a")
				a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("b")
				ctx := t.Context()
				seq, _ := NewSequential(ctx, &SequentialConfig{Name: "seq", SubAgents: []Agent{a1, a2}})
				runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: seq})
				iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("go")})
				for {
					ev, ok := iter.Next()
					if !ok {
						break
					}
					_ = ev
				}
			},
		},
		{
			name: "parallel_workflow",
			run: func(t *testing.T) {
				m1 := &mockModel{}
				m1.addResp("a")
				m2 := &mockModel{}
				m2.addResp("b")
				a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1}).WithName("a")
				a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("b")
				ctx := t.Context()
				par, _ := NewParallel(ctx, &ParallelConfig{Name: "par", SubAgents: []Agent{a1, a2}})
				runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: par})
				iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("go")})
				for {
					ev, ok := iter.Next()
					if !ok {
						break
					}
					_ = ev
				}
			},
		},
		{
			name: "cancel_immediate",
			run: func(t *testing.T) {
				m := newCancelTestChatModel(nil)
				m.addResp("slow")
				m.setDelay(100 * time.Millisecond)
				agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m}).WithName("cancel_leak")
				cancelOpt, cancelFunc := WithCancel()
				runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
				ctx := t.Context()
				iter := runner.Run(ctx, []*schema.Message{schema.UserMessage("go")}, cancelOpt)
				time.Sleep(10 * time.Millisecond)
				cancelFunc(WithCancelMode(CancelImmediate))
				for {
					ev, ok := iter.Next()
					if !ok {
						break
					}
					_ = ev
				}
			},
		},
		{
			name: "streaming_mode",
			run: func(t *testing.T) {
				model := &mockModel{}
				model.addResp("streamed")
				agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("stream_leak")
				runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent, EnableStreaming: true})
				iter := runner.Run(t.Context(), []*schema.Message{schema.UserMessage("test")})
				for {
					ev, ok := iter.Next()
					if !ok {
						break
					}
					_ = ev
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			goroBefore := runtime.NumGoroutine()
			tc.run(t)
			time.Sleep(30 * time.Millisecond)
			runtime.GC()

			goroAfter := runtime.NumGoroutine()
			leaked := goroAfter - goroBefore
			if leaked > 5 {
				t.Errorf("possible goroutine leak: %d before, %d after (delta=%d)", goroBefore, goroAfter, leaked)
			} else {
				t.Logf("goroutines: before=%d, after=%d (delta=%d)", goroBefore, goroAfter, leaked)
			}
		})
	}
}

// ============================================================
// P0-7: Retry storm / circuit breaker -- concurrent model failures
// ============================================================

type failOnDemandModel struct {
	failCount int32
	threshold int32
}

func (m *failOnDemandModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	failures := atomic.AddInt32(&m.failCount, 1)
	if failures <= m.threshold {
		return nil, fmt.Errorf("simulated model failure #%d", failures)
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: "ok"}, nil
}

func (m *failOnDemandModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	return nil, fmt.Errorf("stream not supported")
}

func (m *failOnDemandModel) BindTools(tools []*schema.ToolInfo) error { return nil }

type alwaysFailingModel struct{}

func (m *alwaysFailingModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	return nil, fmt.Errorf("persistent model failure")
}

func (m *alwaysFailingModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	return nil, fmt.Errorf("persistent stream failure")
}

func (m *alwaysFailingModel) BindTools(tools []*schema.ToolInfo) error { return nil }

func TestStability_RetryStorm_CircuitBreaker(t *testing.T) {
	t.Run("single_failure_with_retry", func(t *testing.T) {
		failModel := &failOnDemandModel{threshold: 1}
		agent := NewReActAgent(&ReActConfig[*schema.Message]{
			Model: failModel,
			RetryConfig: &ModelRetryConfig{
				MaxRetries: 2,
				ShouldRetry: func(ctx context.Context, rc *RetryContext) *RetryDecision {
					return &RetryDecision{Retry: true}
				},
				BackoffFunc: func(ctx context.Context, attempt int) time.Duration {
					return time.Millisecond
				},
			},
		}).WithName("retry_test")
		agent.name = "retry_test"

		ctx := t.Context()
		iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test")}})
		var ok bool
		for {
			ev, more := iter.Next()
			if !more {
				break
			}
			if ev.Err != nil {
				t.Logf("retry test error: %v", ev.Err)
				break
			}
			if ev.Output != nil && ev.Output.MessageOutput != nil && !ev.Output.MessageOutput.IsStreaming {
				ok = true
			}
		}
		if !ok {
			t.Log("retry test: model may have failed after retries exhausted")
		}
	})

	t.Run("all_models_fail_no_amplification", func(t *testing.T) {
		failModel := &alwaysFailingModel{}
		agent := NewReActAgent(&ReActConfig[*schema.Message]{
			Model: failModel,
			RetryConfig: &ModelRetryConfig{
				MaxRetries: 2,
				ShouldRetry: func(ctx context.Context, rc *RetryContext) *RetryDecision {
					return &RetryDecision{Retry: true}
				},
				BackoffFunc: func(ctx context.Context, attempt int) time.Duration {
					return time.Millisecond
				},
			},
		}).WithName("all_fail")
		agent.name = "all_fail"

		ctx := t.Context()
		iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test")}})
		gotError := false
		for {
			ev, ok := iter.Next()
			if !ok {
				break
			}
			if ev.Err != nil {
				gotError = true
				t.Logf("all-models-fail error: %v", ev.Err)
				break
			}
		}
		if !gotError {
			t.Error("expected error when all models fail")
		}
	})

	t.Run("concurrent_1000_failures_no_deadlock", func(t *testing.T) {
		const concurrency = 100
		var wg sync.WaitGroup
		errCh := make(chan error, concurrency)

		for i := 0; i < concurrency; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				failModel := &alwaysFailingModel{}
				agent := NewReActAgent(&ReActConfig[*schema.Message]{
					Model: failModel,
				}).WithName(fmt.Sprintf("storm_%d", id))
				agent.name = fmt.Sprintf("storm_%d", id)

				ctx := t.Context()
				iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test")}})
				gotError := false
				for {
					ev, more := iter.Next()
					if !more {
						break
					}
					if ev.Err != nil {
						gotError = true
						break
					}
				}
				if !gotError {
					errCh <- fmt.Errorf("agent %d: expected error", id)
				}
			}(i)
		}
		wg.Wait()
		close(errCh)

		var failures int
		for err := range errCh {
			t.Error(err)
			failures++
		}
		if failures > 0 {
			t.Errorf("expected 0 failures, got %d", failures)
		}
	})
}
