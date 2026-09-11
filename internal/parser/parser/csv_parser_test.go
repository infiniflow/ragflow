package parser

import (
	"context"
	"strings"
	"testing"
)

func TestCSVParser_BasicHTMLAndJSON(t *testing.T) {
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

	if len(res.JSON) != 1 {
		t.Fatalf("len(res.JSON) = %d, want 1", len(res.JSON))
	}

	item := res.JSON[0]
	if text, ok := item["text"].(string); !ok || !strings.Contains(text, "<table>") {
		t.Fatal("item text should contain rendered <table>")
	}
	if item["doc_type_kwd"] != "table" {
		t.Errorf("item doc_type_kwd = %v, want 'table'", item["doc_type_kwd"])
	}
	if item["ck_type"] != "table" {
		t.Errorf("item ck_type = %v, want 'table'", item["ck_type"])
	}
	if item["sheet"] != "Data" {
		t.Errorf("item sheet = %v, want 'Data'", item["sheet"])
	}
	positions, ok := item["positions"].([][]float64)
	if !ok || len(positions) == 0 {
		t.Fatalf("item positions invalid: %v", item["positions"])
	}
	pos := positions[0]
	// [sheet, rowStart, rowEnd, colStart, colEnd]
	if len(pos) != 5 || pos[0] != 1 || pos[1] != 2 || pos[2] != 3 || pos[3] != 1 || pos[4] != 3 {
		t.Errorf("unexpected positions: %v", pos)
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
		t.Fatalf("len(res.JSON) = %d, want 1", len(res.JSON))
	}
	if text, ok := res.JSON[0]["text"].(string); !ok || !strings.Contains(text, "<table>") {
		t.Fatal("item text should contain rendered <table>")
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
	if len(res.JSON) != 1 {
		t.Fatalf("len(res.JSON) = %d, want 1", len(res.JSON))
	}
	item := res.JSON[0]
	if item["doc_type_kwd"] != "table" {
		t.Errorf("item doc_type_kwd = %v, want 'table'", item["doc_type_kwd"])
	}
}

func TestCSVParser_Chunking(t *testing.T) {
	// Header + 4 data rows, chunk size = 2
	csvData := []byte("Header\nr1\nr2\nr3\nr4\n")
	p := NewCSVParser()
	p.ChunkRows = 2

	res := p.ParseWithResult(context.Background(), "chunked.csv", csvData)
	if res.Err != nil {
		t.Fatalf("ParseWithResult failed: %v", res.Err)
	}

	// 4 data rows with chunk_rows=2 => 2 chunks
	if len(res.JSON) != 2 {
		t.Fatalf("len(res.JSON) = %d, want 2", len(res.JSON))
	}

	pos1 := res.JSON[0]["positions"].([][]float64)[0]
	pos2 := res.JSON[1]["positions"].([][]float64)[0]

	// chunk 1: rows 2..3
	if pos1[1] != 2 || pos1[2] != 3 {
		t.Errorf("chunk 1 row range: [%v, %v], want [2, 3]", pos1[1], pos1[2])
	}
	// chunk 2: rows 4..5
	if pos2[1] != 4 || pos2[2] != 5 {
		t.Errorf("chunk 2 row range: [%v, %v], want [4, 5]", pos2[1], pos2[2])
	}
}
