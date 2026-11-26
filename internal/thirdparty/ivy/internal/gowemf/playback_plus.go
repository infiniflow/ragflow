// Copied from github.com/mgilbir/gowemf, file playback_plus.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"fmt"
	"image"
	"image/color"
	"math"
)

// plusState is the EMF+ graphics state saved by EmfPlusSave and containers.
// World coordinates map to the device through world, then container (the
// accumulated container mappings and enclosing world transforms), then the
// page transform. Clip and meta are in destination coordinates.
type plusState struct {
	world, container Matrix
	pageUnit         uint8
	pageScale        float64
	clip             *ClipRegion
	meta             []*ClipRegion // enclosing containers' clips
	compositing      uint32
	interpolation    uint32
	pixelOffset      uint32
	origin           Point
	// skip suppresses the contents of a container whose coordinate mapping
	// was reported unsupported, rather than drawing them misplaced.
	skip bool
}

type plusSaved struct {
	index     uint32
	container bool
	state     plusState
}

// plusObject is an EMF+ object slot. Bitmap images are decoded once, on
// first use, and charged to the playback pixel budget then.
type plusObject struct {
	value    any
	decoded  image.Image
	metafile *embeddedMetafile
}

// The initial page transform is one device pixel per world unit: GDI+ writes
// SetPageTransform for any other page unit it records.
func defaultPlusState() plusState {
	return plusState{world: Identity(), container: Identity(), pageUnit: 2, pageScale: 1}
}

// unitScale returns device pixels per unit on each axis (MS-EMFPLUS
// 2.1.1.32), using the EMF+ header's logical DPI. World and Display units have
// no fixed size.
func (p *player) unitScale(unit uint32) (Point, bool) {
	d := p.plusDPI
	if unit == 2 {
		return Point{1, 1}, true
	}
	if d.X <= 0 || d.Y <= 0 {
		return Point{}, false
	}
	switch unit {
	case 3:
		return Point{d.X / 72, d.Y / 72}, true
	case 4:
		return d, true
	case 5:
		return Point{d.X / 300, d.Y / 300}, true
	case 6:
		return Point{d.X / 25.4, d.Y / 25.4}, true
	}
	return Point{}, false
}

// plusDevice maps device pixels to destination coordinates. Under the
// PixelOffsetMode values Default, HighSpeed and None, pixel centers have
// integer device coordinates (MS-EMFPLUS 2.1.1.26), so device coordinates
// shift by half a pixel into the destination's pixel-area convention; Half
// and HighQuality place pixel centers at half-integers already.
func (p *player) plusDevice() Matrix {
	if m := p.plus.pixelOffset; m == 2 || m == 4 {
		return p.base
	}
	return Matrix{M11: 1, M22: 1, Dx: .5, Dy: .5}.Then(p.base)
}

// plusMatrix maps world coordinates to destination coordinates.
func (p *player) plusMatrix(r Record) (Matrix, error) {
	s := &p.plus
	if s.skip {
		return Matrix{}, errSkip
	}
	u, ok := p.unitScale(uint32(s.pageUnit))
	if !ok {
		return Matrix{}, p.unsupported(r, "EMF+ page unit without a defined size")
	}
	page := Matrix{M11: u.X * s.pageScale, M22: u.Y * s.pageScale}
	m := s.world.Then(s.container).Then(page).Then(p.plusDevice())
	if !m.Finite() {
		return Matrix{}, malformed(r.Offset, "non-finite EMF+ transform")
	}
	return m, nil
}

func linear(m Matrix) Matrix { return Matrix{M11: m.M11, M12: m.M12, M21: m.M21, M22: m.M22} }

func invertMatrix(m Matrix) (Matrix, bool) {
	det := m.M11*m.M22 - m.M12*m.M21
	if det == 0 || !finite(det) {
		return Matrix{}, false
	}
	inv := Matrix{M11: m.M22 / det, M12: -m.M12 / det, M21: -m.M21 / det, M22: m.M11 / det}
	inv.Dx = -(m.Dx*inv.M11 + m.Dy*inv.M21)
	inv.Dy = -(m.Dx*inv.M12 + m.Dy*inv.M22)
	return inv, inv.Finite()
}

func argb(v uint32) color.NRGBA {
	return color.NRGBA{byte(v >> 16), byte(v >> 8), byte(v), byte(v >> 24)}
}

// plusObjectAt returns a defined object; Stream has checked its type.
func (p *player) plusObjectAt(r Record, id uint32) (*plusObject, error) {
	if id > 63 || p.plusObjects[id].value == nil {
		return nil, malformed(r.Offset, "EMF+ object reference")
	}
	return &p.plusObjects[id], nil
}

