//go:build cgo

package native

// det_helpers.go — small sorting / rasterization helpers for the pure-Go DB
// post-process (det.go).

import (
	"math"
	"sort"
)

// sortPts orders points by x then y (used by the monotone-chain convex hull).
func sortPts(p []pt) {
	sort.Slice(p, func(i, j int) bool {
		if p[i].X != p[j].X {
			return p[i].X < p[j].X
		}
		return p[i].Y < p[j].Y
	})
}

// sortPtsByX is a STABLE sort by x (mirrors Python sorted(..., key=lambda x: x[0]),
// which getMiniBoxes relies on for tie-breaking).
func sortPtsByX(p []pt) {
	sort.SliceStable(p, func(i, j int) bool {
		return p[i].X < p[j].X
	})
}

// fillPoly rasterizes a polygon into a bool mask, bit-for-bit matching
// cv2.fillPoly (OpenCV 4.10.0, modules/imgproc/src/drawing.cpp). It is a
// faithful port of the general polygon path that cv2.fillPoly actually uses:
//
//   - CollectPolyEdges: for each edge it draws the 1px outline via cv::line
//     (8-connected LineIterator DDA, see drawLine8) on the integer vertices,
//     then builds a fixed-point PolyEdge with dx = (pt1c.x - pt0c.x)/(pt1c.y -
//     pt0c.y) using C++/Go truncation-toward-zero integer division (NOT floor
//     division — Python's // floors, which is the classic source of a 1px
//     boundary mismatch on edges with negative slope);
//   - FillEdgeCollection: a scanline fill over the active edges with delta=0,
//     i.e. pixel columns are fixed_x >> 16 (truncation, matching OpenCV).
//
// Vertices are truncated toward zero (math.Trunc), exactly mirroring cv2's
// np.int32 cast on the box coordinates. The det score is mean(pred) over the
// masked pixels, so this bit-exact mask rasterization is what removes the
// gap-3 orphans that the old FillConvexPoly scanline introduced.
func fillPoly(mask []bool, mw, mh int, poly [4]pt) {
	const xyShift = 16
	const xyOne = int64(1) << xyShift

	// Integer (truncated-toward-zero) vertex coords, matching cv2 int32 cast.
	v := [4]struct{ x, y int64 }{}
	for i := range poly {
		v[i].x = int64(math.Trunc(poly[i].X))
		v[i].y = int64(math.Trunc(poly[i].Y))
	}

	edges := make([]polyEdge, 0, 4)
	for i := 0; i < 4; i++ {
		prev := (i + 3) % 4
		pt0x := v[prev].x << xyShift
		pt0y := v[prev].y
		pt1x := v[i].x << xyShift
		pt1y := v[i].y

		// Outline: cv2.fillPoly draws cv::line between the integer vertices
		// (t0.x = (pt0.x + 0.5) truncated = pt0.x for integer vertices).
		drawLine8(mask, mw, mh, int(v[prev].x), int(v[prev].y), int(v[i].x), int(v[i].y))

		// Build the fixed-point edge. Mirror CollectPolyEdges: clip the
		// outline endpoints to the image, and use the clipped integer points
		// for the edge geometry when the edge leaves the image.
		t0x := (pt0x + (xyOne >> 1)) >> xyShift
		t0y := pt0y
		t1x := (pt1x + (xyOne >> 1)) >> xyShift
		t1y := pt1y
		var pt0cX, pt0cY, pt1cX, pt1cY int64
		if uint64(t0x) >= uint64(mw) || uint64(t1x) >= uint64(mw) ||
			uint64(t0y) >= uint64(mh) || uint64(t1y) >= uint64(mh) {
			cx0, cy0, cx1, cy1 := clipLine(mw, mh, int(t0x), int(t0y), int(t1x), int(t1y))
			if cy0 != cy1 {
				pt0cY, pt1cY = int64(cy0), int64(cy1)
				pt0cX, pt1cX = int64(cx0)<<xyShift, int64(cx1)<<xyShift
			} else {
				pt0cX, pt0cY = pt0x+(xyOne>>1), pt0y
				pt1cX, pt1cY = pt1x+(xyOne>>1), pt1y
			}
		} else {
			pt0cX, pt0cY = pt0x+(xyOne>>1), pt0y
			pt1cX, pt1cY = pt1x+(xyOne>>1), pt1y
		}

		if pt0y == pt1y {
			continue
		}
		// Truncation toward zero — Go's / on int64 matches C++ (and OpenCV).
		dx := (pt1cX - pt0cX) / (pt1cY - pt0cY)
		if pt0y < pt1y {
			edges = append(edges, polyEdge{
				y0: int(pt0y), y1: int(pt1y),
				x: pt0cX + (pt0y-pt0cY)*dx, dx: dx,
			})
		} else {
			edges = append(edges, polyEdge{
				y0: int(pt1y), y1: int(pt0y),
				x: pt1cX + (pt1y-pt1cY)*dx, dx: dx,
			})
		}
	}

	if len(edges) == 0 {
		return
	}
	ymin, ymax := mh, 0
	for _, e := range edges {
		if e.y0 < ymin {
			ymin = e.y0
		}
		if e.y1 > ymax {
			ymax = e.y1
		}
	}
	if ymin < 0 {
		ymin = 0
	}
	if ymax > mh {
		ymax = mh
	}
	for y := ymin; y < ymax; y++ {
		xs := make([]int64, 0, len(edges))
		for _, e := range edges {
			if y >= e.y0 && y < e.y1 {
				xs = append(xs, e.x+int64(y-e.y0)*e.dx)
			}
		}
		sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
		for k := 0; k+1 < len(xs); k += 2 {
			a := xs[k] >> xyShift
			b := xs[k+1] >> xyShift
			if b >= 0 && a < int64(mw) {
				xa := int(a)
				if xa < 0 {
					xa = 0
				}
				xb := int(b)
				if xb >= mw {
					xb = mw - 1
				}
				base := y * mw
				for x := xa; x <= xb; x++ {
					mask[base+x] = true
				}
			}
		}
	}
}

