package parser

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCSVParser_EmitsSpreadsheetRows(t *testing.T) {
	csvData := []byte("Name,Age,City\nAlice,30,New York\nBob,25,San Francisco\n")
	p := NewCSVParser()

	res := p.ParseWithResult(context.Background(), "test.csv", csvData)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}

	// Default output format is "json"
	if res.OutputFormat != "json" {
		t.Fatalf("res.OutputFormat = %q, want %q", res.OutputFormat, "json")
	}

	// One wire shape: the whole sheet is one segmented HTML table item.
	if len(res.JSON) != 1 {
		t.Fatalf("len(res.JSON) = %d, want one HTML table item", len(res.JSON))
	}
	item := res.JSON[0]
	if item["ck_type"] != "table" || item["doc_type_kwd"] != "table" {
		t.Fatalf("item types = %v/%v, want table/table", item["ck_type"], item["doc_type_kwd"])
	}
	wantText := "<table><caption>Data</caption>\n" +
		"<tr><th>Name</th><th>Age</th><th>City</th></tr>\n" +
		"<tr><td>Alice</td><td>30</td><td>New York</td></tr>\n" +
		"<tr><td>Bob</td><td>25</td><td>San Francisco</td></tr>\n" +
		"</table>\n"
	if item["text"] != wantText {
		t.Errorf("text = %q, want %q", item["text"], wantText)
	}
	if item["sheet"] != "Data" || item["sheet_index"] != 1 {
		t.Errorf("sheet metadata = %#v", item)
	}
	positions, ok := item["positions"].([][]float64)
	if !ok {
		t.Fatalf("item positions invalid: %T", item["positions"])
	}
	// Row-aligned matrix: one tuple per <tr> in markup order, header first.
	if len(positions) != 3 {
		t.Fatalf("len(positions) = %d, want 3 (one per <tr>)", len(positions))
	}
	for i, want := range [][]float64{{1, 1, 1, 1, 3}, {1, 2, 2, 1, 3}, {1, 3, 3, 1, 3}} {
		for j := range want {
			if positions[i][j] != want[j] {
				t.Errorf("positions[%d] = %v, want %v", i, positions[i], want)
				break
			}
		}
	}
}

// TestCSVParserHTML4ExcelIsIgnored pins the retirement: html4excel is still
// accepted at the entry (with a deprecation warning) but selects nothing —
// the wire is identical to the default path.
func TestCSVParserHTML4ExcelIsIgnored(t *testing.T) {
	data := []byte("Question,Answer\nQ1,A1\n")
	plain := NewCSVParser()
	plainRes := plain.ParseWithResult(context.Background(), "qa.csv", data)
	if plainRes.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", plainRes.Err)
	}
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{"html4excel": true})
	res := p.ParseWithResult(context.Background(), "qa.csv", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}
	if len(res.JSON) != 1 || res.JSON[0]["ck_type"] != "table" {
		t.Fatalf("items = %#v, want one table item", res.JSON)
	}
	if !reflect.DeepEqual(res.JSON, plainRes.JSON) {
		t.Errorf("html4excel changed the wire: %v vs %v", res.JSON, plainRes.JSON)
	}
}

func TestCSVParser_ConfigureOutputFormatJSON(t *testing.T) {
	csvData := []byte("col1,col2\nval1,val2\n")
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
	})

	res := p.ParseWithResult(context.Background(), "test.csv", csvData)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}

	if res.OutputFormat != "json" {
		t.Fatalf("res.OutputFormat = %q, want 'json'", res.OutputFormat)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("len(res.JSON) = %d, want one HTML table item", len(res.JSON))
	}
	if res.JSON[0]["ck_type"] != "table" {
		t.Fatalf("ck_type = %v, want table", res.JSON[0]["ck_type"])
	}
	if positions := res.JSON[0]["positions"].([][]float64); len(positions) != 2 {
		t.Fatalf("len(positions) = %d, want one tuple per <tr>", len(positions))
	}
}

func TestCSVParser_EmptyContent(t *testing.T) {
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
	})

	res := p.ParseWithResult(context.Background(), "empty.csv", []byte("   \n\n  "))
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}

	if res.OutputFormat != "json" {
		t.Fatalf("res.OutputFormat = %q, want 'json'", res.OutputFormat)
	}
	if len(res.JSON) != 0 {
		t.Fatalf("len(res.JSON) = %d, want 0", len(res.JSON))
	}
}

func TestCSVParser_PreservesEveryDataRow(t *testing.T) {
	// Header + 4 data rows; GeneralChunker owns any later token merge.
	csvData := []byte("Header\nr1\nr2\nr3\nr4\n")
	p := NewCSVParser()
	res := p.ParseWithResult(context.Background(), "chunked.csv", csvData)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}

	if len(res.JSON) != 1 {
		t.Fatalf("len(res.JSON) = %d, want one HTML table item", len(res.JSON))
	}
	positions := res.JSON[0]["positions"].([][]float64)
	if len(positions) != 5 {
		t.Fatalf("len(positions) = %d, want one tuple per <tr> (header plus four rows)", len(positions))
	}
	for i, pos := range positions[1:] {
		wantRow := float64(i + 2)
		if pos[1] != wantRow || pos[2] != wantRow {
			t.Errorf("row %d position = [%v, %v], want [%v, %v]", i, pos[1], pos[2], wantRow, wantRow)
		}
	}
}

