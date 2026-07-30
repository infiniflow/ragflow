package common

import (
	"testing"
)

func vec(a, b, c float32) []float32 { return []float32{a, b, c} }

func TestMemStoreTopK(t *testing.T) {
	m := NewMemStore()
	m.Add(Product{ID: "a", Vector: vec(1, 0, 0)})
	m.Add(Product{ID: "b", Vector: vec(0, 1, 0)})
	m.Add(Product{ID: "c", Vector: vec(0, 0, 1)})

	// query aligned with "a" → exact match
	hits := m.TopK(vec(1, 0, 0), 3, 0.0)
	if len(hits) != 3 {
		t.Fatalf("expected 3 hits, got %d", len(hits))
	}
	if hits[0].ID != "a" || hits[0].Score < 0.999 {
		t.Fatalf("expected top hit a with score ~1, got %s %.3f", hits[0].ID, hits[0].Score)
	}

	// threshold filters out orthogonal vectors
	hits = m.TopK(vec(1, 0, 0), 3, 0.5)
	if len(hits) != 1 || hits[0].ID != "a" {
		t.Fatalf("threshold=0.5 should keep only a, got %v", hits)
	}

	// k caps results
	hits = m.TopK(vec(0.6, 0.6, 0.6), 2, 0.0)
	if len(hits) != 2 {
		t.Fatalf("k=2 should cap to 2, got %d", len(hits))
	}
}

func TestMemStoreUpsertDelete(t *testing.T) {
	m := NewMemStore()
	m.Add(Product{ID: "x", Vector: vec(1, 0, 0)})
	m.Add(Product{ID: "y", Vector: vec(0, 1, 0)})
	m.Upsert(Product{ID: "x", Vector: vec(0, 0, 1)})
	if got := m.TopK(vec(0, 0, 1), 1, 0.99); len(got) != 1 || got[0].ID != "x" {
		t.Fatalf("upsert did not replace x vector: %v", got)
	}
	m.Delete("x")
	if m.Len() != 1 {
		t.Fatalf("delete failed, len=%d", m.Len())
	}
	if got := m.Snapshot(); len(got) != 1 || got[0].ID != "y" {
		t.Fatalf("snapshot wrong after delete: %v", got)
	}
}

func TestStableRowID(t *testing.T) {
	a := StableRowID("t1", "d1", "structure", "hello")
	b := StableRowID("t1", "d1", "structure", "hello")
	c := StableRowID("t1", "d1", "structure", "world")
	if a != b {
		t.Fatalf("same parts must yield same id: %s vs %s", a, b)
	}
	if a == c {
		t.Fatalf("different content must yield different id")
	}
	// part order must not collide
	if StableRowID("ab", "c") == StableRowID("a", "bc") {
		t.Fatalf("part-ordering collided")
	}
}

func TestLocalize(t *testing.T) {
	if got := Localize("plain", "zh"); got != "plain" {
		t.Fatalf("string passthrough: %q", got)
	}
	// Lists render as numbered lines (mirrors Python's _struct_localize).
	if got := Localize([]string{"en-only"}, "zh"); got != "1. en-only" {
		t.Fatalf("slice numbered join: %q", got)
	}
	if got := Localize([]any{"a", "b"}, "en"); got != "1. a\n2. b" {
		t.Fatalf("any-slice numbered join: %q", got)
	}
	m := map[string]string{"en": "E", "zh": "中"}
	if got := Localize(m, "zh"); got != "中" {
		t.Fatalf("map lang pick: %q", got)
	}
	if got := Localize(m, "fr"); got != "E" {
		t.Fatalf("map fallback en: %q", got)
	}
	// lang == "en" with no "en" key yields "" (no arbitrary-language fallback,
	// matching Python).
	if got := Localize(map[string]string{"zh": "中"}, "en"); got != "" {
		t.Fatalf("no arbitrary fallback: %q", got)
	}
	// Nested map values localize recursively (list under lang key).
	if got := Localize(map[string]any{"en": []any{"x", "y"}}, "zh"); got != "1. x\n2. y" {
		t.Fatalf("nested en fallback: %q", got)
	}
}

func TestPackBatches(t *testing.T) {
	chunks := []Chunk{
		{ID: "1", Text: "aaaa"},
		{ID: "2", Text: "bbbb"},
		{ID: "3", Text: "cccccccccccc"}, // oversized
		{ID: "4", Text: "dddd"},
	}
	// EstimateTokens ≈ len/4 → 1,1,3,1. budget 2 → [1,2],[3],[4]
	batches := PackBatches(chunks, 2, nil)
	if len(batches) != 3 {
		t.Fatalf("expected 3 batches, got %d: %v", len(batches), batches)
	}
	if len(batches[1]) != 1 || batches[1][0].ID != "3" {
		t.Fatalf("oversized chunk must be alone: %v", batches[1])
	}

	// zero budget → single batch
	if got := PackBatches(chunks, 0, nil); len(got) != 1 || len(got[0]) != 4 {
		t.Fatalf("zero budget should be one batch of all: %v", got)
	}
}
