package chunker

import (
	"strings"
	"testing"
)

// TestTokenChunker_BareDelimiterHonored locks the #17723 fix: a bare
// (non-backtick) delimiter entry is now honored by TokenChunker. The payload
// is split on the delimiter into paragraphs (the delimiter is DROPPED, matching
// Python naive_merge), and those paragraphs are then merged by token_size.
// Regression guard for the "bare entries are active" contract.
func TestTokenChunker_BareDelimiterHonored(t *testing.T) {
	c, err := NewTokenChunker(map[string]any{
		"delimiter_mode":   "delimiter",
		"delimiters":       []string{"::"},
		"chunk_token_size": float64(8),
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	const text = "alpha::beta::gamma::delta"
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          text,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatalf("no chunks produced")
	}
	var joined strings.Builder
	for _, ck := range chunks {
		joined.WriteString(ck["text"].(string))
	}
	// No content dropped, and the bare "::" is split away (not preserved inside
	// a chunk). Python's naive_merge rebuilds each paragraph with a leading
	// "\n", so the joined text equals the source with "::" replaced by "\n".
	const want = "alpha\nbeta\ngamma\ndelta"
	if joined.String() != want {
		t.Errorf("bare delimiter not honored: joined=%q want %q (chunks=%v)", joined.String(), want, chunkTexts(chunks))
	}
	for _, ck := range chunks {
		if strings.Contains(ck["text"].(string), "::") {
			t.Errorf("bare delimiter leaked into chunk: %q", ck["text"].(string))
		}
	}
}

// TestTokenChunker_MultiByteBacktickDelimiter locks T2: a backtick-wrapped
// multi-byte (CJK) delimiter contributes its INNER content as the split
// pattern, and the longest delimiter wins over a shorter prefix of it
// (rune-descending sort). Both ensure the chunker-level delimiter handling
// is correct for non-ASCII delimiters.
func TestTokenChunker_MultiByteBacktickDelimiter(t *testing.T) {
	// Single multi-byte delimiter splits on its inner content.
	c, err := NewTokenChunker(map[string]any{
		"delimiters":       []string{"`段落`"},
		"chunk_token_size": float64(1024),
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "第一部分段落第二部分",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	want := []string{"第一部分", "第二部分"}
	if len(chunks) != len(want) {
		t.Fatalf("chunk count: want %d got %d (%v)", len(want), len(chunks), chunkTexts(chunks))
	}
	for i, w := range want {
		if got := chunks[i]["text"].(string); got != w {
			t.Errorf("chunk[%d] text: want %q got %q", i, w, got)
		}
	}

	// Longest delimiter must win over its shorter prefix: `段落` (2 runes)
	// beats `段` (1 rune), so "A段落B" splits on "段落", not "段".
	c2, err := NewTokenChunker(map[string]any{
		"delimiters":       []string{"`段落`", "`段`"},
		"chunk_token_size": float64(1024),
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out2, err := c2.Invoke(t.Context(), nil, map[string]any{
		"name":          "doc.txt",
		"output_format": "text",
		"text":          "A段落B",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks2, _ := out2["chunks"].([]map[string]any)
	if len(chunks2) != 2 || chunks2[0]["text"].(string) != "A" || chunks2[1]["text"].(string) != "B" {
		t.Errorf("longest delimiter not preferred: got %v", chunkTexts(chunks2))
	}
}
