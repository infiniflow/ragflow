package knowledge_compile

import (
	"context"
	"testing"
	"time"

	kccommon "ragflow/internal/ingestion/component/knowledge_compiler/common"
)

// TestMergedChunkMapKeepsWikiFields locks the fix for the merged-row metadata
// gap: the dataset-level merged row written by mergedChunkMap must carry the
// wiki page fields (page_type_kwd/topic_kwd/title_kwd/slug_kwd/...) that the
// artifact API (ListArtifacts/ListWikiTopics) and page renderers read. Without
// them the compilation page surfaces no wiki pages from the merged products.
func TestMergedChunkMapKeepsWikiFields(t *testing.T) {
	p := kccommon.Product{
		ID: "merged-1", DocID: "kb1", TenantID: "t1", Variant: kccommon.VariantWiki,
		Content: "# Alpha\n\nBody",
		Vector:  []float32{0.1, 0.2, 0.3},
		Meta: map[string]any{
			"slug":             "entity/alpha",
			"title":            "Alpha",
			"page_type":        "entity",
			"topic":            " Knowledge / Core / Alpha ",
			"summary":          "A page about Alpha",
			"entity_names":     []string{"Alpha"},
			"related_kb_pages": []string{"entity/beta"},
			"source_doc_ids":   []string{"d1"},
			"source_chunk_ids": []string{"c1"},
		},
	}
	now := time.Now()
	m := mergedChunkMap("t1", "kb1", "run-abc", "hash-123", now, p)

	cases := map[string]string{
		"slug_kwd":            "entity/alpha",
		"artifact_slug_kwd":   "entity/alpha",
		"title_kwd":           "Alpha",
		"page_type_kwd":       "entity",
		"topic_kwd":           "Knowledge/Core/Alpha",
		"summary_with_weight": "A page about Alpha",
		"plan_kwd":            "run-abc",
		"input_hash_kwd":      "hash-123",
	}
	// The wall-clock audit fields must be stamped from `now`, not omitted.
	if m["create_time"] != now.Format("2006-01-02 15:04:05") {
		t.Errorf("create_time = %v, want %v", m["create_time"], now.Format("2006-01-02 15:04:05"))
	}
	if m["create_timestamp_flt"] != float64(now.Unix()) {
		t.Errorf("create_timestamp_flt = %v, want %v", m["create_timestamp_flt"], float64(now.Unix()))
	}
	for k, want := range cases {
		if got, _ := m[k].(string); got != want {
			t.Errorf("merged row[%q] = %q, want %q", k, got, want)
		}
	}
	if v, _ := m["entity_names_kwd"].([]string); len(v) != 1 || v[0] != "Alpha" {
		t.Errorf("entity_names_kwd = %#v, want [Alpha]", m["entity_names_kwd"])
	}
	if m["doc_id"] != "kb1" || m["available_int"] != 1 {
		t.Errorf("merged flags wrong: doc_id=%v available_int=%v", m["doc_id"], m["available_int"])
	}
	if _, ok := m["kc_merged"]; ok {
		t.Errorf("merged row must not persist undefined kc_merged field")
	}
	if m["q_3_vec"] == nil {
		t.Errorf("vector column missing")
	}
}

