//go:build cgo && manual

package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestGroupCellsInjectsSpanning verifies that GroupCells propagates EVERY TSR
// "table spanning cell" label into the assembled grid — both the wide col-span
// header and the tall row-span header cell — instead of dropping a span whose
// box only reaches one row/column center.
//
// Repro from real_pdfs/1.pdf table 0 (tsr_raw coordinates, raw PDF points —
// GroupCells only compares relative positions, so no crop/scale mapping is
// needed here):
//   - colspan spanning cell #17: x0=209.3 y0=112.2 x1=446.8 y1=138.0
//   - rowspan spanning cell #18: x0=210.3 y0=135.3 x1=249.7 y1=229.7
//
// The real PDF cell "名称" spans TWO rows (rowspan=2); TSR only emits a span
// box that reaches row1's center (rowspan=1) — a known TSR-model limitation
// tracked separately. Go must still faithfully label BOTH TSR span boxes so
// the recognition is not lost; restoring the true rowspan=2 is the TSR model's
// job, not Go's. The previous code used `len(covered) < 2` to skip a span that
// covered only one cell, which dropped #18 entirely.
func TestGroupCellsInjectsSpanning(t *testing.T) {
	cells := []pdf.TSRCell{
		// 8 columns
		{X0: 422.5, Y0: 125.0, X1: 465.1, Y1: 727.3, Label: "table column"},
		{X0: 465.2, Y0: 125.0, X1: 503.1, Y1: 724.6, Label: "table column"},
		{X0: 81.1, Y0: 110.4, X1: 212.6, Y1: 726.2, Label: "table column"},
		{X0: 376.8, Y0: 125.0, X1: 422.4, Y1: 724.6, Label: "table column"},
		{X0: 287.2, Y0: 121.7, X1: 331.3, Y1: 724.6, Label: "table column"},
		{X0: 331.4, Y0: 125.0, X1: 376.9, Y1: 724.6, Label: "table column"},
		{X0: 249.4, Y0: 112.8, X1: 286.1, Y1: 726.3, Label: "table column"},
		{X0: 211.5, Y0: 113.2, X1: 249.5, Y1: 724.6, Label: "table column"},
		// 7 rows
		{X0: 80.3, Y0: 138.3, X1: 502.2, Y1: 231.4, Label: "table row"},
		{X0: 80.3, Y0: 290.8, X1: 502.4, Y1: 391.1, Label: "table row"},
		{X0: 79.9, Y0: 484.3, X1: 501.7, Y1: 576.8, Label: "table row"},
		{X0: 80.0, Y0: 578.1, X1: 501.7, Y1: 726.9, Label: "table row"},
		{X0: 80.3, Y0: 230.3, X1: 501.8, Y1: 290.6, Label: "table row"},
		{X0: 80.3, Y0: 111.4, X1: 503.0, Y1: 138.9, Label: "table row"},
		{X0: 80.0, Y0: 390.2, X1: 501.7, Y1: 484.3, Label: "table row"},
		// column header region
		{X0: 79.1, Y0: 110.9, X1: 503.6, Y1: 231.0, Label: "table column header"},
		// spanning cells (both must be labelled in the grid)
		{X0: 209.3, Y0: 112.2, X1: 446.8, Y1: 138.0, Label: "table spanning cell"},
		{X0: 210.3, Y0: 135.3, X1: 249.7, Y1: 229.7, Label: "table spanning cell"},
	}

	grid := (&DeepDocTableBuilder{}).GroupCells(cells)
	if grid == nil {
		t.Fatal("GroupCells returned nil grid")
	}

	spanCount := 0
	var spanPositions [][2]int
	for r, row := range grid {
		for c, cell := range row {
			if strings.Contains(cell.Label, "spanning") {
				spanCount++
				spanPositions = append(spanPositions, [2]int{r, c})
			}
		}
	}
	t.Logf("grid %dx%d, spanning cells injected: %d at %v",
		len(grid), len(grid[0]), spanCount, spanPositions)

	if spanCount != 2 {
		t.Errorf("expected BOTH TSR spanning cells (#17 colspan header + #18 rowspan '名称') "+
			"to be labelled in the grid, got %d (positions=%v). "+
			"GroupCells must not drop a span whose box only reaches one "+
			"row/column center — the TSR recognition must be preserved "+
			"(restoring the true rowspan=2 is the TSR model's job).",
			spanCount, spanPositions)
	}
}
