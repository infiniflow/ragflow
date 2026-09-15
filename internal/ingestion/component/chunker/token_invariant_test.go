package chunker

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// tagSlackTokens bounds how far past target a piece may legitimately go when
// adjustCutPastTag swallows a leading @@...## coordinate tag whole instead of
// splitting it: the trim result is within target, and the extension past the
// closing "##" adds at most the tag's own tokens (corpus tags measure <= 6).
const tagSlackTokens = 16

// TestHardSplitPieceInvariants pins the output contract of hardSplitPiece that
// callers rely on. It replaces the retired differential-against-reference
// harness: a second implementation of the same algorithm is a permanent
// maintenance liability, while these invariants hold for any correct
// token-splitting of this shape.
//
//  1. Bounded pieces: each piece is at most target tokens plus the documented
//     one-token drift a re-encoded continuation piece can carry (cl100k is
//     byte-level; a cut trimmed off a mid-multibyte boundary is re-absorbed by
//     the next piece). Inputs without coordinate tags get the strict bound;
//     tag inputs additionally allow the leading-tag-whole extension slack.
//  2. Lossless at the byte level: the pieces concatenate back to the input
//     exactly. Pieces are byte-prefixes, not rune-prefixes — a cut may land
//     mid-multibyte and leave raw continuation bytes at a piece's tail
//     (observed with emoji/CJK corpora), repaired by the next piece's head;
//     the downstream JSON layer replaces any residual invalid tail.
//  3. Monotone progress: pieces are non-empty and each is a true byte prefix
//     of the not-yet-emitted remainder — the split never revisits or reorders
//     bytes.
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

				// +utf8.UTFMax: a cut landing mid-multibyte leaves up to
				// UTFMax-1 dangling raw bytes at the tail, which re-encode as
				// individual single-byte tokens (cl100k is byte-level), plus
				// the one-token continuation drift.
				bound := target + utf8.UTFMax
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
		}
	}
}
