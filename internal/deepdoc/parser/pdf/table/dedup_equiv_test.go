package table

import (
	"fmt"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ---- 3.4 AnnotateBoxLayouts: cands-by-label bucketing equivalence ----

// Table + text regions over the same boxes: priority order (table before
// text) and per-type LayoutNo indexing must be preserved after bucketing.
func annotateBucketFixture() ([]pdf.TextBox, []pdf.DLARegion) {
	boxes := []pdf.TextBox{
		{Text: "A", X0: 0, X1: 100, Top: 0, Bottom: 20},
		{Text: "B", X0: 200, X1: 300, Top: 0, Bottom: 20},
	}
	regions := []pdf.DLARegion{
		{X0: 0, Y0: 0, X1: 100, Y1: 20, Label: "text", Confidence: 0.99},
		{X0: 0, Y0: 0, X1: 100, Y1: 20, Label: "table", Confidence: 0.99},
		{X0: 200, Y0: 0, X1: 300, Y1: 20, Label: "text", Confidence: 0.99},
	}
	return boxes, regions
}

func TestAnnotateBoxLayoutsBucketEquivalence(t *testing.T) {
	boxes, regions := annotateBucketFixture()
	out := AnnotateBoxLayouts(boxes, regions, 1, 0)
	if len(out) != 2 {
		t.Fatalf("len: got %d want 2", len(out))
	}
	// box0: both "table" and "text" regions cover it; table has higher
	// priority and is checked first, so it must win even though both are full.
	if out[0].LayoutType != "table" {
		t.Errorf("box0 LayoutType: got %q want table", out[0].LayoutType)
	}
	// box1: only a "text" region.
	if out[1].LayoutType != "text" {
		t.Errorf("box1 LayoutType: got %q want text", out[1].LayoutType)
	}
}

func BenchmarkAnnotateBoxLayoutsBucketed(b *testing.B) {
	baseBoxes, baseRegions := annotateBucketFixture()
	var boxes []pdf.TextBox
	var regions []pdf.DLARegion
	for i := 0; i < 40; i++ {
		for _, bx := range baseBoxes {
			c := bx
			c.X0 += float64(i) * 400
			c.X1 += float64(i) * 400
			boxes = append(boxes, c)
		}
		for _, r := range baseRegions {
			c := r
			c.X0 += float64(i) * 400
			c.X1 += float64(i) * 400
			regions = append(regions, c)
		}
	}
	b.ReportAllocs()
	clean := append([]pdf.TextBox(nil), boxes...)
	for i := 0; i < b.N; i++ {
		// AnnotateBoxLayouts writes LayoutType/LayoutNo onto boxes; without a
		// reset, later iterations skip already-annotated boxes and never reach
		// the candidate scan the benchmark is meant to measure.
		copy(boxes, clean)
		AnnotateBoxLayouts(boxes, regions, 1, 0)
	}
}

// ---- 3.5 compressColIndices single-pass equivalence ----

// Dense-grid fixture: C labels 0..7 all appear in rank, so this exercises the
// row→column mapping but NOT a rank-missing C (which the old code skips).
func gridFixture() []pdf.TextBox {
	var boxes []pdf.TextBox
	for r := 0; r < 12; r++ {
		for c := 0; c < 8; c++ {
			boxes = append(boxes, pdf.TextBox{
				R: r, C: c, HasTableAnnotations: true,
				Text: fmt.Sprintf("r%dc%d", r, c),
				X0:   float64(c * 100), X1: float64(c*100 + 80),
				Top: float64(r * 30), Bottom: float64(r*30 + 20),
			})
		}
	}
	return boxes
}

func TestGroupBoxesByRCSinglePassEquivalence(t *testing.T) {
	rows := GroupBoxesByRC(gridFixture())
	if len(rows) != 12 {
		t.Fatalf("rows: got %d want 12", len(rows))
	}
	for r, row := range rows {
		if len(row) != 8 {
			t.Fatalf("row %d cols: got %d want 8", r, len(row))
		}
		for c, cell := range row {
			want := fmt.Sprintf("r%dc%d", r, c)
			if cell.Text != want {
				t.Fatalf("cell[%d][%d]: got %q want %q", r, c, cell.Text, want)
			}
		}
	}
}

func BenchmarkGroupBoxesByRCSinglePass(b *testing.B) {
	boxes := gridFixture()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		GroupBoxesByRC(boxes)
	}
}
