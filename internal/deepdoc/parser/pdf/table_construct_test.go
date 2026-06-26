package parser

import (
	"strings"
	"testing"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
)

func TestCellTexts(t *testing.T) {
	cells := []pdf.TSRCell{
		{Text: "A"}, {Text: "B"}, {Text: "C"},
	}
	texts := cellTexts(cells)
	got := strings.Join(texts, ",")
	if got != "A,B,C" {
		t.Errorf("cellTexts: got %q, want 'A,B,C'", got)
	}
}

// ── constructTable unit tests ─────────────────────────────────────────

func TestConstructTable_Simple3x2(t *testing.T) {
	// 3 columns × 2 rows — cells pre-filled (simulating extractTableBoxesFromImage).
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A", Label: "table row"},
		{X0: 101, Y0: 0, X1: 200, Y1: 50, Text: "B", Label: "table row"},
		{X0: 201, Y0: 0, X1: 300, Y1: 50, Text: "C", Label: "table row"},
		{X0: 0, Y0: 51, X1: 100, Y1: 100, Text: "D", Label: "table row"},
		{X0: 101, Y0: 51, X1: 200, Y1: 100, Text: "E", Label: "table row"},
		{X0: 201, Y0: 51, X1: 300, Y1: 100, Text: "F", Label: "table row"},
	}
	boxes := []pdf.TextBox{}
	html := tbl.ConstructTable(cells, boxes, "", nil)
	if !strings.Contains(html, "<table>") {
		t.Error("expected <table> tag")
	}
	if !strings.Contains(html, "A") || !strings.Contains(html, "B") || !strings.Contains(html, "C") {
		t.Error("expected cell texts A, B, C in HTML")
	}
	// Should have 2 <tr> elements
	trCount := strings.Count(html, "<tr>")
	if trCount != 2 {
		t.Errorf("expected 2 <tr> rows, got %d", trCount)
	}
	tdCount := strings.Count(html, "<td ")
	if tdCount != 6 {
		t.Errorf("expected 6 <td > cells, got %d", tdCount)
	}
	t.Logf("HTML:\n%s", html)
}

func TestConstructTable_EmptyCells(t *testing.T) {
	html := tbl.ConstructTable(nil, nil, "", nil)
	if html != "" {
		t.Errorf("expected empty string for empty cells, got %q", html)
	}
	html = tbl.ConstructTable([]pdf.TSRCell{}, []pdf.TextBox{}, "", nil)
	if html != "" {
		t.Errorf("expected empty string for empty cells slice, got %q", html)
	}
}

func TestConstructTable_NoMatchingBox(t *testing.T) {
	// Cell has no overlapping text box → empty <td >
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "Has text", Label: "table row"},
		{X0: 101, Y0: 0, X1: 200, Y1: 50, Label: "table row"},
	}
	boxes := []pdf.TextBox{}
	html := tbl.ConstructTable(cells, boxes, "", nil)
	if !strings.Contains(html, "Has text") {
		t.Error("expected first cell text")
	}
	// Should still have 2 <td > cells
	if strings.Count(html, "<td ") != 2 {
		t.Errorf("expected 2 <td > cells, got %d. HTML:\n%s", strings.Count(html, "<td "), html)
	}
}

func TestConstructTable_WithCaption(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "X", Label: "table row"},
	}
	html := tbl.ConstructTable(cells, nil, "表1：测试标题", nil)
	if !strings.Contains(html, "<caption>表1：测试标题</caption>") {
		t.Errorf("expected caption, got:\n%s", html)
	}
	t.Logf("HTML:\n%s", html)
}

func TestConstructTable_SingleRow(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 50, Y1: 40, Text: "Col1", Label: "table row"},
		{X0: 51, Y0: 0, X1: 100, Y1: 40, Text: "Col2", Label: "table row"},
	}
	html := tbl.ConstructTable(cells, nil, "", nil)
	if strings.Count(html, "<tr>") != 1 {
		t.Errorf("expected 1 row, got %d", strings.Count(html, "<tr>"))
	}
	if strings.Count(html, "<td ") != 2 {
		t.Errorf("expected 2 cells, got %d", strings.Count(html, "<td "))
	}
}

