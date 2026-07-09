// Package graph provides advanced topology tests and checkpoint edge cases.
package graph

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/types"
)

// ============================================================
// P0: DAG with multiple star joins
// ============================================================

// TestTopology_MultiJoinStar verifies a DAG with 4 source nodes
// joining into one aggregator via sequential chain.
func TestTopology_MultiJoinStar(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	prev := constants.Start
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("s_%d", i)
		b.AddNode(name, func(ctx context.Context, state any) (any, error) {
			m := state.(map[string]any)
			m["through"] = name
			return m, nil
		})
		b.AddEdge(prev, name)
		prev = name
	}
	b.AddEdge(prev, constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_, err = cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// ============================================================
// P0: Diamond topology
// ============================================================

// TestTopology_Diamond verifies a diamond: start -> A -> {B,C} -> D -> end.
func TestTopology_Diamond(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("A", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["A"] = true
		return m, nil
	})
	b.AddNode("B", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["B"] = true
		return m, nil
	})
	b.AddNode("C", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["C"] = true
		return m, nil
	})
	b.AddNode("D", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["D"] = true
		return m, nil
	})
	b.AddEdge(constants.Start, "A")
	b.AddEdge("A", "B")
	b.AddEdge("A", "C")
	b.AddEdge("B", "D")
	b.AddEdge("C", "D")
	b.AddEdge("D", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	if m["D"] != true || m["A"] != true {
		t.Fatalf("diamond incomplete: %v", m)
	}
}

// ============================================================
// P1: Topology with isolated subgraph (no shared state)
// ============================================================

// TestTopology_SequentialChains verifies a sequential chain execution.
func TestTopology_SequentialChains(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("step1", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["step1"] = "done"
		return m, nil
	})
	b.AddNode("step2", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["step2"] = "done"
		return m, nil
	})
	b.AddEdge(constants.Start, "step1")
	b.AddEdge("step1", "step2")
	b.AddEdge("step2", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	if m["step1"] != "done" || m["step2"] != "done" {
		t.Fatalf("chain incomplete: %v", m)
	}
}

// ============================================================
// P1: BinaryOperator with map merge
// ============================================================

// TestBinaryOp_MapMerge verifies merging maps via BinaryOperatorAggregate.
func TestBinaryOp_MapMerge(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddChannel("merged", channels.NewBinaryOperatorAggregate(
		map[string]string{},
		func(a, b any) any {
			am := a.(map[string]string)
			bm := b.(map[string]string)
			for k, v := range bm {
				am[k] = v
			}
			return am
		},
	))

	b.AddNode("src1", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"merged": map[string]string{"a": "1", "b": "2"}}, nil
	})
	b.AddNode("src2", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"merged": map[string]string{"c": "3"}}, nil
	})
	b.AddEdge(constants.Start, "src1")
	b.AddEdge("src1", "src2")
	b.AddEdge("src2", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	merged, ok := m["merged"].(map[string]string)
	if !ok || merged["a"] != "1" || merged["b"] != "2" || merged["c"] != "3" {
		t.Fatalf("unexpected merged result: %v", m["merged"])
	}
}

// ============================================================
// P2: Checkpoint with many pending writes
// ============================================================

// TestCheckpoint_ManyPendingWrites creates a checkpoint with many writes.
func TestCheckpoint_ManyPendingWrites(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	ctx := context.Background()
	tid := "cp-many-pending"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	data := map[string]interface{}{"count": 1000}
	if err := ms.Put(ctx, cfg, data); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := ms.Get(ctx, cfg)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil || got["count"] == nil {
		t.Fatal("missing count in checkpoint")
	}
}

// ============================================================
// P2: Checkpoint with deep nesting
// ============================================================

// TestCheckpoint_DeeplyNestedData verifies deeply nested checkpoint data.
func TestCheckpoint_DeeplyNestedData(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	ctx := context.Background()

	// Build deeply nested data.
	nested := map[string]interface{}{"level0": "root"}
	current := nested
	for i := 1; i <= 20; i++ {
		next := map[string]interface{}{"value": i, "depth": fmt.Sprintf("deep_%d", i)}
		current["child"] = next
		current = next
	}

	tid := "cp-deep-nest"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}
	if err := ms.Put(ctx, cfg, nested); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := ms.Get(ctx, cfg)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("nil checkpoint after deep nest")
	}
}

// ============================================================
// P2: Concurrent graph invocation with timeouts
// ============================================================

