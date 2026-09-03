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
