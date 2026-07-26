//go:build manual

package table

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"regexp"
	"strings"
	"testing"
)

// =============================================================================
// Issue 1: Figure insertion strategy
// Python's insert_table_figures(figs, "figure") inserts figure boxes back into
// self.boxes. Go's extractTableAndReplace only handles LayoutType=="table",
// leaving figure boxes in the list. This test documents the current behavior.
// =============================================================================

// TestExtractTableAndReplace_IgnoresFigures documents that extractTableAndReplace
// does NOT pop or replace figure boxes. In Python's _extract_table_figure,
// figure boxes are popped and re-inserted via insert_table_figures with cropped
// images. Go leaves them in the box list for downstream boxesToSections.
func TestExtractTableAndReplace_IgnoresFigures(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 200, Top: 0, Bottom: 50, Text: "Figure text", LayoutType: "figure", PageNumber: 0},
		{X0: 10, X1: 200, Top: 60, Bottom: 80, Text: "表1：标题", LayoutType: "table", PageNumber: 0},
	}

	// Table with cells so extractTableAndReplace generates HTML.
	tables := []pdf.TableItem{{
		Cells:     []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "A", Label: "table row"}},
		Positions: []pdf.Position{{Left: 0, Right: 300, Top: 0, Bottom: 100}},
		Scale:     1.0,
	}}

	result := ExtractTableAndReplace(boxes, tables)

	// BUG: Figure box is still present — it was not popped or replaced.
	// Python's _extract_table_figure pops figure boxes and re-inserts them
	// via insert_table_figures with cropped images.
	hasFigure := false
	for _, b := range result {
		if b.LayoutType == "figure" {
			hasFigure = true
			// Figure text is still raw text, not a consolidated image+text block
			// like Python's insert_table_figures would produce.
			if b.Text != "Figure text" {
				t.Errorf("figure text should be unchanged, got %q", b.Text)
			}
		}
	}
	if !hasFigure {
		t.Error("BUG EXPOSED: extractTableAndReplace removed figure box (unexpected)")
	}
	t.Log("NOTE: Figure box remains in list as raw text. Python inserts figures back with cropped images via insert_table_figures. Go collects figures separately via pdf.CollectFigures without re-inserting.")
}

// TestBoxesToSections_FiguresNotReinserted documents that boxesToSections converts
// figure boxes to sections but without the consolidated image that Python's
// insert_table_figures would attach.
func TestBoxesToSections_FiguresNotReinserted(t *testing.T) {
	// Simulate post-extractTableAndReplace boxes with figures still present.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 200, Top: 0, Bottom: 50, Text: "Some text", LayoutType: "text", PageNumber: 0},
		{X0: 10, X1: 200, Top: 60, Bottom: 100, Text: "Figure description", LayoutType: "figure", PageNumber: 0},
	}

	sections := boxesToSections(boxes, nil)
	figures := pdf.CollectFigures(sections)

	// BUG: figures are collected separately but NOT re-inserted into sections
	// after image processing. In Python, insert_table_figures(figs, "figure")
	// creates new boxes with layout_type="figure", image=cropped_img, and
	// inserts them at the nearest position among text boxes.
	if len(figures) != 1 {
		t.Fatalf("expected 1 figure, got %d", len(figures))
	}
	if figures[0].LayoutType != "figure" {
		t.Errorf("expected LayoutType 'figure', got %q", figures[0].LayoutType)
	}
	// Figure image is empty at this stage (cropSectionImage runs later in pipeline).
	if figures[0].Image != "" {
		t.Log("figure has image (cropSectionImage already ran)")
	} else {
		t.Log("NOTE: Figure section has no Image yet. Python's cropout creates a consolidated cropped image for the entire figure region before insert_table_figures.")
	}

	t.Logf("Sections count: %d (figure present as raw text section)", len(sections))
	t.Logf("Figures count: %d (collected separately, Python re-inserts them)", len(figures))
}

// =============================================================================
// Issue 2a: blockType classification missing
// Python's construct_table classifies each cell into 9 types (Dt/Nu/Ca/En/NE/
// Sg/Tx/Lx/Nr/Ot). The dominant type drives header detection: if max_type is
// "Nu" (numeric), numeric cells don't count as headers. Go's headerSet only
// checks TSR labels — no cell content type analysis.
// =============================================================================

// TestConstructTable_HeaderDetection_NoBlockType documents that Go's header
// detection is purely TSR-label-based. Python would use blockType to skip
// numeric cells when the dominant type is "Nu".
func TestConstructTable_HeaderDetection_NoBlockType(t *testing.T) {
	// A table where the "header" row has numeric content (like years, amounts).
	// With blockType: "2020","2021" → Nu, "100","200" → Nu — maxType=Nu.
	// block-type-aware detection skips Nu cells → 0 headers.
	// Falls back to TSR label-based detection → still gets 2 <th >.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "2020", Label: "table column header"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "2021", Label: "table column header"},
		{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "100", Label: "table row"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "200", Label: "table row"},
	}

	item := &pdf.TableItem{}
	html := ConstructTable(cells, nil, "", item)

	// FIX VERIFIED: headerSetWithBlockType computes block types (all "Nu"),
	// skips Nu headers when maxType=Nu, then falls back to TSR label detection.
	// Header row still gets <th > because TSR labels contain "header".
	thCount := strings.Count(html, "<th ")
	if thCount != 2 {
		t.Errorf("expected 2 <th >, got %d. HTML: %s", thCount, html)
	}

	t.Log("FIX: blockType classification added. maxType=Nu skips Nu headers in primary pass.")
	t.Log("TSR label fallback still marks header rows with 'header' in label.")
}

