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
	"encoding/xml"
	"fmt"
	"ragflow/internal/parser/tableutil"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

// tableIllegalCharsRe replaces illegal control characters (everything except
// TAB/LF/CR) with a single space. The pattern matches all C0 control chars
// except TAB (0x09), LF (0x0A) and CR (0x0D).
var tableIllegalCharsRe = regexp.MustCompile(`[\x00-\x08]|\x0B|\x0C|[\x0E-\x1F]`)

// spreadsheetSegmentRow is one emitted data row: its cells for the markup
// and its row-aligned position tuple.
type spreadsheetSegmentRow struct {
	grid      []entity.TableCell
	sourceRow int
	tuple     []float64
	rowNum    int
	colStart  int
}

// trimCells normalizes cell whitespace before storing plain text in TableData.
func trimCells(row []string) []string {
	out := make([]string, len(row))
	for i, c := range row {
		out[i] = strings.TrimSpace(c)
	}
	return out
}

// buildSheetItems segments an already classified table at image anchors. Every
// segment repeats only confirmed header rows and preserves source row positions.
func buildSheetItems(source spreadsheetTable, sheet string, sheetIndex int, images []map[string]any) []map[string]any {
	sortImagesByAnchor(images)
	td := source.data
	if td == nil || len(td.Grid) == 0 {
		return images
	}
	headerRows := td.HeaderRows
	rows := make([]spreadsheetSegmentRow, 0, len(td.Grid))
	var headerGrid [][]entity.TableCell
	var headerMatrix [][]float64
	for r, row := range td.Grid {
		cells := make([]string, len(row))
		for c, cell := range row {
			cells[c] = cell.Text
		}
		colStart, colEnd := nonEmptyColumnRange(cells)
		for c, cell := range row {
			if cell.Covered {
				if colStart == 0 || c+1 < colStart {
					colStart = c + 1
				}
				if c+1 > colEnd {
					colEnd = c + 1
				}
			}
		}
		if colStart == 0 && r >= headerRows {
			continue
		}
		if colStart == 0 {
			colStart, colEnd = 1, len(row)
		}
		rowNum := source.rowNumbers[r]
		tuple := []float64{float64(sheetIndex), float64(rowNum), float64(rowNum), float64(colStart), float64(colEnd)}
		if r < headerRows {
			headerGrid = append(headerGrid, row)
			headerMatrix = append(headerMatrix, tuple)
		} else {
			rows = append(rows, spreadsheetSegmentRow{sourceRow: r, grid: row, tuple: tuple, rowNum: rowNum, colStart: colStart})
		}
	}
	spanThrough := make([]int, len(td.Grid))
	last := -1
	for r, row := range td.Grid {
		for _, cell := range row {
			if !cell.Covered && cell.RowSpan > 1 && r+cell.RowSpan-1 > last {
				last = r + cell.RowSpan - 1
			}
		}
		spanThrough[r] = last
	}

	anchored := make([]struct {
		cut int
		img map[string]any
	}, 0, len(images))
	for _, img := range images {
		row, _ := numericItemInt(img["row_start"])
		col, _ := numericItemInt(img["col_start"])
		cut := 0
		for cut < len(rows) {
			r := rows[cut]
			if r.rowNum > row || (r.rowNum == row && r.colStart > col) {
				break
			}
			cut++
		}
		// Move image boundaries after the complete rowspan group. Covered
		// rows retain their own positions even when they contain no new text.
		for cut > 0 && cut < len(rows) {
			end := spanThrough[rows[cut-1].sourceRow]
			if end < rows[cut].sourceRow {
				break
			}
			for cut < len(rows) && rows[cut].sourceRow <= end {
				cut++
			}
		}
		anchored = append(anchored, struct {
			cut int
			img map[string]any
		}{cut, img})
	}
	sort.SliceStable(anchored, func(i, j int) bool {
		if anchored[i].cut != anchored[j].cut {
			return anchored[i].cut < anchored[j].cut
		}
		ri, _ := numericItemInt(anchored[i].img["row_start"])
		rj, _ := numericItemInt(anchored[j].img["row_start"])
		if ri != rj {
			return ri < rj
		}
		ci, _ := numericItemInt(anchored[i].img["col_start"])
		cj, _ := numericItemInt(anchored[j].img["col_start"])
		return ci < cj
	})

	items := make([]map[string]any, 0, len(rows)+len(images)+len(source.leading))
	for _, item := range source.leading {
		item["sheet"], item["sheet_index"] = sheet, sheetIndex
		matrix := item["positions"].([][]float64)
		matrix[0][0] = float64(sheetIndex)
		items = append(items, item)
	}
	newSegment := func(body []spreadsheetSegmentRow) map[string]any {
		matrix := append([][]float64{}, headerMatrix...)
		grid := append([][]entity.TableCell{}, headerGrid...)
		sourceRows := make([]int, headerRows, headerRows+len(body))
		for r := range sourceRows {
			sourceRows[r] = r
		}
		for _, r := range body {
			grid = append(grid, r.grid)
			matrix = append(matrix, r.tuple)
			sourceRows = append(sourceRows, r.sourceRow)
		}
		// Rebase covered-slot pointers and constrain repeated header spans to
		// rows actually present in this segment. Never mutate the source grid.
		rowIndex := make(map[int]int, len(sourceRows))
		for r, source := range sourceRows {
			rowIndex[source] = r
		}
		for r, row := range grid {
			grid[r] = append([]entity.TableCell{}, row...)
			for c := range grid[r] {
				cell := &grid[r][c]
				if cell.Covered {
					cell.OriginRow = rowIndex[cell.OriginRow]
				} else if cell.RowSpan > 1 {
					span := 1
					for next := r + 1; next < len(grid) && sourceRows[next] < sourceRows[r]+cell.RowSpan; next++ {
						span++
					}
					cell.RowSpan = span
				}
			}
		}
		part := &entity.TableData{Grid: grid, HeaderRows: headerRows, Caption: sheet}
		item := NewTableDataJSONItem(part, sheet, matrix)
		item["sheet_index"] = sheetIndex
		return item
	}

	prev := 0
	headerLeadUsed := false
	for i := 0; i < len(anchored); {
		cut := anchored[i].cut
		if cut == 0 && !headerLeadUsed && headerRows > 0 {
			// Keep a confirmed header before an image anchored ahead of data.
			items = append(items, newSegment(nil))
			headerLeadUsed = true
		} else if cut > prev {
			items = append(items, newSegment(rows[prev:cut]))
			prev = cut
		}
		for i < len(anchored) && anchored[i].cut == cut {
			items = append(items, anchored[i].img)
			i++
		}
	}
	if prev < len(rows) || !headerLeadUsed && headerRows > 0 && len(rows) == 0 && len(anchored) == 0 {
		// A header-only sheet (no data rows, no images) still emits its
		// header as the only searchable representation of the column schema.
		items = append(items, newSegment(rows[prev:]))
	}
	return items
}