func (p *player) plusDispatch(c Command) error {
	r, body := c.Source, c.Body
	s := &p.plus
	if c.HasObjectID {
		if c.ObjectID > 63 {
			return malformed(r.Offset, "EMF+ object ID")
		}
		p.plusObjects[c.ObjectID] = plusObject{value: body}
		return nil
	}
	switch r.Type {
	case PlusHeaderRecord, PlusEndOfFileRecord, PlusCommentRecord, PlusGetDCRecord, PlusSerializableObjectRecord:
		// Stream delivers GDI records inside GetDC intervals to the GDI
		// player, and binds serialized effects to the image draws using them.
		return nil
	case PlusClearRecord:
		return p.plusClear(c, body.(Value).Value)
	case PlusFillRectsRecord, PlusDrawRectsRecord:
		return p.plusRects(c, body.(PlusRects))
	case PlusFillPolygonRecord, PlusDrawLinesRecord, PlusDrawBeziersRecord:
		return p.plusPoly(c, body.(PlusPoly))
	case PlusFillEllipseRecord, PlusDrawEllipseRecord, PlusFillPieRecord, PlusDrawPieRecord, PlusDrawArcRecord:
		return p.plusEllipse(c, body.(PlusEllipse))
	case PlusFillRegionRecord, PlusFillPathRecord, PlusDrawPathRecord:
		return p.plusPathDraw(c, body.(PlusPathDraw))
	case PlusFillClosedCurveRecord, PlusDrawClosedCurveRecord, PlusDrawCurveRecord:
		return p.plusCurve(c, body.(PlusCurve))
	case PlusDrawImageRecord, PlusDrawImagePointsRecord:
		return p.plusImage(c, body.(PlusImageDraw))
	case PlusDrawDriverStringRecord:
		return p.plusDriverString(c, body.(PlusDriverString))
	case PlusDrawStringRecord:
		// ivy: MS-EMFPLUS leaves GDI+ string layout open: the unit of the
		// default 1/6 margins, how the 1.03 default tracking applies, line
		// breaking, trimming and line spacing. plusDrawString in
		// playback_plus_string.go reads them closely enough to read the
		// text. Upstream reports the record as not supported.
		return p.plusDrawString(c, body.(PlusText))
	case PlusSetRenderingOriginRecord:
		s.origin = body.(PointRecord).Point
	case PlusSetInterpolationModeRecord:
		s.interpolation = body.(Value).Value & 0xff
	case PlusSetPixelOffsetModeRecord:
		s.pixelOffset = body.(Value).Value & 0xff
	case PlusSetCompositingModeRecord:
		s.compositing = body.(Value).Value & 0xff
	case PlusSetAntiAliasModeRecord, PlusSetTextRenderingHintRecord, PlusSetTextContrastRecord, PlusSetCompositingQualityRecord:
		// Rendering-quality hints; anti-aliasing is the backend's policy.
		return nil
	case PlusSaveRecord:
		p.plusSaved = append(p.plusSaved, plusSaved{body.(Value).Value, false, p.plus})
	case PlusRestoreRecord, PlusEndContainerRecord:
		// Stream has matched the index against Save or a container.
		container, index := r.Type == PlusEndContainerRecord, body.(Value).Value
		for i := len(p.plusSaved) - 1; i >= 0; i-- {
			if e := p.plusSaved[i]; e.index == index && e.container == container {
				p.plus = e.state
				p.plusSaved = p.plusSaved[:i]
				return nil
			}
		}
		return malformed(r.Offset, "EMF+ saved state index")
	case PlusBeginContainerRecord, PlusBeginContainerNoParamsRecord:
		return p.plusBeginContainer(r, body)
	case PlusSetWorldTransformRecord:
		return p.plusWorld(r, body.(Transform).Matrix)
	case PlusResetWorldTransformRecord:
		s.world = Identity()
	case PlusMultiplyWorldTransformRecord:
		return p.plusMultiply(r, body.(Transform).Matrix)
	case PlusTranslateWorldTransformRecord:
		v := body.(PointRecord).Point
		return p.plusMultiply(r, Matrix{M11: 1, M22: 1, Dx: v.X, Dy: v.Y})
	case PlusScaleWorldTransformRecord:
		v := body.(PointRecord).Point
		return p.plusMultiply(r, Matrix{M11: v.X, M22: v.Y})
	case PlusRotateWorldTransformRecord:
		// A positive angle turns the x axis toward the y axis: clockwise as
		// displayed in y-down space.
		sn, cs := math.Sincos(body.(FloatValue).Value * math.Pi / 180)
		return p.plusMultiply(r, Matrix{M11: cs, M12: sn, M21: -sn, M22: cs})
	case PlusSetPageTransformRecord:
		v := body.(PlusPageTransform)
		if !finite(v.Scale) || v.Scale == 0 {
			return malformed(r.Offset, "EMF+ page scale")
		}
		// An undefined unit is reported when drawing depends on it.
		s.pageUnit, s.pageScale = v.Unit, v.Scale
	case PlusResetClipRecord:
		s.clip = nil
	case PlusSetClipRectRecord, PlusSetClipPathRecord, PlusSetClipRegionRecord:
		return p.plusClip(r, body.(PlusClip))
	case PlusOffsetClipRecord:
		return p.plusOffsetClip(r, body.(PointRecord).Point)
	default:
		return p.unsupported(r, fmt.Sprintf("EMF+ record 0x%04x", r.Type))
	}
	return nil
}

// plusMultiply applies m before the world transform, or after it when the
// record's flag 0x2000 requests append order.
func (p *player) plusMultiply(r Record, m Matrix) error {
	if r.Flags&0x2000 != 0 {
		return p.plusWorld(r, p.plus.world.Then(m))
	}
	return p.plusWorld(r, m.Then(p.plus.world))
}

func (p *player) plusWorld(r Record, m Matrix) error {
	if !m.Finite() {
		return malformed(r.Offset, "non-finite EMF+ world transform")
	}
	p.plus.world = m
	return nil
}

// plusBeginContainer starts a container (MS-EMFPLUS 2.3.7.1): drawing inside
// is clipped by the enclosing clip, kept as a metaregion, and coordinates map
// through the source-to-destination rectangle mapping into the enclosing
// world space. The world transform and clip start out reset.
func (p *player) plusBeginContainer(r Record, body any) error {
	s := &p.plus
	mapping := Identity()
	var index uint32
	var reason string
	switch v := body.(type) {
	case PlusContainer:
		index = v.StackIndex
		// The rectangles share no unit with the page, so only their
		// correspondence is defined when the unit is World or Pixel.
		if v.Unit != 0 && v.Unit != 2 {
			reason = "EMF+ container in a physical unit"
		} else if v.Source.Width == 0 || v.Source.Height == 0 {
			reason = "degenerate EMF+ container source rectangle"
		} else {
			sx, sy := v.Destination.Width/v.Source.Width, v.Destination.Height/v.Source.Height
			mapping = Matrix{M11: sx, M22: sy, Dx: v.Destination.X - v.Source.X*sx, Dy: v.Destination.Y - v.Source.Y*sy}
		}
	case Value:
		index = v.Value
	}
	container := mapping.Then(s.world).Then(s.container)
	if !container.Finite() {
		return malformed(r.Offset, "non-finite EMF+ container transform")
	}
	// The saved entry is pushed even for a skipped container, so that its
	// EndContainer restores the enclosing state.
	p.plusSaved = append(p.plusSaved, plusSaved{index, true, p.plus})
	if reason != "" && !s.skip {
		if err := p.unsupported(r, reason+"; its contents are skipped"); err != errSkip {
			return err
		}
		s.skip = true
		return nil
	}
	if s.clip != nil {
		// Copy: saved states share the old slice.
		meta := make([]*ClipRegion, len(s.meta), len(s.meta)+1)
		copy(meta, s.meta)
		s.meta = append(meta, s.clip)
	}
	s.clip, s.world, s.container = nil, Identity(), container
	return nil
}

func (p *player) plusCurrentClip() Clip {
	s := &p.plus
	c := make(Clip, len(s.meta), len(s.meta)+1)
	copy(c, s.meta)
	if s.clip != nil {
		c = append(c, s.clip)
	}
	return c
}

var plusClipOps = [...]ClipOp{ClipReplace, ClipIntersect, ClipUnion, ClipXor, ClipDifference, ClipComplement}

// plusCombine combines the clip with an area, or with a region tree when
// operand is non-nil (MS-EMFPLUS 2.1.1.4 CombineMode).
func (p *player) plusCombine(r Record, mode uint32, area Path, rule FillRule, operand *ClipRegion) error {
	s := &p.plus
	if mode >= uint32(len(plusClipOps)) {
		return malformed(r.Offset, "EMF+ combine mode")
	}
	op := plusClipOps[mode]
	if op == ClipReplace {
		if operand == nil {
			operand = &ClipRegion{Op: ClipReplace, Area: area, Rule: rule, depth: 1}
		}
		s.clip = operand
		return nil
	}
	depth := uint64(1)
	if operand != nil {
		depth += uint64(operand.depth)
	}
	if s.clip != nil {
		depth += uint64(s.clip.depth)
	}
	if depth > uint64(p.options.MaxClipSteps) {
		return failure(r.Offset, "clip region steps", ErrLimit)
	}
	s.clip = &ClipRegion{Base: s.clip, Op: op, Area: area, Rule: rule, Operand: operand, depth: uint32(depth)}
	return nil
}

