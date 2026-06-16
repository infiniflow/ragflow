// Package visualization provides graph visualization utilities for Agent Harness Go.
// It supports multiple output formats including Mermaid, Graphviz DOT, and ASCII art.
package visualization

import (
	"fmt"
	"sort"
	"strings"

	"ragflow/internal/harness/graph/constants"
)

// DrawFormat represents the output format for graph visualization.
type DrawFormat string

const (
	// FormatASCII generates ASCII art representation.
	FormatASCII DrawFormat = "ascii"
	// FormatMermaid generates Mermaid flowchart syntax.
	FormatMermaid DrawFormat = "mermaid"
	// FormatGraphviz generates Graphviz DOT format.
	FormatGraphviz DrawFormat = "graphviz"
)

// DrawOptions configures the graph drawing behavior.
type DrawOptions struct {
	// Format specifies the output format.
	Format DrawFormat
	// Horizontal determines if the graph should be drawn horizontally (left to right).
	Horizontal bool
	// ShowStartEnd determines if START and END nodes should be shown.
	ShowStartEnd bool
	// NodeStyles allows custom styling for specific nodes.
	NodeStyles map[string]string
	// EdgeStyles allows custom styling for specific edges.
	EdgeStyles map[string]string
}

// DefaultDrawOptions returns default drawing options.
func DefaultDrawOptions() *DrawOptions {
	return &DrawOptions{
		Format:       FormatMermaid,
		Horizontal:   true,
		ShowStartEnd: true,
		NodeStyles:   make(map[string]string),
		EdgeStyles:   make(map[string]string),
	}
}

// GraphProvider is the interface required to draw a graph.
// Implement this interface for your graph type to enable visualization.
type GraphProvider interface {
	// GetNodes returns all node names in the graph.
	GetNodes() []string
	// GetEntryPoint returns the entry point node name.
	GetEntryPoint() string
	// GetEdges returns all edges as (from, to) pairs.
	GetEdges() [][2]string
	// GetConditionalEdges returns conditional edges as (from, condition, to) triples.
	GetConditionalEdges() [][3]string
}

// DrawGraph generates a visual representation of the graph.
func DrawGraph(graph GraphProvider, opts *DrawOptions) (string, error) {
	if graph == nil {
		return "", fmt.Errorf("graph cannot be nil")
	}

	if opts == nil {
		opts = DefaultDrawOptions()
	}

	switch opts.Format {
	case FormatASCII:
		return drawASCII(graph, opts)
	case FormatMermaid:
		return drawMermaid(graph, opts)
	case FormatGraphviz:
		return drawGraphviz(graph, opts)
	default:
		return "", fmt.Errorf("unsupported format: %s", opts.Format)
	}
}

// drawMermaid generates a Mermaid flowchart.
func drawMermaid(graph GraphProvider, opts *DrawOptions) (string, error) {
	var sb strings.Builder

	// Start the diagram
	if opts.Horizontal {
		sb.WriteString("graph LR\n")
	} else {
		sb.WriteString("graph TD\n")
	}

	// Track nodes for styling
	nodes := make(map[string]bool)
	startNode := graph.GetEntryPoint()

	// Add regular edges
	edges := graph.GetEdges()
	for _, edge := range edges {
		from, to := edge[0], edge[1]
		nodes[from] = true
		nodes[to] = true

		fromID := sanitizeNodeID(from)
		toID := sanitizeNodeID(to)

		// Style START and END nodes
		if from == constants.Start && opts.ShowStartEnd {
			fromID = "START"
			sb.WriteString(fmt.Sprintf("    %s((\"\"))\n", fromID))
		}
		if to == constants.End && opts.ShowStartEnd {
			toID = "END"
			sb.WriteString(fmt.Sprintf("    %s(((\"\")))\n", toID))
		}

		edgeStyle := ""
		if style, ok := opts.EdgeStyles[fmt.Sprintf("%s->%s", from, to)]; ok {
			edgeStyle = fmt.Sprintf(" |%s|", style)
		}

		sb.WriteString(fmt.Sprintf("    %s -->%s %s\n", fromID, edgeStyle, toID))
	}

	// Add conditional edges
	condEdges := graph.GetConditionalEdges()
	for _, edge := range condEdges {
		from, condition, to := edge[0], edge[1], edge[2]
		nodes[from] = true
		nodes[to] = true

		fromID := sanitizeNodeID(from)
		toID := sanitizeNodeID(to)

		if to == constants.End && opts.ShowStartEnd {
			toID = "END"
		}

		label := sanitizeLabel(condition)
		sb.WriteString(fmt.Sprintf("    %s -->|\"%s\"| %s\n", fromID, label, toID))
	}

	// Add node styles
	for node := range nodes {
		if style, ok := opts.NodeStyles[node]; ok {
			nodeID := sanitizeNodeID(node)
			sb.WriteString(fmt.Sprintf("    style %s %s\n", nodeID, style))
		}
	}

	// Highlight entry point
	if startNode != "" {
		nodeID := sanitizeNodeID(startNode)
		sb.WriteString(fmt.Sprintf("    style %s fill:#e1f5e1,stroke:#333,stroke-width:2px\n", nodeID))
	}

	return sb.String(), nil
}

