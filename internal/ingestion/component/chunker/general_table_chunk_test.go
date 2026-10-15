package chunker

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

// TestGeneralChunkerPDFTableRendersAsHTML pins the regression where the
// GeneralChunker (parser_id "general") dropped a parsed PDF table chunk
// entirely.
//
// The production PDF parser projects a table item's HTML into a structured
// TableData (Grid) and deletes the redundant HTML `text` (see projectPDFTable
// in internal/parser/parser). A table chunk therefore arrives at the chunker
// with TableData populated and Text empty. The TokenChunker and the
// spreadsheet path (splitSpreadsheetTable, #20918) both render the table into
// Text and drop TableData; the GeneralChunker PDF path (chunkPDF) did not, so
// finalizeGeneralChunks discarded the chunk (Text, Image and ImgID all empty)
// and the table vanished from the document.
//
// The fix mirrors splitSpreadsheetTable (#20918): when a chunk has empty Text
// but a populated TableData, render the table as HTML into Text and drop
// TableData, so the chunk is retained and displayable and stays byte-consistent
// with the spreadsheet path.
func TestGeneralChunkerPDFTableRendersAsHTML(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "序号"}, {Text: "学校名称"}, {Text: "专业代码"}},
			{{Text: "1"}, {Text: "南京大学"}, {Text: "020109T"}},
			{{Text: "2"}, {Text: "苏州大学"}, {Text: "020109T"}},
		},
	}
	units := []schema.ChunkDoc{
		{
			DocType:      "text",
			Text:         "附件",
			PDFPositions: []byte(`[[1,87,124,79,99]]`),
		},
		{
			// Mirrors the parser output: structured table, HTML text deleted.
			DocType:      "table",
			TableData:    td,
			Text:         "",
			PDFPositions: []byte(`[[1,10,200,20,300]]`),
		},
	}

	c := &GeneralChunkerComponent{param: generalChunkerParam{}.Defaults()}
	up := schema.ChunkerFromUpstream{
		JSONResult: units,
		File:       &schema.ChunkerFileMeta{Name: "doc.pdf"},
	}
	out, err := c.chunkPDF(context.Background(), nil, up)
	if err != nil {
		t.Fatalf("chunkPDF: %v", err)
	}

	outChunks, _ := out["chunks"].([]map[string]any)
	var tableOut map[string]any
	for _, m := range outChunks {
		if dt, _ := m["doc_type_kwd"].(string); dt == "table" {
			tableOut = m
		}
	}
	if tableOut == nil {
		t.Fatalf("structured table chunk was dropped by GeneralChunker; output chunks=%v", outChunks)
	}
	if dt, _ := tableOut["doc_type_kwd"].(string); dt != "table" {
		t.Fatalf("table chunk doc_type_kwd = %q, want table", dt)
	}
	text, _ := tableOut["text"].(string)
	if strings.TrimSpace(text) == "" {
		t.Fatalf("table chunk has empty Text; TableData must render into Text: %#v", tableOut)
	}
	if !strings.Contains(text, "南京大学") {
		t.Fatalf("table chunk Text missing cell content: %q", text)
	}
	if !strings.Contains(text, "<td") || !strings.Contains(text, "<table") {
		t.Fatalf("table chunk Text is not rendered HTML: %q", text)
	}
	// Aligned with #20918: the structured TableData is dropped, never indexed.
	if tbl, ok := tableOut["table"]; ok && tbl != nil {
		t.Fatalf("aligned behavior drops TableData; table field must be absent, got %#v", tableOut)
	}
}

// TestGeneralChunkerPDFTableSingleton: a lone table chunk (no surrounding text)
// must survive as HTML.
func TestGeneralChunkerPDFTableSingleton(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "k"}, {Text: "v"}},
			{{Text: "a"}, {Text: "1"}},
		},
	}
	units := []schema.ChunkDoc{
		{DocType: "table", TableData: td, Text: "", PDFPositions: []byte(`[[1,1,2,3,4]]`)},
	}
	c := &GeneralChunkerComponent{param: generalChunkerParam{}.Defaults()}
	out, err := c.chunkPDF(context.Background(), nil, schema.ChunkerFromUpstream{
		JSONResult: units, File: &schema.ChunkerFileMeta{Name: "doc.pdf"},
	})
	if err != nil {
		t.Fatalf("chunkPDF: %v", err)
	}
	outChunks, _ := out["chunks"].([]map[string]any)
	if len(outChunks) != 1 {
		t.Fatalf("expected 1 retained chunk, got %d: %v", len(outChunks), outChunks)
	}
	text, _ := outChunks[0]["text"].(string)
	if !strings.Contains(text, "<table") {
		t.Fatalf("singleton table not rendered as HTML: %q", text)
	}
}