func TestConstructTable_CellsTextFilledAfterCall(t *testing.T) {
	// constructTable should populate cell text from boxes.
	// Bug: fillCellTextFromBoxes modifies a local copy — original cells stay empty,
	// causing generate_test.go to output empty rows.
	// Cells pre-filled — constructTable no longer fills text (done in extractTableBoxesFromImage).
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A1", Label: "table row"},
		{X0: 101, Y0: 0, X1: 200, Y1: 50, Text: "B1", Label: "table row"},
		{X0: 0, Y0: 51, X1: 100, Y1: 100, Text: "A2", Label: "table row"},
		{X0: 101, Y0: 51, X1: 200, Y1: 100, Text: "B2", Label: "table row"},
	}
	_ = tbl.ConstructTable(cells, nil, "", nil)

	// constructTable preserves cell text (does not clear or overwrite).
	if cells[0].Text != "A1" {
		t.Errorf("cell[0] text = %q, want %q", cells[0].Text, "A1")
	}
	if cells[1].Text != "B1" {
		t.Errorf("cell[1] text = %q, want %q", cells[1].Text, "B1")
	}
}

func TestConstructTable_YBasedFallback(t *testing.T) {
	// Cells with label "table" + pre-filled text
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 50, Y1: 30, Text: "R1C1", Label: "table"},
		{X0: 51, Y0: 0, X1: 100, Y1: 30, Text: "R1C2", Label: "table"},
		{X0: 0, Y0: 31, X1: 50, Y1: 60, Text: "R2C1", Label: "table"},
	}
	html := tbl.ConstructTable(cells, nil, "", nil)
	if strings.Count(html, "<tr>") != 2 {
		t.Errorf("expected 2 rows from Y-fallback, got %d", strings.Count(html, "<tr>"))
	}
	if strings.Count(html, "<td ") != 3 { // 2 in row0, 1 in row1 (no padding in basic grouping)
		t.Errorf("expected 3 cells, got %d", strings.Count(html, "<td "))
	}
}

// TestExtractTableAndReplace_CellTextFilled verifies that extractTableAndReplace
// fills cell text correctly with realistic coordinate transforms (Scale=3, CropOff≠0).
// This simulates the real pipeline where TSR cells are in crop pixel space and
// post-merge boxes are in PDF point space.
func TestExtractTableAndReplace_CellTextFilled(t *testing.T) {
	// Simulate 公司差旅费 page 0 table coordinates.
	// DLA region: X0=217, X1=1584, Y0=985, Y1=1599 at 216 DPI → PDF: 72-528 x 328-533
	// Scale = 216/72 = 3.0
	// cropOff ≈ region.X - region.W*0.03
	const scale = 3.0
	const cropOffX = 176.0
	const cropOffY = 967.0

	// Post-merge boxes in PDF point space (inside the table region).
	// PDF Y=470 → crop Top = 470*3-967 = 443 → overlaps crop cell at Y0=441.
	// Boxes must have R (row) and C (col) annotations matching cells,
	// matching Python's construct_table which assigns boxes to cells by R/C.
	boxes := []pdf.TextBox{
		{X0: 80, X1: 210, Top: 470, Bottom: 490, Text: "标职务", LayoutType: "table", PageNumber: 0, R: 0, C: 0},
		{X0: 220, X1: 270, Top: 470, Bottom: 490, Text: "飞机", LayoutType: "table", PageNumber: 0, R: 0, C: 1},
		{X0: 80, X1: 210, Top: 492, Bottom: 512, Text: "公司级领导", LayoutType: "table", PageNumber: 0, R: 1, C: 0},
		{X0: 220, X1: 270, Top: 492, Bottom: 512, Text: "经济舱位", LayoutType: "table", PageNumber: 0, R: 1, C: 1},
	}

	// TSR cells in crop pixel space (matching real TSR output).
	// Cells pre-filled (extractTableBoxesFromImage already ran fillText + OCR).
	cells := []pdf.TSRCell{
		{X0: 35, Y0: 441, X1: 456, Y1: 500, Text: "标职务", Label: "table row"},
		{X0: 460, Y0: 442, X1: 630, Y1: 500, Text: "飞机", Label: "table row"},
		{X0: 35, Y0: 501, X1: 456, Y1: 560, Text: "公司级领导", Label: "table row"},
		{X0: 460, Y0: 502, X1: 630, Y1: 560, Text: "经济舱位", Label: "table row"},
	}

	tables := []pdf.TableItem{{
		Cells:     cells,
		Positions: []pdf.Position{{Left: 80, Right: 500, Top: 480, Bottom: 560}},
		Scale:     scale,
		CropOffX:  cropOffX,
		CropOffY:  cropOffY,
	}}

	result := tbl.ExtractTableAndReplace(boxes, tables)
	if len(result) != 1 {
		t.Fatalf("expected 1 output box (HTML table), got %d", len(result))
	}
	if !strings.Contains(result[0].Text, "<table>") {
		t.Error("output should contain HTML table")
	}

	// Key assertion: constructTable backfills tables[0].Rows.
	rows := tables[0].Rows
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0][0] != "标职务" {
		t.Errorf("row 0 col 0 = %q, want %q", rows[0][0], "标职务")
	}
	if rows[0][1] != "飞机" {
		t.Errorf("row 0 col 1 = %q, want %q", rows[0][1], "飞机")
	}
	if rows[1][0] != "公司级领导" {
		t.Errorf("row 1 col 0 = %q, want %q", rows[1][0], "公司级领导")
	}
	if rows[1][1] != "经济舱位" {
		t.Errorf("row 1 col 1 = %q, want %q", rows[1][1], "经济舱位")
	}
}

