// Package pregel provides boundary condition tests for the engine.
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

func TestBoundary_EmptyStateGraph(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddNode("nop", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	_ = sg.AddEdge(constants.Start, "nop")
	_ = sg.AddEdge("nop", constants.End)
	engine := NewEngine(sg, WithRecursionLimit(10))
	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	_ = result
}

func TestBoundary_NilConfig(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg, WithRecursionLimit(10))
	result, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	_ = result
}

func TestBoundary_NoTasksStillCompletes(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddNode("only", func(ctx context.Context, state any) (any, error) {
		return state, nil
	})
	_ = sg.AddEdge(constants.Start, "only")
	_ = sg.AddEdge("only", constants.End)
	engine := NewEngine(sg, WithRecursionLimit(10))
	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	_ = result
}

func TestBoundary_BinOpWithCheckpointer(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("sum", channels.NewBinaryOperatorAggregate(0, func(a, b any) any {
		return a.(int) + b.(int)
	}))
	sg.AddNode("add1", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"sum": 5}, nil
	})
	sg.AddNode("add2", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"sum": 10}, nil
	})
	_ = sg.AddEdge(constants.Start, "add1")
	_ = sg.AddEdge("add1", "add2")
	_ = sg.AddEdge("add2", constants.End)

	ms := checkpoint.NewMemorySaver()
	tid := "binop-cp"
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	engine := NewEngine(sg, WithRecursionLimit(10), WithCheckpointer(ms), WithConfig(cfg))
	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["sum"].(int) != 15 {
		t.Fatalf("expected sum=15, got %v", m["sum"])
	}
}

func TestBoundary_ManyIndependentCheckpointers(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			ms := checkpoint.NewMemorySaver()
			tid := fmt.Sprintf("indep-cp-%d", idx)
			cfg := &types.RunnableConfig{
				Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
			}
			engine := NewEngine(newSimpleGraph(t), WithRecursionLimit(10), WithCheckpointer(ms), WithConfig(cfg))
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
			if err != nil {
				t.Errorf("engine %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

func TestBoundary_NodeContextDeadline(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("slow", func(ctx context.Context, state any) (any, error) {
		select {
		case <-time.After(5 * time.Second):
			return state, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	_ = sg.AddEdge(constants.Start, "slow")
	_ = sg.AddEdge("slow", constants.End)
	engine := NewEngine(sg, WithRecursionLimit(10))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := engine.RunSync(ctx, map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected deadline exceeded")
	}
}

func TestBoundary_SequentialChannelAccumulator(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("counter", channels.NewBinaryOperatorAggregate(0, func(a, b any) any {
		return a.(int) + b.(int)
	}))
	sg.AddNode("a", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"counter": 1}, nil
	})
	sg.AddNode("b", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"counter": 2}, nil
	})
	_ = sg.AddEdge(constants.Start, "a")
	_ = sg.AddEdge("a", "b")
	_ = sg.AddEdge("b", constants.End)
	engine := NewEngine(sg, WithRecursionLimit(10))
	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["counter"].(int) != 3 {
		t.Fatalf("expected counter=3, got %v", m["counter"])
	}
}

func TestBoundary_RetryInterruptCheckpointer(t *testing.T) {
	var attempts atomic.Int32
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("flaky", func(ctx context.Context, state any) (any, error) {
		n := attempts.Add(1)
		if n < 3 {
			return nil, fmt.Errorf("transient %d", n)
		}
		return state, nil
	})
	_ = sg.AddEdge(constants.Start, "flaky")
	_ = sg.AddEdge("flaky", constants.End)

	ms := checkpoint.NewMemorySaver()
	tid := "retry-int-cp"
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 5
	engine := NewEngine(sg, WithRecursionLimit(10), WithCheckpointer(ms), WithConfig(cfg), WithRetryPolicy(&rp), WithInterrupts("*"))
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected interrupt")
	}
	t.Logf("retry+interrupt+cp: %d attempts", attempts.Load())
}

func TestBoundary_MaxRecursionLimit(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t), WithRecursionLimit(1<<31-1))
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
}
