package table

import (
	"math"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
	"ragflow/internal/deepdoc/parser/pdf/util"
	"sort"
)

// ── Post-TSR layout annotation (Python: pdf_parser.py gather/layouts_cleanup) ──

// SortYFirstly sorts cells by top, with fuzzy threshold: if two cells are
// within threshold Y pixels, sort by X instead (same-row ordering).
// Python: Recognizer.sort_Y_firstly(arr, threshold)
func SortYFirstly(cells []pdf.TSRCell, threshold float64) {
	sort.Slice(cells, func(i, j int) bool {
		diff := cells[i].Y0 - cells[j].Y0
		if math.Abs(diff) < threshold {
			return cells[i].X0 < cells[j].X0
		}
		return diff < 0
	})
}

// SortXFirstly sorts cells by x0, with fuzzy threshold for top.
func SortXFirstly(cells []pdf.TSRCell, threshold float64) {
	sort.Slice(cells, func(i, j int) bool {
		diff := cells[i].X0 - cells[j].X0
		if math.Abs(diff) < threshold {
			return cells[i].Y0 < cells[j].Y0
		}
		return diff < 0
	})
}

// layoutCleanup removes duplicate/overlapping cells of the same type.
// Python: Recognizer.layouts_cleanup(boxes, layouts, far=2, thr=0.7)
//
// For each cell, checks the next `far` cells; if they overlap significantly
// AND have the same label type, the one with lower score is removed when both
// carry a detection score (recognizer.py:141, the primary branch — TSR always
// emits scores), otherwise the one with less box-overlap area is removed
// (the area branch, which sums overlap against `boxes`).
func layoutCleanup(cells []pdf.TSRCell, boxes []pdf.TextBox, far int, thr float64) []pdf.TSRCell {
	// cells are assumed pre-sorted (caller sorts before passing)
	out := make([]pdf.TSRCell, len(cells))
	copy(out, cells)

	i := 0
	for i+1 < len(out) {
		j := i + 1
		limit := min(i+far, len(out))
		for j < limit && (out[i].Label != "" && out[i].Label != out[j].Label || notOverlapped(out[i], out[j])) {
			j++
		}
		if j >= limit {
			i++
			continue
		}
		// Cells i and j overlap and have same type. Keep one.
		areaI := util.OverlapRatioA(&out[i], &out[j])
		areaJ := util.OverlapRatioA(&out[j], &out[i])
		if areaI < thr && areaJ < thr {
			i++
			continue
		}

		// Python: when both carry a detection score, keep the higher score
		// (score tie keeps cells[i], matching `else: layouts.pop(i)`).
		if out[i].Score > 0 && out[j].Score > 0 {
			if out[i].Score > out[j].Score {
				out = append(out[:j], out[j+1:]...)
			} else {
				out = append(out[:i], out[i+1:]...)
			}
			continue
		}

		// Prefer the one that overlaps more with text boxes.
		boxAreaI, boxAreaJ := 0.0, 0.0
		for _, b := range boxes {
			if !tsrBoxOverlap(b, out[i]) {
				boxAreaI += util.OverlapInter(&b, &out[i])
			}
			if !tsrBoxOverlap(b, out[j]) {
				boxAreaJ += util.OverlapInter(&b, &out[j])
			}
		}
		if boxAreaI >= boxAreaJ {
			out = append(out[:j], out[j+1:]...)
		} else {
			out = append(out[:i], out[i+1:]...)
		}
	}
	return out
}

// notOverlapped returns true if cells a and b do NOT overlap.
func notOverlapped(a, b pdf.TSRCell) bool {
	return a.X1 < b.X0 || a.X0 > b.X1 || a.Y1 < b.Y0 || a.Y0 > b.Y1
}

// isHeaderLabel reports whether a TSR cell label denotes a header region,
// matching Python's gather(r".*header$") in t_recognizer.py.
func isHeaderLabel(label string) bool {
	return strings.HasSuffix(strings.ToLower(label), "header")
}

// tsrBoxOverlap returns true if a pdf.TextBox and a pdf.TSRCell do NOT overlap.
func tsrBoxOverlap(b pdf.TextBox, c pdf.TSRCell) bool {
	return b.X1 < c.X0 || b.X0 > c.X1 || b.Bottom < c.Y0 || b.Top > c.Y1
}

