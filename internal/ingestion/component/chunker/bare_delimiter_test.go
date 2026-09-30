package chunker

import (
	"context"
	"strings"
	"testing"
)

// bare_delimiter_test covers the fix for #17723: bare (non-backtick)
// delimiters must now be honored by TokenChunker — splitting the payload into
// paragraphs that are then merged by token size — instead of being silently
// ignored (the previous keepBare=false contract).
//
// Lossless contract (plan B): a delimiter is a HINT for where the chunker MAY
// break, never a character to DELETE. After chunking, concatenating every
// emitted chunk's text must reproduce the source exactly — the delimiter is
// retained (glued to the end of the segment that precedes it), and no stray
// "\n" is synthesized in its place. One chunk per segment (no token merge)
// happens ONLY for a backtick-wrapped CUSTOM delimiter.
//
// These tests target BOTH the text/markdown/html path (invokeTextPayload) and
// the JSON path (invokeJSONPayload -> chunkFromItem + global merge), and lock
// the custom(backtick) vs bare distinction.

// newBareChunker builds a TokenChunker for the given delimiter list and a small
// token budget so merges are observable.
func newBareChunker(t *testing.T, delims []string, tokenSize float64) *TokenChunkerComponent {
	t.Helper()
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "delimiter",
		"delimiters":       delims,
		"chunk_token_size": tokenSize,
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	return c.(*TokenChunkerComponent)
}

func invokeText(t *testing.T, c *TokenChunkerComponent, text string) []map[string]any {
	t.Helper()
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          text,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if msg, ok := out["_ERROR"].(string); ok && msg != "" {
		t.Fatalf("Go returned _ERROR: %s", msg)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	return chunks
}

func joinedText(chunks []map[string]any) string {
	var b strings.Builder
	for _, ck := range chunks {
		b.WriteString(ck["text"].(string))
	}
	return b.String()
}

// TestBareDelimiterSplitsAndTokenMergesText asserts the core fix: a bare
// multi-char delimiter ("::") now splits the text into paragraphs, each
// paragraph KEEPS its trailing delimiter (lossless), and the paragraphs are
// merged by token size. Concatenating the chunks reproduces the source exactly.
func TestBareDelimiterSplitsAndTokenMergesText(t *testing.T) {
	c := newBareChunker(t, []string{"::"}, 1024)
	const text = "alpha::beta::gamma::delta"
	chunks := invokeText(t, c, text)
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	// Lossless: the joined content equals the source, delimiter included.
	if got := joinedText(chunks); got != text {
		t.Errorf("joined text: want %q got %q (chunks=%v)", text, got, chunkTexts(chunks))
	}
	// Every paragraph retains its delimiter (it is not deleted).
	for _, ck := range chunks {
		if !strings.Contains(ck["text"].(string), "::") {
			t.Errorf("bare delimiter dropped from chunk: %q", ck["text"].(string))
		}
	}
}

// TestBareSingleCharDelimiterSplitsText covers the classic case: a single
// ASCII char delimiter (".") splits sentences and is retained.
func TestBareSingleCharDelimiterSplitsText(t *testing.T) {
	c := newBareChunker(t, []string{"."}, 1024)
	const text = "first.second.third"
	chunks := invokeText(t, c, text)
	if got := joinedText(chunks); got != text {
		t.Errorf("joined text: want %q got %q (chunks=%v)", text, got, chunkTexts(chunks))
	}
}

// TestBareCJKBoundarySplitsText covers a CJK punctuation delimiter.
func TestBareCJKBoundarySplitsText(t *testing.T) {
	c := newBareChunker(t, []string{"；"}, 1024)
	const text = "第一部分；第二部分；第三部分"
	chunks := invokeText(t, c, text)
	if got := joinedText(chunks); got != text {
		t.Errorf("joined text: want %q got %q (chunks=%v)", text, got, chunkTexts(chunks))
	}
}

// TestBareDelimiterSplitsJSON asserts the JSON path also honors bare
// delimiters and reproduces the source exactly (delimiter retained, no "\n"
// synthesized).
func TestBareDelimiterSplitsJSON(t *testing.T) {
	c := newBareChunker(t, []string{"。"}, 1024)
	const text = "第一句内容。第二句内容。第三句内容。"
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name":          "doc.json",
		"output_format": "json",
		"json": []map[string]any{
			{"text": text, "doc_type_kwd": "text"},
		},
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if got := joinedText(chunks); got != text {
		t.Errorf("joined text: want %q got %q (chunks=%v)", text, got, chunkTexts(chunks))
	}
}

// TestCustomDelimiterStillOneChunkPerSegment locks the distinction: a
// backtick-wrapped delimiter yields one chunk per segment with NO token merge,
// and the custom delimiter is DROPPED (Python-compatible split instruction).
func TestCustomDelimiterStillOneChunkPerSegment(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "delimiter",
		"delimiters":       []string{"`::`"},
		"chunk_token_size": float64(4),
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	chunks := invokeText(t, c.(*TokenChunkerComponent), "alpha::beta::gamma::delta")
	want := []string{"alpha", "beta", "gamma", "delta"}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count: want %d got %d (%v)", len(want), len(chunks), chunkTexts(chunks))
	}
	for i, w := range want {
		if got := chunks[i]["text"].(string); got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
	}
}

