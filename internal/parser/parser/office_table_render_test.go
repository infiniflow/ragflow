package parser

import (
	"html"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

func TestXLSXParserEmitsSegmentedHTMLTable(t *testing.T) {
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
	if len(res.JSON) != 1 {
		t.Fatalf("items = %d, want one segmented HTML table item", len(res.JSON))
	}
	item := res.JSON[0].ToMap()
	if item["doc_type_kwd"] != "table" {
		t.Fatalf("item doc_type_kwd = %v, want table", item["doc_type_kwd"])
	}
	wantText := "<table><caption>Sheet1</caption>\n" +
		"<tr><th>ID</th><th>Status</th></tr>\n" +
		"<tr><td>A-100</td><td>paid</td></tr>\n" +
		"<tr><td>A-101</td><td>pending</td></tr>\n" +
		"</table>\n"
	rendered := tableItemHTML(t, item)
	if rendered != wantText {
		t.Fatalf("table HTML = %q, want %q", rendered, wantText)
	}
	if item["sheet"] != "Sheet1" || item["sheet_index"] != 1 {
		t.Fatalf("sheet metadata = %#v", item)
	}
	positions := item["positions"].([][]float64)
	if len(positions) != 3 {
		t.Fatalf("len(positions) = %d, want one tuple per <tr>", len(positions))
	}
	if got := positions[1]; !reflect.DeepEqual(got, []float64{1, 2, 2, 1, 2}) {
		t.Fatalf("row 2 tuple = %v, want [1 2 2 1 2]", got)
	}
	if got := positions[2]; !reflect.DeepEqual(got, []float64{1, 3, 3, 1, 2}) {
		t.Fatalf("row 3 tuple = %v, want [1 3 3 1 2]", got)
	}
}

// TestXLSXParserHTML4ExcelIsIgnored pins the retirement: html4excel is still
// accepted at the entry (with a deprecation warning) but selects nothing —
// the wire is identical to the default path.
func TestXLSXParserHTML4ExcelIsIgnored(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Question")
		mustSetCell(t, f, "Sheet1", "B1", "Answer")
		mustSetCell(t, f, "Sheet1", "A2", "Q1")
		mustSetCell(t, f, "Sheet1", "B2", "A1")
	})
	plain, _ := NewXLSXParser("")
	plainRes := plain.ParseWithResult(t.Context(), "qa.xlsx", data)
	if plainRes.Err != nil {
		t.Fatalf("ParseWithResult: %v", plainRes.Err)
	}
	p, _ := NewXLSXParser("")
	p.ConfigureFromSetup(map[string]any{"html4excel": true})
	res := p.ParseWithResult(t.Context(), "qa.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("items = %d, want one atomic table item", len(res.JSON))
	}
	if res.JSON[0].ToMap()["doc_type_kwd"] != "table" || res.JSON[0].ToMap()["table"] == nil {
		t.Fatalf("html4excel item = %#v", res.JSON[0])
	}
	if !reflect.DeepEqual(res.JSON, plainRes.JSON) {
		t.Errorf("html4excel changed the wire: %v vs %v", res.JSON, plainRes.JSON)
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
	if len(res.JSON) != 1 {
		t.Fatalf("items = %d, want one HTML table item (parser must not pre-split rows)", len(res.JSON))
	}
	positions := res.JSON[0].ToMap()["positions"].([][]float64)
	if len(positions) != dataRows+1 {
		t.Fatalf("len(positions) = %d, want one tuple per <tr> (header plus %d rows)", len(positions), dataRows)
	}
	last := positions[len(positions)-1]
	if last[1] != float64(dataRows+1) || last[2] != float64(dataRows+1) {
		t.Fatalf("last tuple = %v, want source row %d", last, dataRows+1)
	}
	text := tableItemHTML(t, res.JSON[0].ToMap())
	if !strings.Contains(text, "<td>258</td>") {
		t.Fatalf("last data row missing from markup:\n%s", text)
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
	if len(res.JSON) != 2 {
		t.Fatalf("items = %d, want one HTML table per sheet", len(res.JSON))
	}
	first, second := res.JSON[0].ToMap(), res.JSON[1].ToMap()
	if first["sheet"] != "Sheet1" || first["sheet_index"] != 1 {
		t.Fatalf("first sheet identity = %#v", first)
	}
	if second["sheet"] != "Orders" || second["sheet_index"] != 2 {
		t.Fatalf("second sheet identity = %#v", second)
	}
	// Table grouping collapses into sheet_index; table_id no longer exists
	// on HTML table items (segmentation is done by the parser itself).
	if _, ok := first["table_id"]; ok {
		t.Errorf("table_id must not be emitted on segmented table items")
	}
}

// TestXLSXParserEmitsHeaderOnlySegment: a sheet with no data rows still
// emits its header as the only searchable representation of the column
// schema — one segment whose markup holds the single <tr> of <th> cells.
func TestXLSXParserEmitsHeaderOnlySegment(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Name")
		mustSetCell(t, f, "Sheet1", "B1", "Amount")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "headers.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("items = %d, want one header-only segment", len(res.JSON))
	}
	item := res.JSON[0].ToMap()
	wantText := "<table><caption>Sheet1</caption>\n" +
		"<tr><th>Name</th><th>Amount</th></tr>\n" +
		"</table>\n"
	if rendered := tableItemHTML(t, item); rendered != wantText {
		t.Fatalf("table HTML = %q, want %q", rendered, wantText)
	}
	positions, _ := item["positions"].([][]float64)
	if len(positions) != 1 || !reflect.DeepEqual(positions[0], []float64{1, 1, 1, 1, 2}) {
		t.Fatalf("positions = %v, want the header tuple only", positions)
	}
}

