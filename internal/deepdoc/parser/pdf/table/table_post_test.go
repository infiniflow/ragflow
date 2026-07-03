package table

import (
	"strings"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ============================================
// 第一部分：findTableAnchors 的测试
// ============================================

func TestFindTableAnchors_SingleTable(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "before", LayoutType: pdf.LayoutTypeText, PageNumber: 0, X0: 10, X1: 100, Top: 10, Bottom: 50},
		{Text: "table1", LayoutType: pdf.LayoutTypeTable, PageNumber: 0, X0: 10, X1: 400, Top: 60, Bottom: 200},
		{Text: "after", LayoutType: pdf.LayoutTypeText, PageNumber: 0, X0: 10, X1: 100, Top: 210, Bottom: 250},
	}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 60, Bottom: 200}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 60, RegionBottom: 200,
			Cells: []pdf.TSRCell{{Text: "cell"}},
		},
	}

	anchors := findTableAnchors(boxes, tables)
	if len(anchors) != 1 {
		t.Errorf("expected 1 anchor, got %d", len(anchors))
	}
	if anchors[0].pos != 1 {
		t.Errorf("expected anchor at pos 1, got %d", anchors[0].pos)
	}
}

func TestFindTableAnchors_NoBoxes(t *testing.T) {
	boxes := []pdf.TextBox{}
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 60, Bottom: 200}},
			Cells:     []pdf.TSRCell{{Text: "cell"}},
		},
	}

	anchors := findTableAnchors(boxes, tables)
	if len(anchors) != 0 {
		t.Errorf("expected 0 anchors, got %d", len(anchors))
	}
}

func TestFindTableAnchors_MultipleTables(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "text1", LayoutType: pdf.LayoutTypeText, PageNumber: 0, X0: 10, X1: 100, Top: 10, Bottom: 30},
		{Text: "table1", LayoutType: pdf.LayoutTypeTable, PageNumber: 0, X0: 10, X1: 400, Top: 40, Bottom: 100},
		{Text: "text2", LayoutType: pdf.LayoutTypeText, PageNumber: 0, X0: 10, X1: 100, Top: 110, Bottom: 140},
		{Text: "table2", LayoutType: pdf.LayoutTypeTable, PageNumber: 0, X0: 10, X1: 400, Top: 150, Bottom: 210},
	}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 40, Bottom: 100}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 40, RegionBottom: 100,
			Cells: []pdf.TSRCell{{Text: "cell1"}},
		},
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 150, Bottom: 210}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 150, RegionBottom: 210,
			Cells: []pdf.TSRCell{{Text: "cell2"}},
		},
	}

	anchors := findTableAnchors(boxes, tables)
	if len(anchors) != 2 {
		t.Errorf("expected 2 anchors, got %d", len(anchors))
	}
}

func TestFindTableAnchors_AnchorAboveTable(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "above", LayoutType: pdf.LayoutTypeText, PageNumber: 0, X0: 10, X1: 100, Top: 10, Bottom: 30},
		{Text: "table", LayoutType: pdf.LayoutTypeTable, PageNumber: 0, X0: 10, X1: 400, Top: 40, Bottom: 100},
	}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 40, Bottom: 100}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 40, RegionBottom: 100,
			Cells: []pdf.TSRCell{{Text: "cell"}},
		},
	}

	anchors := findTableAnchors(boxes, tables)
	if len(anchors) != 1 {
		t.Errorf("expected 1 anchor, got %d", len(anchors))
	}
	if anchors[0].pos != 1 {
		t.Errorf("expected anchor at pos 1 (insert after above text), got %d", anchors[0].pos)
	}
}

func TestFindTableAnchors_DifferentPage(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "page0", LayoutType: pdf.LayoutTypeText, PageNumber: 0, X0: 10, X1: 100, Top: 10, Bottom: 30},
		{Text: "page1", LayoutType: pdf.LayoutTypeText, PageNumber: 1, X0: 10, X1: 100, Top: 10, Bottom: 30},
	}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 40, Bottom: 100}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 40, RegionBottom: 100,
			Cells: []pdf.TSRCell{{Text: "cell"}},
		},
	}

	anchors := findTableAnchors(boxes, tables)
	if len(anchors) != 1 {
		t.Errorf("expected 1 anchor, got %d", len(anchors))
	}
	// The text box is above the table, so pos is incremented to 1
	if anchors[0].pos != 1 {
		t.Errorf("expected anchor at pos 1, got %d", anchors[0].pos)
	}
}

