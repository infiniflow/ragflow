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

// TestMergeTablesAcrossPages_RebuildsGridAcrossPages verifies that after a
// cross-page merge the merged table's Grid contains rows from BOTH pages,
// not just the anchor (page-0) grid. This catches the regression where
// ConstructTable reads the stale anchor Grid and drops all continuation rows.
func TestMergeTablesAcrossPages_RebuildsGridAcrossPages(t *testing.T) {
	pageGrid := func(rows [][]string) [][]pdf.TSRCell {
		g := make([][]pdf.TSRCell, len(rows))
		for r, row := range rows {
			g[r] = make([]pdf.TSRCell, len(row))
			for c := range row {
				g[r][c] = pdf.TSRCell{
					X0: float64(c) * 100, Y0: float64(r) * 30,
					X1: float64(c)*100 + 100, Y1: float64(r)*30 + 30,
					Text: row[c],
				}
			}
		}
		return g
	}
	pg0 := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 200, Top: 0, Bottom: 60}},
		Scale:     1.0,
		Grid:      pageGrid([][]string{{"a", "b"}, {"c", "d"}}),
	}
	pg1 := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 0, Right: 200, Top: 0, Bottom: 60}},
		Scale:     1.0,
		Grid:      pageGrid([][]string{{"e", "f"}, {"g", "h"}}),
	}

	merged := MergeTablesAcrossPages([]pdf.TableItem{pg0, pg1}, nil)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	// Anchor has 2 rows, continuation has 2 rows → merged Grid must be 4.
	if len(merged[0].Grid) != 4 {
		t.Fatalf("merged Grid must contain rows from both pages (want 4), got %d", len(merged[0].Grid))
	}
	// Continuation rows must appear after anchor rows, in page order.
	if merged[0].Grid[0][0].Text != "a" || merged[0].Grid[2][0].Text != "e" {
		t.Errorf("row order wrong after stacking: %s / %s", merged[0].Grid[0][0].Text, merged[0].Grid[2][0].Text)
	}
	// Continuation rows must be Y-shifted strictly below the anchor rows so
	// Y-monotonic downstream logic (span detection, ordering) stays correct.
	// Catches a regression where the shift is dropped but stacking is kept:
	// row order would still be correct by page order, so only this assertion
	// would fail.
	if merged[0].Grid[2][0].Y0 <= merged[0].Grid[1][0].Y1 {
		t.Errorf("continuation row was not shifted below the anchor rows (Grid[2][0].Y0=%v <= Grid[1][0].Y1=%v)",
			merged[0].Grid[2][0].Y0, merged[0].Grid[1][0].Y1)
	}
}