// polyEdge is one fixed-point scanline edge (OpenCV PolyEdge).
type polyEdge struct {
	y0, y1 int
	x, dx  int64
}

// drawLine8 draws an 8-connected (Bresenham) line into mask, matching cv2.line
// with thickness=1 and lineType=LINE_8. It is a faithful port of OpenCV's
// cv::LineIterator (connectivity == 8): the DDA error term and the swap for the
// major axis are reproduced exactly so the outline pixels equal cv::line's.
func drawLine8(mask []bool, mw, mh, x0, y0, x1, y1 int) {
	dx := x1 - x0
	dy := y1 - y0
	deltaX, deltaY := 1, 1
	if dx < 0 {
		// LineIterator leftToRight == true: walk from the far endpoint.
		dx = -dx
		dy = -dy
		x0, y0 = x1, y1
	}
	if dy < 0 {
		dy = -dy
		deltaY = -1
	}
	vert := dy > dx
	if vert {
		dx, dy = dy, dx
		deltaX, deltaY = deltaY, deltaX
	}
	// connectivity == 8
	err := dx - (dy + dy)
	plusDelta := dx + dx
	minusDelta := -(dy + dy)
	minusShift := deltaX
	plusShift := 0
	minusStep := 0
	plusStep := deltaY
	count := dx + 1
	if vert {
		plusStep, plusShift = plusShift, plusStep
		minusStep, minusShift = minusShift, minusStep
	}
	px, py := x0, y0
	for i := 0; i < count; i++ {
		if px >= 0 && px < mw && py >= 0 && py < mh {
			mask[py*mw+px] = true
		}
		// OpenCV LineIterator::operator++ (imgproc.hpp): when err < 0 BOTH
		// the minor and major steps are taken, producing a diagonal pixel.
		// This is what makes an 8-connected line reach its exact endpoint.
		if err < 0 {
			err += minusDelta + plusDelta
			px += minusShift + plusShift
			py += minusStep + plusStep
		} else {
			err += minusDelta
			px += minusShift
			py += minusStep
		}
	}
}

// clipLine clips the segment (x0,y0)-(x1,y1) to the [0,mw)x[0,mh) rectangle
// (Cohen–Sutherland, integer), mirroring OpenCV's clipLine. Returns the clipped
// endpoints; callers pass these straight to masked writes.
func clipLine(mw, mh, x0, y0, x1, y1 int) (int, int, int, int) {
	inside := func(x, y int) int {
		code := 0
		if x < 0 {
			code |= 1
		} else if x >= mw {
			code |= 2
		}
		if y < 0 {
			code |= 4
		} else if y >= mh {
			code |= 8
		}
		return code
	}
	c0, c1 := inside(x0, y0), inside(x1, y1)
	for c0|c1 != 0 {
		if c0&c1 != 0 {
			return x0, y0, x1, y1 // fully outside
		}
		var x, y, c int
		if c0 != 0 {
			c, x, y = c0, x0, y0
		} else {
			c, x, y = c1, x1, y1
		}
		if c&1 != 0 {
			y = y0 + (y1-y0)*(0-x0)/(x1-x0)
			x = 0
		} else if c&2 != 0 {
			y = y0 + (y1-y0)*(mw-1-x0)/(x1-x0)
			x = mw - 1
		} else if c&4 != 0 {
			x = x0 + (x1-x0)*(0-y0)/(y1-y0)
			y = 0
		} else if c&8 != 0 {
			x = x0 + (x1-x0)*(mh-1-y0)/(y1-y0)
			y = mh - 1
		}
		if c == c0 {
			x0, y0, c0 = x, y, inside(x, y)
		} else {
			x1, y1, c1 = x, y, inside(x, y)
		}
	}
	return x0, y0, x1, y1
}

func math_min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func math_max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
