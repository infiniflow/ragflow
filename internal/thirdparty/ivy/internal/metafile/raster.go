package metafile

import (
	"fmt"
	"image"
	"image/color"
	"math"

	"github.com/xo/okage"

	"ragflow/internal/thirdparty/ivy/internal/display"
	"ragflow/internal/thirdparty/ivy/internal/gowemf"
)

// The interfaces that a backend implements.
var _ gowemf.RasterBackend = (*backend)(nil)

// DrawRaster satisfies the gowemf.RasterBackend interface. It draws a
// ternary raster operation of GDI, which combines a pattern P, a source
// bitmap S and the destination D bit by bit.
//
// The device cannot read its pixels, so the backend draws the operation as
// blend modes. For each pair of bits of P and S, the operation gives 0, 1,
// D or the inverse of D. So the result is ((D xor A) and M) or O, where A,
// M and O are images of P and S alone. The backend draws A with the blend
// mode Difference, M with Multiply and O with Lighten, and leaves out each
// one that changes nothing. The result is exact where P, S and D are black
// or white in each channel, which is the case of the masks that programs
// draw with these operations, such as SRCAND and SRCPAINT. It is close for
// other colors. A stroke with a mix mode (ROP2) that reads the page, such as
// R2_XORPEN, is an operation on the pixels of the stroke, and the backend
// draws it in the same way.
func (b *backend) DrawRaster(r gowemf.RasterDraw, clip gowemf.Clip) error {
	if err := b.begin(); err != nil {
		return err
	}
	area := b.path(r.Area)
	if area == nil {
		return nil
	}
	var a, m, o gowemf.RasterOperation
	for i := uint(0); i < 8; i += 2 {
		r0, r1 := r.Operation>>i&1, r.Operation>>(i+1)&1
		// the bits i and i+1 of each derived operation, so that it does not
		// depend on D
		both := gowemf.RasterOperation(3) << i
		if r0 == 1 && r1 == 0 {
			a |= both
		}
		if r0 != r1 {
			m |= both
		}
		if r0 == 1 && r1 == 1 {
			o |= both
		}
	}
	type pass struct {
		op    gowemf.RasterOperation
		blend okage.BlendMode
	}
	// the result does not depend on D if m is 0
	passes := []pass{{o, okage.BlendModeSourceOver}}
	if m != 0 {
		passes = passes[:0]
		if a != 0 {
			passes = append(passes, pass{a, okage.BlendModeDifference})
		}
		if m != 0xff {
			passes = append(passes, pass{m, okage.BlendModeMultiply})
		}
		if o != 0 {
			passes = append(passes, pass{o, okage.BlendModeLighten})
		}
	}
	// make each operand before the first one is drawn, so that the
	// operation is drawn whole or not at all
	draws := make([]func(), len(passes))
	for i, p := range passes {
		var why string
		draws[i], why = b.rasterOperand(p.op, r, area)
		if why != "" {
			b.skip(fmt.Sprintf("raster operation 0x%02x: %s", uint8(r.Operation), why))
			return nil
		}
	}
	for i, p := range passes {
		draw := draws[i]
		if draw == nil {
			continue
		}
		if p.blend != okage.BlendModeSourceOver {
			b.endClip()
			b.d.BeginGroup(display.Group{Opacity: 1, Blend: p.blend, Isolated: true})
		}
		b.clipped(clip, func() {
			b.d.Save()
			if r.Stroke == nil {
				b.d.ClipPath(area, okage.FillRuleWinding)
			}
			draw()
			b.d.Restore()
		})
		if p.blend != okage.BlendModeSourceOver {
			b.endClip()
			b.d.EndGroup()
		}
	}
	return nil
}

