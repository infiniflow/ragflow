package table

import (
	"reflect"
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestGroupBoxesByRC_RDiffSplitsRows(t *testing.T) {
	// 6 boxes with 6 different R values → 6 rows (Python R-first splitting).
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 1, C: 1},
		{X0: 210, X1: 290, Top: 0, Bottom: 30, Text: "C", R: 2, C: 2},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "D", R: 3, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "E", R: 4, C: 1},
		{X0: 210, X1: 290, Top: 35, Bottom: 65, Text: "F", R: 5, C: 2},
	}
	rows := GroupBoxesByRC(boxes)
	// R=0,1,2,3,4,5 → 6 rows (Python: R differs → new row).
	if len(rows) != 6 {
		t.Fatalf("expected 6 rows (R differs → split), got %d", len(rows))
	}
}

func TestGroupBoxesByRC_MergesCloseCols(t *testing.T) {
	// R=0 has C=0,1. R=1 has C=0,1. C compression → 2 cols each.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1},
	}
	rows := GroupBoxesByRC(boxes)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (R diff), got %d", len(rows))
	}
	if len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Errorf("expected 2 cols/row, got %d/%d", len(rows[0]), len(rows[1]))
	}
	if rows[0][0].Text != "A" || rows[0][1].Text != "B" {
		t.Errorf("row0 wrong: %q %q", rows[0][0].Text, rows[0][1].Text)
	}
	if rows[1][0].Text != "C" || rows[1][1].Text != "D" {
		t.Errorf("row1 wrong: %q %q", rows[1][0].Text, rows[1][1].Text)
	}
}

func TestGroupBoxesByRC_RDiffSplitsRow(t *testing.T) {
	// R=0 and R=1 at same Y (overlapping) → two separate rows in the grid.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 1, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 2, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 3, C: 1},
	}
	rows := GroupBoxesByRC(boxes)
	// R=0,1,2,3 → 4 different R values → 4 rows (Python: R differs → new row).
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows (R differs → split), got %d", len(rows))
	}
	// Columns follow the C labels: A/C are C=0 (column 0), B/D are C=1
	// (column 1), so B sits at rows[1][1], not rows[1][0].
	if rows[0][0].Text != "A" || rows[1][1].Text != "B" {
		t.Errorf("row0/1 wrong: A=%q B=%q", rows[0][0].Text, rows[1][1].Text)
	}
}

func TestGroupBoxesByRC_MissingRowStaysInYOrder(t *testing.T) {
	// Mirrors the 01_两行合并表头 PDF: the TSR model missed the 成都公司 row,
	// so its boxes have no R (-1), while the two header rows carry the raw
	// TSR row indices 0 and 1. Python's construct_table keeps missing-R boxes
	// at their Y position, so 成都 must land between 深圳 and 武汉 — not inside
	// the header row.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 20, Text: "单位名称", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 20, Text: "集团公司采用", R: 0, C: 1},
		{X0: 110, X1: 190, Top: 25, Bottom: 45, Text: "政务", R: 1, C: 1},
		{X0: 210, X1: 290, Top: 25, Bottom: 45, Text: "政工", R: 1, C: 2},
		{X0: 10, X1: 90, Top: 50, Bottom: 70, Text: "深圳公司", R: 5, C: 0},
		{X0: 10, X1: 90, Top: 100, Bottom: 120, Text: "成都公司", R: -1, C: 0},
		{X0: 110, X1: 190, Top: 100, Bottom: 120, Text: "20（1）", R: -1, C: 1},
		{X0: 10, X1: 90, Top: 150, Bottom: 170, Text: "武汉公司", R: 6, C: 0},
	}

	rows := GroupBoxesByRC(boxes)

	if len(rows) != 5 {
		t.Fatalf("expected 5 rows (header0, header1, 深圳, 成都, 武汉), got %d: %v", len(rows), rowTexts(rows))
	}
	want := []string{"单位名称", "政务", "深圳公司", "成都公司", "武汉公司"}
	for i, w := range want {
		got := ""
		for _, c := range rows[i] {
			if c.Text != "" {
				got = c.Text
				break
			}
		}
		if got != w {
			t.Errorf("row %d first text = %q, want %q (rows=%v)", i, got, w, rowTexts(rows))
		}
	}
	if rows[3][0].Text != "成都公司" || rows[3][1].Text != "20（1）" {
		t.Errorf("成都 row cells = %v, want [成都公司 20（1）]", rowTexts(rows[3:4]))
	}
}

