//go:build cgo

package native

import (
	"encoding/json"
	"math"
	"os"
	"sort"
	"testing"
)

// TestGenerateContoursFixture regenerates testdata/contours.json when
// GEN_CONTOURS=1 is set. It extracts real detection contours from binarized
// test images via the package's own findContours, then freezes each contour's
// hull-path pre-unclip box (convexHull + minAreaRect + getMiniBoxes) as the
// recorded "deepdoc" pre_box.
//
// TestPreUnclipMatchesDeepdoc replays the hull path on the same contours and
// asserts it stays within 1px of the frozen box — a regression guard for the
// pre-unclip geometry. Because the production path only ever uses the hull
// path, the fixture is a self-consistent Go baseline (not a separate deepdoc
// oracle); it still catches any future change to convexHull / minAreaRect /
// getMiniBoxes.
func TestGenerateContoursFixture(t *testing.T) {
	if os.Getenv("GEN_CONTOURS") == "" {
		t.Skip("set GEN_CONTOURS=1 to regenerate testdata/contours.json")
	}
	stems := []string{"page0", "line0", "line_cn", "deg_solid", "mp_cn_sm_p0"}
	type entry struct {
		Contour [][]float64 `json:"contour"`
		PreBox  [][]float64 `json:"pre_box"`
	}
	var out []entry
	for _, stem := range stems {
		img, err := Decode("testdata/" + stem + ".png")
		if err != nil {
			t.Fatalf("decode %s: %v", stem, err)
		}
		mask := binarizeImage(img)
		contours := findContours(mask, img.W, img.H, 1<<20)
		for _, c := range contours {
			if len(c) < 5 {
				continue
			}
			if a := math.Abs(contourArea(c)); a < 40 || a > 4000 {
				continue
			}
			rHull, _ := minAreaRect(convexHull(c))
			box := getMiniBoxes(rHull)
			ec := make([][]float64, len(c))
			for i, p := range c {
				ec[i] = []float64{p.X, p.Y}
			}
			eb := make([][]float64, 4)
			for i := 0; i < 4; i++ {
				eb[i] = []float64{box[i].X, box[i].Y}
			}
			out = append(out, entry{Contour: ec, PreBox: eb})
			if len(out) >= 120 {
				break
			}
		}
		if len(out) >= 120 {
			break
		}
	}
	if len(out) == 0 {
		t.Fatal("no contours extracted from any fixture image")
	}
	// Largest contours first so the fixture is stable and self-describing.
	sort.Slice(out, func(i, j int) bool { return len(out[i].Contour) > len(out[j].Contour) })
	data := struct {
		Contours []entry `json:"contours"`
	}{out}
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("testdata/contours.json", raw, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote %d contours to testdata/contours.json", len(out))
}

// binarizeImage converts an RGB image to a foreground mask (true = dark text
// on light background) using a fixed grayscale threshold.
func binarizeImage(img *Image) []bool {
	mask := make([]bool, img.W*img.H)
	for i := 0; i < img.W*img.H; i++ {
		r := float64(img.Pix[i*3])
		g := float64(img.Pix[i*3+1])
		b := float64(img.Pix[i*3+2])
		gray := 0.299*r + 0.587*g + 0.114*b
		mask[i] = gray < 128
	}
	return mask
}

// contourArea returns the absolute polygon area via the shoelace formula.
func contourArea(c []pt) float64 {
	n := len(c)
	if n < 3 {
		return 0
	}
	var a float64
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		a += c[i].X*c[j].Y - c[j].X*c[i].Y
	}
	return a / 2
}
