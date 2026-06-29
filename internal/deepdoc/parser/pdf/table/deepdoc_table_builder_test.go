package table

import (
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"strings"
	"testing"
)

func TestDeepDocTableBuildService_GroupCells_Basic4x5(t *testing.T) {
	b := &DeepDocTableBuilder{}

	cells := buildOSSCells(4, 5, 0, 0, 500, 200)
	grid := b.GroupCells(cells)

	if len(grid) != 4 {
		t.Fatalf("expected 4 rows, got %d", len(grid))
	}
	for i, row := range grid {
		if len(row) != 5 {
			t.Fatalf("row %d: expected 5 cols, got %d", i, len(row))
		}
	}
}

func TestDeepDocTableBuildService_GroupCells_Coords(t *testing.T) {
	b := &DeepDocTableBuilder{}

	cells := buildOSSCells(2, 2, 0, 0, 200, 100)
	grid := b.GroupCells(cells)

	// grid[0][0] = row[0] × col[0]
	if grid[0][0].X0 != 0 || grid[0][0].Y0 != 0 {
		t.Errorf("grid[0][0] pos: got (%.0f,%.0f), want (0,0)", grid[0][0].X0, grid[0][0].Y0)
	}
	if grid[0][0].X1 != 100 || grid[0][0].Y1 != 50 {
		t.Errorf("grid[0][0] size: got (%.0f,%.0f), want (100,50)", grid[0][0].X1, grid[0][0].Y1)
	}

	// grid[1][1] = row[1] × col[1]
	if grid[1][1].X0 != 100 || grid[1][1].Y0 != 50 {
		t.Errorf("grid[1][1] pos: got (%.0f,%.0f), want (100,50)", grid[1][1].X0, grid[1][1].Y0)
	}
	if grid[1][1].X1 != 200 || grid[1][1].Y1 != 100 {
		t.Errorf("grid[1][1] size: got (%.0f,%.0f), want (200,100)", grid[1][1].X1, grid[1][1].Y1)
	}
}

func TestDeepDocTableBuildService_GroupCells_HeaderPropagation(t *testing.T) {
	b := &DeepDocTableBuilder{}

	// 3 rows: header(Y=0-50) should map to row 0
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 200, Y1: 150, Label: "table"},
		{X0: 0, Y0: 0, X1: 200, Y1: 50, Label: "table row"},
		{X0: 0, Y0: 50, X1: 200, Y1: 100, Label: "table row"},
		{X0: 0, Y0: 100, X1: 200, Y1: 150, Label: "table row"},
		{X0: 0, Y0: 0, X1: 100, Y1: 150, Label: "table column"},
		{X0: 100, Y0: 0, X1: 200, Y1: 150, Label: "table column"},
		{X0: 0, Y0: 0, X1: 200, Y1: 50, Label: "table column header"},
	}

	grid := b.GroupCells(cells)
	if len(grid) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(grid))
	}

	// Row 0 should have header labels.
	for c := range grid[0] {
		if grid[0][c].Label != "table column header" {
			t.Errorf("grid[0][%d].Label = %q, want 'table column header'", c, grid[0][c].Label)
		}
	}

	// Row 1 should have empty labels (data rows).
	for c := range grid[1] {
		if grid[1][c].Label != "" {
			t.Errorf("grid[1][%d].Label = %q, want empty", c, grid[1][c].Label)
		}
	}
}

