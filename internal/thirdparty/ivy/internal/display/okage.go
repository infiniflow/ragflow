package display

import (
	"image"
	"math"
	"sync"

	"github.com/xo/okage"
	"ragflow/internal/thirdparty/ivy/internal/raster"
)

// The bounds of the caches of an OkageDevice.
const (
	// maxMasks is the number of soft masks that a device keeps. A mask
	// holds one byte for each pixel of the target.
	maxMasks = 16
	// maxLayers is the number of free layers that a device keeps for the
	// next group.
	maxLayers = 4
)

// maxFineEm is the largest size of a glyph, an em in pixels, that a device
// fills with the coverage of 8 by 8 samples of okage (D46). On a larger
// glyph, a step of a quarter of a pixel is a smaller part of a stem, and
// the glyph fills as FillPath fills it.
const maxFineEm = 48

// OkageDevice draws the commands onto a pixmap with okage. It is not safe for
// concurrent use: a page draws with a device of its own.
type OkageDevice struct {
	// dst is the pixmap that the commands draw onto: the target, or the layer
	// of the innermost group that has a layer.
	dst *okage.Pixmap
	// alpha is the alpha of that group alone, for a group that is not
	// isolated and that blends with its backdrop, or nil. Each command
	// draws onto dst and onto alpha.
	alpha  *okage.Pixmap
	state  state
	stack  []state
	groups []group

	images sync.Map // image.Image -> *okage.Pixmap
	// masks holds the soft masks with a key that the device drew.
	masks []cachedMask
	// free holds the layers that a group can use again. Each is
	// transparent.
	free []*okage.Pixmap
	// scratch is the mask that EndGroup composes from the soft mask and
	// the clip of a group.
	scratch *okage.Mask
	// regions holds the copies of the pixels of a rectangle that the
	// compositing of a group gives to okage.
	regions [4][]byte
	// area holds the pixels of dst that matter: the whole target, or the
	// pixels of a soft mask that its group needs. An object outside area
	// draws nothing.
	area image.Rectangle
	// aliased draws the paths and the clips without antialiasing.
	aliased bool
	// whole composites each group over the bounds of its clip, and draws
	// its soft mask there, as the device did before it kept the drawn
	// pixels of a group. The tests set it to compare the pixels of the two.
	whole bool
}

// state is the part of the device that Save pushes.
type state struct {
	t    okage.Transform
	clip *okage.Mask
	// bounds holds every pixel that clip does not hide: the bounds of the
	// target when clip is nil. A group draws and composites only inside
	// the bounds of its clip.
	bounds image.Rectangle
}

// group is a group that BeginGroup started.
type group struct {
	// parent is the pixmap that the group draws onto at its end. It holds
	// the backdrop of the group.
	parent *okage.Pixmap
	// layer is the pixmap that the group draws to, or nil when the group
	// draws straight onto its parent.
	layer *okage.Pixmap
	// alpha is the alpha of the group alone, or nil. A group that is not
	// isolated gets it when an object of the group blends with the
	// backdrop, and the layer then holds the backdrop too.
	alpha  *okage.Pixmap
	g      Group
	bounds image.Rectangle
	// drawn holds each pixel of the layer that the group alone can cover:
	// the union of the bounds of the objects and of the nested groups that
	// drew onto the layer. Outside drawn, the group alone is transparent,
	// and EndGroup does not composite it there.
	drawn image.Rectangle
	depth int
}

// cachedMask is a soft mask with a key that a device drew.
type cachedMask struct {
	key any
	t   okage.Transform
	// bounds holds the pixels of the pixmap that the content drew onto, and
	// area the pixels of the mask, inside bounds.
	bounds, area image.Rectangle
	mask         *okage.Mask
}

// The interfaces that an OkageDevice implements.
var (
	_ Device         = (*OkageDevice)(nil)
	_ TextFiller     = (*OkageDevice)(nil)
	_ StrokeAdjuster = (*OkageDevice)(nil)
)

// NewOkage returns a device that draws onto dst, with the transform t from
// the user space to the pixels of dst.
func NewOkage(dst *okage.Pixmap, t okage.Transform) *OkageDevice {
	r := image.Rect(0, 0, int(dst.Width()), int(dst.Height()))
	return &OkageDevice{dst: dst, state: state{t: t, bounds: r}, area: r}
}

// Save satisfies the Device interface.
func (d *OkageDevice) Save() {
	d.stack = append(d.stack, d.state)
}

// Restore satisfies the Device interface. A Restore without a Save does
// nothing, and the content of a group cannot restore a state from before
// the group.
func (d *OkageDevice) Restore() {
	if len(d.stack) == 0 {
		return
	}
	if n := len(d.groups); n > 0 && len(d.stack) <= d.groups[n-1].depth+1 {
		return
	}
	d.state = d.stack[len(d.stack)-1]
	d.stack = d.stack[:len(d.stack)-1]
}