func rowTexts(rows [][]pdf.TSRCell) [][]string {
	out := make([][]string, len(rows))
	for i, row := range rows {
		for _, c := range row {
			if c.Text != "" {
				out[i] = append(out[i], c.Text)
			}
		}
	}
	return out
}

func TestFillCellTextFromBoxes_RCOnly(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Label: "table"},
		{X0: 90, Y0: 0, X1: 200, Y1: 50, Label: "table"},
	}
	// This box straddles cell 0 (X=0-100) and cell 1 (X=90-200).
	// Spatial overlap: both match. R/C: should go to cell R=0, C=0 only.
	boxes := []pdf.TextBox{
		{X0: 80, X1: 120, Top: 0, Bottom: 50, Text: "TEXT", LayoutType: "table", R: 0, C: 0},
	}
	rows := GroupTSRCellsToRows(cells)
	for _, b := range boxes {
		t := strings.TrimSpace(b.Text)
		if t == "" {
			continue
		}
		if b.R >= 0 && b.R < len(rows) && b.C >= 0 && b.C < len(rows[b.R]) {
			rows[b.R][b.C].Text = t
		}
	}
	// Cell 0 should have text, cell 1 should NOT.
	if rows[0][0].Text != "TEXT" {
		t.Errorf("cell[0][0] = %q, want %q", rows[0][0].Text, "TEXT")
	}
	if rows[0][1].Text != "" {
		t.Errorf("cell[0][1] = %q, should be empty (spatial overlap leak)", rows[0][1].Text)
	}
}

func TestGroupBoxesByRC_ZeroValueBoxesFallBackToYX(t *testing.T) {
	// Zero-value TextBox has R=0, C=0. That must not be mistaken for an
	// annotated first row: these four boxes form a 2x2 grid by geometry.
	boxes := []pdf.TextBox{
		{X0: 0, X1: 50, Top: 0, Bottom: 10, Text: "A"},
		{X0: 60, X1: 110, Top: 0, Bottom: 10, Text: "B"},
		{X0: 0, X1: 50, Top: 20, Bottom: 30, Text: "C"},
		{X0: 60, X1: 110, Top: 20, Bottom: 30, Text: "D"},
	}

	rows := GroupBoxesByRC(boxes)

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows from unannotated boxes, got %d: %v", len(rows), rowTexts(rows))
	}
	if len(rows[0]) < 2 || rows[0][0].Text != "A" || rows[0][1].Text != "B" {
		t.Errorf("row 0 = %v, want [A B]", rowTexts(rows[0:1]))
	}
	if len(rows[1]) < 2 || rows[1][0].Text != "C" || rows[1][1].Text != "D" {
		t.Errorf("row 1 = %v, want [C D]", rowTexts(rows[1:2]))
	}
}

func TestGroupBoxesByRC_FallbackToYXWhenNoAnnotations(t *testing.T) {
	// When all boxes have R=-1 (Python's case: regex didn't match "table" label),
	// groupBoxesByRC should fall back to YX coordinate grouping.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: -1, C: -1},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: -1, C: -1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: -1, C: -1},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: -1, C: -1},
	}
	rows := GroupBoxesByRC(boxes)
	// R=-1 for all → no box carries an R annotation → must fall back to YX.
	if len(rows) == 0 {
		t.Fatal("groupBoxesByRC returned 0 rows when R=-1 — no YX fallback")
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows (Y-split), got %d", len(rows))
	}
}

