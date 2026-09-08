package service

import (
	"reflect"
	"testing"
)

// TestProjectEntity_FromPayload verifies projectEntity maps the tree-node
// payload shape to the graph-node shape (mirroring _struct_graph_entity).
func TestProjectEntity_FromPayload(t *testing.T) {
	row := map[string]interface{}{
		"content_with_weight": `{"name":"NVIDIA","type":"tree_node","description":"chip maker","source_chunk_ids":["c1","c2"]}`,
		"source_chunk_ids":    []string{"c1"},
		"mention_count_int":   3,
	}
	n := projectEntity(row)
	if n == nil {
		t.Fatal("projectEntity returned nil")
	}
	if n["name"] != "NVIDIA" || n["type"] != "tree_node" {
		t.Errorf("name/type = %v/%v, want NVIDIA/tree_node", n["name"], n["type"])
	}
	if n["mention_count"] != 3 {
		t.Errorf("mention_count = %v, want 3", n["mention_count"])
	}
	chunks, _ := n["source_chunk_ids"].([]string)
	if !reflect.DeepEqual(chunks, []string{"c1", "c2"}) {
		t.Errorf("source_chunk_ids = %v, want [c1 c2]", chunks)
	}
}

// TestProjectEntity_RejectsInvalidName verifies sentinel/empty names are dropped.
func TestProjectEntity_RejectsInvalidName(t *testing.T) {
	for _, payload := range []string{`{"name":""}`, `{"name":"unknown"}`} {
		if n := projectEntity(map[string]interface{}{"content_with_weight": payload}); n != nil {
			t.Errorf("expected nil for payload %s, got %v", payload, n)
		}
	}
}

// TestProjectRelation_FromPayload verifies the relation projection.
func TestProjectRelation_FromPayload(t *testing.T) {
	row := map[string]interface{}{"content_with_weight": `{"from":"NVIDIA","to":"GPU","type":"child"}`}
	r := projectRelation(row)
	if r == nil {
		t.Fatal("projectRelation returned nil")
	}
	if r["from"] != "NVIDIA" || r["to"] != "GPU" || r["type"] != "child" {
		t.Errorf("relation = %v, want {NVIDIA GPU child}", r)
	}
}

// TestProjectRelation_FallsBackToKwdColumns verifies the authoritative *_entity_kwd
// columns are used when the payload has no from/to.
func TestProjectRelation_FallsBackToKwdColumns(t *testing.T) {
	row := map[string]interface{}{
		"content_with_weight": `{"type":"related"}`,
		"from_entity_kwd":     "NVIDIA",
		"to_entity_kwd":       "GPU",
	}
	r := projectRelation(row)
	if r == nil || r["from"] != "NVIDIA" || r["to"] != "GPU" || r["type"] != "related" {
		t.Errorf("relation = %v, want {NVIDIA GPU related}", r)
	}
}

// TestDedupEntities_OrderPreserving verifies dedup by (lowercased name, type).
func TestDedupEntities_OrderPreserving(t *testing.T) {
	in := []StructureGraphNode{
		{"name": "A", "type": "x"},
		{"name": "a", "type": "x"}, // dup (case-insensitive)
		{"name": "B", "type": "y"},
		{"name": ""}, // dropped (empty name)
	}
	out := dedupEntities(in)
	if len(out) != 2 || out[0]["name"] != "A" || out[1]["name"] != "B" {
		t.Errorf("dedupEntities = %v, want [A B]", out)
	}
}

// TestNormalizeRelationEndpoints aligns relation endpoints to entity ids/names.
func TestNormalizeRelationEndpoints(t *testing.T) {
	entities := []StructureGraphNode{{"name": "NVIDIA", "type": "x"}}
	relations := []StructureGraphRelation{{"from": "nvidia", "to": "GPU", "type": "child"}}
	out := normalizeRelationEndpoints(entities, relations)
	if out[0]["from"] != "NVIDIA" {
		t.Errorf("normalized from = %v, want NVIDIA (matched to entity name)", out[0]["from"])
	}
}

// TestCompilationTemplateKind_Normalization covers kind normalization.
func TestCompilationTemplateKind_Normalization(t *testing.T) {
	if compilationTemplateKind("Page_Index") != "page_index" {
		t.Errorf("Page_Index => %q, want page_index", compilationTemplateKind("Page_Index"))
	}
	if compilationTemplateKind("tree") != "tree" {
		t.Errorf("tree => %q, want tree", compilationTemplateKind("tree"))
	}
}

// TestRowTemplateID_FromList covers compilation_template_ids extraction.
func TestRowTemplateID_FromList(t *testing.T) {
	row := map[string]interface{}{"compilation_template_ids": []interface{}{"", "tid1", "tid2"}}
	if got := rowTemplateID(row); got != "tid1" {
		t.Errorf("rowTemplateID = %q, want tid1", got)
	}
}
