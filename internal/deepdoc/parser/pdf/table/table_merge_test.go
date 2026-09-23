package table

import (
	"fmt"
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
	merged := MergeTablesAcrossPages(tables, nil, map[int]float64{0: 820, 1: 820})
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
	merged := MergeTablesAcrossPages(tables, nil, map[int]float64{0: 820, 1: 820})
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
	merged := MergeTablesAcrossPages(tables, nil, map[int]float64{0: 842, 3: 842})
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
	merged := MergeTablesAcrossPages(tables, nil, map[int]float64{0: 842})
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
	merged := MergeTablesAcrossPages(tables, nil, map[int]float64{0: 842})
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
	merged := MergeTablesAcrossPages(tables, medianHeights, map[int]float64{0: 842, 1: 842})
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
	merged := MergeTablesAcrossPages(tables, nil, map[int]float64{0: 300})
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

	merged := MergeTablesAcrossPages([]pdf.TableItem{pg0, pg1}, nil, map[int]float64{0: 70})
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

// TestMergeTablesAcrossPages_JaggedContinuationPreservesRows verifies that
// when a continuation page's grid is not column-uniform with the anchor (a
// jagged cross-page stack), MergeTablesAcrossPages still preserves every
// continuation row. It aligns both per-page grids to a shared column model
// (the max column count, padding shorter rows by index) and stacks all rows,
// instead of dropping the continuation page's Grid. Regression test for the
// bug where a non-uniform cross-page grid silently deleted an entire
// continuation page from the output.
func TestMergeTablesAcrossPages_JaggedContinuationPreservesRows(t *testing.T) {
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
	cases := []struct {
		name         string
		anchorRows   [][]string
		contRows     [][]string
		contCellText string
	}{
		{
			name:         "first row column count differs",
			anchorRows:   [][]string{{"a", "b", "c"}, {"d", "e", "f"}},
			contRows:     [][]string{{"g", "h"}, {"i", "j"}},
			contCellText: "g",
		},
		{
			name:         "interior row column count differs",
			anchorRows:   [][]string{{"a", "b", "c"}, {"d", "e", "f"}},
			contRows:     [][]string{{"g", "h", "i"}, {"j", "k"}},
			contCellText: "g",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pg0 := pdf.TableItem{
				Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 300, Top: 0, Bottom: 60}},
				Scale:     1.0,
				Grid:      pageGrid(tc.anchorRows),
				Cells:     cells(tc.anchorRows),
			}
			pg1 := pdf.TableItem{
				Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 0, Right: 300, Top: 0, Bottom: 60}},
				Scale:     1.0,
				Grid:      pageGrid(tc.contRows),
				Cells:     cells(tc.contRows),
			}

			merged := MergeTablesAcrossPages([]pdf.TableItem{pg0, pg1}, nil, map[int]float64{0: 70})
			if len(merged) != 1 {
				t.Fatalf("expected 1 merged table, got %d", len(merged))
			}
			// Columns differ (anchor 3 vs continuation jagged) → aligned to
			// uniCols=3, rows still stacked: anchor + continuation (no drop).
			wantRows := len(tc.anchorRows) + len(tc.contRows)
			if len(merged[0].Grid) != wantRows {
				t.Fatalf("jagged continuation must preserve all rows (want %d), got %d", wantRows, len(merged[0].Grid))
			}
			// Aligned width is the max column count (3) for every row.
			for r, row := range merged[0].Grid {
				if len(row) != 3 {
					t.Errorf("row %d: aligned grid width must be max cols (3), got %d", r, len(row))
				}
			}
			// Anchor rows preserved first.
			if merged[0].Grid[0][0].Text != tc.anchorRows[0][0] || merged[0].Grid[1][0].Text != tc.anchorRows[1][0] {
				t.Errorf("anchor rows corrupted after alignment: %s / %s", merged[0].Grid[0][0].Text, merged[0].Grid[1][0].Text)
			}
			// Continuation rows appended in page order, padded by index.
			base := len(tc.anchorRows)
			for r, crow := range tc.contRows {
				for c, txt := range crow {
					if merged[0].Grid[base+r][c].Text != txt {
						t.Errorf("continuation cell lost after alignment: Grid[%d][%d]=%q want %q", base+r, c, merged[0].Grid[base+r][c].Text, txt)
					}
				}
			}
			// Continuation Cells are still appended (merge decision unchanged).
			hasCont := false
			for _, c := range merged[0].Cells {
				if c.Text == tc.contCellText {
					hasCont = true
					break
				}
			}
			if !hasCont {
				t.Errorf("continuation Cells should still be appended after alignment")
			}
		})
	}
}

