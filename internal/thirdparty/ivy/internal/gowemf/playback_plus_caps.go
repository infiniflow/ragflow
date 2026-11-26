// Copied from github.com/mgilbir/gowemf, file playback_plus_caps.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import "math"

// Custom line caps (MS-EMFPLUS 2.2.1.2) are drawn only with
// PlayOptions.CustomLineCaps, because neither MS-EMFPLUS nor Microsoft's
// GDI+ reference defines their geometry. The interpretation used, unverified
// against Windows:
//
//   - A cap is built in cap space: the origin is the end point of the figure
//     and +y points outward along the line (back along the first segment for
//     a start cap); +x is +y turned a quarter clockwise in pen space as
//     displayed. Cap units are the pen width times the cap's WidthScale.
//   - A path cap fills its fill path with the winding rule, or strokes its
//     line path with the pen width and the cap's stroke caps and join; when
//     both are present the line path wins, as Microsoft's CustomLineCap
//     reference states. The figure itself ends with the cap's BaseCap and is
//     shortened by BaseInset cap units.
//   - An adjustable arrow has its vertex at the end point, its base Width
//     cap units wide and Height cap units back along the line, and the
//     base's midpoint moved MiddleInset units toward the vertex (Microsoft's
//     AdjustableArrowCap reference). It is filled, or outlined with the pen
//     when FillState is clear; the figure ends flat at the base midpoint.

// plusCustomCaps checks a pen's custom caps and returns the base caps used
// for the figure's own ends.
func (p *player) plusCustomCaps(r Record, pen PlusPen, startCap, endCap LineCap) (caps [2]*PlusCustomLineCap, base [2]LineCap, err error) {
	base = [2]LineCap{startCap, endCap}
	if !p.options.CustomLineCaps {
		return caps, base, p.unsupported(r, "EMF+ custom line cap")
	}
	for i, c := range [2]struct {
		value  uint32
		custom *PlusCustomLineCap
	}{{pen.StartCap, pen.CustomStartCap}, {pen.EndCap, pen.CustomEndCap}} {
		if (c.value == 0xff) != (c.custom != nil) {
			return caps, base, p.unsupported(r, "EMF+ custom line cap data without LineCapTypeCustom, or the reverse")
		}
		if c.custom == nil {
			continue
		}
		caps[i] = c.custom
		switch {
		case c.custom.Default != nil:
			d := c.custom.Default
			b, ok := plusCaps[d.BaseCap]
			if !ok {
				return caps, base, p.unsupported(r, "EMF+ custom line cap with an unspecified base cap")
			}
			_, okS := plusCaps[d.StrokeStartCap]
			_, okE := plusCaps[d.StrokeEndCap]
			if d.Flags&3 == 0 || (d.Flags&2 != 0 && (!okS || !okE || d.StrokeJoin > 2)) {
				return caps, base, p.unsupported(r, "EMF+ custom line cap without a drawable path")
			}
			if !finite(d.BaseInset) || !finite(d.WidthScale) || !finite(d.StrokeMiterLimit) {
				return caps, base, malformed(r.Offset, "non-finite EMF+ custom line cap")
			}
			base[i] = b
		case c.custom.Arrow != nil:
			a := c.custom.Arrow
			if !finite(a.Width) || !finite(a.Height) || !finite(a.MiddleInset) || !finite(a.WidthScale) || !finite(a.LineMiterLimit) {
				return caps, base, malformed(r.Offset, "non-finite EMF+ arrow cap")
			}
			if a.LineJoin > 2 {
				return caps, base, p.unsupported(r, "EMF+ arrow cap with a clipped miter join")
			}
			base[i] = CapFlat
		default:
			return caps, base, malformed(r.Offset, "EMF+ custom line cap")
		}
	}
	return caps, base, nil
}

// capEnd is one end of an open figure: the point index in the path, its
// position, the outward direction in pen space, and whether the adjoining
// segment is a line whose other end is at index other.
type capEnd struct {
	index, other int
	at           Point
	out          Point
	line         bool
}

// openEnds lists the start and end of every open figure.
func openEnds(path Path, toPen Matrix) [][2]capEnd {
	var out [][2]capEnd
	k := 0
	var first, last capEnd
	var started, hasSeg bool
	dir := func(from, to Point) Point { return toPen.Apply(Point{to.X - from.X, to.Y - from.Y}) }
	finish := func() {
		if started && hasSeg {
			out = append(out, [2]capEnd{first, last})
		}
		started, hasSeg = false, false
	}
	var cur Point
	curIndex := 0
	for _, v := range path.Verbs {
		switch v {
		case PathMoveTo:
			finish()
			cur, curIndex, started = path.Points[k], k, true
			k++
		case PathLineTo, PathCubicTo:
			n := 1
			if v == PathCubicTo {
				n = 3
			}
			pts := path.Points[k : k+n]
			end := pts[n-1]
			// Tangents at each end of the segment, skipping coincident
			// control points.
			var in, outDir Point
			for _, q := range pts {
				if q != cur {
					in = dir(cur, q)
					break
				}
			}
			for i := n - 2; i >= -1; i-- {
				q := cur
				if i >= 0 {
					q = pts[i]
				}
				if q != end {
					outDir = dir(q, end)
					break
				}
			}
			if in != (Point{}) {
				if !hasSeg {
					first = capEnd{index: curIndex, other: k, at: cur, out: Point{-in.X, -in.Y}, line: v == PathLineTo}
				}
				last = capEnd{index: k + n - 1, other: curIndex, at: end, out: outDir, line: v == PathLineTo}
				hasSeg = true
			}
			cur, curIndex = end, k+n-1
			k += n
		case PathClose:
			started, hasSeg = false, false
		}
	}
	finish()
	return out
}

