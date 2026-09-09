package chunker

import (
	"strings"
	"testing"
	"unicode/utf8"

	"ragflow/internal/tokenizer"
)

// TestHardSplitPiece_EncoderMissingFallback pins the encoder-missing fallback
// path of hardSplitPiece. With no cl100k table on disk, NumTokensFromString
// returns 0 and TrimContentToTokenLimit falls back to a UTF-8-safe byte-length
// trim. The pre-optimization loop guard (tokenizeStr(rest) > target) never
// fired in that state, so the whole over-limit remainder was emitted as ONE
// piece; the current loop drives off TrimContentToTokenLimit's result and keeps
// emitting bounded pieces instead — strictly more correct, and the behavior
// this test locks in so the fallback stays a first-class, tested path.
func TestHardSplitPiece_EncoderMissingFallback(t *testing.T) {
	// Scope the BPE loader to an empty directory so the cl100k table is
	// missing, and reset the encoder cache so the failure actually loads.
	empty := t.TempDir()
	t.Setenv("TIKTOKEN_CACHE_DIR", "")
	t.Setenv("DATA_GYM_CACHE_DIR", "")
	tokenizer.SetBpeSearchRootsForTest([]string{empty})
	tokenizer.ResetCL100KEncoderForTest()
	t.Cleanup(func() {
		// Restore the default search roots and clear the failed load so
		// sibling tests reload the real table.
		tokenizer.SetBpeSearchRootsForTest(nil)
		tokenizer.ResetCL100KEncoderForTest()
	})

	// Sanity: the encoder really is unavailable in this scope.
	if n := tokenizeStr("hello world"); n != 0 {
		t.Fatalf("tokenizeStr = %d under a missing table; expected the 0-error path", n)
	}

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
