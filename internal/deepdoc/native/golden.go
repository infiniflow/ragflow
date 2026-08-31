//go:build cgo

package native

// Shared golden-loading and box-comparison helpers for the equivalence tests.
//
// These used to live (unexported) inside native_integration_test.go. They are
// extracted here so the SAME comparison logic is reused by:
//   - the native integration tests (package native), and
//   - the in-process DeepDoc backend tests (package infnative), which prove the
//     NativeAnalyzer DocAnalyzer seam is functionally equivalent to the Python
//     deepdoc service using the very same Python-reference goldens.
//
// Keeping one implementation avoids two diverging copies of the matching math.
// These are pure comparison helpers with no runtime model dependency, so the
// file is gated by `cgo` only (not `integration`): the manual-tier
// raster-alignment tests reuse them without pulling in the integration tag.

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

const (
	// CoordFloor is the documented hard accuracy floor (px) of the comparison
	// tool: det stabilizes at ~3px from bilinearResize + box#8 postprocess,
	// format-independent. DLA/TSR are tighter, but tolerances are sized above
	// this worst case so any regression past the floor trips the gate instead
	// of hiding under it.
	CoordFloor = 3.0
	// CoordTolMargin lifts the coordinate tolerance just above CoordFloor.
	CoordTolMargin = 0.5

	// CmpTolCoord is the coordinate tolerance (px) used for golden comparisons.
	CmpTolCoord = CoordFloor + CoordTolMargin // 3.5
	// CmpTolScore is the tolerance on detection scores.
	CmpTolScore = 0.05
)

// LoadGoldenBoxes reads a golden JSON file produced by the Python reference
// scripts (ref_dla.py / ref_tsr.py / ref_det.py). DLA/TSR goldens use the Go
// DocAnalyzer wire shape: {"bboxes": [[x0,y0,x1,y1,score,class], ...]}.
func LoadGoldenBoxes(tb testing.TB, path string) [][]float64 {
	tb.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read golden %s: %v", path, err)
	}
	var wrap struct {
		Bboxes [][]float64 `json:"bboxes"`
	}
	if err := json.Unmarshal(raw, &wrap); err != nil {
		tb.Fatalf("parse golden %s: %v", path, err)
	}
	return wrap.Bboxes
}

// CompareBoxes matches every golden box to a Go box of the same class by
// nearest center and fails the test on any per-coordinate difference beyond
// CmpTolCoord (or score difference beyond CmpTolScore).
func CompareBoxes(tb testing.TB, gold, got [][]float64) {
	tb.Helper()
	if len(gold) == 0 {
		tb.Fatalf("golden has no boxes")
	}
	used := make([]bool, len(got))
	maxd := 0.0
	matched := 0
	for _, gb := range gold {
		cls := int(gb[5])
		bcx, bcy := (gb[0]+gb[2])/2, (gb[1]+gb[3])/2
		best, bd := -1, math.MaxFloat64
		for i, vb := range got {
			if used[i] || int(vb[5]) != cls {
				continue
			}
			vcx, vcy := (vb[0]+vb[2])/2, (vb[1]+vb[3])/2
			d := (bcx-vcx)*(bcx-vcx) + (bcy-vcy)*(bcy-vcy)
			if d < bd {
				bd, best = d, i
			}
		}
		if best < 0 {
			tb.Errorf("no Go box matched golden class %d at (%.0f,%.0f)", cls, bcx, bcy)
			continue
		}
		used[best] = true
		matched++
		for j := 0; j < 6; j++ {
			tol := CmpTolCoord
			if j == 4 {
				tol = CmpTolScore
			}
			if math.Abs(gb[j]-got[best][j]) > tol {
				tb.Errorf("class %d coord %d diff %.3f > tol %.2f (gold=%v got=%v)",
					cls, j, math.Abs(gb[j]-got[best][j]), tol, gb, got[best])
			}
			if j != 4 {
				maxd = math.Max(maxd, math.Abs(gb[j]-got[best][j]))
			}
		}
	}
	tb.Logf("matched %d/%d golden boxes, max coord diff %.4f px", matched, len(gold), maxd)
}

// MatchBoxesRelaxed returns (matched count, max coordinate diff among matches,
// unmatched goldens) using caller-supplied tolerances. Unlike CompareBoxes it
// does NOT fail the test — callers decide what a match/mismatch means. A golden
// box counts as matched only if its nearest same-class Go box is within
// coordTol (on any coordinate) and scoreTol; otherwise it is returned as
// unmatched. Used by the extreme-aspect boundary test and by the analyzer
// golden tests, whose tolerances are deliberately wider than the real-table
// parity floor.
func MatchBoxesRelaxed(tb testing.TB, gold, got [][]float64, coordTol, scoreTol float64) (matched int, maxd float64, unmatched [][]float64) {
	tb.Helper()
	used := make([]bool, len(got))
	for _, gb := range gold {
		cls := int(gb[5])
		bcx, bcy := (gb[0]+gb[2])/2, (gb[1]+gb[3])/2
		best, bd := -1, math.MaxFloat64
		for i, vb := range got {
			if used[i] || int(vb[5]) != cls {
				continue
			}
			vcx, vcy := (vb[0]+vb[2])/2, (vb[1]+vb[3])/2
			d := (bcx-vcx)*(bcx-vcx) + (bcy-vcy)*(bcy-vcy)
			if d < bd {
				bd, best = d, i
			}
		}
		if best < 0 {
			unmatched = append(unmatched, gb)
			continue
		}
		// Enforce the relaxed tolerance: if even the nearest same-class box is
		// farther than the tolerance, treat it as unmatched (structural miss).
		coordDiff, scoreDiff := 0.0, math.Abs(gb[4]-got[best][4])
		for j := 0; j < 4; j++ {
			coordDiff = math.Max(coordDiff, math.Abs(gb[j]-got[best][j]))
		}
		if coordDiff > coordTol || scoreDiff > scoreTol {
			unmatched = append(unmatched, gb)
			continue
		}
		used[best] = true
		matched++
		maxd = math.Max(maxd, coordDiff)
	}
	return matched, maxd, unmatched
}

