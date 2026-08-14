//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language of permissions and
// limitations under the License.
//

package parser

import (
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// tableIllegalCharsRe replaces illegal control characters (everything except
// TAB/LF/CR) with a single space, mirroring Python's ILLEGAL_CHARACTERS_RE in
// deepdoc/parser/excel_parser.py.
var tableIllegalCharsRe = regexp.MustCompile(`[\x00-\x08]|\x0B|\x0C|[\x0E-\x1F]`)

// numericCellRe recognises number-like cell text (integers, decimals, comma
// thousands separators, optional leading sign/$ and trailing %). It is a
// deliberately loose heuristic used only for header-vs-data contrast, not for
// value parsing.
var numericCellRe = regexp.MustCompile(`^[\$\+\-]?[\d,]+(\.\d+)?%?$`)

const defaultTableChunkRows = 256

// cleanIllegalControlChars replaces illegal control characters in all cells
// with a single space.
func cleanIllegalControlChars(records [][]string) [][]string {
	out := make([][]string, len(records))
	for i, row := range records {
		out[i] = make([]string, len(row))
		for j, cell := range row {
			out[i][j] = tableIllegalCharsRe.ReplaceAllString(cell, " ")
		}
	}
	return out
}

// buildHeaderRow renders a row as an HTML <th> header row.
func buildHeaderRow(row []string) string {
	var b strings.Builder
	b.WriteString("<tr>")
	for _, cell := range row {
		b.WriteString("<th>")
		b.WriteString(html.EscapeString(strings.TrimSpace(cell)))
		b.WriteString("</th>")
	}
	b.WriteString("</tr>\n")
	return b.String()
}

// recordsToHTMLTableChunks renders records as one or more self-contained HTML
// <table> chunks. The first row is always the header (<th>). Data rows are
// split into chunks of chunkRows, each chunk being a complete <table> with
// <caption> and a repeated header row. Chunks are joined with newlines.
//
// The tag schema deliberately mirrors Python's
// deepdoc/parser/excel_parser.py:RAGFlowExcelParser.html() (and the Go
// CSVParser): <table><caption>{caption}</caption><tr><th>…</th></tr>
// <tr><td>…</td></tr>…</table>. It does NOT wrap rows in <thead>/<tbody> so the
// produced markup is byte-compatible with the established spreadsheet-HTML
// contract that downstream chunkers already consume atomically per <table>.
func recordsToHTMLTableChunks(records [][]string, chunkRows int, caption string) string {
	if len(records) == 0 {
		return "<table><caption>" + html.EscapeString(caption) + "</caption></table>"
	}

	// Build the header row once — repeated in every chunk.
	headerHTML := buildHeaderRow(records[0])
	dataRows := records[1:]
	nData := len(dataRows)

	if nData == 0 {
		// Only a header row exists.
		return "<table><caption>" + html.EscapeString(caption) + "</caption>\n" + headerHTML + "</table>"
	}

	if chunkRows <= 0 {
		chunkRows = defaultTableChunkRows
	}

	nChunks := (nData + chunkRows - 1) / chunkRows
	var b strings.Builder
	for ci := 0; ci < nChunks; ci++ {
		start := ci * chunkRows
		end := start + chunkRows
		if end > nData {
			end = nData
		}

		b.WriteString("<table><caption>")
		b.WriteString(html.EscapeString(caption))
		b.WriteString("</caption>\n")
		b.WriteString(headerHTML)

		for _, row := range dataRows[start:end] {
			b.WriteString("<tr>")
			for _, cell := range row {
				b.WriteString("<td>")
				b.WriteString(html.EscapeString(strings.TrimSpace(cell)))
				b.WriteString("</td>")
			}
			b.WriteString("</tr>\n")
		}
		b.WriteString("</table>\n")
	}
	return b.String()
}

// ──────────────────────────────────────────────────────────── axis helpers

// axisToRC parses an "A1"-style cell reference (optionally with a leading "$")
// into 1-based (row, col). A malformed reference yields (0, 0).
func axisToRC(axis string) (row, col int) {
	axis = strings.TrimSpace(axis)
	axis = strings.TrimPrefix(axis, "$")
	i := 0
	for i < len(axis) && (axis[i] < '0' || axis[i] > '9') {
		i++
	}
	if i == 0 || i == len(axis) {
		return 0, 0
	}
	letter, num := axis[:i], axis[i:]
	r, err := strconv.Atoi(num)
	if err != nil {
		return 0, 0
	}
	c := 0
	for _, ch := range strings.ToUpper(letter) {
		if ch < 'A' || ch > 'Z' {
			return 0, 0
		}
		c = c*26 + int(ch-'A'+1)
	}
	return r, c
}

// rangeTopRow returns the top (first) row of an A1:B10-style range reference.
func rangeTopRow(ref string) int {
	ref = strings.SplitN(ref, ":", 2)[0]
	r, _ := axisToRC(ref)
	return r
}

// cellAxis builds an "A1"-style reference for 1-based (row, col).
func cellAxis(row, col int) string {
	// Build the column letters least-significant digit first, then reverse.
	var digits []byte
	c := col
	for c > 0 {
		c-- // 1-based → 0-based for this digit
		digits = append(digits, byte('A'+c%26))
		c /= 26
	}
	for i, j := 0, len(digits)-1; i < j; i, j = i+1, j-1 {
		digits[i], digits[j] = digits[j], digits[i]
	}
	return string(digits) + strconv.Itoa(row)
}

// ──────────────────────────────────────────────────────────── merge inheritance

// mergeMasterMap builds a map from every merged slave cell to its master cell
// for the given sheet, using excelize's GetMergeCells.
func mergeMasterMap(f *excelize.File, sheet string) map[[2]int][2]int {
	mm := map[[2]int][2]int{}
	cells, err := f.GetMergeCells(sheet)
	if err != nil {
		return mm
	}
	for _, mc := range cells {
		start, end := mc.GetStartAxis(), mc.GetEndAxis()
		sr, sc := axisToRC(start)
		er, ec := axisToRC(end)
		if sr == 0 || sc == 0 || er == 0 || ec == 0 {
			continue
		}
		for r := sr; r <= er; r++ {
			for c := sc; c <= ec; c++ {
				mm[[2]int{r, c}] = [2]int{sr, sc}
			}
		}
	}
	return mm
}

// inheritMergedHeader fills empty cells in the header row with the value of
// their merge master (typically a wide horizontally-merged title cell). This
// fixes the "merged header cell renders empty" defect that the Python deepdoc
// path suffers from, without changing the HTML tag schema.
func inheritMergedHeader(records [][]string, headerRowIdx int, mm map[[2]int][2]int) {
	if headerRowIdx < 1 || headerRowIdx > len(records) {
		return
	}
	row := records[headerRowIdx-1]
	for c := 0; c < len(row); c++ {
		if strings.TrimSpace(row[c]) != "" {
			continue
		}
		master, ok := mm[[2]int{headerRowIdx, c + 1}]
		if !ok || (master[0] == headerRowIdx && master[1] == c+1) {
			continue
		}
		if master[0] < 1 || master[0] > len(records) || master[1] < 1 || master[1] > len(records[master[0]-1]) {
			continue
		}
		val := records[master[0]-1][master[1]-1]
		if strings.TrimSpace(val) != "" {
			row[c] = val
		}
	}
}

// padRecordsToWidth grows every row to at least the widest of (the longest row,
// the furthest merged column). excelize's GetRows truncates a row at its last
// valued cell, so a merged slave beyond that point is absent from the slice and
// cannot inherit its master's text. Padding with empty strings before
// inheritMergedHeader restores those slots without affecting sheets that have
// no merges (where maxCol equals the longest data row).
func padRecordsToWidth(records [][]string, mm map[[2]int][2]int) {
	maxCol := 0
	for _, row := range records {
		if len(row) > maxCol {
			maxCol = len(row)
		}
	}
	for rc := range mm {
		if rc[1] > maxCol {
			maxCol = rc[1]
		}
	}
	if maxCol == 0 {
		return
	}
	for i, row := range records {
		if len(row) >= maxCol {
			continue
		}
		padded := make([]string, maxCol)
		copy(padded, row)
		records[i] = padded
	}
}

// ──────────────────────────────────────────────────────────── header detection

// isNumericCell reports whether a cell value reads as a number-like string.
func isNumericCell(s string) bool {
	s = strings.TrimSpace(s)
	return s != "" && numericCellRe.MatchString(s)
}

// cellIsStyled reports whether a cell is bold or carries a fill, using
// excelize's style lookup.
func cellIsStyled(f *excelize.File, sheet string, row, col int) bool {
	idx, err := f.GetCellStyle(sheet, cellAxis(row, col))
	if err != nil {
		return false
	}
	st, err := f.GetStyle(idx)
	if err != nil || st == nil {
		return false
	}
	if st.Font != nil && st.Font.Bold {
		return true
	}
	if st.Fill.Type != "" || len(st.Fill.Color) > 0 {
		return true
	}
	return false
}

// detectHeaderRow returns the 1-based row that should be treated as the column
// header of a sheet, defaulting to 1 (matching Python deepdoc's assumption
// that the first row is the header). It layers two "better-than-Python" signals
// on top of that default so it only diverges from row 1 when there is high
// confidence that row 1 is not the header:
//
//  1. ListObject first: if the sheet defines Excel tables (ListObjects) whose
//     top row is > 1, that row is the header. This is the cheapest, most
//     accurate signal and never fires for the common case (table starts at row 1).
//  2. Lightweight detection (no ListObject override): scan the top few rows for
//     an anchor — a row whose cells are mostly bold/filled, or a row holding
//     text labels over numeric data columns below (contrast ≥ 2 columns). A
//     candidate that looks like a data row (majority numeric) is skipped. The
//     override only applies when the anchor is not already row 1, so the common
//     header-on-row-1 sheet stays byte-identical to Python.
func detectHeaderRow(f *excelize.File, sheet string, records [][]string) int {
	n := len(records)
	if n == 0 {
		return 1
	}

	// 1) ListObject first.
	if tables, err := f.GetTables(sheet); err == nil && len(tables) > 0 {
		minTop := 0
		for _, t := range tables {
			tr := rangeTopRow(t.Range)
			if tr < 1 {
				continue
			}
			if minTop == 0 || tr < minTop {
				minTop = tr
			}
		}
		if minTop > 1 {
			return minTop
		}
	}

	// 2) Lightweight detection over the top window (bounded to 4 candidate rows,
	//    and we must leave at least one data row below the candidate).
	maxScan := 4
	if maxScan > n-1 {
		maxScan = n - 1
	}
	if maxScan < 1 {
		return 1
	}
	for k := 0; k < maxScan; k++ {
		row := records[k]
		nc := len(row)
		if nc < 2 {
			continue // title / section stub — keep looking below it
		}

		// Data-majority reject: a row that is mostly numbers is data, not a
		// header. Keep scanning downward for the real header.
		nonEmpty, numeric := 0, 0
		for _, v := range row {
			if v == "" {
				continue
			}
			nonEmpty++
			if isNumericCell(v) {
				numeric++
			}
		}
		if nonEmpty > 0 && numeric*2 >= nonEmpty {
			continue
		}

		// Styled signal.
		styled := 0
		for c := 0; c < nc && c < 64; c++ {
			if cellIsStyled(f, sheet, k+1, c+1) {
				styled++
			}
		}

		// Contrast signal: text label over a numeric column below.
		contrast := 0
		if k+1 < n {
			below := records[k+1]
			for c := 0; c < nc && c < 64; c++ {
				v := row[c]
				if v == "" || isNumericCell(v) {
					continue
				}
				if c < len(below) && isNumericCell(below[c]) {
					contrast++
					if contrast >= 2 {
						break
					}
				}
			}
		}

		if styled*2 >= nc || contrast >= 2 {
			if k == 0 {
				return 1 // row 1 is already the header
			}
			// Body-brake: a genuine header sits directly above a data row
			// that contains at least one text cell (a name/category/ID
			// column). A bold "Total"/"Summary" subtotal sitting over a
			// purely-numeric continuation looks identical locally, so we
			// refuse to override unless the row below proves it is a real
			// header row over data — keeping row 1 (Python's default).
			if rowHasTextCell(records[k+1]) {
				return k + 1
			}
		}
	}
	return 1
}

// rowHasTextCell reports whether a row contains at least one non-empty,
// non-numeric (i.e. text) cell. It is used as a body-signature brake for
// header detection.
func rowHasTextCell(row []string) bool {
	for _, v := range row {
		if v != "" && !isNumericCell(v) {
			return true
		}
	}
	return false
}
