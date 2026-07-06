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
	if len(rows) == 0 || len(rows[0]) == 0 { return spanInfo, covered }

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
			if j >= nCols { continue }
			// Exclude spanning cells from column/row boundary calculations.
			// Use label-based detection (O(1), no dependency on column midpoints).
			if strings.Contains(cell.Label, "spanning") { continue }
			if cell.X0 < colLeft[j] { colLeft[j] = cell.X0 }
			if cell.X1 > colRight[j] { colRight[j] = cell.X1 }
			if cell.Y0 < rowTop[i] { rowTop[i] = cell.Y0 }
			if cell.Y1 > rowBott[i] { rowBott[i] = cell.Y1 }
		}
	}

	// For each spanning cell, compute how many cols/rows it covers.
	for i, row := range rows {
		for j, cell := range row {
			if j >= nCols || covered[[2]int{i,j}] { continue }
			// Skip cells without position data (they can't span).
			if cell.X0 == 0 && cell.X1 == 0 && cell.Y0 == 0 && cell.Y1 == 0 { continue }
			cs, rs := 1, 1
			// Count columns whose center is inside this cell's X range.
			for k := j+1; k < nCols; k++ {
				// Skip columns with no non-spanning cells (initial values unchanged).
				if colLeft[k] == 1e9 && colRight[k] == -1e9 { continue }
				colCenter := (colLeft[k] + colRight[k]) / 2
				if colCenter >= cell.X0 && colCenter <= cell.X1 { cs++ }
			}
			// Count rows whose center is inside this cell's Y range.
			for k := i+1; k < nRows; k++ {
				// Skip rows with no non-spanning cells.
				if rowTop[k] == 1e9 && rowBott[k] == -1e9 { continue }
				rowCenter := (rowTop[k] + rowBott[k]) / 2
				if rowCenter >= cell.Y0 && rowCenter <= cell.Y1 { rs++ }
			}
			if cs > 1 || rs > 1 {
				spanInfo[[2]int{i,j}] = [2]int{cs, rs}
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

// flattenGrid flattens a 2D grid into a 1D slice for fillCellTextFromBoxes.
func FlattenGrid(grid [][]pdf.TSRCell) []pdf.TSRCell {
	n := 0
	for _, row := range grid { n += len(row) }
	flat := make([]pdf.TSRCell, 0, n)
	for _, row := range grid { flat = append(flat, row...) }
	return flat
}
