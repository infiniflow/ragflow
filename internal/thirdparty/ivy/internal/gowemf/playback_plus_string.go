// This file is not in github.com/mgilbir/gowemf. ivy added it to its copy
// (internal/gowemf/README.md).

package gowemf

import "math"

// The flags of an EmfPlusStringFormat object (MS-EMFPLUS 2.1.2.8).
const (
	plusStringRightToLeft = 0x0001
	plusStringVertical    = 0x0002
	plusStringNoWrap      = 0x1000
	plusStringNoClip      = 0x4000
)

// plusDrawString draws an EmfPlusDrawString record (MS-EMFPLUS 2.3.4.14).
// MS-EMFPLUS leaves the layout of GDI+ open, so this is a close reading,
// good enough to read the text:
//
//   - The text breaks at each line feed, and at spaces to fit the width of
//     the layout rectangle, unless the format sets NoWrap or the width is 0.
//   - The lines are aligned in the rectangle by the Alignment and the
//     LineAlignment of the format. RightToLeft swaps near and far.
//   - The margins of the format are fractions of the em, and its tracking
//     multiplies the advances. Without a format, the margins are 1/6 of the
//     em and the tracking is 1.03, the values of the default format of
//     GDI+.
//   - The text is clipped to the rectangle, unless the format sets NoClip
//     or the rectangle is empty.
//   - A vertical format turns the layout and the glyphs a quarter turn
//     clockwise. The lines run down, and the next line is at the left.
func (p *player) plusDrawString(c Command, v PlusText) error {
	r := c.Source
	tb, ok := backendAs[TextBackend](p.backend)
	if !ok {
		return p.unsupported(r, "text output")
	}
	n := len(v.UTF16) / 2
	if n == 0 {
		return nil
	}
	format := PlusStringFormat{LeadingMargin: 1.0 / 6, TrailingMargin: 1.0 / 6, Tracking: 1.03}
	if v.FormatID != 0xffffffff {
		obj, err := p.plusObjectAt(r, v.FormatID)
		if err != nil {
			return err
		}
		f, ok := obj.value.(PlusStringFormat)
		if !ok {
			return nil // Stream drops a reference of another type
		}
		format = f
	}
	if !finite(format.Tracking) || format.Tracking <= 0 {
		format.Tracking = 1
	}
	m, err := p.plusMatrix(r)
	if err != nil {
		return err
	}
	box := v.Layout
	// clipBox is the layout rectangle in the world space, for the clip.
	clipBox, clipM := box, m
	if format.Flags&plusStringVertical != 0 {
		// a vertical format turns the layout a quarter turn clockwise: the
		// lines run down, and the first line is at the right of the
		// rectangle
		turn := Matrix{M12: 1, M21: -1, Dx: box.X + box.Width, Dy: box.Y}
		m = turn.Then(m)
		box = Box{Width: box.Height, Height: box.Width}
		if !m.Finite() {
			return malformed(r.Offset, "non-finite EMF+ string layout")
		}
	}
	obj, err := p.plusObjectAt(r, uint32(v.FontID))
	if err != nil {
		return err
	}
	f, ok := obj.value.(PlusFont)
	if !ok {
		return nil
	}
	font, err := p.plusFontRequest(r, f)
	if err != nil {
		return err
	}
	paint, err := p.plusPaint(r, v.BrushID, v.Solid, m)
	if err != nil || paint == nil {
		return err
	}
	if err := p.plusComposite(r, paint); err != nil {
		return err
	}
	text := make([]uint16, n)
	for i := range text {
		text[i] = u16(v.UTF16[2*i:])
	}
	metrics, err := tb.MeasureText(TextRun{Font: font, Text: text, Transform: m})
	if err != nil {
		return err
	}
	if len(metrics.Advances) != n || !(metrics.Ascent >= 0) || !(metrics.Descent >= 0) || !finite(metrics.Ascent) || !finite(metrics.Descent) {
		return nil
	}
	advances := make([]float64, n)
	for i, a := range metrics.Advances {
		if !finite(a) {
			return nil
		}
		advances[i] = a * format.Tracking
	}
	em := math.Abs(font.Height)
	lead, trail := format.LeadingMargin*em, format.TrailingMargin*em
	if !finite(lead) || !finite(trail) {
		lead, trail = 0, 0
	}
	width := box.Width - lead - trail
	wrap := box.Width > 0 && format.Flags&plusStringNoWrap == 0
	lines := plusBreakLines(text, advances, width, wrap)
	lineHeight := metrics.Ascent + metrics.Descent
	align, lineAlign := format.Alignment, format.LineAlignment
	if format.Flags&plusStringRightToLeft != 0 && align != 1 {
		align = 2 - min(align, 2)
	}
	top := box.Y
	if box.Height > 0 {
		total := float64(len(lines)) * lineHeight
		switch lineAlign {
		case 1:
			top += (box.Height - total) / 2
		case 2:
			top += box.Height - total
		}
	}
	clip := p.plusCurrentClip()
	if format.Flags&plusStringNoClip == 0 && box.Width > 0 && box.Height > 0 {
		clip = append(clip, &ClipRegion{Op: ClipReplace, Area: boxPath(clipBox, clipM), Rule: NonZero, depth: 1})
	}
	for i, line := range lines {
		if line.end <= line.start {
			continue
		}
		x := box.X + lead
		if box.Width > 0 {
			switch align {
			case 1:
				x += (width - line.width) / 2
			case 2:
				x += width - line.width
			}
		}
		y := top + float64(i)*lineHeight + metrics.Ascent
		k := line.end - line.start
		run := TextRun{Font: font, Text: text[line.start:line.end], Transform: m, Paint: *paint,
			Origins: make([]Point, k), Advances: advances[line.start:line.end]}
		for j := range run.Origins {
			run.Origins[j] = Point{x, y}
			x += run.Advances[j]
		}
		if !finite(x) || !finite(y) {
			return malformed(r.Offset, "non-finite EMF+ string position")
		}
		if err := tb.DrawText(run, clip); err != nil {
			return err
		}
	}
	return nil
}

