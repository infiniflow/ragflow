//go:build cgo

package native

import (
	"encoding/json"
	"math"
	"os"
	"testing"
)

// TestPreUnclipMatchesDeepdoc isolates the pre-unclip stage: given deepdoc's
// raw cv2.findContours points, does the Go convexHull+minAreaRect+getMiniBoxes
// reproduce deepdoc's pre-unclip box? Also checks raw (no hull) to detect any
// minAreaRect discrepancy vs cv2.
func TestPreUnclipMatchesDeepdoc(t *testing.T) {
	raw, err := os.ReadFile("testdata/contours.json")
	if err != nil {
		t.Fatalf("contours fixture not found (run with GEN_CONTOURS=1 to regenerate): %v", err)
	}
	var data struct {
		Contours []struct {
			Contour [][]float64 `json:"contour"`
			PreBox  [][]float64 `json:"pre_box"`
		} `json:"contours"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatalf("parse: %v", err)
	}

	var maxRaw, maxHull float64
	for i, c := range data.Contours {
		cpts := make([]pt, len(c.Contour))
		for j := range c.Contour {
			cpts[j] = pt{X: c.Contour[j][0], Y: c.Contour[j][1]}
		}
		want := [4]pt{}
		for j := 0; j < 4; j++ {
			want[j] = pt{X: c.PreBox[j][0], Y: c.PreBox[j][1]}
		}

		// raw path: minAreaRect on the raw contour (what cv2 does).
		rRaw, _ := minAreaRect(cpts)
		mbRaw := getMiniBoxes(rRaw)
		// hull path: what the Go dbPostProcess does.
		rHull, _ := minAreaRect(convexHull(cpts))
		mbHull := getMiniBoxes(rHull)

		dr := 0.0
		for j := 0; j < 4; j++ {
			dr = math.Max(dr, math.Max(math.Abs(mbRaw[j].X-want[j].X), math.Abs(mbRaw[j].Y-want[j].Y)))
		}
		dh := 0.0
		for j := 0; j < 4; j++ {
			dh = math.Max(dh, math.Max(math.Abs(mbHull[j].X-want[j].X), math.Abs(mbHull[j].Y-want[j].Y)))
		}
		if dr > maxRaw {
			maxRaw = dr
		}
		if dh > maxHull {
			maxHull = dh
		}
		// The Go dbPostProcess always convexifies before minAreaRect, so the
		// hull path is what the pipeline uses. cv2.minAreaRect on the raw
		// (non-convex) contour agrees with the hull result, but our pure-Go
		// rotating-calipers does not (it is only validated against the hull);
		// that raw divergence is expected and not exercised by the pipeline.
		if dh > 1.0 {
			t.Logf("contour %d: hullDiff=%.3f  want=%v hullMB=%v",
				i, dh, want, mbHull)
		}
	}
	t.Logf("max pre-unclip diff vs deepdoc: raw(no hull)=%.3f px (informational), hull=%.3f px (pipeline path)",
		maxRaw, maxHull)
	if maxHull > 1.0 {
		t.Errorf("hull minAreaRect diverges from deepdoc by %.3f px", maxHull)
	}
}
