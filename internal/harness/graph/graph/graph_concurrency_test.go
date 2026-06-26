package graph

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/types"
)

// Concurrency safety tests for CompiledGraph.
// All tests must pass with `go test -race`.
// Goal: find data races in the compiled graph execution path, not work around them.

// TestGraph_ConcurrentInvoke_SharedGraph: 50 goroutines invoke the same CompiledGraph.
// The graph itself (nodes, edges) is read-only during execution, but channel registries
// and state maps are created per-invocation. This test validates that shared graph metadata
// access is race-free.
func TestGraph_ConcurrentInvoke_SharedGraph(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"counter": 0})
	sg.AddNode("inc", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		c, _ := s["counter"].(int)
		s["counter"] = c + 1
		return s, nil
	})
	sg.AddEdge(constants.Start, "inc")
	sg.AddEdge("inc", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 50
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			result, err := cg.Invoke(context.Background(), map[string]interface{}{"counter": 0})
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: %w", id, err)
				return
			}
			if result == nil {
				return
			}
			m := result.(map[string]interface{})
			if c, ok := m["counter"].(int); ok && c != 1 {
				errs <- fmt.Errorf("goroutine %d: expected counter=1, got %d", id, c)
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	var failures int
	for err := range errs {
		t.Error(err)
		failures++
	}
	if failures > 0 {
		t.Errorf("expected 0 failures, got %d", failures)
	}
}

// TestGraph_ConcurrentInvoke_ComplexGraph: 50 goroutines on a 10-node chain graph
// with conditional edges, testing that concurrent invocations don't corrupt shared state.
func TestGraph_ConcurrentInvoke_ComplexGraph(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"idx": 0, "path": ""})
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("n%d", i)
		val := i
		sg.AddNode(name, func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(map[string]interface{})
			s["idx"] = val
			s["path"] = name
			return s, nil
		})
	}
	sg.AddNode("final", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["path"] = "done"
		return s, nil
	})
	sg.AddEdge(constants.Start, "n0")
	for i := 0; i < 4; i++ {
		sg.AddEdge(fmt.Sprintf("n%d", i), fmt.Sprintf("n%d", i+1))
	}
	sg.AddEdge("n4", "final")
	sg.AddEdge("final", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(20))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 50
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cg.Invoke(context.Background(), map[string]interface{}{"idx": 0, "path": ""})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// TestGraph_ConcurrentInvoke_LoopGraph: 30 goroutines invoke a graph with conditional
// loop edges. Each invocation creates its own channel registry, so loop state is isolated.
func TestGraph_ConcurrentInvoke_LoopGraph(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"counter": 0, "value": ""})
	sg.AddNode("entry", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["counter"] = 0
		s["value"] = "start"
		return s, nil
	})
	sg.AddNode("loop", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		c, _ := s["counter"].(int)
		s["counter"] = c + 1
		s["value"] = fmt.Sprintf("iter_%d", c+1)
		return s, nil
	})
	sg.AddNode("done", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["value"] = "done"
		return s, nil
	})
	sg.AddEdge(constants.Start, "entry")
	sg.AddEdge("entry", "loop")
	sg.AddConditionalEdges("loop",
		func(ctx context.Context, state interface{}) (interface{}, error) {
			s := state.(map[string]interface{})
			c, _ := s["counter"].(int)
			if c >= 5 {
				return "done", nil
			}
			return "loop", nil
		},
		map[string]string{"loop": "loop", "done": "done"},
	)
	sg.AddEdge("done", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(20))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 30
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := cg.Invoke(context.Background(), map[string]interface{}{})
			if err != nil {
				errs <- err
				return
			}
			if result == nil {
				return
			}
			m := result.(map[string]interface{})
			v, _ := m["value"].(string)
			if v != "done" {
				errs <- fmt.Errorf("expected done, got %s", v)
			}
		}()
	}
	wg.Wait()
	close(errs)

	var failures int
	for err := range errs {
		t.Error(err)
		failures++
	}
	if failures > 0 {
		t.Errorf("expected 0 failures, got %d", failures)
	}
}

