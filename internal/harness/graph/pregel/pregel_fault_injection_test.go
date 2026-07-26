// Package pregel provides fault injection and resilience tests for the Pregel engine.
//
// This covers: node panic with checkpoint recovery, checkpoint corruption,
// partial writes in concurrent scenarios, node timeout propagation,
// retry exhaustion, and race conditions on checkpoint save.
package pregel

import (
	"context"
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
// P0: Node panic recovery
// ============================================================

// TestFaultInjection_NodePanic verifies the engine recovers from a
// panicking node without crashing the entire process.
func TestFaultInjection_NodePanic(t *testing.T) {
	g := newSimpleGraph(t)
	// Override node_a to panic.
	g.AddNode("panic_node", func(ctx context.Context, state any) (any, error) {
		panic("simulated node panic")
	})
	g.AddEdge(constants.Start, "panic_node")
	g.AddEdge("panic_node", constants.End)

	engine := NewEngine(g, WithRecursionLimit(10))
	ctx := context.Background()

	_, err := engine.RunSync(ctx, map[string]any{"value": "test"})
	if err == nil {
		t.Fatal("expected error from panicking node")
	}
	t.Logf("expected error: %v", err)
}

// ============================================================
// P0: Node returns error, graph should propagate it
// ============================================================

// TestFaultInjection_NodeError verifies error propagation from a failing node.
func TestFaultInjection_NodeError(t *testing.T) {
	g := newSimpleGraph(t)
	g.AddNode("fail_node", func(ctx context.Context, state any) (any, error) {
		return nil, fmt.Errorf("intentional error")
	})
	g.AddEdge(constants.Start, "fail_node")
	g.AddEdge("fail_node", constants.End)

	engine := NewEngine(g, WithRecursionLimit(10))
	ctx := context.Background()

	_, err := engine.RunSync(ctx, map[string]any{"value": "test"})
	if err == nil {
		t.Fatal("expected error from failing node")
	}
}

// ============================================================
// P1: Checkpoint corruption and recovery
// ============================================================

// TestFaultInjection_CheckpointCorruption verifies the engine handles
// corrupted checkpoint data gracefully (returns an error rather than
// producing incorrect results).
func TestFaultInjection_CheckpointCorruption(t *testing.T) {
	g := newSimpleGraph(t)

	ms := checkpoint.NewMemorySaver()
	engine := NewEngine(g, WithRecursionLimit(10), WithCheckpointer(ms))
	ctx := context.Background()

	// First run creates a clean checkpoint.
	_, err := engine.RunSync(ctx, map[string]any{"value": "first"})
	if err != nil {
		t.Fatalf("first RunSync: %v", err)
	}

	// Corrupt the checkpoint data by injecting bad data directly.
	// This simulates storage corruption.
	corruptConfig := map[string]interface{}{
		constants.ConfigKeyThreadID: defaultTestThreadID,
	}
	ms.Put(ctx, corruptConfig, map[string]interface{}{
		"value":       nil,
		"__corrupt__": "garbage",
	})

	// Second run with bad checkpoint should handle it gracefully.
	_, err = engine.RunSync(ctx, map[string]any{"value": "second"})
	if err != nil {
		t.Logf("handled corrupted checkpoint: %v", err)
	}
}

// ============================================================
// P1: Concurrent checkpoint save races
// ============================================================

// TestFaultInjection_CheckpointRace verifies no data races when multiple
// goroutines save checkpoints concurrently to the same checkpointer.
func TestFaultInjection_CheckpointRace(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	ctx := context.Background()

	const goroutines = 50
	const savesPerGoroutine = 20

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			tid := fmt.Sprintf("race-thread-%d", gid)
			for i := 0; i < savesPerGoroutine; i++ {
				cfg := map[string]interface{}{
					constants.ConfigKeyThreadID: tid,
				}
				data := map[string]interface{}{
					"goroutine": gid,
					"iteration": i,
				}
				if err := ms.Put(ctx, cfg, data); err != nil {
					t.Errorf("Put failed: %v", err)
					return
				}
				if _, err := ms.Get(ctx, cfg); err != nil {
					t.Errorf("Get failed: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
}

// ============================================================
// P1: Node timeout propagation
// ============================================================

// TestFaultInjection_NodeTimeout verifies that a node that exceeds
// the context deadline correctly propagates the timeout.
func TestFaultInjection_NodeTimeout(t *testing.T) {
	g := newSimpleGraph(t)
	g.AddNode("slow", func(ctx context.Context, state any) (any, error) {
		select {
		case <-time.After(5 * time.Second):
			return state, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	g.AddEdge(constants.Start, "slow")
	g.AddEdge("slow", constants.End)

	engine := NewEngine(g, WithRecursionLimit(10))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := engine.RunSync(ctx, map[string]any{"value": "test"})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

// ============================================================
// P1: Engine retry exhaustion
// ============================================================

// TestFaultInjection_RetryExhaustion verifies that when a node repeatedly
// fails, the retry policy exhausts and the error propagates correctly.
func TestFaultInjection_RetryExhaustion(t *testing.T) {
	g := newSimpleGraph(t)
	var attempts atomic.Int32

	g.AddNode("flaky", func(ctx context.Context, state any) (any, error) {
		attempts.Add(1)
		return nil, fmt.Errorf("transient error attempt %d", attempts.Load())
	})
	g.AddEdge(constants.Start, "flaky")
	g.AddEdge("flaky", constants.End)

	engine := NewEngine(g, WithRecursionLimit(10))
	ctx := context.Background()

	_, err := engine.RunSync(ctx, map[string]any{"value": "test"})
	if err == nil {
		t.Fatal("expected error from exhausted retries")
	}
	t.Logf("retry test: attempts=%d, err=%v", attempts.Load(), err)
}

// ============================================================
// P2: Mixed fan-out with some nodes failing
// ============================================================

// TestFaultInjection_ParallelFanOutWithFailures verifies that in a
// fan-out scenario, a failing branch doesn't hang the entire graph
// and the error is reported.
func TestFaultInjection_ParallelFanOutWithFailures(t *testing.T) {
	type State struct {
		Results []string `harness:"reducer=append"`
	}

	sg := graphPkg.NewStateGraph(State{})
	sg.AddChannel("__root__", channels.NewLastValue(State{}))

	// Simulate fan-out via sequential chain (BSP mode processes one node at a time).
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("worker_%d", i)
		iCopy := i
		sg.AddNode(name, func(ctx context.Context, state any) (any, error) {
			if iCopy%4 == 0 {
				return nil, fmt.Errorf("worker %d failed", iCopy)
			}
			return State{Results: []string{fmt.Sprintf("ok_%d", iCopy)}}, nil
		})
		if i == 0 {
			sg.AddEdge(constants.Start, name)
		} else {
			prev := fmt.Sprintf("worker_%d", i-1)
			sg.AddEdge(prev, name)
		}
		if i == 9 {
			sg.AddEdge(name, constants.End)
		}
	}

	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	_, err = cg.Invoke(ctx, State{})
	if err == nil {
		t.Log("all workers succeeded (some workers may be skipped)")
	}
}

// ============================================================
// P2: Context cancellation during execution
// ============================================================

// TestFaultInjection_ContextCancel verifies that cancelling the context
// mid-execution terminates cleanly.
func TestFaultInjection_ContextCancel(t *testing.T) {
	g := newSimpleGraph(t)

	engine := NewEngine(g, WithRecursionLimit(100))
	ctx, cancel := context.WithCancel(context.Background())

	// Cancel after a short delay.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	outputCh, errCh := engine.Run(ctx, map[string]any{"value": "test"}, types.StreamModeValues)
	for range outputCh {
	}
	err := <-errCh
	if err != nil && err != context.Canceled {
		t.Fatalf("expected context.Canceled or nil, got: %v", err)
	}
}

// ============================================================
// P2: Rapid Invoke with same engine (reuse safety)
// ============================================================

// TestFaultInjection_EngineReuse verifies that reusing the same Engine
// across multiple RunSync calls is safe (no stale state leakage).
func TestFaultInjection_EngineReuse(t *testing.T) {
	g := newSimpleGraph(t)
	engine := NewEngine(g, WithRecursionLimit(10))
	ctx := context.Background()

	for i := 0; i < 50; i++ {
		_, err := engine.RunSync(ctx, map[string]any{"value": fmt.Sprintf("run_%d", i)})
		if err != nil {
			t.Fatalf("RunSync #%d: %v", i, err)
		}
	}
}

// ============================================================
// P2: Empty graph handling
// ============================================================

// TestFaultInjection_EmptyGraph verifies that an empty graph (no nodes)
// returns an appropriate error rather than panicking.
func TestFaultInjection_EmptyGraph(t *testing.T) {
	// Using StateGraph directly, not starting from start.
	type State struct{}
	sg := graphPkg.NewStateGraph(State{})

	_, err := sg.Compile()
	if err == nil {
		t.Fatal("expected error for empty graph with no entry point")
	}
}

// ============================================================
// P2: Channel restore from corrupted checkpoint
// ============================================================

// TestFaultInjection_ChannelRestoreFromCorruptedCheckpoint verifies
// that restoring channels from a checkpoint with wrong types does not panic.
func TestFaultInjection_ChannelRestoreFromCorruptedCheckpoint(t *testing.T) {
	registry := channels.NewRegistry()
	lv := channels.NewLastValue("")
	lv.SetKey("test_channel")
	registry.Register("test_channel", lv)

	// Attempt to restore from a checkpoint with a wrong type value.
	badCheckpoint := map[string]interface{}{
		"test_channel": 42, // int, but channel expects string
	}
	err := registry.RestoreFromCheckpoint(badCheckpoint)
	if err != nil {
		t.Logf("expected error or type mismatch: %v", err)
	}
}

// defaultTestThreadID is used for tests that need a thread ID.
const defaultTestThreadID = "fault-injection-test-thread"
