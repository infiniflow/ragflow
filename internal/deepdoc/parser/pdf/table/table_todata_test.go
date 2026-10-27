package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// cellsFromStrings builds a [][]pdf.TSRCell grid from string rows for tests.
func cellsFromStrings(rows [][]string) [][]pdf.TSRCell {
	out := make([][]pdf.TSRCell, len(rows))
	for ri, row := range rows {
		out[ri] = make([]pdf.TSRCell, len(row))
		for ci, s := range row {
			out[ri][ci] = pdf.TSRCell{Text: s}
		}
	}
	return out
}

// TestToTableData_Simple2x2 verifies a plain 2x2 grid with no span/caption
// produces the canonical Grid and the default single header row.
func TestToTableData_Simple2x2(t *testing.T) {
	g := cellsFromStrings([][]string{{"姓名", "年龄"}, {"张三", "25"}})
	td := ToTableData(g, "", nil, nil, nil)
	if td == nil {
		t.Fatal("expected non-nil TableData")
	}
	if len(td.Grid) != 2 {
		t.Fatalf("rows=%d want 2", len(td.Grid))
	}
	if len(td.Grid[0]) != 2 {
		t.Fatalf("cols=%d want 2", len(td.Grid[0]))
	}
	if td.Grid[0][0].Text != "姓名" || td.Grid[1][1].Text != "25" {
		t.Errorf("cell text mismatch: %q/%q", td.Grid[0][0].Text, td.Grid[1][1].Text)
	}
	if td.HeaderRows != 1 {
		t.Errorf("HeaderRows=%d want 1 (default)", td.HeaderRows)
	}
	for _, row := range td.Grid {
		for _, c := range row {
			if c.ColSpan != 1 || c.RowSpan != 1 || c.Covered {
				t.Errorf("unexpected span/covered: %+v", c)
			}
		}
	}
}

// TestToTableData_Empty verifies an empty / nil grid yields a nil Grid rather
// than a panic or a ragged zero-width slice.
func TestToTableData_Empty(t *testing.T) {
	td := ToTableData(nil, "", nil, nil, nil)
	if td == nil {
		t.Fatal("expected non-nil TableData wrapper")
	}
	if td.Grid != nil {
		t.Errorf("expected nil Grid, got %v", td.Grid)
	}
}

// TestToTableData_Colspan verifies a colspan origin keeps its ColSpan and the
// engulfed position becomes a Covered marker pointing back at the origin.
func TestToTableData_Colspan(t *testing.T) {
	rows := cellsFromStrings([][]string{{"Name", ""}, {"John", "30"}})
	spanInfo := map[[2]int][2]int{{0, 0}: {2, 1}}
	covered := map[[2]int]bool{{0, 1}: true}
	td := ToTableData(rows, "", nil, spanInfo, covered)
	if len(td.Grid) != 2 {
		t.Fatalf("rows=%d want 2", len(td.Grid))
	}
	if td.Grid[0][0].ColSpan != 2 {
		t.Errorf("origin ColSpan=%d want 2", td.Grid[0][0].ColSpan)
	}
	if !td.Grid[0][1].Covered {
		t.Errorf("expected Covered at [0][1]")
	}
	if td.Grid[0][1].OriginRow != 0 || td.Grid[0][1].OriginCol != 0 {
		t.Errorf("origin back-pointer wrong: %+v", td.Grid[0][1])
	}
}

// TestToTableData_Rowspan verifies a rowspan origin keeps its RowSpan and the
// continuation row stays (not trimmed) as a Covered marker.
func TestToTableData_Rowspan(t *testing.T) {
	rows := cellsFromStrings([][]string{{"H", "x"}, {"", "y"}})
	spanInfo := map[[2]int][2]int{{0, 0}: {1, 2}}
	covered := map[[2]int]bool{{1, 0}: true}
	td := ToTableData(rows, "", nil, spanInfo, covered)
	if td.Grid[0][0].RowSpan != 2 {
		t.Errorf("RowSpan=%d want 2", td.Grid[0][0].RowSpan)
	}
	if !td.Grid[1][0].Covered {
		t.Errorf("expected Covered continuation at [1][0]")
	}
	if td.Grid[1][0].OriginRow != 0 || td.Grid[1][0].OriginCol != 0 {
		t.Errorf("origin back-pointer wrong: %+v", td.Grid[1][0])
	}
}

// TestToTableData_RaggedNoPad verifies a short row keeps its actual (ragged)
// length and is NOT padded to the widest row — matching the legacy Rows text
// view produced by ParseTableHTML.
func TestToTableData_RaggedNoPad(t *testing.T) {
	rows := cellsFromStrings([][]string{{"a", "b", "c"}, {"d"}})
	td := ToTableData(rows, "", nil, nil, nil)
	if len(td.Grid[0]) != 3 {
		t.Errorf("row0 cols=%d want 3", len(td.Grid[0]))
	}
	if len(td.Grid[1]) != 1 {
		t.Errorf("row1 cols=%d want 1 (no padding)", len(td.Grid[1]))
	}
}

// TestToTableData_CaptionEscape verifies the caption is stored verbatim. The
// legacy round trip escaped then un-escaped it; ToTableData stores it directly
// and must reach the same decoded value.
func TestToTableData_CaptionEscape(t *testing.T) {
	rows := cellsFromStrings([][]string{{"x"}})
	td := ToTableData(rows, "a<b&c", nil, nil, nil)
	if td.Caption != "a<b&c" {
		t.Errorf("Caption=%q want %q", td.Caption, "a<b&c")
	}
}

// TestToTableData_HeaderLeadingOnly locks the equivalence with the legacy
// ParseTableHTML behaviour: headerRows may flag non-leading rows as headers,
// but only the leading consecutive run counts (this matches ParseTableHTML's
// leading-consecutive <th> scan, NOT a per-row flag).
func TestToTableData_HeaderLeadingOnly(t *testing.T) {
	rows := cellsFromStrings([][]string{{"h0"}, {"d0"}, {"h2"}})
	headerRows := map[int]bool{0: true, 2: true}
	td := ToTableData(rows, "", headerRows, nil, nil)
	if td.HeaderRows != 1 {
		t.Errorf("HeaderRows=%d want 1 (leading only)", td.HeaderRows)
	}
}

// TestToTableData_TrailingEmptyTrimmed verifies a trailing row that carries NO
// cells at all (zero cells, len==0) is trimmed. A row holding an empty-text
// cell is NOT trimmed — it is a real row, matching buildGrid's legacy behaviour.
func TestToTableData_TrailingEmptyTrimmed(t *testing.T) {
	rows := cellsFromStrings([][]string{{"a"}, {}})
	td := ToTableData(rows, "", nil, nil, nil)
	if len(td.Grid) != 1 {
		t.Errorf("rows=%d want 1 (trailing zero-cell row trimmed)", len(td.Grid))
	}
}

// TestToTableData_EmptyTextCellKept verifies a trailing row that holds an
// empty-text cell is a real row and is NOT trimmed (adversarial guard against
// over-eager trim).
func TestToTableData_EmptyTextCellKept(t *testing.T) {
	rows := cellsFromStrings([][]string{{"a"}, {""}})
	td := ToTableData(rows, "", nil, nil, nil)
	if len(td.Grid) != 2 {
		t.Errorf("rows=%d want 2 (empty-text cell row kept)", len(td.Grid))
	}
}