// Concat satisfies the Device interface.
func (d *OkageDevice) Concat(t okage.Transform) {
	d.state.t = d.state.t.PreConcat(t)
}

// SetAntiAlias turns the antialiasing of the paths, the glyphs and the clips
// on or off. It is on by default. Without it, a pixel is covered or not, as
// in Ghostscript with -dGraphicsAlphaBits=1, which the tests of the layout
// compare with.
func (d *OkageDevice) SetAntiAlias(on bool) {
	d.aliased = !on
}

// paint returns a paint of okage with the shader. For the shape of an
// object in a knockout group, the shader is an opaque black, so that the
// alpha of each pixel is its coverage.
func (d *OkageDevice) paint(shader okage.Shader, shape bool) okage.Paint {
	p := okage.DefaultPaint()
	if !shape {
		p.Shader = shader
	}
	p.AntiAlias = !d.aliased
	return p
}

// draw draws one object. paint draws the object onto dst. When shape is
// true, it draws the coverage of the object, in an opaque color and at an
// opacity of 1. The object draws onto the pixmap of the innermost group,
// and onto the alpha of that group when the group has one. In a knockout
// group, bounds gives the pixels that the object can cover, and the object
// replaces the earlier objects of the group there. An object whose bounds
// are outside the bounds of the clip draws nothing, and draw skips it, so
// that a band of a page pays only for the objects of the band. An object
// outside the area of the device draws nothing too. draw adds the bounds of
// the object inside the clip to the drawn pixels of the group.
func (d *OkageDevice) draw(bounds func() image.Rectangle, paint func(dst *okage.Pixmap, shape bool)) {
	b := bounds()
	if !d.visible(b) {
		return
	}
	if i := d.top(); i >= 0 {
		g := &d.groups[i]
		g.drawn = g.drawn.Union(b.Intersect(d.state.bounds))
		if g.g.Knockout {
			d.knockout(g, b, paint)
			return
		}
	}
	paint(d.dst, false)
	if d.alpha != nil {
		paint(d.alpha, false)
	}
}

// visible reports whether an object with the bounds b can change a pixel
// that matters: a pixel inside the clip and inside the area of the device.
func (d *OkageDevice) visible(b image.Rectangle) bool {
	return b.Overlaps(d.state.bounds) && b.Overlaps(d.area)
}

// FillPath satisfies the Device interface.
func (d *OkageDevice) FillPath(p *okage.Path, rule okage.FillRule, shader okage.Shader) {
	d.fillPath(p, rule, shader, okage.Coverage4x4)
}

// fillPath fills p with the shader and the grid of samples of coverage.
func (d *OkageDevice) fillPath(p *okage.Path, rule okage.FillRule, shader okage.Shader, coverage okage.Coverage) {
	if p == nil || shader == nil {
		return
	}
	d.draw(func() image.Rectangle { return d.pathBounds(p) }, func(dst *okage.Pixmap, shape bool) {
		pt := d.paint(shader, shape)
		pt.Coverage = coverage
		dst.FillPath(p, &pt, rule, d.state.t, d.state.clip)
	})
}

// StrokePath satisfies the Device interface.
func (d *OkageDevice) StrokePath(p *okage.Path, stroke okage.Stroke, shader okage.Shader) {
	d.strokePath(p, stroke, shader, false)
}

// StrokeAdjusted satisfies the StrokeAdjuster interface. It strokes p with
// Paint.StrokeAdjust of okage.
func (d *OkageDevice) StrokeAdjusted(p *okage.Path, stroke okage.Stroke, shader okage.Shader) {
	d.strokePath(p, stroke, shader, true)
}

// strokePath strokes p with the shader, and with the stroke adjustment of
// okage when adjust is true.
func (d *OkageDevice) strokePath(p *okage.Path, stroke okage.Stroke, shader okage.Shader, adjust bool) {
	if p == nil || shader == nil {
		return
	}
	d.draw(func() image.Rectangle { return d.strokeBounds(p, &stroke) }, func(dst *okage.Pixmap, shape bool) {
		pt := d.paint(shader, shape)
		pt.StrokeAdjust = adjust
		dst.StrokePath(p, &pt, &stroke, d.state.t, d.state.clip)
	})
}

// ClipPath satisfies the Device interface. An empty path clips everything.
// A clip inside a clip clones the mask of the state, and only the first clip
// makes a new mask. A clip outside the bounds of the clip of the state makes
// no mask: its bounds are empty, and each object then draws nothing.
func (d *OkageDevice) ClipPath(p *okage.Path, rule okage.FillRule) {
	bounds := d.state.bounds.Intersect(d.pathBounds(p))
	if bounds.Empty() {
		d.state.bounds = image.Rectangle{}
		return
	}
	var clip *okage.Mask
	if p != nil && d.state.clip != nil {
		clip = d.state.clip.Clone()
		clip.IntersectPath(p, rule, !d.aliased, d.state.t)
	} else {
		m, ok := okage.NewMask(d.dst.Width(), d.dst.Height())
		if !ok {
			return
		}
		if p != nil {
			m.FillPath(p, rule, !d.aliased, d.state.t)
		}
		clip = m
	}
	d.state.bounds = bounds
	d.state.clip = clip
}

