//go:build cgo

package native

// clipper_offset.go — faithful pure-Go port of Clipper1's ClipperOffset
// (Angus Johnson, MIT) restricted to the single case DBNet's unclip needs:
// JoinType.JT_ROUND + EndType.ET_CLOSEDPOLYGON on one convex quad.
//
// Why this exists: pyclipper (the Python binding the deepdoc oracle uses) is
// Clipper1, which works entirely in INTEGER coordinates. The float quad is
// first cast to int64 by truncation toward zero (IntPoint's cInt cast), the
// offset is computed with round-half-away (C++ Round == Go math.Round), and
// the result is returned as int64 coordinates. Our earlier hand-rolled
// roundOffset operated on the float coordinates directly, so its corners
// settled at a different radius than pyclipper's on slightly-skewed real text
// boxes (~1.4px pre-scale; ~3-4px at source). Porting the actual integer
// algorithm bit-reproduces pyclipper.
//
// Scope note: the Clipper boolean union "clean-up" that Execute runs after
// DoOffset is omitted. For a simple convex quad expanded outward by a small
// positive delta the offset path is already a simple (non-self-intersecting)
// polygon, and the union returns it unchanged. The final minAreaRect +
// getMiniBoxes normalization is invariant to any vertex reordering the union
// might do, so returning the offset path directly matches pyclipper's
// Execute(solution, delta)[0].

import "math"

// --- Clipper1 constants (clipper.cpp) ---
var (
	clipperPi        = 3.141592653589793238
	clipperTwoPi     = clipperPi * 2
	clipperDefArcTol = 0.25
)

// cInt mirrors Clipper1's signed 64-bit coordinate type.
type cInt = int64

type cIntPt struct{ X, Y cInt }

type dPoint struct{ X, Y float64 }

// clipperRound mirrors Clipper1's inline Round: round-half-away-from-zero.
// Go's math.Round has identical semantics (2.5->3, -2.5->-3), so the int64
// cast after it reproduces `static_cast<cInt>(val ± 0.5)`.
func clipperRound(v float64) cInt {
	return cInt(math.Round(v))
}

// getUnitNormal mirrors Clipper1 GetUnitNormal: the LEFT unit normal of the
// directed edge p1->p2.
func getUnitNormal(p1, p2 cIntPt) dPoint {
	if p2.X == p1.X && p2.Y == p1.Y {
		return dPoint{0, 0}
	}
	dx := float64(p2.X - p1.X)
	dy := float64(p2.Y - p1.Y)
	f := 1.0 / math.Sqrt(dx*dx+dy*dy)
	dx *= f
	dy *= f
	return dPoint{dy, -dx}
}

// clipperArea mirrors Clipper1 Area (shoelace, sign convention as in C++).
func clipperArea(poly []cIntPt) float64 {
	n := len(poly)
	if n < 3 {
		return 0
	}
	var a float64
	for i, j := 0, n-1; i < n; i++ {
		a += float64(poly[j].X+poly[i].X) * float64(poly[j].Y-poly[i].Y)
		j = i
	}
	return -a * 0.5
}

func clipperOrientation(poly []cIntPt) bool { return clipperArea(poly) >= 0 }

func reversePath(p *[]cIntPt) {
	for i, j := 0, len(*p)-1; i < j; i, j = i+1, j-1 {
		(*p)[i], (*p)[j] = (*p)[j], (*p)[i]
	}
}

// dedupClosed drops consecutive duplicate vertices (including the wrap-around
// duplicate) the way ClipperOffset::AddPath does for a closed path.
func dedupClosed(in []cIntPt) []cIntPt {
	n := len(in)
	for n > 1 && in[0] == in[n-1] {
		in = in[:n-1]
		n--
	}
	out := in[:0]
	last := cIntPt{1 << 62, 1 << 62} // sentinel never equal to a real point
	for _, p := range in {
		if p != last {
			out = append(out, p)
			last = p
		}
	}
	return out
}

// clipperOffsetState holds the per-call mutable state of ClipperOffset.
type clipperOffsetState struct {
	src         []cIntPt
	normals     []dPoint
	delta       float64
	msin, mcos  float64
	stepsPerRad float64
	sinA        float64
}

// clipperOffset mirrors DBNet db_postprocess.unclip: delta = poly.area * ratio
// / poly.length is computed from the FLOAT box (Shapely/Polygon semantics), and
// the offset is run on the FLOAT box truncated to int64 — exactly what pyclipper
// does internally (IntPoint cInt cast). Returns the expanded polygon as int64
// coordinates, matching pyclipper's integer output.
func clipperOffset(box [4]pt, ratio float64) []pt {
	area := math.Abs(polygonArea(box[:]))
	perim := polygonPerimeter(box[:])
	if perim == 0 {
		return box[:]
	}
	delta := area * ratio / perim

	// Truncate float coords toward zero to int64 — pyclipper's IntPoint cast.
	src := make([]cIntPt, 4)
	for i := range box {
		src[i] = cIntPt{cInt(math.Trunc(box[i].X)), cInt(math.Trunc(box[i].Y))}
	}
	offset := clipperDoOffset(src, delta)
	out := make([]pt, len(offset))
	for i := range offset {
		out[i] = pt{X: float64(offset[i].X), Y: float64(offset[i].Y)}
	}
	return out
}

