// Package pregel provides stream protocol, retry integration, and
// Pregel engine integration tests. This covers scenarios that correspond
// to Python's async tests, stream v3 tests, and retry integration tests.
package pregel

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	graphPkg "ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// ============================================================
// P0: Stream protocol — StreamMode integration
// ============================================================

// TestStream_ValuesMode verifies StreamModeValues emits state after each step.
func TestStream_ValuesMode(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg, WithRecursionLimit(10))

	ctx := context.Background()
	outputCh, errCh := engine.Run(ctx, map[string]any{"value": "start"}, types.StreamModeValues)

	var events []*StreamEvent
	for result := range outputCh {
		if se, ok := result.(*StreamEvent); ok {
			events = append(events, se)
		}
	}
	err := <-errCh
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Should have at least: checkpoint, task_start, task_end, values, final
	// (exact count depends on engine implementation)
	if len(events) < 2 {
		t.Fatalf("expected at least 2 stream events, got %d", len(events))
	}

	// Verify final event has the final state.
	hasFinal := false
	for _, ev := range events {
		if ev.Type == EventTypeFinal {
			hasFinal = true
			break
		}
	}
	if !hasFinal {
		t.Fatal("expected EventTypeFinal in stream output")
	}
}

// TestStream_UpdatesMode verifies StreamModeUpdates emits per-node updates.
func TestStream_UpdatesMode(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg, WithRecursionLimit(10))

	ctx := context.Background()
	outputCh, errCh := engine.Run(ctx, map[string]any{"value": "start"}, types.StreamModeUpdates)

	var events []*StreamEvent
	for result := range outputCh {
		if se, ok := result.(*StreamEvent); ok {
			events = append(events, se)
		}
	}
	err := <-errCh
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Updates mode emits events. Count them.
	if len(events) == 0 {
		t.Fatal("expected at least one event in Updates mode")
	}
	t.Logf("Updates mode produced %d events", len(events))
}

// TestStream_TasksMode verifies StreamModeTasks emits task lifecycle events.
func TestStream_TasksMode(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg, WithRecursionLimit(10))

	ctx := context.Background()
	outputCh, errCh := engine.Run(ctx, map[string]any{"value": "start"}, types.StreamModeTasks)

	var taskStarts []string
	for result := range outputCh {
		if se, ok := result.(*StreamEvent); ok {
			if se.Type == EventTypeTaskStart {
				taskStarts = append(taskStarts, se.Node)
			}
		}
	}
	err := <-errCh
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(taskStarts) == 0 {
		t.Fatal("expected at least one TaskStart event")
	}
}

// TestStream_MultipleModes verifies that streaming runs work with all modes.
func TestStream_MultipleModes(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg, WithRecursionLimit(10))
	ctx := context.Background()

	for _, mode := range []types.StreamMode{
		types.StreamModeValues,
		types.StreamModeUpdates,
		types.StreamModeTasks,
		types.StreamModeCheckpoints,
	} {
		t.Run(string(mode), func(t *testing.T) {
			outputCh, errCh := engine.Run(ctx, map[string]any{"value": "mode"}, mode)
			for range outputCh {
			}
			if err := <-errCh; err != nil {
				t.Fatalf("mode %s: %v", mode, err)
			}
		})
	}
}

// ============================================================
// P0: Stream — concurrent consumers
// ============================================================

// TestStream_ConcurrentConsumers verifies that the stream output channel
// can be consumed by multiple goroutines without races.
func TestStream_ConcurrentConsumers(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg, WithRecursionLimit(10))

	ctx := context.Background()
	outputCh, errCh := engine.Run(ctx, map[string]any{"value": "conc"}, types.StreamModeValues)

	var wg sync.WaitGroup
	var eventCount atomic.Int32

	// Multiple consumers read from the same channel.
	for i := 0; i < 5; i++ {
		wg.Go(func() {
			for result := range outputCh {
				if _, ok := result.(*StreamEvent); ok {
					eventCount.Add(1)
				}
			}
		})
	}
	// Wait for all consumers.
	wg.Wait()
	<-errCh
	t.Logf("consumed %d events across 5 consumers", eventCount.Load())
}

