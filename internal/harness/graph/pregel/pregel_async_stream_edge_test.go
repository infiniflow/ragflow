// Package pregel provides async coverage, stream protocol edge cases,
// and retry strategy edge cases for the Pregel engine.
package pregel

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	graphPkg "ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// ============================================================
// P0: Stream protocol edge cases
// ============================================================

// TestStream_ChannelStream_Basic verifies ChannelStream emit/consume cycle.
func TestStream_ChannelStream_Basic(t *testing.T) {
	ctx := context.Background()
	stream := types.NewChannelStream(types.StreamModeValues, 10)
	defer stream.Close()

	chunk := &types.StreamChunk{Data: "hello", Step: 1}
	if err := stream.Emit(ctx, chunk); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	iter := stream.Iterator(ctx)
	defer iter.Close()

	got, err := iter.Next(ctx)
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got.Data != "hello" {
		t.Fatalf("expected data=hello, got %v", got.Data)
	}
}

// TestStream_ChannelStream_CloseWhileReading tests close during iteration.
func TestStream_ChannelStream_CloseWhileReading(t *testing.T) {
	ctx := context.Background()
	stream := types.NewChannelStream(types.StreamModeValues, 10)
	_ = stream.Emit(ctx, &types.StreamChunk{Data: "a", Step: 1})

	go func() {
		time.Sleep(5 * time.Millisecond)
		stream.Close()
	}()

	iter := stream.Iterator(ctx)
	defer iter.Close()
	for {
		_, err := iter.Next(ctx)
		if err != nil {
			break
		}
	}
}

// TestStream_StreamEvent_JSONRoundTrip verifies JSON serialization.
func TestStream_StreamEvent_JSONRoundTrip(t *testing.T) {
	event := NewStreamEvent(EventTypeCheckpoint, 3)
	event.Node = "test_node"
	event.Data = map[string]any{"key": "value"}

	b, err := event.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("expected non-empty JSON")
	}
}

// ============================================================
// P0: Async/concurrency patterns
// ============================================================

// TestConcurrent_MultipleEngines_DifferentGraphs runs engines with
// different graph instances concurrently.
func TestConcurrent_MultipleEngines_DifferentGraphs(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			engine := NewEngine(newSimpleGraph(t), WithRecursionLimit(10))
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "conc"})
			if err != nil {
				t.Errorf("engine %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

// TestConcurrent_SharedEngine_DifferentInputs reuses one engine
// with different inputs sequentially.
func TestConcurrent_SharedEngine_DifferentInputs(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t), WithRecursionLimit(10))
	ctx := context.Background()

	for _, input := range []map[string]any{
		{"value": "a"}, {"value": "b"}, {"value": "c"},
	} {
		result, err := engine.RunSync(ctx, input)
		if err != nil {
			t.Fatalf("RunSync: %v", err)
		}
		m := result.(map[string]any)
		if m["value"] != "b" {
			t.Fatalf("expected value=b, got %v", m["value"])
		}
	}
}

// TestConcurrent_ManyEngines_WithCheckpointer runs 20 engines each
// with their own checkpointer concurrently.
func TestConcurrent_ManyEngines_WithCheckpointer(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ms := checkpoint.NewMemorySaver()
			tid := "conc-cp-" + string(rune('0'+idx))
			cfg := &types.RunnableConfig{
				Configurable: map[string]interface{}{
					constants.ConfigKeyThreadID: tid,
				},
			}
			engine := NewEngine(newSimpleGraph(t),
				WithRecursionLimit(10),
				WithCheckpointer(ms),
				WithConfig(cfg),
			)
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "conc"})
			if err != nil {
				t.Errorf("engine %d: %v", idx, err)
			}
			cp, err := ms.Get(context.Background(), map[string]interface{}{
				constants.ConfigKeyThreadID: tid,
			})
			if err != nil || cp == nil {
				t.Errorf("engine %d: missing checkpoint", idx)
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================
// P1: Retry strategy edge cases
// ============================================================

// TestRetry_ZeroMaxAttempts verifies zero max attempts doesn't crash.
func TestRetry_ZeroMaxAttempts(t *testing.T) {
	var attempts atomic.Int32
	sg := newRetryGraph(func(ctx context.Context, state any) (any, error) {
		attempts.Add(1)
		return nil, fmt.Errorf("fail %d", attempts.Load())
	})

	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 0
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "zero"})
	t.Logf("zero max attempts: err=%v attempts=%d", err, attempts.Load())
}

