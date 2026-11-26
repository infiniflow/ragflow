// Copied from github.com/mgilbir/gowemf, file playback_bitmap.go, at commit
// 377668cb7b13 (D7, docs/UPSTREAM.md). Its MIT license is in
// internal/gowemf/LICENSE.

package gowemf

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
)

const (
	blitROP = iota
	blitAlpha
	blitTransparent
)

// blit is a normalized bitmap transfer. Destination coordinates are logical;
// source coordinates are bitmap pixels after any EMF source transform.
type blit struct {
	r                Record
	kind             int
	rop, operation   uint32
	dest, destSize   Point
	src, srcSize     Point
	plg              []Point // PlgBlt upper-left, upper-right, lower-left
	deviceSize       bool    // SetDIBitsToDevice: no stretching, size in device pixels
	lowerLeftOrigin  bool    // source y is the lower-left corner, measured from the bottom row
	info, bits       []byte  // EMF split DIB
	packed           []byte  // WMF packed DIB
	hasBitmap        bool
	usage            uint32
	startScan, scans uint32
	scanned          bool
	sourceTransform  *Matrix
	colorState       *ColorPlaybackState
	deviceBitmapWhy  string
	maskPresent      bool
}

// RasterOperation is a ternary raster operation (MS-WMF 2.1.1.31): the index
// byte of a ROP3 code, which is its truth table. For pattern bit P, source
// bit S and destination bit D, the result is bit P<<2|S<<1|D of the value.
type RasterOperation uint8

// Apply combines pattern, source and destination values bit by bit.
func (op RasterOperation) Apply(p, s, d uint8) uint8 {
	var out uint8
	for i := uint(0); i < 8; i++ {
		if op>>i&1 == 0 {
			continue
		}
		m := ^uint8(0)
		for _, t := range [3]struct {
			v   uint8
			bit uint
		}{{p, 4}, {s, 2}, {d, 1}} {
			if i&t.bit != 0 {
				m &= t.v
			} else {
				m &^= t.v
			}
		}
		out |= m
	}
	return out
}

// UsesPattern, UsesSource and UsesDestination report whether the result
// depends on that operand.
func (op RasterOperation) UsesPattern() bool     { return (op>>4^op)&0x0f != 0 }
func (op RasterOperation) UsesSource() bool      { return (op>>2^op)&0x33 != 0 }
func (op RasterOperation) UsesDestination() bool { return (op>>1^op)&0x55 != 0 }

// RasterDraw is a bitmap or pattern transfer whose raster operation Play
// cannot express as painting. Every destination pixel in Area and the clip
// whose source pixel, when Source is set, lies in Source.Source is replaced by
// Operation applied bit by bit to the 8-bit sRGB red, green and blue values of
// the pattern (P), source (S) and destination (D) pixels; the destination
// stays opaque. GDI has no anti-aliasing: the backend decides which edge
// pixels belong to Area. Source places an opaque bitmap as for DrawImage and
// is nil when Operation does not use the source; Pattern is the brush and is
// nil when Operation does not use the pattern. Hatch patterns always carry a
// Background.
//
// ivy: Stroke, when it is set, is the pen of a stroke with a mix mode
// (ROP2) that reads the destination. The pixels of the operation are then
// the pixels that the pen draws along Area, not the pixels inside Area.
// Pattern is the paint of the pen, and Source is nil.
type RasterDraw struct {
	Operation RasterOperation
	Area      Path
	Source    *ImageDraw
	Pattern   *Paint
	Stroke    *Stroke
}

// RasterBackend is a Backend with a pixel destination it can read. Play passes
// it the bitmap and pattern transfers whose raster operations, such as SRCAND
// (0x88) and SRCPAINT (0xEE), combine their operands with the destination
// or with each other. Other backends get those transfers reported
// unsupported, except an SRCAND of a black and white mask immediately
// followed by an SRCPAINT of a sprite placed identically, which is drawn
// exactly as one image with the mask as its alpha. SRCCOPY, NOTSRCCOPY,
// PATCOPY, BLACKNESS, WHITENESS and DSTCOPY are always drawn with DrawImage
// and FillPath.
type RasterBackend interface {
	Backend
	DrawRaster(r RasterDraw, clip Clip) error
}

