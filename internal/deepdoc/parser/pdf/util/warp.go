package util

import (
	"image"
	"image/color"
	"image/draw"
	"math"
)

// Pt is a 2D float point used for warp corners.
type Pt struct {
	X, Y float64
}

// WarpCrop de-skews a quadrilateral region from src using a perspective
// transform, producing the rectangular crop fed to text recognition.
//
// points must be the 4 corners in order: top-left, top-right, bottom-right,
// bottom-left (the DBNet quad order emitted by the OCR detector). The output
// size is (W, H) where
//
//	W = int(max(|p0-p1|, |p2-p3|))
//	H = int(max(|p0-p3|, |p1-p2|))
//
// Each destination pixel is mapped back to the source via the inverse
// homography and sampled with Catmull-Rom (bicubic) interpolation. Out-of-
// bounds source coordinates use BORDER_REPLICATE semantics (edge pixels
// repeated).
//
// WarpCrop performs NO rotation selection (the h/w >= 1.5 branch) — that
// belongs to the caller / layer 2.
//
// If the quad is degenerate (collinear / non-invertible homography), WarpCrop
// falls back to an axis-aligned crop of the quad's bounding box so callers
// stay safe.
func WarpCrop(src image.Image, points [4]Pt) *image.RGBA {
	w := int(math.Max(dist(points[0], points[1]), dist(points[2], points[3])))
	h := int(math.Max(dist(points[0], points[3]), dist(points[1], points[2])))
	if w <= 0 || h <= 0 {
		return axisFallback(src, points)
	}

	dst := [4]Pt{{0, 0}, {float64(w), 0}, {float64(w), float64(h)}, {0, float64(h)}}
	hMat, ok := perspectiveTransform(points, dst)
	if !ok {
		return axisFallback(src, points)
	}
	inv, ok := invert3x3(hMat)
	if !ok {
		return axisFallback(src, points)
	}

	rgba := toRGBA(src)
	b := rgba.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			// Backward map: src = inv * [x, y, 1].
			den := inv[6]*float64(x) + inv[7]*float64(y) + inv[8]
			if den == 0 {
				continue
			}
			sx := (inv[0]*float64(x) + inv[1]*float64(y) + inv[2]) / den
			sy := (inv[3]*float64(x) + inv[4]*float64(y) + inv[5]) / den
			out.SetRGBA(x, y, sampleBicubic(rgba, sx, sy, b))
		}
	}
	return out
}

// perspectiveTransform solves the 8-DOF homography H (row-major 3x3 with
// H[8]=1) such that dst_i = H * src_i in homogeneous coordinates. It fixes the
// bottom-right homography element to 1 (the 8-DOF normalization). Returns
// ok=false if the linear system is singular.
func perspectiveTransform(src, dst [4]Pt) ([9]float64, bool) {
	var A [8][9]float64
	for i := 0; i < 4; i++ {
		sx, sy := src[i].X, src[i].Y
		dx, dy := dst[i].X, dst[i].Y
		// x' equation.
		A[2*i][0] = sx
		A[2*i][1] = sy
		A[2*i][2] = 1
		A[2*i][6] = -sx * dx
		A[2*i][7] = -sy * dx
		A[2*i][8] = dx
		// y' equation.
		A[2*i+1][3] = sx
		A[2*i+1][4] = sy
		A[2*i+1][5] = 1
		A[2*i+1][6] = -sx * dy
		A[2*i+1][7] = -sy * dy
		A[2*i+1][8] = dy
	}
	x, ok := solveLinear8(A)
	if !ok {
		return [9]float64{}, false
	}
	return [9]float64{x[0], x[1], x[2], x[3], x[4], x[5], x[6], x[7], 1}, true
}

// solveLinear8 solves A * x = b for an 8x8 system via Gaussian elimination
// with partial pivoting. b is stored in the last column of A.
func solveLinear8(A [8][9]float64) ([8]float64, bool) {
	for col := 0; col < 8; col++ {
		// Partial pivot.
		pivot := col
		maxAbs := math.Abs(A[col][col])
		for r := col + 1; r < 8; r++ {
			if v := math.Abs(A[r][col]); v > maxAbs {
				maxAbs = v
				pivot = r
			}
		}
		if maxAbs < 1e-12 {
			return [8]float64{}, false
		}
		A[col], A[pivot] = A[pivot], A[col]
		// Eliminate below.
		for r := col + 1; r < 8; r++ {
			f := A[r][col] / A[col][col]
			for c := col; c < 9; c++ {
				A[r][c] -= f * A[col][c]
			}
		}
	}
	// Back-substitution.
	var x [8]float64
	for r := 7; r >= 0; r-- {
		sum := A[r][8]
		for c := r + 1; c < 8; c++ {
			sum -= A[r][c] * x[c]
		}
		x[r] = sum / A[r][r]
	}
	return x, true
}

