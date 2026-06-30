package table

import (
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

// ── construct table ─────────────────────────────────────────────────────

// MergeTablesAcrossPages merges TableItems on consecutive pages with
// overlapping X and close Y proximity.  Matches Python's
// _extract_table_figure table merge (pdf_parser.py:1061-1080).
func MergeTablesAcrossPages(tables []pdf.TableItem, medianHeights map[int]float64) []pdf.TableItem {
	if len(tables) <= 1 {
		return tables
	}
	// Sort by position for deterministic adjacency.
	type indexed struct {
		idx int
		pg  int
		top float64
	}
	var items []indexed
	for i, tbl := range tables {
		if len(tbl.Positions) == 0 {
			continue
		}
		p := tbl.Positions[0]
		pg := 0
		if len(p.PageNumbers) > 0 {
			pg = p.PageNumbers[0]
		}
		items = append(items, indexed{i, pg, p.Top})
	}
	sort.Slice(items, func(a, b int) bool {
		if items[a].pg != items[b].pg {
			return items[a].pg < items[b].pg
		}
		return items[a].top < items[b].top
	})

	merged := make([]bool, len(tables))
	var result []pdf.TableItem

	for _, it := range items {
		if merged[it.idx] {
			continue
		}
		anchor := tables[it.idx]
		merged[it.idx] = true

		// Python nomerge_lout_no: tables whose box is followed by a
		// caption/title/reference should not be merged cross-page.
		if anchor.NoMerge {
			result = append(result, anchor)
			continue
		}

		anchorPg := it.pg
		anchorBott := anchor.Positions[0].Bottom

		// Look for consecutive-page continuations.
		for _, jt := range items {
			if merged[jt.idx] || jt.pg <= anchorPg {
				continue
			}
			// Python nomerge_lout_no: skip continuation candidates
			// tagged as no-merge.
			if tables[jt.idx].NoMerge {
				continue
			}
			if jt.pg-anchorPg > 1 {
				break // pages must be consecutive
			}
			if len(tables[jt.idx].Positions) == 0 {
				continue
			}
			bp := tables[jt.idx].Positions[0]
			bpg := 0
			if len(bp.PageNumbers) > 0 {
				bpg = bp.PageNumbers[0]
			}
			if bpg != anchorPg+1 {
				continue
			}
			// Check X overlap.
			ap := anchor.Positions[0]
			if ap.Right < bp.Left || bp.Right < ap.Left {
				continue
			}
			// Check Y proximity: page 1 table top should be close below
			// page 0 table bottom.  Python: y_dis ≤ mh * 23.
			mh := 10.0
			if medianHeights != nil {
				if h, ok := medianHeights[anchorPg]; ok && h > 0 {
					mh = h
				}
			}
			yDis := (bp.Top + bp.Bottom - anchorBott - ap.Bottom) / 2
			if yDis > mh*23 {
				continue
			}
			// Merge: combine cells and positions.
			anchor.Cells = append(anchor.Cells, tables[jt.idx].Cells...)
			anchor.Positions = append(anchor.Positions, tables[jt.idx].Positions...)
			if tables[jt.idx].Caption != "" {
				if anchor.Caption != "" {
					anchor.Caption += " "
				}
				anchor.Caption += tables[jt.idx].Caption
			}
			merged[jt.idx] = true
			anchorPg = bpg
			anchorBott = bp.Bottom
		}
		result = append(result, anchor)
	}
	return result
}

// constructTable produces an HTML table string from TSR cells and text boxes.
// Both cells and boxes must be in the same coordinate space (crop pixel space).
// Fills item.Rows so downstream consumers don't need to re-group cells.
//
// Python equivalent: TableStructureRecognizer.construct_table()
// stripCaptionFromCells clears caption-like text from TSR cells.
// This catches captions that fillCellTextFromBoxes missed (e.g. text
// that doesn't match isCaptionBox patterns like "公司差旅费管理办法").
// Only clears cells whose text matches caption patterns or that contain
// only number+separator text (pure "1. ", "一、" etc. without data).
func StripCaptionFromCells(cells []pdf.TSRCell) {
	for i := range cells {
		t := strings.TrimSpace(cells[i].Text)
		if t == "" {
			continue
		}
		// Clear cells that match caption patterns (e.g. "表1", "Table 1").
		if IsCaptionBox(t, "") {
			cells[i].Text = ""
		}
	}
	// Second pass: if the first row (lowest Y) has all-numeric/numbering text
	// (e.g. "1", "1.", "一"), it's likely a caption numbering line — clear it.
	// But don't clear actual numeric data cells.
	// This pass is intentionally conservative — only clears clearly-non-data text.
}

func ConstructTable(cells []pdf.TSRCell, boxes []pdf.TextBox, caption string, item *pdf.TableItem) string {
	// Strip caption-like text from cells (defense-in-depth: fillCellTextFromBoxes
	// may include caption text that doesn't match isCaptionBox patterns).
	StripCaptionFromCells(cells)

	// Use the pre-computed grid from pdf.TableBuilder.GroupCells.
	// Falls back to cell-level grouping only when called directly by
	// tests without a pre-computed Grid (production always sets it).
	var rows [][]pdf.TSRCell
	if item != nil {
		rows = item.Grid
	}
	if rows == nil && len(cells) > 0 && HasAnyText(cells) {
		rows = GroupTSRCellsToRows(cells)
	}
	if len(rows) > 0 && HasText(rows) {
		hdrs := HeaderSetWithBlockType(rows)
		if item != nil {
			item.Rows = RowsToStrings(rows)
		}
		rows = CleanupOrphanColumns(rows)
		spanInfo, covered := CalSpans(rows)
		return RowsToHTML(rows, caption, hdrs, spanInfo, covered)
	}
	// Fallback: boxes with R/C annotations.
	if len(boxes) > 0 && BoxesHaveAnnotations(boxes) {
		rows := GroupBoxesByRC(boxes)
		if HasText(rows) {
			if item != nil {
				item.Rows = RowsToStrings(rows)
			}
			spanInfo, covered := CalSpans(rows)
			return RowsToHTML(rows, caption, BoxHeaderSet(rows, boxes), spanInfo, covered)
		}
	}
	// Test-only: Y/X coordinate grouping (matching Python construct_table).
	// Used by table_parity_test.go to verify pipeline with Python boxes.
	if len(boxes) > 0 && !BoxesHaveAnnotations(boxes) {
		rows := GroupBoxesByYX(boxes)
		if HasText(rows) {
			if item != nil {
				item.Rows = RowsToStrings(rows)
			}
			spanInfo, covered := CalSpans(rows)
			return RowsToHTML(rows, caption, BoxHeaderSet(rows, boxes), spanInfo, covered)
		}
	}
	return ""
}

// boxHeaderSet returns rows that contain boxes with H annotations.
func BoxHeaderSet(rows [][]pdf.TSRCell, boxes []pdf.TextBox) map[int]bool {
	hdrs := make(map[int]bool)
	for _, b := range boxes {
		if b.H > 0 && b.R >= 0 && b.R < len(rows) {
			hdrs[b.R] = true
		}
	}
	return hdrs
}

func HasAnyText(cells []pdf.TSRCell) bool {
	for _, c := range cells {
		if strings.TrimSpace(c.Text) != "" {
			return true
		}
	}
	return false
}

// groupBoxesByRC groups text boxes into a cell grid by R/C annotations.
// Matches Python's construct_table: sort by R, merge nearby rows by Y proximity,
// sort by C within each row, merge nearby columns by X proximity.
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
	// R differs → always a new row.  Same R + Y gap → also new row.
	rowMap := make(map[int]int) // original R → compressed row index
	compressed := 0
	rowMap[boxes[0].R] = 0
	lastR := boxes[0].R
	btm := boxes[0].Bottom
	for i := 1; i < len(boxes); i++ {
		// Python: b["R"] != last_R → new row.
		// Same R → always same row (Python doesn't check Y for same R).
		if boxes[i].R != lastR {
			compressed++
			rowMap[boxes[i].R] = compressed
			lastR = boxes[i].R
			btm = boxes[i].Bottom
		} else {
			// Same R → same physical row.
			rowMap[boxes[i].R] = compressed
			btm = (btm + boxes[i].Bottom) / 2.0
		}
	}

	// Collect boxes per row, sort by C within each row.
	type rb struct {
		row, col       int
		txt            string
		x0, y0, x1, y1 float64
		label          string
	}
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
			v.txt += " " + t
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

	// Compress C indices per row: sort boxes by X0 within the row,
	// group disjoint X ranges into separate columns.  This is equivalent
	// to Python's sort_C_firstly but uses X0 ordering instead of C labels.
	cCompressed := make(map[int]map[int]int) // row → (original C → compressed col)
	cMaxCol := make(map[int]int)
	for ri := 0; ri <= compressed; ri++ {
		rowEntries := cmap[ri]
		if rowEntries == nil {
			continue
		}
		// Collect all boxes in this row, sorted by X0.
		type rowBox struct {
			c, idx int
			x0, x1 float64
			txt    string
		}
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
		for _, rb := range rowBoxes {
			if len(cMap) == 0 || rb.x0 >= right {
				cc := len(cMap)
				cMap[rb.c] = cc
				right = rb.x1
			} else {
				// Overlapping X → merge into last column.
				cMap[rb.c] = len(cMap) - 1
				if rb.x1 > right {
					right = rb.x1
				}
			}
		}
		cCompressed[ri] = cMap
		cMaxCol[ri] = len(cMap) - 1
	}

	// Build grid.
	rows := make([][]pdf.TSRCell, compressed+1)
	for ri := 0; ri <= compressed; ri++ {
		maxC := cMaxCol[ri]
		rows[ri] = make([]pdf.TSRCell, maxC+1)
		for ci, v := range cmap[ri] {
			cci := cCompressed[ri][ci]
			if cci <= maxC {
				rows[ri][cci].Text = v.txt
				rows[ri][cci].X0 = v.x0
				rows[ri][cci].Y0 = v.y0
				rows[ri][cci].X1 = v.x1
				rows[ri][cci].Y1 = v.y1
				rows[ri][cci].Label = v.label
			}
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

// groupBoxesByYX groups boxes into a cell grid by Y/X coordinates,
// matching Python's construct_table which uses sort_R_firstly and
// sort_C_firstly when R/C annotations are absent.
// This is test-only — used by table_parity_test.go to verify pipeline
// parity with Python boxes that lack R/C annotations.
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
	rowGroups = append(rowGroups, rowGroup{boxes: []pdf.TextBox{boxes[0]}, top: boxes[0].Top, btm: boxes[0].Bottom})
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
			rowGroups = append(rowGroups, rowGroup{boxes: []pdf.TextBox{boxes[i]}, top: boxes[i].Top, btm: boxes[i].Bottom})
		}
	}

	// Within each row, group into columns by X proximity.
	rows := make([][]pdf.TSRCell, len(rowGroups))
	for ri, rg := range rowGroups {
		// Sort by X0.
		sort.Slice(rg.boxes, func(i, j int) bool { return rg.boxes[i].X0 < rg.boxes[j].X0 })
		// Group by X overlap.
		var cols []struct {
			boxes []pdf.TextBox
			x1    float64
		}
		cols = append(cols, struct {
			boxes []pdf.TextBox
			x1    float64
		}{boxes: []pdf.TextBox{rg.boxes[0]}, x1: rg.boxes[0].X1})
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
				}{boxes: []pdf.TextBox{rg.boxes[i]}, x1: rg.boxes[i].X1})
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

