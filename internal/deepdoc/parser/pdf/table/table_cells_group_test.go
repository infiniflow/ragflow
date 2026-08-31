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

// ── enrichOnePageWithDeepDoc noop ──────────────────────────────────────

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

func TestFillCellTextFromBoxes_RowStripUsesTSRRowBBox(t *testing.T) {
	// 13_crosspage_table root cause: TSR "table row" 44 has a narrower X
	// bbox (x0=106.9) than the grid column union (x0=90.8). A col-0 box
	// spanning rows 43/44 ("2024-43 2024-44", x0=87) overlaps row 44's true
	// bbox by only ~21% (Python's find_overlapped_with_threshold rejects it,
	// thr=0.3) but overlaps the full-width grid strip by ~50% — so Go picked
	// row 44 while Python fell back to row 43. The rowStrip variant must use
	// the TSR row bbox so the box lands in the upper row.
	grid := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 1500, Y1: 54},
			{X0: 1500, Y0: 0, X1: 3000, Y1: 54},
		},
		{
			{X0: 0, Y0: 54, X1: 1500, Y1: 108},
			{X0: 1500, Y0: 54, X1: 3000, Y1: 108},
		},
	}
	// TSR row bboxes (sorted top-to-bottom): row 1 is narrower on the left,
	// like table_rotation/13's second-row-of-a-pair. Without rowStrips the
	// full-width strip makes the box overlap row 1 at ~51% (> row 0's 49%)
	// and Go wrongly assigns it to the lower row.
	rowStrips := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 3000, Y1: 54},
		{X0: 57, Y0: 54, X1: 1400, Y1: 108}, // narrow left edge: col-0 box partially outside
	}
	// Box spans both rows, x0=0 is inside row 0's strip but outside row 1's
	// narrow strip (x0=57), so only row 0 gets >= 0.3 overlap.
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 20, Bottom: 90, Text: "AB"}}

	flat := FlattenGrid(grid)
	FillCellTextFromBoxesWithRows(flat, boxes, rowStrips)

	if flat[0].Text != "AB" { // grid row0 col0
		t.Fatalf("row0 col0 = %q, want AB (box must reject narrow row1 bbox)", flat[0].Text)
	}
	if flat[2].Text != "" { // grid row1 col0
		t.Errorf("row1 col0 = %q, want empty (box belongs to row 0)", flat[2].Text)
	}
}

// TestFillCellTextFromBoxes_RowStripHeaderTable locks that the TSR row-strip
// override applies to data rows even when the table HAS a header. The caller
// (table_extract.go) collects only "table row" components into rowStrips and
// excludes header / projrowheader; the former all-or-nothing guard
// (len(rowStrips) == len(rows)) disabled the override for any header-bearing
// table. Matching is now by Y, so data rows get the narrow strip X while the
// header row keeps the grid-union X and is unaffected.
func TestFillCellTextFromBoxes_RowStripHeaderTable(t *testing.T) {
	// 3 grid rows: header + 2 data rows. Header is wider in Y only.
	grid := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 3000, Y1: 50}, // header row
		},
		{
			{X0: 0, Y0: 50, X1: 3000, Y1: 100}, // data row A (full width)
		},
		{
			{X0: 0, Y0: 100, X1: 3000, Y1: 150}, // data row B (narrow on left)
		},
	}
	// Only data rows are collected into rowStrips (header excluded), matching
	// production: data A full-width, data B narrow (x0=57).
	rowStrips := []pdf.TSRCell{
		{X0: 0, Y0: 50, X1: 3000, Y1: 100},
		{X0: 57, Y0: 100, X1: 1400, Y1: 150}, // narrow left edge
	}
	// Box spans both data rows but is outside data row B's narrow strip
	// (x0=57), so it must land in data row A.
	boxes := []pdf.TextBox{{X0: 0, X1: 100, Top: 60, Bottom: 140, Text: "AB"}}

	flat := FlattenGrid(grid)
	FillCellTextFromBoxesWithRows(flat, boxes, rowStrips)

	if flat[1].Text != "AB" { // header=row0, data A=row1
		t.Fatalf("data row A (flat[1]) = %q, want AB (override must apply to header tables)", flat[1].Text)
	}
	if flat[2].Text != "" { // data B=row2
		t.Errorf("data row B (flat[2]) = %q, want empty (box rejected by narrow strip)", flat[2].Text)
	}
	if flat[0].Text != "" { // header row: box does not overlap it
		t.Errorf("header row (flat[0]) = %q, want empty", flat[0].Text)
	}
}
