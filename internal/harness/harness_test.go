package harness

import (
	"context"
	"testing"
)

func TestNewStateGraph(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{})
	if sg == nil {
		t.Fatal("expected non-nil StateGraph")
	}
}

func TestNewMemorySaver(t *testing.T) {
	ms := NewMemorySaver()
	if ms == nil {
		t.Fatal("expected non-nil MemorySaver")
	}
}

func TestCompileSimpleGraph(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{"value": ""})
	sg.AddNode("echo", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})
	sg.AddEdge(Start, "echo")
	sg.AddEdge("echo", End)

	cg, err := sg.Compile()
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if cg == nil {
		t.Fatal("expected non-nil compiled graph")
	}

	// Verify graph structure
	if cg.GetGraph() == nil {
		t.Fatal("expected non-nil underlying graph")
	}
}

func TestCompileWithRecursionLimit(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{})
	sg.AddNode("n", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})
	sg.AddEdge(Start, "n")
	sg.AddEdge("n", End)

	cg, err := sg.Compile(WithRecursionLimit(5))
	if err != nil {
		t.Fatalf("Compile failed: %v", err)
	}
	if cg == nil {
		t.Fatal("expected non-nil compiled graph")
	}
}

func TestStateGraphNodeOperations(t *testing.T) {
	sg := NewStateGraph(map[string]interface{}{})
	sg.AddNode("agent", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})

	node, ok := sg.GetNode("agent")
	if !ok {
		t.Fatal("expected to find node 'agent'")
	}
	if node.Name != "agent" {
		t.Errorf("expected node name 'agent', got '%s'", node.Name)
	}
}

func TestNewCommand(t *testing.T) {
	cmd := NewCommand()
	if cmd == nil {
		t.Fatal("expected non-nil Command")
	}
}

func TestNewSend(t *testing.T) {
	s := NewSend("target", "arg")
	if s == nil {
		t.Fatal("expected non-nil Send")
	}
	if s.Node != "target" {
		t.Errorf("expected Node='target', got '%s'", s.Node)
	}
}

func TestDefaultRetryPolicy(t *testing.T) {
	rp := DefaultRetryPolicy()
	if rp.MaxAttempts == 0 {
		t.Error("expected non-zero MaxAttempts in default retry policy")
	}
}