func (p *player) plusClip(r Record, v PlusClip) error {
	m, err := p.plusMatrix(r)
	if err != nil {
		return err
	}
	if r.Type == PlusSetClipRectRecord {
		return p.plusCombine(r, v.Mode, boxPath(v.Rect, m), NonZero, nil)
	}
	obj, err := p.plusObjectAt(r, uint32(v.ObjectID))
	if err != nil {
		return err
	}
	if r.Type == PlusSetClipPathRecord {
		path, err := p.plusPath(r, obj.value.(PlusPath), m, true)
		if err != nil {
			return err
		}
		return p.plusCombine(r, v.Mode, path, EvenOdd, nil)
	}
	tree, err := p.plusRegion(r, obj.value.(PlusRegion), m)
	if err != nil {
		return err
	}
	return p.plusCombine(r, v.Mode, Path{}, NonZero, tree)
}

// plusOffsetClip translates the clip by a world-space offset.
func (p *player) plusOffsetClip(r Record, v Point) error {
	s := &p.plus
	if s.clip == nil {
		return nil // the infinite region is unchanged
	}
	m, err := p.plusMatrix(r)
	if err != nil {
		return err
	}
	if uint64(s.clip.depth)+1 > uint64(p.options.MaxClipSteps) {
		return failure(r.Offset, "clip region steps", ErrLimit)
	}
	s.clip = &ClipRegion{Base: s.clip, Op: ClipOffset, Offset: linear(m).Apply(v), depth: s.clip.depth + 1}
	return nil
}

// maxRegionDepth bounds recursion over EMF+ region trees.
const maxRegionDepth = 256

// plusRegion converts an EMF+ region tree (MS-EMFPLUS 2.2.1.8, 2.1.1.27) to
// clip steps. Infinite is the whole surface less nothing and Empty an empty
// replacement. Exclude keeps left minus right and Complement right minus left.
func (p *player) plusRegion(r Record, region PlusRegion, m Matrix) (*ClipRegion, error) {
	var visit func(i int, level int) (*ClipRegion, error)
	visit = func(i int, level int) (*ClipRegion, error) {
		if level > maxRegionDepth {
			return nil, failure(r.Offset, "EMF+ region depth", ErrLimit)
		}
		if i < 0 || i >= len(region.Nodes) {
			return nil, malformed(r.Offset, "EMF+ region node")
		}
		n := region.Nodes[i]
		switch n.Type {
		case 0x10000000:
			return &ClipRegion{Op: ClipReplace, Area: boxPath(n.Rect, m), Rule: NonZero, depth: 1}, nil
		case 0x10000001:
			path, err := p.plusPath(r, n.Path, m, true)
			if err != nil {
				return nil, err
			}
			return &ClipRegion{Op: ClipReplace, Area: path, Rule: EvenOdd, depth: 1}, nil
		case 0x10000002:
			return &ClipRegion{Op: ClipReplace, depth: 1}, nil
		case 0x10000003:
			return &ClipRegion{Op: ClipDifference, depth: 1}, nil
		case 1, 2, 3, 4, 5:
			left, err := visit(n.Left, level+1)
			if err != nil {
				return nil, err
			}
			right, err := visit(n.Right, level+1)
			if err != nil {
				return nil, err
			}
			d := uint64(left.depth) + uint64(right.depth) + 1
			if d > uint64(p.options.MaxClipSteps) {
				return nil, failure(r.Offset, "clip region steps", ErrLimit)
			}
			op := [...]ClipOp{1: ClipIntersect, 2: ClipUnion, 3: ClipXor, 4: ClipDifference, 5: ClipComplement}[n.Type]
			return &ClipRegion{Base: left, Op: op, Operand: right, depth: uint32(d)}, nil
		}
		return nil, malformed(r.Offset, "EMF+ region node type")
	}
	return visit(0, 1)
}

func boxPath(v Box, m Matrix) Path {
	b := pathBuilder{limit: 4}
	s := shape{&b, m}
	s.moveTo(Point{v.X, v.Y})
	s.lineTo(Point{v.X + v.Width, v.Y})
	s.lineTo(Point{v.X + v.Width, v.Y + v.Height})
	s.lineTo(Point{v.X, v.Y + v.Height})
	b.close()
	return b.path
}

// plusPath builds an EMF+ path object (MS-EMFPLUS 2.2.1.6, 2.1.1.23): point
// types are Start, Line and Bézier, and flag 0x80 closes the subpath. The
// object stores no fill mode; fills use GDI+'s default, alternate.
func (p *player) plusPath(r Record, v PlusPath, m Matrix, closeAll bool) (Path, error) {
	b := pathBuilder{limit: p.options.MaxPathPoints}
	s := shape{&b, m}
	n := v.Points.Len()
	for i := 0; i < n; i++ {
		t := v.Types[i]
		switch t & 7 {
		case 0:
			s.moveTo(v.Points.At(i))
		case 1:
			s.lineTo(v.Points.At(i))
		case 3:
			// Validate guarantees complete triples.
			s.cubicTo(v.Points.At(i), v.Points.At(i+1), v.Points.At(i+2))
			i += 2
			t = v.Types[i]
		}
		if t&0x80 != 0 {
			b.close()
		}
	}
	if b.err {
		return Path{}, failure(r.Offset, "path points", ErrLimit)
	}
	if closeAll {
		return b.closeAll(), nil
	}
	return b.path, nil
}

// plusPaint resolves a brush argument: an ARGB color when solid, otherwise
// an object reference.
func (p *player) plusPaint(r Record, id uint32, solid bool, m Matrix) (*Paint, error) {
	if solid {
		return &Paint{Kind: PaintSolid, Color: argb(id)}, nil
	}
	obj, err := p.plusObjectAt(r, id)
	if err != nil {
		return nil, err
	}
	return p.plusBrushPaint(r, obj.value.(PlusBrush), obj, m)
}

// plusBrushPaint resolves a brush (MS-EMFPLUS 2.2.1.1). obj is the slot
// caching a texture image: the brush's own, or the pen's for a pen brush.
func (p *player) plusBrushPaint(r Record, b PlusBrush, obj *plusObject, m Matrix) (*Paint, error) {
	switch b.Type {
	case 0:
		return &Paint{Kind: PaintSolid, Color: argb(b.Color)}, nil
	case 1:
		return p.plusHatch(r, b)
	case 2:
		t := b.Texture
		if t == nil || t.Image == nil {
			return nil, malformed(r.Offset, "EMF+ texture brush image")
		}
		im, err := p.plusBitmap(r, obj, *t.Image)
		if err != nil || im == nil {
			return nil, err
		}
		place := t.Transform.Then(m)
		if _, ok := invertMatrix(place); !ok {
			return nil, nil // a degenerate texture covers nothing
		}
		return &Paint{Kind: PaintPattern, Pattern: im, PatternTransform: place, Wrap: WrapMode(t.WrapMode)}, nil
	case 3:
		return nil, p.unsupported(r, "EMF+ path gradient brush")
	case 4:
		return p.plusLinearGradient(r, b.LinearGradient, m)
	}
	return nil, malformed(r.Offset, "EMF+ brush type")
}

