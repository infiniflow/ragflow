package layout

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// Synthetic fixtures regression test.
//
// This file is the CI-runnable guardrail for the column detector. Unlike
// TestAssignColumnCombined_Labeled (which needs the gitignored 70-page label
// sheet + 343MB corpus and is therefore skipped in CI), these fixtures are
// hand-built []pdf.TextBox geometries committed to the repo, so they run on
// every `go test ./...` with no external data.
//
// Green assertions pin behavior that must NOT regress. The title-bridged
// double is a skipped TODO that captures the #18079 acceptance target: a 2D
// spatial column detector should split it into 2, but the current x0-based
// balance gate rejects it (sparse minority + x0 overlap).

// columnCount runs AssignColumn on one page of boxes and returns the number of
// distinct columns (max ColID + 1).
func columnCount(boxes []pdf.TextBox) int {
	in := make([]pdf.TextBox, len(boxes))
	copy(in, boxes)
	for i := range in {
		in[i].PageNumber = 0
	}
	res := AssignColumn(in)
	k := 1
	for _, b := range res {
		if b.ColID+1 > k {
			k = b.ColID + 1
		}
	}
	return k
}

// stackedColumn builds n text boxes in a vertical column at [x0,x1], starting
// at top0 with line spacing dy.
func stackedColumn(x0, x1, n int, top0, dy float64) []pdf.TextBox {
	boxes := make([]pdf.TextBox, n)
	for i := 0; i < n; i++ {
		top := top0 + float64(i)*dy
		boxes[i] = pdf.TextBox{X0: float64(x0), X1: float64(x1), Top: top, Bottom: top + 8}
	}
	return boxes
}

func concat(dst, src []pdf.TextBox) []pdf.TextBox { return append(dst, src...) }

// A single reading column must stay single (no over-split).
func TestSyntheticSingleColumn(t *testing.T) {
	boxes := stackedColumn(50, 240, 10, 10, 10)
	if got := columnCount(boxes); got != 1 {
		t.Errorf("single column: got %d, want 1", got)
	}
}

// A balanced two-column page must be recovered by the balance gate.
func TestSyntheticBalancedDouble(t *testing.T) {
	boxes := concat(stackedColumn(50, 240, 10, 10, 10), stackedColumn(270, 460, 10, 10, 10))
	if got := columnCount(boxes); got != 2 {
		t.Errorf("balanced double: got %d, want 2", got)
	}
}

// Narrow side-by-side columns are a table, not text columns: returned as 1.
func TestSyntheticTableNarrowColumns(t *testing.T) {
	var boxes []pdf.TextBox
	for _, c := range [][2]int{{50, 130}, {200, 280}, {350, 430}} {
		boxes = concat(boxes, stackedColumn(c[0], c[1], 5, 10, 10))
	}
	if got := columnCount(boxes); got != 1 {
		t.Errorf("narrow table: got %d, want 1", got)
	}
}

// A real left column plus a sparse right column: the balance gate must reject
// the sparse column (minority < 30%), keeping the page single. Guards the
// sparse-column prune (minColLineFrac) against regression.
func TestSyntheticSparseSecondColumn(t *testing.T) {
	boxes := concat(stackedColumn(50, 240, 30, 10, 10), stackedColumn(300, 460, 3, 10, 20))
	if got := columnCount(boxes); got != 1 {
		t.Errorf("sparse second column: got %d, want 1", got)
	}
}

// TODO #18079: a 2D spatial column detector should split this into 2.
//
// The page has a full-width title block at the top that bridges the gutter,
// and below it two side-by-side columns where the right column is sparse and
// its x0 overlaps the left column's x0 range. The x0-based balance gate
// therefore rejects it (minority < 30%, x0 overlap) and the page is reported
// as single. Skipped until #18079 lands; then unskip and assert got == 2.
func TestSyntheticTitleBridgedDouble(t *testing.T) {
	t.Skip("TODO #18079: title-bridged double should be detected as 2 columns")
	boxes := concat(
		concat(
			stackedColumn(50, 440, 3, 10, 10),  // full-width title (bridges the gutter)
			stackedColumn(50, 240, 30, 40, 10), // left column
		),
		stackedColumn(200, 440, 4, 40, 20), // right column: sparse, x0 overlaps left
	)
	if got := columnCount(boxes); got != 2 {
		t.Errorf("title-bridged double: got %d, want 2", got)
	}
}
