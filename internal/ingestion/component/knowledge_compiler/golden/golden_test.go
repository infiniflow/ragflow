package golden

import (
	"encoding/json"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

func TestFixedCorpus_IsStable(t *testing.T) {
	first := FixedCorpus()
	if len(first) != 12 {
		t.Fatalf("FixedCorpus len = %d, want 12 (locked in fixtures)", len(first))
	}
	second := FixedCorpus()
	if len(first) != len(second) {
		t.Fatalf("FixedCorpus not stable: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].ID != second[i].ID || first[i].Text != second[i].Text {
			t.Errorf("FixedCorpus[%d] differs across calls", i)
			break
		}
	}
}

func TestChunksToAny(t *testing.T) {
	chunks := FixedCorpus()
	raw := ChunksToAny(chunks)
	if len(raw) != len(chunks) {
		t.Fatalf("ChunksToAny len = %d, want %d", len(raw), len(chunks))
	}
	for i, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("ChunksToAny[%d] = %T, want map[string]any", i, item)
		}
		if m["id"] != chunks[i].ID {
			t.Errorf("ChunksToAny[%d].id = %v, want %q", i, m["id"], chunks[i].ID)
		}
		if m["text"] != chunks[i].Text {
			t.Errorf("ChunksToAny[%d].text mismatch", i)
		}
	}
}

func TestAnalyzeTreeProducts_TreeShape(t *testing.T) {
	vector := json.RawMessage(`[0.1,0.2,0.3]`)
	chunks := []schema.ChunkDoc{
		{Text: "root summary", Extra: mustExtras(t, map[string]any{
			"id": "r1", "doc_id": "d1", "tenant_id": "t1", "compile_kwd": "tree",
			"kc_kind": "root", "kc_level": float64(-1), "q_3_vec": vector,
		})},
		{Text: "leaf A", Extra: mustExtras(t, map[string]any{
			"id": "a1", "doc_id": "d1", "tenant_id": "t1", "compile_kwd": "tree",
			"kc_kind": "summary", "kc_level": float64(0), "parent_kwd": "r1", "q_3_vec": vector,
		})},
		{Text: "leaf B", Extra: mustExtras(t, map[string]any{
			"id": "b1", "doc_id": "d1", "tenant_id": "t1", "compile_kwd": "tree",
			"kc_kind": "summary", "kc_level": float64(0), "parent_kwd": "r1", "q_3_vec": vector,
		})},
		{Text: "mid A", Extra: mustExtras(t, map[string]any{
			"id": "m1", "doc_id": "d1", "tenant_id": "t1", "compile_kwd": "tree",
			"kc_kind": "summary", "kc_level": float64(1), "parent_kwd": "r1", "q_3_vec": vector,
		})},
	}
	m := AnalyzeTreeProducts(chunks)
	if m.RootCount != 1 {
		t.Errorf("RootCount = %d, want 1", m.RootCount)
	}
	if m.LeafClusters != 2 {
		t.Errorf("LeafClusters = %d, want 2 (a1+b1)", m.LeafClusters)
	}
	if m.MaxDepth != 2 {
		t.Errorf("MaxDepth = %d, want 2 (max level 1 + 1)", m.MaxDepth)
	}
	if !m.AllParented {
		t.Error("AllParented = false, want true (every node's parent is in the set)")
	}
	if !m.VectorOK || !m.SchemaOK {
		t.Errorf("VectorOK=%v SchemaOK=%v, want both true", m.VectorOK, m.SchemaOK)
	}
}

func TestAnalyzeTreeProducts_DetectsDanglingParent(t *testing.T) {
	chunks := []schema.ChunkDoc{
		{Text: "root", Extra: mustExtras(t, map[string]any{
			"id": "r1", "doc_id": "d1", "tenant_id": "t1", "compile_kwd": "tree",
			"kc_kind": "root",
		})},
		{Text: "orphan", Extra: mustExtras(t, map[string]any{
			"id": "o1", "doc_id": "d1", "tenant_id": "t1", "compile_kwd": "tree",
			"kc_kind": "summary", "kc_level": float64(0), "parent_kwd": "missing-parent",
		})},
	}
	m := AnalyzeTreeProducts(chunks)
	if m.AllParented {
		t.Error("AllParented = true, want false (o1's parent is missing)")
	}
}