// FlattenQuads collapses a det Wire()/golden output payload to its box list.
// Both nest quads under output[0][0].
func FlattenQuads(out [][][][][2]float64) [][][2]float64 {
	if len(out) == 0 || len(out[0]) == 0 {
		return nil
	}
	return out[0][0]
}

// MatchBothDirections matches two quad sets by nearest center within tol (px),
// in BOTH directions. It returns the number of golden boxes that found a Go
// match, the number of Go boxes that found a golden match, and the worst
// per-corner coordinate difference observed among matched pairs.
func MatchBothDirections(gold, got [][][2]float64, tol float64) (matchedGold, matchedGo int, maxd float64) {
	sq := func(x float64) float64 { return x * x }
	// golden -> Go
	usedGo := make([]bool, len(got))
	for _, gb := range gold {
		gcx, gcy := quadCenter(gb)
		best, bd := -1, math.MaxFloat64
		for i, vb := range got {
			if usedGo[i] {
				continue
			}
			vcx, vcy := quadCenter(vb)
			d := sq(gcx-vcx) + sq(gcy-vcy)
			if d < bd {
				bd, best = d, i
			}
		}
		if best < 0 || math.Sqrt(bd) > tol {
			continue
		}
		usedGo[best] = true
		matchedGold++
		for j := 0; j < 4; j++ {
			for k := 0; k < 2; k++ {
				if d := math.Abs(gb[j][k] - got[best][j][k]); d > maxd {
					maxd = d
				}
			}
		}
	}
	// Go -> golden (reverse), to surface Go boxes with no golden counterpart.
	usedGold := make([]bool, len(gold))
	for _, vb := range got {
		vcx, vcy := quadCenter(vb)
		best, bd := -1, math.MaxFloat64
		for i, gb := range gold {
			if usedGold[i] {
				continue
			}
			gcx, gcy := quadCenter(gb)
			d := sq(gcx-vcx) + sq(gcy-vcy)
			if d < bd {
				bd, best = d, i
			}
		}
		if best < 0 || math.Sqrt(bd) > tol {
			continue
		}
		usedGold[best] = true
		matchedGo++
		for j := 0; j < 4; j++ {
			for k := 0; k < 2; k++ {
				if d := math.Abs(gold[best][j][k] - vb[j][k]); d > maxd {
					maxd = d
				}
			}
		}
	}
	return matchedGold, matchedGo, maxd
}

// quadAABB returns the axis-aligned bounding box of a quad.
func quadAABB(q [][2]float64) (x0, y0, x1, y1 float64) {
	x0, y0, x1, y1 = q[0][0], q[0][1], q[0][0], q[0][1]
	for _, p := range q {
		if p[0] < x0 {
			x0 = p[0]
		}
		if p[1] < y0 {
			y0 = p[1]
		}
		if p[0] > x1 {
			x1 = p[0]
		}
		if p[1] > y1 {
			y1 = p[1]
		}
	}
	return
}

// iou returns the intersection-over-union of two quads' AABBs.
func iou(a, b [][2]float64) float64 {
	ax0, ay0, ax1, ay1 := quadAABB(a)
	bx0, by0, bx1, by1 := quadAABB(b)
	ix0, iy0 := math.Max(ax0, bx0), math.Max(ay0, by0)
	ix1, iy1 := math.Min(ax1, bx1), math.Min(ay1, by1)
	iw, ih := ix1-ix0, iy1-iy0
	if iw <= 0 || ih <= 0 {
		return 0
	}
	inter := iw * ih
	areaA := (ax1 - ax0) * (ay1 - ay0)
	areaB := (bx1 - bx0) * (by1 - by0)
	return inter / (areaA + areaB - inter)
}

// MatchIoUBothDirections matches two quad sets by greedy best-IoU in BOTH
// directions. A pair matches only if IoU >= thr. This isolates true
// box-membership divergence (one box split into two, two merged into one,
// spurious detections) from mere coordinate drift: a box shifted 20px but
// still overlapping its twin scores high IoU and is NOT an orphan.
func MatchIoUBothDirections(gold, got [][][2]float64, thr float64) (matchedGold, matchedGo int) {
	usedGo := make([]bool, len(got))
	for _, gb := range gold {
		best, bestI := -1, 0.0
		for i, vb := range got {
			if usedGo[i] {
				continue
			}
			if v := iou(gb, vb); v > bestI {
				bestI, best = v, i
			}
		}
		if best >= 0 && bestI >= thr {
			usedGo[best] = true
			matchedGold++
		}
	}
	usedGold := make([]bool, len(gold))
	for _, vb := range got {
		best, bestI := -1, 0.0
		for i, gb := range gold {
			if usedGold[i] {
				continue
			}
			if v := iou(gb, vb); v > bestI {
				bestI, best = v, i
			}
		}
		if best >= 0 && bestI >= thr {
			usedGold[best] = true
			matchedGo++
		}
	}
	return matchedGold, matchedGo
}

// quadCenter returns the centroid of a quad.
func quadCenter(q [][2]float64) (float64, float64) {
	var sx, sy float64
	for _, p := range q {
		sx += p[0]
		sy += p[1]
	}
	return sx / float64(len(q)), sy / float64(len(q))
}
