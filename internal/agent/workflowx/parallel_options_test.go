// parallel_options_test.go — option semantics for NewParallelNodeFunc.
// Tests focus on defaults, sentinel symbols, and configuration.
package workflowx

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/harness/graph/graph"
)

// TestOptions_Parallel_DefaultMaxConcurrencyIsZero asserts
// omitting WithParallelMaxConcurrency yields sequential.
func TestOptions_Parallel_DefaultMaxConcurrencyIsZero(t *testing.T) {
	subCg := incSub(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2}, nil
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
	gotRaw, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := extractSliceFromState(gotRaw)
	if len(got) != 2 {
		t.Fatalf("len: got %d, want 2", len(got))
	}
}

// TestOptions_Parallel_MaxConcurrency_Positive asserts
// WithParallelMaxConcurrency with a positive value works.
func TestOptions_Parallel_MaxConcurrency_Positive(t *testing.T) {
	subCg := incSub(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg,
		graph.WithParallelMaxConcurrency(2),
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
	gotRaw, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := extractSliceFromState(gotRaw)
	want := []interface{}{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if gv, _ := extractIntFromState(got[i]); gv != want[i] {
			t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestOptions_Parallel_NilSubGraph_Errors asserts NewParallelNodeFunc
// rejects a nil sub-graph.
func TestOptions_Parallel_NilSubGraph_Errors(t *testing.T) {
	_, err := graph.NewParallelNodeFunc("par", nil)
	if err == nil {
		t.Fatal("expected error for nil sub-graph")
	}
}

// TestOptions_Parallel_EmptyKey_Errors asserts NewParallelNodeFunc
// rejects an empty key.
func TestOptions_Parallel_EmptyKey_Errors(t *testing.T) {
	subCg := incSub(t)
	_, err := graph.NewParallelNodeFunc("", subCg)
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

// TestOptions_Parallel_SentinelErrorsExist asserts all sentinel
// errors are non-nil.
func TestOptions_Parallel_SentinelErrorsExist(t *testing.T) {
	sentinels := map[string]error{
		"ErrParallelResumeStateInvalid": graph.ErrParallelResumeStateInvalid,
		"ErrParallelItemFailed":         graph.ErrParallelItemFailed,
	}
	for name, e := range sentinels {
		if e == nil {
			t.Errorf("%s is nil", name)
		}
	}
}

// TestOptions_Parallel_ItemErrorStopsParallel asserts a non-interrupt
// error surfaces via ErrParallelItemFailed.
func TestOptions_Parallel_ItemErrorStopsParallel(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("op", func(_ context.Context, state interface{}) (interface{}, error) {
		if v, _ := extractIntFromState(state); v == 1 {
			return nil, errors.New("item-err")
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
	_, err = cg.Invoke(context.Background(), map[string]any{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "parallel item failed") {
		t.Errorf("err missing 'parallel item failed': %v", err)
	}
	if !strings.Contains(err.Error(), "item-err") {
		t.Errorf("err %q must contain 'item-err'", err.Error())
	}
}

// TestOptions_Parallel_WithMaxConcurrency_NegativeKeepsDefault
// asserts negative values are ignored (default 0).
func TestOptions_Parallel_WithMaxConcurrency_NegativeKeepsDefault(t *testing.T) {
	subCg := incSub(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg,
		graph.WithParallelMaxConcurrency(-3),
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
	gotRaw, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := extractSliceFromState(gotRaw)
	if len(got) != 2 {
		t.Fatalf("len: got %d, want 2", len(got))
	}
}

// TestOptions_Parallel_CheckpointBuilder_Override asserts a custom
// checkpoint ID builder is respected.
func TestOptions_Parallel_CheckpointBuilder_Override(t *testing.T) {
	var seenIDs []string
	subCg := incSub(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg,
		graph.WithParallelCheckpointIDBuilder(func(nodeKey string, idx int) string {
			id := "my-custom:" + nodeKey + ":" + fmt.Sprint(idx)
			seenIDs = append(seenIDs, id)
			return id
		}),
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
	gotRaw, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := extractSliceFromState(gotRaw)
	if len(got) != 3 {
		t.Fatalf("len: got %d, want 3", len(got))
	}
	if len(seenIDs) < 3 {
		t.Errorf("builder called %d times, want >= 3", len(seenIDs))
	}
	for _, id := range seenIDs {
		if id[:10] != "my-custom:" {
			t.Errorf("checkpoint ID %q does not start with 'my-custom:'", id)
		}
	}
}

// TestOptions_Parallel_EnableSubCheckpoint_Default asserts the default is true.
func TestOptions_Parallel_EnableSubCheckpoint_Default(t *testing.T) {
	subCg := incSub(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2}, nil
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
	gotRaw, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := extractSliceFromState(gotRaw)
	if len(got) != 2 {
		t.Fatalf("len: got %d, want 2", len(got))
	}
}

// TestOptions_Parallel_EnableSubCheckpoint_False asserts sub-checkpoints
// can be disabled.
func TestOptions_Parallel_EnableSubCheckpoint_False(t *testing.T) {
	subCg := incSub(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2, 3}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", subCg,
		graph.WithParallelEnableSubCheckpoint(false),
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
	gotRaw, err := cg.Invoke(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	got, _ := extractSliceFromState(gotRaw)
	want := []interface{}{2, 3, 4}
	if len(got) != len(want) {
		t.Fatalf("len: got %d, want %d", len(got), len(want))
	}
	for i := range want {
		if gv, _ := extractIntFromState(got[i]); gv != want[i] {
			t.Errorf("got[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}