func HasText(rows [][]pdf.TSRCell) bool {
	for _, row := range rows {
		for _, c := range row {
			if strings.TrimSpace(c.Text) != "" {
				return true
			}
		}
	}
	return false
}

func RowsToStrings(rows [][]pdf.TSRCell) [][]string {
	out := make([][]string, len(rows))
	for ri, row := range rows {
		out[ri] = make([]string, len(row))
		for ci, c := range row {
			out[ri][ci] = c.Text
		}
	}
	return out
}

// fillCellTextFromAnnotations fills cell text from text boxes using R/C labels.
// This matches Python's construct_table which assigns boxes to cells by their
// R (row) and C (col) annotations rather than spatial overlap.
func FillCellTextFromAnnotations(rows [][]pdf.TSRCell, boxes []pdf.TextBox) {
	// Build R→(C→text) map: row index → (col index → text).
	rBoxes := make(map[int]map[int][]string)
	for _, b := range boxes {
		if b.Text == "" {
			continue
		}
		if rBoxes[b.R] == nil {
			rBoxes[b.R] = make(map[int][]string)
		}
		rBoxes[b.R][b.C] = append(rBoxes[b.R][b.C], b.Text)
	}
	// Fill each cell from the matching R/C position.
	for ri, row := range rows {
		colMap := rBoxes[ri]
		if colMap == nil {
			continue
		}
		// Build sorted column list for positional matching.
		type colEntry struct {
			c     int
			texts []string
		}
		var cols []colEntry
		for c, texts := range colMap {
			cols = append(cols, colEntry{c, texts})
		}
		sort.Slice(cols, func(i, j int) bool { return cols[i].c < cols[j].c })
		for ci, col := range cols {
			if ci < len(row) {
				row[ci].Text = strings.TrimSpace(strings.Join(col.texts, " "))
			}
		}
	}
}

