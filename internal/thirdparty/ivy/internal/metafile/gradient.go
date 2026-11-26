package metafile

import (
	"image"
	"image/color"
	"math"

	"github.com/xo/okage"

	"ragflow/internal/thirdparty/ivy/internal/display"
	"ragflow/internal/thirdparty/ivy/internal/gowemf"
)

// maxGradientSide is the largest side, in pixels, of the image that draws a
// triangle of a gradient whose colors do not change along one direction.
const maxGradientSide = 256

// FillGradient satisfies the gowemf.GradientBackend interface. A triangle
// whose colors change along one direction, such as each triangle of a
// rectangle of EMR_GRADIENTFILL, is a linear gradient. The triangles that
// follow it with the same gradient join it in one path, so that no seam
// shows between them. Another triangle is an image of its colors, clipped to
// the triangle.
func (b *backend) FillGradient(mesh []gowemf.GradientTriangle, clip gowemf.Clip) error {
	if err := b.begin(); err != nil {
		return err
	}
	b.clipped(clip, func() {
		var (
			pb   *okage.PathBuilder
			cur  linear
			have bool
		)
		flush := func() {
			if !have {
				return
			}
			if p, ok := pb.Finish(); ok {
				if shader := cur.shader(); shader != nil {
					b.d.FillPath(p, okage.FillRuleWinding, shader)
				}
			}
			have = false
		}
		for _, t := range mesh {
			if b.ctx.Err() != nil {
				return
			}
			g, ok := linearOf(t)
			if !ok {
				flush()
				b.shade(t)
				continue
			}
			if !have || !g.same(cur) {
				flush()
				pb, cur, have = okage.NewPathBuilder(), g, true
			}
			pb.MoveTo(float32(t.Points[0].X), float32(t.Points[0].Y))
			pb.LineTo(float32(t.Points[1].X), float32(t.Points[1].Y))
			pb.LineTo(float32(t.Points[2].X), float32(t.Points[2].Y))
			pb.Close()
		}
		flush()
	})
	return nil
}

// linear is a linear gradient from the color c0 at p0 to the color c1 at p1.
type linear struct {
	p0, p1 gowemf.Point
	c0, c1 color.NRGBA64
}

// shader returns the shader of the gradient.
func (g linear) shader() okage.Shader {
	c0, c1 := rgba64(g.c0), rgba64(g.c1)
	if g.p0 == g.p1 {
		return okage.SolidColor(c0)
	}
	stops := []okage.GradientStop{okage.NewGradientStop(0, c0), okage.NewGradientStop(1, c1)}
	s, ok := okage.NewLinearGradient(okage.PointFromXY(float32(g.p0.X), float32(g.p0.Y)),
		okage.PointFromXY(float32(g.p1.X), float32(g.p1.Y)), stops, okage.SpreadModePad, okage.IdentityTransform())
	if !ok {
		return okage.SolidColor(c0)
	}
	return s
}

// rgba64 converts a color of 16 bits for each channel.
func rgba64(c color.NRGBA64) okage.Color {
	return okage.ColorFromRGBA8(uint8(c.R>>8), uint8(c.G>>8), uint8(c.B>>8), uint8(c.A>>8))
}

// linearOf returns the linear gradient of the triangle t, and false if its
// colors do not change along one direction. The color of a triangle is an
// affine function of the position. Each channel has a gradient vector, and
// the colors change along one direction when the vectors are parallel.
func linearOf(t gowemf.GradientTriangle) (linear, bool) {
	p, c := t.Points, t.Colors
	ax, ay := p[1].X-p[0].X, p[1].Y-p[0].Y
	bx, by := p[2].X-p[0].X, p[2].Y-p[0].Y
	det := ax*by - ay*bx
	if det == 0 || math.IsNaN(det) || math.IsInf(det, 0) {
		return linear{}, false
	}
	// the gradient vector of each channel, from the differences of its
	// values along the two edges from p[0]
	var grads [4][2]float64
	var best int
	var bestNorm float64
	for i := range 4 {
		v0, v1, v2 := channel(c[0], i), channel(c[1], i), channel(c[2], i)
		da, db := v1-v0, v2-v0
		gx := (da*by - db*ay) / det
		gy := (db*ax - da*bx) / det
		grads[i] = [2]float64{gx, gy}
		if n := gx*gx + gy*gy; n > bestNorm {
			best, bestNorm = i, n
		}
	}
	if bestNorm == 0 {
		return linear{c0: c[0], c1: c[0]}, true
	}
	u := grads[best]
	un := math.Sqrt(bestNorm)
	for _, g := range grads {
		// the cross product, against the length of both vectors
		if math.Abs(g[0]*u[1]-g[1]*u[0]) > 1e-6*un*(math.Hypot(g[0], g[1])+un) {
			return linear{}, false
		}
	}
	// the parameter of each corner along u, and the corners at its ends
	lo, hi := 0, 0
	var s [3]float64
	for i := range 3 {
		s[i] = (p[i].X-p[0].X)*u[0] + (p[i].Y-p[0].Y)*u[1]
		if s[i] < s[lo] {
			lo = i
		}
		if s[i] > s[hi] {
			hi = i
		}
	}
	// the ends of the gradient are on the line through the origin along u,
	// so that two triangles of the same gradient give the same ends
	at := func(i int) gowemf.Point {
		v := (p[i].X*u[0] + p[i].Y*u[1]) / bestNorm
		return gowemf.Point{X: v * u[0], Y: v * u[1]}
	}
	return linear{p0: at(lo), p1: at(hi), c0: c[lo], c1: c[hi]}, true
}

