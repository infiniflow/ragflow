// Copied from github.com/mgilbir/gowemf, file playback_plus_text.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"fmt"
	"math"
	"unicode/utf16"
)

// plusFontRequest realizes an EmfPlusFont (MS-EMFPLUS 2.2.1.3) in world
// units: World sizes are already world units, and physical sizes are converted
// to device pixels with the header DPI and then into the page's units, so the
// world transform still scales and rotates them. The em is a vertical measure,
// converted with the vertical DPI.
func (p *player) plusFontRequest(r Record, f PlusFont) (FontRequest, error) {
	if !finite(f.EmSize) || f.EmSize <= 0 {
		return FontRequest{}, malformed(r.Offset, "EMF+ font size")
	}
	em := f.EmSize
	if f.Unit != 0 {
		u, ok := p.unitScale(f.Unit)
		page, pageOK := p.unitScale(uint32(p.plus.pageUnit))
		if !ok || !pageOK {
			return FontRequest{}, p.unsupported(r, "EMF+ font size unit without a defined size")
		}
		em = em * u.Y / (page.Y * p.plus.pageScale)
	}
	if !finite(em) || em == 0 {
		return FontRequest{}, malformed(r.Offset, "EMF+ font size")
	}
	name := make([]uint16, len(f.FamilyUTF16)/2)
	for i := range name {
		name[i] = u16(f.FamilyUTF16[2*i:])
	}
	req := FontRequest{
		FaceName:  string(utf16.Decode(name)),
		Height:    -math.Abs(em),
		Weight:    400,
		Italic:    f.Style&2 != 0,
		Underline: f.Style&4 != 0,
		StrikeOut: f.Style&8 != 0,
		CharSet:   1, // DEFAULT_CHARSET: EMF+ text is Unicode
	}
	if f.Style&1 != 0 {
		req.Weight = 700
	}
	return req, nil
}

// plusDriverString draws an EmfPlusDrawDriverString record (MS-EMFPLUS
// 2.3.4.6): glyphs or Unicode code units at explicit baseline origins in world
// space, or at the first origin advanced by the font's measured advances. The
// optional matrix is supported when it is a translation, which every reading
// of "applied to each value in the text array" agrees on.
func (p *player) plusDriverString(c Command, v PlusDriverString) error {
	r := c.Source
	tb, ok := backendAs[TextBackend](p.backend)
	switch {
	case !ok:
		return p.unsupported(r, "text output")
	case v.Options&2 != 0:
		return p.unsupported(r, "EMF+ vertical driver string")
	case v.HasMatrix && linear(v.Matrix) != Identity():
		return p.unsupported(r, "EMF+ driver-string transform other than a translation")
	}
	n := v.Glyphs.Len()
	if n == 0 {
		return nil
	}
	m, err := p.plusMatrix(r)
	if err != nil {
		return err
	}
	if v.HasMatrix {
		m = Matrix{M11: 1, M22: 1, Dx: v.Matrix.Dx, Dy: v.Matrix.Dy}.Then(m)
		if !m.Finite() {
			return malformed(r.Offset, "non-finite EMF+ driver-string transform")
		}
	}
	obj, err := p.plusObjectAt(r, uint32(v.FontID))
	if err != nil {
		return err
	}
	font, err := p.plusFontRequest(r, obj.value.(PlusFont))
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
	run := TextRun{Font: font, Text: make([]uint16, n), Glyphs: v.Options&1 == 0, Transform: m}
	for i := range run.Text {
		run.Text[i] = uint16(v.Glyphs.At(i))
	}
	// ivy: glyph indexes draw the text of the GDI fallback of the record
	// (change 19)
	if run.Glyphs {
		if units, ok := p.textOfGlyphs(r.Offset, nil, n, false); ok {
			run.Text, run.Glyphs = units, false
		}
	}
	metrics, err := tb.MeasureText(run)
	if err != nil {
		return err
	}
	if len(metrics.Advances) != n || !(metrics.Ascent >= 0) || !(metrics.Descent >= 0) || !finite(metrics.Ascent) || !finite(metrics.Descent) {
		return fmt.Errorf("gowemf: text backend returned %d advances for %d elements, ascent %v, descent %v", len(metrics.Advances), n, metrics.Ascent, metrics.Descent)
	}
	run.Origins = make([]Point, n)
	run.Advances = make([]float64, n)
	if v.Options&4 != 0 {
		// RealizedAdvance: only the first position is meaningful.
		x := v.Positions.At(0)
		for i := range run.Origins {
			run.Origins[i] = x
			x.X += metrics.Advances[i]
		}
	} else {
		for i := range run.Origins {
			run.Origins[i] = v.Positions.At(i)
		}
	}
	// Advances span each origin to the next along the baseline; the last
	// element advances by its measured width, which bounds decorations.
	for i := 0; i < n-1; i++ {
		run.Advances[i] = run.Origins[i+1].X - run.Origins[i].X
	}
	run.Advances[n-1] = metrics.Advances[n-1]
	for i := range run.Origins {
		if o := run.Origins[i]; !finite(o.X) || !finite(o.Y) || !finite(run.Advances[i]) {
			return malformed(r.Offset, "non-finite EMF+ glyph position")
		}
	}
	run.Paint = *paint
	return tb.DrawText(run, p.plusCurrentClip())
}
