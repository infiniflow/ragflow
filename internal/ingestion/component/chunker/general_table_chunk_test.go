package chunker

import (
	"context"
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
)

// TestGeneralChunkerKeepsStructuredTable pins the regression where the
// GeneralChunker (selected by parser_id "general") dropped a parsed table
// chunk entirely.
//
// The production PDF parser projects a table item's HTML into a structured
// `table` field and deletes the redundant HTML `text` (see projectPDFTable in
// internal/parser/parser). A table chunk therefore arrives at the chunker with
// TableData populated and Text empty. The TokenChunker backfills Text from
// TableData (token.go:264) before the retention filter, so it survives; the
// GeneralChunker path did not, so finalizeGeneralChunks discarded the chunk
// (its Text, Image and ImgID were all empty) and the table vanished from the
// document.
//
// The fix mirrors TokenChunker: render the structured TableData into Text when
// Text is empty, so the table chunk carries retrievable/displayable content and
// is no longer dropped.
func TestGeneralChunkerKeepsStructuredTable(t *testing.T) {
	td := &entity.TableData{
		Rows: [][]string{
			{"序号", "学校名称", "专业代码"},
			{"1", "南京大学", "020109T"},
			{"2", "苏州大学", "020109T"},
		},
		HeaderRows: 1,
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
	text, _ := tableOut["text"].(string)
	if strings.TrimSpace(text) == "" {
		t.Fatalf("table chunk has empty Text; TableData must render into Text: %#v", tableOut)
	}
	if _, ok := tableOut["table"]; !ok {
		t.Fatalf("table chunk lost its structured `table` field: %#v", tableOut)
	}
	if !strings.Contains(text, "南京大学") {
		t.Fatalf("table chunk Text missing cell content: %q", text)
	}
}
