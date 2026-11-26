// Copied from github.com/mgilbir/gowemf, file playback_path.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import "math"

// pathBuilder accumulates destination-space geometry under a point budget.
// GDI path brackets store points in device space when they are recorded
// (MS-EMF 2.3.10), so later transform changes do not move them.
type pathBuilder struct {
	path  Path
	limit uint64
	open  bool // the last figure accepts further segments
	err   bool
}

func (b *pathBuilder) room(n int) bool {
	if b.err || uint64(len(b.path.Points))+uint64(n) > b.limit {
		b.err = true
		return false
	}
	return true
}
func (b *pathBuilder) moveTo(p Point) {
	if b.room(1) {
		b.path.Verbs = append(b.path.Verbs, PathMoveTo)
		b.path.Points = append(b.path.Points, p)
		b.open = true
	}
}
func (b *pathBuilder) lineTo(p Point) {
	if b.room(1) {
		b.path.Verbs = append(b.path.Verbs, PathLineTo)
		b.path.Points = append(b.path.Points, p)
	}
}
func (b *pathBuilder) cubicTo(c1, c2, p Point) {
	if b.room(3) {
		b.path.Verbs = append(b.path.Verbs, PathCubicTo)
		b.path.Points = append(b.path.Points, c1, c2, p)
	}
}
func (b *pathBuilder) close() {
	if b.open && !b.err {
		b.path.Verbs = append(b.path.Verbs, PathClose)
		b.open = false
	}
}

// closeAll closes every open figure, as FillPath, StrokeAndFillPath and
// SelectClipPath require.
func (b *pathBuilder) closeAll() Path {
	src := b.path
	var out Path
	out.Points = src.Points
	out.Verbs = make([]PathVerb, 0, len(src.Verbs)+len(src.Verbs)/2+1)
	open := false
	for _, v := range src.Verbs {
		if v == PathMoveTo && open {
			out.Verbs = append(out.Verbs, PathClose)
		}
		out.Verbs = append(out.Verbs, v)
		open = v != PathClose
	}
	if open {
		out.Verbs = append(out.Verbs, PathClose)
	}
	return out
}

// flatten replaces Béziers by line segments within a quarter destination
// unit, implementing EMR_FLATTENPATH.
func (b *pathBuilder) flatten() {
	src := b.path
	out := pathBuilder{limit: b.limit}
	i := 0
	var cur Point
	for _, v := range src.Verbs {
		switch v {
		case PathMoveTo:
			cur = src.Points[i]
			out.moveTo(cur)
			i++
		case PathLineTo:
			cur = src.Points[i]
			out.lineTo(cur)
			i++
		case PathCubicTo:
			c1, c2, end := src.Points[i], src.Points[i+1], src.Points[i+2]
			n := cubicSteps(cur, c1, c2, end)
			for k := 1; k <= n; k++ {
				out.lineTo(cubicAt(cur, c1, c2, end, float64(k)/float64(n)))
			}
			cur = end
			i += 3
		case PathClose:
			out.path.Verbs = append(out.path.Verbs, PathClose)
		}
	}
	out.open = b.open
	*b = out
}

func cubicAt(p0, p1, p2, p3 Point, t float64) Point {
	u := 1 - t
	a, c, d, e := u*u*u, 3*u*u*t, 3*u*t*t, t*t*t
	return Point{a*p0.X + c*p1.X + d*p2.X + e*p3.X, a*p0.Y + c*p1.Y + d*p2.Y + e*p3.Y}
}

// cubicSteps bounds the flattening error by the control polygon deviation.
func cubicSteps(p0, p1, p2, p3 Point) int {
	dd := math.Max(math.Hypot(p0.X-2*p1.X+p2.X, p0.Y-2*p1.Y+p2.Y), math.Hypot(p1.X-2*p2.X+p3.X, p1.Y-2*p2.Y+p3.Y))
	n := int(math.Ceil(math.Sqrt(dd * 3))) // 3/4*dd/n² <= 0.25
	if n < 1 || !finite(dd) {
		return 1
	}
	if n > 256 {
		return 256
	}
	return n
}

// shape is logical-space geometry that is transformed point by point. Affine
// maps preserve cubic Bézier curves exactly.
type shape struct {
	b *pathBuilder
	m Matrix
}

func (s shape) moveTo(p Point)          { s.b.moveTo(s.m.Apply(p)) }
func (s shape) lineTo(p Point)          { s.b.lineTo(s.m.Apply(p)) }
func (s shape) cubicTo(c1, c2, p Point) { s.b.cubicTo(s.m.Apply(c1), s.m.Apply(c2), s.m.Apply(p)) }

// ellipse parameterization in y-down logical space: θ increases
// counterclockwise as displayed.
func ellipsePoint(c, r Point, t float64) Point {
	return Point{c.X + r.X*math.Cos(t), c.Y - r.Y*math.Sin(t)}
}

// arc appends an elliptical arc from angle t0 sweeping by sweep radians, as
// cubic segments of at most 90 degrees. The caller positions the start point.
func (s shape) arc(c, r Point, t0, sweep float64) {
	n := int(math.Ceil(math.Abs(sweep) / (math.Pi / 2)))
	if n < 1 {
		n = 1
	}
	d := sweep / float64(n)
	k := 4.0 / 3 * math.Tan(d/4)
	for i := 0; i < n; i++ {
		a, e := t0+float64(i)*d, t0+float64(i+1)*d
		p0, p3 := ellipsePoint(c, r, a), ellipsePoint(c, r, e)
		c1 := Point{p0.X - k*r.X*math.Sin(a), p0.Y - k*r.Y*math.Cos(a)}
		c2 := Point{p3.X + k*r.X*math.Sin(e), p3.Y + k*r.Y*math.Cos(e)}
		s.cubicTo(c1, c2, p3)
	}
}

