//go:build manual

// Package parity isolates the orphan-row / orphan-column cleanup parity tests
// from the rest of the `table` package's test files. The package's other
// manual-tier test file (table_parity_issues_test.go) does not currently
// compile on main (references removed/renamed symbols), which would otherwise
// break the whole package's manual test binary. These tests only depend on the
// EXPORTED surface (ConstructTable, CleanupOrphanColumns) plus the exported
// pdf.TSRCell / pdf.TableItem types, so they build independently.
//
// Run:
//
//	bash build.sh --test-manual ./internal/deepdoc/parser/pdf/table/parity/
package parity

import (
	"strings"
	"testing"

	"ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// =============================================================================
// Parity finding #2 (orphan-row cleanup): Go lacks a row counterpart to
// CleanupOrphanColumns.
//
// Python's construct_table (deepdoc/vision/table_structure_recognizer.py:279-333)
// merges a row that holds EXACTLY ONE non-empty cell into its nearest vertical
// neighbor when len(cols) >= 4. The merge direction is chosen by the minimum
// vertical gap (up vs down); a row "sandwiched" between two populated cells in
// the same column is kept. Go's ConstructTable (table_construct.go:39) only
// calls CleanupOrphanColumns and has no orphan-row pass.
//
// Item state after cleanup: ConstructTable sets item.Grid and item.Rows from
// the cleaned grid (Python-aligned). Both must be derived AFTER the cleanup
// passes, since cleanup may drop rows/columns.
//
// The tests below assert the Python-aligned behavior. They are RED on main
// because the row-cleanup pass does not exist.
// =============================================================================

// buildGrid builds a rectangular [][]pdf.TSRCell from a text matrix using
// deterministic coordinates: column ci occupies [ci*100, ci*100+100],
// row ri occupies [ri*40, ri*40+30]. Empty strings become empty cells.
// Coordinates matter for the merge-distance logic but not for row/col counting.
func buildGrid(text [][]string) [][]pdf.TSRCell {
	rows := make([][]pdf.TSRCell, len(text))
	for ri := range text {
		rows[ri] = make([]pdf.TSRCell, len(text[ri]))
		for ci := range text[ri] {
			rows[ri][ci] = pdf.TSRCell{
				X0:   float64(ci) * 100,
				X1:   float64(ci)*100 + 100,
				Y0:   float64(ri) * 40,
				Y1:   float64(ri)*40 + 30,
				Text: text[ri][ci],
			}
		}
	}
	return rows
}

func trCount(html string) int { return strings.Count(html, "<tr>") }

func colCount(rows [][]pdf.TSRCell) int {
	if len(rows) == 0 {
		return 0
	}
	return len(rows[0])
}

// =============================================================================
// A) Orphan MIDDLE row merges UP.
// Row 2 has a single cell ("孤儿") at col 0. Row 1 col 0 is EMPTY (not
// sandwiched above) and row 3 col 0 is populated (ff=true). Python merges the
// orphan UP into row 1 (up gap finite, down gap +inf) and pops row 2 → 3 rows.
// =============================================================================

func TestConstructTable_OrphanRow_MergesUp(t *testing.T) {
	grid := buildGrid([][]string{
		{"H0", "H1", "H2", "H3"},
		{"", "A1", "A2", "A3"},
		{"孤儿", "", "", ""},
		{"D0", "D1", "D2", "D3"},
	})

	html := table.ConstructTable(nil, nil, "", &pdf.TableItem{Grid: grid})

	// FIXED: 4 rows → 3 after merging the single-cell row up.
	if got := trCount(html); got != 3 {
		t.Errorf("ORPHAN-ROW BUG: expected 3 <tr> after merging the single-cell row up, got %d. Python merges row 2 into row 1 (table_structure_recognizer.py:279-333).", got)
	}

	// '孤儿' must land in the row containing 'A1' (merged UP), not its own row.
	mergedUp := false
	for _, tr := range strings.Split(html, "<tr>") {
		if strings.Contains(tr, "孤儿") && strings.Contains(tr, "A1") {
			mergedUp = true
		}
	}
	if !mergedUp {
		t.Errorf("ORPHAN-ROW BUG: '孤儿' should merge UP into the row containing 'A1'. HTML:\n%s", html)
	}
	t.Logf("HTML:\n%s", html)
}

// =============================================================================
// B) Orphan TOP row merges DOWN.
// Row 0 (top) is an orphan at col 0. i==0 → f=true; row 1 col 0 EMPTY → ff=false
// → merge DOWN into row 1. Row 0 popped → 3 rows.
// =============================================================================

func TestConstructTable_OrphanRow_TopMergesDown(t *testing.T) {
	grid := buildGrid([][]string{
		{"顶孤", "", "", ""},
		{"", "B1", "B2", "B3"},
		{"C0", "C1", "C2", "C3"},
		{"D0", "D1", "D2", "D3"},
	})

	html := table.ConstructTable(nil, nil, "", &pdf.TableItem{Grid: grid})

	if got := trCount(html); got != 3 {
		t.Errorf("ORPHAN-ROW BUG: expected 3 <tr> after merging the top orphan row down, got %d. Python merges row 0 into row 1 (table_structure_recognizer.py:279-333).", got)
	}

	// '顶孤' must land in the row containing 'B1' (merged DOWN).
	mergedDown := false
	for _, tr := range strings.Split(html, "<tr>") {
		if strings.Contains(tr, "顶孤") && strings.Contains(tr, "B1") {
			mergedDown = true
		}
	}
	if !mergedDown {
		t.Errorf("ORPHAN-ROW BUG: top orphan '顶孤' should merge DOWN into the row containing 'B1'. HTML:\n%s", html)
	}
	t.Logf("HTML:\n%s", html)
}

// =============================================================================
// C) Sandwiched orphan KEPT (regression guard). GREEN on main and after fix.
// Row 2 is an orphan at col 0, but BOTH vertical neighbors (row 1 col 0 and
// row 3 col 0) are populated → 'sandwiched'. Python skips the merge
// (table_structure_recognizer.py:293-297). Must stay 4 rows.
// =============================================================================

func TestConstructTable_OrphanRow_SandwichedKept(t *testing.T) {
	grid := buildGrid([][]string{
		{"H0", "H1", "H2", "H3"},
		{"X0", "A1", "A2", "A3"},
		{"夹孤", "", "", ""},
		{"Z0", "D1", "D2", "D3"},
	})

	html := table.ConstructTable(nil, nil, "", &pdf.TableItem{Grid: grid})

	if got := trCount(html); got != 4 {
		t.Errorf("REGRESSION GUARD: sandwiched orphan must be KEPT (4 rows), got %d. Python skips merge when both vertical neighbors are populated.", got)
	}

	// '夹孤' must NOT be merged into a neighbor row.
	for _, tr := range strings.Split(html, "<tr>") {
		if strings.Contains(tr, "夹孤") && (strings.Contains(tr, "X0") || strings.Contains(tr, "Z0")) {
			t.Errorf("REGRESSION GUARD: sandwiched orphan '夹孤' must stay in its own row, not merge with 'X0'/'Z0'. HTML:\n%s", html)
		}
	}
	t.Logf("HTML:\n%s", html)
}

// =============================================================================
// F) Gate is on COLUMN count (>=4). Regression guard. GREEN on main and after fix.
// A 3-column table with an orphan middle row must be KEPT, because Python gates
// row cleanup on len(cols) >= 4 (columns), not rows. Guards the fix against
// using the wrong axis (as the column cleanup currently does).
// =============================================================================

func TestConstructTable_OrphanRow_GateNeeds4Columns(t *testing.T) {
	grid := buildGrid([][]string{
		{"H0", "H1", "H2"},
		{"", "A1", "A2"},
		{"三孤", "", ""},
		{"D0", "D1", "D2"},
	})

	html := table.ConstructTable(nil, nil, "", &pdf.TableItem{Grid: grid})

	if got := trCount(html); got != 4 {
		t.Errorf("REGRESSION GUARD: with only 3 columns, orphan row must be KEPT (4 rows). Python gates row cleanup on len(cols)>=4 (columns), not rows. Got %d.", got)
	}
	t.Logf("HTML:\n%s", html)
}

// =============================================================================
// D) Column cleanup gate is on the ROW count (>=4). Regression guard.
// Python gates column cleanup on `len(rows) >= 4` (construct_table:221), NOT
// unconditionally. So for a 2-row table Python does NOT run column cleanup and
// keeps the single-cell orphan column. The Go gate (len(rows) < 4) must be
// preserved to match; this guards against removing it (which would make Go
// drop orphan columns Python keeps).
// =============================================================================

func TestCleanupOrphanColumns_SmallTableKeepsOrphanColumn(t *testing.T) {
	grid := buildGrid([][]string{
		{"", "孤列", "A2"}, // col 1 is the orphan; left neighbor empty → mergeable
		{"B0", "", "B2"},
	})

	out := table.CleanupOrphanColumns(grid)

	if colCount(out) != 3 {
		t.Errorf("REGRESSION GUARD: a 2-row table must KEEP its orphan column (3 cols). Python gates column cleanup on len(rows)>=4 (construct_table:221), so it does not run here. Got %d columns.", colCount(out))
	}
	t.Logf("columns after cleanup: %d", colCount(out))
}

// D2) Column cleanup DOES run for >=4 rows and removes a single-cell orphan
// column. Positive counterpart to D: confirms the gate is row-count based, not
// removed entirely. col 1 has text only in row 0 (e==1 orphan); col 0/col 2
// have text in other rows → mergeable → column removed (3 → 2 cols).
func TestCleanupOrphanColumns_LargeTableRemovesOrphanColumn(t *testing.T) {
	grid := buildGrid([][]string{
		{"", "孤列", "A2"},
		{"B0", "", "B2"},
		{"C0", "", "C2"},
		{"D0", "", "D2"},
	})

	out := table.CleanupOrphanColumns(grid)

	if colCount(out) != 2 {
		t.Errorf("expected 2 columns after removing the single-cell orphan column for a >=4-row table (Python runs column cleanup when len(rows)>=4). Got %d columns.", colCount(out))
	}
	t.Logf("columns after cleanup: %d", colCount(out))
}

// =============================================================================
// E) item.Rows is stale after cleanup (related bug in the same function).
// 5-row, 3-column table with a single-cell orphan column (left neighbor empty
// in the populated row) → CleanupOrphanColumns removes it (3 → 2 cols).
// ConstructTable sets item.Rows (line 57) BEFORE CleanupOrphanColumns (line 59),
// so item.Rows keeps 3 cols while the HTML has 2. RED on main.
// =============================================================================

func TestConstructTable_RowsFieldReflectsCleanup(t *testing.T) {
	grid := buildGrid([][]string{
		{"", "孤列", "H2"},
		{"R1_0", "", "R1_2"},
		{"R2_0", "", "R2_2"},
		{"R3_0", "", "R3_2"},
		{"R4_0", "", "R4_2"},
	})

	item := &pdf.TableItem{Grid: grid}
	html := table.ConstructTable(nil, nil, "", item)

	htmlCols := 0
	if trCount(html) > 0 {
		firstRow := strings.Split(html, "<tr>")[1]
		htmlCols = strings.Count(firstRow, "<td ") + strings.Count(firstRow, "<th ")
	}

	if len(item.Rows) > 0 && len(item.Rows[0]) != htmlCols {
		t.Errorf("STALE item.Rows BUG: item.Rows[0] has %d cols but HTML row has %d cols. ConstructTable must derive item.Rows AFTER CleanupOrphanColumns.", len(item.Rows[0]), htmlCols)
	}
	// item.Grid must also reflect the cleaned grid: CleanupOrphanRows re-slices
	// the rows header, so a stale item.Grid would feed downstream consumers
	// (e.g. cross-page merge) the un-cleaned grid.
	if len(item.Grid) > 0 && len(item.Grid[0]) != htmlCols {
		t.Errorf("STALE item.Grid BUG: item.Grid[0] has %d cols but HTML row has %d cols. ConstructTable must re-assign item.Grid from the cleaned rows.", len(item.Grid[0]), htmlCols)
	}
	t.Logf("item.Grid cols=%d item.Rows cols=%d html cols=%d", len(item.Grid[0]), len(item.Rows[0]), htmlCols)
}