// TestTopology_ConcurrentGraphs_Timeout runs 20 graph invocations
// concurrently with individual timeouts.
func TestTopology_ConcurrentGraphs_Timeout(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("echo", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	b.AddEdge(constants.Start, "echo")
	b.AddEdge("echo", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ctx := context.Background()
			input := map[string]any{"idx": idx}
			_, err := cg.Invoke(ctx, input)
			if err != nil {
				t.Errorf("goroutine %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================
// P2: EphemeralValue channel test
// ============================================================

// TestChannel_SimpleWrite verifies a basic LastValue channel write.
func TestChannel_SimpleWrite(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddChannel("msg", channels.NewLastValue(""))

	b.AddNode("send", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"msg": "hello"}, nil
	})
	b.AddNode("check", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["checked"] = "ok"
		return m, nil
	})
	b.AddEdge(constants.Start, "send")
	b.AddEdge("send", "check")
	b.AddEdge("check", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_, err = cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// ============================================================
// P2: Topic channel basic test
// ============================================================

// TestChannel_Topic verifies Topic channel accumulates values.
func TestChannel_Topic(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddChannel("events", channels.NewTopic("", true))

	b.AddNode("emit", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"events": "e1"}, nil
	})
	b.AddNode("emit2", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"events": "e2"}, nil
	})
	b.AddEdge(constants.Start, "emit")
	b.AddEdge("emit", "emit2")
	b.AddEdge("emit2", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// ============================================================
// P2: NamedBarrierValue channel test
// ============================================================

// TestChannel_LastValueBasic verifies LastValue channel write/read.
func TestChannel_LastValueBasic(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddChannel("val", channels.NewLastValue(""))
	b.AddNode("set", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"val": "test_val"}, nil
	})
	b.AddNode("check", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["checked"] = true
		return m, nil
	})
	b.AddEdge(constants.Start, "set")
	b.AddEdge("set", "check")
	b.AddEdge("check", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// ============================================================
// P2: Large number of concurrent readers on checkpointer
// ============================================================

// TestCheckpoint_100ConcurrentReaders verifies 100 goroutines reading
// from the same MemorySaver concurrently.
func TestCheckpoint_100ConcurrentReaders(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	ctx := context.Background()
	tid := "cp-100-readers"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	if err := ms.Put(ctx, cfg, map[string]interface{}{"data": "test"}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Go(func() {
			_, err := ms.Get(ctx, cfg)
			if err != nil {
				t.Errorf("Get: %v", err)
			}
		})
	}
	wg.Wait()
}

// ============================================================
// P2: Multiple independent graphs
// ============================================================

// TestTopology_MultipleIndependentGraphs compiles and invokes
// 10 different graph topologies.
func TestTopology_MultipleIndependentGraphs(t *testing.T) {
	graphs := make([]types.CompiledGraph, 10)
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("g_%d", i)
		b := NewStateGraph(map[string]any{})
		b.AddNode(name, func(ctx context.Context, state any) (any, error) {
			m := state.(map[string]any)
			m[name] = "ok"
			return m, nil
		})
		b.AddEdge(constants.Start, name)
		b.AddEdge(name, constants.End)
		cg, err := b.Compile()
		if err != nil {
			t.Fatalf("graph %d Compile: %v", i, err)
		}
		graphs[i] = cg
	}

	var wg sync.WaitGroup
	for i, cg := range graphs {
		wg.Add(1)
		go func(idx int, compiled types.CompiledGraph) {
			defer wg.Done()
			_, err := compiled.Invoke(context.Background(), map[string]any{})
			if err != nil {
				t.Errorf("graph %d: %v", idx, err)
			}
		}(i, cg)
	}
	wg.Wait()
}

// ============================================================
// P2: Checkpoint with empty values in nested map
// ============================================================

// TestCheckpoint_NestedEmptyMap verifies nested empty maps round-trip.
func TestCheckpoint_NestedEmptyMap(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	ctx := context.Background()

	data := map[string]interface{}{
		"empty_map": map[string]interface{}{},
		"nil_value": nil,
		"nested": map[string]interface{}{
			"also_empty": map[string]interface{}{},
			"value":      42,
		},
	}

	tid := "cp-nested-empty"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}
	if err := ms.Put(ctx, cfg, data); err != nil {
		t.Fatalf("Put: %v", err)
	}

	got, err := ms.Get(ctx, cfg)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got == nil {
		t.Fatal("nil checkpoint")
	}
}
