package parser

import (
	"reflect"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestXLSXParserEmitsSpreadsheetRows(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "ID")
		mustSetCell(t, f, "Sheet1", "B1", "Status")
		mustSetCell(t, f, "Sheet1", "A2", "A-100")
		mustSetCell(t, f, "Sheet1", "B2", "paid")
		mustSetCell(t, f, "Sheet1", "A3", "A-101")
		mustSetCell(t, f, "Sheet1", "B3", "pending")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "orders.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 3 {
		t.Fatalf("items = %d, want header plus two rows", len(res.JSON))
	}
	header := res.JSON[0]
	if header["ck_type"] != "table_header" {
		t.Fatalf("header ck_type = %v, want table_header", header["ck_type"])
	}
	if got, want := header["cells"], []string{"ID", "Status"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("header cells = %#v, want %#v", got, want)
	}
	row := res.JSON[1]
	if row["ck_type"] != "table_row" || row["doc_type_kwd"] != "text" {
		t.Fatalf("row types = ck_type:%v doc_type:%v", row["ck_type"], row["doc_type_kwd"])
	}
	if got, want := row["cells"], []string{"A-100", "paid"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("row cells = %#v, want %#v", got, want)
	}
	if row["sheet"] != "Sheet1" || row["sheet_index"] != 1 {
		t.Fatalf("row sheet metadata = %#v", row)
	}
	if row["row_start"] != 2 || row["row_end"] != 2 || row["col_start"] != 1 || row["col_end"] != 2 {
		t.Fatalf("row coordinates = %#v", row)
	}
}

func TestXLSXParserHTML4ExcelRemainsAtomic(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Question")
		mustSetCell(t, f, "Sheet1", "B1", "Answer")
		mustSetCell(t, f, "Sheet1", "A2", "Q1")
		mustSetCell(t, f, "Sheet1", "B2", "A1")
	})
	p, _ := NewXLSXParser("")
	p.ConfigureFromSetup(map[string]any{"html4excel": true})
	res := p.ParseWithResult(t.Context(), "qa.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("items = %d, want one atomic table item", len(res.JSON))
	}
	if res.JSON[0]["ck_type"] != "table" || !strings.Contains(res.JSON[0]["text"].(string), "<table>") {
		t.Fatalf("html4excel item = %#v", res.JSON[0])
	}
}

func TestXLSXParserDoesNotPrechunkRows(t *testing.T) {
	const dataRows = 257
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Value")
		for row := 2; row <= dataRows+1; row++ {
			mustSetCell(t, f, "Sheet1", cellAxis(row, 1), row)
		}
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "rows.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != dataRows+1 {
		t.Fatalf("items = %d, want one header plus %d data rows", len(res.JSON), dataRows)
	}
	last := res.JSON[len(res.JSON)-1]
	if last["ck_type"] != "table_row" || last["row_start"] != dataRows+1 {
		t.Fatalf("last row = %#v, want source row %d", last, dataRows+1)
	}
}

func TestXLSXParserPreservesSheetOrderAndIdentity(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "First")
		mustSetCell(t, f, "Sheet1", "A2", "one")
		_, err := f.NewSheet("Orders")
		if err != nil {
			t.Fatalf("new sheet: %v", err)
		}
		mustSetCell(t, f, "Orders", "A1", "Second")
		mustSetCell(t, f, "Orders", "A2", "two")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "multi.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 4 {
		t.Fatalf("items = %d, want two headers and two rows", len(res.JSON))
	}
	seen := map[string]map[string]any{}
	for _, item := range res.JSON {
		if item["ck_type"] == "table_header" {
			seen[item["sheet"].(string)] = item
		}
	}
	if first := seen["Sheet1"]; first == nil || first["table_id"] != "sheet-1" || first["sheet_index"] != 1 {
		t.Fatalf("Sheet1 identity = %#v", first)
	}
	if second := seen["Orders"]; second == nil || second["table_id"] != "sheet-2" || second["sheet_index"] != 2 {
		t.Fatalf("Orders identity = %#v", second)
	}
}