// findOverlappedWithThreshold returns the index of the cell with the best
// bidirectional overlap >= thr, or -1 if none.
// Python: Recognizer.find_overlapped_with_threshold(box, boxes, thr=0.3)
// The gate is the BOX ratio only (fraction of the box covered by the cell),
// and scoring is the (boxRatio, cellRatio) tuple lexicographically — Python
// picks the candidate with the largest boxRatio, tie-broken by cellRatio.
func findOverlappedWithThreshold(box pdf.TextBox, cells []pdf.TSRCell, thr float64) int {
	boxArea := util.Area(&box)
	if boxArea <= 0 {
		return -1
	}
	bestIdx := -1
	bestOv, bestOv2 := thr, 0.0
	for i, c := range cells {
		cellArea := util.Area(&c)
		if cellArea <= 0 {
			continue
		}
		ol := util.OverlapInter(&box, &c)
		if ol <= 0 {
			continue
		}
		boxRatio := ol / boxArea
		cellRatio := ol / cellArea
		// Python: if (ov, _ov) < (best, best2): continue
		if boxRatio < bestOv || (boxRatio == bestOv && cellRatio < bestOv2) {
			continue
		}
		bestIdx, bestOv, bestOv2 = i, boxRatio, cellRatio
	}
	return bestIdx
}

