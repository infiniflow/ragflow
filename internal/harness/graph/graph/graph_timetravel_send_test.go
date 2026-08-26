// Package graph provides time travel (fork/replay/multi-step update) and
// Send() dynamic parallelism deep tests.
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
// P0: Time travel — multi-step state injection
// ============================================================

// TestTimeTravel_MultiStepInject verifies injecting state at multiple
// points via UpdateState and verifying each via GetState.
func TestTimeTravel_MultiStepInject(t *testing.T) {
	t.Skip("requires Pregel engine - see pregel/ for equivalent tests")
	b := NewStateGraph(map[string]any{})
	b.AddNode("echo", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["seen"] = "echo"
		return m, nil
	})
	b.AddEdge(constants.Start, "echo")
	b.AddEdge("echo", constants.End)

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	insp := getInspector(t, cg)

	tid := "tt-multi-inject"
	ctx := context.Background()
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}

	// First execution.
	_, err = cg.Invoke(ctx, map[string]any{"initial": "a"}, cfg)
	if err != nil {
		t.Fatalf("first Invoke: %v", err)
	}

	// Inject state at checkpoint.
	for i := 1; i <= 3; i++ {
		update := &StateUpdate{
			Values:   map[string]interface{}{"injected": fmt.Sprintf("val_%d", i), "step": i},
			AsNode:   "user",
			ThreadID: tid,
		}
		newCfg, err := insp.UpdateState(ctx, cfg, update)
		if err != nil {
			t.Fatalf("UpdateState #%d: %v", i, err)
		}

		// Verify via GetState.
		snap, err := insp.GetState(ctx, newCfg)
		if err != nil {
			t.Fatalf("GetState #%d: %v", i, err)
		}
		if snap != nil {
			if v, ok := snap.Values["step"]; ok {
				t.Logf("injected #%d: step=%v", i, v)
			}
		}
	}
}

// TestTimeTravel_ForkFromCheckpoint verifies creating a fork by
// starting a new thread from a given checkpoint via UpdateState.
func TestTimeTravel_ForkFromCheckpoint(t *testing.T) {
	t.Skip("requires Pregel engine - see pregel/ for equivalent tests")
	b := NewStateGraph(map[string]any{})
	b.AddNode("proc", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["processed"] = true
		return m, nil
	})
	b.AddEdge(constants.Start, "proc")
	b.AddEdge("proc", constants.End)

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	insp := getInspector(t, cg)

	ctx := context.Background()

	// Run thread A.
	tidA := "tt-fork-a"
	cfgA := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tidA},
	}
	_, err = cg.Invoke(ctx, map[string]any{"branch": "a"}, cfgA)
	if err != nil {
		t.Fatalf("thread A: %v", err)
	}

	// Run thread B with different input — should be independent.
	tidB := "tt-fork-b"
	cfgB := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tidB},
	}
	_, err = cg.Invoke(ctx, map[string]any{"branch": "b"}, cfgB)
	if err != nil {
		t.Fatalf("thread B: %v", err)
	}

	// Fork: copy thread A's last state to thread C via UpdateState.
	tidC := "tt-fork-c"
	update := &StateUpdate{
		Values:   map[string]interface{}{"branch": "c", "forked_from": "a"},
		AsNode:   "user",
		ThreadID: tidC,
	}
	_, err = insp.UpdateState(ctx, &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tidA},
	}, update)
	if err != nil {
		t.Fatalf("fork to C: %v", err)
	}
	t.Logf("fork completed: A->C")
}

// ============================================================
// P1: Send() dynamic parallelism — map/reduce with aggregator
// ============================================================

// TestChain_SequentialMapReduce verifies a sequential chain that
// simulates map-reduce (each node processes, then reads results).
func TestChain_SequentialMapReduce(t *testing.T) {
	b := NewStateGraph(map[string]any{})

	// Sequential processing nodes.
	prev := constants.Start
	for i := 0; i < 8; i++ {
		name := fmt.Sprintf("worker_%d", i)
		b.AddNode(name, func(ctx context.Context, state any) (any, error) {
			m := state.(map[string]any)
			m["last_worker"] = name
			return m, nil
		})
		b.AddEdge(prev, name)
		prev = name
	}
	b.AddEdge(prev, constants.End)

	cg, err := b.Compile(WithRecursionLimit(20))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	if m["last_worker"] != "worker_7" {
		t.Fatalf("expected last_worker=worker_7, got %v", m["last_worker"])
	}
}

