package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
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

func TestConstructTable_Simple3x2(t *testing.T) {
	// 3 columns × 2 rows — cells pre-filled (simulating enrichOnePageWithDeepDoc).
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A", Label: "table row"},
		{X0: 101, Y0: 0, X1: 200, Y1: 50, Text: "B", Label: "table row"},
		{X0: 201, Y0: 0, X1: 300, Y1: 50, Text: "C", Label: "table row"},
		{X0: 0, Y0: 51, X1: 100, Y1: 100, Text: "D", Label: "table row"},
		{X0: 101, Y0: 51, X1: 200, Y1: 100, Text: "E", Label: "table row"},
		{X0: 201, Y0: 51, X1: 300, Y1: 100, Text: "F", Label: "table row"},
	}
	boxes := []pdf.TextBox{}
	html := ConstructTable(cells, boxes, "", nil)
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

func TestConstructTable_GroupsFallbackBoxesByPage(t *testing.T) {
	item := pdf.TableItem{
		Positions: []pdf.Position{
			{PageNumbers: []int{0}},
			{PageNumbers: []int{1}},
		},
		Cells: []pdf.TSRCell{
			{Text: "A", X0: 0, Y0: 10, X1: 80, Y1: 20},
			{Text: "B", X0: 100, Y0: 10, X1: 180, Y1: 20},
			{Text: "C", X0: 0, Y0: 30, X1: 80, Y1: 40},
			{Text: "D", X0: 100, Y0: 30, X1: 180, Y1: 40},
			{Text: "E", X0: 0, Y0: 10, X1: 80, Y1: 20},
			{Text: "F", X0: 100, Y0: 10, X1: 180, Y1: 20},
			{Text: "G", X0: 0, Y0: 30, X1: 80, Y1: 40},
			{Text: "H", X0: 100, Y0: 30, X1: 180, Y1: 40},
		},
	}
	boxes := []pdf.TextBox{
		{Text: "A", X0: 0, X1: 80, Top: 10, Bottom: 20, PageNumber: 0, HasPageNumber: true, R: 0, C: 0, RTop: 10, RBott: 20},
		{Text: "B", X0: 100, X1: 180, Top: 10, Bottom: 20, PageNumber: 0, HasPageNumber: true, R: 0, C: 1, RTop: 10, RBott: 20},
		{Text: "C", X0: 0, X1: 80, Top: 30, Bottom: 40, PageNumber: 0, HasPageNumber: true, R: 1, C: 0, RTop: 30, RBott: 40},
		{Text: "D", X0: 100, X1: 180, Top: 30, Bottom: 40, PageNumber: 0, HasPageNumber: true, R: 1, C: 1, RTop: 30, RBott: 40},
		{Text: "E", X0: 0, X1: 80, Top: 10, Bottom: 20, PageNumber: 1, HasPageNumber: true, R: 0, C: 0, RTop: 10, RBott: 20},
		{Text: "F", X0: 100, X1: 180, Top: 10, Bottom: 20, PageNumber: 1, HasPageNumber: true, R: 0, C: 1, RTop: 10, RBott: 20},
		{Text: "G", X0: 0, X1: 80, Top: 30, Bottom: 40, PageNumber: 1, HasPageNumber: true, R: 1, C: 0, RTop: 30, RBott: 40},
		{Text: "H", X0: 100, X1: 180, Top: 30, Bottom: 40, PageNumber: 1, HasPageNumber: true, R: 1, C: 1, RTop: 30, RBott: 40},
	}

	ConstructTable(nil, boxes, "", &item)

	want := [][]string{{"A", "B"}, {"C", "D"}, {"E", "F"}, {"G", "H"}}
	if len(item.Rows) != len(want) {
		t.Fatalf("got %d rows, want %d: %v", len(item.Rows), len(want), item.Rows)
	}
	for i := range want {
		if strings.Join(item.Rows[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("row %d = %v, want %v", i, item.Rows[i], want[i])
		}
	}
}

func TestConstructTable_EmptyGridFallsBackToCells(t *testing.T) {
	item := pdf.TableItem{Grid: make([][]pdf.TSRCell, 0)}
	cells := []pdf.TSRCell{{Text: "value", X0: 0, Y0: 0, X1: 80, Y1: 20}}
	html := ConstructTable(cells, nil, "", &item)
	if !strings.Contains(html, ">value<") {
		t.Fatalf("empty grid lost cell text: %s", html)
	}
}

// TestConstructTable_DropsAllEmptyRow pins Python parity: when the
// TSR-derived grid carries a row that no text box landed in
// (e.g. an extra "table row" detected next to a "table projected row
// header" on a cross-page table's continuation page), Python's
// construct_table builds rows from box.R grouping and never emits that
// row, while Go's grid carries every TSR row and previously kept the
// empty row — leaking it into item.Rows and the HTML table.
//
// Repro fixture from 13_crosspage_table.pdf page 2: the first
// "table row" (y0=885) sits on top of a "table projected row header"
// with no OCR box overlap. Go's GroupCells keeps it as a 5-cell
// row, all empty; this test pins the drop.
func TestConstructTable_DropsAllEmptyRow(t *testing.T) {
	mkRow := func(y0 float64, texts ...string) []pdf.TSRCell {
		row := make([]pdf.TSRCell, 5)
		for c := range row {
			row[c] = pdf.TSRCell{X0: float64(c * 100), Y0: y0, X1: float64(c*100 + 100), Y1: y0 + 20}
		}
		for c, t := range texts {
			row[c].Text = t
		}
		return row
	}
	item := &pdf.TableItem{
		Grid: [][]pdf.TSRCell{
			mkRow(0, "r0c0", "r0c1", "r0c2", "r0c3", "r0c4"),
			mkRow(20), // all empty: TSR-detected row with no OCR box
			mkRow(40, "r2c0", "r2c1", "r2c2", "r2c3", "r2c4"),
		},
	}
	_ = ConstructTable(nil, nil, "", item)
	if len(item.Rows) != 2 {
		t.Fatalf("expected 2 rows after dropping all-empty row, got %d:\n%v", len(item.Rows), item.Rows)
	}
	if item.Rows[0][0] != "r0c0" || item.Rows[1][0] != "r2c0" {
		t.Errorf("row content wrong: %v", item.Rows)
	}
	if len(item.Grid) != 2 {
		t.Errorf("expected item.Grid also to drop the empty row, got len=%d", len(item.Grid))
	}
}

func TestConstructTable_EmptyCells(t *testing.T) {
	html := ConstructTable(nil, nil, "", nil)
	if html != "" {
		t.Errorf("expected empty string for empty cells, got %q", html)
	}
	html = ConstructTable([]pdf.TSRCell{}, []pdf.TextBox{}, "", nil)
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
	html := ConstructTable(cells, boxes, "", nil)
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
	html := ConstructTable(cells, nil, "表1：测试标题", nil)
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
	html := ConstructTable(cells, nil, "", nil)
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
	// Cells pre-filled — constructTable no longer fills text (done in enrichOnePageWithDeepDoc).
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A1", Label: "table row"},
		{X0: 101, Y0: 0, X1: 200, Y1: 50, Text: "B1", Label: "table row"},
		{X0: 0, Y0: 51, X1: 100, Y1: 100, Text: "A2", Label: "table row"},
		{X0: 101, Y0: 51, X1: 200, Y1: 100, Text: "B2", Label: "table row"},
	}
	_ = ConstructTable(cells, nil, "", nil)

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
	html := ConstructTable(cells, nil, "", nil)
	if strings.Count(html, "<tr>") != 2 {
		t.Errorf("expected 2 rows from Y-fallback, got %d", strings.Count(html, "<tr>"))
	}
	// Y/X grouping sizes a row by the cells it actually holds, so row 1 comes
	// back one cell short; ConstructTable widens every row to the shared column
	// count before the cleanup passes, and the missing cell renders empty the
	// way Python's rectangular construct_table grid does.
	if strings.Count(html, "<td ") != 4 { // 2 in row0, 2 (one empty) in row1
		t.Errorf("expected 4 cells, got %d", strings.Count(html, "<td "))
	}
}

