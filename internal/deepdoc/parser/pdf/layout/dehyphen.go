package layout

import (
	"math"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// garbageHyphenRE removes U+FFFD (replacement char) and U+00AD (soft hyphen)
// — both are pure extraction garbage, never real content — along with the
// optional single space a line break inserts after them, so the two halves of
// a hyphenated word rejoin ("pre� cipitation" → "precipitation").
var garbageHyphenRE = regexp.MustCompile(`[` + "\uFFFD\u00AD" + `]\s?`)

// lineBreakHyphenRE rejoins a real hyphen that a PDF placed at a line break
// ("re- covery" → "recovery"). It fires only when a space follows the hyphen,
// so in-word hyphens without a space ("lithium-ion", "well-known") survive.
var lineBreakHyphenRE = regexp.MustCompile(`(\p{L})[` + "-\u2010\u2011\u2043" + `]\s(\p{L})`)

// Dehyphenate rejoins words that a PDF split across a line break with a
// hyphen. Some PDFs (e.g. Elsevier journals set in CharisSIL) encode the
// line-break hyphen with a glyph whose Unicode mapping the extractor fails to
// resolve, so the boundary shows up as U+FFFD (replacement char) or U+00AD
// (soft hyphen); others use a literal '-'. In every case the two halves land
// in two vertically-adjacent text boxes and the later vertical merge glues
// them with a space, yielding "pre- cipitation" or "pre� cipitation" instead
// of "precipitation".
//
// Dehyphenate must run AFTER FinalReadingOrderMerge (so consecutive lines are
// adjacent in the slice and share a column id) and BEFORE NaiveVerticalMerge
// (which would otherwise insert the offending space). It collapses a box that
// ends in a broken-hyphen marker with the following box when that box continues
// the word (starts with a lowercase letter and is the next physical line).
//
// It also strips stray U+FFFD / U+00AD markers embedded inside a word (the
// "text cleaning" half of the fix) so a split that happens to survive inside a
// single box is still repaired.
func Dehyphenate(boxes []pdf.TextBox) []pdf.TextBox {
	out := make([]pdf.TextBox, 0, len(boxes))
	for i := 0; i < len(boxes); i++ {
		b := boxes[i]
		// Rejoin a line-broken word first (the broken-hyphen marker must
		// still be present on b's end for canDehyphenate to detect it).
		if i+1 < len(boxes) && canDehyphenate(b, boxes[i+1]) {
			b = joinBrokenHyphen(b, boxes[i+1])
			i++ // consume the continuation box
		}
		// Then strip any remaining replacement/soft-hyphen garbage (e.g. a
		// broken hyphen that landed mid-box with a line-break space).
		b.Text = cleanIntraWordHyphens(b.Text)
		out = append(out, b)
	}
	return out
}

// canDehyphenate reports whether prev ends in a broken-hyphen marker and curr
// is the next line continuing the same word.
func canDehyphenate(prev, curr pdf.TextBox) bool {
	if prev.PageNumber != curr.PageNumber {
		return false
	}
	if prev.ColID != curr.ColID {
		return false
	}
	if !endsWithBrokenHyphen(prev.Text) {
		return false
	}
	if !startsWithWordContinuation(curr.Text) {
		return false
	}
	return linesAreVerticallyAdjacent(prev, curr)
}

// endsWithBrokenHyphen reports whether s ends in a broken-hyphen marker that
// is preceded by at least one letter (so a lone "-" is not treated as a break).
func endsWithBrokenHyphen(s string) bool {
	s = strings.TrimRight(s, " \t")
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s)
	if !isBrokenHyphenRune(r) {
		return false
	}
	before := s[:len(s)-utf8.RuneLen(r)]
	rb, _ := utf8.DecodeLastRuneInString(before)
	return unicode.IsLetter(rb)
}

// stripTrailingBrokenHyphen removes a single trailing broken-hyphen marker.
func stripTrailingBrokenHyphen(s string) string {
	s = strings.TrimRight(s, " \t")
	if s == "" {
		return s
	}
	r, size := utf8.DecodeLastRuneInString(s)
	if isBrokenHyphenRune(r) {
		return s[:len(s)-size]
	}
	return s
}

// startsWithWordContinuation reports whether s begins with a lowercase letter.
// A new sentence starts with an uppercase letter, so a lowercase start strongly
// implies the previous line's hyphen was a real break rather than a dash.
func startsWithWordContinuation(s string) bool {
	s = strings.TrimLeft(s, " \t")
	if s == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.IsLetter(r) && unicode.IsLower(r)
}

// linesAreVerticallyAdjacent reports whether curr is roughly the next physical
// line below prev (not a distant paragraph). It is self-contained: the allowed
// gap scales with the box heights so no external median height is needed.
func linesAreVerticallyAdjacent(prev, curr pdf.TextBox) bool {
	if curr.Top <= prev.Top {
		return false
	}
	gap := curr.Top - prev.Bottom
	lineH := math.Max(prev.Bottom-prev.Top, curr.Bottom-curr.Top)
	if lineH <= 0 {
		lineH = 1
	}
	return gap <= lineH*2.5
}

// joinBrokenHyphen concatenates curr onto prev after dropping prev's trailing
// broken-hyphen marker, and expands prev's geometry to cover both lines.
func joinBrokenHyphen(prev, curr pdf.TextBox) pdf.TextBox {
	prev.Text = stripTrailingBrokenHyphen(prev.Text) + curr.Text
	prev.Top = math.Min(prev.Top, curr.Top)
	prev.Bottom = math.Max(prev.Bottom, curr.Bottom)
	prev.X0 = math.Min(prev.X0, curr.X0)
	prev.X1 = math.Max(prev.X1, curr.X1)
	return prev
}

// cleanIntraWordHyphens repairs broken-hyphen garbage inside a single box's
// text. U+FFFD / U+00AD are pure extraction noise and are deleted (with the
// optional trailing line-break space) so the word halves rejoin; a real hyphen
// at a line break ("re- covery") is rejoined the same way, while an in-word
// hyphen without a following space ("lithium-ion", "well-known") is kept.
func cleanIntraWordHyphens(s string) string {
	if !strings.ContainsRune(s, '\uFFFD') && !strings.ContainsRune(s, '\u00AD') && !strings.ContainsRune(s, '-') {
		return s
	}
	s = garbageHyphenRE.ReplaceAllString(s, "")
	s = lineBreakHyphenRE.ReplaceAllString(s, "$1$2")
	return s
}

// isBrokenHyphenRune reports whether r is a hyphen-like codepoint that a PDF
// may use to split a word across a line break.
func isBrokenHyphenRune(r rune) bool {
	switch r {
	case '\uFFFD', // replacement char (unresolved glyph)
		'\u00AD', // soft hyphen
		'-',      // hyphen-minus
		'\u2010', // hyphen
		'\u2011': // non-breaking hyphen
		return true
	}
	return false
}
