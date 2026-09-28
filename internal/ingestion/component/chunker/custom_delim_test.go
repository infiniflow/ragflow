package chunker

import (
	"context"
	"slices"
	"testing"
)

// custom_delim_test pins the backtick-wrapped (custom) delimiter behavior of
// TokenChunker: a backtick delimiter is an explicit split instruction, not text
// to preserve, so it is DROPPED from the chunk text (mirroring Python's
// _split_text_by_pattern). Bare (non-backtick) delimiters are retained
// losslessly.
//
// Plain text/markdown/html inputs must not gain doc_type_kwd merely because a
// custom delimiter is configured: Python emits only text for those paths.
// Structured JSON inputs keep their source doc_type_kwd, while ck_type remains
// on Go text chunks for downstream crop dispatch.

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

// assertPlainCustomDelimiterMetadata checks the metadata contract shared by
// plain text, Markdown, and HTML custom-delimiter paths.
func assertPlainCustomDelimiterMetadata(t *testing.T, chunks []map[string]any) {
	t.Helper()
	for i, chunk := range chunks {
		if _, ok := chunk["doc_type_kwd"]; ok {
			t.Errorf("chunk[%d] must not synthesize doc_type_kwd: %v", i, chunk)
		}
		if got := chunk["ck_type"]; got != "text" {
			t.Errorf("chunk[%d] ck_type: want %q got %v", i, "text", got)
		}
	}
}

// TestCustomDelimTextChildrenKeepCKType covers primary and child splitting in
// combination: child chunks remain unstructured but retain Go's live CKType,
// and every delimiter (primary "::" and children "--") is kept.
func TestCustomDelimTextChildrenKeepCKType(t *testing.T) {
	params := map[string]any{
		"chunk_token_size":    float64(128),
		"delimiters":          []string{"`::`"},
		"children_delimiters": []string{"--"},
	}
	input := map[string]any{
		"name": "t", "output_format": "text",
		"text": "alpha--beta::gamma--delta",
	}
	chunks := invokeTokenChunks(t, params, input)

	// The custom primary ("`::`") delimiter is DROPPED, so "::" vanishes from
	// the joined text; the bare children delimiter "--" is RETAINED (lossless).
	if got := joinedText(chunks); got != "alpha--betagamma--delta" {
		t.Fatalf("joined text failed: got %q", got)
	}
	want := []string{"alpha--", "beta", "gamma--", "delta"}
	if got := chunkTexts(chunks); !slices.Equal(got, want) {
		t.Fatalf("chunk texts: want %v got %v", want, got)
	}
	assertPlainCustomDelimiterMetadata(t, chunks)
}

// TestCustomDelimTextKeepsDelimiter reproduces token__text_backtick under the
// lossless contract: the backtick-newline delimiter is retained at the end of
// every segment.
func TestCustomDelimTextDropsDelimiter(t *testing.T) {
	params := map[string]any{"chunk_token_size": float64(128), "delimiters": []string{backtickNewline}}
	input := map[string]any{
		"name": "t", "output_format": "text",
		"text": "first sentence here\nsecond sentence here\nthird sentence here",
	}
	chunks := invokeTokenChunks(t, params, input)

	// The custom ("`\n`") delimiter is DROPPED, so each segment loses its
	// trailing newline.
	want := []string{"first sentence here", "second sentence here", "third sentence here"}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count: want %d got %d (%v)", len(want), len(chunks), chunkTexts(chunks))
	}
	for i, w := range want {
		got := chunks[i]["text"].(string)
		if got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
	}
	assertPlainCustomDelimiterMetadata(t, chunks)
}

// TestCustomDelimJSONKeepsDelimiter reproduces token__json_backtick under the
// lossless contract: the primary (custom backtick) delimiter is kept on every
// chunk text, and the joined output reproduces the source exactly.
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

	// The custom ("`\n`") delimiter is DROPPED (Python-compatible split
	// instruction), so the joined output is the bare segment texts with no
	// inter-segment newline.
	if got := joinedText(chunks); got != "first segment line onefirst segment line twosecond segment line onesecond segment line two" {
		t.Fatalf("joined text failed: got %q", got)
	}
	for i, w := 0, []string{
		"first segment line one", "first segment line two",
		"second segment line one", "second segment line two",
	}; i < len(w); i++ {
		got := chunks[i]["text"].(string)
		if got != w[i] {
			t.Errorf("chunk[%d] text: want %q got %q", i, w[i], got)
		}
	}
	for i, chunk := range chunks {
		if kd, _ := chunk["doc_type_kwd"].(string); kd != "text" {
			t.Errorf("chunk[%d] doc_type_kwd: structured JSON must keep it, want %q got %q", i, "text", kd)
		}
	}
}

// TestCustomDelimMarkdownKeepsDelimiter reproduces token__markdown_backtick
// under the lossless contract: the markdown text is split on the
// backtick-newline delimiter and every chunk keeps its delimiter, so the joined
// output reproduces the markdown source exactly.
func TestCustomDelimMarkdownDropsDelimiter(t *testing.T) {
	params := map[string]any{"chunk_token_size": float64(128), "delimiters": []string{backtickNewline}}
	input := map[string]any{
		"name": "t", "output_format": "markdown",
		"markdown": "# Title\n\nParagraph one.\n\nParagraph two.",
	}
	chunks := invokeTokenChunks(t, params, input)
	// The custom ("`\n`") delimiter is DROPPED, so the paragraphs are joined
	// with the newline separator removed.
	if got := joinedText(chunks); got != "# TitleParagraph one.Paragraph two." {
		t.Fatalf("joined text failed: got %q", got)
	}
	assertPlainCustomDelimiterMetadata(t, chunks)
}

// TestCustomDelimHTMLKeepsDelimiter reproduces token__html_backtick under the
// lossless contract.
func TestCustomDelimHTMLDropsDelimiter(t *testing.T) {
	params := map[string]any{"chunk_token_size": float64(128), "delimiters": []string{backtickNewline}}
	input := map[string]any{
		"name": "t", "output_format": "html",
		"html": "<p>one</p>\n<p>two</p>\n<p>three</p>",
	}
	chunks := invokeTokenChunks(t, params, input)
	// The custom ("`\n`") delimiter is DROPPED, so the <p> blocks are joined
	// with the newline separator removed.
	if got := joinedText(chunks); got != "<p>one</p><p>two</p><p>three</p>" {
		t.Fatalf("joined text failed: got %q", got)
	}
	assertPlainCustomDelimiterMetadata(t, chunks)
}
