package parser

import (
	"math"
	"regexp"
	"sort"
)

// ── Post-TSR layout annotation (Python: pdf_parser.py gather/layouts_cleanup) ──

// Pre-compiled label patterns — compiled once at package init.
var (
	reTableHeader = regexp.MustCompile(`.*header$`)
	reTableRowHdr = regexp.MustCompile(`table$|.* (row|header)`)
	// "table$" catches the default TSR label "table" (class 0), matching
	// Python's behavior which uses all cells regardless of label.
	reTableSpan   = regexp.MustCompile(`.*spanning`)
	reTableColumn = regexp.MustCompile(`table column$`)
)

// gatherTSR filters cells by label regex pattern. Matching Python's
// gather() which uses re.match(kwd, r["label"]).
func gatherTSR(cells []TSRCell, re *regexp.Regexp) []TSRCell {
	var result []TSRCell
	for _, c := range cells {
		if re.MatchString(c.Label) {
			result = append(result, c)
		}
	}
	return result
}

// sortYFirstly sorts cells by top, with fuzzy threshold: if two cells are
// within threshold Y pixels, sort by X instead (same-row ordering).
// Python: Recognizer.sort_Y_firstly(arr, threshold)
func sortYFirstly(cells []TSRCell, threshold float64) {
	sort.Slice(cells, func(i, j int) bool {
		diff := cells[i].Y0 - cells[j].Y0
		if abs(diff) < threshold {
			return cells[i].X0 < cells[j].X0
		}
		return diff < 0
	})
}

// sortXFirstly sorts cells by x0, with fuzzy threshold for top.
func sortXFirstly(cells []TSRCell, threshold float64) {
	sort.Slice(cells, func(i, j int) bool {
		diff := cells[i].X0 - cells[j].X0
		if abs(diff) < threshold {
			return cells[i].Y0 < cells[j].Y0
		}
		return diff < 0
	})
}

func abs(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}

