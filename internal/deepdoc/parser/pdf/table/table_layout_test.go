package table

import (
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"sort"
	"testing"
)

// ── Mock TSR data ──────────────────────────────────────────────────────

// makeMockTableCells returns a 2x3 table with header, rows, and spanning cell.
// Layout:
//
//	+----------+----------+
//	| col A    | col B    |  ← column headers (Y=10..30)
//	|  (span)  |          |  ← spanning cell covers both
//	+----------+----------+
//	| row 1A   | row 1B   |  ← row 1 (Y=30..50)
//	+----------+----------+
//	| row 2A   | row 2B   |  ← row 2 (Y=50..70)
//	+----------+----------+
func makeMockTableCells() []pdf.TSRCell {
	return []pdf.TSRCell{
		{X0: 10, Y0: 10, X1: 50, Y1: 30, Label: "table column header"},
		{X0: 50, Y0: 10, X1: 90, Y1: 30, Label: "table column header"},
		{X0: 70, Y0: 30, X1: 90, Y1: 50, Label: "table row"},
		{X0: 10, Y0: 30, X1: 70, Y1: 50, Label: "table row"},
		{X0: 10, Y0: 50, X1: 50, Y1: 70, Label: "table row"},
		{X0: 50, Y0: 50, X1: 90, Y1: 70, Label: "table row"},
		{X0: 10, Y0: 10, X1: 90, Y1: 30, Label: "table spanning cell"},
	}
}

func makeMockBoxes() []pdf.TextBox {
	return []pdf.TextBox{
		{X0: 10, X1: 90, Top: 25, Bottom: 55, LayoutType: "table", Text: "test table"},
		// row at Y=30..50 overlaps ~80% → should match
	}
}

func TestSortYFirstly(t *testing.T) {
	t.Run("basic sort", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 10, Y0: 50, Label: "c"},
			{X0: 10, Y0: 10, Label: "a"},
			{X0: 10, Y0: 30, Label: "b"},
		}
		SortYFirstly(cells, 5)
		if cells[0].Label != "a" || cells[1].Label != "b" || cells[2].Label != "c" {
			t.Errorf("sort order wrong: %v", cells)
		}
	})

	t.Run("same Y sorts by X", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 90, Y0: 10, Label: "right"},
			{X0: 10, Y0: 10, Label: "left"},
		}
		SortYFirstly(cells, 5)
		if cells[0].Label != "left" || cells[1].Label != "right" {
			t.Errorf("same Y should sort X ascending: %v", cells)
		}
	})
}

// ── layoutCleanup ──────────────────────────────────────────────────────

func TestLayoutCleanup(t *testing.T) {
	boxes := makeMockBoxes()

	t.Run("no overlap different types", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 10, Y0: 10, X1: 50, Y1: 30, Label: "table column header"},
			{X0: 10, Y0: 10, X1: 50, Y1: 30, Label: "table row"},
		}
		result := layoutCleanup(cells, boxes, 2, 0.7)
		if len(result) != 2 {
			t.Errorf("different types should both keep: got %d", len(result))
		}
	})

	t.Run("overlap same type keeps one", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 10, Y0: 10, X1: 50, Y1: 30, Label: "table row"},
			{X0: 12, Y0: 12, X1: 48, Y1: 28, Label: "table row"}, // mostly contained
		}
		result := layoutCleanup(cells, boxes, 2, 0.7)
		if len(result) != 1 {
			t.Errorf("overlapping same type should dedup: got %d", len(result))
		}
	})

	t.Run("non overlapping same type keeps both", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 10, Y0: 10, X1: 50, Y1: 30, Label: "table row"},
			{X0: 200, Y0: 10, X1: 250, Y1: 30, Label: "table row"}, // far away
		}
		result := layoutCleanup(cells, boxes, 2, 0.7)
		if len(result) != 2 {
			t.Errorf("non-overlapping same type should keep both: got %d", len(result))
		}
	})

	t.Run("empty boxes", func(t *testing.T) {
		result := layoutCleanup(nil, nil, 2, 0.7)
		if len(result) != 0 {
			t.Errorf("empty input should return empty: got %d", len(result))
		}
	})
}

// ── findOverlappedWithThreshold ────────────────────────────────────────

