package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// TestConstructTable_HeaderRowDetected pins that a TSR row whose cells carry
// the "table column header" block type is counted as the header row
// (HeaderRows) in the produced TableData.
func TestConstructTable_HeaderRowDetected(t *testing.T) {
	cells := []pdf.TSRCell{
		{X0: 0, Y0: 0, X1: 100, Y1: 30, Text: "Name", Label: "table column header"},
		{X0: 101, Y0: 0, X1: 200, Y1: 30, Text: "Age", Label: "table column header"},
		{X0: 0, Y0: 35, X1: 100, Y1: 65, Text: "John", Label: "table row"},
		{X0: 101, Y0: 35, X1: 200, Y1: 65, Text: "30", Label: "table row"},
	}
	td := constructTableData(t, cells, nil, "", &pdf.TableItem{})
	if td == nil {
		t.Fatal("expected non-nil TableData")
	}
	if td.HeaderRows != 1 {
		t.Errorf("expected 1 header row, got %d", td.HeaderRows)
	}
	if cellTextAt(td, 0, 0) != "Name" || cellTextAt(td, 1, 0) != "John" {
		t.Errorf("unexpected cell text: %v", td.Grid)
	}
}

// TestToTableData_ColspanFromCalSpans verifies that a span derived from CalSpans
// (an "SP"-annotated box covering two columns) produces a colspan origin in the
// produced TableData — the same geometry the removed RowsToHTML emitted.
func TestToTableData_ColspanFromCalSpans(t *testing.T) {
	boxes := []pdf.TextBox{
		{X0: 10, X1: 90, Top: 0, Bottom: 30, Text: "Name", R: 0, C: 0},
		{X0: 10, X1: 190, Top: 0, Bottom: 30, Text: "", R: 0, C: 1, SP: 1},
		{X0: 10, X1: 90, Top: 35, Bottom: 65, Text: "John", R: 1, C: 0},
		{X0: 110, X1: 190, Top: 35, Bottom: 65, Text: "30", R: 1, C: 1},
	}
	rows := GroupBoxesByRC(boxes)
	spanInfo, covered := CalSpans(rows)
	td := ToTableData(rows, "", nil, spanInfo, covered)
	if td == nil {
		t.Fatal("expected non-nil TableData")
	}
	if td.Grid[0][0].ColSpan != 2 {
		t.Errorf("expected colspan=2 origin, got %d", td.Grid[0][0].ColSpan)
	}
	if !td.Grid[0][1].Covered {
		t.Errorf("expected covered cell at [0][1]")
	}
}
