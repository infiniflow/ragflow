package table

// =============================================================================
// Parity: Go's EvaluateTableOrientation must agree with Python's
// _evaluate_table_orientation (deepdoc/parser/pdf_parser.py:367) on which
// rotation angle is chosen.
//
// Both score each candidate rotation (0/90/180/270) by OCR *recognition
// confidence*:
//   combined = avg_conf * (1 + 0.1 * min(regions, 50) / 50)
// so the orientation where the text is actually legible wins.
//
// Why recognition confidence (not detection geometry) is the right signal:
// detection box count and axis-aligned bbox area are rotation-invariant — a
// 90°-rotated text line yields the same boxes and area as at 0° (the bbox just
// swaps width/height). Detection therefore carries no orientation signal; only
// recognition legibility does. This is the rationale for scoring by confidence
// rather than by detection geometry.
//
// Run with:
//   ./build.sh --test -run TestEvaluateTableOrientation_MatchesPythonRecognitionConfidence ./internal/deepdoc/parser/pdf/table/
// =============================================================================

import (
	"context"
	"image"
	"math"
	"testing"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// orientConfMock implements DocAnalyzer with per-angle recognition confidence
// as ground truth — the signal EvaluateTableOrientation consumes to choose the
// best rotation. It mirrors the real path: OCRDetect returns `regions` line
// boxes per angle, then OCRRecognize returns one text per line carrying the
// angle's avgConf, so the aggregated score equals
// avgConf * (1 + 0.1*min(regions,50)/50) — exactly the Python formula.
type orientConfMock struct {
	// angle → {regions, avgConf}
	angles map[int]struct {
		regions int
		avgConf float64
	}
	seq          int
	currentAngle int
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
	angle := rotationOrder[m.seq%len(rotationOrder)]
	m.seq++
	m.currentAngle = angle
	cfg := m.angles[angle]
	boxes := make([]pdf.OCRBox, cfg.regions)
	for i := 0; i < cfg.regions; i++ {
		y := float64(i * 10)
		boxes[i] = pdf.OCRBox{X0: 0, Y0: y, X1: 100, Y1: y, X2: 100, Y2: y + 8, X3: 0, Y3: y + 8}
	}
	return boxes, nil
}

func (m *orientConfMock) OCRRecognize(_ context.Context, _ image.Image) ([]pdf.OCRText, error) {
	return []pdf.OCRText{{Text: "X", Confidence: m.angles[m.currentAngle].avgConf}}, nil
}

// TestEvaluateTableOrientation_MatchesPythonRecognitionConfidence verifies that
// Go's EvaluateTableOrientation picks the same rotation angle as Python's
// _evaluate_table_orientation when scoring purely by recognition confidence.
// Detection geometry is rotation-invariant, so only recognition legibility can
// distinguish the correct orientation — Go must agree with Python on which
// angle that is.
func TestEvaluateTableOrientation_MatchesPythonRecognitionConfidence(t *testing.T) {
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
	goAngle, _, goScores := EvaluateTableOrientation(t.Context(), makeTestTableImage(), doc)

	t.Logf("Python-expected angle: %d° (score0=%.3f, best=%.3f)", pyBest, pyScore0, pyBestScore)
	t.Logf("Go angle: %d° scores=%v", goAngle, goScores)

	// Both implementations score purely by recognition confidence, so they must
	// agree on the chosen angle. A mismatch means Go diverged from the Python
	// formula or threshold.
	if goAngle != pyBest {
		t.Errorf("TABLE ORIENTATION PARITY DIVERGENCE: Go returns %d° but Python (recognition-confidence scoring) returns %d°. "+
			"Both should score each angle by avg_conf*(1+0.1*min(regions,50)/50) with threshold "+
			"best-score_0>0.2 && score_0<0.8, yielding angle %d°.",
			goAngle, pyBest, pyBest)
	}
}
