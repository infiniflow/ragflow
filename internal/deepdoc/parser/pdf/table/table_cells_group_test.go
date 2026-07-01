package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func cellTexts(cells []pdf.TSRCell) []string {
	out := make([]string, len(cells))
	for i, c := range cells {
		out[i] = c.Text
	}
	return out
}

func TestGroupTSRCellsToRows(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if rows := GroupTSRCellsToRows(nil); rows != nil {
			t.Error("nil → nil")
		}
		if rows := GroupTSRCellsToRows([]pdf.TSRCell{}); rows != nil {
			t.Error("empty → nil")
		}
	})

	t.Run("single cell", func(t *testing.T) {
		cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A"}}
		rows := GroupTSRCellsToRows(cells)
		if len(rows) != 1 || rows[0][0].Text != "A" {
			t.Error("single cell not preserved")
		}
	})

	t.Run("two rows two cols", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "A"},
			{X0: 50, Y0: 0, X1: 100, Y1: 30, Text: "B"},
			{X0: 0, Y0: 50, X1: 50, Y1: 80, Text: "C"},
			{X0: 50, Y0: 50, X1: 100, Y1: 80, Text: "D"},
		}
		rows := GroupTSRCellsToRows(cells)
		if len(rows) != 2 {
			t.Fatalf("2 rows expected, got %d", len(rows))
		}
		if rows[0][0].Text != "A" || rows[0][1].Text != "B" {
			t.Errorf("row0: %v", cellTexts(rows[0]))
		}
		if rows[1][0].Text != "C" || rows[1][1].Text != "D" {
			t.Errorf("row1: %v", cellTexts(rows[1]))
		}
	})

	t.Run("unsorted input", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 50, Y0: 50, X1: 100, Y1: 80, Text: "D"},
			{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "A"},
			{X0: 0, Y0: 50, X1: 50, Y1: 80, Text: "C"},
			{X0: 50, Y0: 0, X1: 100, Y1: 30, Text: "B"},
		}
		rows := GroupTSRCellsToRows(cells)
		if len(rows) != 2 {
			t.Fatalf("unsorted: 2 rows expected, got %d", len(rows))
		}
		if rows[0][0].Text != "A" || rows[0][1].Text != "B" {
			t.Errorf("unsorted row0: %v", cellTexts(rows[0]))
		}
	})

	t.Run("tall merged cell", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 50, Y1: 100, Text: "merged"},
			{X0: 50, Y0: 0, X1: 100, Y1: 30, Text: "B"},
			{X0: 50, Y0: 50, X1: 100, Y1: 80, Text: "D"},
		}
		rows := GroupTSRCellsToRows(cells)
		// merged cell starts Y0=0 → row 0; Y0=50 cell → row 1
		if len(rows) != 2 {
			t.Fatalf("merged cell: 2 rows expected, got %d", len(rows))
		}
	})

	t.Run("large gap different rows", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "top"},
			{X0: 0, Y0: 200, X1: 50, Y1: 230, Text: "far"},
		}
		rows := GroupTSRCellsToRows(cells)
		if len(rows) != 2 {
			t.Fatalf("large gap: 2 rows expected, got %d", len(rows))
		}
	})

	t.Run("close rows", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 10, Y1: 8, Text: "Row1"},
			{X0: 0, Y0: 9, X1: 10, Y1: 17, Text: "Row2"},
		}
		rows := GroupTSRCellsToRows(cells)
		if len(rows) != 2 {
			t.Errorf("close rows: expected 2, got %d", len(rows))
		}
	})

	t.Run("varying heights", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 10, Y1: 5, Text: "A"},
			{X0: 0, Y0: 50, X1: 10, Y1: 70, Text: "B"},
			{X0: 0, Y0: 50, X1: 10, Y1: 70, Text: "C"},
		}
		rows := GroupTSRCellsToRows(cells)
		if len(rows) != 2 {
			t.Fatalf("varying heights: expected 2 rows, got %d", len(rows))
		}
		if len(rows[0]) != 1 || rows[0][0].Text != "A" {
			t.Errorf("row 0: expected [A], got %v", cellTexts(rows[0]))
		}
	})
}

