package table

import (
	"math"
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
// AND have the same label type, the one with lower score (or less box overlap
// area) is removed.
func layoutCleanup(cells []pdf.TSRCell, boxes []pdf.TextBox, far int, thr float64) []pdf.TSRCell {
	// cells are assumed pre-sorted (caller sorts before passing)
	out := make([]pdf.TSRCell, len(cells))
	copy(out, cells)

	i := 0
	for i+1 < len(out) {
		j := i + 1
		limit := i + far
		if limit > len(out) {
			limit = len(out)
		}
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

// tsrBoxOverlap returns true if a pdf.TextBox and a pdf.TSRCell do NOT overlap.
func tsrBoxOverlap(b pdf.TextBox, c pdf.TSRCell) bool {
	return b.X1 < c.X0 || b.X0 > c.X1 || b.Bottom < c.Y0 || b.Top > c.Y1
}

// findOverlappedWithThreshold returns the index of the cell with the best
// bidirectional overlap >= thr, or -1 if none.
// Python: Recognizer.find_overlapped_with_threshold(box, boxes, thr=0.3)
// Python uses max(boxRatio, cellRatio) for both gate and scoring.
func findOverlappedWithThreshold(box pdf.TextBox, cells []pdf.TSRCell, thr float64) int {
	boxArea := util.Area(&box)
	if boxArea <= 0 {
		return -1
	}
	bestIdx := -1
	bestOverlap := thr // Python: max_overlap starts at thr
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
		// Python: max(cls.overlapped_area(box, layout), cls.overlapped_area(layout, box))
		overlap := math.Max(boxRatio, cellRatio)
		if overlap >= bestOverlap {
			bestOverlap = overlap
			bestIdx = i
		}
	}
	return bestIdx
}

// findHorizontallyTightestFit returns the index of the column cell that
// horizontally contains the box with minimal width difference.
// Python: Recognizer.find_horizontally_tightest_fit(b, clmns)
// findHorizontallyTightestFit returns the column index with minimum
// edge distance to the box.  Python: Recognizer.find_horizontally_tightest_fit.
func findHorizontallyTightestFit(box pdf.TextBox, clmns []pdf.TSRCell) int {
	best := -1
	bestDist := float64(1<<63 - 1)
	for i, c := range clmns {
		// Minimum edge distance between box and column boundaries.
		dl := math.Abs(box.X0 - c.X0)
		dr := math.Abs(box.X1 - c.X1)
		d := math.Min(dl, dr)
		if d < bestDist {
			bestDist = d
			best = i
		}
	}
	return best
}

// annotateTableBoxes tags table boxes with row/header/column indices using
// TSR cell labels. Matching Python's R/H/C/SP annotation logic.
//
// Python: pdf_parser.py:518-554
func AnnotateTableBoxes(boxes []pdf.TextBox, grid [][]pdf.TSRCell) {
	// grid[0] is the header row.  Spans are computed by calSpans later.
	var headers, spans []pdf.TSRCell
	var clmns []pdf.TSRCell
	if len(grid) > 0 {
		headers = grid[0]
		clmns = append(clmns, grid[0]...)
	}
	SortYFirstly(headers, 10)
	SortXFirstly(clmns, 10)

	for i := range boxes {
		if boxes[i].LayoutType != pdf.LayoutTypeTable {
			continue
		}
		// Grid-based R/C: match box to the row and column it overlaps.
		for ri, row := range grid {
			if idx := findOverlappedWithThreshold(boxes[i], row, 0.3); idx >= 0 {
				boxes[i].R = ri
				boxes[i].RTop = row[0].Y0
				boxes[i].RBott = row[0].Y1
				for ci, cell := range row {
					if !tsrBoxOverlap(boxes[i], cell) {
						boxes[i].C = ci
						boxes[i].CLeft = cell.X0
						boxes[i].CRight = cell.X1
						break
					}
				}
				break
			}
		}
		if idx := findOverlappedWithThreshold(boxes[i], headers, 0.3); idx >= 0 {
			boxes[i].HTop = headers[idx].Y0
			boxes[i].HBott = headers[idx].Y1
			boxes[i].HLeft = headers[idx].X0
			boxes[i].HRight = headers[idx].X1
			boxes[i].H = idx
		}
		if len(clmns) > 1 {
			if idx := findHorizontallyTightestFit(boxes[i], clmns); idx >= 0 {
				boxes[i].C = idx
				boxes[i].CLeft = clmns[idx].X0
				boxes[i].CRight = clmns[idx].X1
			}
		}
		if idx := findOverlappedWithThreshold(boxes[i], spans, 0.3); idx >= 0 {
			boxes[i].SP = idx
		}
	}

	// Two-pass C fallback: after all R values are assigned, compute C by X-order within each row.
	// This matches Python's behavior when TSR provides few "table column" cells.
	if len(clmns) <= 1 {
		// Collect all table boxes grouped by R.
		rBoxes := make(map[int][]int)
		for i := range boxes {
			if boxes[i].LayoutType == pdf.LayoutTypeTable {
				rBoxes[boxes[i].R] = append(rBoxes[boxes[i].R], i)
			}
		}
		for _, indices := range rBoxes {
			sort.Slice(indices, func(a, b int) bool { return boxes[indices[a]].X0 < boxes[indices[b]].X0 })
			for ci, bi := range indices {
				boxes[bi].C = ci
			}
		}
	}
}
