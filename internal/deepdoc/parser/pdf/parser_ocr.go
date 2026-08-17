package pdf

import (
	"context"
	"image"
	"log/slog"
	"math"
	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
	"sort"
	"strings"
)

func (p *Parser) ocrDetectAndRecognize(ctx context.Context, pageImg image.Image, doc pdf.DocAnalyzer, pageNum int, logLabel string) []pdf.TextBox {
	boxes, err := p.inferOCRDetect(ctx, doc, pageImg)
	if err != nil || len(boxes) == 0 {
		if err != nil {
			slog.Warn(logLabel+" OCR detect failed", "page", pageNum, "err", err)
		}
		return nil
	}

	// detectBoxes returns image-pixel coords; ocrMergeChars divides by
	// pdf.DlaScale before emitting boxes so downstream layout receives
	// PDF-point coordinates. ocrDetectAndRecognize must match the same
	// conversion so both OCR paths produce the same coordinate space.
	imgW := float64(pageImg.Bounds().Dx()) / pdf.DlaScale
	imgH := float64(pageImg.Bounds().Dy()) / pdf.DlaScale

	var result []pdf.TextBox
	for _, b := range boxes {
		x0 := int(math.Min(b.X0, math.Min(b.X1, math.Min(b.X2, b.X3))))
		y0 := int(math.Min(b.Y0, math.Min(b.Y1, math.Min(b.Y2, b.Y3))))
		x1 := int(math.Max(b.X0, math.Max(b.X1, math.Max(b.X2, b.X3))))
		y1 := int(math.Max(b.Y0, math.Max(b.Y1, math.Max(b.Y2, b.Y3))))
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		// De-skew the quad with a perspective transform (WarpCrop, layer 1),
		// then recognize via ocrRecognizeWithRotation which applies layer-2
		// score-based 0/CW90/CCW90 selection for tall crops (get_rotate_crop_image
		// parity), so the recognizer receives a rectangular, horizontal crop
		// instead of the slanted detection region. The emitted box bounds below
		// still use the axis-aligned detection bbox (x0..y1); only the crop fed
		// to recognition is transformed.
		cropped := util.WarpCrop(pageImg, [4]util.Pt{
			{X: b.X0, Y: b.Y0},
			{X: b.X1, Y: b.Y1},
			{X: b.X2, Y: b.Y2},
			{X: b.X3, Y: b.Y3},
		})
		texts, recErr := p.ocrRecognizeWithRotation(ctx, doc, cropped)
		if recErr != nil {
			slog.Warn(logLabel+" OCR recognize failed", "page", pageNum, "err", recErr)
			continue
		}
		// Convert crop bounds back to PDF-point space before emitting.
		px0 := float64(x0) / pdf.DlaScale
		py0 := float64(y0) / pdf.DlaScale
		px1 := float64(x1) / pdf.DlaScale
		py1 := float64(y1) / pdf.DlaScale
		if px0 < 0 {
			px0 = 0
		}
		if py0 < 0 {
			py0 = 0
		}
		if px1 > imgW {
			px1 = imgW
		}
		if py1 > imgH {
			py1 = imgH
		}
		if px0 >= px1 || py0 >= py1 {
			continue
		}
		for _, t := range texts {
			if strings.TrimSpace(t.Text) != "" {
				result = append(result, pdf.TextBox{
					X0:         px0,
					X1:         px1,
					Top:        py0,
					Bottom:     py1,
					Text:       t.Text,
					PageNumber: pageNum,
				})
			}
		}
	}
	return result
}

// ocrRecognizeWithRotation recognizes a single cropped text region, applying
// layer-2 rotation selection for tall, narrow crops.
//
// When a crop's height is at least 1.5x its width, the text is most likely a
// vertical line and the recognizer — trained on horizontal text — only reads
// it cleanly after a 90 deg rotation. The crop is recognized at 0, CW90, and
// CCW90 and the orientation with the highest recognition confidence is kept.
// The emitted box bounds are unchanged; only the recognized text is
// effectively rotated. Layer 1 (the perspective de-skew) is applied at crop
// time by WarpCrop before this runs.
//
// The DeepDoc rec service surfaces the real recognition confidence, so the
// orientation is picked by score rather than by a fixed rotation.
func (p *Parser) ocrRecognizeWithRotation(ctx context.Context, doc pdf.DocAnalyzer, cropped image.Image) ([]pdf.OCRText, error) {
	b := cropped.Bounds()
	// Short / wide crops are already horizontal — recognize once at 0 deg.
	if float64(b.Dy()) < 1.5*float64(b.Dx()) {
		return p.inferOCRRecognize(ctx, doc, cropped)
	}
	candidates := []image.Image{
		cropped,
		util.RotateImageCW(cropped, 90),  // CW90
		util.RotateImageCW(cropped, 270), // CCW90
	}
	var best []pdf.OCRText
	bestScore := -1.0
	for _, c := range candidates {
		texts, err := p.inferOCRRecognize(ctx, doc, c)
		if err != nil {
			return nil, err
		}
		if s := ocrBestScore(texts); s > bestScore {
			bestScore = s
			best = texts
		}
	}
	return best, nil
}

