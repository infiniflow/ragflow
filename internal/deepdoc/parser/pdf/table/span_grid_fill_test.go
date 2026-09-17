package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// spanGridFixture builds a 3×3 TSR grid with a spanning cell whose bbox
// covers rows 0-1 × cols 0-1 and whose top edge (0.3) sits a fraction of a
// pixel above the other cells of its own grid row (row 0 starts at 0).
//
// This mirrors the real-PDF failure (dell/prodeploy/screenshot parity):
// GroupCells extends the span cell's bbox across the covered region, so its
// Y0 differs from the sibling cells' Y0. The Y0-based row-banding in
// FillCellTextFromBoxesWithRows then splits the span cell into its own row
// band; because that band spans several TSR rows, the top-containment rule
// swallows every box in the covered region into the span cell, emptying the
// real rows (which are then dropped by orphan-row cleanup: 11→6 rows).
func spanGridFixture() (grid [][]pdf.TSRCell, boxes []pdf.TextBox, rowStrips []pdf.TSRCell) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 150, Y1: 120, Label: "table"},
		// rows
		{X0: 0, Y0: 0, X1: 150, Y1: 40, Label: "table row"},   // r0
		{X0: 0, Y0: 40, X1: 150, Y1: 80, Label: "table row"},  // r1
		{X0: 0, Y0: 80, X1: 150, Y1: 120, Label: "table row"}, // r2
		// cols
		{X0: 0, Y0: 0, X1: 50, Y1: 120, Label: "table column"},    // c0
		{X0: 50, Y0: 0, X1: 100, Y1: 120, Label: "table column"},  // c1
		{X0: 100, Y0: 0, X1: 150, Y1: 120, Label: "table column"}, // c2
		// spanning cell covering r0+r1 in c0+c1; top edge 0.3px above r0
		{X0: 0, Y0: 0.3, X1: 100, Y1: 80, Label: "table spanning cell"},
	}
	grid = (&DeepDocTableBuilder{}).GroupCells(cells)
	boxes = []pdf.TextBox{
		{X0: 5, X1: 30, Top: 5, Bottom: 35, Text: "A"},     // r0 c0 area
		{X0: 55, X1: 95, Top: 5, Bottom: 35, Text: "B"},    // r0 c1 area
		{X0: 5, X1: 30, Top: 45, Bottom: 75, Text: "C"},    // r1 c0 area
		{X0: 105, X1: 145, Top: 45, Bottom: 75, Text: "D"}, // r1 c2 area
		{X0: 5, X1: 30, Top: 85, Bottom: 115, Text: "E"},   // r2 c0 area
	}
	for _, c := range cells {
		if strings.HasSuffix(c.Label, "table row") {
			rowStrips = append(rowStrips, c)
		}
	}
	SortYFirstly(rowStrips, 10)
	return grid, boxes, rowStrips
}

// fillGrid copies flat cell text back onto the 2-D grid (same copy-back as
// table_extract.go processOneTable).
func fillGrid(grid [][]pdf.TSRCell, flat []pdf.TSRCell) {
	idx := 0
	for ri := range grid {
		for ci := range grid[ri] {
			grid[ri][ci].Text = flat[idx].Text
			idx++
		}
	}
}

// TestFillCellTextFromBoxes_SpanCellDoesNotSwallowOtherRows guards the
// primary span-grid failure: a spanning cell must NOT form its own row band
// (its bbox spans several TSR rows), so boxes in the covered rows stay in
// their own rows instead of being swallowed into the span cell.
func TestFillCellTextFromBoxes_SpanCellDoesNotSwallowOtherRows(t *testing.T) {
	grid, boxes, rowStrips := spanGridFixture()
	flat := FlattenGrid(grid)
	FillCellTextFromBoxesWithRows(flat, boxes, rowStrips)
	fillGrid(grid, flat)

	// r1 (covered by the span) keeps its own boxes.
	if got := grid[1][0].Text; got != "C" {
		t.Errorf("r1 c0 text = %q, want %q (box swallowed into span band)", got, "C")
	}
	if got := grid[1][2].Text; got != "D" {
		t.Errorf("r1 c2 text = %q, want %q (box swallowed into span band)", got, "D")
	}
	// r2 keeps its box.
	if got := grid[2][0].Text; got != "E" {
		t.Errorf("r2 c0 text = %q, want %q", got, "E")
	}
	// r0 c1 (covered by the span) keeps box B.
	if got := grid[0][1].Text; got != "B" {
		t.Errorf("r0 c1 text = %q, want %q", got, "B")
	}
}