// TestGraph_ConcurrentInvoke_DAGGraph: 30 goroutines invoke a DAG fan-in graph.
func TestGraph_ConcurrentInvoke_DAGGraph(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"count": 0, "value": ""})
	sg.NodeTriggerMode = types.NodeTriggerAllPredecessor
	sg.AddChannel("count", channels.NewBinaryOperatorAggregate(0, func(a, b interface{}) interface{} {
		return a.(int) + b.(int)
	}))
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("split", func(ctx context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"count": 0}, nil
	})
	for i := 0; i < 5; i++ {
		name := fmt.Sprintf("w%d", i)
		sg.AddNode(name, func(ctx context.Context, state interface{}) (interface{}, error) {
			return map[string]interface{}{"count": 1}, nil
		})
	}
	sg.AddNode("join", func(ctx context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"value": "joined"}, nil
	})
	if n, ok := sg.GetNode("join"); ok {
		n.Triggers = []string{"count", "value"}
	}
	sg.AddEdge(constants.Start, "split")
	for i := 0; i < 5; i++ {
		sg.AddEdge("split", fmt.Sprintf("w%d", i))
		sg.AddEdge(fmt.Sprintf("w%d", i), "join")
	}
	sg.AddEdge("join", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(20))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 30
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := cg.Invoke(context.Background(), map[string]interface{}{})
			if err != nil {
				errs <- err
				return
			}
			if result == nil {
				return
			}
			m := result.(map[string]interface{})
			v, _ := m["value"].(string)
			if v != "joined" {
				errs <- fmt.Errorf("expected joined, got %s", v)
			}
		}()
	}
	wg.Wait()
	close(errs)

	var failures int
	for err := range errs {
		t.Error(err)
		failures++
	}
	if failures > 0 {
		t.Errorf("expected 0 failures, got %d", failures)
	}
}

// TestGraph_ConcurrentInvoke_MixedGraph: 30 goroutines on a mixed graph that uses
// channels, conditional edges, and multiple entry-like fan-out from Start.
func TestGraph_ConcurrentInvoke_MixedGraph(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"result": "", "order": ""})
	sg.AddNode("a", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["result"] = "a"
		s["order"] = s["order"].(string) + "a"
		return s, nil
	})
	sg.AddNode("b", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["result"] = "b"
		s["order"] = s["order"].(string) + "b"
		return s, nil
	})
	sg.AddNode("merge", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["result"] = "merged"
		return s, nil
	})
	sg.AddEdge(constants.Start, "a")
	sg.AddEdge("a", "b")
	sg.AddEdge("b", "merge")
	sg.AddEdge("merge", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 30
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := cg.Invoke(context.Background(), map[string]interface{}{"result": "", "order": ""})
			if err != nil {
				errs <- err
				return
			}
			if result == nil {
				return
			}
			m := result.(map[string]interface{})
			r, _ := m["result"].(string)
			if r != "merged" {
				errs <- fmt.Errorf("expected merged, got %s", r)
			}
		}()
	}
	wg.Wait()
	close(errs)

	var failures int
	for err := range errs {
		t.Error(err)
		failures++
	}
	if failures > 0 {
		t.Errorf("expected 0 failures, got %d", failures)
	}
}

// TestGraph_ConcurrentInvoke_WithChannels: 30 goroutines invoke a graph that has
// registered channels. Each invocation copies channels independently, but shared
// channel definitions in the StateGraph are accessed concurrently.
func TestGraph_ConcurrentInvoke_WithChannels(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"val": ""})
	sg.AddChannel("val", channels.NewLastValue(""))

	sg.AddNode("writer", func(ctx context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"val": "done"}, nil
	})
	sg.AddEdge(constants.Start, "writer")
	sg.AddEdge("writer", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 30
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := cg.Invoke(context.Background(), map[string]interface{}{"val": "start"})
			if err != nil {
				errs <- err
				return
			}
			if result == nil {
				return
			}
			m := result.(map[string]interface{})
			v, _ := m["val"].(string)
			if v != "done" {
				errs <- fmt.Errorf("expected done, got %s", v)
			}
		}()
	}
	wg.Wait()
	close(errs)

	var failures int
	for err := range errs {
		t.Error(err)
		failures++
	}
	if failures > 0 {
		t.Errorf("expected 0 failures, got %d", failures)
	}
}

