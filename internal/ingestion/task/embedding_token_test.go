package task

import (
	"testing"
)

// =============================================================================
// GetEmbeddingTokenConsumption
// =============================================================================

func TestGetEmbeddingTokenConsumption_Int(t *testing.T) {
	input := map[string]any{EmbeddingTokenConsumptionKey: 42}
	result := GetEmbeddingTokenConsumption(input)
	if result != 42 {
		t.Errorf("got %d, want 42", result)
	}
}

func TestGetEmbeddingTokenConsumption_Float64(t *testing.T) {
	input := map[string]any{EmbeddingTokenConsumptionKey: float64(42)}
	result := GetEmbeddingTokenConsumption(input)
	if result != 42 {
		t.Errorf("got %d, want 42", result)
	}
}

func TestGetEmbeddingTokenConsumption_MissingKey(t *testing.T) {
	result := GetEmbeddingTokenConsumption(map[string]any{})
	if result != 0 {
		t.Errorf("got %d, want 0", result)
	}
}

func TestGetEmbeddingTokenConsumption_NilMap(t *testing.T) {
	result := GetEmbeddingTokenConsumption(nil)
	if result != 0 {
		t.Errorf("got %d, want 0", result)
	}
}

func TestGetEmbeddingTokenConsumption_WrongType(t *testing.T) {
	input := map[string]any{EmbeddingTokenConsumptionKey: "not a number"}
	result := GetEmbeddingTokenConsumption(input)
	if result != 0 {
		t.Errorf("got %d, want 0", result)
	}
}

func TestGetEmbeddingTokenConsumption_Zero(t *testing.T) {
	input := map[string]any{EmbeddingTokenConsumptionKey: 0}
	result := GetEmbeddingTokenConsumption(input)
	if result != 0 {
		t.Errorf("got %d, want 0", result)
	}
}