func sortImagesByAnchor(images []map[string]any) {
	sort.SliceStable(images, func(i, j int) bool {
		ri, _ := numericItemInt(images[i]["row_start"])
		rj, _ := numericItemInt(images[j]["row_start"])
		if ri != rj {
			return ri < rj
		}
		ci, _ := numericItemInt(images[i]["col_start"])
		cj, _ := numericItemInt(images[j]["col_start"])
		return ci < cj
	})
}

func nonEmptyColumnRange(row []string) (int, int) {
	start, end := 0, 0
	for i, cell := range row {
		if strings.TrimSpace(cell) == "" {
			continue
		}
		col := i + 1
		if start == 0 {
			start = col
		}
		end = col
	}
	return start, end
}

func numericItemInt(value any) (int, bool) {
	switch n := value.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(n))
		return parsed, err == nil
	default:
		return 0, false
	}
}

// deprecatedChunkRows reports the removed parser-side chunking option. Row
// segmentation is now owned by the parser's semantic segments and the
// selected chunker owns token-budget splitting; silently accepting
// chunk_rows would make an existing configuration look effective when it is
// not.
func deprecatedChunkRows(setup map[string]any, parserName string) {
	raw, exists := setup["chunk_rows"]
	if !exists {
		return
	}
	rows, ok := numericItemInt(raw)
	if !ok {
		common.Warn("spreadsheet parser ignored invalid chunk_rows; configure row merging on the chunker",
			zap.String("parser", parserName), zap.Any("chunk_rows", raw))
		return
	}
	common.Warn("spreadsheet parser ignored deprecated chunk_rows; configure row merging on the chunker",
		zap.String("parser", parserName), zap.Int("chunk_rows", rows))
}

// deprecatedHTML4Excel reports the retired html4excel option. Both
// spreadsheet builders collapsed into the segmented-HTML wire, so the flag
// selects nothing anymore. Only an enabled flag warns: false is the default
// every parser config carries, and warning there would be noise.
func deprecatedHTML4Excel(setup map[string]any, parserName string) {
	if enabled, ok := setup["html4excel"].(bool); ok && enabled {
		common.Warn("spreadsheet parser ignored deprecated html4excel; spreadsheet tables are always emitted as segmented HTML",
			zap.String("parser", parserName))
	}
}

