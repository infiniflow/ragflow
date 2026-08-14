package parser

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

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

// TestRecordsToHTMLTableChunks_Alignment asserts the chunked output matches the
// Python deepdoc / Go-CSV contract: <caption>, first row as <th>, data as <td>,
// repeated header per 256-row chunk, and NO <thead>/<tbody> wrapper.
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
// header, matching Python's ceil(n_data / chunk_rows) behaviour.
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

// TestXLSXParser_HeaderAndCaption asserts the XLSX parser emits a <caption> and
// renders the first row as <th>, and that the header text appears only in <th>.
func TestXLSXParser_HeaderAndCaption(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		f.SetCellValue("Sheet1", "A1", "Product")
		f.SetCellValue("Sheet1", "B1", "Price")
		f.SetCellValue("Sheet1", "A2", "Widget")
		f.SetCellValue("Sheet1", "B2", "9.99")
		f.SetCellValue("Sheet1", "A3", "Gadget")
		f.SetCellValue("Sheet1", "B3", "19.99")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	html := res.HTML
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
// cell's slave columns inherit the master text, fixing the empty-<th> defect
// that the Python deepdoc path suffers from.
func TestXLSXParser_MergedHeaderInheritance(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		// Row 1 is the header; A1:C1 merged into one wide label "Sales Report".
		f.SetCellValue("Sheet1", "A1", "Sales Report")
		f.MergeCell("Sheet1", "A1", "C1")
		// Make the header row bold so detection anchors row 1.
		style := &excelize.Style{Font: &excelize.Font{Bold: true}}
		idx, _ := f.NewStyle(style)
		f.SetCellStyle("Sheet1", "A1", "C1", idx)
		f.SetCellValue("Sheet1", "A2", "North")
		f.SetCellValue("Sheet1", "B2", "10")
		f.SetCellValue("Sheet1", "C2", "20")
		f.SetCellValue("Sheet1", "A3", "South")
		f.SetCellValue("Sheet1", "B3", "30")
		f.SetCellValue("Sheet1", "C3", "40")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	html := res.HTML
	// The merged master text must have propagated into all three <th> slots.
	if !strings.Contains(html, "<tr><th>Sales Report</th><th>Sales Report</th><th>Sales Report</th></tr>") {
		t.Fatalf("merged master text not inherited into header <th>, got:\n%s", html)
	}
}

// TestDetectHeaderRow_ListObject asserts a ListObject whose top row is > 1 is
// detected as the header row.
func TestDetectHeaderRow_ListObject(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		f.SetCellValue("Sheet1", "A1", "Title")
		f.SetCellValue("Sheet1", "A2", "This is a banner")
		f.SetCellValue("Sheet1", "A3", "Name")
		f.SetCellValue("Sheet1", "B3", "Age")
		f.SetCellValue("Sheet1", "A4", "Alice")
		f.SetCellValue("Sheet1", "B4", "30")
		f.AddTable("Sheet1", &excelize.Table{Range: "A3:B4", Name: "Table1"})
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(res.HTML, "<tr><th>Name</th><th>Age</th></tr>") {
		t.Fatalf("want ListObject header row detected, got:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, "<th>Title</th>") {
		t.Fatalf("title row must not be the header:\n%s", res.HTML)
	}
}

// TestDetectHeaderRow_Lightweight asserts that when row 1 is numeric data and
// row 2 is a styled text label row, the lightweight detector picks row 2.
func TestDetectHeaderRow_Lightweight(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		f.SetCellValue("Sheet1", "A1", "100")
		f.SetCellValue("Sheet1", "B1", "200")
		f.SetCellValue("Sheet1", "A2", "Item")
		f.SetCellValue("Sheet1", "B2", "Count")
		// Make the header row bold so the styled signal fires.
		style := &excelize.Style{Font: &excelize.Font{Bold: true}}
		idx, _ := f.NewStyle(style)
		f.SetCellStyle("Sheet1", "A2", "B2", idx)
		f.SetCellValue("Sheet1", "A3", "Apple")
		f.SetCellValue("Sheet1", "B3", "5")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(res.HTML, "<tr><th>Item</th><th>Count</th></tr>") {
		t.Fatalf("want row-2 header detected, got:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, "<th>100</th>") {
		t.Fatalf("numeric row 1 must not be the header:\n%s", res.HTML)
	}
}

// TestXLSXParser_CommonCaseNoRegression asserts the dominant case (header on
// row 1, no merges, no ListObject) is byte-compatible with the Python/CSV path:
// row 1 becomes <th> unchanged.
func TestXLSXParser_CommonCaseNoRegression(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		f.SetCellValue("Sheet1", "A1", "col_a")
		f.SetCellValue("Sheet1", "B1", "col_b")
		f.SetCellValue("Sheet1", "A2", "1")
		f.SetCellValue("Sheet1", "B2", "2")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(res.HTML, "<tr><th>col_a</th><th>col_b</th></tr>") {
		t.Fatalf("common-case header must render as <th>:\n%s", res.HTML)
	}
}

// TestDetectHeaderRow_BoldSubtotalNotHeader asserts that a bold "Total"-style
// subtotal row sitting directly under a numeric row 1 is NOT promoted to the
// header. The body-brake (the row directly below the candidate must contain a
// text cell) blocks the override so the numeric row 1 stays the header —
// matching Python's "first row is the header" default.
func TestDetectHeaderRow_BoldSubtotalNotHeader(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		// Row 1: numeric year header (Python default header row).
		f.SetCellValue("Sheet1", "A1", "2023")
		f.SetCellValue("Sheet1", "B1", "2024")
		// Row 2: bold "Total/Summary" subtotal — must NOT become the header.
		style := &excelize.Style{Font: &excelize.Font{Bold: true}}
		idx, _ := f.NewStyle(style)
		f.SetCellValue("Sheet1", "A2", "Total")
		f.SetCellValue("Sheet1", "B2", "Summary")
		f.SetCellStyle("Sheet1", "A2", "B2", idx)
		// Row 3: purely numeric continuation under the subtotal.
		f.SetCellValue("Sheet1", "A3", "100")
		f.SetCellValue("Sheet1", "B3", "200")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(res.HTML, "<tr><th>2023</th><th>2024</th></tr>") {
		t.Fatalf("numeric row 1 must remain the header:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, "<th>Total</th>") {
		t.Fatalf("bold subtotal row must not be promoted to header:\n%s", res.HTML)
	}
}

// TestDetectHeaderRow_StyledHeaderPastFarMerge asserts that a bold header on a
// narrow row (row 3) is still detected even when an earlier wide merge
// (A1:Z1 title) would otherwise pad every row to 26 columns. The padding step
// must run AFTER header detection so the styled-majority signal is not diluted.
func TestDetectHeaderRow_StyledHeaderPastFarMerge(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		// Row 1: a wide merged title cell A1:Z1.
		f.SetCellValue("Sheet1", "A1", "Sales Report")
		f.MergeCell("Sheet1", "A1", "Z1")
		titleStyle := &excelize.Style{Font: &excelize.Font{Bold: true}}
		tidx, _ := f.NewStyle(titleStyle)
		f.SetCellStyle("Sheet1", "A1", "A1", tidx)
		// Row 3: the real, narrow, bold header.
		hdrStyle := &excelize.Style{Font: &excelize.Font{Bold: true}}
		hidx, _ := f.NewStyle(hdrStyle)
		f.SetCellValue("Sheet1", "A3", "Name")
		f.SetCellValue("Sheet1", "B3", "Desc")
		f.SetCellStyle("Sheet1", "A3", "B3", hidx)
		// Row 4: data (text in col A, so the body-brake passes for row 3).
		f.SetCellValue("Sheet1", "A4", "Alice")
		f.SetCellValue("Sheet1", "B4", "x")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if !strings.Contains(res.HTML, "<th>Name</th><th>Desc</th>") {
		t.Fatalf("narrow bold header past far merge must be detected:\n%s", res.HTML)
	}
	if strings.Contains(res.HTML, "<th>Sales Report</th>") {
		t.Fatalf("wide merged title must not be the header:\n%s", res.HTML)
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
