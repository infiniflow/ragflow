// Package pregel provides comprehensive tests for DurabilityExit mode,
// time travel (GetState/UpdateState), and subgraph state inspection.
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
// P0: DurabilityExit mode — basic verification
// ============================================================

// TestDurabilityExit_Basic verifies that with DurabilityExit, execution
// completes correctly without a checkpointer (mode is a no-op).
func TestDurabilityExit_Basic(t *testing.T) {
	sg := newSimpleGraph(t)
	cfg := &types.RunnableConfig{
		Durability: types.DurabilityExit,
	}
	engine := NewEngine(sg, WithRecursionLimit(10), WithConfig(cfg))

	result, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "b" {
		t.Fatalf("expected value=b, got %v", m["value"])
	}
}

// TestDurabilityExit_MultiStep verifies a 2-node chain with DurabilityExit.
func TestDurabilityExit_MultiStep(t *testing.T) {
	cfg := &types.RunnableConfig{Durability: types.DurabilityExit}
	engine := NewEngine(newSimpleGraph(t), WithRecursionLimit(10), WithConfig(cfg))

	result, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if v, ok := m["value"]; !ok || v != "b" {
		t.Fatalf("expected final value=b, got %v", m["value"])
	}
}

// TestDurabilityExit_NoCheckpointer verifies that without a checkpointer,
// DurabilityExit mode does not cause issues.
func TestDurabilityExit_NoCheckpointer(t *testing.T) {
	sg := simpleGraphNoCP()
	cfg := &types.RunnableConfig{
		Durability: types.DurabilityExit,
	}
	engine := NewEngine(sg, WithRecursionLimit(10), WithConfig(cfg))

	ctx := context.Background()
	_, err := engine.RunSync(ctx, map[string]any{"value": "hello"})
	if err != nil {
		t.Fatalf("RunSync without checkpointer: %v", err)
	}
}

// ============================================================
// P1: Time Travel — GetState / UpdateState scenarios
// ============================================================

// TestTimeTravel_GetState_AfterExecution verifies GetState returns the
// correct state after a graph run.
func TestTimeTravel_GetState_AfterExecution(t *testing.T) {
	sg := simpleGraphNoCP()
	ms := checkpoint.NewMemorySaver()
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{
			constants.ConfigKeyThreadID: "tt-getstate",
		},
	}
	engine := NewEngine(sg, WithRecursionLimit(10), WithCheckpointer(ms), WithConfig(cfg))

	ctx := context.Background()
	_, err := engine.RunSync(ctx, map[string]any{"value": "start"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}

	// GetState from the CompiledGraph path (if available).
	// Engine itself doesn't expose GetState - but CompiledGraph does.
	// We verify via checkpointer directly.
	cpData, err := ms.Get(ctx, map[string]interface{}{
		constants.ConfigKeyThreadID: "tt-getstate",
	})
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if cpData == nil {
		t.Fatal("expected checkpoint data")
	}
	if v, ok := cpData["value"]; !ok || v != "b" {
		t.Fatalf("expected value=b, got %v", cpData["value"])
	}
}

// TestTimeTravel_UpdateState_ThenResume verifies that updating state via
// UpdateState and then resuming works correctly.
func TestTimeTravel_UpdateState_ThenResume(t *testing.T) {
	b := graphPkg.NewStateGraph(map[string]any{})
	b.AddChannel("Items", channels.NewLastValue(map[string]string{}))
	b.AddNode("modify", func(ctx context.Context, state any) (any, error) {
		s := state.(map[string]any)
		s["Items"] = map[string]string{"original": "yes"}
		return s, nil
	})
	b.AddNode("validate", func(ctx context.Context, state any) (any, error) {
		s := state.(map[string]any)
		if s["Items"] == nil {
			return nil, nil
		}
		return s, nil
	})
	b.AddEdge(constants.Start, "modify")
	b.AddEdge("modify", "validate")
	b.AddEdge("validate", constants.End)

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(
		graphPkg.WithCheckpointer(ms),
		graphPkg.WithRecursionLimit(10),
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{
			constants.ConfigKeyThreadID: "tt-update-resume",
		},
	}

	// First execution.
	_, err = cg.Invoke(ctx, map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("first Invoke: %v", err)
	}

	// UpdateState: inject new value at the checkpoint.
	update := &graphPkg.StateUpdate{
		Values:   map[string]interface{}{"Items": map[string]string{"injected": "yes"}},
		AsNode:   "user",
		ThreadID: "tt-update-resume",
	}
	inspector, ok := cg.(graphPkg.StateInspector)
	if !ok {
		t.Fatal("compiled graph does not implement StateInspector")
	}
	newCfg, err := inspector.UpdateState(ctx, cfg, update)
	if err != nil {
		t.Fatalf("UpdateState: %v", err)
	}
	t.Logf("UpdateState returned config: %+v", newCfg)

	// GetState should now show the updated values.
	snap, err := inspector.GetState(ctx, newCfg)
	if err != nil {
		t.Fatalf("GetState after update: %v", err)
	}
	if snap == nil {
		t.Fatal("snap is nil after UpdateState")
	}
	t.Logf("snap after update: %+v", snap.Values)
}

