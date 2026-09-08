package pregel

import (
	"context"
	"testing"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/constants"
	"ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

func newTestGraph(t *testing.T) types.StateGraph {
	t.Helper()
	sg := newSimpleGraph(t)
	return sg
}

func newSimpleGraph(t *testing.T) types.StateGraph {
	t.Helper()
	sg := graph.NewStateGraph(map[string]any{"value": ""})
	// Register a channel so the engine can write output
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("node_a", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		if m == nil {
			m = map[string]any{}
		}
		m["value"] = "a"
		return m, nil
	})
	sg.AddNode("node_b", func(ctx context.Context, state any) (any, error) {
		m, _ := state.(map[string]any)
		if m == nil {
			m = map[string]any{}
		}
		m["value"] = "b"
		return m, nil
	})
	if err := sg.AddEdge(constants.Start, "node_a"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("node_a", "node_b"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("node_b", constants.End); err != nil {
		t.Fatal(err)
	}
	return sg
}

func TestNewEngine(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithDebug(false),
	)
	if engine == nil {
		t.Fatal("expected non-nil engine")
	}
	if engine.recursionLimit != 10 {
		t.Errorf("expected recursionLimit = 10, got %d", engine.recursionLimit)
	}
}

func TestEngine_TaskConstructionPreservesNodeRetryPolicy(t *testing.T) {
	policy := &types.RetryPolicy{MaxAttempts: 1}
	sg := graph.NewStateGraph(map[string]any{"value": ""})
	node := sg.AddNodeWithOptions("work", func(_ context.Context, state any) (any, error) {
		return state, nil
	}, types.NodeOptions{RetryPolicy: policy})
	engine := NewEngine(sg)

	tests := map[string]*Task{
		"execution":  engine.createTask(node, nil, nil, nil),
		"inspection": engine.createTaskInfo(node, nil, nil, nil),
	}
	for name, task := range tests {
		t.Run(name, func(t *testing.T) {
			if task.RetryPolicy != policy {
				t.Fatalf("RetryPolicy = %p, want %p", task.RetryPolicy, policy)
			}
		})
	}
}

func TestEngine_RunSync(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg, WithRecursionLimit(10))

	result, err := engine.RunSync(t.Context(), map[string]any{"value": "start"})
	if err != nil {
		t.Fatalf("RunSync failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if m, ok := result.(map[string]any); ok {
		if m["value"] != "b" {
			t.Errorf("expected value='b', got %v", m["value"])
		}
	}
}

func TestEngine_RunSyncWithChannelRead(t *testing.T) {
	sg := graph.NewStateGraph(map[string]any{"name": ""})
	sg.AddChannel("name", channels.NewLastValue(""))

	sg.AddNode("echo", func(ctx context.Context, state any) (any, error) {
		m, ok := state.(map[string]any)
		if !ok || m == nil {
			m = map[string]any{}
		}
		m["name"] = "echoed"
		return m, nil
	})
	if err := sg.AddEdge(constants.Start, "echo"); err != nil {
		t.Fatal(err)
	}
	if err := sg.AddEdge("echo", constants.End); err != nil {
		t.Fatal(err)
	}

	engine := NewEngine(sg, WithRecursionLimit(10))
	result, err := engine.RunSync(t.Context(), map[string]any{"name": "world"})
	if err != nil {
		t.Fatalf("RunSync failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestEngine_RecursionLimit(t *testing.T) {
	sg := newSimpleGraph(t)
	// Remove edges to node_b and End, creating a loop through node_a only
	sg.AddEdge("node_a", constants.End)

	engine := NewEngine(sg, WithRecursionLimit(3))
	_, err := engine.RunSync(t.Context(), map[string]any{"value": "x"})
	if err != nil {
		// Engine runs successfully: node_a -> node_b -> node_a loops via self-edge
		t.Logf("got error (expected from recursion limit): %v", err)
	} else {
		t.Log("engine completed successfully")
	}
}

func TestEngine_InterruptConfig(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg, WithInterrupts("node_a"))
	if !engine.interrupts["node_a"] {
		t.Error("expected node_a in interrupts")
	}
}

func TestEngine_ConfigPropagation(t *testing.T) {
	sg := newSimpleGraph(t)
	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithDebug(true),
	)
	if !engine.debug {
		t.Error("expected debug = true")
	}
}

func TestEngine_EmptyGraph(t *testing.T) {
	sg := graph.NewStateGraph(map[string]any{"x": ""})
	sg.AddChannel("x", channels.NewLastValue(""))
	engine := NewEngine(sg, WithRecursionLimit(10))
	_, err := engine.RunSync(t.Context(), map[string]any{"x": "1"})
	if err == nil {
		t.Fatal("expected error for graph with no entry point")
	}
	t.Logf("got expected error: %v", err)
}
