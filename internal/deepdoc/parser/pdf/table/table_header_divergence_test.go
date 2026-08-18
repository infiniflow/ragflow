package table

import (
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"testing"
)

// These tests pin the remaining divergences between Go's header detection and
// Python's construct_table (deepdoc/vision/table_structure_recognizer.py:336-348)
// and t_recognizer.py. They assert the Python behavior, so they are expected to
// FAIL (RED) until the follow-up fix lands.
//
// Python reference:
//   - Header region = cells whose label ends in "header"
//     (t_recognizer.py:64 `headers = gather(r".*header$")`), NOT the first grid
//     row (grid[0] approximation).
//   - Per-column predicate `any(a.get("H")) or (max_type=="Nu" and btype!="Nu")`,
//     with a numeric cell in a numeric-dominant table SKIPPED entirely
//     (table_structure_recognizer.py:343 `continue`). Every row is scored
//     independently — there is NO early stop / prefix break.

// TestAnnotateTableBoxes_HeaderNotOnFirstRow exposes the grid[0] approximation.
// Python's header region is the set of cells the layout model labeled as header
// (t_recognizer.py:64 `gather(r".*header$")`), NOT the first grid row. So a
// header that sits on a row other than row 0 is still detected via H.
// Go used to hardcode `headers = grid[0]` in AnnotateTableBoxes, so a header not
// on row 0 got no H.
//
// Here row 1 (not row 0) is the header. The box on row 1 must receive H>0.
func TestAnnotateTableBoxes_HeaderNotOnFirstRow(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 10, X1: 100, Y1: 30, Label: "table row"},           // row 0: data
		{X0: 0, Y0: 30, X1: 100, Y1: 50, Label: "table column header"}, // row 1: header
	}
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 10, Bottom: 30, LayoutType: pdf.LayoutTypeTable, Text: "Data"},   // overlaps row 0
		{X0: 0, X1: 100, Top: 30, Bottom: 50, LayoutType: pdf.LayoutTypeTable, Text: "Header"}, // overlaps row 1
	}

	AnnotateTableBoxes(boxes, GroupTSRCellsToRows(cells))

	hdrIdx, dataIdx := -1, -1
	for i := range boxes {
		if boxes[i].R == 1 {
			hdrIdx = i
		}
		if boxes[i].R == 0 {
			dataIdx = i
		}
	}
	if hdrIdx < 0 || dataIdx < 0 {
		t.Fatal("boxes were not annotated with a row index")
	}

	// Python: header region = header-labeled cells (row 1) -> the row-1 box overlaps it -> H>0.
	if boxes[hdrIdx].H <= 0 {
		t.Errorf("GRID[0] DIVERGENCE: the box on the real header row (row 1) must get H>0 (Python matches header-labeled boxes, not grid[0]). Got H=%d", boxes[hdrIdx].H)
	}
	// Python: the row-0 data box does NOT overlap the header region -> H stays 0.
	if boxes[dataIdx].H > 0 {
		t.Errorf("GRID[0] DIVERGENCE: Go's grid[0] approximation wrongly sets H on the non-header row-0 box. Got H=%d", boxes[dataIdx].H)
	}
}

// TestHeaderSetWithBlockType_NumericCellsSkipped exposes the per-cell predicate
// divergence. In a numeric-dominant table Python SKIPS numeric cells
// (table_structure_recognizer.py:343 `continue`): a numeric column with H
// contributes NOTHING — only non-numeric columns can push a row past the >0.5
// majority. Go's OLD code evaluated the geometric H signal in a separate pass
// that counted numeric-with-H columns, so it over-detected.
//
// Here 3/4 columns are numeric and carry H, only 1 is non-numeric. Python:
// 1/4 non-numeric < 0.5 -> NOT a header. The faithful fold must agree.
func TestHeaderSetWithBlockType_NumericCellsSkipped(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{
			{Text: "100", Label: "table row"},  // numeric, gets H below
			{Text: "200", Label: "table row"},  // numeric, gets H
			{Text: "300", Label: "table row"},  // numeric, gets H
			{Text: "Name", Label: "table row"}, // non-numeric, no H
		},
	}

	// col0/1/2 boxes carry H (they overlap the header region); col3 does not.
	boxes := []pdf.TextBox{
		{Text: "100", R: 0, C: 0, H: 1},
		{Text: "200", R: 0, C: 1, H: 1},
		{Text: "300", R: 0, C: 2, H: 1},
		{Text: "Name", R: 0, C: 3, H: -1},
	}

	hdrs := HeaderSetWithBlockType(rows, boxes)

	// Python skips the 3 numeric columns; only 1/4 is non-numeric -> not a header.
	if hdrs[0] {
		t.Errorf("NUMERIC-SKIP DIVERGENCE: Python skips numeric cells, so 3/4 numeric-with-H columns cannot form a header; only 1/4 non-numeric -> not a header. Go's old per-pass geometric counted them and over-detected: %v", hdrs)
	}
}
