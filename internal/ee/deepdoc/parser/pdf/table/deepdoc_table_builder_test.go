package table

import (
	"strings"
	"testing"

	pdft "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestEEService_GroupCells(t *testing.T) {
	b := &DeepDocTableBuilder{}

	t.Run("labels group into rows", func(t *testing.T) {
		cells := []pdft.TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "H1", Label: "table column header"},
			{X0: 100, Y0: 0, X1: 200, Y1: 30, Text: "H2", Label: "table column header"},
			{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "A1", Label: "table row"},
			{X0: 100, Y0: 35, X1: 200, Y1: 65, Text: "B1", Label: "table row"},
			{X0: 0, Y0: 70, X1: 100, Y1: 100, Text: "A2", Label: "table row"},
			{X0: 100, Y0: 70, X1: 200, Y1: 100, Text: "B2", Label: "table row"},
		}
		grid := b.GroupCells(cells)
		if len(grid) != 3 {
			t.Fatalf("expected 3 rows, got %d", len(grid))
		}
		if len(grid[0]) != 2 || len(grid[1]) != 2 || len(grid[2]) != 2 {
			t.Errorf("expected 2 cols per row, got %d/%d/%d",
				len(grid[0]), len(grid[1]), len(grid[2]))
		}
	})

	t.Run("spanning cell added to row", func(t *testing.T) {
		cells := []pdft.TSRCell{
			{X0: 0, Y0: 0, X1: 200, Y1: 30, Text: "H1", Label: "table column header"},
			{X0: 100, Y0: 0, X1: 200, Y1: 30, Text: "H2", Label: "table column header"},
			{X0: 0, Y0: 0, X1: 200, Y1: 30, Text: "Span", Label: "table spanning cell"},
			{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "D1", Label: "table row"},
			{X0: 100, Y0: 35, X1: 200, Y1: 65, Text: "D2", Label: "table row"},
		}
		grid := b.GroupCells(cells)
		if len(grid) != 2 {
			t.Fatalf("expected 2 rows (header + data), got %d", len(grid))
		}
		if len(grid[0]) < 3 {
			t.Errorf("expected row 0 to contain 2 headers + spanning = 3 cells, got %d", len(grid[0]))
		}
	})

	t.Run("fallback to Y-proximity when no labels match", func(t *testing.T) {
		cells := []pdft.TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "C1", Label: "unknown"},
			{X0: 100, Y0: 0, X1: 200, Y1: 30, Text: "C2", Label: "unknown"},
			{X0: 0, Y0: 50, X1: 100, Y1: 80, Text: "D1", Label: "unknown"},
			{X0: 100, Y0: 50, X1: 200, Y1: 80, Text: "D2", Label: "unknown"},
		}
		grid := b.GroupCells(cells)
		if len(grid) != 2 {
			t.Fatalf("expected 2 rows from Y-proximity fallback, got %d", len(grid))
		}
		if len(grid[0]) != 2 || len(grid[1]) != 2 {
			t.Errorf("expected 2 cols per row, got %d/%d", len(grid[0]), len(grid[1]))
		}
	})
}

func TestEEService_Name(t *testing.T) {
	b := &DeepDocTableBuilder{}
	if b.Name() != "deepdoc" {
		t.Errorf("expected 'deepdoc', got %q", b.Name())
	}
}

func TestGatherTSR(t *testing.T) {
	cells := []pdft.TSRCell{
		{Label: "table row", Text: "A"},
		{Label: "table column header", Text: "H"},
		{Label: "table row", Text: "B"},
	}
	result := gatherTSR(cells, reRowHdr)
	if len(result) < 2 {
		t.Errorf("expected at least 2 matching cells, got %d", len(result))
	}
	for _, c := range result {
		if !strings.Contains("ABH", c.Text[:1]) {
			t.Errorf("unexpected cell in result: %+v", c)
		}
	}
}

func TestGroupTSRCellsToRowsLabeled_NoZeroHeightPhantomCells(t *testing.T) {
	// Row0: 1 row cell + 1 spanning cell → 2 cells.
	// Row1: 1 row cell → 1 cell.  maxCols=2 → Row1 padded.
	// The padded cell must have valid height from the real cell.
	cells := []pdft.TSRCell{
		{Label: "table row", X0: 0, Y0: 0, X1: 100, Y1: 20},
		{Label: "table spanning cell", X0: 120, Y0: 0, X1: 200, Y1: 20},
		{Label: "table row", X0: 0, Y0: 100, X1: 100, Y1: 120},
	}
	result := groupTSRCellsToRowsLabeled(cells)
	if len(result) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(result))
	}
	if len(result[0]) != 2 {
		t.Fatalf("row 0: expected 2 cells, got %d", len(result[0]))
	}
	if len(result[1]) != 2 {
		t.Fatalf("row 1: expected 2 cells (padded), got %d", len(result[1]))
	}
	phantom := result[1][1]
	if phantom.Y1 <= phantom.Y0 {
		t.Errorf("phantom cell has zero height: Y0=%v Y1=%v", phantom.Y0, phantom.Y1)
	}
}