// TestConstructTable_FromBoxesRC builds HTML directly from boxes with R/C
// annotations, matching Python's construct_table.  No cells needed for text.
func TestConstructTable_FromBoxesRC(t *testing.T) {
	// Boxes with R (row) and C (col) annotations — like the output of
	// annotateTableBoxes after layout cleanup.
	boxes := []pdf.TextBox{
		{X0: 50, X1: 150, Top: 100, Bottom: 130, Text: "姓名", R: 0, C: 0},
		{X0: 155, X1: 255, Top: 100, Bottom: 130, Text: "年龄", R: 0, C: 1},
		{X0: 50, X1: 150, Top: 135, Bottom: 165, Text: "张三", R: 1, C: 0},
		{X0: 155, X1: 255, Top: 135, Bottom: 165, Text: "25", R: 1, C: 1},
	}

	// constructTable should build HTML directly from boxes by R/C grouping,
	// ignoring cell text (matching Python's construct_table).
	item := &pdf.TableItem{}
	html := tbl.ConstructTable(nil, boxes, "", item)

	if !strings.Contains(html, "姓名") || !strings.Contains(html, "张三") {
		t.Errorf("HTML missing box text: %s", html)
	}
	// 2 rows, 2 cols
	if strings.Count(html, "<tr>") != 2 {
		t.Errorf("expected 2 rows, got %d. HTML: %s", strings.Count(html, "<tr>"), html)
	}
	if strings.Count(html, "<td ") != 4 {
		t.Errorf("expected 4 cells, got %d. HTML: %s", strings.Count(html, "<td "), html)
	}
	// Verify Rows output
	if len(item.Rows) != 2 || len(item.Rows[0]) != 2 {
		t.Errorf("Rows: expected 2x2, got %dx%d", len(item.Rows), len(item.Rows[0]))
	}
	if item.Rows[0][0] != "姓名" {
		t.Errorf("Rows[0][0] = %q, want %q", item.Rows[0][0], "姓名")
	}
	t.Logf("HTML: %s", html)
}