// ============================================
// 第二部分：buildTableHTMLs 的测试
// ============================================

func TestBuildTableHTMLs_SingleTable(t *testing.T) {
	boxes := []pdf.TextBox{}
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 60, Bottom: 200}},
			Scale:     1.0,
			Cells: []pdf.TSRCell{
				{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A"},
				{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "B"},
			},
		},
	}

	htmls := buildTableHTMLs(boxes, tables)
	if len(htmls) != 1 {
		t.Errorf("expected 1 HTML entry, got %d", len(htmls))
	}
	if htmls[0] == "" {
		t.Error("expected non-empty HTML")
	}
}

func TestBuildTableHTMLs_NoCells(t *testing.T) {
	boxes := []pdf.TextBox{}
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 60, Bottom: 200}},
			Cells:     []pdf.TSRCell{},
		},
	}

	htmls := buildTableHTMLs(boxes, tables)
	if len(htmls) != 0 {
		t.Errorf("expected 0 HTML entries for no cells, got %d", len(htmls))
	}
}

func TestBuildTableHTMLs_WithTableBoxes(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "cell text", LayoutType: pdf.LayoutTypeTable, PageNumber: 0, X0: 10, X1: 100, Top: 60, Bottom: 100},
	}
	tables := []pdf.TableItem{
		{
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 60, Bottom: 200}},
			Scale:     1.0,
			Cells: []pdf.TSRCell{
				{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A"},
			},
		},
	}

	htmls := buildTableHTMLs(boxes, tables)
	if len(htmls) != 1 {
		t.Errorf("expected 1 HTML entry, got %d", len(htmls))
	}
}

// ============================================
// 第三部分：insertTableBoxes 的测试
// ============================================

func TestInsertTableBoxes_Basic(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "before", PageNumber: 0},
		{Text: "to replace", LayoutType: pdf.LayoutTypeTable, PageNumber: 0},
		{Text: "after", PageNumber: 0},
	}
	removeSet := map[int]bool{1: true}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 60, Bottom: 200}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 60, RegionBottom: 200,
		},
	}
	anchors := []struct{ ti, pos int }{{ti: 0, pos: 1}}
	htmls := map[int]string{0: "<table>test</table>"}

	result := insertTableBoxes(boxes, tables, removeSet, anchors, htmls)
	if len(result) != 3 {
		t.Errorf("expected 3 boxes (before + table + after), got %d", len(result))
	}
	if result[1].Text != "<table>test</table>" {
		t.Errorf("expected table HTML at position 1, got %q", result[1].Text)
	}
}

func TestInsertTableBoxes_NoRemove(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "before", PageNumber: 0},
		{Text: "after", PageNumber: 0},
	}
	removeSet := map[int]bool{}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 60, Bottom: 200}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 60, RegionBottom: 200,
		},
	}
	anchors := []struct{ ti, pos int }{{ti: 0, pos: 1}}
	htmls := map[int]string{0: "<table>test</table>"}

	result := insertTableBoxes(boxes, tables, removeSet, anchors, htmls)
	if len(result) != 3 {
		t.Errorf("expected 3 boxes, got %d", len(result))
	}
}

func TestInsertTableBoxes_AtEnd(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "first", PageNumber: 0},
		{Text: "second", PageNumber: 0},
	}
	removeSet := map[int]bool{}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 60, Bottom: 200}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 60, RegionBottom: 200,
		},
	}
	anchors := []struct{ ti, pos int }{{ti: 0, pos: 2}}
	htmls := map[int]string{0: "<table>end</table>"}

	result := insertTableBoxes(boxes, tables, removeSet, anchors, htmls)
	if len(result) != 3 {
		t.Errorf("expected 3 boxes, got %d", len(result))
	}
	if result[2].Text != "<table>end</table>" {
		t.Errorf("expected table at end, got %q", result[2].Text)
	}
}