// plusLinearGradient maps the brush rectangle's left edge to parameter 0 and
// its right edge to 1, in brush space, which the brush transform places in
// world space (MS-EMFPLUS 2.2.2.24). Blend factors give the fraction of the
// end color at each position; preset colors list the stops directly.
func (p *player) plusLinearGradient(r Record, g *PlusLinearGradient, m Matrix) (*Paint, error) {
	switch {
	case g == nil || g.Rect.Width == 0 || g.Rect.Height == 0:
		return nil, malformed(r.Offset, "EMF+ linear gradient rectangle")
	case g.Flags&0x10 != 0:
		return nil, p.unsupported(r, "EMF+ vertical blend factors on a linear gradient")
	case g.Flags&0x80 != 0:
		return nil, p.unsupported(r, "EMF+ gamma-corrected gradient")
	case g.WrapMode == uint32(WrapClamp):
		return nil, p.unsupported(r, "EMF+ clamped linear gradient")
	}
	inv, ok := invertMatrix(g.Transform.Then(m))
	if !ok {
		return nil, nil // a degenerate brush space paints nothing
	}
	unit := Matrix{M11: 1 / g.Rect.Width, M22: 1 / g.Rect.Height, Dx: -g.Rect.X / g.Rect.Width, Dy: -g.Rect.Y / g.Rect.Height}
	lg := &LinearGradient{Transform: inv.Then(unit), Wrap: WrapMode(g.WrapMode)}
	if !lg.Transform.Finite() {
		return nil, malformed(r.Offset, "non-finite EMF+ gradient transform")
	}
	start, end := argb(g.StartColor), argb(g.EndColor)
	mix := func(f float64) color.NRGBA {
		l := func(a, b uint8) uint8 { return uint8(math.Round(float64(a) + (float64(b)-float64(a))*f)) }
		return color.NRGBA{l(start.R, end.R), l(start.G, end.G), l(start.B, end.B), l(start.A, end.A)}
	}
	switch {
	case g.Flags&0x04 != 0:
		n := len(g.PresetPositions)
		if n < 2 || g.PresetPositions[0] != 0 || g.PresetPositions[n-1] != 1 {
			return nil, malformed(r.Offset, "EMF+ preset color positions")
		}
		lg.Stops = make([]GradientStop, n)
		for i, pos := range g.PresetPositions {
			lg.Stops[i] = GradientStop{pos, argb(g.PresetColors.At(i))}
		}
	case g.Flags&0x08 != 0:
		lg.Stops = make([]GradientStop, len(g.Horizontal.Positions))
		for i, pos := range g.Horizontal.Positions {
			lg.Stops[i] = GradientStop{pos, mix(g.Horizontal.Factors[i])}
		}
	default:
		lg.Stops = []GradientStop{{0, start}, {1, end}}
	}
	for i, s := range lg.Stops {
		if i > 0 && s.Offset < lg.Stops[i-1].Offset {
			return nil, malformed(r.Offset, "EMF+ gradient positions out of order")
		}
		// Translucent stops differ between straight and premultiplied
		// interpolation, and MS-EMFPLUS does not say which applies.
		if s.Color.A != lg.Stops[0].Color.A {
			return nil, p.unsupported(r, "EMF+ gradient with varying alpha")
		}
	}
	return &Paint{Kind: PaintLinearGradient, Gradient: lg}, nil
}

// plusBitmap decodes an image object once and caches it in its slot.
func (p *player) plusBitmap(r Record, obj *plusObject, img PlusImage) (image.Image, error) {
	if obj.decoded != nil {
		return obj.decoded, nil
	}
	im, err := p.decodePlusBitmap(r, img)
	if err == nil {
		obj.decoded = im
	}
	return im, err
}

// decodePlusBitmap decodes a bitmap image, charging the declared size before
// decoding and any excess of the decoded size after.
func (p *player) decodePlusBitmap(r Record, img PlusImage) (image.Image, error) {
	if img.Type != 1 {
		return nil, p.unsupported(r, "EMF+ metafile texture")
	}
	declared := uint64(max(img.Width, 0)) * uint64(max(img.Height, 0))
	if err := p.spend(r, declared); err != nil {
		return nil, err
	}
	im, err := img.ImageWithColorTransform(p.options.Images, p.options.ColorTransform)
	if err != nil {
		return nil, p.imageError(r, err)
	}
	b := im.Bounds()
	if n := uint64(b.Dx()) * uint64(b.Dy()); n > declared {
		if err := p.spend(r, n-declared); err != nil {
			return nil, err
		}
	}
	return im, nil
}

func (p *player) spend(r Record, n uint64) error {
	if n > p.options.MaxImagePixels-p.budget.pixels {
		return failure(r.Offset, "playback bitmap pixels", ErrLimit)
	}
	p.budget.pixels += n
	return nil
}

func isOpaque(im image.Image) bool {
	o, ok := im.(interface{ Opaque() bool })
	return ok && o.Opaque()
}

func paintTranslucent(p *Paint) bool {
	switch p.Kind {
	case PaintSolid:
		return p.Color.A != 255
	case PaintHatch:
		return p.Color.A != 255 || p.Background == nil || p.Background.A != 255
	case PaintLinearGradient:
		return p.Gradient.Stops[0].Color.A != 255
	case PaintPattern:
		return p.Wrap == WrapClamp || !isOpaque(p.Pattern)
	}
	return true
}

// plusComposite rejects SourceCopy compositing (MS-EMFPLUS 2.1.1.5) of
// translucent paint, which replaces destination pixels rather than blending.
// Opaque paint composites identically in both modes.
func (p *player) plusComposite(r Record, paint *Paint) error {
	if p.plus.compositing == 1 && paintTranslucent(paint) {
		return p.unsupported(r, "EMF+ SourceCopy compositing of translucent paint")
	}
	return nil
}

// plusCaps maps the line caps MS-EMFPLUS 2.1.1.17 fully specifies: NoAnchor
// ends the line at its last point like Flat, and SquareAnchor is a square
// centered on the last point with the line width as its side, like Square.
var plusCaps = map[uint32]LineCap{0: CapFlat, 1: CapSquare, 2: CapRound, 0x10: CapFlat, 0x11: CapSquare}

// plusCompound validates a compound line (MS-EMFPLUS 2.2.2.9). Which side
// of the line fraction 0 lies on is not specified, so only arrays symmetric
// about the center are drawn. How bands meet at joins is not specified: each
// band edge is joined like the whole pen, so a band is the stroke at its
// outer edge minus the stroke at its inner edge. With round joins that is
// exactly the points at the band's distances from the path, as Windows GDI+
// draws them; with miter joins it requires every corner within the miter
// limit (checked against the path in plusDraw), where GDI+ would bevel.
// Bevel joins are reported.
func (p *player) plusCompound(r Record, pen PlusPen, s *Stroke) error {
	c := pen.Compound
	if len(c)%2 != 0 {
		return malformed(r.Offset, "EMF+ compound line array")
	}
	for i, v := range c {
		if !(v >= 0 && v <= 1) || (i > 0 && v < c[i-1]) {
			return malformed(r.Offset, "EMF+ compound line array")
		}
		if math.Abs(v+c[len(c)-1-i]-1) > 1e-6 {
			return p.unsupported(r, "EMF+ asymmetric compound pen")
		}
	}
	switch {
	case pen.Width == 0:
		return p.unsupported(r, "EMF+ zero-width compound pen")
	case pen.Join == 1:
		// GDI+ joins the inner sides of bevelled band edges with crossing
		// connectors, which reach into the gaps between bands (ORACLES.md).
		return p.unsupported(r, "EMF+ compound pen with bevel joins")
	case pen.LineStyle != 0:
		return p.unsupported(r, "EMF+ dashed compound pen")
	}
	s.Compound = append([]float64(nil), c...)
	return nil
}

