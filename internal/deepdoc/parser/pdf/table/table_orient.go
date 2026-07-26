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
// the best orientation based on OCR detect-region count and area coverage.
//
// Returns bestAngle (0/90/180/270), the rotated image, and per-angle scores.
//
// Absolute threshold: non-0° wins only if its combined score exceeds 0° by
// more than 1.4× AND the 0° score is below 6.0.
//
// Python: pdf_parser.py:314 _evaluate_table_orientation()
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

		detectBoxes, err := doc.OCRDetect(ctx, rotated)
		if err != nil || len(detectBoxes) == 0 {
			scores[rot.angle] = 0
			continue
		}

		// Score by detect-region count (primary) + area (tiebreaker).
		imageArea := float64(rotated.Bounds().Dx() * rotated.Bounds().Dy())
		totalRegions := 0
		var totalArea float64
		for _, box := range detectBoxes {
			x0 := math.Min(box.X0, math.Min(box.X1, math.Min(box.X2, box.X3)))
			y0 := math.Min(box.Y0, math.Min(box.Y1, math.Min(box.Y2, box.Y3)))
			x1 := math.Max(box.X0, math.Max(box.X1, math.Max(box.X2, box.X3)))
			y1 := math.Max(box.Y0, math.Max(box.Y1, math.Max(box.Y2, box.Y3)))
			if x0 >= x1 || y0 >= y1 {
				continue
			}
			totalRegions++
			totalArea += (x1 - x0) * (y1 - y0)
		}
		if totalRegions == 0 {
			scores[rot.angle] = 0
			continue
		}
		areaRatio := totalArea / imageArea
		combined := float64(totalRegions) * (1 + 0.06*areaRatio)
		scores[rot.angle] = combined

		slog.Debug("table orientation",
			"angle", rot.angle,
			"regions", totalRegions,
			"area_ratio", fmt.Sprintf("%.4f", areaRatio),
			"combined", fmt.Sprintf("%.2f", combined))

		if combined > bestScore {
			bestScore = combined
			bestAngle = rot.angle
			bestImg = rotated
		}
	}

	// Absolute threshold: only accept non-0° if region count is clearly
	// higher (≥1.4×) AND 0° has few regions (< 6).
	score0 := scores[0]
	if bestAngle != 0 && score0 > 0 {
		if !(bestScore > score0*1.4 && score0 < 6.0) {
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
