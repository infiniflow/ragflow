package knowledge_compile

import (
	"testing"

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
			"topic":            "Alpha",
			"summary":          "A page about Alpha",
			"entity_names":     []string{"Alpha"},
			"related_kb_pages": []string{"entity/beta"},
			"source_doc_ids":   []string{"d1"},
			"source_chunk_ids": []string{"c1"},
		},
	}
	m := mergedChunkMap("t1", "kb1", p)

	cases := map[string]string{
		"slug_kwd":            "entity/alpha",
		"artifact_slug_kwd":   "entity/alpha",
		"title_kwd":           "Alpha",
		"page_type_kwd":       "entity",
		"topic_kwd":           "Alpha",
		"summary_with_weight": "A page about Alpha",
	}
	for k, want := range cases {
		if got, _ := m[k].(string); got != want {
			t.Errorf("merged row[%q] = %q, want %q", k, got, want)
		}
	}
	if v, _ := m["entity_names_kwd"].([]string); len(v) != 1 || v[0] != "Alpha" {
		t.Errorf("entity_names_kwd = %#v, want [Alpha]", m["entity_names_kwd"])
	}
	if m["doc_id"] != "kb1" || m["kc_merged"] != 1 || m["available_int"] != 1 {
		t.Errorf("merged flags wrong: doc_id=%v kc_merged=%v available_int=%v", m["doc_id"], m["kc_merged"], m["available_int"])
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
	p, ok := productFromChunkMap(c, "t1")
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
