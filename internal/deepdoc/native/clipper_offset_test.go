//go:build cgo

package native

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// matchPolygon reports the max distance between the two polygons when matched
// as unordered point sets (Clipper's Execute may rotate the starting vertex and
// the union may drop interior arc points, so we compare geometrically, not by
// index).
func matchPolygon(got []pt, want [][]int64) float64 {
	maxd := 0.0
	used := make([]bool, len(want))
	for _, g := range got {
		best := 1e18
		for i, w := range want {
			if used[i] {
				continue
			}
			d := math.Hypot(g.X-float64(w[0]), g.Y-float64(w[1]))
			if d < best {
				best = d
			}
		}
		if best > maxd {
			maxd = best
		}
	}
	// also ensure every want point is covered by some got point
	for _, w := range want {
		best := 1e18
		for _, g := range got {
			d := math.Hypot(g.X-float64(w[0]), g.Y-float64(w[1]))
			if d < best {
				best = d
			}
		}
		if best > maxd {
			maxd = best
		}
	}
	return maxd
}

// TestClipperOffsetMatchesPyclipper validates the pure-Go Clipper1 port against
// pyclipper (the deepdoc oracle) box-by-box on real pre-unclip quads captured
// from deepdoc's TextDetector on page0.jpg:
//  1. the offset polygon matches pyclipper's Execute output geometrically, and
//  2. minAreaRect(getMiniBoxes(offset)) matches deepdoc's pre-scale quad.
func TestClipperOffsetMatchesPyclipper(t *testing.T) {
	raw, err := os.ReadFile("testdata/clipper_quads4.json")
	if err != nil {
		t.Skipf("fresh pyclipper oracle not found: %v", err)
	}
	var data struct {
		Quads []struct {
			Box      [][]float64 `json:"box"`
			Poly     [][][]int64 `json:"poly"`
			Distance float64     `json:"distance"`
			PreScale [][]float64 `json:"pre_scale"`
		} `json:"quads"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse fixture: %v", err)
	}

	var maxPoly, maxRect float64
	var dbg []struct {
		Box  [4][2]float64 `json:"box"`
		Got  [][2]float64  `json:"got"`
		Want [][]int64     `json:"want"`
	}
	for qi, q := range data.Quads {
		if len(q.Box) != 4 {
			t.Fatalf("quad %d: expected 4 points, got %d", qi, len(q.Box))
		}
		var box [4]pt
		for i := range q.Box {
			box[i] = pt{X: q.Box[i][0], Y: q.Box[i][1]}
		}
		got := clipperOffset(box, detUnclipRatio)

		if os.Getenv("DUMP_CLIPPER") != "" {
			dbg = append(dbg, struct {
				Box  [4][2]float64 `json:"box"`
				Got  [][2]float64  `json:"got"`
				Want [][]int64     `json:"want"`
			}{
				Box: [4][2]float64{{box[0].X, box[0].Y}, {box[1].X, box[1].Y}, {box[2].X, box[2].Y}, {box[3].X, box[3].Y}},
				Got: func() [][2]float64 {
					p := make([][2]float64, len(got))
					for i := range got {
						p[i] = [2]float64{got[i].X, got[i].Y}
					}
					return p
				}(),
				Want: q.Poly[0],
			})
		}

		// (1) offset polygon vs pyclipper output (as a point set). The
		// faithful Clipper1 port must reproduce pyclipper's integer polygon
		// vertex-for-vertex, so the residual is sub-pixel.
		if len(q.Poly) > 0 {
			d := matchPolygon(got, q.Poly[0])
			if d > maxPoly {
				maxPoly = d
			}
			if d > 1.0 {
				t.Errorf("quad %d: offset polygon diverges from pyclipper by %.2f px (n_got=%d n_want=%d)",
					qi, d, len(got), len(q.Poly[0]))
			}
		}

		// (2) minAreaRect + getMiniBoxes vs deepdoc pre-scale quad (the
		// post-unclip quad, in resized coords). Must match sub-pixel.
		if len(q.PreScale) == 4 {
			rect, _ := minAreaRect(got)
			mb := getMiniBoxes(rect)
			for i := 0; i < 4; i++ {
				dx := math.Abs(mb[i].X - q.PreScale[i][0])
				dy := math.Abs(mb[i].Y - q.PreScale[i][1])
				d := math.Max(dx, dy)
				if d > maxRect {
					maxRect = d
				}
				if d > 0.75 {
					t.Errorf("quad %d pt %d: rect got (%.2f,%.2f) want (%.2f,%.2f) diff=%.3f",
						qi, i, mb[i].X, mb[i].Y, q.PreScale[i][0], q.PreScale[i][1], d)
				}
			}
		}
	}
	t.Logf("compared %d quads: max offset-polygon diff = %.3f px, max pre-scale rect diff = %.3f px",
		len(data.Quads), maxPoly, maxRect)
	if os.Getenv("DUMP_CLIPPER") != "" {
		_ = json.NewEncoder(os.Stdout).Encode(dbg) // also write to file below
		if b, err := json.Marshal(dbg); err == nil {
			_ = os.WriteFile("/tmp/go_clipper_out.json", b, 0o644)
		}
	}
}

// TestTuneArcTol sweeps clipperDefArcTol to find the value whose Clipper1 port
// best reproduces pyclipper's integer offset polygon (and therefore the
// post-unclip rect) on the 15 real pre-unclip quads. Run:
//
//	go test ./native/ -run TestTuneArcTol -v
func TestTuneArcTol(t *testing.T) {
	raw, err := os.ReadFile("testdata/clipper_quads4.json")
	if err != nil {
		t.Skipf("oracle not found: %v", err)
	}
	var data struct {
		Quads []struct {
			Box      [][]float64 `json:"box"`
			Poly     [][][]int64 `json:"poly"`
			PreScale [][]float64 `json:"pre_scale"`
		} `json:"quads"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, tol := range []float64{0.15, 0.17, 0.18, 0.20, 0.22, 0.25} {
		clipperDefArcTol = tol
		var maxPoly, maxRect float64
		for _, q := range data.Quads {
			var box [4]pt
			for i := range q.Box {
				box[i] = pt{X: q.Box[i][0], Y: q.Box[i][1]}
			}
			got := clipperOffset(box, detUnclipRatio)
			if len(q.Poly) > 0 {
				d := matchPolygon(got, q.Poly[0])
				if d > maxPoly {
					maxPoly = d
				}
			}
			if len(q.PreScale) == 4 {
				rect, _ := minAreaRect(got)
				mb := getMiniBoxes(rect)
				for i := 0; i < 4; i++ {
					d := math.Max(math.Abs(mb[i].X-q.PreScale[i][0]), math.Abs(mb[i].Y-q.PreScale[i][1]))
					if d > maxRect {
						maxRect = d
					}
				}
			}
		}
		t.Logf("arcTol=%.2f  maxPoly=%.3f  maxRect=%.3f", tol, maxPoly, maxRect)
	}
	// Isolate minAreaRect: feed pyclipper's EXACT polygon to Go's minAreaRect
	// and compare to the oracle pre_scale. If this is ~0, the re-rect is exact
	// and the residual lives in clipperOffset; if ~1px, minAreaRect diverges
	// on expanded polygons.
	var maxRectOnPy float64
	for _, q := range data.Quads {
		if len(q.PreScale) != 4 || len(q.Poly) == 0 {
			continue
		}
		var poly []pt
		for _, p := range q.Poly[0] {
			poly = append(poly, pt{X: float64(p[0]), Y: float64(p[1])})
		}
		rect, _ := minAreaRect(poly)
		mb := getMiniBoxes(rect)
		for i := 0; i < 4; i++ {
			d := math.Max(math.Abs(mb[i].X-q.PreScale[i][0]), math.Abs(mb[i].Y-q.PreScale[i][1]))
			if d > maxRectOnPy {
				maxRectOnPy = d
			}
		}
	}
	t.Logf("minAreaRect(EXACT py polygon) vs pre_scale: maxRect=%.4f", maxRectOnPy)
}

// TestDebugRotatedSquare dumps Go's clipperOffset for a single rotated square
// to /tmp/go_rotsq.json so it can be diffed vertex-by-vertex against pyclipper.
func TestDebugRotatedSquare(t *testing.T) {
	box := [4]pt{{100, 200}, {300, 190}, {310, 210}, {110, 220}} // rotated square
	got := clipperOffset(box, 1.5)
	out := make([][2]float64, len(got))
	for i := range got {
		out[i] = [2]float64{got[i].X, got[i].Y}
	}
	if b, err := json.Marshal(map[string]any{"box": box, "got": out}); err == nil {
		_ = os.WriteFile("/tmp/go_rotsq.json", b, 0o644)
	}
	t.Logf("wrote /tmp/go_rotsq.json with %d vertices", len(got))
}