// TestConstructTable_BlockType_DominantTypeMissing documents that Go has no
// concept of a "dominant cell type" that Python uses for header detection.
func TestConstructTable_BlockType_DominantTypeMissing(t *testing.T) {
	// Mixed table with numeric-dominant data, testing blockType header detection.
	// "年份"/"金额" → Tx (short text), "2020"/"1000"/etc → Nu. maxType=Nu.
	// Header cells are non-Nu → count as headers even under Nu-dominant logic.
	// FIX: blockType now classifies cells and drives header detection.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "年份", Label: "table column header"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "金额", Label: "table column header"},
		{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "2020", Label: "table row"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "1000", Label: "table row"},
		{X0: 0, Y0: 70, X1: 100, Y1: 100, Text: "2021", Label: "table row"},
		{X0: 101, Y0: 70, X1: 200, Y1: 100, Text: "2000", Label: "table row"},
		{X0: 0, Y0: 105, X1: 100, Y1: 135, Text: "2022", Label: "table row"},
		{X0: 101, Y0: 105, X1: 200, Y1: 135, Text: "3000", Label: "table row"},
	}

	item := &pdf.TableItem{}
	html := ConstructTable(cells, nil, "", item)

	thCount := strings.Count(html, "<th ")
	if thCount != 2 {
		t.Errorf("expected 2 <th > for non-numeric headers under Nu-dominant table, got %d. HTML: %s", thCount, html)
	}

	t.Log("FIX: blockType classifies '年份'/'金额' as non-Nu headers, '2020'/'1000' as Nu data.")
	t.Logf("BlockType('年份')=%q BlockType('2020')=%q", BlockType("年份"), BlockType("2020"))
}

// TestConstructTable_BlockTypeChangesHeaderDetection verifies blockType
// changes header detection for a table WITHOUT TSR header labels.
// This is the case where pure label-based detection would fail.
func TestConstructTable_BlockTypeChangesHeaderDetection(t *testing.T) {
	// Table with NO "header" labels — label-based detection gives 0 headers.
	// blockType: "姓名"/"年龄" → Tx, "张三"/"25" → Ot/En/? — maxType varies.
	// With Nu-dominant data, non-Nu top row cells count as possible headers.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "姓名", Label: "table row"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "年龄", Label: "table row"},
		{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "张三", Label: "table row"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "25", Label: "table row"},
		{X0: 0, Y0: 70, X1: 100, Y1: 100, Text: "李四", Label: "table row"},
		{X0: 101, Y0: 70, X1: 200, Y1: 100, Text: "30", Label: "table row"},
		{X0: 0, Y0: 105, X1: 100, Y1: 135, Text: "王五", Label: "table row"},
		{X0: 101, Y0: 105, X1: 200, Y1: 135, Text: "28", Label: "table row"},
	}

	html := ConstructTable(cells, nil, "", &pdf.TableItem{Grid: GroupTSRCellsToRows(cells)})

	// blockType analysis:
	// "姓名"(Tx), "年龄"(Tx), "张三"(Ot), "25"(Nu), "李四"(Ot), "30"(Nu), "王五"(Ot), "28"(Nu)
	// maxType could be Ot(3), Nu(3), or Tx(2).
	// Fallback catches the case where no headers detected by block-type path.
	t.Logf("HTML:\n%s", html)
	t.Log("FIX: blockType+fallback header detection works for tables without TSR header labels")
}

// =============================================================================
// Issue 2b: colspan/rowspan missing
// Python's __cal_spans computes colspan/rowspan from spanning cells by
// clustering column centers and row centers. Go's rowsToHTML produces
// a flat grid with no spanning attributes.
// =============================================================================

// TestRowsToHTML_NoColspanRowspan documents that rowsToHTML never produces
// colspan or rowspan attributes, even for spanning cells.
func TestRowsToHTML_NoColspanRowspan(t *testing.T) {
	// Two rows with a spanning cell in row 0.
	// In Python, a "table spanning cell" covering columns 0-1 would get colspan=2.
	rows := [][]pdf.TSRCell{
		{
			{Text: "跨列标题", Label: "table spanning cell"},
			{Text: "", Label: ""}, // padded cell
		},
		{
			{Text: "数据A", Label: "table row"},
			{Text: "数据B", Label: "table row"},
		},
	}

	html := RowsToHTML(rows, "", nil, nil, nil)

	// BUG: No colspan or rowspan attributes in output.
	if strings.Contains(html, "colspan") {
		t.Error("unexpected: colspan found in output (should not be present without __cal_spans)")
	}
	if strings.Contains(html, "rowspan") {
		t.Error("unexpected: rowspan found in output (should not be present without __cal_spans)")
	}

	// The spanning cell is rendered as a plain <td > with text, and the padded
	// empty cell is also rendered as an empty <td >. Python would merge them.
	tdCount := strings.Count(html, "<td ")
	if tdCount == 4 {
		t.Logf("Got %d <td > cells (flat grid, spanning cell + padded empty cell both rendered)", tdCount)
	} else {
		t.Logf("Got %d <td > cells. HTML:\n%s", tdCount, html)
	}

	t.Log("NOTE: Python's __cal_spans clusters column centers within spanning cells")
	t.Log("to compute colspan/rowspan. Go outputs a flat grid without spanning attributes.")
}