// dataSourceRe matches table/figure boxes that should be discarded as
// data-source attribution lines rather than extracted content.
//
// Python: pdf_parser.py:1040-1042, 1050-1052
//
//	re.match(r"(数据|资料|图表)*来源[:： ]", self.boxes[i]["text"])
var dataSourceRe = regexp.MustCompile(`^(数据|资料|图表)*来源[:： ]`)

// isDataSourceBox returns true if the box text matches the data-source
// discard pattern (Python's _extract_table_figure data-source filter).
func isDataSourceBox(text string) bool {
	return dataSourceRe.MatchString(text)
}

// tableRegionBox returns a pdf.TextBox for a table replacement, using DLA region
// boundaries when available (Region* set), falling back to anchor box coordinates.
// Python's insert_table_figures uses DLA layout region boundaries; the fallback
// handles test TableItems or bare engines without DLA.
func tableRegionBox(tbl *pdf.TableItem, ref *pdf.TextBox, html string) pdf.TextBox {
	pg := 0
	if len(tbl.Positions) > 0 && len(tbl.Positions[0].PageNumbers) > 0 {
		pg = tbl.Positions[0].PageNumbers[0]
	}
	// Use DLA region boundaries when set.
	if tbl.RegionLeft != 0 || tbl.RegionRight != 0 || tbl.RegionTop != 0 || tbl.RegionBottom != 0 {
		return pdf.TextBox{
			X0: tbl.RegionLeft, X1: tbl.RegionRight,
			Top: tbl.RegionTop, Bottom: tbl.RegionBottom,
			Text:       html,
			PageNumber: pg,
			LayoutType: pdf.LayoutTypeTable,
		}
	}
	// Fallback: use anchor box coordinates.
	x0, x1, top, bot := ref.X0, ref.X1, ref.Top, ref.Bottom
	return pdf.TextBox{
		X0: x0, X1: x1, Top: top, Bottom: bot,
		Text:       html,
		PageNumber: pg,
		LayoutType: pdf.LayoutTypeTable,
	}
}