func TestInsertTableBoxes_MultipleAnchors(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "1", PageNumber: 0},
		{Text: "2", PageNumber: 0},
		{Text: "3", PageNumber: 0},
	}
	removeSet := map[int]bool{}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 60, Bottom: 200}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 60, RegionBottom: 200,
		},
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 210, Bottom: 350}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 210, RegionBottom: 350,
		},
	}
	anchors := []struct{ ti, pos int }{{ti: 0, pos: 1}, {ti: 1, pos: 3}}
	htmls := map[int]string{0: "<table>A</table>", 1: "<table>B</table>"}

	result := insertTableBoxes(boxes, tables, removeSet, anchors, htmls)
	if len(result) != 5 {
		t.Errorf("expected 5 boxes, got %d", len(result))
	}
	if result[1].Text != "<table>A</table>" {
		t.Errorf("expected table A at pos 1")
	}
	if result[4].Text != "<table>B</table>" {
		t.Errorf("expected table B at pos 4")
	}
}

func TestInsertTableBoxes_EmptyHTML(t *testing.T) {
	boxes := []pdf.TextBox{{Text: "text", PageNumber: 0}}
	removeSet := map[int]bool{}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 60, Bottom: 200}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 60, RegionBottom: 200,
		},
	}
	anchors := []struct{ ti, pos int }{{ti: 0, pos: 1}}
	htmls := map[int]string{0: ""}

	result := insertTableBoxes(boxes, tables, removeSet, anchors, htmls)
	if len(result) != 1 {
		t.Errorf("expected 1 box (no empty HTML inserted), got %d", len(result))
	}
}

// ============================================
// 第四部分：集成测试 - 验证重构后功能保持一致
// ============================================

func TestExtractTableAndReplace_Integration(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "intro", LayoutType: pdf.LayoutTypeText, PageNumber: 0, X0: 10, X1: 100, Top: 10, Bottom: 30},
		{Text: "table box", LayoutType: pdf.LayoutTypeTable, PageNumber: 0, X0: 10, X1: 400, Top: 40, Bottom: 150},
		{Text: "outro", LayoutType: pdf.LayoutTypeText, PageNumber: 0, X0: 10, X1: 100, Top: 160, Bottom: 190},
	}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 40, Bottom: 150}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 40, RegionBottom: 150,
			Scale: 1.0,
			Cells: []pdf.TSRCell{
				{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A"},
				{X0: 100, Y0: 0, X1: 200, Y1: 50, Text: "B"},
			},
		},
	}

	result := ExtractTableAndReplace(boxes, tables)
	if len(result) != 3 {
		t.Errorf("expected 3 boxes, got %d", len(result))
	}
}

func TestExtractTableAndReplace_NoTables(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "text1", PageNumber: 0},
		{Text: "text2", PageNumber: 0},
	}
	tables := []pdf.TableItem{}

	result := ExtractTableAndReplace(boxes, tables)
	if len(result) != 2 {
		t.Errorf("expected 2 boxes unchanged, got %d", len(result))
	}
}

func TestExtractTableAndReplace_DataSourceBox(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "数据来源: somewhere", LayoutType: pdf.LayoutTypeTable, PageNumber: 0},
		{Text: "normal text", LayoutType: pdf.LayoutTypeText, PageNumber: 0},
	}
	tables := []pdf.TableItem{}

	result := ExtractTableAndReplace(boxes, tables)
	if len(result) != 1 {
		t.Errorf("expected 1 box (data source removed), got %d", len(result))
	}
	if result[0].Text != "normal text" {
		t.Errorf("expected normal text to remain, got %q", result[0].Text)
	}
}

func TestExtractTableAndReplace_ZeroBoxesWithTables(t *testing.T) {
	boxes := []pdf.TextBox{}
	tables := []pdf.TableItem{
		{
			Scale: 1.0,
			Cells: []pdf.TSRCell{
				{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "A"},
			},
		},
	}

	result := ExtractTableAndReplace(boxes, tables)
	if len(result) != 1 {
		t.Errorf("expected 1 table box for zero input boxes, got %d", len(result))
	}
}

