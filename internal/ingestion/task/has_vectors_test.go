package task

import (
	"testing"
)

// =============================================================================
// hasVectors — checks if any chunk has q_\d+_vec key
// =============================================================================

func TestHasVectors_WithVector(t *testing.T) {
	chunks := []map[string]any{
		{"text": "hello", "q_768_vec": []float64{0.1, 0.2}},
	}
	if !hasVectors(chunks) {
		t.Error("expected true for chunk with q_768_vec")
	}
}

func TestHasVectors_WithoutVector(t *testing.T) {
	chunks := []map[string]any{
		{"text": "hello"},
	}
	if hasVectors(chunks) {
		t.Error("expected false for chunk without q_*_vec")
	}
}

func TestHasVectors_MultipleChunksOneHasVector(t *testing.T) {
	chunks := []map[string]any{
		{"text": "hello"},
		{"text": "world", "q_3_vec": []float64{0.1, 0.2, 0.3}},
	}
	if !hasVectors(chunks) {
		t.Error("expected true when one chunk has vector")
	}
}

func TestHasVectors_VectorInFirstChunkOnly(t *testing.T) {
	chunks := []map[string]any{
		{"q_128_vec": []float64{0.5}, "text": "a"},
		{"text": "b"},
	}
	if !hasVectors(chunks) {
		t.Error("expected true when first chunk has vector")
	}
}

func TestHasVectors_EmptyChunks(t *testing.T) {
	if hasVectors(nil) {
		t.Error("expected false for nil chunks")
	}
	if hasVectors([]map[string]any{}) {
		t.Error("expected false for empty chunks")
	}
}

func TestHasVectors_OtherNumberKeys(t *testing.T) {
	chunks := []map[string]any{
		{"text": "hello", "q_abc_vec": "not a vector"},
	}
	if hasVectors(chunks) {
		t.Error("q_abc_vec should not match q_\\d+_vec")
	}
}

func TestHasVectors_EmptySliceValue(t *testing.T) {
	chunks := []map[string]any{
		{"text": "hello", "q_0_vec": []float64{}},
	}
	if !hasVectors(chunks) {
		t.Error("q_0_vec should match even with empty vector")
	}
}