// ocrBestScore is the layer-2 orientation score: the highest recognition
// confidence among the recognized items. A correctly oriented line reads with
// high confidence; a mis-rotated line reads with low confidence.
func ocrBestScore(texts []pdf.OCRText) float64 {
	best := 0.0
	for _, t := range texts {
		if t.Confidence > best {
			best = t.Confidence
		}
	}
	return best
}

// ocrMergeChars runs full-page detect on a page that has embedded chars,
// merges the chars into detect regions, and OCRs any regions without chars.
// Matches Python's __ocr: detect → match chars to boxes → use char text
// for boxes with embedded chars → OCR recognize only empty/garbled boxes.
type ocrDetectBox struct {
	box            pdf.TextBox
	x0, y0, x1, y1 float64
}

func (p *Parser) ocrMergeChars(ctx context.Context, pageImg image.Image, chars []pdf.TextChar, doc pdf.DocAnalyzer, pageNum int) []pdf.TextBox {
	boxes, scale, err := p.detectBoxes(ctx, pageImg, doc, pageNum)
	if err != nil || len(boxes) == 0 {
		return nil
	}
	boxChars := matchCharsToBoxes(boxes, chars)
	return p.buildTextBoxes(ctx, pageImg, boxes, boxChars, doc, scale, pageNum)
}

func (p *Parser) detectBoxes(ctx context.Context, pageImg image.Image, doc pdf.DocAnalyzer, pageNum int) ([]ocrDetectBox, float64, error) {
	ocrDetectBoxes, err := p.inferOCRDetect(ctx, doc, pageImg)
	if err != nil || len(ocrDetectBoxes) == 0 {
		return nil, 0, err
	}
	slog.Debug("ocrMergeChars detect", "page", pageNum, "boxes", len(ocrDetectBoxes))

	scale := pdf.DlaScale // 3.0
	imgBounds := pageImg.Bounds()
	imgW := float64(imgBounds.Dx()) / scale
	imgH := float64(imgBounds.Dy()) / scale

	boxes := make([]ocrDetectBox, 0, len(ocrDetectBoxes))
	for _, b := range ocrDetectBoxes {
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
		boxes = append(boxes, ocrDetectBox{box: pdf.TextBox{
			X0: x0, X1: x1, Top: y0, Bottom: y1, PageNumber: pageNum,
		}, x0: x0, y0: y0, x1: x1, y1: y1})
	}

	if len(boxes) > 1 {
		boxHeights := make([]float64, len(boxes))
		for i := range boxes {
			boxHeights[i] = boxes[i].y1 - boxes[i].y0
		}
		sort.Float64s(boxHeights)
		threshold := boxHeights[len(boxHeights)/2] / 3
		sort.Slice(boxes, func(i, j int) bool {
			if math.Abs(boxes[i].y0-boxes[j].y0) < threshold {
				return boxes[i].x0 < boxes[j].x0
			}
			return boxes[i].y0 < boxes[j].y0
		})
	}
	return boxes, scale, nil
}