// TestBuildSheetItemsEscapesCellText: cell text carrying markup characters is
// escaped in the wire and reads back byte-identical from the markup, so
// user content can never become markup.
func TestBuildSheetItemsEscapesCellText(t *testing.T) {
	const tricky = `<b>bold</b> & "quoted" > 1`
	items := buildSheetItems([][]string{{"Name"}, {tricky}}, "Sheet1", 1, 1, nil, nil)
	if len(items) != 1 {
		t.Fatalf("items = %d, want one segment", len(items))
	}
	text := tableItemHTML(t, items[0])
	if strings.Contains(text, "<b>") || !strings.Contains(text, "&lt;b&gt;") {
		t.Fatalf("cell text was not escaped: %q", text)
	}
	rows := markupRows(t, text)
	if len(rows) != 2 || rows[1][0] != tricky {
		t.Fatalf("round trip = %#v, want %q back", rows, tricky)
	}
}

// TestXLSXParserImageAnchorsSplitSegments pins the segmentation semantics: an
// image anchored between rows ends the current segment at its anchor, every
// segment repeats the header row, and a data row sharing the anchor's row is
// ordered by column — cells at or left of the anchor column stay in front of
// the image, cells to its right follow it.
func TestXLSXParserImageAnchorsSplitSegments(t *testing.T) {
	const pngBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNk+M8AAAMBAQDJ/pLvAAAAAElFTkSuQmCC"

	build := func(t *testing.T, fillRow4 func(*excelize.File)) []map[string]any {
		t.Helper()
		data := newTestXLSX(t, func(f *excelize.File) {
			mustSetCell(t, f, "Sheet1", "A1", "Name")
			mustSetCell(t, f, "Sheet1", "B1", "Value")
			// Every non-anchor row carries two columns so the header
			// detector never sees row 4 as the only wide row.
			for row, value := range map[int]string{2: "row2", 3: "row3", 5: "row5", 6: "row6"} {
				mustSetCell(t, f, "Sheet1", cellAxis(row, 1), value)
				mustSetCell(t, f, "Sheet1", cellAxis(row, 2), "x")
			}
			fillRow4(f)
			if err := f.AddPictureFromBytes("Sheet1", "A4", &excelize.Picture{
				Extension: ".png",
				File:      mustDecodeBase64(t, pngBase64),
				Format:    &excelize.GraphicOptions{AltText: "fig"},
			}); err != nil {
				t.Fatalf("AddPictureFromBytes: %v", err)
			}
		})
		p, _ := NewXLSXParser("")
		res := p.ParseWithResult(t.Context(), "anchors.xlsx", data)
		if res.Err != nil {
			t.Fatalf("ParseWithResult: %v", res.Err)
		}
		return ItemsToMaps(res.JSON)
	}

	assertSegments := func(t *testing.T, items []map[string]any, wantFirst, wantSecond []string) {
		t.Helper()
		if len(items) != 3 {
			t.Fatalf("items = %d, want table, image, table", len(items))
		}
		if items[1]["doc_type_kwd"] != "image" || items[1]["row_start"] != 4 {
			t.Fatalf("image item = %#v", items[1])
		}
		for segIdx, want := range [][]string{wantFirst, wantSecond} {
			item := items[segIdx*2]
			text := tableItemHTML(t, item)
			rows := markupRows(t, text)
			if len(rows) != len(want)+1 {
				t.Fatalf("segment %d rows = %d, want header plus %d", segIdx, len(rows), len(want))
			}
			if rows[0][0] != "Name" {
				t.Errorf("segment %d lost the replicated header: %#v", segIdx, rows[0])
			}
			for i, value := range want {
				found := false
				for _, cell := range rows[i+1] {
					if cell == value {
						found = true
					}
				}
				if !found {
					t.Errorf("segment %d row %d = %#v, want %q in it", segIdx, i, rows[i+1], value)
				}
			}
			positions, _ := item["positions"].([][]float64)
			if len(positions) != len(rows) {
				t.Errorf("segment %d positions = %d, want one tuple per <tr> (%d)", segIdx, len(positions), len(rows))
			}
		}
	}

	t.Run("row at the anchor cell stays in front", func(t *testing.T) {
		items := build(t, func(f *excelize.File) {
			mustSetCell(t, f, "Sheet1", "A4", "row4")
			mustSetCell(t, f, "Sheet1", "B4", "x")
		})
		assertSegments(t, items, []string{"row2", "row3", "row4"}, []string{"row5", "row6"})
	})

	t.Run("row right of the anchor cell follows the image", func(t *testing.T) {
		items := build(t, func(f *excelize.File) { mustSetCell(t, f, "Sheet1", "B4", "row4") })
		assertSegments(t, items, []string{"row2", "row3"}, []string{"row4", "row5", "row6"})
	})
}