// TestMergeTablesAcrossPages_JaggedContinuationFallsBackToAnchorGrid verifies
// that when a continuation page's grid has a different number of columns than
// the anchor (a jagged cross-page stack), MergeTablesAcrossPages does NOT
// rebuild a non-uniform Grid. Instead it keeps the anchor-only Grid, so
// ConstructTable emits a structurally valid (if continuation-dropping) table
// rather than malformed HTML. This is the same safe degrade as the
// len(anchor.Grid)==0 path, and keeps the merge decision (and the appended
// continuation Cells) unchanged.
func TestMergeTablesAcrossPages_JaggedContinuationFallsBackToAnchorGrid(t *testing.T) {
	pageGrid := func(rows [][]string) [][]pdf.TSRCell {
		g := make([][]pdf.TSRCell, len(rows))
		for r, row := range rows {
			g[r] = make([]pdf.TSRCell, len(row))
			for c := range row {
				g[r][c] = pdf.TSRCell{
					X0: float64(c) * 100, Y0: float64(r) * 30,
					X1: float64(c)*100 + 100, Y1: float64(r)*30 + 30,
					Text: row[c],
				}
			}
		}
		return g
	}
	cells := func(rows [][]string) []pdf.TSRCell {
		var cs []pdf.TSRCell
		for r, row := range rows {
			for c := range row {
				cs = append(cs, pdf.TSRCell{
					X0: float64(c) * 100, Y0: float64(r) * 30,
					X1: float64(c)*100 + 100, Y1: float64(r)*30 + 30,
					Text: row[c],
				})
			}
		}
		return cs
	}
	// Anchor: 3 columns. Continuation: 2 columns (jagged).
	pg0 := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 300, Top: 0, Bottom: 60}},
		Scale:     1.0,
		Grid:      pageGrid([][]string{{"a", "b", "c"}, {"d", "e", "f"}}),
		Cells:     cells([][]string{{"a", "b", "c"}, {"d", "e", "f"}}),
	}
	pg1 := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 0, Right: 200, Top: 0, Bottom: 60}},
		Scale:     1.0,
		Grid:      pageGrid([][]string{{"g", "h"}, {"i", "j"}}),
		Cells:     cells([][]string{{"g", "h"}, {"i", "j"}}),
	}

	merged := MergeTablesAcrossPages([]pdf.TableItem{pg0, pg1}, nil)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	// Columns differ (3 vs 2) → rebuild must be skipped → Grid stays
	// anchor-only (2 rows), NOT a 4-row jagged grid.
	if len(merged[0].Grid) != 2 {
		t.Fatalf("jagged continuation must fall back to anchor-only Grid (want 2 rows), got %d", len(merged[0].Grid))
	}
	// Anchor rows preserved; continuation NOT stacked into the Grid.
	if merged[0].Grid[0][0].Text != "a" || merged[0].Grid[1][0].Text != "d" {
		t.Errorf("anchor rows corrupted after jagged fallback: %s / %s", merged[0].Grid[0][0].Text, merged[0].Grid[1][0].Text)
	}
	// Continuation Cells are still appended (pre-fix behaviour) — the merge
	// decision is unchanged; only the Grid stays uniform so HTML is valid.
	hasCont := false
	for _, c := range merged[0].Cells {
		if c.Text == "g" {
			hasCont = true
			break
		}
	}
	if !hasCont {
		t.Errorf("continuation Cells should still be appended even when Grid rebuild is skipped")
	}
}

// TestMergeTablesAcrossPages_ThreePageCumulativeShift verifies that with three
// consecutive pages the per-page grids stack cumulatively: each continuation
// page sits strictly below the previous page's last row, and the Y shift
// accumulates (page2 below page1 below page0). Catches a regression where
// stackGrids resets prevMaxY to the anchor instead of carrying the prior
// page's shifted bottom forward.
func TestMergeTablesAcrossPages_ThreePageCumulativeShift(t *testing.T) {
	pageGrid := func(rows [][]string) [][]pdf.TSRCell {
		g := make([][]pdf.TSRCell, len(rows))
		for r, row := range rows {
			g[r] = make([]pdf.TSRCell, len(row))
			for c := range row {
				g[r][c] = pdf.TSRCell{
					X0: float64(c) * 100, Y0: float64(r) * 30,
					X1: float64(c)*100 + 100, Y1: float64(r)*30 + 30,
					Text: row[c],
				}
			}
		}
		return g
	}
	// Three pages, 2 rows × 2 cols each, identical layout.
	pages := []pdf.TableItem{
		{Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 200, Top: 0, Bottom: 60}}, Scale: 1.0, Grid: pageGrid([][]string{{"a", "b"}, {"c", "d"}})},
		{Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 0, Right: 200, Top: 0, Bottom: 60}}, Scale: 1.0, Grid: pageGrid([][]string{{"e", "f"}, {"g", "h"}})},
		{Positions: []pdf.Position{{PageNumbers: []int{2}, Left: 0, Right: 200, Top: 0, Bottom: 60}}, Scale: 1.0, Grid: pageGrid([][]string{{"i", "j"}, {"k", "l"}})},
	}

	merged := MergeTablesAcrossPages(pages, nil)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	// 3 pages × 2 rows → 6 rows, in page order.
	if len(merged[0].Grid) != 6 {
		t.Fatalf("3-page merge must stack all rows (want 6), got %d", len(merged[0].Grid))
	}
	if merged[0].Grid[0][0].Text != "a" || merged[0].Grid[2][0].Text != "e" || merged[0].Grid[4][0].Text != "i" {
		t.Errorf("row order wrong after 3-page stacking: %s / %s / %s",
			merged[0].Grid[0][0].Text, merged[0].Grid[2][0].Text, merged[0].Grid[4][0].Text)
	}
	// Cumulative Y shift: page1 strictly below page0, page2 strictly below
	// page1, and page2 below page1 (monotonic accumulation).
	if merged[0].Grid[2][0].Y0 <= merged[0].Grid[1][0].Y1 {
		t.Errorf("page1 not shifted below page0 (Grid[2][0].Y0=%v <= Grid[1][0].Y1=%v)",
			merged[0].Grid[2][0].Y0, merged[0].Grid[1][0].Y1)
	}
	if merged[0].Grid[4][0].Y0 <= merged[0].Grid[3][0].Y1 {
		t.Errorf("page2 not shifted below page1 (Grid[4][0].Y0=%v <= Grid[3][0].Y1=%v)",
			merged[0].Grid[4][0].Y0, merged[0].Grid[3][0].Y1)
	}
	if merged[0].Grid[4][0].Y0 <= merged[0].Grid[2][0].Y0 {
		t.Errorf("Y shift not cumulative: page2 (Y0=%v) must be below page1 (Y0=%v)",
			merged[0].Grid[4][0].Y0, merged[0].Grid[2][0].Y0)
	}
}

