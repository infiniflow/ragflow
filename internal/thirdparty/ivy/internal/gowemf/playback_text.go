// Copied from github.com/mgilbir/gowemf, file playback_text.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"fmt"
	"math"
	"unicode/utf16"
)

// FontRequest is a logical font as the playback device context selected it.
// Height and Width are in text-space units and keep LOGFONT semantics: a
// negative Height is the em (character) height, a positive Height the cell
// height including internal leading, and zero asks for a default size. A zero
// Width keeps the face's aspect ratio. Orientation rotates each glyph relative
// to the baseline, in radians counterclockwise as displayed (GM_ADVANCED
// only). Stock is the stock object index (10–17) for stock fonts, otherwise 0.
// FaceName is decoded; flags and enumerations keep their LOGFONT values.
type FontRequest struct {
	FaceName                                                      string
	Height, Width, Orientation                                    float64
	Weight                                                        int32
	Italic, Underline, StrikeOut                                  bool
	CharSet, OutPrecision, ClipPrecision, Quality, PitchAndFamily uint8
	Stock                                                         uint32
}

// TextRun is one string in text space: x runs along the baseline and y down
// from it, in the units of Font. Text holds UTF-16 code units in logical
// order, or glyph indexes of the selected font when Glyphs is set. Transform
// maps text space to destination coordinates; it can rotate, scale
// anisotropically and, under GM_ADVANCED, reflect. Origins are the baseline
// origins of each element in text space, already in display order, and
// Advances the spacing Play applied to each one (explicit, or measured plus
// character and justification extra); underline and strikeout span from Left,
// the origin of the element displayed first, over the sum of Advances.
//
// Levels, when not nil, holds each element's resolved bidirectional embedding
// level (UAX #9 after rule L1); odd levels read right to left. Play has
// already reordered the elements (rule L2) into Origins, placing an element
// at an odd level so that its measured advance ends at the right end of the
// advance it was given. A backend draws each element at its origin, shapes each maximal run of logically adjacent
// elements of one level in that level's direction, and depicts characters
// with the Bidi_Mirrored property mirrored at odd levels (rule L4). Levels is
// nil when every element is at level 0. MeasureText receives a run with
// Levels but without Origins, Advances, Left or Paint.
type TextRun struct {
	Font      FontRequest
	Text      []uint16
	Glyphs    bool
	Levels    []uint8
	Origins   []Point
	Advances  []float64
	Left      float64
	Transform Matrix
	Paint     Paint
}

// TextMetrics are the backend's measurements of a run in text-space units:
// Ascent and Descent of the realized font, both non-negative, and the advance
// of every element of Text (zero for the second unit of a surrogate pair).
type TextMetrics struct {
	Ascent, Descent float64
	Advances        []float64
}

// TextBackend is a Backend that can measure and draw text. Play performs the
// GDI placement itself (alignment, explicit and default spacing, background
// boxes, opaque and clip rectangles, current-position updates) using the
// backend's metrics; the backend realizes the font, shapes glyphs and draws
// underline and strikeout. Backends without it get text reported unsupported.
type TextBackend interface {
	Backend
	MeasureText(run TextRun) (TextMetrics, error)
	DrawText(run TextRun, clip Clip) error
}

type gdiFont struct {
	request                 FontRequest // Height and Width in logical units
	escapement, orientation float64     // tenths of a degree
	unsupportedWhy          string
}

const systemFont = 13

// Stock fonts per MS-EMF 2.1.31. The faces of the implementation-dependent
// fonts are left to the backend; their text uses the system character set.
var stockFonts = func() (s [20]*gdiFont) {
	for n, cs := range map[uint32]uint8{10: 255, 11: 0, 12: 0, 13: 1, 14: 1, 16: 1, 17: 1} {
		f := &gdiFont{request: FontRequest{Stock: n, CharSet: cs}}
		if n == 10 || n == 11 || n == 16 {
			f.request.PitchAndFamily = 1 // FIXED_PITCH
		} else if n == 12 {
			f.request.PitchAndFamily = 2 // VARIABLE_PITCH
		}
		s[n] = f
	}
	return
}()

