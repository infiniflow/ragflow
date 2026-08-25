package table

// These tests expose the row0 residual of real_pdfs/1.pdf tracked as
// table-1pdf-colspan-assembly-loss (known_diffs.json, status resolved):
// Go's header row keeps one extra empty cell (4 vs Python's 3) because
// cellPosFromBox ignores the span bounds (HLeft/HRight/HTop/HBott) copied onto
// a pure-SP box (H==0, SP>0) by AnnotateTableBoxes (PR #18707). CalSpans then
// covers only the columns whose centers fall inside the box's own narrow
// bounds, leaving the trailing covered column un-marked and alive as a 4th cell.
//
// This is the GroupBoxesByRC/CalSpans-level regression test CodeRabbit asked
// for in PR #18707 (the shipped tests only cover AnnotateTableBoxes field
// propagation, not the colspan geometry). It is a default unit test (no cgo
// tag) so it runs in `go test ./...` and CI.

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestGroupBoxesByRCSpanHeaderRowCellCount drives the real assembly path
// (GroupBoxesByRC -> CalSpans -> MarkCoveredCells -> RowsToStrings) with a
// synthetic 1.pdf-like header: 8 logical columns, a TSR "table spanning cell"
// covering 6 of them. Python's golden collapses the row to 3 cells (the 6-col
// span + 2 trailing single columns). Go must match.
func TestGroupBoxesByRCSpanHeaderRowCellCount(t *testing.T) {
	const nCols = 8
	boxes := make([]pdf.TextBox, 0, nCols)

	// Spanning header text box at col 0. Its OCR box is narrow [0,450], but the
	// TSR span component reaches the full extent [0,600]; AnnotateTableBoxes
	// copies that onto the box as HLeft/HRight/HTop/HBott. For a pure-SP box
	// (H==0, SP>0) cellPosFromBox must use those bounds, otherwise the span
	// cell falls back to [0,450] and covers only 5 columns.
	//
	// R starts at 1 (not 0): GroupBoxesByRC falls back to YX grouping when
	// maxR<=0, which would skip cellPosFromBox entirely; a real table has
	// multiple rows so maxR>0 and the R/C path runs.
	boxes = append(boxes, pdf.TextBox{
		X0: 0, X1: 450, Top: 0, Bottom: 20,
		Text: "HEADER", R: 1, C: 0,
		SP: 1, HLeft: 0, HRight: 600, HTop: 0, HBott: 20,
	})

	// Remaining columns 1..7 as header/content cells, defining column geometry.
	trailing := []string{"c1", "c2", "c3", "c4", "c5", "c6", "c7"}
	for c := 1; c < nCols; c++ {
		boxes = append(boxes, pdf.TextBox{
			X0: float64(c) * 100, X1: float64(c)*100 + 100, Top: 0, Bottom: 20,
			Text: trailing[c-1], R: 1, C: c,
			H: 1,
		})
	}
	// One body row (R=2) so the table has more than one row; its cells are
	// irrelevant to the header-row assertion.
	for c := 0; c < nCols; c++ {
		boxes = append(boxes, pdf.TextBox{
			X0: float64(c) * 100, X1: float64(c)*100 + 100, Top: 30, Bottom: 50,
			Text: "b", R: 2, C: c,
		})
	}

	rows := GroupBoxesByRC(boxes)
	if len(rows) == 0 {
		t.Fatal("GroupBoxesByRC returned an empty grid")
	}
	spanInfo, covered := CalSpans(rows)
	MarkCoveredCells(rows, covered)

	row0 := RowsToStrings(rows)[0]
	if len(row0) != 3 {
		t.Errorf("header row should collapse to 3 cells (6-col span + 2 trailing), "+
			"got %d: %v (spanInfo=%v)", len(row0), row0, spanInfo)
	}
	// Sanity: the span must actually be colspan=6, not a narrower span that
	// happens to leave 3 cells via a different arrangement.
	if cs, ok := spanInfo[[2]int{0, 0}]; !ok || cs[0] != 6 {
		t.Errorf("expected colspan=6 on the span origin (0,0), got %v (ok=%v)", cs, ok)
	}
}

// TestCellPosFromBoxSpanUsesSpanBounds pins the root cause directly: a pure-SP
// box (H==0, SP>0) with propagated span bounds must rebuild its cell from those
// bounds, not from the box's own narrow text bounds. HLeft/HRight (and
// HTop/HBott) are set together by AnnotateTableBoxes, so an axis with a zero
// edge is still propagated when the opposite edge is set — a real span edge at
// coordinate 0 must not be mistaken for "unset". The synthetic box uses a
// nonzero text box (X0=200) and a zero left/top propagated edge to lock this in.
func TestCellPosFromBoxSpanUsesSpanBounds(t *testing.T) {
	b := pdf.TextBox{
		X0: 200, X1: 300, Top: 0, Bottom: 20,
		H: 0, SP: 1, HLeft: 0, HRight: 600, HTop: 0, HBott: 25,
	}
	x0, y0, x1, y1, _ := cellPosFromBox(b)
	if x0 != 0 || x1 != 600 {
		t.Errorf("pure-SP box must use propagated span bounds even when HLeft=0 "+
			"(span edge at coord 0), got x0=%v x1=%v", x0, x1)
	}
	if y0 != 0 || y1 != 25 {
		t.Errorf("pure-SP box must use propagated span bounds even when HTop=0 "+
			"(span edge at coord 0), got y0=%v y1=%v", y0, y1)
	}
}
