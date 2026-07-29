package graph

import (
	"context"
	"testing"

	"ragflow/internal/harness/graph/constants"
)

// State type for testing
type testState struct {
	Messages []string
	Counter  int
}

func TestStateGraphCreation(t *testing.T) {
	builder := NewStateGraph(testState{})

	if builder == nil {
		t.Fatal("Expected non-nil builder")
	}

	if len(builder.GetNodes()) != 0 {
		t.Error("Expected empty nodes initially")
	}
}

func TestAddNode(t *testing.T) {
	builder := NewStateGraph(testState{})

	nodeFn := func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(testState)
		s.Counter++
		return s, nil
	}

	builder.AddNode("test_node", nodeFn)

	node, ok := builder.GetNode("test_node")
	if !ok {
		t.Fatal("Expected to find added node")
	}

	if node.Name != "test_node" {
		t.Errorf("Expected node name 'test_node', got '%s'", node.Name)
	}

	if node.Function == nil {
		t.Error("Expected non-nil function")
	}
}

func TestAddEdge(t *testing.T) {
	builder := NewStateGraph(testState{})

	// Add nodes first
	builder.AddNode("node_a", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})
	builder.AddNode("node_b", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})

	// Add valid edge
	err := builder.AddEdge("node_a", "node_b")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	// Edge to non-existent node should error
	err = builder.AddEdge("node_a", "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent node")
	}

	// Edge from non-existent node should error
	err = builder.AddEdge("nonexistent", "node_b")
	if err == nil {
		t.Error("Expected error for non-existent source node")
	}

	// Special nodes
	err = builder.AddEdge(constants.Start, "node_a")
	if err != nil {
		t.Errorf("Unexpected error for start edge: %v", err)
	}

	err = builder.AddEdge("node_b", constants.End)
	if err != nil {
		t.Errorf("Unexpected error for end edge: %v", err)
	}
}

func TestSetEntryPoint(t *testing.T) {
	builder := NewStateGraph(testState{})

	builder.AddNode("entry", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})

	err := builder.SetEntryPoint("entry")
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	if builder.GetEntryPoint() != "entry" {
		t.Errorf("Expected entry point 'entry', got '%s'", builder.GetEntryPoint())
	}

	// Non-existent node should error
	err = builder.SetEntryPoint("nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent entry point")
	}
}

func TestValidateGraph(t *testing.T) {
	builder := NewStateGraph(testState{})

	// Empty graph should fail validation
	err := builder.Validate()
	if err == nil {
		t.Error("Expected error for empty graph")
	}

	// Add nodes and edges
	builder.AddNode("node_a", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})

	// Still missing entry point and finish points
	err = builder.Validate()
	if err == nil {
		t.Error("Expected error for graph without entry point")
	}

	builder.SetEntryPoint("node_a")

	// Still missing finish points
	err = builder.Validate()
	if err == nil {
		t.Error("Expected error for graph without finish points")
	}

	builder.SetFinishPoint("node_a")

	// Now should validate
	err = builder.Validate()
	if err != nil {
		t.Errorf("Unexpected validation error: %v", err)
	}
}

func TestCompileGraph(t *testing.T) {
	builder := NewStateGraph(testState{})

	builder.AddNode("node_a", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(testState)
		s.Messages = append(s.Messages, "node_a")
		return s, nil
	})

	builder.SetEntryPoint("node_a")
	builder.SetFinishPoint("node_a")

	graph, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile graph: %v", err)
	}

	if graph == nil {
		t.Fatal("Expected non-nil graph")
	}

	if graph.GetGraph() != builder {
		t.Error("Expected graph to reference builder")
	}
}

func TestGraphExecution(t *testing.T) {
	// Create a simple map-based state graph for testing
	builder := NewStateGraph(map[string]interface{}{})

	builder.AddNode("increment", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		counter, _ := s["counter"].(int)
		s["counter"] = counter + 1
		return s, nil
	})

	builder.SetEntryPoint("increment")
	builder.SetFinishPoint("increment")

	graph, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile graph: %v", err)
	}

	ctx := context.Background()
	initialState := map[string]interface{}{
		"counter": 0,
	}

	result, err := graph.Invoke(ctx, initialState)
	if err != nil {
		t.Fatalf("Failed to invoke graph: %v", err)
	}

	finalState, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected map[string]interface{}, got %T", result)
	}

	if finalState["counter"] != 1 {
		t.Errorf("Expected counter=1, got %v", finalState["counter"])
	}
}

func TestGraphWithMultipleNodes(t *testing.T) {
	builder := NewStateGraph(map[string]interface{}{})

	builder.AddNode("first", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		counter, _ := s["counter"].(int)
		s["counter"] = counter + 1
		s["step"] = "first"
		return s, nil
	})

	builder.AddNode("second", func(ctx context.Context, state interface{}) (interface{}, error) {
		s := state.(map[string]interface{})
		counter, _ := s["counter"].(int)
		s["counter"] = counter + 10
		s["step"] = "second"
		return s, nil
	})

	builder.AddEdge(constants.Start, "first")
	builder.AddEdge("first", "second")
	builder.AddEdge("second", constants.End)
	builder.SetEntryPoint("first")
	builder.SetFinishPoint("second")

	graph, err := builder.Compile()
	if err != nil {
		t.Fatalf("Failed to compile graph: %v", err)
	}

	ctx := context.Background()
	initialState := map[string]interface{}{
		"counter": 0,
		"step":    "",
	}

	result, err := graph.Invoke(ctx, initialState)
	if err != nil {
		t.Fatalf("Failed to invoke graph: %v", err)
	}

	finalState := result.(map[string]interface{})

	if finalState["counter"] != 11 {
		t.Errorf("Expected counter=11, got %v", finalState["counter"])
	}

	if finalState["step"] != "second" {
		t.Errorf("Expected step='second', got %v", finalState["step"])
	}
}

func TestCompileOptions(t *testing.T) {
	builder := NewStateGraph(testState{})

	builder.AddNode("node", func(ctx context.Context, state interface{}) (interface{}, error) {
		return state, nil
	})

	builder.SetEntryPoint("node")
	builder.SetFinishPoint("node")

	// Test with recursion limit
	graph, err := builder.Compile(WithRecursionLimit(50))
	if err != nil {
		t.Fatalf("Failed to compile with options: %v", err)
	}

	if graph.GetRecursionLimit() != 50 {
		t.Errorf("Expected recursion limit 50, got %d", graph.GetRecursionLimit())
	}

	// Test with debug
	graph, err = builder.Compile(WithDebug(true))
	if err != nil {
		t.Fatalf("Failed to compile with debug: %v", err)
	}

	if !graph.IsDebug() {
		t.Error("Expected debug to be true")
	}

	// Test with interrupts
	graph, err = builder.Compile(WithInterrupts("node"))
	if err != nil {
		t.Fatalf("Failed to compile with interrupts: %v", err)
	}

	if !graph.GetInterrupts()["node"] {
		t.Error("Expected interrupt for 'node'")
	}
}
