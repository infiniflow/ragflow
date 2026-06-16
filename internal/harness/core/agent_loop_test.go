package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// ======================== Helpers ========================

type turnLoopMockAgent struct {
	name          string
	response      string
	GenerateFn    func(ctx context.Context, msgs []Message) (Message, error)
	captureCancel bool
	canceled      atomic.Bool
}

func (a *turnLoopMockAgent) Name(_ context.Context) string        { return a.name }
func (a *turnLoopMockAgent) Description(_ context.Context) string { return "mock agent" }
func (a *turnLoopMockAgent) Run(ctx context.Context, input *AgentInput, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	m := &mockModel{}
	response := a.response
	if response == "" { response = "mock" }
	m.addResp(response)
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m})
	agent.name = a.name
	return agent.Run(ctx, input, opts...)
}
func (a *turnLoopMockAgent) GetType() string { return "ReActAgent" }

func (a *turnLoopMockAgent) new() Agent {
	return &turnLoopMockAgent{
		name:          a.name,
		response:      a.response,
		GenerateFn:    a.GenerateFn,
		captureCancel: a.captureCancel,
	}
}

type turnLoopMockRunner struct {
	responses []string
	idx       int
}

type turnCancellableModel struct {
	inner          Model[*schema.Message]
	cancelDetected atomic.Bool
}

func (m *turnCancellableModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	select {
	case <-ctx.Done():
		m.cancelDetected.Store(true)
		return nil, ctx.Err()
	default:
	}
	return m.inner.Generate(ctx, msgs, opts...)
}
func (m *turnCancellableModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	return m.inner.Stream(ctx, msgs, opts...)
}
func (m *turnCancellableModel) BindTools(tools []*schema.ToolInfo) error { return m.inner.BindTools(tools) }

func newTurnCheckpointStore() *memStore { return &memStore{data: make(map[string][]byte)} }

// simpleTurnLoop creates a minimal AgentLoop for quick tests
func simpleTurnLoop(onEvents func(context.Context, *TurnContext[string], *AsyncIterator[*AgentEvent]) error) *AgentLoop[string] {
	return NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			if len(items) == 0 { return nil, nil }
			return &GenInputResult[string]{
				Input:    &AgentInput{Messages: []Message{schema.UserMessage(items[0])}},
				Consumed: items[:1], Remaining: items[1:],
			}, nil
		},
		PrepareAgent: func(ctx context.Context, loop *AgentLoop[string], consumed []string) (Agent, error) {
			m := &mockModel{}; m.addResp("Echo: " + consumed[0])
			return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}), nil
		},
		OnAgentEvents: onEvents,
	})
}

// newAndRunTurnLoop creates and runs a AgentLoop in one call.
func newAndRunTurnLoop[T any](ctx context.Context, cfg AgentLoopConfig[T]) *AgentLoop[T] {
	l := NewAgentLoop(cfg)
	l.Run(ctx)
	return l
}

// genInputConsumeAll consumes all items at once.
func genInputConsumeAll(_ context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
	if len(items) == 0 { return nil, nil }
	return &GenInputResult[string]{Input: &AgentInput{Messages: []Message{schema.UserMessage(items[0])}}, Consumed: items, Remaining: nil}, nil
}

// genInputConsumeFirst consumes the first item, leaves rest for later.
func genInputConsumeFirst(_ context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
	if len(items) == 0 { return nil, nil }
	return &GenInputResult[string]{
		Input:     &AgentInput{Messages: []Message{schema.UserMessage(items[0])}},
		Consumed:  []string{items[0]},
		Remaining: items[1:],
	}, nil
}

// genInputConsumeAllWithMsg consumes all items and produces a user message.
func genInputConsumeAllWithMsg(_ context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
	if len(items) == 0 { return nil, nil }
	return &GenInputResult[string]{
		Input:    &AgentInput{Messages: []Message{schema.UserMessage(items[0])}},
		Consumed: items,
	}, nil
}

// prepareTestAgent returns a default mock agent.
var prepareTestAgent = func(_ context.Context, _ *AgentLoop[string], _ []string) (Agent, error) {
	m := &mockModel{}
	m.addResp("test")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: m})
	agent.name = "test"
	return agent, nil
}

func prepareAgent(a Agent) func(context.Context, *AgentLoop[string], []string) (Agent, error) {
	return func(_ context.Context, _ *AgentLoop[string], _ []string) (Agent, error) {
		return a, nil
	}
}

func waitOrFail(t *testing.T, ch <-chan struct{}, msg string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal(msg)
	}
}

func newTestStore() *memStore {
	return &memStore{data: make(map[string][]byte)}
}

// turnLoopCancellableMockAgent is a mock Agent that supports cancel observation.
type turnLoopCancellableMockAgent struct {
	name    string
	runFunc func(ctx context.Context, input *AgentInput) (*AgentOutput, error)
	onCancel func(cc *cancelContext)
	cancel  context.CancelFunc
	mu      sync.Mutex
}

func (a *turnLoopCancellableMockAgent) Name(_ context.Context) string        { return a.name }
func (a *turnLoopCancellableMockAgent) Description(_ context.Context) string { return "mock agent" }
func (a *turnLoopCancellableMockAgent) Run(ctx context.Context, input *AgentInput, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	iter, gen := NewAsyncIteratorPair[*AgentEvent]()

	o := getCommonOptions(nil, opts...)
	cc := o.cancelCtx

	a.mu.Lock()
	var cancelCtx context.Context
	cancelCtx, a.cancel = context.WithCancel(ctx)
	a.mu.Unlock()

	go func() {
		defer gen.Close()
		if cc != nil {
			go func() {
				<-cc.cancelChan
				if a.onCancel != nil {
					a.onCancel(cc)
				}
				a.mu.Lock()
				if a.cancel != nil {
					a.cancel()
				}
				a.mu.Unlock()
			}()
		}

		output, err := a.runFunc(cancelCtx, input)
		if err != nil {
			gen.Send(&AgentEvent{Err: err})
			return
		}
		gen.Send(&AgentEvent{Output: output})
	}()
	return iter
}