// pathBounds returns the pixels that a fill of p can cover under the
// transform of the state, with a margin of a pixel for the antialiasing. An
// empty path covers no pixel.
func (d *OkageDevice) pathBounds(p *okage.Path) image.Rectangle {
	if p == nil {
		return image.Rectangle{}
	}
	return d.rectBounds(p.Bounds(), d.state.t, 1)
}

// strokeBounds returns the pixels that a stroke of p can cover under the
// transform of the state. The margin holds half the width, times the miter
// limit, at the largest scale of the transform, and two pixels for a
// hairline and for the stroke adjustment.
func (d *OkageDevice) strokeBounds(p *okage.Path, stroke *okage.Stroke) image.Rectangle {
	t := d.state.t
	// the norm of Frobenius is at least the largest scale of t
	scale := math.Sqrt(float64(t.SX*t.SX + t.KX*t.KX + t.KY*t.KY + t.SY*t.SY))
	pad := float64(stroke.Width)/2*math.Max(float64(stroke.MiterLimit), math.Sqrt2)*scale + 2
	if !(pad < 1<<20) {
		return d.state.bounds
	}
	return d.rectBounds(p.Bounds(), t, int(math.Ceil(pad)))
}

// rectBounds returns the pixels that r covers under the transform t, with
// a margin of pad pixels, or the bounds of the state when they are too
// large.
func (d *OkageDevice) rectBounds(r okage.Rect, t okage.Transform, pad int) image.Rectangle {
	b, ok := r.Transform(t)
	if !ok {
		return d.state.bounds
	}
	l, tp := float64(b.Left()), float64(b.Top())
	rt, bt := float64(b.Right()), float64(b.Bottom())
	const limit = 1 << 30
	if !(l > -limit && tp > -limit && rt < limit && bt < limit) {
		return d.state.bounds
	}
	return image.Rect(int(math.Floor(l))-pad, int(math.Floor(tp))-pad, int(math.Ceil(rt))+pad, int(math.Ceil(bt))+pad)
}

// imageBounds returns the pixels that an image of size covers under the
// transform t of its pixels, with a margin of two pixels for the filter.
func (d *OkageDevice) imageBounds(size image.Point, t okage.Transform) image.Rectangle {
	r, ok := okage.RectFromXYWH(0, 0, float32(size.X), float32(size.Y))
	if !ok {
		return d.state.bounds
	}
	return d.rectBounds(r, d.state.t.PreConcat(t), 2)
}

// pixmap returns the pixmap of img. The device keeps the pixmap of each
// image that needs a conversion, for an image that a page draws more than
// once. An *image.RGBA from the origin needs none.
func (d *OkageDevice) pixmap(img image.Image) *okage.Pixmap {
	if rgba, ok := img.(*image.RGBA); ok && rgba.Rect.Min == (image.Point{}) && rgba.Stride == 4*rgba.Rect.Dx() {
		pm, err := raster.Pixmap(rgba)
		if err != nil {
			return nil
		}
		return pm
	}
	pix, ok := d.images.Load(img)
	if !ok {
		p, err := raster.Pixmap(img)
		if err != nil {
			return nil
		}
		pix, _ = d.images.LoadOrStore(img, p)
	}
	pm, _ := pix.(*okage.Pixmap)
	return pm
}

// quality returns the filter of okage for the options.
func quality(opts ImageOptions) okage.FilterQuality {
	if opts.Smooth {
		return okage.FilterQualityBilinear
	}
	return okage.FilterQualityNearest
}

// DrawImage satisfies the Device interface. The shape of an image in a
// knockout group is its alpha.
func (d *OkageDevice) DrawImage(img image.Image, t okage.Transform, opts ImageOptions) {
	if img == nil || !d.visible(d.imageBounds(img.Bounds().Size(), t)) {
		return
	}
	pm := d.pixmap(img)
	if pm == nil {
		return
	}
	size := img.Bounds().Size()
	d.draw(func() image.Rectangle { return d.imageBounds(size, t) }, func(dst *okage.Pixmap, shape bool) {
		pp := okage.DefaultPixmapPaint()
		pp.Opacity = opts.Opacity
		if shape {
			pp.Opacity = 1
		}
		pp.Quality = quality(opts)
		dst.DrawPixmap(0, 0, pm.AsRef(), &pp, d.state.t.PreConcat(t), d.state.clip)
	})
}

