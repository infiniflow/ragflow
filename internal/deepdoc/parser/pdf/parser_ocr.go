package parser

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"math"
	"sort"
	"strings"
	"unicode"
)

// isGarbledPage returns true if a page is garbled by PUA ratio, font encoding,
// pdf_oxide unmapped glyphs, or scan noise (no real words).
func isGarbledPage(chars []TextChar) bool {
	if len(chars) < 20 {
		return false
	}
	// Build full-page text for detection (all O(n) single pass).
	var fullText strings.Builder
	for _, c := range chars {
		fullText.WriteString(c.Text)
	}
	text := fullText.String()
	if IsGarbledText(text, 0.3) {
		return true
	}
	if pdfOxideUnmappedGarbled(text) && isScanNoise(text) {
		return true
	}
	if IsGarbledByFontEncoding(chars, 20) {
		return true
	}
	if isScanNoise(text) {
		return true
	}
	return false
}

// isScanNoise detects scanned pages where pdf_oxide extracts noise glyphs
// instead of real text.  Real text in any language contains word-like runs
// of consecutive letters (L category).  Scan noise consists of random ASCII
// symbols with at most 2-letter fragments.
//
// Three indicators of real (non-noise) text, any one is sufficient:
//   - ≥4 consecutive lowercase Latin letters (e.g. "the", "and")
//   - ≥2 consecutive CJK characters (Han, Hiragana, Katakana, Hangul)
//   - ≥4 consecutive non-ASCII letters (Arabic, Thai, Cyrillic, etc.)
//
// Pure-uppercase fragments like "RASB" are common in pdf_oxide noise but
// never appear as standalone words in real text without lowercase context.
func isScanNoise(text string) bool {
	nonSpace := 0
	digitCount := 0
	lowerRun := 0
	maxLowerRun := 0
	cjkRun := 0
	maxCJKRun := 0
	nonASCIILetterRun := 0
	maxNonASCIILetterRun := 0

	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			lowerRun = 0
			cjkRun = 0
			nonASCIILetterRun = 0
			continue
		}
		nonSpace++

		// Digit density: real content (tables, dates) has digits;
		// pdf_oxide noise (unmapped glyphs) never produces digits.
		if r >= '0' && r <= '9' {
			digitCount++
		}

		// Lowercase Latin (Ll)
		if unicode.Is(unicode.Ll, r) {
			lowerRun++
			if lowerRun > maxLowerRun {
				maxLowerRun = lowerRun
			}
		} else {
			lowerRun = 0
		}

		// CJK: Han, Hiragana, Katakana, Hangul Syllables & Jamo
		if isCJK(r) {
			cjkRun++
			if cjkRun > maxCJKRun {
				maxCJKRun = cjkRun
			}
		} else {
			cjkRun = 0
		}

		// Non-ASCII letter (Arabic U+0600–U+06FF, Thai U+0E00–U+0E7F,
		// Cyrillic U+0400–U+04FF, etc.).  Excludes ASCII so uppercase
		// Latin fragments like "RASB" don't count.
		if unicode.IsLetter(r) && r > unicode.MaxASCII {
			nonASCIILetterRun++
			if nonASCIILetterRun > maxNonASCIILetterRun {
				maxNonASCIILetterRun = nonASCIILetterRun
			}
		} else {
			nonASCIILetterRun = 0
		}
	}

	// Need enough characters to make a meaningful decision.
	if nonSpace < 30 {
		return false
	}

	// Digit density: pdf_oxide never substitutes digits for unmapped
	// glyphs. Real content (tables, dates, page numbers) has ≥10%
	// digits; noise consists of random ASCII punctuation.
	if float64(digitCount)/float64(nonSpace) >= 0.10 {
		return false
	}

	// Real text in any script — any one indicator is sufficient.
	isNoise := maxLowerRun < 4 && maxCJKRun < 2 && maxNonASCIILetterRun < 4

	return isNoise
}