func TestExtractTableAndReplace_CellTextFilled(t *testing.T) {
	// Simulate 公司差旅费 page 0 table coordinates.
	// DLA region: X0=217, X1=1584, Y0=985, Y1=1599 at 216 DPI → PDF: 72-528 x 328-533
	// Scale = 216/72 = 3.0
	// cropOff = region.X - 10pt*ZM = region.X - 30px (fixed margin, matches
	// Python's MARGIN=10; NOT the old Go 3% proportional margin).
	const scale = 3.0
	const cropOffX = 187.0
	const cropOffY = 955.0

	// Post-merge boxes in PDF point space (inside the table region).
	// PDF Y=470 → crop Top = 470*3-955 = 455 → overlaps crop cell at Y0=441.
	// Boxes must have R (row) and C (col) annotations matching cells,
	// matching Python's construct_table which assigns boxes to cells by R/C.
	boxes := []pdf.TextBox{
		{X0: 80, X1: 210, Top: 470, Bottom: 490, Text: "标职务", LayoutType: "table", PageNumber: 0, R: 0, C: 0},
		{X0: 220, X1: 270, Top: 470, Bottom: 490, Text: "飞机", LayoutType: "table", PageNumber: 0, R: 0, C: 1},
		{X0: 80, X1: 210, Top: 492, Bottom: 512, Text: "公司级领导", LayoutType: "table", PageNumber: 0, R: 1, C: 0},
		{X0: 220, X1: 270, Top: 492, Bottom: 512, Text: "经济舱位", LayoutType: "table", PageNumber: 0, R: 1, C: 1},
	}

	// TSR cells in crop pixel space (matching real TSR output).
	// Cells pre-filled (enrichOnePageWithDeepDoc already ran fillText + OCR).
	cells := []pdf.TSRCell{
		{X0: 35, Y0: 441, X1: 456, Y1: 500, Text: "标职务", Label: "table row"},
		{X0: 460, Y0: 441, X1: 630, Y1: 500, Text: "飞机", Label: "table row"},
		{X0: 35, Y0: 501, X1: 456, Y1: 560, Text: "公司级领导", Label: "table row"},
		{X0: 460, Y0: 501, X1: 630, Y1: 560, Text: "经济舱位", Label: "table row"},
	}

	tables := []pdf.TableItem{{
		Cells:     cells,
		Positions: []pdf.Position{{Left: 80, Right: 500, Top: 480, Bottom: 560}},
		Scale:     scale,
		CropOffX:  cropOffX,
		CropOffY:  cropOffY,
	}}

	result := ExtractTableAndReplace(boxes, tables)
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
	html := ConstructTable(nil, boxes, "", item)

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
	// is marginal (real-world scenario). R/C path should still fill text.
	boxes := []pdf.TextBox{
		{X0: 12, X1: 198, Top: 12, Bottom: 48, Text: "A", R: 0, C: 0},  // overlap ~92% → OK
		{X0: 215, X1: 395, Top: 12, Bottom: 48, Text: "B", R: 0, C: 1}, // overlap ~90% → OK
		{X0: 12, X1: 198, Top: 58, Bottom: 92, Text: "C", R: 1, C: 0},  // overlap ~92% → OK
		{X0: 215, X1: 350, Top: 58, Bottom: 92, Text: "D", R: 1, C: 1}, // overlap ~50% → MARGINAL
	}

	// This SHOULD fill all 4 cells via R/C, but spatial-only may fail on D.
	FillCellTextFromBoxes(cells, boxes)

	// When spatial overlap is marginal (box "D" at 50%), fillCellTextFromBoxes
	// may still match because cell is empty (0.3 threshold). But the real
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
	rows := GroupTSRCellsToRows(cells2)
	FillCellTextFromAnnotations(rows, boxes)

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

func TestConstructTable_SingleRowMultiCol(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, Text: "姓名", R: 0, C: 0},
		{X0: 101, X1: 200, Top: 0, Bottom: 30, Text: "年龄", R: 0, C: 1},
		{X0: 201, X1: 300, Top: 0, Bottom: 30, Text: "性别", R: 0, C: 2},
	}
	item := &pdf.TableItem{}
	html := ConstructTable(nil, boxes, "", item)
	if strings.Count(html, "<td ") != 3 {
		t.Errorf("expected 3 cells, got %d. HTML: %s", strings.Count(html, "<td "), html)
	}
	if item.Rows[0][0] != "姓名" || item.Rows[0][1] != "年龄" || item.Rows[0][2] != "性别" {
		t.Errorf("wrong row text: %v", item.Rows[0])
	}
}

func TestConstructTable_MultiRowSingleCol(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 30, Text: "第一行", R: 0, C: 0},
		{X0: 0, X1: 100, Top: 35, Bottom: 65, Text: "第二行", R: 1, C: 0},
		{X0: 0, X1: 100, Top: 70, Bottom: 100, Text: "第三行", R: 2, C: 0},
	}
	item := &pdf.TableItem{}
	html := ConstructTable(nil, boxes, "", item)
	if strings.Count(html, "<tr>") != 3 {
		t.Errorf("expected 3 rows, got %d. HTML: %s", strings.Count(html, "<tr>"), html)
	}
	if item.Rows[0][0] != "第一行" || item.Rows[1][0] != "第二行" || item.Rows[2][0] != "第三行" {
		t.Errorf("wrong text: row0=%q row1=%q row2=%q", item.Rows[0][0], item.Rows[1][0], item.Rows[2][0])
	}
}

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
	html := ConstructTable(nil, postMerge, "", item)
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