// TestBareDelimiterEmptyText asserts empty input yields no chunks (no panic).
func TestBareDelimiterEmptyText(t *testing.T) {
	c := newBareChunker(t, []string{"::"}, 8)
	chunks := invokeText(t, c, "")
	if len(chunks) != 0 {
		t.Errorf("empty text should produce no chunks, got %d", len(chunks))
	}
}

// TestBareDelimiterKeepsDoubledDelimiter asserts that a doubled delimiter
// ("alpha||beta") is reproduced exactly: both delimiters survive, so the
// joined text is "alpha||beta" (lossless), with no empty paragraph dropped.
func TestBareDelimiterDropsEmptySegment(t *testing.T) {
	c := newBareChunker(t, []string{"|"}, 1024)
	chunks := invokeText(t, c, "alpha||beta")
	const want = "alpha||beta"
	if got := joinedText(chunks); got != want {
		t.Errorf("joined text: want %q got %q (chunks=%v)", want, got, chunkTexts(chunks))
	}
}

// TestBareDelimiterCRLFNormalized asserts CRLF/CR are normalized to LF before
// splitting (mirrors Python naive_merge text normalization), and the retained
// delimiter reproduces the source exactly.
func TestBareDelimiterCRLFNormalized(t *testing.T) {
	c := newBareChunker(t, []string{"."}, 1024)
	chunks := invokeText(t, c, "line one.\r\nline two.\rline three.")
	const want = "line one.\nline two.\nline three."
	if got := joinedText(chunks); got != want {
		t.Errorf("joined text: want %q got %q (chunks=%v)", want, got, chunkTexts(chunks))
	}
}

// TestBareDelimiterNoDelimiterFallback asserts that with no delimiter at all
// the classic single-section merge still applies (regression guard for the
// fallback path): the bare-delimiter machinery is bypassed entirely.
func TestBareDelimiterNoDelimiterFallback(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "delimiter",
		"delimiters":       []string{},
		"chunk_token_size": float64(8),
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	chunks := invokeText(t, c.(*TokenChunkerComponent), "alpha beta gamma delta epsilon zeta eta theta")
	if len(chunks) == 0 {
		t.Fatalf("expected at least one chunk")
	}
	// No delimiter configured, so the whole text is one section merged by
	// token size; its content must be preserved in order.
	if got := joinedText(chunks); !strings.Contains(got, "alpha beta gamma") {
		t.Errorf("no-delimiter fallback lost content: %q", got)
	}
}

// TestBareDelimiterTokenMergeCoalesces asserts that short paragraphs separated
// by a bare delimiter are merged up to the token budget (not one chunk per
// paragraph when below budget), and the merged text keeps every retained
// delimiter — reproducing the source exactly.
func TestBareDelimiterTokenMergeCoalesces(t *testing.T) {
	c := newBareChunker(t, []string{"。"}, 1024)
	chunks := invokeText(t, c, "短句一。短句二。短句三。")
	// All three short sentences fit under the large budget, so they coalesce
	// into a single merged chunk whose text retains every "。" delimiter.
	if len(chunks) != 1 {
		t.Fatalf("expected single merged chunk, got %d (%v)", len(chunks), chunkTexts(chunks))
	}
	const want = "短句一。短句二。短句三。"
	if got := chunks[0]["text"].(string); got != want {
		t.Errorf("merged text: want %q got %q", want, got)
	}
}
