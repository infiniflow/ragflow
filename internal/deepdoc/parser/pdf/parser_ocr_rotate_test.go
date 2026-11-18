package pdf

import (
	"context"
	"image"
	"sync"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// fakeRotateDoc is a DocAnalyzer whose OCRRecognize returns text and a
// recognition score keyed on the image orientation: a "wide" image (w >= h)
// reads as a clean coherent line with wideScore, a "tall" image (h > w) reads
// as a low-score / garbled result with tallScore. This mirrors reality: a
// correctly oriented horizontal line reads with high confidence, while a
// 90°-rotated line reads with low confidence. It also counts calls so the
// tests can assert the layer-2 fan-out (1 call for short crops, 3 for tall
// crops) and the score-based selection outcome.
type fakeRotateDoc struct {
	mu        sync.Mutex
	calls     int
	wideText  string
	tallText  string
	wideScore float64
	tallScore float64
}

func (f *fakeRotateDoc) OCRRecognize(_ context.Context, img image.Image) ([]pdf.OCRText, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	b := img.Bounds()
	txt := f.tallText
	score := f.tallScore
	if b.Dx() >= b.Dy() {
		txt = f.wideText
		score = f.wideScore
	}
	if txt == "" {
		return nil, nil
	}
	return []pdf.OCRText{{Text: txt, Confidence: score}}, nil
}
func (f *fakeRotateDoc) Health() bool { return true }
func (f *fakeRotateDoc) DLA(context.Context, image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (f *fakeRotateDoc) TSR(context.Context, image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (f *fakeRotateDoc) OCRDetect(context.Context, image.Image) ([]pdf.OCRBox, error) {
	return nil, nil
}

// TestOCRRecognizeWithRotation_ShortCrop_NoRotation verifies that crops whose
// height is below 1.5x their width are recognized once at 0° with no rotation
// fan-out — i.e. layer 2 is a no-op for the common horizontal case.
func TestOCRRecognizeWithRotation_ShortCrop_NoRotation(t *testing.T) {
	crop := image.NewRGBA(image.Rect(0, 0, 60, 20)) // w=60, h=20 -> ratio 0.33
	doc := &fakeRotateDoc{wideText: "Hello", tallText: "x"}
	p := &Parser{}

	texts, err := p.ocrRecognizeWithRotation(t.Context(), doc, crop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.calls != 1 {
		t.Fatalf("expected 1 rec call for short crop, got %d", doc.calls)
	}
	if len(texts) != 1 || texts[0].Text != "Hello" {
		t.Fatalf("expected [Hello], got %+v", texts)
	}
}

// TestOCRRecognizeWithRotation_TallCrop_PicksBestScore verifies that a tall
// narrow crop (h/w >= 1.5) is tried at 0°, CW90°, CCW90° and the orientation
// with the highest recognition score wins (score-based, matching Python).
func TestOCRRecognizeWithRotation_TallCrop_PicksBestScore(t *testing.T) {
	crop := image.NewRGBA(image.Rect(0, 0, 20, 60)) // w=20, h=60 -> ratio 3.0
	doc := &fakeRotateDoc{wideText: "Hello world", tallText: "x", wideScore: 0.95, tallScore: 0.30}
	p := &Parser{}

	texts, err := p.ocrRecognizeWithRotation(t.Context(), doc, crop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.calls != 3 {
		t.Fatalf("expected 3 rec calls for tall crop, got %d", doc.calls)
	}
	if len(texts) != 1 || texts[0].Text != "Hello world" {
		t.Fatalf("expected best-score text [Hello world], got %+v", texts)
	}
}

// TestOCRRecognizeWithRotation_TallCrop_TieKeepsZero verifies the stable
// tie-break: when all three orientations yield equal score, the 0° result is
// kept (matches Python's first-wins selection order).
func TestOCRRecognizeWithRotation_TallCrop_TieKeepsZero(t *testing.T) {
	crop := image.NewRGBA(image.Rect(0, 0, 20, 60)) // w=20, h=60 -> ratio 3.0
	doc := &fakeRotateDoc{wideText: "AB", tallText: "CD", wideScore: 0.5, tallScore: 0.5}
	p := &Parser{}

	texts, err := p.ocrRecognizeWithRotation(t.Context(), doc, crop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.calls != 3 {
		t.Fatalf("expected 3 rec calls, got %d", doc.calls)
	}
	if len(texts) != 1 || texts[0].Text != "CD" {
		t.Fatalf("expected tie to keep 0° text [CD], got %+v", texts)
	}
}

// TestOCRBestScore verifies the orientation score is the max recognition
// confidence across items.
func TestOCRBestScore(t *testing.T) {
	cases := []struct {
		name  string
		texts []pdf.OCRText
		want  float64
	}{
		{"empty", nil, 0},
		{"single", []pdf.OCRText{{Text: "ab", Confidence: 0.9}}, 0.9},
		{"multi", []pdf.OCRText{{Text: "ab", Confidence: 0.7}, {Text: "c", Confidence: 0.95}}, 0.95},
		{"keeps max", []pdf.OCRText{{Text: "a", Confidence: 0.2}, {Text: "b", Confidence: 0.8}}, 0.8},
	}
	for _, c := range cases {
		if got := ocrBestScore(c.texts); got != c.want {
			t.Fatalf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
