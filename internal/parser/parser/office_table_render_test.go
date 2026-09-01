package parser

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func xlsxTableHTML(res ParseResult) string {
	var out strings.Builder
	for _, item := range res.JSON {
		if item["doc_type_kwd"] != "table" {
			continue
		}
		text, _ := item["text"].(string)
		out.WriteString(text)
	}
	return out.String()
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

// TestRecordsToHTMLTableChunks_Alignment asserts the chunked output uses the
// shared schema: <caption>, first row as <th>, data as <td>, repeated header
// per 256-row chunk, and NO <thead>/<tbody> wrapper.
func TestRecordsToHTMLTableChunks_Alignment(t *testing.T) {
	records := [][]string{{"Name", "Age"}, {"Alice", "30"}, {"Bob", "25"}}
	out := recordsToHTMLTableChunks(records, 256, "Sheet1")

	if !strings.Contains(out, `<caption>Sheet1</caption>`) {
		t.Fatalf("want <caption>Sheet1</caption>, got:\n%s", out)
	}
	if !strings.Contains(out, "<tr><th>Name</th><th>Age</th></tr>") {
		t.Fatalf("want header row as <th>, got:\n%s", out)
	}
	if !strings.Contains(out, "<tr><td>Alice</td><td>30</td></tr>") {
		t.Fatalf("want data row as <td>, got:\n%s", out)
	}
	if strings.Contains(out, "<thead>") || strings.Contains(out, "<tbody>") {
		t.Fatalf("must not emit <thead>/<tbody> to stay byte-compatible with Python/CSV, got:\n%s", out)
	}
	// Exactly one <table> for <=256 data rows.
	if n := strings.Count(out, "<table>"); n != 1 {
		t.Fatalf("want 1 <table>, got %d:\n%s", n, out)
	}
}

// TestRecordsToHTMLTableChunks_Chunking asserts 256-row chunking with a repeated
// header (ceil(n_data / chunk_rows) chunks).
func TestRecordsToHTMLTableChunks_Chunking(t *testing.T) {
	const dataRows = 300
	records := make([][]string, 0, dataRows+1)
	records = append(records, []string{"C1", "C2"})
	for i := 0; i < dataRows; i++ {
		records = append(records, []string{"x", "y"})
	}
	out := recordsToHTMLTableChunks(records, 256, "S")
	// 300 data rows → ceil(300/256) = 2 chunks, each repeating the header.
	if n := strings.Count(out, "<table>"); n != 2 {
		t.Fatalf("want 2 <table> chunks, got %d", n)
	}
	if n := strings.Count(out, "<tr><th>C1</th><th>C2</th></tr>"); n != 2 {
		t.Fatalf("want header repeated in both chunks, got %d", n)
	}
}

func TestRecordsToHTMLTableChunkList_RowRange(t *testing.T) {
	records := make([][]string, 0, 25)
	records = append(records, []string{"H"})
	for i := 0; i < 24; i++ {
		records = append(records, []string{"x"})
	}
	chunks := recordsToHTMLTableChunkList(records, 12, "S", 1)
	if len(chunks) != 2 {
		t.Fatalf("want 2 chunks, got %d", len(chunks))
	}
	if chunks[0].RowStart != 2 || chunks[0].RowEnd != 13 {
		t.Fatalf("chunk0 rows = %d-%d, want 2-13", chunks[0].RowStart, chunks[0].RowEnd)
	}
	if chunks[1].RowStart != 14 || chunks[1].RowEnd != 25 {
		t.Fatalf("chunk1 rows = %d-%d, want 14-25", chunks[1].RowStart, chunks[1].RowEnd)
	}
}

func TestRecordsToHTMLTableChunkList_ColEndUsesWidestRow(t *testing.T) {
	records := [][]string{
		{"H"},
		{"a", "b", "c"},
	}
	chunks := recordsToHTMLTableChunkList(records, 12, "S", 1)
	if len(chunks) != 1 {
		t.Fatalf("want 1 chunk, got %d", len(chunks))
	}
	if chunks[0].ColEnd != 3 {
		t.Fatalf("ColEnd = %d, want 3", chunks[0].ColEnd)
	}
}

// TestXLSXParser_HeaderAndCaption asserts the XLSX parser emits a <caption> and
// renders the first row as <th>, and that the header text appears only in <th>.
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
	html := xlsxTableHTML(res)
	if !strings.Contains(html, `<caption>Sheet1</caption>`) {
		t.Fatalf("want <caption>Sheet1</caption>, got:\n%s", html)
	}
	if !strings.Contains(html, "<tr><th>Product</th><th>Price</th></tr>") {
		t.Fatalf("want header as <th>, got:\n%s", html)
	}
	// "Product" must only appear inside a <th>, never inside a <td>.
	if strings.Contains(html, "<td>Product</td>") {
		t.Fatalf("header value leaked into <td>:\n%s", html)
	}
}

