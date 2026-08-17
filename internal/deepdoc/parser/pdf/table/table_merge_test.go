package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestCrossPageTableMerge(t *testing.T) {
	// Page 0 table: 2 cells, positioned at page 0.
	pg0 := pdf.TableItem{
		Positions: []pdf.Position{
			{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 800},
		},
		Scale: 1.0,
		Cells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "pg0_r0c0"},
			{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "pg0_r0c1"},
		},
	}
	// Page 1 table: 2 cells, same X range, positioned at page 1.
	pg1 := pdf.TableItem{
		Positions: []pdf.Position{
			{PageNumbers: []int{1}, Left: 50, Right: 500, Top: 100, Bottom: 300},
		},
		Scale: 1.0,
		Cells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "pg1_r0c0"},
			{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "pg1_r0c1"},
		},
	}
	tables := []pdf.TableItem{pg0, pg1}

	// mergeTablesAcrossPages merges tables on consecutive pages with X overlap.
	merged := MergeTablesAcrossPages(tables, nil)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	if len(merged[0].Cells) != 4 {
		t.Errorf("expected 4 merged cells, got %d", len(merged[0].Cells))
	}
	if len(merged[0].Positions) != 2 {
		t.Errorf("expected 2 merged positions, got %d", len(merged[0].Positions))
	}
	t.Logf("Merged %d cells across %d pages", len(merged[0].Cells), len(merged[0].Positions))
}

// TestMergeTablesAcrossPages_NoOverlap verifies that non-adjacent or
// non-overlapping tables are NOT merged.
func TestMergeTablesAcrossPages_NoOverlap(t *testing.T) {
	// Tables with no X overlap should NOT be merged.
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 50, Right: 100, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []pdf.TSRCell{{Text: "left"}},
		},
		{
			Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 500, Right: 600, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []pdf.TSRCell{{Text: "right"}},
		},
	}
	merged := MergeTablesAcrossPages(tables, nil)
	if len(merged) != 2 {
		t.Fatalf("non-overlapping tables: expected 2 tables, got %d", len(merged))
	}
}

// TestMergeTablesAcrossPages_NonConsecutive verifies that tables on
// non-consecutive pages are NOT merged.
func TestMergeTablesAcrossPages_NonConsecutive(t *testing.T) {
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []pdf.TSRCell{{Text: "page0"}},
		},
		{
			Positions: []pdf.Position{{PageNumbers: []int{3}, Left: 50, Right: 500, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []pdf.TSRCell{{Text: "page3"}},
		},
	}
	merged := MergeTablesAcrossPages(tables, nil)
	if len(merged) != 2 {
		t.Fatalf("non-consecutive pages: expected 2 tables, got %d", len(merged))
	}
}

// TestMergeTablesAcrossPages_SingleTable verifies that a single table
// passes through unchanged.
func TestMergeTablesAcrossPages_SingleTable(t *testing.T) {
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []pdf.TSRCell{{Text: "only"}},
		},
	}
	merged := MergeTablesAcrossPages(tables, nil)
	if len(merged) != 1 {
		t.Fatalf("single table: expected 1 table, got %d", len(merged))
	}
}

func TestMergeTablesAcrossPages_EmptyPositions(t *testing.T) {
	// Tables with empty Positions should be preserved (not dropped).
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{},
			Cells:     []pdf.TSRCell{{Text: "posless"}},
		},
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 500}},
			Scale:     1.0,
			Cells:     []pdf.TSRCell{{Text: "normal"}},
		},
	}
	merged := MergeTablesAcrossPages(tables, nil)
	if len(merged) != 2 {
		t.Fatalf("empty Positions: expected 2 tables (preserved), got %d", len(merged))
	}
	// Tables with Positions come first (from items list), positionless tables are appended.
	if len(merged[0].Positions) == 0 {
		t.Error("expected table with Positions first in result")
	}
	if len(merged[1].Positions) != 0 {
		t.Error("expected positionless table second in result")
	}
	if merged[1].Cells[0].Text != "posless" {
		t.Errorf("positionless table content lost: got %q", merged[1].Cells[0].Text)
	}
}

func TestMergeTablesAcrossPages_LargeYGap(t *testing.T) {
	// Tables with large Y gap should NOT be merged.
	medianHeights := map[int]float64{0: 10}
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 150}},
			Scale:     1.0,
			Cells:     []pdf.TSRCell{{Text: "page0"}},
		},
		{
			Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 50, Right: 500, Top: 5000, Bottom: 5100}},
			Scale:     1.0,
			Cells:     []pdf.TSRCell{{Text: "page1_far"}},
		},
	}
	merged := MergeTablesAcrossPages(tables, medianHeights)
	if len(merged) != 2 {
		t.Fatalf("large Y gap: expected 2 tables (not merged), got %d", len(merged))
	}
}

func TestMergeTablesAcrossPages_NoMedianHeights(t *testing.T) {
	// Without medianHeights, mh defaults to 10 and threshold is 230.
	// yDis = (10 + 120 - 150 - 150) / 2 = -85, which is <= 230, so they merge.
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 50, Right: 500, Top: 100, Bottom: 150}},
			Scale:     1.0,
			Cells:     []pdf.TSRCell{{Text: "page0"}},
		},
		{
			Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 50, Right: 500, Top: 10, Bottom: 120}},
			Scale:     1.0,
			Cells:     []pdf.TSRCell{{Text: "page1_near"}},
		},
	}
	merged := MergeTablesAcrossPages(tables, nil)
	if len(merged) != 1 {
		t.Fatalf("no medianHeights: expected 1 merged table, got %d", len(merged))
	}
	if len(merged[0].Cells) != 2 {
		t.Errorf("expected 2 cells after merge, got %d", len(merged[0].Cells))
	}
}
