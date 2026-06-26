package parser

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"log/slog"
	"math"
	"sort"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	util "ragflow/internal/deepdoc/parser/pdf/util"
	tbl "ragflow/internal/deepdoc/parser/pdf/table"
)

// enrichWithDeepDoc runs DLA+TSR via p.DeepDoc and returns detected tables.
// pageImages optionally provides pre-rendered page images to avoid re-rendering.
func (p *Parser) enrichWithDeepDoc(ctx context.Context, engine pdf.PDFEngine, boxes []pdf.TextBox, pageImages map[int]image.Image) []pdf.TableItem {
	if !p.DeepDoc.Health() {
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
		tables := p.extractTableBoxes(ctx, pageBoxes, engine, pg, pageImages, len(tableItems))
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

func (p *Parser) extractTableBoxes(ctx context.Context, boxes []pdf.TextBox, engine pdf.PDFEngine, pageNum int, pageImages map[int]image.Image, tableBaseIdx int) []pdf.TableItem {
	pageImg, ok := pageImages[pageNum]
	if !ok {
		var err error
		pageImg, err = renderPageToImage(engine, pageNum)
		if err != nil {
			slog.Warn("render page for DeepDoc failed", "page", pageNum, "err", err)
			return nil
		}
	}
	return p.extractTableBoxesFromImage(ctx, boxes, pageImg, pageNum, tableBaseIdx)
}

func (p *Parser) extractTableBoxesFromImage(ctx context.Context, boxes []pdf.TextBox, pageImg image.Image, pageNum int, tableBaseIdx int) []pdf.TableItem {
	regions, err := p.DeepDoc.DLA(ctx, pageImg)
	if err != nil {
		slog.Warn("DLA failed", "page", pageNum, "err", err)
		return nil
	}
	// Collect DLA debug intermediates.
	p.debugDLA = append(p.debugDLA, pdf.DLAPageRegions{Page: pageNum, Regions: regions})
	// Annotate boxes with DLA layout types (title, text, figure, table, ...).
	scale := pdf.DlaScale
	boxes = annotateBoxLayouts(boxes, regions, scale, float64(pageImg.Bounds().Dy()))

	tableMatches := matchTableRegions(boxes, regions, scale)
	var items []pdf.TableItem
	for _, tm := range tableMatches {
		cropped, cropErr := util.CropImageRegion(pageImg, tm.region)
		if cropErr != nil {
			// DLA returned an invalid region (e.g. x1 < x0).  Python
			// PIL.Image.crop() raises ValueError here; we skip this
			// table instead of passing a full-page image to TSR.
			continue
		}

		// Rotation detection (Python: _evaluate_table_orientation).
		// If rotated, TSR and OCR use the rotated image; cell coords
		// are mapped back to original crop space for box matching.
		autoRotate := p.Config.AutoRotateTables != nil && *p.Config.AutoRotateTables
		bestAngle := 0
		origW, origH := cropped.Bounds().Dx(), cropped.Bounds().Dy()
		tsrImg := cropped
		if autoRotate {
			angle, rotated, _ := tbl.EvaluateTableOrientation(ctx, cropped, p.DeepDoc)
			bestAngle = angle
			tsrImg = rotated
		}

		imgB64, encErr := encodeImageToBase64PNG(cropped)
		if encErr != nil {
			slog.Warn("table PNG encode failed", "page", pageNum, "err", encErr)
		}

		var cells []pdf.TSRCell
		var tsrErr error
		cells, tsrErr = p.tableBuilder.DetectCells(ctx, tsrImg)
		if tsrErr != nil {
			slog.Warn("TSR failed", "page", pageNum, "err", tsrErr)
		}
		// Collect TSR raw cells for debug comparison.
		if tsrErr == nil {
			for _, c := range cells {
				p.debugTSR = append(p.debugTSR, pdf.TSRRawCell{
					TableIndex: tableBaseIdx + len(items), Page: pageNum,
					Label: c.Label, X0: c.X0, Y0: c.Y0, X1: c.X1, Y1: c.Y1,
					Text: c.Text,
				})
			}
		}
		// Python margin: w*0.03, h*0.03 (_table_transformer_job:374-376).
		w := tm.region.X1 - tm.region.X0
		h := tm.region.Y1 - tm.region.Y0
		marginX := w * 0.03
		marginY := h * 0.03
		cropOffX := math.Max(0, tm.region.X0-marginX)
		cropOffY := math.Max(0, tm.region.Y0-marginY)

		var boxInCrop []pdf.TextBox
		if tsrErr == nil && len(cells) > 0 {
			if bestAngle != 0 {
				// OCR on rotated image before mapping cells back.
				// Cells are in rotated-pixel space; OCR works best
				// on upright text.  After mapping, cells move to
				// original crop space where boxInCrop lives.
				if !p.Config.SkipOCR {
					ocrTableCells(ctx, cells, tsrImg, p.DeepDoc)
				}
				for i := range cells {
					cells[i].X0, cells[i].Y0 = util.MapRotatedPointToOriginal(cells[i].X0, cells[i].Y0, bestAngle, origW, origH)
					cells[i].X1, cells[i].Y1 = util.MapRotatedPointToOriginal(cells[i].X1, cells[i].Y1, bestAngle, origW, origH)
				}
			}
			// Fill cell text from pre-merge boxes, skipping caption boxes
			// (text entirely above the first TSR cell row).
			firstCellTop := 1e9
			for _, c := range cells {
				if c.Y0 >= 0 && c.Y0 < firstCellTop {
					firstCellTop = c.Y0
				}
			}
			if firstCellTop == 1e9 {
				firstCellTop = cells[0].Y0 // fallback if all cells have Y0 < 0
			}
			boxInCrop = make([]pdf.TextBox, 0, len(tm.boxIdx))
			for _, idx := range tm.boxIdx {
				b := boxes[idx]
				if b.Bottom*scale-cropOffY < firstCellTop {
					continue // caption box above first TSR cell
				}
				boxInCrop = append(boxInCrop, tbl.BoxToCropSpace(b, scale, cropOffX, cropOffY))
			}
		}
		var positions []pdf.Position
		for _, idx := range tm.boxIdx {
			b := boxes[idx]
			positions = append(positions, pdf.Position{
				PageNumbers: []int{pageNum},
				Left:        b.X0, Right: b.X1,
				Top: b.Top, Bottom: b.Bottom,
			})
		}
		// Pre-compute grid from raw TSR cells (without crop offset).
		// Stored in pdf.TableItem for constructTable; annotateTableBoxes
		// recomputes with offset cells for spatial matching precision.
		var grid [][]pdf.TSRCell
		if len(cells) > 0 {
			grid = p.tableBuilder.GroupCells(cells)
			// Fill cell text from boxes in crop space. Works for both
			// Label-aware grouping (cells rearranged) vs. cross-product (creates new cells).
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
					ocrTableCells(ctx, flat, tsrImg, p.DeepDoc)
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
		items = append(items, pdf.TableItem{
			ImageB64:  imgB64,
			Cells:     cells,
			Grid:      grid,
			Positions: positions,
			Scale:     scale,
			CropOffX:  cropOffX,
			CropOffY:  cropOffY,
			// DLA region in PDF point space (Python's cropout uses layout region boundaries).
			RegionLeft:   tm.region.X0 / scale,
			RegionRight:  tm.region.X1 / scale,
			RegionTop:    tm.region.Y0 / scale,
			RegionBottom: tm.region.Y1 / scale,
		})

		writeTableAnnotations(boxes, tm.boxIdx, cells, scale, cropOffX, cropOffY, p.tableBuilder)
	}
	return items
}

// tableMatch pairs a DLA table region with the indices of boxes that overlap it.
type tableMatch struct {
	region pdf.DLARegion
	boxIdx []int
}

// ── cell row grouping ──────────────────────────────────────────────────

// ── region matching ────────────────────────────────────────────────────

func regionOverlapsBox(region pdf.DLARegion, box pdf.TextBox, scale float64) bool {
	rx0 := region.X0 / scale
	ry0 := region.Y0 / scale
	rx1 := region.X1 / scale
	ry1 := region.Y1 / scale
	scaledR := pdf.DLARegion{X0: rx0, Y0: ry0, X1: rx1, Y1: ry1}
	inter := pdf.OverlapInter(&scaledR, &box)
	boxArea := pdf.Area(&box)
	if boxArea <= 0 {
		return false
	}
	return inter/boxArea >= 0.4 // matches Python thr=0.4
}

// matchTableRegions pairs DLA table regions with boxes that overlap them.
// Each table region is matched if at least one box overlaps it (>40% of box
// area) or if there are no boxes at all (image-only PDF), matching Python's
// _table_transformer_job which processes every table DLA region.
func matchTableRegions(boxes []pdf.TextBox, regions []pdf.DLARegion, scale float64) []tableMatch {
	var matches []tableMatch
	for _, r := range regions {
		if r.Label != pdf.LayoutTypeTable {
			continue
		}
		var matched []int
		for i, b := range boxes {
			if regionOverlapsBox(r, b, scale) {
				matched = append(matched, i)
			}
		}
		if len(matched) > 0 || len(boxes) == 0 {
			matches = append(matches, tableMatch{region: r, boxIdx: matched})
		}
	}
	return matches
}

// writeTableAnnotations annotates boxes at boxIdx with table cell grid
// information (R/C/H/SP).  Cells are offset by cropOff, grouped into a grid,
// and annotation fields are scaled back to PDF space for each box.
func writeTableAnnotations(boxes []pdf.TextBox, boxIdx []int, cells []pdf.TSRCell, scale, cropOffX, cropOffY float64, tb pdf.TableBuilder) {
	tableCells := make([]pdf.TSRCell, len(cells))
	for k := range cells {
		tableCells[k] = tbl.CellAddOffset(cells[k], cropOffX, cropOffY)
	}
	tblBoxes := make([]pdf.TextBox, len(boxIdx))
	for k, idx := range boxIdx {
		b := boxes[idx]
		tblBoxes[k] = pdf.TextBox{
			X0: b.X0 * scale, X1: b.X1 * scale,
			Top: b.Top * scale, Bottom: b.Bottom * scale,
			LayoutType: b.LayoutType,
			Text:       b.Text,
		}
	}
	annotGrid := tb.GroupCells(tableCells)
	tbl.AnnotateTableBoxes(tblBoxes, annotGrid)
	// Write back per-box annotations scaled to PDF space.
	for k, idx := range boxIdx {
		bp := &tblBoxes[k]
		boxes[idx].R = bp.R
		boxes[idx].RTop = bp.RTop / scale
		boxes[idx].RBott = bp.RBott / scale
		boxes[idx].H = bp.H
		boxes[idx].HTop = bp.HTop / scale
		boxes[idx].HBott = bp.HBott / scale
		boxes[idx].HLeft = bp.HLeft / scale
		boxes[idx].HRight = bp.HRight / scale
		boxes[idx].C = bp.C
		boxes[idx].CLeft = bp.CLeft / scale
		boxes[idx].CRight = bp.CRight / scale
		boxes[idx].SP = bp.SP
	}
}

// ── image helpers ──────────────────────────────────────────────────────

// table crop margin in DLA pixel space. Python uses MARGIN=10 in DPI 72

// annotateBoxLayouts sets LayoutType and LayoutNo on each box, matching
// Python's LayoutRecognizer.__call__ which assigns layout types in priority
// order (footer→header→…→equation) with an overlap threshold of 40% of the
// box's area.
//
// Python: _layouts_rec (pdf_parser.py:827) → LayoutRecognizer.__call__ →
//
//	for lt in priority_order: findLayout(lt)
//
// Each findLayout(ty): for each unannotated box, find the DLA region of
// type ty with max overlap ≥ 0.4 × box_area.  First type to match wins.
//
// CID-pattern boxes (e.g. "(cid:123)") are skipped as garbage.
// annotateBoxLayouts assigns LayoutType and LayoutNo to boxes based on DLA
// regions.  Returns the filtered slice (Python pops CID-garbled boxes and
// garbage-layout boxes at wrong positions — Go mirrors with compact).
// Also creates synthetic figure boxes for unmatched figure/equation regions.
func annotateBoxLayouts(boxes []pdf.TextBox, regions []pdf.DLARegion, scale float64, pageImgHeight float64) []pdf.TextBox {
	if len(regions) == 0 {
		return boxes
	}

	// Scale all regions to PDF space once.
	type scaledRegion struct {
		x0, y0, x1, y1 float64
		label          string
	}
	scaled := make([]scaledRegion, len(regions))
	for i, r := range regions {
		scaled[i] = scaledRegion{
			x0: r.X0 / scale, y0: r.Y0 / scale,
			x1: r.X1 / scale, y1: r.Y1 / scale,
			label: r.Label,
		}
	}

	// DLA confidence filter — matches Python's `score >= 0.4`.
	regionOK := make([]bool, len(regions))
	for i, r := range regions {
		regionOK[i] = r.Confidence >= 0.4 || !isGarbageLayoutType(r.Label)
	}

	// Pre-compute per-type index for each region (Python: matched index within
	// filtered layouts_of_type list). "text" regions get 0,1,2... independent
	// of "figure" regions.
	typeIndex := make([]int, len(regions))
	typeCounters := make(map[string]int)
	for j, r := range scaled {
		if regionOK[j] {
			typeIndex[j] = typeCounters[r.label]
			typeCounters[r.label]++
		}
	}

	// Track visited regions (Python: layout["visited"] = True).
	visited := make([]bool, len(regions))

	// Marks for Python-style pop removal.
	dropped := make([]bool, len(boxes))

	// Priority order matching Python's findLayout loop.
	priorityOrder := []string{
		pdf.LayoutTypeFooter, pdf.LayoutTypeHeader, pdf.LayoutTypeReference,
		pdf.DLALabelFigureCaption, pdf.DLALabelTableCaption,
		pdf.LayoutTypeTitle, pdf.LayoutTypeTable, pdf.LayoutTypeText,
		pdf.LayoutTypeFigure, pdf.LayoutTypeEquation,
	}
	for _, ty := range priorityOrder {
		for i := range boxes {
			if boxes[i].LayoutType != "" || dropped[i] {
				continue
			}
			// CID garbage: pop the box entirely (Python: bxs.pop(i)).
			if util.CIDPattern.MatchString(boxes[i].Text) {
				dropped[i] = true
				continue
			}
			boxArea := (boxes[i].X1 - boxes[i].X0) * (boxes[i].Bottom - boxes[i].Top)
			if boxArea <= 0 {
				continue
			}
			bestOverlap := 0.0
			bestJ := -1
			for j, r := range scaled {
				if r.label != ty || !regionOK[j] {
					continue
				}
				ix0 := math.Max(r.x0, boxes[i].X0)
				iy0 := math.Max(r.y0, boxes[i].Top)
				ix1 := math.Min(r.x1, boxes[i].X1)
				iy1 := math.Min(r.y1, boxes[i].Bottom)
				if ix0 < ix1 && iy0 < iy1 {
					ov := (ix1 - ix0) * (iy1 - iy0) / boxArea
					if ov > bestOverlap {
						bestOverlap = ov
						bestJ = j
					}
				}
			}
			if bestJ >= 0 && bestOverlap >= 0.4 {
				// Garbage layout not at page edge → pop (Python: bxs.pop(i)).
				if isGarbageLayoutType(ty) && pageImgHeight > 0 && !garbageKeepFeat(ty, boxes[i], pageImgHeight/scale) {
					dropped[i] = true
					continue
				}
				visited[bestJ] = true
				// Python: equation mapped to "figure" for layout_type
				if ty == pdf.LayoutTypeEquation {
					boxes[i].LayoutType = pdf.LayoutTypeFigure
				} else {
					boxes[i].LayoutType = ty
				}
				// Python: f"{layout_type}-{matched}" where matched is per-type index
				boxes[i].LayoutNo = fmt.Sprintf("%s-%d", ty, typeIndex[bestJ])
			}
		}
	}

	// Compact: remove popped boxes into a new backing array (Python
	// bxs.pop).  Allocating a fresh slice is deliberate: annotations were
	// set in-place on the input elements, and callers (enrichWithDeepDoc)
	// rely on positional stability of the original slice for their
	// write-back loop.  Reusing the input backing array would shift
	// survivors forward and break that index mapping.
	survivors := 0
	for i := range boxes {
		if !dropped[i] {
			survivors++
		}
	}
	compacted := make([]pdf.TextBox, 0, survivors)
	for i := range boxes {
		if !dropped[i] {
			compacted = append(compacted, boxes[i])
		}
	}
	boxes = compacted

	// Synthetic figure boxes for unmatched figure/equation regions (Python:
	// dla_cli.py:187-195). Use a fresh per-type counter for synthetic boxes.
	synthIdx := 0
	for j, r := range scaled {
		if !regionOK[j] || visited[j] {
			continue
		}
		if r.label != pdf.LayoutTypeFigure && r.label != pdf.LayoutTypeEquation {
			continue
		}
		boxes = append(boxes, pdf.TextBox{
			X0:         r.x0,
			X1:         r.x1,
			Top:        r.y0,
			Bottom:     r.y1,
			Text:       "",
			LayoutType: pdf.LayoutTypeFigure,
			LayoutNo:   fmt.Sprintf("figure-%d", synthIdx),
		})
		synthIdx++
	}

	return boxes
}

// garbageLayoutTypes matches Python's self.garbage_layouts.
var garbageLayoutTypes = map[string]bool{
	pdf.LayoutTypeFooter: true, pdf.LayoutTypeHeader: true, pdf.LayoutTypeReference: true,
}

func isGarbageLayoutType(ty string) bool {
	return garbageLayoutTypes[ty]
}

// garbageKeepFeat matches Python's keep_feats in LayoutRecognizer.__call__:
// footer near page bottom (>90% of page height) or header near page top (<10%)
// are real page decorations — keep them.  Others are DLA noise.
func garbageKeepFeat(ty string, box pdf.TextBox, pageImgHeight float64) bool {
	switch ty {
	case pdf.LayoutTypeFooter:
		return box.Bottom < pageImgHeight*0.9
	case pdf.LayoutTypeHeader:
		return box.Top > pageImgHeight*0.1
	}
	return false
}

func encodeImageToBase64PNG(img image.Image) (string, error) {
	data, err := util.EncodePNG(img)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}