// TestMergeTablesAcrossPages_MixedColumnCountsPreservesAllRows reproduces the
// 中加纯债 cross-page table: page 0 has 27 rows × 6 cols and page 1 has 35
// rows × 7 cols (TSR detects one extra spurious column on page 1). The merged
// table must contain ALL 62 rows (27 + 35) at the shared width of 7 columns.
func TestMergeTablesAcrossPages_MixedColumnCountsPreservesAllRows(t *testing.T) {
	gridWithTag := func(rows, cols int, tag string) [][]pdf.TSRCell {
		g := make([][]pdf.TSRCell, rows)
		for r := 0; r < rows; r++ {
			g[r] = make([]pdf.TSRCell, cols)
			for c := 0; c < cols; c++ {
				g[r][c] = pdf.TSRCell{
					X0: float64(c) * 100, Y0: float64(r) * 30,
					X1: float64(c)*100 + 100, Y1: float64(r)*30 + 30,
					Text: fmt.Sprintf("%s_r%d_c%d", tag, r, c),
				}
			}
		}
		return g
	}
	pg0 := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 600, Top: 0, Bottom: 810}},
		Scale:     1.0,
		Grid:      gridWithTag(27, 6, "p0"),
	}
	pg1 := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 0, Right: 700, Top: 0, Bottom: 1050}},
		Scale:     1.0,
		Grid:      gridWithTag(35, 7, "p1"),
	}

	merged := MergeTablesAcrossPages([]pdf.TableItem{pg0, pg1}, nil, map[int]float64{0: 510})
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	// All 62 rows (27 + 35) must survive the cross-page merge.
	if len(merged[0].Grid) != 62 {
		t.Fatalf("merged Grid must contain all rows from both pages (want 62), got %d", len(merged[0].Grid))
	}
	// Shared width is the max column count (7) for every row.
	for r, row := range merged[0].Grid {
		if len(row) != 7 {
			t.Errorf("row %d: aligned grid width must be max cols (7), got %d", r, len(row))
		}
	}
	// Anchor rows first, continuation rows appended, both complete.
	if merged[0].Grid[0][0].Text != "p0_r0_c0" {
		t.Errorf("first anchor row lost: %s", merged[0].Grid[0][0].Text)
	}
	if merged[0].Grid[26][5].Text != "p0_r26_c5" {
		t.Errorf("last anchor row lost: %s", merged[0].Grid[26][5].Text)
	}
	if merged[0].Grid[27][0].Text != "p1_r0_c0" {
		t.Errorf("first continuation row lost: %s", merged[0].Grid[27][0].Text)
	}
	if merged[0].Grid[61][6].Text != "p1_r34_c6" {
		t.Errorf("last continuation row lost: %s", merged[0].Grid[61][6].Text)
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

	merged := MergeTablesAcrossPages(pages, nil, map[int]float64{0: 70, 1: 70, 2: 70})
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

	merged := MergeTablesAcrossPages([]pdf.TableItem{pg0, pg1}, nil, map[int]float64{0: 70})
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

// TestMergeTablesAcrossPages_PageLocalYRepeatsButSeparatePages is the
// regression test for the icbccs deployment.pdf over-merge (known_diffs.json
// rule icbccs-crosspage-table-overmerge). Two API-parameter tables sit on
// consecutive pages but their page-LOCAL Y coordinates happen to repeat every
// page (anchor page 4 bottom=172, continuation page 5 local top=262) — the
// same pattern that, evaluated as if continuous, yields yDis≈99 and wrongly
// merges. Python keeps them separate because in absolute page-stacked
// coordinates the gap is ~862pt. This test locks that Go must NOT merge them:
// the Y proximity gate must be measured in a page-absolute frame using the
// anchor page's height.
func TestMergeTablesAcrossPages_PageLocalYRepeatsButSeparatePages(t *testing.T) {
	// Anchor on page 4: near the top of the page (local bottom=172).
	anchor := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{4}, Left: 30, Right: 566, Top: 85, Bottom: 172}},
		Scale:     1.0,
		Cells:     []pdf.TSRCell{{Text: "req_params_p4"}},
		Caption:   "请求参数",
	}
	// Continuation on page 5: local top=262 (again near the top of its page).
	cont := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{5}, Left: 30, Right: 566, Top: 262, Bottom: 350}},
		Scale:     1.0,
		Cells:     []pdf.TSRCell{{Text: "req_params_p5"}},
		Caption:   "请求参数",
	}
	pageHeights := map[int]float64{4: 842, 5: 842} // standard A4 point height
	// Both with a realistic median char height AND with the nil (mh=10) default.
	for _, mh := range []map[int]float64{nil, {4: 13, 5: 13}} {
		merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, cont}, mh, pageHeights)
		if len(merged) != 2 {
			t.Fatalf("page-local Y repeats across pages: expected 2 SEPARATE tables (no merge), got %d (over-merge bug)", len(merged))
		}
	}
}