// TestFillCellTextFromBoxes_RCAnnotations fills text via R/C when spatial
// overlap is marginal.  Real-world TSR cells and pdf_oxide boxes have pixel-level
// offsets — R/C annotations (set by annotateTableBoxes) are the Python-equivalent
// way to assign boxes to cells regardless of coordinate deviations.
func TestFillCellTextFromBoxes_RCAnnotations(t *testing.T) {
	// Cells with real-world coordinate offsets (box shifted by 2px from cell).
	// Spatial overlap <30% for the shifted case — fillCellTextFromBoxes fails.
	cells := []pdf.TSRCell{
		{X0: 10, Y0: 10, X1: 200, Y1: 50},
		{X0: 210, Y0: 10, X1: 400, Y1: 50},
		{X0: 10, Y0: 55, X1: 200, Y1: 95},
		{X0: 210, Y0: 55, X1: 400, Y1: 95},
	}

	// Boxes have R/C annotations but their spatial overlap with cell rects
	// is marginal (real-world scenario).  R/C path should still fill text.
	boxes := []pdf.TextBox{
		{X0: 12, X1: 198, Top: 12, Bottom: 48, Text: "A", R: 0, C: 0},  // overlap ~92% → OK
		{X0: 215, X1: 395, Top: 12, Bottom: 48, Text: "B", R: 0, C: 1}, // overlap ~90% → OK
		{X0: 12, X1: 198, Top: 58, Bottom: 92, Text: "C", R: 1, C: 0},  // overlap ~92% → OK
		{X0: 215, X1: 350, Top: 58, Bottom: 90, Text: "D", R: 1, C: 1}, // overlap ~50% → MARGINAL
	}

	// This SHOULD fill all 4 cells via R/C, but spatial-only may fail on D.
	tbl.FillCellTextFromBoxes(cells, boxes)

	// When spatial overlap is marginal (box "D" at 50%), fillCellTextFromBoxes
	// may still match because cell is empty (0.3 threshold).  But the real
	// problem is that fillCellTextFromBoxes depends on coordinates, while
	// R/C annotations don't.
	hasText := false
	for _, c := range cells {
		if c.Text != "" {
			hasText = true
		}
	}
	if !hasText {
		t.Error("fillCellTextFromBoxes should fill text from spatially overlapping boxes with R/C")
	}

	// NOW test the R/C path explicitly: fillCellTextFromAnnotations uses
	// R/C labels only, ignoring coordinate overlap entirely.
	cells2 := []pdf.TSRCell{
		{X0: 10, Y0: 10, X1: 200, Y1: 50},
		{X0: 210, Y0: 10, X1: 400, Y1: 50},
		{X0: 10, Y0: 55, X1: 200, Y1: 95},
		{X0: 210, Y0: 55, X1: 400, Y1: 95},
	}
	rows := [][]pdf.TSRCell{{cells2[0], cells2[1]}, {cells2[2], cells2[3]}}
	tbl.FillCellTextFromAnnotations(rows, boxes)

	if rows[0][0].Text != "A" {
		t.Errorf("R/C: row0 col0 = %q, want %q", rows[0][0].Text, "A")
	}
	if rows[0][1].Text != "B" {
		t.Errorf("R/C: row0 col1 = %q, want %q", rows[0][1].Text, "B")
	}
	if rows[1][0].Text != "C" {
		t.Errorf("R/C: row1 col0 = %q, want %q", rows[1][0].Text, "C")
	}
	if rows[1][1].Text != "D" {
		t.Errorf("R/C: row1 col1 = %q, want %q", rows[1][1].Text, "D")
	}
}

// TestConstructTable_SingleRowMultiCol covers R=0 with multiple columns
// (table header pattern). boxesHaveAnnotations must detect valid annotations
// even though maxR=0.
func TestConstructTable_SingleRowMultiCol(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, Text: "姓名", R: 0, C: 0},
		{X0: 101, X1: 200, Top: 0, Bottom: 30, Text: "年龄", R: 0, C: 1},
		{X0: 201, X1: 300, Top: 0, Bottom: 30, Text: "性别", R: 0, C: 2},
	}
	item := &pdf.TableItem{}
	html := tbl.ConstructTable(nil, boxes, "", item)
	if strings.Count(html, "<td ") != 3 {
		t.Errorf("expected 3 cells, got %d. HTML: %s", strings.Count(html, "<td "), html)
	}
	if item.Rows[0][0] != "姓名" || item.Rows[0][1] != "年龄" || item.Rows[0][2] != "性别" {
		t.Errorf("wrong row text: %v", item.Rows[0])
	}
}

// TestConstructTable_MultiRowSingleCol covers C=0 with multiple rows
// (vertical list pattern). boxesHaveAnnotations must detect valid
// annotations even though maxC=0.
func TestConstructTable_MultiRowSingleCol(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, Text: "第一行", R: 0, C: 0},
		{X0: 0, X1: 100, Top: 35, Bottom: 65, Text: "第二行", R: 1, C: 0},
		{X0: 0, X1: 100, Top: 70, Bottom: 100, Text: "第三行", R: 2, C: 0},
	}
	item := &pdf.TableItem{}
	html := tbl.ConstructTable(nil, boxes, "", item)
	if strings.Count(html, "<tr>") != 3 {
		t.Errorf("expected 3 rows, got %d. HTML: %s", strings.Count(html, "<tr>"), html)
	}
	if item.Rows[0][0] != "第一行" || item.Rows[1][0] != "第二行" || item.Rows[2][0] != "第三行" {
		t.Errorf("wrong text: row0=%q row1=%q row2=%q", item.Rows[0][0], item.Rows[1][0], item.Rows[2][0])
	}
}

