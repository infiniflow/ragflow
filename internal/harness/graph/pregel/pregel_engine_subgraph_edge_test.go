// Package pregel provides engine edge cases and subgraph tests.
package pregel

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	graphPkg "ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// ============================================================
// P1: Engine basic execution (uses newSimpleGraph)
// ============================================================

func TestEngine_BasicExecution(t *testing.T) {
	result, err := NewEngine(newSimpleGraph(t), WithRecursionLimit(10)).
		RunSync(context.Background(), map[string]any{"value": "start"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "b" {
		t.Fatalf("expected value=b, got %v", m["value"])
	}
}

// ============================================================
// P1: Engine with BinaryOperatorAggregate
// ============================================================

func TestEngine_BinOpAggregate(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("sum", channels.NewBinaryOperatorAggregate(0, func(a, b any) any {
		return a.(int) + b.(int)
	}))

	sg.AddNode("add5", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"sum": 5}, nil
	})
	sg.AddNode("add10", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"sum": 10}, nil
	})
	_ = sg.AddEdge(constants.Start, "add5")
	_ = sg.AddEdge("add5", "add10")
	_ = sg.AddEdge("add10", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))
	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if v, ok := m["sum"]; !ok || v.(int) != 15 {
		t.Fatalf("expected sum=15, got %v", m["sum"])
	}
}

// ============================================================
// P1: Engine with Topic channel
// ============================================================

func TestEngine_TopicChannel(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("events", channels.NewTopic("", true))

	sg.AddNode("emit1", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"events": "ev1"}, nil
	})
	sg.AddNode("emit2", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"events": "ev2"}, nil
	})
	_ = sg.AddEdge(constants.Start, "emit1")
	_ = sg.AddEdge("emit1", "emit2")
	_ = sg.AddEdge("emit2", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(10))
	_, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
}

// ============================================================
// P1: Engine with checkpointer
// ============================================================

func TestEngine_WithCheckpointer(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	tid := "engine-wcp"
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithCheckpointer(ms),
		WithConfig(cfg),
	)
	result, err := engine.RunSync(context.Background(), map[string]any{"value": "start"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "b" {
		t.Fatalf("expected value=b, got %v", m["value"])
	}
}

// ============================================================
// P2: Engine with UntrackedValue
// ============================================================

func TestEngine_UntrackedValue(t *testing.T) {
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddChannel("scratch", channels.NewUntrackedValue(""))

	sg.AddNode("writer", func(ctx context.Context, state any) (any, error) {
		return map[string]any{"value": "persisted", "scratch": "temporary"}, nil
	})
	_ = sg.AddEdge(constants.Start, "writer")
	_ = sg.AddEdge("writer", constants.End)

	ms := checkpoint.NewMemorySaver()
	tid := "engine-untracked"
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithCheckpointer(ms),
		WithConfig(cfg),
	)

	result, err := engine.RunSync(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "persisted" {
		t.Fatalf("expected value=persisted, got %v", m["value"])
	}
}

// ============================================================
// P2: Engine reuse with different configs
// ============================================================

func TestEngine_ReuseDiffConfig(t *testing.T) {
	for i := 0; i < 10; i++ {
		ms := checkpoint.NewMemorySaver()
		tid := fmt.Sprintf("reuse-diff-%d", i)
		cfg := &types.RunnableConfig{
			Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
		}
		engine := NewEngine(newSimpleGraph(t),
			WithRecursionLimit(10),
			WithCheckpointer(ms),
			WithConfig(cfg),
		)
		_, err := engine.RunSync(context.Background(), map[string]any{"value": "reuse"})
		if err != nil {
			t.Fatalf("iteration %d: %v", i, err)
		}
	}
}

// ============================================================
// P2: Many parallel runs (no sharing)
// ============================================================

func TestEngine_ManyParallelRuns(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Go(func() {
			engine := NewEngine(newSimpleGraph(t), WithRecursionLimit(10))
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "par"})
			if err != nil {
				t.Errorf("RunSync: %v", err)
			}
		})
	}
	wg.Wait()
}

// ============================================================
// P2: Shared MemorySaver across 50 threads
// ============================================================

func TestEngine_SharedMemorySaver50(t *testing.T) {
	ms := checkpoint.NewMemorySaver()
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tid := fmt.Sprintf("sh-ms-50-%d", idx)
			cfg := &types.RunnableConfig{
				Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
			}
			engine := NewEngine(newSimpleGraph(t),
				WithRecursionLimit(10),
				WithCheckpointer(ms),
				WithConfig(cfg),
			)
			_, err := engine.RunSync(context.Background(), map[string]any{"value": "shared"})
			if err != nil {
				t.Errorf("engine %d: %v", idx, err)
			}
		}(i)
	}
	wg.Wait()
}

// ============================================================
// P2: Debug mode doesn't crash
// ============================================================

func TestEngine_DebugMode(t *testing.T) {
	engine := NewEngine(newSimpleGraph(t),
		WithRecursionLimit(10),
		WithDebug(true),
	)
	result, err := engine.RunSync(context.Background(), map[string]any{"value": "debug"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "b" {
		t.Fatalf("expected value=b, got %v", m["value"])
	}
}
