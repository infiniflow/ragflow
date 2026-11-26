// This file is not in github.com/mgilbir/gowemf. ivy added it to its copy
// (internal/gowemf/README.md, change 19).

package gowemf

import (
	"slices"
	"unicode"
	"unicode/utf16"
)

// glyphText is the text of a record that names its glyphs by index. The
// indexes belong to the font that the metafile names, and a backend draws
// with another font, which orders its glyphs in another way. So when another
// record gives the text of the same glyphs, the player draws that text in
// place of the indexes (ivy D53). One of the fields is set:
//
//   - text is an EMF text record that draws the same glyphs. The player
//     decodes it with the font of the record of the indexes.
//   - units is the text of the GDI fallback of an EMF+ DrawDriverString.
type glyphText struct {
	text  *Text
	units []uint16
}

// The EMF+ records that draw, and EmfPlusGetDC. In an EMF+ Dual file, the
// GDI records that follow a comment with a drawing record are the fallback
// of that record.
const (
	plusFirstDrawingRecord = 0x4009 // EmfPlusClear
	plusLastDrawingRecord  = 0x401c // EmfPlusDrawString
	plusGetDCRecord        = 0x4004
)

// findGlyphText returns the text of each record of data that names its glyphs
// by index, when another record gives that text exactly. The key is the
// offset of the record of the indexes. It finds two pairs:
//
//   - An EMR_EXTTEXTOUTW or EMR_SMALLTEXTOUT with ETO_GLYPH_INDEX, and the
//     text record just before or after it that draws the same elements as
//     text, at the same reference point, with the same font. Only the
//     records that set the text color, the background color and the
//     background mode can come between the two.
//   - An EMF+ DrawDriverString with glyph indexes in an EMF+ Dual file, and
//     its GDI fallback: the Unicode text records after its comment, up to
//     the next EMF+ record. The comment holds no other drawing record.
//
// The player still checks that the text has one element for each glyph. The
// result is nil when no record pairs.
func findGlyphText(data []byte, h Header, o StreamOptions) map[int]glyphText {
	if h.EMF == nil {
		return nil
	}
	var found map[int]glyphText
	add := func(off int, g glyphText) {
		if found == nil {
			found = make(map[int]glyphText)
		}
		if _, ok := found[off]; !ok {
			found[off] = g
		}
	}
	dual := h.EMFPlus != nil && h.EMFPlus.Dual

	// last is the last EMF text record, while only the records that leave
	// its text as it is follow it, and lastOff is its offset.
	var last *Text
	lastOff := 0
	// group is the comment of the last EMF+ records. driver is the offset
	// of the DrawDriverString with glyph indexes in it, or -1, and units
	// collect the text of its fallback while ok.
	group, driver := -1, -1
	var units []uint16
	ok := false
	finish := func() {
		if driver >= 0 && ok && len(units) > 0 {
			add(driver, glyphText{units: units})
		}
		driver, units, ok = -1, nil, false
	}
	_, _ = Walk(data, o.Framing, func(r Record) error {
		if r.Format == EMFPlus {
			last = nil
			if r.ParentOffset != group {
				finish()
				group, ok = r.ParentOffset, dual
			}
			switch {
			case r.Type == PlusDrawDriverStringRecord && driver < 0:
				v, err := Decode(r, o.Decoding)
				if d, isDriver := v.(PlusDriverString); err == nil && isDriver && d.Options&1 == 0 {
					driver = r.Offset
				} else {
					ok = false
				}
			case r.Type == PlusDrawDriverStringRecord, r.Type == plusGetDCRecord,
				r.Type >= plusFirstDrawingRecord && r.Type <= plusLastDrawingRecord:
				ok = false
			}
			return nil
		}
		switch r.Type {
		case EMRExtTextOutA, EMRExtTextOutW, EMRSmallTextOut:
		case EMRSetTextColor, EMRSetBkColor, EMRSetBkMode:
			return nil
		default:
			last = nil
			return nil
		}
		v, err := Decode(r, o.Decoding)
		t, isText := v.(Text)
		if err != nil || !isText {
			last, ok = nil, false
			return nil
		}
		if driver >= 0 && ok {
			if t.Unicode && t.Options&0x90 == 0 {
				units = append(units, textUnits(t)...)
			} else {
				ok = false
			}
		}
		if last != nil {
			switch {
			case indexed(t) && !indexed(*last) && sameRun(t, *last):
				add(r.Offset, glyphText{text: last})
			case indexed(*last) && !indexed(t) && sameRun(t, *last):
				add(lastOff, glyphText{text: &t})
			}
		}
		last, lastOff = &t, r.Offset
		return nil
	})
	finish()
	return found
}

