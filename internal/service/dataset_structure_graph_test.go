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

// TestProjectRelation_CarriesItsEvidence verifies an assertion names the chunks
// and documents it came from: without them the ontology view can say an edge
// exists but not where it was read, which is the only way to judge it.
func TestProjectRelation_CarriesItsEvidence(t *testing.T) {
	row := map[string]interface{}{
		"content_with_weight": `{"from":"Ada","to":"London","type":"born_in","source_chunk_ids":["c2"]}`,
		"doc_id":              "d1",
		"source_chunk_ids":    []string{"c1", "c2"},
	}
	r := projectRelation(row)
	if r == nil {
		t.Fatal("projectRelation returned nil")
	}
	if chunks, _ := r["source_chunk_ids"].([]string); !reflect.DeepEqual(chunks, []string{"c2"}) {
		t.Errorf("source_chunk_ids = %v, want the payload's [c2]", chunks)
	}
	if docs, _ := r["document_ids"].([]string); !reflect.DeepEqual(docs, []string{"d1"}) {
		t.Errorf("document_ids = %v, want [d1]", docs)
	}
}

// TestProjectEntity_CarriesItsDocument verifies the entity page can open an
// instance in the document it was compiled from.
func TestProjectEntity_CarriesItsDocument(t *testing.T) {
	row := map[string]interface{}{
		"content_with_weight": `{"name":"Ada","type":"person"}`,
		"doc_id":              "d1",
	}
	n := projectEntity(row)
	if n == nil {
		t.Fatal("projectEntity returned nil")
	}
	if docs, _ := n["document_ids"].([]string); !reflect.DeepEqual(docs, []string{"d1"}) {
		t.Errorf("document_ids = %v, want [d1]", docs)
	}
}

// TestProjectRowsCarryTheModelVersion pins the version stamp's read side: a row
// extracted by a published model version reports it, and a row that predates the
// column reports nothing rather than version 0 — the two mean different things
// ("unknown" vs "never published").
func TestProjectRowsCarryTheModelVersion(t *testing.T) {
	row := map[string]interface{}{
		"content_with_weight":              `{"name":"Ada","type":"person"}`,
		"doc_id":                           "d1",
		"compilation_template_version_int": float64(3),
	}
	node := projectEntity(row)
	if version := inspectRowInt(node, "version"); version != 3 {
		t.Errorf("entity version = %v, want 3", node["version"])
	}

	relation := projectRelation(map[string]interface{}{
		"content_with_weight":              `{"from":"Ada","to":"London","type":"born_in"}`,
		"doc_id":                           "d1",
		"compilation_template_version_int": int64(4),
	})
	if relation == nil {
		t.Fatal("projectRelation returned nil")
	}
	if version := inspectRowInt(relation, "version"); version != 4 {
		t.Errorf("relation version = %v, want 4", relation["version"])
	}

	unversioned := projectEntity(map[string]interface{}{
		"content_with_weight": `{"name":"Ada","type":"person"}`,
		"doc_id":              "d1",
	})
	if _, present := unversioned["version"]; present {
		t.Errorf("unversioned row carries a version: %v", unversioned)
	}
}

// TestGraphSourceDocIDsKeepsDatasetScopeHonest pins the dataset-level row: its
// doc_id is the KB id, so reporting it as a document would send the reader to a
// document that does not exist. The compiled documents are the source_doc_ids
// union instead.
func TestGraphSourceDocIDsKeepsDatasetScopeHonest(t *testing.T) {
	datasetRow := map[string]interface{}{
		"doc_id":         "kb1",
		"scope_kwd":      "dataset",
		"source_doc_ids": []string{"d1", "d2"},
	}
	if docs := graphSourceDocIDs(nil, datasetRow); !reflect.DeepEqual(docs, []string{"d1", "d2"}) {
		t.Errorf("dataset row documents = %v, want [d1 d2]", docs)
	}
	bareDatasetRow := map[string]interface{}{"doc_id": "kb1", "scope_kwd": "dataset"}
	if docs := graphSourceDocIDs(nil, bareDatasetRow); len(docs) != 0 {
		t.Errorf("dataset row without source docs = %v, want none", docs)
	}
	docRow := map[string]interface{}{"doc_id": "d1", "scope_kwd": "doc"}
	if docs := graphSourceDocIDs(nil, docRow); !reflect.DeepEqual(docs, []string{"d1"}) {
		t.Errorf("document row documents = %v, want [d1]", docs)
	}
}