// ============================================
// 第五部分：FilterBoxesByRemoveSet 单元测试
// ============================================

func TestFilterBoxesByRemoveSet_EmptyRemoveSet(t *testing.T) {
	boxes := []pdf.TextBox{{Text: "a"}, {Text: "b"}, {Text: "c"}}
	removeSet := map[int]bool{}

	result := FilterBoxesByRemoveSet(boxes, removeSet)
	if len(result) != 3 {
		t.Errorf("expected all boxes to remain, got %d", len(result))
	}
}

func TestFilterBoxesByRemoveSet_RemoveSome(t *testing.T) {
	boxes := []pdf.TextBox{{Text: "keep0"}, {Text: "remove1"}, {Text: "keep2"}, {Text: "remove3"}}
	removeSet := map[int]bool{1: true, 3: true}

	result := FilterBoxesByRemoveSet(boxes, removeSet)
	if len(result) != 2 {
		t.Errorf("expected 2 boxes, got %d", len(result))
	}
	if result[0].Text != "keep0" || result[1].Text != "keep2" {
		t.Errorf("unexpected filtered result: %+v", result)
	}
}

func TestFilterBoxesByRemoveSet_RemoveAll(t *testing.T) {
	boxes := []pdf.TextBox{{Text: "a"}, {Text: "b"}}
	removeSet := map[int]bool{0: true, 1: true}

	result := FilterBoxesByRemoveSet(boxes, removeSet)
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d", len(result))
	}
}

func TestFilterBoxesByRemoveSet_EmptyInput(t *testing.T) {
	var boxes []pdf.TextBox
	removeSet := map[int]bool{0: true}

	result := FilterBoxesByRemoveSet(boxes, removeSet)
	if len(result) != 0 {
		t.Errorf("expected empty result for empty input, got %d", len(result))
	}
}

func TestFilterBoxesByRemoveSet_Preallocation(t *testing.T) {
	// 验证容量预分配是否合理
	boxes := make([]pdf.TextBox, 100)
	removeSet := map[int]bool{}
	for i := 0; i < 30; i++ {
		removeSet[i] = true // 标记 30 个要移除
	}

	result := FilterBoxesByRemoveSet(boxes, removeSet)
	if len(result) != 70 {
		t.Errorf("expected 70 boxes, got %d", len(result))
	}
	// 验证容量至少为 70
	if cap(result) < 70 {
		t.Errorf("expected capacity >= 70, got %d", cap(result))
	}
}

// ============================================
// 第六部分：createTableBoxFromItem 单元测试
// ============================================

func TestCreateTableBoxFromItem_Basic(t *testing.T) {
	table := &pdf.TableItem{
		RegionLeft:   10,
		RegionRight:  400,
		RegionTop:    60,
		RegionBottom: 200,
		Positions: []pdf.Position{{
			PageNumbers: []int{1},
		}},
	}

	box := createTableBoxFromItem(table, "<table>test</table>")
	if box.Text != "<table>test</table>" {
		t.Errorf("expected HTML text, got %q", box.Text)
	}
	if box.LayoutType != pdf.LayoutTypeTable {
		t.Errorf("expected table layout, got %v", box.LayoutType)
	}
	if box.PageNumber != 1 {
		t.Errorf("expected page 1, got %d", box.PageNumber)
	}
	if box.X0 != 10 || box.X1 != 400 || box.Top != 60 || box.Bottom != 200 {
		t.Errorf("expected correct coordinates, got (%.0f,%.0f,%.0f,%.0f)", box.X0, box.X1, box.Top, box.Bottom)
	}
}

func TestCreateTableBoxFromItem_FallbackToPosition(t *testing.T) {
	// Region 字段为空时，使用 Position 的坐标
	table := &pdf.TableItem{
		Positions: []pdf.Position{{
			PageNumbers: []int{2},
			Left:        20,
			Right:       300,
			Top:         50,
			Bottom:      150,
		}},
	}

	box := createTableBoxFromItem(table, "<table>fallback</table>")
	if box.X0 != 20 || box.X1 != 300 || box.Top != 50 || box.Bottom != 150 {
		t.Errorf("expected fallback coordinates, got (%.0f,%.0f,%.0f,%.0f)", box.X0, box.X1, box.Top, box.Bottom)
	}
}

