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

func ocrDetectAndRecognize(ctx context.Context, pageImg image.Image, doc pdf.DocAnalyzer, pageNum int, logLabel string) []pdf.TextBox {
	boxes, err := doc.OCRDetect(ctx, pageImg)
	if err != nil || len(boxes) == 0 {
		if err != nil {
			slog.Warn(logLabel+" OCR detect failed", "page", pageNum, "err", err)
		}
		return nil
	}

	var result []pdf.TextBox
	for _, b := range boxes {
		x0 := int(math.Min(b.X0, math.Min(b.X1, math.Min(b.X2, b.X3))))
		y0 := int(math.Min(b.Y0, math.Min(b.Y1, math.Min(b.Y2, b.Y3))))
		x1 := int(math.Max(b.X0, math.Max(b.X1, math.Max(b.X2, b.X3))))
		y1 := int(math.Max(b.Y0, math.Max(b.Y1, math.Max(b.Y2, b.Y3))))
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		cropped := util.FastCrop(pageImg, x0, y0, x1, y1)
		texts, recErr := doc.OCRRecognize(ctx, cropped)
		if recErr != nil {
			slog.Warn(logLabel+" OCR recognize failed", "page", pageNum, "err", recErr)
			continue
		}
		for _, t := range texts {
			if strings.TrimSpace(t.Text) != "" {
				result = append(result, pdf.TextBox{
					X0: float64(x0), X1: float64(x1),
					Top: float64(y0), Bottom: float64(y1),
					Text: t.Text,
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
type ocrDetectBox struct {
	box            pdf.TextBox
	x0, y0, x1, y1 float64
}

func ocrMergeChars(ctx context.Context, pageImg image.Image, chars []pdf.TextChar, doc pdf.DocAnalyzer, pageNum int) []pdf.TextBox {
	boxes, scale, err := detectBoxes(ctx, pageImg, doc, pageNum)
	if err != nil || len(boxes) == 0 {
		return nil
	}
	boxChars := matchCharsToBoxes(boxes, chars)
	return buildTextBoxes(ctx, pageImg, boxes, boxChars, doc, scale, pageNum)
}

func detectBoxes(ctx context.Context, pageImg image.Image, doc pdf.DocAnalyzer, pageNum int) ([]ocrDetectBox, float64, error) {
	ocrDetectBoxes, err := doc.OCRDetect(ctx, pageImg)
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
func ocrTableCells(ctx context.Context, cells []pdf.TSRCell, tableImg image.Image, doc pdf.DocAnalyzer) {
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
		cropped := util.FastCrop(tableImg, x0, y0, x1, y1)
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

// buildTextBoxes assembles detect box text from embedded chars and fills empty boxes via batch OCR.
func buildTextBoxes(ctx context.Context, pageImg image.Image,
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
			if totalCnt > 0 && float64(garbledCnt)/float64(totalCnt) >= 0.5 {
				tb.Text = ""
			}
			if tb.Text != "" && util.IsGarbledByFontEncoding(boxChars[i], 5) {
				tb.Text = ""
			}
		}
		if strings.TrimSpace(tb.Text) == "" {
			tb.Text = ""
			needOCR = append(needOCR, i)
		}
		result = append(result, tb)
	}
	if len(needOCR) > 0 {
		cropped := make([]image.Image, len(needOCR))
		for j, idx := range needOCR {
			cropped[j] = util.FastCrop(pageImg,
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
	filtered := result[:0]
	for _, tb := range result {
		if strings.TrimSpace(tb.Text) != "" {
			filtered = append(filtered, tb)
		}
	}
	slog.Debug("ocrMergeChars result", "page", pageNum, "boxes", len(filtered))
	return filtered
}