// markupRows extracts the cell texts of every <tr> in rendered spreadsheet
// markup, unescaped. It is a deliberately minimal test-local reader: these
// tests assert the parser's rendered output, so the markup itself is the
// contract under test.
func markupRows(t *testing.T, markup string) [][]string {
	t.Helper()
	rowRe := regexp.MustCompile(`(?s)<tr>(.*?)</tr>`)
	cellRe := regexp.MustCompile(`(?s)<t[hd]>(.*?)</t[hd]>`)
	var rows [][]string
	for _, row := range rowRe.FindAllStringSubmatch(markup, -1) {
		var cells []string
		for _, cell := range cellRe.FindAllStringSubmatch(row[1], -1) {
			cells = append(cells, html.UnescapeString(cell[1]))
		}
		rows = append(rows, cells)
	}
	return rows
}

// tableItemHTML renders a parser-produced structured table item back to the
// legacy spreadsheet HTML so the byte-level assertions in these tests keep
// pinning the same wire shape. The producer emits *entity.TableData now, and
// RenderTableHTML reproduces the legacy markup exactly.
func tableItemHTML(t *testing.T, item map[string]any) string {
	t.Helper()
	td, ok := item["table"].(*entity.TableData)
	if !ok {
		t.Fatalf("item missing *entity.TableData table field: %#v", item)
	}
	return tableutil.RenderTableHTML(td)
}

