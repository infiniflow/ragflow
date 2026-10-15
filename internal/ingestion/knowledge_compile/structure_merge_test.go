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
	buckets               []StructureBucket
	writeMergedCalls      int
	projectWikiGraphCalls int
}

func (f *fakeWriter) WriteMerged(context.Context, string, string, []kccommon.Product) error {
	f.writeMergedCalls++
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
func (f *fakeWriter) ProjectWikiGraph(context.Context, string, string) error {
	f.projectWikiGraphCalls++
	return nil
}
func (f *fakeWriter) DropWikiGraph(context.Context, string, string) error { return nil }

// TestMergeStructureDataset_GroupsByName covers G1: structure products are
// bucketed by name, descriptions folded, source ids unioned, and only
// scope_kwd="dataset" structure rows written.
func TestMergeStructureDataset_GroupsByName(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	products := []kccommon.Product{
		{Variant: kccommon.VariantStructure, DocID: "d1",
			Content: "Engine desc A",
			Meta:    map[string]any{"name": "Engine", "entity_type": "component", "source_chunk_ids": []string{"c1"}}},
		{Variant: kccommon.VariantStructure, DocID: "d2",
			Content: "Engine desc B",
			Meta:    map[string]any{"name": "Engine", "entity_type": "component", "source_chunk_ids": []string{"c2"}}},
		// same name with a different type still belongs to the same graph node
		{Variant: kccommon.VariantStructure, DocID: "d3",
			Content: "Engine desc C",
			Meta:    map[string]any{"name": "Engine", "entity_type": "other", "source_chunk_ids": []string{"c3"}}},
		// non-structure product ignored
		{Variant: kccommon.VariantWiki, DocID: "d4", Content: "wiki page",
			Meta: map[string]any{"name": "Ignored", "kind": "page"}},
	}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw := c.writer.(*fakeWriter)
	if len(fw.buckets) != 1 {
		t.Fatalf("want 1 bucket (Engine), got %d", len(fw.buckets))
	}
	var engineBucket *StructureBucket
	for i := range fw.buckets {
		switch fw.buckets[i].Name {
		case "Engine":
			engineBucket = &fw.buckets[i]
		}
	}
	if engineBucket == nil {
		t.Fatalf("missing expected buckets: %+v", fw.buckets)
	}
	if engineBucket.Type != "component" {
		t.Errorf("Engine type = %q, want component", engineBucket.Type)
	}
	if !strings.Contains(engineBucket.Description, "Engine desc A") || !strings.Contains(engineBucket.Description, "Engine desc B") || !strings.Contains(engineBucket.Description, "Engine desc C") {
		t.Errorf("Engine descriptions not folded: %q", engineBucket.Description)
	}
	if len(engineBucket.SourceDocIDs) != 3 {
		t.Errorf("Engine source doc union = %v, want [d1 d2 d3]", engineBucket.SourceDocIDs)
	}
}

// TestMergeStructureDataset_SplitsSameNameDifferentClasses covers the fold key
// carrying the class: a name shared by two classes must produce one dataset row
// per class, because a row states a single entity_type_kwd and folding them
// would make a class filter answer with one class's instances while dropping the
// other's (ontology2.md §3.2, 同名不同义).
func TestMergeStructureDataset_SplitsSameNameDifferentClasses(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	entity := func(docID, typ, chunk string) kccommon.Product {
		return kccommon.Product{
			Variant: kccommon.VariantStructure, DocID: docID, Content: "Apple desc " + typ,
			Meta: map[string]any{"name": "Apple", "entity_type": typ, "source_chunk_ids": []string{chunk}},
		}
	}
	products := []kccommon.Product{entity("d1", "fruit", "c1"), entity("d2", "company", "c2")}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw := c.writer.(*fakeWriter)
	if len(fw.buckets) != 2 {
		t.Fatalf("want one bucket per class, got %d: %+v", len(fw.buckets), fw.buckets)
	}
	byType := map[string]StructureBucket{}
	for _, b := range fw.buckets {
		byType[b.Type] = b
	}
	for _, want := range []struct{ typ, doc string }{{"fruit", "d1"}, {"company", "d2"}} {
		b, ok := byType[want.typ]
		if !ok {
			t.Fatalf("missing class %q in %+v", want.typ, fw.buckets)
		}
		if len(b.SourceDocIDs) != 1 || b.SourceDocIDs[0] != want.doc {
			t.Errorf("class %q sources = %v, want [%s]", want.typ, b.SourceDocIDs, want.doc)
		}
	}
}

// TestMergeStructureDataset_ResolvesTheUnclassifiedFallback covers the flip side:
// "other" is the model's way of saying it could not classify, so when a name has
// exactly one class in the batch, that instance belongs to it — and when the name
// has two, the unclassified row stays its own row instead of being guessed into
// one class.
func TestMergeStructureDataset_ResolvesTheUnclassifiedFallback(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	entity := func(docID, typ string) kccommon.Product {
		return kccommon.Product{
			Variant: kccommon.VariantStructure, DocID: docID, Content: "Engine desc " + typ,
			Meta: map[string]any{"name": "Engine", "entity_type": typ},
		}
	}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", []kccommon.Product{entity("d1", "component"), entity("d2", "other")}); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw := c.writer.(*fakeWriter)
	if len(fw.buckets) != 1 || fw.buckets[0].Type != "component" {
		t.Fatalf("unclassified instance should join the name's only class: %+v", fw.buckets)
	}
	if len(fw.buckets[0].SourceDocIDs) != 2 {
		t.Errorf("sources = %v, want both documents", fw.buckets[0].SourceDocIDs)
	}

	c = &Consumer{writer: &fakeWriter{}}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", []kccommon.Product{
		entity("d1", "component"), entity("d2", "pump"), entity("d3", "other"),
	}); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw = c.writer.(*fakeWriter)
	if len(fw.buckets) != 3 {
		t.Fatalf("ambiguous names must keep the unclassified row: got %d buckets: %+v", len(fw.buckets), fw.buckets)
	}
	for _, b := range fw.buckets {
		if len(b.SourceDocIDs) != 1 {
			t.Errorf("class %q absorbed another class's sources: %v", b.Type, b.SourceDocIDs)
		}
	}
}