// TestGraph_ConcurrentInvoke_InterruptRace: concurrent invoke where some goroutines
// set interrupt nodes. Validates that interrupt map access is race-free.
func TestGraph_ConcurrentInvoke_InterruptRace(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"val": ""})
	sg.AddNode("a", func(ctx context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"val": "a"}, nil
	})
	sg.AddNode("b", func(ctx context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"val": "b"}, nil
	})
	sg.AddNode("c", func(ctx context.Context, state interface{}) (interface{}, error) {
		return map[string]interface{}{"val": "c"}, nil
	})
	sg.AddEdge(constants.Start, "a")
	sg.AddEdge("a", "b")
	sg.AddEdge("b", "c")
	sg.AddEdge("c", constants.End)

	// Compile once with interrupts
	cg, err := sg.Compile(WithRecursionLimit(10), WithInterrupts("b"))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 30
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cg.Invoke(context.Background(), map[string]interface{}{"val": ""})
			// Interrupt is expected — not a failure
			if err != nil {
				errs <- fmt.Errorf("interrupt (expected): %v", err)
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Log(err) // interrupts are expected
	}
}

// TestGraph_ConcurrentInvoke_SharedNodeClosure: node functions capture a shared
// counter via closure. Validates that the graph itself doesn't introduce races
// on top of whatever the user's node functions do.
func TestGraph_ConcurrentInvoke_SharedNodeClosure(t *testing.T) {
	var sharedCounter int64
	sg := NewStateGraph(map[string]interface{}{"val": ""})

	sg.AddNode("shared", func(ctx context.Context, state interface{}) (interface{}, error) {
		// Intentionally using shared state to test graph concurrency safety.
		// The user's node closure is the user's responsibility, but the graph
		// should not introduce ADDITIONAL races on top of this.
		atomic.AddInt64(&sharedCounter, 1)
		return map[string]interface{}{"val": "done"}, nil
	})
	sg.AddEdge(constants.Start, "shared")
	sg.AddEdge("shared", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 50
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cg.Invoke(context.Background(), map[string]interface{}{"val": ""})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// TestGraph_ConcurrentStream_SharedGraph: 20 goroutines call Stream concurrently.
func TestGraph_ConcurrentStream_SharedGraph(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"val": ""})
	sg.AddNode("echo", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["val"] = "echoed"
		return s, nil
	})
	sg.AddEdge(constants.Start, "echo")
	sg.AddEdge("echo", constants.End)
	cg, err := sg.Compile(WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 20
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			outCh, errCh := cg.Stream(context.Background(), map[string]interface{}{"val": "test"}, types.StreamModeValues)
			for range outCh {
			}
			if err := <-errCh; err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// TestGraph_ConcurrentInvoke_StreamMix: 10 goroutines invoke, 10 goroutines stream,
// all sharing the same CompiledGraph.
func TestGraph_ConcurrentInvoke_StreamMix(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"val": ""})
	sg.AddNode("mix", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["val"] = "mixed"
		return s, nil
	})
	sg.AddEdge(constants.Start, "mix")
	sg.AddEdge("mix", constants.End)
	cg, err := sg.Compile(WithRecursionLimit(10))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 20
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			if id%2 == 0 {
				_, err := cg.Invoke(context.Background(), map[string]interface{}{"val": ""})
				if err != nil {
					errs <- fmt.Errorf("invoke %d: %w", id, err)
				}
			} else {
				outCh, errCh := cg.Stream(context.Background(), map[string]interface{}{"val": ""}, types.StreamModeValues)
				for range outCh {
				}
				if err := <-errCh; err != nil {
					errs <- fmt.Errorf("stream %d: %w", id, err)
				}
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// TestGraph_ConcurrentInvoke_HighContention: 100 goroutines hammer the same graph.
func TestGraph_ConcurrentInvoke_HighContention(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"val": ""})
	sg.AddNode("fast", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["val"] = "fast"
		return s, nil
	})
	sg.AddEdge(constants.Start, "fast")
	sg.AddEdge("fast", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(5))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 100
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cg.Invoke(context.Background(), map[string]interface{}{"val": ""})
			if err != nil {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// TestGraph_ConcurrentInvoke_TimeoutRace: concurrent invocations with short context
// timeouts to trigger context cancellation races.
func TestGraph_ConcurrentInvoke_TimeoutRace(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"val": ""})
	sg.AddNode("slow", func(ctx context.Context, state interface{}) (interface{}, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(50 * time.Millisecond):
		}
		s := state.(map[string]interface{})
		s["val"] = "done"
		return s, nil
	})
	sg.AddEdge(constants.Start, "slow")
	sg.AddEdge("slow", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(5))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 20
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
			defer cancel()
			_, err := cg.Invoke(ctx, map[string]interface{}{"val": ""})
			if err != nil {
				errs <- fmt.Errorf("goroutine %d: %v", id, err)
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Logf("timeout error (expected): %v", err)
	}
}

