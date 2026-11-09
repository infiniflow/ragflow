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

// spanTopRowIndex returns the first physical table row whose centre lies
// inside the span's Y range — the top row of the merged rectangle (Python
// __cal_spans minR). Header components are skipped: they are not rows, and
// using their index would put the span in a different R space from the row
// cells. Returns -1 when no table row centre falls inside the span.
func spanTopRowIndex(rows []pdf.TSRCell, sp pdf.TSRCell) int {
	for i, r := range rows {
		if isHeaderLabel(r.Label) {
			continue
		}
		cy := (r.Y0 + r.Y1) / 2
		if cy >= sp.Y0 && cy <= sp.Y1 {
			return i
		}
	}
	return -1
}

// AnnotateBoxesWithGridAndRows derives R/C/H/SP for boxes in the same
// coordinate frame as grid. R is matched against the raw TSR row bands, not
// span-extended grid rows.
func AnnotateBoxesWithGridAndRows(boxes []pdf.TextBox, grid [][]pdf.TSRCell, rows []pdf.TSRCell) {
	annotateTableBoxes(boxes, grid, rows)
}

// AnnotateTableBoxes is the fallback when raw TSR rows are unavailable. It
// matches R against grid rows, which can over-merge a rowspan header;
// production uses AnnotateBoxesWithGridAndRows.
func AnnotateTableBoxes(boxes []pdf.TextBox, grid [][]pdf.TSRCell) {
	annotateTableBoxes(boxes, grid, nil)
}

func annotateTableBoxes(boxes []pdf.TextBox, grid [][]pdf.TSRCell, rows []pdf.TSRCell) {
	var headers, spans []pdf.TSRCell
	var clmns []pdf.TSRCell
	// Collect header and spanning cells from every grid row. A header is not
	// necessarily in row 0, and box.SP must be set or CalSpans drops the span.
	for _, row := range grid {
		for _, cell := range row {
			if isHeaderLabel(cell.Label) {
				headers = append(headers, cell)
			}
			if strings.Contains(cell.Label, "spanning") {
				spans = append(spans, cell)
			}
		}
	}
	if len(grid) > 0 && len(grid[0]) > 0 {
		// Recover column edges from non-spanning cells: grid[0] alone can be
		// widened by a span origin and would absorb neighbouring columns.
		colLeft, colRight, _, _ := gridBounds(grid)
		nCols := len(colLeft)
		tableTop := grid[0][0].Y0
		tableBot := grid[len(grid)-1][0].Y1
		if len(rows) > 0 {
			tableTop = rows[0].Y0
			tableBot = rows[len(rows)-1].Y1
		}
		clmns = make([]pdf.TSRCell, nCols)
		for ci := 0; ci < nCols; ci++ {
			if colLeft[ci] == 1e9 {
				// No non-spanning cell in this column: keep the grid bounds.
				colLeft[ci], colRight[ci] = grid[0][ci].X0, grid[0][ci].X1
			}
			clmns[ci] = pdf.TSRCell{X0: colLeft[ci], Y0: tableTop, X1: colRight[ci], Y1: tableBot}
		}
	}
	SortYFirstly(headers, 10)
	SortXFirstly(clmns, 10)

	for i := range boxes {
		// Empty LayoutType is table content too (e.g. OCR replay boxes).
		if boxes[i].LayoutType != pdf.LayoutTypeTable && boxes[i].LayoutType != "" {
			continue
		}
		boxes[i].HasTableAnnotations = true
		// R: Python find_overlapped_with_threshold(box, rows, 0.3). Python
		// leaves R unset when no row line matches; using Go's zero value would
		// silently put that box in row 0, so start from -1 and only write a
		// row when the match succeeds.
		boxes[i].R = -1
		boxes[i].RTop = 0
		boxes[i].RBott = 0
		if len(rows) > 0 {
			if ri := findOverlappedWithThreshold(boxes[i], rows, 0.3); ri >= 0 {
				boxes[i].R = ri
				boxes[i].RTop = rows[ri].Y0
				boxes[i].RBott = rows[ri].Y1
			}
		} else {
			// Fallback: match against grid rows. A span-extended row can
			// over-merge; production passes raw rows to avoid it.
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
		}
		if idx := findOverlappedWithThreshold(boxes[i], headers, 0.3); idx >= 0 {
			boxes[i].HTop = headers[idx].Y0
			boxes[i].HBott = headers[idx].Y1
			boxes[i].HLeft = headers[idx].X0
			boxes[i].HRight = headers[idx].X1
			// Store idx+1: H>0 means matched, and an index of 0 is still
			// distinguishable from "no header".
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
			// Store idx+1: SP>0 means matched, including span index 0.
			boxes[i].SP = idx + 1
			// Copy the span's full extent so cellPosFromBox can build a cell
			// that covers every row/column the span crosses.
			boxes[i].HTop = spans[idx].Y0
			boxes[i].HBott = spans[idx].Y1
			boxes[i].HLeft = spans[idx].X0
			boxes[i].HRight = spans[idx].X1
			// A rowspan cell's text is vertically centred and may sit in the
			// lower row; move R to the span's top row (Python __cal_spans
			// places the merged cell at tbl[minR][minC]).
			if len(rows) > 0 {
				if sr := spanTopRowIndex(rows, spans[idx]); sr >= 0 {
					boxes[i].R = sr
					boxes[i].RTop = rows[sr].Y0
					boxes[i].RBott = rows[sr].Y1
				}
			}
		}
	}

	// No usable column structure: assign C by X order within each R group.
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