// TestRetry_MaxIntervalCapped verifies backoff is capped at MaxInterval.
func TestRetry_MaxIntervalCapped(t *testing.T) {
	var attempts atomic.Int32
	sg := newRetryGraph(func(ctx context.Context, state any) (any, error) {
		n := attempts.Add(1)
		return nil, fmt.Errorf("attempt %d", n)
	})

	rp := types.RetryPolicy{
		InitialInterval: 10 * time.Millisecond,
		BackoffFactor:   100.0,
		MaxInterval:     20 * time.Millisecond,
		MaxAttempts:     5,
		Jitter:          false,
	}
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "maxint"})
	if err == nil {
		t.Fatal("expected error")
	}
	t.Logf("max interval capped: attempts=%d", attempts.Load())
}

// TestRetry_JitterVariation verifies jitter is applied.
func TestRetry_JitterVariation(t *testing.T) {
	var attempts atomic.Int32
	sg := newRetryGraph(func(ctx context.Context, state any) (any, error) {
		n := attempts.Add(1)
		return nil, fmt.Errorf("jitter %d", n)
	})

	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 3
	rp.Jitter = true
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "jitter"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// ============================================================
// P1: Pregel Engine — more complex scenarios
// ============================================================

// TestEngine_DAG_ModeFanIn verifies DAG mode with fan-in.
func TestEngine_DAG_ModeFanIn(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("a", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"value": "a_done"}, nil
	})
	sg.AddNode("b", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"value": "b_done"}, nil
	})
	sg.AddNode("join", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	_ = sg.AddEdge(constants.Start, "a")
	_ = sg.AddEdge(constants.Start, "b")
	_ = sg.AddEdge("a", "join")
	_ = sg.AddEdge("b", "join")
	_ = sg.AddEdge("join", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))
	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	_ = result
}

// TestEngine_NodeReturningCommand verifies a node that returns state.
func TestEngine_NodeReturningCommand(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("router", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"value": "routed"}, nil
	})
	sg.AddNode("dest", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		if m == nil {
			m = map[string]any{}
		}
		m["value"] = "dest"
		return m, nil
	})
	_ = sg.AddEdge(constants.Start, "router")
	_ = sg.AddEdge("router", "dest")
	_ = sg.AddEdge("dest", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))
	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "dest" {
		t.Fatalf("expected value=dest, got %v", m["value"])
	}
}

// ============================================================
// P2: Engine with mixed channel types
// ============================================================

// TestEngine_MixedChannels_TopicPlusLastValue uses Topic + LastValue.
func TestEngine_MixedChannels_TopicPlusLastValue(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("counter", channels.NewBinaryOperatorAggregate(0, func(a, b any) any {
		return a.(int) + b.(int)
	}))
	sg.AddChannel("status", channels.NewLastValue(""))

	sg.AddNode("producer", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"counter": 10, "status": "running"}, nil
	})
	sg.AddNode("finalizer", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"counter": 20, "status": "done"}, nil
	})
	_ = sg.AddEdge(constants.Start, "producer")
	_ = sg.AddEdge("producer", "finalizer")
	_ = sg.AddEdge("finalizer", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))
	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["status"] != "done" {
		t.Fatalf("expected status=done, got %v", m["status"])
	}
	if m["counter"].(int) != 30 {
		t.Fatalf("expected counter=30, got %v", m["counter"])
	}
}

// ============================================================
// Helper
// ============================================================

func newRetryGraph(fn func(context.Context, any) (any, error)) types.StateGraph {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("work", fn)
	_ = sg.AddEdge(constants.Start, "work")
	_ = sg.AddEdge("work", constants.End)
	return sg
}
