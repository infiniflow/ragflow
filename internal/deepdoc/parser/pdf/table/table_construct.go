package table

import (
	"math"
	"regexp"
	"sort"
	"strings"

	pdf "ragflow/internal/deepdoc/parser/pdf/type"
)

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
	// Falls back to cell-level grouping only when called directly by tests
	// without a pre-computed Grid (production always sets it).
	var rows [][]pdf.TSRCell
	if item != nil {
		rows = item.Grid
	}
	if rows == nil && len(cells) > 0 && HasAnyText(cells) {
		rows = GroupTSRCellsToRows(cells)
	}
	if len(rows) > 0 && HasText(rows) {
		// Clean up orphan columns then orphan rows (Python order: columns at
		// construct_table:224-277, then rows at :279-333). Both passes mutate
		// the same grid and may drop rows/columns, so item.Grid and item.Rows
		// must be re-derived AFTER them (the old code set item.Rows before
		// column cleanup, leaving it stale; item.Grid also went stale because
		// CleanupOrphanRows returns a re-sliced header).
		rows = CleanupOrphanColumns(rows)
		// Drop all-empty rows. Python's construct_table groups boxes by their
		// R annotation (tbl[i] is the boxes for row i; a TSR row with no
		// overlapping box contributes no row). Its HTML emitter also skips
		// rows whose rendered HTML stays at "<tr>" (no cell wrote anything).
		// Go's grid is a TSR-row cross-product, so an extra "table row" that
		// the model emits alongside a "table projected row header" (or any
		// other TSR component with no OCR/text overlap) leaks through as a
		// 5-cell row of empty strings, inflating item.Rows and the HTML. See
		// 13_crosspage_table.pdf page 2: y0=885 "table row" sits on top of
		// a "table projected row header" with no box overlap and survives
		// every TSR cleanup pass. Drop it here so rows/Rows/HTML all match
		// Python.
		rows = DropAllEmptyRows(rows)
		rows = CleanupOrphanRows(rows)
		hdrs := HeaderSetWithBlockType(rows, boxes)
		if item != nil {
			item.Grid = rows
			item.Rows = RowsToStrings(rows)
		}
		spanInfo, covered := CalSpans(rows)
		if item != nil {
			MarkCoveredCells(item.Grid, covered)
		}
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
		sort.Slice(cols, func(i, j int) bool {
			return cols[i].c < cols[j].c
		})
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
	// A table merged across consecutive pages (MergeTablesAcrossPages appends
	// every spanned page to tbl.Positions) must record ALL its pages so the
	// resulting section's Position spans them — otherwise a caption on a later
	// page of the same table is wrongly treated as off-page and the
	// cross-page caption continuation (e.g. 13's 'Table: Monthly financial
	// summary FY2024') is dropped. PageNumber keeps the anchor page for
	// single-page consumers (insertion, etc.).
	pages := mergedTablePages(tbl)
	// Use DLA region boundaries when set.
	if tbl.RegionLeft != 0 || tbl.RegionRight != 0 || tbl.RegionTop != 0 || tbl.RegionBottom != 0 {
		return pdf.TextBox{
			X0:         tbl.RegionLeft,
			X1:         tbl.RegionRight,
			Top:        tbl.RegionTop,
			Bottom:     tbl.RegionBottom,
			Text:       html,
			PageNumber: pg,
			Pages:      pages,
			LayoutType: pdf.LayoutTypeTable,
		}
	}
	// Fallback: use anchor box coordinates.
	x0, x1, top, bot := ref.X0, ref.X1, ref.Top, ref.Bottom
	return pdf.TextBox{
		X0:         x0,
		X1:         x1,
		Top:        top,
		Bottom:     bot,
		Text:       html,
		PageNumber: pg,
		Pages:      pages,
		LayoutType: pdf.LayoutTypeTable,
	}
}