// same reports whether the gradients g and o are the same, but for the
// rounding of their ends.
func (g linear) same(o linear) bool {
	if g.c0 != o.c0 || g.c1 != o.c1 {
		return false
	}
	scale := math.Hypot(g.p1.X-g.p0.X, g.p1.Y-g.p0.Y)
	near := func(a, b gowemf.Point) bool {
		return math.Hypot(a.X-b.X, a.Y-b.Y) <= 1e-6*scale
	}
	return near(g.p0, o.p0) && near(g.p1, o.p1)
}

// channel returns the channel i of c, from 0 to 1.
func channel(c color.NRGBA64, i int) float64 {
	switch i {
	case 0:
		return float64(c.R) / 0xffff
	case 1:
		return float64(c.G) / 0xffff
	case 2:
		return float64(c.B) / 0xffff
	}
	return float64(c.A) / 0xffff
}

// shade draws the triangle t as an image of its colors, clipped to the
// triangle. The image has a pixel for each unit of the user space, and at
// most maxGradientSide pixels on each side.
func (b *backend) shade(t gowemf.GradientTriangle) {
	p := t.Points
	minX, maxX := math.Min(p[0].X, math.Min(p[1].X, p[2].X)), math.Max(p[0].X, math.Max(p[1].X, p[2].X))
	minY, maxY := math.Min(p[0].Y, math.Min(p[1].Y, p[2].Y)), math.Max(p[0].Y, math.Max(p[1].Y, p[2].Y))
	w, h := maxX-minX, maxY-minY
	if !(w > 0 && h > 0) || math.IsInf(w, 0) || math.IsInf(h, 0) {
		return
	}
	pw := int(math.Min(maxGradientSide, math.Max(2, math.Ceil(w))))
	ph := int(math.Min(maxGradientSide, math.Max(2, math.Ceil(h))))
	img := image.NewNRGBA(image.Rect(0, 0, pw, ph))
	det := (p[1].Y-p[2].Y)*(p[0].X-p[2].X) + (p[2].X-p[1].X)*(p[0].Y-p[2].Y)
	if det == 0 {
		return
	}
	for y := range ph {
		for x := range pw {
			px := minX + (float64(x)+0.5)*w/float64(pw)
			py := minY + (float64(y)+0.5)*h/float64(ph)
			l0 := ((p[1].Y-p[2].Y)*(px-p[2].X) + (p[2].X-p[1].X)*(py-p[2].Y)) / det
			l1 := ((p[2].Y-p[0].Y)*(px-p[2].X) + (p[0].X-p[2].X)*(py-p[2].Y)) / det
			l2 := 1 - l0 - l1
			i := img.PixOffset(x, y)
			for ch := range 4 {
				v := l0*channel(t.Colors[0], ch) + l1*channel(t.Colors[1], ch) + l2*channel(t.Colors[2], ch)
				img.Pix[i+ch] = uint8(math.Round(255 * math.Min(1, math.Max(0, v))))
			}
		}
	}
	pb := okage.NewPathBuilder()
	pb.MoveTo(float32(p[0].X), float32(p[0].Y))
	pb.LineTo(float32(p[1].X), float32(p[1].Y))
	pb.LineTo(float32(p[2].X), float32(p[2].Y))
	pb.Close()
	tri, ok := pb.Finish()
	if !ok {
		return
	}
	b.d.Save()
	defer b.d.Restore()
	b.d.ClipPath(tri, okage.FillRuleWinding)
	t2 := okage.TransformFromRow(float32(w/float64(pw)), 0, 0, float32(h/float64(ph)), float32(minX), float32(minY))
	b.d.DrawImage(img, t2, display.ImageOptions{Opacity: 1, Smooth: true})
}
