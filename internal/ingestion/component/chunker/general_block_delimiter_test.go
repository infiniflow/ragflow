package chunker

import (
	"reflect"
	"testing"

	"ragflow/internal/parser/parser"
)

func TestGeneralChunkerPDFAndMarkdownPreserveBlockBoundaries(t *testing.T) {
	for _, fileType := range []string{"pdf", "md"} {
		for _, tc := range []struct {
			name, first, second string
		}{
			{"implicit", "第一段。还有一句。", "第二段。"},
			{"trailing CRLF", "第一段。还有一句。\r\n", "第二段。"},
			{"leading CR", "第一段。还有一句。", "\r第二段。"},
		} {
			t.Run(fileType+"/"+tc.name, func(t *testing.T) {
				c, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512, "delimiters": []string{"\n", "。"}})
				if err != nil {
					t.Fatal(err)
				}
				out, err := c.Invoke(t.Context(), nil, map[string]any{
					"name": "document." + fileType, "file_type": fileType, "output_format": "json",
					"json": []map[string]any{{"text": tc.first, "doc_type_kwd": "text"}, {"text": tc.second, "doc_type_kwd": "text"}},
				})
				if err != nil {
					t.Fatal(err)
				}
				if got, want := outputTexts(t, out), []string{"第一段。还有一句。\n第二段。"}; !reflect.DeepEqual(got, want) {
					t.Fatalf("got %q, want %q", got, want)
				}
			})
		}
	}
}

func TestGeneralChunkerPDFPunctuationPreservesMediaOrderAndContext(t *testing.T) {
	c, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 512, "delimiters": []string{"\n", "。"}, "table_context_size": 512,
	})
	if err != nil {
		t.Fatal(err)
	}
	position := func(top int) [][]int { return [][]int{{1, 0, 10, top, top + 5}} }
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"name": "document.pdf", "file_type": "pdf", "output_format": "json",
		"json": []map[string]any{
			{"text": "后文。还有一句。", "doc_type_kwd": "text", "positions": position(30)},
			{"text": "<table>A</table>", "doc_type_kwd": "table", "positions": position(20)},
			{"text": "前文。还有一句。", "doc_type_kwd": "text", "positions": position(10)},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 3 || chunks[0]["text"] != "前文。还有一句。" || chunks[2]["text"] != "后文。还有一句。" {
		t.Fatalf("chunks = %#v", chunks)
	}
	assertMaterializedMediaContext(t, chunks[1], "前文。还有一句。<table>A</table>后文。还有一句。")
	// The unit "前文。还有一句。" is delimiter-split into "前文。" (3 visible
	// runes) and "还有一句。" (5 visible runes) whose bboxes partition the
	// source [10,15] box proportionally (3:5), then merged back into one
	// chunk. The two rows cover exactly the source span — the Token path
	// likewise retains one row per merged item — so expect the partition
	// rather than a single full-span row.
	if got, want := chunks[0]["positions"], [][]float64{{1, 0, 10, 10, 11.875}, {1, 0, 10, 11.875, 15}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("positions = %#v, want %#v", got, want)
	}
}

func TestGeneralChunkerMarkdownParserPreservesPunctuationAndHeading(t *testing.T) {
	p, err := parser.NewMarkdownParser(parser.GoMarkdown)
	if err != nil {
		t.Fatal(err)
	}
	parsed := p.ParseWithResult(t.Context(), "document.md", []byte("# 标题。还有标题！\n\n第一句。第二句。\n\n下一段。"))
	if parsed.Err != nil {
		t.Fatal(parsed.Err)
	}
	c, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512, "delimiters": []string{"\n", "。", "！"}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{"name": "document.md", "file_type": "md", "output_format": "json", "json": parsed.JSON})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := outputTexts(t, out), []string{"# 标题。还有标题！\n第一句。第二句。\n下一段。"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestGeneralChunkerPDFAndMarkdownCustomDelimiterStillDropsMarker(t *testing.T) {
	for _, fileType := range []string{"pdf", "md"} {
		t.Run(fileType, func(t *testing.T) {
			c, err := NewGeneralChunker(map[string]any{"chunk_token_size": 512, "delimiters": []string{"`SEP`"}})
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), nil, map[string]any{
				"name": "document." + fileType, "file_type": fileType, "output_format": "json",
				"json": []map[string]any{{"text": "第一句。SEP第二句。", "doc_type_kwd": "text"}},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := outputTexts(t, out), []string{"第一句。\n第二句。"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestGeneralChunkerPDFAndMarkdownCustomMediaContextDropsMarker(t *testing.T) {
	for _, fileType := range []string{"pdf", "md"} {
		t.Run(fileType, func(t *testing.T) {
			c, err := NewGeneralChunker(map[string]any{
				"chunk_token_size": 512, "delimiters": []string{"`SEP`"}, "table_context_size": 512,
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), nil, map[string]any{
				"name": "document." + fileType, "file_type": fileType, "output_format": "json",
				"json": []map[string]any{
					{"text": "前文。SEP还有一句。", "doc_type_kwd": "text"},
					{"text": "<table>A</table>", "doc_type_kwd": "table"},
					{"text": "后文。", "doc_type_kwd": "text"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			chunks := outputChunks(t, out)
			if len(chunks) != 3 {
				t.Fatalf("chunks = %#v", chunks)
			}
			assertMaterializedMediaContext(t, chunks[1], "前文。\n还有一句。<table>A</table>后文。")
		})
	}
}

func TestGeneralChunkerMarkdownPunctuationPreservesMediaBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, docType, ckType, first, media string
		contextSize                         int
		want                                []string
	}{
		{"heading and table", "table", "heading", "标题。还有标题！", "<table>A</table>", 512,
			[]string{"标题。还有标题！\n<table>A</table>后文。还有一句。", "后文。还有一句。"}},
		{"image attachment", "image", "text", "前文。还有一句。", "图注。", 0,
			[]string{"前文。还有一句。\n图注。\n后文。还有一句。"}},
		{"standalone image", "image", "text", "前文。还有一句。", "图注。", 512,
			[]string{"前文。还有一句。", "前文。还有一句。图注。后文。还有一句。", "后文。还有一句。"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewGeneralChunker(map[string]any{
				"chunk_token_size": 512, "delimiters": []string{"\n", "。", "！"},
				"table_context_size": tc.contextSize, "image_context_size": tc.contextSize,
			})
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), nil, map[string]any{
				"name": "document.md", "file_type": "md", "output_format": "json",
				"json": []map[string]any{
					{"text": tc.first, "doc_type_kwd": "text", "ck_type": tc.ckType},
					{"text": tc.media, "doc_type_kwd": tc.docType, "img_id": "media-id"},
					{"text": "后文。还有一句。", "doc_type_kwd": "text"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got := outputTexts(t, out); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			foundMedia := false
			for _, chunk := range outputChunks(t, out) {
				foundMedia = foundMedia || chunk["img_id"] == "media-id"
			}
			if !foundMedia {
				t.Fatal("media identity was lost")
			}
		})
	}
}