// compoundDrawable reports whether a compound stroke of path is exact: open
// figures need flat caps, and with miter joins every corner must stay within
// the miter limit, so that the bands' parallel lines are mitered like the
// full stroke.
func compoundDrawable(path Path, s *Stroke) (string, bool) {
	inv, ok := invertMatrix(s.Transform)
	if !ok {
		return "", true // a degenerate pen draws nothing
	}
	var figure []segmentTangents
	start, cur := Point{}, Point{}
	check := func(closed bool) (string, bool) {
		if len(figure) == 0 {
			return "", true
		}
		if !closed && (s.Cap != CapFlat || (s.EndCap != 0 && s.EndCap != CapFlat)) {
			return "EMF+ compound pen with non-flat caps on an open figure", false
		}
		n := len(figure)
		joins := n - 1
		if closed {
			joins = n
		}
		if s.Join != JoinMiter {
			joins = 0
		}
		for i := 0; i < joins; i++ {
			a, b := inv.Apply(figure[i].out), inv.Apply(figure[(i+1)%n].in)
			la, lb := math.Hypot(a.X, a.Y), math.Hypot(b.X, b.Y)
			if la == 0 || lb == 0 {
				continue
			}
			// The miter extends 1/cos(phi/2) half-widths for a turn of phi.
			cos := (a.X*b.X + a.Y*b.Y) / (la * lb)
			half := math.Sqrt(math.Max(0, (1+cos)/2))
			if half == 0 || 1/half > s.MiterLimit {
				return "EMF+ compound pen corner beyond the miter limit", false
			}
		}
		return "", true
	}
	k := 0
	for _, v := range path.Verbs {
		switch v {
		case PathMoveTo:
			if why, ok := check(false); !ok {
				return why, false
			}
			figure = figure[:0]
			start, cur = path.Points[k], path.Points[k]
			k++
		case PathLineTo:
			q := path.Points[k]
			if q != cur {
				d := Point{q.X - cur.X, q.Y - cur.Y}
				figure = append(figure, segmentTangents{d, d})
			}
			cur = q
			k++
		case PathCubicTo:
			c1, c2, q := path.Points[k], path.Points[k+1], path.Points[k+2]
			in := firstNonZero(Point{c1.X - cur.X, c1.Y - cur.Y}, Point{c2.X - cur.X, c2.Y - cur.Y}, Point{q.X - cur.X, q.Y - cur.Y})
			out := firstNonZero(Point{q.X - c2.X, q.Y - c2.Y}, Point{q.X - c1.X, q.Y - c1.Y}, Point{q.X - cur.X, q.Y - cur.Y})
			if in != (Point{}) {
				figure = append(figure, segmentTangents{in, out})
			}
			cur = q
			k += 3
		case PathClose:
			if cur != start {
				d := Point{start.X - cur.X, start.Y - cur.Y}
				figure = append(figure, segmentTangents{d, d})
			}
			if why, ok := check(true); !ok {
				return why, false
			}
			figure = figure[:0]
			cur = start
		}
	}
	return check(false)
}

type segmentTangents struct{ in, out Point }

func firstNonZero(v ...Point) Point {
	for _, p := range v {
		if p != (Point{}) {
			return p
		}
	}
	return Point{}
}

// GDI+ dash patterns in multiples of the pen width (MS-EMFPLUS 2.1.1.21).
var plusDashes = [...][]float64{1: {3, 1}, 2: {1, 1}, 3: {3, 1, 1, 1}, 4: {3, 1, 1, 1, 1, 1}}

// plusStroke resolves a pen (MS-EMFPLUS 2.2.1.7, 2.2.2.35). A World-unit
// width is in world units, shaped by the pen transform and then the world-to-
// destination transform; a Pixel-unit width is in device pixels.
func (p *player) plusStroke(r Record, id uint32, m Matrix) (*Stroke, [2]*PlusCustomLineCap, error) {
	var caps [2]*PlusCustomLineCap
	s, err := p.plusPen(r, id, m, &caps)
	return s, caps, err
}

func (p *player) plusPen(r Record, id uint32, m Matrix, caps *[2]*PlusCustomLineCap) (*Stroke, error) {
	obj, err := p.plusObjectAt(r, id)
	if err != nil {
		return nil, err
	}
	pen := obj.value.(PlusPen)
	startCap, okStart := plusCaps[pen.StartCap]
	endCap, okEnd := plusCaps[pen.EndCap]
	custom := pen.CustomStartCap != nil || pen.CustomEndCap != nil || pen.StartCap == 0xff || pen.EndCap == 0xff
	if custom {
		// MS-EMFPLUS does not define the coordinate system of cap paths or
		// where an adjustable arrow sits relative to the line end; see
		// playback_plus_caps.go for the opt-in interpretation.
		var base [2]LineCap
		if *caps, base, err = p.plusCustomCaps(r, pen, startCap, endCap); err != nil {
			return nil, err
		}
		startCap, endCap = base[0], base[1]
		okStart = okStart || caps[0] != nil
		okEnd = okEnd || caps[1] != nil
	}
	switch {
	case custom && (pen.LineStyle != 0 || len(pen.Compound) != 0 || pen.Width == 0):
		return nil, p.unsupported(r, "EMF+ custom line cap on a dashed, compound or zero-width pen")
	case !validPlusLineCap(pen.StartCap) || !validPlusLineCap(pen.EndCap):
		return nil, malformed(r.Offset, "EMF+ line cap")
	case !okStart || !okEnd:
		// The triangle's height and the round, diamond and arrow anchors'
		// sizes are not specified (MS-EMFPLUS 2.1.1.17).
		return nil, p.unsupported(r, "EMF+ triangle, round, diamond or arrow line cap")
	case pen.LineStyle != 0 && (startCap != CapFlat || endCap != CapFlat):
		// Line caps apply at the figure ends and the dash cap at dash ends,
		// which Stroke cannot express separately.
		return nil, p.unsupported(r, "EMF+ dashed pen with non-flat line caps")
	case pen.Join > 3:
		return nil, malformed(r.Offset, "EMF+ line join")
	case pen.Join == 3:
		return nil, p.unsupported(r, "EMF+ clipped miter join")
	case pen.Alignment != 0:
		return nil, p.unsupported(r, "EMF+ pen alignment other than center")
	case pen.LineStyle > 5:
		return nil, malformed(r.Offset, "EMF+ line style")
	case pen.LineStyle != 0 && pen.DashCap != 0:
		return nil, p.unsupported(r, "EMF+ dash caps")
	case pen.LineStyle != 0 && pen.Width == 0:
		return nil, p.unsupported(r, "EMF+ dashed zero-width pen")
	case pen.Unit != 0 && pen.Unit != 2:
		return nil, p.unsupported(r, "EMF+ pen width in a unit other than World or Pixel")
	case pen.Unit == 2 && pen.Transform != Identity():
		return nil, p.unsupported(r, "EMF+ pen transform on a Pixel-unit pen")
	case !finite(pen.Width) || pen.Width < 0:
		return nil, malformed(r.Offset, "EMF+ pen width")
	}
	paint, err := p.plusBrushPaint(r, pen.Brush, obj, m)
	if err != nil || paint == nil {
		return nil, err
	}
	if err := p.plusComposite(r, paint); err != nil {
		return nil, err
	}
	if custom && paintTranslucent(paint) {
		// Caps overlap the line; how GDI+ composites the overlap is unknown.
		return nil, p.unsupported(r, "EMF+ custom line cap with translucent paint")
	}
	miter := 10.0 // GDI+'s default
	if pen.Flags&16 != 0 {
		miter = pen.MiterLimit
	}
	s := &Stroke{Paint: *paint, Cap: startCap, Join: [...]LineJoin{JoinMiter, JoinBevel, JoinRound}[pen.Join], MiterLimit: miter, Dash: DashSolid}
	if endCap != startCap {
		s.EndCap = endCap
	}
	if len(pen.Compound) != 0 {
		if err := p.plusCompound(r, pen, s); err != nil {
			return nil, err
		}
	}
	switch {
	case pen.Width == 0:
		s.Hairline = true
	case pen.Unit == 0:
		s.Width, s.Transform = pen.Width, linear(pen.Transform.Then(m))
	default:
		s.Width, s.Transform = pen.Width, linear(p.base)
	}
	if pen.LineStyle != 0 {
		pattern := pen.Dashes
		if pen.LineStyle < 5 {
			pattern = plusDashes[pen.LineStyle]
		}
		if len(pattern) == 0 {
			return nil, malformed(r.Offset, "EMF+ custom dash pattern")
		}
		s.Dash = DashUser
		s.Dashes = make([]float64, len(pattern))
		for i, d := range pattern {
			s.Dashes[i] = d * s.Width
		}
		s.DashOffset = pen.DashOffset * s.Width
	}
	if !s.Transform.Finite() || !finite(s.DashOffset) {
		return nil, malformed(r.Offset, "non-finite EMF+ pen")
	}
	return s, nil
}

