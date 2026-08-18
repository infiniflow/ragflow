package table

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"math"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/deepdoc/parser/pdf/util"
)

// EvaluateTableOrientation tests 4 rotation angles (0/90/180/270) and picks
// the best orientation based on OCR recognition confidence, matching Python's
// pdf_parser.py:367 _evaluate_table_orientation().
//
// For each angle the table image is rotated and recognized; the combined score
// is avg_conf * (1 + 0.1*min(regions, 50)/50). Recognition legibility is the
// signal, NOT detection geometry: detection box count and axis-aligned area are
// rotation-invariant (a 90°-rotated text line yields the same boxes and area as
// at 0°), so they cannot tell a table's true orientation apart.
//
// Returns bestAngle (0/90/180/270), the rotated image, and per-angle scores.
//
// Absolute threshold: non-0° wins only if its combined score exceeds 0° by
// more than 0.2 AND the 0° score is below 0.8.
//
// Python: pdf_parser.py:367 _evaluate_table_orientation()
func EvaluateTableOrientation(ctx context.Context, tableImg image.Image, doc pdf.DocAnalyzer) (bestAngle int, bestImg image.Image, scores map[int]float64) {
	rotations := []struct {
		angle int
		name  string
	}{
		{0, "original"},
		{90, "rotate_90"},
		{180, "rotate_180"},
		{270, "rotate_270"},
	}

	scores = make(map[int]float64, 4)
	bestScore := float64(-1)
	bestAngle = 0
	bestImg = tableImg

	for _, rot := range rotations {
		rotated := tableImg
		if rot.angle != 0 {
			rotated = util.RotateImageCW(tableImg, rot.angle)
			if rotated == nil {
				slog.Warn("table rotate failed", "angle", rot.angle)
				continue
			}
		}

		// Score by recognition confidence (legibility), matching Python's
		// _evaluate_table_orientation: avg_conf * (1 + 0.1*min(regions,50)/50).
		texts, err := doc.OCRRecognize(ctx, rotated)
		if err != nil || len(texts) == 0 {
			scores[rot.angle] = 0
			continue
		}

		var confSum float64
		for _, t := range texts {
			confSum += t.Confidence
		}
		avgConf := confSum / float64(len(texts))
		regions := len(texts)
		combined := avgConf * (1 + 0.1*math.Min(float64(regions), 50)/50)
		scores[rot.angle] = combined

		slog.Debug("table orientation",
			"angle", rot.angle,
			"regions", regions,
			"avg_conf", fmt.Sprintf("%.4f", avgConf),
			"combined", fmt.Sprintf("%.4f", combined))

		if combined > bestScore {
			bestScore = combined
			bestAngle = rot.angle
			bestImg = rotated
		}
	}

	// Absolute threshold: only accept non-0° if its combined score exceeds
	// 0° by more than 0.2 AND the 0° score is below 0.8. Mirrors Python's
	// `score_0 is not None` (not score_0 > 0): when 0° has no recognized text
	// (score_0 == 0) the margin clause still gates acceptance.
	score0 := scores[0]
	if bestAngle != 0 {
		if !(bestScore-score0 > 0.2 && score0 < 0.8) {
			bestAngle = 0
			bestImg = tableImg
			bestScore = score0
		}
	}

	slog.Debug("best table orientation",
		"angle", bestAngle,
		"score", fmt.Sprintf("%.4f", bestScore))

	return bestAngle, bestImg, scores
}