// TestMergeTablesAcrossPages_GridlessAnchorUnchanged verifies that when the
// anchor has no Grid, MergeTablesAcrossPages skips the rebuild entirely and
// leaves anchor.Grid empty (nil) so ConstructTable falls back to the Cells
// path — the same pre-fix behaviour. This locks the no-regression promise the
// fix relies on for Grid-less tables (and the 7 pre-existing tests that set no
// Grid). The cross-page merge decision itself is unchanged: continuation
// Cells and Positions are still appended.
func TestMergeTablesAcrossPages_GridlessAnchorUnchanged(t *testing.T) {
	cells := func(texts []string) []pdf.TSRCell {
		cs := make([]pdf.TSRCell, len(texts))
		for i, txt := range texts {
			cs[i] = pdf.TSRCell{
				X0: 0, Y0: float64(i) * 30, X1: 100, Y1: float64(i)*30 + 30,
				Text: txt,
			}
		}
		return cs
	}
	// Anchor has Cells but NO Grid; continuation also Grid-less.
	pg0 := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 200, Top: 0, Bottom: 60}},
		Scale:     1.0,
		Cells:     cells([]string{"a", "b", "c", "d"}),
	}
	pg1 := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 0, Right: 200, Top: 0, Bottom: 60}},
		Scale:     1.0,
		Cells:     cells([]string{"e", "f", "g", "h"}),
	}

	merged := MergeTablesAcrossPages([]pdf.TableItem{pg0, pg1}, nil)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	// Guard: anchor has no Grid → rebuild skipped entirely → Grid stays empty.
	if len(merged[0].Grid) != 0 {
		t.Fatalf("Grid-less anchor must keep Grid empty (rebuild skipped), got %d rows", len(merged[0].Grid))
	}
	// Merge decision unchanged: all continuation Cells still appended.
	have := map[string]bool{}
	for _, c := range merged[0].Cells {
		have[c.Text] = true
	}
	for _, want := range []string{"a", "b", "c", "d", "e", "f", "g", "h"} {
		if !have[want] {
			t.Errorf("merged Cells missing %q (merge decision changed for Grid-less anchor)", want)
		}
	}
	// Positions from both pages present → the cross-page merge did happen.
	pages := map[int]bool{}
	for _, p := range merged[0].Positions {
		for _, pn := range p.PageNumbers {
			pages[pn] = true
		}
	}
	if !pages[0] || !pages[1] {
		t.Errorf("cross-page merge did not combine both pages' positions: %v", pages)
	}
}