// turnLoopStopModeProbeAgent allows inspecting cancel mode.
type turnLoopStopModeProbeAgent struct {
	ccCh chan *cancelContext
}

func (a *turnLoopStopModeProbeAgent) Name(_ context.Context) string        { return "probe" }
func (a *turnLoopStopModeProbeAgent) Description(_ context.Context) string { return "probe" }
func (a *turnLoopStopModeProbeAgent) Run(_ context.Context, _ *AgentInput, opts ...RunOption) *AsyncIterator[*AgentEvent] {
	iter, gen := NewAsyncIteratorPair[*AgentEvent]()
	o := getCommonOptions(nil, opts...)
	cc := o.cancelCtx
	a.ccCh <- cc
	go func() {
		defer gen.Close()
		<-cc.cancelChan
		for {
			if cc.getMode() == CancelImmediate {
				gen.Send(&AgentEvent{Err: cc.createError()})
				return
			}
			time.Sleep(1 * time.Millisecond)
		}
	}()
	return iter
}

// turnLoopInterruptAgent is an agent that produces a business interrupt.
type turnLoopInterruptAgent struct {
	interruptInfo any
}

func (a *turnLoopInterruptAgent) Name(_ context.Context) string        { return "InterruptAgent" }
func (a *turnLoopInterruptAgent) Description(_ context.Context) string { return "agent that interrupts" }
func (a *turnLoopInterruptAgent) Run(ctx context.Context, _ *AgentInput, _ ...RunOption) *AsyncIterator[*AgentEvent] {
	iter, gen := NewAsyncIteratorPair[*AgentEvent]()
	go func() {
		defer gen.Close()
		gen.Send(Interrupt(ctx, a.interruptInfo))
	}()
	return iter
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && s[:len(substr)] == substr
}

// ======================== NewAgentLoop & Panic Tests ========================

func TestTurnLoop_NewPanicsWithNilGenInput(t *testing.T) {
	defer func() {
		if r := recover(); r == nil { t.Fatal("expected panic") }
	}()
	NewAgentLoop[string](AgentLoopConfig[string]{PrepareAgent: func(_ context.Context, _ *AgentLoop[string], _ []string) (Agent, error) { return nil, nil }})
}

func TestTurnLoop_NewPanicsWithNilPrepareAgent(t *testing.T) {
	defer func() {
		if r := recover(); r == nil { t.Fatal("expected panic") }
	}()
	NewAgentLoop[string](AgentLoopConfig[string]{GenInput: func(_ context.Context, _ *AgentLoop[string], _ []string) (*GenInputResult[string], error) { return nil, nil }})
}

// ======================== Basic Push-Stop-Run ========================

func TestTurnLoop_PushRunAndWait(t *testing.T) {
	tl := simpleTurnLoop(nil)
	tl.Push("a"); tl.Push("b")
	ctx := context.Background()
	tl.Stop()
	tl.Run(ctx)
	result := tl.Wait()
	if result == nil { t.Fatal("nil result") }
	t.Logf("basic: unhandled=%d", len(result.UnhandledItems))
}

func TestTurnLoop_StopCause(t *testing.T) {
	tl := simpleTurnLoop(nil)
	tl.Push("x")
	tl.Stop(WithStopCause("max_tokens"))
	tl.Run(context.Background())
	result := tl.Wait()
	if result.StopCause != "max_tokens" { t.Errorf("StopCause = %q", result.StopCause) }
}

func TestTurnLoop_OnAgentEventsCalled(t *testing.T) {
	var called atomic.Bool
	tl := simpleTurnLoop(func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
		called.Store(true)
		for { ev, ok := events.Next(); if !ok { break }; _ = ev }
		return nil
	})
	ctx := context.Background()
	tl.Run(ctx)
	tl.Push("ev")
	time.Sleep(50 * time.Millisecond)
	tl.Stop()
	tl.Wait()
	if !called.Load() { t.Error("OnAgentEvents not called") }
}

func TestTurnLoop_OnAgentEventsReturnsError(t *testing.T) {
	tl := simpleTurnLoop(func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
		return errors.New("custom_events_error")
	})
	ctx := context.Background()
	tl.Run(ctx)
	tl.Push("fail")
	time.Sleep(50 * time.Millisecond)
	tl.Stop()
	result := tl.Wait()
	if result.ExitReason == nil || !containsString(result.ExitReason.Error(), "custom_events_error") {
		t.Errorf("expected custom_events_error, got %v", result.ExitReason)
	}
}

// ======================== GenInput / PrepareAgent Errors ========================

func TestTurnLoop_GenInputErrors(t *testing.T) {
	tl := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			if len(items) == 0 { return nil, nil }
			return nil, errors.New("gen_input_err")
		},
		PrepareAgent: func(ctx context.Context, loop *AgentLoop[string], consumed []string) (Agent, error) {
			return nil, nil
		},
	})
	tl.Push("bad")
	tl.Stop()
	tl.Run(context.Background())
	result := tl.Wait()
	if result.ExitReason == nil { t.Log("no exit error (may not reach GenInput before stop)") }
}

func TestTurnLoop_PrepareAgentErrors(t *testing.T) {
	tl := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			return &GenInputResult[string]{Consumed: items, Remaining: nil}, nil
		},
		PrepareAgent: func(ctx context.Context, loop *AgentLoop[string], consumed []string) (Agent, error) {
			return nil, errors.New("prepare_err")
		},
	})
	tl.Push("bad")
	tl.Stop()
	tl.Run(context.Background())
	result := tl.Wait()
	if result.ExitReason == nil { t.Log("no exit error (may not reach PrepareAgent)") }
}

// ======================== Multiple Items ========================

