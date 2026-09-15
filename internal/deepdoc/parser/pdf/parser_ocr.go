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

	// For each box, de-skew via WarpCrop (layer 1) and build the layer-2
	// rotation candidates: short/wide crops get one candidate at 0 deg; tall
	// crops (h >= 1.5x w) get three (0/CW90/CCW90) so the highest-confidence
	// orientation wins. We remember each box's emitted PDF-point box and the
	// span of its candidates in the flat crop slice so we can pick the best
	// candidate per box after recognition.
	type pendingBox struct {
		x0, y0, x1, y1 float64
		spanStart      int
		spanLen        int
		pageNum        int
	}
	var (
		result     []pdf.TextBox
		cropAcc    []image.Image
		cropBoxIdx []int // box index each crop belongs to (for replay routing)
		pending    []pendingBox
	)
	for i, b := range boxes {
		x0 := int(math.Min(b.X0, math.Min(b.X1, math.Min(b.X2, b.X3))))
		y0 := int(math.Min(b.Y0, math.Min(b.Y1, math.Min(b.Y2, b.Y3))))
		x1 := int(math.Max(b.X0, math.Max(b.X1, math.Max(b.X2, b.X3))))
		y1 := int(math.Max(b.Y0, math.Max(b.Y1, math.Max(b.Y2, b.Y3))))
		if x0 >= x1 || y0 >= y1 {
			continue
		}
		cropped := util.WarpCrop(pageImg, [4]util.Pt{
			{X: b.X0, Y: b.Y0},
			{X: b.X1, Y: b.Y1},
			{X: b.X2, Y: b.Y2},
			{X: b.X3, Y: b.Y3},
		})
		// Convert detection bounds to PDF-point space (mirrors detectBoxes).
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
		cb := cropped.Bounds()
		spanStart := len(cropAcc)
		if float64(cb.Dy()) < 1.5*float64(cb.Dx()) {
			cropAcc = append(cropAcc, cropped)
			cropBoxIdx = append(cropBoxIdx, i)
		} else {
			cropAcc = append(cropAcc, cropped,
				util.RotateImageCW(cropped, 90),  // CW90
				util.RotateImageCW(cropped, 270), // CCW90
			)
			cropBoxIdx = append(cropBoxIdx, i, i, i)
		}
		pending = append(pending, pendingBox{x0: px0, y0: py0, x1: px1, y1: py1, spanStart: spanStart, spanLen: len(cropAcc) - spanStart, pageNum: pageNum})
	}

	// Recognize. A batch-capable analyzer recognizes every crop in one
	// forward pass; everyone else falls back to the per-crop canonical path
	// (also required by the replay analyzer, which routes by box index in
	// ctx and is inherently per-crop).
	if len(cropAcc) == 0 {
		return nil
	}
	allTexts := make([][]pdf.OCRText, len(cropAcc))
	if p.docSupportsBatchOCR(doc) {
		batch, berr := p.inferOCRRecognizeBatch(ctx, doc, cropAcc)
		switch {
		case berr != nil:
			// A batch error must not abort the whole page: the canonical
			// per-crop path below still produces correct results.
			slog.Warn(logLabel+" OCR batch recognize failed; falling back to per-crop", "page", pageNum, "err", berr)
		case len(batch) != len(cropAcc):
			// Defensive: a count mismatch (or a nil result) would corrupt the
			// per-box indexing further down. Fall back to per-crop instead of
			// indexing out of range.
			slog.Warn(logLabel+" OCR batch recognize returned unexpected count; falling back to per-crop", "page", pageNum, "got", len(batch), "want", len(cropAcc))
		default:
			allTexts = batch
		}
	}
	// Fill any crop the batch path did not cover (unsupported doc, batch error,
	// or count mismatch) with the per-crop canonical path. Stamping the
	// detect-box index lets a replay DocAnalyzer route the recognition back to
	// the Python-dumped text for the same box; the production analyzer ignores
	// the key.
	for ci := range cropAcc {
		if allTexts[ci] != nil {
			continue
		}
		c := cropAcc[ci]
		recCtx := context.WithValue(ctx, ocrBoxIdxCtxKey, cropBoxIdx[ci])
		texts, rerr := p.ocrRecognizeWithRotation(recCtx, doc, c)
		if rerr != nil {
			slog.Warn(logLabel+" OCR recognize failed", "page", pageNum, "err", rerr)
			return nil
		}
		allTexts[ci] = texts
	}

	// Per box: pick the highest-confidence candidate (layer-2 winner) and
	// emit a TextBox for each non-empty recognized line.
	for _, pb := range pending {
		var best []pdf.OCRText
		bestScore := -1.0
		for k := 0; k < pb.spanLen; k++ {
			texts := allTexts[pb.spanStart+k]
			if s := ocrBestScore(texts); s > bestScore {
				bestScore = s
				best = texts
			}
		}
		for _, t := range best {
			if strings.TrimSpace(t.Text) != "" {
				result = append(result, pdf.TextBox{
					X0:         pb.x0,
					X1:         pb.x1,
					Top:        pb.y0,
					Bottom:     pb.y1,
					Text:       t.Text,
					PageNumber: pb.pageNum,
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
	// srcIdx is the box's index in the OCRDetect result (before detectBoxes
	// re-sorts). It routes per-box OCR fallback (buildTextBoxes) back to the
	// same Python-dumped box in replay, matching ocrDetectAndRecognize.
	srcIdx int
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
	for i, b := range ocrDetectBoxes {
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
		}, x0: x0, y0: y0, x1: x1, y1: y1, srcIdx: i})
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
	// deferred holds fully-contained small glyphs (candidates for inline text)
	// until we know whether the box also carries any normal-height content. A
	// box whose ONLY char-layer chars are small (ratio >= 0.7) must stay empty
	// so buildTextBoxes' OCR fallback can recognize the full line from the
	// image — keeping the small glyphs alone would emit a partial fragment and
	// suppress the OCR fill (三国人物/反间谍法 regressed under that rule).
	deferred := make([][]pdf.TextChar, len(boxes))
	for _, c := range chars {
		bestIdx := -1
		bestOverlap := 1e-6
		bestArea := 0.0
		for i := range boxes {
			overlap := charBoxOverlapRatio(c, boxes[i].x0, boxes[i].x1, boxes[i].y0, boxes[i].y1)
			if overlap < bestOverlap {
				continue
			}
			area := (boxes[i].x1 - boxes[i].x0) * (boxes[i].y1 - boxes[i].y0)
			// Tie-break: when a char is fully inside several boxes (a full-line
			// box and a contained OCR fragment that over-segments it), prefer
			// the LARGER container so the fragment cannot steal the glyph and
			// truncate the container. Mirrors Python's Recognizer.find_overlapped
			// (recognizer.py:223), which keeps the max-overlapped (largest-area)
			// box on ties via a strict `>`. The previous `>=`-with-last-wins
			// rule let the smaller fragment win, truncating the container; after
			// DedupSubstringOverlaps could no longer recognise the fragment as a
			// substring, NaiveVerticalMerge glued it back on and duplicated text
			// (ocr_real RAG分词 doubling).
			if overlap > bestOverlap || area > bestArea {
				bestOverlap = overlap
				bestArea = area
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
		// Char-height filter (mirrors Python pdf_parser.py:798): drop chars
		// whose height differs greatly from the box height — they belong to
		// another line. A fully-contained small glyph (overlap >= 0.90,
		// ratio < 0.9) is an inline-text candidate (e.g. a code span like
		// "certifi", ~8pt, inside a tall two-line detect box ~36pt — the Python
		// golden keeps it, plugin-daemon box[16]): it cannot be from an
		// adjacent line because it lies almost entirely inside the box. The
		// 0.90 bound (not 0.95) absorbs a sub-point detection overshoot where
		// the glyph's top pokes ~0.6pt above the box edge (刑法's footnote ①,
		// overlap 0.92) while still excluding a true partial-overlap adjacent
		// line (overlap well below 0.90). It is deferred and re-kept only when
		// the box also carries normal-height content, so an isolated small
		// glyph cannot suppress the OCR fallback.
		ratio := math.Abs(ch-bh) / math.Max(ch, bh)
		if ratio < 0.7 || c.Text == " " {
			boxChars[bestIdx] = append(boxChars[bestIdx], c)
		} else if bestOverlap >= 0.90 && ratio < 0.9 {
			deferred[bestIdx] = append(deferred[bestIdx], c)
		}
	}
	for i := range boxChars {
		// Re-keep the deferred inline glyphs only when the box actually carries
		// a non-space normal-height char: a box whose only normal chars are
		// spaces (the real line text lives in a tighter neighbor box) must not
		// absorb the small glyphs — they are another line's content there.
		hasText := false
		for _, c := range boxChars[i] {
			if strings.TrimSpace(c.Text) != "" {
				hasText = true
				break
			}
		}
		if hasText {
			boxChars[i] = append(boxChars[i], deferred[i]...)
		}
	}
	return boxChars
}

// boxIsCoveredLeftFragment reports whether detect box i is a spurious
// over-segmentation fragment whose content was already resolved into a
// same-line RIGHT neighbor by the char layer, so OCR-filling i would only
// re-read and duplicate that neighbor's text. The detector sometimes splits a
// single TOC line into a narrow left box plus the real text box; the char
// layer assigns the glyphs to the real box, leaving the left box with no
// usable text (its stray char was deferred then dropped by the height gate,
// so its assembled text is empty). If we then OCR-fill the left box we
// duplicate its glyph (刑法's 妨妨).
//
// selfText is box i's assembled char-layer text. i is a covered left fragment
// when: selfText is empty (no usable content of its own); some same-line
// neighbor j (Y-overlap >= 0.9) carries real char text; j starts INSIDE i's
// x-span (j.x0 in (i.x0, i.x1]) and i ends at/before j (i.x1 <= j.x1) — i is
// a left overhang of j; and i is much narrower than j (width < j.width/2), so
// it is a fragment, not a genuine second column. The narrow gate plus the
// right-neighbor requirement keep legitimate char-less boxes (font-encoded
// captions with no same-line right neighbor) OCR-filled.
func boxIsCoveredLeftFragment(boxes []ocrDetectBox, boxChars [][]pdf.TextChar, i int, selfText string) bool {
	if i < 0 || i >= len(boxes) || len(boxChars) != len(boxes) {
		return false
	}
	if strings.TrimSpace(selfText) != "" {
		return false // i has usable text of its own; not a fragment
	}
	ai := boxes[i]
	aw := ai.x1 - ai.x0
	if aw <= 0 {
		return false
	}
	for j := range boxes {
		if j == i {
			continue
		}
		bj := boxes[j]
		// Same line: Y-overlap ratio against the shorter box >= 0.9.
		interY := math.Min(ai.y1, bj.y1) - math.Max(ai.y0, bj.y0)
		if interY <= 0 {
			continue
		}
		minH := math.Min(ai.y1-ai.y0, bj.y1-bj.y0)
		if minH <= 0 {
			continue
		}
		if interY/minH < 0.9 {
			continue
		}
		// Neighbor must carry real (non-space) char content.
		hasText := false
		for _, c := range boxChars[j] {
			if strings.TrimSpace(c.Text) != "" {
				hasText = true
				break
			}
		}
		if !hasText {
			continue
		}
		// i overhangs to the LEFT of j: j starts inside i's x-span and i does
		// not extend right past j.
		if !(bj.x0 > ai.x0 && bj.x0 < ai.x1 && ai.x1 <= bj.x1) {
			continue
		}
		// i is a small fragment, not a genuine column.
		bw := bj.x1 - bj.x0
		if aw >= bw*0.5 {
			continue
		}
		return true
	}
	return false
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
			// A char-less detect box that is a left-overhang fragment of a
			// same-line neighbor (which already carries the glyphs via the
			// char layer) would only re-read and duplicate the neighbor's text
			// if OCR-filled. Leave it empty; the trailing filter drops it.
			if !boxIsCoveredLeftFragment(boxes, boxChars, i, tb.Text) {
				needOCR = append(needOCR, i)
			}
		}
		result = append(result, tb)
	}
	if len(needOCR) > 0 && doc != nil && doc.Health() {
		// Collect every OCR-hungry box's de-skewed crop and recognize them in
		// one batched forward pass when the analyzer supports it; otherwise
		// fall back to the per-crop canonical path (required by the replay
		// analyzer, which routes by srcIdx in ctx and is inherently per-crop).
		// Char/table-derived boxes are axis-aligned, so WarpCrop early-exits to
		// FastCrop and each box yields exactly one 0-deg candidate here, while
		// still inheriting WarpCrop's bounds-clamp / non-finite guard.
		type ocrJob struct {
			boxIdx    int // index into result
			srcIdx    int // detect-box index for replay routing
			spanStart int
			spanLen   int
		}
		var crops []image.Image
		var jobs []ocrJob
		for _, idx := range needOCR {
			cropped := util.WarpCrop(pageImg, [4]util.Pt{
				{X: boxes[idx].x0 * scale, Y: boxes[idx].y0 * scale},
				{X: boxes[idx].x1 * scale, Y: boxes[idx].y0 * scale},
				{X: boxes[idx].x1 * scale, Y: boxes[idx].y1 * scale},
				{X: boxes[idx].x0 * scale, Y: boxes[idx].y1 * scale},
			})
			spanStart := len(crops)
			crops = append(crops, cropped)
			jobs = append(jobs, ocrJob{boxIdx: idx, srcIdx: boxes[idx].srcIdx, spanStart: spanStart, spanLen: 1})
		}
		allTexts := make([][]pdf.OCRText, len(crops))
		if p.docSupportsBatchOCR(doc) {
			batch, berr := p.inferOCRRecognizeBatch(ctx, doc, crops)
			if berr != nil {
				slog.Warn("ocr merge: batch recognize failed", "page", pageNum, "err", berr)
				return nil
			}
			allTexts = batch
		} else {
			for ci, c := range crops {
				// Stamp the source detect-box index so a replay DocAnalyzer
				// routes this fallback to the same Python-dumped box
				// (detectBoxes may have re-sorted, so use srcIdx, not the loop
				// index). The production analyzer ignores the key.
				recCtx := context.WithValue(ctx, ocrBoxIdxCtxKey, jobs[ci].srcIdx)
				texts, rerr := p.ocrRecognizeWithRotation(recCtx, doc, c)
				if rerr != nil {
					slog.Warn("ocr merge: recognize failed", "page", pageNum, "err", rerr)
					continue
				}
				allTexts[ci] = texts
			}
		}
		for _, j := range jobs {
			var best []pdf.OCRText
			bestScore := -1.0
			for k := 0; k < j.spanLen; k++ {
				texts := allTexts[j.spanStart+k]
				if s := ocrBestScore(texts); s > bestScore {
					bestScore = s
					best = texts
				}
			}
			var ocrParts []string
			for _, t := range best {
				if strings.TrimSpace(t.Text) != "" {
					ocrParts = append(ocrParts, t.Text)
				}
			}
			result[j.boxIdx].Text = strings.TrimSpace(strings.Join(ocrParts, " "))
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
