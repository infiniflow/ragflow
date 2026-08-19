package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestCalSpans_NonSpanningCellsNotPolluted(t *testing.T) {
	// Simulate the SpannedTable test grid: row 0 has Q1(regular), 部门开支汇总(span), Q2(regular)
	rows := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Q1", Label: "table row"},
			{X0: 0, Y0: 0, X1: 200, Y1: 30, Text: "部门开支汇总", Label: "table spanning cell"},
			{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "Q2", Label: "table row"},
		},
		{
			{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "100", Label: "table row"},
			{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "200", Label: "table row"},
		},
	}

	spans, covered := CalSpans(rows)

	// Q1 at [0,0] has X0=0, X1=100 which should only cover its own column.
	// It should NOT get a colspan.
	if s, ok := spans[[2]int{0, 0}]; ok {
		t.Errorf("Q1 at [0,0] should NOT have colspan, got %v. "+
			"Spanning cell at [0,1] polluted column boundaries", s)
	}

	// 部门开支汇总 at [0,1] has X0=0, X1=200 which DOES span columns 0 and 1.
	if s, ok := spans[[2]int{0, 1}]; !ok {
		t.Error("部门开支汇总 at [0,1] should have colspan=2 (covers X=0-200)")
	} else if s[0] != 2 {
		t.Errorf("部门开支汇总 colspan = %d, want 2", s[0])
	}

	// Q2 at [0,2] should be covered by the spanning cell (col 2 is within X=0-200).
	if !covered[[2]int{0, 2}] {
		t.Error("Q2 at [0,2] should be covered by spanning cell at [0,1]")
	}

	t.Logf("spans: %v, covered: %v", spans, covered)
}

// TestCalSpans_IgnoresZeroPositionPaddedCells guards against a regression
// where MergeTablesAcrossPages pads a continuation page's grid with
// zero-coordinate cells (X0=X1=Y0=Y1=0) to align per-page column counts.
// Those padding cells must not define column geometry: without the
// zero-position skip, the padded column's left boundary is dragged to the
// origin, its center lands inside the neighbouring column's X range, and the
// neighbour is falsely reported as spanning into the padded column.
func TestCalSpans_IgnoresZeroPositionPaddedCells(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "a"},
			{X0: 100, Y0: 0, X1: 200, Y1: 30, Text: "b"},
			{X0: 0, Y0: 0, X1: 0, Y1: 0, Text: ""}, // zero-position padding cell
		},
		{
			{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "c"},
			{X0: 100, Y0: 35, X1: 200, Y1: 65, Text: "d"},
			{X0: 200, Y0: 35, X1: 300, Y1: 65, Text: "e"},
		},
	}

	spans, _ := CalSpans(rows)

	// Column 1 (cell "b" at [0,1], X=100-200) must NOT span into the padded
	// column 2 just because the padding cell pulled column 2's center left.
	if s, ok := spans[[2]int{0, 1}]; ok {
		t.Errorf("cell [0,1] should NOT span into the zero-position padded column, got %v", s)
	}
	// Sanity: real adjacent columns keep their own geometry.
	if s, ok := spans[[2]int{0, 0}]; ok {
		t.Errorf("cell [0,0] should not span, got %v", s)
	}
}
