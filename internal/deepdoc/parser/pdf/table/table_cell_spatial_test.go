package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ---- boxOverlapsCell ----

func TestBoxMatchesCell_FullOverlap(t *testing.T) {
	// Box is entirely inside cell → ≥85% of box area inside cell → match.
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50}
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "hello"}
	if !BoxMatchesCell(cell, box, false) {
		t.Error("full overlap should return true")
	}
	// Box is still entirely inside cell → box→cell = 100% ≥ 85% → match.
	box2 := pdf.TextBox{X0: 10, X1: 90, Top: 10, Bottom: 40, Text: "partial"}
	if !BoxMatchesCell(cell, box2, false) {
		t.Error("box entirely inside cell (100% of box) should match")
	}
}

func TestBoxMatchesCell_NoOverlap(t *testing.T) {
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50}
	box := pdf.TextBox{X0: 200, X1: 300, Top: 10, Bottom: 40, Text: "away"}
	if BoxMatchesCell(cell, box, false) {
		t.Error("no X overlap should return false")
	}
}

func TestBoxMatchesCell_PartialOverlap(t *testing.T) {
	// Box is entirely inside cell (100% of box area) → matches.
	// boxOverlapsCell uses box→cell overlap (≥85% of box area inside cell).
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50}
	box := pdf.TextBox{X0: 0, X1: 30, Top: 0, Bottom: 25, Text: "small"}
	if !BoxMatchesCell(cell, box, false) {
		t.Error("box entirely inside cell should match")
	}
	// Box straddles cell boundary (< 85% of box inside cell) → no match.
	box2 := pdf.TextBox{X0: 80, X1: 180, Top: 0, Bottom: 25, Text: "spill"}
	if BoxMatchesCell(cell, box2, false) {
		t.Error("box straddling boundary (<85% inside) should NOT match")
	}
}

func TestBoxMatchesCell_ZeroArea(t *testing.T) {
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 0, Y1: 50}
	box := pdf.TextBox{X0: 0, X1: 10, Top: 0, Bottom: 10, Text: "x"}
	if BoxMatchesCell(cell, box, false) {
		t.Error("zero cell area should return false")
	}
}

// ---- fillCellTextFromBoxes ----

func TestFillCellTextFromBoxes_Simple(t *testing.T) {
	// Box covering entire cell (>85%) → match
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50},
		{X0: 100, Y0: 0, X1: 200, Y1: 50},
	}
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "cell1"},
		{X0: 100, X1: 200, Top: 0, Bottom: 50, Text: "cell2"},
	}
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "cell1" {
		t.Errorf("cell 0: got %q, want 'cell1'", cells[0].Text)
	}
	if cells[1].Text != "cell2" {
		t.Errorf("cell 1: got %q, want 'cell2'", cells[1].Text)
	}
}

func TestFillCellTextFromBoxes_MultipleBoxesPerCell(t *testing.T) {
	// Two boxes, each covering >85% of the cell → concatenated
	// (boxes must overlap the cell near-completely to match individually)
	cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50}}
	boxes := []pdf.TextBox{
		{X0: 0, X1: 95, Top: 0, Bottom: 47, Text: "part1"},
		{X0: 5, X1: 100, Top: 3, Bottom: 50, Text: "part2"},
	}
	FillCellTextFromBoxes(cells, boxes)
	// Both boxes cover >85% → both match → concatenated with space
	if cells[0].Text == "" {
		t.Error("expected non-empty cell text")
	}
}

func TestFillCellTextFromBoxes_EmptyBoxText(t *testing.T) {
	cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50}}
	boxes := []pdf.TextBox{
		{X0: 5, X1: 95, Top: 5, Bottom: 45, Text: "   "},
	}
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("empty box text: got %q, want empty", cells[0].Text)
	}
}

func TestFillCellTextFromBoxes_NoMatchingBox(t *testing.T) {
	cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50}}
	boxes := []pdf.TextBox{
		{X0: 500, X1: 600, Top: 500, Bottom: 550, Text: "far away"},
	}
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("no match: got %q, want empty", cells[0].Text)
	}
}

// TestBoxMatchesCell_FilledCellWeakOverlapRejected locks the Go-only 0.85
// guard: a cell that already carries text (e.g. per-cell OCR in the rotated
// path) rejects a box whose area is only partially inside the cell.
// Cell (0,0)-(100,50); box (40,5)-(140,15).
//
//	overlap  = (40,100)x(5,15) = 60*10 = 600
//	box area = (140-40)*(15-5) = 100*10 = 1000  → ratio 0.6
//
// 0.6 < 0.85 → rejected.
func TestBoxMatchesCell_FilledCellWeakOverlapRejected(t *testing.T) {
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "ocr"}
	box := pdf.TextBox{X0: 40, X1: 140, Top: 5, Bottom: 15, Text: "元"}
	if BoxMatchesCell(cell, box, false) {
		t.Error("filled cell should reject box overlapping only 60% (needs >=85%)")
	}
}

