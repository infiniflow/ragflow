package table

import (
	"fmt"
	"math"
	"sort"

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

// annRegion is a layout region in PDF space, used internally by
// AnnotateBoxLayouts. It carries the fields needed for cleanup, sort, and
// annotation bookkeeping.
type annRegion struct {
	x0, y0, x1, y1 float64
	label          string
	score          float64
	visited        bool
	typeIndex      int
}

// regionIntersect returns the intersection area of two regions, or 0.
func regionIntersect(a, b annRegion) float64 {
	ix0 := math.Max(a.x0, b.x0)
	iy0 := math.Max(a.y0, b.y0)
	ix1 := math.Min(a.x1, b.x1)
	iy1 := math.Min(a.y1, b.y1)
	if ix0 < ix1 && iy0 < iy1 {
		return (ix1 - ix0) * (iy1 - iy0)
	}
	return 0
}

// regionArea returns the area of a region, or 0 if degenerate.
func regionArea(a annRegion) float64 {
	w := a.x1 - a.x0
	h := a.y1 - a.y0
	if w <= 0 || h <= 0 {
		return 0
	}
	return w * h
}

// overlapRatio returns intersection / area(a), matching
// Recognizer.overlapped_area with ratio=True (recognizer.py:106-122).
func overlapRatio(a, b annRegion) float64 {
	ar := regionArea(a)
	if ar <= 0 {
		return 0
	}
	return regionIntersect(a, b) / ar
}

// annNotOverlapped mirrors recognizer.py:126-127 (annRegion variant).
func annNotOverlapped(a, b annRegion) bool {
	return a.x1 < b.x0 || a.x0 > b.x1 || a.y1 < b.y0 || a.y0 > b.y1
}

func imin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// sortYFirstly orders regions top-to-bottom (and left-to-right within a
// vertical threshold), matching Recognizer.sort_Y_firstly (recognizer.py:54)
// which LayoutRecognizer.__call__ applies before annotation
// (layout_recognizer.py:99).
func sortYFirstly(regs []annRegion) {
	if len(regs) == 0 {
		return
	}
	avgH := 0.0
	for _, r := range regs {
		avgH += r.y1 - r.y0
	}
	thr := avgH / float64(len(regs)) / 2
	sort.SliceStable(regs, func(i, j int) bool {
		di := regs[i].y0 - regs[j].y0
		if di < -thr {
			return true
		}
		if di > thr {
			return false
		}
		return regs[i].x0 < regs[j].x0
	})
}

// cleanupLayouts de-duplicates overlapping same-type regions, matching
// Recognizer.layouts_cleanup (recognizer.py:124) called by
// LayoutRecognizer.__call__ at layout_recognizer.py:100. A pair of same-type
// regions whose overlap exceeds thr (0.7) in either direction collapses to a
// single region: the higher-score one, or - when scores are absent - the one
// covering more text-box area. far=2 limits comparison to nearby regions.
func cleanupLayouts(regs []annRegion, boxes []pdf.TextBox) []annRegion {
	const far = 2
	const thr = 0.7
	i := 0
	for i+1 < len(regs) {
		j := i + 1
		for j < imin(i+far, len(regs)) && (regs[j].label != regs[i].label || annNotOverlapped(regs[i], regs[j])) {
			j++
		}
		if j >= imin(i+far, len(regs)) {
			i++
			continue
		}
		if overlapRatio(regs[i], regs[j]) < thr && overlapRatio(regs[j], regs[i]) < thr {
			i++
			continue
		}
		// Collapse the pair. Python layouts_cleanup keeps the HIGHER-score
		// region; on equal scores it keeps the later one (j) via pop(i).
		// Match that exactly so equal-confidence pairs converge with Python.
		drop := j
		if regs[i].score > 0 && regs[j].score > 0 {
			if regs[i].score > regs[j].score {
				drop = j
			} else {
				drop = i
			}
		} else {
			areaI, areaJ := 0.0, 0.0
			for _, b := range boxes {
				tb := annRegion{x0: b.X0, y0: b.Top, x1: b.X1, y1: b.Bottom}
				if !annNotOverlapped(tb, regs[i]) {
					areaI += regionIntersect(tb, regs[i])
				}
				if !annNotOverlapped(tb, regs[j]) {
					areaJ += regionIntersect(tb, regs[j])
				}
			}
			if areaJ > areaI {
				drop = i
			}
		}
		regs = append(regs[:drop], regs[drop+1:]...)
	}
	return regs
}

// nmsDLARegions applies per-class non-maximum suppression to raw DLA regions,
// mirroring Python's layout model postprocess (layout_recognizer.py:246,
// operators.py:667 nms with iou_thresh). For each label, detections are sorted
// by confidence descending; the top one is kept and any other same-label
// detection whose IoU (using the +1 overlap convention from operators.py:685)
// exceeds iouThresh is suppressed. This runs on the raw, pre-scale detections
// just as Python's postprocess does, before cleanup/annotation.
//
// It is idempotent with a server-side NMS: applying it again to already-suppressed
// boxes yields the same set, so it safely converges Go to Python regardless of
// where suppression happens upstream.
func nmsDLARegions(regions []pdf.DLARegion, iouThresh float64) []pdf.DLARegion {
	if len(regions) == 0 {
		return regions
	}
	byLabel := map[string][]int{}
	for i, r := range regions {
		byLabel[r.Label] = append(byLabel[r.Label], i)
	}
	suppressed := make([]bool, len(regions))
	for _, idxs := range byLabel {
		// Highest confidence first (greedy NMS keeps the top box). On equal
		// confidence, break the tie by original index so the result is
		// deterministic for identical inputs — otherwise the same PDF could
		// keep a different region (and emit different LayoutNo) on each run.
		sort.SliceStable(idxs, func(a, b int) bool {
			ca, cb := regions[idxs[a]].Confidence, regions[idxs[b]].Confidence
			if ca != cb {
				return ca > cb
			}
			return idxs[a] < idxs[b]
		})
		for k := 0; k < len(idxs); k++ {
			i := idxs[k]
			if suppressed[i] {
				continue
			}
			for m := k + 1; m < len(idxs); m++ {
				j := idxs[m]
				if suppressed[j] {
					continue
				}
				if nmsIoU(regions[i], regions[j]) > iouThresh {
					suppressed[j] = true
				}
			}
		}
	}
	out := make([]pdf.DLARegion, 0, len(regions))
	for i, r := range regions {
		if !suppressed[i] {
			out = append(out, r)
		}
	}
	return out
}

// nmsIoU computes IoU using the +1 overlap convention from operators.py:685-688
// (w = max(0, x22-x11+1), h = max(0, y22-y11+1)) while area uses no +1. This
// must match Python exactly so borderline suppressions (around the 0.45 threshold)
// align.
func nmsIoU(a, b pdf.DLARegion) float64 {
	w := math.Max(0, math.Min(a.X1, b.X1)-math.Max(a.X0, b.X0)+1)
	h := math.Max(0, math.Min(a.Y1, b.Y1)-math.Max(a.Y0, b.Y0)+1)
	inter := w * h
	areaA := (a.X1 - a.X0) * (a.Y1 - a.Y0)
	areaB := (b.X1 - b.X0) * (b.Y1 - b.Y0)
	if areaA <= 0 || areaB <= 0 {
		return 0
	}
	return inter / (areaA + areaB - inter)
}

// AnnotateBoxLayouts sets LayoutType and LayoutNo on each box, matching
// Python's LayoutRecognizer.__call__ which assigns layout types in priority
// order (footer->header->...->equation) with an overlap threshold of 40% of the
// box's area.
//
// Python: _layouts_rec (pdf_parser.py:827) -> LayoutRecognizer.__call__ ->
//
//	for lt in priority_order: findLayout(lt)
//
// Each findLayout(ty): for each unannotated box, find the DLA region of
// type ty with max overlap >= 0.4 * box_area.  First type to match wins.
//
// CID-pattern boxes (e.g. "(cid:123)") are skipped as garbage.
// AnnotateBoxLayouts assigns LayoutType and LayoutNo to boxes based on DLA
// regions.  Returns the filtered slice (Python pops CID-garbled boxes and
// garbage-layout boxes at wrong positions - Go mirrors with compact).
// Also creates synthetic figure boxes for unmatched figure/equation regions.
//
// Before annotation, regions are de-duplicated (layouts_cleanup) and sorted
// top-to-bottom (sort_Y_firstly) to match Python, and unmatched figure and
// equation regions receive SEPARATE synthetic namespaces (figure-N /
// equation-N) so they never collide.
// FilteredDLARegions returns the DLA regions after per-class NMS, the
// confidence filter, Y-sort, and cleanup — i.e. the exact set fed to
// annotation. It mirrors Python's page_layout (layout_recognizer.py:84-100):
//   - nmsDLARegions(0.45)                         == layout model postprocess (operators.py:667)
//   - keep if score >= 0.4 OR type not garbage    == layout_recognizer.py:97
//   - sortYFirstly                                == layout_recognizer.py:99 (sort_Y_firstly)
//   - cleanupLayouts                              == layout_recognizer.py:100 (layouts_cleanup)
//
// Regions are returned in image-pixel space (no scale division) so callers
// that only need the region set — e.g. the parity harness dumping post-filter
// regions for comparison with Python's page_layout — get a stable comparison
// point regardless of render DPI. cleanupLayouts only consults boxes when both
// compared regions have score 0, which never happens for real DLA output, so
// passing nil boxes from the harness is safe.
func FilteredDLARegions(regions []pdf.DLARegion, boxes []pdf.TextBox) []pdf.DLARegion {
	regions = nmsDLARegions(regions, 0.45)
	if len(regions) == 0 {
		return nil
	}
	kept := regions[:0]
	for _, r := range regions {
		if r.Confidence >= pdf.GarbageLayoutScoreThreshold || !pdf.GarbageLayoutTypes[r.Label] {
			kept = append(kept, r)
		}
	}
	ars := make([]annRegion, len(kept))
	for i, r := range kept {
		ars[i] = annRegion{x0: r.X0, y0: r.Y0, x1: r.X1, y1: r.Y1, label: r.Label, score: r.Confidence}
	}
	sortYFirstly(ars)
	ars = cleanupLayouts(ars, boxes)
	out := make([]pdf.DLARegion, len(ars))
	for i, a := range ars {
		out[i] = pdf.DLARegion{X0: a.x0, Y0: a.y0, X1: a.x1, Y1: a.y1, Label: a.label, Confidence: a.score}
	}
	return out
}

func AnnotateBoxLayouts(boxes []pdf.TextBox, regions []pdf.DLARegion, scale float64, pageImgHeight float64) []pdf.TextBox {
	// NMS, confidence filter, Y-sort, and cleanup — the exact region set fed
	// to annotation. This mirrors Python's layout_recognizer.py:84-100
	// (filter + sort_Y_firstly + layouts_cleanup) and is the same pipeline the
	// parity harness uses (via FilteredDLARegions) to dump post-filter regions
	// comparable with Python's page_layout.
	filtered := FilteredDLARegions(regions, boxes)
	if len(filtered) == 0 {
		return boxes
	}

	// Scale filtered regions from image-pixel space to PDF space.
	cands := make([]annRegion, 0, len(filtered))
	for _, r := range filtered {
		cands = append(cands, annRegion{
			x0: r.X0 / scale, y0: r.Y0 / scale,
			x1: r.X1 / scale, y1: r.Y1 / scale,
			label: r.Label, score: r.Confidence,
		})
	}

	// Per-type index in the cleaned, Y-sorted list (Python: ii in lts_).
	typeCounters := make(map[string]int)
	for j := range cands {
		cands[j].typeIndex = typeCounters[cands[j].label]
		typeCounters[cands[j].label]++
	}

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
			bestRegionOverlap := 0.0
			bestJ := -1
			for j, r := range cands {
				if r.label != ty {
					continue
				}
				ix0 := math.Max(r.x0, boxes[i].X0)
				iy0 := math.Max(r.y0, boxes[i].Top)
				ix1 := math.Min(r.x1, boxes[i].X1)
				iy1 := math.Min(r.y1, boxes[i].Bottom)
				if ix0 < ix1 && iy0 < iy1 {
					inter := (ix1 - ix0) * (iy1 - iy0)
					ov := inter / boxArea // fraction of the box covered (Python's ov)
					rArea := (r.x1 - r.x0) * (r.y1 - r.y0)
					ovRegion := 0.0
					if rArea > 0 {
						ovRegion = inter / rArea // fraction of the region covered (Python's _ov)
					}
					// Mirror Python's (ov, _ov) tuple comparison
					// (recognizer.py:255-269): primary key is the box
					// coverage ratio; on a tie the region-coverage ratio
					// wins (prefer the region the box sits more "inside"
					// of); on a full tie keep the first (topmost) region.
					if ov > bestOverlap || (ov == bestOverlap && ovRegion > bestRegionOverlap) {
						bestOverlap = ov
						bestRegionOverlap = ovRegion
						bestJ = j
					}
				}
			}
			if bestJ >= 0 && bestOverlap >= 0.4 {
				// Garbage layout not at page edge -> pop (Python: bxs.pop(i)).
				if pdf.GarbageLayoutTypes[ty] && pageImgHeight > 0 && !garbageKeepFeat(ty, boxes[i], pageImgHeight/scale) {
					dropped[i] = true
					continue
				}
				cands[bestJ].visited = true
				// Python: equation mapped to "figure" for layout_type
				if ty == pdf.LayoutTypeEquation {
					boxes[i].LayoutType = pdf.LayoutTypeFigure
				} else {
					boxes[i].LayoutType = ty
				}
				// Python: f"{layout_type}-{matched}" where matched is per-type index
				boxes[i].LayoutNo = fmt.Sprintf("%s-%d", ty, cands[bestJ].typeIndex)
			}
		}
	}

	// Compact: remove popped boxes into a new backing array (Python
	// bxs.pop).  Allocating a fresh slice is deliberate: annotations were
	// set in-place on the input elements, and callers (enrichOnePageWithDeepDoc)
	// rely on positional stability of the input slice for their
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
	// layout_recognizer.py:145-155).  Python numbers each unmatched region with
	// its index WITHIN the per-type list that also includes already-visited
	// regions (enumerate([lt for lt in lts if lt["type"] == ty])), so we reuse
	// the per-type typeIndex computed above rather than a separate
	// unvisited-only counter. Python keeps figure-N / equation-N in SEPARATE
	// namespaces, so the typeIndex is keyed by the original type label.
	for j := range cands {
		if cands[j].visited {
			continue
		}
		if cands[j].label != pdf.LayoutTypeFigure && cands[j].label != pdf.LayoutTypeEquation {
			continue
		}
		boxes = append(boxes, pdf.TextBox{
			X0:         cands[j].x0,
			X1:         cands[j].x1,
			Top:        cands[j].y0,
			Bottom:     cands[j].y1,
			Text:       "",
			LayoutType: pdf.LayoutTypeFigure,
			LayoutNo:   fmt.Sprintf("%s-%d", cands[j].label, cands[j].typeIndex),
		})
	}

	return boxes
}

// ── garbage layout helpers ────────────────────────────────────────────
// garbageKeepFeat matches Python's keep_feats in LayoutRecognizer.__call__:
// footer near page bottom (>90% of page height) or header near page top (<10%)
// are real page decorations - keep them.  Others are DLA noise.
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