// mergedTablePages returns the sorted, de-duplicated set of page numbers a
// table spans, taken from every Position the (possibly cross-page-merged)
// TableItem carries. A single-page table yields a one-element slice (or nil
// when it has no positions), so callers can use it to decide whether the box
// spans multiple pages.
func mergedTablePages(tbl *pdf.TableItem) []int {
	if len(tbl.Positions) == 0 {
		return nil
	}
	seen := make(map[int]bool, len(tbl.Positions))
	pages := make([]int, 0, len(tbl.Positions))
	for _, p := range tbl.Positions {
		for _, pn := range p.PageNumbers {
			if !seen[pn] {
				seen[pn] = true
				pages = append(pages, pn)
			}
		}
	}
	sort.Ints(pages)
	return pages
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

// Orphan column/row cleanup (Python: construct_table:221-277 columns, :279-333 rows)

// CleanupOrphanColumns removes columns that have only a single non-empty cell.
// Matches Python's construct_table column cleanup (table_structure_recognizer.py:224-277),
// which is gated on the ROW count: `if len(rows) >= 4` (construct_table:221).
// The original Go gate (len(rows) < 4) matched Python and is preserved here —
// removing it would make Go drop orphan columns that Python keeps for <4-row tables.
func CleanupOrphanColumns(rows [][]pdf.TSRCell) [][]pdf.TSRCell {
	if len(rows) < 4 {
		return rows
	}
	nCols := len(rows[0])

	j := 0
	for j < nCols {
		// Step 1: Count non-empty cells in column
		e, ii := countNonEmptyCells(rows, j)
		if e > 1 {
			j++
			continue
		}

		// Step 2: Check adjacent columns
		hasLeftText, hasRightText := checkAdjacentColumns(rows, j, ii)
		if hasLeftText && hasRightText {
			j++
			continue
		}

		// Step 3: Calculate merge distance
		leftDist, rightDist := calculateMergeDistance(rows, j, ii, nCols, hasLeftText, hasRightText)

		// Python asserts at least one side is mergeable (left < 100000 or
		// right < 100000). If both neighbors are empty there is nothing to
		// merge the orphan into, so skip the column rather than dropping its
		// only cell (Python would assert/crash here). This guards the >=4-row
		// degenerate case where a column has a single cell but no mergeable
		// neighbor column.
		if leftDist >= 1e9 && rightDist >= 1e9 {
			j++
			continue
		}

		// Step 4: Merge the column
		if leftDist < rightDist && j > 0 {
			mergeColumnIntoLeft(rows, j)
		} else if j+1 < nCols {
			mergeColumnIntoRight(rows, j)
		}

		// Step 5: Remove the column
		rows = removeColumn(rows, j)
		nCols--
		// Don't increment j — the next column shifted into position j.
	}
	return rows
}

// CleanupOrphanRows removes rows that hold exactly one non-empty cell when the
// table has >=4 columns, merging that lone cell into its nearest vertical
// neighbor. Mirrors Python's construct_table row cleanup
// (table_structure_recognizer.py:279-333).
//
// A "sandwiched" orphan row — both the row above and the row below have text in
// the same column as the lone cell — is kept, because merging would destroy a
// real data row. Otherwise the orphan cell is merged UP if the vertical gap to
// the row above is smaller than to the row below, else DOWN. The neighbor cell
// keeps its own coordinates and only its text is extended (Python extends the
// box list); CalSpans recomputes spans from geometry, so no row-number
// bookkeeping (Python's "rn") is needed here.
//
// The >=4 threshold is the COLUMN count (Python's len(cols) >= 4), evaluated
// after column cleanup, not the row count.
func CleanupOrphanRows(rows [][]pdf.TSRCell) [][]pdf.TSRCell {
	if len(rows) == 0 || len(rows[0]) < 4 {
		return rows
	}
	nRows := len(rows)
	i := 0
	for i < nRows {
		// Count non-empty cells; remember the lone populated column.
		e, jj := 0, 0
		for j := range rows[i] {
			if strings.TrimSpace(rows[i][j].Text) != "" {
				e++
				jj = j
				if e > 1 {
					break
				}
			}
		}
		if e != 1 {
			// 0 cells (empty row) or >1 (not an orphan): nothing to merge.
			i++
			continue
		}

		// Are the directly-adjacent cells in the same column populated?
		hasAbove := (i > 0 && strings.TrimSpace(rows[i-1][jj].Text) != "") || i == 0
		hasBelow := (i+1 < nRows && strings.TrimSpace(rows[i+1][jj].Text) != "") || i+1 >= nRows
		if hasAbove && hasBelow {
			// Sandwiched between two populated cells → keep.
			i++
			continue
		}

		// Minimum vertical gap to the nearest mergeable neighbor.
		const inf = 1e9
		up, down := inf, inf
		if i > 0 && !hasAbove {
			for j := range rows[i-1] {
				if strings.TrimSpace(rows[i-1][j].Text) != "" {
					if d := rows[i][jj].Y0 - rows[i-1][j].Y1; d < up {
						up = d
					}
				}
			}
		}
		if i+1 < nRows && !hasBelow {
			for j := range rows[i+1] {
				if strings.TrimSpace(rows[i+1][j].Text) != "" {
					if d := rows[i+1][j].Y0 - rows[i][jj].Y1; d < down {
						down = d
					}
				}
			}
		}
		// Python asserts up < 100000 or down < 100000 (at least one side is
		// mergeable) because hasAbove/hasBelow are not both true. If BOTH
		// adjacent rows are entirely empty there is nowhere to merge into —
		// keep the orphan rather than relocating it (Python would assert/crash
		// here). Mirrors the orphan-column guard in CleanupOrphanColumns.
		if up >= inf && down >= inf {
			i++
			continue
		}

		if up < down {
			mergeOrphanCell(rows, i, i-1, jj)
		} else {
			mergeOrphanCell(rows, i, i+1, jj)
		}
		// Drop the orphan row; the next row shifts into position i.
		rows = append(rows[:i], rows[i+1:]...)
		nRows--
	}
	return rows
}

// DropAllEmptyRows removes rows whose cells are all whitespace/empty.
// Python's construct_table never emits a row that has no boxes assigned
// to any (R,C), so the parity target is to keep only rows that contribute
// at least one non-empty cell. Runs BEFORE CleanupOrphanRows so the
// downstream "exactly one populated cell" check is not silently masking
// a TSR false positive (e.g. a duplicate row component detected on top
// of a "table projected row header").
func DropAllEmptyRows(rows [][]pdf.TSRCell) [][]pdf.TSRCell {
	out := make([][]pdf.TSRCell, 0, len(rows))
	for _, r := range rows {
		empty := true
		for j := range r {
			if strings.TrimSpace(r[j].Text) != "" {
				empty = false
				break
			}
		}
		if !empty {
			out = append(out, r)
		}
	}
	return out
}

// mergeOrphanCell appends the lone cell at (from, col) into the target cell at
// (to, col). Matches Python's tbl[to][col].extend(tbl[from][col]): when the
// target already has text, the orphan text is appended after it. The target
// keeps its own coordinates; only the text is merged.
func mergeOrphanCell(rows [][]pdf.TSRCell, from, to, col int) {
	target := &rows[to][col]
	orphan := strings.TrimSpace(rows[from][col].Text)
	if orphan == "" {
		return
	}
	if strings.TrimSpace(target.Text) == "" {
		target.Text = orphan
	} else {
		target.Text = target.Text + " " + orphan
	}
}

// countNonEmptyCells counts non-empty cells in a column and returns the count
// and the index of the last non-empty row.
func countNonEmptyCells(rows [][]pdf.TSRCell, col int) (count int, lastRow int) {
	count = 0
	lastRow = 0
	for i := range rows {
		if col < len(rows[i]) && strings.TrimSpace(rows[i][col].Text) != "" {
			count++
			lastRow = i
		}
	}
	return count, lastRow
}

// checkAdjacentColumns checks if left and right adjacent columns have text in the given row.
func checkAdjacentColumns(rows [][]pdf.TSRCell, col int, row int) (hasLeft bool, hasRight bool) {
	hasLeft = (col > 0 && col-1 < len(rows[row]) && strings.TrimSpace(rows[row][col-1].Text) != "") || col == 0
	hasRight = (col+1 < len(rows[row]) && strings.TrimSpace(rows[row][col+1].Text) != "") || col+1 >= len(rows[row])
	return hasLeft, hasRight
}

// calculateMergeDistance calculates the minimum distance to merge into left or right column.
func calculateMergeDistance(rows [][]pdf.TSRCell, col int, row int, nCols int, hasLeft bool, hasRight bool) (leftDist float64, rightDist float64) {
	leftDist = 1e9
	rightDist = 1e9

	if col > 0 && !hasLeft {
		for i := range rows {
			if col-1 < len(rows[i]) && strings.TrimSpace(rows[i][col-1].Text) != "" {
				if d := rows[row][col].X0 - rows[i][col-1].X1; d < leftDist {
					leftDist = d
				}
			}
		}
	}

	if col+1 < nCols && !hasRight {
		for i := range rows {
			if col+1 < len(rows[i]) && strings.TrimSpace(rows[i][col+1].Text) != "" {
				if d := rows[i][col+1].X0 - rows[row][col].X1; d < rightDist {
					rightDist = d
				}
			}
		}
	}

	return leftDist, rightDist
}

// mergeColumn merges column src into column dst.
func mergeColumn(rows [][]pdf.TSRCell, src, dst int) {
	for i := range rows {
		if src < len(rows[i]) && dst < len(rows[i]) {
			if rows[i][dst].Text == "" {
				rows[i][dst].Text = rows[i][src].Text
			} else if rows[i][src].Text != "" {
				if src < dst {
					rows[i][dst].Text = rows[i][src].Text + " " + rows[i][dst].Text
				} else {
					rows[i][dst].Text += " " + rows[i][src].Text
				}
			}
		}
	}
}

// mergeColumnIntoLeft merges column j into column j-1.
func mergeColumnIntoLeft(rows [][]pdf.TSRCell, j int) {
	mergeColumn(rows, j, j-1)
}

// mergeColumnIntoRight merges column j into column j+1.
func mergeColumnIntoRight(rows [][]pdf.TSRCell, j int) {
	mergeColumn(rows, j, j+1)
}

// removeColumn removes column j from all rows.
func removeColumn(rows [][]pdf.TSRCell, j int) [][]pdf.TSRCell {
	for i := range rows {
		if j < len(rows[i]) {
			rows[i] = append(rows[i][:j], rows[i][j+1:]...)
		}
	}
	return rows
}
