package pdf

import (
	"context"
	"image"
	"log/slog"
	"math"
	"sort"

	tbl "ragflow/internal/deepdoc/parser/pdf/table"
	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
)

// enrichWithDeepDoc runs DLA+TSR via docAnalyzer and returns detected tables.
// pageImages optionally provides pre-rendered page images to avoid re-rendering.
func (p *Parser) enrichWithDeepDoc(ctx context.Context, result *pdf.ParseResult, engine pdf.PDFEngine, boxes []pdf.TextBox, pageImages map[int]image.Image, docAnalyzer pdf.DocAnalyzer, tb pdf.TableBuilder) []pdf.TableItem {
	if !docAnalyzer.Health() {
		return nil
	}
	// Group boxes by page for annotation write-back.
	byPage := make(map[int][]int)
	for i, b := range boxes {
		byPage[b.PageNumber] = append(byPage[b.PageNumber], i)
	}

	// Collect all pages that have images (from pageImages) or boxes.
	// This matches Python's __images__ which processes every page regardless
	// of embedded chars — image-only PDFs still get DLA+TSR.
	allPages := make(map[int]bool)
	for pg := range pageImages {
		allPages[pg] = true
	}
	for pg := range byPage {
		allPages[pg] = true
	}
	pageKeys := make([]int, 0, len(allPages))
	for pg := range allPages {
		pageKeys = append(pageKeys, pg)
	}
	sort.Ints(pageKeys)

	var tableItems []pdf.TableItem
	for _, pg := range pageKeys {
		if err := ctx.Err(); err != nil {
			return tableItems
		}
		indices := byPage[pg]
		pageBoxes := make([]pdf.TextBox, len(indices))
		for i, idx := range indices {
			pageBoxes[i] = boxes[idx]
		}
		tables := p.extractTableBoxes(ctx, result, pageBoxes, engine, pg, pageImages, len(tableItems), docAnalyzer, tb)
		tableItems = append(tableItems, tables...)
		// Write back DLA and TSR annotations (R/C/H/SP) to the original boxes.
		for i, idx := range indices {
			if pageBoxes[i].LayoutType != "" {
				boxes[idx].LayoutType = pageBoxes[i].LayoutType
				boxes[idx].LayoutNo = pageBoxes[i].LayoutNo
			}
			tbl.CopyBoxAnnotations(&boxes[idx], &pageBoxes[i])
		}

	}
	return tableItems
}

func (p *Parser) extractTableBoxes(ctx context.Context, result *pdf.ParseResult, boxes []pdf.TextBox, engine pdf.PDFEngine, pageNum int, pageImages map[int]image.Image, tableBaseIdx int, docAnalyzer pdf.DocAnalyzer, tb pdf.TableBuilder) []pdf.TableItem {
	pageImg, ok := pageImages[pageNum]
	if !ok {
		var err error
		pageImg, err = RenderPageToImage(engine, pageNum)
		if err != nil {
			slog.Warn("render page for DeepDoc failed", "page", pageNum, "err", err)
			return nil
		}
	}
	return p.extractTableBoxesFromImage(ctx, result, boxes, pageImg, pageNum, tableBaseIdx, docAnalyzer, tb)
}

func (p *Parser) extractTableBoxesFromImage(ctx context.Context, result *pdf.ParseResult, boxes []pdf.TextBox, pageImg image.Image, pageNum int, tableBaseIdx int, docAnalyzer pdf.DocAnalyzer, tb pdf.TableBuilder) []pdf.TableItem {
	regions, err := docAnalyzer.DLA(ctx, pageImg)
	if err != nil {
		slog.Warn("DLA failed", "page", pageNum, "err", err)
		return nil
	}
	// Collect DLA debug intermediates.
	if result != nil {
		result.DLADebug = append(result.DLADebug, pdf.DLAPageRegions{Page: pageNum, Regions: regions})
	}
	// Annotate boxes with DLA layout types (title, text, figure, table, ...).
	scale := pdf.DlaScale
	boxes = tbl.AnnotateBoxLayouts(boxes, regions, scale, float64(pageImg.Bounds().Dy()))

	tableMatches := tbl.MatchTableRegions(boxes, regions, scale)
	var items []pdf.TableItem
	for _, tm := range tableMatches {
		item := p.processOneTable(ctx, result, boxes, pageImg, pageNum, docAnalyzer, tb, tm, scale, tableBaseIdx+len(items))
		if item.ImageB64 != "" || len(item.Cells) > 0 || len(item.Positions) > 0 {
			items = append(items, item)
		}
	}
	return items
}

// processOneTable handles DLA+TSR+OCR for a single table region match.
func (p *Parser) processOneTable(ctx context.Context, result *pdf.ParseResult, boxes []pdf.TextBox, pageImg image.Image, pageNum int, docAnalyzer pdf.DocAnalyzer, tb pdf.TableBuilder, tm tbl.TableMatch, scale float64, tableIdx int) pdf.TableItem {
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
	cells, tsrErr := tb.DetectCells(ctx, tsrImg)
	if tsrErr != nil {
		slog.Warn("TSR failed", "page", pageNum, "err", tsrErr)
	}
	if tsrErr == nil && result != nil {
		for _, c := range cells {
			result.TSRDebug = append(result.TSRDebug, pdf.TSRRawCell{
				TableIndex: tableIdx, Page: pageNum,
				Label: c.Label, X0: c.X0, Y0: c.Y0, X1: c.X1, Y1: c.Y1, Text: c.Text,
			})
		}
	}
	w := tm.Region.X1 - tm.Region.X0
	h := tm.Region.Y1 - tm.Region.Y0
	cropOffX := math.Max(0, tm.Region.X0-w*0.03)
	cropOffY := math.Max(0, tm.Region.Y0-h*0.03)
	var boxInCrop []pdf.TextBox
	if tsrErr == nil && len(cells) > 0 {
		if bestAngle != 0 {
			if !p.Config.SkipOCR {
				ocrTableCells(ctx, cells, tsrImg, docAnalyzer)
			}
			for i := range cells {
				cells[i].X0, cells[i].Y0 = util.MapRotatedPointToOriginal(cells[i].X0, cells[i].Y0, bestAngle, origW, origH)
				cells[i].X1, cells[i].Y1 = util.MapRotatedPointToOriginal(cells[i].X1, cells[i].Y1, bestAngle, origW, origH)
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
				ocrTableCells(ctx, flat, tsrImg, docAnalyzer)
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