func TestFindOverlappedWithThreshold(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 10, Y0: 10, X1: 50, Y1: 30},
		{X0: 50, Y0: 30, X1: 90, Y1: 50},
		{X0: 10, Y0: 50, X1: 50, Y1: 70},
	}

	t.Run("exact match", func(t *testing.T) {
		box := pdf.TextBox{X0: 10, X1: 50, Top: 10, Bottom: 30}
		if idx := findOverlappedWithThreshold(box, cells, 0.3); idx != 0 {
			t.Errorf("expected idx=0, got %d", idx)
		}
	})

	t.Run("no match", func(t *testing.T) {
		box := pdf.TextBox{X0: 200, X1: 250, Top: 200, Bottom: 230}
		if idx := findOverlappedWithThreshold(box, cells, 0.3); idx != -1 {
			t.Errorf("expected idx=-1, got %d", idx)
		}
	})

	t.Run("zero area box", func(t *testing.T) {
		box := pdf.TextBox{X0: 10, X1: 10, Top: 10, Bottom: 10}
		if idx := findOverlappedWithThreshold(box, cells, 0.3); idx != -1 {
			t.Errorf("zero-area box should return -1: got %d", idx)
		}
	})
}

// ── annotateTableBoxes ─────────────────────────────────────────────────

func TestAnnotateTableBoxes(t *testing.T) {
	cells := makeMockTableCells()
	boxes := makeMockBoxes()

	AnnotateTableBoxes(boxes, GroupTSRCellsToRows(cells))

	b := boxes[0]

	// Check header annotation
	if b.H < 0 {
		t.Error("header index should be >= 0 for a table with headers")
	}

	// Check row annotation
	if b.R == 0 {
		t.Error("row index should be set")
	}

	// Column annotation (2 columns)
	if b.C < 0 {
		t.Error("col index should be >= 0")
	}
}

// ── GroupTSRCellsToRows ─────────────────────────────────────────

func TestGroupTSRCellsToRowsLabeled(t *testing.T) {
	cells := makeMockTableCells()

	t.Run("label-based grouping", func(t *testing.T) {
		rows := GroupTSRCellsToRows(cells)
		if len(rows) < 2 {
			t.Errorf("expected >= 2 rows, got %d", len(rows))
		}
		// Each row should be sorted by X
		for ri, row := range rows {
			if !sort.SliceIsSorted(row, func(i, j int) bool { return row[i].X0 < row[j].X0 }) {
				t.Errorf("row %d not sorted by X", ri)
			}
		}
	})

	t.Run("fallback to Y-based", func(t *testing.T) {
		unlabeled := []pdf.TSRCell{
			{X0: 10, Y0: 10, X1: 50, Y1: 20, Label: ""},
			{X0: 10, Y0: 30, X1: 50, Y1: 40, Label: ""},
		}
		rows := GroupTSRCellsToRows(unlabeled)
		if len(rows) < 2 {
			t.Errorf("fallback: expected >= 2 rows, got %d", len(rows))
		}
	})

	t.Run("single cell", func(t *testing.T) {
		cells := []pdf.TSRCell{{X0: 0, Y0: 0, X1: 10, Y1: 10, Label: "table row"}}
		rows := GroupTSRCellsToRows(cells)
		if len(rows) != 1 {
			t.Errorf("expected 1 row, got %d", len(rows))
		}
	})
}

// TestAnnotateTableBoxes_PixelSpace verifies that boxes in pixel space
// (as from DLA-scaled coordinates) correctly match TSR cells. Regression test for Bug #1.
func TestAnnotateTableBoxes_PixelSpace(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 150, X1: 750, Top: 300, Bottom: 420, LayoutType: "table"},
	}
	cells := []pdf.TSRCell{
		{X0: 150, Y0: 300, X1: 750, Y1: 350, Label: "table column header"},
		{X0: 150, Y0: 350, X1: 750, Y1: 380, Label: "table row"},
		{X0: 150, Y0: 380, X1: 750, Y1: 420, Label: "table row"},
	}
	AnnotateTableBoxes(boxes, GroupTSRCellsToRows(cells))
	if boxes[0].R < 0 {
		t.Error("row index should be set (pixel-space matching)")
	}
	if boxes[0].H < 0 {
		t.Error("header index should be set")
	}
}

