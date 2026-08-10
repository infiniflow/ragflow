package chunker

import (
	"context"
	"testing"
)

// custom_delim_test pins the backtick-wrapped newline delimiter behaviour of
// TokenChunker against Python's rag/flow/chunker/token_chunker.py.
//
// All delimiter paths (primary and children, text/markdown/html and json)
// must DROP the captured delimiter from each chunk's text, matching Python's
// _split_text_by_pattern (token_chunker.py:79-90, used by both _build_json_chunks
// and _split_chunk_docs_by_children). Go's splitDroppingDelim reproduces this.
//
// The json primary path is the one most prone to regress: Python's
// _build_json_chunks (token_chunker.py:121) splits each item through
// _split_text_by_pattern, which keeps only the even-index (text) parts and
// DISCARDS the captured delimiter. So a "first segment line one\n" item yields
// "first segment line one" with the newline dropped. Go's chunkFromItem does
// the same via splitDroppingDelim, matching the Python reference.
//
// doc_type_kwd is intentionally asserted to remain present on every chunk.
// It is a load-bearing Go field (index column + media dispatch) and is NOT
// part of the divergence; it is classified go_intentional in known_diffs.json,
// not a bug to fix here.

const backtickNewline = "`\n`"

func invokeTokenChunks(t *testing.T, params, input map[string]any) []map[string]any {
	t.Helper()
	c, err := NewTokenChunker(params)
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, input)
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if msg, ok := out["_ERROR"].(string); ok && msg != "" {
		t.Fatalf("Go returned _ERROR: %s", msg)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	return chunks
}

func chunkTexts(chunks []map[string]any) []string {
	out := make([]string, 0, len(chunks))
	for _, c := range chunks {
		if s, ok := c["text"].(string); ok {
			out = append(out, s)
		}
	}
	return out
}

// TestCustomDelimTextDropsDelimiter reproduces token__text_backtick.
func TestCustomDelimTextDropsDelimiter(t *testing.T) {
	params := map[string]any{"chunk_token_size": float64(128), "delimiters": []string{backtickNewline}}
	input := map[string]any{
		"name": "t", "output_format": "text",
		"text": "first sentence here\nsecond sentence here\nthird sentence here",
	}
	chunks := invokeTokenChunks(t, params, input)

	want := []string{"first sentence here", "second sentence here", "third sentence here"}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count: want %d got %d (%v)", len(want), len(chunks), chunkTexts(chunks))
	}
	for i, w := range want {
		got := chunks[i]["text"].(string)
		if got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
		if kd, _ := chunks[i]["doc_type_kwd"].(string); kd != "text" {
			t.Errorf("chunk[%d] doc_type_kwd: Go must keep it, want %q got %q", i, "text", kd)
		}
	}
}

// TestCustomDelimJSONDropsDelimiter reproduces token__json_backtick: the
// primary (custom backtick) delimiter is dropped from every chunk text,
// matching Python's _split_text_by_pattern.
func TestCustomDelimJSONDropsDelimiter(t *testing.T) {
	params := map[string]any{"chunk_token_size": float64(128), "delimiters": []string{backtickNewline}}
	input := map[string]any{
		"name": "t", "output_format": "json",
		"json": []map[string]any{
			{"text": "first segment line one\nfirst segment line two", "doc_type_kwd": "text"},
			{"text": "second segment line one\nsecond segment line two", "doc_type_kwd": "text"},
		},
	}
	chunks := invokeTokenChunks(t, params, input)

	want := []string{
		"first segment line one", "first segment line two",
		"second segment line one", "second segment line two",
	}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count: want %d got %d (%v)", len(want), len(chunks), chunkTexts(chunks))
	}
	for i, w := range want {
		got := chunks[i]["text"].(string)
		if got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
		if kd, _ := chunks[i]["doc_type_kwd"].(string); kd != "text" {
			t.Errorf("chunk[%d] doc_type_kwd: Go must keep it, want %q got %q", i, "text", kd)
		}
	}
}

// TestCustomDelimMarkdownDropsDelimiter reproduces token__markdown_backtick:
// the delimiter is dropped and no chunk text ends with a newline.
func TestCustomDelimMarkdownDropsDelimiter(t *testing.T) {
	params := map[string]any{"chunk_token_size": float64(128), "delimiters": []string{backtickNewline}}
	input := map[string]any{
		"name": "t", "output_format": "markdown",
		"markdown": "# Title\n\nParagraph one.\n\nParagraph two.",
	}
	chunks := invokeTokenChunks(t, params, input)
	// The upstream decode normalizes markdown block boundaries into the
	// backtick-newline delimiter, so the text path must split into exactly
	// three trimmed chunks with the delimiter dropped (no trailing newline).
	want := []string{"# Title", "Paragraph one.", "Paragraph two."}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count: want %d got %d (%v)", len(want), len(chunks), chunkTexts(chunks))
	}
	for i, w := range want {
		got := chunks[i]["text"].(string)
		if got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
		if kd, _ := chunks[i]["doc_type_kwd"].(string); kd != "text" {
			t.Errorf("chunk[%d] doc_type_kwd: Go must keep it, want %q got %q", i, "text", kd)
		}
	}
}

// TestCustomDelimHTMLDropsDelimiter reproduces token__html_backtick.
func TestCustomDelimHTMLDropsDelimiter(t *testing.T) {
	params := map[string]any{"chunk_token_size": float64(128), "delimiters": []string{backtickNewline}}
	input := map[string]any{
		"name": "t", "output_format": "html",
		"html": "<p>one</p>\n<p>two</p>\n<p>three</p>",
	}
	chunks := invokeTokenChunks(t, params, input)
	want := []string{"<p>one</p>", "<p>two</p>", "<p>three</p>"}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count: want %d got %d (%v)", len(want), len(chunks), chunkTexts(chunks))
	}
	for i, w := range want {
		got := chunks[i]["text"].(string)
		if got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
		if kd, _ := chunks[i]["doc_type_kwd"].(string); kd != "text" {
			t.Errorf("chunk[%d] doc_type_kwd: Go must keep it, want %q got %q", i, "text", kd)
		}
	}
}