func newFont(v Font, format Format) *gdiFont {
	f := &gdiFont{escapement: float64(v.Escapement), orientation: float64(v.Orientation)}
	f.request = FontRequest{
		Height: float64(v.Height), Width: float64(v.Width), Weight: v.Weight,
		Italic: v.Italic != 0, Underline: v.Underline != 0, StrikeOut: v.StrikeOut != 0,
		CharSet: v.CharSet, OutPrecision: v.OutPrecision, ClipPrecision: v.ClipPrecision,
		Quality: v.Quality, PitchAndFamily: v.PitchAndFamily,
	}
	if format == EMF {
		var units []uint16
		for i := 0; i+1 < len(v.FaceName); i += 2 {
			u := u16(v.FaceName[i:])
			if u == 0 {
				break
			}
			units = append(units, u)
		}
		f.request.FaceName = string(utf16.Decode(units))
	} else {
		name := v.FaceName
		for i, b := range name {
			if b == 0 {
				name = name[:i]
				break
			}
		}
		units := make([]uint16, len(name))
		for i, b := range name {
			units[i] = uint16(b)
		}
		if !ascii(name) {
			var ok bool
			if units, _, ok = decodeANSI(charSetCodePage[v.CharSet], name); !ok {
				f.unsupportedWhy = "font face name in an undecoded character set"
			}
		}
		f.request.FaceName = string(utf16.Decode(units))
	}
	if len(f.request.FaceName) > 0 && f.request.FaceName[0] == '@' {
		f.unsupportedWhy = "vertical font"
	}
	return f
}

// charSetCodePage maps CharacterSet values (MS-WMF 2.1.1.5) to their Windows
// ANSI code pages, as Windows' TranslateCharsetInfo documents.
var charSetCodePage = map[uint8]uint16{0: 1252, 128: 932, 129: 949, 134: 936, 136: 950, 161: 1253, 162: 1254, 163: 1258, 177: 1255, 178: 1256, 186: 1257, 204: 1251, 222: 874, 238: 1250}

func ascii(b []byte) bool {
	for _, c := range b {
		if c >= 0x80 {
			return false
		}
	}
	return true
}

// decodeText converts a record's string to UTF-16 code units, or glyph
// indexes. spans, when not nil, gives the number of string bytes each unit
// came from (double-byte characters take two). Charsets that cannot be
// decoded are not interpreted (MS-EMF 2.2.13).
func (p *player) decodeText(t Text, f *gdiFont, wide bool) ([]uint16, []int, bool, string) {
	glyphs := t.Options&0x10 != 0
	switch {
	case t.SmallChars:
		if glyphs {
			return nil, nil, false, "8-bit glyph indexes"
		}
		out := make([]uint16, len(t.Bytes))
		for i, b := range t.Bytes {
			out[i] = uint16(b)
		}
		return out, nil, false, ""
	case wide:
		out := make([]uint16, len(t.Bytes)/2)
		for i := range out {
			out[i] = u16(t.Bytes[i*2:])
		}
		return out, nil, glyphs, ""
	case glyphs:
		return nil, nil, false, "8-bit glyph indexes"
	}
	cs := f.request.CharSet
	if cs == 1 {
		if p.options.DefaultCharSet == nil {
			return nil, nil, false, "DEFAULT_CHARSET text without PlayOptions.DefaultCharSet"
		}
		cs = *p.options.DefaultCharSet
	}
	if cs == 2 {
		// Symbol fonts map their byte codes into the private-use area.
		out := make([]uint16, len(t.Bytes))
		for i, b := range t.Bytes {
			out[i] = 0xf000 | uint16(b)
		}
		return out, nil, false, ""
	}
	out, spans, ok := decodeANSI(charSetCodePage[cs], t.Bytes)
	if !ok {
		return nil, nil, false, fmt.Sprintf("text in character set %d", cs)
	}
	return out, spans, false, ""
}

// textSpace is the frame text is laid out in. Under GM_COMPATIBLE it is device
// space rotated by the escapement, with text upright and only the font height
// scaled (MS-EMF 2.1.16); under GM_ADVANCED it is world space rotated by the
// escapement, and the full transform applies to the glyphs.
type textSpace struct {
	toDest   Matrix // text space at the reference point to destination
	toLogic  Matrix // text space at the reference point to logical
	scale    Point  // logical units to text units along and across the baseline
	advanced bool
}

func (p *player) textSpace(r Record, ref Point, f *gdiFont, advanced bool) (textSpace, error) {
	theta := f.escapement / 10 * math.Pi / 180
	s, c := math.Sincos(theta)
	rot := Matrix{M11: c, M12: -s, M21: s, M22: c}
	var ts textSpace
	if advanced {
		m, err := p.toDestination(r)
		if err != nil {
			return ts, err
		}
		rot.Dx, rot.Dy = ref.X, ref.Y
		ts = textSpace{toDest: rot.Then(m), toLogic: rot, scale: Point{1, 1}, advanced: true}
	} else {
		page := p.dc.pageMatrix()
		d := page.Apply(ref)
		rot.Dx, rot.Dy = d.X, d.Y
		inv := Matrix{M11: 1 / page.M11, M22: 1 / page.M22, Dx: -page.Dx / page.M11, Dy: -page.Dy / page.M22}
		ts = textSpace{toDest: rot.Then(p.base), toLogic: rot.Then(inv), scale: Point{math.Abs(page.M11), math.Abs(page.M22)}}
	}
	if !ts.toDest.Finite() || !ts.toLogic.Finite() {
		return ts, malformed(r.Offset, "non-finite text transform")
	}
	return ts, nil
}