// TestFindHorizontallyTightestFit verifies the edge-distance matching
// (Python's minimum edge distance, not Go's old containment check).
func TestFindHorizontallyTightestFit(t *testing.T) {
	clmns := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50},
		{X0: 100, Y0: 0, X1: 200, Y1: 50},
	}

	t.Run("exact match left edge", func(t *testing.T) {
		box := pdf.TextBox{X0: 100, X1: 150, Top: 0, Bottom: 50}
		if idx := findHorizontallyTightestFit(box, clmns); idx != 1 {
			t.Errorf("box at col 1 left edge: got idx=%d, want 1", idx)
		}
	})

	t.Run("partial containment — still matches nearest", func(t *testing.T) {
		// Box mostly in col 0 but spills into col 1. Old containment check
		// would fail; distance check matches col 0 (closer edges).
		box := pdf.TextBox{X0: 80, X1: 120, Top: 0, Bottom: 50}
		if idx := findHorizontallyTightestFit(box, clmns); idx != 0 {
			t.Errorf("spill box: got idx=%d, want 0 (nearest edges)", idx)
		}
	})

	t.Run("empty columns", func(t *testing.T) {
		if idx := findHorizontallyTightestFit(pdf.TextBox{}, nil); idx != -1 {
			t.Errorf("empty: got %d, want -1", idx)
		}
	})
}

// TestFindOverlappedWithThreshold_BestMatch verifies the best-match
// (bidirectional overlap) replaces the old first-match behavior.
func TestFindOverlappedWithThreshold_BestMatch(t *testing.T) {
	// Two cells overlap the same box. Cell 1 has MORE overlap → should win.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 50, Y1: 50},   // 30% overlap
		{X0: 0, Y0: 0, X1: 100, Y1: 100}, // 100% overlap — best match
	}
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 100}
	if idx := findOverlappedWithThreshold(box, cells, 0.2); idx != 1 {
		t.Errorf("best-match: got idx=%d, want 1 (100%% overlap beats 30%%)", idx)
	}
}

// TestFindOverlappedWithThreshold_BidirectionalGate verifies that the gate
// uses max(boxRatio, cellRatio) — matching Python's bidirectional check.
// A large box that fully contains a tiny cell should match because the
// cell-perspective ratio is 1.0 (the cell is entirely inside the box).
// Python: max(overlap/boxArea, overlap/cellArea) = max(0.02, 1.0) = 1.0 ≥ 0.3 ✓
// Old Go (box-only gate):  overlap/boxArea = 0.02 > 0.3? → NO MATCH ✗
func TestFindOverlappedWithThreshold_BidirectionalGate(t *testing.T) {
	// Large box fully contains a tiny cell.
	box := pdf.TextBox{X0: 0, X1: 500, Top: 0, Bottom: 20} // area = 10000
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 10, Y1: 20}, // area = 200, entirely inside box
	}
	// boxRatio = 200/10000 = 0.02, cellRatio = 200/200 = 1.0
	// Python: max(0.02, 1.0) = 1.0 ≥ 0.3 → match!
	idx := findOverlappedWithThreshold(box, cells, 0.3)
	if idx != 0 {
		t.Errorf("bidirectional gate: cell fully inside large box should match (cellRatio=1.0 ≥ 0.3). got idx=%d, want 0", idx)
	}
}

