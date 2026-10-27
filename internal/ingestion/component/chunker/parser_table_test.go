package chunker

import (
	"archive/zip"
	"bytes"
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

func TestGeneralChunkerPreservesEPUBParagraphBoundaries(t *testing.T) {
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for name, content := range map[string]string{
		"META-INF/container.xml": `<container><rootfiles><rootfile full-path="OEBPS/content.opf"/></rootfiles></container>`,
		"OEBPS/content.opf":      `<package><manifest><item id="a" href="chapter.xhtml"/></manifest><spine><itemref idref="a"/></spine></package>`,
		"OEBPS/chapter.xhtml":    `<html><body><p>Alpha</p><p>Beta</p><table><tr><td>Item</td><td>Value</td></tr></table><p>Gamma</p><p>Delta</p></body></html>`,
	} {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	parsed := parser.NewEPUBParser().ParseWithResult(t.Context(), "book.epub", data.Bytes())
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	component, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512})
	if err != nil {
		t.Fatal(err)
	}
	out, err := component.Invoke(t.Context(), nil, map[string]any{"name": "book.epub", "output_format": "json", "json": parsed.JSON})
	if err != nil {
		t.Fatal(err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 3 {
		t.Fatalf("chunks=%#v", chunks)
	}
	before, _ := chunks[0]["text"].(string)
	after, _ := chunks[2]["text"].(string)
	if before != "Alpha Beta" || chunks[1]["doc_type_kwd"] != "table" || after != "Gamma Delta" {
		t.Fatalf("paragraph or table boundaries lost: %#v", chunks)
	}
}
