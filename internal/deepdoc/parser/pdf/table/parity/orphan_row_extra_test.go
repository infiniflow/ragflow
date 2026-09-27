//go:build manual

package parity

// These tests drive CleanupOrphanRows directly (bypassing CleanupOrphanColumns)
// so the row-merge DIRECTION logic can be asserted precisely. Going through
// ConstructTable would let column cleanup remove the single-cell column first,
// which makes the "both neighbors empty in the same column" case impossible to
// isolate (that column would be an orphan column and get merged away).

import (
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// MergesUpWhenCloser: orphan row (col 2) with empty same-column cells above
// and below; the vertical gap to the row above is smaller → merge UP.
func TestCleanupOrphanRows_MergesUpWhenCloser(t *testing.T) {
	grid := [][]pdf.TSRCell{
		{
			{Text: "A", X0: 0, X1: 100, Y0: 0, Y1: 30},
			{Text: "B", X0: 101, X1: 200, Y0: 0, Y1: 30},
			{Text: "", X0: 201, X1: 300, Y0: 0, Y1: 30},
			{Text: "D", X0: 301, X1: 400, Y0: 0, Y1: 30},
		},
		{
			{Text: "", X0: 0, X1: 100, Y0: 40, Y1: 70},
			{Text: "", X0: 101, X1: 200, Y0: 40, Y1: 70},
			{Text: "孤", X0: 201, X1: 300, Y0: 40, Y1: 70},
			{Text: "", X0: 301, X1: 400, Y0: 40, Y1: 70},
		},
		{
			{Text: "E", X0: 0, X1: 100, Y0: 200, Y1: 230},
			{Text: "F", X0: 101, X1: 200, Y0: 200, Y1: 230},
			{Text: "", X0: 201, X1: 300, Y0: 200, Y1: 230},
			{Text: "H", X0: 301, X1: 400, Y0: 200, Y1: 230},
		},
	}
	// up = 40-30 = 10; down = 200-70 = 130 → up < down → merge UP into row 0.
	out := table.CleanupOrphanRows(grid)
	if len(out) != 2 {
		t.Fatalf("expected 2 rows after up-merge, got %d", len(out))
	}
	if out[0][2].Text != "孤" {
		t.Errorf("orphan should merge UP into row 0 col 2, got %q at [0][2]", out[0][2].Text)
	}
}

// MergesDownWhenCloser: same shape, but the row below is closer → merge DOWN.
func TestCleanupOrphanRows_MergesDownWhenCloser(t *testing.T) {
	grid := [][]pdf.TSRCell{
		{
			{Text: "A", X0: 0, X1: 100, Y0: 0, Y1: 30},
			{Text: "B", X0: 101, X1: 200, Y0: 0, Y1: 30},
			{Text: "", X0: 201, X1: 300, Y0: 0, Y1: 30},
			{Text: "D", X0: 301, X1: 400, Y0: 0, Y1: 30},
		},
		{
			{Text: "", X0: 0, X1: 100, Y0: 100, Y1: 130},
			{Text: "", X0: 101, X1: 200, Y0: 100, Y1: 130},
			{Text: "孤", X0: 201, X1: 300, Y0: 100, Y1: 130},
			{Text: "", X0: 301, X1: 400, Y0: 100, Y1: 130},
		},
		{
			{Text: "E", X0: 0, X1: 100, Y0: 140, Y1: 170},
			{Text: "F", X0: 101, X1: 200, Y0: 140, Y1: 170},
			{Text: "", X0: 201, X1: 300, Y0: 140, Y1: 170},
			{Text: "H", X0: 301, X1: 400, Y0: 140, Y1: 170},
		},
	}
	// up = 100-30 = 70; down = 140-130 = 10 → down < up → merge DOWN into row 2.
	out := table.CleanupOrphanRows(grid)
	if len(out) != 2 {
		t.Fatalf("expected 2 rows after down-merge, got %d", len(out))
	}
	if out[1][2].Text != "孤" {
		t.Errorf("orphan should merge DOWN into row 1 col 2, got %q at [1][2]", out[1][2].Text)
	}
}

// BottomMergesUp: orphan at the LAST row (col 0), row above same column empty.
// i+1 >= nRows → hasBelow true → merge UP (up is finite).
func TestCleanupOrphanRows_BottomMergesUp(t *testing.T) {
	grid := [][]pdf.TSRCell{
		{
			{Text: "X", X0: 0, X1: 100, Y0: 0, Y1: 30},
			{Text: "B", X0: 101, X1: 200, Y0: 0, Y1: 30},
			{Text: "C", X0: 201, X1: 300, Y0: 0, Y1: 30},
			{Text: "D", X0: 301, X1: 400, Y0: 0, Y1: 30},
		},
		{
			{Text: "", X0: 0, X1: 100, Y0: 35, Y1: 65},
			{Text: "E", X0: 101, X1: 200, Y0: 35, Y1: 65},
			{Text: "F", X0: 201, X1: 300, Y0: 35, Y1: 65},
			{Text: "G", X0: 301, X1: 400, Y0: 35, Y1: 65},
		},
		{
			{Text: "孤", X0: 0, X1: 100, Y0: 100, Y1: 130},
			{Text: "", X0: 101, X1: 200, Y0: 100, Y1: 130},
			{Text: "", X0: 201, X1: 300, Y0: 100, Y1: 130},
			{Text: "", X0: 301, X1: 400, Y0: 100, Y1: 130},
		},
	}
	out := table.CleanupOrphanRows(grid)
	if len(out) != 2 {
		t.Fatalf("expected 2 rows after bottom up-merge, got %d", len(out))
	}
	if out[1][0].Text != "孤" {
		t.Errorf("bottom orphan should merge UP into row 1 col 0, got %q at [1][0]", out[1][0].Text)
	}
}

// NoNeighborsKept: orphan row sandwiched between TWO entirely empty rows.
// Both up and down distances are +inf → the guard keeps the row in place
// (Python would assert/crash here). This preserves data instead of relocating.
func TestCleanupOrphanRows_NoNeighborsKept(t *testing.T) {
	grid := [][]pdf.TSRCell{
		{
			{Text: "", X0: 0, X1: 100, Y0: 0, Y1: 30},
			{Text: "", X0: 101, X1: 200, Y0: 0, Y1: 30},
			{Text: "", X0: 201, X1: 300, Y0: 0, Y1: 30},
			{Text: "", X0: 301, X1: 400, Y0: 0, Y1: 30},
		},
		{
			{Text: "", X0: 0, X1: 100, Y0: 40, Y1: 70},
			{Text: "", X0: 101, X1: 200, Y0: 40, Y1: 70},
			{Text: "孤", X0: 201, X1: 300, Y0: 40, Y1: 70},
			{Text: "", X0: 301, X1: 400, Y0: 40, Y1: 70},
		},
		{
			{Text: "", X0: 0, X1: 100, Y0: 80, Y1: 110},
			{Text: "", X0: 101, X1: 200, Y0: 80, Y1: 110},
			{Text: "", X0: 201, X1: 300, Y0: 80, Y1: 110},
			{Text: "", X0: 301, X1: 400, Y0: 80, Y1: 110},
		},
	}
	out := table.CleanupOrphanRows(grid)
	if len(out) != 3 {
		t.Fatalf("expected 3 rows kept (no mergeable neighbor), got %d", len(out))
	}
	if out[1][2].Text != "孤" {
		t.Errorf("orphan should stay in its own row when both neighbors are empty, got %q at [1][2]", out[1][2].Text)
	}
}