func (p *player) plusFill(r Record, path Path, rule FillRule, id uint32, solid bool, m Matrix, clip Clip) error {
	if !solid {
		obj, err := p.plusObjectAt(r, id)
		if err != nil {
			return err
		}
		if b := obj.value.(PlusBrush); b.Type == 3 {
			return p.plusPathGradientFill(r, b.PathGradient, path, rule, m, clip)
		}
	}
	paint, err := p.plusPaint(r, id, solid, m)
	if err != nil || paint == nil {
		return err
	}
	if err := p.plusComposite(r, paint); err != nil {
		return err
	}
	return p.backend.FillPath(path, rule, *paint, clip)
}

func (p *player) plusDraw(r Record, path Path, id uint32, m Matrix) error {
	s, caps, err := p.plusStroke(r, id, m)
	if err != nil || s == nil {
		return err
	}
	if len(s.Compound) != 0 {
		if why, ok := compoundDrawable(path, s); !ok {
			return p.unsupported(r, why)
		}
	}
	if caps[0] != nil || caps[1] != nil {
		return p.plusDrawCaps(r, path, s, caps)
	}
	return p.backend.StrokePath(path, *s, p.plusCurrentClip())
}

func (p *player) destinationPath() Path {
	d := p.options.Destination
	return boxPath(Box{d.X, d.Y, d.Width, d.Height}, Identity())
}

// plusClear fills the clip with a color (MS-EMFPLUS 2.3.4.1). Clear replaces
// pixels, which only an opaque color does by source-over painting.
func (p *player) plusClear(c Command, v uint32) error {
	if p.plus.skip {
		return nil
	}
	col := argb(v)
	if col.A != 255 {
		return p.unsupported(c.Source, "EMF+ Clear with a translucent color")
	}
	return p.backend.FillPath(p.destinationPath(), NonZero, Paint{Kind: PaintSolid, Color: col}, p.plusCurrentClip())
}

func (p *player) plusRects(c Command, v PlusRects) error {
	r := c.Source
	m, err := p.plusMatrix(r)
	if err != nil {
		return err
	}
	if r.Type == PlusFillRectsRecord {
		// One fill: overlapping rectangles paint once, as one region.
		b := pathBuilder{limit: p.options.MaxPathPoints}
		s := shape{&b, m}
		for i := 0; i < v.Rectangles.Len(); i++ {
			q := v.Rectangles.At(i)
			s.moveTo(Point{q.X, q.Y})
			s.lineTo(Point{q.X + q.Width, q.Y})
			s.lineTo(Point{q.X + q.Width, q.Y + q.Height})
			s.lineTo(Point{q.X, q.Y + q.Height})
			b.close()
		}
		if b.err {
			return failure(r.Offset, "path points", ErrLimit)
		}
		return p.plusFill(r, b.path, NonZero, v.BrushID, v.Solid, m, p.plusCurrentClip())
	}
	for i := 0; i < v.Rectangles.Len(); i++ {
		if err := p.plusDraw(r, boxPath(v.Rectangles.At(i), m), uint32(v.ObjectID), m); err != nil {
			return err
		}
	}
	return nil
}

func (p *player) plusPoly(c Command, v PlusPoly) error {
	r := c.Source
	m, err := p.plusMatrix(r)
	if err != nil {
		return err
	}
	b := pathBuilder{limit: p.options.MaxPathPoints}
	s := shape{&b, m}
	n := v.Points.Len()
	s.moveTo(v.Points.At(0))
	if r.Type == PlusDrawBeziersRecord {
		// The decoder requires 3k+1 points.
		for i := 1; i+2 < n; i += 3 {
			s.cubicTo(v.Points.At(i), v.Points.At(i+1), v.Points.At(i+2))
		}
	} else {
		for i := 1; i < n; i++ {
			s.lineTo(v.Points.At(i))
		}
	}
	if v.Closed {
		b.close()
	}
	if b.err {
		return failure(r.Offset, "path points", ErrLimit)
	}
	if r.Type == PlusFillPolygonRecord {
		return p.plusFill(r, b.path, EvenOdd, v.BrushID, v.Solid, m, p.plusCurrentClip())
	}
	return p.plusDraw(r, b.path, uint32(v.ObjectID), m)
}

// plusArcParam converts a GDI+ angle, measured clockwise as displayed from
// the x axis to the ray through the arc point, to the parameter of
// ellipsePoint, which runs counterclockwise as displayed.
func plusArcParam(deg float64, rad Point) float64 {
	s, c := math.Sincos(deg * math.Pi / 180)
	return -math.Atan2(s*rad.X, c*rad.Y)
}

