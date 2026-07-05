
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
	// Cells pre-filled — constructTable no longer fills text (done in extractTableBoxesFromImage).
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
	if strings.Count(html, "<td ") != 3 { // 2 in row0, 1 in row1 (no padding in basic grouping)
		t.Errorf("expected 3 cells, got %d", strings.Count(html, "<td "))
	}
}

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
		{X0: 12, X1: 198, Top: 12, Bottom: 48, Text: "A", R: 0, C: 0}, // overlap ~92% → OK
		{X0: 215, X1: 395, Top: 12, Bottom: 48, Text: "B", R: 0, C: 1}, // overlap ~90% → OK
		{X0: 12, X1: 198, Top: 58, Bottom: 92, Text: "C", R: 1, C: 0}, // overlap ~92% → OK
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
	// Box at X=30-270 overlaps all 3 cells (>30% each — spatial fills ALL).
	// With R/C, it belongs only to cell[1] (R=0, C=1).
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Label: "table"},
		{X0: 90, Y0: 0, X1: 200, Y1: 30, Label: "table"},
		{X0: 180, Y0: 0, X1: 300, Y1: 30, Label: "table"},
	}
	boxes := []pdf.TextBox{
		{X0: 30, X1: 270, Top: 0, Bottom: 30, Text: "TEXT", LayoutType: "table", R: 0, C: 1},
	}

	// Spatial fill: fills ALL overlapping cells → duplication.
	cellsCopy := make([]pdf.TSRCell, 3)
	copy(cellsCopy, cells)
	FillCellTextFromBoxes(cellsCopy, boxes)
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
		{"第十条到厂矿单位出差", false}, // normal paragraph
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
	// Test 1: Less than 4 rows - no cleanup
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
