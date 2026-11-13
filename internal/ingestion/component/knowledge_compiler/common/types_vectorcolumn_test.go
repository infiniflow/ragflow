package common

import (
	"testing"
)

// TestVectorFromChunkMap exercises the detection logic that VectorFromChunkMap
// uses to pick the embedding column out of a chunk map. It guards the
// q_<dim>_vec vocabulary regardless of whether the detection is inline or
// routed through the shared helper.
func TestVectorFromChunkMap(t *testing.T) {
	t.Run("exact dim match returns the vector", func(t *testing.T) {
		m := map[string]any{"q_768_vec": []float64{1, 2, 3}}
		got, err := VectorFromChunkMap(m, 768)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 || got[0] != 1 || got[2] != 3 {
			t.Fatalf("got %v, want [1 2 3]", got)
		}
	})

	t.Run("[]float32 payload is normalised", func(t *testing.T) {
		m := map[string]any{"q_768_vec": []float32{4, 5}}
		got, err := VectorFromChunkMap(m, 768)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 || got[1] != 5 {
			t.Fatalf("got %v, want [4 5]", got)
		}
	})

	t.Run("dim=0 scans for any q_*_vec", func(t *testing.T) {
		m := map[string]any{"q_1536_vec": []float64{9}}
		got, err := VectorFromChunkMap(m, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0] != 9 {
			t.Fatalf("got %v, want [9]", got)
		}
	})

	t.Run("shadow column q_<dim>_vec_n is excluded when scanning", func(t *testing.T) {
		// SereneDB normalization appends "_n" to the stored column; it must not
		// be mistaken for a real embedding vector when dim is unknown.
		m := map[string]any{"q_768_vec_n": []float64{1, 2}}
		got, err := VectorFromChunkMap(m, 0)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("shadow column should be ignored, got %v", got)
		}
	})

	t.Run("multiple embedding vectors are rejected", func(t *testing.T) {
		m := map[string]any{
			"q_768_vec":  []float64{1},
			"q_1536_vec": []float64{2},
		}
		if _, err := VectorFromChunkMap(m, 0); err == nil {
			t.Fatal("expected error for multiple embedding vectors, got nil")
		}
	})

	t.Run("missing vector returns nil without error", func(t *testing.T) {
		m := map[string]any{"content_with_weight": "{}"}
		got, err := VectorFromChunkMap(m, 768)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != nil {
			t.Fatalf("expected nil, got %v", got)
		}
	})
}
