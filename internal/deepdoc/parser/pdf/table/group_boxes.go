package table

import (
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// rb is a row-box entry that holds cell data during grid construction.
type rb struct {
	row, col       int
	txt            string
	x0, y0, x1, y1 float64
	label          string
}

// GroupBoxesByRC groups text boxes into a cell grid by R/C annotations.
// Matches Python's construct_table: sort by R, sort by C within each row,
// merge nearby columns by X proximity.
func GroupBoxesByRC(boxes []pdf.TextBox) [][]pdf.TSRCell {
	if len(boxes) == 0 {
		return nil
	}
	// If no real R/C annotations (maxR <= 0), fall back to YX coordinate
	// grouping — matching Python's construct_table when all R=-1.
	maxR := 0
	for _, b := range boxes {
		if b.R > maxR {
			maxR = b.R
		}
	}
	if maxR <= 0 {
		return GroupBoxesByYX(boxes)
	}
	// Sort by R index first (Python: sort_R_firstly), then Y, then X.
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].R != boxes[j].R {
			return boxes[i].R < boxes[j].R
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	// Compress R indices: Python's sort_R_firstly grouping.
	rowMap, compressed := compressRowIndices(boxes)

	// Collect boxes per row.
	cmap, _ := collectBoxesPerRow(boxes, rowMap)

	// Compress C indices per row.
	cCompressed, cMaxCol := compressColIndices(boxes, rowMap, compressed)

	// Build grid.
	return buildGrid(cmap, cCompressed, cMaxCol, compressed)
}

// GroupBoxesByYX groups boxes into a cell grid by Y/X coordinates,
// matching Python's construct_table which uses sort_R_firstly and
// sort_C_firstly when R/C annotations are absent. Falls back from
// GroupBoxesByRC when boxes lack R/C annotations.
func GroupBoxesByYX(boxes []pdf.TextBox) [][]pdf.TSRCell {
	if len(boxes) == 0 {
		return nil
	}
	// Sort by (page, top, x0) — same as Python sort_R_firstly with R=-1.
	sort.Slice(boxes, func(i, j int) bool {
		if boxes[i].PageNumber != boxes[j].PageNumber {
			return boxes[i].PageNumber < boxes[j].PageNumber
		}
		if boxes[i].Top != boxes[j].Top {
			return boxes[i].Top < boxes[j].Top
		}
		return boxes[i].X0 < boxes[j].X0
	})

	// Group into rows by Y proximity (Python's row grouping).
	type rowGroup struct {
		boxes    []pdf.TextBox
		top, btm float64
	}
	var rowGroups []rowGroup
	rowGroups = append(rowGroups, rowGroup{
		boxes: []pdf.TextBox{boxes[0]},
		top:   boxes[0].Top,
		btm:   boxes[0].Bottom,
	})
	for i := 1; i < len(boxes); i++ {
		prev := &rowGroups[len(rowGroups)-1]
		// Python: same row if top < prev.btm (Y overlaps) and same page.
		if boxes[i].PageNumber == prev.boxes[0].PageNumber && boxes[i].Top < prev.btm {
			prev.boxes = append(prev.boxes, boxes[i])
			if boxes[i].Top < prev.top {
				prev.top = boxes[i].Top
			}
			if boxes[i].Bottom > prev.btm {
				prev.btm = boxes[i].Bottom
			}
		} else {
			rowGroups = append(rowGroups, rowGroup{
				boxes: []pdf.TextBox{boxes[i]},
				top:   boxes[i].Top,
				btm:   boxes[i].Bottom,
			})
		}
	}

	// Within each row, group into columns by X proximity.
	rows := make([][]pdf.TSRCell, len(rowGroups))
	for ri, rg := range rowGroups {
		// Sort by X0.
		sort.Slice(rg.boxes, func(i, j int) bool {
			return rg.boxes[i].X0 < rg.boxes[j].X0
		})
		// Group by X overlap.
		var cols []struct {
			boxes []pdf.TextBox
			x1    float64
		}
		cols = append(cols, struct {
			boxes []pdf.TextBox
			x1    float64
		}{
			boxes: []pdf.TextBox{rg.boxes[0]},
			x1:    rg.boxes[0].X1,
		})
		for i := 1; i < len(rg.boxes); i++ {
			prev := &cols[len(cols)-1]
			if rg.boxes[i].X0 < prev.x1 {
				prev.boxes = append(prev.boxes, rg.boxes[i])
				if rg.boxes[i].X1 > prev.x1 {
					prev.x1 = rg.boxes[i].X1
				}
			} else {
				cols = append(cols, struct {
					boxes []pdf.TextBox
					x1    float64
				}{
					boxes: []pdf.TextBox{rg.boxes[i]},
					x1:    rg.boxes[i].X1,
				})
			}
		}
		rows[ri] = make([]pdf.TSRCell, len(cols))
		for ci, col := range cols {
			var sb strings.Builder
			for _, b := range col.boxes {
				t := strings.TrimSpace(b.Text)
				if t == "" {
					continue
				}
				if sb.Len() > 0 {
					sb.WriteByte(' ')
				}
				sb.WriteString(t)
			}
			rows[ri][ci].Text = sb.String()
		}
	}
	return rows
}

// cellPosFromBox returns the position coordinates and label for a cell
// derived from a text box.  Header cells use HLeft/HRight/HTop/HBott
// for spanning-aware positions; regular cells use the box's own bounds.
func cellPosFromBox(b pdf.TextBox) (x0, y0, x1, y1 float64, label string) {
	x0, y0, x1, y1 = b.X0, b.Top, b.X1, b.Bottom
	if b.H > 0 {
		label = "table header"
		if b.HLeft != 0 || b.HRight != 0 {
			if b.HLeft != 0 {
				x0 = b.HLeft
			}
			if b.HRight != 0 {
				x1 = b.HRight
			}
		}
		if b.HTop != 0 {
			y0 = b.HTop
		}
		if b.HBott != 0 {
			y1 = b.HBott
		}
	} else if b.SP > 0 {
		label = "table spanning cell"
	}
	return
}

// cellLabelFromBox returns the TSR label for a box based on H/SP annotations.
// Used when merging multiple boxes into one cell — preserves the spanning label.
func cellLabelFromBox(b pdf.TextBox) string {
	if b.H > 0 {
		return "table header"
	}
	if b.SP > 0 {
		return "table spanning cell"
	}
	return ""
}

// compressRowIndices compresses R values into contiguous row indices.
// Returns rowMap (original R → compressed index) and the maximum compressed index.
// Boxes must already be sorted by R, Y, X.
func compressRowIndices(boxes []pdf.TextBox) (map[int]int, int) {
	rowMap := make(map[int]int) // original R → compressed row index
	compressed := 0
	rowMap[boxes[0].R] = 0
	lastR := boxes[0].R
	for i := 1; i < len(boxes); i++ {
		if boxes[i].R != lastR {
			compressed++
			rowMap[boxes[i].R] = compressed
			lastR = boxes[i].R
		} else {
			rowMap[boxes[i].R] = compressed
		}
	}
	return rowMap, compressed
}

// collectBoxesPerRow collects boxes into row groups, merging boxes in the same cell.
// Returns cmap (row → col → entry) and maxCols (max column index per row).
func collectBoxesPerRow(boxes []pdf.TextBox, rowMap map[int]int) (map[int]map[int]*rb, map[int]int) {
	cmap := make(map[int]map[int]*rb) // row → col → entry
	maxCols := make(map[int]int)
	for _, b := range boxes {
		t := strings.TrimSpace(b.Text)
		// Keep boxes with SP/H annotations even if text is empty —
		// their coordinates are needed for colspan/rowspan calculation.
		if t == "" && b.H <= 0 && b.SP <= 0 {
			continue
		}
		r := rowMap[b.R]
		c := b.C
		if cmap[r] == nil {
			cmap[r] = make(map[int]*rb)
		}
		x0, y0, x1, y1, label := cellPosFromBox(b)
		if v, ok := cmap[r][c]; ok {
			if t != "" {
				v.txt += " " + t
			}
			// Merge spanning coordinates (use widest extent).
			if b.H > 0 || b.SP > 0 {
				v.label = cellLabelFromBox(b)
				if v.x0 > x0 {
					v.x0 = x0
				}
				if v.y0 > y0 {
					v.y0 = y0
				}
				if v.x1 < x1 {
					v.x1 = x1
				}
				if v.y1 < y1 {
					v.y1 = y1
				}
			}
		} else {
			cmap[r][c] = &rb{r, c, t, x0, y0, x1, y1, label}
		}
		if c > maxCols[r] {
			maxCols[r] = c
		}
	}
	return cmap, maxCols
}

// rowBox is a helper for compressColIndices.
type rowBox struct {
	c, idx int
	x0, x1 float64
	txt    string
}

// compressColIndices compresses column indices per row based on X0 ordering and overlap.
// Returns cCompressed (row → original C → compressed C) and cMaxCol (max compressed C per row).
func compressColIndices(boxes []pdf.TextBox, rowMap map[int]int, compressed int) (map[int]map[int]int, map[int]int) {
	cCompressed := make(map[int]map[int]int) // row → (original C → compressed col)
	cMaxCol := make(map[int]int)
	for ri := 0; ri <= compressed; ri++ {
		// Collect all boxes in this row, sorted by X0.
		var rowBoxes []rowBox
		for i, b := range boxes {
			if rowMap[b.R] == ri && (strings.TrimSpace(b.Text) != "" || b.H > 0 || b.SP > 0) {
				rowBoxes = append(rowBoxes, rowBox{c: b.C, idx: i, x0: b.X0, x1: b.X1, txt: b.Text})
			}
		}
		sort.Slice(rowBoxes, func(i, j int) bool { return rowBoxes[i].x0 < rowBoxes[j].x0 })
		// Assign compressed column by X-order (disjoint X → new col).
		cMap := make(map[int]int) // original C → compressed col
		right := 0.0
		nCols := 0
		for _, rb := range rowBoxes {
			if len(cMap) == 0 || rb.x0 >= right {
				cMap[rb.c] = nCols
				nCols++
				right = rb.x1
			} else {
				// Overlapping X → merge into last column.
				cMap[rb.c] = nCols - 1
				if rb.x1 > right {
					right = rb.x1
				}
			}
		}
		cCompressed[ri] = cMap
		cMaxCol[ri] = nCols - 1
	}
	return cCompressed, cMaxCol
}

// buildGrid builds the final cell grid from the collected and compressed data.
func buildGrid(cmap map[int]map[int]*rb, cCompressed map[int]map[int]int, cMaxCol map[int]int, compressed int) [][]pdf.TSRCell {
	rows := make([][]pdf.TSRCell, compressed+1)
	for ri := 0; ri <= compressed; ri++ {
		maxC := cMaxCol[ri]
		rows[ri] = make([]pdf.TSRCell, maxC+1)
		for ci, v := range cmap[ri] {
			cci := cCompressed[ri][ci]
			if cci <= maxC {
				if rows[ri][cci].Text == "" {
					rows[ri][cci].Text = v.txt
					rows[ri][cci].X0 = v.x0
					rows[ri][cci].Y0 = v.y0
					rows[ri][cci].X1 = v.x1
					rows[ri][cci].Y1 = v.y1
					rows[ri][cci].Label = v.label
				} else {
					// Multiple originals map to same compressed cell — merge deterministically.
					if v.txt != "" {
						rows[ri][cci].Text += " " + v.txt
					}
					if v.x0 < rows[ri][cci].X0 {
						rows[ri][cci].X0 = v.x0
					}
					if v.y0 < rows[ri][cci].Y0 {
						rows[ri][cci].Y0 = v.y0
					}
					if v.x1 > rows[ri][cci].X1 {
						rows[ri][cci].X1 = v.x1
					}
					if v.y1 > rows[ri][cci].Y1 {
						rows[ri][cci].Y1 = v.y1
					}
					if rows[ri][cci].Label == "" && v.label != "" {
						rows[ri][cci].Label = v.label
					}
				}
			}
		}
	}
	return rows
}

func BoxesHaveAnnotations(boxes []pdf.TextBox) bool {
	maxR, maxC := 0, 0
	for _, b := range boxes {
		if b.R > maxR {
			maxR = b.R
		}
		if b.C > maxC {
			maxC = b.C
		}
	}
	// True if at least 2 rows or 2 cols (R/C are 0-based, so maxR>0 means ≥2 rows).
	return maxR > 0 || maxC > 0
}
