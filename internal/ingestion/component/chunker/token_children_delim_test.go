package chunker

import (
	"context"
	"testing"
)

// TestTokenChunker_ChildrenDelimiterDroppedJSON asserts that the JSON-path
// secondary children_delimiters split DROPS the delimiter from each child's
// text (matching Python's _split_chunk_docs_by_children /
// _split_text_by_pattern), while keeping the full source text in "mom".
func TestTokenChunker_ChildrenDelimiterDroppedJSON(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"children_delimiters": []string{"。"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.json",
		"output_format": "json",
		"json": []map[string]any{
			{"text": "第一句内容。第二句内容。第三句内容。", "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) != 3 {
		t.Fatalf("chunk count: want 3 got %d (%v)", len(chunks), chunkTexts(chunks))
	}
	want := []string{"第一句内容", "第二句内容", "第三句内容"}
	const mom = "第一句内容。第二句内容。第三句内容。"
	for i, w := range want {
		got := chunks[i]["text"].(string)
		if got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
		if m, _ := chunks[i]["mom"].(string); m != mom {
			t.Errorf("chunk[%d] mom: want %q got %q", i, mom, m)
		}
	}
}

// TestTokenChunker_ChildrenDelimiterDroppedText asserts the text/markdown/html
// children_delimiters split also DROPS the delimiter (applyChildrenDelim /
// applyChildrenDelimText mirror _split_text_by_pattern), keeping the parent
// segment in "mom".
func TestTokenChunker_ChildrenDelimiterDroppedText(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":      "delimiter",
		"delimiters":          []string{"\n"},
		"children_delimiters": []string{". "},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "alpha one. alpha two. alpha three.",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	want := []string{"alpha one", "alpha two", "alpha three."}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count: want %d got %d (%v)", len(want), len(chunks), chunkTexts(chunks))
	}
	for i, w := range want {
		got := chunks[i]["text"].(string)
		if got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
	}
}