// TestMergeEntities_FoldsByNameAndClass covers the identity rule and the fields
// that must survive the fold: same name + same class is one instance whose
// sources and quotes are unioned, a different class is a different instance, and
// an empty name is dropped.
func TestMergeEntities_FoldsByNameAndClass(t *testing.T) {
	in := []StructureGraphNode{
		{
			"name": "A", "type": "x", "description": "folded",
			"source_chunk_ids": []string{"c1"}, "document_ids": []string{"d1"},
			"evidence": []map[string]interface{}{{"quote": "q1", "chunk_id": "c1"}},
		},
		{
			"name": "a", "type": "x", "description": "",
			"source_chunk_ids": []string{"c2"}, "document_ids": []string{"d2"},
			"evidence": []map[string]interface{}{{"quote": "q1", "chunk_id": "c1"}, {"quote": "q2", "chunk_id": "c2"}},
			"scope":    "dataset",
		},
		{"name": "A", "type": "y"}, // same name, other class: a different instance
		{"name": ""},               // dropped (empty name)
	}
	out := mergeEntities(in)
	if len(out) != 2 {
		t.Fatalf("mergeEntities = %v, want 2 instances", out)
	}
	if out[0]["description"] != "folded" {
		t.Errorf("description was lost in the fold: %v", out[0])
	}
	if got := stringSliceValue(out[0]["source_chunk_ids"]); len(got) != 2 {
		t.Errorf("source chunks not unioned: %v", got)
	}
	if got := stringSliceValue(out[0]["document_ids"]); len(got) != 2 {
		t.Errorf("documents not unioned: %v", got)
	}
	evidence, _ := out[0]["evidence"].([]map[string]interface{})
	if len(evidence) != 2 {
		t.Errorf("evidence not unioned without duplicates: %v", out[0]["evidence"])
	}
	if out[1]["type"] != "y" {
		t.Errorf("second class should be its own instance: %v", out[1])
	}
}