// invert3x3 returns the inverse of the row-major 3x3 matrix m. Returns
// ok=false if singular.
func invert3x3(m [9]float64) ([9]float64, bool) {
	det := m[0]*(m[4]*m[8]-m[5]*m[7]) -
		m[1]*(m[3]*m[8]-m[5]*m[6]) +
		m[2]*(m[3]*m[7]-m[4]*m[6])
	if math.Abs(det) < 1e-12 {
		return [9]float64{}, false
	}
	invDet := 1.0 / det
	return [9]float64{
		(m[4]*m[8] - m[5]*m[7]) * invDet,
		(m[2]*m[7] - m[1]*m[8]) * invDet,
		(m[1]*m[5] - m[2]*m[4]) * invDet,
		(m[5]*m[6] - m[3]*m[8]) * invDet,
		(m[0]*m[8] - m[2]*m[6]) * invDet,
		(m[2]*m[3] - m[0]*m[5]) * invDet,
		(m[3]*m[7] - m[4]*m[6]) * invDet,
		(m[1]*m[6] - m[0]*m[7]) * invDet,
		(m[0]*m[4] - m[1]*m[3]) * invDet,
	}, true
}

// sampleBicubic returns the bicubic-interpolated (Catmull-Rom) color at the
// (possibly sub-pixel, out-of-bounds) location (x, y). Out-of-bounds
// coordinates use BORDER_REPLICATE semantics (edge pixels repeated). b is the
// source image bounds; sampling indices are offset by b.Min so a non-zero
// origin image samples correctly.
func sampleBicubic(img *image.RGBA, x, y float64, b image.Rectangle) color.RGBA {
	ox, oy := float64(b.Min.X), float64(b.Min.Y)
	x0 := int(math.Floor(x - ox))
	y0 := int(math.Floor(y - oy))
	tx := x - ox - float64(x0)
	ty := y - oy - float64(y0)
	maxX, maxY := b.Dx()-1, b.Dy()-1

	// Interpolate each of the 4 source rows horizontally, then combine
	// the 4 results vertically.
	colX := func(cy int) (uint8, uint8, uint8, uint8) {
		r0, g0, b0, a0 := pxAt(img, b.Min.X+clampIdx(x0-1, maxX), b.Min.Y+clampIdx(cy, maxY))
		r1, g1, b1, a1 := pxAt(img, b.Min.X+clampIdx(x0, maxX), b.Min.Y+clampIdx(cy, maxY))
		r2, g2, b2, a2 := pxAt(img, b.Min.X+clampIdx(x0+1, maxX), b.Min.Y+clampIdx(cy, maxY))
		r3, g3, b3, a3 := pxAt(img, b.Min.X+clampIdx(x0+2, maxX), b.Min.Y+clampIdx(cy, maxY))
		return uint8(clampByte(cubic(tx, [4]float64{float64(r0), float64(r1), float64(r2), float64(r3)}))),
			uint8(clampByte(cubic(tx, [4]float64{float64(g0), float64(g1), float64(g2), float64(g3)}))),
			uint8(clampByte(cubic(tx, [4]float64{float64(b0), float64(b1), float64(b2), float64(b3)}))),
			uint8(clampByte(cubic(tx, [4]float64{float64(a0), float64(a1), float64(a2), float64(a3)})))
	}

	rA, gA, bA, aA := colX(y0 - 1)
	rB, gB, bB, aB := colX(y0)
	rC, gC, bC, aC := colX(y0 + 1)
	rD, gD, bD, aD := colX(y0 + 2)
	return color.RGBA{
		R: uint8(clampByte(cubic(ty, [4]float64{float64(rA), float64(rB), float64(rC), float64(rD)}))),
		G: uint8(clampByte(cubic(ty, [4]float64{float64(gA), float64(gB), float64(gC), float64(gD)}))),
		B: uint8(clampByte(cubic(ty, [4]float64{float64(bA), float64(bB), float64(bC), float64(bD)}))),
		A: uint8(clampByte(cubic(ty, [4]float64{float64(aA), float64(aB), float64(aC), float64(aD)}))),
	}
}

// pxAt returns the RGBA bytes at (x, y), with coordinates already clamped by
// the caller (BORDER_REPLICATE).
func pxAt(img *image.RGBA, x, y int) (r, g, b, a uint8) {
	c := img.RGBAAt(x, y)
	return c.R, c.G, c.B, c.A
}

func clampIdx(i, max int) int {
	if i < 0 {
		return 0
	}
	if i > max {
		return max
	}
	return i
}

func clampByte(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

// cubic is the Catmull-Rom cubic basis for parameter t in [0,1] over the four
// control samples p0..p3.
func cubic(t float64, p [4]float64) float64 {
	t2 := t * t
	t3 := t2 * t
	return 0.5 * ((2 * p[1]) +
		(-p[0]+p[2])*t +
		(2*p[0]-5*p[1]+4*p[2]-p[3])*t2 +
		(-p[0]+3*p[1]-3*p[2]+p[3])*t3)
}

// toRGBA returns src as *image.RGBA, converting when necessary.
func toRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	b := src.Bounds()
	out := image.NewRGBA(b)
	draw.Draw(out, b, src, b.Min, draw.Src)
	return out
}

// axisFallback crops the bounding box of the quad with FastCrop.
func axisFallback(src image.Image, points [4]Pt) *image.RGBA {
	minX, minY := math.MaxFloat64, math.MaxFloat64
	maxX, maxY := -math.MaxFloat64, -math.MaxFloat64
	for _, p := range points {
		minX = math.Min(minX, p.X)
		minY = math.Min(minY, p.Y)
		maxX = math.Max(maxX, p.X)
		maxY = math.Max(maxY, p.Y)
	}
	return FastCrop(src, int(minX), int(minY), int(maxX), int(maxY))
}

func dist(a, b Pt) float64 {
	return math.Hypot(a.X-b.X, a.Y-b.Y)
}