func TestCreateTableBoxFromItem_EmptyPositions(t *testing.T) {
	// 没有 Positions 时也能工作
	table := &pdf.TableItem{
		RegionLeft:   10,
		RegionRight:  100,
		RegionTop:    10,
		RegionBottom: 100,
	}

	box := createTableBoxFromItem(table, "<table>empty-pos</table>")
	if box.PageNumber != 0 {
		t.Errorf("expected page 0, got %d", box.PageNumber)
	}
}

// ============================================
// 第七部分：handleImageOnlyPDFs 单元测试
// ============================================

func TestHandleImageOnlyPDFs_EmptyTables(t *testing.T) {
	result := handleImageOnlyPDFs([]pdf.TableItem{})
	if len(result) != 0 {
		t.Errorf("expected empty result, got %d boxes", len(result))
	}
}

func TestHandleImageOnlyPDFs_EmptyCells(t *testing.T) {
	tables := []pdf.TableItem{
		{Cells: []pdf.TSRCell{}}, // 没有 cell 的 table
	}
	result := handleImageOnlyPDFs(tables)
	if len(result) != 0 {
		t.Errorf("expected no boxes for empty cells, got %d", len(result))
	}
}

func TestHandleImageOnlyPDFs_SingleTable(t *testing.T) {
	tables := []pdf.TableItem{
		{
			Scale:        1.0,
			CropOffX:     0,
			CropOffY:     0,
			RegionLeft:   10,
			RegionRight:  200,
			RegionTop:    20,
			RegionBottom: 100,
			Positions:    []pdf.Position{{PageNumbers: []int{0}}},
			Cells: []pdf.TSRCell{
				{X0: 0, Y0: 0, X1: 100, Y1: 50, Text: "cell1"},
			},
		},
	}
	result := handleImageOnlyPDFs(tables)
	if len(result) != 1 {
		t.Errorf("expected 1 box, got %d", len(result))
	}
	if result[0].LayoutType != pdf.LayoutTypeTable {
		t.Error("expected table layout type")
	}
}

func TestHandleImageOnlyPDFs_MultipleTables(t *testing.T) {
	tables := []pdf.TableItem{
		{
			Scale:      1.0,
			RegionLeft: 10, RegionRight: 200,
			RegionTop: 20, RegionBottom: 100,
			Positions: []pdf.Position{{PageNumbers: []int{0}}},
			Cells:     []pdf.TSRCell{{Text: "table1"}},
		},
		{
			// 没有 cell 的 table，应该被跳过
			Cells: []pdf.TSRCell{},
		},
		{
			Scale:      1.0,
			RegionLeft: 10, RegionRight: 200,
			RegionTop: 120, RegionBottom: 200,
			Positions: []pdf.Position{{PageNumbers: []int{1}}},
			Cells:     []pdf.TSRCell{{Text: "table2"}},
		},
	}
	result := handleImageOnlyPDFs(tables)
	if len(result) != 2 {
		t.Errorf("expected 2 boxes, got %d", len(result))
	}
}

// ============================================
// 阶段 2: buildAndSortAnchors 和 processTablesWithReplacements
// ============================================

func TestBuildAndSortAnchors(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "text1", PageNumber: 0, Top: 10},
		{Text: "table1", LayoutType: pdf.LayoutTypeTable, PageNumber: 0, Top: 50},
		{Text: "text2", PageNumber: 0, Top: 100},
	}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 50, Bottom: 80}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 50, RegionBottom: 80,
			Cells: []pdf.TSRCell{{Text: "cell1"}},
		},
	}
	_, replacements := buildReplacements(boxes, tables)

	anchors := findTableAnchorsWithReplacements(boxes, tables, replacements)
	result := buildAndSortAnchors(anchors)

	if len(result) != 1 {
		t.Errorf("expected 1 anchor, got %d", len(result))
	}
}

// ============================================
// 第八部分：ConsolidateFigures 子函数的单元测试
// ============================================