func TestDeepDocTableBuildService_GroupCells_SpanInjection(t *testing.T) {
	b := &DeepDocTableBuilder{}

	// 2×3 table, spanning cell covers cols 0-1 in row 0
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 300, Y1: 100, Label: "table"},
		{X0: 0, Y0: 0, X1: 300, Y1: 50, Label: "table row"},
		{X0: 0, Y0: 50, X1: 300, Y1: 100, Label: "table row"},
		{X0: 0, Y0: 0, X1: 100, Y1: 100, Label: "table column"},
		{X0: 100, Y0: 0, X1: 200, Y1: 100, Label: "table column"},
		{X0: 200, Y0: 0, X1: 300, Y1: 100, Label: "table column"},
		{X0: 0, Y0: 0, X1: 200, Y1: 50, Label: "table spanning cell"},
	}

	grid := b.GroupCells(cells)
	if len(grid) != 2 || len(grid[0]) != 3 {
		t.Fatalf("expected 2×3 grid, got %d×%d", len(grid), len(grid[0]))
	}

	// The spanning cell at [0,0] should have Label "table spanning cell"
	// and its bbox should cover the full span (X=0-200).
	spanCell := grid[0][0]
	if !strings.Contains(strings.ToLower(spanCell.Label), "spanning") {
		t.Errorf("grid[0][0].Label = %q, want label containing 'spanning'", spanCell.Label)
	}
	if spanCell.X0 != 0 || spanCell.X1 != 200 {
		t.Errorf("grid[0][0] X range = (%.0f,%.0f), want (0,200)", spanCell.X0, spanCell.X1)
	}

	// grid[0][1] should be covered (bbox zeroed).
	if !isZeroCell(grid[0][1]) {
		t.Errorf("grid[0][1] should be covered (zero bbox), got (%.0f,%.0f,%.0f,%.0f)",
			grid[0][1].X0, grid[0][1].Y0, grid[0][1].X1, grid[0][1].Y1)
	}

	// grid[0][2] should be normal (not covered by span).
	if isZeroCell(grid[0][2]) {
		t.Error("grid[0][2] should NOT be covered")
	}
}

func TestDeepDocTableBuildService_GroupCells_IrregularSize(t *testing.T) {
	b := &DeepDocTableBuilder{}
	cells := buildOSSCells(3, 2, 0, 0, 200, 120)
	grid := b.GroupCells(cells)

	if len(grid) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(grid))
	}
	if len(grid[0]) != 2 {
		t.Fatalf("expected 2 cols, got %d", len(grid[0]))
	}
}

func TestDeepDocTableBuildService_GroupCells_EmptyInput(t *testing.T) {
	b := &DeepDocTableBuilder{}
	grid := b.GroupCells(nil)
	if len(grid) != 0 {
		t.Errorf("expected empty grid, got %d rows", len(grid))
	}
}

func TestDeepDocTableBuildService_GroupCells_NoRows(t *testing.T) {
	b := &DeepDocTableBuilder{}
	// Only a "table" cell, no row cells.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 500, Y1: 200, Label: "table"},
	}
	grid := b.GroupCells(cells)
	if len(grid) != 0 {
		t.Errorf("expected empty grid without row cells, got %d rows", len(grid))
	}
}

func TestDeepDocTableBuildService_GroupCells_NoColumns(t *testing.T) {
	b := &DeepDocTableBuilder{}
	// Table + rows but no column cells → each row gets 1 wide column.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 500, Y1: 100, Label: "table"},
		{X0: 0, Y0: 0, X1: 500, Y1: 50, Label: "table row"},
		{X0: 0, Y0: 50, X1: 500, Y1: 100, Label: "table row"},
	}
	grid := b.GroupCells(cells)
	if len(grid) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(grid))
	}
	if len(grid[0]) != 1 {
		t.Errorf("expected 1 col (default wide column), got %d", len(grid[0]))
	}
}

// ── helpers ──────────────────────────────────────────────────────────

// buildOSSCells constructs a set of OSS-style structural cells for
// an R×C table with the given overall bounding box.
func buildOSSCells(rows, cols int, x0, y0, x1, y1 float64) []pdf.TSRCell {
	rowH := (y1 - y0) / float64(rows)
	colW := (x1 - x0) / float64(cols)

	cells := []pdf.TSRCell{
		{X0: x0, Y0: y0, X1: x1, Y1: y1, Label: "table"},
	}

	for r := 0; r < rows; r++ {
		cells = append(cells, pdf.TSRCell{
			X0: x0, Y0: y0 + float64(r)*rowH,
			X1: x1, Y1: y0 + float64(r+1)*rowH,
			Label: "table row",
		})
	}
	for c := 0; c < cols; c++ {
		cells = append(cells, pdf.TSRCell{
			X0: x0 + float64(c)*colW, Y0: y0,
			X1: x0 + float64(c+1)*colW, Y1: y1,
			Label: "table column",
		})
	}

	return cells
}

// isZeroCell reports whether a cell has its bbox zeroed (covered by a span).
func isZeroCell(c pdf.TSRCell) bool {
	return c.X0 == 0 && c.Y0 == 0 && c.X1 == 0 && c.Y1 == 0
}

// hasLabel reports whether any cell in a row has a label containing substr.
func hasLabel(row []pdf.TSRCell, substr string) bool {
	for _, c := range row {
		if strings.Contains(strings.ToLower(c.Label), strings.ToLower(substr)) {
			return true
		}
	}
	return false
}