// indexed tells whether a text record names its glyphs by index
// (ETO_GLYPH_INDEX), with 16 bits for each.
func indexed(t Text) bool {
	return t.Options&0x10 != 0 && t.Unicode && !t.SmallChars
}

// sameRun tells whether two text records draw at the same place in the same
// way: the same reference point, graphics mode and scales, no right to left
// reading (ETO_RTLREADING), and the same advances when both give them.
func sameRun(a, b Text) bool {
	if a.Reference != b.Reference || a.GraphicsMode != b.GraphicsMode || a.ScaleX != b.ScaleX || a.ScaleY != b.ScaleY ||
		(a.Options|b.Options)&0x80 != 0 || (a.Options^b.Options)&0x2000 != 0 {
		return false
	}
	if a.Advances.Len() == 0 || b.Advances.Len() == 0 {
		return true
	}
	return slices.Equal(a.Advances.data, b.Advances.data) && a.Advances.width == b.Advances.width
}

// textUnits returns the UTF-16 code units of a Unicode text record.
func textUnits(t Text) []uint16 {
	if t.SmallChars {
		out := make([]uint16, len(t.Bytes))
		for i, b := range t.Bytes {
			out[i] = uint16(b)
		}
		return out
	}
	out := make([]uint16, len(t.Bytes)/2)
	for i := range out {
		out[i] = u16(t.Bytes[2*i:])
	}
	return out
}

// textOfGlyphs returns the text of the n glyph indexes of the record at off,
// in the font f of that record, when the text has one element for each
// glyph in the same order. rtl tells whether the record reads right to
// left.
func (p *player) textOfGlyphs(off int, f *gdiFont, n int, rtl bool) ([]uint16, bool) {
	g, ok := p.glyphText[off]
	if !ok {
		return nil, false
	}
	units := g.units
	if g.text != nil {
		var glyphs bool
		var why string
		units, _, glyphs, why = p.decodeText(*g.text, f, g.text.Unicode)
		if glyphs || why != "" {
			return nil, false
		}
	}
	if len(units) != n || textLevels(units, false, rtl) != nil {
		return nil, false
	}
	for _, u := range units {
		if !oneGlyph(rune(u)) {
			return nil, false
		}
	}
	return units, true
}

// oneGlyph tells whether a character is one glyph of its own in any font: a
// character of a script that needs no shaping, and not a mark, a control or
// half of a surrogate pair. A character of another script can join with its
// neighbors, so the count of the glyphs does not show which glyph is which.
func oneGlyph(r rune) bool {
	switch {
	case utf16.IsSurrogate(r), unicode.IsControl(r), unicode.Is(unicode.M, r):
		return false
	case r >= 0xe000 && r <= 0xf8ff:
		// the private use area, where a symbol font maps its byte codes
		return true
	case r >= 0xac00 && r <= 0xd7a3:
		// the Hangul syllables, not the jamo that combine
		return true
	}
	return unicode.In(r, unicode.Latin, unicode.Greek, unicode.Cyrillic, unicode.Common,
		unicode.Han, unicode.Hiragana, unicode.Katakana)
}
