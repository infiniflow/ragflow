package table

import (
	"ragflow/internal/deepdoc/parser/pdf/layout"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"reflect"
	"testing"
)

func TestMergedTablePreviewKeepsEachPageRegion(t *testing.T) {
	for _, withBoxes := range []bool{false, true} {
		t.Run(map[bool]string{false: "no text boxes", true: "text boxes"}[withBoxes], func(t *testing.T) {
			anchor := previewTable([]string{"ID", "Name"}, []string{"1", "Alice"})
			next := previewTable([]string{"ID", "Name"}, []string{"2", "Bob"})
			anchor.RegionLeft, anchor.RegionRight, anchor.RegionTop, anchor.RegionBottom = 10, 250, 40, 90
			next.RegionLeft, next.RegionRight, next.RegionTop, next.RegionBottom = 20, 260, 5, 30
			anchor.Positions = []pdf.Position{{PageNumbers: []int{0}, Left: 10, Right: 250, Top: 40, Bottom: 90}}
			next.Positions = []pdf.Position{{PageNumbers: []int{1}, Left: 20, Right: 260, Top: 5, Bottom: 30}}
			merged := MergeTablesAcrossPages([]pdf.TableItem{anchor, next}, nil, map[int]float64{0: 100})
			if len(merged) != 1 {
				t.Fatalf("want a merged table, got %d", len(merged))
			}
			var boxes []pdf.TextBox
			if withBoxes {
				boxes = []pdf.TextBox{
					{X0: 10, X1: 250, Top: 40, Bottom: 90, PageNumber: 0, HasPageNumber: true, LayoutType: "table", Text: "Alice"},
					{X0: 20, X1: 260, Top: 5, Bottom: 30, PageNumber: 1, HasPageNumber: true, LayoutType: "table", Text: "Bob"},
				}
			}
			sections := layout.BoxesToSections(ExtractTableAndReplace(boxes, merged), map[int]float64{0: 100, 1: 100})
			want := []pdf.Position{
				{PageNumbers: []int{0}, Left: 10, Right: 250, Top: 40, Bottom: 90},
				{PageNumbers: []int{1}, Left: 20, Right: 260, Top: 5, Bottom: 30},
			}
			if len(sections) != 1 || !reflect.DeepEqual(sections[0].Positions, want) {
				t.Fatalf("want each page's own crop region, got %#v", sections)
			}
		})
	}
}

func previewTable(header []string, rows ...[]string) pdf.TableItem {
	var grid [][]pdf.TSRCell
	for _, values := range append([][]string{header}, rows...) {
		row := make([]pdf.TSRCell, len(values))
		for i, v := range values {
			row[i] = pdf.TSRCell{X0: float64(i) * 100, X1: float64(i+1) * 100, Y0: float64(len(grid)) * 30, Y1: float64(len(grid)+1) * 30, Text: v}
		}
		grid = append(grid, row)
	}
	var cells []pdf.TSRCell
	for _, row := range grid {
		cells = append(cells, row...)
	}
	return pdf.TableItem{Positions: []pdf.Position{{PageNumbers: []int{0}, Left: 0, Right: 300, Top: 0, Bottom: 60}}, Scale: 1, Grid: grid, Cells: cells}
}