// TestProductFromChunkMapRestoresWikiFields locks the reader side: the wiki page
// columns must be reconstructed into the product Meta so the merge step can carry
// them onto the merged row.
func TestProductFromChunkMapRestoresWikiFields(t *testing.T) {
	c := map[string]interface{}{
		"id":                   "wiki/1",
		"doc_id":               "d1",
		"compile_kwd":          "wiki_page",
		"content_with_weight":  "# Alpha",
		"kc_payload":           "# Alpha\n\nBody",
		"slug_kwd":             "entity/alpha",
		"page_type_kwd":        "entity",
		"topic_kwd":            "Alpha",
		"title_kwd":            "Alpha",
		"summary_with_weight":  "A page about Alpha",
		"entity_names_kwd":     []interface{}{"Alpha"},
		"related_kb_pages_kwd": []interface{}{"entity/beta"},
		"section_level_int":    float64(2),
	}
	p, ok := productFromChunkMap(c, "t1", kccommon.VariantWiki)
	if !ok {
		t.Fatalf("productFromChunkMap returned not-ok")
	}
	want := map[string]string{
		"slug": "entity/alpha", "page_type": "entity", "topic": "Alpha",
		"title": "Alpha", "summary": "A page about Alpha",
	}
	for k, v := range want {
		if got, _ := p.Meta[k].(string); got != v {
			t.Errorf("meta[%q] = %q, want %q", k, got, v)
		}
	}
	if v, _ := p.Meta["section_level"].(int64); v != 2 {
		t.Errorf("section_level = %v, want 2", p.Meta["section_level"])
	}
	if v, _ := p.Meta["entity_names"].([]string); len(v) != 1 || v[0] != "Alpha" {
		t.Errorf("entity_names = %#v, want [Alpha]", p.Meta["entity_names"])
	}
	if p.Merged {
		t.Errorf("per-doc row must not be marked merged")
	}
}

// TestDeleteMergedScopesToMergedWikiRows locks the W1 contract: DeleteMerged
// must only target dataset-level (available_int=1) wiki merged rows. The
// structural filter (kb_id + available_int + wiki page/section compile_kwd) is
// the source of truth, so a wrong tenant/kb can never cascade to per-document
// rows or to rows of other variants.
func TestDeleteMergedScopesToMergedWikiRows(t *testing.T) {
	eng := &fakeEngine{}
	w := engineWriter{eng: eng}
	if err := w.DeleteMerged(context.Background(), "t1", "kb1"); err != nil {
		t.Fatalf("DeleteMerged: %v", err)
	}
	cond := eng.lastDeleteCond
	if cond == nil {
		t.Fatal("engine.DeleteChunks was not called")
	}
	if cond["kb_id"] != "kb1" {
		t.Errorf("DeleteMerged kb_id = %v, want kb1", cond["kb_id"])
	}
	if cond["available_int"] != 1 {
		t.Errorf("DeleteMerged must scope to available_int=1, got %v", cond["available_int"])
	}
	variants, ok := cond["compile_kwd"].([]string)
	if !ok {
		t.Fatalf("DeleteMerged must pass a compile_kwd string slice, got %T", cond["compile_kwd"])
	}
	if len(variants) != 2 || variants[0] != compileKwdWikiPage || variants[1] != compileKwdWikiSection {
		t.Errorf("DeleteMerged compile_kwd = %v, want [%q %q]", variants, compileKwdWikiPage, compileKwdWikiSection)
	}
}

// TestDeleteMergedForVariant_EmptySetClearsAll covers B4/B1b: an empty variants
// slice (full rebuild) clears the full managed set — nav + wiki + structure
// compile_kwds — plus the structure scope_kwd="dataset" sweep.
func TestDeleteMergedForVariant_EmptySetClearsAll(t *testing.T) {
	eng := &fakeEngine{}
	w := engineWriter{eng: eng}
	if err := w.DeleteMergedForVariant(context.Background(), "t1", "kb1", nil); err != nil {
		t.Fatalf("DeleteMergedForVariant: %v", err)
	}
	// The full clean issues two deletes: the compile_kwd bucket first, then the
	// scope_kwd="dataset" sweep. Assert on the first (compile_kwd) delete.
	if len(eng.deleteConds) < 1 {
		t.Fatal("engine.DeleteChunks was not called")
	}
	kwds, ok := eng.deleteConds[0]["compile_kwd"].([]string)
	if !ok {
		t.Fatalf("compile_kwd filter not a string slice, got %T", eng.deleteConds[0]["compile_kwd"])
	}
	want := []string{compileKwdNav, compileKwdWikiPage, compileKwdWikiSection, compileKwdStructure}
	if len(kwds) != len(want) {
		t.Fatalf("full clean compile_kwd = %v, want %v", kwds, want)
	}
	got := map[string]bool{}
	for _, k := range kwds {
		got[k] = true
	}
	for _, k := range want {
		if !got[k] {
			t.Errorf("full clean missing compile_kwd %q", k)
		}
	}
	// The second delete sweeps structure scope_kwd="dataset".
	foundScope := false
	for _, cond := range eng.deleteConds {
		if cond["scope_kwd"] == "dataset" {
			foundScope = true
		}
	}
	if !foundScope {
		t.Error("full clean must also sweep scope_kwd=dataset")
	}
}

