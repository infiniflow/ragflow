package chunker

import (
	"context"
	"strings"
	"testing"
)

// custom_delim_test pins the backtick-wrapped newline delimiter behaviour of
// TokenChunker against Python's rag/flow/chunker/token_chunker.py.
//
// There are two distinct, path-specific divergences that this file locks down:
//
//  1. text/markdown/html path: Python's _split_text_by_pattern (token_chunker.py:79-90)
//     uses re.split with a captured group and keeps only the even-index
//     (text) parts, so the delimiter is DROPPED. Go's splitKeepingDelim glues
//     the delimiter to the end of the preceding segment, so a chunk reads
//     "first sentence here\n" instead of "first sentence here". The fix makes
//     the text path drop the delimiter like Python.
//
//  2. json path: Python's _build_json_chunks + _finalize_json_chunks never
//     strip the chunk text, so the delimiter's trailing newline survives
//     ("first segment line one\n"). Go's invokeJSONPayload ran every chunk
//     through strings.TrimSpace, which deleted that newline. The fix stops
//     trimming, so the newline is kept like Python.
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

// TestCustomDelimJSONKeepsNewline reproduces token__json_backtick.
func TestCustomDelimJSONKeepsNewline(t *testing.T) {
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
		"first segment line one\n", "first segment line two",
		"second segment line one\n", "second segment line two",
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
	if len(chunks) == 0 {
		t.Fatal("want >=1 chunk, got 0")
	}
	for i, c := range chunks {
		got := c["text"].(string)
		if strings.HasSuffix(got, "\n") {
			t.Errorf("chunk[%d] text ends with delimiter newline: %q", i, got)
		}
		if kd, _ := c["doc_type_kwd"].(string); kd != "text" {
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