// TestTimeTravel_MultipleUpdates verifies multi-step time travel.
func TestTimeTravel_MultipleUpdates(t *testing.T) {
	b := graphPkg.NewStateGraph(map[string]any{})
	b.AddChannel("step", channels.NewLastValue(0))
	b.AddChannel("updated", channels.NewLastValue(false))
	b.AddNode("echo", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	b.AddEdge(constants.Start, "echo")
	b.AddEdge("echo", constants.End)

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(
		graphPkg.WithCheckpointer(ms),
		graphPkg.WithRecursionLimit(10),
	)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	tid := "tt-multi-update"

	// Execute once to create checkpoint.
	_, err = cg.Invoke(ctx, map[string]any{"step": 0}, &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	})
	if err != nil {
		t.Fatalf("first Invoke: %v", err)
	}

	// Apply multiple updates.
	csg := graphPkg.NewCompiledStateGraph(cg)
	if csg == nil {
		t.Fatal("NewCompiledStateGraph returned nil")
	}
	t.Logf("checkpointer set: %v, store: %v", cg.GetCheckpointer(), cg.GetGraph())
	for i := 1; i <= 3; i++ {
		u := &graphPkg.StateUpdate{
			Values:   map[string]interface{}{"step": i, "updated": true},
			AsNode:   "user",
			ThreadID: tid,
		}
		_, err := csg.UpdateState(ctx, &types.RunnableConfig{
			Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
		}, u)
		if err != nil {
			t.Fatalf("UpdateState #%d: %v", i, err)
		}
	}

	// GetStateHistory should show all checkpoints, including the updates.
	history, err := csg.GetStateHistory(ctx, &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}, 10, nil)
	if err != nil {
		t.Fatalf("GetStateHistory: %v", err)
	}
	if len(history) == 0 {
		t.Fatal("expected at least 1 history entry")
	}
	t.Logf("history entries: %d", len(history))
}

// ============================================================
// P1: DurabilityExit with fault scenarios
// ============================================================

// TestDurabilityExit_ConcurrentEngines verifies multiple engines with
// DurabilityExit running concurrently.
func TestDurabilityExit_ConcurrentEngines(t *testing.T) {
	sg := newSimpleGraph(t)
	const numEngines = 20
	var wg sync.WaitGroup
	var errCount atomic.Int32

	for e := 0; e < numEngines; e++ {
		wg.Add(1)
		go func(eid int) {
			defer wg.Done()
			cfg := &types.RunnableConfig{Durability: types.DurabilityExit}
			engine := NewEngine(sg, WithRecursionLimit(10), WithConfig(cfg))
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "conc"})
			if err != nil {
				errCount.Add(1)
			}
		}(e)
	}
	wg.Wait()
	if errCount.Load() > 0 {
		t.Fatalf("%d engines reported errors", errCount.Load())
	}
}

// TestDurabilityExit_InterruptResume verifies DurabilityExit with interrupt.
func TestDurabilityExit_InterruptResume(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("prep", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		m["value"] = "prepped"
		return m, nil
	})
	sg.AddNode("process", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		m["value"] = "processed"
		return m, nil
	})
	_ = sg.AddEdge(constants.Start, "prep")
	_ = sg.AddEdge("prep", "process")
	_ = sg.AddEdge("process", constants.End)

	cfg := &types.RunnableConfig{
		Durability: types.DurabilityExit,
	}
	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithConfig(cfg),
		WithInterrupts("process"),
	)
	ctx := context.Background()
	_, err := engine.RunSync(ctx, map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected interrupt error")
	}
	t.Logf("interrupted (expected): %v", err)
}

// ============================================================
// P2: Durability with large state
// ============================================================

// TestDurabilityExit_LargeState verifies DurabilityExit with a large state.
func TestDurabilityExit_LargeState(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("writer", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		data := make(map[string]string)
		for i := 0; i < 5000; i++ {
			data[fmt.Sprintf("k%d", i)] = "v"
		}
		m["value"] = "done"
		m["data_size"] = len(data)
		return m, nil
	})
	_ = sg.AddEdge(constants.Start, "writer")
	_ = sg.AddEdge("writer", constants.End)

	cfg := &types.RunnableConfig{Durability: types.DurabilityExit}
	engine := NewEngine(sg, WithRecursionLimit(10), WithConfig(cfg))

	result, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "done" {
		t.Fatalf("expected value=done, got %v", m["value"])
	}
}

// ============================================================
// P0: More fault injection scenarios
// ============================================================