// text plays ExtTextOut, TextOut and SmallTextOut strings.
func (p *player) text(c Command, v Text, wide bool) error {
	return p.textString(c, v, wide)
}

func (p *player) polyText(c Command, v PolyText, wide bool) error {
	for _, t := range v.Strings {
		if err := p.textString(c, t, wide); err != nil && err != errSkip {
			return err
		}
	}
	return nil
}

func (p *player) textString(c Command, v Text, wide bool) error {
	r := c.Source
	hasRect := v.HasRectangle && v.Options&6 != 0
	if len(v.Bytes) == 0 && !hasRect {
		return nil
	}
	if p.constructing {
		return p.unsupported(r, "text output in a path bracket")
	}
	if err := p.drawable(r, c.ColorState); err != nil {
		return err
	}
	advanced := p.advanced()
	if r.Format == EMF {
		// EMF text records state the graphics mode they were recorded in.
		advanced = v.GraphicsMode == 2
	}
	clip := p.currentClip()
	if hasRect {
		m, err := p.toDestination(r)
		if err != nil {
			return err
		}
		box := rectPath(v.Rectangle, m)
		if v.Options&2 != 0 {
			col, err := p.color(r, p.dc.bkColor)
			if err != nil {
				return err
			}
			if err := p.backend.FillPath(box, NonZero, Paint{Kind: PaintSolid, Color: col}, clip); err != nil {
				return err
			}
		}
		if v.Options&4 != 0 {
			clip = append(clip[:len(clip):len(clip)], &ClipRegion{Op: ClipReplace, Area: box, Rule: NonZero, depth: 1})
		}
	}
	if len(v.Bytes) == 0 {
		return nil
	}
	tb, ok := backendAs[TextBackend](p.backend)
	if !ok {
		return p.unsupported(r, "text output")
	}
	f := p.dc.font.font
	if f.unsupportedWhy != "" {
		return p.unsupported(r, f.unsupportedWhy)
	}
	text, spans, glyphs, why := p.decodeText(v, f, wide)
	if why != "" {
		return p.unsupported(r, why)
	}
	// ETO_RTLREADING and TA_RTLREADING select right-to-left reading order
	// (MS-EMF 2.1.11, MS-WMF 2.1.2.3).
	rtl := v.Options&0x80 != 0 || p.dc.textAlign&0x100 != 0
	// ivy: glyph indexes draw the text that another record gives for them
	// (change 19). indexed keeps the spacing of the indexes.
	indexed := glyphs
	if glyphs {
		if units, ok := p.textOfGlyphs(r.Offset, f, len(text), rtl); ok {
			text, glyphs = units, false
		}
	}
	levels := textLevels(text, glyphs, rtl)
	updateCP := p.dc.textAlign&1 != 0
	ref := v.Reference
	if updateCP {
		ref = p.dc.position
	}
	ts, err := p.textSpace(r, ref, f, advanced)
	if err != nil {
		return err
	}
	run := TextRun{Font: f.request, Text: text, Glyphs: glyphs, Levels: levels, Transform: ts.toDest}
	run.Font.Height *= ts.scale.Y
	run.Font.Width *= ts.scale.X
	if advanced {
		run.Font.Orientation = (f.orientation - f.escapement) / 10 * math.Pi / 180
	}
	metrics, err := tb.MeasureText(run)
	if err != nil {
		return err
	}
	if len(metrics.Advances) != len(text) || !(metrics.Ascent >= 0) || !(metrics.Descent >= 0) || !finite(metrics.Ascent) || !finite(metrics.Descent) {
		return fmt.Errorf("gowemf: text backend returned %d advances for %d elements, ascent %v, descent %v", len(metrics.Advances), len(text), metrics.Ascent, metrics.Descent)
	}
	advances := make([]float64, len(text))
	if n := v.Advances.Len(); n > 0 {
		// Advances belong to the string's elements: bytes for ANSI text. A
		// double-byte character advances by the sum of its bytes' advances,
		// so every later origin stays where the record puts it.
		elements := len(text)
		if spans != nil {
			elements = len(v.Bytes)
		}
		pdy := v.Options&0x2000 != 0
		if want := elements * map[bool]int{false: 1, true: 2}[pdy]; n != want {
			return malformed(r.Offset, "text advance count")
		}
		e := 0
		for i := range advances {
			span := 1
			if spans != nil {
				span = spans[i]
			}
			for k := 0; k < span; k++ {
				if pdy {
					if v.Advances.SignedAt(2*e+1) != 0 {
						return p.unsupported(r, "vertical character displacement (ETO_PDY)")
					}
					advances[i] += float64(v.Advances.SignedAt(2*e)) * ts.scale.X
				} else {
					advances[i] += float64(v.Advances.SignedAt(e)) * ts.scale.X
				}
				e++
			}
		}
	} else {
		// Default spacing: the font's advances plus the character extra and
		// the justification break extra. Under GM_COMPATIBLE these are
		// transformed and rounded to whole device pixels (MS-WMF 2.3.5.25,
		// 2.3.5.27); the break extra goes to spaces, the first remainder
		// spaces receiving one more unit.
		round := func(v float64) float64 {
			if ts.advanced {
				return v
			}
			return math.Round(v)
		}
		extra := round(float64(p.dc.charExtra) * ts.scale.X)
		var perBreak, remainder float64
		if p.dc.breakCount > 0 && !indexed {
			total := round(float64(p.dc.breakExtra) * ts.scale.X)
			perBreak = math.Trunc(total / float64(p.dc.breakCount))
			remainder = total - perBreak*float64(p.dc.breakCount)
		}
		for i, a := range metrics.Advances {
			if !finite(a) {
				return fmt.Errorf("gowemf: text backend returned a non-finite advance")
			}
			advances[i] = a + extra
			if !indexed && text[i] == ' ' && (perBreak != 0 || remainder != 0) {
				advances[i] += perBreak
				if remainder >= 1 {
					advances[i]++
					remainder--
				} else if remainder <= -1 {
					advances[i]--
					remainder++
				}
			}
		}
	}
	var width float64
	for _, a := range advances {
		width += a
	}
	var x0, baseline float64
	switch p.dc.textAlign & 6 {
	case 6:
		x0 = -width / 2
	case 2:
		x0 = -width
	}
	switch p.dc.textAlign & 24 {
	case 24:
	case 8:
		baseline = -metrics.Descent
	default:
		baseline = metrics.Ascent
	}
	if p.dc.bkMode == 2 {
		// OPAQUE background mode fills the character cells.
		col, err := p.color(r, p.dc.bkColor)
		if err != nil {
			return err
		}
		cell := pathBuilder{limit: 4}
		s := shape{&cell, ts.toDest}
		top, bottom := baseline-metrics.Ascent, baseline+metrics.Descent
		s.moveTo(Point{x0, top})
		s.lineTo(Point{x0 + width, top})
		s.lineTo(Point{x0 + width, bottom})
		s.lineTo(Point{x0, bottom})
		cell.close()
		if width != 0 {
			if err := p.backend.FillPath(cell.path, NonZero, Paint{Kind: PaintSolid, Color: col}, clip); err != nil {
				return err
			}
		}
	}
	col, err := p.color(r, p.dc.textColor)
	if err != nil {
		return err
	}
	run.Paint = Paint{Kind: PaintSolid, Color: col}
	run.Advances = advances
	run.Origins = make([]Point, len(text))
	run.Left = x0
	x := x0
	for _, i := range displayOrder(text, glyphs, levels) {
		// The pen moves against the reading direction at odd levels, so a
		// right-to-left element ends at the right of the advance it is
		// given: spacing beyond its measured advance falls on its left.
		o := x
		if levels != nil && levels[i]%2 == 1 {
			o += advances[i] - metrics.Advances[i]
		}
		run.Origins[i] = Point{o, baseline}
		x += advances[i]
	}
	if err := tb.DrawText(run, clip); err != nil {
		return err
	}
	if updateCP {
		// The current position moves to the end of the string in its
		// drawing direction; centered text leaves it in place.
		var end float64
		switch p.dc.textAlign & 6 {
		case 0:
			end = width
		case 2:
			end = -width
		}
		p.dc.position = ts.toLogic.Apply(Point{end, 0})
	}
	return nil
}

func rectPath(v Rect, m Matrix) Path {
	l, t, r, b := normalize(v)
	path := pathBuilder{limit: 4}
	s := shape{&path, m}
	s.moveTo(Point{l, t})
	s.lineTo(Point{r, t})
	s.lineTo(Point{r, b})
	s.lineTo(Point{l, b})
	path.close()
	return path.path
}