// TestBoxMatchesCell_FilledCellStrongOverlapAccepted locks that a box almost
// entirely inside an already-filled cell still matches (so per-cell OCR text
// can be overridden by a confident box). Box fully inside → ratio 1.0.
func TestBoxMatchesCell_FilledCellStrongOverlapAccepted(t *testing.T) {
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "ocr"}
	box := pdf.TextBox{X0: 5, X1: 95, Top: 5, Bottom: 45, Text: "text"}
	if !BoxMatchesCell(cell, box, false) {
		t.Error("filled cell should accept box fully inside (>=85%)")
	}
}

// TestFillCellTextFromBoxes_EmptyCellJoinsMultiplePartialBoxes proves the 0.85
// guard only bites cells that ENTER FillCellTextFromBoxes with text. For a cell
// that starts empty, every overlapping box (here each 60%) is matched at the
// 0.3 threshold and all are joined — no in-cell text loss in the normal path.
// cell (0,0)-(100,50); box1 (40,5)-(140,15) ratio 0.6; box2 (-40,30)-(60,45) ratio 0.6.
func TestFillCellTextFromBoxes_EmptyCellJoinsMultiplePartialBoxes(t *testing.T) {
	cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50}}
	boxes := []pdf.TextBox{
		{X0: 40, X1: 140, Top: 5, Bottom: 15, Text: "part1"},
		{X0: -40, X1: 60, Top: 30, Bottom: 45, Text: "part2"},
	}
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "part1 part2" {
		t.Errorf("empty cell should join all overlapping boxes: got %q, want 'part1 part2'", cells[0].Text)
	}
}

// TestFillCellTextFromBoxes_PrefilledCellDropsSecondaryBox documents the
// deliberate go_intentional divergence from Python.
//
// Scenario: a cell already holds per-cell OCR text ("Total"). A separate
// detected text box ("元") physically sits in the same cell but only overlaps
// 60% of its area. Go applies the 0.85 guard (cell entered with text) and
// DROPS the secondary box, keeping "Total".
//
// Python has no per-cell OCR and no filled-cell threshold: both fragments
// would be joined via find_overlapped_with_threshold(thr=0.3), yielding
// "Total 元". This test LOCKS Go's current behavior so the divergence stays
// visible; it is registered as go_intentional, not a regression target.
func TestFillCellTextFromBoxes_PrefilledCellDropsSecondaryBox(t *testing.T) {
	cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "Total"}} // pre-filled by per-cell OCR
	boxes := []pdf.TextBox{
		{X0: 40, X1: 140, Top: 5, Bottom: 15, Text: "元"}, // 60% inside
	}
	FillCellTextFromBoxes(cells, boxes)
	// Go: 0.85 guard drops the 60%-overlap box. Python would keep "Total 元".
	if cells[0].Text != "Total" {
		t.Errorf("go_intentional divergence: got %q, want 'Total' (Python would yield 'Total 元')", cells[0].Text)
	}
}

// =============================================================================
// Implementation divergences in box→cell ASSIGNMENT (NOT architecture: both
// sides build the grid from TSR rows×columns via cross-product — see
// deepdoc_table_builder.go GroupCells vs Python construct_table). The divergences
// are purely in how a box is assigned to a (row,column) cell. Registered in
// testdata/parity/known_diffs.json as go_bug (#1, #2, #3).
// =============================================================================

// TestFillCellTextFromBoxes_BoxOverlappingTwoCells_SingleAssignment is the
// regression test for go_bug #1. Python assigns each box to EXACTLY ONE cell
// (greedy best row + tightest column); Go must no longer duplicate a
// straddling box into both cells.
//
// Two side-by-side cells; one box straddles the boundary, overlapping BOTH at
// 50% of its area. cell A (0,0)-(100,50); cell B (100,0)-(200,50).
// box (50,5)-(150,15): both columns are equally tight (dis=50), so Python
// keeps the first (C0). The box lands in exactly ONE cell.
func TestFillCellTextFromBoxes_BoxOverlappingTwoCells_SingleAssignment(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50},
		{X0: 100, Y0: 0, X1: 200, Y1: 50},
	}
	boxes := []pdf.TextBox{
		{X0: 50, X1: 150, Top: 5, Bottom: 15, Text: "shared"},
	}
	FillCellTextFromBoxes(cells, boxes)
	count := 0
	for _, c := range cells {
		if c.Text == "shared" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("regression #1: box duplicated into %d cells; Python assigns it to exactly ONE cell", count)
	}
}