// FillMask satisfies the Device interface. It colors the stencil directly
// for a solid color. For another shader, it draws the stencil into a mask of
// the size of the target, and fills the target through it.
func (d *OkageDevice) FillMask(mask *image.Alpha, t okage.Transform, opts ImageOptions, shader okage.Shader) {
	if mask == nil || shader == nil || mask.Rect.Empty() || !d.visible(d.imageBounds(mask.Rect.Size(), t)) {
		return
	}
	pp := okage.DefaultPixmapPaint()
	pp.Opacity = opts.Opacity
	pp.Quality = quality(opts)
	tt := d.state.t.PreConcat(t)
	bounds := func() image.Rectangle { return d.imageBounds(mask.Rect.Size(), t) }
	if c, ok := shader.(okage.SolidColor); ok {
		pm := stencil(mask, okage.Color(c))
		if pm == nil {
			return
		}
		d.draw(bounds, func(dst *okage.Pixmap, shape bool) {
			src, p := pm, pp
			if shape {
				if src = stencil(mask, okage.ColorBlack); src == nil {
					return
				}
				p.Opacity = 1
			}
			dst.DrawPixmap(0, 0, src.AsRef(), &p, tt, d.state.clip)
		})
		return
	}
	pm := stencil(mask, okage.ColorBlack)
	layer, ok := okage.NewPixmap(d.dst.Width(), d.dst.Height())
	if pm == nil || !ok {
		return
	}
	layer.DrawPixmap(0, 0, pm.AsRef(), &pp, tt, nil)
	cover := intersect(okage.MaskFromPixmap(layer.AsRef(), okage.MaskTypeAlpha), d.state.clip)
	r, ok := okage.RectFromXYWH(0, 0, float32(d.dst.Width()), float32(d.dst.Height()))
	if !ok {
		return
	}
	d.draw(bounds, func(dst *okage.Pixmap, shape bool) {
		pt := d.paint(shader.Transform(d.state.t), shape)
		dst.FillRect(r, &pt, okage.IdentityTransform(), cover)
	})
}

// stencil returns a pixmap of the color c, with the alpha of each pixel of
// mask as its coverage.
func stencil(mask *image.Alpha, c okage.Color) *okage.Pixmap {
	b := mask.Rect
	pm, ok := okage.NewPixmap(uint32(b.Dx()), uint32(b.Dy()))
	if !ok {
		return nil
	}
	pc := c.Premultiply().ToColorU8()
	r, g, bl, a := uint32(pc.Red()), uint32(pc.Green()), uint32(pc.Blue()), uint32(pc.Alpha())
	data := pm.DataMut()
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		row := mask.Pix[mask.PixOffset(b.Min.X, y):]
		for x := range b.Dx() {
			m := uint32(row[x])
			data[i] = uint8((r*m + 127) / 255)
			data[i+1] = uint8((g*m + 127) / 255)
			data[i+2] = uint8((bl*m + 127) / 255)
			data[i+3] = uint8((a*m + 127) / 255)
			i += 4
		}
	}
	return pm
}

// intersect returns the product of the masks a and b. A nil mask covers
// everything. It changes a in place when b is not nil.
func intersect(a, b *okage.Mask) *okage.Mask {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	}
	ad, bd := a.DataMut(), b.Data()
	for i := range min(len(ad), len(bd)) {
		ad[i] = uint8((uint32(ad[i])*uint32(bd[i]) + 127) / 255)
	}
	return a
}

// FillGlyphs satisfies the Device interface. It fills a run of small
// glyphs with the finer coverage, as FillText does.
func (d *OkageDevice) FillGlyphs(run GlyphRun, shader okage.Shader) {
	if !d.visible(d.glyphBounds(run)) {
		return
	}
	p := run.Path()
	if p == nil || shader == nil {
		return
	}
	t := d.state.t.PreConcat(run.Transform)
	em := float64(run.Size) * math.Sqrt(math.Abs(float64(t.SX)*float64(t.SY)-float64(t.KX)*float64(t.KY)))
	coverage := okage.Coverage4x4
	if em <= maxFineEm {
		coverage = okage.Coverage8x8
	}
	d.fillPath(p, okage.FillRuleWinding, shader, coverage)
}

// glyphEms is the margin of glyphBounds, in ems: more than the reach of the
// outline of a glyph from its origin.
const glyphEms = 8

// glyphBounds returns the pixels that the glyphs of run can cover, from the
// origins of its glyphs and a margin of glyphEms ems, without the outlines.
func (d *OkageDevice) glyphBounds(run GlyphRun) image.Rectangle {
	if len(run.Glyphs) == 0 {
		return image.Rectangle{}
	}
	t := run.Transform
	scale := math.Sqrt(float64(t.SX*t.SX + t.KX*t.KX + t.KY*t.KY + t.SY*t.SY))
	pad := float64(glyphEms)*scale*math.Abs(float64(run.Size)) + math.Abs(float64(t.TX)) + math.Abs(float64(t.TY))
	minX, minY := run.Glyphs[0].X, run.Glyphs[0].Y
	maxX, maxY := minX, minY
	for _, g := range run.Glyphs[1:] {
		minX, maxX = min(minX, g.X), max(maxX, g.X)
		minY, maxY = min(minY, g.Y), max(maxY, g.Y)
	}
	r, ok := okage.RectFromLTRB(minX-float32(pad), minY-float32(pad), maxX+float32(pad), maxY+float32(pad))
	if !ok || !(pad < 1<<20) {
		return d.state.bounds
	}
	return d.rectBounds(r, d.state.t, 2)
}