// ── fillCellTextFromBoxes ──────────────────────────────────────────────

func TestFillCellTextFromBoxes(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50},
			{X0: 100, Y0: 0, X1: 200, Y1: 50},
		}
		boxes := []pdf.TextBox{
			{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "A"},
			{X0: 100, X1: 200, Top: 0, Bottom: 50, Text: "B"},
		}
		FillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "A" || cells[1].Text != "B" {
			t.Errorf("got %q/%q, want A/B", cells[0].Text, cells[1].Text)
		}
	})

	t.Run("empty cells", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50},
			{X0: 100, Y0: 0, X1: 200, Y1: 50},
		}
		boxes := []pdf.TextBox{
			{X0: 0, X1: 100, Top: 0, Bottom: 50, Text: "only first"},
		}
		FillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "only first" {
			t.Errorf("cell[0]: got %q", cells[0].Text)
		}
		if cells[1].Text != "" {
			t.Errorf("cell[1] should be empty, got %q", cells[1].Text)
		}
	})

	t.Run("partial cell coverage — empty cell filled from any overlapping box", func(t *testing.T) {
		// Box covers 40% of cell area.  Old code rejected (<85% cell coverage).
		// New code: cell is empty → accepts box (≥30% box area inside cell).
		cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 200, Y1: 50}}
		boxes := []pdf.TextBox{{X0: 0, X1: 80, Top: 0, Bottom: 50, Text: "partial"}}
		FillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "partial" {
			t.Errorf("empty cell should be filled from overlapping box, got %q", cells[0].Text)
		}
	})

	t.Run("box inside cell >85%", func(t *testing.T) {
		cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 500, Y1: 300}}
		boxes := []pdf.TextBox{{X0: 10, X1: 490, Top: 10, Bottom: 290, Text: "inside"}}
		FillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "inside" {
			t.Errorf("got %q", cells[0].Text)
		}
	})

	t.Run("concatenate two boxes to same cell", func(t *testing.T) {
		cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 200, Y1: 100}}
		boxes := []pdf.TextBox{
			{X0: 5, X1: 195, Top: 2, Bottom: 98, Text: "hello"},
			{X0: 5, X1: 195, Top: 2, Bottom: 98, Text: "world"},
		}
		FillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "hello world" {
			t.Errorf("got %q, want 'hello world'", cells[0].Text)
		}
	})

	t.Run("empty inputs", func(t *testing.T) {
		FillCellTextFromBoxes(nil, nil)
		FillCellTextFromBoxes([]pdf.TSRCell{}, []pdf.TextBox{})
		c := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 1, Y1: 1}}
		FillCellTextFromBoxes(c, nil)
		if c[0].Text != "" {
			t.Error("no boxes → text empty")
		}
	})
}

// ── enrichWithDeepDoc noop ─────────────────────────────────────────────

func TestGroupTSRCellsToRows_SameHeight(t *testing.T) {
	// All cells have identical height → medianH is that value → threshold = medianH/2
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "A"},
		{X0: 50, Y0: 0, X1: 100, Y1: 30, Text: "B"},
		{X0: 0, Y0: 31, X1: 50, Y1: 61, Text: "C"}, // gap = 31-30=1 < 30/2=15 → same row? NO, Y0=31 is right at edge
	}
	rows := GroupTSRCellsToRows(cells)
	// medianH=30, threshold=15. C.Y0=31 > curY+threshold?" curY=0, 31 > 15 → new row.
	// So A,B in row 0, C in row 1.
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if len(rows[0]) != 2 || len(rows[1]) != 1 {
		t.Errorf("row sizes: %d %d, want 2 1", len(rows[0]), len(rows[1]))
	}
}

func TestFillCellTextFromBoxes_WhitespaceTrim(t *testing.T) {
	cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 100}}
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 100, Text: "  hello  "}}
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "hello" {
		t.Errorf("got %q, want 'hello'", cells[0].Text)
	}
}

func TestFillCellTextFromBoxes_EmptyBoxIgnored(t *testing.T) {
	cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 100}}
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 0, Bottom: 100, Text: "   "}} // all whitespace
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("whitespace text should produce empty, got %q", cells[0].Text)
	}
}
