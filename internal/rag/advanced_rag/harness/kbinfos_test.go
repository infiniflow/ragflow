package harness

import "testing"

// TestMergeSkipsAllWhenNoChunks mirrors Python _merge_kbinfos's early return:
// `if not result or not result.get("chunks"): return`. When the incoming chunk
// list is empty, neither chunks nor doc_aggs are merged, and the call returns
// nil (no global indices contributed).
func TestMergeSkipsAllWhenNoChunks(t *testing.T) {
	kb := &Kbinfos{DocAggs: []map[string]any{{"doc_id": "d1"}}}
	added := kb.Merge(nil, []map[string]any{{"doc_id": "d2"}})
	if added != nil {
		t.Fatalf("Merge with no chunks should return nil, got %v", added)
	}
	if len(kb.DocAggs) != 1 {
		t.Fatalf("doc_aggs must not be merged without chunks, got %d", len(kb.DocAggs))
	}
	if len(kb.Chunks) != 0 {
		t.Fatalf("chunks should stay empty, got %d", len(kb.Chunks))
	}
}

// TestMergeCollapsesEmptyDocIDAggs mirrors Python's doc_agg dedup: the dseen set
// is built from the raw doc_id value, so a missing/None doc_id still enters the
// set and a second empty-doc_id agg is dropped. Only the first is kept.
func TestMergeCollapsesEmptyDocIDAggs(t *testing.T) {
	kb := &Kbinfos{}
	added := kb.Merge(
		[]map[string]any{{"id": "c1"}},
		[]map[string]any{{"name": "a"}, {"name": "b"}, {"name": "c"}},
	)
	if len(added) != 1 {
		t.Fatalf("added = %v, want 1 chunk index", added)
	}
	if len(kb.DocAggs) != 1 {
		t.Fatalf("empty-doc_id aggs should collapse to 1, got %d", len(kb.DocAggs))
	}
	if kb.DocAggs[0]["name"] != "a" {
		t.Fatalf("first empty-doc_id agg should be kept, got %v", kb.DocAggs[0])
	}
}

// TestMergeDedupsSameDocID mirrors Python's exact-match dedup for real doc_ids,
// independent of the empty bucket above.
func TestMergeDedupsSameDocID(t *testing.T) {
	kb := &Kbinfos{}
	kb.Merge([]map[string]any{{"id": "c1"}}, []map[string]any{{"doc_id": "x"}, {"doc_id": "x"}})
	if len(kb.DocAggs) != 1 {
		t.Fatalf("same-doc_id aggs should dedup, got %d", len(kb.DocAggs))
	}
}

// TestChunkKeyUsesTextFallback pins that the dedup key reads the SAME alias chain
// as chunkText (content_with_weight -> content -> text). Reading only the first
// two made two id-less, text-only chunks from one document share the doc-level
// fallback key, so Merge/MemoryAdd discarded distinct evidence.
func TestChunkKeyUsesTextFallback(t *testing.T) {
	a := map[string]any{"text": "alpha", "doc_id": "d1", "docnm_kwd": "doc"}
	b := map[string]any{"text": "beta", "doc_id": "d1", "docnm_kwd": "doc"}
	if chunkKey(a) == chunkKey(b) {
		t.Fatalf("distinct text-only chunks must not share a key: %q", chunkKey(a))
	}
	// The same text under any alias must produce the same key.
	if c := map[string]any{"content": "alpha"}; chunkKey(a) != chunkKey(c) {
		t.Errorf("text and content aliases must agree: %q vs %q", chunkKey(a), chunkKey(c))
	}
	// A chunk with no text at all still lands on the doc-level fallback.
	if got := chunkKey(map[string]any{"doc_id": "d1", "docnm_kwd": "doc"}); got == "" {
		t.Error("doc-level fallback key must not be empty")
	}
}

// TestMergeKeepsDistinctTextOnlyChunks pins the end-to-end consequence for the
// evidence pool: two id-less chunks that share only a document must both survive.
func TestMergeKeepsDistinctTextOnlyChunks(t *testing.T) {
	kb := &Kbinfos{}
	added := kb.Merge([]map[string]any{
		{"text": "alpha", "doc_id": "d1", "docnm_kwd": "doc"},
		{"text": "beta", "doc_id": "d1", "docnm_kwd": "doc"},
	}, nil)
	if len(kb.Chunks) != 2 {
		t.Fatalf("chunks = %d, want 2 (distinct text-only evidence must not collapse)", len(kb.Chunks))
	}
	if len(added) != 2 {
		t.Fatalf("added = %v, want 2 global indices", added)
	}
	// Re-merging identical content still dedups.
	kb.Merge([]map[string]any{{"text": "alpha", "doc_id": "d1", "docnm_kwd": "doc"}}, nil)
	if len(kb.Chunks) != 2 {
		t.Fatalf("identical text must still dedup, chunks = %d", len(kb.Chunks))
	}
}

// TestMemoryAddKeepsDistinctTextOnlyChunks pins the same consequence for the
// lossless memory store.
func TestMemoryAddKeepsDistinctTextOnlyChunks(t *testing.T) {
	kb := &Kbinfos{}
	MemoryAdd(kb, []map[string]any{
		{"text": "alpha", "doc_id": "d1", "docnm_kwd": "doc"},
		{"text": "beta", "doc_id": "d1", "docnm_kwd": "doc"},
	})
	if len(kb.Memory) != 2 {
		t.Fatalf("memory = %d, want 2 (distinct text-only evidence must not collapse)", len(kb.Memory))
	}
	MemoryAdd(kb, []map[string]any{{"text": "alpha", "doc_id": "d1", "docnm_kwd": "doc"}})
	if len(kb.Memory) != 2 {
		t.Fatalf("identical text must still dedup, memory = %d", len(kb.Memory))
	}
}
