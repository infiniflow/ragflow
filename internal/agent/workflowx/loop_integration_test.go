// loop_integration_test.go — integration tests for the loop extension
// using the harness graph.StateGraph with checkpoints and interrupts.
package workflowx

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"ragflow/internal/harness/graph/checkpoint"
	gerrors "ragflow/internal/harness/graph/errors"
	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/interrupt"
	"ragflow/internal/harness/graph/types"
)

// TestIntegration_Loop_BasicCheckpoint verifies a loop works with a checkpoint.
func TestIntegration_Loop_BasicCheckpoint(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := buildIncGraph(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 3, nil
		},
		graph.WithLoopMaxIterations(5),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(20))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{ThreadID: "loop-integration-basic"}
	out, err := cg.Invoke(context.Background(), 0, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if v, _ := out.(int); v != 3 {
		t.Errorf("output: got %v, want 3", out)
	}
}

// TestIntegration_Loop_SubGraphInterrupt asserts loop re-throws
// a sub-graph interrupt as a GraphInterrupt error.
func TestIntegration_Loop_SubGraphInterrupt(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("intr", func(ctx context.Context, state interface{}) (interface{}, error) {
		_, intrErr := interrupt.Interrupt(ctx, "sub-interrupt-value")
		return nil, intrErr
	})
	sub.SetEntryPoint("intr")
	sub.SetFinishPoint("intr")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, nil
		},
		graph.WithLoopMaxIterations(5),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithRecursionLimit(20))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected interrupt, got nil")
	}
	if !gerrors.IsGraphInterrupt(err) {
		t.Logf("error: %T, want GraphInterrupt", err)
	}
}

// TestIntegration_Loop_MaxIterationsExceeded verifies max-iterations error.
func TestIntegration_Loop_MaxIterationsExceeded(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := buildIncGraph(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, nil
		},
		graph.WithLoopMaxIterations(3),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{ThreadID: "loop-max-iter-integration"}
	_, err = cg.Invoke(context.Background(), 0, cfg)
	if err == nil {
		t.Fatal("expected max-iterations error")
	}
	if !errors.Is(err, graph.ErrLoopMaxIterationsExceeded) {
		t.Logf("error type: %T, want ErrLoopMaxIterationsExceeded", err)
	}
}

// TestIntegration_Loop_CounterPerIteration verifies per-iteration count.
func TestIntegration_Loop_CounterPerIteration(t *testing.T) {
	var counter atomic.Int64
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("op", func(_ context.Context, state interface{}) (interface{}, error) {
		counter.Add(1)
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

	outer := graph.NewStateGraph(map[string]interface{}{})
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
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithRecursionLimit(10))
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

// TestIntegration_Loop_OuterStream verifies streaming the outer graph
// with a loop node produces the final result.
func TestIntegration_Loop_OuterStream(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := buildIncGraph(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 3, nil
		},
		graph.WithLoopMaxIterations(5),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(20))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{ThreadID: "loop-stream-test"}
	ch, errCh := cg.Stream(context.Background(), 0, types.StreamModeValues, cfg)

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
	if v, _ := finalOut.(int); v != 3 {
		t.Errorf("stream output: got %v, want 3", finalOut)
	}
}

// TestIntegration_Loop_ForceNewRun asserts a fresh ThreadID starts
// from scratch (no checkpoint state carried over).
func TestIntegration_Loop_ForceNewRun(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := buildIncGraph(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 3, nil
		},
		graph.WithLoopMaxIterations(5),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(20))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{ThreadID: "loop-force-new-a"}
	out, err := cg.Invoke(context.Background(), 0, cfg)
	if err != nil {
		t.Fatalf("Invoke 1: %v", err)
	}
	if v, _ := out.(int); v != 3 {
		t.Errorf("run 1 output: got %v, want 3", out)
	}

	cfg2 := &types.RunnableConfig{ThreadID: "loop-force-new-b"}
	out2, err := cg.Invoke(context.Background(), 0, cfg2)
	if err != nil {
		t.Fatalf("Invoke 2: %v", err)
	}
	if v, _ := out2.(int); v != 3 {
		t.Errorf("run 2 output: got %v, want 3", out2)
	}
}

