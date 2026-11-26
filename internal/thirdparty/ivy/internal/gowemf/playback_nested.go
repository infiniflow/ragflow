// Copied from github.com/mgilbir/gowemf, file playback_nested.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import "errors"

// playBudget holds the resources one Play call shares with the metafiles
// embedded in it.
type playBudget struct {
	pixels, regionWork, records uint64
}

// backendAs returns b as an optional backend interface T. An embedded
// metafile's nestedBackend implements every interface; it offers T only
// when the backend it draws into does.
func backendAs[T any](b Backend) (T, bool) {
	var zero T
	inner := b
	for {
		n, ok := inner.(*nestedBackend)
		if !ok {
			break
		}
		inner = n.inner
	}
	if _, ok := inner.(T); !ok {
		return zero, false
	}
	t, ok := b.(T)
	return t, ok
}

// embeddedMetafile is a parsed EmfPlusMetafile image: its bytes, header and
// size in image pixels, the units of a drawing's source rectangle.
//
// ivy: origin is the top left corner of the image in image pixels. GDI+
// gives a metafile image the bounds of its frame or of its placeable
// header, which need not start at 0, and a source rectangle is in those
// coordinates. The pictures of poi/vsdx/60973.vsdx have frames that start
// at (274, -658).
type embeddedMetafile struct {
	data   []byte
	header Header
	origin Point
	size   Point
}

