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
	"encoding/base64"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// tableIllegalCharsRe replaces illegal control characters (everything except
// TAB/LF/CR) with a single space. The pattern matches all C0 control chars
// except TAB (0x09), LF (0x0A) and CR (0x0D).
var tableIllegalCharsRe = regexp.MustCompile(`[\x00-\x08]|\x0B|\x0C|[\x0E-\x1F]`)

// numericCellRe recognises number-like cell text (integers, decimals, comma
// thousands separators, optional leading sign/$ and trailing %). It is a
// deliberately loose heuristic used only for header-vs-data contrast, not for
// value parsing.
var numericCellRe = regexp.MustCompile(`^[\$\+\-]?[\d,]+(\.\d+)?%?$`)

const defaultTableChunkRows = 256

// extractXLSXImages returns the floating and in-cell images anchored to a
// worksheet as structured parser items. Excelize exposes both kinds through
// GetPictureCells/GetPictures; walking the reported anchor cells avoids
// scanning the worksheet's entire coordinate space.
func extractXLSXImages(f *excelize.File, sheet string) ([]map[string]any, []string) {
	cells, err := f.GetPictureCells(sheet)
	if err != nil {
		return nil, []string{fmt.Sprintf("XLSX image discovery failed for sheet %q: %v", sheet, err)}
	}
	if len(cells) == 0 {
		return nil, nil
	}

	items := make([]map[string]any, 0, len(cells))
	warnings := make([]string, 0)
	for _, cell := range cells {
		pictures, err := f.GetPictures(sheet, cell)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("XLSX image extraction failed for sheet %q cell %s: %v", sheet, cell, err))
			continue
		}
		for _, picture := range pictures {
			if len(picture.File) == 0 {
				continue
			}
			mimeType, ok := xlsxImageMIMEType(picture.Extension)
			if !ok {
				warnings = append(warnings, fmt.Sprintf("XLSX image skipped for sheet %q cell %s: unsupported extension %q", sheet, cell, picture.Extension))
				continue
			}
			encoded := base64.StdEncoding.EncodeToString(picture.File)
			alt := cell
			if picture.Format != nil && picture.Format.AltText != "" {
				alt = picture.Format.AltText
			}
			items = append(items, map[string]any{
				"text":         alt,
				"doc_type_kwd": "image",
				"ck_type":      "image",
				"image":        "data:" + mimeType + ";base64," + encoded,
				"sheet":        sheet,
				"cell":         cell,
			})
		}
	}
	return items, warnings
}

func xlsxImageMIMEType(extension string) (string, bool) {
	switch strings.TrimPrefix(strings.ToLower(extension), ".") {
	case "jpg", "jpeg":
		return "image/jpeg", true
	case "gif":
		return "image/gif", true
	case "bmp":
		return "image/bmp", true
	case "tif", "tiff":
		return "image/tiff", true
	case "svg":
		return "image/svg+xml", true
	case "png":
		return "image/png", true
	case "emf":
		return "image/x-emf", true
	case "emz":
		return "image/x-emz", true
	case "ico":
		return "image/x-icon", true
	case "wmf":
		return "image/x-wmf", true
	case "wmz":
		return "image/x-wmz", true
	default:
		return "", false
	}
}

// maxMergeExtentCols caps how far a merged range may widen the header row.
// excelize's GetRows truncates each row at its last valued cell, so a merged
// slave beyond that point is absent and cannot inherit its master's text; we
// pad the header row to the furthest merged column to recover it. A pathological
// far merge (e.g. A1:XFD1) must not be allowed to allocate one cell per column
// for every row, so the width is capped here.
const maxMergeExtentCols = 1024

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

// htmlTableChunk is one self-contained <table> chunk with 1-based sheet/row/col
// coordinates for source location (mirrors Excel position_int semantics).
type htmlTableChunk struct {
	HTML     string
	RowStart int // inclusive, 1-based Excel row of first data row (header-only chunks use headerRowAbs)
	RowEnd   int // inclusive, 1-based Excel row of last data row
	ColStart int
	ColEnd   int
}