// TestMergeStructureDataset_KeepsTheNewestModelVersion covers the version stamp
// on a folded dataset bucket: a publish does not recompile the corpus, so one
// bucket can fold rows extracted by different model versions. It reports the
// newest, which is the version its latest rows come from.
func TestMergeStructureDataset_KeepsTheNewestModelVersion(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	entity := func(docID string, version int) kccommon.Product {
		return kccommon.Product{
			Variant: kccommon.VariantStructure, DocID: docID, Content: "Engine desc",
			Meta: map[string]any{
				"name": "Engine", "entity_type": "component",
				"compilation_template_version": version,
			},
		}
	}
	products := []kccommon.Product{entity("d1", 2), entity("d2", 5), entity("d3", 3)}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw := c.writer.(*fakeWriter)
	if len(fw.buckets) != 1 {
		t.Fatalf("want 1 bucket, got %d", len(fw.buckets))
	}
	if fw.buckets[0].TemplateVersion != 5 {
		t.Errorf("TemplateVersion = %d, want the newest 5", fw.buckets[0].TemplateVersion)
	}
}

func TestMergeStructureDataset_IsolatesTemplates(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	products := []kccommon.Product{
		{Variant: kccommon.VariantStructure, DocID: "d1", TemplateID: "tpl1", Kind: "knowledge_graph", Content: "first", Meta: map[string]any{"name": "Engine", "entity_type": "component", "compile_kwd": "hypergraph"}},
		{Variant: kccommon.VariantStructure, DocID: "d2", TemplateID: "tpl2", Kind: "knowledge_graph", Content: "second", Meta: map[string]any{"name": "engine", "entity_type": "system", "compile_kwd": "hypergraph"}},
	}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	buckets := c.writer.(*fakeWriter).buckets
	if len(buckets) != 2 {
		t.Fatalf("same-name entities from different templates must remain isolated: %+v", buckets)
	}
	seen := map[string]bool{}
	for _, bucket := range buckets {
		seen[bucket.TemplateID] = true
	}
	if !seen["tpl1"] || !seen["tpl2"] {
		t.Fatalf("template identities were lost: %+v", buckets)
	}
}