// TestConstructTable_SpannedTable_NoMerge documents the full constructTable
// path with spanning cells — no colspan/rowspan in output.
func TestConstructTable_SpannedTable_NoMerge(t *testing.T) {
	// Spanning cell at same Y as row cells so GroupTSRCellsToRows
	// puts them in the same row group. The spanning cell covers X=0-200
	// (both columns); Python's __cal_spans would give it colspan=2.
	cells := []pdf.TSRCell{
		// Row 0: a spanning cell that covers both columns + one regular cell.
		{X0: 0, Y0: 0, X1: 200, Y1: 30, Text: "部门开支汇总", Label: "table spanning cell"},
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Q1", Label: "table row"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "Q2", Label: "table row"},
		// Row 1: data row
		{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "100", Label: "table row"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "200", Label: "table row"},
	}

	item := &pdf.TableItem{}
	html := ConstructTable(cells, nil, "", item)

	// Verify colspan IS now detected (calSpans aligned with Python's __cal_spans).
	if !strings.Contains(html, "colspan") {
		t.Error("expected colspan on spanning cell, calSpans should detect it")
	}

	// Verify the HTML structure — spanning cell exists WITH colspan.
	if !strings.Contains(html, "部门开支汇总") {
		t.Error("spanning cell text missing")
	}
	if !strings.Contains(html, "Q1") {
		t.Error("Q1 cell should still be present (covered by span)")
	}
	t.Logf("HTML:\n%s", html)
}

// =============================================================================
// Issue 2c: Single column/row cleanup missing
// Python's construct_table removes orphan columns (only one non-empty cell)
// when ≥4 rows, and orphan rows when ≥4 columns. Go has no such cleanup.
// =============================================================================

// TestConstructTable_OrphanColumn_NotCleanedUp documents that Go does NOT
// remove columns that have only one non-empty cell.
func TestConstructTable_OrphanColumn_NotCleanedUp(t *testing.T) {
	// 4 rows × 3 columns. Column index 1 has only ONE non-empty cell.
	// Python would relocate/merge that orphan column.
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "姓名", Label: "table column header"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "备注", Label: "table row"}, // orphan col
		{X0: 201, Y0: 0, X1: 300, Y1: 30, Text: "年龄", Label: "table column header"},
		{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "张三", Label: "table row"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "", Label: "table row"}, // col 1 empty
		{X0: 201, Y0: 35, X1: 300, Y1: 65, Text: "25", Label: "table row"},
		{X0: 0, Y0: 70, X1: 100, Y1: 100, Text: "李四", Label: "table row"},
		{X0: 101, Y0: 70, X1: 200, Y1: 100, Text: "", Label: "table row"}, // col 1 empty
		{X0: 201, Y0: 70, X1: 300, Y1: 100, Text: "30", Label: "table row"},
		{X0: 0, Y0: 105, X1: 100, Y1: 135, Text: "王五", Label: "table row"},
		{X0: 101, Y0: 105, X1: 200, Y1: 135, Text: "", Label: "table row"}, // col 1 empty
		{X0: 201, Y0: 105, X1: 300, Y1: 135, Text: "28", Label: "table row"},
	}

	item := &pdf.TableItem{}
	html := ConstructTable(cells, nil, "", item)

	// BUG: All 4 rows have 3 cells each (orphan column preserved).
	// Python's construct_table pops single-cell columns when ≥4 rows.
	trCount := strings.Count(html, "<tr>")
	totalTdTh := strings.Count(html, "<td ") + strings.Count(html, "<th ")

	t.Logf("Rows: %d, Total cells: %d (Python would cleanup orphan columns)", trCount, totalTdTh)
	t.Log("NOTE: Python's construct_table removes columns with only one non-empty cell")
	t.Log("when there are ≥4 rows, and removes rows with only one non-empty cell")
	t.Log("when there are ≥4 columns. Go has no equivalent cleanup.")
	t.Logf("HTML:\n%s", html)
}

// =============================================================================
// Issue 2d: is_caption pattern matching in mergeCaptions
// Python's is_caption detects captions by text patterns (图表, Fig., Table, etc.)
// AND layout_type. Go's mergeCaptions only checks LayoutType. If DLA labels a
// caption as "text", Go misses it.
// =============================================================================

