package table

import (
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// gridBounds returns the per-column and per-row geometry from non-spanning
// cells. Spanning cells are excluded because their bbox is extended over the
// covered region; zero-position padding cells are excluded because they would
// drag boundaries to the origin. Both span detection and R/C annotation use
// these bounds, so the two stages agree on the table's real column edges.
func gridBounds(rows [][]pdf.TSRCell) (colLeft, colRight, rowTop, rowBott []float64) {
	if len(rows) == 0 || len(rows[0]) == 0 {
		return nil, nil, nil, nil
	}
	nCols, nRows := len(rows[0]), len(rows)
	colLeft = make([]float64, nCols)
	colRight = make([]float64, nCols)
	rowTop = make([]float64, nRows)
	rowBott = make([]float64, nRows)
	for j := 0; j < nCols; j++ {
		colLeft[j], colRight[j] = 1e9, -1e9
	}
	for i := 0; i < nRows; i++ {
		rowTop[i], rowBott[i] = 1e9, -1e9
	}
	for i, row := range rows {
		for j, cell := range row {
			if j >= nCols || strings.Contains(cell.Label, "spanning") {
				continue
			}
			if cell.X0 == 0 && cell.X1 == 0 && cell.Y0 == 0 && cell.Y1 == 0 {
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
	return colLeft, colRight, rowTop, rowBott
}

// calSpans computes colspan and rowspan for spanning cells in the grid.
// Returns spanInfo (row,col → colspan,rowspan) and covered (cells hidden by spans).
// Matches Python's __cal_spans (table_structure_recognizer.py:535).
func CalSpans(rows [][]pdf.TSRCell) (map[[2]int][2]int, map[[2]int]bool) {
	spanInfo := make(map[[2]int][2]int)
	covered := make(map[[2]int]bool)
	if len(rows) == 0 || len(rows[0]) == 0 {
		return spanInfo, covered
	}

	colLeft, colRight, rowTop, rowBott := gridBounds(rows)
	if colLeft == nil {
		return spanInfo, covered
	}
	nCols := len(colLeft)
	nRows := len(rowTop)

	// Only cells explicitly labelled "spanning" participate (Python's SP).
	// A plain header must never become a span origin: a cross-page merge can
	// widen a header bbox over a neighbouring column.
	for i, row := range rows {
		for j, cell := range row {
			if j >= nCols || covered[[2]int{i, j}] {
				continue
			}
			if !strings.Contains(cell.Label, "spanning") {
				continue
			}
			// Skip cells without position data (they can't span).
			if cell.X0 == 0 && cell.X1 == 0 && cell.Y0 == 0 && cell.Y1 == 0 {
				continue
			}
			// Collect every column/row whose center lies inside this cell's
			// X/Y range — on BOTH sides of the origin, matching Python's
			// __cal_spans (it iterates all columns j != b["cn"], not just the
			// ones to the right). A leftward span (SP box at the right edge
			// covering a column to its left) is therefore detected too.
			csCols := []int{j}
			for k := 0; k < nCols; k++ {
				if k == j {
					continue
				}
				// Skip columns with no non-spanning cells (initial values unchanged).
				if colLeft[k] == 1e9 && colRight[k] == -1e9 {
					continue
				}
				colCenter := (colLeft[k] + colRight[k]) / 2
				if colCenter >= cell.X0 && colCenter <= cell.X1 {
					csCols = append(csCols, k)
				}
			}
			rsRows := []int{i}
			for k := 0; k < nRows; k++ {
				if k == i {
					continue
				}
				// Skip rows with no non-spanning cells.
				if rowTop[k] == 1e9 && rowBott[k] == -1e9 {
					continue
				}
				rowCenter := (rowTop[k] + rowBott[k]) / 2
				if rowCenter >= cell.Y0 && rowCenter <= cell.Y1 {
					rsRows = append(rsRows, k)
				}
			}
			if len(csCols) <= 1 && len(rsRows) <= 1 {
				continue
			}
			// The covered region is the full contiguous rectangle from the
			// min to the max covered index (Python: colspan/rowspan =
			// range(min, max+1)), so cs/rs are the rectangle's width/height.
			minC, maxC := j, j
			for _, c := range csCols {
				if c < minC {
					minC = c
				}
				if c > maxC {
					maxC = c
				}
			}
			minR, maxR := i, i
			for _, r := range rsRows {
				if r < minR {
					minR = r
				}
				if r > maxR {
					maxR = r
				}
			}
			cs, rs := maxC-minC+1, maxR-minR+1
			spanInfo[[2]int{i, j}] = [2]int{cs, rs}
			// Mark covered cells first, unconditionally — a covered cell
			// is dropped from rendered output whether or not it carries
			// text (Python sets tbl[r][c] = None for every covered cell).
			for ri := minR; ri <= maxR && ri < nRows; ri++ {
				for cj := minC; cj <= maxC && cj < nCols; cj++ {
					if cj >= len(rows[ri]) {
						continue // ragged row (post-cleanup grid)
					}
					if ri != i || cj != j {
						covered[[2]int{ri, cj}] = true
					}
				}
			}
			// Fold covered-cell text into the span origin, matching Python's
			// __cal_spans. Covered cells keep their bbox and text, so this is
			// where their text reaches the rendered span cell.
			var merged []string
			seen := map[string]bool{}
			for ri := minR; ri <= maxR && ri < nRows; ri++ {
				for cj := minC; cj <= maxC && cj < nCols; cj++ {
					if cj >= len(rows[ri]) {
						continue
					}
					txt := strings.TrimSpace(rows[ri][cj].Text)
					if txt == "" || seen[txt] {
						continue
					}
					seen[txt] = true
					merged = append(merged, txt)
				}
			}
			if len(merged) > 0 {
				rows[i][j].Text = strings.Join(merged, " ")
			}
		}
	}
	return spanInfo, covered
}

// MarkCoveredCells tags every cell covered by a span with a "covered" label
// so downstream consumers can drop them, matching Python's HTML output, which
// omits covered <td> entirely (construct_table __html_table skips arr is
// None). The parity harness reads this to reproduce Python's per-row column
// counts (a span merges covered columns into one rendered cell).
func MarkCoveredCells(rows [][]pdf.TSRCell, covered map[[2]int]bool) {
	for pos := range covered {
		r, c := pos[0], pos[1]
		if r >= len(rows) || c >= len(rows[r]) {
			continue
		}
		if rows[r][c].Label != "" {
			rows[r][c].Label += " "
		}
		rows[r][c].Label += "table covered"
	}
}

// flattenGrid flattens a 2D grid into a 1D slice for fillCellTextFromBoxes.
func FlattenGrid(grid [][]pdf.TSRCell) []pdf.TSRCell {
	n := 0
	for _, row := range grid {
		n += len(row)
	}
	flat := make([]pdf.TSRCell, 0, n)
	for _, row := range grid {
		flat = append(flat, row...)
	}
	return flat
}