// TestDeleteMergedForVariant_WikiScopesToAvailableOne covers the per-variant
// wiki clean: it must issue a delete scoped to available_int=1 and the wiki
// page/section kwds (wiki merged rows carry available_int=1).
func TestDeleteMergedForVariant_WikiScopesToAvailableOne(t *testing.T) {
	eng := &fakeEngine{}
	w := engineWriter{eng: eng}
	if err := w.DeleteMergedForVariant(context.Background(), "t1", "kb1", []kccommon.Variant{kccommon.VariantWiki}); err != nil {
		t.Fatalf("DeleteMergedForVariant: %v", err)
	}
	if len(eng.deleteConds) != 1 {
		t.Fatalf("wiki clean should issue exactly one delete, got %d", len(eng.deleteConds))
	}
	cond := eng.deleteConds[0]
	if cond["available_int"] != 1 {
		t.Errorf("wiki clean must scope to available_int=1, got %v", cond["available_int"])
	}
	kwds, ok := cond["compile_kwd"].([]string)
	if !ok || len(kwds) != 2 {
		t.Fatalf("wiki clean compile_kwd = %v, want [wiki_page wiki_section]", cond["compile_kwd"])
	}
}

// TestDeleteMergedForVariant_StructureScopesToDataset covers the per-variant
// structure clean: dataset rows are tagged scope_kwd="dataset" (compile_kwd may
// be the inferred sub-kind), so the delete is scoped to scope_kwd="dataset".
func TestDeleteMergedForVariant_StructureScopesToDataset(t *testing.T) {
	eng := &fakeEngine{}
	w := engineWriter{eng: eng}
	if err := w.DeleteMergedForVariant(context.Background(), "t1", "kb1", []kccommon.Variant{kccommon.VariantStructure}); err != nil {
		t.Fatalf("DeleteMergedForVariant: %v", err)
	}
	if len(eng.deleteConds) != 1 {
		t.Fatalf("structure clean should issue exactly one delete, got %d", len(eng.deleteConds))
	}
	if eng.deleteConds[0]["scope_kwd"] != "dataset" {
		t.Errorf("structure clean must scope to scope_kwd=dataset, got %v", eng.deleteConds[0]["scope_kwd"])
	}
}

// TestDeleteMergedForVariant_WikiPlusNavCoversNavRows covers the combined-clean
// bug: each variant issues its OWN delete (one AND-ed filter would exclude the
// other's rows — nav rows carry available_int=0, structure rows scope_kwd=
// "dataset"). A wiki+tree clean must produce one wiki delete (available_int=1)
// and one nav delete (compile_kwd=dataset_nav, no available_int).
func TestDeleteMergedForVariant_WikiPlusNavCoversNavRows(t *testing.T) {
	eng := &fakeEngine{}
	w := engineWriter{eng: eng}
	if err := w.DeleteMergedForVariant(context.Background(), "t1", "kb1",
		[]kccommon.Variant{kccommon.VariantWiki, kccommon.VariantTree}); err != nil {
		t.Fatalf("DeleteMergedForVariant: %v", err)
	}
	if len(eng.deleteConds) != 2 {
		t.Fatalf("wiki+tree clean should issue 2 deletes (wiki + nav), got %d", len(eng.deleteConds))
	}
	var wikiCond, navCond map[string]interface{}
	for _, cond := range eng.deleteConds {
		kwds, _ := cond["compile_kwd"].([]string)
		for _, k := range kwds {
			if k == compileKwdNav {
				navCond = cond
			}
		}
		if cond["available_int"] == 1 {
			wikiCond = cond
		}
	}
	if wikiCond == nil {
		t.Error("expected a wiki delete scoped to available_int=1")
	}
	if navCond == nil {
		t.Error("expected a nav delete (compile_kwd=dataset_nav)")
	} else if _, hasAvail := navCond["available_int"]; hasAvail {
		t.Errorf("nav delete must not carry available_int (nav rows are 0), got %v", navCond["available_int"])
	}
}