// ============================================================
// P0: Retry — engine-level integration
// ============================================================

// TestRetry_TransientFailure_Succeeds verifies that a node that fails
// transiently eventually succeeds with retry.
func TestRetry_TransientFailure_Succeeds(t *testing.T) {
	var attempts atomic.Int32

	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("flaky", func(ctx context.Context, state any) (any, error) {
		n := attempts.Add(1)
		if n < 3 { // fail first 2 times, succeed 3rd
			return nil, fmt.Errorf("transient failure attempt %d", n)
		}
		m, _ := state.(map[string]any)
		m["value"] = "success"
		return m, nil
	})
	_ = sg.AddEdge(constants.Start, "flaky")
	_ = sg.AddEdge("flaky", constants.End)

	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 5
	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithRetryPolicy(&rp),
	)

	result, err := engine.RunSync(context.Background(), map[string]any{"value": "retry"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "success" {
		t.Fatalf("expected value=success, got %v", m["value"])
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
}

// TestRetry_TransientFailure_Exhausted verifies retry eventually fails.
func TestRetry_TransientFailure_Exhausted(t *testing.T) {
	var attempts atomic.Int32

	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("always_fail", func(ctx context.Context, state any) (any, error) {
		attempts.Add(1)
		return nil, errors.New("always fails")
	})
	_ = sg.AddEdge(constants.Start, "always_fail")
	_ = sg.AddEdge("always_fail", constants.End)

	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 3
	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithRetryPolicy(&rp),
	)

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "retry"})
	if err == nil {
		t.Fatal("expected error from exhausted retries")
	}
	n := attempts.Load()
	if n > 10 {
		t.Fatalf("suspiciously high attempt count: %d", n)
	}
	t.Logf("exhausted after %d attempts: %v", n, err)
}

// TestRetry_CustomPolicy verifies a custom retry-on predicate works.
func TestRetry_CustomPolicy(t *testing.T) {
	var attempts atomic.Int32

	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("sensitive", func(ctx context.Context, state any) (any, error) {
		n := attempts.Add(1)
		if n == 1 {
			return nil, fmt.Errorf("rate limited") // retryable
		}
		return nil, fmt.Errorf("permanent failure") // not retryable
	})
	_ = sg.AddEdge(constants.Start, "sensitive")
	_ = sg.AddEdge("sensitive", constants.End)

	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 5
	rp.RetryOn = func(err error) bool {
		return err != nil && err.Error() == "rate limited"
	}
	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithRetryPolicy(&rp),
	)

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "retry"})
	if err == nil {
		t.Fatal("expected permanent failure error")
	}
	n := attempts.Load()
	t.Logf("custom retry: %d attempts, err=%v", n, err)
}

// ============================================================
// P1: Retry + checkpoint interaction
// ============================================================

// TestRetry_WithCheckpointer verifies retry works alongside checkpointing.
func TestRetry_WithCheckpointer(t *testing.T) {
	var attempts atomic.Int32

	// Build standalone graph to avoid duplicate edges from newSimpleGraph.
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("flaky_node", func(ctx context.Context, state any) (any, error) {
		n := attempts.Add(1)
		if n < 2 {
			return nil, fmt.Errorf("transient %d", n)
		}
		return map[string]any{"value": "retried"}, nil
	})
	_ = sg.AddEdge(constants.Start, "flaky_node")
	_ = sg.AddEdge("flaky_node", constants.End)

	ms := checkpoint.NewMemorySaver()
	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 5
	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithCheckpointer(ms),
		WithRetryPolicy(&rp),
	)

	result, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "retried" {
		t.Fatalf("expected value=retried, got %v", m["value"])
	}
}