// TestMergeStructureDataset_RelationsNotDropped covers review issue 3: structure
// relation products (kind=relation with from/to, no name) must be folded into a
// dataset-level relation bucket instead of being silently dropped.
func TestMergeStructureDataset_RelationsNotDropped(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	products := []kccommon.Product{
		{Variant: kccommon.VariantStructure, DocID: "d1", Content: "entity A",
			Meta: map[string]any{"kind": "entity", "name": "A"}},
		{Variant: kccommon.VariantStructure, DocID: "d1", Content: "entity B",
			Meta: map[string]any{"kind": "entity", "name": "B"}},
		{Variant: kccommon.VariantStructure, DocID: "d1", Content: "rel desc",
			Meta: map[string]any{"kind": "relation", "from": "A", "to": "B", "source_chunk_ids": []string{"c1"}}},
		{Variant: kccommon.VariantStructure, DocID: "d2", Content: "rel desc 2",
			Meta: map[string]any{"kind": "relation", "from": "A", "to": "B", "source_chunk_ids": []string{"c2"}}},
	}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw := c.writer.(*fakeWriter)
	if len(fw.buckets) != 3 {
		t.Fatalf("want 2 entity buckets and 1 relation bucket, got %d", len(fw.buckets))
	}
	var b *StructureBucket
	for i := range fw.buckets {
		if fw.buckets[i].Type == "relation" {
			b = &fw.buckets[i]
			break
		}
	}
	if b == nil {
		t.Fatalf("missing relation bucket: %+v", fw.buckets)
	}
	if b.Type != "relation" || b.FromEntity != "A" || b.ToEntity != "B" {
		t.Errorf("relation bucket = %+v, want Type=relation From=A To=B", b)
	}
	if len(b.SourceDocIDs) != 2 {
		t.Errorf("relation source doc union = %v, want [d1 d2]", b.SourceDocIDs)
	}
}

