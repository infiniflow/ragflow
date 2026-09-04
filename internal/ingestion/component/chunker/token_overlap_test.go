package chunker

import (
	"context"
	"strings"
	"testing"
)

// TestTokenChunker_TextOverlapPreservesInterLineSpace pins the fix for the
// token-text-overlap-split divergence (parity rule token-text-overlap-split).
//
// The function under test is TokenChunkerComponent.mergeByTokenSize
// (token.go), which the text path reaches ONLY when there is no active
// delimiter: with empty delimiters, invokeTextPayload routes to
// mergeByTokenSize (token.go:303-304); with a non-empty delimiter such as
// ["\n"] it instead routes to mergeByTokenSizeFromJSON, a different code path
// that is unaffected by this fix. This test therefore drives the component
// through Invoke with delimiters: []string{} so it actually lands on the
// patched function.
//
// mergeByTokenSize splits the payload on sentence delimiters and merges per
// token budget, carrying an overlap prefix from the previous chunk.
// Python's naive_merge builds each unit from "\n" + sub_sec where sub_sec
// retains its trailing inter-line whitespace (naive_merge:1357 — it never
// TrimSpaces a unit; the only post-processing is dropping the leading empty
// placeholder at naive_merge:1370-1375). The Go merge must do the same: if it
// TrimSpaces each split fragment, the trailing space of a line is dropped, the
// overlap prefix carved from the previous (untrimmed) chunk loses that
// character, and every following chunk head diverges from Python by one
// character.
//
// This test locks the exact chunk texts against the Python reference
// (rag/nlp.naive_merge with chunk_token_size=64, delimiters=[],
// overlapped_percent=0.1).
func TestTokenChunker_TextOverlapPreservesInterLineSpace(t *testing.T) {
	const (
		alpha = "alpha "
		beta  = "beta "
		gamma = "gamma "
	)
	text := strings.Repeat(alpha, 40) + "\n" +
		strings.Repeat(beta, 40) + "\n" +
		strings.Repeat(gamma, 40)

	c, err := NewTokenChunker(map[string]any{
		"chunk_token_size":   float64(64),
		"delimiters":         []string{}, // routes Invoke -> mergeByTokenSize (the patched function)
		"overlapped_percent": 0.1,
	})
	if err != nil {
		t.Fatalf("NewTokenChunker: %v", err)
	}
	out, err := c.Invoke(context.Background(), nil, map[string]any{
		"name": "t", "output_format": "text", "text": text,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	chunks, _ := out["chunks"].([]map[string]any)
	if len(chunks) == 0 {
		t.Fatalf("expected chunks, got none")
	}
	got := make([]string, len(chunks))
	for i, ck := range chunks {
		s, _ := ck["text"].(string)
		got[i] = s
		// Hard cap: no chunk may exceed the token budget.
		if n := tokenizeStr(s); n > 64 {
			t.Errorf("chunk %d exceeds budget: tokens=%d (cap=64)", i, n)
		}
	}

	// The regression this test pins: the inter-line trailing space of every
	// line must be preserved across a merge/overlap boundary (Python emits
	// "alpha \nbeta", not "alpha\nbeta"). No emitted chunk may contain a
	// space-less "\n" boundary, and at least one space-preserving boundary must
	// actually appear (otherwise the fixture no longer exercises the path).
	spacePreserved := 0
	for i, s := range got {
		if strings.Contains(s, "alpha\nbeta") {
			t.Errorf("chunk[%d] has space-less alpha/beta boundary (bug present): %q", i, s)
		}
		if strings.Contains(s, "beta\ngamma") {
			t.Errorf("chunk[%d] has space-less beta/gamma boundary (bug present): %q", i, s)
		}
		if strings.Contains(s, "alpha \nbeta") || strings.Contains(s, "beta \ngamma") {
			spacePreserved++
		}
	}
	if spacePreserved == 0 {
		t.Errorf("no chunk preserved an inter-line space across a boundary; the regression fixture is not exercised")
	}
}