// TestMergeTablesAcrossPages_RealAdjacentAcrossPagesStillMerges locks that a
// genuine cross-page split — anchor table near the BOTTOM of its page and the
// continuation near the TOP of the next page — is still merged after the
// page-absolute Y fix. This prevents the fix from over-correcting and
// splitting real cross-page tables (e.g. the 13_crosspage_table case).
func TestMergeTablesAcrossPages_RealAdjacentAcrossPagesStillMerges(t *testing.T) {
	// Anchor page 4: near the bottom (local bottom=800).
	anchor := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{4}, Left: 30, Right: 566, Top: 740, Bottom: 800}},
		Scale:     1.0,
		Cells:     []pdf.TSRCell{{Text: "head_p4"}},
	}
	// Continuation page 5: near the top (local top=50).
	cont := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{5}, Left: 30, Right: 566, Top: 50, Bottom: 110}},
		Scale:     1.0,
		Cells:     []pdf.TSRCell{{Text: "head_p5"}},
	}
	pageHeights := map[int]float64{4: 842, 5: 842}
	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, cont}, map[int]float64{4: 13, 5: 13}, pageHeights)
	if len(merged) != 1 {
		t.Fatalf("genuine adjacent cross-page split: expected 1 merged table, got %d", len(merged))
	}
	if len(merged[0].Positions) != 2 {
		t.Errorf("merged table should record both pages, got %d positions", len(merged[0].Positions))
	}
}

// TestMergeTablesAcrossPages_GenuineContinuationMergesWithoutMedianHeights
// locks the #18688 bond cross-page regression fix's core insight: the
// page-absolute Y shift is now gated by the SIGN of the page-local yDis. A
// genuine continuation sits at the top of the next page, so its page-local
// yDis is NEGATIVE and must NOT be shifted — it merges even when medianHeights
// is unavailable (replay char-height is 0, so mh falls back to the default 10).
//
// Geometry: anchor page 0 near the bottom (local bottom=800), continuation
// page 1 near the top (local top=50), page height 842 → page-local
// yDis = (50+110-800-800)/2 = -720 (< 0) ⇒ no shift ⇒ MERGE. Under the
// pre-fix #18688 code the shift would make yDis=122 (< mh*23=230) and also
// merge, so this test mainly guards that the merge no longer depends on
// medianHeights being populated for a legitimate continuation.
func TestMergeTablesAcrossPages_GenuineContinuationMergesWithoutMedianHeights(t *testing.T) {
	anchor := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 30, Right: 566, Top: 740, Bottom: 800}},
		Scale:     1.0,
		Cells:     []pdf.TSRCell{{Text: "anchor_p0"}},
	}
	cont := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 30, Right: 566, Top: 50, Bottom: 110}},
		Scale:     1.0,
		Cells:     []pdf.TSRCell{{Text: "cont_p1"}},
	}
	pageHeights := map[int]float64{0: 842, 1: 842}
	// medianHeights=nil mimics the replay path (char-height 0).
	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, cont}, nil, pageHeights)
	if len(merged) != 1 {
		t.Fatalf("genuine cross-page continuation: expected 1 merged table, got %d", len(merged))
	}
	if len(merged[0].Positions) != 2 {
		t.Errorf("merged table should record both pages, got %d positions", len(merged[0].Positions))
	}
}