// TestFindOverlappedWithThreshold_MaxScoring verifies that scoring uses
// max(boxRatio, cellRatio) — NOT sum.  Python picks the cell with the
// highest max(boxRatio, cellRatio).
//
// Cell A: boxRatio=0.60, cellRatio=0.05 → max=0.60, sum=0.65
// Cell B: boxRatio=0.40, cellRatio=0.40 → max=0.40, sum=0.80
// Python (max): picks A (0.60 > 0.40).  Old Go (sum): picks B (0.80 > 0.65).
func TestFindOverlappedWithThreshold_MaxScoring(t *testing.T) {
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 100} // area = 10000
	cells := []pdf.TSRCell{
		// Cell A: narrow but tall (60×2000), covers 60% of box width.
		// boxRatio=60*100/10000=0.60, cellRatio=60*100/(60*2000)=0.05, max=0.60
		{X0: 0, Y0: 0, X1: 60, Y1: 2000},
		// Cell B: moderate width (35×100), covers 35% of box. cellRatio=1.0.
		// boxRatio=35*100/10000=0.35, cellRatio=35*100/(35*100)=1.0, max=1.0
		// Hmm that gives cellRatio=1.0. Need to adjust for max=0.4 not 1.0.
		// Actually cell B should be: overlap/boxArea=0.35, overlap/cellArea=0.4.
		// overlap=3500, cellArea=3500/0.4=8750 → e.g., 35×250.
		{X0: 0, Y0: 0, X1: 35, Y1: 250},
	}
	// Cell A: overlap=6000, boxRatio=0.60, cellRatio=6000/120000=0.05, max=0.60
	// Cell B: overlap=3500, boxRatio=0.35, cellRatio=3500/8750=0.40, max=0.40
	// Python picks A (0.60 > 0.40). Old Go picks B (0.75 > 0.65).
	idx := findOverlappedWithThreshold(box, cells, 0.3)
	if idx != 0 {
		t.Errorf("max scoring: cell A (max=0.60) should beat cell B (max=0.40). got idx=%d, want 0 (Python uses max, not sum)", idx)
	}
}

// TestGroupTSRCellsToRowsLabeled_FallbackY verifies the fallback
// Y-based grouping path when all cells have label "table" (real
// DeepDoc HTTP API with wrong TSR model).  Must produce correct
// row×col structure even without row/column labels.
func TestGroupTSRCellsToRowsLabeled_FallbackY(t *testing.T) {
	// 4 rows × 5 cols = 20 cells, all label="table".
	cells := make([]pdf.TSRCell, 20)
	for r := 0; r < 4; r++ {
		for c := 0; c < 5; c++ {
			cells[r*5+c] = pdf.TSRCell{
				X0: float64(c * 100), Y0: float64(r * 30),
				X1: float64(c*100 + 80), Y1: float64(r*30 + 25),
				Label: "table",
			}
		}
	}
	rows := GroupTSRCellsToRows(cells)
	if len(rows) != 4 {
		t.Fatalf("fallback Y-grouping: expected 4 rows, got %d", len(rows))
	}
	for i, row := range rows {
		if len(row) != 5 {
			t.Errorf("row %d: expected 5 columns, got %d", i, len(row))
		}
	}
	// Verify X-order within each row.
	for i, row := range rows {
		for j := 1; j < len(row); j++ {
			if row[j].X0 < row[j-1].X0 {
				t.Errorf("row %d: cells not sorted by X (cell %d at X=%.0f, cell %d at X=%.0f)",
					i, j-1, row[j-1].X0, j, row[j].X0)
			}
		}
	}
}

// TestGroupTSRCellsToRowsLabeled_Irregular verifies Y-grouping
// tolerates irregular cell layouts: overlapping rows, missing
// cells, varying sizes.  Real DeepDoc output is not always a
// clean 4×5 grid.
func TestGroupTSRCellsToRowsLabeled_Irregular(t *testing.T) {
	// Irregular layout: row 0 has 3 cells, row 1 has 5, row 2 has 2.
	// Cells within a row have slightly different Y (within threshold).
	// Basic Y-proximity grouping does not pad rows to equal column counts.
	cells := []pdf.TSRCell{
		// Row 0 — 3 cells at ~Y=0 (slightly staggered tops).
		{X0: 0, Y0: 0, X1: 80, Y1: 25, Label: "table"},
		{X0: 90, Y0: 2, X1: 170, Y1: 27, Label: "table"},
		{X0: 180, Y0: 1, X1: 260, Y1: 26, Label: "table"},
		// Row 1 — 5 cells at ~Y=30.
		{X0: 0, Y0: 30, X1: 80, Y1: 55, Label: "table"},
		{X0: 90, Y0: 31, X1: 170, Y1: 56, Label: "table"},
		{X0: 180, Y0: 30, X1: 260, Y1: 55, Label: "table"},
		{X0: 270, Y0: 32, X1: 350, Y1: 57, Label: "table"},
		{X0: 360, Y0: 30, X1: 440, Y1: 55, Label: "table"},
		// Row 2 — 2 cells at ~Y=60.
		{X0: 0, Y0: 60, X1: 80, Y1: 85, Label: "table"},
		{X0: 90, Y0: 61, X1: 170, Y1: 86, Label: "table"},
	}
	rows := GroupTSRCellsToRows(cells)
	if len(rows) != 3 {
		t.Fatalf("irregular: expected 3 rows, got %d", len(rows))
	}
	if len(rows[0]) != 3 {
		t.Errorf("row 0: expected 3 cols, got %d", len(rows[0]))
	}
	if len(rows[1]) != 5 {
		t.Errorf("row 1: expected 5 cols, got %d", len(rows[1]))
	}
	if len(rows[2]) != 2 {
		t.Errorf("row 2: expected 2 cols, got %d", len(rows[2]))
	}
}