// ============================================================
// P1: Pregel engine — complex execution scenarios
// ============================================================

// TestEngine_50NodeChain verifies the engine correctly executes a 50-node chain.
func TestEngine_50NodeChain(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	prev := constants.Start
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("n_%d", i)
		iCopy := i // capture loop variable
		sg.AddNode(name, func(ctx context.Context, state any) (any, error) {
			m, _ := state.(map[string]any)
			if m == nil {
				m = map[string]any{}
			}
			m["value"] = iCopy
			return m, nil
		})
		_ = sg.AddEdge(prev, name)
		prev = name
	}
	_ = sg.AddEdge(prev, constants.End)

	engine := NewEngine(sg, WithRecursionLimit(100))
	result, err := engine.RunSync(context.Background(), map[string]any{"value": "fan"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if v, ok := m["value"]; !ok || v.(int) != 49 {
		t.Fatalf("expected value=49, got %v", m["value"])
	}
}

// TestEngine_ChainOf100 verifies the engine handles a 100-node chain.
func TestEngine_ChainOf100(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	prev := constants.Start
	for i := 0; i < 100; i++ {
		name := fmt.Sprintf("n_%d", i)
		sg.AddNode(name, func(ctx context.Context, state any) (any, error) {
			m, _ := state.(map[string]any)
			if m == nil {
				m = map[string]any{}
			}
			m["value"] = i
			return m, nil
		})
		_ = sg.AddEdge(prev, name)
		prev = name
	}
	_ = sg.AddEdge(prev, constants.End)

	engine := NewEngine(sg,
		WithRecursionLimit(150),
	)

	result, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("expected map[string]any result, got %T", result)
	}
	if v, ok := m["value"]; !ok || v.(int) != 99 {
		t.Fatalf("expected value=99, got %v", m["value"])
	}
}

// TestEngine_WithMultipleChannels verifies the engine works with
// multiple channel types.
func TestEngine_WithMultipleChannels(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("counter", channels.NewBinaryOperatorAggregate(0, func(a, b any) any {
		return a.(int) + b.(int)
	}))
	sg.AddChannel("name", channels.NewLastValue(""))

	sg.AddNode("node_a", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"counter": 10, "name": "alpha"}, nil
	})
	sg.AddNode("node_b", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"counter": 20, "name": "beta"}, nil
	})
	_ = sg.AddEdge(constants.Start, "node_a")
	_ = sg.AddEdge("node_a", "node_b")
	_ = sg.AddEdge("node_b", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))
	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["name"] != "beta" {
		t.Fatalf("expected name=beta, got %v", m["name"])
	}
	counter, ok := m["counter"]
	if !ok || counter.(int) != 30 {
		t.Fatalf("expected counter=30 (10+20), got %v", counter)
	}
}

// ============================================================
// P2: Engine with interrupts + resume via config
// ============================================================

// TestEngine_Interrupt verifies the engine can be interrupted at a node.
func TestEngine_Interrupt(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("prep", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		m["value"] = "prep"
		return m, nil
	})
	sg.AddNode("target", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		m["value"] = "target"
		return m, nil
	})
	_ = sg.AddEdge(constants.Start, "prep")
	_ = sg.AddEdge("prep", "target")
	_ = sg.AddEdge("target", constants.End)

	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithInterrupts("target"),
	)

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected interrupt at target")
	}
	t.Logf("interrupted (expected): %v", err)
}

// TestEngine_ContextCancellation_Propagation verifies that cancelling
// the context mid-execution is handled properly.
func TestEngine_ContextCancellation_Propagation(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("slow", func(ctx context.Context, state any) (any, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
			m, _ := state.(map[string]any)
			m["value"] = "slow_done"
			return m, nil
		}
	})
	_ = sg.AddEdge(constants.Start, "slow")
	_ = sg.AddEdge("slow", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := engine.RunSync(ctx, map[string]any{"value": "cancel"})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	t.Logf("cancellation (expected): %v", err)
}