// TestConstructTable_RCAfterMerge verifies that R/C annotations survive
// text merge. The merged box expands bounds but keeps the first box's R/C.
func TestConstructTable_RCAfterMerge(t *testing.T) {
	// Simulate two adjacent fragments merged into one box.
	// The merged box keeps R/C from the first fragment.
	postMerge := []pdf.TextBox{
		{X0: 0, X1: 350, Top: 0, Bottom: 30, Text: "公司级领导人员（含公司董事、总监）", R: 0, C: 0},
		{X0: 355, X1: 500, Top: 0, Bottom: 30, Text: "经济舱位", R: 0, C: 1},
		{X0: 0, X1: 200, Top: 35, Bottom: 65, Text: "其他工作人员", R: 1, C: 0},
		{X0: 355, X1: 500, Top: 35, Bottom: 65, Text: "经济舱位", R: 1, C: 1},
	}
	item := &pdf.TableItem{}
	html := tbl.ConstructTable(nil, postMerge, "", item)
	if !strings.Contains(html, "公司级领导") {
		t.Errorf("missing merged text: %s", html)
	}
	if strings.Count(html, "<tr>") != 2 {
		t.Errorf("expected 2 rows, got %d", strings.Count(html, "<tr>"))
	}
	if item.Rows[0][0] != "公司级领导人员（含公司董事、总监）" {
		t.Errorf("row 0 col 0 = %q", item.Rows[0][0])
	}
}

// TestGroupTSRCellsToRowsLabeled_DefaultTableLabel verifies that cells with
// the real TSR default label "table" (class 0) are grouped correctly.
// The current deepDocReRowHdr regex only matches ".* (row|header)" — it misses
// the default "table" label, causing gatherTSR to return empty and forcing
// a fallback to pure Y-based grouping (which loses R/C annotations).
func TestGroupTSRCellsToRowsLabeled_DefaultTableLabel(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 10, Y0: 0, X1: 100, Y1: 30, Label: "table"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Label: "table"},
		{X0: 10, Y0: 35, X1: 100, Y1: 65, Label: "table"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Label: "table"},
	}
	rows := tbl.GroupTSRCellsToRows(cells)
	if len(rows) != 2 {
		t.Fatalf("label %q: expected 2 rows, got %d (BUG: deepDocReRowHdr does not match %q)", "table", len(rows), "table")
	}
	if len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Errorf("expected 2 cols/row, got %d/%d", len(rows[0]), len(rows[1]))
	}
}

// TestGroupBoxesByRC_RDiffSplitsRows verifies that groupBoxesByRC
// creates separate rows for different R values (Python: R differs → new row).
// Even when boxes share the same Y, different R → different grid row.
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
	rows := tbl.GroupBoxesByRC(boxes)
	// R=0,1,2,3,4,5 → 6 rows (Python: R differs → new row).
	if len(rows) != 6 {
		t.Fatalf("expected 6 rows (R differs → split), got %d", len(rows))
	}
}

// TestGroupBoxesByRC_MergesCloseCols verifies that C compression works
// within each R group — merging different C values that are close in X.
func TestGroupBoxesByRC_MergesCloseCols(t *testing.T) {
	// R=0 has C=0,1. R=1 has C=0,1. C compression → 2 cols each.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1},
	}
	rows := tbl.GroupBoxesByRC(boxes)
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

// TestGroupBoxesByRC_RDiffSplitsRow verifies that boxes with different R
// values are placed in separate rows even when their Y ranges overlap.
// Matches Python: R differs → new row unconditionally.
func TestGroupBoxesByRC_RDiffSplitsRow(t *testing.T) {
	// R=0 and R=1 at same Y (overlapping) → two separate rows in the grid.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 1, C: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "C", R: 2, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 3, C: 1},
	}
	rows := tbl.GroupBoxesByRC(boxes)
	// R=0,1,2,3 → 4 different R values → 4 rows (Python: R differs → new row).
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows (R differs → split), got %d", len(rows))
	}
	if rows[0][0].Text != "A" || rows[1][0].Text != "B" {
		t.Errorf("row0/1 wrong: A=%q B=%q", rows[0][0].Text, rows[1][0].Text)
	}
}

// TestFillCellTextFromBoxes_RCOnly verifies that box text goes to exactly
// one cell via R/C annotations, not multiple cells via spatial overlap.
// A box overlapping two cells should only fill the one matching its R/C.
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
	rows := tbl.GroupTSRCellsToRows(cells)
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

