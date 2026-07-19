package pdf

import (
	"context"
	"image"
	"log/slog"
	"math"

	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
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
	for _, tm := range tableMatches {
		item := p.processOneTable(ctx, pageImg, annotated, pg, docAnalyzer, tb, tm, scale)
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
	cells, tsrErr := p.inferTSR(ctx, tb, tsrImg)
	if tsrErr != nil {
		slog.Warn("TSR failed", "page", pageNum, "err", tsrErr)
	}
	w := tm.Region.X1 - tm.Region.X0
	h := tm.Region.Y1 - tm.Region.Y0
	cropOffX := math.Max(0, tm.Region.X0-w*0.03)
	cropOffY := math.Max(0, tm.Region.Y0-h*0.03)
	var boxInCrop []pdf.TextBox
	if tsrErr == nil && len(cells) > 0 {
		if bestAngle != 0 {
			if !p.Config.SkipOCR {
				p.ocrTableCells(ctx, cells, tsrImg, docAnalyzer)
			}
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
		boxInCrop = make([]pdf.TextBox, 0, len(tm.BoxIdx))
		for _, idx := range tm.BoxIdx {
			b := boxes[idx]
			if b.Bottom*scale-cropOffY < firstCellTop {
				continue
			}
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
	if len(cells) > 0 {
		grid = tb.GroupCells(cells)
		if len(grid) > 0 {
			flat := tbl.FlattenGrid(grid)
			tbl.FillCellTextFromBoxes(flat, boxInCrop)
			idx := 0
			for ri := range grid {
				for ci := range grid[ri] {
					grid[ri][ci].Text = flat[idx].Text
					idx++
				}
			}
			if bestAngle == 0 && !p.Config.SkipOCR {
				p.ocrTableCells(ctx, flat, tsrImg, docAnalyzer)
				idx = 0
				for ri := range grid {
					for ci := range grid[ri] {
						grid[ri][ci].Text = flat[idx].Text
						idx++
					}
				}
			}
		}
	}
	item := pdf.TableItem{
		ImageB64: imgB64, Cells: cells, Grid: grid, Positions: positions,
		Scale: scale, CropOffX: cropOffX, CropOffY: cropOffY,
		RegionLeft: tm.Region.X0 / scale, RegionRight: tm.Region.X1 / scale,
		RegionTop: tm.Region.Y0 / scale, RegionBottom: tm.Region.Y1 / scale,
	}
	tbl.WriteTableAnnotations(boxes, tm.BoxIdx, cells, scale, cropOffX, cropOffY, tb)
	return item
}