func TestGroupTSRCellsToRowsLabeled_DefaultTableLabel(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 10, Y0: 0, X1: 100, Y1: 30, Label: "table"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Label: "table"},
		{X0: 10, Y0: 35, X1: 100, Y1: 65, Label: "table"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Label: "table"},
	}
	rows := GroupTSRCellsToRows(cells)
	if len(rows) != 2 {
		t.Fatalf("label %q: expected 2 rows, got %d (BUG: deepDocReRowHdr does not match %q)", "table", len(rows), "table")
	}
	if len(rows[0]) != 2 || len(rows[1]) != 2 {
		t.Errorf("expected 2 cols/row, got %d/%d", len(rows[0]), len(rows[1]))
	}
}

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
	result := ExtractTableAndReplace(boxes, tables)
	// constructTable should produce HTML with "A", "B", "D" but NOT "NOT_TABLE".
	if !strings.Contains(result[0].Text, "A") || !strings.Contains(result[0].Text, "D") {
		t.Errorf("missing table box text: %s", result[0].Text)
	}
	if strings.Contains(result[0].Text, "NOT_TABLE") {
		t.Errorf("non-table box leaked into HTML: %s", result[0].Text)
	}
}

func TestFillCellText_RCOverSpatial(t *testing.T) {
	// Box at X=30-270 overlaps all 3 cells, but Python assigns it to exactly
	// ONE cell via greedy best row + tightest column. With R/C it belongs to
	// cell[1] (R=0, C=1); spatial fill must now agree (single assignment, the
	// go_bug #1 fix) instead of duplicating across all overlapping cells.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Label: "table"},
		{X0: 90, Y0: 0, X1: 200, Y1: 30, Label: "table"},
		{X0: 180, Y0: 0, X1: 300, Y1: 30, Label: "table"},
	}
	boxes := []pdf.TextBox{
		{X0: 30, X1: 270, Top: 0, Bottom: 30, Text: "TEXT", LayoutType: "table", R: 0, C: 1},
	}

	// Spatial fill: assigns the box to the single tightest column (cell[1]).
	cellsCopy := make([]pdf.TSRCell, 3)
	copy(cellsCopy, cells)
	FillCellTextFromBoxes(cellsCopy, boxes)
	spatialCount := 0
	for _, c := range cellsCopy {
		if c.Text != "" {
			spatialCount++
		}
	}
	if spatialCount != 1 {
		t.Errorf("spatial fill: expected exactly 1 cell with text, got %d", spatialCount)
	}
	t.Logf("spatial fill: %d cell (single assignment, matches R/C)", spatialCount)

	// R/C fill: only cell matching box.R/C gets text.
	cellsRC := make([]pdf.TSRCell, 3)
	copy(cellsRC, cells)
	rows := GroupTSRCellsToRows(cellsRC)
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
		if got := IsCaptionBox(tt.text, ""); got != tt.want {
			t.Errorf("IsCaptionBox(%q) = %v, want %v", tt.text, got, tt.want)
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
	FillCellTextFromBoxes(cells, boxes)
	if cells[0].Text != "" {
		t.Errorf("caption leaked into cell 0: %q", cells[0].Text)
	}
	if cells[1].Text != "数据行" {
		t.Errorf("data not in cell 1: %q", cells[1].Text)
	}
}

func TestFillCellText_RCPreventsCrossCellLeak(t *testing.T) {
	// Caption box at Y=0-15 overlaps BOTH cell rows (both are "empty").
	// Spatial fill: text leaks to cells[1]. R/C fill: only cell[0] gets text.
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
	FillCellTextFromBoxes(cellsSp, boxes)
	if cellsSp[1].Text != "" {
		t.Errorf("spatial fill: caption leaked to cell[1]: %q", cellsSp[1].Text)
	}

	// R/C fill → only cell[0] (R=0,C=0).
	cellsRC := make([]pdf.TSRCell, 2)
	copy(cellsRC, cells)
	rows := GroupTSRCellsToRows(cellsRC)
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

func TestStripCaptionFromCells_ClearsCaptionPattern(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "表1：差旅费标准"},
		{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: ""},
		{X0: 0, Y0: 60, X1: 100, Y1: 110, Text: "张三"},
		{X0: 100, Y0: 60, X1: 200, Y1: 110, Text: "100"},
	}
	StripCaptionFromCells(cells)
	if cells[0].Text != "" {
		t.Errorf("caption cell should be cleared, got %q", cells[0].Text)
	}
	if cells[2].Text != "张三" {
		t.Errorf("data cell should be preserved, got %q", cells[2].Text)
	}
}

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
	StripCaptionFromCells(cells)
	for i := range cells {
		if cells[i].Text != orig[i] {
			t.Errorf("cell[%d] changed: %q → %q", i, orig[i], cells[i].Text)
		}
	}
}

func TestStripCaptionFromCells_Empty(t *testing.T) {
	cells := []pdf.TSRCell{}
	StripCaptionFromCells(cells) // must not panic
}

func TestConstructTable_StripsCaptionFromCells(t *testing.T) {
	// Cell[0] has caption text "表1：标题"; cell[1] has real data.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "表1：标题"},
		{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "数据"},
	}
	html := ConstructTable(cells, nil, "", nil)
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

func TestExtractTableAndReplace(t *testing.T) {
	// Build boxes with table labels and a pdf.TableItem with cells.
	boxes := []pdf.TextBox{
		{X0: 0, X1: 100, Top: 0, Bottom: 20, Text: "A", LayoutType: "table", PageNumber: 0, R: 0, C: 0},
		{X0: 0, X1: 100, Top: 21, Bottom: 40, Text: "B", LayoutType: "table", PageNumber: 0, R: 0, C: 0},
		{X0: 110, X1: 200, Top: 0, Bottom: 20, Text: "C", LayoutType: "table", PageNumber: 0, R: 0, C: 1},
		{X0: 110, X1: 200, Top: 21, Bottom: 40, Text: "D", LayoutType: "table", PageNumber: 0, R: 0, C: 1},
	}
	ti := pdf.TableItem{
		Cells: []pdf.TSRCell{
			{X0: 0, Y0: 0, X1: 100, Y1: 20, Label: "table row"},
			{X0: 110, Y0: 0, X1: 200, Y1: 20, Label: "table row"},
			{X0: 0, Y0: 21, X1: 100, Y1: 40, Label: "table row"},
			{X0: 110, Y0: 21, X1: 200, Y1: 40, Label: "table row"},
		},
		Positions: []pdf.Position{{Left: 0, Right: 200, Top: 0, Bottom: 40}},
		Scale:     1.0,
	}
	result := ExtractTableAndReplace(boxes, []pdf.TableItem{ti})
	if len(result) != 1 {
		t.Fatalf("expected 1 box (replaced), got %d", len(result))
	}
	if result[0].LayoutType != "table" {
		t.Errorf("expected LayoutType table, got %q", result[0].LayoutType)
	}
	if !strings.Contains(result[0].Text, "<table>") {
		t.Errorf("expected HTML table, got %q", result[0].Text)
	}
}