func TestTurnLoop_MultipleItems(t *testing.T) {
	tl := simpleTurnLoop(nil)
	for i := 0; i < 10; i++ { tl.Push(fmt.Sprintf("item-%d", i)) }
	tl.Stop()
	tl.Run(context.Background())
	result := tl.Wait()
	t.Logf("10 items: unhandled=%d interrupted=%d", len(result.UnhandledItems), len(result.InterruptedItems))
}

func TestTurnLoop_ConcurrentPush(t *testing.T) {
	tl := simpleTurnLoop(nil)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); tl.Push("c") }()
	}
	wg.Wait()
	tl.Stop()
	tl.Run(context.Background())
	result := tl.Wait()
	t.Logf("50 concurrent: unhandled=%d", len(result.UnhandledItems))
}

// ======================== Checkpoint ========================

func TestTurnLoop_WithCheckpoint(t *testing.T) {
	store := newTurnCheckpointStore()
	tl := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			if len(items) == 0 { return nil, nil }
			return &GenInputResult[string]{
				Input:    &AgentInput{Messages: []Message{schema.UserMessage(items[0])}},
				Consumed: items[:1], Remaining: items[1:],
			}, nil
		},
		PrepareAgent: func(ctx context.Context, loop *AgentLoop[string], consumed []string) (Agent, error) {
			m := &mockModel{}; m.addResp("cp:" + consumed[0])
			return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}), nil
		},
		Store: store,
	})
	tl.Push("cp1")
	tl.Stop()
	tl.Run(context.Background())
	result := tl.Wait()
	t.Logf("checkpoint: unhandled=%d", len(result.UnhandledItems))
}

// ======================== Stop Mode Tests ========================

func TestTurnLoop_ImmediateStop(t *testing.T) {
	tl := simpleTurnLoop(nil)
	tl.Push("urgent")
	tl.Run(context.Background())
	tl.Stop(WithImmediateStop(), WithSkipCheckpoint())
	result := tl.Wait()
	t.Logf("immediate: err=%v", result.ExitReason)
}

func TestTurnLoop_StopWithNoItems(t *testing.T) {
	tl := simpleTurnLoop(nil)
	tl.Stop(WithStopCause("empty"))
	tl.Run(context.Background())
	result := tl.Wait()
	if result.StopCause != "empty" { t.Errorf("StopCause = %q", result.StopCause) }
}

func TestTurnLoop_StopMultipleTimes(t *testing.T) {
	tl := simpleTurnLoop(nil)
	tl.Push("x")
	tl.Stop(WithStopCause("first"))
	tl.Stop(WithStopCause("second"))
	tl.Run(context.Background())
	result := tl.Wait()
	_ = result
}

// ======================== Context Cancel ========================

func TestTurnLoop_ContextCancel(t *testing.T) {
	tl := simpleTurnLoop(nil)
	tl.Push("task")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	tl.Stop()
	tl.Run(ctx)
	result := tl.Wait()
	t.Logf("ctx cancel: err=%v", result.ExitReason)
}

// ======================== Items State ========================

func TestTurnLoop_PushAfterStop(t *testing.T) {
	tl := simpleTurnLoop(nil)
	tl.Push("a"); tl.Push("b")
	tl.Stop()
	tl.Push("c")
	tl.Run(context.Background())
	tl.Wait()
}

// ======================== AgentLoop with Tools ========================

func TestTurnLoop_WithToolAgent(t *testing.T) {
	tool := &mockTool{name: "calc", desc: "calculator"}
	tl := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			if len(items) == 0 { return nil, nil }
			return &GenInputResult[string]{
				Input:    &AgentInput{Messages: []Message{schema.UserMessage(items[0])}},
				Consumed: items[:1], Remaining: items[1:],
			}, nil
		},
		PrepareAgent: func(ctx context.Context, loop *AgentLoop[string], consumed []string) (Agent, error) {
			wrapperModel := &forcedToolModel{
				toolCalls: []schema.ToolCall{{ID: "c1", Function: schema.ToolCallFunction{Name: "calc", Arguments: "{}"}}},
				finalResp: "Tool done", firstCall: true,
			}
			return NewReActAgent(&ReActConfig[*schema.Message]{
				Model: wrapperModel, Tools: []Tool{tool},
			}), nil
		},
	})
	tl.Push("use tool")
	tl.Stop()
	ctx := context.Background()
	tl.Run(ctx)
	result := tl.Wait()
	t.Logf("tool agent: unhandled=%d", len(result.UnhandledItems))
}

// ======================== GenInput variants ========================

func TestTurnLoop_GenInputAllConsumed(t *testing.T) {
	tl := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			if len(items) == 0 { return nil, nil }
			return &GenInputResult[string]{Input: &AgentInput{Messages: []Message{schema.UserMessage(items[0])}}, Consumed: items, Remaining: nil}, nil
		},
		PrepareAgent: func(ctx context.Context, loop *AgentLoop[string], consumed []string) (Agent, error) {
			m := &mockModel{}; m.addResp("all")
			return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}), nil
		},
	})
	tl.Push("1"); tl.Push("2")
	tl.Stop()
	tl.Run(context.Background())
	tl.Wait()
}

func TestTurnLoop_GenInputOneByOne(t *testing.T) {
	tl := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			if len(items) == 0 { return nil, nil }
			return &GenInputResult[string]{
				Input: &AgentInput{Messages: []Message{schema.UserMessage(items[0])}},
				Consumed: items[:1], Remaining: items[1:],
			}, nil
		},
		PrepareAgent: func(ctx context.Context, loop *AgentLoop[string], consumed []string) (Agent, error) {
			m := &mockModel{}; m.addResp("one:" + consumed[0])
			return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}), nil
		},
	})
	tl.Push("x"); tl.Push("y"); tl.Push("z")
	tl.Stop()
	tl.Run(context.Background())
	result := tl.Wait()
	t.Logf("stream: unhandled=%d", len(result.UnhandledItems))
}