// drawGraphviz generates a Graphviz DOT format diagram.
func drawGraphviz(graph GraphProvider, opts *DrawOptions) (string, error) {
	var sb strings.Builder

	sb.WriteString("digraph Graph {\n")

	// Set direction
	if opts.Horizontal {
		sb.WriteString("    rankdir=LR;\n")
	}

	sb.WriteString("    node [shape=box, style=rounded];\n\n")

	// Track nodes
	nodes := make(map[string]bool)
	startNode := graph.GetEntryPoint()

	// Define special nodes
	if opts.ShowStartEnd {
		sb.WriteString("    // Special nodes\n")
		sb.WriteString(fmt.Sprintf("    \"%s\" [shape=circle, label=\"\", width=0.5, style=filled, fillcolor=green];\n", constants.Start))
		sb.WriteString(fmt.Sprintf("    \"%s\" [shape=doublecircle, label=\"\", width=0.5, style=filled, fillcolor=red];\n\n", constants.End))
	}

	// Define regular nodes
	sb.WriteString("    // Nodes\n")
	allNodes := graph.GetNodes()
	sort.Strings(allNodes)

	for _, node := range allNodes {
		nodes[node] = true
		attrs := []string{fmt.Sprintf("label=\"%s\"", node)}

		// Highlight entry point
		if node == startNode {
			attrs = append(attrs, "style=filled", "fillcolor=lightblue")
		} else if style, ok := opts.NodeStyles[node]; ok {
			attrs = append(attrs, fmt.Sprintf("style=filled, fillcolor=%s", style))
		}

		sb.WriteString(fmt.Sprintf("    \"%s\" [%s];\n", node, strings.Join(attrs, ", ")))
	}

	// Add edges
	sb.WriteString("\n    // Edges\n")

	// Regular edges
	edges := graph.GetEdges()
	for _, edge := range edges {
		from, to := edge[0], edge[1]
		attrs := ""
		if style, ok := opts.EdgeStyles[fmt.Sprintf("%s->%s", from, to)]; ok {
			attrs = fmt.Sprintf(" [%s]", style)
		}
		sb.WriteString(fmt.Sprintf("    \"%s\" -> \"%s\"%s;\n", from, to, attrs))
	}

	// Conditional edges
	condEdges := graph.GetConditionalEdges()
	for _, edge := range condEdges {
		from, condition, to := edge[0], edge[1], edge[2]
		label := sanitizeLabel(condition)
		sb.WriteString(fmt.Sprintf("    \"%s\" -> \"%s\" [label=\"%s\"];\n", from, to, label))
	}

	sb.WriteString("}\n")

	return sb.String(), nil
}

// drawASCII generates a simple ASCII art representation.
func drawASCII(graph GraphProvider, opts *DrawOptions) (string, error) {
	var sb strings.Builder

	sb.WriteString("Graph Structure:\n")
	sb.WriteString(strings.Repeat("=", 50))
	sb.WriteString("\n\n")

	// Entry point
	startNode := graph.GetEntryPoint()
	sb.WriteString(fmt.Sprintf("Entry Point: %s\n\n", startNode))

	// Nodes
	sb.WriteString("Nodes:\n")
	nodes := graph.GetNodes()
	sort.Strings(nodes)
	for _, node := range nodes {
		marker := "  "
		if node == startNode {
			marker = "* "
		}
		sb.WriteString(fmt.Sprintf("  %s%s\n", marker, node))
	}

	// Edges
	sb.WriteString("\nEdges:\n")
	edges := graph.GetEdges()
	for _, edge := range edges {
		sb.WriteString(fmt.Sprintf("  %s --> %s\n", edge[0], edge[1]))
	}

	// Conditional edges
	condEdges := graph.GetConditionalEdges()
	if len(condEdges) > 0 {
		sb.WriteString("\nConditional Edges:\n")
		for _, edge := range condEdges {
			sb.WriteString(fmt.Sprintf("  %s --[%s]--> %s\n", edge[0], edge[1], edge[2]))
		}
	}

	return sb.String(), nil
}