func (p *player) blit(b blit) error {
	op := RasterOperation(b.rop >> 16)
	_, hasRaster := backendAs[RasterBackend](p.backend)
	sprite := b.kind == blitROP && !hasRaster && (op == ropSrcAnd || op == ropSrcPaint)
	if p.sprite != nil && !(sprite && op == ropSrcPaint) {
		if err := p.flushSprite(); err != nil {
			return err
		}
	}
	if b.maskPresent {
		return p.unsupported(b.r, "masked bitmap transfer")
	}
	var raster RasterBackend
	if b.kind == blitROP {
		switch op {
		case 0xaa: // D: the destination is unchanged.
			return nil
		case 0x00, 0xff, 0xf0:
			return p.patBlt(b)
		case 0xcc, 0x33:
		default:
			rb, ok := backendAs[RasterBackend](p.backend)
			if !ok && !sprite {
				return p.unsupported(b.r, fmt.Sprintf("raster operation 0x%02x", uint8(op)))
			}
			if ok && !op.UsesSource() {
				return p.rasterBlit(b, rb, nil, nil)
			}
			raster = rb
		}
	}
	if b.deviceBitmapWhy != "" {
		return p.unsupported(b.r, b.deviceBitmapWhy)
	}
	if !b.hasBitmap {
		return malformed(b.r.Offset, "missing source bitmap")
	}
	if err := p.drawable(b.r, b.colorState); err != nil {
		return err
	}
	if b.srcSize.X == 0 || b.srcSize.Y == 0 || (b.plg == nil && !b.deviceSize && (b.destSize.X == 0 || b.destSize.Y == 0)) {
		return nil
	}
	logical, err := p.logicalPalette(b.r, b.usage)
	if err != nil {
		return err
	}
	var d *DIB
	if b.packed != nil {
		d, err = ParsePackedDIB(b.packed, b.usage, logical, p.options.Images)
	} else {
		d, err = ParseDIB(b.info, b.bits, b.usage, logical, p.options.Images)
	}
	if err != nil {
		return p.imageError(b.r, err)
	}
	if b.scanned && (b.startScan != 0 || uint64(b.scans) != uint64(d.height)) {
		return p.unsupported(b.r, "partial scan-line bitmap transfer")
	}
	if b.lowerLeftOrigin && !b.fullSource(d) {
		if d.topDown && b.scanned {
			return p.unsupported(b.r, "lower-left source origin in a top-down DIB")
		}
		// StretchDIBits measures the source from the bottom row of the
		// image in either row order (ORACLES.md).
		b.src.Y = float64(d.height) - b.src.Y - b.srcSize.Y
	}
	if err := p.spendPixels(b.r, d.width, d.height); err != nil {
		return err
	}
	var im image.Image
	if b.kind == blitAlpha && b.operation>>24&0xff == 1 {
		im, err = d.AlphaImageWithColorTransform(p.options.ColorTransform)
	} else {
		im, err = d.ImageWithColorTransform(p.options.ColorTransform)
	}
	if err != nil {
		return p.imageError(b.r, err)
	}
	draw := ImageDraw{Image: im, Opacity: 1, Smooth: p.dc.stretchMode == 4}
	if draw.Smooth && b.colorState != nil && b.colorState.Adjustment != initialColorState().Adjustment {
		return p.unsupported(b.r, "halftone color adjustment")
	}
	switch b.kind {
	case blitROP:
		if raster != nil {
			draw.Image = opaqueImage{im}
		} else if b.rop>>16&0xff == 0x33 {
			if err := p.spendPixels(b.r, d.width, d.height); err != nil {
				return err
			}
			draw.Image = invertImage(im)
		} else {
			draw.Image = opaqueImage{im}
		}
	case blitAlpha:
		draw.Opacity = float64(b.operation>>16&0xff) / 255
		if b.operation>>24&0xff == 0 {
			draw.Image = opaqueImage{im}
		}
	case blitTransparent:
		key, err := p.color(b.r, b.operation)
		if err != nil {
			return err
		}
		if err := p.spendPixels(b.r, d.width, d.height); err != nil {
			return err
		}
		draw.Image = colorKeyImage(im, key)
	}
	if b.sourceTransform != nil {
		t := *b.sourceTransform
		if t.M12 != 0 || t.M21 != 0 || t.M11 == 0 || t.M22 == 0 {
			return p.unsupported(b.r, "rotated, sheared or singular source transform")
		}
		b.src = t.Apply(b.src)
		b.srcSize = Point{b.srcSize.X * t.M11, b.srcSize.Y * t.M22}
	}
	m, err := p.toDestination(b.r)
	if err != nil {
		return err
	}
	var place Matrix
	switch {
	case b.plg != nil:
		p0, p1, p2 := b.plg[0], b.plg[1], b.plg[2]
		place = Matrix{M11: (p1.X - p0.X) / b.srcSize.X, M12: (p1.Y - p0.Y) / b.srcSize.X, M21: (p2.X - p0.X) / b.srcSize.Y, M22: (p2.Y - p0.Y) / b.srcSize.Y}
		place.Dx = p0.X - b.src.X*place.M11 - b.src.Y*place.M21
		place.Dy = p0.Y - b.src.X*place.M12 - b.src.Y*place.M22
		place = place.Then(m)
	case b.deviceSize:
		// SetDIBitsToDevice copies pixels 1:1 to the device at a logical origin.
		o := p.dc.world.Then(p.dc.pageMatrix()).Apply(b.dest)
		place = Matrix{M11: 1, M22: 1, Dx: o.X - b.src.X, Dy: o.Y - b.src.Y}.Then(p.base)
	default:
		sx, sy := b.destSize.X/b.srcSize.X, b.destSize.Y/b.srcSize.Y
		place = Matrix{M11: sx, M22: sy, Dx: b.dest.X - b.src.X*sx, Dy: b.dest.Y - b.src.Y*sy}.Then(m)
	}
	if !place.Finite() {
		return malformed(b.r.Offset, "non-finite bitmap placement")
	}
	draw.Transform = place
	draw.Source = sourceRect(b.src, b.srcSize, im.Bounds())
	if draw.Source.Empty() {
		return nil
	}
	clip := sourceClip(draw, b.src, b.srcSize, p.currentClip())
	switch {
	case raster != nil:
		return p.rasterBlit(b, raster, &draw, clip)
	case sprite:
		return p.spriteBlit(b, op, draw, clip)
	}
	return p.backend.DrawImage(draw, clip)
}