func TestBoxMatchesCell_FalsePositive(t *testing.T) {
	// Cell: narrow table cell (40x20 px)
	cell := pdf.TSRCell{X0: 0, Y0: 0, X1: 40, Y1: 20}

	// Box A: entirely inside the cell → should match
	boxA := pdf.TextBox{X0: 5, X1: 35, Top: 2, Bottom: 18, Text: "标职务"}

	// Box B: a wide body-text box that only slightly overlaps the cell
	boxB := pdf.TextBox{X0: 30, X1: 200, Top: 5, Bottom: 15, Text: "第二条出差人员应按规定等级乘坐交通工具..."}

	if !BoxMatchesCell(cell, boxA, true) {
		t.Error("boxA entirely inside cell should match with cellIsEmpty=true")
	}
	if BoxMatchesCell(cell, boxB, true) {
		t.Error("boxB mostly outside cell should NOT match even with cellIsEmpty=true")
	}
	if !BoxMatchesCell(cell, boxA, false) {
		t.Error("boxA entirely inside cell should match with cellIsEmpty=false")
	}
	if BoxMatchesCell(cell, boxB, false) {
		t.Error("boxB mostly outside cell should NOT match with cellIsEmpty=false")
	}
}

func TestFillCellTextFromBoxes_PageGlobal(t *testing.T) {
	t.Run("exact alignment matches", func(t *testing.T) {
		cells := []pdf.TSRCell{
			{X0: 73, Y0: 329, X1: 214, Y1: 345},
			{X0: 214, Y0: 329, X1: 272, Y1: 345},
			{X0: 272, Y0: 329, X1: 407, Y1: 345},
		}
		boxes := []pdf.TextBox{
			{X0: 73, X1: 214, Top: 329, Bottom: 345, Text: "标职务"},
			{X0: 214, X1: 272, Top: 329, Bottom: 345, Text: "飞机"},
			{X0: 272, X1: 407, Top: 329, Bottom: 345, Text: "火车"},
		}
		FillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "标职务" {
			t.Errorf("cell[0] = %q, want %q", cells[0].Text, "标职务")
		}
		if cells[1].Text != "飞机" {
			t.Errorf("cell[1] = %q, want %q", cells[1].Text, "飞机")
		}
		if cells[2].Text != "火车" {
			t.Errorf("cell[2] = %q, want %q", cells[2].Text, "火车")
		}
	})

	t.Run("body text box does not leak into cell", func(t *testing.T) {
		cells := []pdf.TSRCell{{X0: 73, Y0: 329, X1: 214, Y1: 345}}
		boxes := []pdf.TextBox{
			{X0: 73, X1: 214, Top: 329, Bottom: 345, Text: "标职务"},
			{X0: 73, X1: 520, Top: 310, Bottom: 360, Text: "第二条出差人员应按规定"},
		}
		FillCellTextFromBoxes(cells, boxes)
		if cells[0].Text != "标职务" {
			t.Errorf("cell text = %q, want %q (body text should not leak in)", cells[0].Text, "标职务")
		}
	})

	t.Run("empty cells list is no-op", func(t *testing.T) {
		FillCellTextFromBoxes(nil, []pdf.TextBox{{Text: "x"}})
	})

	t.Run("empty boxes list preserves cell text", func(t *testing.T) {
		cells := []pdf.TSRCell{{Text: "existing"}}
		FillCellTextFromBoxes(cells, nil)
		if cells[0].Text != "existing" {
			t.Errorf("existing text should be preserved, got %q", cells[0].Text)
		}
	})
}

func TestMergeCaptions_NeedsCaptionLayoutType(t *testing.T) {
	// Simulate what happens when DLA doesn't produce a "table caption" region:
	// a "text" section adjacent to a table is NOT treated as caption.
	sections := []pdf.Section{
		{LayoutType: "table", Text: "<table><tr><td >data</td></tr></table>",
			Positions: []pdf.Position{{Left: 100, Right: 500, Top: 200, Bottom: 400}}},
		{LayoutType: "text", Text: "公司领导班子成员、出差地",
			Positions: []pdf.Position{{Left: 100, Right: 500, Top: 180, Bottom: 198}}},
	}
	figures := pdf.CollectFigures(sections)
	result := MergeCaptions(sections, figures)
	// BUG: "text" layout type is NOT matched by mergeCaptions (only "table caption"/"figure caption").
	// The caption text survives as a separate section instead of being prepended to the table.
	for _, s := range result {
		if s.LayoutType == "text" && strings.Contains(s.Text, "公司领导班子") {
			t.Log("KNOWN LIMITATION: caption with LayoutType='text' not stripped by mergeCaptions")
		}
	}
}

func TestCleanupOrphanColumns(t *testing.T) {
	// Test 1: column cleanup is gated on >=4 rows, so a 3-row table is
	// skipped; even if it ran, this single-column grid has no orphan column.
	t.Run("less than 4 rows", func(t *testing.T) {
		rows := [][]pdf.TSRCell{
			{{Text: "a"}},
			{{Text: "b"}},
			{{Text: "c"}},
		}
		result := CleanupOrphanColumns(rows)
		if len(result) != 3 {
			t.Errorf("expected 3 rows, got %d", len(result))
		}
	})

	// Test 2: 4 rows, no orphan columns
	t.Run("4 rows no orphans", func(t *testing.T) {
		rows := [][]pdf.TSRCell{
			{{Text: "a"}, {Text: "b"}},
			{{Text: "c"}, {Text: "d"}},
			{{Text: "e"}, {Text: "f"}},
			{{Text: "g"}, {Text: "h"}},
		}
		result := CleanupOrphanColumns(rows)
		if len(result[0]) != 2 {
			t.Errorf("expected 2 columns, got %d", len(result[0]))
		}
	})

	// Test 3: 4 rows, one orphan column in the middle
	t.Run("4 rows orphan column in middle kept", func(t *testing.T) {
		rows := [][]pdf.TSRCell{
			{{Text: "a", X0: 0, X1: 10}, {Text: ""}, {Text: "b", X0: 30, X1: 40}},
			{{Text: "c", X0: 0, X1: 10}, {Text: ""}, {Text: "d", X0: 30, X1: 40}},
			{{Text: "e", X0: 0, X1: 10}, {Text: "orphan", X0: 15, X1: 25}, {Text: "f", X0: 30, X1: 40}},
			{{Text: "g", X0: 0, X1: 10}, {Text: ""}, {Text: "h", X0: 30, X1: 40}},
		}
		result := CleanupOrphanColumns(rows)
		if len(result[0]) != 3 {
			t.Errorf("expected 3 columns (kept because both sides have text), got %d", len(result[0]))
		}
	})
}

