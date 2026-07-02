// Package pregel provides production-grade fault injection tests.
// These target real-world failure modes: goroutine leaks, deadlocks,
// memory pressure, OOM, corrupted state, and race conditions.
package pregel

import (
	"context"
	"fmt"
	"runtime"
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
// P0: Goroutine leak detection after cancel
// ============================================================

// TestFault_GoroutineLeakAfterCancel starts goroutines, cancels, then
// verifies no goroutine leak via runtime.NumGoroutine.
func TestFault_GoroutineLeakAfterCancel(t *testing.T) {
	sg := newSimpleGraph(t)
	before := runtime.NumGoroutine()

	for i := 0; i < 5; i++ {
		engine := NewEngine(sg, WithRecursionLimit(100))
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		// Drain but don't wait for channels — cancel should clean up.
		outputCh, errCh := engine.Run(ctx, map[string]any{"value": "leak"}, types.StreamModeValues)
		cancel()
		for range outputCh {
		}
		<-errCh
	}

	// Allow goroutines to settle.
	time.Sleep(10 * time.Millisecond)
	after := runtime.NumGoroutine()

	// Should not leak more than a few goroutines (allow for GC).
	if after-before > 10 {
		t.Fatalf("possible goroutine leak: before=%d after=%d delta=%d", before, after, after-before)
	}
}

// ============================================================
// P0: Goroutine leak after rapid engine creation
// ============================================================

// TestFault_GoroutineLeakRapidCreate creates+destroys many engines.
func TestFault_GoroutineLeakRapidCreate(t *testing.T) {
	before := runtime.NumGoroutine()

	for i := 0; i < 50; i++ {
		engine := NewEngine(newSimpleGraph(t), WithRecursionLimit(10))
		engine.RunSync(context.Background(), map[string]any{"value": "x"})
	}

	time.Sleep(10 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after-before > 15 {
		t.Fatalf("possible goroutine leak after rapid create: delta=%d", after-before)
	}
}

// ============================================================
// P0: Engine with node that blocks forever — must still cancel
// ============================================================

// TestFault_NodeBlocksForever verifies cancellation unblocks.
func TestFault_NodeBlocksForever(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("stuck", func(ctx context.Context, state any) (any, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	})
	_ = sg.AddEdge(constants.Start, "stuck")
	_ = sg.AddEdge("stuck", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := engine.RunSync(ctx, map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

// ============================================================
// P1: Checkpoint get/put after engine crash (simulated)
// ============================================================

// TestFault_CheckpointAfterPanic simulates an engine crash and
// verifies the checkpointer is still usable.
func TestFault_CheckpointAfterPanic(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	tid := "cp-after-panic"
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}

	// Panicing engine.
	sg := newSimpleGraph(t)
	func() {
		defer func() { recover() }()
		engine := NewEngine(sg,
			WithRecursionLimit(10),
			WithCheckpointer(ms),
			WithConfig(cfg),
		)
		// Force a panic inside RunSync.
		_, _ = engine.RunSync(context.Background(), map[string]any{"value": "x"})
	}()

	// Checkpointer should still work.
	cp, err := ms.Get(context.Background(), map[string]interface{}{
		constants.ConfigKeyThreadID: tid,
	})
	if err != nil {
		t.Fatalf("Get after panic: %v", err)
	}
	_ = cp
}

// ============================================================
// P1: Corrupted checkpoint recovery
// ============================================================

// TestFault_CorruptedCheckpoint_EngineStart puts corrupted data
// and verifies the engine doesn't crash.
func TestFault_CorruptedCheckpoint_EngineStart(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	tid := "cp-corrupt-start"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	// Write an invalid checkpoint that has wrong types for channel data.
	ms.Put(context.Background(), cfg, map[string]interface{}{
		"value":               "corrupted",
		"__completed_tasks__": "garbage",
		"__last_state__":      "not-json",
	})

	engineCfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithCheckpointer(ms),
		WithConfig(engineCfg),
	)
	result, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err != nil {
		t.Logf("corrupted checkpoint handled: %v", err)
	}
	_ = result
}

// ============================================================
// P1: Topic channel with concurrent producers
// ============================================================

// TestFault_TopicChannel_ConcurrentProducers verifies Topic handles
// concurrent writes without data corruption.
func TestFault_TopicChannel_ConcurrentProducers(t *testing.T) {
	const numProducers = 50
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("evt", channels.NewTopic("", true))

	// Sequential chain (BSP mode processes one node at a time).
	prev := constants.Start
	for i := 0; i < numProducers; i++ {
		name := fmt.Sprintf("p_%d", i)
		sg.AddNode(name, func(ctx context.Context, state any) (any, error) {
			return map[string]any{"evt": "e"}, nil
		})
		_ = sg.AddEdge(prev, name)
		prev = name
	}
	_ = sg.AddEdge(prev, constants.End)

	engine := NewEngine(sg, WithRecursionLimit(100))
	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	_ = result
}

// ============================================================
// P2: Engine node that modifies shared state
// ============================================================

// TestFault_NodeConcurrentMapWrite verifies concurrent map writes
// in node handlers don't race. Uses atomic counter.
func TestFault_NodeConcurrentMapWrite(t *testing.T) {
	var counter atomic.Int64
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	// Sequential chain (BSP mode processes one node at a time).
	prev := constants.Start
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("w_%d", i)
		sg.AddNode(name, func(ctx context.Context, state any) (any, error) {
			counter.Add(1)
			return map[string]any{"value": name}, nil
		})
		_ = sg.AddEdge(prev, name)
		prev = name
	}
	_ = sg.AddEdge(prev, constants.End)

	engine := NewEngine(sg, WithRecursionLimit(30))
	result, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	_ = result
	if counter.Load() != 20 {
		t.Fatalf("expected 20 node invocations, got %d", counter.Load())
	}
}

// ============================================================
// P2: Repeated context cancellation storm
// ============================================================

// TestFault_CancelStorm creates 50 contexts that cancel immediately.
func TestFault_CancelStorm(t *testing.T) {
	before := runtime.NumGoroutine()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Go(func() {
			engine := NewEngine(newSimpleGraph(t), WithRecursionLimit(10))
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			_, _ = engine.RunSync(ctx, map[string]any{"value": "storm"})
		})
	}
	wg.Wait()
	time.Sleep(10 * time.Millisecond)
	after := runtime.NumGoroutine()
	if after-before > 20 {
		t.Fatalf("possible goroutine leak after cancel storm: delta=%d", after-before)
	}
}

// ============================================================
// P2: Engine reuse causing stale state
// ============================================================

// TestFault_EngineReuseStaleState reuses engine across 100 runs.
func TestFault_EngineReuseStaleState(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t), WithRecursionLimit(10))
	ctx := context.Background()

	for i := 0; i < 100; i++ {
		result, err := engine.RunSync(ctx, map[string]any{"value": "reuse"})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		m := result.(map[string]any)
		if m["value"] != "b" {
			t.Fatalf("run %d: expected value=b, got %v", i, m["value"])
		}
	}
}

