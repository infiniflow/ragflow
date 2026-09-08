package table

import (
	"context"
	"image"
	"math"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// =============================================================================
// TDD regression lock for the orientation OCR call mode.
//
// The bug: EvaluateTableOrientation previously sent the WHOLE table image to a
// single OCRRecognize(rec) call, treating the entire table as one text line.
// That collapses `regions` to 1 and yields a degenerate score, unlike Python's
// _evaluate_table_orientation which runs detection first and recognizes each
// text line separately.
//
// These tests pin the corrected behavior:
//   1. OCRDetect is called once per candidate angle.
//   2. OCRRecognize is called once PER DETECTED LINE (not once per whole image).
//   3. The score aggregates the per-line mean confidence with regions = line count.
//   4. No whole-image rec fallback — empty detection scores 0 (matches Python).
// =============================================================================

// callCountDoc counts OCRDetect / OCRRecognize invocations and records the
// number of detected lines per angle. OCRRecognize returns one text per call,
// so recCalls == total detected lines — the literal opposite of the old
// one-call-per-angle behavior.
type callCountDoc struct {
	angles map[int]struct {
		regions int
		avgConf float64
		err     error
	}
	detectSeq    int
	currentAngle int
	detectCalls  int
	recCalls     int
}

func (m *callCountDoc) DLA(_ context.Context, _ image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (m *callCountDoc) TSR(_ context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (m *callCountDoc) OCR(_ image.Image) (string, error) { return "", nil }
func (m *callCountDoc) Health() bool                      { return true }

func (m *callCountDoc) OCRDetect(_ context.Context, _ image.Image) ([]pdf.OCRBox, error) {
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
	for i := 0; i < cfg.regions; i++ {
		y := float64(i * 10)
		boxes[i] = pdf.OCRBox{X0: 0, Y0: y, X1: 100, Y1: y, X2: 100, Y2: y + 8, X3: 0, Y3: y + 8}
	}
	return boxes, nil
}

func (m *callCountDoc) OCRRecognize(_ context.Context, _ image.Image) ([]pdf.OCRText, error) {
	m.recCalls++
	cfg, ok := m.angles[m.currentAngle]
	if !ok {
		cfg = m.angles[0]
	}
	if cfg.err != nil {
		return nil, cfg.err
	}
	return []pdf.OCRText{{Text: "line", Confidence: cfg.avgConf}}, nil
}

// TestEvaluateTableOrientation_DetectThenPerLineRecognize pins the core fix:
// detection runs once per angle and recognition runs once per detected line —
// NOT one whole-image rec call per angle.
func TestEvaluateTableOrientation_DetectThenPerLineRecognize(t *testing.T) {
	doc := &callCountDoc{
		angles: map[int]struct {
			regions int
			avgConf float64
			err     error
		}{
			0:   {regions: 5, avgConf: 0.2},
			90:  {regions: 7, avgConf: 0.2},
			180: {regions: 3, avgConf: 0.2},
			270: {regions: 9, avgConf: 0.95},
		},
	}
	angle, _, scores := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)

	if doc.detectCalls != 4 {
		t.Errorf("expected OCRDetect called once per candidate angle (4), got %d", doc.detectCalls)
	}
	// 5 + 7 + 3 + 9 = 24 per-line recognition calls; the buggy path made 4.
	if doc.recCalls != 24 {
		t.Errorf("expected one OCRRecognize per detected line (24), got %d — whole-image rec path not fixed", doc.recCalls)
	}
	// Equal per-line confidence but 270° has the most lines, so its bonus term
	// (1 + 0.1*min(regions,50)/50) is largest → it must win.
	if angle != 270 {
		t.Errorf("expected 270° (most legible lines at equal conf), got %d° (scores: %v)", angle, scores)
	}
}

// perLineDoc returns a distinct recognition confidence per detected line,
// cycling through the configured per-angle list. This lets a test assert that
// the orientation score is the MEAN over lines with regions = line count.
type perLineDoc struct {
	lines        map[int][]float64
	detectSeq    int
	currentAngle int
	recIdx       map[int]int
}

func (m *perLineDoc) DLA(_ context.Context, _ image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (m *perLineDoc) TSR(_ context.Context, _ image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (m *perLineDoc) OCR(_ image.Image) (string, error) { return "", nil }
func (m *perLineDoc) Health() bool                      { return true }

func (m *perLineDoc) OCRDetect(_ context.Context, _ image.Image) ([]pdf.OCRBox, error) {
	angle := rotationOrder[m.detectSeq%len(rotationOrder)]
	m.detectSeq++
	m.currentAngle = angle
	n := len(m.lines[angle])
	boxes := make([]pdf.OCRBox, n)
	for i := 0; i < n; i++ {
		y := float64(i * 10)
		boxes[i] = pdf.OCRBox{X0: 0, Y0: y, X1: 100, Y1: y, X2: 100, Y2: y + 8, X3: 0, Y3: y + 8}
	}
	return boxes, nil
}

func (m *perLineDoc) OCRRecognize(_ context.Context, _ image.Image) ([]pdf.OCRText, error) {
	confs := m.lines[m.currentAngle]
	idx := m.recIdx[m.currentAngle]
	m.recIdx[m.currentAngle]++
	c := confs[idx%len(confs)]
	return []pdf.OCRText{{Text: "line", Confidence: c}}, nil
}

// TestEvaluateTableOrientation_AggregatesPerLineConfidence proves the score is
// the mean of per-line confidences with regions = number of lines, and that a
// mixed-legibility table at 0° loses to a fully-legible one at 180°.
func TestEvaluateTableOrientation_AggregatesPerLineConfidence(t *testing.T) {
	doc := &perLineDoc{
		lines: map[int][]float64{
			0:   {0.2, 1.0}, // avg 0.6, regions 2 → 0.6 * (1 + 0.1*2/50)
			90:  {0.1, 0.1},
			180: {1.0, 1.0}, // avg 1.0, regions 2 → 1.0 * (1 + 0.1*2/50)
			270: {0.1, 0.1},
		},
		recIdx: map[int]int{},
	}
	angle, _, scores := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)

	if angle != 180 {
		t.Errorf("expected 180° (fully legible lines), got %d° (scores: %v)", angle, scores)
	}
	want0 := (0.2 + 1.0) / 2 * (1 + 0.1*2.0/50)
	if math.Abs(scores[0]-want0) > 1e-9 {
		t.Errorf("score[0]=%.6f, want per-line mean %.6f (regions=2)", scores[0], want0)
	}
}

// TestEvaluateTableOrientation_EmptyDetectionScoresZero confirms there is no
// whole-image rec fallback: when detection finds no text lines, the angle
// scores 0 — matching Python's OCR.__call__ returning [] for an unreadable
// table. This guards against silently reverting to the degenerate buggy path.
func TestEvaluateTableOrientation_EmptyDetectionScoresZero(t *testing.T) {
	doc := &perLineDoc{lines: map[int][]float64{}, recIdx: map[int]int{}} // no lines at any angle
	angle, img, scores := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)

	if angle != 0 {
		t.Errorf("expected 0° fallback, got %d°", angle)
	}
	if img == nil {
		t.Error("expected non-nil fallback image")
	}
	for _, s := range scores {
		if s != 0 {
			t.Error("all scores should be 0 when detection finds no lines")
		}
	}
}
