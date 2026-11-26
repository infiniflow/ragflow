package metafile

import (
	"unicode"
	"unicode/utf16"

	"github.com/xo/okage/typesetting/di"
	"github.com/xo/okage/typesetting/language"
	"github.com/xo/okage/typesetting/shaping"
	"golang.org/x/image/math/fixed"

	"ragflow/internal/thirdparty/ivy/internal/display"
	"ragflow/internal/thirdparty/ivy/internal/font"
	"ragflow/internal/thirdparty/ivy/internal/gowemf"
)

// shapedRun is a run of text that the shaper shaped. The glyphs of a
// cluster go with the element of the first character of the cluster: its
// advance is the advance of the whole cluster, and the other elements of
// the cluster have none.
type shapedRun struct {
	// advances is the advance of each element, in the units of the text
	// space.
	advances []float64
	// glyphs holds the glyphs of the cluster of each element, at their
	// positions from the origin of the element, in the units of the text
	// space.
	glyphs [][]display.Glyph
}

// simpleScripts are the scripts whose characters are their glyphs, as long
// as the text has no combining mark. Text in another script, such as
// Arabic or Devanagari, needs the shaper. A character without a script,
// such as a character of the private use area of a symbol font, is its
// glyph.
var simpleScripts = map[language.Script]bool{
	language.Latin: true, language.Greek: true, language.Cyrillic: true,
	language.Common: true, language.Inherited: true, language.Han: true,
	language.Hiragana: true, language.Katakana: true, language.Hangul: true,
	language.Bopomofo: true, language.Unknown: true,
}

// needsShaping reports whether the characters need the shaper: a combining
// mark, or a character of a complex script.
func needsShaping(runes []rune) bool {
	for _, r := range runes {
		if r < 0x300 {
			continue
		}
		if unicode.In(r, unicode.Mn, unicode.Me) || !simpleScripts[language.LookupScript(r)] {
			return true
		}
	}
	return false
}

// shape shapes the text of run with the shaper of the font, and returns
// false when the text does not need it. GDI shapes the text of the complex
// scripts with Uniscribe. Each maximal run of logically adjacent elements
// at one level, and in one script, is shaped in the direction of its
// level. The shaper also mirrors the characters at an odd level. A run of a
// simple script without a combining mark is not shaped.
func (tf textFont) shape(run gowemf.TextRun) (out shapedRun, ok bool) {
	if run.Glyphs || len(run.Text) == 0 || tf.shapeFace == nil {
		return shapedRun{}, false
	}
	// the characters, and the element of each one
	runes := make([]rune, 0, len(run.Text))
	elems := make([]int, 0, len(run.Text))
	for i := 0; i < len(run.Text); i++ {
		r := rune(run.Text[i])
		if utf16.IsSurrogate(r) && i+1 < len(run.Text) {
			if p := utf16.DecodeRune(r, rune(run.Text[i+1])); p != unicode.ReplacementChar {
				runes, elems = append(runes, p), append(elems, i)
				i++
				continue
			}
		}
		runes, elems = append(runes, r), append(elems, i)
	}
	if !needsShaping(runes) {
		return shapedRun{}, false
	}
	level := func(k int) uint8 {
		if run.Levels == nil {
			return 0
		}
		return run.Levels[elems[k]]
	}
	out = shapedRun{
		advances: make([]float64, len(run.Text)),
		glyphs:   make([][]display.Glyph, len(run.Text)),
	}
	tf.shapeMu.Lock()
	defer tf.shapeMu.Unlock()
	defer func() {
		// go-text can panic on a malformed font
		if recover() != nil {
			out, ok = shapedRun{}, false
		}
	}()
	upem := float64(tf.shapeFace.Upem())
	if upem <= 0 {
		upem = 1000
	}
	// the shaper gives its lengths in 1/64 of a unit of the font
	sx, sy := float64(tf.em*tf.scaleX)/upem/64, float64(tf.em)/upem/64
	for start := 0; start < len(runes); {
		end, script := segment(runes, start, level)
		if !needsShaping(runes[start:end]) {
			// GDI does not kern the text of the simple scripts, and makes
			// no ligatures in it, so each character is its glyph
			for k := start; k < end; k++ {
				r := runes[k]
				if level(k)%2 == 1 {
					r = mirror(r)
				}
				gid, _ := tf.face.Glyph(r)
				e := elems[k]
				out.glyphs[e] = append(out.glyphs[e], display.Glyph{ID: gid})
				out.advances[e] = float64(tf.face.Advance(gid) * tf.em * tf.scaleX)
			}
			start = end
			continue
		}
		dir := di.DirectionLTR
		if level(start)%2 == 1 {
			dir = di.DirectionRTL
		}
		shaped := tf.shaper.Shape(shaping.Input{
			Text:      runes,
			RunStart:  start,
			RunEnd:    end,
			Direction: dir,
			Face:      tf.shapeFace,
			Size:      fixed.I(int(upem)),
			Script:    script,
		})
		// the glyphs come in the order of display, left to right, and the
		// glyphs of a cluster are next to each other
		var pen, left float64
		cluster := -1
		for _, g := range shaped.Glyphs {
			c := g.TextIndex()
			if c < start || c >= end {
				continue
			}
			if c != cluster {
				cluster, left = c, pen
			}
			e := elems[c]
			out.glyphs[e] = append(out.glyphs[e], display.Glyph{
				ID: font.GID(g.GlyphID),
				X:  float32((pen - left + float64(g.XOffset)) * sx),
				// the shaper measures y up, and the text space measures it
				// down
				Y: float32(-float64(g.YOffset) * sy),
			})
			pen += float64(g.Advance)
			out.advances[e] += float64(g.Advance) * sx
		}
		start = end
	}
	return out, true
}

// segment returns the end of the run of characters from start that share
// one level and one script, and the script. A character of the scripts
// Common and Inherited, such as a space or a mark, takes the script of the
// characters around it.
func segment(runes []rune, start int, level func(int) uint8) (int, language.Script) {
	script := language.Common
	end := start
	for ; end < len(runes) && level(end) == level(start); end++ {
		s := language.LookupScript(runes[end])
		if !s.Strong() {
			continue
		}
		if script == language.Common {
			script = s
		} else if s != script {
			break
		}
	}
	if script == language.Common {
		script = language.Latin
	}
	return end, script
}