func spreadsheetText(res ParseResult) string {
	var out strings.Builder
	for _, item := range res.JSON {
		text, _ := item["text"].(string)
		out.WriteString(text)
	}
	return out.String()
}

func spreadsheetHeaderItem(res ParseResult) map[string]any {
	for _, item := range res.JSON {
		if item["ck_type"] == "table_header" {
			return item
		}
	}
	return nil
}

func TestXLSXImageMIMEType(t *testing.T) {
	tests := []struct {
		extension string
		mime      string
		ok        bool
	}{
		{extension: ".png", mime: "image/png", ok: true},
		{extension: ".JPG", mime: "image/jpeg", ok: true},
		{extension: ".emf", mime: "image/x-emf", ok: true},
		{extension: ".emz", mime: "image/x-emz", ok: true},
		{extension: ".ico", mime: "image/x-icon", ok: true},
		{extension: ".wmf", mime: "image/x-wmf", ok: true},
		{extension: ".wmz", mime: "image/x-wmz", ok: true},
		{extension: ".unknown", ok: false},
	}
	for _, tc := range tests {
		mime, ok := xlsxImageMIMEType(tc.extension)
		if mime != tc.mime || ok != tc.ok {
			t.Errorf("xlsxImageMIMEType(%q) = (%q, %v), want (%q, %v)", tc.extension, mime, ok, tc.mime, tc.ok)
		}
	}
}

func TestExtractXLSXImagesWarningForInvalidSheet(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	_, warnings := extractXLSXImages(f, "MissingSheet")
	if len(warnings) != 1 || !strings.Contains(warnings[0], "image discovery failed") {
		t.Fatalf("warnings = %v, want image discovery warning", warnings)
	}
}

// newTestXLSX builds an in-memory .xlsx from a cell writer.
func newTestXLSX(t *testing.T, fill func(f *excelize.File)) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	fill(f)
	buf, err := f.WriteToBuffer()
	if err != nil {
		t.Fatalf("WriteToBuffer: %v", err)
	}
	return buf.Bytes()
}

// The following helpers fail the test immediately when an Excelize fixture
// operation errors, so incomplete workbook data is never handed to the parser.
func mustSetCell(t *testing.T, f *excelize.File, sheet, axis string, val any) {
	t.Helper()
	if err := f.SetCellValue(sheet, axis, val); err != nil {
		t.Fatalf("SetCellValue(%s!%s): %v", sheet, axis, err)
	}
}

func mustMergeCell(t *testing.T, f *excelize.File, sheet, topLeft, bottomRight string) {
	t.Helper()
	if err := f.MergeCell(sheet, topLeft, bottomRight); err != nil {
		t.Fatalf("MergeCell(%s:%s-%s): %v", sheet, topLeft, bottomRight, err)
	}
}

func mustNewStyle(t *testing.T, f *excelize.File, style *excelize.Style) int {
	t.Helper()
	idx, err := f.NewStyle(style)
	if err != nil {
		t.Fatalf("NewStyle: %v", err)
	}
	return idx
}

func mustSetCellStyle(t *testing.T, f *excelize.File, sheet, topLeft, bottomRight string, idx int) {
	t.Helper()
	if err := f.SetCellStyle(sheet, topLeft, bottomRight, idx); err != nil {
		t.Fatalf("SetCellStyle(%s:%s-%s): %v", sheet, topLeft, bottomRight, err)
	}
}

func mustAddTable(t *testing.T, f *excelize.File, sheet string, table *excelize.Table) {
	t.Helper()
	if err := f.AddTable(sheet, table); err != nil {
		t.Fatalf("AddTable(%s): %v", sheet, err)
	}
}

