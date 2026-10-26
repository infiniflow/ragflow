package chunker

import (
	"strings"
	"testing"

	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
)

func TestPDFTableNotesDoNotHideCellsDuringChunking(t *testing.T) {
	result := parser.NewPDFParser().NormalizeResult(t.Context(), "sample.pdf", parser.ParseResult{JSON: []parser.DocItem{
		parser.TableItem{Table: &entity.TableData{Grid: [][]entity.TableCell{{{Text: "Cell value"}}}}, Extra: map[string]any{"text": "Footnote"}},
	}})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512})
	if err != nil {
		t.Fatal(err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{"name": "sample.pdf", "file_type": "pdf", "output_format": "json", "json": result.JSON})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for _, chunk := range out["chunks"].([]map[string]any) {
		value, _ := chunk["text"].(string)
		text += value
	}
	for _, want := range []string{"Cell value", "Footnote"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %q", want, text)
		}
	}
}