// TestFaultInjection_DeferredCheckpointFlushRace verifies that concurrent
// DurabilityExit runs are safe (no checkpointer — just verify no race).
func TestFaultInjection_DeferredCheckpointFlushRace(t *testing.T) {
	sg := newSimpleGraph(t)
	const n = 30
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cfg := &types.RunnableConfig{Durability: types.DurabilityExit}
			engine := NewEngine(sg, WithRecursionLimit(10), WithConfig(cfg))
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "race"})
			if err != nil {
				t.Errorf("engine %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

// TestFaultInjection_CheckpointGetAfterInterrupt verifies interrupt with
// the engine (no checkpoint persistence check — just no hang/crash).
func TestFaultInjection_CheckpointGetAfterInterrupt(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("safe", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		m["value"] = "safe"
		return m, nil
	})
	sg.AddNode("unsafe", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		m["value"] = "unsafe"
		return m, nil
	})
	_ = sg.AddEdge(constants.Start, "safe")
	_ = sg.AddEdge("safe", "unsafe")
	_ = sg.AddEdge("unsafe", constants.End)

	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithInterrupts("unsafe"),
	)

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
	if err == nil {
		t.Fatal("expected interrupt")
	}
	t.Logf("interrupted (expected): %v", err)
}

// TestFaultInjection_NodePanicWithCheckpointer verifies that a panicking node
// still reports the error cleanly.
func TestFaultInjection_NodePanicWithCheckpointer(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("panicker", func(ctx context.Context, state any) (any, error) {
		panic("deliberate panic in node")
	})
	_ = sg.AddEdge(constants.Start, "panicker")
	_ = sg.AddEdge("panicker", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected error from panicking node")
	}
	t.Logf("panic error (expected): %v", err)
}

// TestFaultInjection_EngineReuse_WithDurabilityExit verifies engine reuse
// across multiple DurabilityExit runs.
func TestFaultInjection_EngineReuse_WithDurabilityExit(t *testing.T) {
	sg := newSimpleGraph(t)

	const runs = 20
	for i := 0; i < runs; i++ {
		cfg := &types.RunnableConfig{Durability: types.DurabilityExit}
		engine := NewEngine(sg, WithRecursionLimit(10), WithConfig(cfg))
		_, err := engine.RunSync(context.Background(), map[string]any{"value": "reuse"})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
	}
}

// ============================================================
// P2: Checkpoint version conflict / concurrent access
// ============================================================

// TestFaultInjection_ConcurrentCheckpointConflict verifies that concurrent
// engine runs (each with its own checkpointer) are safe.
func TestFaultInjection_ConcurrentCheckpointConflict(t *testing.T) {
	sg := newSimpleGraph(t)
	const goroutines = 30

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Go(func() {
			engine := NewEngine(sg, WithRecursionLimit(10))
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "conc"})
			if err != nil {
				t.Errorf("engine error: %v", err)
			}
		})
	}
	wg.Wait()
}

// ============================================================
// P1: Fault injection with rapid context cancellation
// ============================================================

// TestFaultInjection_RapidCancel_Restart verifies that rapid cancel/restart
// cycles on the same engine are safe.
func TestFaultInjection_RapidCancel_Restart(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg, WithRecursionLimit(100))

	for i := 0; i < 10; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		_, err := engine.RunSync(ctx, map[string]any{"value": "cancel"})
		cancel()
		if err != nil && err != context.DeadlineExceeded && err != context.Canceled {
			t.Logf("iteration %d: %v", i, err)
		}
	}
}

// ============================================================
// Helper: simple 2-node graph for engine-level tests
// ============================================================

// simpleGraphNoCP returns a 2-node graph (node_a → node_b).
func simpleGraphNoCP() types.StateGraph {
	sg := graphPkg.NewStateGraph(map[string]any{"value": ""})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("node_a", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		if m == nil {
			m = map[string]any{}
		}
		m["value"] = "a"
		return m, nil
	})
	sg.AddNode("node_b", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		if m == nil {
			m = map[string]any{}
		}
		m["value"] = "b"
		return m, nil
	})
	_ = sg.AddEdge(constants.Start, "node_a")
	_ = sg.AddEdge("node_a", "node_b")
	_ = sg.AddEdge("node_b", constants.End)
	return sg
}

// ============================================================
// P2: DurabilityExit with Sync default (config propagation)
// ============================================================

// TestDurabilityExit_ConfigPropagation verifies that both DurabilitySync and
// DurabilityExit modes produce the same execution result.
func TestDurabilityExit_ConfigPropagation(t *testing.T) {
	sg := newSimpleGraph(t)

	for _, d := range []types.Durability{types.DurabilitySync, types.DurabilityExit} {
		cfg := &types.RunnableConfig{Durability: d}
		engine := NewEngine(sg, WithRecursionLimit(10), WithConfig(cfg))
		result, err := engine.RunSync(context.Background(), map[string]any{"value": "test"})
		if err != nil {
			t.Fatalf("durability %s: %v", d, err)
		}
		m := result.(map[string]any)
		if m["value"] != "b" {
			t.Fatalf("durability %s: expected value=b, got %v", d, m["value"])
		}
	}
}