// TestXLSXParser_HeaderAndCaption asserts the XLSX parser emits a typed header
// row and data row with the header labels preserved separately from values.
func TestXLSXParser_HeaderAndCaption(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Product")
		mustSetCell(t, f, "Sheet1", "B1", "Price")
		mustSetCell(t, f, "Sheet1", "A2", "Widget")
		mustSetCell(t, f, "Sheet1", "B2", "9.99")
		mustSetCell(t, f, "Sheet1", "A3", "Gadget")
		mustSetCell(t, f, "Sheet1", "B3", "19.99")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	text := spreadsheetText(res)
	if !strings.Contains(text, "Product; Price") {
		t.Fatalf("want header text, got:\n%s", text)
	}
	if !strings.Contains(text, "Product：Widget; Price：9.99") {
		t.Fatalf("want row text, got:\n%s", text)
	}
	// The header label must not be duplicated as a value in the row text.
	if strings.Contains(text, "Product：Product") {
		t.Fatalf("header value leaked into row text:\n%s", text)
	}
}

// TestXLSXParser_MergedHeaderInheritance asserts a horizontally merged header
// cell's slave columns inherit the master text, so a wide merged title does not
// render as a row of blank header cells.
func TestXLSXParser_MergedHeaderInheritance(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		// Row 1 is the header; A1:C1 merged into one wide label "Sales Report".
		mustSetCell(t, f, "Sheet1", "A1", "Sales Report")
		mustMergeCell(t, f, "Sheet1", "A1", "C1")
		// Make the header row bold so detection anchors row 1.
		idx := mustNewStyle(t, f, &excelize.Style{Font: &excelize.Font{Bold: true}})
		mustSetCellStyle(t, f, "Sheet1", "A1", "C1", idx)
		mustSetCell(t, f, "Sheet1", "A2", "North")
		mustSetCell(t, f, "Sheet1", "B2", "10")
		mustSetCell(t, f, "Sheet1", "C2", "20")
		mustSetCell(t, f, "Sheet1", "A3", "South")
		mustSetCell(t, f, "Sheet1", "B3", "30")
		mustSetCell(t, f, "Sheet1", "C3", "40")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	text := spreadsheetText(res)
	if !strings.Contains(text, "Sales Report; Sales Report; Sales Report") {
		t.Fatalf("merged master text not inherited into header cells, got:\n%s", text)
	}
}

// TestDetectHeaderRow_ListObject asserts a ListObject whose top row is > 1 is
// detected as the header row.
func TestDetectHeaderRow_ListObject(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Title")
		mustSetCell(t, f, "Sheet1", "A2", "This is a banner")
		mustSetCell(t, f, "Sheet1", "A3", "Name")
		mustSetCell(t, f, "Sheet1", "B3", "Age")
		mustSetCell(t, f, "Sheet1", "A4", "Alice")
		mustSetCell(t, f, "Sheet1", "B4", "30")
		mustAddTable(t, f, "Sheet1", &excelize.Table{Range: "A3:B4", Name: "Table1"})
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(spreadsheetText(res), "Name; Age") {
		t.Fatalf("want ListObject header row detected, got:\n%s", spreadsheetText(res))
	}
	header := spreadsheetHeaderItem(res)
	if header == nil {
		t.Fatal("missing table_header item")
	}
	if got := header["text"]; got != "Name; Age" {
		t.Fatalf("header text = %v, want Name; Age", got)
	}
}

// TestDetectHeaderRow_Lightweight asserts that when row 1 is numeric data and
// row 2 is a styled text label row, the lightweight detector picks row 2.
func TestDetectHeaderRow_Lightweight(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "100")
		mustSetCell(t, f, "Sheet1", "B1", "200")
		mustSetCell(t, f, "Sheet1", "A2", "Item")
		mustSetCell(t, f, "Sheet1", "B2", "Count")
		// Make the header row bold so the styled signal fires.
		idx := mustNewStyle(t, f, &excelize.Style{Font: &excelize.Font{Bold: true}})
		mustSetCellStyle(t, f, "Sheet1", "A2", "B2", idx)
		mustSetCell(t, f, "Sheet1", "A3", "Apple")
		mustSetCell(t, f, "Sheet1", "B3", "5")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(spreadsheetText(res), "Item; Count") {
		t.Fatalf("want row-2 header detected, got:\n%s", spreadsheetText(res))
	}
	if strings.Contains(spreadsheetText(res), "100; 200") {
		t.Fatalf("numeric row 1 must not be the header:\n%s", spreadsheetText(res))
	}
}