// TestXLSXParser_MergedHeaderInheritance asserts a horizontally merged header
// cell's slave columns inherit the master text, so a wide merged title does not
// render as a row of blank <th> cells.
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
	html := xlsxTableHTML(res)
	// The merged master text must have propagated into all three <th> slots.
	if !strings.Contains(html, "<tr><th>Sales Report</th><th>Sales Report</th><th>Sales Report</th></tr>") {
		t.Fatalf("merged master text not inherited into header <th>, got:\n%s", html)
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
	if !strings.Contains(xlsxTableHTML(res), "<tr><th>Name</th><th>Age</th></tr>") {
		t.Fatalf("want ListObject header row detected, got:\n%s", xlsxTableHTML(res))
	}
	if strings.Contains(xlsxTableHTML(res), "<th>Title</th>") {
		t.Fatalf("title row must not be the header:\n%s", xlsxTableHTML(res))
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
	if !strings.Contains(xlsxTableHTML(res), "<tr><th>Item</th><th>Count</th></tr>") {
		t.Fatalf("want row-2 header detected, got:\n%s", xlsxTableHTML(res))
	}
	if strings.Contains(xlsxTableHTML(res), "<th>100</th>") {
		t.Fatalf("numeric row 1 must not be the header:\n%s", xlsxTableHTML(res))
	}
}

// TestXLSXParser_CommonCaseNoRegression asserts the dominant case (header on
// row 1, no merges, no ListObject) renders row 1 as <th> unchanged.
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
	if !strings.Contains(xlsxTableHTML(res), "<tr><th>col_a</th><th>col_b</th></tr>") {
		t.Fatalf("common-case header must render as <th>:\n%s", xlsxTableHTML(res))
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
	if !strings.Contains(xlsxTableHTML(res), "<tr><th>2023</th><th>2024</th></tr>") {
		t.Fatalf("numeric row 1 must remain the header:\n%s", xlsxTableHTML(res))
	}
	if strings.Contains(xlsxTableHTML(res), "<th>Total</th>") {
		t.Fatalf("bold subtotal row must not be promoted to header:\n%s", xlsxTableHTML(res))
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
	if !strings.Contains(xlsxTableHTML(res), "<th>Name</th><th>Desc</th>") {
		t.Fatalf("narrow bold header past far merge must be detected:\n%s", xlsxTableHTML(res))
	}
	if strings.Contains(xlsxTableHTML(res), "<th>Sales Report</th>") {
		t.Fatalf("wide merged title must not be the header:\n%s", xlsxTableHTML(res))
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
	if !strings.Contains(xlsxTableHTML(res), "<tr><th>Product</th><th>Units</th></tr>") {
		t.Fatalf("styled text header over numeric data must be detected:\n%s", xlsxTableHTML(res))
	}
	if strings.Contains(xlsxTableHTML(res), "<th>Report Title</th>") {
		t.Fatalf("title stub must not be the header:\n%s", xlsxTableHTML(res))
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

// TestDecodeChunkRows exercises the chunk_rows setup knob across all the types
// JSON/yaml decoding can produce, plus the defaulting paths.
func TestDecodeChunkRows(t *testing.T) {
	if got := decodeChunkRows(nil); got != defaultTableChunkRows {
		t.Fatalf("nil setup: want default %d, got %d", defaultTableChunkRows, got)
	}
	if got := decodeChunkRows(map[string]any{}); got != defaultTableChunkRows {
		t.Fatalf("empty setup: want default, got %d", got)
	}
	if got := decodeChunkRows(map[string]any{"chunk_rows": 100.0}); got != 100 {
		t.Fatalf("float64 100: want 100, got %d", got)
	}
	if got := decodeChunkRows(map[string]any{"chunk_rows": 0.0}); got != defaultTableChunkRows {
		t.Fatalf("float64 0: want default, got %d", got)
	}
	if got := decodeChunkRows(map[string]any{"chunk_rows": -5.0}); got != defaultTableChunkRows {
		t.Fatalf("float64 -5: want default, got %d", got)
	}
	if got := decodeChunkRows(map[string]any{"chunk_rows": 256}); got != 256 {
		t.Fatalf("int 256: want 256, got %d", got)
	}
	if got := decodeChunkRows(map[string]any{"chunk_rows": int64(512)}); got != 512 {
		t.Fatalf("int64 512: want 512, got %d", got)
	}
	if got := decodeChunkRows(map[string]any{"chunk_rows": "256"}); got != defaultTableChunkRows {
		t.Fatalf("string (unsupported type): want default, got %d", got)
	}
}

// TestRenderSheetTables_FarMergeDoesNotBloatDataRows guards the memory blow-up
// from review: a far merge on a non-header row must not widen the header or any
// data row to the merge's full span. The header stays its natural width and no
// data row carries empty <td> cells.
func TestRenderSheetTables_FarMergeDoesNotBloatDataRows(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		// Row 1: a wide merged title A1:Z1 (NOT the header).
		mustSetCell(t, f, "Sheet1", "A1", "Report Title")
		mustMergeCell(t, f, "Sheet1", "A1", "Z1")
		// Row 2: the real header, made bold so detection anchors it.
		hdrIdx := mustNewStyle(t, f, &excelize.Style{Font: &excelize.Font{Bold: true}})
		mustSetCell(t, f, "Sheet1", "A2", "Name")
		mustSetCell(t, f, "Sheet1", "B2", "Price")
		mustSetCellStyle(t, f, "Sheet1", "A2", "B2", hdrIdx)
		// Rows 3-4: data.
		mustSetCell(t, f, "Sheet1", "A3", "Alice")
		mustSetCell(t, f, "Sheet1", "B3", "9.99")
		mustSetCell(t, f, "Sheet1", "A4", "Bob")
		mustSetCell(t, f, "Sheet1", "B4", "19.99")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	html := xlsxTableHTML(res)
	if !strings.Contains(html, "<tr><th>Name</th><th>Price</th></tr>") {
		t.Fatalf("want row-2 header detected, got:\n%s", html)
	}
	if strings.Contains(html, "<th>Title</th>") {
		t.Fatalf("wide merged title must not be the header:\n%s", html)
	}
	// The far merge is on row 1, not the header, so nothing is widened to 26
	// columns: the header keeps 2 columns (no empty <th>) and data rows carry
	// no empty <td>.
	if strings.Contains(html, "<th></th>") {
		t.Fatalf("header was bloated by the far merge:\n%s", html)
	}
	if strings.Count(html, "<td></td>") != 0 {
		t.Fatalf("data rows were bloated by the far merge:\n%s", html)
	}
}

// TestRenderSheetTables_MultiSheet asserts each sheet becomes its own <table>
// chunk, each carrying its own <caption>.
func TestRenderSheetTables_MultiSheet(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
		mustSetCell(t, f, "Sheet1", "B1", "Age")
		mustSetCell(t, f, "Sheet1", "A2", "Alice")
		mustSetCell(t, f, "Sheet1", "B2", "30")
		if _, err := f.NewSheet("HR"); err != nil {
			t.Fatalf("NewSheet: %v", err)
		}
		mustSetCell(t, f, "HR", "A1", "Dept")
		mustSetCell(t, f, "HR", "B1", "Head")
		mustSetCell(t, f, "HR", "A2", "Eng")
		mustSetCell(t, f, "HR", "B2", "Bob")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	html := xlsxTableHTML(res)
	if !strings.Contains(html, "<caption>Sheet1</caption>") {
		t.Fatalf("want Sheet1 caption, got:\n%s", html)
	}
	if !strings.Contains(html, "<caption>HR</caption>") {
		t.Fatalf("want HR caption, got:\n%s", html)
	}
	// Two small sheets → two <table> chunks.
	if n := strings.Count(html, "<table>"); n != 2 {
		t.Fatalf("want 2 <table> chunks, got %d:\n%s", n, html)
	}
}

// TestRenderSheetTables_EmptySheet asserts an empty/unreadable sheet yields an
// empty string rather than an empty <table> wrapper.
func TestRenderSheetTables_EmptySheet(t *testing.T) {
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