func TestMarkDataSourceBoxesForRemoval(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "数据来源: test", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0},
		{Text: "资料来源：abc", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0},
		{Text: "图表来源 def", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0},
		{Text: "正常图片内容", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0},
		{Text: "数据来源: 不应该移除", LayoutType: pdf.LayoutTypeText, PageNumber: 0}, // 不是 figure 类型
	}

	removeSet := markDataSourceBoxesForRemoval(boxes)
	if len(removeSet) != 3 {
		t.Errorf("expected 3 boxes marked for removal, got %d", len(removeSet))
	}
	if !removeSet[0] || !removeSet[1] || !removeSet[2] {
		t.Error("expected boxes 0, 1, 2 to be marked for removal")
	}
	if removeSet[3] || removeSet[4] {
		t.Error("expected boxes 3 and 4 NOT to be marked")
	}
}

func TestGroupFigureBoxes(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "fig1-part1", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0, LayoutNo: "fig-0"},
		{Text: "fig1-part2", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0, LayoutNo: "fig-0"},
		{Text: "fig2", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0, LayoutNo: "fig-1"},
		{Text: "fig3", LayoutType: pdf.LayoutTypeFigure, PageNumber: 1, LayoutNo: "fig-0"}, // 不同页面
		{Text: "text", LayoutType: pdf.LayoutTypeText, PageNumber: 0},
	}
	removeSet := map[int]bool{}

	groups := groupFigureBoxes(boxes, removeSet)
	if len(groups) != 3 {
		t.Errorf("expected 3 groups, got %d", len(groups))
	}

	// 验证组的内容
	key1 := figKey{page: 0, ln: "fig-0"}
	if len(groups[key1]) != 2 {
		t.Errorf("expected 2 boxes in fig-0 group, got %d", len(groups[key1]))
	}
	key2 := figKey{page: 0, ln: "fig-1"}
	if len(groups[key2]) != 1 {
		t.Errorf("expected 1 box in fig-1 group, got %d", len(groups[key2]))
	}
	key3 := figKey{page: 1, ln: "fig-0"}
	if len(groups[key3]) != 1 {
		t.Errorf("expected 1 box in page 1 fig-0 group, got %d", len(groups[key3]))
	}
}

func TestMergeFigureGroups(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "part1", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0, LayoutNo: "fig-0",
			X0: 10, X1: 100, Top: 10, Bottom: 50},
		{Text: "part2", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0, LayoutNo: "fig-0",
			X0: 50, X1: 150, Top: 30, Bottom: 80},
		{Text: "single", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0, LayoutNo: "fig-1",
			X0: 200, X1: 300, Top: 10, Bottom: 50},
	}
	removeSet := make(map[int]bool)
	groups := map[figKey][]int{
		{page: 0, ln: "fig-0"}: {0, 1},
		{page: 0, ln: "fig-1"}: {2},
	}

	mergeFigureGroups(boxes, groups, removeSet)

	// 验证合并后的结果
	if boxes[0].Text != "part1\npart2" {
		t.Errorf("expected merged text, got %q", boxes[0].Text)
	}
	if boxes[0].X0 != 10 || boxes[0].X1 != 150 || boxes[0].Top != 10 || boxes[0].Bottom != 80 {
		t.Error("expected merged bounding box")
	}
	if !removeSet[1] {
		t.Error("expected box 1 to be marked for removal")
	}
	if removeSet[2] {
		t.Error("expected single box NOT to be marked for removal")
	}
}

func TestConsolidateFigures_Integration(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "数据来源: test", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0, LayoutNo: "fig-0"},
		{Text: "fig1-part1", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0, LayoutNo: "fig-0",
			X0: 10, X1: 100, Top: 10, Bottom: 50},
		{Text: "fig1-part2", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0, LayoutNo: "fig-0",
			X0: 50, X1: 150, Top: 30, Bottom: 80},
		{Text: "normal text", LayoutType: pdf.LayoutTypeText, PageNumber: 0},
	}

	result := ConsolidateFigures(boxes)

	// 验证结果
	if len(result) != 2 { // 合并后的 figure + normal text
		t.Errorf("expected 2 boxes, got %d", len(result))
	}

	// 检查 figure 是否正确合并
	var figureFound bool
	for _, b := range result {
		if b.LayoutType == pdf.LayoutTypeFigure {
			figureFound = true
			if b.Text != "fig1-part1\nfig1-part2" {
				t.Errorf("expected merged figure text, got %q", b.Text)
			}
		}
	}
	if !figureFound {
		t.Error("expected figure box in result")
	}
}