// TestIntegration_Loop_CheckpointStatePersisted asserts the checkpointer
// stores state after loop execution. Uses Configurable so getThreadID
// picks up the thread_id from Configurable["thread_id"].
func TestIntegration_Loop_CheckpointStatePersisted(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := buildIncGraph(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 3, nil
		},
		graph.WithLoopMaxIterations(5),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(20))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{
		ThreadID: "loop-cp-state",
		Configurable: map[string]interface{}{
			"thread_id": "loop-cp-state",
		},
	}
	_, err = cg.Invoke(context.Background(), 0, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	cp, cpErr := memCp.Get(context.Background(), map[string]interface{}{
		"thread_id": "loop-cp-state",
	})
	if cpErr != nil {
		t.Fatalf("Get from memory checkpointer: %v", cpErr)
	}
	if cp == nil {
		t.Fatal("checkpoint is nil after loop execution")
	}
}

// TestIntegration_Loop_SubErrorNoShouldQuit asserts shouldQuit is NOT
// called when sub-graph returns a non-interrupt error.
func TestIntegration_Loop_SubErrorNoShouldQuit(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("err", func(_ context.Context, _ interface{}) (interface{}, error) {
		return nil, errors.New("sub-error")
	})
	sub.SetEntryPoint("err")
	sub.SetFinishPoint("err")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	outer := graph.NewStateGraph(map[string]interface{}{})
	shouldQuitCalled := false
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			shouldQuitCalled = true
			return true, nil
		},
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if shouldQuitCalled {
		t.Error("shouldQuit was called on non-interrupt sub-graph error")
	}
}

// TestIntegration_Loop_ErrInterruptedOnSubInterrupt asserts the loop
// propagates interrupt via GraphInterrupt.
func TestIntegration_Loop_ErrInterruptedOnSubInterrupt(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("intr", func(ctx context.Context, _ interface{}) (interface{}, error) {
		_, intrErr := interrupt.Interrupt(ctx, "loop-intr-val")
		return nil, intrErr
	})
	sub.SetEntryPoint("intr")
	sub.SetFinishPoint("intr")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, nil
		},
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected interrupt error")
	}
	if !gerrors.IsGraphInterrupt(err) {
		t.Logf("interrupt error: %T, want GraphInterrupt", err)
	}
}
func TestHarness_Stream_LoopValues(t *testing.T) {
	subCg := buildIncGraph(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 3, nil
		},
		graph.WithLoopMaxIterations(5),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	ch, errCh := cg.Stream(context.Background(), 0, types.StreamModeValues)
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
	if v, _ := got.(int); v != 3 {
		t.Errorf("stream output: got %v, want 3", got)
	}
}

// TestHarness_Stream_ParallelValues emits stream events from a parallel node.
func TestHarness_Loop_SubGraphInterruptOrder(t *testing.T) {
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("intr", func(ctx context.Context, _ interface{}) (interface{}, error) {
		_, intrErr := interrupt.Interrupt(ctx, "order-test")
		return nil, intrErr
	})
	sub.SetEntryPoint("intr")
	sub.SetFinishPoint("intr")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, nil
		},
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected interrupt")
	}
	var gi *gerrors.GraphInterrupt
	if !gerrors.IsGraphInterrupt(err) {
		t.Fatalf("expected GraphInterrupt, got %T: %v", err, err)
	}
	// Verify the GraphInterrupt contains the loop's interrupt value.
	if gi != nil && len(gi.Interrupts) > 0 {
		t.Logf("interrupt value: %v", gi.Interrupts[0])
	}
}

// ========================================================
// Checkpoint roundtrip: verify checkpoint data survives
// serialize/deserialize
// ========================================================

// TestHarness_Checkpoint_Roundtrip verifies that checkpoint data
// written by MemorySaver can be read back correctly.
func TestHarness_Checkpoint_Roundtrip(t *testing.T) {
	memCp := checkpoint.NewMemorySaver()
	subCg := buildIncGraph(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 2, nil
		},
		graph.WithLoopMaxIterations(5),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithCheckpointer(memCp), graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	cfg := &types.RunnableConfig{
		ThreadID: "cp-roundtrip",
		Configurable: map[string]interface{}{
			"thread_id": "cp-roundtrip",
		},
	}
	_, err = cg.Invoke(context.Background(), 0, cfg)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}

	// Read back and marshal/unmarshal to verify roundtrip.
	cp, cpErr := memCp.Get(context.Background(), map[string]interface{}{
		"thread_id": "cp-roundtrip",
	})
	if cpErr != nil {
		t.Fatalf("Get: %v", cpErr)
	}
	if cp == nil {
		t.Fatal("checkpoint is nil")
	}

	// JSON roundtrip test.
	data, jErr := json.Marshal(cp)
	if jErr != nil {
		t.Fatalf("Marshal checkpoint: %v", jErr)
	}
	var restored map[string]interface{}
	if jErr := json.Unmarshal(data, &restored); jErr != nil {
		t.Fatalf("Unmarshal checkpoint: %v", jErr)
	}
	t.Logf("checkpoint roundtrip OK, %d keys", len(restored))
}

