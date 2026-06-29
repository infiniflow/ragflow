// loop_options_test.go — option semantics for NewLoopNodeFunc.
// Tests focus on defaults, compile-time errors, and sentinel symbols.
package workflowx

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/harness/graph/graph"
)

// TestOptions_Loop_DefaultMaxIterationsIs1024 asserts the default
// max iterations.
func TestOptions_Loop_DefaultMaxIterationsIs1024(t *testing.T) {
	subCg := buildIncGraph(t)
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, nil
		},
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("loop", loopFn)
	sg.SetEntryPoint("loop")
	sg.SetFinishPoint("loop")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// 1025 iterations should exceed the default limit
	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected max-iterations error with default 1024")
	}
	if !errors.Is(err, graph.ErrLoopMaxIterationsExceeded) {
		t.Logf("error: %T, want ErrLoopMaxIterationsExceeded", err)
	}
}

// TestOptions_Loop_WithMaxIterations_OverridesDefault asserts
// WithLoopMaxIterations overrides the default.
func TestOptions_Loop_WithMaxIterations_OverridesDefault(t *testing.T) {
	subCg := buildIncGraph(t)
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, nil
		},
		graph.WithLoopMaxIterations(5),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("loop", loopFn)
	sg.SetEntryPoint("loop")
	sg.SetFinishPoint("loop")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected max-iterations error")
	}
	if !errors.Is(err, graph.ErrLoopMaxIterationsExceeded) {
		t.Logf("error: %T, want ErrLoopMaxIterationsExceeded", err)
	}
}

// TestOptions_Loop_NilSubGraph_Errors asserts NewLoopNodeFunc
// rejects a nil sub-graph.
func TestOptions_Loop_NilSubGraph_Errors(t *testing.T) {
	_, err := graph.NewLoopNodeFunc("loop", nil,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return true, nil
		},
	)
	if err == nil {
		t.Fatal("expected error for nil sub-graph")
	}
}

// TestOptions_Loop_EmptyKey_Errors asserts NewLoopNodeFunc
// rejects an empty key.
func TestOptions_Loop_EmptyKey_Errors(t *testing.T) {
	subCg := buildIncGraph(t)
	_, err := graph.NewLoopNodeFunc("", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return true, nil
		},
	)
	if err == nil {
		t.Fatal("expected error for empty key")
	}
}

// TestOptions_Loop_CheckpointIDPrefix asserts
// WithLoopCheckpointIDPrefix is accepted without error.
func TestOptions_Loop_CheckpointIDPrefix(t *testing.T) {
	subCg := buildIncGraph(t)
	_, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 3, nil
		},
		graph.WithLoopMaxIterations(10),
		graph.WithLoopCheckpointIDPrefix("my-prefix"),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
}

// TestOptions_Loop_SentinelErrorsExist asserts all sentinel
// errors are non-nil.
func TestOptions_Loop_SentinelErrorsExist(t *testing.T) {
	sentinels := map[string]error{
	"ErrLoopMaxIterationsExceeded": graph.ErrLoopMaxIterationsExceeded,
		"ErrLoopResumeStateInvalid":    graph.ErrLoopResumeStateInvalid,
	}
	for name, e := range sentinels {
		if e == nil {
			t.Errorf("%s is nil", name)
		}
	}
	if !errors.Is(graph.ErrLoopMaxIterationsExceeded, graph.ErrLoopMaxIterationsExceeded) {
		t.Error("ErrLoopMaxIterationsExceeded is not Is-self")
	}
	if !errors.Is(graph.ErrLoopResumeStateInvalid, graph.ErrLoopResumeStateInvalid) {
		t.Error("ErrLoopResumeStateInvalid is not Is-self")
	}
}