func unit(v Point) Point {
	l := math.Hypot(v.X, v.Y)
	return Point{v.X / l, v.Y / l}
}

// plusDrawCaps strokes path with custom caps at the ends of its open
// figures: it trims the figures by the caps' insets, strokes them, then draws
// each cap.
func (p *player) plusDrawCaps(r Record, path Path, s *Stroke, caps [2]*PlusCustomLineCap) error {
	toPen, ok := invertMatrix(s.Transform)
	if !ok {
		return nil // a degenerate pen draws nothing
	}
	trimmed := Path{Verbs: path.Verbs, Points: append([]Point(nil), path.Points...)}
	type capDraw struct {
		cap   *PlusCustomLineCap
		end   capEnd
		frame Matrix // cap space to destination offsets from the end point
	}
	var draws []capDraw
	for _, fig := range openEnds(path, toPen) {
		for i, end := range fig {
			c := caps[i]
			if c == nil {
				continue
			}
			u := unit(end.out)
			scale, inset := s.Width, 0.0
			if c.Default != nil {
				scale *= c.Default.WidthScale
				inset = c.Default.BaseInset * scale
			} else {
				scale *= c.Arrow.WidthScale
				inset = (c.Arrow.Height - c.Arrow.MiddleInset) * scale
			}
			// Cap space: +y along u, +x a quarter turn clockwise from it.
			frame := Matrix{M11: u.Y * scale, M12: -u.X * scale, M21: u.X * scale, M22: u.Y * scale}.Then(s.Transform)
			if !frame.Finite() {
				return malformed(r.Offset, "non-finite EMF+ custom line cap")
			}
			if inset != 0 {
				seg := toPen.Apply(Point{end.at.X - path.Points[end.other].X, end.at.Y - path.Points[end.other].Y})
				if !end.line || math.Hypot(seg.X, seg.Y) <= inset || inset < 0 {
					return p.unsupported(r, "EMF+ custom line cap inset beyond a line segment")
				}
				back := s.Transform.Apply(Point{-u.X * inset, -u.Y * inset})
				trimmed.Points[end.index] = Point{end.at.X + back.X, end.at.Y + back.Y}
			}
			draws = append(draws, capDraw{c, end, frame})
		}
	}
	clip := p.plusCurrentClip()
	if err := p.backend.StrokePath(trimmed, *s, clip); err != nil {
		return err
	}
	for _, d := range draws {
		place := d.frame
		place.Dx, place.Dy = d.end.at.X, d.end.at.Y
		if a := d.cap.Arrow; a != nil {
			b := pathBuilder{limit: 5}
			sh := shape{&b, place}
			sh.moveTo(Point{0, 0})
			sh.lineTo(Point{a.Width / 2, -a.Height})
			sh.lineTo(Point{0, -a.Height + a.MiddleInset})
			sh.lineTo(Point{-a.Width / 2, -a.Height})
			b.close()
			if a.Filled {
				if err := p.backend.FillPath(b.path, NonZero, s.Paint, clip); err != nil {
					return err
				}
				continue
			}
			outline := *s
			outline.Cap, outline.EndCap, outline.Join, outline.MiterLimit = CapFlat, 0, [...]LineJoin{JoinMiter, JoinBevel, JoinRound}[a.LineJoin], a.LineMiterLimit
			if err := p.backend.StrokePath(b.path, outline, clip); err != nil {
				return err
			}
			continue
		}
		dc := d.cap.Default
		if dc.Flags&2 != 0 {
			outline, err := p.plusPath(r, *dc.StrokePath, place, false)
			if err != nil {
				return err
			}
			pen := *s
			pen.Cap, pen.EndCap, pen.Join, pen.MiterLimit = plusCaps[dc.StrokeStartCap], plusCaps[dc.StrokeEndCap], [...]LineJoin{JoinMiter, JoinBevel, JoinRound}[dc.StrokeJoin], dc.StrokeMiterLimit
			if pen.EndCap == pen.Cap {
				pen.EndCap = 0
			}
			if err := p.backend.StrokePath(outline, pen, clip); err != nil {
				return err
			}
			continue
		}
		fill, err := p.plusPath(r, *dc.FillPath, place, true)
		if err != nil {
			return err
		}
		if err := p.backend.FillPath(fill, NonZero, s.Paint, clip); err != nil {
			return err
		}
	}
	return nil
}