// TestStackGrids_StripsRepeatedHeaderRow verifies that continuation pages
// repeating the header row have that duplicate header row stripped.
func TestStackGrids_StripsRepeatedHeaderRow(t *testing.T) {
	grid1 := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 50, Y1: 20, Text: "序号"},
			{X0: 50, Y0: 0, X1: 100, Y1: 20, Text: "材料名称"},
			{X0: 100, Y0: 0, X1: 150, Y1: 20, Text: "规格型号"},
		},
		{
			{X0: 0, Y0: 20, X1: 50, Y1: 40, Text: "1"},
			{X0: 50, Y0: 20, X1: 100, Y1: 40, Text: "钢筋"},
			{X0: 100, Y0: 20, X1: 150, Y1: 40, Text: "HRB400"},
		},
	}
	grid2 := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 50, Y1: 20, Text: "序号"},
			{X0: 50, Y0: 0, X1: 100, Y1: 20, Text: "材料名称"},
			{X0: 100, Y0: 0, X1: 150, Y1: 20, Text: "规格型号"},
		},
		{
			{X0: 0, Y0: 20, X1: 50, Y1: 40, Text: "2"},
			{X0: 50, Y0: 20, X1: 100, Y1: 40, Text: "水泥"},
			{X0: 100, Y0: 20, X1: 150, Y1: 40, Text: "P.O 42.5"},
		},
	}

	stacked := stackGrids(grid1, grid2)
	// Expect 3 rows: header, row 1, row 2 (not 4 rows).
	if len(stacked) != 3 {
		t.Fatalf("expected 3 rows after stripping repeated header, got %d", len(stacked))
	}
	if stacked[1][0].Text != "1" {
		t.Errorf("row 1 expected '1', got %q", stacked[1][0].Text)
	}
	if stacked[2][0].Text != "2" {
		t.Errorf("row 2 expected '2', got %q", stacked[2][0].Text)
	}
}

// TestMergeTablesAcrossPages_DeduplicateCaption verifies that identical or
// overlapping captions are deduplicated during cross-page merge.
func TestMergeTablesAcrossPages_DeduplicateCaption(t *testing.T) {
	anchor := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 30, Right: 566, Top: 740, Bottom: 800}},
		Scale:     1.0,
		Cells:     []pdf.TSRCell{{Text: "cell1"}},
		Caption:   "江西省材料价格参考信息",
	}
	cont := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 30, Right: 566, Top: 50, Bottom: 110}},
		Scale:     1.0,
		Cells:     []pdf.TSRCell{{Text: "cell2"}},
		Caption:   "江西省材料价格参考信息",
	}
	pageHeights := map[int]float64{0: 842, 1: 842}
	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, cont}, nil, pageHeights)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	if merged[0].Caption != "江西省材料价格参考信息" {
		t.Errorf("expected clean deduplicated caption, got %q", merged[0].Caption)
	}
}