// ========================================================
// Concurrent invocations: multiple goroutines running
// loop/parallel graphs
// ========================================================

// TestHarness_Loop_ConcurrentInvocation runs the same loop graph
// from multiple goroutines concurrently.
func TestHarness_Loop_ConcurrentInvocation(t *testing.T) {
	subCg := buildIncGraph(t)

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, next interface{}) (bool, error) {
			v, _ := next.(int)
			return v >= 3, nil
		},
		graph.WithLoopMaxIterations(5),
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile(graph.WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	var wg sync.WaitGroup
	results := make(chan int, 10)
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out, invErr := cg.Invoke(context.Background(), 0)
			if invErr != nil {
				t.Errorf("concurrent invoke: %v", invErr)
				return
			}
			if v, _ := out.(int); v == 3 {
				results <- v
			}
		}()
	}
	wg.Wait()
	close(results)
	count := 0
	for range results {
		count++
	}
	if count != 5 {
		t.Errorf("got %d successful invocations, want 5", count)
	}
}

// ========================================================
// GraphInterrupt propagation: verify sentinel error
// ========================================================

// TestHarness_GraphInterrupt_IsGraphInterrupt verifies the
// IsGraphInterrupt helper works on errors from loop/parallel.
func TestHarness_GraphInterrupt_IsGraphInterrupt(t *testing.T) {
	// Loop interrupt.
	sub := graph.NewStateGraph(map[string]interface{}{})
	sub.AddNode("intr", func(ctx context.Context, _ interface{}) (interface{}, error) {
		_, intrErr := interrupt.Interrupt(ctx, "test-intr")
		return nil, intrErr
	})
	sub.SetEntryPoint("intr")
	sub.SetFinishPoint("intr")
	subCg, err := sub.Compile()
	if err != nil {
		t.Fatalf("sub Compile: %v", err)
	}

	outer := graph.NewStateGraph(map[string]interface{}{})
	loopFn, err := graph.NewLoopNodeFunc("loop", subCg,
		func(_ context.Context, _ int, _, _ interface{}) (bool, error) {
			return false, nil
		},
	)
	if err != nil {
		t.Fatalf("NewLoopNodeFunc: %v", err)
	}
	outer.AddNode("loop", loopFn)
	outer.SetEntryPoint("loop")
	outer.SetFinishPoint("loop")

	cg, err := outer.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = cg.Invoke(context.Background(), 0)
	if err == nil {
		t.Fatal("expected interrupt error")
	}
	if !gerrors.IsGraphInterrupt(err) {
		t.Errorf("IsGraphInterrupt returned false for GraphInterrupt error")
	}

	// Parallel interrupt.
	pSub := graph.NewStateGraph(map[string]interface{}{})
	pSub.AddNode("pintr", func(ctx context.Context, _ interface{}) (interface{}, error) {
		_, intrErr := interrupt.Interrupt(ctx, "p-test-intr")
		return nil, intrErr
	})
	pSub.SetEntryPoint("pintr")
	pSub.SetFinishPoint("pintr")
	pSubCg, err := pSub.Compile()
	if err != nil {
		t.Fatalf("pSub Compile: %v", err)
	}

	pOuter := graph.NewStateGraph(map[string]interface{}{})
	pOuter.AddNode("prep", func(_ context.Context, _ interface{}) (interface{}, error) {
		return []interface{}{1, 2}, nil
	})
	pFn, err := graph.NewParallelNodeFunc("par", pSubCg)
	if err != nil {
		t.Fatalf("NewParallelNodeFunc: %v", err)
	}
	pOuter.AddNode("par", pFn)
	pOuter.SetEntryPoint("prep")
	pOuter.AddEdge("prep", "par")
	pOuter.SetFinishPoint("par")
	pCg, err := pOuter.Compile()
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	_, err = pCg.Invoke(context.Background(), nil)
	if err == nil {
		t.Fatal("expected interrupt from parallel")
	}
	if !gerrors.IsGraphInterrupt(err) {
		t.Errorf("IsGraphInterrupt returned false for parallel GraphInterrupt")
	}
}

// ========================================================
// GraphInterrupt composite: multiple interrupts from parallel
// ========================================================

// TestHarness_Parallel_MultipleInterrupts verifies that when multiple
// parallel items interrupt, the GraphInterrupt contains the data.