// TestGeneralChunkerPDFTableInterleaved: multiple tables interleaved with text
// are all retained, in order, and none is dropped.
func TestGeneralChunkerPDFTableInterleaved(t *testing.T) {
	td := func() *entity.TableData {
		return &entity.TableData{
			HeaderRows: 1,
			Grid: [][]entity.TableCell{
				{{Text: "c1"}, {Text: "c2"}},
				{{Text: "x"}, {Text: "y"}},
			},
		}
	}
	units := []schema.ChunkDoc{
		{DocType: "text", Text: "intro", PDFPositions: []byte(`[[1,1,2,3,4]]`)},
		{DocType: "table", TableData: td(), Text: "", PDFPositions: []byte(`[[2,1,2,3,4]]`)},
		{DocType: "text", Text: "middle", PDFPositions: []byte(`[[3,1,2,3,4]]`)},
		{DocType: "table", TableData: td(), Text: "", PDFPositions: []byte(`[[4,1,2,3,4]]`)},
	}
	c := &GeneralChunkerComponent{param: generalChunkerParam{}.Defaults()}
	out, err := c.chunkPDF(context.Background(), nil, schema.ChunkerFromUpstream{
		JSONResult: units, File: &schema.ChunkerFileMeta{Name: "doc.pdf"},
	})
	if err != nil {
		t.Fatalf("chunkPDF: %v", err)
	}
	outChunks, _ := out["chunks"].([]map[string]any)
	var tables int
	for _, m := range outChunks {
		if dt, _ := m["doc_type_kwd"].(string); dt == "table" {
			tables++
			text, _ := m["text"].(string)
			if !strings.Contains(text, "<table") {
				t.Fatalf("interleaved table not rendered as HTML: %q", text)
			}
		}
	}
	if tables != 2 {
		t.Fatalf("expected 2 retained tables, got %d", tables)
	}
}

// TestGeneralChunkerPDFTableWithExistingText: a table unit that already carries
// non-empty Text must keep its Text and keep TableData — we only act when Text
// is empty.
func TestGeneralChunkerPDFTableWithExistingText(t *testing.T) {
	td := &entity.TableData{
		HeaderRows: 1,
		Grid: [][]entity.TableCell{
			{{Text: "h"}, {Text: "k"}},
			{{Text: "1"}, {Text: "2"}},
		},
	}
	units := []schema.ChunkDoc{
		{
			DocType:      "table",
			TableData:    td,
			Text:         "<existing>markup</existing>",
			PDFPositions: []byte(`[[1,1,2,3,4]]`),
		},
	}
	c := &GeneralChunkerComponent{param: generalChunkerParam{}.Defaults()}
	out, err := c.chunkPDF(context.Background(), nil, schema.ChunkerFromUpstream{
		JSONResult: units, File: &schema.ChunkerFileMeta{Name: "doc.pdf"},
	})
	if err != nil {
		t.Fatalf("chunkPDF: %v", err)
	}
	outChunks, _ := out["chunks"].([]map[string]any)
	if len(outChunks) != 1 {
		t.Fatalf("expected 1 chunk, got %d", len(outChunks))
	}
	text, _ := outChunks[0]["text"].(string)
	if text != "<existing>markup</existing>" {
		t.Fatalf("expected Text preserved, got %q", text)
	}
	if _, ok := outChunks[0]["table"]; !ok {
		t.Fatalf("existing-Text table must keep TableData; table field absent: %#v", outChunks[0])
	}
}

// TestGeneralChunkerPDFTableEmptyUnits: degenerate input must not panic.
func TestGeneralChunkerPDFTableEmptyUnits(t *testing.T) {
	c := &GeneralChunkerComponent{param: generalChunkerParam{}.Defaults()}
	out, err := c.chunkPDF(context.Background(), nil, schema.ChunkerFromUpstream{
		JSONResult: []schema.ChunkDoc{}, File: &schema.ChunkerFileMeta{Name: "doc.pdf"},
	})
	if err != nil {
		t.Fatalf("chunkPDF: %v", err)
	}
	if out == nil {
		t.Fatalf("expected non-nil output map")
	}
}