// minRectangleDistance computes the Euclidean distance between two rectangles.
// Returns 0 when rectangles overlap.  Matches Python's min_rectangle_distance
// in insert_table_figures (pdf_parser.py:1609-1626).
func minRectangleDistance(left1, right1, top1, bottom1, left2, right2, top2, bottom2 float64) float64 {
	if right1 >= left2 && right2 >= left1 && bottom1 >= top2 && bottom2 >= top1 {
		return 0
	}
	var dx, dy float64
	if right1 < left2 {
		dx = left2 - right1
	} else if right2 < left1 {
		dx = left1 - right2
	}
	if bottom1 < top2 {
		dy = top2 - bottom1
	} else if bottom2 < top1 {
		dy = top1 - bottom2
	}
	return math.Sqrt(dx*dx + dy*dy)
}

func RowsToHTML(rows [][]pdf.TSRCell, caption string, headerRows map[int]bool, spanInfo map[[2]int][2]int, covered map[[2]int]bool) string {
	var b strings.Builder
	b.WriteString("<table>")
	if caption != "" {
		b.WriteString("<caption>")
		b.WriteString(caption)
		b.WriteString("</caption>")
	}
	for ri, row := range rows {
		b.WriteString("<tr>")
		for ci, cell := range row {
			if covered[[2]int{ri, ci}] {
				continue
			}
			tag := "td"
			if headerRows[ri] {
				tag = "th"
			}
			b.WriteString("<")
			b.WriteString(tag)
			sp := ""
			if s, ok := spanInfo[[2]int{ri, ci}]; ok {
				if s[0] > 1 {
					sp = fmt.Sprintf("colspan=%d", s[0])
				}
				if s[1] > 1 {
					if sp != "" {
						sp += " "
					}
					sp += fmt.Sprintf("rowspan=%d", s[1])
				}
			}
			if sp != "" {
				b.WriteString(" ")
				b.WriteString(sp)
			}
			b.WriteString(" >")
			b.WriteString(cell.Text)
			b.WriteString("</")
			b.WriteString(tag)
			b.WriteString(">")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</table>")
	return b.String()
}

// ── Span computation (Python: __cal_spans) ──

// calSpans computes colspan and rowspan for spanning cells in the grid.
// Returns spanInfo (row,col → colspan,rowspan) and covered (cells hidden by spans).
// Matches Python's __cal_spans (table_structure_recognizer.py:535).
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
	for i, row := range rows {
		for j, cell := range row {
			if j >= nCols || covered[[2]int{i, j}] {
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

// ── Orphan column/row cleanup (Python: construct_table lines 256-368) ──

// cleanupOrphanColumns removes columns that have only a single non-empty cell
// when there are ≥4 rows.  Matches Python's construct_table column cleanup.
func CleanupOrphanColumns(rows [][]pdf.TSRCell) [][]pdf.TSRCell {
	if len(rows) < 4 || len(rows) == 0 {
		return rows
	}
	nCols := len(rows[0])

	j := 0
colLoop:
	for j < nCols {
		e, ii := 0, 0
		for i := range rows {
			if j < len(rows[i]) && strings.TrimSpace(rows[i][j].Text) != "" {
				e++
				ii = i
			}
			if e > 1 {
				j++
				continue colLoop
			}
		}
		// Column j has only one non-empty cell at row ii.
		// Check if adjacent columns have text for this row.
		f := (j > 0 && j-1 < len(rows[ii]) && strings.TrimSpace(rows[ii][j-1].Text) != "") || j == 0
		ff := (j+1 < len(rows[ii]) && strings.TrimSpace(rows[ii][j+1].Text) != "") || j+1 >= len(rows[ii])
		if f && ff {
			// Both adjacent columns are ok for merging — but this means
			// there's text on both sides, keep column.
			j++
			continue
		}

		// Determine which side to merge into.
		left := 1e9
		right := 1e9
		if j > 0 && !f {
			for i := range rows {
				if j-1 < len(rows[i]) && strings.TrimSpace(rows[i][j-1].Text) != "" {
					// Distance from orphan cell to left neighbor.
					if d := rows[ii][j].X0 - rows[i][j-1].X1; d < left {
						left = d
					}
				}
			}
		}
		if j+1 < nCols && !ff {
			for i := range rows {
				if j+1 < len(rows[i]) && strings.TrimSpace(rows[i][j+1].Text) != "" {
					if d := rows[i][j+1].X0 - rows[ii][j].X1; d < right {
						right = d
					}
				}
			}
		}

		if left < right && j > 0 {
			// Merge into left column.
			for i := range rows {
				if j-1 < len(rows[i]) && j < len(rows[i]) {
					if rows[i][j-1].Text == "" {
						rows[i][j-1].Text = rows[i][j].Text
					} else if rows[i][j].Text != "" {
						rows[i][j-1].Text += " " + rows[i][j].Text
					}
				}
			}
		} else if j+1 < nCols {
			// Merge into right column.
			for i := range rows {
				if j < len(rows[i]) && j+1 < len(rows[i]) {
					if rows[i][j+1].Text == "" {
						rows[i][j+1].Text = rows[i][j].Text
					} else if rows[i][j].Text != "" {
						rows[i][j+1].Text = rows[i][j].Text + " " + rows[i][j+1].Text
					}
				}
			}
		}
		// Remove column j.
		for i := range rows {
			if j < len(rows[i]) {
				rows[i] = append(rows[i][:j], rows[i][j+1:]...)
			}
		}
		nCols--
		// Don't increment j — the next column shifted into position j.
	}
	return rows
}