func (p *player) plusEllipse(c Command, v PlusEllipse) error {
	r := c.Source
	m, err := p.plusMatrix(r)
	if err != nil {
		return err
	}
	b := pathBuilder{limit: 64}
	s := shape{&b, m}
	center := Point{v.Rect.X + v.Rect.Width/2, v.Rect.Y + v.Rect.Height/2}
	rad := Point{math.Abs(v.Rect.Width) / 2, math.Abs(v.Rect.Height) / 2}
	switch r.Type {
	case PlusFillEllipseRecord, PlusDrawEllipseRecord:
		s.moveTo(ellipsePoint(center, rad, 0))
		s.arc(center, rad, 0, -2*math.Pi)
		b.close()
	default:
		// Sweeps are clockwise when positive and limited to a full turn.
		sweep := math.Max(-360, math.Min(360, v.SweepAngle))
		if !finite(v.StartAngle) || !finite(sweep) {
			return malformed(r.Offset, "EMF+ arc angles")
		}
		if sweep == 0 && r.Type == PlusDrawArcRecord {
			return nil // an empty arc has no outline
		}
		t0 := plusArcParam(v.StartAngle, rad)
		var d float64
		if math.Abs(sweep) == 360 {
			d = -math.Copysign(2*math.Pi, sweep)
		} else if sweep != 0 {
			d = plusArcParam(v.StartAngle+sweep, rad) - t0
			for sweep > 0 && d > 0 {
				d -= 2 * math.Pi
			}
			for sweep < 0 && d < 0 {
				d += 2 * math.Pi
			}
		}
		s.moveTo(ellipsePoint(center, rad, t0))
		if d != 0 {
			s.arc(center, rad, t0, d)
		}
		if r.Type != PlusDrawArcRecord {
			s.lineTo(center)
			b.close()
		}
	}
	if r.Type == PlusFillEllipseRecord || r.Type == PlusFillPieRecord {
		return p.plusFill(r, b.path, NonZero, v.BrushID, v.Solid, m, p.plusCurrentClip())
	}
	return p.plusDraw(r, b.path, uint32(v.ObjectID), m)
}

func (p *player) plusPathDraw(c Command, v PlusPathDraw) error {
	r := c.Source
	m, err := p.plusMatrix(r)
	if err != nil {
		return err
	}
	obj, err := p.plusObjectAt(r, uint32(v.ObjectID))
	if err != nil {
		return err
	}
	switch r.Type {
	case PlusFillRegionRecord:
		tree, err := p.plusRegion(r, obj.value.(PlusRegion), m)
		if err != nil {
			return err
		}
		return p.plusFill(r, p.destinationPath(), NonZero, v.PaintID, v.Solid, m, append(p.plusCurrentClip(), tree))
	case PlusFillPathRecord:
		path, err := p.plusPath(r, obj.value.(PlusPath), m, true)
		if err != nil {
			return err
		}
		return p.plusFill(r, path, EvenOdd, v.PaintID, v.Solid, m, p.plusCurrentClip())
	}
	path, err := p.plusPath(r, obj.value.(PlusPath), m, false)
	if err != nil {
		return err
	}
	return p.plusDraw(r, path, v.PaintID, m)
}

// plusCurve draws a cardinal spline as Béziers: the segment from P[i] to
// P[i+1] has control points P[i]+T/3*(P[i+1]-P[i-1]) and
// P[i+1]-T/3*(P[i+2]-P[i]). Open curves repeat their end points; closed
// curves wrap around.
func (p *player) plusCurve(c Command, v PlusCurve) error {
	r := c.Source
	m, err := p.plusMatrix(r)
	if err != nil {
		return err
	}
	if !finite(v.Tension) {
		return malformed(r.Offset, "EMF+ curve tension")
	}
	n := v.Points.Len()
	at := func(i int) Point {
		if v.Closed {
			return v.Points.At(((i % n) + n) % n)
		}
		return v.Points.At(max(0, min(n-1, i)))
	}
	first, segments := 0, n
	if !v.Closed {
		// The decoder bounds Offset+Segments below n.
		first, segments = int(v.Offset), int(v.Segments)
	}
	k := v.Tension / 3
	b := pathBuilder{limit: p.options.MaxPathPoints}
	s := shape{&b, m}
	s.moveTo(at(first))
	for i := first; i < first+segments; i++ {
		p0, p1, p2, p3 := at(i-1), at(i), at(i+1), at(i+2)
		s.cubicTo(Point{p1.X + k*(p2.X-p0.X), p1.Y + k*(p2.Y-p0.Y)}, Point{p2.X - k*(p3.X-p1.X), p2.Y - k*(p3.Y-p1.Y)}, p2)
	}
	if v.Closed {
		b.close()
	}
	if b.err {
		return failure(r.Offset, "path points", ErrLimit)
	}
	if r.Type == PlusFillClosedCurveRecord {
		rule := EvenOdd
		if v.Winding {
			rule = NonZero
		}
		return p.plusFill(r, b.path, rule, v.BrushID, v.Solid, m, p.plusCurrentClip())
	}
	return p.plusDraw(r, b.path, uint32(v.ObjectID), m)
}