func TestConsolidateFigures_NoFigures(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "text1", LayoutType: pdf.LayoutTypeText, PageNumber: 0},
		{Text: "text2", LayoutType: pdf.LayoutTypeText, PageNumber: 0},
	}

	result := ConsolidateFigures(boxes)
	if len(result) != 2 {
		t.Errorf("expected 2 boxes unchanged, got %d", len(result))
	}
}

func TestConsolidateFigures_OnlyDataSource(t *testing.T) {
	boxes := []pdf.TextBox{
		{Text: "数据来源: test", LayoutType: pdf.LayoutTypeFigure, PageNumber: 0},
	}

	result := ConsolidateFigures(boxes)
	if len(result) != 0 {
		t.Errorf("expected 0 boxes (data source removed), got %d", len(result))
	}
}

func TestExtractTableAndReplace_MergeTablesAcrossPages(t *testing.T) {
	// Regression test: two tables on consecutive pages with overlapping X
	// should be merged by MergeTablesAcrossPages, and buildReplacementsAfterMerge
	// must correctly index into the merged slice (not the original pre-merge slice).
	boxes := []pdf.TextBox{
		{Text: "intro", LayoutType: pdf.LayoutTypeText, PageNumber: 0, X0: 10, X1: 100, Top: 10, Bottom: 30},
		{Text: "table1", LayoutType: pdf.LayoutTypeTable, PageNumber: 0, X0: 10, X1: 400, Top: 40, Bottom: 150},
		{Text: "middle", LayoutType: pdf.LayoutTypeText, PageNumber: 0, X0: 10, X1: 100, Top: 160, Bottom: 190},
		{Text: "table2", LayoutType: pdf.LayoutTypeTable, PageNumber: 1, X0: 10, X1: 400, Top: 10, Bottom: 120},
		{Text: "outro", LayoutType: pdf.LayoutTypeText, PageNumber: 1, X0: 10, X1: 100, Top: 130, Bottom: 160},
	}
	tables := []pdf.TableItem{
		{
			Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 400, Top: 40, Bottom: 150}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 40, RegionBottom: 150,
			Scale: 1.0,
			Cells: []pdf.TSRCell{
				{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Page0_A"},
				{X0: 100, Y0: 0, X1: 200, Y1: 30, Text: "Page0_B"},
			},
		},
		{
			Positions:  []pdf.Position{{PageNumbers: []int{1}, Left: 10, Right: 400, Top: 10, Bottom: 120}},
			RegionLeft: 10, RegionRight: 400, RegionTop: 10, RegionBottom: 120,
			Scale: 1.0,
			Cells: []pdf.TSRCell{
				{X0: 0, Y0: 50, X1: 100, Y1: 80, Text: "Page1_C"},
				{X0: 100, Y0: 50, X1: 200, Y1: 80, Text: "Page1_D"},
			},
		},
	}

	result := ExtractTableAndReplace(boxes, tables)
	if len(result) == 0 {
		t.Fatal("expected non-empty result")
	}
	// After merge: 2 table boxes replaced by 1 merged HTML box.
	// Original 5 boxes → 4 expected (intro, merged_table, middle, outro).
	if len(result) != 4 {
		t.Errorf("expected 4 boxes after merge+replace, got %d", len(result))
	}
	// The merged HTML box should contain cells from both pages.
	htmlBox := result[1]
	if !strings.Contains(htmlBox.Text, "Page0") || !strings.Contains(htmlBox.Text, "Page1") {
		t.Errorf("merged HTML should contain cells from both pages, got: %s", htmlBox.Text[:min(100, len(htmlBox.Text))])
	}
	// Verify the original text boxes are preserved in the right order.
	if result[0].Text != "intro" || result[2].Text != "middle" || result[3].Text != "outro" {
		t.Error("non-table boxes should be preserved in original order")
	}
}