// rasterOperand returns the function that draws the image of P and S that op
// gives, over area, the area of the transfer. It returns a reason instead when
// the backend cannot draw it.
func (b *backend) rasterOperand(op gowemf.RasterOperation, r gowemf.RasterDraw, area *okage.Path) (draw func(), why string) {
	fill := func(shader okage.Shader) func() {
		return func() { b.d.FillPath(area, okage.FillRuleWinding, shader) }
	}
	if s := r.Stroke; s != nil {
		// the pixels of a stroke: the pen draws the operand along the area
		fill = func(shader okage.Shader) func() {
			return func() {
				st := *s
				st.Gap = nil
				b.stroke(area, r.Area, st, shader)
			}
		}
	}
	switch {
	case op == 0x00:
		return fill(okage.SolidColor(okage.ColorBlack)), ""
	case op == 0xff:
		return fill(okage.SolidColor(okage.ColorWhite)), ""
	case !op.UsesSource():
		// op is P or the inverse of P
		if r.Pattern == nil {
			return nil, "no brush"
		}
		paint := *r.Pattern
		if op == 0x0f {
			var ok bool
			if paint, ok = invertPaint(paint); !ok {
				return nil, "the inverse of a gradient brush"
			}
		}
		shader := b.shader(paint)
		if shader == nil {
			return nil, ""
		}
		return fill(shader), ""
	}
	if r.Source == nil {
		return nil, "no source bitmap"
	}
	// pattern gives the color of the brush at a point of the source bitmap.
	// cells is the number of cells across a pixel of the source, so that
	// each pixel of the brush has its cell when the bitmap is stretched.
	pattern := func(_, _ float64) color.NRGBA { return color.NRGBA{} }
	cells := 1
	if op.UsesPattern() {
		if r.Pattern == nil {
			return nil, "no brush"
		}
		var scale float64
		var ok bool
		if pattern, scale, ok = b.patternAt(*r.Pattern, r.Source.Transform); !ok {
			return nil, "a gradient brush with a source bitmap"
		}
		cells = int(min(math.Ceil(scale), 256))
	}
	src := r.Source.Source.Intersect(r.Source.Image.Bounds())
	if src.Empty() {
		return nil, ""
	}
	if int64(src.Dx())*int64(src.Dy()) > b.pic.lim.MaxPixels {
		return nil, "a source bitmap past the limit of pixels"
	}
	for cells > 1 && int64(src.Dx()*cells)*int64(src.Dy()*cells) > b.pic.lim.MaxPixels {
		cells--
	}
	k := float64(cells)
	out := image.NewNRGBA(image.Rect(src.Min.X*cells, src.Min.Y*cells, src.Max.X*cells, src.Max.Y*cells))
	for y := src.Min.Y; y < src.Max.Y; y++ {
		for x := src.Min.X; x < src.Max.X; x++ {
			s := color.NRGBAModel.Convert(r.Source.Image.At(x, y)).(color.NRGBA)
			for cy := y * cells; cy < (y+1)*cells; cy++ {
				for cx := x * cells; cx < (x+1)*cells; cx++ {
					pc := pattern((float64(cx)+0.5)/k, (float64(cy)+0.5)/k)
					out.SetNRGBA(cx, cy, color.NRGBA{op.Apply(pc.R, s.R, 0), op.Apply(pc.G, s.G, 0), op.Apply(pc.B, s.B, 0), 255})
				}
			}
		}
	}
	im := *r.Source
	im.Image, im.Source, im.Opacity = out, out.Bounds(), 1
	im.Transform = gowemf.Matrix{M11: 1 / k, M22: 1 / k}.Then(im.Transform)
	img, t, ok := b.placeImage(im)
	if !ok {
		return nil, ""
	}
	return func() {
		b.d.DrawImage(img, t, display.ImageOptions{Opacity: 1, Smooth: im.Smooth})
	}, ""
}

// patternAt returns the function that gives the color of the brush p at a
// point of a source bitmap, which place maps to the user space. It also
// returns the number of pixels of the brush across a pixel of the source. It
// returns false for a gradient.
func (b *backend) patternAt(p gowemf.Paint, place gowemf.Matrix) (at func(x, y float64) color.NRGBA, scale float64, ok bool) {
	var tile image.Image
	switch p.Kind {
	case gowemf.PaintSolid:
		c := p.Color
		return func(float64, float64) color.NRGBA { return c }, 1, true
	case gowemf.PaintHatch:
		tile = hatchImage(p)
	case gowemf.PaintPattern:
		tile = p.Pattern
	}
	if tile == nil || tile.Bounds().Empty() {
		return nil, 0, false
	}
	toTile, ok := transform(p.PatternTransform).Invert()
	if !ok {
		return nil, 0, false
	}
	t := transform(place).PostConcat(toTile)
	scale = max(math.Hypot(float64(t.SX), float64(t.KY)), math.Hypot(float64(t.KX), float64(t.SY)))
	if !(scale >= 1) || math.IsInf(scale, 0) {
		scale = 1
	}
	bounds := tile.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	return func(x, y float64) color.NRGBA {
		pt := okage.PointFromXY(float32(x), float32(y))
		t.MapPoint(&pt)
		// a pattern of GDI repeats its tile
		tx, ty := int(math.Floor(float64(pt.X))), int(math.Floor(float64(pt.Y)))
		tx, ty = ((tx%w)+w)%w, ((ty%h)+h)%h
		return color.NRGBAModel.Convert(tile.At(bounds.Min.X+tx, bounds.Min.Y+ty)).(color.NRGBA)
	}, scale, true
}

// invertPaint returns the inverse of the colors of a solid color, of a
// hatch or of a pattern, and false for another paint.
func invertPaint(p gowemf.Paint) (gowemf.Paint, bool) {
	invert := func(c color.NRGBA) color.NRGBA { return color.NRGBA{^c.R, ^c.G, ^c.B, c.A} }
	switch p.Kind {
	case gowemf.PaintSolid, gowemf.PaintHatch:
		p.Color = invert(p.Color)
		if p.Background != nil {
			bg := invert(*p.Background)
			p.Background = &bg
		}
		return p, true
	case gowemf.PaintPattern:
		if p.Pattern == nil {
			return p, false
		}
		bounds := p.Pattern.Bounds()
		out := image.NewNRGBA(bounds)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				out.SetNRGBA(x, y, invert(color.NRGBAModel.Convert(p.Pattern.At(x, y)).(color.NRGBA)))
			}
		}
		p.Pattern = out
		return p, true
	}
	return p, false
}