// FillText satisfies the TextFiller interface. When the outlines are at most
// maxFineEm pixels wide or high, it fills them with a grid of 8 by 8 samples
// in each pixel (D46). Otherwise it fills them as FillPath does.
func (d *OkageDevice) FillText(p *okage.Path, rule okage.FillRule, shader okage.Shader) {
	if p == nil || shader == nil {
		return
	}
	coverage := okage.Coverage4x4
	if b, ok := p.Bounds().Transform(d.state.t); ok && min(b.Width(), b.Height()) <= maxFineEm {
		coverage = okage.Coverage8x8
	}
	d.fillPath(p, rule, shader, coverage)
}

// layer returns a transparent pixmap of the size of the target, or nil.
// A group clears the part of a layer that it drew before it frees the
// layer.
func (d *OkageDevice) layer() *okage.Pixmap {
	if n := len(d.free); n > 0 {
		layer := d.free[n-1]
		d.free = d.free[:n-1]
		return layer
	}
	layer, ok := okage.NewPixmap(d.dst.Width(), d.dst.Height())
	if !ok {
		return nil
	}
	return layer
}

// release clears the pixels of layer inside r, and keeps the layer for a
// later group. Each pixel outside r must be transparent.
func (d *OkageDevice) release(layer *okage.Pixmap, r image.Rectangle) {
	if layer == nil || len(d.free) >= maxLayers {
		return
	}
	clearRect(layer, r)
	d.free = append(d.free, layer)
}

// top returns the index of the innermost group that has a layer, or -1.
func (d *OkageDevice) top() int {
	for i := len(d.groups) - 1; i >= 0; i-- {
		if d.groups[i].layer != nil {
			return i
		}
	}
	return -1
}

// needsLayer reports whether a group draws to a layer of its own. A group
// that is not isolated, not knockout, with an opacity of 1 or more, the
// normal blend mode and no mask draws straight onto its parent. In a
// knockout group, each group has a layer, because the whole group is one
// object.
func (d *OkageDevice) needsLayer(g Group) bool {
	if g.Isolated || g.Knockout || g.Opacity < 1 || blendMode(g.Blend) != okage.BlendModeSourceOver || g.Mask != nil {
		return true
	}
	i := d.top()
	return i >= 0 && d.groups[i].g.Knockout
}

// BeginGroup satisfies the Device interface. A group draws to a layer of
// its own when needsLayer says so. The layer of a group starts transparent.
// A group that is not isolated gets its backdrop only when an object of
// the group blends with it, in blend. A knockout group that is not
// isolated gets its backdrop at once. EndGroup draws the soft mask of the
// group.
func (d *OkageDevice) BeginGroup(g Group) {
	gr := group{parent: d.dst, g: g, bounds: d.state.bounds, depth: len(d.stack)}
	if d.needsLayer(g) && !gr.bounds.Empty() {
		gr.layer = d.layer()
	}
	if gr.layer != nil && g.Knockout && !g.Isolated {
		if i := d.top(); i >= 0 {
			d.blend(i)
		}
		if gr.alpha = d.layer(); gr.alpha != nil {
			copyRect(gr.layer, gr.parent, gr.bounds)
		}
	}
	d.groups = append(d.groups, gr)
	d.Save()
	if gr.layer != nil {
		d.dst, d.alpha = gr.layer, gr.alpha
	}
}

// blend gives the group i, when it is not isolated, its backdrop, so that
// an object of the group can blend with it (ISO 32000-2, 11.6.3). The alpha
// of the group takes a copy of the layer, which holds the group alone, and
// the layer then draws over the backdrop. The backdrop is the pixmap of the
// parent, which gets its own backdrop first. Each later object of the group
// draws onto both. Before the first object that blends, the layer holds
// the same colors as a layer that started with the backdrop, except for
// the rounding.
func (d *OkageDevice) blend(i int) {
	g := &d.groups[i]
	if g.g.Isolated || g.alpha != nil {
		return
	}
	for j := i - 1; j >= 0; j-- {
		if d.groups[j].layer != nil {
			d.blend(j)
			break
		}
	}
	alpha := d.layer()
	if alpha == nil {
		return
	}
	copyRect(alpha, g.layer, g.bounds)
	composite(g.layer, g.parent, g.bounds, okage.BlendModeDestinationOver)
	g.alpha = alpha
	if i == d.top() {
		d.alpha = alpha
	}
}