// metafileImage parses an embedded metafile once per object definition.
// MS-EMFPLUS defines no pixel grid for a metafile image. GDI+, recording and
// drawing such images, uses the logical units of placeable WMF bounds, and
// for an EMF the frame in reference-device pixels plus one: the frame fills
// an image one pixel wider and taller (ORACLES.md).
func (p *player) metafileImage(r Record, obj *plusObject, img PlusImage) (*embeddedMetafile, error) {
	if obj.metafile != nil {
		return obj.metafile, nil
	}
	if img.MetafileType == 1 {
		return nil, p.unsupported(r, "EMF+ metafile image: WMF without placeable bounds")
	}
	data, err := img.MetafileBytes(p.options.Stream.Framing)
	if err != nil {
		return nil, p.embeddedError(r, err)
	}
	h, err := Walk(data, p.options.Stream.Framing, nil)
	if err != nil {
		return nil, p.embeddedError(r, err)
	}
	m := &embeddedMetafile{data: data, header: h}
	switch {
	case h.WMF != nil && h.Placeable != nil:
		b := h.Placeable.Bounds
		m.size = Point{abs(float64(b.Right) - float64(b.Left)), abs(float64(b.Bottom) - float64(b.Top))}
		m.origin = Point{min(float64(b.Left), float64(b.Right)), min(float64(b.Top), float64(b.Bottom))} // ivy: see origin
	case h.EMF != nil:
		_, left, top, w, ht, err := emfFrame(h.EMF)
		if err != nil {
			return nil, p.embeddedError(r, err)
		}
		if f := h.EMF.Frame; f.Right > f.Left && f.Bottom > f.Top {
			w, ht = w+1, ht+1 // the bounds fallback is already inclusive
		}
		m.size = Point{w, ht}
		m.origin = Point{left, top} // ivy: see origin
	default:
		return nil, p.unsupported(r, "EMF+ metafile image: WMF without placeable bounds")
	}
	if !(m.size.X > 0 && m.size.Y > 0) || !finite(m.size.X) || !finite(m.size.Y) || !finite(m.origin.X) || !finite(m.origin.Y) {
		return nil, p.embeddedError(r, malformed(0, "empty embedded metafile frame"))
	}
	obj.metafile = m
	return m, nil
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// embeddedError reports an embedded metafile that cannot be read: GDI+ would
// not draw the image, and the enclosing picture continues. Exceeded limits
// still stop playback.
func (p *player) embeddedError(r Record, err error) error {
	if errors.Is(err, ErrLimit) {
		return failure(r.Offset, "embedded metafile", ErrLimit)
	}
	return p.unsupported(r, "EMF+ metafile image: "+err.Error())
}

const (
	defaultMetafileDepth   = 4
	defaultEmbeddedRecords = 1_000_000
	embeddedReasonPrefix   = "embedded metafile: "
)

// playEmbedded plays an embedded metafile into its image pixel space, which
// place maps to destination coordinates, drawing only within area (the
// source rectangle's image) and clip. The embedded picture starts from its
// own default state, shares this Play call's resource budget, and reports its
// omissions as this record's.
func (p *player) playEmbedded(r Record, m *embeddedMetafile, place Matrix, clip Clip) error {
	depth := p.options.MaxMetafileDepth
	if depth == 0 {
		depth = defaultMetafileDepth
	}
	if p.depth >= depth {
		return p.unsupported(r, "EMF+ metafile image nested too deeply")
	}
	inv, ok := invertMatrix(place)
	if !ok {
		return nil // a degenerate placement covers no area
	}
	limit := p.options.MaxEmbeddedRecords
	if limit == 0 {
		limit = defaultEmbeddedRecords
	}
	// Count the records before playing any, so a large metafile drawn many
	// times cannot spend more than the budget even on framing.
	remaining := limit - min(limit, p.budget.records)
	var n uint64
	if _, err := Walk(m.data, p.options.Stream.Framing, func(Record) error {
		if n++; n > remaining {
			return failure(r.Offset, "embedded metafile records", ErrLimit)
		}
		return nil
	}); err != nil {
		return p.embeddedError(r, err)
	}
	p.budget.records += n

	o := p.options
	o.Destination = Box{Width: m.size.X, Height: m.size.Y}
	o.Placeable = nil
	if report := p.options.Unsupported; report != nil {
		o.Unsupported = func(u UnsupportedOperation) error {
			return report(UnsupportedOperation{Source: r, Reason: embeddedReasonPrefix + u.Reason})
		}
	}
	child := &player{options: o, budget: p.budget, depth: p.depth + 1,
		backend: &nestedBackend{inner: p.backend, m: place, inv: inv, outer: clip, nodes: map[*ClipRegion]*ClipRegion{}}}
	err := child.start(m.header)
	if err == nil {
		child.glyphText = findGlyphText(m.data, m.header, o.Stream) // ivy: change 19
		_, err = Stream(m.data, o.Stream, child.play)
	}
	var pe *ParseError
	if err != nil && errors.As(err, &pe) && (errors.Is(err, ErrMalformed) || errors.Is(err, ErrFormat)) {
		// A malformed embedded file stops its own playback only.
		return p.unsupported(r, "EMF+ metafile image: "+err.Error())
	}
	return err
}

// nestedBackend draws an embedded metafile: it maps the embedded picture's
// destination coordinates through m into the enclosing destination and adds
// the enclosing clip layers.
type nestedBackend struct {
	inner Backend
	m     Matrix
	inv   Matrix
	outer Clip
	nodes map[*ClipRegion]*ClipRegion
}

func (n *nestedBackend) path(p Path) Path {
	out := Path{Verbs: p.Verbs, Points: make([]Point, len(p.Points))}
	for i, q := range p.Points {
		out.Points[i] = n.m.Apply(q)
	}
	return out
}

func (n *nestedBackend) paint(p Paint) Paint {
	switch p.Kind {
	case PaintHatch, PaintPattern:
		p.PatternTransform = p.PatternTransform.Then(n.m)
	case PaintLinearGradient:
		g := *p.Gradient
		g.Transform = n.inv.Then(g.Transform)
		p.Gradient = &g
	}
	return p
}

func (n *nestedBackend) region(r *ClipRegion) *ClipRegion {
	if r == nil {
		return nil
	}
	if out, ok := n.nodes[r]; ok {
		return out
	}
	out := &ClipRegion{Base: n.region(r.Base), Op: r.Op, Area: n.path(r.Area), Rule: r.Rule, Operand: n.region(r.Operand), Offset: linear(n.m).Apply(r.Offset), depth: r.depth}
	n.nodes[r] = out
	return out
}

func (n *nestedBackend) clip(c Clip) Clip {
	out := make(Clip, len(n.outer), len(n.outer)+len(c))
	copy(out, n.outer)
	for _, r := range c {
		out = append(out, n.region(r))
	}
	return out
}

func (n *nestedBackend) image(d ImageDraw) ImageDraw {
	d.Transform = d.Transform.Then(n.m)
	return d
}

func (n *nestedBackend) FillPath(path Path, rule FillRule, paint Paint, clip Clip) error {
	return n.inner.FillPath(n.path(path), rule, n.paint(paint), n.clip(clip))
}

func (n *nestedBackend) StrokePath(path Path, s Stroke, clip Clip) error {
	s.Paint = n.paint(s.Paint)
	if s.Gap != nil {
		g := n.paint(*s.Gap)
		s.Gap = &g
	}
	s.Transform = s.Transform.Then(linear(n.m))
	s.PixelCenter = linear(n.m).Apply(s.PixelCenter)
	return n.inner.StrokePath(n.path(path), s, n.clip(clip))
}

func (n *nestedBackend) DrawImage(d ImageDraw, clip Clip) error {
	return n.inner.DrawImage(n.image(d), n.clip(clip))
}

func (n *nestedBackend) MeasureText(run TextRun) (TextMetrics, error) {
	run.Transform = run.Transform.Then(n.m)
	return n.inner.(TextBackend).MeasureText(run)
}

func (n *nestedBackend) DrawText(run TextRun, clip Clip) error {
	run.Transform = run.Transform.Then(n.m)
	run.Paint = n.paint(run.Paint)
	return n.inner.(TextBackend).DrawText(run, n.clip(clip))
}

func (n *nestedBackend) FillGradient(mesh []GradientTriangle, clip Clip) error {
	out := make([]GradientTriangle, len(mesh))
	for i, t := range mesh {
		out[i] = t
		for k := range t.Points {
			out[i].Points[k] = n.m.Apply(t.Points[k])
		}
	}
	return n.inner.(GradientBackend).FillGradient(out, n.clip(clip))
}

func (n *nestedBackend) DrawRaster(r RasterDraw, clip Clip) error {
	r.Area = n.path(r.Area)
	if r.Source != nil {
		d := n.image(*r.Source)
		r.Source = &d
	}
	if r.Pattern != nil {
		pt := n.paint(*r.Pattern)
		r.Pattern = &pt
	}
	// ivy: the pen of a stroke with a mix mode (change 18), as StrokePath
	// places it
	if r.Stroke != nil {
		s := *r.Stroke
		s.Paint = n.paint(s.Paint)
		s.Gap = nil
		s.Transform = s.Transform.Then(linear(n.m))
		s.PixelCenter = linear(n.m).Apply(s.PixelCenter)
		r.Stroke = &s
	}
	return n.inner.(RasterBackend).DrawRaster(r, n.clip(clip))
}
