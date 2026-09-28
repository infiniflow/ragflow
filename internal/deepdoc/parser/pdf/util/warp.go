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
// maxWarpDim bounds the allocated crop so a (clamped) quad can never drive an
// unbounded image.NewRGBA. Detector boxes arrive from the DocAnalyzer backend
// and are treated as untrusted; this ceiling is a last line of
// defence against an unexpectedly large source image even after clamping.
const maxWarpDim = 1 << 16

func WarpCrop(src image.Image, points [4]Pt) *image.RGBA {
	// Detection boxes come from the DocAnalyzer backend and are
	// effectively untrusted. FastCrop clamps its rectangle to the source
	// bounds before allocating; this path must do the same on its four
	// corners and must reject non-finite coordinates, so a malformed or
	// out-of-range response cannot drive an unbounded image.NewRGBA (panic /
	// OOM). On a normal in-bounds quad the clamp is a no-op, so detection
	// accuracy is unchanged.
	if !pointsFinite(points) {
		return image.NewRGBA(image.Rect(0, 0, 1, 1))
	}
	rgba := toRGBA(src)
	b := rgba.Bounds()
	pts := clampQuad(points, b)

	// Axis-aligned fast path: an axis-parallel quad is just a sub-rectangle,
	// so the perspective warp degenerates to a copy. FastCrop does exactly
	// that with a direct Pix slice copy (no per-pixel bicubic resampling),
	// which is far cheaper. Table cells and char-derived boxes are always
	// axis-aligned, so this short-circuits the common OCR paths to the cheap
	// copy — the de-skew is only paid for genuinely slanted detection quads.
	if axisAligned(pts) {
		minX := int(math.Min(pts[0].X, math.Min(pts[1].X, math.Min(pts[2].X, pts[3].X))))
		minY := int(math.Min(pts[0].Y, math.Min(pts[1].Y, math.Min(pts[2].Y, pts[3].Y))))
		maxX := int(math.Max(pts[0].X, math.Max(pts[1].X, math.Max(pts[2].X, pts[3].X))))
		maxY := int(math.Max(pts[0].Y, math.Max(pts[1].Y, math.Max(pts[2].Y, pts[3].Y))))
		return FastCrop(rgba, minX, minY, maxX, maxY)
	}

	w := int(math.Max(dist(pts[0], pts[1]), dist(pts[2], pts[3])))
	h := int(math.Max(dist(pts[0], pts[3]), dist(pts[1], pts[2])))
	if w <= 0 || h <= 0 || w > maxWarpDim || h > maxWarpDim {
		return axisFallback(src, pts)
	}

	dst := [4]Pt{{0, 0}, {float64(w), 0}, {float64(w), float64(h)}, {0, float64(h)}}
	hMat, ok := perspectiveTransform(pts, dst)
	if !ok {
		return axisFallback(src, pts)
	}
	inv, ok := invert3x3(hMat)
	if !ok {
		return axisFallback(src, pts)
	}

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

// pointsFinite reports whether all four corner coordinates are finite. A
// non-finite value from a malformed detector response must be rejected before
// any dimension derivation or allocation.
func pointsFinite(p [4]Pt) bool {
	for _, q := range p {
		if math.IsNaN(q.X) || math.IsNaN(q.Y) || math.IsInf(q.X, 0) || math.IsInf(q.Y, 0) {
			return false
		}
	}
	return true
}

// clampQuad clamps every corner to the source image bounds. FastCrop performs
// the equivalent clamp on its axis-aligned rectangle; WarpCrop must do the same
// on its four corners so an out-of-range detector box cannot produce an
// out-of-bounds or unbounded crop. Corners already inside the bounds are
// returned unchanged, so a well-formed detection box is unaffected.
func clampQuad(p [4]Pt, b image.Rectangle) [4]Pt {
	out := p
	minX, minY := float64(b.Min.X), float64(b.Min.Y)
	maxX, maxY := float64(b.Max.X), float64(b.Max.Y)
	for i := range out {
		if out[i].X < minX {
			out[i].X = minX
		} else if out[i].X > maxX {
			out[i].X = maxX
		}
		if out[i].Y < minY {
			out[i].Y = minY
		} else if out[i].Y > maxY {
			out[i].Y = maxY
		}
	}
	return out
}

// axisAligned reports whether the quad is axis-parallel: its left/right edges
// are vertical and its top/bottom edges are horizontal, within a small epsilon.
// The OCR detector can emit sub-pixel jitter on an otherwise upright box; that
// jitter is negligible for recognition, so the cheap FastCrop path is still
// correct for it. A genuinely slanted detection quad fails this test and pays
// the full perspective warp instead.
func axisAligned(p [4]Pt) bool {
	const eps = 1e-3
	// Quad order is TL, TR, BR, BL.
	// Left edge TL-BL vertical:   p0.X == p3.X
	// Right edge TR-BR vertical:  p1.X == p2.X
	// Top edge TL-TR horizontal:  p0.Y == p1.Y
	// Bottom edge BL-BR horizonal: p3.Y == p2.Y
	return approxEq(p[0].X, p[3].X, eps) &&
		approxEq(p[1].X, p[2].X, eps) &&
		approxEq(p[0].Y, p[1].Y, eps) &&
		approxEq(p[3].Y, p[2].Y, eps)
}

func approxEq(a, b, eps float64) bool { return math.Abs(a-b) <= eps }
