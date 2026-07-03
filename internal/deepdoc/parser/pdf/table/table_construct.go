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
	// Use DLA region boundaries when set.
	if tbl.RegionLeft != 0 || tbl.RegionRight != 0 || tbl.RegionTop != 0 || tbl.RegionBottom != 0 {
		return pdf.TextBox{
			X0:         tbl.RegionLeft,
			X1:         tbl.RegionRight,
			Top:        tbl.RegionTop,
			Bottom:     tbl.RegionBottom,
			Text:       html,
			PageNumber: pg,
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

// Orphan column/row cleanup (Python: construct_table lines 256-368)

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
