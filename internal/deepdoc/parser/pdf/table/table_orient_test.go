package table

import (
	"context"
	"image"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"testing"
)

// mockRotationDoc implements DocAnalyzer with deterministic OCR results per angle.
// It mirrors the real orientation path: EvaluateTableOrientation calls
// OCRDetect once per angle (returning `regions` text-line boxes), then
// OCRRecognize once per detected line. Each OCRRecognize returns a single
// text with that angle's average confidence, so the aggregated score is
// avgConf * (1 + 0.1*min(regions,50)/50) — identical to the formula tests'
// expectations. The mock records call counts so tests can assert the
// detect-then-recognize pattern is actually used.
type mockRotationDoc struct {
	// angle → {regions count, average confidence, error}
	angles map[int]struct {
		regions int
		avgConf float64
		err     error
	}
	detectSeq    int // incremented per OCRDetect call, selects the angle's data
	currentAngle int
	detectCalls  int
	recCalls     int
}

var rotationOrder = []int{0, 90, 180, 270}

func (m *mockRotationDoc) DLA(_ context.Context, _ image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (m *mockRotationDoc) TSR(_ context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (m *mockRotationDoc) OCR(_ image.Image) (string, error) { return "", nil }
func (m *mockRotationDoc) Health() bool                      { return true }

func (m *mockRotationDoc) OCRDetect(_ context.Context, _ image.Image) ([]pdf.OCRBox, error) {
	angle := rotationOrder[m.detectSeq%len(rotationOrder)]
	m.detectSeq++
	m.currentAngle = angle
	m.detectCalls++
	cfg, ok := m.angles[angle]
	if !ok {
		cfg = m.angles[0]
	}
	if cfg.err != nil {
		return nil, cfg.err
	}
	boxes := make([]pdf.OCRBox, cfg.regions)
	// Give each line a non-degenerate axis-aligned quad so WarpCrop produces a
	// valid crop. Exact geometry is irrelevant to the mock's scoring.
	for i := 0; i < cfg.regions; i++ {
		y := float64(i * 10)
		boxes[i] = pdf.OCRBox{
			X0: 0, Y0: y, X1: 100, Y1: y,
			X2: 100, Y2: y + 8, X3: 0, Y3: y + 8,
		}
	}
	return boxes, nil
}

func (m *mockRotationDoc) OCRRecognize(_ context.Context, _ image.Image) ([]pdf.OCRText, error) {
	m.recCalls++
	cfg, ok := m.angles[m.currentAngle]
	if !ok {
		cfg = m.angles[0]
	}
	if cfg.err != nil {
		return nil, cfg.err
	}
	// One recognized text per detected line, carrying the angle's avgConf.
	return []pdf.OCRText{{Text: "X", Confidence: cfg.avgConf}}, nil
}

func makeTestTableImage() image.Image {
	return image.NewRGBA(image.Rect(0, 0, 200, 100))
}

func TestEvaluateTableOrientation(t *testing.T) {
	t.Run("normal table 0° wins", func(t *testing.T) {
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0: {regions: 10, avgConf: 0.9},
			},
		}
		angle, _, scores := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)
		if angle != 0 {
			t.Errorf("expected 0°, got %d° (scores: %v)", angle, scores)
		}
	})

	t.Run("90° rotated table wins", func(t *testing.T) {
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:   {regions: 2, avgConf: 0.2},
				90:  {regions: 10, avgConf: 0.9},
				180: {regions: 2, avgConf: 0.2},
				270: {regions: 2, avgConf: 0.2},
			},
		}
		angle, _, scores := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)
		if angle != 90 {
			t.Errorf("expected 90°, got %d° (scores: %v)", angle, scores)
		}
	})

	t.Run("180° rotated table wins", func(t *testing.T) {
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:   {regions: 1, avgConf: 0.1},
				90:  {regions: 1, avgConf: 0.1},
				180: {regions: 8, avgConf: 0.85},
				270: {regions: 1, avgConf: 0.1},
			},
		}
		angle, _, scores := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)
		if angle != 180 {
			t.Errorf("expected 180°, got %d° (scores: %v)", angle, scores)
		}
	})

	t.Run("270° rotated table wins", func(t *testing.T) {
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:   {regions: 1, avgConf: 0.1},
				90:  {regions: 1, avgConf: 0.1},
				180: {regions: 1, avgConf: 0.1},
				270: {regions: 9, avgConf: 0.88},
			},
		}
		angle, _, scores := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)
		if angle != 270 {
			t.Errorf("expected 270°, got %d° (scores: %v)", angle, scores)
		}
	})

	t.Run("threshold protection — 0° keeps when confidence diff too small", func(t *testing.T) {
		// Recognition scores 0.50 vs 0.55 are too close (< 0.2 margin) → 0° wins.
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:  {regions: 8, avgConf: 0.50},
				90: {regions: 8, avgConf: 0.55},
			},
		}
		angle, _, _ := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)
		if angle != 0 {
			t.Errorf("expected 0° (threshold protection), got %d°", angle)
		}
	})

	t.Run("threshold pass — 90° wins when recognition confidence is clearly higher", func(t *testing.T) {
		// 0° reads poorly (0.30) AND 90° reads well (0.90) → 90° wins.
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:  {regions: 4, avgConf: 0.30},
				90: {regions: 10, avgConf: 0.90},
			},
		}
		angle, _, _ := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)
		if angle != 90 {
			t.Errorf("expected 90° (threshold passed), got %d°", angle)
		}
	})

	t.Run("threshold guard — score_0 >= 0.8 blocks rotation despite large margin", func(t *testing.T) {
		// Isolate the score_0 < 0.8 clause: 0° reads well (0.80) and 90° is
		// clearly higher (1.00), so the margin clause (combined diff 0.22 > 0.2)
		// passes, but score_0 = 0.88 >= 0.8 must still force keeping 0°.
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:  {regions: 50, avgConf: 0.80},
				90: {regions: 50, avgConf: 1.00},
			},
		}
		angle, _, _ := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)
		if angle != 0 {
			t.Errorf("expected 0° (score_0 >= 0.8 guard), got %d°", angle)
		}
	})

	t.Run("all angles fail OCR → fallback 0°", func(t *testing.T) {
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:   {err: errMockOCR},
				90:  {err: errMockOCR},
				180: {err: errMockOCR},
				270: {err: errMockOCR},
			},
		}
		angle, img, scores := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)
		if angle != 0 {
			t.Errorf("expected 0° fallback, got %d°", angle)
		}
		if img == nil {
			t.Error("expected non-nil fallback image")
		}
		for _, s := range scores {
			if s != 0 {
				t.Error("all scores should be 0 on OCR failure")
			}
		}
	})

	t.Run("zero score_0 with low non-zero score — keep 0°", func(t *testing.T) {
		// 0° has no recognized text (score_0 == 0). A non-zero angle with a
		// low combined score must NOT be accepted, matching Python's
		// `score_0 is not None` threshold (not `score_0 > 0`).
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:  {regions: 0, avgConf: 0},
				90: {regions: 2, avgConf: 0.05},
			},
		}
		angle, _, _ := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)
		if angle != 0 {
			t.Errorf("expected 0° (score_0 == 0, low non-zero score), got %d°", angle)
		}
	})

	t.Run("zero score_0 with high non-zero score — accept rotation", func(t *testing.T) {
		// 0° has no recognized text (score_0 == 0) but 90° reads clearly
		// (combined 1.045 > 0.2). Mirrors Python: score_0 is not None, so the
		// margin clause alone decides and 90° is accepted.
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:  {regions: 0, avgConf: 0},
				90: {regions: 50, avgConf: 0.95},
			},
		}
		angle, _, _ := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)
		if angle != 90 {
			t.Errorf("expected 90° (score_0 == 0, high non-zero score), got %d°", angle)
		}
	})
}

var errMockOCR = &mockError{"mock OCR failure"}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }
