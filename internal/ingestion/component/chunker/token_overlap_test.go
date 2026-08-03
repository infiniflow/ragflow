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
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	got := make([]string, len(chunks))
	for i, ck := range chunks {
		s, _ := ck["text"].(string)
		got[i] = s
	}

	// Exact expected texts match Python's naive_merge output. The trailing
	// inter-line space of every line is preserved; only the chunk's own
	// leading/trailing whitespace is trimmed at final output (token.go:587) —
	// naive_merge itself never TrimSpaces a unit, so Go must not either here.
	want := []string{
		strings.TrimSuffix(strings.Repeat(alpha, 40), " "),
		strings.Repeat(alpha, 4) + "\n" + strings.TrimSuffix(strings.Repeat(beta, 40), " "),
		"ta " + strings.Repeat(beta, 4) + "\n" + strings.TrimSuffix(strings.Repeat(gamma, 40), " "),
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("chunk[%d]:\n got=%q\nwant=%q", i, got[i], want[i])
		}
	}

	// The regression-specific assertion: the overlap boundary must keep the
	// inter-line trailing space (Python emits "alpha \nbeta", not "alpha\nbeta").
	if !strings.Contains(got[1], "alpha \nbeta") {
		t.Errorf("chunk[1] lost inter-line space before newline: %q", got[1])
	}
	if strings.Contains(got[1], "alpha\nbeta") {
		t.Errorf("chunk[1] has space-less boundary (bug present): %q", got[1])
	}
}