const ropSrcAnd, ropSrcPaint RasterOperation = 0x88, 0xee

// pendingSprite is an SRCAND transfer of a black and white mask, held until
// the next record shows whether an SRCPAINT completes a sprite.
type pendingSprite struct {
	r    Record
	draw ImageDraw
	clip Clip
}

// spriteBlit draws the SRCAND mask and SRCPAINT image pair that GDI programs
// use to draw a sprite, for backends that cannot read their destination.
// Where the mask is black, the pair leaves the image; where it is white, it
// leaves the destination, provided the image is black there. When both
// transfers place their bitmaps identically under the same clip, without
// halftone averaging, the pair is therefore exactly the image drawn with
// the mask's black pixels opaque and its white pixels transparent. Any other
// SRCAND or SRCPAINT transfer is reported.
func (p *player) spriteBlit(b blit, op RasterOperation, draw ImageDraw, clip Clip) error {
	if op == ropSrcAnd {
		if draw.Smooth || !blackAndWhite(draw.Image, draw.Source) {
			return p.unsupported(b.r, "raster operation 0x88")
		}
		p.sprite = &pendingSprite{r: b.r, draw: draw, clip: clip}
		return nil
	}
	mask := p.sprite
	p.sprite = nil
	merged, ok := (*image.NRGBA)(nil), false
	if mask != nil && !draw.Smooth && draw.Source == mask.draw.Source && draw.Transform == mask.draw.Transform && sameClip(clip, mask.clip) {
		if err := p.spendPixels(b.r, draw.Source.Dx(), draw.Source.Dy()); err != nil {
			return err
		}
		merged, ok = spriteImage(mask.draw.Image, draw.Image, draw.Source)
	}
	if !ok {
		if mask != nil {
			if err := p.reportSprite(mask); err != nil {
				return err
			}
		}
		return p.unsupported(b.r, "raster operation 0xee")
	}
	draw.Image = merged
	return p.backend.DrawImage(draw, clip)
}

// flushSprite reports a held SRCAND transfer that no SRCPAINT completed.
func (p *player) flushSprite() error {
	s := p.sprite
	if s == nil {
		return nil
	}
	p.sprite = nil
	return p.reportSprite(s)
}

func (p *player) reportSprite(s *pendingSprite) error {
	if err := p.unsupported(s.r, "raster operation 0x88"); err != errSkip {
		return err
	}
	return nil
}

func blackAndWhite(im image.Image, r image.Rectangle) bool {
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
			if c != (color.NRGBA{A: 255}) && c != (color.NRGBA{255, 255, 255, 255}) {
				return false
			}
		}
	}
	return true
}

