package parser

import (
	"context"
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

	if len(res.JSON) != 3 {
		t.Fatalf("len(res.JSON) = %d, want header plus two rows", len(res.JSON))
	}
	if res.JSON[0]["ck_type"] != "table_header" {
		t.Fatalf("header ck_type = %v, want table_header", res.JSON[0]["ck_type"])
	}
	if res.JSON[1]["ck_type"] != "table_row" {
		t.Fatalf("row ck_type = %v, want table_row", res.JSON[1]["ck_type"])
	}
	if res.JSON[1]["text"] != "Name：Alice; Age：30; City：New York ——Data" {
		t.Errorf("row text = %v", res.JSON[1]["text"])
	}
	if res.JSON[1]["sheet"] != "Data" || res.JSON[1]["sheet_index"] != 1 {
		t.Errorf("row sheet metadata = %#v", res.JSON[1])
	}
	positions, ok := res.JSON[1]["positions"].([][]float64)
	if !ok || len(positions) == 0 {
		t.Fatalf("item positions invalid: %v", res.JSON[1]["positions"])
	}
	pos := positions[0]
	// [sheet, rowStart, rowEnd, colStart, colEnd]
	if len(pos) != 5 || pos[0] != 1 || pos[1] != 2 || pos[2] != 2 || pos[3] != 1 || pos[4] != 3 {
		t.Errorf("unexpected positions: %v", pos)
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
	if len(res.JSON) != 2 {
		t.Fatalf("len(res.JSON) = %d, want header plus row", len(res.JSON))
	}
	if res.JSON[1]["ck_type"] != "table_row" {
		t.Fatalf("row ck_type = %v, want table_row", res.JSON[1]["ck_type"])
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

	if len(res.JSON) != 5 {
		t.Fatalf("len(res.JSON) = %d, want header plus four rows", len(res.JSON))
	}
	for i, item := range res.JSON[1:] {
		pos := item["positions"].([][]float64)[0]
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
	if len(res.JSON) != 3 {
		t.Fatalf("items = %d, want header plus two rows", len(res.JSON))
	}
	if got := len(res.JSON[1]["cells"].([]string)); got != 2 {
		t.Fatalf("first data row width = %d, want 2", got)
	}
	if got := len(res.JSON[2]["cells"].([]string)); got != 3 {
		t.Fatalf("second data row width = %d, want 3", got)
	}
}

func TestCSVParser_TSVSupport(t *testing.T) {
	tsvData := []byte("col1\tcol2\tcol3\nval1\tval2\tval3\n")
	p := NewCSVParser()
	p.OutputFormat = "json"
	p.ColumnMode = "manual"
	p.ColumnRoles = map[string]string{
		"col1": "indexing",
		"col2": "metadata",
		"col3": "both",
	}

	res := p.ParseWithResult(context.Background(), "test.tsv", tsvData)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(res.JSON))
	}
	chunk := res.JSON[0]
	text := chunk["text"].(string)
	if !strings.Contains(text, "col1: val1") || !strings.Contains(text, "col3: val3") {
		t.Errorf("text missing expected columns: %q", text)
	}
	if strings.Contains(text, "col2: val2") {
		t.Errorf("text should not contain metadata-only column col2: %q", text)
	}
	chunkData := chunk["chunk_data"].(map[string]any)
	if chunkData["col2"] != "val2" || chunkData["col3"] != "val3" {
		t.Errorf("chunk_data unexpected: %+v", chunkData)
	}
}