// softMask returns the soft mask m of a group with the bounds b, on the
// pixels r inside b, and the pixels that the mask covers, which hold r. The
// content of m draws onto a transparent pixmap of the size of b, which
// softMask does not keep. Only the objects that can change a pixel of r
// draw. A mask of luminosity takes the luminance of the content over the
// backdrop of m.
//
// The pixmap of the content is at the origin of b for each r, so the
// content draws with the same transform, and each pixel of r gets the same
// value as when the mask covers all of b. A transform with another origin
// can round a coordinate in another way, and change a pixel.
//
// A mask with a key comes from the cache when the cache holds the mask of
// the same key, transform and bounds on pixels that hold r. When the cache
// holds that mask on other pixels, softMask draws the mask on all of b, for
// the next groups that use the key.
func (d *OkageDevice) softMask(m *Mask, b, r image.Rectangle) (*okage.Mask, image.Rectangle) {
	slot := -1
	if m.Key != nil {
		for i, c := range d.masks {
			if c.key != m.Key || c.t != d.state.t || c.bounds != b {
				continue
			}
			if r.In(c.area) {
				return c.mask, c.area
			}
			slot, r = i, b
			break
		}
	}
	pix, ok := okage.NewPixmap(uint32(b.Dx()), uint32(b.Dy()))
	if !ok {
		return nil, r
	}
	t := okage.TransformFromTranslate(float32(-b.Min.X), float32(-b.Min.Y)).PreConcat(d.state.t)
	child := NewOkage(pix, t)
	child.aliased, child.whole = d.aliased, d.whole
	area := r.Sub(b.Min)
	child.area = area
	if m.Content != nil {
		m.Content(child)
	}
	src := pix.AsRef()
	if r != b {
		d.regions[0] = copyRegion(d.regions[0], pix, area)
		if src, ok = okage.PixmapRefFromBytes(d.regions[0], uint32(r.Dx()), uint32(r.Dy())); !ok {
			return nil, r
		}
	}
	var mask *okage.Mask
	if m.Type == okage.MaskTypeLuminance {
		mask = okage.MaskFromLuminance(src, m.Backdrop)
	} else {
		mask = okage.MaskFromPixmap(src, m.Type)
	}
	if m.Transfer != nil {
		mask.ApplyTransfer(m.Transfer)
	}
	if m.Key != nil {
		c := cachedMask{key: m.Key, t: d.state.t, bounds: b, area: r, mask: mask}
		switch {
		case slot >= 0:
			d.masks[slot] = c
		case len(d.masks) >= maxMasks:
			clear(d.masks)
			d.masks = append(d.masks[:0], c)
		default:
			d.masks = append(d.masks, c)
		}
	}
	return mask, r
}

// EndGroup satisfies the Device interface. It restores the state of its
// BeginGroup, and draws the layer onto the parent with the opacity, the
// blend mode and the soft mask of the group, inside the clip of the parent.
// For a group that got its backdrop, it first removes the backdrop from the
// layer. It draws the soft mask with the state of BeginGroup, and only on
// the pixels that it composites.
//
// EndGroup draws only inside the drawn pixels of the group, when the blend
// mode of the group keeps the parent under a transparent pixel. Outside
// them, the group alone is transparent: the layer is transparent, or it
// holds the backdrop and the alpha of the group alone is 0, so that
// RemoveBackdrop of okage makes it transparent. So the parent does not
// change there. For another blend mode, such as DestinationIn, EndGroup
// draws inside the bounds of the clip, and outside them the clip hides the
// layer.
func (d *OkageDevice) EndGroup() {
	if len(d.groups) == 0 {
		return
	}
	g := d.groups[len(d.groups)-1]
	d.groups = d.groups[:len(d.groups)-1]
	for len(d.stack) > g.depth+1 {
		d.Restore()
	}
	d.Restore()
	if g.layer == nil {
		return
	}
	d.dst, d.alpha = g.parent, nil
	i := d.top()
	if i >= 0 {
		d.alpha = d.groups[i].alpha
	}
	// r holds the pixels that the group composites, and cleared the pixels
	// of the layer that are not transparent. A layer that holds the
	// backdrop holds it inside the bounds.
	blend := blendMode(g.g.Blend)
	r, cleared := g.drawn.Intersect(g.bounds), g.drawn
	if d.whole || !keepsParent(blend) {
		r = g.bounds
	}
	r = r.Intersect(d.area)
	if g.alpha != nil {
		cleared = g.bounds
		d.removeBackdrop(g.layer, g.parent, g.alpha, r)
	}
	if blend != okage.BlendModeSourceOver && i >= 0 {
		d.blend(i)
	}
	if rect, ok := okage.RectFromLTRB(float32(r.Min.X), float32(r.Min.Y), float32(r.Max.X), float32(r.Max.Y)); ok && !r.Empty() {
		mask := d.state.clip
		if g.g.Mask != nil {
			if m, mr := d.softMask(g.g.Mask, g.bounds, r); m != nil {
				mask = d.groupMask(m, mr, r)
			}
		}
		d.draw(func() image.Rectangle { return r }, func(dst *okage.Pixmap, shape bool) {
			// The paint of DrawPixmap of okage, which fills the whole
			// layer. The shape of a group in a knockout group is the alpha
			// of its layer.
			opacity, blend, m := g.g.Opacity, blend, mask
			if shape {
				opacity, blend, m = 1, okage.BlendModeSourceOver, d.state.clip
			}
			pt := okage.Paint{
				Shader:     okage.NewPattern(g.layer.AsRef(), okage.SpreadModePad, okage.FilterQualityNearest, opacity, okage.IdentityTransform()),
				BlendMode:  blend,
				Colorspace: okage.ColorSpaceLinear,
			}
			dst.FillRect(rect, &pt, okage.IdentityTransform(), m)
		})
	}
	d.release(g.layer, cleared.Union(r))
	d.release(g.alpha, cleared.Union(r))
}

