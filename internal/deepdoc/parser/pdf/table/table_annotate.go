package table

import (
	"fmt"
	"math"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/deepdoc/parser/pdf/util"
)

// ── region matching ────────────────────────────────────────────────────

// tableMatch pairs a DLA table region with the indices of boxes that overlap it.
type TableMatch struct {
	Region pdf.DLARegion
	BoxIdx []int
}

// ── region matching ────────────────────────────────────────────────────

func regionOverlapsBox(region pdf.DLARegion, box pdf.TextBox, scale float64) bool {
	rx0 := region.X0 / scale
	ry0 := region.Y0 / scale
	rx1 := region.X1 / scale
	ry1 := region.Y1 / scale
	scaledR := pdf.DLARegion{X0: rx0, Y0: ry0, X1: rx1, Y1: ry1}
	inter := util.OverlapInter(&scaledR, &box)
	boxArea := util.Area(&box)
	if boxArea <= 0 {
		return false
	}
	return inter/boxArea >= 0.4 // matches Python thr=0.4
}

// matchTableRegions pairs DLA table regions with boxes that overlap them.
// Each table region is matched if at least one box overlaps it (>40% of box
// area) or if there are no boxes at all (image-only PDF), matching Python's
// _table_transformer_job which processes every table DLA region.
func MatchTableRegions(boxes []pdf.TextBox, regions []pdf.DLARegion, scale float64) []TableMatch {
	var matches []TableMatch
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
			matches = append(matches, TableMatch{Region: r, BoxIdx: matched})
		}
	}
	return matches
}

// ── layout annotation ──────────────────────────────────────────────────

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
func AnnotateBoxLayouts(boxes []pdf.TextBox, regions []pdf.DLARegion, scale float64, pageImgHeight float64) []pdf.TextBox {
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

// ── garbage layout helpers ────────────────────────────────────────────
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

// writeTableAnnotations annotates boxes at boxIdx with table cell grid
// information (R/C/H/SP).  Cells are offset by cropOff, grouped into a grid,
// and annotation fields are scaled back to PDF space for each box.
func WriteTableAnnotations(boxes []pdf.TextBox, boxIdx []int, cells []pdf.TSRCell, scale, cropOffX, cropOffY float64, tb pdf.TableBuilder) {
	tableCells := make([]pdf.TSRCell, len(cells))
	for k := range cells {
		tableCells[k] = CellAddOffset(cells[k], cropOffX, cropOffY)
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
	AnnotateTableBoxes(tblBoxes, annotGrid)
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
