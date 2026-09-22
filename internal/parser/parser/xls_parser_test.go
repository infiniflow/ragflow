package parser

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func createTestExcelBytes(t *testing.T) []byte {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()

	sheet := "Sheet1"
	_ = f.SetCellValue(sheet, "A1", "Header1")
	_ = f.SetCellValue(sheet, "B1", "Header2")
	_ = f.SetCellValue(sheet, "A2", "Val1")
	_ = f.SetCellValue(sheet, "B2", "Val2")

	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatalf("Write excel file failed: %v", err)
	}
	return buf.Bytes()
}

func TestXLSParser_SpreadsheetJSONOutput(t *testing.T) {
	data := createTestExcelBytes(t)
	p, err := NewXLSParser("excelize")
	if err != nil {
		t.Fatalf("NewXLSParser failed: %v", err)
	}

	res := p.ParseWithResult(context.Background(), "test.xls", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}

	// Default output format for XLSParser is "json"
	if res.OutputFormat != "json" {
		t.Errorf("res.OutputFormat = %q, want 'json'", res.OutputFormat)
	}
	if len(res.JSON) == 0 {
		t.Fatalf("res.JSON should have structured table items")
	}
	item := res.JSON[0]
	if item["ck_type"] != "table" {
		t.Errorf("table item ck_type = %v, want table", item["ck_type"])
	}
	text, _ := item["text"].(string)
	if !strings.Contains(text, "<tr><th>Header1</th><th>Header2</th></tr>") ||
		!strings.Contains(text, "<tr><td>Val1</td><td>Val2</td></tr>") {
		t.Errorf("table markup = %q, want captioned header and data row", text)
	}
	if item["doc_type_kwd"] != "table" {
		t.Errorf("doc_type_kwd = %v, want 'table'", item["doc_type_kwd"])
	}

	// Configure output format to "json"
	p.ConfigureFromSetup(map[string]any{
		"output_format": "json",
	})
	resJSON := p.ParseWithResult(context.Background(), "test.xls", data)
	if resJSON.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", resJSON.Err)
	}
	if resJSON.OutputFormat != "json" {
		t.Errorf("resJSON.OutputFormat = %q, want 'json'", resJSON.OutputFormat)
	}
	if len(resJSON.JSON) == 0 {
		t.Errorf("resJSON.JSON should not be empty")
	}
}

func TestXLSXParser_JSONOutput(t *testing.T) {
	data := createTestExcelBytes(t)
	p, err := NewXLSXParser("excelize")
	if err != nil {
		t.Fatalf("NewXLSXParser failed: %v", err)
	}

	// Default output format is "json"
	res := p.ParseWithResult(context.Background(), "test.xlsx", data)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}
	if res.OutputFormat != "json" {
		t.Errorf("res.OutputFormat = %q, want 'json'", res.OutputFormat)
	}
	if len(res.JSON) != 1 {
		t.Fatalf("res.JSON = %#v, want one segmented HTML table item", res.JSON)
	}
	if res.JSON[0]["ck_type"] != "table" {
		t.Errorf("item ck_type = %v, want table", res.JSON[0]["ck_type"])
	}

	// Even if configured to legacy "html", format is unified to "json"
	p.ConfigureFromSetup(map[string]any{
		"output_format": "html",
	})
	resHTML := p.ParseWithResult(context.Background(), "test.xlsx", data)
	if resHTML.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", resHTML.Err)
	}
	if resHTML.OutputFormat != "json" {
		t.Errorf("resHTML.OutputFormat = %q, want 'json'", resHTML.OutputFormat)
	}
	if len(resHTML.JSON) == 0 {
		t.Errorf("resHTML.JSON should have items")
	}
}