// spriteImage keeps the image where the mask is black and makes it
// transparent where the mask is white; ok is false if the image is not
// black under every white mask pixel.
func spriteImage(mask, im image.Image, r image.Rectangle) (*image.NRGBA, bool) {
	out := image.NewNRGBA(r)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
			if color.NRGBAModel.Convert(mask.At(x, y)).(color.NRGBA).R == 0 {
				out.SetNRGBA(x, y, c)
			} else if c.R|c.G|c.B != 0 {
				return nil, false
			}
		}
	}
	return out, true
}

// sameClip reports whether two clips are the same layers. Layers added for
// fractional sources are new nodes each time and compare by value.
func sameClip(a, b Clip) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		x, y := a[i], b[i]
		if x == y {
			continue
		}
		if x == nil || y == nil || x.Base != y.Base || x.Op != y.Op || x.Rule != y.Rule || x.Operand != y.Operand || x.Offset != y.Offset ||
			len(x.Area.Verbs) != len(y.Area.Verbs) || len(x.Area.Points) != len(y.Area.Points) {
			return false
		}
		for k := range x.Area.Verbs {
			if x.Area.Verbs[k] != y.Area.Verbs[k] {
				return false
			}
		}
		for k := range x.Area.Points {
			if x.Area.Points[k] != y.Area.Points[k] {
				return false
			}
		}
	}
	return true
}

// rasterBlit passes a transfer to a RasterBackend. source and clip are the
// placed bitmap and its clip when the operation uses the source.
func (p *player) rasterBlit(b blit, rb RasterBackend, source *ImageDraw, clip Clip) error {
	op := RasterOperation(b.rop >> 16)
	if source == nil {
		if err := p.drawable(b.r, b.colorState); err != nil {
			return err
		}
		clip = p.currentClip()
	}
	m, err := p.toDestination(b.r)
	if err != nil {
		return err
	}
	d := RasterDraw{Operation: op, Area: b.destinationArea(m), Source: source}
	if op.UsesPattern() {
		d.Pattern, err = p.brushPaint(b.r, p.dc.brush.brush, m, false)
		if err != nil || d.Pattern == nil {
			// A null brush leaves the destination unchanged.
			return err
		}
		if d.Pattern.Kind == PaintHatch && d.Pattern.Background == nil {
			return p.unsupported(b.r, "raster operation with a hatch brush in TRANSPARENT mode")
		}
	}
	return rb.DrawRaster(d, clip)
}

// destinationArea is the transfer's destination rectangle or parallelogram.
func (b blit) destinationArea(m Matrix) Path {
	var pts [4]Point
	if b.plg != nil {
		p0, p1, p2 := b.plg[0], b.plg[1], b.plg[2]
		pts = [4]Point{p0, p1, {p1.X + p2.X - p0.X, p1.Y + p2.Y - p0.Y}, p2}
	} else {
		d, s := b.dest, b.destSize
		pts = [4]Point{d, {d.X + s.X, d.Y}, {d.X + s.X, d.Y + s.Y}, {d.X, d.Y + s.Y}}
	}
	path := pathBuilder{limit: 4}
	sh := shape{&path, m}
	sh.moveTo(pts[0])
	for _, q := range pts[1:] {
		sh.lineTo(q)
	}
	path.close()
	return path.path
}

// fullSource reports a source rectangle covering the whole bitmap, for
// which the source origin convention is irrelevant.
func (b blit) fullSource(d *DIB) bool {
	return b.src == (Point{}) && b.srcSize == Point{float64(d.width), float64(d.height)}
}

// sourceRect clamps before integer conversion so oversized logical source
// values cannot overflow int on 32-bit platforms.
func sourceRect(o, s Point, bounds image.Rectangle) image.Rectangle {
	x0, x1 := math.Min(o.X, o.X+s.X), math.Max(o.X, o.X+s.X)
	y0, y1 := math.Min(o.Y, o.Y+s.Y), math.Max(o.Y, o.Y+s.Y)
	clamp := func(v float64, lo, hi int) int {
		return int(math.Max(float64(lo), math.Min(float64(hi), v)))
	}
	r := image.Rect(clamp(math.Floor(x0), bounds.Min.X, bounds.Max.X), clamp(math.Floor(y0), bounds.Min.Y, bounds.Max.Y),
		clamp(math.Ceil(x1), bounds.Min.X, bounds.Max.X), clamp(math.Ceil(y1), bounds.Min.Y, bounds.Max.Y))
	return r.Intersect(bounds)
}

