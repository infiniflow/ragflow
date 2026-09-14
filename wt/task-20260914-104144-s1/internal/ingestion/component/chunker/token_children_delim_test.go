package chunker

import (
	"strings"
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
	out, err := c.Invoke(t.Context(), nil, map[string]any{
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
// children_delimiters split also DROPS the delimiter (applyChildrenDelimText
// mirrors _split_text_by_pattern), keeping the parent segment in "mom".
// TestTokenChunker_ChildrenDelimiterBacktickStripped asserts that a
// backtick-wrapped children_delimiter contributes its INNER content as the
// split pattern (not the literal wrapped token), and the matched delimiter is
// dropped from each child — consistent with the main delimiter list behavior.
func TestTokenChunker_ChildrenDelimiterBacktickStripped(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":      "delimiter",
		"delimiters":          []string{"\n"},
		"children_delimiters": []string{"`###`"},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "sec one###sec two###sec three",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	want := []string{"sec one", "sec two", "sec three"}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count: want %d got %d (%v)", len(want), len(chunks), chunkTexts(chunks))
	}
	for i, w := range want {
		got := chunks[i]["text"].(string)
		if got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
	}
	// The literal backtick token `###` must never match as a whole string.
	c2, _ := NewTokenChunker(map[string]any{
		"delimiter_mode":      "delimiter",
		"delimiters":          []string{"\n"},
		"children_delimiters": []string{"`###`"},
	})
	out2, err := c2.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "a `###` b",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks2, _ := out2["chunks"].([]map[string]any)
	for _, ck := range chunks2 {
		if strings.Contains(ck["text"].(string), "###") {
			t.Errorf("child text kept literal backtick-wrapped token: %q", ck["text"].(string))
		}
	}
}

func TestTokenChunker_ChildrenDelimiterDroppedText(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":      "delimiter",
		"delimiters":          []string{"\n"},
		"children_delimiters": []string{". "},
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
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