// clipperDoOffset mirrors ClipperOffset::DoOffset for a single
// ET_CLOSEDPOLYGON path with JT_ROUND.
func clipperDoOffset(src []cIntPt, delta float64) []cIntPt {
	contour := dedupClosed(src)
	if len(contour) < 3 {
		return src
	}
	// FixOrientations: for a single closed polygon, reverse it if its area is
	// negative so that a positive delta expands outward.
	if !clipperOrientation(contour) {
		reversePath(&contour)
	}

	n := len(contour)
	normals := make([]dPoint, n)
	for j := 0; j < n-1; j++ {
		normals[j] = getUnitNormal(contour[j], contour[j+1])
	}
	normals[n-1] = getUnitNormal(contour[n-1], contour[0])

	st := &clipperOffsetState{src: contour, normals: normals, delta: delta}

	// Arc step count (offset_triginometry2.svg in the Clipper docs).
	y := clipperDefArcTol
	if y > math.Abs(delta)*clipperDefArcTol {
		y = math.Abs(delta) * clipperDefArcTol
	}
	steps := clipperPi / math.Acos(1-y/math.Abs(delta))
	if steps > math.Abs(delta)*clipperPi {
		steps = math.Abs(delta) * clipperPi
	}
	st.msin = math.Sin(clipperTwoPi / steps)
	st.mcos = math.Cos(clipperTwoPi / steps)
	st.stepsPerRad = steps / clipperTwoPi
	if delta < 0 {
		st.msin = -st.msin
	}

	dest := make([]cIntPt, 0, n*8)
	k := n - 1
	for j := 0; j < n; j++ {
		st.offsetPoint(&dest, j, &k)
	}
	// Clipper1's Execute runs a union cleanup that (a) removes consecutive
	// duplicate vertices and (b) removes collinear vertices (CleanPolygons).
	// DoOffset can emit the same point twice where edge normals coincide, and
	// the trailing edge-normal vertex at an axis-aligned corner sits exactly
	// on the straight offset edge. Stripping both makes the returned path
	// match pyclipper vertex-for-vertex.
	return cleanCollinear(dedupClosed(dest))
}

// cleanCollinear drops vertices that are collinear with their neighbours on
// the closed polygon (cross product of the two incident edges == 0). This
// mirrors Clipper1's CleanPolygons, which removes the trailing edge-normal
// vertex at an axis-aligned corner (it lies on the straight offset edge)
// while keeping it on a skewed corner. Removing a collinear vertex does not
// change the polygon's minAreaRect.
func cleanCollinear(in []cIntPt) []cIntPt {
	out := in
	for {
		n := len(out)
		if n < 3 {
			return out
		}
		kept := make([]cIntPt, 0, n)
		removed := false
		for i := 0; i < n; i++ {
			prev := out[(i-1+n)%n]
			cur := out[i]
			next := out[(i+1)%n]
			cx := (cur.X-prev.X)*(next.Y-cur.Y) - (cur.Y-prev.Y)*(next.X-cur.X)
			if cx == 0 {
				removed = true
				continue
			}
			kept = append(kept, cur)
		}
		if !removed {
			return out
		}
		out = kept
	}
}

// offsetPoint mirrors ClipperOffset::OffsetPoint (JT_ROUND branch). The
// non-standard "<1px turn" short-circuit that the earlier port had is gone:
// every convex corner runs DoRound exactly as Clipper1 does, which is what
// makes Go's integer offset polygon match pyclipper vertex-for-vertex.
func (st *clipperOffsetState) offsetPoint(dest *[]cIntPt, j int, k *int) {
	n := st.normals
	st.sinA = n[*k].X*n[j].Y - n[j].X*n[*k].Y
	if st.sinA > 1.0 {
		st.sinA = 1.0
	} else if st.sinA < -1.0 {
		st.sinA = -1.0
	}

	if st.sinA*st.delta < 0 {
		// reflex corner: insert the original vertex between the two edge
		// offsets.
		*dest = append(*dest,
			cIntPt{clipperRound(float64(st.src[j].X) + n[*k].X*st.delta),
				clipperRound(float64(st.src[j].Y) + n[*k].Y*st.delta)})
		*dest = append(*dest, st.src[j])
		*dest = append(*dest,
			cIntPt{clipperRound(float64(st.src[j].X) + n[j].X*st.delta),
				clipperRound(float64(st.src[j].Y) + n[j].Y*st.delta)})
	} else {
		st.doRound(dest, j, *k)
	}
	*k = j
}

// doRound mirrors ClipperOffset::DoRound (JT_ROUND): for each convex corner it
// emits the incoming edge normal (normals[k]), then rotates the normal one arc
// step at a time emitting a vertex per step, and finally emits the outgoing
// edge normal (normals[j]). The arc step count uses round-half-away (same as
// Clipper1's Round), which makes the per-corner arc sampling match pyclipper
// and keeps the enclosing minAreaRect exact. The trailing normals[j] vertex is
// kept here; it is later dropped by cleanCollinear when it sits exactly on the
// straight offset edge (as Clipper1's CleanPolygons does for axis-aligned
// corners), so the returned path matches pyclipper vertex-for-vertex.
func (st *clipperOffsetState) doRound(dest *[]cIntPt, j, k int) {
	n := st.normals
	cosA := n[k].X*n[j].X + n[k].Y*n[j].Y
	a := math.Atan2(st.sinA, cosA)
	steps := int(math.Max(float64(clipperRound(st.stepsPerRad*math.Abs(a))), 1))

	X := n[k].X
	Y := n[k].Y
	for i := 0; i < steps; i++ {
		*dest = append(*dest, cIntPt{
			clipperRound(float64(st.src[j].X) + X*st.delta),
			clipperRound(float64(st.src[j].Y) + Y*st.delta),
		})
		X2 := X
		X = X*st.mcos - st.msin*Y
		Y = X2*st.msin + Y*st.mcos
	}
	*dest = append(*dest, cIntPt{
		clipperRound(float64(st.src[j].X) + n[j].X*st.delta),
		clipperRound(float64(st.src[j].Y) + n[j].Y*st.delta),
	})
}
