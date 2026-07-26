// Package graph provides advanced fault injection edge cases.
package graph

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
)

// ============================================================
// P0: Node returns different key than channel
// ============================================================

// TestFault_NodeReturnsUnknownKey verifies node returning a key not in channels.
func TestFault_NodeReturnsUnknownKey(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("producer", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"unknown_key": "value"}, nil
	})
	b.AddNode("consumer", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	b.AddEdge(constants.Start, "producer")
	b.AddEdge("producer", "consumer")
	b.AddEdge("consumer", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	// Should not panic — unknown keys are ignored.
	_, err = cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
}

// ============================================================
// P0: Graph with single node (minimal)
// ============================================================

// TestFault_SingleNodeGraph verifies a graph with exactly one node.
func TestFault_SingleNodeGraph(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("only", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["result"] = "single"
		return m, nil
	})
	b.AddEdge(constants.Start, "only")
	b.AddEdge("only", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	if m["result"] != "single" {
		t.Fatalf("expected result=single, got %v", m)
	}
}

// ============================================================
// P1: Node returns large string
// ============================================================

// TestFault_LargeReturnValue verifies nodes returning large strings.
func TestFault_LargeReturnValue(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("big", func(ctx context.Context, state any) (any, error) {
		large := ""
		for i := 0; i < 10000; i++ {
			large += "x"
		}
		return map[string]any{"data": large}, nil
	})
	b.AddNode("small", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["done"] = true
		return m, nil
	})
	b.AddEdge(constants.Start, "big")
	b.AddEdge("big", "small")
	b.AddEdge("small", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	if m["done"] != true {
		t.Fatalf("expected done=true, got %v", m)
	}
}

// ============================================================
// P1: Chain with deep state mutation
// ============================================================

// TestFault_DeepStateMutation verifies a chain that accumulates state.
func TestFault_DeepStateMutation(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	prev := constants.Start
	for i := 0; i < 20; i++ {
		name := fmt.Sprintf("n_%d", i)
		b.AddNode(name, func(ctx context.Context, state any) (any, error) {
			m := state.(map[string]any)
			if v, ok := m["depth"]; ok {
				m["depth"] = v.(int) + 1
			} else {
				m["depth"] = 1
			}
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

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	if m["depth"].(int) != 20 {
		t.Fatalf("expected depth=20, got %v", m["depth"])
	}
}

// ============================================================
// P2: Branching with conditional on value not present
// ============================================================

// TestFault_ConditionalEdge_MissingKey verifies conditional routing
// when the routing key is missing from state.
func TestFault_ConditionalEdge_MissingKey(t *testing.T) {
	t.Skip("requires Pregel engine - see pregel/ for equivalent tests")
	b := NewStateGraph(map[string]any{})
	b.AddNode("router", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	b.AddNode("default", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["route"] = "default"
		return m, nil
	})
	b.AddEdge(constants.Start, "router")
	b.AddConditionalEdges("router",
		func(ctx context.Context, state any) (any, error) {
			m := state.(map[string]any)
			if v, ok := m["route"]; ok {
				return v, nil
			}
			return "default", nil
		},
		map[string]string{
			"default": "default",
		},
	)
	b.AddEdge("default", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	if m["route"] != "default" {
		t.Fatalf("expected route=default, got %v", m)
	}
}

// ============================================================
// P2: Checkpoint with concurrent Put/List
// ============================================================

// TestFault_ConcurrentPutList verifies concurrent Put+List on same thread.
func TestFault_ConcurrentPutList(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	ctx := context.Background()
	tid := "cp-conc-put-list"
	cfg := map[string]interface{}{constants.ConfigKeyThreadID: tid}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			err := ms.Put(ctx, cfg, map[string]interface{}{"i": idx})
			if err != nil {
				t.Errorf("Put: %v", err)
			}
		}(i)
	}
	wg.Wait()

	// List should return at least some entries.
	entries, err := ms.List(ctx, cfg, 50)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}
}

// ============================================================
// P2: 100 invocations of same graph (stress)
// ============================================================

// TestFault_100SequentialInvocations invokes the same graph 100 times.
func TestFault_100SequentialInvocations(t *testing.T) {
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

	ctx := context.Background()
	for i := 0; i < 100; i++ {
		_, err := cg.Invoke(ctx, map[string]any{"i": i})
		if err != nil {
			t.Fatalf("invocation %d: %v", i, err)
		}
	}
}

// ============================================================
// P2: Reducer with append across parallel branches
// ============================================================

// TestFault_ParallelAppend verifies parallel branches appending to
// the same slice (sequential chain simulates this).
func TestFault_ParallelAppend(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddNode("a", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["items"] = []string{"a"}
		return m, nil
	})
	b.AddNode("b", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		items, _ := m["items"].([]string)
		m["items"] = append(items, "b")
		return m, nil
	})
	b.AddEdge(constants.Start, "a")
	b.AddEdge("a", "b")
	b.AddEdge("b", constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	items, ok := m["items"].([]string)
	if !ok || len(items) != 2 {
		t.Fatalf("expected 2 items, got %v", m["items"])
	}
}

// ============================================================
// P2: EphemeralValue with engine pattern
// ============================================================

// TestFault_AnyValueChannel verifies AnyValue channel.
func TestFault_AnyValueChannel(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	b.AddChannel("any", channels.NewAnyValue(""))

	b.AddNode("writer", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"any": 42}, nil
	})
	b.AddNode("reader", func(ctx context.Context, state any) (any, error) {
		m := state.(map[string]any)
		m["read"] = true
		return m, nil
	})
	b.AddEdge(constants.Start, "writer")
	b.AddEdge("writer", "reader")
	b.AddEdge("reader", constants.End)

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
// P2: Enum-like state values
// ============================================================

// TestFault_EnumStateValue verifies state transitions through phases.
func TestFault_EnumStateValue(t *testing.T) {
	b := NewStateGraph(map[string]any{})
	phases := []string{"init", "process", "finalize", "done"}
	prev := constants.Start
	for _, phase := range phases {
		p := phase
		b.AddNode(p, func(ctx context.Context, state any) (any, error) {
			m := state.(map[string]any)
			m["phase"] = p
			return m, nil
		})
		b.AddEdge(prev, p)
		prev = p
	}
	b.AddEdge(prev, constants.End)

	cg, err := b.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	result, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	m := result.(map[string]any)
	if m["phase"] != "done" {
		t.Fatalf("expected phase=done, got %v", m["phase"])
	}
}