func TestCountNonEmptyCells(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{{Text: "a"}, {Text: ""}},
		{{Text: ""}, {Text: ""}},
		{{Text: "b"}, {Text: "c"}},
		{{Text: ""}, {Text: ""}},
	}

	count, rowIdx := countNonEmptyCells(rows, 0)
	if count != 2 {
		t.Errorf("expected 2 non-empty cells in column 0, got %d", count)
	}
	if rowIdx != 2 {
		t.Errorf("expected last non-empty cell at row 2, got %d", rowIdx)
	}

	count, rowIdx = countNonEmptyCells(rows, 1)
	if count != 1 {
		t.Errorf("expected 1 non-empty cell in column 1, got %d", count)
	}
	if rowIdx != 2 {
		t.Errorf("expected last non-empty cell at row 2, got %d", rowIdx)
	}

	count, rowIdx = countNonEmptyCells(rows, 999)
	if count != 0 {
		t.Errorf("expected 0 non-empty cells for invalid column, got %d", count)
	}
}

func TestCheckAdjacentColumns(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{{Text: "left"}, {Text: "orphan"}, {Text: "right"}},
	}

	hasLeft, hasRight := checkAdjacentColumns(rows, 1, 0)
	if !hasLeft {
		t.Error("expected left column to have text")
	}
	if !hasRight {
		t.Error("expected right column to have text")
	}

	rows2 := [][]pdf.TSRCell{
		{{Text: ""}, {Text: "orphan"}, {Text: ""}},
	}
	hasLeft, hasRight = checkAdjacentColumns(rows2, 1, 0)
	if hasLeft {
		t.Error("expected left column to be empty")
	}
	if hasRight {
		t.Error("expected right column to be empty")
	}

	// Test edge cases
	rows3 := [][]pdf.TSRCell{
		{{Text: "only column"}},
	}
	hasLeft, hasRight = checkAdjacentColumns(rows3, 0, 0)
	if !hasLeft { // j == 0 should count hasLeft as true
		t.Error("expected hasLeft to be true when j == 0")
	}
	if !hasRight { // j+1 >= len should count hasRight as true
		t.Error("expected hasRight to be true when j+1 >= len")
	}
}

func TestCalculateMergeDistance(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{{Text: "left", X0: 0, X1: 10}, {Text: "orphan", X0: 15, X1: 25}, {Text: "right", X0: 30, X1: 40}},
	}

	leftDist, rightDist := calculateMergeDistance(rows, 1, 0, 3, false, false)
	if leftDist != 5 { // 15 - 10 = 5
		t.Errorf("expected left distance 5, got %v", leftDist)
	}
	if rightDist != 5 { // 30 - 25 = 5
		t.Errorf("expected right distance 5, got %v", rightDist)
	}
}

func TestMergeColumn(t *testing.T) {
	tests := []struct {
		name     string
		mergeDir string // "left" or "right"
		srcCol   int
		wantCol0 string
		wantCol1 string
	}{
		{
			name:     "merge left",
			mergeDir: "left",
			srcCol:   1,
			wantCol0: "a b",
			wantCol1: "b",
		},
		{
			name:     "merge right",
			mergeDir: "right",
			srcCol:   0,
			wantCol0: "a",
			wantCol1: "a b",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := [][]pdf.TSRCell{
				{{Text: "a"}, {Text: "b"}},
				{{Text: ""}, {Text: "d"}},
				{{Text: "e"}, {Text: ""}},
			}

			if tt.mergeDir == "left" {
				mergeColumnIntoLeft(rows, tt.srcCol)
				if rows[0][0].Text != tt.wantCol0 {
					t.Errorf("expected '%s', got '%s'", tt.wantCol0, rows[0][0].Text)
				}
				if rows[1][0].Text != "d" {
					t.Errorf("expected 'd', got '%s'", rows[1][0].Text)
				}
				if rows[2][0].Text != "e" {
					t.Errorf("expected 'e', got '%s'", rows[2][0].Text)
				}
			} else {
				mergeColumnIntoRight(rows, tt.srcCol)
				if rows[0][1].Text != tt.wantCol1 {
					t.Errorf("expected '%s' in right column, got '%s'", tt.wantCol1, rows[0][1].Text)
				}
				if rows[1][1].Text != "d" {
					t.Errorf("expected 'd' in right column, got '%s'", rows[1][1].Text)
				}
				if rows[2][1].Text != "e" {
					t.Errorf("expected 'e' in right column, got '%s'", rows[2][1].Text)
				}
			}
		})
	}
}

func TestRemoveColumn(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{{Text: "a"}, {Text: "b"}, {Text: "c"}},
		{{Text: "d"}, {Text: "e"}, {Text: "f"}},
	}

	result := removeColumn(rows, 1)
	if len(result[0]) != 2 {
		t.Errorf("expected 2 columns after removal, got %d", len(result[0]))
	}
	if result[0][0].Text != "a" || result[0][1].Text != "c" {
		t.Errorf("unexpected column content after removal")
	}
}

// TestCleanupOrphanColumns_PreservesSparseColumnsWithNormalGaps verifies a
// single-cell column wider than maxOrphanMergeGap from its neighbors is a
// legitimate sparse column and is preserved, not force-merged.
func TestCleanupOrphanColumns_PreservesSparseColumnsWithNormalGaps(t *testing.T) {
	// A 4-row table with 3 columns where column 1 has a single note in row 2.
	// The gap between column 0 and column 1 is 50pt (> maxOrphanMergeGap = 25pt).
	// It must be preserved rather than merged and deleted.
	rows := [][]pdf.TSRCell{
		{{Text: "H0", X0: 0, X1: 40}, {Text: "", X0: 90, X1: 150}, {Text: "H2", X0: 200, X1: 250}},
		{{Text: "A0", X0: 0, X1: 40}, {Text: "", X0: 90, X1: 150}, {Text: "A2", X0: 200, X1: 250}},
		{{Text: "B0", X0: 0, X1: 40}, {Text: "Note", X0: 90, X1: 150}, {Text: "B2", X0: 200, X1: 250}},
		{{Text: "C0", X0: 0, X1: 40}, {Text: "", X0: 90, X1: 150}, {Text: "C2", X0: 200, X1: 250}},
	}
	result := CleanupOrphanColumns(rows)
	if len(result[0]) != 3 {
		t.Fatalf("expected 3 columns preserved, got %d", len(result[0]))
	}
	if result[2][1].Text != "Note" {
		t.Errorf("expected 'Note' preserved in col 1, got %q", result[2][1].Text)
	}
}