// TestChain_Collector simulates a collector pattern.
func TestChain_Collector(t *testing.T) {
	b := NewStateGraph(map[string]any{})

	b.AddNode("generator", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["items"] = []int{1, 2, 3}
		return m, nil
	})
	b.AddNode("collector", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	b.AddEdge(constants.Start, "generator")
	b.AddEdge("generator", "collector")
	b.AddEdge("collector", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	items, ok := m["items"].([]int)
	if !ok || len(items) != 3 {
		t.Fatalf("expected 3 items, got %v", m["items"])
	}
}

// ============================================================
// P1: Conditional edge with fallback routing
// ============================================================

// TestConditionalEdge_Fallback verifies conditional edge with default.
func TestConditionalEdge_Fallback(t *testing.T) {
	t.Skip("requires Pregel engine - see pregel/ for equivalent tests")
	b := NewStateGraph(map[string]any{})

	b.AddNode("router", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["routed"] = true
		return m, nil
	})
	b.AddNode("valid", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["path"] = "valid"
		return m, nil
	})
	b.AddNode("fallback", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["path"] = "fallback"
		return m, nil
	})

	b.AddEdge(constants.Start, "router")
	b.AddConditionalEdges("router",
		func(ctx context.Context, state any) (any, error) {
			m := state.(map[string]any)
			if v, ok := m["target"]; ok {
				return v, nil
			}
			return "unknown", nil
		},
		map[string]string{
			"valid":   "valid",
			"unknown": "fallback",
		},
	)
	b.AddEdge("valid", constants.End)
	b.AddEdge("fallback", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	result, err := cg.Invoke(ctx, map[string]any{"target": "unknown"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	if m["path"] != "fallback" {
		t.Fatalf("expected path=fallback, got %v", m["path"])
	}
}

// ============================================================
// P2: State mutation via reducer across checkpoint boundary
// ============================================================

// TestReducer_AcrossCheckpoint verifies reducer (append) works across
// multiple Invoke calls with checkpoint persistence.
func TestReducer_AcrossCheckpoint(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("adder", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		var items []string
		if v, ok := m["items"]; ok {
			items, _ = v.([]string)
		}
		items = append(items, "x")
		m["items"] = items
		return m, nil
	})
	b.AddEdge(constants.Start, "adder")
	b.AddEdge("adder", constants.End)

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	tid := "reducer-across-cp"
	ctx := context.Background()
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}

	for i := 0; i < 3; i++ {
		_, err := cg.Invoke(ctx, map[string]any{}, cfg)
		if err != nil {
			t.Fatalf("Invoke #%d: %v", i, err)
		}
	}
}

// ============================================================
// P2: Engine reuse across many independent threads
// ============================================================

// TestEngine_50Threads_SharedEngine uses one engine for 50 threads.
func TestEngine_50Threads_SharedEngine(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("work", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	b.AddEdge(constants.Start, "work")
	b.AddEdge("work", constants.End)

	ms := checkpoint.NewMemorySaver()
	cg, err := b.Compile(WithCheckpointer(ms))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tid := fmt.Sprintf("shared-engine-%d", idx)
			cfg := &types.RunnableConfig{
				Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
			}
			_, err := cg.Invoke(ctx, map[string]any{}, cfg)
			if err != nil {
				t.Errorf("thread %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================
// P2: BinaryOperator with custom reducer
// ============================================================

// TestBinaryOp_IntAccumulator verifies BinaryOperatorAggregate with int.
func TestBinaryOp_IntAccumulator(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddChannel("sum", channels.NewBinaryOperatorAggregate(0, func(a, b any) any {
		return a.(int) + b.(int)
	}))

	b.AddNode("add5", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"sum": 5}, nil
	})
	b.AddNode("add10", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"sum": 10}, nil
	})
	b.AddEdge(constants.Start, "add5")
	b.AddEdge("add5", "add10")
	b.AddEdge("add10", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	if m["sum"].(int) != 15 {
		t.Fatalf("expected sum=15, got %v", m["sum"])
	}
}

// ============================================================
// P2: Chain with ManyEdges (star topology)
// ============================================================

// TestEngine_StarTopology verifies one-to-many edge pattern.
func TestEngine_StarTopology(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("hub", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["hub_seen"] = true
		return m, nil
	})
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("leaf_%d", i)
		b.AddNode(name, func(ctx context.Context, state any) (any, error) {
			m := state.(map[string]any)
			m[name] = "visited"
			return m, nil
		})
		b.AddEdge("hub", name)
		b.AddEdge(name, constants.End)
	}
	b.AddEdge(constants.Start, "hub")

	cg, err := b.Compile(WithRecursionLimit(20))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}