func TestCoverageFraction_AllSourcesCovered(t *testing.T) {
	// Coverage is now measured from source_chunk_ids of level-0 leaf clusters,
	// not structural well-formedness. When every input chunk is referenced, the
	// fraction is 1.0 regardless of root/parent structure.
	m := TreeMetrics{RootCount: 1, LeafClusters: 3, AllParented: true, CoveredSources: 12}
	if cov := m.CoverageFraction(12); cov != 1.0 {
		t.Errorf("CoverageFraction = %v, want 1.0", cov)
	}
}

func TestCoverageFraction_NoCoveredSourcesIsZero(t *testing.T) {
	// A structurally well-formed tree that references no source chunks covers 0.
	m := TreeMetrics{RootCount: 1, LeafClusters: 3, AllParented: true, CoveredSources: 0}
	if cov := m.CoverageFraction(12); cov != 0.0 {
		t.Errorf("CoverageFraction = %v, want 0.0 (no covered sources)", cov)
	}
}

func TestCoverageFraction_PartialCoverage(t *testing.T) {
	// A tree that drops some source chunks scores below 1.0.
	m := TreeMetrics{RootCount: 1, LeafClusters: 3, AllParented: true, CoveredSources: 9}
	if cov := m.CoverageFraction(12); cov != 0.75 {
		t.Errorf("CoverageFraction = %v, want 0.75", cov)
	}
}

// TestAnalyzeTreeProducts_IgnoresUnknownSourceIDs verifies that an unknown
// (leaked/garbage) ID inside source_chunk_ids does not inflate CoveredSources
// past the corpus size, so CoverageFraction stays <= 1.0. A level-0 leaf that
// references 2 valid + 1 unknown ID over an nChunks=2 corpus must still report
// coverage 1.0, not 1.5.
func TestAnalyzeTreeProducts_IgnoresUnknownSourceIDs(t *testing.T) {
	vector := json.RawMessage(`[0.1,0.2,0.3]`)
	chunks := []schema.ChunkDoc{
		{Text: "root", Extra: mustExtras(t, map[string]any{
			"id": "r1", "doc_id": "d1", "tenant_id": "t1", "compile_kwd": "tree",
			"kc_kind": "root", "kc_level": float64(-1), "q_3_vec": vector,
		})},
		{Text: "leaf", Extra: mustExtras(t, map[string]any{
			"id": "a1", "doc_id": "d1", "tenant_id": "t1", "compile_kwd": "tree",
			"kc_kind": "summary", "kc_level": float64(0), "parent_kwd": "r1", "q_3_vec": vector,
			"source_chunk_ids": []string{"chunk-01", "chunk-02", "leaked-unknown-id"},
		})},
	}
	m := AnalyzeTreeProducts(chunks, "chunk-01", "chunk-02")
	if m.CoveredSources != 2 {
		t.Errorf("CoveredSources = %d, want 2 (unknown id excluded)", m.CoveredSources)
	}
	if cov := m.CoverageFraction(2); cov != 1.0 {
		t.Errorf("CoverageFraction = %v, want 1.0 (must not exceed 1.0)", cov)
	}
}

func TestExtraFloat_Roundtrip(t *testing.T) {
	doc := schema.ChunkDoc{Extra: mustExtras(t, map[string]any{"kc_level": float64(3)})}
	if v, ok := extraFloat(doc, "kc_level"); !ok || v != 3 {
		t.Errorf("extraFloat = %v/%v, want 3/true", v, ok)
	}
	if _, ok := extraFloat(doc, "missing"); ok {
		t.Error("extraFloat(missing) = ok, want false")
	}
}

// mustExtras builds a schema.Extra json.RawMessage map from a plain map. The
// test fixtures only need a stable shape; this keeps the golden_test
// independent from the production ChunkDoc constructor details.
func mustExtras(t *testing.T, in map[string]any) map[string]json.RawMessage {
	t.Helper()
	out := make(map[string]json.RawMessage, len(in))
	for k, v := range in {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal %s: %v", k, err)
		}
		out[k] = b
	}
	return out
}