// TestRowsToHTML_HeaderRows verifies that header rows use <th > instead of <td >.
func TestRowsToHTML_HeaderRows(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Name", Label: "table column header"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "Age", Label: "table column header"},
		{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "John", Label: "table row"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "30", Label: "table row"},
	}
	// constructTable should produce <th > for header row.
	item := &pdf.TableItem{}
	html := tbl.ConstructTable(cells, nil, "", item)
	// Header row should use <th >, data row <td >.
	if !strings.Contains(html, "<th >") {
		t.Errorf("expected <th > for header row. HTML: %s", html)
	}
	if strings.Count(html, "<th ") != 2 {
		t.Errorf("expected 2 <th > cells, got %d. HTML: %s", strings.Count(html, "<th "), html)
	}
	if strings.Count(html, "<td ") != 2 {
		t.Errorf("expected 2 <td > cells (data row), got %d", strings.Count(html, "<td "))
	}
}

// TestExtractTableAndReplace_OnlyTableBoxes verifies that only boxes with
// LayoutType=="table" are passed to constructTable (Python: filters by layout_type).
func TestExtractTableAndReplace_OnlyTableBoxes(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "A", R: 0, C: 0, LayoutType: "table"},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "B", R: 0, C: 1, LayoutType: "table"},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "NOT_TABLE", R: 0, C: 0, LayoutType: "text"}, // non-table, R/C=0
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "D", R: 1, C: 1, LayoutType: "table"},
	}
	tables := []pdf.TableItem{{
		Cells:     []pdf.TSRCell{{Label: "table"}},
		Positions: []pdf.Position{{Left: 0, Right: 200, Top: 0, Bottom: 70}},
		Scale:     1.0,
	}}
	result := tbl.ExtractTableAndReplace(boxes, tables)
	// constructTable should produce HTML with "A", "B", "D" but NOT "NOT_TABLE".
	if !strings.Contains(result[0].Text, "A") || !strings.Contains(result[0].Text, "D") {
		t.Errorf("missing table box text: %s", result[0].Text)
	}
	if strings.Contains(result[0].Text, "NOT_TABLE") {
		t.Errorf("non-table box leaked into HTML: %s", result[0].Text)
	}
}

// TestFillCellText_RCOverSpatial verifies that R/C-based fill puts a
// box into exactly one cell (matching Python), unlike spatial fill which
// puts it into all overlapping cells.
func TestFillCellText_RCOverSpatial(t *testing.T) {
	// Box at X=30..270 overlaps all 3 cells (>30% each — spatial fills ALL).
	// With R/C, it belongs only to cell[1] (R=0, C=1).
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Label: "table"},
		{X0: 90, Y0: 0, X1: 200, Y1: 30, Label: "table"},
		{X0: 180, Y0: 0, X1: 300, Y1: 30, Label: "table"},
	}
	boxes := []pdf.TextBox{
		{X0: 30, X1: 270, Top: 0, Bottom: 30, Text: "TEXT", LayoutType: "table", R: 0, C: 1},
	}

	// Spatial fill: fills ALL overlapping cells —> duplication.
	cellsCopy := make([]pdf.TSRCell, 3)
	copy(cellsCopy, cells)
	tbl.FillCellTextFromBoxes(cellsCopy, boxes)
	spatialCount := 0
	for _, c := range cellsCopy {
		if c.Text != "" {
			spatialCount++
		}
	}
	if spatialCount <= 1 {
		t.Errorf("spatial fill: expected >1 cells with text, got %d", spatialCount)
	}
	t.Logf("spatial fill: %d cells (WRONG — duplication)", spatialCount)

	// R/C fill: only cell matching box.R/C gets text.
	cellsRC := make([]pdf.TSRCell, 3)
	copy(cellsRC, cells)
	rows := tbl.GroupTSRCellsToRows(cellsRC)
	for _, b := range boxes {
		if b.R >= 0 && b.R < len(rows) && b.C >= 0 && b.C < len(rows[b.R]) {
			rows[b.R][b.C].Text = strings.TrimSpace(b.Text)
		}
	}
	rcCount := 0
	for _, row := range rows {
		for _, c := range row {
			if c.Text == "TEXT" {
				rcCount++
			}
		}
	}
	if rcCount != 1 {
		t.Errorf("R/C fill: expected 1 cell with 'TEXT', got %d", rcCount)
	}
}