// TestMergeTablesAcrossPages_MisalignedColumnsAlignByX reproduces a real
// materials-price-list cross-page case: the anchor page detects all 3 columns
// while the continuation page misses the first separator, so its first cell
// carries the merged "number name" text at the first column's position and
// every later cell sits one grid index to the left of its logical column.
// Index-based padding would place the last-column value under the middle
// header; the merged grid must instead re-align by X so each value lands
// under the anchor column it overlaps.
func TestMergeTablesAcrossPages_MisalignedColumnsAlignByX(t *testing.T) {
	cell := func(x0, y0, x1, y1 float64, text string) pdf.TSRCell {
		return pdf.TSRCell{X0: x0, Y0: y0, X1: x1, Y1: y1, Text: text}
	}
	anchor := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 400, Top: 0, Bottom: 90}},
		Scale:     1.0,
		Grid: [][]pdf.TSRCell{
			{cell(0, 0, 100, 30, "序号"), cell(100, 0, 250, 30, "材料名称"), cell(250, 0, 400, 30, "规格")},
			{cell(0, 30, 100, 60, "1"), cell(100, 30, 250, 60, "闸阀"), cell(250, 30, 400, 60, "Z15")},
		},
	}
	cont := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 0, Right: 400, Top: 0, Bottom: 60}},
		Scale:     1.0,
		Grid: [][]pdf.TSRCell{
			// The first two logical columns merged into one cell; the last
			// column keeps the anchor's X range but sits at grid index 1.
			{cell(0, 0, 100, 30, "21 切换模块"), cell(250, 0, 400, 30, "K-30")},
		},
	}
	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, cont}, nil, map[int]float64{0: 100})
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	g := merged[0].Grid
	if len(g) != 3 {
		t.Fatalf("expected 3 rows (2 anchor + 1 continuation), got %d", len(g))
	}
	for r, row := range g {
		if len(row) != 3 {
			t.Fatalf("row %d: width must be 3, got %d", r, len(row))
		}
	}
	if g[2][0].Text != "21 切换模块" {
		t.Errorf("continuation merged 序号/名称 cell must stay in column 0, got %q", g[2][0].Text)
	}
	if g[2][1].Text != "" {
		t.Errorf("column 1 (材料名称) must stay empty on the misaligned page, got %q", g[2][1].Text)
	}
	if g[2][2].Text != "K-30" {
		t.Errorf("规格 value must align to anchor column 2 by X, got %q at cols [%q %q]", g[2][2].Text, g[2][0].Text, g[2][1].Text)
	}
}

// TestRebuildMergedGrid_EmptyContinuationGridFallsBackToCells verifies that a
// degenerate continuation grid (no rows, but its Cells were merged into the
// anchor by MergeTablesAcrossPages) clears the anchor grid instead of leaving
// a stale page-0-only grid: ConstructTable then rebuilds rows from the FULL
// merged cell set rather than omitting the continuation cells.
func TestRebuildMergedGrid_EmptyContinuationGridFallsBackToCells(t *testing.T) {
	anchor := pdf.TableItem{
		Grid: [][]pdf.TSRCell{{
			{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "a"},
			{X0: 100, Y0: 0, X1: 200, Y1: 30, Text: "b"},
		}},
		Rows: [][]string{{"a", "b"}},
	}
	emptyCont := [][]pdf.TSRCell{}
	rebuildMergedGrid(&anchor, [][][]pdf.TSRCell{emptyCont})
	if anchor.Grid != nil {
		t.Errorf("stale anchor grid must be cleared for the cells fallback, got %d rows", len(anchor.Grid))
	}
	if anchor.Rows != nil {
		t.Errorf("stale anchor Rows must be cleared alongside Grid, got %v", anchor.Rows)
	}
}