// recordsToHTMLTableChunks renders records as one or more self-contained HTML
// <table> chunks. The first row is always the header (<th>). Data rows are
// split into chunks of chunkRows, each chunk being a complete <table> with
// <caption> and a repeated header row. Chunks are joined with newlines.
//
// The tag schema is <table><caption>{caption}</caption><tr><th>…</th></tr>
// <tr><td>…</td></tr>…</table>. Rows are intentionally NOT wrapped in
// <thead>/<tbody>, so every <table> is one atomic chunk that downstream
// chunkers can consume independently.
func recordsToHTMLTableChunks(records [][]string, chunkRows int, caption string) string {
	chunks := recordsToHTMLTableChunkList(records, chunkRows, caption, 1)
	parts := make([]string, len(chunks))
	for i, ch := range chunks {
		parts[i] = ch.HTML
	}
	return strings.Join(parts, "")
}

// recordsToHTMLTableChunkList is the structured form of recordsToHTMLTableChunks.
// headerRowAbs is the 1-based workbook row number of records[0] (normally 1).
func recordsToHTMLTableChunkList(records [][]string, chunkRows int, caption string, headerRowAbs int) []htmlTableChunk {
	if headerRowAbs <= 0 {
		headerRowAbs = 1
	}
	colEnd := 1
	for _, row := range records {
		if n := len(row); n > colEnd {
			colEnd = n
		}
	}
	if len(records) == 0 {
		return []htmlTableChunk{{
			HTML:     "<table><caption>" + html.EscapeString(caption) + "</caption></table>",
			RowStart: headerRowAbs,
			RowEnd:   headerRowAbs,
			ColStart: 1,
			ColEnd:   colEnd,
		}}
	}

	headerHTML := buildHeaderRow(records[0])
	dataRows := records[1:]
	nData := len(dataRows)

	if nData == 0 {
		return []htmlTableChunk{{
			HTML:     "<table><caption>" + html.EscapeString(caption) + "</caption>\n" + headerHTML + "</table>",
			RowStart: headerRowAbs,
			RowEnd:   headerRowAbs,
			ColStart: 1,
			ColEnd:   colEnd,
		}}
	}

	if chunkRows <= 0 {
		chunkRows = defaultTableChunkRows
	}

	nChunks := (nData + chunkRows - 1) / chunkRows
	out := make([]htmlTableChunk, 0, nChunks)
	for ci := 0; ci < nChunks; ci++ {
		start := ci * chunkRows
		end := start + chunkRows
		if end > nData {
			end = nData
		}

		var b strings.Builder
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

		out = append(out, htmlTableChunk{
			HTML:     b.String(),
			RowStart: headerRowAbs + start + 1,
			RowEnd:   headerRowAbs + end,
			ColStart: 1,
			ColEnd:   colEnd,
		})
	}
	return out
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

// mergeRange is an excelize merged-cell rectangle, 1-based inclusive.
type mergeRange struct {
	sr, sc, er, ec int
}

// mergeRanges returns the merged-cell rectangles of a sheet.
func mergeRanges(f *excelize.File, sheet string) ([]mergeRange, error) {
	var out []mergeRange
	cells, err := f.GetMergeCells(sheet, true)
	if err != nil {
		return nil, err
	}
	for _, mc := range cells {
		sr, sc := axisToRC(mc.GetStartAxis())
		er, ec := axisToRC(mc.GetEndAxis())
		if sr == 0 || sc == 0 || er == 0 || ec == 0 {
			continue
		}
		out = append(out, mergeRange{sr, sc, er, ec})
	}
	return out, nil
}

// mergeMaxCol returns the furthest merged column across all ranges, or 0 if
// there are none.
func mergeMaxCol(ranges []mergeRange) int {
	m := 0
	for _, r := range ranges {
		if r.ec > m {
			m = r.ec
		}
	}
	return m
}

// mergeMasterForRow materialises the slave→master map for a single row only.
// A large merged block (e.g. A1:Z100) would otherwise expand to every one of
// its cells; we only ever need the header row's slaves, so expanding per row
// keeps the map O(cols) instead of O(rows×cols).
func mergeMasterForRow(ranges []mergeRange, row int) map[[2]int][2]int {
	mm := map[[2]int][2]int{}
	for _, r := range ranges {
		if row < r.sr || row > r.er {
			continue
		}
		for c := r.sc; c <= r.ec; c++ {
			mm[[2]int{row, c}] = [2]int{r.sr, r.sc}
		}
	}
	return mm
}

// inheritMergedHeader fills empty cells in the header row with the value of
// their merge master (typically a wide horizontally-merged title cell). This
// keeps a wide merged title from rendering as a row of blank <th> cells.
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

// mergeExtentCol returns the furthest merged column across all ranges, capped
// at maxMergeExtentCols so a pathological far merge cannot exhaust parser
// memory when the header row is padded to inherit merged text.
func mergeExtentCol(ranges []mergeRange) int {
	m := mergeMaxCol(ranges)
	if m > maxMergeExtentCols {
		return maxMergeExtentCols
	}
	return m
}

// padRowToWidth grows a single row to at least maxCol, padding with empty
// strings. Only the header row is padded (see renderSheetTables): merged-master
// text is inherited into the header alone, so data rows must not be widened —
// widening them would emit a sea of empty <td> cells for every far merge in the
// sheet and is the memory blow-up flagged in review.
func padRowToWidth(row *[]string, maxCol int) {
	if maxCol <= len(*row) {
		return
	}
	padded := make([]string, maxCol)
	copy(padded, *row)
	*row = padded
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
// header of a sheet, defaulting to 1. It only diverges from row 1 when there is
// high confidence that row 1 is not the header:
//
//  1. ListObject first: if the sheet defines Excel tables (ListObjects) whose
//     top row is > 1, that row is the header. This is the cheapest, most
//     accurate signal and never fires for the common case (table starts at row 1).
//  2. Lightweight detection (no ListObject override): scan the top few rows for
//     an anchor — a row whose cells are mostly bold/filled, or a row holding
//     text labels over numeric data columns below (contrast ≥ 2 columns). A
//     candidate that looks like a data row (majority numeric) is skipped. The
//     override only applies when the anchor is not already row 1, so the common
//     header-on-row-1 sheet is left unchanged.
func detectHeaderRow(f *excelize.File, sheet string, records [][]string, tables []excelize.Table) int {
	n := len(records)
	if n == 0 {
		return 1
	}

	// 1) ListObject first.
	if len(tables) > 0 {
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
			below := records[k+1]
			// Accept the candidate as the header when the row directly below
			// is a genuine data row (it contains at least one text cell), or
			// when that row is purely numeric but the candidate is not a
			// subtotal label. The second clause lets a styled text header
			// sitting above numeric-only data win; a bold "Total"/"Summary"
			// subtotal looks identical locally, but its label matches a
			// subtotal keyword so it is still refused and row 1 is kept.
			if rowHasTextCell(below) || (isPurelyNumeric(below) && !isSubtotalRow(row)) {
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

// isPurelyNumeric reports whether every non-empty cell of a row reads as a
// number-like string. It is used to recognise a numeric continuation row below
// a candidate header.
func isPurelyNumeric(row []string) bool {
	nonEmpty := 0
	for _, v := range row {
		if strings.TrimSpace(v) == "" {
			continue
		}
		nonEmpty++
		if !isNumericCell(v) {
			return false
		}
	}
	return nonEmpty > 0
}

// subtotalWordRe matches labels that mark a totals/subtotals row. A candidate
// header sitting above a purely-numeric row is refused when any of its cells
// matches, so bold "Total"/"Summary" subtotals are not promoted to the header.
var subtotalWordRe = regexp.MustCompile(`^(total|totals|sum|summary|subtotal|subtotals|grand total|合计|总计|小计|汇总|总额)$`)

// isSubtotalRow reports whether any non-empty cell of a row is a subtotal label
// (case-insensitive, tolerating a trailing colon).
func isSubtotalRow(row []string) bool {
	for _, v := range row {
		v = strings.ToLower(strings.TrimSpace(v))
		v = strings.TrimRight(v, ":：")
		if v != "" && subtotalWordRe.MatchString(v) {
			return true
		}
	}
	return false
}

// decodeChunkRows reads the "chunk_rows" setup knob, returning the default when
// it is absent or non-positive.
func decodeChunkRows(setup map[string]any) int {
	if setup == nil {
		return defaultTableChunkRows
	}
	v, ok := setup["chunk_rows"]
	if !ok {
		return defaultTableChunkRows
	}
	switch n := v.(type) {
	case float64:
		rows := int(n)
		if rows <= 0 {
			return defaultTableChunkRows
		}
		return rows
	case int:
		if n <= 0 {
			return defaultTableChunkRows
		}
		return n
	case int64:
		rows := int(n)
		if rows <= 0 {
			return defaultTableChunkRows
		}
		return rows
	}
	return defaultTableChunkRows
}

// renderSheetTables renders a single workbook sheet into one or more
// self-contained <table> chunks using the shared spreadsheet-HTML contract:
// detect the header row, inherit merged-master text into the header, and split
// data into chunkRows-sized atomic tables each repeating the header.
func renderSheetTables(f *excelize.File, sheet string, chunkRows int) (string, []string, error) {
	chunks, warnings, err := renderSheetTableChunks(f, sheet, chunkRows)
	if err != nil {
		return "", warnings, err
	}
	parts := make([]string, len(chunks))
	for i, ch := range chunks {
		parts[i] = ch.HTML
	}
	return strings.Join(parts, ""), warnings, nil
}

func renderSheetTableChunks(f *excelize.File, sheet string, chunkRows int) ([]htmlTableChunk, []string, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, nil, fmt.Errorf("read XLSX sheet %q rows: %w", sheet, err)
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	rows = cleanIllegalControlChars(rows)

	var warnings []string
	ranges, err := mergeRanges(f, sheet)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("read XLSX sheet %q merged cells: %v", sheet, err))
	}
	tables, err := f.GetTables(sheet)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("read XLSX sheet %q table metadata: %v", sheet, err))
	}
	headerRow := detectHeaderRow(f, sheet, rows, tables)

	// Inherit merged-master text into the header row. excelize's GetRows
	// truncates each row at its last valued cell, so a merged slave beyond that
	// point is absent and cannot inherit its master's text. We therefore pad
	// ONLY the header row (the only row we inherit into) to the furthest merged
	// column — capped by mergeExtentCol so a pathological far merge cannot
	// exhaust memory. Padding runs after detection so a wide merge (e.g. A1:Z1
	// title) does not dilute the styled-majority signal of a narrow header.
	mm := mergeMasterForRow(ranges, headerRow)
	if len(mm) > 0 {
		padRowToWidth(&rows[headerRow-1], mergeExtentCol(ranges))
	}
	inheritMergedHeader(rows, headerRow, mm)

	// Reorder so the detected header row becomes records[0]; every other row is
	// data. For the common case (header on row 1) this is a no-op.
	records := make([][]string, 0, len(rows))
	records = append(records, rows[headerRow-1])
	absDataRows := make([]int, 0, len(rows)-1)
	for i, r := range rows {
		if i == headerRow-1 {
			continue
		}
		records = append(records, r)
		absDataRows = append(absDataRows, i+1)
	}

	chunks := recordsToHTMLTableChunkList(records, chunkRows, sheet, headerRow)
	if len(chunks) == 0 || len(absDataRows) == 0 {
		return chunks, warnings, nil
	}
	if chunkRows <= 0 {
		chunkRows = defaultTableChunkRows
	}
	for ci := range chunks {
		start := ci * chunkRows
		end := start + chunkRows
		if end > len(absDataRows) {
			end = len(absDataRows)
		}
		if start < end {
			chunks[ci].RowStart = absDataRows[start]
			chunks[ci].RowEnd = absDataRows[end-1]
		}
	}
	return chunks, warnings, nil
}
