package chunker

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"ragflow/internal/ingestion/component/schema"
)

func TestTokenChunkerCustomDelimiterAcrossParserItems(t *testing.T) {
	var paragraphs, sections []string
	paragraph := 0
	for section := range 9 {
		var lines []string
		count := 2
		if section < 3 {
			count++
		}
		for range count {
			paragraph++
			lines = append(lines, fmt.Sprintf("Paragraph %d.", paragraph))
		}
		sections = append(sections, strings.Join(lines, "\n"))
		paragraphs = append(paragraphs, lines...)
		if section < 8 {
			paragraphs = append(paragraphs, "#")
		}
	}
	c, err := NewTokenChunker(map[string]any{"delimiters": []string{"`#`"}, "chunk_token_size": 512})
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"docx", "txt"} {
		t.Run(format, func(t *testing.T) {
			items := make([]map[string]any, 0, len(paragraphs))
			for _, text := range paragraphs {
				items = append(items, map[string]any{"text": text, "doc_type_kwd": "text"})
			}
			if format == "txt" {
				items = []map[string]any{{"text": strings.Join(paragraphs, "\n"), "doc_type_kwd": "text"}}
			}
			out, err := c.Invoke(t.Context(), nil, map[string]any{"name": "document." + format, "output_format": "json", "json": items})
			if err != nil {
				t.Fatal(err)
			}
			chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
			got := chunkTexts(chunks)
			for i := range got {
				got[i] = strings.TrimSpace(got[i])
			}
			if !reflect.DeepEqual(got, sections) {
				t.Fatalf("chunks = %q, want %q", got, sections)
			}
		})
	}
}

func TestTokenChunkerCustomDelimiterItemsPreserveMediaAndPositions(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{"delimiters": []string{"`#`"}})
	if err != nil {
		t.Fatal(err)
	}
	items := []schema.ChunkDoc{
		{Text: "first", DocType: "text", Mom: "section", PDFPositions: json.RawMessage(`[[1,0,10,0,10]]`)},
		{Text: "second#third", DocType: "text", PDFPositions: json.RawMessage(`[[2,0,10,0,20]]`)},
		{Text: "image", DocType: "image", ImgID: "image-id"},
		{Text: "fourth", DocType: "text"},
		{Text: "table", DocType: "table"},
		{Text: "fifth", DocType: "text"},
		{Text: "#", DocType: "text"},
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"name": "document.docx", "output_format": "json", "json": schema.ChunkDocsToMaps(items)})
	if err != nil {
		t.Fatal(err)
	}
	chunks, _ := schema.ChunkCarrierToMaps(out["chunks"])
	want := []string{"first\nsecond", "third", "image", "fourth", "table", "fifth"}
	if got := chunkTexts(chunks); !reflect.DeepEqual(got, want) {
		t.Fatalf("chunks = %q, want %q", got, want)
	}
	if chunks[0]["mom"] != "section" || chunks[2]["img_id"] != "image-id" {
		t.Fatalf("metadata lost: %#v", chunks)
	}
	decoded, _, err := schema.ChunkDocsFromAny(chunks)
	if err != nil {
		t.Fatal(err)
	}
	first := matrixOfRaw(t, decoded[0].PDFPositions)
	second := matrixOfRaw(t, decoded[1].PDFPositions)
	if len(first) != 2 || len(second) != 1 || first[0][0] != 1 || first[1][0] != 2 || second[0][0] != 2 || first[1][4] != second[0][3] {
		t.Fatalf("source positions lost or duplicated: %v / %v", first, second)
	}
}

func TestTokenChunkerCustomDelimiterOnlyItem(t *testing.T) {
	if got := chunkFromItem(schema.ChunkDoc{Text: " # # ", DocType: "text"}, compileDelimPattern([]string{"`#`"}), false); len(got) != 0 {
		t.Fatalf("delimiter-only item produced chunks: %#v", got)
	}
}

func TestTokenChunkerEmptyCustomDelimiter(t *testing.T) {
	chunks := invokeTokenChunks(t, map[string]any{"delimiters": []string{"``"}}, map[string]any{
		"name": "document.docx", "output_format": "json",
		"json": []map[string]any{
			{"text": "first", "doc_type_kwd": "text"},
			{"text": "second", "doc_type_kwd": "text"},
		},
	})
	if got, want := chunkTexts(chunks), []string{"first", "second"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("chunks = %q, want %q", got, want)
	}
}
