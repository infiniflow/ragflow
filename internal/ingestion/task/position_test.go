package task

import (
	"testing"
)

// =============================================================================
// AddPositions
// Canonical format: [pageNum, left, right, top, bottom] × N
// Mirrors Python: rag.nlp.add_positions()
// =============================================================================

func TestAddPositions_Basic(t *testing.T) {
	chunk := map[string]any{}
	// [pn=0, left=100, right=50, top=200, bottom=150]
	positions := []float64{0, 100, 50, 200, 150}
	AddPositions(chunk, positions)

	pageNum, ok := chunk["page_num_int"].([]int)
	if !ok || len(pageNum) != 1 || pageNum[0] != 1 {
		t.Errorf("page_num_int = %v, want [1]", pageNum)
	}
	top, ok := chunk["top_int"].([]int)
	if !ok || len(top) != 1 || top[0] != 200 {
		t.Errorf("top_int = %v, want [200]", top)
	}
	position, ok := chunk["position_int"].([][]int)
	if !ok || len(position) != 1 {
		t.Fatalf("position_int = %v, want [[1 100 50 200 150]]", position)
	}
	if position[0][0] != 1 || position[0][1] != 100 || position[0][2] != 50 || position[0][3] != 200 || position[0][4] != 150 {
		t.Errorf("position_int[0] = %v, want [1 100 50 200 150]", position[0])
	}
}

func TestAddPositions_MultiplePositions(t *testing.T) {
	chunk := map[string]any{}
	positions := []float64{
		0, 100, 50, 200, 150, // pn=0, left=100, right=50, top=200, bottom=150
		1, 200, 60, 300, 250, // pn=1, left=200, right=60, top=300, bottom=250
	}
	AddPositions(chunk, positions)

	pageNum := chunk["page_num_int"].([]int)
	if len(pageNum) != 2 || pageNum[0] != 1 || pageNum[1] != 2 {
		t.Errorf("page_num_int = %v, want [1 2]", pageNum)
	}
	top := chunk["top_int"].([]int)
	if len(top) != 2 || top[0] != 200 || top[1] != 300 {
		t.Errorf("top_int = %v, want [200 300]", top)
	}
	position := chunk["position_int"].([][]int)
	if len(position) != 2 {
		t.Fatalf("position_int len = %d, want 2", len(position))
	}
}

func TestAddPositions_NilPositions(t *testing.T) {
	chunk := map[string]any{}
	AddPositions(chunk, nil)
	if _, exists := chunk["page_num_int"]; exists {
		t.Error("page_num_int should not be set for nil positions")
	}
}

func TestAddPositions_EmptyPositions(t *testing.T) {
	chunk := map[string]any{}
	AddPositions(chunk, []float64{})
	if _, exists := chunk["page_num_int"]; exists {
		t.Error("page_num_int should not be set for empty positions")
	}
}

func TestAddPositions_PartialPositions(t *testing.T) {
	chunk := map[string]any{}
	positions := []float64{0, 100} // only 2 elements, not a complete position
	AddPositions(chunk, positions)
	if _, exists := chunk["page_num_int"]; exists {
		t.Error("page_num_int should not be set for partial positions")
	}
}

func TestAddPositions_PageNumOffset(t *testing.T) {
	chunk := map[string]any{}
	positions := []float64{5, 100, 50, 200, 150} // pn=5 → 5+1=6
	AddPositions(chunk, positions)

	pageNum := chunk["page_num_int"].([]int)
	if pageNum[0] != 6 {
		t.Errorf("page_num_int = %d, want 6 (5+1)", pageNum[0])
	}
}
