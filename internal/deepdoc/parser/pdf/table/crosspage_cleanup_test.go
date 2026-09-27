package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestMergeTablesAcrossPages_PostMergeCleanup locks the production behavior
// flagged as untested in review: when two same-column tables on consecutive
// pages are merged, the rebuilt cross-page grid must run the post-GroupCells
// cleanup (DropAllEmptyRows + CleanupOrphanColumns + CleanupOrphanRows) at the
// merge site. Without it, an empty "table row" detected next to a header on a
// continuation page leaks into the merged grid as a row of empty cells,
// inflating item.Grid and breaking gridSim (13_crosspage_table.pdf page 2).
//
// Uses synthetic grids only — no Python dump, CI-runnable.
func TestMergeTablesAcrossPages_PostMergeCleanup(t *testing.T) {
	anchor := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 200, Top: 100, Bottom: 200}},
		Grid: [][]pdf.TSRCell{
			{{X0: 0, Y0: 100, X1: 100, Y1: 130, Text: "h1"}, {X0: 100, Y0: 100, X1: 200, Y1: 130, Text: "h2"}},
			{{X0: 0, Y0: 130, X1: 100, Y1: 160, Text: "a"}, {X0: 100, Y0: 130, X1: 200, Y1: 160, Text: "b"}},
			// Empty orphan row (e.g. a stray "table row" next to a header on
			// the continuation page): must be dropped by the merge-site cleanup.
			{{X0: 0, Y0: 160, X1: 100, Y1: 190}, {X0: 100, Y0: 160, X1: 200, Y1: 190}},
		},
	}
	continuation := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 0, Right: 200, Top: 210, Bottom: 260}},
		Grid: [][]pdf.TSRCell{
			{{X0: 0, Y0: 210, X1: 100, Y1: 240, Text: "c"}, {X0: 100, Y0: 210, X1: 200, Y1: 240, Text: "d"}},
		},
	}

	out := MergeTablesAcrossPages([]pdf.TableItem{anchor, continuation}, nil, map[int]float64{0: 190})
	if len(out) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(out))
	}
	merged := out[0]

	// Stacked grid = 3 anchor rows + 1 continuation row = 4; the empty orphan
	// row must be removed, leaving 3 rows (h, a, c). Continuation rows preserved.
	if len(merged.Grid) != 3 {
		t.Fatalf("expected 3 rows after empty-row cleanup, got %d: %+v", len(merged.Grid), merged.Grid)
	}
	// The cleaned grid must be reflected in the public Rows field.
	if len(merged.Rows) != 3 {
		t.Errorf("merged.Rows should have 3 rows after cleanup, got %d", len(merged.Rows))
	}
	// Spot-check the continuation content survived the merge.
	last := merged.Grid[len(merged.Grid)-1]
	if len(last) < 2 || last[0].Text != "c" || last[1].Text != "d" {
		t.Errorf("continuation row content lost: %+v", last)
	}
}
