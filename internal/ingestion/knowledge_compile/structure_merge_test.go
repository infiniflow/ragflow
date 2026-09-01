package knowledge_compile

import (
	"context"
	"strings"
	"testing"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// fakeWriter is a Writer double that records the last WriteMergedStructure
// buckets so a test can assert the grouping behavior without an engine.
type fakeWriter struct {
	buckets []StructureBucket
}

func (f *fakeWriter) WriteMerged(context.Context, string, string, []kccommon.Product) error {
	return nil
}
func (f *fakeWriter) WriteMergedStructure(_ context.Context, _, _ string, buckets []StructureBucket) error {
	f.buckets = buckets
	return nil
}
func (f *fakeWriter) DeleteMergedForVariant(context.Context, string, string, []kccommon.Variant) error {
	return nil
}
func (f *fakeWriter) DeleteStructureForDocs(context.Context, string, string, []string) error {
	return nil
}
func (f *fakeWriter) DeleteMerged(context.Context, string, string) error { return nil }
func (f *fakeWriter) DeleteDocLevelForDocs(context.Context, string, string, []string) error {
	return nil
}
func (f *fakeWriter) StripMergedSources(context.Context, string, string, []string) error {
	return nil
}
func (f *fakeWriter) ProjectWikiGraph(context.Context, string, string) error { return nil }
func (f *fakeWriter) DropWikiGraph(context.Context, string, string) error    { return nil }

// TestMergeStructureDataset_GroupsByNameType covers G1: structure products are
// bucketed by (name, type), descriptions folded, source ids unioned, and only
// scope_kwd="dataset" structure rows written.
func TestMergeStructureDataset_GroupsByNameType(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	products := []kccommon.Product{
		{Variant: kccommon.VariantStructure, DocID: "d1",
			Content: "Engine desc A",
			Meta:    map[string]any{"name": "Engine", "entity_type": "component", "source_chunk_ids": []string{"c1"}}},
		{Variant: kccommon.VariantStructure, DocID: "d2",
			Content: "Engine desc B",
			Meta:    map[string]any{"name": "Engine", "entity_type": "component", "source_chunk_ids": []string{"c2"}}},
		// different type -> separate bucket
		{Variant: kccommon.VariantStructure, DocID: "d3",
			Content: "Fuel desc",
			Meta:    map[string]any{"name": "Fuel", "entity_type": "substance", "source_chunk_ids": []string{"c3"}}},
		// non-structure product ignored
		{Variant: kccommon.VariantWiki, DocID: "d4", Content: "wiki page",
			Meta: map[string]any{"name": "Ignored", "kind": "page"}},
	}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw := c.writer.(*fakeWriter)
	if len(fw.buckets) != 2 {
		t.Fatalf("want 2 buckets (Engine + Fuel), got %d", len(fw.buckets))
	}
	var engineBucket, fuelBucket *StructureBucket
	for i := range fw.buckets {
		switch fw.buckets[i].Name {
		case "Engine":
			engineBucket = &fw.buckets[i]
		case "Fuel":
			fuelBucket = &fw.buckets[i]
		}
	}
	if engineBucket == nil || fuelBucket == nil {
		t.Fatalf("missing expected buckets: %+v", fw.buckets)
	}
	if engineBucket.Type != "component" {
		t.Errorf("Engine type = %q, want component", engineBucket.Type)
	}
	if !strings.Contains(engineBucket.Description, "Engine desc A") || !strings.Contains(engineBucket.Description, "Engine desc B") {
		t.Errorf("Engine descriptions not folded: %q", engineBucket.Description)
	}
	if len(engineBucket.SourceDocIDs) != 2 {
		t.Errorf("Engine source doc union = %v, want [d1 d2]", engineBucket.SourceDocIDs)
	}
}

// TestMergeStructureDataset_RelationsNotDropped covers review issue 3: structure
// relation products (kind=relation with from/to, no name) must be folded into a
// dataset-level relation bucket instead of being silently dropped.
func TestMergeStructureDataset_RelationsNotDropped(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	products := []kccommon.Product{
		{Variant: kccommon.VariantStructure, DocID: "d1", Content: "rel desc",
			Meta: map[string]any{"kind": "relation", "from": "A", "to": "B", "source_chunk_ids": []string{"c1"}}},
		{Variant: kccommon.VariantStructure, DocID: "d2", Content: "rel desc 2",
			Meta: map[string]any{"kind": "relation", "from": "A", "to": "B", "source_chunk_ids": []string{"c2"}}},
	}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw := c.writer.(*fakeWriter)
	if len(fw.buckets) != 1 {
		t.Fatalf("want 1 relation bucket, got %d", len(fw.buckets))
	}
	b := fw.buckets[0]
	if b.Type != "relation" || b.FromEntity != "A" || b.ToEntity != "B" {
		t.Errorf("relation bucket = %+v, want Type=relation From=A To=B", b)
	}
	if len(b.SourceDocIDs) != 2 {
		t.Errorf("relation source doc union = %v, want [d1 d2]", b.SourceDocIDs)
	}
}

// TestMergeStructureDataset_DescriptionIsPlainText covers the description bug:
// the product Content is the doc row's content_with_weight JSON, so the dataset
// row's folded description must be the plain-text "description" field, NOT the raw
// JSON object (which would leak the whole entity payload into the rendered graph).
func TestMergeStructureDataset_DescriptionIsPlainText(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	products := []kccommon.Product{
		{Variant: kccommon.VariantStructure, DocID: "d1",
			Content: `{"category":"Person","description":"汉桓帝，禁锢善类","name":"桓帝","type":"entity"}`,
			Meta:    map[string]any{"name": "桓帝", "entity_type": "Person", "compile_kwd": "list", "source_chunk_ids": []string{"c1"}}},
	}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw := c.writer.(*fakeWriter)
	if len(fw.buckets) != 1 {
		t.Fatalf("want 1 bucket, got %d", len(fw.buckets))
	}
	got := fw.buckets[0].Description
	if got != "汉桓帝，禁锢善类" {
		t.Errorf("description = %q, want plain-text \"汉桓帝，禁锢善类\" (not the raw JSON payload)", got)
	}
	if strings.Contains(got, "{") || strings.Contains(got, "category") {
		t.Errorf("description leaked the JSON payload: %q", got)
	}
}

// TestMergeStructureDataset_CompileKwdIsAutotypeNotTemplateKind covers the
// option-A alignment: the dataset row's compile_kwd is the doc row's inferred
// compile type (autotype, e.g. "hypergraph"/"mindmap"), NOT the template kind.
// The template kind travels on the separate TemplateKind field (stamped to
// compilation_template_kind_kwd by WriteMergedStructure), which is what read/
// delete paths match on. This mirrors Python _do_build (compile_kwd passes
// through verbatim) + get_dataset_structure (matches template kind).
func TestMergeStructureDataset_CompileKwdIsAutotypeNotTemplateKind(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	products := []kccommon.Product{
		{Variant: kccommon.VariantStructure, DocID: "d1", Kind: "knowledge_graph", TemplateID: "tpl-graph",
			Content: "Engine desc",
			Meta:    map[string]any{"name": "Engine", "entity_type": "component", "compile_kwd": "hypergraph", "source_chunk_ids": []string{"c1"}}},
		{Variant: kccommon.VariantMindmap, DocID: "d2", Kind: "mind_map", TemplateID: "tpl-mindmap",
			Content: "Fuel desc",
			Meta:    map[string]any{"name": "Fuel", "entity_type": "substance", "compile_kwd": "mindmap", "source_chunk_ids": []string{"c2"}}},
	}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw := c.writer.(*fakeWriter)
	if len(fw.buckets) != 2 {
		t.Fatalf("want 2 buckets, got %d", len(fw.buckets))
	}
	for i := range fw.buckets {
		b := fw.buckets[i]
		switch b.Name {
		case "Engine":
			if b.CompileKwd != "hypergraph" {
				t.Errorf("Engine compile_kwd = %q, want autotype \"hypergraph\" (not template kind)", b.CompileKwd)
			}
			if b.TemplateKind != "knowledge_graph" {
				t.Errorf("Engine TemplateKind = %q, want \"knowledge_graph\"", b.TemplateKind)
			}
			if b.TemplateID != "tpl-graph" {
				t.Errorf("Engine TemplateID = %q, want \"tpl-graph\"", b.TemplateID)
			}
		case "Fuel":
			if b.CompileKwd != "mindmap" {
				t.Errorf("Fuel compile_kwd = %q, want autotype \"mindmap\" (not template kind)", b.CompileKwd)
			}
			if b.TemplateKind != "mind_map" {
				t.Errorf("Fuel TemplateKind = %q, want \"mind_map\"", b.TemplateKind)
			}
			if b.TemplateID != "tpl-mindmap" {
				t.Errorf("Fuel TemplateID = %q, want \"tpl-mindmap\"", b.TemplateID)
			}
		}
	}
}

// TestDatasetLevelStructureID_StableAndCaseInsensitive covers G1: the id is
// deterministic and case-insensitive on name so merges hit the same row.
func TestDatasetLevelStructureID_StableAndCaseInsensitive(t *testing.T) {
	a := datasetLevelStructureID("t1", "kb1", "Engine", "component", "timeline", "")
	b := datasetLevelStructureID("t1", "kb1", "engine", "component", "timeline", "")
	if a != b {
		t.Errorf("structure id should be case-insensitive on name: %q vs %q", a, b)
	}
	if a == datasetLevelStructureID("t1", "kb1", "Fuel", "component", "timeline", "") {
		t.Errorf("different names must yield different ids")
	}
	if a == datasetLevelStructureID("t1", "kb1", "Engine", "component", "mindmap", "") {
		t.Errorf("different compile kinds must yield different ids")
	}
	if datasetLevelStructureID("t1", "kb1", "A -> B", "relation", "graph", "causes") ==
		datasetLevelStructureID("t1", "kb1", "A -> B", "relation", "graph", "contradicts") {
		t.Errorf("different relation types between the same endpoints must yield different ids")
	}
}

// TestKwdToVariant_MapsStructureSubKinds covers B1a: structure doc-level products
// stamp their inferred compile kind verbatim (hypergraph/list/set/timeline/
// page_index/...), and KwdToVariant must reverse-map each to VariantStructure so
// the reader does NOT drop them before the dataset-level merge / nav dispatch.
func TestKwdToVariant_MapsStructureSubKinds(t *testing.T) {
	for _, kwd := range []string{
		"hypergraph", "list", "set", "timeline", "page_index",
		"session_essence", "session_graph", "knowledge_graph", "graph", "structure",
	} {
		v, err := KwdToVariant(kwd)
		if err != nil {
			t.Errorf("KwdToVariant(%q) unexpected error: %v", kwd, err)
			continue
		}
		if v != kccommon.VariantStructure {
			t.Errorf("KwdToVariant(%q) = %q, want structure", kwd, v)
		}
	}
}

// TestKwdToVariant_RejectsUnknown still hard-fails on a non-whitelisted kwd
// (O2a): a malformed or unknown compile_kwd is rejected, not silently folded.
func TestKwdToVariant_RejectsUnknown(t *testing.T) {
	for _, kwd := range []string{"garbage", "artifact_page", ""} {
		if _, err := KwdToVariant(kwd); err == nil {
			t.Errorf("KwdToVariant(%q) should error", kwd)
		}
	}
}

// TestProductFromChunkMap_RestoresStructureTreeKind covers the second-round fix:
// the reader must restore Meta["kind"] for structure (from knowledge_graph_kwd)
// and tree (from raptor_kwd) products so the dataset-nav dispatch (B2) can pick
// the graph/root summary instead of skipping them.
func TestProductFromChunkMap_RestoresStructureTreeKind(t *testing.T) {
	// structure graph product: compile_kwd is the inferred sub-kind "list",
	// kind is stored in knowledge_graph_kwd.
	structRow := map[string]interface{}{
		"id":                            "s1",
		"doc_id":                        "d1",
		"compile_kwd":                   "list",
		"kc_payload":                    `{"entities":[]}`,
		"knowledge_graph_kwd":           "graph",
		"name_kwd":                      "engine",
		"source_chunk_ids":              []interface{}{"c1"},
		"source_doc_ids":                []interface{}{"d1"},
		"compilation_template_kind_kwd": "list",
	}
	sp, ok := productFromChunkMap(structRow, "t1", kccommon.VariantStructure)
	if !ok {
		t.Fatal("structure row should reconstruct")
	}
	if kind, _ := sp.Meta["kind"].(string); kind != "graph" {
		t.Errorf("structure Meta.kind = %q, want graph (so nav dispatch can pick it)", kind)
	}
	// Round-trip: the raw compile_kwd (autotype) must be preserved in Meta so the
	// dataset merge stamps the SAME value on the dataset row as the doc row.
	if ckwd, _ := sp.Meta["compile_kwd"].(string); ckwd != "list" {
		t.Errorf("structure Meta.compile_kwd = %q, want autotype \"list\" (not \"structure\")", ckwd)
	}

	// tree root product: kind stored in raptor_kwd.
	treeRow := map[string]interface{}{
		"id":                            "t1",
		"doc_id":                        "d1",
		"compile_kwd":                   "tree",
		"kc_payload":                    "overall summary",
		"raptor_kwd":                    "root",
		"source_chunk_ids":              []interface{}{"c1"},
		"source_doc_ids":                []interface{}{"d1"},
		"compilation_template_kind_kwd": "tree",
	}
	tp, ok := productFromChunkMap(treeRow, "t1", kccommon.VariantTree)
	if !ok {
		t.Fatal("tree row should reconstruct")
	}
	if kind, _ := tp.Meta["kind"].(string); kind != "root" {
		t.Errorf("tree Meta.kind = %q, want root (so nav dispatch can pick it)", kind)
	}
}
