package pdf

import (
	"context"
	"image"
	"log/slog"
	"math"
	"strings"

	lyt "ragflow/internal/deepdoc/parser/pdf/layout"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// pageNumCtxKey / tableIdxCtxKey let a DocAnalyzer that replays
// pre-computed Python intermediates (the pipeline-parity harness) recover
// which page, and which per-page table, a DLA/TSR call refers to. The
// production DeepDoc analyzer ignores them; they matter only for replay
// tests, where the DocAnalyzer interface cannot otherwise carry page
// context (pages are processed concurrently and TSR receives a cropped
// image with no page identifier).
type replayCtxKey int

const (
	pageNumCtxKey replayCtxKey = iota
	tableIdxCtxKey
	// cropOffXKey / cropOffYKey carry the crop origin (image pixels) of the
	// table region that processOneTable hands to TSR. A replay TableBuilder
	// reads them to map Python's page-space TSR cells into the exact crop
	// space Go uses, so cells and boxInCrop share one coordinate frame.
	cropOffXKey
	cropOffYKey
	// ocrBoxIdxCtxKey carries the index of the OCR detect box that a
	// per-crop OCRRecognize call belongs to (stamped by ocrDetectAndRecognize
	// before recognition). The Phase 3 replay analyzer reads it to return the
	// Python-dumped recognized text for that box instead of running OCR.
	ocrBoxIdxCtxKey
)

// enrichOnePageWithDeepDoc runs DLA+TSR for a single page and returns
// worker-local artifacts. Boxes may be empty (image-only pages); the
// function still runs DLA/TSR if pageImg is available so a page can
// contribute tables and debug payloads even when no embedded text exists.
//
// Parameters:
//   - pageImg: the page bitmap DLA/TSR run against (rendered at the DLA
//     DPI); also the source image for table cropping.
//   - pageBoxes: line/word-level []pdf.TextBox (NOT per-rune) from
//     processPageBoxes, in PDF-point space. DLA/TSR annotations are
//     written back onto a shallow copy of this slice (see Returns).
//   - pg: page number (0-based), stamped onto tables and debug payloads.
//   - renderErr: non-nil short-circuits to (pageBoxes, nil, nil, nil).
//   - docAnalyzer: the DLA/OCR/Tensor backend used for region inference
//     and TSR.
//   - tb: table builder used to group TSR cells into a grid.
//   - scale: the points-to-pixels multiplier of pageImg. DLA returns
//     region coordinates in image-pixel space while box coordinates are in
//     PDF-point space, so scale bridges the two when matching tables and
//     writing annotations. Typically pdf.DlaScale (base render) or
//     retryDPI/72 (retry-zoom render) so annotation stays consistent with
//     the image that produced it.
//
// Returns:
//   - annotated: page boxes after DLA/TSR annotation write-back (LayoutType,
//     LayoutNo, R/C/H/SP fields) — same length as input pageBoxes.
//   - tables:    table candidates detected on this page.
//   - dlaRegions: page-local DLA regions payload.
func (p *Parser) enrichOnePageWithDeepDoc(ctx context.Context,
	pageImg image.Image, pageBoxes []pdf.TextBox, pg int, renderErr error,
	docAnalyzer pdf.DocAnalyzer, tb pdf.TableBuilder, scale float64,
) (annotated []pdf.TextBox, tables []pdf.TableItem,
	dlaRegions []pdf.DLAPageRegions,
) {
	if docAnalyzer == nil || !docAnalyzer.Health() || renderErr != nil || pageImg == nil {
		return pageBoxes, nil, nil
	}
	// Stamp the page number before DLA so a replay analyzer can map the call
	// back to the correct Python DLA page. The production analyzer ignores it.
	ctx = context.WithValue(ctx, pageNumCtxKey, pg)
	regions, err := p.inferDLA(ctx, docAnalyzer, pageImg)
	if err != nil {
		slog.Warn("DLA failed", "page", pg, "err", err)
		return pageBoxes, nil, nil
	}
	dlaRegions = []pdf.DLAPageRegions{{Page: pg, Regions: regions}}

	// Copy page boxes so DLA annotation can append synthetic figure boxes
	// without mutating the caller's slice. The annotated copy is what the
	// caller should use downstream for layout/text-merge.
	annotated = append([]pdf.TextBox(nil), pageBoxes...)
	annotated = tbl.AnnotateBoxLayouts(annotated, regions, scale, float64(pageImg.Bounds().Dy()))

	tableMatches := tbl.MatchTableRegions(annotated, regions, scale)
	var items []pdf.TableItem
	for i, tm := range tableMatches {
		// Stamp the per-page table index so a replay analyzer can map a
		// TSR call back to the correct Python intermediate table.
		tctx := context.WithValue(ctx, tableIdxCtxKey, i)
		item := p.processOneTable(tctx, pageImg, annotated, pg, docAnalyzer, tb, tm, scale)
		if item.ImageB64 != "" || len(item.Cells) > 0 || len(item.Positions) > 0 {
			items = append(items, item)
		}
	}
	return annotated, items, dlaRegions
}

// processOneTable handles DLA+TSR+OCR for a single table region match.
// It mutates `boxes` in place to write back R/C/H/SP annotations. The
// function is page-local and never touches the document-wide ParseResult.
func (p *Parser) processOneTable(ctx context.Context, pageImg image.Image, boxes []pdf.TextBox, pageNum int, docAnalyzer pdf.DocAnalyzer, tb pdf.TableBuilder, tm tbl.TableMatch, scale float64) pdf.TableItem {
	cropped, cropErr := util.CropImageRegion(pageImg, tm.Region)
	if cropErr != nil {
		return pdf.TableItem{}
	}
	cropOffX := math.Max(0, tm.Region.X0-util.TSRRegionMarginPx)
	cropOffY := math.Max(0, tm.Region.Y0-util.TSRRegionMarginPx)
	autoRotate := p.Config.AutoRotateTables != nil && *p.Config.AutoRotateTables
	bestAngle := 0
	origW, origH := cropped.Bounds().Dx(), cropped.Bounds().Dy()
	tsrImg := cropped
	if autoRotate {
		angle, rotated, _ := tbl.EvaluateTableOrientation(ctx, cropped, docAnalyzer)
		bestAngle = angle
		tsrImg = rotated
	}
	imgB64, encErr := util.EncodeImageToBase64PNG(cropped)
	if encErr != nil {
		slog.Warn("table PNG encode failed", "page", pageNum, "err", encErr)
	}
	// Hand the crop origin to TSR so a replay TableBuilder can map Python
	// page-space TSR cells into this exact crop frame. Production callers
	// (DeepDocTableBuilder) ignore the value and use the cropped image pixels.
	tsrCtx := context.WithValue(ctx, cropOffXKey, cropOffX)
	tsrCtx = context.WithValue(tsrCtx, cropOffYKey, cropOffY)
	cells, tsrErr := p.inferTSR(tsrCtx, tb, tsrImg)
	if tsrErr != nil {
		slog.Warn("TSR failed", "page", pageNum, "err", tsrErr)
	}
	var boxInCrop []pdf.TextBox
	if tsrErr == nil && len(cells) > 0 {
		if bestAngle != 0 {
			for i := range cells {
				cells[i].X0, cells[i].Y0, cells[i].X1, cells[i].Y1 = util.MapRotatedRectToOriginal(
					cells[i].X0, cells[i].Y0, cells[i].X1, cells[i].Y1, bestAngle, origW, origH)
			}
		}
		firstCellTop := 1e9
		for _, c := range cells {
			if c.Y0 >= 0 && c.Y0 < firstCellTop {
				firstCellTop = c.Y0
			}
		}
		if firstCellTop == 1e9 {
			firstCellTop = cells[0].Y0
		}
		// Collapse overlapping/adjacent OCR boxes before cell-fill, mirroring
		// Python's pipeline order — _naive_vertical_merge runs before
		// construct_table, so overlapping boxes are merged before they reach
		// cell assignment. Go runs table cell-fill per-page (here), before the
		// document-wide vertical merge in buildLayout, so it must run its own
		// collapse on this table's box subset first or overlapping OCR boxes
		// duplicate text across cells (e.g. 13_crosspage_table page 2:
		// '2024-43 2024-44' y=(1014,1045) + nested '2024-44' y=(1032,1045)).
		tableBoxes := make([]pdf.TextBox, 0, len(tm.BoxIdx))
		for _, idx := range tm.BoxIdx {
			b := boxes[idx]
			if b.Bottom*scale-cropOffY < firstCellTop {
				continue
			}
			tableBoxes = append(tableBoxes, b)
		}
		// Drop OCR boxes that are strictly nested inside another box whose
		// text contains theirs (e.g. a re-detected "Hardware" box fully
		// inside "Software Hardware"). Without this, NaiveVerticalMerge
		// concatenates the two into "Software Hardware Hardware" while
		// Python's construct_table keeps the longer box only. Mirrors
		// Python's effective behavior: the nested duplicate is not merged.
		tableBoxes = dedupNestedBoxes(tableBoxes)
		tableBoxes = lyt.NaiveVerticalMerge(tableBoxes, nil, nil, nil)
		boxInCrop = make([]pdf.TextBox, 0, len(tableBoxes))
		for _, b := range tableBoxes {
			boxInCrop = append(boxInCrop, tbl.BoxToCropSpace(b, scale, cropOffX, cropOffY))
		}
	}
	var positions []pdf.Position
	for _, idx := range tm.BoxIdx {
		b := boxes[idx]
		positions = append(positions, pdf.Position{
			PageNumbers: []int{pageNum},
			Left:        b.X0, Right: b.X1, Top: b.Top, Bottom: b.Bottom,
		})
	}
	var grid [][]pdf.TSRCell
	if len(cells) > 0 && len(boxInCrop) > 0 {
		// Cross-product grid (structure lines, de-duplicated like Python's
		// gather) is used ONLY to derive per-box R/C annotations.
		annotGrid := tb.GroupCells(cells)
		if len(annotGrid) > 0 {
			// Derive R/C/H/SP with Python's _table_transformer_job semantics
			// (whole-row/whole-column line matching), in crop space.
			tbl.AnnotateBoxesWithGrid(boxInCrop, annotGrid)
			// Rebuild the grid from the derived per-char R/C, exactly like
			// Python's construct_table groups boxes by their R/C labels —
			// rows are produced only for R values that carry boxes. Fall back
			// to the cross-product grid when no box carries annotations.
			if rcGrid := tbl.GroupBoxesByRC(boxInCrop); len(rcGrid) > 0 {
				grid = rcGrid
			} else {
				grid = annotGrid
			}
		}
	}
	item := pdf.TableItem{
		ImageB64: imgB64, Cells: cells, Grid: grid, Positions: positions,
		Scale: scale, CropOffX: cropOffX, CropOffY: cropOffY,
		RegionLeft: tm.Region.X0 / scale, RegionRight: tm.Region.X1 / scale,
		RegionTop: tm.Region.Y0 / scale, RegionBottom: tm.Region.Y1 / scale,
		Page: pageNum,
	}
	tbl.WriteTableAnnotations(boxes, tm.BoxIdx, cells, scale, cropOffX, cropOffY, tb)
	return item
}

// dedupNestedBoxes drops OCR boxes that are strictly nested inside another
// box whose trimmed text contains the nested box's trimmed text. This removes
// re-detected duplicate boxes (e.g. a "Hardware" detection fully inside a
// "Software Hardware" box) so the subsequent NaiveVerticalMerge does not
// concatenate them into "Software Hardware Hardware". Python's
// construct_table keeps the longer box only, so this matches Python's output.
//
// Containment requires the nested box's bbox to lie entirely within the other
// box's bbox (same or narrower X, same or shorter Y). Side-by-side or
// half-overlapping boxes are never nested, so they are preserved and merged
// normally.
func dedupNestedBoxes(boxes []pdf.TextBox) []pdf.TextBox {
	keep := make([]bool, len(boxes))
	for i := range boxes {
		keep[i] = strings.TrimSpace(boxes[i].Text) != ""
	}
	for i := 0; i < len(boxes); i++ {
		if !keep[i] {
			continue
		}
		at := strings.TrimSpace(boxes[i].Text)
		for j := 0; j < len(boxes); j++ {
			if i == j || !keep[j] {
				continue
			}
			bt := strings.TrimSpace(boxes[j].Text)
			if bt == "" || !strings.Contains(at, bt) {
				continue
			}
			// Drop the nested (shorter) box when it sits fully inside the
			// outer box's bbox.
			if boxes[j].X0 >= boxes[i].X0 && boxes[j].X1 <= boxes[i].X1 &&
				boxes[j].Top >= boxes[i].Top && boxes[j].Bottom <= boxes[i].Bottom {
				keep[j] = false
			}
		}
	}
	out := make([]pdf.TextBox, 0, len(boxes))
	for i := range boxes {
		if keep[i] {
			out = append(out, boxes[i])
		}
	}
	return out
}
