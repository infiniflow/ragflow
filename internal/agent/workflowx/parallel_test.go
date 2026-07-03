// parallel_test.go — pure logic tests for the parallel extension
// using the harness graph.StateGraph.
package workflowx

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// incSub returns a sub-graph that increments an int input by 1.
func incSub(t *testing.T) types.CompiledGraph {
	t.Helper()
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("inc", incNode())
	sub.SetEntryPoint("inc")
	sub.SetFinishPoint("inc")
	cg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}
	return cg
}

// TestParallel_OrderPreservation_Sequential asserts output order
// matches input order under the default sequential path.
func TestParallel_OrderPreservation_Sequential(t *testing.T) {
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3, 4, 5}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", incSub(t))
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	sg.AddNode("par", pFn)
	sg.SetEntryPoint("prep")
	sg.AddEdge("prep", "par")
	sg.SetFinishPoint("par")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	gotRaw, err := cg.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := gotRaw.([]interface{})
	want := []interface{}{2, 3, 4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestParallel_ItemErrorStopsAndReturns asserts a non-interrupt
// error from one item stops processing and surfaces the error.
func TestParallel_ItemErrorStopsAndReturns(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("op", func(_ context.Context, state interface{}) (interface{}, error) {
		if v, ok := state.(int); ok && v == 2 {
			return nil, errors.New("item-2-fail")
		}
		return state, nil
	})
	sub.SetEntryPoint("op")
	sub.SetFinishPoint("op")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	sg.AddNode("par", pFn)
	sg.SetEntryPoint("prep")
	sg.AddEdge("prep", "par")
	sg.SetFinishPoint("par")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_, err = cg.Invoke(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "item-2-fail") {
		t.Errorf("error %q must contain 'item-2-fail'", err.Error())
	}
}

// TestParallel_PanicRecovery asserts that a panic in a sub-graph
// item is recovered and returned as an error.
func TestParallel_PanicRecovery(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("op", func(_ context.Context, state interface{}) (interface{}, error) {
		if v, ok := state.(int); ok && v == 1 {
			panic("kaboom")
		}
		return state, nil
	})
	sub.SetEntryPoint("op")
	sub.SetFinishPoint("op")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{0, 1, 2}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	sg.AddNode("par", pFn)
	sg.SetEntryPoint("prep")
	sg.AddEdge("prep", "par")
	sg.SetFinishPoint("par")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_, err = cg.Invoke(context.Background(), nil)
	if err == nil {
		t.Fatal("expected panic-as-error, got nil")
	}
	if !strings.Contains(err.Error(), "kaboom") {
		t.Errorf("err %q must contain 'kaboom'", err.Error())
	}
}

// TestParallel_ConcurrentSafety asserts fan-out with concurrency > 1.
func TestParallel_ConcurrentSafety(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("op", func(_ context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})
	sub.SetEntryPoint("op")
	sub.SetFinishPoint("op")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	pFn, err := graph.NewParallelNodeFunc("par", subCg, graph.WithParallelMaxConcurrency(4))
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}

	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		n := 20
		items := make([]interface{}, n)
		for i := 0; i < n; i++ {
			items[i] = i
		}
		return items, nil
	})
	sg.AddNode("par", pFn)
	sg.SetEntryPoint("prep")
	sg.AddEdge("prep", "par")
	sg.SetFinishPoint("par")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	gotRaw, err := cg.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := gotRaw.([]interface{})
	if len(got) != 20 {
		t.Fatalf("len: got %d, want 20", len(got))
	}
	for i := 0; i < 20; i++ {
		if got[i] != i {
			t.Errorf("got[%d] = %v, want %d", i, got[i], i)
		}
	}
}

// TestParallel_EmptyInput_NoSubInvoke asserts empty input returns
// immediately without sub-graph invocation.
func TestParallel_EmptyInput_NoSubInvoke(t *testing.T) {
	var invoked bool
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("op", func(_ context.Context, state interface{}) (interface{}, error) {
		invoked = true
		return state, nil
	})
	sub.SetEntryPoint("op")
	sub.SetFinishPoint("op")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	sg.AddNode("par", pFn)
	sg.SetEntryPoint("prep")
	sg.AddEdge("prep", "par")
	sg.SetFinishPoint("par")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	gotRaw, err := cg.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := gotRaw.([]interface{})
	if len(got) != 0 {
		t.Fatalf("len: got %d, want 0", len(got))
	}
	if invoked {
		t.Error("sub-graph invoked for empty input")
	}
}

// TestParallel_Sequential_ZeroGoroutineSpawns asserts maxConcurrency
// 0 produces correct output.
func TestParallel_Sequential_ZeroGoroutineSpawns(t *testing.T) {
	subCg := incSub(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg,
		graph.WithParallelMaxConcurrency(0),
	)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	sg.AddNode("par", pFn)
	sg.SetEntryPoint("prep")
	sg.AddEdge("prep", "par")
	sg.SetFinishPoint("par")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	gotRaw, err := cg.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := gotRaw.([]interface{})
	want := []interface{}{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestParallel_HighConcurrency asserts many items with high concurrency.
func TestParallel_HighConcurrency(t *testing.T) {
	subCg := incSub(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		n := 100
		items := make([]interface{}, n)
		for i := 0; i < n; i++ {
			items[i] = i
		}
		return items, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg,
		graph.WithParallelMaxConcurrency(10),
	)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	sg.AddNode("par", pFn)
	sg.SetEntryPoint("prep")
	sg.AddEdge("prep", "par")
	sg.SetFinishPoint("par")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	gotRaw, err := cg.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := gotRaw.([]interface{})
	if len(got) != 100 {
		t.Fatalf("len: got %d, want 100", len(got))
	}
	for i := 0; i < 100; i++ {
		v, _ := got[i].(int)
		if v != i+1 {
			t.Errorf("got[%d] = %v, want %d", i, got[i], i+1)
			break
		}
	}
}

// TestParallel_SingleItemOnly asserts a single-item parallel node works.
func TestParallel_SingleItemOnly(t *testing.T) {
	subCg := incSub(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{42}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	sg.AddNode("par", pFn)
	sg.SetEntryPoint("prep")
	sg.AddEdge("prep", "par")
	sg.SetFinishPoint("par")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	gotRaw, err := cg.Invoke(context.Background(), nil)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := gotRaw.([]interface{})
	if len(got) != 1 {
		t.Fatalf("len: got %d, want 1", len(got))
	}
	if v, _ := got[0].(int); v != 43 {
		t.Errorf("got[0] = %v, want 43", got[0])
	}
}