// extractXLSXImages returns the floating and in-cell images anchored to a
// worksheet as structured parser items. Excelize exposes both kinds through
// GetPictureCells/GetPictures; walking the reported anchor cells avoids
// scanning the worksheet's entire coordinate space.
func extractXLSXImages(f *excelize.File, sheet string, budget *embeddedMediaBudget) ([]map[string]any, []string) {
	cells, err := f.GetPictureCells(sheet)
	if err != nil {
		return nil, []string{fmt.Sprintf("XLSX image discovery failed for sheet %q: %v", sheet, err)}
	}
	if len(cells) == 0 {
		return nil, nil
	}

	items := make([]map[string]any, 0, len(cells))
	warnings := make([]string, 0)
cellLoop:
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
			included, keepWalking := true, true
			if budget != nil {
				included, keepWalking = budget.include(picture.File)
			}
			if !keepWalking {
				break cellLoop
			}
			alt := cell
			if picture.Format != nil && picture.Format.AltText != "" {
				alt = picture.Format.AltText
			}
			row, col := axisToRC(cell)
			item := map[string]any{
				"text":         alt,
				"doc_type_kwd": "image",
				"ck_type":      "image",
				"sheet":        sheet,
				"cell":         cell,
				"row_start":    row,
				"row_end":      row,
				"col_start":    col,
				"col_end":      col,
			}
			if included {
				encoded := base64.StdEncoding.EncodeToString(picture.File)
				item["image"] = "data:" + mimeType + ";base64," + encoded
			} else {
				item["image"] = nil
				item["media_omitted"] = true
			}
			items = append(items, item)
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

// ──────────────────────────────────────────────────────────── merge geometry

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

// ──────────────────────────────────────────────────────────── header detection

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
	if len(st.Fill.Color) > 0 {
		return true
	}
	return false
}

// readSpreadsheetTable gathers source evidence before image segmentation.
func readSpreadsheetTable(f *excelize.File, sheet string) (spreadsheetTable, []string, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return spreadsheetTable{}, nil, fmt.Errorf("read XLSX sheet %q rows: %w", sheet, err)
	}
	rows = cleanIllegalControlChars(rows)
	grid := make([][]entity.TableCell, len(rows))
	rowNumbers := make([]int, len(rows))
	for r, row := range rows {
		rowNumbers[r] = r + 1
		for _, text := range trimCells(row) {
			grid[r] = append(grid[r], entity.TableCell{Text: text})
		}
	}
	var warnings []string
	ranges, err := mergeRanges(f, sheet)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("read XLSX sheet %q merged cells: %v", sheet, err))
	}
	// Bound malformed far merges and preserve their geometry instead of copying
	// a title's text into every covered column.
	mergeCellBudget := 1 << 20
	for _, m := range ranges {
		if m.sr > len(grid) || m.sc > maxMergeExtentCols {
			continue
		}
		endRow, endCol := m.er, m.ec
		if endRow > len(grid) {
			endRow = len(grid)
		}
		if endCol > maxMergeExtentCols {
			endCol = maxMergeExtentCols
		}
		added := 0
		for r := m.sr - 1; r < endRow; r++ {
			if endCol > len(grid[r]) {
				added += endCol - len(grid[r])
			}
			if added > mergeCellBudget {
				break
			}
		}
		if added > mergeCellBudget {
			warnings = append(warnings, fmt.Sprintf("XLSX sheet %q merge %s:%s exceeds the cell expansion budget; kept source values without merge geometry", sheet, cellAxis(m.sr, m.sc), cellAxis(m.er, m.ec)))
			continue
		}
		mergeCellBudget -= added
		for r := m.sr - 1; r < endRow; r++ {
			for len(grid[r]) < endCol {
				grid[r] = append(grid[r], entity.TableCell{})
			}
			for col := m.sc - 1; col < endCol; col++ {
				if r == m.sr-1 && col == m.sc-1 {
					grid[r][col].RowSpan = endRow - m.sr + 1
					grid[r][col].ColSpan = endCol - m.sc + 1
				} else {
					grid[r][col] = entity.TableCell{Covered: true, OriginRow: m.sr - 1, OriginCol: m.sc - 1}
				}
			}
		}
	}
	evidence := tableutil.HeaderEvidence{}
	// Only the bounded detection window needs styles, not the entire worksheet.
	for r := 0; r < len(grid) && r < 24; r++ {
		style := make([]bool, len(grid[r]))
		for col := 0; col < len(style) && col < 64; col++ {
			style[col] = cellIsStyled(f, sheet, r+1, col+1)
		}
		evidence.Styled = append(evidence.Styled, style)
	}
	tables, err := f.GetTables(sheet)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("read XLSX sheet %q table metadata: %v", sheet, err))
	}
	if len(tables) == 1 {
		// Excelize GetTables does not expose headerRowCount. Read that attribute
		// from the matching table part rather than treating hidden headers as present.
		f.Pkg.Range(func(key, value any) bool {
			name, ok := key.(string)
			if !ok || !strings.HasPrefix(name, "xl/tables/") {
				return true
			}
			data, ok := value.([]byte)
			if !ok {
				return true
			}
			var table struct {
				Name       string `xml:"name,attr"`
				Ref        string `xml:"ref,attr"`
				HeaderRows *int   `xml:"headerRowCount,attr"`
			}
			if xml.Unmarshal(data, &table) != nil || table.Name != tables[0].Name || table.Ref != tables[0].Range {
				return true
			}
			count := 1
			if table.HeaderRows != nil {
				count = *table.HeaderRows
			}
			start := rangeTopRow(table.Ref) - 1
			if count == 0 {
				start = 0
			}
			evidence.Explicit = &tableutil.Header{Start: start, Rows: count}
			return false
		})
	}
	return resolveSpreadsheetTable(grid, rowNumbers, evidence), warnings, nil
}