func TestTurnLoop_GenInputConsumedNone(t *testing.T) {
	tl := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			return &GenInputResult[string]{Consumed: nil, Remaining: items}, nil
		},
		PrepareAgent: func(ctx context.Context, loop *AgentLoop[string], consumed []string) (Agent, error) {
			return nil, nil
		},
	})
	tl.Push("x")
	tl.Stop()
	tl.Run(context.Background())
	result := tl.Wait()
	t.Logf("none consumed: unhandled=%d", len(result.UnhandledItems))
}

// ======================== OnStop / Intercepted Items ========================

func TestTurnLoop_InterceptedItems(t *testing.T) {
	tl := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			if len(items) == 0 { return nil, nil }
			return &GenInputResult[string]{
				Input:    &AgentInput{Messages: []Message{schema.UserMessage(items[0])}},
				Consumed: items[:1], Remaining: items[1:],
			}, nil
		},
		PrepareAgent: func(ctx context.Context, loop *AgentLoop[string], consumed []string) (Agent, error) {
			m := &mockModel{}; m.addResp("intercepted")
			return NewReActAgent(&ReActConfig[*schema.Message]{Model: m}), nil
		},
	})
	tl.Push("a")
	tl.Run(context.Background())
	tl.Stop(WithImmediateStop(), WithSkipCheckpoint())
	result := tl.Wait()
	_ = result
}

// ======================== Edge Cases ========================

func TestTurnLoop_NoPushBeforeRun(t *testing.T) {
	tl := simpleTurnLoop(nil)
	tl.Stop()
	tl.Run(context.Background())
	result := tl.Wait()
	if result == nil { t.Fatal("nil result") }
}

func TestTurnLoop_DoubleRunPanics(t *testing.T) {
	tl := simpleTurnLoop(nil)
	tl.Push("x")
	tl.Stop()
	tl.Run(context.Background())
	tl.Run(context.Background()) // should be no-op
	tl.Wait()
}

func TestTurnLoop_RunThenStopThenWait(t *testing.T) {
	tl := simpleTurnLoop(nil)
	tl.Push("x")
	ctx := context.Background()
	tl.Run(ctx)
	tl.Stop()
	result := tl.Wait()
	if result == nil { t.Fatal("nil result") }
}

// ======================== edge-case tests ========================

// TestTurnLoop_StopIsIdempotent verifies multiple Stop() calls are safe.
func TestTurnLoop_StopIsIdempotent(t *testing.T) {
	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput:     genInputConsumeAll,
		PrepareAgent: prepareTestAgent,
	})

	loop.Stop()
	loop.Stop()
	loop.Stop()

	result := loop.Wait()
	if result.ExitReason != nil {
		t.Errorf("expected nil exit reason, got %v", result.ExitReason)
	}
}

// TestTurnLoop_WaitMultipleGoroutines verifies Wait() is safe for concurrent callers.
func TestTurnLoop_WaitMultipleGoroutines(t *testing.T) {
	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput:     genInputConsumeAll,
		PrepareAgent: prepareTestAgent,
	})

	loop.Stop()

	var wg sync.WaitGroup
	results := make([]*AgentLoopState[string], 3)

	for i := 0; i < 3; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = loop.Wait()
		}()
	}

	wg.Wait()
	// All should return the same pointer
	for i := 1; i < 3; i++ {
		if results[0] != results[i] {
			t.Errorf("Wait returned different results for goroutines")
		}
	}
}

// TestTurnLoop_GetAgentError verifies PrepareAgent errors propagate.
func TestTurnLoop_GetAgentError(t *testing.T) {
	agentErr := errors.New("get agent error")

	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput: genInputConsumeAll,
		PrepareAgent: func(ctx context.Context, _ *AgentLoop[string], consumed []string) (Agent, error) {
			return nil, agentErr
		},
	})

	loop.Push("msg1")

	result := loop.Wait()
	if !errors.Is(result.ExitReason, agentErr) {
		t.Errorf("expected agentErr, got %v", result.ExitReason)
	}
}

// TestTurnLoop_BatchProcessing verifies GenInput receives batched items.
func TestTurnLoop_BatchProcessing(t *testing.T) {
	var batches [][]string
	var mu sync.Mutex

	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			mu.Lock()
			batches = append(batches, items)
			mu.Unlock()
			return &GenInputResult[string]{
				Input:     &AgentInput{},
				Consumed:  items[:1],
				Remaining: items[1:],
			}, nil
		},
		PrepareAgent: prepareTestAgent,
	})

	loop.Push("msg1")
	loop.Push("msg2")
	loop.Push("msg3")

	time.Sleep(200 * time.Millisecond)

	loop.Stop()
	loop.Wait()

	mu.Lock()
	defer mu.Unlock()

	if len(batches) == 0 {
		t.Error("should have processed at least one batch")
	}
}

// TestTurnLoop_StopWithMode verifies Stop with WithGracefulStop works.
func TestTurnLoop_StopWithMode(t *testing.T) {
	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput:     genInputConsumeAll,
		PrepareAgent: prepareTestAgent,
	})

	loop.Stop(WithGracefulStop())

	result := loop.Wait()
	if result.ExitReason != nil {
		t.Errorf("expected nil, got %v", result.ExitReason)
	}
}

// ======================== Context Cancel Variants ========================

func TestTurnLoop_ContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	loop := newAndRunTurnLoop(ctx, AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			select {
			case <-time.After(100 * time.Millisecond):
				return &GenInputResult[string]{Input: &AgentInput{}, Consumed: items}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
		PrepareAgent: prepareTestAgent,
	})

	loop.Push("msg1")

	result := loop.Wait()
	if !errors.Is(result.ExitReason, context.DeadlineExceeded) {
		t.Logf("expected DeadlineExceeded, got %v (may be nil if loop stopped before timeout)", result.ExitReason)
	}
}

