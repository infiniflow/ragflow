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
// Python contract (rag/nlp/__init__.py:1406-1415, naive_merge default path):
// the payload is split on the delimiter, the delimiter text is DROPPED, and
// each kept paragraph is rebuilt with a leading "\n" (its original surrounding
// whitespace is preserved). Paragraphs are thus joined by "\n", never by the
// raw delimiter. One chunk per segment (no token merge) happens ONLY for a
// backtick-wrapped CUSTOM delimiter.
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
// multi-char delimiter ("::") now splits the text into paragraphs, the
// delimiter text is dropped, and the paragraphs are rejoined with "\n"
// (matching Python's "\n"+sub_sec reconstruction) — not with the raw
// delimiter. The paragraphs are then merged by token size.
func TestBareDelimiterSplitsAndTokenMergesText(t *testing.T) {
	c := newBareChunker(t, []string{"::"}, 1024)
	const text = "alpha::beta::gamma::delta"
	chunks := invokeText(t, c, text)
	if len(chunks) == 0 {
		t.Fatal("expected at least one chunk")
	}
	// The delimiter must never survive inside a chunk.
	for _, ck := range chunks {
		if strings.Contains(ck["text"].(string), "::") {
			t.Errorf("bare delimiter leaked into chunk: %q", ck["text"].(string))
		}
	}
	// Paragraphs are rejoined with "\n" (each paragraph keeps a leading "\n"),
	// so the joined content equals the source with "::" replaced by "\n".
	const want = "alpha\nbeta\ngamma\ndelta"
	if got := joinedText(chunks); got != want {
		t.Errorf("joined text: want %q got %q (chunks=%v)", want, got, chunkTexts(chunks))
	}
}

// TestBareSingleCharDelimiterSplitsText covers the classic case: a single
// ASCII char delimiter (".") splits sentences, the delimiter is dropped, and
// paragraphs are rejoined with "\n".
func TestBareSingleCharDelimiterSplitsText(t *testing.T) {
	c := newBareChunker(t, []string{"."}, 1024)
	chunks := invokeText(t, c, "first.second.third")
	const want = "first\nsecond\nthird"
	if got := joinedText(chunks); got != want {
		t.Errorf("joined text: want %q got %q (chunks=%v)", want, got, chunkTexts(chunks))
	}
	for _, ck := range chunks {
		if strings.Contains(ck["text"].(string), ".") {
			t.Errorf("bare delimiter leaked into chunk: %q", ck["text"].(string))
		}
	}
}

// TestBareCJKBoundarySplitsText covers a CJK punctuation delimiter.
func TestBareCJKBoundarySplitsText(t *testing.T) {
	c := newBareChunker(t, []string{"；"}, 1024)
	chunks := invokeText(t, c, "第一部分；第二部分；第三部分")
	const want = "第一部分\n第二部分\n第三部分"
	if got := joinedText(chunks); got != want {
		t.Errorf("joined text: want %q got %q (chunks=%v)", want, got, chunkTexts(chunks))
	}
}

// TestBareDelimiterSplitsJSON asserts the JSON path also honors bare
// delimiters (chunkFromItem now receives a non-nil pattern and the global
// merge applies because there is no custom delimiter).
func TestBareDelimiterSplitsJSON(t *testing.T) {
	c := newBareChunker(t, []string{"。"}, 1024)
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
	const want = "第一句内容\n第二句内容\n第三句内容"
	if got := joinedText(chunks); got != want {
		t.Errorf("joined text: want %q got %q (chunks=%v)", want, got, chunkTexts(chunks))
	}
}

// TestCustomDelimiterStillOneChunkPerSegment locks the distinction: a
// backtick-wrapped delimiter must yield one chunk per segment with NO token
// merge (and the delimiter is dropped with no "\n" rejoin), even with a tight
// budget.
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

// TestBareDelimiterDropsEmptySegment asserts that a genuinely empty segment
// produced by a doubled delimiter ("alpha||beta") is dropped, so the joined
// text has no empty paragraph (mirrors Python's `if not sub_sec: continue`).
// (Whitespace-only segments are NOT dropped — Python keeps them — so this test
// deliberately uses an empty, not whitespace, segment.)
func TestBareDelimiterDropsEmptySegment(t *testing.T) {
	c := newBareChunker(t, []string{"|"}, 1024)
	chunks := invokeText(t, c, "alpha||beta")
	const want = "alpha\nbeta"
	if got := joinedText(chunks); got != want {
		t.Errorf("joined text: want %q got %q (chunks=%v)", want, got, chunkTexts(chunks))
	}
}

// TestBareDelimiterCRLFNormalized asserts CRLF/CR are normalized to LF before
// splitting (mirrors Python naive_merge text normalization). The "\n" left
// behind by the dropped delimiter is preserved between paragraphs.
func TestBareDelimiterCRLFNormalized(t *testing.T) {
	c := newBareChunker(t, []string{"."}, 1024)
	chunks := invokeText(t, c, "line one.\r\nline two.\rline three.")
	const want = "line one\n\nline two\n\nline three"
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
// paragraph when below budget), while still keeping the "\n" paragraph
// boundary — exactly mirroring Python's merge.
func TestBareDelimiterTokenMergeCoalesces(t *testing.T) {
	c := newBareChunker(t, []string{"。"}, 1024)
	chunks := invokeText(t, c, "短句一。短句二。短句三。")
	// All three short sentences fit under the large budget, so they coalesce
	// into a single merged chunk whose text keeps the "\n" paragraph joins.
	if len(chunks) != 1 {
		t.Fatalf("expected single merged chunk, got %d (%v)", len(chunks), chunkTexts(chunks))
	}
	const want = "短句一\n短句二\n短句三"
	if got := chunks[0]["text"].(string); got != want {
		t.Errorf("merged text: want %q got %q", want, got)
	}
}
