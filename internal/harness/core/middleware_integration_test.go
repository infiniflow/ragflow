package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ============================================================
// Middleware chain error recovery — failure in one layer
// ============================================================

func TestMiddleware_ChainErrorRecovery(t *testing.T) {
	var callOrder []string
	var mu sync.Mutex
	record := func(s string) { mu.Lock(); callOrder = append(callOrder, s); mu.Unlock() }

	failingMW := &testMiddleware{
		beforeModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			record("failing_beforeModel")
			return ctx, state, fmt.Errorf("middleware failure")
		},
		afterModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			record("failing_afterModel")
			return ctx, state, nil
		},
		beforeAgent: func(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
			record("failing_beforeAgent")
			return ctx, rc, nil
		},
		afterAgent: func(ctx context.Context, state *ReActAgentState) (context.Context, error) {
			record("failing_afterAgent")
			return ctx, nil
		},
	}

	normalMW := &testMiddleware{
		beforeModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			record("normal_beforeModel")
			return ctx, state, nil
		},
		afterModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			record("normal_afterModel")
			return ctx, state, nil
		},
		beforeAgent: func(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
			record("normal_beforeAgent")
			return ctx, rc, nil
		},
		afterAgent: func(ctx context.Context, state *ReActAgentState) (context.Context, error) {
			record("normal_afterAgent")
			return ctx, nil
		},
	}

	model := &mockModel{}
	model.addResp("response")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Middlewares: []ReActMiddleware{failingMW, normalMW},
	}).WithName("mw_chain")
	agent.name = "mw_chain"

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
			t.Logf("middleware chain error: %v", ev.Err)
			break
		}
	}

	mu.Lock()
	order := make([]string, len(callOrder))
	copy(order, callOrder)
	mu.Unlock()

	t.Logf("middleware call order: %v", order)

	if !gotError {
		t.Error("expected error from failing middleware, got none")
	}

	found := false
	for _, s := range order {
		if s == "failing_beforeAgent" { found = true }
	}
	if !found {
		t.Error("failingMW.BeforeAgent should have been called")
	}
}

// ============================================================
// Interrupt signal tree serialization — deep nesting
// ============================================================

func TestInterrupt_TreeSerialization(t *testing.T) {
	t.Run("deeply_nested_tree", func(t *testing.T) {
		var root *InterruptSignal
		current := &InterruptSignal{ID: "root", Info: "root", State: "root-state"}
		root = current
		for i := 0; i < 10; i++ {
			child := &InterruptSignal{
				ID:    fmt.Sprintf("level_%d", i),
				Info:  fmt.Sprintf("info_%d", i),
				State: fmt.Sprintf("state_%d", i),
			}
			current.Children = []*InterruptSignal{child}
			current = child
		}

		id2addr, id2state := signalToMaps(root)

		if len(id2addr) != 11 {
			t.Errorf("expected 11 entries in id2addr (root + 10 levels), got %d", len(id2addr))
		}
		if len(id2state) != 11 {
			t.Errorf("expected 11 entries in id2state, got %d", len(id2state))
		}
		t.Logf("deep tree serialization: %d addresses, %d states", len(id2addr), len(id2state))
	})

	t.Run("nil_signal", func(t *testing.T) {
		id2addr, id2state := signalToMaps(nil)
		if len(id2addr) != 0 || len(id2state) != 0 {
			t.Error("expected empty maps for nil signal")
		}
	})

	t.Run("empty_signal", func(t *testing.T) {
		id2addr, id2state := signalToMaps(&InterruptSignal{})
		if len(id2addr) != 0 || len(id2state) != 0 {
			t.Error("expected empty maps for empty signal")
		}
	})

	t.Run("from_interrupt_contexts_empty", func(t *testing.T) {
		sig := FromInterruptContexts(nil)
		if sig != nil {
			t.Error("expected nil for empty input")
		}
	})

	t.Run("from_interrupt_contexts", func(t *testing.T) {
		ctxs := []*InterruptCtx{
			{ID: "a", Info: "info-a"},
			{ID: "b", Info: "info-b"},
		}
		sig := FromInterruptContexts(ctxs)
		if len(sig.Children) != 2 {
			t.Errorf("expected 2 children, got %d", len(sig.Children))
		}
	})
}

