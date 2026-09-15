package table

import (
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// calSpans computes colspan and rowspan for spanning cells in the grid.
// Returns spanInfo (row,col → colspan,rowspan) and covered (cells hidden by spans).
// Matches Python's __cal_spans (table_structure_recognizer.py:535).
func CalSpans(rows [][]pdf.TSRCell) (map[[2]int][2]int, map[[2]int]bool) {
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
			// Cells without position data (e.g. the zero-coordinate cells
			// padded by MergeTablesAcrossPages to align per-page column
			// counts) must not define column/row geometry, or they would drag
			// boundaries to the origin and corrupt span detection.
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

	// For each spanning cell, compute how many cols/rows it covers.
	// Only cells explicitly recognized as a span participate. Python's
	// __cal_spans (table_structure_recognizer.py:500-528) iterates boxes that
	// carry an "SP" annotation (`if "SP" not in b: continue`) and never
	// treats an ordinary "table header" as a span origin. In Go the TSR
	// "table spanning cell" component is the SP equivalent and is labelled
	// "spanning" by GroupBoxesByRC/GroupCells, so that is the only branch we
	// honour here.
	//
	// A previous Go-only branch also spanned "table header" cells whose
	// HLeft/HRight bbox straddled a neighbour column. That diverges from
	// Python and is unsafe: when a cross-page merge widens a header cell's
	// bbox (e.g. icbccs "类型" after MergeTablesAcrossPages stacks two pages'
	// grids), its X1 can reach past the neighbour column's center and get
	// misclassified as a colspan, folding the neighbour's text in and dropping
	// a column from the rendered row. Removing the header branch makes Go
	// match Python and avoids that regression.
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
			// Fold the covered cells' text into the span origin cell,
			// matching Python's __cal_spans (table_structure_recognizer.py:
			// 530-577): it walks the covered region row-major and extends
			// the span cell's text with every covered cell's text (skipping
			// already-folded duplicates via join(arr)). Because Go's
			// GroupCells no longer zeroes covered bboxes, those cells hold
			// their own box text by fill time; without this fold the span
			// cell would render empty and the covered text would be lost.
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