// renderedText returns the legacy HTML rendering of an item's structured table
// when present, otherwise its text field. It lets table-contract tests pin
// the byte-level wire shape whether the producer emitted TableData or plain
// text (e.g. non-table items).
func renderedText(item map[string]any) string {
	if td, ok := item["table"].(*entity.TableData); ok && td != nil {
		return tableutil.RenderTableHTML(td)
	}
	text, _ := item["text"].(string)
	return text
}

func spreadsheetText(res ParseResult) string {
	var out strings.Builder
	for _, item := range ItemsToMaps(res.JSON) {
		if td, ok := item["table"].(*entity.TableData); ok {
			out.WriteString(tableutil.RenderTableHTML(td))
		}
	}
	return out.String()
}

// spreadsheetHeaderText returns the leading <table> item's header labels
// joined by "; " — the rendered <th> cells of its first <tr> — or "" when no
// table item exists.
func spreadsheetHeaderText(res ParseResult) string {
	for _, item := range ItemsToMaps(res.JSON) {
		td, ok := item["table"].(*entity.TableData)
		if !ok {
			continue
		}
		text := tableutil.RenderTableHTML(td)
		row := text
		if i := strings.Index(row, "<tr>"); i >= 0 {
			row = row[i+len("<tr>"):]
		}
		if i := strings.Index(row, "</tr>"); i >= 0 {
			row = row[:i]
		}
		var labels []string
		for _, cell := range strings.Split(row, "<th>")[1:] {
			if i := strings.Index(cell, "</th>"); i >= 0 {
				labels = append(labels, cell[:i])
			}
		}
		if len(labels) > 0 {
			return strings.Join(labels, "; ")
		}
	}
	return ""
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
	_, warnings := extractXLSXImages(f, "MissingSheet", nil)
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

// TestXLSXParser_HeaderAndCaption asserts the XLSX parser renders the typed
// header row as <th> cells and data rows as <td> cells inside one captioned
// table, with header labels never duplicated as values.
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
	if !strings.Contains(text, "<tr><th>Product</th><th>Price</th></tr>") {
		t.Fatalf("want header row, got:\n%s", text)
	}
	if !strings.Contains(text, "<tr><td>Widget</td><td>9.99</td></tr>") {
		t.Fatalf("want data row, got:\n%s", text)
	}
	// The header label must not be duplicated as a value in any row.
	if strings.Contains(text, "<td>Product</td>") {
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
	if !strings.Contains(text, "<th>Sales Report</th><th>Sales Report</th><th>Sales Report</th>") {
		t.Fatalf("merged master text not inherited into header cells, got:\n%s", text)
	}
}

// TestXLSXParser_MergedDataCellInheritance asserts a vertically merged data
// cell's slave rows inherit the master text, so every row chunk keeps its
// group label instead of rendering an empty first cell.
func TestXLSXParser_MergedDataCellInheritance(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Group")
		mustSetCell(t, f, "Sheet1", "B1", "Item")
		mustSetCell(t, f, "Sheet1", "A2", "G1")
		mustMergeCell(t, f, "Sheet1", "A2", "A3")
		mustSetCell(t, f, "Sheet1", "B2", "x")
		mustSetCell(t, f, "Sheet1", "B3", "y")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	text := spreadsheetText(res)
	if !strings.Contains(text, "<tr><td>G1</td><td>y</td></tr>") {
		t.Fatalf("slave row did not inherit merged master text, got:\n%s", text)
	}
	positions := res.JSON[0].ToMap()["positions"].([][]float64)
	if len(positions) != 3 {
		t.Fatalf("len(positions) = %d, want one tuple per <tr>", len(positions))
	}
	if got := positions[2]; !reflect.DeepEqual(got, []float64{1, 3, 3, 1, 2}) {
		t.Fatalf("row 3 tuple = %v, want [1 3 3 1 2]", got)
	}
}

// TestXLSXParser_MergedDataCellHorizontalUntouched asserts a horizontally
// merged data cell is NOT duplicated: the slave column stays empty because
// the master column already shows the value.
func TestXLSXParser_MergedDataCellHorizontalUntouched(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Group")
		mustSetCell(t, f, "Sheet1", "B1", "Item")
		mustSetCell(t, f, "Sheet1", "C1", "Note")
		mustSetCell(t, f, "Sheet1", "A2", "G1")
		mustSetCell(t, f, "Sheet1", "B2", "wide")
		mustMergeCell(t, f, "Sheet1", "B2", "C2")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	text := spreadsheetText(res)
	if strings.Contains(text, "<td>wide</td><td>wide</td>") {
		t.Fatalf("horizontal merge master duplicated into slave cell, got:\n%s", text)
	}
}

// TestXLSXParser_MergedDataCellBeyondTruncation asserts a merged slave cell
// past its row's truncation point (excelize drops trailing empties) is still
// filled: the row grows exactly up to the slave column to host the value.
func TestXLSXParser_MergedDataCellBeyondTruncation(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "name")
		mustSetCell(t, f, "Sheet1", "B1", "age")
		mustSetCell(t, f, "Sheet1", "C1", "department")
		mustSetCell(t, f, "Sheet1", "D1", "notes")
		mustSetCell(t, f, "Sheet1", "A2", "Mike")
		mustSetCell(t, f, "Sheet1", "B2", 28)
		mustSetCell(t, f, "Sheet1", "C2", "Sales")
		mustSetCell(t, f, "Sheet1", "D2", "Important customer")
		mustMergeCell(t, f, "Sheet1", "A2", "A3")
		mustMergeCell(t, f, "Sheet1", "B2", "B3")
		mustMergeCell(t, f, "Sheet1", "D2", "D3")
		// Row 3's own value sits before the merged columns, so GetRows
		// truncates the row short of the D3 slave.
		mustSetCell(t, f, "Sheet1", "C3", "IT")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	text := spreadsheetText(res)
	want := "<tr><td>Mike</td><td>28</td><td>IT</td><td>Important customer</td></tr>"
	if !strings.Contains(text, want) {
		t.Fatalf("truncated slave cells not filled, got:\n%s", text)
	}
}