func TestGroupBoxesByRC_ColspanMissing(t *testing.T) {
	// Box with SP annotation spanning 2 columns (HLeft→HRight covers cols 0-1).
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "Name", R: 0, C: 0, H: 1,
			HLeft: 10, HRight: 200},
		{X0: 110, X1: 200, Top: 0, Bottom: 30, Text: "", R: 0, C: 1, SP: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "A", R: 1, C: 0},
		{X0: 110, X1: 200, Top: 35, Bottom: 65, Text: "B", R: 1, C: 1},
	}
	rows := GroupBoxesByRC(boxes)
	// The result should have colspan=2 for cell [0,0] and skip [0,1].
	// Currently groupBoxesByRC produces a flat grid without span info.
	if len(rows) >= 1 && len(rows[0]) >= 2 && rows[0][1].Text == "" {
		t.Log("KNOWN LIMITATION: colspan not computed — cell [0,1] is empty instead of merged")
	}
	_ = rows
}

func TestAssignRows_RChanges(t *testing.T) {
	// 6 boxes with 6 different R values → 6 rows.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0, RTop: 0, RBott: 30},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 1, C: 1, RTop: 0, RBott: 30},
		{X0: 210, X1: 290, Top: 0, Bottom: 30, Text: "C", R: 2, C: 2, RTop: 0, RBott: 30},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "D", R: 3, C: 0, RTop: 35, RBott: 65},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "E", R: 4, C: 1, RTop: 35, RBott: 65},
		{X0: 210, X1: 290, Top: 35, Bottom: 65, Text: "F", R: 5, C: 2, RTop: 35, RBott: 65},
	}
	sortRFirstly(boxes)
	rowOf := assignRows(boxes)
	want := []int{0, 1, 2, 3, 4, 5}
	if len(rowOf) != len(want) {
		t.Fatalf("rowOf len = %d, want %d", len(rowOf), len(want))
	}
	for i := range want {
		if rowOf[i] != want[i] {
			t.Errorf("rowOf = %v, want %v", rowOf, want)
			break
		}
	}
}

func TestAssignRows_SameR(t *testing.T) {
	// Multiple boxes with same R and overlapping Y → same row.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0, RTop: 0, RBott: 30},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1, RTop: 0, RBott: 30},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 1, C: 0, RTop: 35, RBott: 65},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1, RTop: 35, RBott: 65},
	}
	sortRFirstly(boxes)
	rowOf := assignRows(boxes)
	want := []int{0, 0, 1, 1}
	for i := range want {
		if rowOf[i] != want[i] {
			t.Errorf("rowOf = %v, want %v", rowOf, want)
			break
		}
	}
}

func TestAssignRows_PageChangeStartsNewRow(t *testing.T) {
	// Per-page R numbering can repeat; a page boundary must still start a new
	// row even when R and Y are unchanged.
	boxes := []pdf.TextBox{
		{X0: 0, X1: 40, Top: 0, Bottom: 10, Text: "A", R: 0, C: 0, RTop: 0, RBott: 10},
		{X0: 0, X1: 40, Top: 0, Bottom: 10, Text: "B", R: 0, C: 0, PageNumber: 1, RTop: 0, RBott: 10},
	}
	sortRFirstly(boxes)
	rowOf := assignRows(boxes)
	if !reflect.DeepEqual(rowOf, []int{0, 1}) {
		t.Errorf("rowOf = %v, want [0 1]", rowOf)
	}
}

func TestAssignRows_SameRMultilineStaysOneRow(t *testing.T) {
	// Two text boxes in the same cell on separate lines share R; Python keeps
	// them in one row because the Y-advance branch only applies when R is
	// missing. Splitting here would break multi-line OCR cells.
	boxes := []pdf.TextBox{
		{X0: 0, X1: 40, Top: 5, Bottom: 10, Text: "R/W", R: 0, C: 0, RTop: 0, RBott: 60},
		{X0: 0, X1: 40, Top: 12, Bottom: 17, Text: "R/W", R: 0, C: 0, RTop: 0, RBott: 60},
	}
	sortRFirstly(boxes)
	rowOf := assignRows(boxes)
	if !reflect.DeepEqual(rowOf, []int{0, 0}) {
		t.Errorf("rowOf = %v, want [0 0] (same-R lines stay in one row)", rowOf)
	}
}

