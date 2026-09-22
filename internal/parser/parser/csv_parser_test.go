package parser

import (
	"context"
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

func TestCSVParserHTML4ExcelRemainsAtomic(t *testing.T) {
	p := NewCSVParser()
	p.ConfigureFromSetup(map[string]any{"html4excel": true})
	res := p.ParseWithResult(context.Background(), "qa.csv", []byte("Question,Answer\nQ1,A1\n"))
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}
	if len(res.JSON) != 1 || res.JSON[0]["ck_type"] != "table" {
		t.Fatalf("items = %#v, want one table item", res.JSON)
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