// keepsParent reports whether a transparent pixel drawn with the blend mode
// leaves the pixel under it the same, in the pipelines of okage. In each of
// these modes, a source of 0 with an alpha of 0 gives the destination: for
// example multiply gives s*(1-da) + d*(1-sa) + s*d = d, color burn and color
// dodge take their branch for s = 0 or s = sa, which gives d, and hue,
// saturation, color and luminosity set the saturation and the luminosity of
// a color of 0 to 0. The modes that it leaves out, such as Source,
// SourceIn, DestinationIn, SourceOut, DestinationAtop and Modulate, clear the
// destination there.
func keepsParent(b okage.BlendMode) bool {
	switch b {
	case okage.BlendModeDestination, okage.BlendModeSourceOver,
		okage.BlendModeDestinationOver, okage.BlendModeDestinationOut,
		okage.BlendModeSourceAtop, okage.BlendModeXor, okage.BlendModePlus,
		okage.BlendModeScreen, okage.BlendModeOverlay, okage.BlendModeDarken,
		okage.BlendModeLighten, okage.BlendModeColorDodge,
		okage.BlendModeColorBurn, okage.BlendModeHardLight,
		okage.BlendModeSoftLight, okage.BlendModeDifference,
		okage.BlendModeExclusion, okage.BlendModeMultiply, okage.BlendModeHue,
		okage.BlendModeSaturation, okage.BlendModeColor,
		okage.BlendModeLuminosity:
		return true
	}
	return false
}

// knockout draws one object of the knockout group g, whose layer is the
// target. The object draws onto a copy of the backdrop of the group inside
// r, or onto a transparent pixmap for an isolated group. Then Knockout of
// okage puts it over the earlier objects with its shape. A group that is
// not isolated also knocks out the object from its alpha.
func (d *OkageDevice) knockout(g *group, r image.Rectangle, paint func(dst *okage.Pixmap, shape bool)) {
	r = r.Intersect(d.state.bounds).Intersect(g.bounds)
	if r.Empty() {
		return
	}
	elem, shape := d.layer(), d.layer()
	if elem == nil || shape == nil {
		paint(d.dst, false)
		d.release(elem, image.Rectangle{})
		d.release(shape, image.Rectangle{})
		return
	}
	var backdrop *okage.Pixmap
	if !g.g.Isolated {
		backdrop = g.parent
		copyRect(elem, backdrop, r)
	}
	paint(elem, false)
	paint(shape, true)
	d.knockoutRegion(g.layer, elem, backdrop, shape, r)
	if g.alpha != nil {
		clearRect(elem, r)
		paint(elem, false)
		d.knockoutRegion(g.alpha, elem, nil, shape, r)
	}
	d.release(elem, r)
	d.release(shape, r)
}

// knockoutRegion calls Knockout of okage on the pixels of dst inside r. The
// object is elem, the backdrop is backdrop, or transparent when it is nil,
// and the shape is the alpha of shape.
func (d *OkageDevice) knockoutRegion(dst, elem, backdrop, shape *okage.Pixmap, r image.Rectangle) {
	w, h := uint32(r.Dx()), uint32(r.Dy())
	size, ok := okage.IntSizeFromWH(w, h)
	if !ok {
		return
	}
	d.regions[0] = copyRegion(d.regions[0], dst, r)
	d.regions[1] = copyRegion(d.regions[1], elem, r)
	d.regions[3] = copyRegion(d.regions[3], shape, r)
	cover := make([]byte, r.Dx()*r.Dy())
	for i := range cover {
		cover[i] = d.regions[3][4*i+3]
	}
	mask, ok := okage.MaskFromVec(cover, size)
	if !ok {
		return
	}
	var bd okage.PixmapRef
	if backdrop != nil {
		d.regions[2] = copyRegion(d.regions[2], backdrop, r)
		if bd, ok = okage.PixmapRefFromBytes(d.regions[2], w, h); !ok {
			return
		}
	}
	m, ok := okage.PixmapMutFromBytes(d.regions[0], w, h)
	if !ok {
		return
	}
	e, ok := okage.PixmapRefFromBytes(d.regions[1], w, h)
	if !ok {
		return
	}
	m.Knockout(e, bd, mask)
	pasteRegion(dst, r, d.regions[0])
}

