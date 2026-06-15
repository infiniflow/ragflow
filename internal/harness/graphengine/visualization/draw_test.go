// Package visualization provides tests for graph visualization.
package visualization

import (
	"strings"
	"testing"

	"ragflow/internal/harness/graphengine/constants"
)

func TestDrawMermaid(t *testing.T) {
	// Create a simple graph
	g := NewSimpleGraph("start")
	g.AddNode("start")
	g.AddNode("process")
	g.AddNode("end")
	g.AddEdge("start", "process")
	g.AddEdge("process", "end")

	opts := DefaultDrawOptions()
	opts.Format = FormatMermaid
	opts.Horizontal = true

	output, err := DrawGraph(g, opts)
	if err != nil {
		t.Fatalf("DrawGraph failed: %v", err)
	}

	// Check for Mermaid syntax
	if !strings.Contains(output, "graph LR") {
		t.Error("Output should contain 'graph LR' for horizontal layout")
	}

	// Check for nodes
	if !strings.Contains(output, "start") {
		t.Error("Output should contain 'start' node")
	}
	if !strings.Contains(output, "process") {
		t.Error("Output should contain 'process' node")
	}

	// Check for edges
	if !strings.Contains(output, "-->") {
		t.Error("Output should contain edges (-->")
	}
}

func TestDrawGraphviz(t *testing.T) {
	g := NewSimpleGraph("start")
	g.AddEdge("start", "process")
	g.AddEdge("process", constants.End)

	opts := DefaultDrawOptions()
	opts.Format = FormatGraphviz

	output, err := DrawGraph(g, opts)
	if err != nil {
		t.Fatalf("DrawGraph failed: %v", err)
	}

	// Check for DOT syntax
	if !strings.Contains(output, "digraph Graph") {
		t.Error("Output should contain 'digraph Graph'")
	}

	// Check for rankdir
	if !strings.Contains(output, "rankdir=LR") {
		t.Error("Output should contain rankdir for horizontal layout")
	}

	// Check for nodes
	if !strings.Contains(output, "start") {
		t.Error("Output should contain 'start' node")
	}
}

func TestDrawASCII(t *testing.T) {
	g := NewSimpleGraph("start")
	g.AddEdge("start", "middle")
	g.AddConditionalEdge("middle", "condition", constants.End)

	opts := DefaultDrawOptions()
	opts.Format = FormatASCII

	output, err := DrawGraph(g, opts)
	if err != nil {
		t.Fatalf("DrawGraph failed: %v", err)
	}

	// Check for ASCII art headers
	if !strings.Contains(output, "Graph Structure:") {
		t.Error("Output should contain 'Graph Structure:' header")
	}

	// Check for nodes section
	if !strings.Contains(output, "Nodes:") {
		t.Error("Output should contain 'Nodes:' section")
	}

	// Check for edges section
	if !strings.Contains(output, "Edges:") {
		t.Error("Output should contain 'Edges:' section")
	}

	// Check for conditional edges section
	if !strings.Contains(output, "Conditional Edges:") {
		t.Error("Output should contain 'Conditional Edges:' section")
	}
}

func TestDrawGraph_InvalidFormat(t *testing.T) {
	g := NewSimpleGraph("start")

	opts := DefaultDrawOptions()
	opts.Format = "invalid"

	_, err := DrawGraph(g, opts)
	if err == nil {
		t.Error("Should return error for invalid format")
	}
}

func TestDrawGraph_NilGraph(t *testing.T) {
	opts := DefaultDrawOptions()
	_, err := DrawGraph(nil, opts)
	if err == nil {
		t.Error("Should return error for nil graph")
	}
}

func TestSimpleGraph(t *testing.T) {
	g := NewSimpleGraph("start")

	// Test adding nodes
	g.AddNode("node1")
	g.AddNode("node2")

	if len(g.GetNodes()) != 3 { // start + node1 + node2
		t.Errorf("Expected 3 nodes, got %d", len(g.GetNodes()))
	}

	// Test adding duplicate node (should not add)
	g.AddNode("node1")
	if len(g.GetNodes()) != 3 {
		t.Error("Duplicate node should not be added")
	}

	// Test adding edges
	g.AddEdge("start", "node1")
	g.AddEdge("node1", "node2")

	edges := g.GetEdges()
	if len(edges) != 2 {
		t.Errorf("Expected 2 edges, got %d", len(edges))
	}

	// Test conditional edges
	g.AddConditionalEdge("node2", "condition", constants.End)

	condEdges := g.GetConditionalEdges()
	if len(condEdges) != 1 {
		t.Errorf("Expected 1 conditional edge, got %d", len(condEdges))
	}

	// Check conditional edge structure
	if condEdges[0][0] != "node2" || condEdges[0][1] != "condition" || condEdges[0][2] != constants.End {
		t.Error("Conditional edge structure incorrect")
	}
}

func TestSanitizeNodeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"node-name", "node_name"},
		{"node name", "node_name"},
		{"node.name", "node_name"},
		{"123node", "_123node"},
		{"valid_node", "valid_node"},
	}

	for _, tt := range tests {
		result := sanitizeNodeID(tt.input)
		if result != tt.expected {
			t.Errorf("sanitizeNodeID(%q): expected %q, got %q", tt.input, tt.expected, result)
		}
	}
}

func TestSanitizeLabel(t *testing.T) {
	// Test quote escaping
	input := `say "hello"`
	result := sanitizeLabel(input)
	if !strings.Contains(result, `\"`) {
		t.Error("Quotes should be escaped")
	}

	// Test length limiting
	longInput := strings.Repeat("a", 100)
	result = sanitizeLabel(longInput)
	if len(result) > 60 {
		t.Error("Long labels should be truncated")
	}
}

func TestDefaultDrawOptions(t *testing.T) {
	opts := DefaultDrawOptions()

	if opts.Format != FormatMermaid {
		t.Error("Default format should be Mermaid")
	}
	if !opts.Horizontal {
		t.Error("Default should be horizontal layout")
	}
	if !opts.ShowStartEnd {
		t.Error("Default should show start/end nodes")
	}
	if opts.NodeStyles == nil {
		t.Error("NodeStyles should be initialized")
	}
	if opts.EdgeStyles == nil {
		t.Error("EdgeStyles should be initialized")
	}
}

func TestConvenienceFunctions(t *testing.T) {
	g := NewSimpleGraph("start")
	g.AddEdge("start", "end")

	// Test DrawMermaid
	mermaid, err := DrawMermaid(g, true)
	if err != nil {
		t.Errorf("DrawMermaid failed: %v", err)
	}
	if !strings.Contains(mermaid, "graph LR") {
		t.Error("DrawMermaid should produce horizontal graph")
	}

	// Test DrawGraphviz
	dot, err := DrawGraphviz(g, false)
	if err != nil {
		t.Errorf("DrawGraphviz failed: %v", err)
	}
	if !strings.Contains(dot, "digraph Graph") {
		t.Error("DrawGraphviz should produce DOT format")
	}

	// Test DrawASCII
	ascii, err := DrawASCII(g)
	if err != nil {
		t.Errorf("DrawASCII failed: %v", err)
	}
	if !strings.Contains(ascii, "Graph Structure:") {
		t.Error("DrawASCII should produce ASCII art")
	}
}

func TestExportToFormat(t *testing.T) {
	g := NewSimpleGraph("start")
	g.AddEdge("start", "end")

	output, err := ExportToFormat(g, FormatMermaid)
	if err != nil {
		t.Errorf("ExportToFormat failed: %v", err)
	}
	if output == "" {
		t.Error("Export should produce non-empty output")
	}
}