// TestXLSXParser_CommonCaseNoRegression asserts the dominant case (header on
// row 1, no merges, no ListObject) keeps the header row unchanged.
func TestXLSXParser_CommonCaseNoRegression(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "col_a")
		mustSetCell(t, f, "Sheet1", "B1", "col_b")
		mustSetCell(t, f, "Sheet1", "A2", "1")
		mustSetCell(t, f, "Sheet1", "B2", "2")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(spreadsheetText(res), "col_a; col_b") {
		t.Fatalf("common-case header must be preserved:\n%s", spreadsheetText(res))
	}
}

// TestDetectHeaderRow_BoldSubtotalNotHeader asserts that a bold "Total"-style
// subtotal row sitting directly under a numeric row 1 is NOT promoted to the
// header. The subtotal-label check blocks the override so the numeric row 1
// stays the header.
func TestDetectHeaderRow_BoldSubtotalNotHeader(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		// Row 1: numeric year header (Python default header row).
		mustSetCell(t, f, "Sheet1", "A1", "2023")
		mustSetCell(t, f, "Sheet1", "B1", "2024")
		// Row 2: bold "Total/Summary" subtotal — must NOT become the header.
		idx := mustNewStyle(t, f, &excelize.Style{Font: &excelize.Font{Bold: true}})
		mustSetCell(t, f, "Sheet1", "A2", "Total")
		mustSetCell(t, f, "Sheet1", "B2", "Summary")
		mustSetCellStyle(t, f, "Sheet1", "A2", "B2", idx)
		// Row 3: purely numeric continuation under the subtotal.
		mustSetCell(t, f, "Sheet1", "A3", "100")
		mustSetCell(t, f, "Sheet1", "B3", "200")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(spreadsheetText(res), "2023; 2024") {
		t.Fatalf("numeric row 1 must remain the header:\n%s", spreadsheetText(res))
	}
	if strings.Contains(spreadsheetText(res), "Total; Summary") {
		t.Fatalf("bold subtotal row must not be promoted to header:\n%s", spreadsheetText(res))
	}
}

// TestDetectHeaderRow_StyledHeaderPastFarMerge asserts that a bold header on a
// narrow row (row 3) is still detected even when an earlier wide merge
// (A1:Z1 title) would otherwise pad every row to 26 columns. The padding step
// must run AFTER header detection so the styled-majority signal is not diluted.
func TestDetectHeaderRow_StyledHeaderPastFarMerge(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		// Row 1: a wide merged title cell A1:Z1.
		mustSetCell(t, f, "Sheet1", "A1", "Sales Report")
		mustMergeCell(t, f, "Sheet1", "A1", "Z1")
		titleIdx := mustNewStyle(t, f, &excelize.Style{Font: &excelize.Font{Bold: true}})
		mustSetCellStyle(t, f, "Sheet1", "A1", "A1", titleIdx)
		// Row 3: the real, narrow, bold header.
		hdrIdx := mustNewStyle(t, f, &excelize.Style{Font: &excelize.Font{Bold: true}})
		mustSetCell(t, f, "Sheet1", "A3", "Name")
		mustSetCell(t, f, "Sheet1", "B3", "Desc")
		mustSetCellStyle(t, f, "Sheet1", "A3", "B3", hdrIdx)
		// Row 4: data (text in col A, so the body-brake passes for row 3).
		mustSetCell(t, f, "Sheet1", "A4", "Alice")
		mustSetCell(t, f, "Sheet1", "B4", "x")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(spreadsheetText(res), "Name; Desc") {
		t.Fatalf("narrow bold header past far merge must be detected:\n%s", spreadsheetText(res))
	}
	header := spreadsheetHeaderItem(res)
	if header == nil {
		t.Fatal("missing table_header item")
	}
	if got := header["text"]; got != "Name; Desc" {
		t.Fatalf("header text = %v, want Name; Desc", got)
	}
}

