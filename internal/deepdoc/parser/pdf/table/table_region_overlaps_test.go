package table

import (
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ---- regionOverlapsBox ----

func TestRegionOverlapsBox_StrongOverlap(t *testing.T) {
	region := pdf.DLARegion{X0: 0, Y0: 0, X1: 216, Y1: 108} // DLA coords at 216 DPI
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 50}
	if !regionOverlapsBox(region, box, 3.0) {
		t.Error("full overlap should match")
	}
}

func TestRegionOverlapsBox_NoOverlap(t *testing.T) {
	region := pdf.DLARegion{X0: 0, Y0: 0, X1: 216, Y1: 108}
	box := pdf.TextBox{X0: 500, X1: 600, Top: 500, Bottom: 550}
	if regionOverlapsBox(region, box, 3.0) {
		t.Error("no overlap should return false")
	}
}

func TestRegionOverlapsBox_WeakOverlap(t *testing.T) {
	// Overlap at 30% → below 40% threshold → false.
	region := pdf.DLARegion{X0: 0, Y0: 0, X1: 90, Y1: 90}   // 30x30 at scale 3
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 100} // overlap = 30*30/10000 = 9%? No: 30x30=900 / 10000 = 9%
	if regionOverlapsBox(region, box, 3.0) {
		t.Error("9% overlap should return false")
	}
	// Overlap ≥ 40% → should match (Python thr=0.4).
	// box 100x100=10000 area; region 100x40=4000 → exactly 40%.
	region2 := pdf.DLARegion{X0: 0, Y0: 0, X1: 300, Y1: 120, Label: "table"} // 100x40 at scale 3
	if !regionOverlapsBox(region2, box, 3.0) {
		t.Error("40% overlap should match (>= 0.4)")
	}
	// Region that covers most of the box → should match
	region3 := pdf.DLARegion{X0: 0, Y0: 0, X1: 270, Y1: 270} // 90x90 at scale 3
	if !regionOverlapsBox(region3, box, 3.0) {
		t.Error("81% overlap should match")
	}
}

func TestRegionOverlapsBox_ThresholdAt040(t *testing.T) {
	// Exact 40% overlap: 100x100 box, region just covering 40%
	// 0.4 * 10000 = 4000. Need region with area 4000 in box space.
	// 63.2*63.2 ≈ 3994. Let's use 100x40 = 4000.
	box := pdf.TextBox{X0: 0, X1: 100, Top: 0, Bottom: 100}
	region := pdf.DLARegion{X0: 0, Y0: 0, X1: 300, Y1: 120, Label: "table"} // 100x40 at scale 3
	if !regionOverlapsBox(region, box, 3.0) {
		t.Error("exact 40% overlap should match (>= 0.4)")
	}
	// 39% overlap should NOT match
	region2 := pdf.DLARegion{X0: 0, Y0: 0, X1: 300, Y1: 117, Label: "table"} // 100x39 at scale 3
	if regionOverlapsBox(region2, box, 3.0) {
		t.Error("39% overlap should NOT match")
	}
}