// plusImage places a bitmap (MS-EMFPLUS 2.3.4.8, 2.3.4.9): the source
// rectangle, in pixels, maps onto the destination rectangle or onto the
// parallelogram of the upper-left, upper-right and lower-left points.
func (p *player) plusImage(c Command, v PlusImageDraw) error {
	r := c.Source
	if v.Effect {
		return p.unsupported(r, "EMF+ image effect")
	}
	src := v.Source
	if !finite(src.X) || !finite(src.Y) || !finite(src.Width) || !finite(src.Height) {
		return malformed(r.Offset, "non-finite EMF+ image source")
	}
	if src.Width <= 0 || src.Height <= 0 {
		return nil
	}
	obj, err := p.plusObjectAt(r, uint32(v.ImageID))
	if err != nil {
		return err
	}
	m, err := p.plusMatrix(r)
	if err != nil {
		return err
	}
	img := obj.value.(PlusImage)
	var im image.Image
	var meta *embeddedMetafile
	var bounds image.Rectangle
	var lo, hi Point // the image's extent in image pixels
	if img.Type == 2 {
		if meta, err = p.metafileImage(r, obj, img); err != nil || meta == nil {
			return err
		}
		lo, hi = meta.origin, Point{meta.origin.X + meta.size.X, meta.origin.Y + meta.size.Y} // ivy: see embeddedMetafile
	} else {
		if im, err = p.plusBitmap(r, obj, img); err != nil || im == nil {
			return err
		}
		bounds = im.Bounds()
		lo, hi = Point{float64(bounds.Min.X), float64(bounds.Min.Y)}, Point{float64(bounds.Max.X), float64(bounds.Max.Y)}
	}
	// ivy: the source rectangle is in float32, so a rectangle that is the
	// frame of a metafile image can be outside it by a rounding error.
	const eps = 1e-3
	inside := src.X >= lo.X-eps && src.Y >= lo.Y-eps && src.X+src.Width <= hi.X+eps && src.Y+src.Height <= hi.Y+eps
	// ivy: wrap holds the attributes of a bitmap whose pixels outside the
	// image are not transparent. plusImageWrap draws them.
	var wrap *PlusImageAttributes
	if !inside {
		// Pixels outside the image come from the attributes' wrap mode;
		// only transparent clamping matches drawing the image alone. Without
		// attributes, Windows GDI+ draws nothing outside the image, as with
		// transparent clamping (ORACLES.md).
		clamped := true
		var attrs PlusImageAttributes
		if v.AttributesID != 0xffffffff {
			a, err := p.plusObjectAt(r, v.AttributesID)
			if err != nil {
				return err
			}
			attrs = a.value.(PlusImageAttributes)
			clamped = attrs.WrapMode == 4 && argb(attrs.ClampColor).A == 0
		}
		if !clamped {
			if meta != nil || attrs.WrapMode > 4 {
				return p.unsupported(r, "EMF+ image source outside the image")
			}
			wrap = &attrs
		}
	}
	var place Matrix
	if r.Type == PlusDrawImageRecord {
		d := v.Destination
		sx, sy := d.Width/src.Width, d.Height/src.Height
		place = Matrix{M11: sx, M22: sy, Dx: d.X - src.X*sx, Dy: d.Y - src.Y*sy}
	} else {
		p0, p1, p2 := v.Points.At(0), v.Points.At(1), v.Points.At(2)
		place = Matrix{M11: (p1.X - p0.X) / src.Width, M12: (p1.Y - p0.Y) / src.Width, M21: (p2.X - p0.X) / src.Height, M22: (p2.Y - p0.Y) / src.Height}
		place.Dx = p0.X - src.X*place.M11 - src.Y*place.M21
		place.Dy = p0.Y - src.X*place.M12 - src.Y*place.M22
	}
	place = place.Then(m)
	if !place.Finite() {
		return malformed(r.Offset, "non-finite EMF+ image placement")
	}
	if meta != nil {
		if p.plus.compositing == 1 {
			return p.unsupported(r, "EMF+ SourceCopy compositing of a metafile image")
		}
		// Only the source rectangle's part of the picture is drawn.
		x0, y0 := max(src.X, lo.X), max(src.Y, lo.Y)
		x1, y1 := min(src.X+src.Width, hi.X), min(src.Y+src.Height, hi.Y)
		if x0 >= x1 || y0 >= y1 {
			return nil
		}
		area := Path{Verbs: []PathVerb{PathMoveTo, PathLineTo, PathLineTo, PathLineTo, PathClose},
			Points: []Point{place.Apply(Point{x0, y0}), place.Apply(Point{x1, y0}), place.Apply(Point{x1, y1}), place.Apply(Point{x0, y1})}}
		clip := p.plusCurrentClip()
		clip = append(clip[:len(clip):len(clip)], &ClipRegion{Op: ClipReplace, Area: area, Rule: NonZero, depth: 1})
		// ivy: the embedded picture plays into [0, size], which starts at
		// the origin of the image.
		shift := Matrix{M11: 1, M22: 1, Dx: meta.origin.X, Dy: meta.origin.Y}
		return p.playEmbedded(r, meta, shift.Then(place), clip)
	}
	if p.plus.compositing == 1 && !isOpaque(im) {
		return p.unsupported(r, "EMF+ SourceCopy compositing of an image with alpha")
	}
	if wrap != nil {
		if done, err := p.plusImageWrap(r, im, place, src, *wrap); done || err != nil {
			return err
		}
	}
	draw := ImageDraw{Image: im, Transform: place, Opacity: 1, Smooth: p.plus.interpolation != 5}
	o, size := Point{src.X, src.Y}, Point{src.Width, src.Height}
	draw.Source = sourceRect(o, size, bounds)
	if draw.Source.Empty() {
		return nil
	}
	return p.backend.DrawImage(draw, sourceClip(draw, o, size, p.plusCurrentClip()))
}

// ivy: plusImageWrap draws the part of the source rectangle src of a bitmap
// that is outside the image im, with the wrap mode of the image attributes,
// as GDI+ does. A tile mode repeats the image over the whole rectangle, and
// it reports done. The clamp mode fills the part outside the image with the
// clamp color, and the caller then draws the image.
func (p *player) plusImageWrap(r Record, im image.Image, place Matrix, src Box, attrs PlusImageAttributes) (done bool, err error) {
	quad := func(x0, y0, x1, y1 float64) Path {
		return Path{Verbs: []PathVerb{PathMoveTo, PathLineTo, PathLineTo, PathLineTo, PathClose},
			Points: []Point{place.Apply(Point{x0, y0}), place.Apply(Point{x1, y0}), place.Apply(Point{x1, y1}), place.Apply(Point{x0, y1})}}
	}
	area := quad(src.X, src.Y, src.X+src.Width, src.Y+src.Height)
	clip := p.plusCurrentClip()
	if attrs.WrapMode < 4 {
		paint := Paint{Kind: PaintPattern, Pattern: im, PatternTransform: place, Wrap: WrapMode(attrs.WrapMode)}
		return true, p.backend.FillPath(area, NonZero, paint, clip)
	}
	paint := Paint{Kind: PaintSolid, Color: argb(attrs.ClampColor)}
	if err := p.plusComposite(r, &paint); err != nil {
		return true, err
	}
	// the area less the part of the image in it, with the even-odd rule
	b := im.Bounds()
	x0, y0 := max(src.X, float64(b.Min.X)), max(src.Y, float64(b.Min.Y))
	x1, y1 := min(src.X+src.Width, float64(b.Max.X)), min(src.Y+src.Height, float64(b.Max.Y))
	if x0 < x1 && y0 < y1 {
		inner := quad(x0, y0, x1, y1)
		area.Verbs = append(area.Verbs, inner.Verbs...)
		area.Points = append(area.Points, inner.Points...)
	}
	return false, p.backend.FillPath(area, EvenOdd, paint, clip)
}

// plusHatch draws an EMF+ hatch brush as GDI+ does: its 8x8 pattern of
// foreground coverage (plusHatchCoverage), repeated over device pixels from
// the rendering origin. Pattern pixels blend the two colors by coverage; only
// the anti-aliased diagonals of styles 2, 3 and 5 have partial coverage, and
// how GDI+ blends those with translucent colors is unknown.
func (p *player) plusHatch(r Record, b PlusBrush) (*Paint, error) {
	if b.Hatch >= uint32(len(plusHatchCoverage)) {
		return nil, p.unsupported(r, fmt.Sprintf("EMF+ hatch style %d", b.Hatch))
	}
	fg, bg := argb(b.Color), argb(b.BackColor)
	cell := &plusHatchCoverage[b.Hatch]
	im := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			c := cell[y][x]
			if c != 0 && c != 255 && (fg.A != 255 || bg.A != 255) {
				return nil, p.unsupported(r, "EMF+ translucent anti-aliased hatch")
			}
			mix := func(f, g uint8) uint8 { return uint8((int(g)*(255-int(c)) + int(f)*int(c) + 127) / 255) }
			im.SetNRGBA(x, y, color.NRGBA{mix(fg.R, bg.R), mix(fg.G, bg.G), mix(fg.B, bg.B), mix(fg.A, bg.A)})
		}
	}
	o := p.plus.origin
	return &Paint{Kind: PaintPattern, Pattern: im, PatternTransform: Matrix{M11: 1, M22: 1, Dx: o.X, Dy: o.Y}.Then(p.base), Wrap: WrapTile}, nil
}
