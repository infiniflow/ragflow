package tree

import (
	"reflect"
	"testing"

	"ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// TestRaptorTreeToGraph_CollapsesUnaryAndProjects verifies the tree→graph
// projection matches Python raptor_tree_to_graph: unary chains are collapsed
// (descriptions concatenated), each node becomes an entity of type tree_node,
// and each parent→child edge (skipping self-loops) becomes a child relation.
func TestRaptorTreeToGraph_CollapsesUnaryAndProjects(t *testing.T) {
	// Build a tree: root → [X, A → [B]]. A is a unary wrapper (only child B); the
	// collapse merges A into B, while root (two children) and B (no children)
	// stay. Entities: root, X, B. Relations: root→X, root→B.
	root := &graphNode{
		title:       "root",
		description: "root desc",
		children: []*graphNode{
			{title: "X", description: "X desc"},
			{
				title:       "A",
				description: "A desc",
				children: []*graphNode{
					{title: "B", description: "B desc", sourceChunkIDs: []string{"c1"}},
				},
			},
		},
	}
	root = collapseUnary(root)
	entities, relations := raptorTreeToGraph(root)

	// The unary node A survives but the child B is folded into it (it keeps A's
	// title, concatenated descriptions and B's source chunk ids), matching
	// Python _collapse_unary. Entities: root, X, A.
	var names []string
	for _, e := range entities {
		names = append(names, e["name"].(string))
	}
	if !reflect.DeepEqual(names, []string{"root", "X", "A"}) {
		t.Fatalf("entities after unary collapse = %v, want [root X A]", names)
	}
	// A must have B folded in: concatenated descriptions + the source chunk id.
	byName := map[string]map[string]any{}
	for _, e := range entities {
		byName[e["name"].(string)] = e
	}
	a := byName["A"]
	if a["description"] != "A desc\n\nB desc" {
		t.Errorf("collapsed A description = %q, want %q", a["description"], "A desc\n\nB desc")
	}
	if !reflect.DeepEqual(a["source_chunk_ids"], []string{"c1"}) {
		t.Errorf("collapsed A source_chunk_ids = %v, want [c1]", a["source_chunk_ids"])
	}
	if a["type"] != "tree_node" {
		t.Errorf("entity type = %v, want tree_node", a["type"])
	}
	// Two relations: root → X and root → A (A is root's child after collapse).
	if len(relations) != 2 {
		t.Fatalf("relations = %v, want exactly [root->X, root->A]", relations)
	}
	wantRels := []struct{ from, to string }{{"root", "X"}, {"root", "A"}}
	gotRels := []struct{ from, to string }{
		{relations[0]["from"].(string), relations[0]["to"].(string)},
		{relations[1]["from"].(string), relations[1]["to"].(string)},
	}
	if !reflect.DeepEqual(gotRels, wantRels) {
		t.Errorf("relations = %v, want %v", gotRels, wantRels)
	}
}

// TestRaptorTreeToGraph_SkipsSelfLoop verifies a parent whose child has the same
// title does not produce a self-loop relation (Python's guard).
func TestRaptorTreeToGraph_SkipsSelfLoop(t *testing.T) {
	root := &graphNode{
		title: "same",
		children: []*graphNode{
			{title: "same", description: "child"},
		},
	}
	_, relations := raptorTreeToGraph(root)
	if len(relations) != 0 {
		t.Fatalf("self-loop relation must be skipped, got %v", relations)
	}
}

// TestReconstructTree_FromFlatProducts verifies the flat summary products
// (root + ParentID chains) are reassembled into a nested tree.
func TestReconstructTree_FromFlatProducts(t *testing.T) {
	products := []common.Product{
		{ID: "root", Meta: map[string]any{"kind": "root", "title": "root"}},
		{ID: "n1", ParentID: "root", Meta: map[string]any{"kind": "summary", "title": "N1"}, Content: "n1 desc"},
		{ID: "n2", ParentID: "n1", Meta: map[string]any{"kind": "summary", "title": "N2"}, Content: "n2 desc"},
	}
	root := reconstructTree(products)
	if root == nil || root.title != "root" {
		t.Fatalf("reconstructed root = %+v, want title root", root)
	}
	if len(root.children) != 1 || root.children[0].title != "N1" {
		t.Fatalf("root children = %+v, want [N1]", root.children)
	}
	if len(root.children[0].children) != 1 || root.children[0].children[0].title != "N2" {
		t.Fatalf("N1 children = %+v, want [N2]", root.children[0].children)
	}
}