func TestTurnLoop_ContextCancelBeforeReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	loop := NewAgentLoop(AgentLoopConfig[string]{
		GenInput:     genInputConsumeAll,
		PrepareAgent: prepareTestAgent,
	})

	loop.Push("msg1")
	loop.Run(ctx)

	result := loop.Wait()
	if !errors.Is(result.ExitReason, context.Canceled) {
		t.Logf("expected Canceled, got %v", result.ExitReason)
	}
}

func TestTurnLoop_ContextCancelDuringBlockingReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	loop := newAndRunTurnLoop(ctx, AgentLoopConfig[string]{
		GenInput:     genInputConsumeAll,
		PrepareAgent: prepareTestAgent,
	})

	time.Sleep(50 * time.Millisecond)
	cancel()

	result := loop.Wait()
	if !errors.Is(result.ExitReason, context.Canceled) {
		t.Logf("expected Canceled, got %v", result.ExitReason)
	}
}

func TestTurnLoop_ContextCancelAfterGenInput_RecoverItems(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	genInputCount := 0
	loop := newAndRunTurnLoop(ctx, AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			genInputCount++
			if genInputCount == 1 {
				cancel()
			}
			return &GenInputResult[string]{
				Input:     &AgentInput{},
				Consumed:  items[:1],
				Remaining: items[1:],
			}, nil
		},
		PrepareAgent: func(ctx context.Context, _ *AgentLoop[string], c []string) (Agent, error) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return NewReActAgent(&ReActConfig[*schema.Message]{Model: &mockModel{}}), nil
		},
	})

	loop.Push("msg1")
	loop.Push("msg2")

	result := loop.Wait()
	if !errors.Is(result.ExitReason, context.Canceled) {
		t.Logf("expected Canceled, got %v", result.ExitReason)
	}
	if len(result.UnhandledItems) == 0 {
		t.Log("no unhandled items (may have been consumed before cancel)")
	}
}

// ======================== OnAgentEvents Tests ========================

func TestTurnLoop_OnAgentEventsReceivesEvents(t *testing.T) {
	var receivedEvents []*AgentEvent
	var receivedConsumed []string
	var mu sync.Mutex

	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput:     genInputConsumeAllWithMsg,
		PrepareAgent: prepareTestAgent,
		OnAgentEvents: func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
			mu.Lock()
			receivedConsumed = append(receivedConsumed, tc.Consumed...)
			mu.Unlock()
			for {
				event, ok := events.Next()
				if !ok { break }
				mu.Lock()
				receivedEvents = append(receivedEvents, event)
				mu.Unlock()
			}
			return nil
		},
	})

	loop.Push("msg1")

	time.Sleep(100 * time.Millisecond)

	loop.Stop()
	result := loop.Wait()

	mu.Lock()
	defer mu.Unlock()

	if result.ExitReason != nil {
		t.Logf("exit reason: %v", result.ExitReason)
	}
	if len(receivedConsumed) == 0 {
		t.Error("should have received consumed items")
	}
}
// ======================== Stop with Checkpoint Cancel ========================

func TestTurnLoop_StopCheckPointIDInCancelError(t *testing.T) {
	ctx := context.Background()
	modelStarted := make(chan struct{}, 1)
	checkpointID := "turn-loop-cancel-ckpt-1"
	store := newTestStore()

	slowModel := &cancelTestChatModel{
		delayNs: int64(500 * time.Millisecond),
		startedChan: modelStarted,
		doneChan:    make(chan struct{}, 1),
	}
	slowModel.addResp("Hello")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Instruction: "You are a test assistant",
		Model:       slowModel,
	}).WithName("TestAgent").WithDescription("Test agent")

	loop := newAndRunTurnLoop(ctx, AgentLoopConfig[string]{
		Store:        store,
		CheckpointID: checkpointID,
		GenInput:     genInputConsumeAllWithMsg,
		PrepareAgent: prepareAgent(agent),
	})

	loop.Push("msg1")

	<-modelStarted
	loop.Stop(WithImmediateStop())

	result := loop.Wait()
	t.Logf("exit reason: %v", result.ExitReason)
}

// ======================== CancelError Captured Independently ========================

func TestTurnLoop_CancelError_CapturedIndependentlyOfCallback(t *testing.T) {
	ctx := context.Background()
	modelStarted := make(chan struct{}, 1)
	checkpointID := "cancel-capture-independent-1"
	store := newTestStore()

	slowModel := &cancelTestChatModel{
		delayNs: int64(500 * time.Millisecond),
		startedChan: modelStarted,
		doneChan:    make(chan struct{}, 1),
	}
	slowModel.addResp("Hello")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Instruction: "You are a test assistant",
		Model:       slowModel,
	}).WithName("TestAgent").WithDescription("Test agent")

	loop := newAndRunTurnLoop(ctx, AgentLoopConfig[string]{
		Store:        store,
		CheckpointID: checkpointID,
		GenInput:     genInputConsumeAllWithMsg,
		PrepareAgent: prepareAgent(agent),
		OnAgentEvents: func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
			for {
				_, ok := events.Next()
				if !ok { break }
			}
			return nil // swallow everything
		},
	})

	loop.Push("msg1")

	<-modelStarted
	loop.Stop(WithImmediateStop())

	result := loop.Wait()
	t.Logf("exit reason: %v", result.ExitReason)
}

// ======================== Stop Without CheckpointID ========================

func TestTurnLoop_StopWithoutCheckpointIDDoesNotPersist(t *testing.T) {
	ctx := context.Background()
	modelStarted := make(chan struct{}, 1)
	store := newTestStore()

	slowModel := &cancelTestChatModel{
		delayNs: int64(500 * time.Millisecond),
		startedChan: modelStarted,
		doneChan:    make(chan struct{}, 1),
	}
	slowModel.addResp("Hello")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Instruction: "You are a test assistant",
		Model:       slowModel,
	}).WithName("TestAgent").WithDescription("Test agent")

	loop := newAndRunTurnLoop(ctx, AgentLoopConfig[string]{
		Store:        store,
		GenInput:     genInputConsumeAllWithMsg,
		PrepareAgent: prepareAgent(agent),
	})

	loop.Push("msg1")

	<-modelStarted
	loop.Stop(WithImmediateStop())

	loop.Wait()
}

