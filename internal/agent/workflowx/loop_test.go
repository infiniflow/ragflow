// loop_test.go — pure logic and state-machine tests for the loop
// extension using the harness graph.StateGraph.
package workflowx

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"

	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// incNode returns a nodefunc that increments the value under __root__ by 1.
// Works with both the inline engine (raw int) and the pregel engine (map).
func incNode() func(context.Context, interface{}) (interface{}, error) {
	return func(_ context.Context, state interface{}) (interface{}, error) {
		v, _ := extractIntFromState(state)
		return map[string]interface{}{"__root__": v + 1}, nil
	}
}

// buildIncGraph compiles a single-node sub-graph that increments by 1.
func buildIncGraph(t *testing.T) types.CompiledGraph {
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

// TestLoop_BasicIteration asserts a simple loop that counts to 3.
func TestLoop_BasicIteration(t *testing.T) {
	subCg := buildIncGraph(t)

	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := extractIntFromState(next)
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
	v, _ := extractIntFromState(out)
	if v != 3 {
		t.Errorf("output: got %v, want 3", out)
	}
}

// TestLoop_MaxIterationsExceeded asserts the loop stops at maxIterations.
func TestLoop_MaxIterationsExceeded(t *testing.T) {
	subCg := buildIncGraph(t)

	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, nil
		},
		graph.WithLoopMaxIterations(3),
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
	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected max-iterations error, got nil")
	}
	if !errors.Is(err, graph.ErrLoopMaxIterationsExceeded) {
		t.Logf("error: %T, want ErrLoopMaxIterationsExceeded", err)
	}
}

// TestLoop_SubErrorStopsLoop asserts a sub-graph error stops the loop.
func TestLoop_SubErrorStopsLoop(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("err", func(_ context.Context, _ interface{}) (interface{}, error) {
		return map[string]interface{}{}, errors.New("sub-fail")
	})
	sub.SetEntryPoint("err")
	sub.SetFinishPoint("err")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			t.Fatal("shouldQuit not called on sub error")
			return false, nil
		},
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
	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "sub-fail") {
		t.Errorf("error %q must contain 'sub-fail'", err.Error())
	}
}

// TestLoop_CounterIncrementedPerIteration asserts per-iteration count.
func TestLoop_CounterIncrementedPerIteration(t *testing.T) {
	var counter atomic.Int64

	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("inc", func(_ context.Context, state interface{}) (interface{}, error) {
		counter.Add(1)
		v, _ := extractIntFromState(state)
		return map[string]interface{}{"__root__": v + 1}, nil
	})
	sub.SetEntryPoint("inc")
	sub.SetFinishPoint("inc")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := extractIntFromState(next)
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
	if _, err := cg.Invoke(context.Background(), 0); err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if got := counter.Load(); got != 3 {
		t.Errorf("counter: got %d, want 3", got)
	}
}

// TestLoop_DoWhileContract asserts the sub-workflow runs at least once
// even when shouldQuit returns true on the first iteration.
func TestLoop_DoWhileContract(t *testing.T) {
	var seen int
	subCg := buildIncGraph(t)

	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, iter int, _, _ interface{}) (bool, error) {
			seen = iter
			return true, nil
		},
		graph.WithLoopMaxIterations(1),
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
	out, err := cg.Invoke(context.Background(), 7)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if v, _ := extractIntFromState(out); v != 8 {
		t.Errorf("output: got %v, want 8", out)
	}
	if seen != 1 {
		t.Errorf("shouldQuit saw iter %d, want 1", seen)
	}
}

// TestLoop_IterationNumbering records iteration numbers passed to shouldQuit.
func TestLoop_IterationNumbering(t *testing.T) {
	var iterations []int
	subCg := buildIncGraph(t)

	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, iter int, _, next interface{}) (bool, error) {
			iterations = append(iterations, iter)
			v, _ := extractIntFromState(next)
			return v >= 4, nil
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
	out, err := cg.Invoke(context.Background(), 1)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if v, _ := extractIntFromState(out); v != 4 {
		t.Errorf("output: got %v, want 4", out)
	}
	want := []int{1, 2, 3}
	if len(iterations) != len(want) {
		t.Fatalf("iterations: got %v, want %v", iterations, want)
	}
	for i := range want {
		if iterations[i] != want[i] {
			t.Errorf("iterations[%d]: got %d, want %d", i, iterations[i], want[i])
		}
	}
}

// TestLoop_QuitConditionError asserts that an error from shouldQuit
// surfaces through Invoke.
func TestLoop_QuitConditionError(t *testing.T) {
	subCg := buildIncGraph(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, errors.New("boom")
		},
		graph.WithLoopMaxIterations(5),
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
	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("error %q must contain 'boom'", err.Error())
	}
}

// TestLoop_NormalConvergence asserts the basic happy path converges at 5.
func TestLoop_NormalConvergence(t *testing.T) {
	subCg := buildIncGraph(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := extractIntFromState(next)
			return v >= 5, nil
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
	if v, _ := extractIntFromState(out); v != 5 {
		t.Errorf("output: got %v, want 5", out)
	}
}

// TestLoop_ShouldQuitCalledWithCorrectIteration verifies iteration
// counting starts at 1.
func TestLoop_ShouldQuitCalledWithCorrectIteration(t *testing.T) {
	var iterations []int
	subCg := buildIncGraph(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, iter int, _, next interface{}) (bool, error) {
			iterations = append(iterations, iter)
			v, _ := extractIntFromState(next)
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
	_, err = cg.Invoke(context.Background(), 0)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	want := []int{1, 2, 3}
	if len(iterations) != len(want) {
		t.Fatalf("iterations: got %v, want %v", iterations, want)
	}
	for i := range want {
		if iterations[i] != want[i] {
			t.Errorf("iterations[%d]: got %d, want %d", i, iterations[i], want[i])
		}
	}
}

// TestLoop_SingleIterationDoWhile verifies that when shouldQuit returns
// true immediately, the sub-graph still runs exactly once (do-while contract).
func TestLoop_SingleIterationDoWhile(t *testing.T) {
	subCg := buildIncGraph(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	var called bool
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			called = true
			return true, nil
		},
		graph.WithLoopMaxIterations(1),
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
	out, err := cg.Invoke(context.Background(), 5)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if !called {
		t.Error("shouldQuit was not called")
	}
	if v, _ := extractIntFromState(out); v != 6 {
		t.Errorf("output: got %v, want 6", out)
	}
}

// TestLoop_ShouldQuitError_IterationTerminates asserts a shouldQuit
// error terminates the loop (falls through to max iterations).
func TestLoop_ShouldQuitError_IterationTerminates(t *testing.T) {
	subCg := buildIncGraph(t)
	sg := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, errors.New("quit-boom")
		},
		graph.WithLoopMaxIterations(5),
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
	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error")
	}
}