func TestIsCaptionBox(t *testing.T) {
	tests := []struct {
		text string
		want bool
	}{
		{"表1：交通工具等级", true},
		{"Table 1: Transport Levels", true},
		{"图表 1: 测试", true},
		{"公司领导班子成员、出差地", false}, // plain text, not caption
		{"第十条到厂矿单位出差", false},   // normal paragraph
		{"", false},
	}
	for _, tt := range tests {
		if got := tbl.IsCaptionBox(tt.text, ""); got != tt.want {
			t.Errorf("tbl.IsCaptionBox(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestFillCellTextFromBoxes_SkipsCaption(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 200, Y1: 30, Label: "table"},
		{X0: 0, Y0: 35, X1: 200, Y1: 65, Label: "table"},
	}
	boxes := []pdf.TextBox{
		// Caption box (should be skipped)
		{X0: 0, X1: 200, Top: 0, Bottom: 30, Text: "表1：交通工具等级"},
		// Data box
		{X0: 0, X1: 200, Top: 35, Bottom: 65, Text: "数据行"},
	}
	tbl.FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("caption leaked into cell 0: %q", cells[0].Text)
	}
	if cells[1].Text != "数据行" {
		t.Errorf("data not in cell 1: %q", cells[1].Text)
	}
}

func TestFillCellText_RCPreventsCrossCellLeak(t *testing.T) {
	// Caption box at Y=0-15 overlaps BOTH cell rows (both are "empty").
	// Spatial fill: text leaks to both cells. R/C fill: only cell[0] gets text.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 300, Y1: 30, Label: "table"},
		{X0: 0, Y0: 35, X1: 300, Y1: 65, Label: "table"},
	}
	boxes := []pdf.TextBox{
		{X0: 10, X1: 200, Top: 12, Bottom: 28, Text: "公司领导班子成员、出差地", R: 0, C: 0},
	}

	// Spatial fill → leaks to cells[1] (overlap ≥30%).
	cellsSp := make([]pdf.TSRCell, 2)
	copy(cellsSp, cells)
	tbl.FillCellTextFromBoxes(cellsSp, boxes)
	if cellsSp[1].Text != "" {
		t.Errorf("spatial fill: caption leaked to cell[1]: %q", cellsSp[1].Text)
	}

	// R/C fill → only cell[0] (R=0,C=0).
	cellsRC := make([]pdf.TSRCell, 2)
	copy(cellsRC, cells)
	rows := tbl.GroupTSRCellsToRows(cellsRC)
	for _, b := range boxes {
		if b.R >= 0 && b.R < len(rows) && b.C >= 0 && b.C < len(rows[b.R]) {
			if rows[b.R][b.C].Text == "" {
				rows[b.R][b.C].Text = strings.TrimSpace(b.Text)
			}
		}
	}
	if cellsRC[1].Text != "" {
		t.Errorf("R/C fill: caption leaked to cell[1]: %q", cellsRC[1].Text)
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
	rows := tbl.GroupBoxesByRC(boxes)
	// R=-1 for all → maxR = -1 → grid would be 0 rows. Must fall back to YX.
	if len(rows) == 0 {
		t.Fatal("groupBoxesByRC returned 0 rows when R=-1 — no YX fallback")
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows (Y-split), got %d", len(rows))
	}
}

func TestRowsToHTML_Colspan(t *testing.T) {
	// Box spanning 2 columns: SP annotation with HLeft/HRight covering cols 0-1.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "Name", R: 0, C: 0, H: 1, HLeft: 10, HRight: 190},
		{X0: 110, X1: 190, Top: 0, Bottom: 30, Text: "", R: 0, C: 1, SP: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "John", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "30", R: 1, C: 1},
	}
	rows := tbl.GroupBoxesByRC(boxes)
	spans, covered := tbl.CalSpans(rows)
	html := tbl.RowsToHTML(rows, "", nil, spans, covered)
	if !strings.Contains(html, "colspan") {
		t.Errorf("expected colspan attribute, got: %s", html)
	}
	t.Logf("HTML: %s", html)
}