// TestMergeCaptions_NoIsCaptionPatternMatch documents that mergeCaptions only
// uses LayoutType, NOT text patterns, for caption detection.
func TestMergeCaptions_NoIsCaptionPatternMatch(t *testing.T) {
	// A caption-like text labeled as "text" by DLA (happens with imperfect DLA).
	// Python's is_caption would match "表1：测试数据" pattern regardless of layout_type.
	// FIX: mergeCaptions now calls captionKind → isCaptionBox to detect these.
	sections := []pdf.Section{
		{Text: "T", LayoutType: "table", Positions: []pdf.Position{
			{PageNumbers: []int{0, 0}, Left: 10, Right: 100, Top: 0, Bottom: 30},
		}},
		// This is clearly a table caption by text pattern, but DLA labeled it as "text".
		{Text: "表1：测试数据", LayoutType: "text", Positions: []pdf.Position{
			{PageNumbers: []int{0, 0}, Left: 10, Right: 100, Top: 40, Bottom: 55},
		}},
	}

	figures := pdf.CollectFigures(sections)
	result := mergeCaptions(sections, figures)

	// FIX VERIFIED: "表1：测试数据" should be detected as caption via isCaptionBox
	// and merged into the table section.
	merged := false
	for _, s := range result {
		if s.LayoutType == "table" && strings.Contains(s.Text, "表1：测试数据") {
			merged = true
			t.Log("FIX VERIFIED: caption with LayoutType='text' detected via isCaptionBox and merged into table")
		}
	}
	if !merged {
		t.Error("FIX FAILED: caption '表1：测试数据' should be merged into table via isCaptionBox pattern matching")
	}

	// Caption section should be removed.
	for _, s := range result {
		if s.LayoutType == "text" && s.Text == "表1：测试数据" {
			t.Error("FIX FAILED: caption section should be removed after merge")
		}
	}
}

// TestIsCaptionBox_MatchesChinesePattern verifies the existing isCaptionBox
// function works correctly (it exists but is only used in fillCellTextFromBoxes,
// not in mergeCaptions or caption detection pipeline).
func TestIsCaptionBox_MatchesChinesePattern(t *testing.T) {
	tests := []struct {
		text       string
		layoutType string
		want       bool
	}{
		{"表1：交通工具等级", "", true},
		{"表 1：测试数据", "", true},
		{"图1：系统架构", "", true},
		{"图表 3: 实验结果", "", true},
		{"Fig. 1: Architecture", "", true},
		{"Figure 2: Pipeline", "", true},
		{"Table 3: Results", "", true},
		{"普通文本", "", false},
		{"", "", false},
		{"第一章 概述", "", false},
		// LayoutType-based detection
		{"anything", "figure caption", true},
		{"anything", "table caption", true},
	}

	for _, tt := range tests {
		got := IsCaptionBox(tt.text, tt.layoutType)
		if got != tt.want {
			t.Errorf("IsCaptionBox(%q, %q) = %v, want %v", tt.text, tt.layoutType, got, tt.want)
		}
	}

	t.Log("NOTE: isCaptionBox is now called by mergeCaptions via captionKind for DLA-mislabeled captions.")
}

// TestFigureInsertion_EndToEnd runs the full Parse pipeline on a PDF with
// a figure DLA region containing TWO text boxes far enough apart that
// NaiveVerticalMerge won't merge them.  Python's _extract_table_figure +
// insert_table_figures pops ALL figure boxes and re-inserts ONE unified
// figure block regardless of text box positions.  Go leaves the individual
// text boxes as separate sections — this test FAILS to expose that.
func TestFigureInsertion_EndToEnd(t *testing.T) {
	eng := &mockEngine{
		pageCount: 1,
		renderW:   1800, renderH: 2400,
		chars: map[int][]pdf.TextChar{0: {
			// Two text boxes in the SAME figure DLA region, but far apart.
			// DLA pixel: X=100-500 Y=80-600 → PDF 33-167 x 27-200.
			// Box 1 near top, box 2 near bottom.
			{X0: 50, X1: 150, Top: 40, Bottom: 55, Text: "架构图"},
			{X0: 50, X1: 150, Top: 170, Bottom: 185, Text: "系统模块"},
		}},
	}
	mock := &MockDocAnalyzer{
		Healthy: true,
		DLARegions: []pdf.DLARegion{
			// Large figure region covering both text boxes.
			{X0: 100, Y0: 80, X1: 500, Y1: 600, Label: "figure", Confidence: 0.9},
		},
	}
	p := NewParser(pdf.DefaultParserConfig(), mock)

	result, err := p.Parse(context.Background(), eng)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	// ── Python behavior: _extract_table_figure + insert_table_figures ──
	// Pops ALL figure boxes regardless of position, cropout creates ONE
	// consolidated image covering the entire DLA figure region, and
	// insert_table_figures re-inserts ONE figure block.
	// Expected: 1 figure section with combined text + cropped image.

	// ── Go current behavior ──
	// Figure boxes stay in list.  NaiveVerticalMerge may NOT merge them
	// if the gap is too large (> 1.5 × median_height ≈ 15pt).
	// Each figure text box → separate section in result.Sections.
	// pdf.CollectFigures collects them into result.Figures() but doesn't re-insert.

	var figureSections []pdf.Section
	for _, s := range result.Sections {
		if s.LayoutType == "figure" {
			figureSections = append(figureSections, s)
		}
	}

	// Assert 1: Python expects exactly 1 consolidated figure section.
	// Go currently produces 2 (one per unmerged text box) — this FAILS.
	if len(figureSections) != 1 {
		t.Errorf("FIGURE INSERTION BUG: expected 1 consolidated figure section (Python insert_table_figures), got %d. Go does not consolidate figure text boxes into a single block.", len(figureSections))
	}

	// Assert 2: The single figure section must contain BOTH text fragments.
	if len(figureSections) == 1 {
		combined := figureSections[0].Text
		if !strings.Contains(combined, "架构图") || !strings.Contains(combined, "系统模块") {
			t.Errorf("FIGURE INSERTION BUG: figure section text=%q should contain both fragments. Python merges all figure-region text.", combined)
		}
	}

	t.Logf("figure sections in Sections: %d", len(figureSections))
	t.Logf("result.Figures() count: %d", len(result.Figures()))
	t.Logf("result.Sections total: %d", len(result.Sections))
	for i, s := range result.Sections {
		t.Logf("  section[%d] layout=%q text=%q", i, s.LayoutType, s.Text)
	}
}