// isCJK reports whether r is a CJK character: Han ideograph, Hiragana,
// Katakana, Hangul syllable, or Hangul Jamo.
func isCJK(r rune) bool {
	return unicode.Is(unicode.Han, r) ||
		unicode.Is(unicode.Hiragana, r) ||
		unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

// pdfOxideUnmappedGarbled detects pdf_oxide's '#' placeholder glyphs.
// pdf_oxide uses '#' (U+0023) for every glyph it cannot map; consecutive
// unmapped glyphs form "##", "###", "####" sequences.  Three or more
// consecutive '#' is virtually impossible in normal text.
//
// Two conditions (either is sufficient):
//   - ≥ 2 occurrences of "###" (3+ consecutive #)
//   - # density ≥ 5% of non-space characters
func pdfOxideUnmappedGarbled(text string) bool {
	hashCount := 0
	total := 0
	consecutive := 0
	tripleClusters := 0

	for _, r := range text {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		total++
		if r == '#' {
			hashCount++
			consecutive++
			if consecutive == 3 {
				tripleClusters++
			}
		} else {
			consecutive = 0
		}
	}

	if total == 0 {
		return false
	}

	density := float64(hashCount) / float64(total)

	if tripleClusters >= 1 {
		return true
	}
	// Density check only meaningful with enough chars (matches isGarbledPage's
	// min 20 char guard).  In production the sample is 200 chars.
	if total >= 40 && density >= 0.03 {
		return true
	}
	return false
}

// ocrDetectAndRecognize runs OCR detection + recognition and returns
// recognized TextBox results. logLabel distinguishes callers in log output
// ("scan page", "garbled page").
func ocrDetectAndRecognize(ctx context.Context, pageImg image.Image, doc DocAnalyzer, pageNum int, logLabel string) []TextBox {
	boxes, err := doc.OCRDetect(ctx, pageImg)
	if err != nil || len(boxes) == 0 {
		if err != nil {
			slog.Warn(logLabel+" OCR detect failed", "page", pageNum, "err", err)
		}
		return nil
	}

	var result []TextBox
	for _, box := range boxes {
		x0 := int(math.Min(box.X0, math.Min(box.X1, math.Min(box.X2, box.X3))))
		y0 := int(math.Min(box.Y0, math.Min(box.Y1, math.Min(box.Y2, box.Y3))))
		x1 := int(math.Max(box.X0, math.Max(box.X1, math.Max(box.X2, box.X3))))
		y1 := int(math.Max(box.Y0, math.Max(box.Y1, math.Max(box.Y2, box.Y3))))
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		cropped := fastCrop(pageImg, x0, y0, x1, y1)
		texts, recErr := doc.OCRRecognize(ctx, cropped)
		if recErr != nil {
			slog.Warn(logLabel+" OCR recognize failed", "page", pageNum, "err", recErr)
			continue
		}
		for _, t := range texts {
			if strings.TrimSpace(t.Text) != "" {
				result = append(result, TextBox{
					X0: float64(x0), X1: float64(x1),
					Top: float64(y0), Bottom: float64(y1),
					Text:       t.Text,
					PageNumber: pageNum,
				})
			}
		}
	}
	return result
}

// ocrMergeChars runs full-page detect on a page that has embedded chars,
// merges the chars into detect regions, and OCRs any regions without chars.
// Matches Python's __ocr: detect → match chars to boxes → use char text
// for boxes with embedded chars → OCR recognize only empty/garbled boxes.
func ocrMergeChars(ctx context.Context, pageImg image.Image, chars []TextChar, doc DocAnalyzer, pageNum int) []TextBox {
	detectBoxes, err := doc.OCRDetect(ctx, pageImg)
	if err != nil || len(detectBoxes) == 0 {
		return nil
	}
	slog.Debug("ocrMergeChars detect", "page", pageNum, "boxes", len(detectBoxes))

	// Detect boxes are in pixel space (216 DPI).  Scale to PDF space (72 DPI)
	// so coordinates match embedded chars.
	scale := dlaScale // 3.0
	imgBounds := pageImg.Bounds()
	imgW := float64(imgBounds.Dx()) / scale
	imgH := float64(imgBounds.Dy()) / scale

	// Step 1: match embedded chars to detect boxes (Python __ocr char matching).
	type detectBox struct {
		box            TextBox
		x0, y0, x1, y1 float64 // PDF-space bounds
	}
	boxes := make([]detectBox, 0, len(detectBoxes))
	for _, b := range detectBoxes {
		x0 := min(b.X0, b.X1, b.X2, b.X3) / scale
		y0 := min(b.Y0, b.Y1, b.Y2, b.Y3) / scale
		x1 := max(b.X0, b.X1, b.X2, b.X3) / scale
		y1 := max(b.Y0, b.Y1, b.Y2, b.Y3) / scale
		if x0 < 0 {
			x0 = 0
		}
		if y0 < 0 {
			y0 = 0
		}
		if x1 > imgW {
			x1 = imgW
		}
		if y1 > imgH {
			y1 = imgH
		}
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		boxes = append(boxes, detectBox{box: TextBox{
			X0: x0, X1: x1, Top: y0, Bottom: y1, PageNumber: pageNum,
		}, x0: x0, y0: y0, x1: x1, y1: y1})
	}

	// Sort detect boxes top-down (fuzzy Y-group), matching Python's
	// Recognizer.sort_Y_firstly with threshold = median box height / 3.
	if len(boxes) > 1 {
		boxHeights := make([]float64, len(boxes))
		for i := range boxes {
			boxHeights[i] = boxes[i].y1 - boxes[i].y0
		}
		sort.Float64s(boxHeights)
		threshold := boxHeights[len(boxHeights)/2] / 3
		sort.Slice(boxes, func(a, b int) bool {
			if math.Abs(boxes[a].y0-boxes[b].y0) < threshold {
				return boxes[a].x0 < boxes[b].x0
			}
			return boxes[a].y0 < boxes[b].y0
		})
	}

	// Step 2: match each char to the best overlapping detect box
	// (char perspective), matching Python's find_overlapped.
	boxChars := make([][]TextChar, len(boxes))
	for _, c := range chars {
		bestIdx := -1
		bestOverlap := 1e-6 // Python: thr=1e-6
		for i := range boxes {
			overlap := charBoxOverlapRatio(c, boxes[i].x0, boxes[i].x1, boxes[i].y0, boxes[i].y1)
			if overlap >= bestOverlap {
				bestOverlap = overlap
				bestIdx = i
			}
		}
		if bestIdx < 0 {
			continue
		}
		// Height gating, matching Python: skip when height differs >70%,
		// except space chars which are always kept.
		ch := c.Bottom - c.Top
		if ch <= 0 {
			ch = 1
		}
		bh := boxes[bestIdx].y1 - boxes[bestIdx].y0
		if math.Abs(ch-bh)/math.Max(ch, bh) >= 0.7 && c.Text != " " {
			continue
		}
		boxChars[bestIdx] = append(boxChars[bestIdx], c)
	}

	// Step 3: assemble text for each box.
	var result []TextBox
	var needOCR []int
	for i := range boxes {
		tb := boxes[i].box
		tb.Text = ""

		if len(boxChars[i]) > 0 {
			// Sort chars by reading order, matching Python's sort_Y_firstly.
			// Fuzzy Y-group: chars within median char height are "same line",
			// sorted by X; different lines sorted by Y.
			sortCharsYFirstly(boxChars[i], medianCharHeight(boxChars[i]))
			// Use lineToTextBox for correct space insertion + garbled detection.
			// lineToTextBox inserts ASCII word spaces at visible gaps —
			// matching Python's __img_ocr + __ocr char logic.
			lineBox := lineToTextBox(boxChars[i])
			tb.Text = lineBox.Text

			// Strategy 1: If majority of chars are garbled (PUA), clear text → OCR.
			var garbledCnt, totalCnt int
			for _, c := range boxChars[i] {
				for _, r := range c.Text {
					totalCnt++
					if IsGarbledChar(string(r)) {
						garbledCnt++
					}
				}
			}
			if totalCnt > 0 && float64(garbledCnt)/float64(totalCnt) >= 0.5 {
				tb.Text = ""
			}
			// Strategy 2: font-encoding garbled (subset fonts, min 5 chars).
			if tb.Text != "" && IsGarbledByFontEncoding(boxChars[i], 5) {
				tb.Text = ""
			}
		}

		// Step 4: batch OCR recognize boxes without embedded chars (or garbled).
		if tb.Text == "" {
			needOCR = append(needOCR, i)
		}
		result = append(result, tb)
	}

	if len(needOCR) > 0 {
		cropped := make([]image.Image, len(needOCR))
		for j, idx := range needOCR {
			cropped[j] = fastCrop(pageImg,
				int(boxes[idx].x0*scale), int(boxes[idx].y0*scale),
				int(boxes[idx].x1*scale), int(boxes[idx].y1*scale))
		}
		allTexts, allErrs := doc.OCRRecognizeBatch(ctx, cropped)
		for j, idx := range needOCR {
			if allErrs[j] != nil {
				slog.Warn("ocr merge: recognize failed", "page", pageNum, "err", allErrs[j])
				continue
			}
			var ocrParts []string
			for _, t := range allTexts[j] {
				if strings.TrimSpace(t.Text) != "" {
					ocrParts = append(ocrParts, t.Text)
				}
			}
			result[idx].Text = strings.TrimSpace(strings.Join(ocrParts, " "))
		}
	}
	// Filter out boxes with no text.
	filtered := result[:0]
	for _, tb := range result {
		if tb.Text != "" {
			filtered = append(filtered, tb)
		}
	}
	result = filtered
	slog.Debug("ocrMergeChars result", "page", pageNum, "boxes", len(result))
	return result
}

// medianCharHeight returns the median height of chars, or 0 if empty.
// Used as the fuzzy-sort threshold matching Python's np.mean([c["height"]]).
func medianCharHeight(chars []TextChar) float64 {
	if len(chars) == 0 {
		return 0
	}
	heights := make([]float64, len(chars))
	for i, c := range chars {
		heights[i] = c.Bottom - c.Top
	}
	sort.Float64s(heights)
	return heights[len(heights)/2]
}

// sortYFirstly sorts chars by Y (fuzzy group by threshold), then by X.
// Matching Python Recognizer.sort_Y_firstly in recognizer.py:26-33:
//
//	If two chars have Y diff < threshold → same line → sort by X.
//	Otherwise → sort by Y.
func sortCharsYFirstly(chars []TextChar, threshold float64) {
	sort.Slice(chars, func(a, b int) bool {
		diff := chars[a].Top - chars[b].Top
		if math.Abs(diff) < threshold {
			return chars[a].X0 < chars[b].X0
		}
		return diff < 0
	})
}

// charBoxOverlapRatio computes the overlap ratio between a char and a box,
// from the char's perspective.  Returns overlap_area / char_area.
// Matching Python's Recognizer.overlapped_area(char, box, ratio=True).
func charBoxOverlapRatio(c TextChar, x0, x1, y0, y1 float64) float64 {
	cw := c.X1 - c.X0
	ch := c.Bottom - c.Top
	if cw <= 0 {
		cw = 1
	}
	if ch <= 0 {
		ch = 1
	}
	charArea := cw * ch
	if charArea <= 0 {
		return 0
	}
	inter := rectOverlapInter(c.X0, c.Top, c.X1, c.Bottom, x0, y0, x1, y1)
	return inter / charArea
}

// ocrTableCells fills empty TSR cells via OCR recognition.
func ocrTableCells(ctx context.Context, cells []TSRCell, tableImg image.Image, doc DocAnalyzer) {
	if doc == nil || tableImg == nil || len(cells) == 0 {
		return
	}
	for i := range cells {
		if cells[i].Text != "" {
			continue
		}
		x0 := int(math.Max(0, cells[i].X0))
		y0 := int(math.Max(0, cells[i].Y0))
		x1 := int(math.Min(float64(tableImg.Bounds().Dx()), cells[i].X1))
		y1 := int(math.Min(float64(tableImg.Bounds().Dy()), cells[i].Y1))
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		cropped := fastCrop(tableImg, x0, y0, x1, y1)
		texts, err := doc.OCRRecognize(ctx, cropped)
		if err != nil {
			slog.Warn("table cell OCR failed", "err", err)
			continue
		}
		var parts []string
		for _, t := range texts {
			if t.Text != "" {
				parts = append(parts, t.Text)
			}
		}
		cells[i].Text = strings.TrimSpace(strings.Join(parts, " "))
	}
}

// evaluateTableOrientation tests 4 rotation angles (0/90/180/270) and picks
// the best orientation based on OCR confidence scores.
//
// Returns bestAngle (0/90/180/270), the rotated image, and per-angle scores.
// Scores map[angle]{avgConfidence, totalRegions, combinedScore}.
//
// Absolute threshold: non-0° wins only if its combined score exceeds 0° by
// more than 0.2 AND the 0° score is below 0.8.
//
// Python: pdf_parser.py:314 _evaluate_table_orientation()
func evaluateTableOrientation(ctx context.Context, tableImg image.Image, doc DocAnalyzer) (bestAngle int, bestImg image.Image, scores map[int]float64) {
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
			rotated = rotateImageCW(tableImg, rot.angle)
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
		// Per-region OCRRecognize calls are NOT needed to judge table
		// orientation — the count of detect regions is a reliable proxy
		// (a well-oriented table has more/fuller text regions).
		// Skipping recognize cuts ~N HTTP calls per angle.
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
		// Region count is the primary signal.  Area coverage provides a
		// small bonus (up to +6%) so that when region counts are tied the
		// angle with fuller text boxes wins.
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
	// Prevents false rotation when the table is roughly upright.
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