func TestCleanupOrphanColumns_MergesCloseFragment(t *testing.T) {
	rows := [][]pdf.TSRCell{
		{{Text: "A0", X0: 0, X1: 40}, {X0: 45, X1: 60}, {Text: "A2", X0: 70, X1: 100}},
		{{Text: "B0", X0: 0, X1: 40}, {X0: 45, X1: 60}, {Text: "B2", X0: 70, X1: 100}},
		{{X0: 0, X1: 40}, {Text: "fragment", X0: 45, X1: 60}, {X0: 70, X1: 100}},
		{{Text: "C0", X0: 0, X1: 40}, {X0: 45, X1: 60}, {Text: "C2", X0: 70, X1: 100}},
	}
	result := CleanupOrphanColumns(rows)
	if len(result[0]) != 2 {
		t.Fatalf("orphan within the 25-point fragment gap should merge, got %d columns", len(result[0]))
	}
	if !strings.Contains(result[2][0].Text+result[2][1].Text, "fragment") {
		t.Fatalf("fragment text was lost during merge: %v", RowsToStrings(result))
	}
}

// TestCleanupOrphanRows_PreservesSparseRowsWithNormalGaps verifies a lone-cell
// row separated from its neighbors by more than maxOrphanMergeGap (a subtotal
// or category row) is preserved instead of merged into an adjacent row.
func TestCleanupOrphanRows_PreservesSparseRowsWithNormalGaps(t *testing.T) {
	// A 4-column table where row 2 is a category title spanning row with 1 cell.
	// The vertical gap between row 1 and row 2 is 40pt (> maxOrphanMergeGap = 25pt).
	// It must be preserved rather than merged into row 1.
	rows := [][]pdf.TSRCell{
		{{Text: "H0", Y0: 0, Y1: 15}, {Text: "H1", Y0: 0, Y1: 15}, {Text: "H2", Y0: 0, Y1: 15}, {Text: "H3", Y0: 0, Y1: 15}},
		{{Text: "A0", Y0: 20, Y1: 35}, {Text: "A1", Y0: 20, Y1: 35}, {Text: "A2", Y0: 20, Y1: 35}, {Text: "A3", Y0: 20, Y1: 35}},
		{{Text: "Subtotal", Y0: 80, Y1: 95}, {Text: "", Y0: 80, Y1: 95}, {Text: "", Y0: 80, Y1: 95}, {Text: "", Y0: 80, Y1: 95}},
		{{Text: "B0", Y0: 140, Y1: 155}, {Text: "B1", Y0: 140, Y1: 155}, {Text: "B2", Y0: 140, Y1: 155}, {Text: "B3", Y0: 140, Y1: 155}},
	}
	result := CleanupOrphanRows(rows)
	if len(result) != 4 {
		t.Fatalf("expected 4 rows preserved, got %d", len(result))
	}
	if result[2][0].Text != "Subtotal" {
		t.Errorf("expected 'Subtotal' in row 2, got %q", result[2][0].Text)
	}
}

func TestConstructTable_OrphanGapUsesRenderedScale(t *testing.T) {
	item := pdf.TableItem{
		Scale: 3,
		Grid: [][]pdf.TSRCell{
			{{Text: "L0", X0: 0, X1: 40}, {X0: 90, X1: 140}, {Text: "R0", X0: 190, X1: 230}},
			{{Text: "L1", X0: 0, X1: 40}, {X0: 90, X1: 140}, {Text: "R1", X0: 190, X1: 230}},
			{{Text: "L2", X0: 0, X1: 40}, {Text: "Note", X0: 90, X1: 140}, {X0: 190, X1: 230}},
			{{Text: "L3", X0: 0, X1: 40}, {X0: 90, X1: 140}, {Text: "R3", X0: 190, X1: 230}},
		},
	}

	ConstructTable(nil, nil, "", &item)

	if len(item.Grid[0]) != 2 {
		t.Fatalf("50 rendered pixels at scale 3 are under the 25-point limit; want merged 2-column grid, got %d columns", len(item.Grid[0]))
	}
	if item.Rows[2][1] != "Note" {
		t.Fatalf("orphan text = %q, want it preserved in the neighboring cell", item.Rows[2][1])
	}
}

func TestConstructTable_OrphanRowGapUsesRenderedScale(t *testing.T) {
	cell := func(text string, y float64) pdf.TSRCell {
		return pdf.TSRCell{Text: text, Y0: y, Y1: y + 20}
	}
	item := pdf.TableItem{
		Scale: 3,
		Grid: [][]pdf.TSRCell{
			{cell("H0", 0), cell("H1", 0), cell("H2", 0), cell("H3", 0)},
			{cell("", 30), cell("Note", 30), cell("", 30), cell("", 30)},
			{cell("A", 100), cell("", 100), cell("B", 100), cell("C", 100)},
			{cell("D", 130), cell("E", 130), cell("F", 130), cell("G", 130)},
		},
	}

	ConstructTable(nil, nil, "", &item)

	if len(item.Rows) != 3 {
		t.Fatalf("50 rendered pixels at scale 3 are under the 25-point row limit; want 3 rows, got %d: %v", len(item.Rows), item.Rows)
	}
	if item.Rows[1][1] != "Note" {
		t.Fatalf("orphan row text = %q, want it preserved in the neighboring row", item.Rows[1][1])
	}
}

// fallbackBox builds a page-numbered, R/C annotated text box in PDF point
// space — the input the degraded cross-page path rebuilds a grid from.
func fallbackBox(text string, page, r, c int, x0, x1, top, bottom float64) pdf.TextBox {
	return pdf.TextBox{
		Text:          text,
		X0:            x0,
		X1:            x1,
		Top:           top,
		Bottom:        bottom,
		PageNumber:    page,
		HasPageNumber: true,
		R:             r,
		RTop:          top,
		RBott:         bottom,
		C:             c,
	}
}

// noRBox is a page-numbered text box with no row annotation at all, so the page
// grouping falls through GroupBoxesByRC into the coordinate-based
// GroupBoxesByYX, whose cells hold nothing but text.
func noRBox(text string, page int, x0, x1, top, bottom float64) pdf.TextBox {
	return fallbackBox(text, page, -1, 0, x0, x1, top, bottom)
}

