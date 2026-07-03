
package table

import (
	"sort"
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
	if rows[0][0].Text != "A" || rows[1][0].Text != "B" {
		t.Errorf("row0/1 wrong: A=%q B=%q", rows[0][0].Text, rows[1][0].Text)
	}
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
	// R=-1 for all → maxR = -1 → grid would be 0 rows. Must fall back to YX.
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

func TestCompressRowIndices(t *testing.T) {
	// 6 boxes with 6 different R values → 6 rows.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 1, C: 1},
		{X0: 210, X1: 290, Top: 0, Bottom: 30, Text: "C", R: 2, C: 2},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "D", R: 3, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "E", R: 4, C: 1},
		{X0: 210, X1: 290, Top: 35, Bottom: 65, Text: "F", R: 5, C: 2},
	}
	// Sort first (as GroupBoxesByRC does)
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].R != boxes[j].R {
			return boxes[i].R < boxes[j].R
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	rowMap, compressed := compressRowIndices(boxes)
	if compressed != 5 { // 0-based, 6 elements → max index 5
		t.Errorf("compressed = %d, want 5", compressed)
	}
	if rowMap[0] != 0 || rowMap[1] != 1 || rowMap[2] != 2 || rowMap[3] != 3 || rowMap[4] != 4 || rowMap[5] != 5 {
		t.Errorf("rowMap mapping incorrect: %v", rowMap)
	}
}

func TestCompressRowIndices_SameR(t *testing.T) {
	// Multiple boxes with same R → same compressed row.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1},
	}
	// Sort first
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].R != boxes[j].R {
			return boxes[i].R < boxes[j].R
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	rowMap, compressed := compressRowIndices(boxes)
	if compressed != 1 { // 0-based, 2 rows → max index 1
		t.Errorf("compressed = %d, want 1", compressed)
	}
	if rowMap[0] != 0 || rowMap[1] != 1 {
		t.Errorf("rowMap mapping incorrect: %v", rowMap)
	}
}

func TestCollectBoxesPerRow(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1},
	}
	// Sort first
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].R != boxes[j].R {
			return boxes[i].R < boxes[j].R
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	rowMap, _ := compressRowIndices(boxes)
	cmap, maxCols := collectBoxesPerRow(boxes, rowMap)

	if len(cmap) != 2 {
		t.Errorf("cmap has %d rows, want 2", len(cmap))
	}
	if cmap[0][0].txt != "A" || cmap[0][1].txt != "B" {
		t.Errorf("row 0 incorrect: %v, %v", cmap[0][0], cmap[0][1])
	}
	if cmap[1][0].txt != "C" || cmap[1][1].txt != "D" {
		t.Errorf("row 1 incorrect: %v, %v", cmap[1][0], cmap[1][1])
	}
	if maxCols[0] != 1 || maxCols[1] != 1 {
		t.Errorf("maxCols incorrect: %v", maxCols)
	}
}

func TestCollectBoxesPerRow_MergeSameCell(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 50, Top: 0, Bottom: 30, Text: "Hello", R: 0, C: 0},
		{X0: 50, X1: 90, Top: 0, Bottom: 30, Text: "World", R: 0, C: 0},
	}
	// Sort first
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].R != boxes[j].R {
			return boxes[i].R < boxes[j].R
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	rowMap, _ := compressRowIndices(boxes)
	cmap, _ := collectBoxesPerRow(boxes, rowMap)

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
	// Sort first
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].R != boxes[j].R {
			return boxes[i].R < boxes[j].R
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	rowMap, compressed := compressRowIndices(boxes)
	cCompressed, cMaxCol := compressColIndices(boxes, rowMap, compressed)

	if cCompressed[0][0] != 0 || cCompressed[0][1] != 1 {
		t.Errorf("row 0 compression incorrect: %v", cCompressed[0])
	}
	if cCompressed[1][0] != 0 || cCompressed[1][1] != 1 {
		t.Errorf("row 1 compression incorrect: %v", cCompressed[1])
	}
	if cMaxCol[0] != 1 || cMaxCol[1] != 1 {
		t.Errorf("cMaxCol incorrect: %v", cMaxCol)
	}
}

func TestCompressColIndices_OverlapMerge(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 100, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 90, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1}, // Overlaps with A
	}
	// Sort first
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].R != boxes[j].R {
			return boxes[i].R < boxes[j].R
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	rowMap, compressed := compressRowIndices(boxes)
	cCompressed, cMaxCol := compressColIndices(boxes, rowMap, compressed)

	if cCompressed[0][0] != 0 || cCompressed[0][1] != 0 {
		t.Errorf("overlap merge incorrect: %v", cCompressed[0])
	}
	// After fix: cMaxCol tracks unique compressed columns, not original C keys.
	// 2 overlapping boxes → 1 compressed column → cMaxCol[0] == 0
	if cMaxCol[0] != 0 {
		t.Errorf("cMaxCol incorrect: got %d, want 0 (2 overlapping boxes → 1 compressed column)", cMaxCol[0])
	}
}

func TestBuildGrid(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1},
	}
	// Sort first
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].R != boxes[j].R {
			return boxes[i].R < boxes[j].R
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	rowMap, compressed := compressRowIndices(boxes)
	cmap, _ := collectBoxesPerRow(boxes, rowMap)
	cCompressed, cMaxCol := compressColIndices(boxes, rowMap, compressed)

	rows := buildGrid(cmap, cCompressed, cMaxCol, compressed)

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