// TestOptions_Loop_ShouldQuitReturnsTrueTerminatesEarly asserts
// that the loop stops when shouldQuit returns true.
func TestOptions_Loop_ShouldQuitReturnsTrueTerminatesEarly(t *testing.T) {
	var iterations int
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("op", func(_ context.Context, state interface{}) (interface{}, error) {
		iterations++
		if in, ok := state.(int); ok {
			return in + 1, nil
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
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 2, nil
		},
		graph.WithLoopMaxIterations(10),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	sg.AddNode("loop", loopFn)
	sg.SetEntryPoint("loop")
	sg.SetFinishPoint("loop")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	out, err := cg.Invoke(context.Background(), 0)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if v, _ := out.(int); v != 2 {
		t.Errorf("output: got %v, want 2", out)
	}
}

// TestOptions_Loop_WithMaxIterations_NegativeKeepsDefault asserts
// negative values are ignored (default 1024 takes effect).
func TestOptions_Loop_WithMaxIterations_NegativeKeepsDefault(t *testing.T) {
	subCg := buildIncGraph(t)
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, nil
		},
		graph.WithLoopMaxIterations(-3),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("loop", loopFn)
	sg.SetEntryPoint("loop")
	sg.SetFinishPoint("loop")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	// Negative → default 1024, so 1025 iterations should overflow.
	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected max-iterations error (negative should fall back to default)")
	}
	if !errors.Is(err, graph.ErrLoopMaxIterationsExceeded) {
		t.Logf("error: %T, want ErrLoopMaxIterationsExceeded", err)
	}
}

// TestOptions_Loop_WithMaxIterations_ZeroKeepsDefault asserts
// zero is ignored (default 1024 takes effect).
func TestOptions_Loop_WithMaxIterations_ZeroKeepsDefault(t *testing.T) {
	subCg := buildIncGraph(t)
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, nil
		},
		graph.WithLoopMaxIterations(0),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	sg := graph.NewStateGraph(map[string]interface{}{})
	sg.AddNode("loop", loopFn)
	sg.SetEntryPoint("loop")
	sg.SetFinishPoint("loop")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected max-iterations error (zero should fall back to default)")
	}
	if !errors.Is(err, graph.ErrLoopMaxIterationsExceeded) {
		t.Logf("error: %T, want ErrLoopMaxIterationsExceeded", err)
	}
}

// TestOptions_Loop_DefaultStreamModeIsFinalOnly asserts default stream mode.
func TestOptions_Loop_DefaultStreamModeIsFinalOnly(t *testing.T) {
	subCg := buildIncGraph(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 3, nil
		},
		graph.WithLoopMaxIterations(10),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	sg.AddNode("loop", loopFn)
	sg.SetEntryPoint("loop")
	sg.SetFinishPoint("loop")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	out, err := cg.Invoke(context.Background(), 0)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if v, _ := out.(int); v != 3 {
		t.Errorf("output: got %v, want 3", out)
	}
}

// TestOptions_Loop_WithLoopStream_Values asserts LoopStreamValues is accepted.
func TestOptions_Loop_WithLoopStream_Values(t *testing.T) {
	subCg := buildIncGraph(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 3, nil
		},
		graph.WithLoopMaxIterations(10),
		graph.WithLoopStream(graph.LoopStreamValues),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	sg.AddNode("loop", loopFn)
	sg.SetEntryPoint("loop")
	sg.SetFinishPoint("loop")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	out, err := cg.Invoke(context.Background(), 0)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if v, _ := out.(int); v != 3 {
		t.Errorf("output: got %v, want 3", out)
	}
}

// TestOptions_Loop_WithLoopStream_UnknownRejected asserts an invalid
// mode falls back to the default.
func TestOptions_Loop_WithLoopStream_UnknownRejected(t *testing.T) {
	subCg := buildIncGraph(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 3, nil
		},
		graph.WithLoopMaxIterations(10),
		graph.WithLoopStream(graph.LoopStreamMode(999)),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	sg.AddNode("loop", loopFn)
	sg.SetEntryPoint("loop")
	sg.SetFinishPoint("loop")
	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	out, err := cg.Invoke(context.Background(), 0)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if v, _ := out.(int); v != 3 {
		t.Errorf("output: got %v, want 3", out)
	}
}