// TestDeleteStructureForDocs_RemovesGhostOnly covers G3: structure dataset rows
// whose source docs are ALL deleted are ghosts and removed; rows that still have
// a surviving source doc are kept.
func TestDeleteStructureForDocs_RemovesGhostOnly(t *testing.T) {
	eng := &fakeEngine{searchChunks: []map[string]interface{}{
		{"id": "row_ghost", "source_doc_ids": []any{"d1", "d2"}}, // all deleted -> ghost
		{"id": "row_keep", "source_doc_ids": []any{"d1", "d3"}},  // d3 survives -> keep
	}}
	w := engineWriter{eng: eng}
	if err := w.DeleteStructureForDocs(context.Background(), "t1", "kb1", []string{"d1", "d2"}); err != nil {
		t.Fatalf("DeleteStructureForDocs: %v", err)
	}
	if eng.lastDeleteCond == nil {
		t.Fatal("DeleteChunks not called")
	}
	ids, ok := eng.lastDeleteCond["id"].([]string)
	if !ok || len(ids) != 1 || ids[0] != "row_ghost" {
		t.Fatalf("ghost ids = %v, want [row_ghost]", eng.lastDeleteCond["id"])
	}
}

// TestDeleteStructureForDocs_NoGhostsNoDelete covers G3: when every structure row
// still has a surviving source doc, no delete is issued.
func TestDeleteStructureForDocs_NoGhostsNoDelete(t *testing.T) {
	eng := &fakeEngine{searchChunks: []map[string]interface{}{
		{"id": "row_keep", "source_doc_ids": []any{"d1", "d3"}},
	}}
	before := len(eng.deleteConds)
	w := engineWriter{eng: eng}
	if err := w.DeleteStructureForDocs(context.Background(), "t1", "kb1", []string{"d1"}); err != nil {
		t.Fatalf("DeleteStructureForDocs: %v", err)
	}
	if len(eng.deleteConds) != before {
		t.Fatalf("no ghost should mean no delete, got %d deletes", len(eng.deleteConds)-before)
	}
}

// TestProductFromChunkMapRejectsDirtyKwd locks the dirty-row contract from the
// wiki_incremental plan (Claim 4): productFromChunkMap must reverse-map the raw
// compile_kwd via KwdToVariant and reject any row whose kwd does not map to the
// expected variant, including unknown / malformed kinds (e.g. "artifact_page",
// "garbage", empty). It must not silently fall back to a raw-string comparison.
func TestProductFromChunkMapRejectsDirtyKwd(t *testing.T) {
	base := map[string]interface{}{
		"id":                  "wiki/1",
		"doc_id":              "d1",
		"content_with_weight": "# Alpha",
	}
	dirtyKwds := []string{"artifact_page", "garbage", ""}
	for _, kwd := range dirtyKwds {
		c := map[string]interface{}{}
		for k, v := range base {
			c[k] = v
		}
		c["compile_kwd"] = kwd
		if p, ok := productFromChunkMap(c, "t1", kccommon.VariantWiki); ok {
			t.Errorf("dirty compile_kwd %q should be rejected, got product %+v", kwd, p)
		}
	}

	// Note: wiki_section maps to VariantWiki (page and section share the wiki
	// variant); the page/section distinction lives in Meta.kind and is enforced
	// by filterWikiPageCandidates, NOT by the variant dirty-row check. So a
	// wiki_section row legitimately satisfies a VariantWiki query here — the
	// section is dropped later by the kind filter.

	// A clean wiki_page row must pass for VariantWiki.
	good := map[string]interface{}{}
	for k, v := range base {
		good[k] = v
	}
	good["compile_kwd"] = compileKwdWikiPage
	if _, ok := productFromChunkMap(good, "t1", kccommon.VariantWiki); !ok {
		t.Errorf("clean wiki_page row should satisfy VariantWiki query")
	}
}
