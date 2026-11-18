package chunker

import (
	"strings"
	"testing"
)

// tagSlackTokens bounds how far past target a piece may legitimately go when a
// leading @@...## coordinate tag is swallowed whole instead of split: the
// extension past the closing "##" adds at most the tag's own tokens (corpus
// tags measure <= 6).
const tagSlackTokens = 16

// TestHardSplitPieceInvariants pins the output contract of hardSplitPiece that
// callers rely on. It replaces the retired differential-against-reference
// harness: a second implementation of the same algorithm is a permanent
// maintenance liability, while these invariants hold for any correct
// token-splitting of this shape.
//
//  1. Bounded pieces: every cut lands on an original token boundary and pieces
//     are at most target original tokens (plus the leading-tag-whole
//     extension slack for tag inputs). Re-encoded piece token counts can
//     overshoot only at the two boundary tokens, whose BPE merges depended on
//     now-removed neighbors; the test self-calibrates that overshoot from the
//     longest token in the input instead of hardcoding a vocabulary fact.
//  2. Lossless at the byte level: the pieces concatenate back to the input
//     exactly. Pieces are byte-prefixes, not rune-prefixes — a cut may land
//     mid-multibyte and leave raw continuation bytes at a piece's tail
//     (observed with emoji/CJK corpora), repaired by the next piece's head;
//     the downstream JSON layer replaces any residual invalid tail.
//  3. Monotone progress: pieces are non-empty and each is a true byte prefix
//     of the not-yet-emitted remainder — the split never revisits or reorders
//     bytes.
//  4. Tags stay whole: every complete @@...## span lies entirely inside one
//     piece — never split across a piece boundary.
func TestHardSplitPieceInvariants(t *testing.T) {
	inputs := []string{ // plain, CJK, coordinate tags, emoji, degenerate
		strings.Repeat("boundary-less run of text with no sentence delimiter ", 60),
		strings.Repeat("中文中文中文无句读", 200),
		strings.Repeat("prelude @@12,34## body @@56,78## tail ", 80),
		strings.Repeat("mid-char🎉emoji🎉run-", 80),
		strings.Repeat("a", 600),
		"single-token",
		"",
		"\n\n\n",
	}
	for _, target := range []int{32, 64, 128, 512} {
		for _, in := range inputs {
			hasTags := strings.Contains(in, "@@")
			got := hardSplitPiece(in, "", target)

			// Longest single token in the input bounds the re-encode overshoot
			// at a piece's two boundary tokens (see invariant 1).
			maxTokBytes := 1
			if toks, ok := encodeTokens(in); ok {
				for i := range toks {
					if n := len(decodeTokens(toks[i : i+1])); n > maxTokBytes {
						maxTokBytes = n
					}
				}
			}

			var joined strings.Builder
			progress := 0
			for i, p := range got {
				if p.Text == "" {
					t.Fatalf("target=%d in=%d bytes: piece %d is empty", target, len(in), i)
				}
				if !strings.HasPrefix(in[progress:], p.Text) {
					t.Fatalf("target=%d in=%d bytes: piece %d is not a prefix of the remainder (monotone progress broken)", target, len(in), i)
				}
				progress += len(p.Text)

				// +2*(maxTokBytes-1): the piece's first and last tokens were
				// BPE-merged with neighbors outside the slice; re-encoded
				// alone they can decompose into up to maxTokBytes single-byte
				// tokens each.
				bound := target + 2*(maxTokBytes-1)
				if hasTags {
					bound += tagSlackTokens
				}
				if n := tokenizeStr(p.Text); n > bound {
					t.Fatalf("target=%d in=%d bytes: piece %d has %d tokens; bound is %d", target, len(in), i, n, bound)
				}
				joined.WriteString(p.Text)
			}
			if joined.String() != in {
				t.Fatalf("target=%d in=%d bytes: pieces do not reproduce input (%d != %d bytes)",
					target, len(in), joined.Len(), len(in))
			}

			// Every complete @@...## span sits inside a single piece.
			for i := 0; i < len(in); {
				sRel := strings.Index(in[i:], "@@")
				if sRel < 0 {
					break
				}
				s := i + sRel
				eRel := strings.Index(in[s+2:], "##")
				if eRel < 0 {
					break
				}
				e := s + 2 + eRel + 2
				whole := false
				pos := 0
				for _, p := range got {
					if pos <= s && e <= pos+len(p.Text) {
						whole = true
						break
					}
					pos += len(p.Text)
				}
				if !whole {
					t.Fatalf("target=%d in=%d bytes: tag span [%d,%d) is split across pieces", target, len(in), s, e)
				}
				i = e
			}
		}
	}
}