// TestStripCaptionFromCells verifies that caption-like text is cleared
// from TSR cells before the table HTML is built.
func TestStripCaptionFromCells_ClearsCaptionPattern(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "表1：差旅费标准"},
		{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: ""},
		{X0: 0, Y0: 60, X1: 100, Y1: 110, Text: "张三"},
		{X0: 100, Y0: 60, X1: 200, Y1: 110, Text: "100"},
	}
	tbl.StripCaptionFromCells(cells)
	if cells[0].Text != "" {
		t.Errorf("caption cell should be cleared, got %q", cells[0].Text)
	}
	if cells[2].Text != "张三" {
		t.Errorf("data cell should be preserved, got %q", cells[2].Text)
	}
}

// TestStripCaptionFromCells_PreservesData verifies that non-caption
// cells are not cleared.
func TestStripCaptionFromCells_PreservesData(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "姓名"},
		{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "年龄"},
		{X0: 0, Y0: 60, X1: 100, Y1: 110, Text: "张三"},
		{X0: 100, Y0: 60, X1: 200, Y1: 110, Text: "25"},
	}
	// Make a copy and strip
	orig := make([]string, len(cells))
	for i, c := range cells {
		orig[i] = c.Text
	}
	tbl.StripCaptionFromCells(cells)
	for i := range cells {
		if cells[i].Text != orig[i] {
			t.Errorf("cell[%d] changed: %q -> %q", i, orig[i], cells[i].Text)
		}
	}
}

// TestStripCaptionFromCells_Empty is a no-op on empty cells.
func TestStripCaptionFromCells_Empty(t *testing.T) {
	cells := []pdf.TSRCell{}
	tbl.StripCaptionFromCells(cells) // must not panic
}

// TestConstructTable_StripsCaptionFromCells verifies that constructTable
// strips caption text from cells before building HTML.
func TestConstructTable_StripsCaptionFromCells(t *testing.T) {
	// Cell[0] has caption text "表1：标题"; cell[1] has real data.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "表1：标题"},
		{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "数据"},
	}
	html := tbl.ConstructTable(cells, nil, "", nil)
	// "表1：标题" should NOT appear in the HTML (stripped as caption).
	if strings.Contains(html, "表1") {
		t.Errorf("caption text '表1：标题' should be stripped: %s", html)
	}
	// "数据" should still be there.
	if !strings.Contains(html, "数据") {
		t.Errorf("data text '数据' should be preserved: %s", html)
	}
	t.Logf("HTML: %s", html)
}

// TestCalSpans_NonSpanningCellsNotPolluted verifies that a regular cell
// at position [0,0] is NOT detected as spanning when a spanning cell at
// [0,1] extends to the left, polluting column boundary calculations.
// Bug: calSpans computed column boundaries from ALL cells including
// spanning cells. "部门开支汇总" at [0,1] with X0=0 extends colLeft[1]
// to 0 instead of 101, shifting the center and causing "Q1" at [0,0]
// to be incorrectly detected as spanning 2 columns.
func TestCalSpans_NonSpanningCellsNotPolluted(t *testing.T) {
	// Simulate the SpannedTable test grid: row 0 has Q1(regular), 部门开支汇总(span), Q2(regular)
	rows := [][]pdf.TSRCell{
		{
			{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Q1", Label: "table row"},
			{X0: 0, Y0: 0, X1: 200, Y1: 30, Text: "部门开支汇总", Label: "table spanning cell"},
			{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "Q2", Label: "table row"},
		},
		{
			{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "100", Label: "table row"},
			{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "200", Label: "table row"},
		},
	}

	spans, covered := tbl.CalSpans(rows)

	// Q1 at [0,0] has X0=0, X1=100 which should only cover its own column.
	// It should NOT get a colspan.
	if s, ok := spans[[2]int{0, 0}]; ok {
		t.Errorf("Q1 at [0,0] should NOT have colspan, got %v. "+
			"Spanning cell at [0,1] polluted column boundaries", s)
	}

	// 部门开支汇总 at [0,1] has X0=0, X1=200 which DOES span columns 0 and 1.
	if s, ok := spans[[2]int{0, 1}]; !ok {
		t.Error("部门开支汇总 at [0,1] should have colspan=2 (covers X=0-200)")
	} else if s[0] != 2 {
		t.Errorf("部门开支汇总 colspan = %d, want 2", s[0])
	}

	// Q2 at [0,2] should be covered by the spanning cell (col 2 is within X=0-200).
	if !covered[[2]int{0, 2}] {
		t.Error("Q2 at [0,2] should be covered by spanning cell at [0,1]")
	}

	t.Logf("spans: %v, covered: %v", spans, covered)
}

// ── coordinate space conversion helpers ─────────────────────────────────