// stalePixelGrid returns the crop-pixel anchor grid of a merged table rendered
// at scale 3. It is non-empty precisely when ConstructTable is about to replace
// it with a point-space grid rebuilt from boxes.
func stalePixelGrid() [][]pdf.TSRCell {
	cell := func(text string, x0, y0, x1, y1 float64) pdf.TSRCell {
		return pdf.TSRCell{Text: text, X0: x0, Y0: y0, X1: x1, Y1: y1}
	}
	return [][]pdf.TSRCell{
		{cell("P0", 0, 0, 150, 60), cell("P1", 150, 0, 300, 60)},
		{cell("P2", 0, 60, 150, 120), cell("P3", 150, 60, 300, 120)},
	}
}

func crossPagePositions() []pdf.Position {
	return []pdf.Position{{PageNumbers: []int{0}}, {PageNumbers: []int{1}}}
}

func assertFallbackRows(t *testing.T, got [][]string, want [][]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("rows = %d, want %d:\n%v", len(got), len(want), got)
	}
	for i := range want {
		if strings.Join(got[i], "|") != strings.Join(want[i], "|") {
			t.Errorf("row %d = %v, want %v", i, got[i], want[i])
		}
	}
}

// TestConstructTable_OrphanGapStaysInPointsForPageFallback pins that the
// rendered-scale adjustment applies only to the crop-pixel item.Grid. On the
// degraded cross-page path the grid is rebuilt from page-numbered boxes, whose
// coordinates are PDF points, while the item.Grid that decides the scale is
// still the stale crop-pixel anchor. Multiplying the 25 point fragment gap by
// item.Scale there (x3, up to x9 on a retry zoom) merged legitimate sparse rows
// and shifted their text into the row above.
func TestConstructTable_OrphanGapStaysInPointsForPageFallback(t *testing.T) {
	page0 := []pdf.TextBox{
		fallbackBox("H0", 0, 0, 0, 0, 50, 10, 25),
		fallbackBox("H1", 0, 0, 1, 60, 110, 10, 25),
		fallbackBox("H2", 0, 0, 2, 120, 170, 10, 25),
		fallbackBox("H3", 0, 0, 3, 180, 230, 10, 25),
		fallbackBox("A0", 0, 1, 0, 0, 50, 30, 45),
		fallbackBox("A1", 0, 1, 1, 60, 110, 30, 45),
		fallbackBox("A2", 0, 1, 2, 120, 170, 30, 45),
		fallbackBox("A3", 0, 1, 3, 180, 230, 30, 45),
	}
	// Continuation page whose third column is empty in its first row, so the
	// lone cell below it is an orphan candidate measured in points.
	page1Head := []pdf.TextBox{
		fallbackBox("B0", 1, 0, 0, 0, 50, 10, 25),
		fallbackBox("B1", 1, 0, 1, 60, 110, 10, 25),
		fallbackBox("B3", 1, 0, 3, 180, 230, 10, 25),
	}

	t.Run("a 35 point gap is a sparse row, not a fragment", func(t *testing.T) {
		boxes := append(append([]pdf.TextBox{}, page0...), page1Head...)
		boxes = append(boxes, fallbackBox("Subtotal", 1, 1, 2, 120, 170, 60, 75))
		item := pdf.TableItem{
			Scale:                 3,
			Grid:                  stalePixelGrid(),
			NeedsPageGridFallback: true,
			Positions:             crossPagePositions(),
		}
		ConstructTable(nil, boxes, "", &item)

		// 35pt is above the 25pt fragment gap but below 25pt x scale 3: a
		// scaled threshold would merge this category row into the row above.
		assertFallbackRows(t, item.Rows, [][]string{
			{"H0", "H1", "H2", "H3"},
			{"A0", "A1", "A2", "A3"},
			{"B0", "B1", "", "B3"},
			{"", "", "Subtotal", ""},
		})
	})

	t.Run("a 15 point gap still merges", func(t *testing.T) {
		// Control: the point threshold stays active on the rebuilt grid, so a
		// genuine vertical fragment is still joined with the row above.
		boxes := append(append([]pdf.TextBox{}, page0...), page1Head...)
		boxes = append(boxes, fallbackBox("Note", 1, 1, 2, 120, 170, 40, 55))
		item := pdf.TableItem{
			Scale:                 3,
			Grid:                  stalePixelGrid(),
			NeedsPageGridFallback: true,
			Positions:             crossPagePositions(),
		}
		ConstructTable(nil, boxes, "", &item)

		assertFallbackRows(t, item.Rows, [][]string{
			{"H0", "H1", "H2", "H3"},
			{"A0", "A1", "A2", "A3"},
			{"B0", "B1", "Note", "B3"},
		})
	})
}

// TestGroupFallbackBoxesByPageLeavesGridRawPadsAtConsumer pins where the jagged
// grid gets normalized. Each page's GroupBoxesByRC sizes its grid by that
// page's distinct C labels, so a separator missed on one page yields fewer
// columns there and stacking returns a jagged grid; the producer stays raw and
// ConstructTable widens every row to the shared count once, which is the only
// place that covers all producers. Without that normalization
// cleanupOrphanRows indexes an adjacent row at the orphan's column unguarded
// and an out-of-range panic kills the parse worker.
func TestGroupFallbackBoxesByPageLeavesGridRawPadsAtConsumer(t *testing.T) {
	positions := crossPagePositions()
	boxes := []pdf.TextBox{
		// Page 0: four detected columns, last row a lone right-most cell.
		fallbackBox("H0", 0, 0, 0, 0, 50, 10, 25),
		fallbackBox("H1", 0, 0, 1, 60, 110, 10, 25),
		fallbackBox("H2", 0, 0, 2, 120, 170, 10, 25),
		fallbackBox("H3", 0, 0, 3, 180, 230, 10, 25),
		fallbackBox("A0", 0, 1, 0, 0, 50, 30, 45),
		fallbackBox("A1", 0, 1, 1, 60, 110, 30, 45),
		fallbackBox("A2", 0, 1, 2, 120, 170, 30, 45),
		fallbackBox("A3", 0, 1, 3, 180, 230, 30, 45),
		fallbackBox("Tail", 0, 2, 3, 180, 230, 50, 65),
		// Page 1: a missed separator leaves this page two columns.
		fallbackBox("B0", 1, 0, 0, 0, 50, 10, 25),
		fallbackBox("B1", 1, 0, 1, 60, 110, 10, 25),
		fallbackBox("C0", 1, 1, 0, 0, 50, 40, 55),
		fallbackBox("C1", 1, 1, 1, 60, 110, 40, 55),
	}

	// The producer reports what the pages actually yielded: the two-column
	// continuation page stacks under the four-column anchor page.
	grid := groupFallbackBoxesByPage(boxes, positions)
	if got, want := rowWidths(grid), []int{4, 4, 4, 2, 2}; !sameInts(got, want) {
		t.Fatalf("grouping widths = %v, want %v (the producer stays raw):\n%v",
			got, want, RowsToStrings(grid))
	}

	item := pdf.TableItem{
		Scale:                 3,
		Grid:                  stalePixelGrid(),
		NeedsPageGridFallback: true,
		Positions:             positions,
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ConstructTable panicked on the fallback grid: %v", r)
			}
		}()
		ConstructTable(nil, boxes, "", &item)
	}()
	assertFallbackRows(t, item.Rows, [][]string{
		{"H0", "H1", "H2", "H3"},
		{"A0", "A1", "A2", "A3"},
		{"", "", "", "Tail"},
		{"B0", "B1", "", ""},
		{"C0", "C1", "", ""},
	})
	// The consumer-side normalization is what makes the grid rectangular again,
	// so the emitted grid keeps one column model for every row.
	for i, row := range item.Grid {
		if len(row) != 4 {
			t.Errorf("grid row %d width = %d, want 4 (uniform after padding):\n%v", i, len(row), RowsToStrings(item.Grid))
		}
	}
}

