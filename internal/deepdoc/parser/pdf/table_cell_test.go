package parser

import (
	"testing"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
)

// ---- boxOverlapsCell ----

func TestBoxOverlapsCell_FullOverlap(t *testing.T) {
	// Box is entirely inside cell → ≥85% of box area inside cell → match.
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50}
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "hello"}
	if !tbl.BoxOverlapsCell(cell, box) {
		t.Error("full overlap should return true")
	}
	// Box is still entirely inside cell → box→cell = 100% ≥ 85% → match.
	box2 := pdf.TextBox{X0: 10, X1: 90, Top: 10, Bottom: 40, Text: "partial"}
	if !tbl.BoxOverlapsCell(cell, box2) {
		t.Error("box entirely inside cell (100% of box) should match")
	}
}

func TestBoxOverlapsCell_NoOverlap(t *testing.T) {
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50}
	box := pdf.TextBox{X0: 200, X1: 300, Top: 10, Bottom: 40, Text: "away"}
	if tbl.BoxOverlapsCell(cell, box) {
		t.Error("no X overlap should return false")
	}
}

func TestBoxOverlapsCell_PartialOverlap(t *testing.T) {
	// Box is entirely inside cell (100% of box area) → matches.
	// boxOverlapsCell uses box→cell overlap (≥85% of box area inside cell).
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 100, Y1: 50}
	box := pdf.TextBox{X0: 0, X1: 30, Top: 0, Bottom: 25, Text: "small"}
	if !tbl.BoxOverlapsCell(cell, box) {
		t.Error("box entirely inside cell should match")
	}
	// Box straddles cell boundary (< 85% of box inside cell) → no match.
	box2 := pdf.TextBox{X0: 80, X1: 180, Top: 0, Bottom: 25, Text: "spill"}
	if tbl.BoxOverlapsCell(cell, box2) {
		t.Error("box straddling boundary (<85% inside) should NOT match")
	}
}

func TestBoxOverlapsCell_ZeroArea(t *testing.T) {
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 0, Y1: 50}
	box := pdf.TextBox{X0: 0, X1: 10, Top: 0, Bottom: 10, Text: "x"}
	if tbl.BoxOverlapsCell(cell, box) {
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
	tbl.FillCellTextFromBoxes(cells, boxes)
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
	tbl.FillCellTextFromBoxes(cells, boxes)
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
	tbl.FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("empty box text: got %q, want empty", cells[0].Text)
	}
}

func TestFillCellTextFromBoxes_NoMatchingBox(t *testing.T) {
	cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50}}
	boxes := []pdf.TextBox{
		{X0: 500, X1: 600, Top: 500, Bottom: 550, Text: "far away"},
	}
	tbl.FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("no match: got %q, want empty", cells[0].Text)
	}
}

// ---- regionOverlapsBox ----

func TestRegionOverlapsBox_StrongOverlap(t *testing.T) {
	region := pdf.DLARegion{X0: 0, Y0: 0, X1: 216, Y1: 108} // DLA coords at 216 DPI
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 50}
	if !regionOverlapsBox(region, box, 3.0) {
		t.Error("full overlap should match")
	}
}

func TestRegionOverlapsBox_NoOverlap(t *testing.T) {
	region := pdf.DLARegion{X0: 0, Y0: 0, X1: 216, Y1: 108}
	box := pdf.TextBox{X0: 500, X1: 600, Top: 500, Bottom: 550}
	if regionOverlapsBox(region, box, 3.0) {
		t.Error("no overlap should return false")
	}
}

func TestRegionOverlapsBox_WeakOverlap(t *testing.T) {
	// Overlap at 30% → below 40% threshold → false.
	region := pdf.DLARegion{X0: 0, Y0: 0, X1: 90, Y1: 90}   // 30x30 at scale 3
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 100} // overlap = 30*30/10000 = 9%? No: 30x30=900 / 10000 = 9%
	if regionOverlapsBox(region, box, 3.0) {
		t.Error("9% overlap should return false")
	}
	// Overlap ≥ 40% → should match (Python thr=0.4).
	// box 100x100=10000 area; region 100x40=4000 → exactly 40%.
	region2 := pdf.DLARegion{X0: 0, Y0: 0, X1: 300, Y1: 120, Label: "table"} // 100x40 at scale 3
	if !regionOverlapsBox(region2, box, 3.0) {
		t.Error("40% overlap should match (>= 0.4)")
	}
	// Region that covers most of the box → should match
	region3 := pdf.DLARegion{X0: 0, Y0: 0, X1: 270, Y1: 270} // 90x90 at scale 3
	if !regionOverlapsBox(region3, box, 3.0) {
		t.Error("81% overlap should match")
	}
}

func TestRegionOverlapsBox_ThresholdAt040(t *testing.T) {
	// Exact 40% overlap: 100x100 box, region just covering 40%
	// 0.4 * 10000 = 4000. Need region with area 4000 in box space.
	// 63.2*63.2 ≈ 3994. Let's use 100x40 = 4000.
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 100}
	region := pdf.DLARegion{X0: 0, Y0: 0, X1: 300, Y1: 120, Label: "table"} // 100x40 at scale 3
	if !regionOverlapsBox(region, box, 3.0) {
		t.Error("exact 40% overlap should match (>= 0.4)")
	}
	// 39% overlap should NOT match
	region2 := pdf.DLARegion{X0: 0, Y0: 0, X1: 300, Y1: 117, Label: "table"} // 100x39 at scale 3
	if regionOverlapsBox(region2, box, 3.0) {
		t.Error("39% overlap should NOT match")
	}
}
