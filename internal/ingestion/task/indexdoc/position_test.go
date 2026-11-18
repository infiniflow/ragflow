package indexdoc

import (
	"testing"
)

// =============================================================================
// addPDFPositions
// Canonical format: [pageNum, left, right, top, bottom] × N
// Contract: input page numbers are ALREADY 1-indexed (the 0→1 conversion
// happens once, at the parser boundary in normalizePDFPageNumber). This
// function is a passthrough — it must NOT add +1, otherwise callers that
// already feed 1-indexed values (the PDF path after normalization) get a
// double-incremented page number.
// =============================================================================

func TestAddPDFPositions_Basic(t *testing.T) {
	chunk := map[string]any{}
	// [pn=1 (first page, 1-indexed), left=100, right=50, top=200, bottom=150]
	positions := []float64{1, 100, 50, 200, 150}
	addPDFPositions(chunk, positions)

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

func TestAddPDFPositions_MultiplePositions(t *testing.T) {
	chunk := map[string]any{}
	positions := []float64{
		1, 100, 50, 200, 150, // pn=1, left=100, right=50, top=200, bottom=150
		2, 200, 60, 300, 250, // pn=2, left=200, right=60, top=300, bottom=250
	}
	addPDFPositions(chunk, positions)

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

func TestAddPDFPositions_NilPositions(t *testing.T) {
	chunk := map[string]any{}
	addPDFPositions(chunk, nil)
	if _, exists := chunk["page_num_int"]; exists {
		t.Error("page_num_int should not be set for nil positions")
	}
}

func TestAddPDFPositions_EmptyPositions(t *testing.T) {
	chunk := map[string]any{}
	addPDFPositions(chunk, []float64{})
	if _, exists := chunk["page_num_int"]; exists {
		t.Error("page_num_int should not be set for empty positions")
	}
}

func TestAddPDFPositions_PartialPositions(t *testing.T) {
	chunk := map[string]any{}
	positions := []float64{1, 100} // only 2 elements, not a complete position
	addPDFPositions(chunk, positions)
	if _, exists := chunk["page_num_int"]; exists {
		t.Error("page_num_int should not be set for partial positions")
	}
}

func TestAddPDFPositions_PassthroughNoOffset(t *testing.T) {
	// page numbers are 1-indexed on entry; addPDFPositions must not add +1.
	chunk := map[string]any{}
	positions := []float64{6, 100, 50, 200, 150} // pn=6, 1-indexed
	addPDFPositions(chunk, positions)

	pageNum := chunk["page_num_int"].([]int)
	if pageNum[0] != 6 {
		t.Errorf("page_num_int = %d, want 6 (passthrough, no +1)", pageNum[0])
	}
	position := chunk["position_int"].([][]int)
	if position[0][0] != 6 {
		t.Errorf("position_int[0][0] = %d, want 6 (passthrough, no +1)", position[0][0])
	}
}

// =============================================================================
// addSpreadsheetPositions
// Vocabulary: [sheet, rowStart, rowEnd, colStart, colEnd] × N
// Contract: position_int is the preview's coordinate carrier and is written;
// page_num_int/top_int are PDF layout fields and must NOT be derived from these
// tuples, so a chunk's own top_int (the QA row index) survives.
// =============================================================================

func TestAddSpreadsheetPositions_WritesPositionIntOnly(t *testing.T) {
	chunk := map[string]any{"top_int": []int{41}}
	addSpreadsheetPositions(chunk, []float64{2, 42, 42, 1, 3})

	matrix, ok := chunk["position_int"].([][]int)
	if !ok || len(matrix) != 1 || len(matrix[0]) != 5 {
		t.Fatalf("position_int = %v, want one five-field tuple", chunk["position_int"])
	}
	want := []int{2, 42, 42, 1, 3}
	for i, v := range want {
		if matrix[0][i] != v {
			t.Fatalf("position_int[0] = %v, want %v", matrix[0], want)
		}
	}
	if _, exists := chunk["page_num_int"]; exists {
		t.Errorf("page_num_int must not be derived from a spreadsheet tuple: %v", chunk["page_num_int"])
	}
	if top, ok := chunk["top_int"].([]int); !ok || len(top) != 1 || top[0] != 41 {
		t.Errorf("top_int = %v, want the chunk's own row index [41]", chunk["top_int"])
	}
}

func TestAddSpreadsheetPositions_MalformedNoop(t *testing.T) {
	for _, positions := range [][]float64{nil, {}, {2, 42, 42, 1}} {
		chunk := map[string]any{}
		addSpreadsheetPositions(chunk, positions)
		if _, exists := chunk["position_int"]; exists {
			t.Errorf("positions %v must be dropped, got %v", positions, chunk["position_int"])
		}
	}
}