// TestMergeEntities_FoldsAttributes covers the attribute fold: a document row
// and the dataset row of one instance are the same instance seen twice, and the
// dataset row is the one that holds the attributes of the documents compiled
// earlier — so the fold has to keep both sides' values, not just the first's.
func TestMergeEntities_FoldsAttributes(t *testing.T) {
	in := []StructureGraphNode{
		{
			"name": "Virginia Woolf", "type": "person",
			"attributes": map[string]interface{}{"occupation": []interface{}{"writer"}},
		},
		{
			"name": "virginia woolf", "type": "person",
			"attributes": map[string]interface{}{
				"occupation": []interface{}{"writer", "publisher"},
				"birth_date": "1882-01-25",
			},
		},
	}
	out := mergeEntities(in)
	if len(out) != 1 {
		t.Fatalf("mergeEntities = %v, want one instance", out)
	}
	attrs, _ := out[0]["attributes"].(map[string]interface{})
	occupations, _ := structureAttrList(attrs["occupation"])
	if len(occupations) != 2 || occupations[0] != "writer" || occupations[1] != "publisher" {
		t.Errorf("occupation = %v, want [writer publisher] without duplicates", attrs["occupation"])
	}
	if attrs["birth_date"] != "1882-01-25" {
		t.Errorf("birth_date was dropped in the fold: %v", attrs)
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

// TestResolveGraphBucket_UsesRawStructureMetadata covers the post-#19474
// storage shape: entity/relation rows carry the template stamp, with no
// synthetic knowledge_graph_kwd="graph" row available for discovery.
func TestResolveGraphBucket_UsesRawStructureMetadata(t *testing.T) {
	row := map[string]interface{}{
		"knowledge_graph_kwd":           "entity",
		"compilation_template_ids":      []string{"tree-1"},
		"compilation_template_kind_kwd": "tree",
		"compile_kwd":                   "tree",
	}
	meta, scope := resolveGraphBucket(row, map[string]map[string]interface{}{
		"tree-1": {"template_name": "Tree", "kind": "tree"},
	}, "doc-1")
	if meta["template_id"] != "tree-1" || meta["template_name"] != "Tree" {
		t.Fatalf("meta = %#v, want resolved template metadata", meta)
	}
	if got := scope["compilation_template_ids"].([]string); len(got) != 1 || got[0] != "tree-1" {
		t.Fatalf("scope = %#v, want template-scoped raw-row filter", scope)
	}
}

func TestBuildDocumentGraphTemplateShells_PreservesTemplateMetadata(t *testing.T) {
	templates := buildDocumentGraphTemplateShells([]string{"tree-1"}, map[string]map[string]interface{}{
		"tree-1": {"template_name": "Tree", "kind": "tree"},
	})
	if len(templates) != 1 {
		t.Fatalf("got %d templates, want 1", len(templates))
	}
	if templates[0].TemplateID != "tree-1" || templates[0].TemplateName != "Tree" || templates[0].Kind != "tree" {
		t.Fatalf("template = %#v, want metadata for tree-1", templates[0])
	}
	if templates[0].Entities == nil || templates[0].Relations == nil {
		t.Fatal("empty template collections must be non-nil")
	}
}

// TestTemplateDisplayKind_PrefersTemplateRow locks the shells-path kind rule:
// the template row's kind wins over the canonical stored keyword ("graph"),
// so keyword no-match shells answer in the same dialect as the normal-mode
// bucket ("knowledge_graph") and the frontend keeps recognizing the template.
func TestTemplateDisplayKind_PrefersTemplateRow(t *testing.T) {
	if got := templateDisplayKind("graph", "knowledge_graph"); got != "knowledge_graph" {
		t.Errorf("templateDisplayKind(graph, knowledge_graph) = %q, want knowledge_graph", got)
	}
	if got := templateDisplayKind("graph", ""); got != "graph" {
		t.Errorf("templateDisplayKind(graph, \"\") = %q, want row-kind fallback graph", got)
	}
}

// TestStructureRowSourceOrder_MinChunkOrder verifies a row's source position is
// the smallest chunk_order_int among its source_chunk_ids, and that a row with
// no resolvable chunks reports ok=false (so it sorts to the end).
func TestStructureRowSourceOrder_MinChunkOrder(t *testing.T) {
	order := map[string]int{"c1": 5, "c2": 2, "c3": 9}
	row := map[string]interface{}{"source_chunk_ids": []interface{}{"c1", "c2", "c3"}}
	got, ok := structureRowSourceOrder(row, order)
	if !ok || got != 2 {
		t.Fatalf("structureRowSourceOrder = (%d,%v), want (2,true)", got, ok)
	}
	if _, ok := structureRowSourceOrder(map[string]interface{}{"source_chunk_ids": []string{"zz"}}, order); ok {
		t.Fatal("expected ok=false when no source chunk resolves")
	}
}

// TestNormalizeRelationEndpoints_WhitespaceInsensitive verifies an endpoint that
// differs from the entity name only by the space character (half-width U+0020
// vs ideographic U+3000) is still aligned to the entity, so the exact-name map
// lookup downstream (frontend tree building) matches it.
func TestNormalizeRelationEndpoints_WhitespaceInsensitive(t *testing.T) {
	entities := []StructureGraphNode{{"name": "第一章 文忠公", "type": "title"}}
	relations := []StructureGraphRelation{{"from": "第一章\u3000文忠公", "to": "某句", "type": "include"}}
	out := normalizeRelationEndpoints(entities, relations)
	if out[0]["from"] != "第一章 文忠公" {
		t.Errorf("from = %q, want the entity name 第一章 文忠公", out[0]["from"])
	}
	if out[0]["to"] != "某句" {
		t.Errorf("to = %q, want unchanged when no entity matches", out[0]["to"])
	}
}