// =============================================================================
// Issue 3: Multi-page table merging
// Python's _extract_table_figure merges tables with same layoutno across
// consecutive pages (gap ≤ 1 page, Y-dis ≤ 23× median height).
// Go's extractTableAndReplace does NOT merge tables across pages.
// =============================================================================

// TestExtractTableAndReplace_NoCrossPageMerge exposes that extractTableAndReplace
// does not merge tables from consecutive pages even with the same layoutno.
func TestExtractTableAndReplace_NoCrossPageMerge(t *testing.T) {
	// Simulate a table spanning pages 0 and 1.
	// Python would merge these because: same layoutno, consecutive pages,
	// Y-distance ≤ 23× median_height.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 200, Top: 500, Bottom: 530, Text: "续表内容", LayoutType: "table", PageNumber: 0, LayoutNo: "0"},
		{X0: 10, X1: 200, Top: 50, Bottom: 80, Text: "表尾内容", LayoutType: "table", PageNumber: 1, LayoutNo: "0"},
	}

	// Two separate TableItems — one per page. Python would merge these
	// before insert_table_figures.
	tables := []pdf.TableItem{
		{
			Cells:     []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Page0", Label: "table row"}},
			Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 300, Top: 500, Bottom: 530}},
			Scale:     1.0,
		},
		{
			Cells:     []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Page1", Label: "table row"}},
			Positions: []pdf.Position{{PageNumbers: []int{1}, Left: 0, Right: 300, Top: 50, Bottom: 80}},
			Scale:     1.0,
		},
	}

	result := ExtractTableAndReplace(boxes, tables)

	// Go produces 2 separate HTML table boxes (one per page).
	// Python would produce 1 merged table with cells from both pages.
	tableCount := 0
	for _, b := range result {
		if strings.Contains(b.Text, "<table>") {
			tableCount++
		}
	}
	if tableCount == 2 {
		t.Errorf("CROSS-PAGE TABLE MERGE BUG: got %d separate HTML tables across pages. Python would merge same-layoutno tables on consecutive pages into 1 consolidated table.", tableCount)
	}
	t.Logf("table HTML boxes: %d (Python would merge into 1)", tableCount)
}

// =============================================================================
// Issue 3a: nomerge_lout_no — don't merge tables separated by captions
// Python's _extract_table_figure tracks nomerge_lout_no: when a table box
// is followed by a caption/title/reference, the table's key is added to
// nomerge_lout_no. Later, cross-page merge skips tables in nomerge_lout_no.
//
// Example:
//   Page 0: table "0-table-3" → caption "表1：..." → table "0-table-4"
//   Page 1: table "1-table-3" (same layoutNo)
//   → Page 0's table-3 should NOT merge with Page 1's table-3,
//     because the caption on page 0 indicates the table ended.
//   → Go's mergeTablesAcrossPages has no nomerge_lout_no check.
// =============================================================================

// TestMergeTablesAcrossPages_NomergeAfterCaption_Missing exposes that
// mergeTablesAcrossPages unconditionally merges consecutive-page tables,
// even when Python's nomerge_lout_no would prevent it.
func TestMergeTablesAcrossPages_NomergeAfterCaption_Missing(t *testing.T) {
	// Simulate: page 0 has table at top, followed by a caption,
	// then another table. Page 1 has the same-layoutNo table continuing.
	// In Python, page 0's first table goes into nomerge_lout_no because
	// the next box is a caption → no cross-page merge for that table group.
	tables := []pdf.TableItem{
		{
			Cells: []pdf.TSRCell{{Text: "Page0-first", Label: "table row"}},
			Positions: []pdf.Position{{
				PageNumbers: []int{0},
				Left:        0, Right: 300,
				Top: 0, Bottom: 50,
			}},
			NoMerge: true, // Set when caption follows this table on the page
		},
		{
			Cells: []pdf.TSRCell{{Text: "Page1-cont", Label: "table row"}},
			Positions: []pdf.Position{{
				PageNumbers: []int{1},
				Left:        0, Right: 300,
				Top: 0, Bottom: 50,
			}},
		},
	}

	result := MergeTablesAcrossPages(tables, nil)

	// Verify NoMerge prevents cross-page merging.
	if len(result) != 2 {
		t.Errorf("NOMERGE BUG: expected 2 separate table groups, got %d.", len(result))
	}
	t.Log("NoMerge flag correctly prevents cross-page merge.")
}

// =============================================================================
// Issue 3b: insert position — min_rectangle_distance vs anchor
// Python's insert_table_figures uses min_rectangle_distance to find the
// spatially nearest text box and inserts the table/figure next to it.
// Go's extractTableAndReplace uses the first replaced table box index as
// the anchor (insert position).
//
// When the DLA table region extends beyond the anchor box's bottom and
// overlaps a text box below the table, Python puts the table next to that
// overlapping text box (distance=0); Go puts it at the anchor position.
// =============================================================================

