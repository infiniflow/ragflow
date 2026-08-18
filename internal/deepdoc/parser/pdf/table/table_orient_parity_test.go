package table

// =============================================================================
// Parity: table orientation scoring uses detection geometry, not recognition
// confidence.
//
// Python's _evaluate_table_orientation (deepdoc/parser/pdf_parser.py:367)
// scores each candidate rotation (0/90/180/270) by OCR *recognition
// confidence*:
//   combined = avg_conf * (1 + 0.1 * min(regions, 50) / 50)
// so it picks the orientation where the text is actually legible.
//
// Go's EvaluateTableOrientation (table_orient.go:23) scores by OCR *detection*
// region count + area only:
//   combined = regions * (1 + 0.06 * areaRatio)
//
// Detection geometry is rotation-invariant: a 90°-rotated text line yields the
// same number of detection boxes and the same axis-aligned bbox area as at 0°
// (the bbox just swaps width/height). So for a cleanly-rotated table all four
// candidate angles get the SAME Go score, and Go falls back to 0° — it is
// orientation-blind and cannot tell that the table needs rotating.
//
// Run with:
//   ./build.sh --test -run TestEvaluateTableOrientation_IgnoresRecognitionConfidence ./internal/deepdoc/parser/pdf/table/
// =============================================================================

import (
	"context"
	"image"
	"math"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// orientConfMock returns angle-independent OCR *detection* output (the honest
// model of "detection cannot distinguish orientation") while carrying a
// per-angle recognition confidence as ground truth. EvaluateTableOrientation
// only consumes OCRDetect; OCRRecognize is used solely to compute the
// Python-equivalent expected angle below.
type orientConfMock struct {
	// angle → {regions, avgConf}
	angles map[int]struct {
		regions int
		avgConf float64
	}
	seq int
}

func (m *orientConfMock) DLA(context.Context, image.Image) ([]pdf.DLARegion, error) {
	return nil, nil
}
func (m *orientConfMock) TSR(context.Context, image.Image) ([]pdf.TSRCell, error) {
	return nil, nil
}
func (m *orientConfMock) OCR(image.Image) (string, error) { return "", nil }
func (m *orientConfMock) Health() bool                    { return true }

func (m *orientConfMock) OCRDetect(_ context.Context, _ image.Image) ([]pdf.OCRBox, error) {
	// Identical boxes for every rotation: OCR detection is rotation-invariant.
	cfg := m.angles[0]
	boxes := make([]pdf.OCRBox, cfg.regions)
	for i := 0; i < cfg.regions; i++ {
		x0, y0, x1, y1 := float64(10*i), 10.0, float64(10*i+8), 30.0
		boxes[i] = pdf.OCRBox{X0: x0, Y0: y0, X1: x1, Y1: y0, X2: x1, Y2: y1, X3: x0, Y3: y1}
	}
	return boxes, nil
}

func (m *orientConfMock) OCRRecognize(_ context.Context, _ image.Image) ([]pdf.OCRText, error) {
	angle := rotationOrder[m.seq%len(rotationOrder)]
	m.seq++
	cfg := m.angles[angle]
	texts := make([]pdf.OCRText, cfg.regions)
	for i := range texts {
		texts[i] = pdf.OCRText{Text: "X", Confidence: cfg.avgConf}
	}
	return texts, nil
}

// TestEvaluateTableOrientation_IgnoresRecognitionConfidence exposes that
// Go's EvaluateTableOrientation cannot rotate a table whose only signal of
// correct orientation is recognition confidence. Detection geometry alone is
// insufficient because it is invariant under 90° rotation.
func TestEvaluateTableOrientation_IgnoresRecognitionConfidence(t *testing.T) {
	// Ground truth per angle: detection is identical (8 regions, same area),
	// but recognition confidence differs — only 90° is upright/legible.
	doc := &orientConfMock{
		angles: map[int]struct {
			regions int
			avgConf float64
		}{
			0:   {regions: 8, avgConf: 0.15}, // vertical → garbage confidence
			90:  {regions: 8, avgConf: 0.90}, // upright  → legible confidence
			180: {regions: 8, avgConf: 0.15},
			270: {regions: 8, avgConf: 0.15},
		},
	}

	// ── Python-equivalent scoring from the SAME ground truth ──
	// Mirrors pdf_parser.py:367 _evaluate_table_orientation exactly.
	pyBest, pyBestScore, pyScore0 := 0, -1.0, 0.0
	for _, a := range rotationOrder {
		cfg := doc.angles[a]
		combined := cfg.avgConf * (1 + 0.1*math.Min(float64(cfg.regions), 50)/50)
		if a == 0 {
			pyScore0 = combined
		}
		if combined > pyBestScore {
			pyBestScore = combined
			pyBest = a
		}
	}
	// Python accepts a non-0° orientation only if it beats 0° by > 0.2 and
	// 0° itself reads poorly (< 0.8). Here 90° clearly wins.
	pyPicksNon0 := pyBest != 0 && (pyBestScore-pyScore0 > 0.2 && pyScore0 < 0.8)
	if !pyPicksNon0 {
		t.Fatalf("test setup error: Python-equivalent scoring did not pick 90° (best=%d score0=%.3f best=%.3f)", pyBest, pyScore0, pyBestScore)
	}

	// ── Go's actual behavior ──
	goAngle, _, goScores := EvaluateTableOrientation(context.Background(), makeTestTableImage(), doc)

	t.Logf("Python-expected angle: %d° (score0=%.3f, best=%.3f)", pyBest, pyScore0, pyBestScore)
	t.Logf("Go angle: %d° scores=%v", goAngle, goScores)

	// Go scores every angle identically (detection is rotation-invariant) and
	// falls back to 0°, so it returns the WRONG (unrotated) orientation.
	if goAngle != pyBest {
		t.Errorf("TABLE ORIENTATION DIVERGENCE: Go returns %d° but Python (recognition-confidence scoring) returns %d°. "+
			"Go scores orientation by detection region count + area only, which is rotation-invariant, so it is orientation-blind: "+
			"a correctly-vertical table that should be rotated to %d° is left unrotated at 0°. "+
			"Fix: score each angle by recognition confidence (reuse inferOCRRecognize + ocrBestScore from #18299), e.g. "+
			"avg_conf*(1+0.1*min(regions,50)/50), with threshold best-score0>0.2 && score0<0.8.",
			goAngle, pyBest, pyBest)
	}
}