// sanitizeNodeID creates a valid Mermaid/Graphviz node ID.
func sanitizeNodeID(name string) string {
	// Replace special characters
	id := strings.ReplaceAll(name, "-", "_")
	id = strings.ReplaceAll(id, " ", "_")
	id = strings.ReplaceAll(id, ".", "_")

	// Ensure it starts with a letter or underscore
	if len(id) > 0 && id[0] >= '0' && id[0] <= '9' {
		id = "_" + id
	}

	return id
}

// sanitizeLabel creates a safe label string.
func sanitizeLabel(label string) string {
	// Escape quotes
	label = strings.ReplaceAll(label, "\"", "\\\"")
	// Limit length
	if len(label) > 50 {
		label = label[:47] + "..."
	}
	return label
}

// DrawMermaid is a convenience function to draw a graph in Mermaid format.
func DrawMermaid(graph GraphProvider, horizontal bool) (string, error) {
	opts := DefaultDrawOptions()
	opts.Format = FormatMermaid
	opts.Horizontal = horizontal
	return DrawGraph(graph, opts)
}

// DrawGraphviz is a convenience function to draw a graph in Graphviz DOT format.
func DrawGraphviz(graph GraphProvider, horizontal bool) (string, error) {
	opts := DefaultDrawOptions()
	opts.Format = FormatGraphviz
	opts.Horizontal = horizontal
	return DrawGraph(graph, opts)
}

// DrawASCII is a convenience function to draw a graph in ASCII format.
func DrawASCII(graph GraphProvider) (string, error) {
	opts := DefaultDrawOptions()
	opts.Format = FormatASCII
	return DrawGraph(graph, opts)
}

// SimpleGraph is a simple implementation of GraphProvider for testing and examples.
type SimpleGraph struct {
	Nodes             []string
	EntryPointNode    string
	RegularEdges      [][2]string
	ConditionalEdges  [][3]string
}

// GetNodes returns all nodes.
func (g *SimpleGraph) GetNodes() []string {
	return g.Nodes
}

// GetEntryPoint returns the entry point.
func (g *SimpleGraph) GetEntryPoint() string {
	return g.EntryPointNode
}

// GetEdges returns regular edges.
func (g *SimpleGraph) GetEdges() [][2]string {
	return g.RegularEdges
}

// GetConditionalEdges returns conditional edges.
func (g *SimpleGraph) GetConditionalEdges() [][3]string {
	return g.ConditionalEdges
}

// NewSimpleGraph creates a new simple graph for visualization.
func NewSimpleGraph(entryPoint string) *SimpleGraph {
	return &SimpleGraph{
		Nodes:            []string{entryPoint},
		EntryPointNode:   entryPoint,
		RegularEdges:     make([][2]string, 0),
		ConditionalEdges: make([][3]string, 0),
	}
}

// AddNode adds a node to the graph.
func (g *SimpleGraph) AddNode(node string) {
	for _, n := range g.Nodes {
		if n == node {
			return
		}
	}
	g.Nodes = append(g.Nodes, node)
}

// AddEdge adds a regular edge.
func (g *SimpleGraph) AddEdge(from, to string) {
	g.AddNode(from)
	g.AddNode(to)
	g.RegularEdges = append(g.RegularEdges, [2]string{from, to})
}

// AddConditionalEdge adds a conditional edge.
func (g *SimpleGraph) AddConditionalEdge(from, condition, to string) {
	g.AddNode(from)
	g.AddNode(to)
	g.ConditionalEdges = append(g.ConditionalEdges, [3]string{from, condition, to})
}

// ExportToFile exports the graph visualization to a string that can be saved to a file.
// For Mermaid format, this can be used with Mermaid-compatible tools.
// For Graphviz, use the 'dot' command: dot -Tpng input.dot -o output.png
func ExportToFormat(graph GraphProvider, format DrawFormat) (string, error) {
	opts := DefaultDrawOptions()
	opts.Format = format
	return DrawGraph(graph, opts)
}