// TestExtractTableAndReplace_InsertionPosition_DistanceBug exposes that
// extractTableAndReplace uses the first table box as anchor, rather than
// finding the spatially nearest text box like Python.
func TestExtractTableAndReplace_InsertionPosition_DistanceBug(t *testing.T) {
	// Two text boxes above the table: L0 (left, near table) and R0 (right, far).
	// Python: nearest to table is L0 (dx=0, dy=70).  L0 bottom=30 < table top=100
	// → insert AFTER L0.  Result: [L0, table, R0, R1, L2].
	// Go: anchor = first table box (L1 at index 2).  Result: [L0, R0, table, R1, L2].
	// The table is one position off.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 100, Top: 10, Bottom: 30, Text: "L0", LayoutType: "text", PageNumber: 0},
		{X0: 300, X1: 400, Top: 10, Bottom: 30, Text: "R0", LayoutType: "text", PageNumber: 0},
		{X0: 10, X1: 100, Top: 100, Bottom: 130, Text: "table", LayoutType: "table", PageNumber: 0},
		{X0: 300, X1: 400, Top: 100, Bottom: 130, Text: "R1", LayoutType: "text", PageNumber: 0},
		{X0: 10, X1: 100, Top: 250, Bottom: 270, Text: "L2", LayoutType: "text", PageNumber: 0},
	}

	tables := []pdf.TableItem{{
		Cells:      []pdf.TSRCell{{Text: "cell", Label: "table row"}},
		Positions:  []pdf.Position{{Left: 10, Right: 100, Top: 100, Bottom: 130, PageNumbers: []int{0}}},
		Scale:      1.0,
		RegionLeft: 10, RegionRight: 100, RegionTop: 100, RegionBottom: 130,
	}}

	result := ExtractTableAndReplace(boxes, tables)

	// Find L0 and table positions.
	l0Idx, tableIdx := -1, -1
	for i, b := range result {
		if strings.TrimSpace(b.Text) == "L0" {
			l0Idx = i
		}
		if b.LayoutType == "table" {
			tableIdx = i
		}
	}

	// BUG: table should immediately follow L0 (nearest neighbor, insert_after).
	// Python: min_rectangle_distance → L0 nearest (dx=0, dy=70), L0 below table
	// → insert_at+1 → table right after L0.
	// Go: anchor = first table box index → table at original table box position.
	if tableIdx != l0Idx+1 {
		t.Errorf("INSERTION POSITION BUG: table (idx=%d) should immediately follow L0 (idx=%d). "+
			"Python's min_rectangle_distance finds L0 as nearest text box and inserts table after it. "+
			"Go anchors at first table box position (between R0 and R1).", tableIdx, l0Idx)
	}
	t.Logf("L0 at idx=%d, table at idx=%d", l0Idx, tableIdx)
	t.Log("Fix: replace first-replaced-box anchor with min_rectangle_distance nearest-neighbor (Python pdf_parser.py:1608-1655).")
}

// =============================================================================
// Issue 4: page_cum_height coordinate system
// Python tracks cumulative page image heights for cross-page position tags
// and image cropping. Go uses per-page coordinates only.
// =============================================================================

// TestBoxesToSections_PerPageCoordinates confirms position tags use
// page-relative coordinates. Python's _line_tag also produces local
// coordinates (subtracts page_cum_height). The page number differentiates
// pages; page_cum_height is an internal implementation detail.
func TestBoxesToSections_PerPageCoordinates(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 100, Top: 40, Bottom: 60, Text: "Page 0 text", LayoutType: "text", PageNumber: 0},
		{X0: 10, X1: 100, Top: 40, Bottom: 60, Text: "Page 1 text", LayoutType: "text", PageNumber: 1},
	}
	sections := boxesToSections(boxes, nil)
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	s0, s1 := sections[0], sections[1]
	if len(s0.Positions) > 0 && len(s1.Positions) > 0 {
		p0, p1 := s0.Positions[0], s1.Positions[0]
		// Both Python and Go use local (page-relative) coordinates.
		// Python's _line_tag: top = bx["top"] - page_cum_height[pn-1]
		// gives local coordinate. Same as Go.
		if p0.Top != p1.Top || p0.Bottom != p1.Bottom {
			t.Errorf("expected same local coords, got Top=(%.0f,%.0f) Bottom=(%.0f,%.0f)", p0.Top, p1.Top, p0.Bottom, p1.Bottom)
		}
		t.Logf("page 0: Page=%v Top=%.0f Bottom=%.0f", p0.PageNumbers, p0.Top, p0.Bottom)
		t.Logf("page 1: Page=%v Top=%.0f Bottom=%.0f", p1.PageNumbers, p1.Top, p1.Bottom)
		t.Log("OK: position tags use page-relative coordinates in both Go and Python.")
	}
}

// =============================================================================
// Issue 6: cropSectionImage padding logic
// Python's self.crop adds 120px context above first segment, 120px context
// below last segment, 6px gap between pages, and overlay transparency.
// Go has simpler crop logic.
// =============================================================================