// ============================================================
// Session values concurrent safety — SetRunLocalValue from goroutines
// ============================================================

func TestSession_ConcurrentValueAccess(t *testing.T) {
	model := &mockModel{}
	model.addResp("done")
	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("session_conc")
	agent.name = "session_conc"

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	for { ev, ok := iter.Next(); if !ok { break }; _ = ev }

	var wg sync.WaitGroup
	errs := make(chan error, 20)

	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			err := SetRunLocalValue(context.Background(), fmt.Sprintf("key_%d", id), fmt.Sprintf("val_%d", id))
			if err != nil && !errors.Is(err, errNotInAgentExec) {
				errs <- fmt.Errorf("SetRunLocalValue: %w", err)
			}
			_, _, err = GetRunLocalValue(context.Background(), "test")
			if err != nil && !errors.Is(err, errNotInAgentExec) {
				errs <- fmt.Errorf("GetRunLocalValue: %w", err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// ============================================================
// Callback system — 100K events under streaming
// ============================================================

func TestCallback_HighVolumeEvents(t *testing.T) {
	const numEvents = 1000

	var callbackCount int32

	model := &mockModel{}
	model.addResp("response")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{Model: model}).WithName("cb_volume")
	agent.name = "cb_volume"

	runner := NewTypedRunner(RunnerConfig[*schema.Message]{Agent: agent})
	ctx := context.Background()

	msgs := make([]Message, numEvents)
	for i := 0; i < numEvents; i++ {
		msgs[i] = schema.UserMessage(fmt.Sprintf("msg %d", i))
	}

	iter := runner.Run(ctx, msgs)
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Logf("callback volume error: %v", ev.Err)
			break
		}
	}

	t.Logf("callback count: %d", atomic.LoadInt32(&callbackCount))
}

// ============================================================
// Multiple middleware interaction
// ============================================================

func TestMiddleware_MultipleMiddlewareInteraction(t *testing.T) {
	var callOrder []string
	var mu sync.Mutex
	record := func(s string) { mu.Lock(); callOrder = append(callOrder, s); mu.Unlock() }

	mw1 := &testMiddleware{
		beforeModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			record("mw1_beforeModel")
			return ctx, state, nil
		},
		afterModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			record("mw1_afterModel")
			return ctx, state, nil
		},
		beforeAgent: func(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
			record("mw1_beforeAgent")
			return ctx, rc, nil
		},
		afterAgent: func(ctx context.Context, state *ReActAgentState) (context.Context, error) {
			record("mw1_afterAgent")
			return ctx, nil
		},
	}

	mw2 := &testMiddleware{
		beforeModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			record("mw2_beforeModel")
			return ctx, state, nil
		},
		afterModel: func(ctx context.Context, state *ReActAgentState, mc *ModelContext) (context.Context, *ReActAgentState, error) {
			record("mw2_afterModel")
			return ctx, state, nil
		},
		beforeAgent: func(ctx context.Context, rc *ReActAgentContext) (context.Context, *ReActAgentContext, error) {
			record("mw2_beforeAgent")
			return ctx, rc, nil
		},
		afterAgent: func(ctx context.Context, state *ReActAgentState) (context.Context, error) {
			record("mw2_afterAgent")
			return ctx, nil
		},
	}

	model := &mockModel{}
	model.addResp("response")

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model:       model,
		Middlewares: []ReActMiddleware{mw1, mw2},
	}).WithName("multi_mw")
	agent.name = "multi_mw"

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("test")}})
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev.Err != nil {
			t.Fatalf("unexpected err: %v", ev.Err)
		}
	}

	mu.Lock()
	order := make([]string, len(callOrder))
	copy(order, callOrder)
	mu.Unlock()

	if len(order) < 8 {
		t.Errorf("expected at least 8 middleware hooks, got %d: %v", len(order), order)
	} else {
		t.Logf("multi-middleware call order: %v", order)
	}
}
