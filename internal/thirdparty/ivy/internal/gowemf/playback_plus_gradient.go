// Copied from github.com/mgilbir/gowemf, file playback_plus_gradient.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"image/color"
	"math"
)

// plusPathGradientFill fills path with a path gradient brush (MS-EMFPLUS
// 2.2.2.29): the color changes along each line from the center point to the
// boundary, from the center color to the boundary color, which varies
// linearly between boundary points. For a boundary that is star-shaped from
// the center this is exactly a fan of Gouraud triangles. The area outside the
// boundary is not painted, as WrapModeClamp specifies.
//
// Blend factors and preset colors (2.2.2.4, 2.2.2.5) give the color at
// positions along each of those lines. MS-EMFPLUS says only that they run
// from a "midpoint" to an "endpoint"; Microsoft's GDI+ reference
// (PathGradientBrush::SetBlend and SetInterpolationColors) settles it:
// position 0 is the boundary and 1 the center point, a factor is how far the
// color has moved from the boundary color to the center color, and preset
// colors replace both. Within each fan triangle the position is an affine
// coordinate, so splitting it at the positions gives exact Gouraud bands,
// provided the color depends on the position alone: with blend factors the
// surrounding color must be uniform.
//
// Reported instead: point boundaries, which MS-EMFPLUS 2.2.2.7 calls a closed
// cardinal spline without giving its tension; blend factors with differing
// surrounding colors; focus scales, whose scaling origin 2.2.2.18 does not
// give; gamma correction; tiled wraps; colors of differing alpha; and
// boundaries that are not star-shaped from the center.
func (p *player) plusPathGradientFill(r Record, g *PlusPathGradient, fill Path, rule FillRule, m Matrix, clip Clip) error {
	gb, ok := backendAs[GradientBackend](p.backend)
	switch {
	case g == nil:
		return malformed(r.Offset, "EMF+ path gradient brush")
	case !ok:
		return p.unsupported(r, "EMF+ path gradient without a GradientBackend")
	case g.Flags&1 == 0 || g.BoundaryPath == nil:
		return p.unsupported(r, "EMF+ path gradient with a point (cardinal spline) boundary")
	case g.Flags&0x40 != 0:
		return p.unsupported(r, "EMF+ path gradient focus scales")
	case g.Flags&0x80 != 0:
		return p.unsupported(r, "EMF+ gamma-corrected gradient")
	case g.WrapMode != uint32(WrapClamp):
		return p.unsupported(r, "EMF+ tiled path gradient")
	}
	bm := g.Transform.Then(m)
	if !bm.Finite() {
		return malformed(r.Offset, "non-finite EMF+ path gradient transform")
	}
	boundary := *g.BoundaryPath
	curved := false
	for _, t := range boundary.Types {
		curved = curved || t&7 == 3
	}
	n := g.SurroundingColors.Len()
	if n != 1 && (curved || n != boundary.Points.Len()) {
		// Colors belong to path points; on curves the points include
		// control points, whose role MS-EMFPLUS does not give.
		return p.unsupported(r, "EMF+ path gradient surrounding colors not one per boundary vertex")
	}
	center := argb(g.CenterColor)
	colors := make([]color.NRGBA, boundary.Points.Len())
	uniform := true
	for i := range colors {
		colors[i] = argb(g.SurroundingColors.At(min(i, n-1)))
		uniform = uniform && colors[i] == colors[0]
		if colors[i].A != center.A {
			return p.unsupported(r, "EMF+ gradient with varying alpha")
		}
	}
	// stops gives the color at boundary-to-center positions when it does
	// not depend on the surrounding colors' spread.
	var stops []GradientStop
	switch {
	case g.Flags&0x04 != 0:
		k := len(g.PresetPositions)
		if k < 2 || g.PresetPositions[0] != 0 || g.PresetPositions[k-1] != 1 {
			return malformed(r.Offset, "EMF+ preset color positions")
		}
		for i, pos := range g.PresetPositions {
			stops = append(stops, GradientStop{pos, argb(g.PresetColors.At(i))})
		}
	case g.Flags&0x08 != 0:
		if !uniform {
			return p.unsupported(r, "EMF+ path gradient blend factors with several surrounding colors")
		}
		s := colors[0]
		for i, pos := range g.Blend.Positions {
			f := g.Blend.Factors[i]
			l := func(a, b uint8) uint8 { return uint8(math.Round(float64(a) + (float64(b)-float64(a))*f)) }
			stops = append(stops, GradientStop{pos, color.NRGBA{l(s.R, center.R), l(s.G, center.G), l(s.B, center.B), s.A}})
		}
	}
	for i, s := range stops {
		if i > 0 && s.Offset < stops[i-1].Offset {
			return malformed(r.Offset, "EMF+ gradient positions out of order")
		}
		if s.Color.A != stops[0].Color.A {
			return p.unsupported(r, "EMF+ gradient with varying alpha")
		}
	}
	if len(stops) > 0 {
		center = stops[len(stops)-1].Color
	}
	if p.plus.compositing == 1 && center.A != 255 {
		return p.unsupported(r, "EMF+ SourceCopy compositing of translucent paint")
	}
	path, err := p.plusPath(r, boundary, bm, true)
	if err != nil {
		return err
	}
	b := pathBuilder{path: path, limit: p.options.MaxPathPoints}
	if curved {
		b.flatten()
		if b.err {
			return failure(r.Offset, "path points", ErrLimit)
		}
	}
	// One closed figure of points, each with its color.
	var pts []Point
	var cols []color.NRGBA
	for i, v := range b.path.Verbs {
		switch {
		case v == PathMoveTo && i != 0:
			return p.unsupported(r, "EMF+ path gradient boundary with several figures")
		case v == PathMoveTo || v == PathLineTo:
			q := b.path.Points[len(pts)]
			c := colors[0]
			if !curved {
				c = colors[len(pts)]
			}
			pts, cols = append(pts, q), append(cols, c)
		}
	}
	c := bm.Apply(g.Center)
	mesh := make([]GradientTriangle, 0, len(pts))
	var turn float64
	sign, edges := 0.0, 0
	wide := func(c color.NRGBA) color.NRGBA64 {
		return color.NRGBA64{uint16(c.R) * 257, uint16(c.G) * 257, uint16(c.B) * 257, uint16(c.A) * 257}
	}
	for i := range pts {
		a, e := pts[i], pts[(i+1)%len(pts)]
		if a == e {
			continue
		}
		u, v := Point{a.X - c.X, a.Y - c.Y}, Point{e.X - c.X, e.Y - c.Y}
		cross := u.X*v.Y - u.Y*v.X
		if cross == 0 || (sign != 0 && math.Signbit(cross) != math.Signbit(sign)) {
			return p.unsupported(r, "EMF+ path gradient boundary not star-shaped from its center")
		}
		sign = cross
		turn += math.Atan2(cross, u.X*v.X+u.Y*v.Y)
		edges++
		if stops == nil {
			mesh = append(mesh, GradientTriangle{Points: [3]Point{c, a, e}, Colors: [3]color.NRGBA64{wide(center), wide(cols[i]), wide(cols[(i+1)%len(pts)])}})
			continue
		}
		// Bands between consecutive positions; at position t the line
		// from the center to a boundary point B is at C+(1-t)(B-C).
		at := func(q Point, t float64) Point { return Point{c.X + (1-t)*(q.X-c.X), c.Y + (1-t)*(q.Y-c.Y)} }
		for k := 0; k+1 < len(stops); k++ {
			t0, t1 := stops[k].Offset, stops[k+1].Offset
			if t0 == t1 {
				continue
			}
			c0, c1 := wide(stops[k].Color), wide(stops[k+1].Color)
			a0, e0, a1, e1 := at(a, t0), at(e, t0), at(a, t1), at(e, t1)
			mesh = append(mesh, GradientTriangle{Points: [3]Point{a0, e0, e1}, Colors: [3]color.NRGBA64{c0, c0, c1}})
			if t1 != 1 {
				mesh = append(mesh, GradientTriangle{Points: [3]Point{a0, e1, a1}, Colors: [3]color.NRGBA64{c0, c1, c1}})
			}
		}
	}
	if edges < 3 || math.Abs(math.Abs(turn)-2*math.Pi) > 1e-6 {
		return p.unsupported(r, "EMF+ path gradient boundary not star-shaped from its center")
	}
	if err := p.spendPixels(r, len(mesh), 1); err != nil {
		return err
	}
	// The filled shape bounds the gradient as one more clip layer.
	layers := append(clip[:len(clip):len(clip)], &ClipRegion{Op: ClipReplace, Area: fill, Rule: rule, depth: 1})
	return gb.FillGradient(mesh, layers)
}