// findHorizontallyTightestFit returns the index of the column with the
// minimal horizontal edge distance to the box, restricted to columns that
// share vertical extent with it.
// Python: Recognizer.find_horizontally_tightest_fit(b, clmns). The distance is
// min(|x0-cx0|, |x1-cx1|, |(x0+x1)-(cx0+cx1)|/2), and a column whose Y range
// does not overlap the box's Y range is rejected (page-cumulative Y, so this
// also keeps a same-table column from another page out).
func findHorizontallyTightestFit(box pdf.TextBox, clmns []pdf.TSRCell) int {
	best := -1
	bestDist := float64(1<<63 - 1)
	for i, c := range clmns {
		// Python: min(box.bottom, c.bottom) <= max(box.top, c.top) → skip
		if math.Min(box.Bottom, c.Y1) <= math.Max(box.Top, c.Y0) {
			continue
		}
		// Minimum edge distance between box and column boundaries.
		dl := math.Abs(box.X0 - c.X0)
		dr := math.Abs(box.X1 - c.X1)
		dc := math.Abs(box.X0+box.X1-c.X1-c.X0) / 2
		d := math.Min(math.Min(dl, dr), dc)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

// AnnotateBoxesWithGrid derives per-box R/C/H/SP annotations in the SAME
// coordinate frame as grid (e.g. a table's crop space), using Python's
// _table_transformer_job semantics. It is the production entry point for
// deriving R/C so the grid can be rebuilt from them (GroupBoxesByRC).
func AnnotateBoxesWithGrid(boxes []pdf.TextBox, grid [][]pdf.TSRCell) {
	AnnotateTableBoxes(boxes, grid)
}

// annotateTableBoxes tags table boxes with row/header/column indices using
// TSR cell labels. Matching Python's R/H/C/SP annotation logic.
//
// Python: pdf_parser.py:518-554
func AnnotateTableBoxes(boxes []pdf.TextBox, grid [][]pdf.TSRCell) {
	// grid[0] is the header row.  Spans are computed by calSpans later.
	var headers, spans []pdf.TSRCell
	var clmns []pdf.TSRCell
	// Python t_recognizer.py: headers = gather(r".*header$") — the set of layout
	// cells whose label ends in "header", NOT the first grid row. Collect them
	// from every grid row so a header that sits on a row other than 0 is still
	// matched and tagged with H>0 (fixes the grid[0] approximation).
	for _, row := range grid {
		for _, cell := range row {
			if isHeaderLabel(cell.Label) {
				headers = append(headers, cell)
			}
			// Collect spanning cells so the SP annotation is propagated to
			// overlapping boxes (Python _table_transformer_job appends every
			// "SP" cell to its `spans` list and matches boxes against it at
			// pdf_parser.py:518-554). Without this, box.SP stays 0, the
			// rebuilt grid (GroupBoxesByRC) loses the span, and
			// ConstructTable/CalSpans drops the colspan/rowspan — Go emits
			// independent empty <th> where Python emits <th colspan=6>
			// (e.g. real_pdfs/1.pdf).
			if strings.Contains(cell.Label, "spanning") {
				spans = append(spans, cell)
			}
		}
	}
	if len(grid) > 0 && len(grid[0]) > 0 {
		// Python's clmns are the "table column" lines: vertical bboxes spanning
		// the whole table height. Derive them from the grid (each column's X
		// range from the first row, Y range from the table's top/bottom rows).
		tableTop := grid[0][0].Y0
		tableBot := grid[len(grid)-1][0].Y1
		clmns = make([]pdf.TSRCell, len(grid[0]))
		for ci := range grid[0] {
			clmns[ci] = pdf.TSRCell{X0: grid[0][ci].X0, Y0: tableTop, X1: grid[0][ci].X1, Y1: tableBot}
		}
	}
	SortYFirstly(headers, 10)
	SortXFirstly(clmns, 10)

	for i := range boxes {
		// Python processes only boxes whose layout_type is "table"; callers
		// (processOneTable / WriteTableAnnotations) already pass the table
		// region's box subset, so an empty LayoutType (e.g. OCR-replay boxes
		// that carry no DLA annotation) is treated as table content too.
		if boxes[i].LayoutType != pdf.LayoutTypeTable && boxes[i].LayoutType != "" {
			continue
		}
		// R: Python find_overlapped_with_threshold(box, rows, 0.3) over the
		// WHOLE row line — the grid row's bbox spans the table width (the row
		// line's own X range), not individual grid cells.
		for ri, row := range grid {
			if len(row) == 0 {
				continue
			}
			rowBBox := pdf.TSRCell{X0: row[0].X0, Y0: row[0].Y0, X1: row[len(row)-1].X1, Y1: row[0].Y1}
			if findOverlappedWithThreshold(boxes[i], []pdf.TSRCell{rowBBox}, 0.3) >= 0 {
				boxes[i].R = ri
				boxes[i].RTop = row[0].Y0
				boxes[i].RBott = row[0].Y1
				break
			}
		}
		if idx := findOverlappedWithThreshold(boxes[i], headers, 0.3); idx >= 0 {
			boxes[i].HTop = headers[idx].Y0
			boxes[i].HBott = headers[idx].Y1
			boxes[i].HLeft = headers[idx].X0
			boxes[i].HRight = headers[idx].X1
			// Offset by 1: store idx+1 so a box matching the FIRST header cell
			// (idx == 0) is distinguishable from "no header overlap" (the
			// default H == 0). All readers check H > 0, so this keeps the
			// boolean semantics while fixing single-column / first-column
			// header detection (parity #4, asymmetry 1).
			boxes[i].H = idx + 1
		}
		// C: Python find_horizontally_tightest_fit(box, clmns).
		if len(clmns) > 1 {
			if idx := findHorizontallyTightestFit(boxes[i], clmns); idx >= 0 {
				boxes[i].C = idx
				boxes[i].CLeft = clmns[idx].X0
				boxes[i].CRight = clmns[idx].X1
			}
		}
		if idx := findOverlappedWithThreshold(boxes[i], spans, 0.3); idx >= 0 {
			// Offset by 1 so a box matching the FIRST spanning cell
			// (idx == 0) is distinguishable from "no span overlap" (the
			// default SP == 0). All readers check SP > 0, matching Python's
			// boolean SP semantics (pdf_parser.py:518-554).
			boxes[i].SP = idx + 1
			// Python _annotate_table_boxes (pdf_parser.py:632-635) copies the
			// spanning cell's bbox onto the box as H_top/H_bott/H_left/H_right.
			// GroupBoxesByRC then builds the span cell from these full extents
			// (cellPosFromBox uses HLeft/HRight when H>0), so CalSpans covers
			// every column the span crosses. Without this, the span cell falls
			// back to the box's own narrow bounds and Go emits colspan=5 where
			// Python emits colspan=6 (real_pdfs/1.pdf).
			boxes[i].HTop = spans[idx].Y0
			boxes[i].HBott = spans[idx].Y1
			boxes[i].HLeft = spans[idx].X0
			boxes[i].HRight = spans[idx].X1
		}
	}

	// Two-pass C fallback: after all R values are assigned, compute C by X-order within each row.
	// This matches Python's behavior when TSR provides few "table column" cells.
	if len(clmns) <= 1 {
		// Collect all table boxes grouped by R (LayoutType empty → table content).
		rBoxes := make(map[int][]int)
		for i := range boxes {
			if boxes[i].LayoutType != pdf.LayoutTypeTable && boxes[i].LayoutType != "" {
				continue
			}
			rBoxes[boxes[i].R] = append(rBoxes[boxes[i].R], i)
		}
		for _, indices := range rBoxes {
			sort.Slice(indices, func(a, b int) bool { return boxes[indices[a]].X0 < boxes[indices[b]].X0 })
			for ci, bi := range indices {
				boxes[bi].C = ci
			}
		}
	}
}
