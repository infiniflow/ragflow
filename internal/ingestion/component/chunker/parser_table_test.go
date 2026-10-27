package chunker

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ragflow/internal/parser/parser"
)

func TestGeneralChunkerPreservesRetrievalHandbookTables(t *testing.T) {
	data, err := os.ReadFile("testdata/tables/retrieval_md_20_ret.md")
	if err != nil {
		t.Fatal(err)
	}
	p, err := parser.NewMarkdownParser(parser.GoMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	parsed := p.ParseWithResult(t.Context(), "retrieval_md_20_ret.md", data)
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	wire, err := json.Marshal(parsed.JSON)
	if err != nil {
		t.Fatal(err)
	}
	var items []map[string]any
	if err := json.Unmarshal(wire, &items); err != nil {
		t.Fatal(err)
	}
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512})
	if err != nil {
		t.Fatal(err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{
		"name": "retrieval_md_20_ret.md", "file_type": "md", "output_format": "json", "json": items,
	})
	if err != nil {
		t.Fatal(err)
	}
	tables := 0
	metadataCopies := 0
	for _, chunk := range outputChunks(t, out) {
		text, _ := chunk["text"].(string)
		if chunk["doc_type_kwd"] == "table" {
			tables++
		}
		if strings.Contains(text, "Document ID") {
			metadataCopies++
			if chunk["doc_type_kwd"] != "table" {
				t.Fatalf("metadata flattened: %#v", chunk)
			}
			for _, row := range []string{
				"<td>Document ID</td><td>retrieval_md_20_ret.md</td>",
				"<td>Component code</td><td>RET</td>",
				"<td>Owner</td><td>Platform Reliability (ret-team)</td>",
				"<td>Last reviewed</td><td>2026-09-10</td>",
				"<td>Review cycle</td><td>every 90 days</td>",
			} {
				if !strings.Contains(text, row) {
					t.Errorf("metadata missing row %q: %s", row, text)
				}
			}
		}
	}
	if tables != 4 || metadataCopies != 1 {
		t.Fatalf("tables=%d metadata copies=%d, want 4 and 1", tables, metadataCopies)
	}
}
