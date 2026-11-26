// Copied from github.com/mgilbir/gowemf, file playback_bidi.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.
// ivy: the imports name the copy, github.com/xo/ivy/internal/gowemf.

package gowemf

import (
	"unicode/utf16"

	"ragflow/internal/thirdparty/ivy/internal/gowemf/internal/bidi"
)

// textLevels resolves the bidirectional embedding levels of a GDI string, one
// per element, or returns nil when all are 0. GDI lays out Unicode text with
// the Unicode Bidirectional Algorithm (UAX #9), whose paragraph level is 1 in
// right-to-left reading order and 0 otherwise; each paragraph separator ends a
// paragraph, and the string is one line. Glyph indexes are already shaped:
// in right-to-left reading order they are laid right to left (MS-EMF 2.2.5).
// The elements of one surrogate pair share a level.
func textLevels(text []uint16, glyphs, rtl bool) []uint8 {
	if glyphs {
		if !rtl {
			return nil
		}
		levels := make([]uint8, len(text))
		for i := range levels {
			levels[i] = 1
		}
		return levels
	}
	dir := bidi.LeftToRight
	if rtl {
		dir = bidi.RightToLeft
	}
	runes, starts := codePoints(text)
	classes := make([]bidi.Class, len(runes))
	brackets := make([]bidi.Bracket, len(runes))
	for i, r := range runes {
		classes[i] = bidi.Lookup(r)
		if classes[i] == bidi.ON {
			brackets[i] = bidi.LookupBracket(r)
		}
	}
	levels := make([]uint8, len(text))
	nonzero := false
	for start := 0; start < len(runes); {
		end := start + 1
		for end < len(runes) && classes[end-1] != bidi.B {
			end++
		}
		resolved, para := bidi.Resolve(classes[start:end], brackets[start:end], dir)
		level := para
		for k, l := range resolved {
			// Characters removed by rule X9 keep the preceding level, so
			// they stay beside their neighbors when reordered.
			if l != bidi.Removed {
				level = l
			}
			i := start + k
			for u := starts[i]; u < starts[i+1]; u++ {
				levels[u] = uint8(level)
			}
			nonzero = nonzero || level != 0
		}
		start = end
	}
	if !nonzero {
		return nil
	}
	return levels
}

// codePoints decodes UTF-16 into code points and the index of each one's
// first unit, with a final entry for len(text). An unpaired surrogate is a
// code point of its own.
func codePoints(text []uint16) ([]rune, []int) {
	runes := make([]rune, 0, len(text))
	starts := make([]int, 0, len(text)+1)
	for i := 0; i < len(text); i++ {
		starts = append(starts, i)
		r := rune(text[i])
		if utf16.IsSurrogate(r) && i+1 < len(text) {
			if pair := utf16.DecodeRune(r, rune(text[i+1])); pair != 0xfffd {
				r = pair
				i++
			}
		}
		runes = append(runes, r)
	}
	return runes, append(starts, len(text))
}

// displayOrder returns the element indexes from left to right (rule L2). The
// units of a surrogate pair stay together in logical order.
func displayOrder(text []uint16, glyphs bool, levels []uint8) []int {
	order := make([]int, 0, len(text))
	if levels == nil {
		for i := range text {
			order = append(order, i)
		}
		return order
	}
	starts := make([]int, 0, len(text)+1)
	if glyphs {
		for i := range text {
			starts = append(starts, i)
		}
		starts = append(starts, len(text))
	} else {
		_, starts = codePoints(text)
	}
	cp := make([]int8, len(starts)-1)
	for i := range cp {
		cp[i] = int8(levels[starts[i]])
	}
	for _, i := range bidi.Reorder(cp) {
		for u := starts[i]; u < starts[i+1]; u++ {
			order = append(order, u)
		}
	}
	return order
}
