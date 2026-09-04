package pdf

import (
	"context"
	"image"
	"math"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// captureAnalyzer records the image handed to OCRRecognize so a test can
// assert which crop geometry was fed to recognition.
type captureAnalyzer struct {
	healthy  bool
	boxes    []pdf.OCRBox
	texts    []pdf.OCRText
	recImage image.Image
}

func (c *captureAnalyzer) Health() bool { return c.healthy }
func (c *captureAnalyzer) DLA(context.Context, image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (c *captureAnalyzer) TSR(context.Context, image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (c *captureAnalyzer) OCRDetect(context.Context, image.Image) ([]pdf.OCRBox, error) {
	return c.boxes, nil
}
func (c *captureAnalyzer) OCRRecognize(_ context.Context, img image.Image) ([]pdf.OCRText, error) {
	c.recImage = img
	return c.texts, nil
}

func sameImage(a, b *image.RGBA) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Bounds() != b.Bounds() {
		return false
	}
	for y := 0; y < a.Bounds().Dy(); y++ {
		for x := 0; x < a.Bounds().Dx(); x++ {
			if a.RGBAAt(x, y) != b.RGBAAt(x, y) {
				return false
			}
		}
	}
	return true
}

// TestOCRDetectAndRecognize_WarpsCrop locks that ocrDetectAndRecognize feeds
// the perspective-de-skewed (WarpCrop) crop to recognition, not the old
// axis-aligned FastCrop of the detection bbox. This is the live-path wiring
// of Step 4 / layer 1.
func TestOCRDetectAndRecognize_WarpsCrop(t *testing.T) {
	p := newTestParser()
	page := image.NewRGBA(image.Rect(0, 0, 200, 140))

	// A clearly skewed (perspective) quad: TL, TR, BR, BL. It is intentionally
	// wide (W=120, H=60 after the warp, h/w < 1.5) so layer 2 is a no-op and
	// recognition receives the de-skewed crop unchanged; a tall quad would be
	// rotated by ocrRecognizeWithRotation and break the equality below.
	quad := [4]util.Pt{{X: 50, Y: 40}, {X: 150, Y: 30}, {X: 160, Y: 90}, {X: 40, Y: 100}}
	box := pdf.OCRBox{
		X0: quad[0].X, Y0: quad[0].Y,
		X1: quad[1].X, Y1: quad[1].Y,
		X2: quad[2].X, Y2: quad[2].Y,
		X3: quad[3].X, Y3: quad[3].Y,
	}

	cap := &captureAnalyzer{
		healthy: true,
		boxes:   []pdf.OCRBox{box},
		texts:   []pdf.OCRText{{Text: "x", Confidence: 0.9}},
	}
	got := p.ocrDetectAndRecognize(t.Context(), page, cap, 0, "warp")
	if len(got) != 1 {
		t.Fatalf("expected 1 text box, got %d", len(got))
	}
	if cap.recImage == nil {
		t.Fatal("OCRRecognize was not called with a crop")
	}

	// The crop passed to recognition must be exactly the WarpCrop output.
	want := util.WarpCrop(page, quad)
	rec, ok := cap.recImage.(*image.RGBA)
	if !ok {
		t.Fatalf("rec crop is %T, want *image.RGBA", cap.recImage)
	}
	if !sameImage(rec, want) {
		t.Errorf("rec crop is not the WarpCrop output (size got=%v want=%v)",
			cap.recImage.Bounds(), want.Bounds())
	}

	// And it must NOT be the axis-aligned FastCrop of the detection bbox,
	// proving de-skew actually happened on the live path.
	minX := int(min4(quad[0].X, quad[1].X, quad[2].X, quad[3].X))
	minY := int(min4(quad[0].Y, quad[1].Y, quad[2].Y, quad[3].Y))
	maxX := int(max4(quad[0].X, quad[1].X, quad[2].X, quad[3].X))
	maxY := int(max4(quad[0].Y, quad[1].Y, quad[2].Y, quad[3].Y))
	bboxCrop := util.FastCrop(page, minX, minY, maxX, maxY)
	if sameImage(rec, bboxCrop) {
		t.Errorf("rec crop equals the axis-aligned bbox crop; warp was not applied")
	}

	// Safety property: the emitted TextBox must stay the axis-aligned
	// detection bbox — only the crop fed to recognition is de-skewed, the box
	// geometry itself is never transformed. If warp ever leaked into the
	// emitted coordinates this would drift from the original detection bbox.
	got0 := got[0]
	if math.Abs(got0.X0-float64(minX)/pdf.DlaScale) > 1e-6 ||
		math.Abs(got0.Top-float64(minY)/pdf.DlaScale) > 1e-6 ||
		math.Abs(got0.X1-float64(maxX)/pdf.DlaScale) > 1e-6 ||
		math.Abs(got0.Bottom-float64(maxY)/pdf.DlaScale) > 1e-6 {
		t.Errorf("emitted TextBox is not the axis-aligned detection bbox: got=(%.4f,%.4f,%.4f,%.4f) want=(%.4f,%.4f,%.4f,%.4f)",
			got0.X0, got0.Top, got0.X1, got0.Bottom,
			float64(minX)/pdf.DlaScale, float64(minY)/pdf.DlaScale,
			float64(maxX)/pdf.DlaScale, float64(maxY)/pdf.DlaScale)
	}
}

func min4(a, b, c, d float64) float64 {
	m := a
	if b < m {
		m = b
	}
	if c < m {
		m = c
	}
	if d < m {
		m = d
	}
	return m
}

// TestOCRMergeChars_WarpsEmptyBoxCrop locks that the char-merge path
// (buildTextBoxes) also de-skews the boxes it re-recognizes, matching the
// ocrDetectAndRecognize path, and that for the axis-aligned boxes this path
// actually produces (the WarpCrop is behavior-equivalent to the old FastCrop,
// so this doubles as a no-regression guard for the wiring change).
func TestOCRMergeChars_WarpsEmptyBoxCrop(t *testing.T) {
	p := newTestParser()
	page := image.NewRGBA(image.Rect(0, 0, 200, 140))

	// Axis-aligned detection box (pixel space). Char boxes are axis-aligned,
	// so the merge path only ever sees rectangles like this.
	quad := [4]util.Pt{{X: 40, Y: 40}, {X: 140, Y: 40}, {X: 140, Y: 100}, {X: 40, Y: 100}}
	box := pdf.OCRBox{
		X0: quad[0].X, Y0: quad[0].Y,
		X1: quad[1].X, Y1: quad[1].Y,
		X2: quad[2].X, Y2: quad[2].Y,
		X3: quad[3].X, Y3: quad[3].Y,
	}

	// A single space char inside the box -> matched, but its text trims to
	// empty, so the box is pushed to the need-OCR path.
	chars := []pdf.TextChar{{
		Text:       " ",
		X0:         45,
		X1:         135,
		Top:        45,
		Bottom:     95,
		PageNumber: 0,
	}}

	cap := &captureAnalyzer{
		healthy: true,
		boxes:   []pdf.OCRBox{box},
		texts:   []pdf.OCRText{{Text: "x", Confidence: 0.9}},
	}
	got := p.ocrMergeChars(t.Context(), page, chars, cap, 0)
	if len(got) == 0 {
		t.Fatal("expected at least one text box from the merge path")
	}
	if cap.recImage == nil {
		t.Fatal("OCRRecognize was not called with a crop on the merge path")
	}
	rec, ok := cap.recImage.(*image.RGBA)
	if !ok {
		t.Fatalf("rec crop is %T, want *image.RGBA", cap.recImage)
	}

	// Axis-aligned box: WarpCrop must equal FastCrop (no behavioral change),
	// and the crop fed to recognition must be exactly the warped rectangle.
	wantWarp := util.WarpCrop(page, quad)
	wantFast := util.FastCrop(page, 40, 40, 140, 100)
	if !sameImage(rec, wantWarp) {
		t.Errorf("merge-path rec crop is not the WarpCrop output (size got=%v want=%v)",
			cap.recImage.Bounds(), wantWarp.Bounds())
	}
	if !sameImage(rec, wantFast) {
		t.Errorf("merge-path crop diverged from the old FastCrop behavior; regression risk")
	}
}

func max4(a, b, c, d float64) float64 {
	m := a
	if b > m {
		m = b
	}
	if c > m {
		m = c
	}
	if d > m {
		m = d
	}
	return m
}