// TestXLSXParser_MergedHeaderDownIntoDataUntouched asserts a vertical merge
// mastered on the header row does not leak the header label into data rows.
func TestXLSXParser_MergedHeaderDownIntoDataUntouched(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "H")
		mustMergeCell(t, f, "Sheet1", "A1", "A2")
		mustSetCell(t, f, "Sheet1", "B1", "Name")
		mustSetCell(t, f, "Sheet1", "B2", "Alice")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	text := spreadsheetText(res)
	if !strings.Contains(text, "<tr><th>H</th><th>Name</th></tr>") {
		t.Fatalf("want header row untouched, got:\n%s", text)
	}
	if strings.Contains(text, "<td>H</td>") {
		t.Fatalf("header merge master leaked into data row, got:\n%s", text)
	}
}

// TestXLSXParser_StripsInvisibleFormatChars asserts zero-width format
// characters around cell text are removed, so labels wrapped in U+200C match
// their clean form in search text.
func TestXLSXParser_StripsInvisibleFormatChars(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Group")
		mustSetCell(t, f, "Sheet1", "B1", "Item")
		mustSetCell(t, f, "Sheet1", "A2", "\u200cG1\u200c")
		mustSetCell(t, f, "Sheet1", "B2", "x")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	text := spreadsheetText(res)
	if strings.Contains(text, "\u200c") {
		t.Fatalf("invisible format char survived cleaning, got:\n%q", text)
	}
	if !strings.Contains(text, "<tr><td>G1</td><td>x</td></tr>") {
		t.Fatalf("want cleaned cell text, got:\n%s", text)
	}
}

// TestCleanIllegalControlChars_StripsInvisibleFormatChars pins the shared
// cleaner directly, covering the xls/csv paths that reuse it.
// TestCleanIllegalControlChars_PreservesInternalJoiners asserts format
// characters that are meaningful inside a word survive cleaning: U+200D in
// an emoji ZWJ sequence and U+200C as the Persian half-space.
func TestCleanIllegalControlChars_PreservesInternalJoiners(t *testing.T) {
	emoji := "\U0001F469\u200d\U0001F4BB" // woman technologist
	persian := "\u0645\u06cc\u200c\u0631\u0648\u0645"
	in := [][]string{{emoji, persian}}
	got := cleanIllegalControlChars(in)
	if !reflect.DeepEqual(got, in) {
		t.Fatalf("cleanIllegalControlChars = %q, want internal joiners kept %q", got, in)
	}
}