// TestFillCellTextFromBoxes_PythonAssignsGoRejects_2DThreshold_FIXED is the
// regression test for go_bug #2. Python's 0.3 tests the 1-D VERTICAL overlap
// with the (full-width) row, and the column is chosen by
// find_horizontally_tightest_fit (no threshold). Go must match: a box whose
// row-vertical overlap is >= 0.3 and that is tightest to a column must be
// assigned, even if its 2-D overlap with that single cell is < 0.3.
//
// Grid: row R0 Y=(0,30); col C0 X=(0,50), col C1 X=(50,200).
// cell (R0,C0) = (0,0)-(50,30); cell (R0,C1) = (50,0)-(200,30).
// box (0,0)-(100,100): vertical overlap with R0 = 30/100 = 30% (≥0.3 row
// match); horizontally tightest to C0 (left edge aligns). Python → (R0,C0).
func TestFillCellTextFromBoxes_PythonAssignsGoRejects_2DThreshold_FIXED(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 50, Y1: 30},   // (R0,C0)
		{X0: 50, Y0: 0, X1: 200, Y1: 30}, // (R0,C1)
	}
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 100, Text: "tall"},
	}
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "tall" {
		t.Errorf("regression #2: Go dropped the box (cell0=%q); Python fills (R0,C0) via row-vertical-0.3 ∩ column-tightest", cells[0].Text)
	}
	if cells[1].Text != "" {
		t.Errorf("regression #2: box leaked into (R0,C1)=%q; should land only in (R0,C0)", cells[1].Text)
	}
}

// TestFillCellTextFromBoxes_EqualBoxRatioSingleAssignment is the regression
// test for go_bug #3. Go must no longer inject a box into ALL cells it
// overlaps; it assigns the box to the single tightest column, so a box
// overlapping a small cell A and a large cell B lands in ONLY A.
//
// Small cell A (0,0)-(50,25) area 1250; large cell B (50,0)-(200,50) area 7500.
// box (0,0)-(100,25): overlaps both, but is tightest to A (left edge aligns,
// dis=0 vs B's dis=50). Python keeps only A; Go must too.
func TestFillCellTextFromBoxes_EqualBoxRatioSingleAssignment(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 50, Y1: 25},   // A (small)
		{X0: 50, Y0: 0, X1: 200, Y1: 50}, // B (large)
	}
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 25, Text: "wide"},
	}
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "wide" {
		t.Errorf("regression #3: small cell A should contain 'wide', got %q", cells[0].Text)
	}
	if cells[1].Text == "wide" {
		t.Errorf("regression #3: box leaked into large cell B; Python keeps only small cell A")
	}
}

// TestFillCellTextFromBoxes_RowSelectionIgnores2DCellOverlap strengthens the
// go_bug #2 fix on a 2×2 grid. A tall box overlaps ONLY row R0 vertically
// (≥0.3) but its 2-D intersection with every single cell is < 0.3, so the old
// 2-D 0.3 filter dropped it entirely. The row/column selection must still fill
// (R0,C0) — the tightest column in the matched row — and leave R1 empty.
func TestFillCellTextFromBoxes_RowSelectionIgnores2DCellOverlap(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 50, Y1: 30},    // (R0,C0)
		{X0: 50, Y0: 0, X1: 200, Y1: 30},  // (R0,C1)
		{X0: 0, Y0: 30, X1: 50, Y1: 60},   // (R1,C0)
		{X0: 50, Y0: 30, X1: 200, Y1: 60}, // (R1,C1)
	}
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 100, Text: "tall"}, // vertical overlap with R0 = 30/100 = 0.3
	}
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "tall" {
		t.Errorf("regression #2 (2x2): (R0,C0) should be 'tall', got %q", cells[0].Text)
	}
	if cells[1].Text != "" || cells[2].Text != "" || cells[3].Text != "" {
		t.Errorf("regression #2 (2x2): box leaked to R0C1/R1 = %q/%q/%q; must stay only in (R0,C0)",
			cells[1].Text, cells[2].Text, cells[3].Text)
	}
}

// TestFillCellTextFromBoxes_RowSelectionTiebreak guards the row-selection
// _ov tie-break (inter/rowArea) in FillCellTextFromBoxes. When a box overlaps
// two rows with the SAME vertical-overlap ratio (ov), the row with the higher
// _ov (smaller row area for equal vertical overlap) wins — mirroring Python's
// (ov, _ov) ordering in find_overlapped_with_threshold.
//
// row0 y[0,40] (height 40), row1 y[40,100] (height 60). Box y[0,80] (height 80)
// overlaps BOTH rows by 40 → ov = 40/80 = 0.5 for each. _ov: row0 = 40/40 = 1.0,
// row1 = 40/60 ≈ 0.667. Python keeps row0; Go must too.
func TestFillCellTextFromBoxes_RowSelectionTiebreak(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 40},   // row0 (higher _ov)
		{X0: 0, Y0: 40, X1: 100, Y1: 100}, // row1 (lower _ov)
	}
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 80, Text: "row0"},
	}
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "row0" {
		t.Errorf("row tiebreak: box should land in row0 (higher _ov), got cell0=%q", cells[0].Text)
	}
	if cells[1].Text != "" {
		t.Errorf("row tiebreak: box leaked into row1 (lower _ov), got %q", cells[1].Text)
	}
}
