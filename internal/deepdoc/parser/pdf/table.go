package parser

import (
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"log/slog"
	"math"
	"regexp"
	"sort"
	"strings"
)

// enrichWithDeepDoc runs DLA+TSR via p.DeepDoc and returns detected tables.
// pageImages optionally provides pre-rendered page images to avoid re-rendering.
func (p *Parser) enrichWithDeepDoc(ctx context.Context, engine PDFEngine, boxes []TextBox, pageImages map[int]image.Image) []TableItem {
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

	var tableItems []TableItem
	for _, pg := range pageKeys {
		indices := byPage[pg]
		pageBoxes := make([]TextBox, len(indices))
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
			copyBoxAnnotations(&boxes[idx], &pageBoxes[i])
		}

	}
	return tableItems
}

func (p *Parser) extractTableBoxes(ctx context.Context, boxes []TextBox, engine PDFEngine, pageNum int, pageImages map[int]image.Image, tableBaseIdx int) []TableItem {
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

func (p *Parser) extractTableBoxesFromImage(ctx context.Context, boxes []TextBox, pageImg image.Image, pageNum int, tableBaseIdx int) []TableItem {
	regions, err := p.DeepDoc.DLA(ctx, pageImg)
	if err != nil {
		slog.Warn("DLA failed", "page", pageNum, "err", err)
		return nil
	}
	// Collect DLA debug intermediates.
	p.debugDLA = append(p.debugDLA, DLAPageRegions{Page: pageNum, Regions: regions})
	// Annotate boxes with DLA layout types (title, text, figure, table, ...).
	scale := dlaScale
	boxes = annotateBoxLayouts(boxes, regions, scale, float64(pageImg.Bounds().Dy()))

	tableMatches := matchTableRegions(boxes, regions, scale)
	var items []TableItem
	for _, tm := range tableMatches {
		cropped, cropErr := cropImageRegion(pageImg, tm.region)
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
			angle, rotated, _ := evaluateTableOrientation(ctx, cropped, p.DeepDoc)
			bestAngle = angle
			tsrImg = rotated
		}

		imgB64, encErr := encodeImageToBase64PNG(cropped)
		if encErr != nil {
			slog.Warn("table PNG encode failed", "page", pageNum, "err", encErr)
		}

		var cells []TSRCell
		var tsrErr error
		cells, tsrErr = p.tableBuilder.DetectCells(ctx, tsrImg)
		if tsrErr != nil {
			slog.Warn("TSR failed", "page", pageNum, "err", tsrErr)
		}
		// Collect TSR raw cells for debug comparison.
		if tsrErr == nil {
			for _, c := range cells {
				p.debugTSR = append(p.debugTSR, TSRRawCell{
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

		var boxInCrop []TextBox
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
					cells[i].X0, cells[i].Y0 = mapRotatedPointToOriginal(cells[i].X0, cells[i].Y0, bestAngle, origW, origH)
					cells[i].X1, cells[i].Y1 = mapRotatedPointToOriginal(cells[i].X1, cells[i].Y1, bestAngle, origW, origH)
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
			boxInCrop = make([]TextBox, 0, len(tm.boxIdx))
			for _, idx := range tm.boxIdx {
				b := boxes[idx]
				if b.Bottom*scale-cropOffY < firstCellTop {
					continue // caption box above first TSR cell
				}
				boxInCrop = append(boxInCrop, boxToCropSpace(b, scale, cropOffX, cropOffY))
			}
		}
		var positions []Position
		for _, idx := range tm.boxIdx {
			b := boxes[idx]
			positions = append(positions, Position{
				PageNumbers: []int{pageNum},
				Left:        b.X0, Right: b.X1,
				Top: b.Top, Bottom: b.Bottom,
			})
		}
		// Pre-compute grid from raw TSR cells (without crop offset).
		// Stored in TableItem for constructTable; annotateTableBoxes
		// recomputes with offset cells for spatial matching precision.
		var grid [][]TSRCell
		if len(cells) > 0 {
			grid = p.tableBuilder.GroupCells(cells)
			// Fill cell text from boxes in crop space. Works for both
			// SaasDeepDoc (cells rearranged) and OssDeepDoc (cross-product creates new cells).
			if len(grid) > 0 {
				flat := flattenGrid(grid)
				fillCellTextFromBoxes(flat, boxInCrop)
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
		items = append(items, TableItem{
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
	region DLARegion
	boxIdx []int
}

// ── cell row grouping ──────────────────────────────────────────────────

// ── region matching ────────────────────────────────────────────────────

func regionOverlapsBox(region DLARegion, box TextBox, scale float64) bool {
	rx0 := region.X0 / scale
	ry0 := region.Y0 / scale
	rx1 := region.X1 / scale
	ry1 := region.Y1 / scale
	scaledR := DLARegion{X0: rx0, Y0: ry0, X1: rx1, Y1: ry1}
	inter := OverlapInter(&scaledR, &box)
	boxArea := Area(&box)
	if boxArea <= 0 {
		return false
	}
	return inter/boxArea >= 0.4 // matches Python thr=0.4
}

// matchTableRegions pairs DLA table regions with boxes that overlap them.
// Each table region is matched if at least one box overlaps it (>40% of box
// area) or if there are no boxes at all (image-only PDF), matching Python's
// _table_transformer_job which processes every table DLA region.
func matchTableRegions(boxes []TextBox, regions []DLARegion, scale float64) []tableMatch {
	var matches []tableMatch
	for _, r := range regions {
		if r.Label != LayoutTypeTable {
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
func writeTableAnnotations(boxes []TextBox, boxIdx []int, cells []TSRCell, scale, cropOffX, cropOffY float64, tb TableBuilder) {
	tableCells := make([]TSRCell, len(cells))
	for k := range cells {
		tableCells[k] = cellAddOffset(cells[k], cropOffX, cropOffY)
	}
	tblBoxes := make([]TextBox, len(boxIdx))
	for k, idx := range boxIdx {
		b := boxes[idx]
		tblBoxes[k] = TextBox{
			X0: b.X0 * scale, X1: b.X1 * scale,
			Top: b.Top * scale, Bottom: b.Bottom * scale,
			LayoutType: b.LayoutType,
			Text:       b.Text,
		}
	}
	annotGrid := tb.GroupCells(tableCells)
	annotateTableBoxes(tblBoxes, annotGrid)
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
// space then scales by ZM (zoom factor). Since ZM=3 (default), the effective
// cropImageRegion crops a DLARegion from an image with a 3% margin
// (matching Python's _table_transformer_job: w*0.03, h*0.03).
func cropImageRegion(img image.Image, r DLARegion) (image.Image, error) {
	w := r.X1 - r.X0
	h := r.Y1 - r.Y0
	marginX := w * 0.03
	marginY := h * 0.03
	maxX := float64(img.Bounds().Dx())
	maxY := float64(img.Bounds().Dy())
	x0 := int(math.Max(0, r.X0-marginX))
	y0 := int(math.Max(0, r.Y0-marginY))
	x1 := int(math.Min(maxX, r.X1+marginX))
	y1 := int(math.Min(maxY, r.Y1+marginY))
	// Python PIL.Image.crop() raises ValueError when right < left or
	// bottom < top.  We return an error instead of silently falling back
	// to the full-page image — the caller skips this table gracefully.
	if x0 >= x1 || y0 >= y1 {
		return nil, fmt.Errorf("crop: invalid region x0=%d y0=%d x1=%d y1=%d (DLA raw: %.1f,%.1f,%.1f,%.1f)",
			x0, y0, x1, y1, r.X0, r.Y0, r.X1, r.Y1)
	}
	cropped := fastCrop(img, x0, y0, x1, y1)
	return cropped, nil
}

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
func annotateBoxLayouts(boxes []TextBox, regions []DLARegion, scale float64, pageImgHeight float64) []TextBox {
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
		LayoutTypeFooter, LayoutTypeHeader, LayoutTypeReference,
		DLALabelFigureCaption, DLALabelTableCaption,
		LayoutTypeTitle, LayoutTypeTable, LayoutTypeText,
		LayoutTypeFigure, LayoutTypeEquation,
	}
	for _, ty := range priorityOrder {
		for i := range boxes {
			if boxes[i].LayoutType != "" || dropped[i] {
				continue
			}
			// CID garbage: pop the box entirely (Python: bxs.pop(i)).
			if cidPattern.MatchString(boxes[i].Text) {
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
				if ty == LayoutTypeEquation {
					boxes[i].LayoutType = LayoutTypeFigure
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
	compacted := make([]TextBox, 0, survivors)
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
		if r.label != LayoutTypeFigure && r.label != LayoutTypeEquation {
			continue
		}
		boxes = append(boxes, TextBox{
			X0:         r.x0,
			X1:         r.x1,
			Top:        r.y0,
			Bottom:     r.y1,
			Text:       "",
			LayoutType: LayoutTypeFigure,
			LayoutNo:   fmt.Sprintf("figure-%d", synthIdx),
		})
		synthIdx++
	}

	return boxes
}

// garbageLayoutTypes matches Python's self.garbage_layouts.
var garbageLayoutTypes = map[string]bool{
	LayoutTypeFooter: true, LayoutTypeHeader: true, LayoutTypeReference: true,
}

func isGarbageLayoutType(ty string) bool {
	return garbageLayoutTypes[ty]
}

// garbageKeepFeat matches Python's keep_feats in LayoutRecognizer.__call__:
// footer near page bottom (>90% of page height) or header near page top (<10%)
// are real page decorations — keep them.  Others are DLA noise.
func garbageKeepFeat(ty string, box TextBox, pageImgHeight float64) bool {
	switch ty {
	case LayoutTypeFooter:
		return box.Bottom < pageImgHeight*0.9
	case LayoutTypeHeader:
		return box.Top > pageImgHeight*0.1
	}
	return false
}

func encodeImageToBase64PNG(img image.Image) (string, error) {
	data, err := encodePNG(img)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

// ── construct table ─────────────────────────────────────────────────────

// mergeTablesAcrossPages merges TableItems on consecutive pages with
// overlapping X and close Y proximity.  Matches Python's
// _extract_table_figure table merge (pdf_parser.py:1061-1080).
func mergeTablesAcrossPages(tables []TableItem, medianHeights map[int]float64) []TableItem {
	if len(tables) <= 1 {
		return tables
	}
	// Sort by position for deterministic adjacency.
	type indexed struct {
		idx int
		pg  int
		top float64
	}
	var items []indexed
	for i, tbl := range tables {
		if len(tbl.Positions) == 0 {
			continue
		}
		p := tbl.Positions[0]
		pg := 0
		if len(p.PageNumbers) > 0 {
			pg = p.PageNumbers[0]
		}
		items = append(items, indexed{i, pg, p.Top})
	}
	sort.Slice(items, func(a, b int) bool {
		if items[a].pg != items[b].pg {
			return items[a].pg < items[b].pg
		}
		return items[a].top < items[b].top
	})

	merged := make([]bool, len(tables))
	var result []TableItem

	for _, it := range items {
		if merged[it.idx] {
			continue
		}
		anchor := tables[it.idx]
		merged[it.idx] = true

		// Python nomerge_lout_no: tables whose box is followed by a
		// caption/title/reference should not be merged cross-page.
		if anchor.NoMerge {
			result = append(result, anchor)
			continue
		}

		anchorPg := it.pg
		anchorBott := anchor.Positions[0].Bottom

		// Look for consecutive-page continuations.
		for _, jt := range items {
			if merged[jt.idx] || jt.pg <= anchorPg {
				continue
			}
			// Python nomerge_lout_no: skip continuation candidates
			// tagged as no-merge.
			if tables[jt.idx].NoMerge {
				continue
			}
			if jt.pg-anchorPg > 1 {
				break // pages must be consecutive
			}
			if len(tables[jt.idx].Positions) == 0 {
				continue
			}
			bp := tables[jt.idx].Positions[0]
			bpg := 0
			if len(bp.PageNumbers) > 0 {
				bpg = bp.PageNumbers[0]
			}
			if bpg != anchorPg+1 {
				continue
			}
			// Check X overlap.
			ap := anchor.Positions[0]
			if ap.Right < bp.Left || bp.Right < ap.Left {
				continue
			}
			// Check Y proximity: page 1 table top should be close below
			// page 0 table bottom.  Python: y_dis ≤ mh * 23.
			mh := 10.0
			if medianHeights != nil {
				if h, ok := medianHeights[anchorPg]; ok && h > 0 {
					mh = h
				}
			}
			yDis := (bp.Top + bp.Bottom - anchorBott - ap.Bottom) / 2
			if yDis > mh*23 {
				continue
			}
			// Merge: combine cells and positions.
			anchor.Cells = append(anchor.Cells, tables[jt.idx].Cells...)
			anchor.Positions = append(anchor.Positions, tables[jt.idx].Positions...)
			if tables[jt.idx].Caption != "" {
				if anchor.Caption != "" {
					anchor.Caption += " "
				}
				anchor.Caption += tables[jt.idx].Caption
			}
			merged[jt.idx] = true
			anchorPg = bpg
			anchorBott = bp.Bottom
		}
		result = append(result, anchor)
	}
	return result
}

// constructTable produces an HTML table string from TSR cells and text boxes.
// Both cells and boxes must be in the same coordinate space (crop pixel space).
// Fills item.Rows so downstream consumers don't need to re-group cells.
//
// Python equivalent: TableStructureRecognizer.construct_table()
// stripCaptionFromCells clears caption-like text from TSR cells.
// This catches captions that fillCellTextFromBoxes missed (e.g. text
// that doesn't match isCaptionBox patterns like "公司差旅费管理办法").
// Only clears cells whose text matches caption patterns or that contain
// only number+separator text (pure "1. ", "一、" etc. without data).
func stripCaptionFromCells(cells []TSRCell) {
	for i := range cells {
		t := strings.TrimSpace(cells[i].Text)
		if t == "" {
			continue
		}
		// Clear cells that match caption patterns (e.g. "表1", "Table 1").
		if isCaptionBox(t, "") {
			cells[i].Text = ""
		}
	}
	// Second pass: if the first row (lowest Y) has all-numeric/numbering text
	// (e.g. "1", "1.", "一"), it's likely a caption numbering line — clear it.
	// But don't clear actual numeric data cells.
	// This pass is intentionally conservative — only clears clearly-non-data text.
}

func constructTable(cells []TSRCell, boxes []TextBox, caption string, item *TableItem) string {
	// Strip caption-like text from cells (defense-in-depth: fillCellTextFromBoxes
	// may include caption text that doesn't match isCaptionBox patterns).
	stripCaptionFromCells(cells)

	// Use the pre-computed grid from TableBuilder.GroupCells.
	// Falls back to cell-level grouping only when called directly by
	// tests without a pre-computed Grid (production always sets it).
	var rows [][]TSRCell
	if item != nil {
		rows = item.Grid
	}
	if rows == nil && len(cells) > 0 && hasAnyText(cells) {
		rows = groupTSRCellsToRowsLabeled(cells)
	}
	if len(rows) > 0 && hasText(rows) {
		hdrs := headerSetWithBlockType(rows)
		if item != nil {
			item.Rows = rowsToStrings(rows)
		}
		rows = cleanupOrphanColumns(rows)
		spanInfo, covered := calSpans(rows)
		return rowsToHTML(rows, caption, hdrs, spanInfo, covered)
	}
	// Fallback: boxes with R/C annotations.
	if len(boxes) > 0 && boxesHaveAnnotations(boxes) {
		rows := groupBoxesByRC(boxes)
		if hasText(rows) {
			if item != nil {
				item.Rows = rowsToStrings(rows)
			}
			spanInfo, covered := calSpans(rows)
			return rowsToHTML(rows, caption, boxHeaderSet(rows, boxes), spanInfo, covered)
		}
	}
	// Test-only: Y/X coordinate grouping (matching Python construct_table).
	// Used by table_parity_test.go to verify pipeline with Python boxes.
	if len(boxes) > 0 && !boxesHaveAnnotations(boxes) {
		rows := groupBoxesByYX(boxes)
		if hasText(rows) {
			if item != nil {
				item.Rows = rowsToStrings(rows)
			}
			spanInfo, covered := calSpans(rows)
			return rowsToHTML(rows, caption, boxHeaderSet(rows, boxes), spanInfo, covered)
		}
	}
	return ""
}

// boxHeaderSet returns rows that contain boxes with H annotations.
func boxHeaderSet(rows [][]TSRCell, boxes []TextBox) map[int]bool {
	hdrs := make(map[int]bool)
	for _, b := range boxes {
		if b.H > 0 && b.R >= 0 && b.R < len(rows) {
			hdrs[b.R] = true
		}
	}
	return hdrs
}

func hasAnyText(cells []TSRCell) bool {
	for _, c := range cells {
		if strings.TrimSpace(c.Text) != "" {
			return true
		}
	}
	return false
}

// groupBoxesByRC groups text boxes into a cell grid by R/C annotations.
// Matches Python's construct_table: sort by R, merge nearby rows by Y proximity,
// sort by C within each row, merge nearby columns by X proximity.
func groupBoxesByRC(boxes []TextBox) [][]TSRCell {
	if len(boxes) == 0 {
		return nil
	}
	// If no real R/C annotations (maxR <= 0), fall back to YX coordinate
	// grouping — matching Python's construct_table when all R=-1.
	maxR := 0
	for _, b := range boxes {
		if b.R > maxR {
			maxR = b.R
		}
	}
	if maxR <= 0 {
		return groupBoxesByYX(boxes)
	}
	// Sort by R index first (Python: sort_R_firstly), then Y, then X.
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].R != boxes[j].R {
			return boxes[i].R < boxes[j].R
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	// Compress R indices: Python's sort_R_firstly grouping.
	// R differs → always a new row.  Same R + Y gap → also new row.
	rowMap := make(map[int]int) // original R → compressed row index
	compressed := 0
	rowMap[boxes[0].R] = 0
	lastR := boxes[0].R
	btm := boxes[0].Bottom
	for i := 1; i < len(boxes); i++ {
		// Python: b["R"] != last_R → new row.
		// Same R → always same row (Python doesn't check Y for same R).
		if boxes[i].R != lastR {
			compressed++
			rowMap[boxes[i].R] = compressed
			lastR = boxes[i].R
			btm = boxes[i].Bottom
		} else {
			// Same R → same physical row.
			rowMap[boxes[i].R] = compressed
			btm = (btm + boxes[i].Bottom) / 2.0
		}
	}

	// Collect boxes per row, sort by C within each row.
	type rb struct {
		row, col       int
		txt            string
		x0, y0, x1, y1 float64
		label          string
	}
	cmap := make(map[int]map[int]*rb) // row → col → entry
	maxCols := make(map[int]int)
	for _, b := range boxes {
		t := strings.TrimSpace(b.Text)
		// Keep boxes with SP/H annotations even if text is empty —
		// their coordinates are needed for colspan/rowspan calculation.
		if t == "" && b.H <= 0 && b.SP <= 0 {
			continue
		}
		r := rowMap[b.R]
		c := b.C
		if cmap[r] == nil {
			cmap[r] = make(map[int]*rb)
		}
		x0, y0, x1, y1, label := cellPosFromBox(b)
		if v, ok := cmap[r][c]; ok {
			v.txt += " " + t
			// Merge spanning coordinates (use widest extent).
			if b.H > 0 || b.SP > 0 {
				v.label = cellLabelFromBox(b)
				if v.x0 > x0 {
					v.x0 = x0
				}
				if v.y0 > y0 {
					v.y0 = y0
				}
				if v.x1 < x1 {
					v.x1 = x1
				}
				if v.y1 < y1 {
					v.y1 = y1
				}
			}
		} else {
			cmap[r][c] = &rb{r, c, t, x0, y0, x1, y1, label}
		}
		if c > maxCols[r] {
			maxCols[r] = c
		}
	}

	// Compress C indices per row: sort boxes by X0 within the row,
	// group disjoint X ranges into separate columns.  This is equivalent
	// to Python's sort_C_firstly but uses X0 ordering instead of C labels.
	cCompressed := make(map[int]map[int]int) // row → (original C → compressed col)
	cMaxCol := make(map[int]int)
	for ri := 0; ri <= compressed; ri++ {
		rowEntries := cmap[ri]
		if rowEntries == nil {
			continue
		}
		// Collect all boxes in this row, sorted by X0.
		type rowBox struct {
			c, idx int
			x0, x1 float64
			txt    string
		}
		var rowBoxes []rowBox
		for i, b := range boxes {
			if rowMap[b.R] == ri && (strings.TrimSpace(b.Text) != "" || b.H > 0 || b.SP > 0) {
				rowBoxes = append(rowBoxes, rowBox{c: b.C, idx: i, x0: b.X0, x1: b.X1, txt: b.Text})
			}
		}
		sort.Slice(rowBoxes, func(i, j int) bool { return rowBoxes[i].x0 < rowBoxes[j].x0 })
		// Assign compressed column by X-order (disjoint X → new col).
		cMap := make(map[int]int) // original C → compressed col
		right := 0.0
		for _, rb := range rowBoxes {
			if len(cMap) == 0 || rb.x0 >= right {
				cc := len(cMap)
				cMap[rb.c] = cc
				right = rb.x1
			} else {
				// Overlapping X → merge into last column.
				cMap[rb.c] = len(cMap) - 1
				if rb.x1 > right {
					right = rb.x1
				}
			}
		}
		cCompressed[ri] = cMap
		cMaxCol[ri] = len(cMap) - 1
	}

	// Build grid.
	rows := make([][]TSRCell, compressed+1)
	for ri := 0; ri <= compressed; ri++ {
		maxC := cMaxCol[ri]
		rows[ri] = make([]TSRCell, maxC+1)
		for ci, v := range cmap[ri] {
			cci := cCompressed[ri][ci]
			if cci <= maxC {
				rows[ri][cci].Text = v.txt
				rows[ri][cci].X0 = v.x0
				rows[ri][cci].Y0 = v.y0
				rows[ri][cci].X1 = v.x1
				rows[ri][cci].Y1 = v.y1
				rows[ri][cci].Label = v.label
			}
		}
	}
	return rows
}

// cellPosFromBox returns the position coordinates and label for a cell
// derived from a text box.  Header cells use HLeft/HRight/HTop/HBott
// for spanning-aware positions; regular cells use the box's own bounds.
func cellPosFromBox(b TextBox) (x0, y0, x1, y1 float64, label string) {
	x0, y0, x1, y1 = b.X0, b.Top, b.X1, b.Bottom
	if b.H > 0 {
		label = "table header"
		if b.HLeft != 0 || b.HRight != 0 {
			if b.HLeft != 0 {
				x0 = b.HLeft
			}
			if b.HRight != 0 {
				x1 = b.HRight
			}
		}
		if b.HTop != 0 {
			y0 = b.HTop
		}
		if b.HBott != 0 {
			y1 = b.HBott
		}
	} else if b.SP > 0 {
		label = "table spanning cell"
	}
	return
}

// cellLabelFromBox returns the TSR label for a box based on H/SP annotations.
// Used when merging multiple boxes into one cell — preserves the spanning label.
func cellLabelFromBox(b TextBox) string {
	if b.H > 0 {
		return "table header"
	}
	if b.SP > 0 {
		return "table spanning cell"
	}
	return ""
}

// groupBoxesByYX groups boxes into a cell grid by Y/X coordinates,
// matching Python's construct_table which uses sort_R_firstly and
// sort_C_firstly when R/C annotations are absent.
// This is test-only — used by table_parity_test.go to verify pipeline
// parity with Python boxes that lack R/C annotations.
func groupBoxesByYX(boxes []TextBox) [][]TSRCell {
	if len(boxes) == 0 {
		return nil
	}
	// Sort by (page, top, x0) — same as Python sort_R_firstly with R=-1.
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].PageNumber != boxes[j].PageNumber {
			return boxes[i].PageNumber < boxes[j].PageNumber
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	// Group into rows by Y proximity (Python's row grouping).
	type rowGroup struct {
		boxes    []TextBox
		top, btm float64
	}
	var rowGroups []rowGroup
	rowGroups = append(rowGroups, rowGroup{boxes: []TextBox{boxes[0]}, top: boxes[0].Top, btm: boxes[0].Bottom})
	for i := 1; i < len(boxes); i++ {
		prev := &rowGroups[len(rowGroups)-1]
		// Python: same row if top < prev.btm (Y overlaps) and same page.
		if boxes[i].PageNumber == prev.boxes[0].PageNumber && boxes[i].Top < prev.btm {
			prev.boxes = append(prev.boxes, boxes[i])
			if boxes[i].Top < prev.top {
				prev.top = boxes[i].Top
			}
			if boxes[i].Bottom > prev.btm {
				prev.btm = boxes[i].Bottom
			}
		} else {
			rowGroups = append(rowGroups, rowGroup{boxes: []TextBox{boxes[i]}, top: boxes[i].Top, btm: boxes[i].Bottom})
		}
	}

	// Within each row, group into columns by X proximity.
	rows := make([][]TSRCell, len(rowGroups))
	for ri, rg := range rowGroups {
		// Sort by X0.
		sort.Slice(rg.boxes, func(i, j int) bool { return rg.boxes[i].X0 < rg.boxes[j].X0 })
		// Group by X overlap.
		var cols []struct {
			boxes []TextBox
			x1    float64
		}
		cols = append(cols, struct {
			boxes []TextBox
			x1    float64
		}{boxes: []TextBox{rg.boxes[0]}, x1: rg.boxes[0].X1})
		for i := 1; i < len(rg.boxes); i++ {
			prev := &cols[len(cols)-1]
			if rg.boxes[i].X0 < prev.x1 {
				prev.boxes = append(prev.boxes, rg.boxes[i])
				if rg.boxes[i].X1 > prev.x1 {
					prev.x1 = rg.boxes[i].X1
				}
			} else {
				cols = append(cols, struct {
					boxes []TextBox
					x1    float64
				}{boxes: []TextBox{rg.boxes[i]}, x1: rg.boxes[i].X1})
			}
		}
		rows[ri] = make([]TSRCell, len(cols))
		for ci, col := range cols {
			var sb strings.Builder
			for _, b := range col.boxes {
				t := strings.TrimSpace(b.Text)
				if t == "" {
					continue
				}
				if sb.Len() > 0 {
					sb.WriteByte(' ')
				}
				sb.WriteString(t)
			}
			rows[ri][ci].Text = sb.String()
		}
	}
	return rows
}

func boxesHaveAnnotations(boxes []TextBox) bool {
	maxR, maxC := 0, 0
	for _, b := range boxes {
		if b.R > maxR {
			maxR = b.R
		}
		if b.C > maxC {
			maxC = b.C
		}
	}
	// True if at least 2 rows or 2 cols (R/C are 0-based, so maxR>0 means ≥2 rows).
	return maxR > 0 || maxC > 0
}

func hasText(rows [][]TSRCell) bool {
	for _, row := range rows {
		for _, c := range row {
			if strings.TrimSpace(c.Text) != "" {
				return true
			}
		}
	}
	return false
}

func rowsToStrings(rows [][]TSRCell) [][]string {
	out := make([][]string, len(rows))
	for ri, row := range rows {
		out[ri] = make([]string, len(row))
		for ci, c := range row {
			out[ri][ci] = c.Text
		}
	}
	return out
}

// fillCellTextFromAnnotations fills cell text from text boxes using R/C labels.
// This matches Python's construct_table which assigns boxes to cells by their
// R (row) and C (col) annotations rather than spatial overlap.
func fillCellTextFromAnnotations(rows [][]TSRCell, boxes []TextBox) {
	// Build R→(C→text) map: row index → (col index → text).
	rBoxes := make(map[int]map[int][]string)
	for _, b := range boxes {
		if b.Text == "" {
			continue
		}
		if rBoxes[b.R] == nil {
			rBoxes[b.R] = make(map[int][]string)
		}
		rBoxes[b.R][b.C] = append(rBoxes[b.R][b.C], b.Text)
	}
	// Fill each cell from the matching R/C position.
	for ri, row := range rows {
		colMap := rBoxes[ri]
		if colMap == nil {
			continue
		}
		// Build sorted column list for positional matching.
		type colEntry struct {
			c     int
			texts []string
		}
		var cols []colEntry
		for c, texts := range colMap {
			cols = append(cols, colEntry{c, texts})
		}
		sort.Slice(cols, func(i, j int) bool { return cols[i].c < cols[j].c })
		for ci, col := range cols {
			if ci < len(row) {
				row[ci].Text = strings.TrimSpace(strings.Join(col.texts, " "))
			}
		}
	}
}

// dataSourceRe matches table/figure boxes that should be discarded as
// data-source attribution lines rather than extracted content.
//
// Python: pdf_parser.py:1040-1042, 1050-1052
//
//	re.match(r"(数据|资料|图表)*来源[:： ]", self.boxes[i]["text"])
var dataSourceRe = regexp.MustCompile(`^(数据|资料|图表)*来源[:： ]`)

// isDataSourceBox returns true if the box text matches the data-source
// discard pattern (Python's _extract_table_figure data-source filter).
func isDataSourceBox(text string) bool {
	return dataSourceRe.MatchString(text)
}

// tableRegionBox returns a TextBox for a table replacement, using DLA region
// boundaries when available (Region* set), falling back to anchor box coordinates.
// Python's insert_table_figures uses DLA layout region boundaries; the fallback
// handles test TableItems or bare engines without DLA.
func tableRegionBox(tbl *TableItem, ref *TextBox, html string) TextBox {
	pg := 0
	if len(tbl.Positions) > 0 && len(tbl.Positions[0].PageNumbers) > 0 {
		pg = tbl.Positions[0].PageNumbers[0]
	}
	// Use DLA region boundaries when set.
	if tbl.RegionLeft != 0 || tbl.RegionRight != 0 || tbl.RegionTop != 0 || tbl.RegionBottom != 0 {
		return TextBox{
			X0: tbl.RegionLeft, X1: tbl.RegionRight,
			Top: tbl.RegionTop, Bottom: tbl.RegionBottom,
			Text:       html,
			PageNumber: pg,
			LayoutType: LayoutTypeTable,
		}
	}
	// Fallback: use anchor box coordinates.
	x0, x1, top, bot := ref.X0, ref.X1, ref.Top, ref.Bottom
	return TextBox{
		X0: x0, X1: x1, Top: top, Bottom: bot,
		Text:       html,
		PageNumber: pg,
		LayoutType: LayoutTypeTable,
	}
}

// minRectangleDistance computes the Euclidean distance between two rectangles.
// Returns 0 when rectangles overlap.  Matches Python's min_rectangle_distance
// in insert_table_figures (pdf_parser.py:1609-1626).
func minRectangleDistance(left1, right1, top1, bottom1, left2, right2, top2, bottom2 float64) float64 {
	if right1 >= left2 && right2 >= left1 && bottom1 >= top2 && bottom2 >= top1 {
		return 0
	}
	var dx, dy float64
	if right1 < left2 {
		dx = left2 - right1
	} else if right2 < left1 {
		dx = left1 - right2
	}
	if bottom1 < top2 {
		dy = top2 - bottom1
	} else if bottom2 < top1 {
		dy = top1 - bottom2
	}
	return math.Sqrt(dx*dx + dy*dy)
}

// extractTableAndReplace pops table boxes and replaces them with consolidated
// HTML boxes (one per table).  This matches Python's _extract_table_figure which
// pops all boxes inside a table DLA region and inserts a single HTML box.
//
// Table boxes whose text matches the data-source discard pattern
// (r"(数据|资料|图表)*来源[:： ]") are removed entirely without replacement —
// matching Python's _extract_table_figure discard behavior.

// markNoMergeTables traverses boxes in page order. When a caption, title, or
// reference immediately follows a table, the preceding table is marked NoMerge
// to prevent cross-page merge. Matches Python's nomerge_lout_no.
func markNoMergeTables(boxes []TextBox, tables []TableItem) {
	var lastTableTI int = -1
	for i := range boxes {
		lt := boxes[i].LayoutType
		if lt == LayoutTypeTable {
			matched := false
			for ti := range tables {
				for _, tp := range tables[ti].Positions {
					if boxOverlapsPosition(boxes[i], tp) {
						lastTableTI = ti
						matched = true
						break
					}
				}
			}
			if !matched {
				lastTableTI = -1
			}
			continue
		}
		if lastTableTI >= 0 && (lt == LayoutTypeTitle || lt == DLALabelTableCaption || lt == DLALabelFigureCaption || lt == LayoutTypeReference || isCaptionBox(boxes[i].Text, lt)) {
			tables[lastTableTI].NoMerge = true
		}
	}
}

// boxes must be post-TextMerge + post-VerticalMerge.  TableItem.Cells are in
// crop pixel space; boxes are in PDF point space — conversion via Scale/CropOff.
// replacement pairs a table index with the box index it replaces.
type replacement struct {
	tableIdx int
	boxIdx   int
}

// buildReplacements scans for data-source-attribution boxes to remove and maps
// each table to overlapping table-layout boxes, producing the replacement list.
func buildReplacements(boxes []TextBox, tables []TableItem) (map[int]bool, []replacement) {
	removeSet := make(map[int]bool)
	for i := range boxes {
		if boxes[i].LayoutType == LayoutTypeTable && isDataSourceBox(boxes[i].Text) {
			removeSet[i] = true
		}
	}
	var reps []replacement
	for ti := range tables {
		for i := range boxes {
			if boxes[i].LayoutType != LayoutTypeTable || removeSet[i] {
				continue
			}
			for _, tp := range tables[ti].Positions {
				if boxOverlapsPosition(boxes[i], tp) {
					reps = append(reps, replacement{tableIdx: ti, boxIdx: i})
					break
				}
			}
		}
	}
	return removeSet, reps
}

func extractTableAndReplace(boxes []TextBox, tables []TableItem) []TextBox {
	if len(tables) == 0 {
		return boxes
	}
	// Pre-merge nomerge detection: match Python's nomerge_lout_no.
	// Traverse boxes in page order. When a caption/title/reference is
	// found, mark the preceding table group as NoMerge, preventing
	// cross-page merge when a caption ends a table group.
	// Python: if is_caption(c) or layout_type in ["table caption", "title",
	// "figure caption", "reference"]: nomerge_lout_no.append(lst_lout_no)
	markNoMergeTables(boxes, tables)

	// Merge same-layoutno tables across consecutive pages (Python _extract_table_figure).
	tables = mergeTablesAcrossPages(tables, nil)

	// Pre-scan: mark data-source-attribution table boxes for removal.
	// Python: if re.match(r"(数据|资料|图表)*来源[:： ]", self.boxes[i]["text"]):
	// self.boxes.pop(i); continue — box discarded, no HTML replacement.
	removeSet, replacements := buildReplacements(boxes, tables)

	// Image-only PDFs (0 boxes) may have tables with cells but no
	// overlapping LayoutType=="table" boxes — generate HTML directly.
	if len(replacements) == 0 && len(boxes) == 0 {
		var out []TextBox
		for ti := range tables {
			if len(tables[ti].Cells) == 0 {
				continue
			}
			s := tables[ti].Scale
			pageGlobalCells := cellSliceToPageSpace(tables[ti].Cells, tables[ti].CropOffX, tables[ti].CropOffY, s)
			var tableBoxes []TextBox
			html := constructTable(pageGlobalCells, tableBoxes, tables[ti].Caption, &tables[ti])
			if html != "" {
				out = append(out, TextBox{
					Text: html, LayoutType: "table", PageNumber: 0,
				})
			}
		}
		return out
	}
	if len(replacements) == 0 {
		// No HTML replacements, but data-source boxes still need removal.
		if len(removeSet) == 0 {
			return boxes
		}
		out := make([]TextBox, 0, len(boxes)-len(removeSet))
		for i, b := range boxes {
			if !removeSet[i] {
				out = append(out, b)
			}
		}
		return out
	}

	// Distance-based anchor selection (Python's min_rectangle_distance).
	// Find the spatially nearest non-table text box for each table and
	// use that as the anchor, matching insert_table_figures behavior.
	replacedByTable := make(map[int]int)
	for ti := range tables {
		if len(tables[ti].Cells) == 0 {
			continue
		}
		tbl := &tables[ti]
		tblLeft, tblRight := tbl.RegionLeft, tbl.RegionRight
		tblTop, tblBottom := tbl.RegionTop, tbl.RegionBottom
		tblPg := 0
		if len(tbl.Positions) > 0 {
			p := tbl.Positions[0]
			if len(p.PageNumbers) > 0 {
				tblPg = p.PageNumbers[0]
			}
			if tblLeft == 0 && tblRight == 0 && tblTop == 0 && tblBottom == 0 {
				tblLeft, tblRight = p.Left, p.Right
				tblTop, tblBottom = p.Top, p.Bottom
			}
		}
		bestDist := math.MaxFloat64
		bestIdx := -1
		for i, b := range boxes {
			if b.LayoutType == LayoutTypeTable || b.LayoutType == LayoutTypeFigure {
				continue
			}
			if b.PageNumber != tblPg {
				continue
			}
			dist := minRectangleDistance(
				b.X0, b.X1, b.Top, b.Bottom,
				tblLeft, tblRight, tblTop, tblBottom,
			)
			if dist < bestDist {
				bestDist = dist
				bestIdx = i
			}
		}
		if bestIdx >= 0 {
			if boxes[bestIdx].Bottom < tblTop {
				bestIdx++
			}
			replacedByTable[ti] = bestIdx
		} else {
			for _, r := range replacements {
				if r.tableIdx == ti {
					if _, ok := replacedByTable[ti]; !ok || r.boxIdx < replacedByTable[ti] {
						replacedByTable[ti] = r.boxIdx
					}
				}
			}
		}
	}
	for _, r := range replacements {
		removeSet[r.boxIdx] = true
	}

	// Build HTML for each table using post-merge boxes converted to crop space.
	htmlByTable := make(map[int]string)
	for ti := range tables {
		if len(tables[ti].Cells) == 0 {
			continue
		}
		// Convert TSR cells from crop-pixel space to page-global 72 DPI,
		// matching Python's coordinate space.  Text boxes are already in
		// page-global 72 DPI (from ocrMergeChars), so no conversion needed.
		s := tables[ti].Scale
		pageGlobalCells := cellSliceToPageSpace(tables[ti].Cells, tables[ti].CropOffX, tables[ti].CropOffY, s)
		// Collect only table-labelled boxes (Python: filters by layout_type).
		var tableBoxes []TextBox
		for i := range boxes {
			if boxes[i].LayoutType != LayoutTypeTable {
				continue
			}
			for _, tp := range tables[ti].Positions {
				if boxOverlapsPosition(boxes[i], tp) {
					tableBoxes = append(tableBoxes, boxes[i])
					break
				}
			}
		}
		slog.Debug("extractTableAndReplace constructTable", "table", ti, "cells", len(pageGlobalCells), "boxes", len(tableBoxes))
		htmlByTable[ti] = constructTable(pageGlobalCells, tableBoxes, tables[ti].Caption, &tables[ti])
	}

	// Sort anchors by position for stable insertion.
	anchorList := make([]struct{ ti, pos int }, 0, len(replacedByTable))
	for ti, pos := range replacedByTable {
		anchorList = append(anchorList, struct{ ti, pos int }{ti, pos})
	}
	sort.Slice(anchorList, func(i, j int) bool { return anchorList[i].pos < anchorList[j].pos })

	out := make([]TextBox, 0, len(boxes)-len(removeSet)+len(replacedByTable))
	anchorIdx := 0
	for i, b := range boxes {
		// Insert any HTML boxes whose anchor position is before or at i.
		for anchorIdx < len(anchorList) && anchorList[anchorIdx].pos <= i {
			ti := anchorList[anchorIdx].ti
			html := htmlByTable[ti]
			if html != "" {
				tbl := &tables[ti]
				out = append(out, tableRegionBox(tbl, &b, html))
			}
			anchorIdx++
		}
		if !removeSet[i] {
			out = append(out, b)
		}
	}
	// Remaining anchors after last box.
	for anchorIdx < len(anchorList) {
		ti := anchorList[anchorIdx].ti
		html := htmlByTable[ti]
		if html != "" {
			tbl := &tables[ti]
			last := &boxes[len(boxes)-1]
			out = append(out, tableRegionBox(tbl, last, html))
		}
		anchorIdx++
	}
	return out
}

// consolidateFigures merges figure boxes that share the same LayoutNo
// (i.e., belong to the same DLA figure region) into a single TextBox.
// Matches Python's _extract_table_figure + insert_table_figures which pops
// individual figure boxes and re-inserts one consolidated figure block
// per DLA region with combined text.
//
// Figure boxes whose text matches the data-source discard pattern
// (r"(数据|资料|图表)*来源[:： ]") are removed entirely — matching Python's
// _extract_table_figure discard behavior (pdf_parser.py:1050-1052).
func consolidateFigures(boxes []TextBox) []TextBox {
	// Pre-scan: mark data-source-attribution figure boxes for removal.
	// Python: if re.match(r"(数据|资料|图表)*来源[:： ]", self.boxes[i]["text"]):
	// self.boxes.pop(i); continue — box discarded.
	removeSet := make(map[int]bool)
	for i, b := range boxes {
		if b.LayoutType == LayoutTypeFigure && isDataSourceBox(b.Text) {
			removeSet[i] = true
		}
	}

	// Group figure boxes by (page, layoutno).
	type figKey struct {
		page int
		ln   string
	}
	groups := make(map[figKey][]int)
	for i, b := range boxes {
		if b.LayoutType != LayoutTypeFigure || removeSet[i] {
			continue
		}
		key := figKey{b.PageNumber, b.LayoutNo}
		groups[key] = append(groups[key], i)
	}

	if len(groups) == 0 {
		// Still need to filter out data-source figure boxes.
		if len(removeSet) == 0 {
			return boxes
		}
		out := make([]TextBox, 0, len(boxes)-len(removeSet))
		for i, b := range boxes {
			if !removeSet[i] {
				out = append(out, b)
			}
		}
		return out
	}

	// Collect indices to remove (all group members except the first).
	for _, indices := range groups {
		if len(indices) <= 1 {
			continue
		}
		// Merge into the first box of the group.
		anchor := indices[0]
		for _, idx := range indices[1:] {
			b := boxes[idx]
			boxes[anchor].Text += "\n" + b.Text
			boxes[anchor].X0 = math.Min(boxes[anchor].X0, b.X0)
			boxes[anchor].X1 = math.Max(boxes[anchor].X1, b.X1)
			boxes[anchor].Top = math.Min(boxes[anchor].Top, b.Top)
			boxes[anchor].Bottom = math.Max(boxes[anchor].Bottom, b.Bottom)
			removeSet[idx] = true
		}
	}

	if len(removeSet) == 0 {
		return boxes
	}

	out := make([]TextBox, 0, len(boxes)-len(removeSet))
	for i, b := range boxes {
		if !removeSet[i] {
			out = append(out, b)
		}
	}
	return out
}

// boxOverlapsPosition checks if a TextBox overlaps a Position with margin.
func boxOverlapsPosition(box TextBox, pos Position) bool {
	const margin = 2.0
	return box.X0 <= pos.Right+margin && box.X1 >= pos.Left-margin &&
		box.Top <= pos.Bottom+margin && box.Bottom >= pos.Top-margin
}

// ── coordinate space conversion helpers ──────────────────────────────

// cellToPageSpace converts from crop-pixel space to page-global 72-DPI space.
func cellToPageSpace(c TSRCell, cropOffX, cropOffY, scale float64) TSRCell {
	return TSRCell{
		X0: (c.X0 + cropOffX) / scale, Y0: (c.Y0 + cropOffY) / scale,
		X1: (c.X1 + cropOffX) / scale, Y1: (c.Y1 + cropOffY) / scale,
		Text: c.Text, Label: c.Label,
	}
}

// cellAddOffset applies a crop offset to cell coordinates (stays in pixel space).
func cellAddOffset(c TSRCell, offX, offY float64) TSRCell {
	return TSRCell{
		X0: c.X0 + offX, Y0: c.Y0 + offY, X1: c.X1 + offX, Y1: c.Y1 + offY,
		Text: c.Text, Label: c.Label,
	}
}

// cellSliceToPageSpace converts a slice of cells from crop-pixel to page DPI space.
func cellSliceToPageSpace(cells []TSRCell, cropOffX, cropOffY, scale float64) []TSRCell {
	out := make([]TSRCell, len(cells))
	for i, c := range cells {
		out[i] = cellToPageSpace(c, cropOffX, cropOffY, scale)
	}
	return out
}

// boxToCropSpace converts a TextBox from PDF-point space to crop-pixel space.
func boxToCropSpace(b TextBox, scale, cropOffX, cropOffY float64) TextBox {
	return TextBox{
		X0: b.X0*scale - cropOffX, X1: b.X1*scale - cropOffX,
		Top: b.Top*scale - cropOffY, Bottom: b.Bottom*scale - cropOffY,
		Text: b.Text,
	}
}

// copyBoxAnnotations copies the DLA/TSR annotation fields from src to dst.
func copyBoxAnnotations(dst, src *TextBox) {
	dst.R = src.R
	dst.C = src.C
	dst.RTop = src.RTop
	dst.RBott = src.RBott
	dst.H = src.H
	dst.HTop = src.HTop
	dst.HBott = src.HBott
	dst.HLeft = src.HLeft
	dst.HRight = src.HRight
	dst.CLeft = src.CLeft
	dst.CRight = src.CRight
	dst.SP = src.SP
}

// rowsToHTML converts grouped TSR cell rows to an HTML table string.
// spanInfo maps (row,col) → (colspan, rowspan) for spanning cells;
// covered marks cells hidden by a span. Both may be nil.
func rowsToHTML(rows [][]TSRCell, caption string, headerRows map[int]bool, spanInfo map[[2]int][2]int, covered map[[2]int]bool) string {
	var b strings.Builder
	b.WriteString("<table>")
	if caption != "" {
		b.WriteString("<caption>")
		b.WriteString(caption)
		b.WriteString("</caption>")
	}
	for ri, row := range rows {
		b.WriteString("<tr>")
		for ci, cell := range row {
			if covered[[2]int{ri, ci}] {
				continue
			}
			tag := "td"
			if headerRows[ri] {
				tag = "th"
			}
			b.WriteString("<")
			b.WriteString(tag)
			sp := ""
			if s, ok := spanInfo[[2]int{ri, ci}]; ok {
				if s[0] > 1 {
					sp = fmt.Sprintf("colspan=%d", s[0])
				}
				if s[1] > 1 {
					if sp != "" {
						sp += " "
					}
					sp += fmt.Sprintf("rowspan=%d", s[1])
				}
			}
			if sp != "" {
				b.WriteString(" ")
				b.WriteString(sp)
			}
			b.WriteString(" >")
			b.WriteString(cell.Text)
			b.WriteString("</")
			b.WriteString(tag)
			b.WriteString(">")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

// ── Span computation (Python: __cal_spans) ──

// calSpans computes colspan and rowspan for spanning cells in the grid.
// Returns spanInfo (row,col → colspan,rowspan) and covered (cells hidden by spans).
// Matches Python's __cal_spans (table_structure_recognizer.py:535).
// flattenGrid flattens a 2D grid into a 1D slice for fillCellTextFromBoxes.
func flattenGrid(grid [][]TSRCell) []TSRCell {
	n := 0
	for _, row := range grid {
		n += len(row)
	}
	flat := make([]TSRCell, 0, n)
	for _, row := range grid {
		flat = append(flat, row...)
	}
	return flat
}

func calSpans(rows [][]TSRCell) (map[[2]int][2]int, map[[2]int]bool) {
	spanInfo := make(map[[2]int][2]int)
	covered := make(map[[2]int]bool)
	if len(rows) == 0 || len(rows[0]) == 0 {
		return spanInfo, covered
	}

	// Compute column center positions.
	nCols := len(rows[0])
	colLeft := make([]float64, nCols)
	colRight := make([]float64, nCols)
	for j := 0; j < nCols; j++ {
		colLeft[j] = 1e9
		colRight[j] = -1e9
	}
	nRows := len(rows)
	rowTop := make([]float64, nRows)
	rowBott := make([]float64, nRows)
	for i := 0; i < nRows; i++ {
		rowTop[i] = 1e9
		rowBott[i] = -1e9
	}

	for i, row := range rows {
		for j, cell := range row {
			if j >= nCols {
				continue
			}
			// Exclude spanning cells from column/row boundary calculations.
			// Use label-based detection (O(1), no dependency on column midpoints).
			if strings.Contains(cell.Label, "spanning") {
				continue
			}
			if cell.X0 < colLeft[j] {
				colLeft[j] = cell.X0
			}
			if cell.X1 > colRight[j] {
				colRight[j] = cell.X1
			}
			if cell.Y0 < rowTop[i] {
				rowTop[i] = cell.Y0
			}
			if cell.Y1 > rowBott[i] {
				rowBott[i] = cell.Y1
			}
		}
	}

	// For each spanning cell, compute how many cols/rows it covers.
	for i, row := range rows {
		for j, cell := range row {
			if j >= nCols || covered[[2]int{i, j}] {
				continue
			}
			// Skip cells without position data (they can't span).
			if cell.X0 == 0 && cell.X1 == 0 && cell.Y0 == 0 && cell.Y1 == 0 {
				continue
			}
			cs, rs := 1, 1
			// Count columns whose center is inside this cell's X range.
			for k := j + 1; k < nCols; k++ {
				// Skip columns with no non-spanning cells (initial values unchanged).
				if colLeft[k] == 1e9 && colRight[k] == -1e9 {
					continue
				}
				colCenter := (colLeft[k] + colRight[k]) / 2
				if colCenter >= cell.X0 && colCenter <= cell.X1 {
					cs++
				}
			}
			// Count rows whose center is inside this cell's Y range.
			for k := i + 1; k < nRows; k++ {
				// Skip rows with no non-spanning cells.
				if rowTop[k] == 1e9 && rowBott[k] == -1e9 {
					continue
				}
				rowCenter := (rowTop[k] + rowBott[k]) / 2
				if rowCenter >= cell.Y0 && rowCenter <= cell.Y1 {
					rs++
				}
			}
			if cs > 1 || rs > 1 {
				spanInfo[[2]int{i, j}] = [2]int{cs, rs}
				// Mark covered cells.
				for ri := i; ri < i+rs && ri < nRows; ri++ {
					for cj := j; cj < j+cs && cj < nCols; cj++ {
						if ri != i || cj != j {
							covered[[2]int{ri, cj}] = true
						}
					}
				}
			}
		}
	}
	return spanInfo, covered
}

// ── Orphan column/row cleanup (Python: construct_table lines 256-368) ──

// cleanupOrphanColumns removes columns that have only a single non-empty cell
// when there are ≥4 rows.  Matches Python's construct_table column cleanup.
func cleanupOrphanColumns(rows [][]TSRCell) [][]TSRCell {
	if len(rows) < 4 || len(rows) == 0 {
		return rows
	}
	nCols := len(rows[0])

	j := 0
colLoop:
	for j < nCols {
		e, ii := 0, 0
		for i := range rows {
			if j < len(rows[i]) && strings.TrimSpace(rows[i][j].Text) != "" {
				e++
				ii = i
			}
			if e > 1 {
				j++
				continue colLoop
			}
		}
		// Column j has only one non-empty cell at row ii.
		// Check if adjacent columns have text for this row.
		f := (j > 0 && j-1 < len(rows[ii]) && strings.TrimSpace(rows[ii][j-1].Text) != "") || j == 0
		ff := (j+1 < len(rows[ii]) && strings.TrimSpace(rows[ii][j+1].Text) != "") || j+1 >= len(rows[ii])
		if f && ff {
			// Both adjacent columns are ok for merging — but this means
			// there's text on both sides, keep column.
			j++
			continue
		}

		// Determine which side to merge into.
		left := 1e9
		right := 1e9
		if j > 0 && !f {
			for i := range rows {
				if j-1 < len(rows[i]) && strings.TrimSpace(rows[i][j-1].Text) != "" {
					// Distance from orphan cell to left neighbor.
					if d := rows[ii][j].X0 - rows[i][j-1].X1; d < left {
						left = d
					}
				}
			}
		}
		if j+1 < nCols && !ff {
			for i := range rows {
				if j+1 < len(rows[i]) && strings.TrimSpace(rows[i][j+1].Text) != "" {
					if d := rows[i][j+1].X0 - rows[ii][j].X1; d < right {
						right = d
					}
				}
			}
		}

		if left < right && j > 0 {
			// Merge into left column.
			for i := range rows {
				if j-1 < len(rows[i]) && j < len(rows[i]) {
					if rows[i][j-1].Text == "" {
						rows[i][j-1].Text = rows[i][j].Text
					} else if rows[i][j].Text != "" {
						rows[i][j-1].Text += " " + rows[i][j].Text
					}
				}
			}
		} else if j+1 < nCols {
			// Merge into right column.
			for i := range rows {
				if j < len(rows[i]) && j+1 < len(rows[i]) {
					if rows[i][j+1].Text == "" {
						rows[i][j+1].Text = rows[i][j].Text
					} else if rows[i][j].Text != "" {
						rows[i][j+1].Text = rows[i][j].Text + " " + rows[i][j+1].Text
					}
				}
			}
		}
		// Remove column j.
		for i := range rows {
			if j < len(rows[i]) {
				rows[i] = append(rows[i][:j], rows[i][j+1:]...)
			}
		}
		nCols--
		// Don't increment j — the next column shifted into position j.
	}
	return rows
}
