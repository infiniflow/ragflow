package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

func TestMergeTablesAcrossPagesRejectsDifferentHeaders(t *testing.T) {
	anchor := headerTable([]string{"序号", "学校名称", "专业代码"}, []string{"1", "南京大学", "080710T"})
	candidate := headerTable([]string{"序号", "学校名称", "学位授予门类"}, []string{"2", "东南大学", "工学"})
	candidate.Positions[0].PageNumbers = []int{1}

	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, candidate}, nil, map[int]float64{0: 70})
	if len(merged) != 2 {
		t.Fatalf("different headers must not merge, got %d tables", len(merged))
	}
}

func TestMergeTablesAcrossPagesMergesSameHeaderAndDropsRepeatedHeader(t *testing.T) {
	anchor := headerTable([]string{"序号", "学校名称", "专业代码"}, []string{"1", "南京大学", "080710T"})
	candidate := headerTable([]string{"序号", "学校名称", "专业代码"}, []string{"2", "东南大学", "080902"})
	candidate.Positions[0].PageNumbers = []int{1}

	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, candidate}, nil, map[int]float64{0: 70})
	if len(merged) != 1 {
		t.Fatalf("same header must merge, got %d tables", len(merged))
	}
	if len(merged[0].Grid) != 3 {
		t.Fatalf("merged grid rows = %d, want 3 (one header + two data rows)", len(merged[0].Grid))
	}
	if got := merged[0].Grid[0][0].Text; got != "序号" {
		t.Fatalf("first cell = %q, want header 序号", got)
	}
	if got := merged[0].Grid[1][0].Text; got != "1" {
		t.Fatalf("first data cell = %q, want 1", got)
	}
	if got := merged[0].Grid[2][0].Text; got != "2" {
		t.Fatalf("continuation data cell = %q, want 2", got)
	}
	headerCells := 0
	for _, cell := range merged[0].Cells {
		if cell.Text == "序号" {
			headerCells++
		}
	}
	if headerCells != 1 {
		t.Fatalf("repeated header cells = %d, want 1", headerCells)
	}
}

func TestMergeTablesAcrossPagesHeaderlessFallsBackToGeometry(t *testing.T) {
	anchor := headerTable([]string{"A", "B"}, []string{"1", "2"})
	candidate := headerTable([]string{"X", "Y"}, []string{"3", "4"})
	anchor.HeaderRows = 0
	candidate.HeaderRows = 0
	candidate.Positions[0].PageNumbers = []int{1}

	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, candidate}, nil, map[int]float64{0: 70})
	if len(merged) != 1 {
		t.Fatalf("headerless tables must keep the geometry-only merge, got %d tables", len(merged))
	}
}

func TestMergeTablesAcrossPagesRejectsEqualHeaderTextWithDifferentColumnLayout(t *testing.T) {
	anchor := headerTable([]string{"A", "B"}, []string{"1", "2"})
	candidate := headerTable([]string{"A", "", "B"}, []string{"3", "", "4"})
	candidate.Positions[0].PageNumbers = []int{1}

	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, candidate}, nil, map[int]float64{0: 70})
	if len(merged) != 2 {
		t.Fatalf("equal header text with different columns must not merge, got %d tables", len(merged))
	}
}

func TestMergeTablesAcrossPagesRejectsDifferentEmptyHeaderCellLayout(t *testing.T) {
	anchor := headerTable([]string{"A", "B", ""}, []string{"1", "2", ""})
	candidate := headerTable([]string{"A", "", "B"}, []string{"3", "", "4"})
	candidate.Positions[0].PageNumbers = []int{1}

	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, candidate}, nil, map[int]float64{0: 70})
	if len(merged) != 2 {
		t.Fatalf("different empty-cell layout must not merge, got %d tables", len(merged))
	}
}

func TestMergeTablesAcrossPagesSkipsHeaderOnlyContinuationGrid(t *testing.T) {
	anchor := headerTable([]string{"A", "B"}, []string{"1", "2"})
	dataPage := headerTable([]string{"A", "B"}, []string{"3", "4"})
	headerOnlyPage := headerTable([]string{"A", "B"})
	dataPage.Positions[0].PageNumbers = []int{1}
	headerOnlyPage.Positions[0].PageNumbers = []int{2}

	merged := MergeTablesAcrossPages(
		[]pdf.TableItem{anchor, dataPage, headerOnlyPage},
		nil,
		map[int]float64{0: 70, 1: 70},
	)
	if len(merged) != 1 {
		t.Fatalf("tables = %d, want 1", len(merged))
	}
	if got, want := len(merged[0].Grid), 3; got != want {
		t.Fatalf("merged grid rows = %d, want %d (header + two data rows)", got, want)
	}
	if got := merged[0].Grid[2][0].Text; got != "3" {
		t.Fatalf("continuation data cell = %q, want 3", got)
	}
}

func TestMergeTablesAcrossPagesRejectsHeaderColumnMismatch(t *testing.T) {
	anchor := headerTable([]string{"", ""}, []string{"1", "2"})
	candidate := headerTable([]string{"", ""}, []string{"3", "4"})
	candidate.Grid[0] = append(candidate.Grid[0], pdf.TSRCell{Text: ""})
	candidate.HeaderRows = 1
	candidate.Positions[0].PageNumbers = []int{1}

	merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, candidate}, nil, map[int]float64{0: 70})
	if len(merged) != 2 {
		t.Fatalf("header column mismatch must not merge, got %d tables", len(merged))
	}
}

func headerTable(header []string, rows ...[]string) pdf.TableItem {
	grid := make([][]pdf.TSRCell, 0, 1+len(rows))
	add := func(values []string) {
		row := make([]pdf.TSRCell, len(values))
		for i, value := range values {
			row[i] = pdf.TSRCell{
				X0: float64(i) * 100, Y0: float64(len(grid)) * 30,
				X1: float64(i)*100 + 100, Y1: float64(len(grid))*30 + 30,
				Text: value,
			}
		}
		grid = append(grid, row)
	}
	add(header)
	for _, row := range rows {
		add(row)
	}
	cells := make([]pdf.TSRCell, 0, len(grid)*len(header))
	for _, row := range grid {
		cells = append(cells, row...)
	}
	return pdf.TableItem{
		Positions:  []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 300, Top: 0, Bottom: 60}},
		Scale:      1.0,
		Grid:       grid,
		Cells:      cells,
		HeaderRows: 1,
	}
}