func matchCharsToBoxes(boxes []ocrDetectBox, chars []pdf.TextChar) [][]pdf.TextChar {
	boxChars := make([][]pdf.TextChar, len(boxes))
	for _, c := range chars {
		bestIdx := -1
		bestOverlap := 1e-6
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
	return boxChars
}

// sortCharsYFirstly sorts chars by Y (fuzzy group by threshold), then by X.
// Matching Python Recognizer.sort_Y_firstly in recognizer.py:26-33:
//
//	If two chars have Y diff < threshold → same line → sort by X.
//	Otherwise → sort by Y.
func sortCharsYFirstly(chars []pdf.TextChar, threshold float64) {
	sort.Slice(chars, func(i, j int) bool {
		diff := chars[i].Top - chars[j].Top
		if math.Abs(diff) < threshold {
			return chars[i].X0 < chars[j].X0
		}
		return diff < 0
	})
}

// charBoxOverlapRatio computes overlap ratio between a char and a box,
// from char perspective. Returns overlap_area / char_area.
// Matching Python's Recognizer.overlapped_area(char, box, ratio=True).
func charBoxOverlapRatio(c pdf.TextChar, x0, x1, y0, y1 float64) float64 {
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
	inter := util.RectOverlapInter(c.X0, c.Top, c.X1, c.Bottom, x0, y0, x1, y1)
	return inter / charArea
}

// ocrTableCells fills empty TSR cells via OCR recognition.
func (p *Parser) ocrTableCells(ctx context.Context, cells []pdf.TSRCell, tableImg image.Image, doc pdf.DocAnalyzer) {
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
		// De-skew via WarpCrop and recognize via ocrRecognizeWithRotation like
		// the other OCR paths. Table cells are axis-aligned, so WarpCrop
		// early-exits to FastCrop and ocrRecognizeWithRotation recognizes once
		// at 0 deg; the bounds-clamp / non-finite guard is still inherited.
		cropped := util.WarpCrop(tableImg, [4]util.Pt{
			{X: float64(x0), Y: float64(y0)},
			{X: float64(x1), Y: float64(y0)},
			{X: float64(x1), Y: float64(y1)},
			{X: float64(x0), Y: float64(y1)},
		})
		texts, err := p.ocrRecognizeWithRotation(ctx, doc, cropped)
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

// buildTextBoxes assembles detect box text from embedded chars and fills empty boxes via single-image OCR.
// Each region that lacks embedded text is cropped and recognized with a
// direct doc.OCRRecognize call so empty-box fallback runs through the
// canonical single-image recognition primitive. A nil or unhealthy
// analyzer yields empty results for OCR-hungry regions instead of panicking.
func (p *Parser) buildTextBoxes(ctx context.Context, pageImg image.Image,
	boxes []ocrDetectBox, boxChars [][]pdf.TextChar, doc pdf.DocAnalyzer, scale float64, pageNum int,
) []pdf.TextBox {
	var result []pdf.TextBox
	var needOCR []int
	for i := range boxes {
		tb := boxes[i].box
		tb.Text = ""
		if len(boxChars[i]) > 0 {
			sortCharsYFirstly(boxChars[i], util.MedianCharHeight(boxChars[i]))
			lineBox := lyt.LineToTextBox(boxChars[i])
			tb.Text = lineBox.Text
			var garbledCnt, totalCnt int
			for _, c := range boxChars[i] {
				for _, r := range c.Text {
					totalCnt++
					if util.IsGarbledChar(string(r)) {
						garbledCnt++
					}
				}
			}
			// PUA / unmapped-glyph garbage: genuine noise, re-OCR regardless of script.
			if totalCnt > 0 && float64(garbledCnt)/float64(totalCnt) >= 0.5 {
				tb.Text = ""
			} else if tb.Text != "" && util.OcrCanRepresent(tb.Text) && util.IsGarbledByFontEncoding(boxChars[i], 5) {
				// Font-encoding garbling, but skipped for a script the recogniser
				// cannot spell -- OCR would only produce garbage.
				tb.Text = ""
			}
		}
		if strings.TrimSpace(tb.Text) == "" {
			tb.Text = ""
			needOCR = append(needOCR, i)
		}
		result = append(result, tb)
	}
	if len(needOCR) > 0 && doc != nil && doc.Health() {
		for _, idx := range needOCR {
			// De-skew via WarpCrop and recognize via ocrRecognizeWithRotation
			// the same way ocrDetectAndRecognize does, so all Go OCR paths feed
			// the recognizer an identical geometry. Char/table-derived boxes are
			// axis-aligned, so WarpCrop early-exits to FastCrop and
			// ocrRecognizeWithRotation recognizes once at 0 deg here, while still
			// inheriting WarpCrop's bounds-clamp / non-finite guard on the
			// untrusted detector box.
			cropped := util.WarpCrop(pageImg, [4]util.Pt{
				{X: boxes[idx].x0 * scale, Y: boxes[idx].y0 * scale},
				{X: boxes[idx].x1 * scale, Y: boxes[idx].y0 * scale},
				{X: boxes[idx].x1 * scale, Y: boxes[idx].y1 * scale},
				{X: boxes[idx].x0 * scale, Y: boxes[idx].y1 * scale},
			})
			texts, err := p.ocrRecognizeWithRotation(ctx, doc, cropped)
			if err != nil {
				slog.Warn("ocr merge: recognize failed", "page", pageNum, "err", err)
				continue
			}
			var ocrParts []string
			for _, t := range texts {
				if strings.TrimSpace(t.Text) != "" {
					ocrParts = append(ocrParts, t.Text)
				}
			}
			result[idx].Text = strings.TrimSpace(strings.Join(ocrParts, " "))
		}
	}
	filtered := result[:0]
	for _, tb := range result {
		if strings.TrimSpace(tb.Text) != "" {
			filtered = append(filtered, tb)
		}
	}
	slog.Debug("ocrMergeChars result", "page", pageNum, "boxes", len(filtered))
	return filtered
}