// TestConstructTable_PadsJaggedCellGrid pins the same normalization on the
// cells path, which handleImageOnlyPDFs reaches with no boxes at all.
// GroupTSRCellsToRows emits one row per Y band holding that band's own cells,
// so the widths are inherently per-row. Here the widest band carries its only
// text in its fifth column, and cleanupOrphanRows reads the neighbouring rows
// at that same column — one past their end — so the worker died with
// "index out of range [4] with length 4".
func TestConstructTable_PadsJaggedCellGrid(t *testing.T) {
	bands := [][]string{
		{"A0", "A1", "A2", "A3"},
		{"B0", "B1", "B2", "B3"},
		{"C0", "C1", "C2", "C3"},
		{"", "", "", "", "Subtotal"},
		{"E0", "E1", "E2", "E3"},
	}
	var cells []pdf.TSRCell
	for i, band := range bands {
		y0 := float64(i) * 60
		for j, text := range band {
			cells = append(cells, pdf.TSRCell{
				Text:  text,
				X0:    float64(j) * 100,
				Y0:    y0,
				X1:    float64(j+1) * 100,
				Y1:    y0 + 50,
				Label: "table row",
			})
		}
	}

	// The producer stays raw: one row per band, that band's own cell count.
	raw := GroupTSRCellsToRows(cells)
	if got, want := rowWidths(raw), []int{4, 4, 4, 5, 4}; !sameInts(got, want) {
		t.Fatalf("GroupTSRCellsToRows widths = %v, want %v", got, want)
	}

	var item pdf.TableItem
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ConstructTable panicked on the jagged cell grid: %v", r)
			}
		}()
		ConstructTable(cells, nil, "", &item)
	}()

	if len(item.Rows) != len(bands) {
		t.Fatalf("rows = %d, want %d:\n%v", len(item.Rows), len(bands), item.Rows)
	}
	for i := range item.Grid {
		if len(item.Grid[i]) != len(item.Grid[0]) {
			t.Fatalf("grid row %d width = %d, want the shared width %d:\n%v",
				i, len(item.Grid[i]), len(item.Grid[0]), RowsToStrings(item.Grid))
		}
	}
	for _, band := range bands {
		for _, want := range band {
			if want == "" {
				continue
			}
			if !rowContainsText(item.Rows, want) {
				t.Errorf("cell text %q lost from the grid:\n%v", want, item.Rows)
			}
		}
	}
}

// TestConstructTable_KeepsOrphanRowWithoutGridGeometry pins the no-geometry
// guard. Every page here falls through GroupBoxesByRC into GroupBoxesByYX,
// which stores text only, so all cells report X0==X1==Y0==Y1==0. The orphan row
// pass reads that as a zero gap and folds a lone cell into the row above it no
// matter how far below the page it actually sat.
func TestConstructTable_KeepsOrphanRowWithoutGridGeometry(t *testing.T) {
	// R stays -1 on every box, so the page grouping is coordinate-based.
	boxes := []pdf.TextBox{
		noRBox("H0", 0, 0, 50, 10, 25),
		noRBox("H1", 0, 60, 110, 10, 25),
		noRBox("H2", 0, 120, 170, 10, 25),
		noRBox("H3", 0, 180, 230, 10, 25),
		noRBox("A0", 0, 0, 50, 30, 45),
		noRBox("A1", 0, 60, 110, 30, 45),
		noRBox("A2", 0, 120, 170, 30, 45),
		noRBox("A3", 0, 180, 230, 30, 45),
		// Last anchor row stops one column short, so column 3 is blank there.
		noRBox("B0", 0, 0, 50, 50, 65),
		noRBox("B1", 0, 60, 110, 50, 65),
		noRBox("B2", 0, 120, 170, 50, 65),
		// Continuation page: a subtotal row 265pt below the anchor, whose text
		// sits in the fourth column.
		noRBox("", 1, 0, 50, 330, 345),
		noRBox("", 1, 60, 110, 330, 345),
		noRBox("", 1, 120, 170, 330, 345),
		noRBox("Subtotal", 1, 180, 230, 330, 345),
	}
	grid := groupFallbackBoxesByPage(boxes, crossPagePositions())
	if got, want := rowWidths(grid), []int{4, 4, 3, 4}; !sameInts(got, want) {
		t.Fatalf("page grouping widths = %v, want %v:\n%v", got, want, RowsToStrings(grid))
	}
	for _, row := range grid {
		for _, cell := range row {
			if cell.X0 != 0 || cell.X1 != 0 || cell.Y0 != 0 || cell.Y1 != 0 {
				t.Fatalf("fixture must reach the text-only grouping, got cell %+v", cell)
			}
		}
	}

	item := pdf.TableItem{Positions: crossPagePositions()}
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("ConstructTable panicked on the geometry-free grid: %v", r)
			}
		}()
		ConstructTable(nil, boxes, "", &item)
	}()
	assertFallbackRows(t, item.Rows, [][]string{
		{"H0", "H1", "H2", "H3"},
		{"A0", "A1", "A2", "A3"},
		{"B0", "B1", "B2", ""},
		{"", "", "", "Subtotal"},
	})
}

func rowWidths(rows [][]pdf.TSRCell) []int {
	out := make([]int, len(rows))
	for i, row := range rows {
		out[i] = len(row)
	}
	return out
}

func sameInts(got, want []int) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

func rowContainsText(rows [][]string, want string) bool {
	for _, row := range rows {
		for _, cell := range row {
			if cell == want {
				return true
			}
		}
	}
	return false
}
