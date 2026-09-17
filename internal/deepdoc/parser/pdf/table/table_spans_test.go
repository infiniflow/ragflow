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
	// It must NOT become a span origin — only SP/`spanning` cells may span.
	if s, ok := spans[[2]int{0, 0}]; ok {
		t.Errorf("Q1 at [0,0] should NOT have colspan, got %v. "+
			"Spanning cell at [0,1] polluted column boundaries", s)
	}

	// 部门开支汇总 at [0,1] has X0=0, X1=200, so by Python's __cal_spans
	// (which checks every column centre against the SP box's X range, both
	// directions) it spans columns 0, 1 and 2: colCentre[0]=50 and
	// colCentre[2]=150.5 both fall inside [0,200]. colspan == 3.
	if s, ok := spans[[2]int{0, 1}]; !ok {
		t.Error("部门开支汇总 at [0,1] should have a colspan (covers X=0-200)")
	} else if s[0] != 3 {
		t.Errorf("部门开支汇总 colspan = %d, want 3", s[0])
	}

	// Both neighbours are covered by the spanning cell.
	if !covered[[2]int{0, 0}] {
		t.Error("Q1 at [0,0] should be covered by spanning cell at [0,1]")
	}
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

// TestCalSpans_HeaderCellNeverSpans pins the icbccs cross-page-merge fix at
// the CalSpans unit level (the end-to-end case lives in the manual
// crosspage_merge_test.go, which CI never runs). A "table header" cell whose
// bbox is wide enough to cover the neighbour column's centre must NOT become
// a colspan: Python's __cal_spans only spans boxes carrying "SP", never a
// plain header. The old Go branch did span such headers, which is exactly how
// icbccs "类型" (X1 widened by MergeTablesAcrossPages) falsely absorbed
// "必选".
func TestCalSpans_HeaderCellNeverSpans(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{
			// "类型": a header whose X1=260 reaches past "必选"'s column
			// centre (255), so the OLD header branch would have spanned it.
			{X0: 10, Y0: 0, X1: 260, Y1: 30, Text: "类型", Label: "table header"},
			{X0: 210, Y0: 0, X1: 300, Y1: 30, Text: "必选", Label: "table header"},
		},
	}

	spans, covered := CalSpans(rows)

	if s, ok := spans[[2]int{0, 0}]; ok {
		t.Errorf("header [0,0] must NOT span, got %v", s)
	}
	if covered[[2]int{0, 1}] {
		t.Error("neighbour [0,1] must NOT be covered by a header cell")
	}
	// Each header keeps its own text — nothing is folded across.
	if rows[0][0].Text != "类型" || rows[0][1].Text != "必选" {
		t.Errorf("header text must stay separate, got %q / %q",
			rows[0][0].Text, rows[0][1].Text)
	}
}

// TestCalSpans_RightwardSpan locks that a genuine SP/`spanning` cell still
// spans to the RIGHT after the loop was rewritten to scan columns on both
// sides of the origin (matching Python's both-direction __cal_spans).
func TestCalSpans_RightwardSpan(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{
			// SP cell "A" at col 0 whose X1=260 reaches past "B"'s column
			// centre (255), so it spans rightward onto col 1.
			{X0: 10, Y0: 0, X1: 260, Y1: 30, Text: "A", Label: "table spanning cell"},
			{X0: 210, Y0: 0, X1: 300, Y1: 30, Text: "B", Label: "table row"},
		},
	}
	spans, covered := CalSpans(rows)
	s, ok := spans[[2]int{0, 0}]
	if !ok {
		t.Fatal("SP cell [0,0] should span rightward to cover [0,1]")
	}
	if s[0] != 2 {
		t.Errorf("colspan = %d, want 2", s[0])
	}
	if !covered[[2]int{0, 1}] {
		t.Error("[0,1] should be covered by the rightward span")
	}
}