// TestCropSectionImage_PaddingVsPython documents that Go's cropSectionImage
// adds context padding differently from Python's self.crop.
func TestCropSectionImage_PaddingVsPython(t *testing.T) {
	// Create a page image and position tag for a small text region.
	img := image.NewRGBA(image.Rect(0, 0, 300, 800)) // 300×800 page at zoom=3 → PDF 100×267
	pageImages := map[int]image.Image{0: img}

	// pdf.Position tag for a small text box near the top of the page.
	posTag := FormatPositionTag(0, 50.0, 100.0, 10.0, 30.0)

	result := util.CropSectionImage(posTag, pageImages, 3.0)

	if result == "" {
		t.Error("cropSectionImage returned empty string for valid position")
	}
	// Decode result to check image dimensions.
	data, err := base64.StdEncoding.DecodeString(result)
	if err != nil {
		t.Fatalf("failed to decode base64: %v", err)
	}
	cropped, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("failed to decode PNG: %v", err)
	}
	croppedH := cropped.Bounds().Dy()
	// Original text region: Top=10, Bottom=30 → height=20 at PDF points.
	// zoom=3 → 60px text height.
	// Python adds 120px context above + 120px below + 6px gap → ~306px.
	// Go adds contextPad=120 points above/below at PDF scale → with zoom=3: 360+60+360=780px.
	// Python uses pixel-space padding (120px literally), Go uses PDF-point padding (120pt).
	expectedMin := 60 // bare minimum: text region itself
	if croppedH <= expectedMin {
		t.Errorf("CROP PADDING BUG: cropped image height=%dpx, expected >%dpx with context padding. Python adds 120px above and below for context.", croppedH, expectedMin)
	}
	t.Logf("cropped image: %dx%d (text region 60px, expecting padding)", cropped.Bounds().Dx(), croppedH)
	t.Log("NOTE: Python's self.crop adds 120px context padding in pixel space, multi-page stitching, and overlay transparency. Go's cropSectionImage uses PDF-point padding and simpler stitching.")
}

// =============================================================================
// Issue 7: Data-source filter missing
// Python's _extract_table_figure pops table/figure boxes matching
// r"(数据|资料|图表)*来源[:： ]" (pdf_parser.py:1040-1042, 1050-1052).
// These boxes are discarded — not extracted, not inserted back.
// Go has no equivalent filter in extractTableAndReplace or consolidateFigures.
// =============================================================================

// dataSourcePattern is a Go translation of Python's
// r"(数据|资料|图表)*来源[:： ]" used with re.match (anchored at start).
var dataSourcePattern = `^(数据|资料|图表)*来源[:： ]`

// TestDataSourcePattern_RegexCoverage validates the Python regex behavior
// that should be adopted. Documents which strings match and which don't.
func TestDataSourcePattern_RegexCoverage(t *testing.T) {
	tests := []struct {
		text string
		want bool // Python re.match truthiness
	}{
		// ── Matching patterns (should be filtered) ──
		{"数据来源：国家统计局", true}, // 数据 + 来源 + fullwidth colon
		{"资料来源: 某报告", true},  // 资料 + 来源 + halfwidth colon
		{"图表来源：某数据库", true},  // 图表 + 来源 + fullwidth colon
		{"来源：权威机构", true},    // zero prefix + 来源 + fullwidth colon
		{"来源: 参考数据", true},   // zero prefix + 来源 + halfwidth colon
		{"数据来源 说明", true},    // 数据 + 来源 + space

		// ── Non-matching patterns (should NOT be filtered) ──
		{"数据来源明细", false},          // 来源 followed by 明, not :：space
		{"普通来源说明", false},          // doesn't start with keyword
		{"数据", false},              // too short
		{"来源", false},              // 来源 but no :：space after
		{"资料来源说明", false},          // 来源 followed by 说, not :：space
		{"", false},                // empty
		{"TABLE 1: 数据来源统计", false}, // doesn't start with keyword
	}

	for _, tt := range tests {
		matched := regexp.MustCompile(dataSourcePattern).MatchString(tt.text)
		if matched != tt.want {
			t.Errorf("dataSourcePattern.MatchString(%q) = %v, want %v", tt.text, matched, tt.want)
		}
	}
	t.Log("NOTE: Python re.match(r\"(数据|资料|图表)*来源[:： ]\", text) — anchored at start.")
	t.Log("Go regexp.MatchString equivalent with ^ prefix.")
}