// TestCalSpans_NoSpanningLabelNoCovered guards the regression where
// CalSpans computed cs/rs for EVERY grid cell: a normal (unlabeled) cell
// whose bbox happens to straddle the next column's center was misclassified
// as a span, and MarkCoveredCells then dropped its covered neighbours from
// the rendered rows — turning 4-column rows into 3 (公司差旅费管理办法
// regression). Only "table spanning cell" / "table header" labels may
// trigger coverage; Python's __cal_spans iterates SP/H-annotated boxes only.
func TestCalSpans_NoSpanningLabelNoCovered(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 200, Y1: 30, Text: "a", Label: "table row"}, // wide bbox: X covers col0 + col1 center
			{X0: 100, Y0: 0, X1: 200, Y1: 30, Text: "b", Label: "table row"},
			{X0: 200, Y0: 0, X1: 300, Y1: 30, Text: "c", Label: "table row"},
		},
		{
			{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "1", Label: "table row"},
			{X0: 100, Y0: 35, X1: 200, Y1: 65, Text: "2", Label: "table row"},
			{X0: 200, Y0: 35, X1: 300, Y1: 65, Text: "3", Label: "table row"},
		},
	}
	_, covered := CalSpans(rows)
	if len(covered) != 0 {
		t.Errorf("no spanning-labeled cell, expected 0 covered, got %v", covered)
	}
}

// TestConstructTable_SpanTableKeepsAllRows guards that the full assembly
// (GroupCells + fill + orphan cleanup) retains every TSR row of a
// span-bearing table. The old span handling emptied covered rows and dropped
// them (e.g. dell-configuration 11→6, screenshot 11→6 rows).
func TestConstructTable_SpanTableKeepsAllRows(t *testing.T) {
	grid, boxes, rowStrips := spanGridFixture()
	flat := FlattenGrid(grid)
	FillCellTextFromBoxesWithRows(flat, boxes, rowStrips)
	fillGrid(grid, flat)

	item := &pdf.TableItem{Grid: grid}
	ConstructTable(flat, boxes, "", item)
	if item.Grid == nil {
		t.Fatal("item.Grid is nil")
	}
	if len(item.Grid) != 3 {
		t.Errorf("expected 3 rows retained (every TSR row), got %d", len(item.Grid))
	}
	// Every row must contribute at least one non-empty cell.
	for ri, row := range item.Grid {
		has := false
		for _, c := range row {
			if strings.TrimSpace(c.Text) != "" {
				has = true
				break
			}
		}
		if !has {
			t.Errorf("row %d is empty after ConstructTable", ri)
		}
	}
}

// TestConstructTable_SpanCellFoldsCoveredText checks that the full assembly
// folds the covered cells' text into the span origin cell, matching Python's
// __cal_spans (the covered row boxes "C"/"D" end up inside the span cell's
// text rather than vanishing because their bboxes are no longer zeroed).
func TestConstructTable_SpanCellFoldsCoveredText(t *testing.T) {
	grid, boxes, rowStrips := spanGridFixture()
	flat := FlattenGrid(grid)
	FillCellTextFromBoxesWithRows(flat, boxes, rowStrips)
	fillGrid(grid, flat)

	item := &pdf.TableItem{Grid: grid}
	ConstructTable(flat, boxes, "", item)

	// span cell is r0 c0; its text must contain the covered region's boxes
	// (A=r0c0 self, B=r0c1 covered, C=r1c0 covered) but NOT boxes outside the
	// span (D=r1c2, E=r2c0).
	spanText := ""
	if len(item.Grid) > 0 && len(item.Grid[0]) > 0 {
		spanText = item.Grid[0][0].Text
	}
	for _, want := range []string{"A", "B", "C"} {
		if !strings.Contains(spanText, want) {
			t.Errorf("span cell text = %q, want it to contain %q (covered text folded)", spanText, want)
		}
	}
	for _, notWant := range []string{"D", "E"} {
		if strings.Contains(spanText, notWant) {
			t.Errorf("span cell text = %q, must NOT contain %q (outside covered region)", spanText, notWant)
		}
	}
}

// TestRowsToStrings_SkipsCovered guards that RowsToStrings drops cells marked
// "table covered", so item.Rows / anchor.Rows match Python's variable-width
// rendered rows instead of carrying phantom columns.
func TestRowsToStrings_SkipsCovered(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "a"},
			{X0: 100, Y0: 0, X1: 200, Y1: 30, Text: "b", Label: "table covered"},
			{X0: 200, Y0: 0, X1: 300, Y1: 30, Text: "c"},
		},
		{
			{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "1"},
			{X0: 100, Y0: 35, X1: 200, Y1: 65, Text: "2"},
			{X0: 200, Y0: 35, X1: 300, Y1: 65, Text: "3"},
		},
	}
	got := RowsToStrings(rows)
	if len(got[0]) != 2 || got[0][0] != "a" || got[0][1] != "c" {
		t.Errorf("row 0 = %v, want [a c] (covered cell dropped)", got[0])
	}
	if len(got[1]) != 3 || got[1][2] != "3" {
		t.Errorf("row 1 = %v, want [1 2 3] (no covered cell)", got[1])
	}
}