func TestCSVParserKeepsVariableRowWidths(t *testing.T) {
	p := NewCSVParser()
	res := p.ParseWithResult(context.Background(), "mixed.csv", []byte("Question,Answer\nQ1,A1\nQ2,A2,extra\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("items = %d, want one HTML table item", len(res.JSON))
	}
	text, _ := res.JSON[0]["text"].(string)
	if !strings.Contains(text, "<tr><td>Q1</td><td>A1</td></tr>") ||
		!strings.Contains(text, "<tr><td>Q2</td><td>A2</td><td>extra</td></tr>") {
		t.Fatalf("row widths not preserved in markup: %q", text)
	}
	positions := res.JSON[0]["positions"].([][]float64)
	// Per-row column ranges: Q1 row spans cols 1-2, Q2 row spans 1-3.
	if len(positions) != 3 || positions[1][4] != 2 || positions[2][4] != 3 {
		t.Fatalf("position matrix lost per-row widths: %v", positions)
	}
}

func TestCSVParser_ReadsTheSeparatorTheFileWasWrittenWith(t *testing.T) {
	// A semicolon, tab or pipe separated .csv used to come out as one column
	// holding the whole line.
	for _, sep := range []string{",", ";", "\t", "|"} {
		data := []byte("Name" + sep + "Region" + sep + "Units\nWidget" + sep + "EU" + sep + "12\n")
		res := NewCSVParser().ParseWithResult(context.Background(), "export.csv", data)
		if res.Err != nil {
			t.Fatalf("separator %q: ParseWithResult failed: %v", sep, res.Err)
		}
		if len(res.JSON) != 1 {
			t.Fatalf("separator %q: items = %d, want one HTML table item", sep, len(res.JSON))
		}
		text, _ := res.JSON[0]["text"].(string)
		if !strings.Contains(text, "<tr><th>Name</th><th>Region</th><th>Units</th></tr>") ||
			!strings.Contains(text, "<tr><td>Widget</td><td>EU</td><td>12</td></tr>") {
			t.Errorf("separator %q: row not split into three columns: %q", sep, text)
		}
	}
}

func TestDetectCSVDelimiter(t *testing.T) {
	cases := []struct {
		name string
		text string
		want rune
	}{
		{"comma inside a sentence", "Name;Note\nWidget;Red, blue and green\nCable;Short\n", ';'},
		{"semicolon inside a quoted field", "Name,Note\nWidget,\"a;b\"\nCable,\"c;d;e\"\n", ','},
		{"single column", "Name\nWidget\nCable\n", ','},
		{"whitespace-only row", "Name;Region\n   \nWidget;EU\n", ';'},
		{"whitespace-only lines before the header", strings.Repeat("  \n", csvSampleRows) + "Name;Region\nWidget;EU\n", ';'},
		{"rows disagree under every separator", "Name,Note\nWidget\n", ','},
	}
	for _, tc := range cases {
		if got := detectCSVDelimiter(tc.text); got != tc.want {
			t.Errorf("%s: detectCSVDelimiter = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestDetectCSVDelimiter_LeavesOutTheRowTheSampleCutsShort(t *testing.T) {
	// The sample ends right after "Cable;E", so its last row has two fields
	// where the complete rows have three.
	header := "Name;Region;Units\n"
	filler := "Widget;EU;" + strings.Repeat("9", csvSampleBytes-len(header)-len("Widget;EU;\nCable;E")) + "\n"
	text := header + filler + "Cable;EU;12\n"
	if !strings.HasSuffix(text[:csvSampleBytes], "\nCable;E") {
		t.Fatalf("sample does not end inside the third row: %q", text[csvSampleBytes-12:csvSampleBytes])
	}
	if got := detectCSVDelimiter(text); got != ';' {
		t.Errorf("detectCSVDelimiter = %q, want ';'", got)
	}
}

func TestDetectCSVDelimiter_LeavesOutTheLastSampledRowWhenTheCutEndsIt(t *testing.T) {
	// csvSampleRows rows fit in the sample only because the last of them is
	// cut short: "18;xxx" has two fields where the others have three.
	cell := strings.Repeat("x", 3600)
	var b strings.Builder
	b.WriteString("a;b;c\n")
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&b, "%d;%s;end\n", i, cell)
	}
	text := b.String()
	if n := strings.Count(text[:csvSampleBytes], "\n"); n != csvSampleRows-1 {
		t.Fatalf("sample holds %d complete rows, want %d", n, csvSampleRows-1)
	}
	if got := detectCSVDelimiter(text); got != ';' {
		t.Errorf("detectCSVDelimiter = %q, want ';'", got)
	}
}

func TestCSVParser_KeepsAnEmptyTabSeparatedField(t *testing.T) {
	// Trimming leading space would also trim the tab after "Widget", leaving
	// the row a field short of its header, and the tab would lose to the comma.
	data := []byte("Name\tRegion\tUnits\nWidget\t\t12\n")
	res := NewCSVParser().ParseWithResult(context.Background(), "export.csv", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("items = %d, want one HTML table item", len(res.JSON))
	}
	text, _ := res.JSON[0]["text"].(string)
	if !strings.Contains(text, "<tr><td>Widget</td><td></td><td>12</td></tr>") {
		t.Errorf("empty tab-separated field not kept: %q", text)
	}
}