// TestGraph_ConcurrentInvoke_ErrorPropagation: concurrent invocations where some
// node functions return errors, validating error handling is race-free.
func TestGraph_ConcurrentInvoke_ErrorPropagation(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"val": ""})
	sg.AddNode("good", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["val"] = "good"
		return s, nil
	})
	sg.AddNode("fail", func(ctx context.Context, state interface{}) (interface{}, error) {
		return nil, fmt.Errorf("intentional failure")
	})
	sg.AddEdge(constants.Start, "good")
	sg.AddEdge("good", "fail")
	sg.AddEdge("fail", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(5))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 20
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cg.Invoke(context.Background(), map[string]interface{}{"val": ""})
			if err == nil {
				errs <- fmt.Errorf("expected error, got nil")
			}
		}()
	}
	wg.Wait()
	close(errs)

	var failures int
	for err := range errs {
		t.Error(err)
		failures++
	}
	if failures > 0 {
		t.Errorf("expected 0 failures, got %d", failures)
	}
}

// TestGraph_ConcurrentInvoke_GetNodesRace: concurrent invocations that call
// GetNodes/GetEdges/GetChannels on the graph while running. These accessors
// return the graph's internal maps without locking.
func TestGraph_ConcurrentInvoke_GetNodesRace(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"val": ""})
	sg.AddNode("a", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["val"] = "a"
		return s, nil
	})
	sg.AddEdge(constants.Start, "a")
	sg.AddEdge("a", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(5))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 50)

	// Goroutines that invoke
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cg.Invoke(context.Background(), map[string]interface{}{"val": ""})
			if err != nil {
				errs <- err
			}
		}()
	}

	// Goroutines that read graph metadata
	for i := 0; i < 25; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			g := cg.GetGraph()
			_ = g.GetNodes()
			_ = g.GetEdges()
			_ = g.GetChannels()
			_ = g.GetEntryPoint()
			_ = g.GetConditionalEdges()
		}()
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}

// TestGraph_ConcurrentInvoke_DifferentConfigs: concurrent invocations with different
// RunnableConfig values (thread IDs, metadata).
func TestGraph_ConcurrentInvoke_DifferentConfigs(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"val": ""})
	sg.AddNode("echo", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		s["val"] = "echoed"
		return s, nil
	})
	sg.AddEdge(constants.Start, "echo")
	sg.AddEdge("echo", constants.End)

	cg, err := sg.Compile(WithRecursionLimit(5))
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}

	const concurrency = 20
	var wg sync.WaitGroup
	errs := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			cfg := types.NewRunnableConfig()
			cfg.Configurable["thread_id"] = fmt.Sprintf("thread-%d", id)
			_, err := cg.Invoke(context.Background(), map[string]interface{}{"val": ""}, cfg)
			if err != nil {
				errs <- err
			}
		}(i)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		t.Error(err)
	}
}