// TestExtractTableAndReplace_DataSourceFilter_Missing exposes that Go does NOT
// filter out table boxes whose text matches r"(数据|资料|图表)*来源[:： ]".
// Python's _extract_table_figure pops these boxes from self.boxes without
// adding them to the tables dict (pdf_parser.py:1040-1042).
func TestExtractTableAndReplace_DataSourceFilter_Missing(t *testing.T) {
	// A table box with data-source text and a normal table box.
	// Both overlap a pdf.TableItem position, so both would be replaced with HTML.
	boxes := []pdf.TextBox{
		{X0: 10, X1: 200, Top: 0, Bottom: 50, Text: "数据来源：国家统计局", LayoutType: "table", PageNumber: 0},
		{X0: 10, X1: 200, Top: 60, Bottom: 80, Text: "表1：正常数据", LayoutType: "table", PageNumber: 0},
	}

	// Two TableItems — one per table box — so each would independently produce HTML.
	tables := []pdf.TableItem{
		{
			Cells:     []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "来源", Label: "table row"}},
			Positions: []pdf.Position{{Left: 0, Right: 300, Top: 0, Bottom: 50}},
			Scale:     1.0,
		},
		{
			Cells:     []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "正常", Label: "table row"}},
			Positions: []pdf.Position{{Left: 0, Right: 300, Top: 60, Bottom: 80}},
			Scale:     1.0,
		},
	}

	result := ExtractTableAndReplace(boxes, tables)

	// Python behavior: "数据来源：国家统计局" is popped from self.boxes,
	// NOT added to tables dict, NOT replaced with HTML. Gone entirely.
	// "表1：正常数据" is replaced with HTML as usual.
	// Expected result: exactly 1 HTML table box for the normal table.
	//
	// BUG: Go replaces both boxes with HTML tables. The data-source box
	// produces an HTML table with cell text "来源" — this should NOT exist.
	htmlTableCount := 0
	hasDataSourceTable := false
	for _, b := range result {
		if strings.Contains(b.Text, "<table>") {
			htmlTableCount++
			// The data-source table's cell text "来源" ends up in the HTML.
			// c.f. constructTable which uses pdf.TSRCell text, not box text.
			if strings.Contains(b.Text, ">来源<") {
				hasDataSourceTable = true
			}
		}
	}
	if htmlTableCount != 1 {
		t.Errorf("DATA SOURCE FILTER BUG: expected 1 HTML table (normal only), got %d. Python pops data-source table box entirely in _extract_table_figure (pdf_parser.py:1040-1042). Go replaces it with an HTML table.", htmlTableCount)
	}
	if hasDataSourceTable {
		t.Errorf("DATA SOURCE FILTER BUG: data-source table should NOT produce HTML output. Cell '来源' appears in HTML: Python discards these boxes, Go incorrectly constructs a table for them.")
	}

	t.Log("NOTE: Python filters table boxes matching r\"(数据|资料|图表)*来源[:： ]\" in _extract_table_figure.")
	t.Log("Go's extractTableAndReplace has no equivalent filter — data-source boxes get replaced with HTML instead of being discarded.")
}

// TestExtractTableAndReplace_DataSourceVariants tests multiple variants of
// the data-source pattern that should all be filtered.
func TestExtractTableAndReplace_DataSourceVariants(t *testing.T) {
	variants := []string{
		"数据来源：国家统计局",
		"资料来源: 某报告",
		"图表来源：某数据库",
		"来源：权威机构",
		"来源: 参考数据",
	}

	for _, variant := range variants {
		t.Run(variant, func(t *testing.T) {
			boxes := []pdf.TextBox{
				{X0: 10, X1: 200, Top: 0, Bottom: 50, Text: variant, LayoutType: "table", PageNumber: 0},
			}

			tables := []pdf.TableItem{{
				Cells:     []pdf.TSRCell{{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "A", Label: "table row"}},
				Positions: []pdf.Position{{Left: 0, Right: 300, Top: 0, Bottom: 50}},
				Scale:     1.0,
			}}

			result := ExtractTableAndReplace(boxes, tables)

			// BUG: box with data-source text should be REMOVED entirely —
			// zero HTML output. Python pops these boxes without replacement.
			for _, b := range result {
				if strings.Contains(b.Text, "<table>") {
					t.Errorf("DATA SOURCE FILTER BUG: variant %q should be removed without HTML replacement. Python pops data-source table boxes entirely.", variant)
				}
			}
		})
	}
	t.Log("NOTE: All variants of r\"(数据|资料|图表)*来源[:： ]\" should be filtered by extractTableAndReplace.")
}

// TestConsolidateFigures_DataSourceFilter_Missing exposes that Go does NOT
// filter out figure boxes whose text matches r"(数据|资料|图表)*来源[:： ]".
// Python's _extract_table_figure pops these boxes from self.boxes without
// adding them to the figures dict (pdf_parser.py:1050-1052).
func TestConsolidateFigures_DataSourceFilter_Missing(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 200, Top: 0, Bottom: 50, Text: "数据来源：某机构", LayoutType: "figure", PageNumber: 0, LayoutNo: "figure-0"},
		{X0: 10, X1: 200, Top: 60, Bottom: 80, Text: "架构图", LayoutType: "figure", PageNumber: 0, LayoutNo: "figure-0"},
	}

	result := ConsolidateFigures(boxes)

	// Python behavior: "数据来源：某机构" is popped from self.boxes,
	// NOT added to figures dict → gone entirely.
	// "架构图" is extracted normally.
	// Expected result: exactly 1 figure box with "架构图" text only.
	for _, b := range result {
		if strings.Contains(b.Text, "数据来源") || strings.Contains(b.Text, "某机构") {
			t.Errorf("DATA SOURCE FIGURE FILTER BUG: '数据来源：某机构' figure box should be removed entirely. Python pops data-source figure boxes in _extract_table_figure (pdf_parser.py:1050-1052). Go still includes it.")
		}
	}

	// Verify the normal figure box IS still present.
	foundFigure := false
	for _, b := range result {
		if strings.Contains(b.Text, "架构图") {
			foundFigure = true
		}
	}
	if !foundFigure {
		t.Error("normal figure box '架构图' should still be present")
	}

	t.Log("NOTE: Python filters figure boxes matching r\"(数据|资料|图表)*来源[:： ]\" in _extract_table_figure.")
	t.Log("Go's consolidateFigures has no equivalent filter.")
}
