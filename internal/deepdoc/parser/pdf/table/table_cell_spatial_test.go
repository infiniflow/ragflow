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