func TestMergeStructureDataset_NormalizesRelationIdentity(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	products := []kccommon.Product{
		{Variant: kccommon.VariantStructure, DocID: "d1", Content: "entity A",
			Meta: map[string]any{"kind": "entity", "name": "A"}},
		{Variant: kccommon.VariantStructure, DocID: "d1", Content: "entity B",
			Meta: map[string]any{"kind": "entity", "name": "B"}},
		{Variant: kccommon.VariantStructure, DocID: "d1", Content: "first",
			Meta: map[string]any{"kind": "relation", "from": " A  ", "to": " B ", "relation_type": " Related  ", "source_chunk_ids": []string{"c1"}}},
		{Variant: kccommon.VariantStructure, DocID: "d2", Content: "second",
			Meta: map[string]any{"kind": "relation", "from": "a", "to": "b", "relation_type": "related", "source_chunk_ids": []string{"c2"}}},
	}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	buckets := c.writer.(*fakeWriter).buckets
	if len(buckets) != 3 {
		t.Fatalf("normalized relation identity should produce two entity buckets and one relation bucket, got %d: %v", len(buckets), buckets)
	}
	var relationBucket *StructureBucket
	for i := range buckets {
		if buckets[i].Type == "relation" {
			relationBucket = &buckets[i]
			break
		}
	}
	if relationBucket == nil {
		t.Fatalf("missing relation bucket: %+v", buckets)
	}
	if got := len(relationBucket.SourceDocIDs); got != 2 {
		t.Fatalf("relation source docs = %v, want two docs", buckets[0].SourceDocIDs)
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

func TestMergeStructureDataset_DescriptionFallbacks(t *testing.T) {
	c := &Consumer{writer: &fakeWriter{}}
	products := []kccommon.Product{
		{Variant: kccommon.VariantMindmap, DocID: "d1",
			Content: `{"name":"Root","type":"mindmap"}`,
			Meta:    map[string]any{"kind": "entity", "name": "Root", "entity_type": "mindmap"}},
		{Variant: kccommon.VariantMindmap, DocID: "d1",
			Content: `{"name":"Child","type":"mindmap"}`,
			Meta:    map[string]any{"kind": "entity", "name": "Child", "entity_type": "mindmap"}},
		{Variant: kccommon.VariantMindmap, DocID: "d1",
			Content: `{"from":"Root","to":"Child","type":"related"}`,
			Meta:    map[string]any{"kind": "relation", "from": "Root", "to": "Child", "relation_type": "related"}},
	}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	fw := c.writer.(*fakeWriter)
	if len(fw.buckets) != 3 {
		t.Fatalf("want two entity and one relation bucket, got %d", len(fw.buckets))
	}
	for _, b := range fw.buckets {
		switch b.Type {
		case "mindmap":
			want := b.Name
			if b.Description != want {
				t.Errorf("entity %q fallback description = %q, want %q", b.Name, b.Description, want)
			}
		case "relation":
			if b.Description != "Root related Child" {
				t.Errorf("relation fallback description = %q, want %q", b.Description, "Root related Child")
			}
		default:
			t.Errorf("unexpected bucket: %+v", b)
		}
	}
}

// TestMergeStructureDataset_CompileKwdIsAutotypeNotTemplateKind covers the
// Dataset rows retain the producing compilation kind, independent of extraction shape.
func TestMergeStructureDataset_CompileKwdIsCanonicalKind(t *testing.T) {
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
			if b.CompileKwd != "graph" {
				t.Errorf("Engine compile_kwd = %q, want \"graph\"", b.CompileKwd)
			}
			if b.TemplateKind != "knowledge_graph" {
				t.Errorf("Engine TemplateKind = %q, want \"knowledge_graph\"", b.TemplateKind)
			}
			if b.TemplateID != "tpl-graph" {
				t.Errorf("Engine TemplateID = %q, want \"tpl-graph\"", b.TemplateID)
			}
		case "Fuel":
			if b.CompileKwd != "mind_map" {
				t.Errorf("Fuel compile_kwd = %q, want \"mind_map\"", b.CompileKwd)
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
	entity := func(name, typ, ckwd string) StructureBucket {
		return StructureBucket{Name: name, Type: typ, CompileKwd: ckwd}
	}
	relation := func(name, relType, fromType, toType string) StructureBucket {
		return StructureBucket{Name: name, Type: "relation", RelationType: relType, CompileKwd: "graph", FromType: fromType, ToType: toType}
	}
	a := datasetLevelStructureID("t1", "kb1", "tpl1", entity("Engine", "component", "timeline"))
	b := datasetLevelStructureID("t1", "kb1", "tpl1", entity("engine", "component", "timeline"))
	if a != b {
		t.Errorf("structure id should be case-insensitive on name: %q vs %q", a, b)
	}
	if a == datasetLevelStructureID("t1", "kb1", "tpl1", entity("Fuel", "component", "timeline")) {
		t.Errorf("different names must yield different ids")
	}
	if a == datasetLevelStructureID("t1", "kb1", "tpl1", entity("Engine", "component", "mindmap")) {
		t.Errorf("different compile kinds must yield different ids")
	}
	if a == datasetLevelStructureID("t1", "kb1", "tpl1", entity("Engine", "other", "timeline")) {
		t.Errorf("different entity types must yield different ids: one row states one class")
	}
	if a == datasetLevelStructureID("t1", "kb1", "tpl2", entity("Engine", "component", "timeline")) {
		t.Errorf("different templates must yield different ids")
	}
	if datasetLevelStructureID("t1", "kb1", "tpl1", relation("A -> B", "causes", "part", "part")) ==
		datasetLevelStructureID("t1", "kb1", "tpl1", relation("A -> B", "contradicts", "part", "part")) {
		t.Errorf("different relation types between the same endpoints must yield different ids")
	}
	if datasetLevelStructureID("t1", "kb1", "tpl1", relation("A -> B", "causes", "component", "part")) ==
		datasetLevelStructureID("t1", "kb1", "tpl1", relation("A -> B", "causes", "fault", "part")) {
		t.Errorf("different endpoint classes must yield different ids")
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
		"content_with_weight":           `{"entities":[]}`,
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
		"content_with_weight":           "overall summary",
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

// TestProductFromChunkMap_RestoresModelVersion covers the doc→dataset
// round-trip: without it the dataset row cannot say which published model
// version extracted it, and a stale assertion looks exactly like a current one.
func TestProductFromChunkMap_RestoresModelVersion(t *testing.T) {
	row := map[string]interface{}{
		"id": "entity-acme", "doc_id": "d1", "compile_kwd": "knowledge_graph",
		"content_with_weight":              `{"name":"Acme Corp","type":"organization"}`,
		"type_kwd":                         "entity",
		"name_kwd":                         "acme corp",
		"compilation_template_version_int": int64(4),
	}
	p, ok := productFromChunkMap(row, "t1", kccommon.VariantStructure)
	if !ok {
		t.Fatal("row should reconstruct")
	}
	if v, _ := metaInt(p.Meta, "compilation_template_version"); v != 4 {
		t.Errorf("Meta.compilation_template_version = %v, want 4", p.Meta["compilation_template_version"])
	}
	// An unversioned row (never published, or written before the column existed)
	// must not look like version 0 was published.
	bare, _ := productFromChunkMap(map[string]interface{}{
		"id": "entity-beta", "doc_id": "d1", "compile_kwd": "knowledge_graph",
		"content_with_weight": `{"name":"Beta Ltd","type":"organization"}`,
		"type_kwd":            "entity",
		"name_kwd":            "beta ltd",
	}, "t1", kccommon.VariantStructure)
	if _, present := bare.Meta["compilation_template_version"]; present {
		t.Errorf("unversioned row carries a version: %v", bare.Meta)
	}
}

func TestProductFromChunkMap_PreservesStructurePayloadFieldsThroughMerge(t *testing.T) {
	rows := []map[string]interface{}{
		{
			"id": "entity-acme", "doc_id": "d1", "compile_kwd": "knowledge_graph",
			"content_with_weight": `{"name":" Acme Corp ","type":"organization","description":"Supplier"}`,
			"knowledge_graph_kwd": "graph", "type_kwd": "entity", "name_kwd": "acme corp",
		},
		{
			"id": "entity-beta", "doc_id": "d1", "compile_kwd": "knowledge_graph",
			"content_with_weight": `{"text":"Beta Ltd","type":"organization","description":"Vendor"}`,
			"knowledge_graph_kwd": "graph", "type_kwd": "entity", "name_kwd": "beta ltd",
		},
		{
			"id": "relation-supplies", "doc_id": "d1", "compile_kwd": "knowledge_graph",
			"content_with_weight": `{"from":"Acme Corp","to":"Beta Ltd","type":"supplies","description":"supply"}`,
			"knowledge_graph_kwd": "graph", "type_kwd": "relation",
			"from_entity_kwd": " ACME CORP ", "to_entity_kwd": "BETA LTD ",
		},
		{
			"id": "relation-owns", "doc_id": "d1", "compile_kwd": "knowledge_graph",
			"content_with_weight": `{"from":"Acme Corp","to":"Beta Ltd","type":"owns","description":"ownership"}`,
			"knowledge_graph_kwd": "graph", "type_kwd": "relation",
			"from_entity_kwd": " ACME CORP ", "to_entity_kwd": "BETA LTD ",
		},
	}
	products := make([]kccommon.Product, 0, len(rows))
	for _, row := range rows {
		product, ok := productFromChunkMap(row, "t1", kccommon.VariantStructure)
		if !ok {
			t.Fatalf("row %q should reconstruct", row["id"])
		}
		products = append(products, product)
	}

	c := &Consumer{writer: &fakeWriter{}}
	if err := c.mergeStructureDataset(context.Background(), "t1", "kb1", products); err != nil {
		t.Fatalf("mergeStructureDataset: %v", err)
	}
	buckets := c.writer.(*fakeWriter).buckets
	if len(buckets) != 4 {
		t.Fatalf("want 2 entities and 2 typed relations, got %d: %+v", len(buckets), buckets)
	}
	entities := map[string]bool{}
	relations := map[string]bool{}
	for _, bucket := range buckets {
		if bucket.Type == "relation" {
			relations[bucket.RelationType] = true
			if bucket.FromEntity != "Acme Corp" || bucket.ToEntity != "Beta Ltd" {
				t.Errorf("relation endpoints = %q -> %q, want trimmed entity spellings", bucket.FromEntity, bucket.ToEntity)
			}
			continue
		}
		entities[bucket.Name] = true
	}
	if !entities["Acme Corp"] || !entities["Beta Ltd"] {
		t.Errorf("entity display names = %v, want extracted spelling", entities)
	}
	if !relations["supplies"] || !relations["owns"] {
		t.Errorf("relation types = %v, want supplies and owns", relations)
	}
}