// layoutCleanup removes duplicate/overlapping cells of the same type.
// Python: Recognizer.layouts_cleanup(boxes, layouts, far=2, thr=0.7)
//
// For each cell, checks the next `far` cells; if they overlap significantly
// AND have the same label type, the one with lower score (or less box overlap
// area) is removed.
func layoutCleanup(cells []TSRCell, boxes []TextBox, far int, thr float64) []TSRCell {
	// cells are assumed pre-sorted (caller sorts before passing)
	out := make([]TSRCell, len(cells))
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
		areaI := overlapArea(out[i], out[j])
		areaJ := overlapArea(out[j], out[i])
		if areaI < thr && areaJ < thr {
			i++
			continue
		}

		// Prefer the one that overlaps more with text boxes.
		boxAreaI, boxAreaJ := 0.0, 0.0
		for _, b := range boxes {
			if !tsrBoxOverlap(b, out[i]) {
				boxAreaI += tsrOverlapArea(b, out[i])
			}
			if !tsrBoxOverlap(b, out[j]) {
				boxAreaJ += tsrOverlapArea(b, out[j])
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
func notOverlapped(a, b TSRCell) bool {
	return a.X1 < b.X0 || a.X0 > b.X1 || a.Y1 < b.Y0 || a.Y0 > b.Y1
}

// overlapArea returns the area of cell a that overlaps cell b, as a fraction
// of cell a's area.
func overlapArea(a, b TSRCell) float64 {
	ix0 := max(a.X0, b.X0)
	iy0 := max(a.Y0, b.Y0)
	ix1 := min(a.X1, b.X1)
	iy1 := min(a.Y1, b.Y1)
	if ix0 >= ix1 || iy0 >= iy1 {
		return 0
	}
	intersection := (ix1 - ix0) * (iy1 - iy0)
	area := (a.X1 - a.X0) * (a.Y1 - a.Y0)
	if area <= 0 {
		return 0
	}
	return intersection / area
}

// tsrBoxOverlap returns true if a TextBox and a TSRCell do NOT overlap.
func tsrBoxOverlap(b TextBox, c TSRCell) bool {
	return b.X1 < c.X0 || b.X0 > c.X1 || b.Bottom < c.Y0 || b.Top > c.Y1
}

// tsrOverlapArea returns the overlap area between a TextBox and TSRCell.
func tsrOverlapArea(b TextBox, c TSRCell) float64 {
	ix0 := max(b.X0, c.X0)
	iy0 := max(b.Top, c.Y0)
	ix1 := min(b.X1, c.X1)
	iy1 := min(b.Bottom, c.Y1)
	if ix0 >= ix1 || iy0 >= iy1 {
		return 0
	}
	return (ix1 - ix0) * (iy1 - iy0)
}

// findOverlappedWithThreshold returns the index of the cell with the best
// bidirectional overlap >= thr, or -1 if none.
// Python: Recognizer.find_overlapped_with_threshold(box, boxes, thr=0.3)
// Python uses max(boxRatio, cellRatio) for both gate and scoring.
func findOverlappedWithThreshold(box TextBox, cells []TSRCell, thr float64) int {
	boxArea := (box.X1 - box.X0) * (box.Bottom - box.Top)
	if boxArea <= 0 {
		return -1
	}
	bestIdx := -1
	bestOverlap := thr // Python: max_overlap starts at thr
	for i, c := range cells {
		cellArea := (c.X1 - c.X0) * (c.Y1 - c.Y0)
		if cellArea <= 0 {
			continue
		}
		ol := overlapAreaBoxCell(box, c)
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

func overlapAreaBoxCell(box TextBox, cell TSRCell) float64 {
	ix0 := max(box.X0, cell.X0)
	iy0 := max(box.Top, cell.Y0)
	ix1 := min(box.X1, cell.X1)
	iy1 := min(box.Bottom, cell.Y1)
	if ix0 >= ix1 || iy0 >= iy1 {
		return 0
	}
	return (ix1 - ix0) * (iy1 - iy0)
}

// findHorizontallyTightestFit returns the index of the column cell that
// horizontally contains the box with minimal width difference.
// Python: Recognizer.find_horizontally_tightest_fit(b, clmns)
// findHorizontallyTightestFit returns the column index with minimum
// edge distance to the box.  Python: Recognizer.find_horizontally_tightest_fit.
func findHorizontallyTightestFit(box TextBox, clmns []TSRCell) int {
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
func annotateTableBoxes(boxes []TextBox, cells []TSRCell) {
	headers := gatherTSR(cells, reTableHeader)
	sortYFirstly(headers, 10)
	headers = layoutCleanup(headers, boxes, 5, 0.6)

	rows := gatherTSR(cells, reTableRowHdr)
	sortYFirstly(rows, 10)
	rows = layoutCleanup(rows, boxes, 5, 0.6)

	spans := gatherTSR(cells, reTableSpan)
	sortYFirstly(spans, 10)

	clmns := gatherTSR(cells, reTableColumn)
	sortXFirstly(clmns, 10)
	clmns = layoutCleanup(clmns, boxes, 5, 0.5)

	// Group cells into rows by Y proximity for grid-aligned R indices.
	rowGroups := groupTSRCellsToRows(rows)
	for i := range boxes {
		if boxes[i].LayoutType != "table" {
			continue
		}
		// Find which row GROUP (not flat cell index) this box overlaps.
		for ri, rowGroup := range rowGroups {
			if findOverlappedWithThreshold(boxes[i], rowGroup, 0.3) >= 0 {
				boxes[i].R = ri
				boxes[i].RTop = rowGroup[0].Y0
				boxes[i].RBott = rowGroup[0].Y1
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
			if boxes[i].LayoutType == "table" {
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

// groupTSRCellsToRowsLabeled groups TSR cells into rows using labels
// (header, row, spanning) instead of just Y proximity. Matching Python's
// gather-based approach.
func groupTSRCellsToRowsLabeled(cells []TSRCell) [][]TSRCell {
	rows := gatherTSR(cells, reTableRowHdr)
	spans := gatherTSR(cells, reTableSpan)
	clmns := gatherTSR(cells, reTableColumn)

	if len(rows) == 0 && len(spans) == 0 {
		// Fallback to Y-based grouping if no labels match
		return groupTSRCellsToRows(cells)
	}

	sortYFirstly(rows, 10)
	sortXFirstly(clmns, 10)

	// Group rows by Y proximity
	var grouped [][]TSRCell
	var curRow []TSRCell
	curY := 0.0
	rowThreshold := 0.0
	if len(rows) > 0 {
		// Calculate median row height for threshold
		heights := make([]float64, len(rows))
		for i, r := range rows {
			heights[i] = r.Y1 - r.Y0
		}
		sort.Float64s(heights)
		rowThreshold = heights[len(heights)/2] * 0.5
		if rowThreshold <= 0 {
			rowThreshold = 10
		}
	}

	for _, c := range rows {
		if len(curRow) == 0 {
			curRow = append(curRow, c)
			curY = c.Y0
			continue
		}
		if c.Y0-curY > rowThreshold {
			grouped = append(grouped, curRow)
			curRow = []TSRCell{c}
			curY = c.Y0
		} else {
			curRow = append(curRow, c)
		}
	}
	if len(curRow) > 0 {
		grouped = append(grouped, curRow)
	}

	// Add spanning cells to the first row they overlap
	for _, s := range spans {
		for ri, row := range grouped {
			if len(row) > 0 && s.Y0 <= row[0].Y1 && s.Y1 >= row[0].Y0 {
				grouped[ri] = append(grouped[ri], s)
				break
			}
		}
	}

	// Sort each row by X
	for _, row := range grouped {
		sortXFirstly(row, 10)
	}

	// Pad all rows to the same column count (Python construct_table alignment).
	maxCols := 0
	for _, row := range grouped {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}
	for i := range grouped {
		for len(grouped[i]) < maxCols {
			// Pad with a cell at the right edge so X-sort order is preserved.
			// Inherit Y range from the row so row center calculation in calSpans is correct.
			lastX := 0.0
			rowY0, rowY1 := 0.0, 0.0
			if len(grouped[i]) > 0 {
				lastX = grouped[i][len(grouped[i])-1].X1 + 10
				rowY0 = grouped[i][0].Y0
				rowY1 = grouped[i][0].Y1
			}
			grouped[i] = append(grouped[i], TSRCell{X0: lastX, X1: lastX + 1, Y0: rowY0, Y1: rowY1})
		}
	}

	return grouped
}