// TestDetectHeaderRow_StyledTextHeaderOverNumeric asserts that a styled text
// header sitting directly above a purely-numeric data row is still detected as
// the header (not refused by the body-brake), while a bold "Total"/"Summary"
// subtotal over numeric data is not.
func TestDetectHeaderRow_StyledTextHeaderOverNumeric(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		// Row 1: a single-cell title stub (fewer than 2 columns → skipped).
		mustSetCell(t, f, "Sheet1", "A1", "Report Title")
		// Row 2: the real, bold text header over numeric-only data below.
		idx := mustNewStyle(t, f, &excelize.Style{Font: &excelize.Font{Bold: true}})
		mustSetCell(t, f, "Sheet1", "A2", "Product")
		mustSetCell(t, f, "Sheet1", "B2", "Units")
		mustSetCellStyle(t, f, "Sheet1", "A2", "B2", idx)
		// Row 3: purely numeric continuation.
		mustSetCell(t, f, "Sheet1", "A3", "5")
		mustSetCell(t, f, "Sheet1", "B3", "10")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(spreadsheetText(res), "Product; Units") {
		t.Fatalf("styled text header over numeric data must be detected:\n%s", spreadsheetText(res))
	}
	header := spreadsheetHeaderItem(res)
	if header == nil {
		t.Fatal("missing table_header item")
	}
	if got := header["text"]; got != "Product; Units" {
		t.Fatalf("header text = %v, want Product; Units", got)
	}
}

// TestInheritMergedHeader unit-tests the merge inheritance helper directly.
func TestInheritMergedHeader(t *testing.T) {
	records := [][]string{
		{"MASTER", "", "Q2"}, // header row: B1 blank, merged from master A1
		{"a", "b", "c"},
	}
	mm := map[[2]int][2]int{
		{1, 1}: {1, 1},
		{1, 2}: {1, 1}, // B1's master is A1
		{1, 3}: {1, 3},
	}
	inheritMergedHeader(records, 1, mm)
	if records[0][1] != "MASTER" {
		t.Fatalf("expected B1 to inherit A1 master text, got %q", records[0][1])
	}
	if records[0][2] != "Q2" {
		t.Fatalf("expected non-merged C1 to keep its value, got %q", records[0][2])
	}
}

// TestPadRowToWidth asserts the single-row padder widens with empty strings,
// preserves existing cells, and never shrinks an already-wide row.
func TestPadRowToWidth(t *testing.T) {
	row := []string{"a", "b"}
	padRowToWidth(&row, 5)
	if len(row) != 5 {
		t.Fatalf("want len 5, got %d", len(row))
	}
	if row[0] != "a" || row[1] != "b" {
		t.Fatalf("original cells lost: %v", row)
	}
	for i := 2; i < 5; i++ {
		if row[i] != "" {
			t.Fatalf("pad cell %d want empty, got %q", i, row[i])
		}
	}
	// Never shrinks an already-wide row.
	padRowToWidth(&row, 2)
	if len(row) != 5 {
		t.Fatalf("must not shrink: want 5, got %d", len(row))
	}
}

// TestMergeExtentCol asserts the furthest merged column is reported as-is within
// the cap and clamped to maxMergeExtentCols beyond it (the memory guard).
func TestMergeExtentCol(t *testing.T) {
	if got := mergeExtentCol(nil); got != 0 {
		t.Fatalf("nil ranges: want 0, got %d", got)
	}
	if got := mergeExtentCol([]mergeRange{{1, 1, 3, 10}}); got != 10 {
		t.Fatalf("within cap: want 10, got %d", got)
	}
	if got := mergeExtentCol([]mergeRange{{1, 1, 1, 5000}}); got != maxMergeExtentCols {
		t.Fatalf("beyond cap: want %d, got %d", maxMergeExtentCols, got)
	}
}

// TestXLSXParser_EmptySheet asserts an empty sheet yields no parser items.
func TestXLSXParser_EmptySheet(t *testing.T) {
	// excelize.NewFile yields a single empty "Sheet1".
	data := newTestXLSX(t, func(f *excelize.File) {})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 0 {
		t.Fatalf("empty sheet must yield empty JSON, got:\n%v", res.JSON)
	}
}
