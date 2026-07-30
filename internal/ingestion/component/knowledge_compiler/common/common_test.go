package common

import (
	"context"
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

func TestGuardrails(t *testing.T) {
	g := CapacityGuardrails{MaxItems: 2, OnExceed: ExceedError}
	o := Outputs{Products: make([]Product, 3)}
	if !g.Exceeded(o) {
		t.Fatalf("expected breach on MaxItems=2 with 3 products")
	}
	if err := checkGuardrails(g, o); err != ErrCapacityExceeded {
		t.Fatalf("expected ErrCapacityExceeded, got %v", err)
	}

	// flush path: a sink receives the buffer and it is reset
	flushed := false
	sink := sinkFunc(func(_ context.Context, items []Product) error {
		flushed = true
		if len(items) != 3 {
			t.Fatalf("sink got %d items", len(items))
		}
		return nil
	})
	err := o.EnforceGuardrails(CapacityGuardrails{MaxItems: 2, OnExceed: ExceedFlush}, sink, context.Background())
	if err != nil || !o.Flushed || !flushed {
		t.Fatalf("flush path failed: flushed=%v err=%v", flushed, err)
	}
	if len(o.Products) != 0 {
		t.Fatalf("buffer should be reset after flush, got %d", len(o.Products))
	}
}

func TestEnforceGuardrails(t *testing.T) {
	// Error policy: returns ErrCapacityExceeded, leaves products untouched.
	o := Outputs{Products: make([]Product, 5)}
	err := o.EnforceGuardrails(CapacityGuardrails{MaxItems: 2, OnExceed: ExceedError}, nil, context.Background())
	if err != ErrCapacityExceeded {
		t.Fatalf("EnforceGuardrails(error) = %v, want ErrCapacityExceeded", err)
	}
	if len(o.Products) != 5 {
		t.Errorf("error policy must not drop products, got len=%d", len(o.Products))
	}

	// Flush policy with sink: emits and resets.
	var captured []Product
	sink := sinkFunc(func(_ context.Context, items []Product) error {
		captured = append(captured, items...)
		return nil
	})
	o = Outputs{Products: make([]Product, 5)}
	err = o.EnforceGuardrails(CapacityGuardrails{MaxItems: 2, OnExceed: ExceedFlush}, sink, context.Background())
	if err != nil {
		t.Fatalf("EnforceGuardrails(flush+sink) err = %v", err)
	}
	if !o.Flushed {
		t.Error("Flushed flag not set after flush")
	}
	if len(o.Products) != 0 {
		t.Errorf("products not reset after flush, len=%d", len(o.Products))
	}
	if len(captured) != 5 {
		t.Errorf("sink got %d items, want 5", len(captured))
	}

	// Flush policy WITHOUT sink: degrades to no-op (preserves the buffer
	// for downstream merging; the caller decides whether to continue).
	o = Outputs{Products: make([]Product, 5)}
	err = o.EnforceGuardrails(CapacityGuardrails{MaxItems: 2, OnExceed: ExceedFlush}, nil, context.Background())
	if err != nil {
		t.Fatalf("EnforceGuardrails(flush+nil sink) err = %v", err)
	}
	if o.Flushed {
		t.Error("Flushed should stay false when no sink is available")
	}
	if len(o.Products) != 5 {
		t.Errorf("nil-sink flush should preserve products, got len=%d", len(o.Products))
	}

	// Under-limit: both error and flush policies must be no-ops.
	for _, on := range []ExceedAction{ExceedError, ExceedFlush} {
		o := Outputs{Products: make([]Product, 2)}
		err := o.EnforceGuardrails(CapacityGuardrails{MaxItems: 5, OnExceed: on}, sink, context.Background())
		if err != nil {
			t.Errorf("under-limit EnforceGuardrails(%s) err = %v", on, err)
		}
		if o.Flushed {
			t.Errorf("under-limit Flushed should be false (on=%s)", on)
		}
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

// checkGuardrails mirrors the error-path the variant Run applies before return.
func checkGuardrails(g CapacityGuardrails, o Outputs) error {
	if g.OnExceed == ExceedError && g.Exceeded(o) {
		return ErrCapacityExceeded
	}
	return nil
}

type sinkFunc func(ctx context.Context, items []Product) error

func (f sinkFunc) Emit(ctx context.Context, items []Product) error { return f(ctx, items) }