// removeBackdrop calls RemoveBackdrop of okage on the pixels of layer inside
// r, with the backdrop and the alpha of the group alone.
func (d *OkageDevice) removeBackdrop(layer, backdrop, alpha *okage.Pixmap, r image.Rectangle) {
	if r.Empty() {
		return
	}
	w, h := uint32(r.Dx()), uint32(r.Dy())
	d.regions[0] = copyRegion(d.regions[0], layer, r)
	d.regions[1] = copyRegion(d.regions[1], backdrop, r)
	d.regions[2] = copyRegion(d.regions[2], alpha, r)
	m, ok := okage.PixmapMutFromBytes(d.regions[0], w, h)
	if !ok {
		return
	}
	bd, ok := okage.PixmapRefFromBytes(d.regions[1], w, h)
	if !ok {
		return
	}
	ag, ok := okage.PixmapRefFromBytes(d.regions[2], w, h)
	if !ok {
		return
	}
	m.RemoveBackdrop(bd, ag)
	pasteRegion(layer, r, d.regions[0])
}

// groupMask returns the mask of the target that is the soft mask m of the
// pixels mr, times the clip of the state. It is right only inside r, which
// must be inside mr.
func (d *OkageDevice) groupMask(m *okage.Mask, mr, r image.Rectangle) *okage.Mask {
	if d.scratch == nil {
		s, ok := okage.NewMask(d.dst.Width(), d.dst.Height())
		if !ok {
			return d.state.clip
		}
		d.scratch = s
	}
	w, mw := int(d.dst.Width()), mr.Dx()
	dst, src := d.scratch.DataMut(), m.Data()
	var clip []byte
	if d.state.clip != nil {
		clip = d.state.clip.Data()
	}
	for y := r.Min.Y; y < r.Max.Y; y++ {
		row := dst[y*w+r.Min.X : y*w+r.Max.X]
		i := (y-mr.Min.Y)*mw + r.Min.X - mr.Min.X
		mrow := src[i : i+r.Dx()]
		if clip == nil {
			copy(row, mrow)
			continue
		}
		crow := clip[y*w+r.Min.X : y*w+r.Max.X]
		for i := range row {
			row[i] = uint8((uint32(mrow[i])*uint32(crow[i]) + 127) / 255)
		}
	}
	return d.scratch
}

// composite draws the pixels of src inside r onto the same pixels of dst,
// with the blend mode.
func composite(dst, src *okage.Pixmap, r image.Rectangle, blend okage.BlendMode) {
	rect, ok := okage.RectFromLTRB(float32(r.Min.X), float32(r.Min.Y), float32(r.Max.X), float32(r.Max.Y))
	if !ok {
		return
	}
	pt := okage.Paint{
		Shader:     okage.NewPattern(src.AsRef(), okage.SpreadModePad, okage.FilterQualityNearest, 1, okage.IdentityTransform()),
		BlendMode:  blend,
		Colorspace: okage.ColorSpaceLinear,
	}
	dst.FillRect(rect, &pt, okage.IdentityTransform(), nil)
}

// copyRect copies the pixels of src inside r onto dst. The pixmaps have
// the same size.
func copyRect(dst, src *okage.Pixmap, r image.Rectangle) {
	r = r.Intersect(image.Rect(0, 0, int(dst.Width()), int(dst.Height())))
	w := int(dst.Width()) * 4
	d, s := dst.DataMut(), src.Data()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		copy(d[y*w+r.Min.X*4:y*w+r.Max.X*4], s[y*w+r.Min.X*4:y*w+r.Max.X*4])
	}
}

// copyRegion copies the pixels of p inside r, a rectangle inside p, into
// buf, one row after the other. It grows buf when it is too small.
func copyRegion(buf []byte, p *okage.Pixmap, r image.Rectangle) []byte {
	rw := r.Dx() * 4
	if n := rw * r.Dy(); cap(buf) < n {
		buf = make([]byte, n)
	} else {
		buf = buf[:n]
	}
	w := int(p.Width()) * 4
	data := p.Data()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		copy(buf[(y-r.Min.Y)*rw:(y-r.Min.Y+1)*rw], data[y*w+r.Min.X*4:])
	}
	return buf
}

// pasteRegion copies the rows of buf back into the pixels of p inside r.
func pasteRegion(p *okage.Pixmap, r image.Rectangle, buf []byte) {
	rw := r.Dx() * 4
	w := int(p.Width()) * 4
	data := p.DataMut()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		copy(data[y*w+r.Min.X*4:y*w+r.Max.X*4], buf[(y-r.Min.Y)*rw:])
	}
}

// clearRect makes the pixels of p inside r transparent.
func clearRect(p *okage.Pixmap, r image.Rectangle) {
	r = r.Intersect(image.Rect(0, 0, int(p.Width()), int(p.Height())))
	if r.Empty() {
		return
	}
	w := int(p.Width()) * 4
	data := p.DataMut()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		clear(data[y*w+r.Min.X*4 : y*w+r.Max.X*4])
	}
}
