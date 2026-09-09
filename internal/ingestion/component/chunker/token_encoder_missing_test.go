package chunker

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// byteTrimStub reproduces tokenizer.TrimContentToTokenLimit's encoder-missing
// fallback (UTF-8-safe byte-length trim) verbatim, minus the global encoder
// lookup. Swapping it into the trimToTokenLimit seam pins the encoder-missing
// path of hardSplitPiece deterministically: no global tiktoken state is
// touched, so the test is order-independent in full-package runs (tiktoken-go
// caches the loaded encoding in a package-level map that no reset can evict).
func byteTrimStub(s string, limit int) string {
	if limit < 0 {
		limit = 0
	}
	if limit <= 0 {
		return ""
	}
	b := []byte(s)
	if len(b) <= limit {
		return s
	}
	for limit > 0 && !utf8.Valid(b[:limit]) {
		limit--
	}
	return string(b[:limit])
}

// TestHardSplitPiece_EncoderMissingFallback pins the encoder-missing fallback
// path of hardSplitPiece. With the encoder unavailable, trimToTokenLimit falls
// back to a UTF-8-safe byte-length trim. The pre-optimization loop guard
// (tokenizeStr(rest) > target) never fired in that state, so the whole
// over-limit remainder was emitted as ONE piece; the current loop drives off
// the trim result and keeps emitting bounded pieces instead — strictly more
// correct, and the behavior this test locks in so the fallback stays a
// first-class, tested path.
func TestHardSplitPiece_EncoderMissingFallback(t *testing.T) {
	orig := trimToTokenLimit
	trimToTokenLimit = byteTrimStub
	t.Cleanup(func() { trimToTokenLimit = orig })

	// Pure ASCII+CJK text with no @@...## tags: adjustCutPastTag extends a cut
	// past a tag when tokenizeStr(head) <= target, which a dead encoder makes
	// always true and would swallow the byte-bounded shape under test.
	text := hardSplitBenchText()
	const target = 64

	got := hardSplitPiece(text, "", target)

	// The fallback must emit bounded pieces, not one giant remainder (the old
	// guard-never-fires behavior).
	if len(got) < 2 {
		t.Fatalf("encoder-missing fallback produced %d piece(s); want bounded pieces (>1)", len(got))
	}
	// Lossless: the pieces concatenate back to the input exactly, each within
	// the byte budget and valid UTF-8 (the byte-trim fallback's guarantees).
	var sb strings.Builder
	for i, p := range got {
		if len(p.Text) > target {
			t.Fatalf("piece %d is %d bytes; byte-trim fallback must keep pieces <= %d bytes", i, len(p.Text), target)
		}
		if !utf8.ValidString(p.Text) {
			t.Fatalf("piece %d is not valid UTF-8", i)
		}
		sb.WriteString(p.Text)
	}
	if sb.String() != text {
		t.Fatalf("pieces do not concatenate back to the input (got %d bytes, want %d)", sb.Len(), len(text))
	}
}