// sourceClip handles a source rectangle (origin o, size s, in bitmap pixels)
// whose edges are not whole pixels. draw.Source encloses it and
// draw.Transform maps the exact rectangle onto the destination, so clip gains
// a layer limiting drawing to that rectangle, less any part outside
// draw.Source (the bitmap). Whole-pixel sources leave clip unchanged.
func sourceClip(draw ImageDraw, o, s Point, clip Clip) Clip {
	src := draw.Source
	x0 := math.Max(math.Min(o.X, o.X+s.X), float64(src.Min.X))
	x1 := math.Min(math.Max(o.X, o.X+s.X), float64(src.Max.X))
	y0 := math.Max(math.Min(o.Y, o.Y+s.Y), float64(src.Min.Y))
	y1 := math.Min(math.Max(o.Y, o.Y+s.Y), float64(src.Max.Y))
	if x0 == float64(src.Min.X) && x1 == float64(src.Max.X) && y0 == float64(src.Min.Y) && y1 == float64(src.Max.Y) {
		return clip
	}
	t := draw.Transform
	area := Path{
		Verbs:  []PathVerb{PathMoveTo, PathLineTo, PathLineTo, PathLineTo, PathClose},
		Points: []Point{t.Apply(Point{x0, y0}), t.Apply(Point{x1, y0}), t.Apply(Point{x1, y1}), t.Apply(Point{x0, y1})},
	}
	return append(clip[:len(clip):len(clip)], &ClipRegion{Op: ClipReplace, Area: area, Rule: NonZero, depth: 1})
}

// patBlt fills the destination with black, white or the selected brush.
func (p *player) patBlt(b blit) error {
	if err := p.drawable(b.r, b.colorState); err != nil {
		return err
	}
	var paint *Paint
	m, err := p.toDestination(b.r)
	if err != nil {
		return err
	}
	switch b.rop >> 16 & 0xff {
	case 0x00:
		paint = &Paint{Kind: PaintSolid, Color: color.NRGBA{A: 255}}
	case 0xff:
		paint = &Paint{Kind: PaintSolid, Color: color.NRGBA{255, 255, 255, 255}}
	default:
		paint, err = p.brushPaint(b.r, p.dc.brush.brush, m, false)
		if err != nil || paint == nil {
			return err
		}
	}
	return p.backend.FillPath(b.destinationArea(m), NonZero, *paint, p.currentClip())
}

func (p *player) spendPixels(r Record, w, h int) error {
	n := uint64(w) * uint64(h)
	if n > p.options.MaxImagePixels-p.budget.pixels {
		return failure(r.Offset, "playback bitmap pixels", ErrLimit)
	}
	p.budget.pixels += n
	return nil
}

func (p *player) imageError(r Record, err error) error {
	if errors.Is(err, ErrUnsupported) {
		return p.unsupported(r, "bitmap: "+err.Error())
	}
	var pe *ParseError
	if errors.As(err, &pe) {
		return &ParseError{Offset: r.Offset, Field: "bitmap " + pe.Field, Err: pe.Err}
	}
	return failure(r.Offset, "bitmap: "+err.Error(), ErrMalformed)
}

// opaqueImage implements SRCCOPY: the destination receives the source color
// channels whether or not the decoded DIB carries alpha.
type opaqueImage struct{ image.Image }

func (o opaqueImage) ColorModel() color.Model { return color.NRGBAModel }
func (o opaqueImage) Opaque() bool            { return true }
func (o opaqueImage) At(x, y int) color.Color {
	c := color.NRGBAModel.Convert(o.Image.At(x, y)).(color.NRGBA)
	c.A = 255
	return c
}

func invertImage(im image.Image) *image.NRGBA {
	b := im.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
			out.SetNRGBA(x, y, color.NRGBA{^c.R, ^c.G, ^c.B, 255})
		}
	}
	return out
}

// colorKeyImage implements TransparentBlt: source pixels equal to the key
// color are not transferred.
func colorKeyImage(im image.Image, k color.NRGBA) *image.NRGBA {
	b := im.Bounds()
	out := image.NewNRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			c := color.NRGBAModel.Convert(im.At(x, y)).(color.NRGBA)
			c.A = 255
			if c == k {
				c = color.NRGBA{}
			}
			out.SetNRGBA(x, y, c)
		}
	}
	return out
}
