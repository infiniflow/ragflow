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
	// Only cells that actually represent a span participate: a "table
	// spanning cell" (GroupCells injects this label for TSR span components)
	// or a "table header" cell whose HLeft/HRight bbox straddles neighbour
	// columns (GroupBoxesByRC path, e.g. TestRowsToHTML_Colspan). Python's
	// __cal_spans iterates boxes carrying an SP/H annotation, never every
	// grid cell. Computing cs/rs for arbitrary cells is unsafe: a normal cell
	// whose bbox happens to straddle the next column's center would be
	// misclassified as a span, and its covered neighbours would then be
	// dropped from the rendered rows (a real regression for span-free tables
	// like 公司差旅费管理办法).
	for i, row := range rows {
		for j, cell := range row {
			if j >= nCols || covered[[2]int{i, j}] {
				continue
			}
			if !strings.Contains(cell.Label, "spanning") && !strings.Contains(cell.Label, "table header") {
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
				// Mark covered cells first, unconditionally — a covered cell
				// is dropped from rendered output whether or not it carries
				// text (Python sets tbl[r][c] = None for every covered cell).
				for ri := i; ri < i+rs && ri < nRows; ri++ {
					for cj := j; cj < j+cs && cj < nCols; cj++ {
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
				for ri := i; ri < i+rs && ri < nRows; ri++ {
					for cj := j; cj < j+cs && cj < nCols; cj++ {
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
