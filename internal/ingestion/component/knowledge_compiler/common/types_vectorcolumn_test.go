package common

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// This is an output-invariant refactor: the old inline
// `HasPrefix("q_") && HasSuffix("_vec")` spelling and the shared
// `appcommon.IsVectorColumn` helper yield identical bytes, so a behavioral
// test cannot tell them apart. The migration's only real risk is leaving a
// call site behind on the old spelling, so we assert structurally that no
// inline `q_`/`_vec` detection remains in this file.
func TestVectorFromChunkMapNoInlineDuplication(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "types.go"))
	if err != nil {
		t.Fatalf("read types.go: %v", err)
	}
	s := string(src)
	for _, bad := range []string{
		`HasPrefix(k, "q_")`,
		`HasSuffix(k, "_vec")`,
	} {
		if strings.Contains(s, bad) {
			t.Errorf("inline vector-column detection still present in types.go: %q", bad)
		}
	}
}

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
