package chunk

import (
	"context"
	"ragflow/internal/common"
	"reflect"
	"testing"
)

func TestIsZeroVector(t *testing.T) {
	if !common.IsZeroVector([]float64{0, 0, 0}) {
		t.Error("all zeros should be true")
	}
	if common.IsZeroVector([]float64{0, 1, 0}) {
		t.Error("non-zero should be false")
	}
	if !common.IsZeroVector([]float64{}) {
		t.Error("empty should be true (treated as zero)")
	}
	if !common.IsZeroVector(nil) {
		t.Error("nil should be true")
	}
}

func TestHydrateChunkVectors_AllNonZero(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "c1", "vector": []float64{1, 2, 3}},
		{"id": "c2", "vector": []float64{4, 5, 6}},
	}
	// No zero vectors → nothing to hydrate.
	hydrateChunkVectors(context.Background(), nil, chunks, nil, nil)
	if !reflect.DeepEqual(chunks[0]["vector"], []float64{1, 2, 3}) {
		t.Error("non-zero vector should not be changed")
	}
	if !reflect.DeepEqual(chunks[1]["vector"], []float64{4, 5, 6}) {
		t.Error("non-zero vector should not be changed")
	}
}

func TestHydrateChunkVectors_EmptyChunks(t *testing.T) {
	// Should not panic on empty or nil.
	hydrateChunkVectors(context.Background(), nil, nil, nil, nil)
	hydrateChunkVectors(context.Background(), nil, []map[string]interface{}{}, nil, nil)
}

func TestHydrateChunkVectors_MissingIDs(t *testing.T) {
	chunks := []map[string]interface{}{
		{"vector": []float64{1.0}}, // no id — skipped
	}
	hydrateChunkVectors(context.Background(), nil, chunks, nil, nil)
	// Should not change anything when engine is nil (FetchChunkVectors returns zero vectors).
	// The function doesn't panic — it just can't hydrate because dim is 0.
	// With nil engine, FetchChunkVectors returns zero vectors, so the zero stays zero.
}

func TestHydrateChunkVectors_NoDim(t *testing.T) {
	chunks := []map[string]interface{}{
		{"id": "c1", "vector": []float64{}},
	}
	hydrateChunkVectors(context.Background(), nil, chunks, []string{"kb1"}, []string{"t1"})
	// Empty vectors have dim=0 → early return. No crash.
}