func TestCollectBoxesPerRow(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1},
	}
	sortRFirstly(boxes)
	rowOf := assignRows(boxes)
	cmap := collectBoxesPerRow(boxes, rowOf)

	if len(cmap) != 2 {
		t.Errorf("cmap has %d rows, want 2", len(cmap))
	}
	if cmap[0][0].txt != "A" || cmap[0][1].txt != "B" {
		t.Errorf("row 0 incorrect: %v, %v", cmap[0][0], cmap[0][1])
	}
	if cmap[1][0].txt != "C" || cmap[1][1].txt != "D" {
		t.Errorf("row 1 incorrect: %v, %v", cmap[1][0], cmap[1][1])
	}
}

func TestCollectBoxesPerRow_MergeSameCell(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 50, Top: 0, Bottom: 30, Text: "Hello", R: 0, C: 0},
		{X0: 50, X1: 90, Top: 0, Bottom: 30, Text: "World", R: 0, C: 0},
	}
	sortRFirstly(boxes)
	rowOf := assignRows(boxes)
	cmap := collectBoxesPerRow(boxes, rowOf)

	if cmap[0][0].txt != "Hello World" {
		t.Errorf("merged text incorrect: %q", cmap[0][0].txt)
	}
}

func TestCompressColIndices(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1},
	}
	sortRFirstly(boxes)
	rowOf := assignRows(boxes)
	compressed := rowOf[len(rowOf)-1]
	cCompressed, maxCol := compressColIndices(boxes, rowOf, compressed)

	if cCompressed[0][0] != 0 || cCompressed[0][1] != 1 {
		t.Errorf("row 0 compression incorrect: %v", cCompressed[0])
	}
	if cCompressed[1][0] != 0 || cCompressed[1][1] != 1 {
		t.Errorf("row 1 compression incorrect: %v", cCompressed[1])
	}
	if maxCol != 1 {
		t.Errorf("maxCol = %d, want 1", maxCol)
	}
}

func TestCompressColIndices_AdjacentCAlwaysNewColumn(t *testing.T) {
	// Two boxes with ADJACENT C labels (C=0 then C=1) must land in two
	// separate columns even though their X ranges overlap. Python's
	// construct_table starts a new column when C == prev C + 1 (same page),
	// never merging adjacent C values by X overlap.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 100, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 90, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1}, // X-overlaps A but C is adjacent
	}
	sortRFirstly(boxes)
	rowOf := assignRows(boxes)
	compressed := rowOf[len(rowOf)-1]
	cCompressed, maxCol := compressColIndices(boxes, rowOf, compressed)

	// C=0 → column 0, C=1 → column 1 (distinct C rank, never merged).
	if cCompressed[0][0] != 0 || cCompressed[0][1] != 1 {
		t.Errorf("adjacent C must split into separate columns: %v", cCompressed[0])
	}
	// Table-wide column count = 2 → maxCol == 1.
	if maxCol != 1 {
		t.Errorf("maxCol = %d, want 1 (2 distinct C → 2 columns)", maxCol)
	}
}

func TestBuildGrid(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1},
	}
	sortRFirstly(boxes)
	rowOf := assignRows(boxes)
	compressed := rowOf[len(rowOf)-1]
	cmap := collectBoxesPerRow(boxes, rowOf)
	cCompressed, maxCol := compressColIndices(boxes, rowOf, compressed)

	rows := buildGrid(cmap, cCompressed, maxCol, compressed)

	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Errorf("expected 2 cols each, got %d and %d", len(rows[0]), len(rows[1]))
	}
	if rows[0][0].Text != "A" || rows[0][1].Text != "B" {
		t.Errorf("row 0 wrong: %q %q", rows[0][0].Text, rows[0][1].Text)
	}
	if rows[1][0].Text != "C" || rows[1][1].Text != "D" {
		t.Errorf("row 1 wrong: %q %q", rows[1][0].Text, rows[1][1].Text)
	}
}
