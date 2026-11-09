package chunker

import (
	"strings"
	"testing"
)

func TestGeneralChunkerMediaContextUsesConfiguredDelimiters(t *testing.T) {
	for _, fileType := range []string{"pdf", "md"} {
		for _, tc := range []struct {
			name, delimiter, text, above, below string
		}{
			{"semicolon", ";", "first;middle;last;", "last;<table>A</table>", "<table>A</table>first;"},
			{"literal pipe", "|", "first|middle|last|", "last|<table>A</table>", "<table>A</table>first|"},
			{"multi-character", "。END", "first。ENDmiddle。ENDlast。END", "last。END<table>A</table>", "<table>A</table>first。END"},
		} {
			for _, direction := range []struct {
				name  string
				above bool
			}{{"above", true}, {"below", false}} {
				t.Run(fileType+"/"+tc.name+"/"+direction.name, func(t *testing.T) {
					c, err := NewGeneralChunker(map[string]any{
						"chunk_token_size": 512, "delimiters": []string{"\n", tc.delimiter}, "table_context_size": 1,
					})
					if err != nil {
						t.Fatal(err)
					}
					text := map[string]any{"text": tc.text, "doc_type_kwd": "text"}
					table := map[string]any{"text": "<table>A</table>", "doc_type_kwd": "table"}
					items := []map[string]any{table, text}
					tableIndex, want := 0, tc.below
					if direction.above {
						items = []map[string]any{text, table}
						tableIndex, want = 1, tc.above
					}
					out, err := c.Invoke(t.Context(), nil, map[string]any{
						"name": "test." + fileType, "file_type": fileType, "output_format": "json", "json": items,
					})
					if err != nil {
						t.Fatal(err)
					}
					chunks := outputChunks(t, out)
					if len(chunks) != 2 {
						t.Fatalf("chunks = %#v", chunks)
					}
					assertMaterializedMediaContext(t, chunks[tableIndex], want)
				})
			}
		}
	}
}

func TestGeneralChunkerMarkdownFoldedHeadingDropsPartialContext(t *testing.T) {
	c, err := NewGeneralChunker(map[string]any{
		"chunk_token_size": 512, "delimiters": []string{"\n", "。", "！"}, "table_context_size": 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"name": "test.md", "file_type": "md", "output_format": "json",
		"json": []map[string]any{
			{"text": "# 标题。还有标题！", "doc_type_kwd": "text", "layout": "heading"},
			{"text": "<table>A</table>", "doc_type_kwd": "table"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	chunks := outputChunks(t, out)
	if len(chunks) != 1 {
		t.Fatalf("chunks = %#v", chunks)
	}
	assertMaterializedMediaContext(t, chunks[0], "# 标题。还有标题！\n<table>A</table>")
}

func TestGeneralChunkerMarkdownLongHeadingUsesSourceClassification(t *testing.T) {
	for _, tc := range []struct {
		name, delimiter, marker string
		minChunks, maxWords     int
	}{
		{"bare", "!", "! ", 20, 3},
		{"custom", "`SEP`", "SEP", 12, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			heading := "# " + strings.Repeat("heading"+tc.marker, 60)
			c, err := NewGeneralChunker(map[string]any{"chunk_token_size": 5, "delimiters": []string{tc.delimiter}})
			if err != nil {
				t.Fatal(err)
			}
			out, err := c.Invoke(t.Context(), nil, map[string]any{
				"name": "test.md", "file_type": "md", "output_format": "json",
				"json": []map[string]any{
					{"text": heading, "doc_type_kwd": "text", "layout": "heading"},
					{"text": "body", "doc_type_kwd": "text"},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			texts := outputTexts(t, out)
			if len(texts) < tc.minChunks {
				t.Fatalf("long heading produced only %d chunks: %q", len(texts), texts)
			}
			wordCount := 0
			for _, text := range texts {
				wordCount += strings.Count(text, "heading")
				// Retained punctuation costs tokens; removed custom markers
				// leave one-token atoms, so more occurrences fit the budget.
				if strings.Count(text, "heading") > tc.maxWords {
					t.Fatalf("long heading received the short-heading merge exception: %q", text)
				}
			}
			if wordCount != 60 || !strings.HasSuffix(texts[len(texts)-1], "body") {
				t.Fatalf("heading or body content was lost: %q", texts)
			}
		})
	}
}