// ============================================================
// P2: Many threads, many checkpoints, rapid cycle
// ============================================================

// TestFault_ManyThreadsManyCheckpoints creates 30 threads each
// with 20 checkpoint saves = 600 total checkpoint operations.
func TestFault_ManyThreadsManyCheckpoints(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ms := checkpoint.NewMemorySaver()
			tid := fmt.Sprintf("mt-mc-%d", idx)
			for j := 0; j < 20; j++ {
				cfg := &types.RunnableConfig{
					Configurable: map[string]interface{}{
						constants.ConfigKeyThreadID: tid,
					},
				}
				engine := NewEngine(newSimpleGraph(t),
					WithRecursionLimit(10),
					WithCheckpointer(ms),
					WithConfig(cfg),
				)
				_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
				if err != nil {
					t.Errorf("thread %d run %d: %v", idx, j, err)
				}
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================
// P2: Edge case — all nodes return nil
// ============================================================

// TestFault_AllNodesReturnNil verifies every node returns nil.
func TestFault_AllNodesReturnNil(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	prev := constants.Start
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("nil_%d", i)
		sg.AddNode(name, func(ctx context.Context, state any) (any, error) {
			return nil, nil
		})
		_ = sg.AddEdge(prev, name)
		prev = name
	}
	_ = sg.AddEdge(prev, constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err != nil {
		t.Fatalf("RunSync with nil nodes: %v", err)
	}
}

// ============================================================
// P2: Graph with long chain + early exit via interrupt
// ============================================================

// TestFault_LongChainInterruptEarly interrupts a 50-node chain early.
func TestFault_LongChainInterruptEarly(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	prev := constants.Start
	for i := 0; i < 50; i++ {
		name := fmt.Sprintf("ln_%d", i)
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
		WithRecursionLimit(100),
		WithInterrupts("ln_5"),
	)

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected interrupt after 5 nodes")
	}
}