func normalize(r Rect) (l, t, rr, b float64) {
	l, t, rr, b = float64(r.Left), float64(r.Top), float64(r.Right), float64(r.Bottom)
	if l > rr {
		l, rr = rr, l
	}
	if t > b {
		t, b = b, t
	}
	return
}

// arcAngles returns the start angle and signed sweep for Arc/Chord/Pie/ArcTo.
// Radials whose directions coincide describe a complete ellipse.
func arcAngles(c, r Point, start, end Point, ccw bool) (float64, float64) {
	angle := func(p Point) float64 { return math.Atan2(-(p.Y-c.Y)*r.X, (p.X-c.X)*r.Y) }
	a, e := angle(start), angle(end)
	sweep := e - a
	if ccw {
		for sweep <= 0 {
			sweep += 2 * math.Pi
		}
	} else {
		for sweep >= 0 {
			sweep -= 2 * math.Pi
		}
	}
	return a, sweep
}

const (
	shapeRectangle = iota
	shapeEllipse
	shapeRoundRect
	shapeArc
	shapeArcTo
	shapeChord
	shapePie
)

// boxSpace is where GDI constructs bounding-rectangle shapes. Under
// GM_COMPATIBLE that is device space: the right and bottom edges are excluded
// and the arc direction is not reflected by the transform (MS-EMF 2.1.16).
// Under GM_ADVANCED it is world space, where the edges are included and a
// reflection reverses the displayed arc direction. Windows constructs EMF
// shapes as under GM_ADVANCED even with an identity world transform
// (ORACLES.md), so only WMF uses GM_COMPATIBLE here.
type boxSpace struct {
	s          shape  // box space to destination
	toBox      Matrix // logical to box space
	compatible bool
}

func (p *player) boxSpace(b *pathBuilder, m Matrix) boxSpace {
	if p.format == EMF {
		return boxSpace{shape{b, m}, Identity(), false}
	}
	// WMF has no world transform, so m is the page mapping followed by base.
	return boxSpace{shape{b, p.base}, p.dc.pageMatrix(), true}
}

// closedShape appends a bounding-rectangle figure and returns its end point
// in box space. inset is the pen width along each axis for PS_INSIDEFRAME,
// in box units.
func (p *player) closedShape(bs boxSpace, kind int, box Rect, corner, start, end Point, inset Point) Point {
	s := bs.s
	c0 := bs.toBox.Apply(Point{float64(box.Left), float64(box.Top)})
	c1 := bs.toBox.Apply(Point{float64(box.Right), float64(box.Bottom)})
	l, r := math.Min(c0.X, c1.X), math.Max(c0.X, c1.X)
	t, b := math.Min(c0.Y, c1.Y), math.Max(c0.Y, c1.Y)
	if bs.compatible {
		r, b = r-1, b-1
	}
	l, t, r, b = l+inset.X/2, t+inset.Y/2, r-inset.X/2, b-inset.Y/2
	if l > r {
		l, r = (l+r)/2, (l+r)/2
	}
	if t > b {
		t, b = (t+b)/2, (t+b)/2
	}
	corner = Point{math.Abs(corner.X * bs.toBox.M11), math.Abs(corner.Y * bs.toBox.M22)}
	start, end = bs.toBox.Apply(start), bs.toBox.Apply(end)
	ccw := p.dc.arcDir != 2
	c, rad := Point{(l + r) / 2, (t + b) / 2}, Point{(r - l) / 2, (b - t) / 2}
	switch kind {
	case shapeRectangle:
		s.moveTo(Point{l, t})
		if ccw {
			s.lineTo(Point{l, b})
			s.lineTo(Point{r, b})
			s.lineTo(Point{r, t})
		} else {
			s.lineTo(Point{r, t})
			s.lineTo(Point{r, b})
			s.lineTo(Point{l, b})
		}
		s.b.close()
	case shapeEllipse:
		sweep := 2 * math.Pi
		if !ccw {
			sweep = -sweep
		}
		s.moveTo(ellipsePoint(c, rad, 0))
		s.arc(c, rad, 0, sweep)
		s.b.close()
	case shapeRoundRect:
		cr := Point{math.Min(corner.X/2, rad.X), math.Min(corner.Y/2, rad.Y)}
		// Corner centers in counterclockwise order starting at the top-left.
		centers := [4]Point{{l + cr.X, t + cr.Y}, {l + cr.X, b - cr.Y}, {r - cr.X, b - cr.Y}, {r - cr.X, t + cr.Y}}
		starts := [4]float64{math.Pi / 2, math.Pi, 3 * math.Pi / 2, 0}
		sweep := math.Pi / 2
		if !ccw {
			centers[1], centers[3] = centers[3], centers[1]
			starts = [4]float64{math.Pi, math.Pi / 2, 0, 3 * math.Pi / 2}
			sweep = -sweep
		}
		s.moveTo(ellipsePoint(centers[0], cr, starts[0]))
		for i := range centers {
			s.arc(centers[i], cr, starts[i], sweep)
			if i < 3 {
				s.lineTo(ellipsePoint(centers[i+1], cr, starts[i+1]))
			}
		}
		s.b.close()
	default:
		a, sweep := arcAngles(c, rad, start, end, ccw)
		if kind == shapeArcTo {
			s.lineTo(ellipsePoint(c, rad, a))
		} else {
			s.moveTo(ellipsePoint(c, rad, a))
		}
		s.arc(c, rad, a, sweep)
		if kind == shapePie {
			s.lineTo(c)
		}
		if kind == shapeChord || kind == shapePie {
			s.b.close()
		}
		return ellipsePoint(c, rad, a+sweep)
	}
	return Point{}
}