// TestGridsHaveUniformWidth pins the per-ROW uniformity guarantee that gates
// index padding: a row narrower than its page's maximum signals a locally
// missed separator, and index padding would shift its values left under the
// wrong headers.
func TestGridsHaveUniformWidth(t *testing.T) {
	row := func(n int) []pdf.TSRCell {
		r := make([]pdf.TSRCell, n)
		for i := range r {
			r[i] = pdf.TSRCell{X0: float64(i) * 100, X1: float64(i)*100 + 100, Y1: 30}
		}
		return r
	}
	cases := []struct {
		name  string
		grids [][][]pdf.TSRCell
		want  bool
	}{
		{"all rows equal across pages", [][][]pdf.TSRCell{{row(2), row(2)}, {row(2)}}, true},
		{"per-page column counts differ", [][][]pdf.TSRCell{{row(2), row(2)}, {row(3)}}, false},
		{"jagged row within one page (same max width)", [][][]pdf.TSRCell{{row(2), row(1)}}, false},
	}
	for _, tc := range cases {
		if got := gridsHaveUniformWidth(tc.grids); got != tc.want {
			t.Errorf("%s: gridsHaveUniformWidth = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestRebuildMergedGrid_MixedWidthRowsWithSameMaxAlignByX covers the mixed
// continuation page shape: same maximum row width as the anchor, but one row
// missing an interior separator. Index padding would keep that row's trailing
// cells left-shifted; the grid must instead go through X-based alignment so
// every value lands under the anchor column its X range overlaps.
func TestRebuildMergedGrid_MixedWidthRowsWithSameMaxAlignByX(t *testing.T) {
	grid := func(rows [][]string) [][]pdf.TSRCell {
		g := make([][]pdf.TSRCell, len(rows))
		for r, rowTexts := range rows {
			g[r] = make([]pdf.TSRCell, len(rowTexts))
			for c := range rowTexts {
				g[r][c] = pdf.TSRCell{
					X0: float64(c) * 100, Y0: float64(r) * 30,
					X1: float64(c)*100 + 100, Y1: float64(r)*30 + 30,
					Text: rowTexts[c],
				}
			}
		}
		return g
	}
	anchorGrid := grid([][]string{{"a", "b"}, {"c", "d"}})
	// Continuation: one full-width row, one row whose separator between the
	// two columns was missed — its second cell physically spans column 1 only
	// from X=100, so X alignment must place "z" in column 1, while index
	// padding of the jagged shape must never be trusted.
	contGrid := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "e"},
			{X0: 100, Y0: 0, X1: 200, Y1: 30, Text: "f"},
		},
		{
			{X0: 0, Y0: 30, X1: 100, Y1: 60, Text: "y"},
			{X0: 100, Y0: 30, X1: 200, Y1: 60, Text: "z"},
		},
	}
	// Make contGrid jagged (missed separator merges col 0+1 into one wide cell).
	contGrid[1] = []pdf.TSRCell{{X0: 0, Y0: 30, X1: 200, Y1: 60, Text: "y"}}
	anchor := pdf.TableItem{Grid: anchorGrid}
	rebuildMergedGrid(&anchor, [][][]pdf.TSRCell{contGrid})
	if len(anchor.Grid) != 4 {
		t.Fatalf("merged grid must keep every row, got %d", len(anchor.Grid))
	}
	if len(anchor.Grid[3]) != 2 {
		t.Fatalf("X alignment must normalize every row to the canonical 2 columns, got %d cells", len(anchor.Grid[3]))
	}
	if anchor.Grid[3][0].Text != "y" {
		t.Errorf("wide merged cell must map to its best-overlap column 0, got %q", anchor.Grid[3][0].Text)
	}
	if anchor.Grid[3][1].Text != "" {
		t.Errorf("column 1 must stay empty for the merged cell (no invented data), got %q", anchor.Grid[3][1].Text)
	}
}

// TestMergeTablesAcrossPages_UnrelatedContinuationCaptionDropped pins the
// 江西 price-list shape: every continuation page carries a page-header block
// that TSR labels as a caption; the merged table keeps only the anchor's
// caption instead of concatenating every page's section name.
func TestMergeTablesAcrossPages_UnrelatedContinuationCaptionDropped(t *testing.T) {
	anchor := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 30, Right: 566, Top: 740, Bottom: 800}},
		Scale:     1.0,
		Cells:     []pdf.TSRCell{{Text: "cell1"}},
		Caption:   "全省价格信息汇总表一、阀门类",
	}
	cont := pdf.TableItem{
		Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 30, Right: 566, Top: 50, Bottom: 110}},
		Scale:     1.0,
		Cells:     []pdf.TSRCell{{Text: "cell2"}},
		Caption:   "全省价格信息汇总表八、电管类",
	}
	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, cont}, nil, map[int]float64{0: 842, 1: 842})
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged table, got %d", len(merged))
	}
	if merged[0].Caption != "全省价格信息汇总表一、阀门类" {
		t.Errorf("merged caption = %q, want the anchor caption only (no concatenation)", merged[0].Caption)
	}
}