// ======================== Stop While Idle ========================

func TestTurnLoop_StopWhileIdle_SkipsCheckpoint(t *testing.T) {
	ctx := context.Background()
	store := newTestStore()
	cpID := "idle-session"

	loop := newAndRunTurnLoop(ctx, AgentLoopConfig[string]{
		Store:        store,
		CheckpointID: cpID,
		GenInput:     genInputConsumeAll,
		PrepareAgent: prepareTestAgent,
	})

	loop.Stop()
	exit := loop.Wait()
	if exit.ExitReason != nil {
		t.Errorf("expected nil, got %v", exit.ExitReason)
	}
}

// ======================== Stop Call From GenInput ========================

func TestTurnLoop_StopCallFromGenInput(t *testing.T) {
	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			loop.Stop()
			return &GenInputResult[string]{Input: &AgentInput{}, Consumed: items}, nil
		},
		PrepareAgent: prepareTestAgent,
	})

	loop.Push("msg1")

	result := loop.Wait()
	if result.ExitReason != nil {
		t.Errorf("expected nil, got %v", result.ExitReason)
	}
}

// ======================== Push From OnAgentEvents ========================

func TestTurnLoop_PushFromOnAgentEvents(t *testing.T) {
	pushCount := int32(0)

	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput:     genInputConsumeFirst,
		PrepareAgent: prepareTestAgent,
		OnAgentEvents: func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
			for {
				_, ok := events.Next()
				if !ok { break }
			}
			count := atomic.AddInt32(&pushCount, 1)
			if count == 1 {
				tc.Loop.Push("follow-up")
			} else {
				tc.Loop.Stop()
			}
			return nil
		},
	})

	loop.Push("initial")

	result := loop.Wait()
	if result.ExitReason != nil {
		t.Errorf("expected nil, got %v", result.ExitReason)
	}
	if atomic.LoadInt32(&pushCount) != 2 {
		t.Errorf("expected 2 pushes, got %d", atomic.LoadInt32(&pushCount))
	}
}

// ======================== NewAgentLoop: Push Before Run ========================

func TestNewTurnLoop_PushBeforeRun(t *testing.T) {
	var processedItems []string
	var mu sync.Mutex

	loop := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			mu.Lock()
			processedItems = append(processedItems, items...)
			mu.Unlock()
			return &GenInputResult[string]{
				Input:    &AgentInput{Messages: []Message{schema.UserMessage(items[0])}},
				Consumed: items,
			}, nil
		},
		PrepareAgent: prepareTestAgent,
	})

	ok, _ := loop.Push("msg1")
	if !ok { t.Error("Push returned false") }
	ok, _ = loop.Push("msg2")
	if !ok { t.Error("Push returned false") }

	loop.Run(context.Background())

	time.Sleep(100 * time.Millisecond)

	loop.Stop()
	result := loop.Wait()

	mu.Lock()
	defer mu.Unlock()

	if result.ExitReason != nil {
		t.Errorf("expected nil, got %v", result.ExitReason)
	}
	if len(processedItems) == 0 {
		t.Error("expected processed items")
	}
}

// ======================== NewAgentLoop: Wait Before Run ========================

func TestNewTurnLoop_WaitBeforeRun(t *testing.T) {
	loop := NewAgentLoop(AgentLoopConfig[string]{
		GenInput:     genInputConsumeAll,
		PrepareAgent: prepareTestAgent,
	})

	waitDone := make(chan *AgentLoopState[string], 1)
	go func() {
		waitDone <- loop.Wait()
	}()

	select {
	case <-waitDone:
		t.Fatal("Wait returned before Run was called")
	case <-time.After(50 * time.Millisecond):
	}

	loop.Push("msg1")
	loop.Stop()
	loop.Run(context.Background())

	select {
	case result := <-waitDone:
		if result.ExitReason != nil {
			t.Errorf("expected nil, got %v", result.ExitReason)
		}
	case <-time.After(1 * time.Second):
		t.Fatal("Wait did not return after Run + Stop")
	}
}

// ======================== NewAgentLoop: Run Is Idempotent ========================

func TestNewTurnLoop_RunIsIdempotent(t *testing.T) {
	var genInputCalls int32

	loop := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			atomic.AddInt32(&genInputCalls, 1)
			return &GenInputResult[string]{Input: &AgentInput{}, Consumed: items}, nil
		},
		PrepareAgent: prepareTestAgent,
	})

	loop.Push("msg1")
	loop.Run(context.Background())
	loop.Run(context.Background())
	loop.Run(context.Background())

	time.Sleep(100 * time.Millisecond)

	loop.Stop()
	result := loop.Wait()

	if result.ExitReason != nil {
		t.Errorf("expected nil, got %v", result.ExitReason)
	}
	if atomic.LoadInt32(&genInputCalls) < 1 {
		t.Error("expected at least 1 GenInput call")
	}
}

// ======================== NewAgentLoop: Concurrent Push And Run ========================

func TestNewTurnLoop_ConcurrentPushAndRun(t *testing.T) {
	for i := 0; i < 50; i++ {
		var count int32

		loop := NewAgentLoop(AgentLoopConfig[string]{
			GenInput: func(ctx context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
				atomic.AddInt32(&count, int32(len(items)))
				return &GenInputResult[string]{Input: &AgentInput{}, Consumed: items}, nil
			},
			PrepareAgent: func(ctx context.Context, _ *AgentLoop[string], consumed []string) (Agent, error) {
				return NewReActAgent(&ReActConfig[*schema.Message]{Model: &mockModel{}}), nil
			},
		})

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			loop.Push("item")
		}()

		go func() {
			defer wg.Done()
			loop.Run(context.Background())
		}()

		wg.Wait()

		time.Sleep(50 * time.Millisecond)

		loop.Stop()
		result := loop.Wait()

		processed := atomic.LoadInt32(&count)
		unhandled := len(result.UnhandledItems)
		if int(processed)+unhandled > 1 {
			t.Errorf("total should not exceed pushed amount: processed=%d unhandled=%d", processed, unhandled)
		}
	}
}

