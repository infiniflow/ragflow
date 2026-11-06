package table

import (
	"math"
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// rb is a row-box entry that holds cell data during grid construction.
type rb struct {
	txt            string
	x0, y0, x1, y1 float64
	label          string
}

// GroupBoxesByRC groups text boxes into a cell grid by R/C annotations.
// Matches Python's construct_table: sort_R_firstly, then split rows when the
// R label changes; boxes without an R (a row the TSR model missed) also split
// on a Y advance. A missing-R box keeps its Y position instead of collapsing
// into row 0, so the missed row is still assembled from its text boxes.
func GroupBoxesByRC(boxes []pdf.TextBox) [][]pdf.TSRCell {
	if len(boxes) == 0 {
		return nil
	}
	// If no box carries an R annotation, fall back to YX coordinate grouping —
	// matching Python's construct_table when every R is missing.
	hasR := false
	for _, b := range boxes {
		if b.R >= 0 {
			hasR = true
			break
		}
	}
	if !hasR {
		return GroupBoxesByYX(boxes)
	}

	sortRFirstly(boxes)

	rowOf := assignRows(boxes)
	compressed := rowOf[len(rowOf)-1]

	cmap := collectBoxesPerRow(boxes, rowOf)
	cCompressed, maxCol := compressColIndices(boxes, rowOf, compressed)

	return buildGrid(cmap, cCompressed, maxCol, compressed)
}

// sortRFirstly mirrors Python's Recognizer.sort_R_firstly: sort by Y first
// (with a fuzzy row-height threshold), then bubble boxes that carry an R label
// into R order. Boxes without an R remain at their Y position and act as
// barriers, so a missing-R row stays between the rows that surround it.
func sortRFirstly(boxes []pdf.TextBox) {
	rowh := 0.0
	for _, b := range boxes {
		if b.R >= 0 && b.RBott > b.RTop {
			h := b.RBott - b.RTop
			if rowh == 0 || h < rowh {
				rowh = h
			}
		}
	}

	sortYFirstlyTextBoxes(boxes, rowh/2)

	for i := 0; i < len(boxes)-1; i++ {
		for j := i; j >= 0; j-- {
			a, b := boxes[j], boxes[j+1]
			if a.R < 0 || b.R < 0 {
				continue
			}
			if b.R < a.R || (b.R == a.R && b.X0 < a.X0) {
				boxes[j], boxes[j+1] = b, a
			}
		}
	}
}

// sortYFirstlyTextBoxes is Python's Recognizer.sort_Y_firstly for text boxes:
// primary key is the top edge, with X as the tie-breaker when two boxes are
// within threshold. Pages are ordered first because Go boxes carry page-local
// Y coordinates, while Python's Y values are page-cumulative.
func sortYFirstlyTextBoxes(boxes []pdf.TextBox, threshold float64) {
	sort.SliceStable(boxes, func(i, j int) bool {
		if boxes[i].PageNumber != boxes[j].PageNumber {
			return boxes[i].PageNumber < boxes[j].PageNumber
		}
		diff := boxes[i].Top - boxes[j].Top
		if math.Abs(diff) < threshold {
			return boxes[i].X0 < boxes[j].X0
		}
		return diff < 0
	})
}

// assignRows mirrors Python's construct_table row loop: a new row starts when
// the R label changes. When R is missing, a box whose top has advanced past
// the current row's running bottom also starts a new row (Python's -1/-2
// sentinel comparison); same-R lines stay together so a multi-line cell is not
// split. The running bottom is averaged across the row's boxes, matching
// Python's `btm = (btm + b["bottom"]) / 2`.
func assignRows(boxes []pdf.TextBox) []int {
	rowOf := make([]int, len(boxes))
	if len(boxes) == 0 {
		return rowOf
	}
	btm := boxes[0].Bottom
	for i := 1; i < len(boxes); i++ {
		// Python: a new row when R changes; when R is missing (our -1),
		// a Y advance also starts a new row (Python's -1 vs -2 sentinel
		// comparison). Same-R boxes on different lines stay in one row, so a
		// multi-line cell is not split into two rows.
		if boxes[i-1].R != boxes[i].R || (boxes[i].R < 0 && boxes[i].Top >= btm-3) {
			rowOf[i] = rowOf[i-1] + 1
			btm = boxes[i].Bottom
			continue
		}
		rowOf[i] = rowOf[i-1]
		btm = (btm + boxes[i].Bottom) / 2
	}
	return rowOf
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
	// SP wins over H: a header cell that is also a span origin must carry the
	// spanning label or CalSpans drops its rowspan/colspan. Python's
	// __cal_spans only spans cells with "SP"; header detection reads b.H
	// directly and does not depend on this label.
	if b.SP > 0 || b.H > 0 {
		if b.SP > 0 {
			label = "table spanning cell"
		} else {
			label = "table header"
		}
		// A pure-SP box (H==0) still carries the span's full extent in
		// HLeft/HRight/HTop/HBott, copied together from the TSR spanning cell
		// by AnnotateTableBoxes (PR #18707). Rebuild the cell from those bounds
		// so CalSpans covers every column/row the span crosses — matching
		// Python's _annotate_table_boxes. Without this, the span cell falls
		// back to the box's own narrow text bounds, under-covers by one
		// column, and the header row keeps an extra empty cell (real_pdfs/1.pdf
		// row0 = 4 vs Python's 3; see known_diffs rule
		// table-1pdf-colspan-assembly-loss, resolved).
		//
		// HLeft/HRight (and HTop/HBott) are set together, so treat an axis as
		// propagated whenever either edge is set. This preserves a real edge at
		// coordinate 0 (a nonzero check would mistake it for "unset" and fall
		// back to the box's own bounds, dropping the span's left/top edge).
		if b.HLeft != 0 || b.HRight != 0 {
			x0, x1 = b.HLeft, b.HRight
		}
		if b.HTop != 0 || b.HBott != 0 {
			y0, y1 = b.HTop, b.HBott
		}
	}
	return
}

// collectBoxesPerRow collects boxes into row groups, merging boxes in the same cell.
// rowOf holds the row index assigned to each box by assignRows.
// Returns cmap (row → col → entry).
func collectBoxesPerRow(boxes []pdf.TextBox, rowOf []int) map[int]map[int]*rb {
	cmap := make(map[int]map[int]*rb) // row → col → entry
	for i, b := range boxes {
		t := strings.TrimSpace(b.Text)
		// Keep boxes with SP/H annotations even if text is empty —
		// their coordinates are needed for colspan/rowspan calculation.
		if t == "" && b.H <= 0 && b.SP <= 0 {
			continue
		}
		r := rowOf[i]
		c := b.C
		if cmap[r] == nil {
			cmap[r] = make(map[int]*rb)
		}
		x0, y0, x1, y1, label := cellPosFromBox(b)
		if v, ok := cmap[r][c]; ok {
			if t != "" {
				v.txt += " " + t
			}
			// Merge spanning coordinates (use widest extent). A spanning
			// label must survive a later header-only box in the same cell:
			// Python's __cal_spans only spans SP cells, so overwriting
			// "table spanning cell" with "table header" drops the span.
			if b.SP > 0 {
				v.label = "table spanning cell"
			} else if b.H > 0 && v.label != "table spanning cell" {
				v.label = "table header"
			}
			if b.H > 0 || b.SP > 0 {
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
			cmap[r][c] = &rb{txt: t, x0: x0, y0: y0, x1: x1, y1: y1, label: label}
		}
	}
	return cmap
}

// compressColIndices assigns every box a column from a GLOBAL rank of the
// table's distinct C labels, mirroring Python's construct_table which groups
// boxes into columns by their per-char C assignment (each distinct C yields a
// column, in C order; a box whose C is the previous box's C+1 always starts a
// new column — Python never merges adjacent C values by X overlap).
//
// A table-wide mapping keeps the same C in the same column across ALL rows: a
// per-row re-compression would misalign cell text (a C=4 box landing in
// column 3 in one row and column 4 in another). Every row is sized to the
// table-wide column count, matching Python's uniform-width construct_table
// output (rows missing a C render empty cells).
func compressColIndices(boxes []pdf.TextBox, rowOf []int, compressed int) (map[int]map[int]int, int) {
	distinctC := make(map[int]bool)
	for _, b := range boxes {
		if b.C >= 0 {
			distinctC[b.C] = true
		}
	}
	cs := make([]int, 0, len(distinctC))
	for c := range distinctC {
		cs = append(cs, c)
	}
	sort.Ints(cs)
	rank := make(map[int]int, len(cs))
	for i, c := range cs {
		rank[c] = i
	}
	maxCol := len(cs) - 1

	cCompressed := make(map[int]map[int]int) // row → (original C → compressed col)
	for ri := 0; ri <= compressed; ri++ {
		cMap := make(map[int]int)
		for i, b := range boxes {
			if rowOf[i] != ri {
				continue
			}
			if strings.TrimSpace(b.Text) == "" && b.H <= 0 && b.SP <= 0 {
				continue
			}
			if cr, ok := rank[b.C]; ok {
				cMap[b.C] = cr
			}
		}
		cCompressed[ri] = cMap
	}
	return cCompressed, maxCol
}

// buildGrid builds the final cell grid from the collected and compressed data.
func buildGrid(cmap map[int]map[int]*rb, cCompressed map[int]map[int]int, maxCol, compressed int) [][]pdf.TSRCell {
	rows := make([][]pdf.TSRCell, compressed+1)
	for ri := 0; ri <= compressed; ri++ {
		rows[ri] = make([]pdf.TSRCell, maxCol+1)
		for ci, v := range cmap[ri] {
			cci := cCompressed[ri][ci]
			if cci <= maxCol {
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