func TestCleanIllegalControlChars_StripsInvisibleFormatChars(t *testing.T) {
	in := [][]string{{"\u200cA\u200c", "B\u200bC D", "D\ufeff"}}
	want := [][]string{{"A", "B\u200bC D", "D"}}
	got := cleanIllegalControlChars(in)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanIllegalControlChars = %q, want %q", got, want)
	}
}

// TestXLSXParser_KeepsTrailingDatalessColumn asserts a trailing column with
// a header label but no data in any row is kept as real schema: the header
// renders and its cells stay empty.
func TestXLSXParser_KeepsTrailingDatalessColumn(t *testing.T) {
	data := newTestXLSX(t, func(f *excelize.File) {
		mustSetCell(t, f, "Sheet1", "A1", "Group")
		mustSetCell(t, f, "Sheet1", "B1", "Score")
		mustSetCell(t, f, "Sheet1", "A2", "G1")
		mustSetCell(t, f, "Sheet1", "A3", "G2")
	})
	p, _ := NewXLSXParser("")
	res := p.ParseWithResult(t.Context(), "t.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult: %v", res.Err)
	}
	text := spreadsheetText(res)
	if !strings.Contains(text, "<tr><th>Group</th><th>Score</th></tr>") {
		t.Fatalf("trailing labeled column was dropped, got:\n%s", text)
	}
	if !strings.Contains(text, "<tr><td>G1</td><td></td></tr>") {
		t.Fatalf("dataless cell must render empty, got:\n%s", text)
	}
}

// TestTrimTrailingEmptyColumns_DropsFullyBlankColumn pins the helper
// directly: a trailing column blank in every row, header included (e.g. a
// CSV with trailing commas), is truncated.
func TestTrimTrailingEmptyColumns_DropsFullyBlankColumn(t *testing.T) {
	in := [][]string{
		{"Group", "Score", ""},
		{"G1", "", ""},
		{"G2", "", ""},
	}
	got := trimTrailingEmptyColumns(in)
	want := [][]string{
		{"Group", "Score"},
		{"G1", ""},
		{"G2", ""},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("trimTrailingEmptyColumns = %q, want %q", got, want)
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
	if !strings.Contains(spreadsheetText(res), "<th>Name</th><th>Age</th>") {
		t.Fatalf("want ListObject header row detected, got:\n%s", spreadsheetText(res))
	}
	if got := spreadsheetHeaderText(res); got != "Name; Age" {
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
	if !strings.Contains(spreadsheetText(res), "<th>Item</th><th>Count</th>") {
		t.Fatalf("want row-2 header detected, got:\n%s", spreadsheetText(res))
	}
	if strings.Contains(spreadsheetText(res), "<th>100</th><th>200</th>") {
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
	if !strings.Contains(spreadsheetText(res), "<th>col_a</th><th>col_b</th>") {
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
	if !strings.Contains(spreadsheetText(res), "<th>2023</th><th>2024</th>") {
		t.Fatalf("numeric row 1 must remain the header:\n%s", spreadsheetText(res))
	}
	if strings.Contains(spreadsheetText(res), "<th>Total</th><th>Summary</th>") {
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
	if !strings.Contains(spreadsheetText(res), "<th>Name</th><th>Desc</th>") {
		t.Fatalf("narrow bold header past far merge must be detected:\n%s", spreadsheetText(res))
	}
	if got := spreadsheetHeaderText(res); got != "Name; Desc" {
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
	if !strings.Contains(spreadsheetText(res), "<th>Product</th><th>Units</th>") {
		t.Fatalf("styled text header over numeric data must be detected:\n%s", spreadsheetText(res))
	}
	if got := spreadsheetHeaderText(res); got != "Product; Units" {
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
