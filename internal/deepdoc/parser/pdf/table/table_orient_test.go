package table

import (
	"context"
	"image"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"testing"
)

// mockRotationDoc implements DocAnalyzer with deterministic OCR results per angle.
// The mock tracks the call sequence: evaluateTableOrientation tests angles in
// order 0°, 90°, 180°, 270°. Each call to OCRDetect increments an internal
// counter and returns data for the corresponding angle.
type mockRotationDoc struct {
	// angle → {regions count, average confidence, error}
	angles map[int]struct {
		regions int
		avgConf float64
		err     error
	}
	callSeq int // incremented per OCRDetect call, selects the angle's data
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

func (m *mockRotationDoc) currentAngle() int {
	idx := m.callSeq % len(rotationOrder)
	return rotationOrder[idx]
}

func (m *mockRotationDoc) OCRDetect(_ context.Context, img image.Image) ([]pdf.OCRBox, error) {
	defer func() { m.callSeq++ }()
	angle := m.currentAngle()
	cfg, ok := m.angles[angle]
	if !ok {
		cfg = m.angles[0] // fallback to 0° config
	}
	if cfg.err != nil {
		return nil, cfg.err
	}
	if cfg.regions == 0 {
		return nil, nil
	}
	w, h := img.Bounds().Dx(), img.Bounds().Dy()
	boxes := make([]pdf.OCRBox, cfg.regions)
	step := w / (cfg.regions + 1)
	for i := 0; i < cfg.regions; i++ {
		x := step * (i + 1)
		boxes[i] = pdf.OCRBox{
			X0: float64(x), Y0: float64(h / 4),
			X1: float64(x + 20), Y1: float64(h / 4),
			X2: float64(x + 20), Y2: float64(h * 3 / 4),
			X3: float64(x), Y3: float64(h * 3 / 4),
		}
	}
	return boxes, nil
}

func (m *mockRotationDoc) OCRRecognizeBatch(_ context.Context, cropped []image.Image) ([][]pdf.OCRText, []error) {
	results := make([][]pdf.OCRText, len(cropped))
	errs := make([]error, len(cropped))
	for i, img := range cropped {
		results[i], errs[i] = m.OCRRecognize(context.Background(), img)
	}
	return results, errs
}

func (m *mockRotationDoc) OCRRecognize(_ context.Context, _ image.Image) ([]pdf.OCRText, error) {
	angle := rotationOrder[(m.callSeq-1)%len(rotationOrder)] // use angle from last Detect call
	cfg, ok := m.angles[angle]
	if !ok {
		cfg = m.angles[0]
	}
	if cfg.err != nil {
		return nil, cfg.err
	}
	if cfg.regions == 0 {
		return nil, nil
	}
	texts := make([]pdf.OCRText, cfg.regions)
	for i := 0; i < cfg.regions; i++ {
		texts[i] = pdf.OCRText{Text: "X", Confidence: cfg.avgConf}
	}
	return texts, nil
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
		angle, _, scores := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)
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
		angle, _, scores := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)
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
		angle, _, scores := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)
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
		angle, _, scores := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)
		if angle != 270 {
			t.Errorf("expected 270°, got %d° (scores: %v)", angle, scores)
		}
	})

	t.Run("threshold protection — 0° keeps when diff too small", func(t *testing.T) {
		// Region-count scoring: 8 vs 9 is too close (< 1.4×) → 0° wins.
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:  {regions: 8},
				90: {regions: 9},
			},
		}
		angle, _, _ := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)
		if angle != 0 {
			t.Errorf("expected 0° (threshold protection), got %d°", angle)
		}
	})

	t.Run("threshold pass — 90° wins when region count is clearly higher", func(t *testing.T) {
		// 0° has few regions AND 90° has ≥1.4× more → 90° wins.
		doc := &mockRotationDoc{
			angles: map[int]struct {
				regions int
				avgConf float64
				err     error
			}{
				0:  {regions: 4},
				90: {regions: 10},
			},
		}
		angle, _, _ := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)
		if angle != 90 {
			t.Errorf("expected 90° (threshold passed), got %d°", angle)
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
		angle, img, scores := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)
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
}

var errMockOCR = &mockError{"mock OCR failure"}

type mockError struct{ msg string }

func (e *mockError) Error() string { return e.msg }