// TestFillCellTextFromBoxes_PreservesTSRText verifies that
// fillCellTextFromBoxes only overwrites a cell when matching box
// text is found.  When no box overlaps the cell, the cell keeps
// its existing Text (from TSR or previous steps).
func TestFillCellTextFromBoxes_PreservesTSRText(t *testing.T) {
	// Cell already has text from TSR.  No box overlaps it.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "TSR-provided"},
	}
	boxes := []pdf.TextBox{
		{X0: 500, X1: 600, Top: 500, Bottom: 550, Text: "far away"},
	}
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "TSR-provided" {
		t.Errorf("TSR text overwritten: got %q, want 'TSR-provided'", cells[0].Text)
	}

	// Cell with TSR text, box covers >85% — should be overwritten.
	cells2 := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "TSR-provided"},
	}
	boxes2 := []pdf.TextBox{
		{X0: 1, X1: 99, Top: 1, Bottom: 49, Text: "box-text"},
	}
	FillCellTextFromBoxes(cells2, boxes2)
	if cells2[0].Text != "box-text" {
		t.Errorf("box text should override TSR text: got %q, want 'box-text'", cells2[0].Text)
	}
}

// TestFillCellTextFromBoxes_PartialOverlap verifies that when a cell
// has NO existing text, even a box with partial overlap (< 85% of box
// area inside the cell) fills the cell.  Simulates real DeepDoc TSR
// where cell boundaries are approximate and box coordinates may have
// slight offsets.  Regression test for qa.pdf SKIP_OCR empty cells.
func TestFillCellTextFromBoxes_PartialOverlap(t *testing.T) {
	// Empty cell (no TSR text).  Box only has ~55% of its area inside
	// the cell (spills across the boundary).  Python's 0.3 threshold
	// accepts this; Go's 0.85 rejects it → empty cell.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: ""},
	}
	boxes := []pdf.TextBox{
		// Box: 60% inside cell, 40% outside. Overlap ratio = 60%.
		{X0: 40, X1: 140, Top: 5, Bottom: 15, Text: "spill text"},
	}
	// Cell (0,0)-(100,50). Box (40,5)-(140,15).
	// Overlap: X=(40,100) Y=(5,15) → 60×10=600.
	// Box area: 100×10=1000. ratio = 600/1000 = 60%.
	// Old 85% threshold → rejected. Python's 0.3 → accepted.
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "spill text" {
		t.Errorf("partial overlap (<85%%) on empty cell should still fill: got %q, want 'spill text'", cells[0].Text)
	}
}

// TestGroupTSRCellsToRowsLabeled_ColumnAlignment verifies that basic
// Y-proximity grouping produces the correct row counts. Unlike the EE
// label-aware grouping, Y-proximity does not handle spanning cells
// specially — each cell is simply placed into its Y-based row.
func TestGroupTSRCellsToRowsLabeled_ColumnAlignment(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 200, Y1: 30, Label: "table spanning cell"},
		{X0: 200, Y0: 0, X1: 300, Y1: 30, Label: "table row"},
		{X0: 0, Y0: 30, X1: 100, Y1: 60, Label: "table row"},
		{X0: 100, Y0: 30, X1: 200, Y1: 60, Label: "table row"},
		{X0: 200, Y0: 30, X1: 300, Y1: 60, Label: "table row"},
	}
	rows := GroupTSRCellsToRows(cells)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	// Basic Y-proximity: row0 has 2 cells, row1 has 3 cells.
	// No column alignment padding (EE feature).
	if len(rows[0]) != 2 {
		t.Errorf("row0: expected 2 cells, got %d", len(rows[0]))
	}
	if len(rows[1]) != 3 {
		t.Errorf("row1: expected 3 cells, got %d", len(rows[1]))
	}
}

// TestAnnotateTableBoxes_RealTSRLabels verifies that annotateTableBoxes
// assigns correct R/C annotations with real TSR labels ("table" + "table column").
// Python assigns R/C by spatial overlap, independent of label.
func TestAnnotateTableBoxes_RealTSRLabels(t *testing.T) {
	// Simulate a 2×3 table: 2 rows, 3 columns.
	// TSR cells with label "table" (default TSR class 0) — like 公司差旅费.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Label: "table"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Label: "table"},
		{X0: 201, Y0: 0, X1: 300, Y1: 30, Label: "table"},
		{X0: 0, Y0: 35, X1: 100, Y1: 65, Label: "table"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Label: "table"},
		{X0: 201, Y0: 35, X1: 300, Y1: 65, Label: "table"},
	}
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", LayoutType: "table"},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", LayoutType: "table"},
		{X0: 210, X1: 290, Top: 0, Bottom: 30, Text: "C", LayoutType: "table"},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "D", LayoutType: "table"},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "E", LayoutType: "table"},
		{X0: 210, X1: 290, Top: 35, Bottom: 65, Text: "F", LayoutType: "table"},
	}
	AnnotateTableBoxes(boxes, GroupTSRCellsToRows(cells))

	// Verify R (row) assignments — should be 0 for top row, 1 for bottom row.
	for i, b := range boxes {
		expectedR := i / 3
		if b.R != expectedR {
			t.Errorf("box[%d] %q: R=%d, want %d", i, b.Text, b.R, expectedR)
		}
	}
	// Verify C (column) assignments — 0,1,2 within each row.
	for i, b := range boxes {
		expectedC := i % 3
		if b.C != expectedC {
			t.Errorf("box[%d] %q: C=%d, want %d", i, b.Text, b.C, expectedC)
		}
	}
}

// TestTsrBoxOverlap_ReturnsTrueWhenDisjoint verifies that tsrBoxOverlap
// returns true when the box and cell do NOT overlap (are separated in
// at least one dimension).  Despite the name "Overlap", the function
// tests for disjointness.  All callers must negate it to check for
// actual overlap.  This test locks in the semantics so future readers
// and static analysis tools can rely on the behaviour.
func TestTsrBoxOverlap_ReturnsTrueWhenDisjoint(t *testing.T) {
	box := pdf.TextBox{X0: 50, X1: 100, Top: 0, Bottom: 50}

	// Separated in X (cell to the right) → disjoint → true.
	if !tsrBoxOverlap(box, pdf.TSRCell{X0: 150, Y0: 0, X1: 200, Y1: 50}) {
		t.Error("cell to the right (separated in X): expected true")
	}
	// Separated in X (cell to the left) → disjoint → true.
	if !tsrBoxOverlap(box, pdf.TSRCell{X0: 0, Y0: 0, X1: 30, Y1: 50}) {
		t.Error("cell to the left (separated in X): expected true")
	}
	// Separated in Y (cell below) → disjoint → true.
	if !tsrBoxOverlap(box, pdf.TSRCell{X0: 50, Y0: 100, X1: 100, Y1: 150}) {
		t.Error("cell below (separated in Y): expected true")
	}
	// Separated in Y (cell above) → disjoint → true.
	if !tsrBoxOverlap(box, pdf.TSRCell{X0: 50, Y0: -50, X1: 100, Y1: -10}) {
		t.Error("cell above (separated in Y): expected true")
	}
	// Fully enclosing cell → overlaps in both X and Y → NOT disjoint → false.
	if tsrBoxOverlap(box, pdf.TSRCell{X0: 0, Y0: 0, X1: 200, Y1: 100}) {
		t.Error("cell fully enclosing box (overlaps): expected false")
	}
	// Partially overlapping cell → overlaps in both dims → false.
	if tsrBoxOverlap(box, pdf.TSRCell{X0: 25, Y0: 25, X1: 75, Y1: 75}) {
		t.Error("cell partially overlapping: expected false")
	}
}