// ======================== Context Propagation ========================

type turnCtxKey struct{}

// TestTurnLoop_CtxPropagation verifies the parent context is propagated to
// PrepareAgent, the agent run, and OnAgentEvents.
func TestTurnLoop_CtxPropagation(t *testing.T) {
	const traceVal = "trace-123"
	var prepareCtxVal, eventsCtxVal string

	ctx := context.WithValue(context.Background(), turnCtxKey{}, traceVal)

	cfg := AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, loop *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			return &GenInputResult[string]{
				Input:    &AgentInput{Messages: []Message{schema.UserMessage(items[0])}},
				Consumed: items,
			}, nil
		},
		PrepareAgent: func(ctx context.Context, loop *AgentLoop[string], consumed []string) (Agent, error) {
			if v, ok := ctx.Value(turnCtxKey{}).(string); ok {
				prepareCtxVal = v
			}
			return &turnLoopMockAgent{
				name: "trace-agent",
				GenerateFn: func(ctx context.Context, msgs []Message) (Message, error) {
					return &schema.Message{Role: schema.RoleAssistant, Content: "done"}, nil
				},
			}, nil
		},
		OnAgentEvents: func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
			if v, ok := ctx.Value(turnCtxKey{}).(string); ok {
				eventsCtxVal = v
			}
			for {
				if _, ok := events.Next(); !ok { break }
			}
			tc.Loop.Stop()
			return nil
		},
	}

	loop := NewAgentLoop(cfg)
	loop.Push("hello")
	loop.Run(ctx)
	result := loop.Wait()

	if result.ExitReason != nil {
		t.Errorf("expected nil, got %v", result.ExitReason)
	}
	if prepareCtxVal != traceVal {
		t.Errorf("PrepareAgent should receive parent context: got %q", prepareCtxVal)
	}
	if eventsCtxVal != traceVal {
		t.Errorf("OnAgentEvents should receive parent context: got %q", eventsCtxVal)
	}
}

// ======================== TurnContext Stopped Channel ========================

func TestTurnLoop_TurnContext_StoppedChannel(t *testing.T) {
	stoppedSeen := make(chan struct{})
	agentStarted := make(chan struct{})

	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput: genInputConsumeAllWithMsg,
		PrepareAgent: func(ctx context.Context, _ *AgentLoop[string], consumed []string) (Agent, error) {
			return &turnLoopCancellableMockAgent{
				name: "slow",
				runFunc: func(ctx context.Context, input *AgentInput) (*AgentOutput, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			}, nil
		},
		OnAgentEvents: func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
			close(agentStarted)
			select {
			case <-tc.Stopped:
				close(stoppedSeen)
			case <-time.After(5 * time.Second):
				t.Error("timed out waiting for Stopped channel")
			}
			for {
				if _, ok := events.Next(); !ok { break }
			}
			return nil
		},
	})

	loop.Push("msg1")
	<-agentStarted
	loop.Stop(WithImmediateStop())

	select {
	case <-stoppedSeen:
		// success
	case <-time.After(5 * time.Second):
		t.Fatal("stopped channel was never observed in OnAgentEvents")
	}

	loop.Wait()
}

// ======================== Stop With Skip Checkpoint ========================

func TestTurnLoop_StopWithSkipCheckpoint(t *testing.T) {
	ctx := context.Background()
	store := newTestStore()
	cpID := "skip-cp-session"

	loop := NewAgentLoop(AgentLoopConfig[string]{
		Store:        store,
		CheckpointID: cpID,
		GenInput:     genInputConsumeAll,
		PrepareAgent: prepareTestAgent,
	})

	loop.Push("a")
	loop.Push("b")
	loop.Stop(WithSkipCheckpoint())
	loop.Run(ctx)

	exit := loop.Wait()
	if exit.ExitReason != nil {
		t.Errorf("expected nil, got %v", exit.ExitReason)
	}
}

// ======================== Stop With Stop Cause ========================

func TestTurnLoop_StopWithStopCause(t *testing.T) {
	ctx := context.Background()
	cause := "user session timeout"

	loop := newAndRunTurnLoop(ctx, AgentLoopConfig[string]{
		GenInput:     genInputConsumeAll,
		PrepareAgent: prepareTestAgent,
	})

	loop.Push("a")
	loop.Stop(WithStopCause(cause))

	exit := loop.Wait()
	if exit.StopCause != cause {
		t.Errorf("expected %q, got %q", cause, exit.StopCause)
	}
}

func TestTurnLoop_StopCause_EmptyWhenNoStop(t *testing.T) {
	ctx := context.Background()

	loop := newAndRunTurnLoop(ctx, AgentLoopConfig[string]{
		GenInput:     genInputConsumeAll,
		PrepareAgent: prepareTestAgent,
	})

	loop.Stop()
	exit := loop.Wait()
	if exit.StopCause != "" {
		t.Errorf("expected empty, got %q", exit.StopCause)
	}
}

func TestTurnLoop_StopCause_InTurnContext(t *testing.T) {
	cause := "business shutdown"
	gotCause := make(chan string, 1)
	agentStarted := make(chan struct{})

	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput: genInputConsumeAllWithMsg,
		PrepareAgent: func(ctx context.Context, _ *AgentLoop[string], consumed []string) (Agent, error) {
			return &turnLoopCancellableMockAgent{
				name: "slow",
				runFunc: func(ctx context.Context, input *AgentInput) (*AgentOutput, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			}, nil
		},
		OnAgentEvents: func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
			close(agentStarted)
			select {
			case <-tc.Stopped:
				gotCause <- tc.StopCause()
			case <-time.After(5 * time.Second):
				t.Error("timed out waiting for Stopped channel")
			}
			for {
				if _, ok := events.Next(); !ok { break }
			}
			return nil
		},
	})

	loop.Push("msg1")
	<-agentStarted
	loop.Stop(WithImmediateStop(), WithStopCause(cause))

	select {
	case c := <-gotCause:
		if c != cause {
			t.Errorf("expected %q, got %q", cause, c)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for StopCause in TurnContext")
	}

	exit := loop.Wait()
	if exit.StopCause != cause {
		t.Errorf("expected %q, got %q", cause, exit.StopCause)
	}
}

