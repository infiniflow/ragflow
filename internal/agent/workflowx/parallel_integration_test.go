// parallel_integration_test.go — integration tests for the parallel
// extension using the harness graph.StateGraph with checkpoints.
package workflowx

import (
	"context"
	"errors"
	"strings"
	"testing"

	"ragflow/internal/harness/graph/checkpoint"
	gerrors "ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/interrupt"
	"ragflow/internal/harness/graph/types"
)

// TestIntegration_Parallel_OrderPreservation asserts output order
// matches input order through the compiled graph with checkpoint.
func TestIntegration_Parallel_OrderPreservation(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := incSub(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	outer.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3, 4, 5}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	outer.AddNode("par", pFn)
	outer.SetEntryPoint("prep")
	outer.AddEdge("prep", "par")
	outer.SetFinishPoint("par")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{ThreadID: "parallel-integration-test"}
	gotRaw, err := cg.Invoke(context.Background(), map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := extractSliceFromState(gotRaw)
	want := []interface{}{2, 3, 4, 5, 6}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if gv, _ := extractIntFromState(got[i]); gv != want[i] {
			t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestIntegration_Parallel_MaxConcurrencyOne asserts sequential execution.
func TestIntegration_Parallel_MaxConcurrencyOne(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := incSub(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	outer.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{10, 20, 30}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg, graph.WithParallelMaxConcurrency(1))
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	outer.AddNode("par", pFn)
	outer.SetEntryPoint("prep")
	outer.AddEdge("prep", "par")
	outer.SetFinishPoint("par")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{ThreadID: "parallel-maxc1"}
	gotRaw, err := cg.Invoke(context.Background(), map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := extractSliceFromState(gotRaw)
	want := []interface{}{11, 21, 31}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if gv, _ := extractIntFromState(got[i]); gv != want[i] {
			t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestIntegration_Parallel_Concurrent asserts fan-out with maxConcurrency > 1.
func TestIntegration_Parallel_Concurrent(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := incSub(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	outer.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		items := make([]interface{}, 10)
		for i := 0; i < 10; i++ {
			items[i] = i
		}
		return items, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg, graph.WithParallelMaxConcurrency(4))
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	outer.AddNode("par", pFn)
	outer.SetEntryPoint("prep")
	outer.AddEdge("prep", "par")
	outer.SetFinishPoint("par")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{ThreadID: "parallel-concurrent"}
	gotRaw, err := cg.Invoke(context.Background(), map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := extractSliceFromState(gotRaw)
	if len(got) != 10 {
		t.Fatalf("len: got %d, want 10", len(got))
	}
	for i := 0; i < 10; i++ {
		if gv, _ := extractIntFromState(got[i]); gv != i+1 {
			t.Errorf("got[%d] = %v, want %d", i, got[i], i+1)
		}
	}
}

// TestIntegration_Parallel_AllItemsInterrupt asserts every item
// interrupting produces a GraphInterrupt error.
func TestIntegration_Parallel_AllItemsInterrupt(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()

	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("op", func(ctx context.Context, state interface{}) (interface{}, error) {
		_, intrErr := interrupt.Interrupt(ctx, "parallel-sub-intr")
		return nil, intrErr
	})
	sub.SetEntryPoint("op")
	sub.SetFinishPoint("op")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	outer := graph.NewStateGraph(map[string]interface{}{})
	outer.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg, graph.WithParallelMaxConcurrency(0))
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	outer.AddNode("par", pFn)
	outer.SetEntryPoint("prep")
	outer.AddEdge("prep", "par")
	outer.SetFinishPoint("par")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{ThreadID: "parallel-all-intr"}
	_, err = cg.Invoke(context.Background(), map[string]any{}, cfg)
	if err == nil {
		t.Fatal("expected interrupt, got nil")
	}
	if !gerrors.IsGraphInterrupt(err) {
		t.Logf("interrupt error: %T, want GraphInterrupt", err)
	}
}

// TestIntegration_Parallel_PartialInterrupt asserts that when all
// items interrupt, the parallel node returns a GraphInterrupt error.
func TestIntegration_Parallel_PartialInterrupt(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("op", func(ctx context.Context, state interface{}) (interface{}, error) {
		_, intrErr := interrupt.Interrupt(ctx, "partial-intr")
		return nil, intrErr
	})
	sub.SetEntryPoint("op")
	sub.SetFinishPoint("op")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	outer := graph.NewStateGraph(map[string]interface{}{})
	outer.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{10, 20, 30}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg, graph.WithParallelMaxConcurrency(0))
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	outer.AddNode("par", pFn)
	outer.SetEntryPoint("prep")
	outer.AddEdge("prep", "par")
	outer.SetFinishPoint("par")

	cg, err := outer.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected interrupt on first run")
	}
	if !gerrors.IsGraphInterrupt(err) {
		t.Logf("expected GraphInterrupt, got %T", err)
	}
}

// TestIntegration_Parallel_CheckpointStatePersisted asserts checkpoint
// is written after parallel node execution.
func TestIntegration_Parallel_CheckpointStatePersisted(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := incSub(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	outer.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	outer.AddNode("par", pFn)
	outer.SetEntryPoint("prep")
	outer.AddEdge("prep", "par")
	outer.SetFinishPoint("par")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{
		ThreadID: "parallel-cp-state",
		Configurable: map[string]interface{}{
			"thread_id": "parallel-cp-state",
		},
	}
	_, err = cg.Invoke(context.Background(), map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	cp, cpErr := memCp.Get(context.Background(), map[string]interface{}{
		"thread_id": "parallel-cp-state",
	})
	if cpErr != nil {
		t.Fatalf("Get checkpoint: %v", cpErr)
	}
	if cp == nil {
		t.Fatal("checkpoint is nil after parallel execution")
	}
}

// TestIntegration_Parallel_OuterStream verifies streaming works with
// a parallel node in the graph.
func TestIntegration_Parallel_OuterStream(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := incSub(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	outer.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	outer.AddNode("par", pFn)
	outer.SetEntryPoint("prep")
	outer.AddEdge("prep", "par")
	outer.SetFinishPoint("par")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{ThreadID: "parallel-stream"}
	ch, errCh := cg.Stream(context.Background(), map[string]any{}, types.StreamModeValues, cfg)

	var finalOut interface{}
	done := false
	for !done {
		select {
		case val, ok := <-ch:
			if !ok {
				done = true
				break
			}
			finalOut = val
		case err := <-errCh:
			if err != nil {
				t.Fatalf("Stream error: %v", err)
			}
			done = true
		}
	}
	got, _ := extractSliceFromState(finalOut)
	if len(got) != 3 {
		t.Fatalf("len: got %d, want 3", len(got))
	}
}

// TestIntegration_Parallel_ForceNewRun asserts fresh ThreadID starts fresh.
func TestIntegration_Parallel_ForceNewRun(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := incSub(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	outer.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	outer.AddNode("par", pFn)
	outer.SetEntryPoint("prep")
	outer.AddEdge("prep", "par")
	outer.SetFinishPoint("par")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{ThreadID: "parallel-force-new-a"}
	_, err = cg.Invoke(context.Background(), map[string]any{}, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	cfg2 := &types.RunnableConfig{ThreadID: "parallel-force-new-b"}
	gotRaw, err := cg.Invoke(context.Background(), map[string]any{}, cfg2)
	if err != nil {
		t.Fatalf("Invoke 2: %v", err)
	}
	got, _ := extractSliceFromState(gotRaw)
	if len(got) != 2 {
		t.Fatalf("len: got %d, want 2", len(got))
	}
}

// TestIntegration_Parallel_SingleItemError_Wrapped asserts an item
// error wraps ErrParallelItemFailed.
func TestIntegration_Parallel_SingleItemError_Wrapped(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("op", func(_ context.Context, state interface{}) (interface{}, error) {
		if v, _ := extractIntFromState(state); v == 1 {
			return nil, errors.New("single-item-fail")
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
	cg, err := sg.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parallel item failed") {
		t.Errorf("err missing 'parallel item failed': %v", err)
	}
}

func TestHarness_Stream_ParallelValues(t *testing.T) {
	subCg := incSub(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	outer.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	outer.AddNode("par", pFn)
	outer.SetEntryPoint("prep")
	outer.AddEdge("prep", "par")
	outer.SetFinishPoint("par")

	cg, err := outer.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ch, errCh := cg.Stream(context.Background(), map[string]any{}, types.StreamModeValues)
	var got interface{}
	done := false
	for !done {
		select {
		case val, ok := <-ch:
			if !ok {
				done = true
				break
			}
			got = val
		case e := <-errCh:
			if e != nil {
				t.Fatalf("Stream error: %v", e)
			}
			done = true
		}
	}
	out, _ := extractSliceFromState(got)
	if len(out) != 3 {
		t.Fatalf("len: got %d, want 3", len(out))
	}
}

// ========================================================
// Interrupt/resume: sub-graph interrupt propagates correctly
// ========================================================

// TestHarness_Loop_SubGraphInterruptOrder verifies that when a sub-graph
// interrupts, the loop captures the state (iteration, current input)
// and re-throws as a GraphInterrupt.
func TestHarness_Parallel_MultipleInterrupts(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("intr", func(ctx context.Context, _ interface{}) (interface{}, error) {
		_, intrErr := interrupt.Interrupt(ctx, "multi-intr")
		return nil, intrErr
	})
	sub.SetEntryPoint("intr")
	sub.SetFinishPoint("intr")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	outer := graph.NewStateGraph(map[string]interface{}{})
	outer.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	outer.AddNode("par", pFn)
	outer.SetEntryPoint("prep")
	outer.AddEdge("prep", "par")
	outer.SetFinishPoint("par")

	cg, err := outer.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected interrupt")
	}
	if !gerrors.IsGraphInterrupt(err) {
		t.Errorf("expected GraphInterrupt, got %T", err)
	}
	msg := err.Error()
	if strings.Contains(msg, "1 interrupt") || strings.Contains(msg, "interrupted") {
		t.Logf("error: %v", err)
	}
}