// plusLine is a line of text from start to end, of the width of its
// characters without the spaces at its end.
type plusLine struct {
	start, end int
	width      float64
}

// plusBreakLines breaks text into lines at line feeds, and, if wrap is set,
// at spaces so that each line fits width. A word wider than width takes a
// line of its own. The line feeds and the carriage returns are left out of
// the lines.
func plusBreakLines(text []uint16, advances []float64, width float64, wrap bool) []plusLine {
	var lines []plusLine
	measure := func(start, end int) float64 {
		for end > start && (text[end-1] == ' ' || text[end-1] == '\r') {
			end--
		}
		w := 0.0
		for _, a := range advances[start:end] {
			w += a
		}
		return w
	}
	add := func(start, end int) {
		for end > start && text[end-1] == '\r' {
			end--
		}
		lines = append(lines, plusLine{start, end, measure(start, end)})
	}
	start := 0
	for i := 0; i <= len(text); i++ {
		if i < len(text) && text[i] != '\n' {
			continue
		}
		if !wrap {
			add(start, i)
			start = i + 1
			continue
		}
		// break at the last space before the line is too wide. w is the
		// width from lineStart to j. before is the width up to the last
		// space.
		lineStart, lastSpace := start, -1
		w, before := 0.0, 0.0
		for j := start; j < i; j++ {
			if text[j] == ' ' {
				lastSpace, before = j, w
			}
			w += advances[j]
			if w > width && text[j] != ' ' && lastSpace > lineStart {
				add(lineStart, lastSpace)
				w -= before + advances[lastSpace]
				lineStart, lastSpace = lastSpace+1, -1
			}
		}
		add(lineStart, i)
		start = i + 1
	}
	return lines
}