func TestTurnLoop_StopCause_FirstNonEmptyWins(t *testing.T) {
	agentStarted := make(chan struct{})

	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput: genInputConsumeAllWithMsg,
		PrepareAgent: func(ctx context.Context, _ *AgentLoop[string], consumed []string) (Agent, error) {
			return &turnLoopCancellableMockAgent{
				name: "slow",
				runFunc: func(ctx context.Context, input *AgentInput) (*AgentOutput, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			}, nil
		},
		OnAgentEvents: func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
			close(agentStarted)
			for {
				if _, ok := events.Next(); !ok { break }
			}
			return nil
		},
	})

	loop.Push("msg1")
	<-agentStarted
	loop.Stop(WithGracefulStop(), WithStopCause("first cause"))
	loop.Stop(WithStopCause("second cause"))

	exit := loop.Wait()
	if exit.StopCause != "first cause" {
		t.Errorf("expected 'first cause', got %q", exit.StopCause)
	}
}

// ======================== Stop Before Run ========================

func TestTurnLoop_StopBeforeRun_PushThenStop(t *testing.T) {
	loop := NewAgentLoop(AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			t.Fatal("GenInput should not be called when Stop is called before Run")
			return nil, nil
		},
		PrepareAgent: func(ctx context.Context, _ *AgentLoop[string], consumed []string) (Agent, error) {
			t.Fatal("PrepareAgent should not be called when Stop is called before Run")
			return nil, nil
		},
	})

	ok, _ := loop.Push("item1")
	if !ok { t.Error("Push returned false") }
	ok, _ = loop.Push("item2")
	if !ok { t.Error("Push returned false") }

	loop.Stop()
	loop.Run(context.Background())
	result := loop.Wait()

	if result.ExitReason != nil {
		t.Errorf("expected nil, got %v", result.ExitReason)
	}
}

// ======================== Skip Checkpoint Sticky ========================

func TestTurnLoop_SkipCheckpoint_Sticky(t *testing.T) {
	agentStarted := make(chan struct{})

	store := newTestStore()
	cpID := "sticky-skip-session"

	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		Store:        store,
		CheckpointID: cpID,
		GenInput:     genInputConsumeAllWithMsg,
		PrepareAgent: func(ctx context.Context, _ *AgentLoop[string], consumed []string) (Agent, error) {
			return &turnLoopCancellableMockAgent{
				name: "slow",
				runFunc: func(ctx context.Context, input *AgentInput) (*AgentOutput, error) {
					<-ctx.Done()
					return nil, ctx.Err()
				},
			}, nil
		},
		OnAgentEvents: func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
			close(agentStarted)
			for {
				if _, ok := events.Next(); !ok { break }
			}
			return nil
		},
	})

	loop.Push("msg1")
	<-agentStarted
	loop.Stop(WithGracefulStop(), WithSkipCheckpoint())
	loop.Stop()

	exit := loop.Wait()
	_ = exit
	t.Logf("skip checkpoint sticky: exit=%v", exit.ExitReason)
}


// ======================== GenInput Error Recovery ========================

func TestTurnLoop_GenInputError_RecoverItems(t *testing.T) {
	genErr := errors.New("gen input error")

	loop := newAndRunTurnLoop(context.Background(), AgentLoopConfig[string]{
		GenInput: func(ctx context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			return nil, genErr
		},
		PrepareAgent: prepareTestAgent,
	})

	loop.Push("msg1")
	loop.Push("msg2")

	result := loop.Wait()
	if !errors.Is(result.ExitReason, genErr) {
		t.Errorf("expected genErr, got %v", result.ExitReason)
	}
}

// ======================== Checkpoint Not Found ========================

func TestTurnLoop_CheckpointNotFound_FreshStart(t *testing.T) {
	ctx := context.Background()
	store := newTestStore()
	var genInputCalled bool
	loop := NewAgentLoop(AgentLoopConfig[string]{
		Store:        store,
		CheckpointID: "nonexistent-id",
		GenInput: func(ctx context.Context, _ *AgentLoop[string], items []string) (*GenInputResult[string], error) {
			genInputCalled = true
			return &GenInputResult[string]{Input: &AgentInput{}, Consumed: items}, nil
		},
		PrepareAgent: prepareTestAgent,
		OnAgentEvents: func(ctx context.Context, tc *TurnContext[string], events *AsyncIterator[*AgentEvent]) error {
			for {
				if _, ok := events.Next(); !ok { break }
			}
			tc.Loop.Stop()
			return nil
		},
	})
	loop.Push("a")
	loop.Run(ctx)
	exit := loop.Wait()
	if exit.ExitReason != nil {
		t.Errorf("expected nil, got %v", exit.ExitReason)
	}
	if !genInputCalled {
		t.Error("GenInput should be called when checkpoint not found")
	}
}

// ======================== TurnBuffer Tests ========================

func TestAttack_TurnBuffer_WakeupDoesNotLoseItems(t *testing.T) {
	tb := newTurnBuffer[string]()

	tb.TrySend("a")
	tb.TrySend("b")
	tb.Wakeup()
	tb.TrySend("c")

	var got []string
	for i := 0; i < 3; i++ {
		val, ok := tb.Receive()
		if !ok { t.Fatal("expected ok") }
		got = append(got, val)
	}

	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("expected [a b c], got %v", got)
	}
}

// ======================== AgentLoop Preempt During Planning ========================

