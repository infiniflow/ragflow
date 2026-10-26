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
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// tableIllegalCharsRe replaces illegal control characters (everything except
// TAB/LF/CR) with a single space. The pattern matches all C0 control chars
// except TAB (0x09), LF (0x0A) and CR (0x0D).
var tableIllegalCharsRe = regexp.MustCompile(`[\x00-\x08]|\x0B|\x0C|[\x0E-\x1F]`)

// invisibleFormatCutset lists invisible Unicode format characters stripped
// from the EDGES of cell text only (see cleanIllegalControlChars): ZERO
// WIDTH SPACE / NON-JOINER / JOINER, WORD JOINER and the BOM. The same code
// points are meaningful INSIDE a word — U+200D joins emoji ZWJ sequences
// (e.g. woman technologist), U+200C is the Persian half-space — so they must
// survive mid-cell and only accidental wrappers come off. The escapes below
// are Go string literals naming each code point explicitly.
const invisibleFormatCutset = "\u200b\u200c\u200d\u2060\ufeff"

// numericCellRe recognises number-like cell text (integers, decimals, comma
// thousands separators, optional leading sign/$ and trailing %). It is a
// deliberately loose heuristic used only for header-vs-data contrast, not for
// value parsing.
var numericCellRe = regexp.MustCompile(`^[\$\+\-]?[\d,]+(\.\d+)?%?$`)

// spreadsheetSegmentRow is one emitted data row: its cells for the markup
// and its row-aligned position tuple.
type spreadsheetSegmentRow struct {
	cells    []string
	tuple    []float64
	rowNum   int
	colStart int
}

// trimCells returns a copy of row with every cell TrimSpace'd. The legacy
// spreadsheet producer HTML-escaped each TrimSpace'd cell; the structured
// TableData contract stores the already-TrimSpace'd text, which the chunker
// re-escapes when it renders chunk text (see tableutil.RenderTableHTML), so
// the rendered text matches the legacy HTML byte for byte.
func trimCells(row []string) []string {
	out := make([]string, len(row))
	for i, c := range row {
		out[i] = strings.TrimSpace(c)
	}
	return out
}

// buildSheetItems renders one sheet into its wire items: the structured
// TableData contract (captioned table with a header row and data rows) split
// at image anchor boundaries, interleaved with the anchored image items in
// document order. Every segment repeats the header row (so segment 2+ never
// loses column names) and carries a row-aligned position matrix — one tuple
// per row in strict order, header included — so row-level consumers index
// positions by row number with no side channel. Each segment's TableData keeps
// the sheet's merge geometry: an origin cell carries ColSpan/RowSpan and the
// positions it engulfs are Covered markers. Consumers project those slots with
// tableutil.ResolveRows; the parser no longer pre-expands merges. The chunker
// reads the rows directly and renders each emitted chunk's Text as an HTML
// <table> (see tableutil.RenderTableHTML); no HTML markup is emitted here at
// the parser.
//
// An image lands after the last row whose (row, colStart) sorts at or before
// its anchor; rows before the header's anchor keep the header in front of
// the image (a header-only lead segment); fully empty rows are skipped from
// both rows and matrix. A segment cut never splits a merge group: an anchor
// inside a merged block moves after the block, so every segment owns the
// origins of its Covered slots.
//
// Segmentation is semantic only — sheet boundaries and image anchors, never a
// size budget. The legacy html4excel path pre-cut a fixed 12 rows here, which
// ignored the user's chunk size; that budget belongs to the chunker (see the
// chunker's splitSpreadsheetTable).
//
// Every tuple is a spreadsheet position, [sheet, rowStart, rowEnd, colStart,
// colEnd]. The same `positions` field carries PDF layout boxes
// ([page, left, right, top, bottom]) on PDF items, so a consumer must key the
// vocabulary off sheet_index / doc_type_kwd and never reinterpret one as the other.
func buildSheetItems(records [][]string, sheet string, sheetIndex, headerRow int, dataRows []int, images []map[string]any, ranges []mergeRange) []map[string]any {
	sortImagesByAnchor(images)
	if len(records) == 0 {
		return images
	}
	if headerRow <= 0 {
		headerRow = 1
	}
	rowByNum, rowNums := sheetRowIndex(records, headerRow, dataRows)
	ranges = materializeDetachedMerges(rowByNum, headerRow, ranges)
	header := append([]string(nil), rowByNum[headerRow]...)
	headerColEnd := len(header)
	if headerColEnd == 0 {
		for _, row := range records {
			if len(row) > headerColEnd {
				headerColEnd = len(row)
			}
		}
	}
	if headerColEnd == 0 {
		headerColEnd = 1
	}
	headerTuple := []float64{float64(sheetIndex), float64(headerRow), float64(headerRow), 1, float64(headerColEnd)} // [sheet, rowStart, rowEnd, colStart, colEnd]

	rows := make([]spreadsheetSegmentRow, 0, len(records)-1)
	for i := range records[1:] {
		rowNum := rowNums[i]
		cells := append([]string(nil), rowByNum[rowNum]...)
		colStart, colEnd := nonEmptyColumnRange(cells)
		// A merge slave can be the row's only value in a column before the
		// surviving real cells. The position tuple still has to cover the
		// merged group's columns, exactly as the pre-geometry parser did when
		// it copied the master value into the slave cell. Do this before the
		// empty-row skip: a row with no cells of its own is still a real grid
		// row when a non-blank merge master covers it.
		for _, m := range ranges {
			if m.sr > rowNum || m.er < rowNum {
				continue
			}
			masterRow := rowByNum[m.sr]
			if m.sc < 1 || m.sc > len(masterRow) || strings.TrimSpace(masterRow[m.sc-1]) == "" {
				continue
			}
			if colStart == 0 || m.sc < colStart {
				colStart = m.sc
			}
			if m.ec > colEnd {
				colEnd = m.ec
			}
		}
		if colStart == 0 {
			continue
		}
		rows = append(rows, spreadsheetSegmentRow{
			cells:    cells,
			tuple:    []float64{float64(sheetIndex), float64(rowNum), float64(rowNum), float64(colStart), float64(colEnd)},
			rowNum:   rowNum,
			colStart: colStart,
		})
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
		anchored = append(anchored, struct {
			cut int
			img map[string]any
		}{cut, img})
	}
	// A segment must not cut through a merge group: the segment's canonical
	// grid needs the group's origin to resolve its covered slots. Move an
	// image anchor forward to the first row after the group instead.
	for i := range anchored {
		if anchored[i].cut == 0 {
			continue
		}
		anchored[i].cut = cutAfterMergeGroup(anchored[i].cut, rows, ranges)
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

	items := make([]map[string]any, 0, len(rows)+len(images))
	newSegment := func(body []spreadsheetSegmentRow) map[string]any {
		matrix := make([][]float64, 0, len(body)+1)
		matrix = append(matrix, headerTuple)
		for _, r := range body {
			matrix = append(matrix, r.tuple)
		}
		td := buildCanonicalTable(headerRow, header, body, ranges, sheet)
		ti := NewTableDataJSONItem(td, sheet, SpreadsheetPositions(matrix)).(TableItem)
		ti.Extra = map[string]any{"sheet_index": sheetIndex}
		return ti.ToMap()
	}

	prev := 0
	headerLeadUsed := false
	for i := 0; i < len(anchored); {
		origCut := anchored[i].cut
		cut := origCut
		if cut == 0 && !headerLeadUsed {
			// A header-origin vertical merge spanning into the data rows must
			// stay in the lead segment too; otherwise the next segment would
			// contain a covered slot whose origin is in the previous one.
			if end := headerMergeEnd(ranges, headerRow); end > headerRow {
				cut = cutAfterRow(rows, end)
			}
		}
		if cut == 0 && !headerLeadUsed {
			// A cut at 0 keeps the header in front of the image, matching the
			// old IR's always-first header record: emit a header-only lead
			// segment, the data rows follow in the next segment.
			items = append(items, newSegment(nil))
			headerLeadUsed = true
		} else if cut > prev {
			items = append(items, newSegment(rows[prev:cut]))
			prev = cut
		}
		for i < len(anchored) && anchored[i].cut == origCut {
			items = append(items, anchored[i].img)
			i++
		}
	}
	if prev < len(rows) || !headerLeadUsed && len(rows) == 0 && len(anchored) == 0 {
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

// sheetRowIndex maps absolute sheet row numbers to the reordered records and
// returns the data rows' numbers in record order. The header row is records[0].
func sheetRowIndex(records [][]string, headerRow int, dataRows []int) (map[int][]string, []int) {
	byNum := make(map[int][]string, len(records))
	nums := make([]int, len(records)-1)
	byNum[headerRow] = records[0]
	for i, row := range records[1:] {
		rowNum := headerRow + i + 1
		if len(dataRows) == len(records)-1 {
			rowNum = dataRows[i]
		}
		nums[i] = rowNum
		byNum[rowNum] = row
	}
	return byNum, nums
}

// materializeDetachedMerges handles a merge that starts above the detected
// header and crosses it. Reordering the header to records[0] puts such a
// merge's origin after the row it covers, which a grid cannot represent as a
// span. The legacy parser filled the text into the header (and data rows) with
// no span, so mirror that shape: write the master text into the rows the range
// covers and drop the range from the geometry. All other merges stay canonical.
func materializeDetachedMerges(rowByNum map[int][]string, headerRow int, ranges []mergeRange) []mergeRange {
	if len(ranges) == 0 {
		return ranges
	}
	kept := make([]mergeRange, 0, len(ranges))
	for _, m := range ranges {
		if m.sr >= headerRow || m.er < headerRow {
			kept = append(kept, m)
			continue
		}
		masterRow := rowByNum[m.sr]
		if m.sc < 1 || m.sc > len(masterRow) {
			continue
		}
		master := masterRow[m.sc-1]
		if strings.TrimSpace(master) == "" {
			continue
		}
		// readSpreadsheetRecords caps merge padding at maxMergeExtentCols so a
		// pathological far merge cannot allocate one cell per worksheet column.
		// A detached merge bypasses that padding, so apply the same cap here.
		endCol := m.ec
		if endCol > maxMergeExtentCols {
			endCol = maxMergeExtentCols
		}
		if m.sc > endCol {
			continue
		}
		for row := m.sr; row <= m.er; row++ {
			cells, ok := rowByNum[row]
			if !ok {
				continue
			}
			if row == headerRow {
				// The header row carried a merged title across the columns
				// the range covers; the legacy header-inheritance path filled
				// every one of them.
				if endCol > len(cells) {
					grown := make([]string, endCol)
					copy(grown, cells)
					cells = grown
					rowByNum[row] = cells
				}
				for c := m.sc; c <= endCol; c++ {
					if strings.TrimSpace(cells[c-1]) == "" {
						cells[c-1] = master
					}
				}
				continue
			}
			// Data rows only inherit a vertical merge's master. A horizontal
			// data merge keeps its value on the origin column, matching the
			// legacy data-inheritance rule.
			if m.sc == m.ec || row == m.sr {
				if m.sc > len(cells) {
					grown := make([]string, m.sc)
					copy(grown, cells)
					cells = grown
					rowByNum[row] = cells
				}
				if strings.TrimSpace(cells[m.sc-1]) == "" {
					cells[m.sc-1] = master
				}
			}
		}
	}
	return kept
}

// buildCanonicalTable assembles one segment's TableData from the segment's
// source rows and the sheet's merge rectangles. The grid keeps merge geometry
// instead of pre-expanding it: an origin cell carries ColSpan/RowSpan and the
// positions it engulfs are Covered markers. Consumers choose how to project
// those covered slots (see tableutil.ResolveRows), so the parser no longer has
// to decide whether a merged value is a header label or a data value.
func buildCanonicalTable(headerRow int, header []string, body []spreadsheetSegmentRow, ranges []mergeRange, sheet string) *entity.TableData {
	type sourceRow struct {
		num   int
		cells []string
	}
	sources := make([]sourceRow, 0, len(body)+1)
	sources = append(sources, sourceRow{num: headerRow, cells: header})
	for _, r := range body {
		sources = append(sources, sourceRow{num: r.rowNum, cells: r.cells})
	}

	present := make(map[int]bool, len(sources))
	for _, src := range sources {
		present[src.num] = true
	}
	localRanges := clampMergeRanges(ranges, present)

	logical := make([][]tableutil.CellInput, 0, len(sources))
	headerFlags := make([]bool, 0, len(sources))
	for i, src := range sources {
		cells := trimCells(src.cells)
		// A merge origin past the row's truncation point still has to be
		// emitted so the grid can place the span; GetRows truncates at the
		// last valued cell.
		width := len(cells)
		for _, m := range localRanges {
			if m.sr == src.num && m.sc > width {
				width = m.sc
			}
		}
		row := make([]tableutil.CellInput, 0, width)
		for c := 0; c < width; c++ {
			m, origin := mergeAt(localRanges, src.num, c+1)
			if m != nil && !origin {
				continue
			}
			text := ""
			if c < len(cells) {
				text = cells[c]
			}
			in := tableutil.CellInput{Text: text}
			if m != nil && origin {
				in.ColSpan = m.ec - m.sc + 1
				in.RowSpan = m.er - m.sr + 1
			}
			row = append(row, in)
		}
		logical = append(logical, row)
		headerFlags = append(headerFlags, i == 0)
	}

	td := tableutil.BuildTableData(logical, headerFlags, sheet)
	if td == nil || len(td.Grid) == 0 {
		return td
	}
	// Preserve the legacy spreadsheet row width: data rows were padded to the
	// header width so a short trailing column still renders under its <th>.
	// Only appending is done here, so every existing cell keeps its index.
	width := len(td.Grid[0])
	for r := 1; r < len(td.Grid); r++ {
		for len(td.Grid[r]) < width {
			td.Grid[r] = append(td.Grid[r], entity.TableCell{})
		}
	}
	return td
}

// clampMergeRanges keeps only the consecutive rows of each merge that are
// present in this segment. A header is repeated in every segment, so a header-
// origin merge whose covered rows live in an earlier segment must not reserve
// a column in a later one; clamping its RowSpan to 1 (or its ColSpan-only form)
// prevents the later segment from marking an unrelated body row Covered.
func clampMergeRanges(ranges []mergeRange, present map[int]bool) []mergeRange {
	if len(ranges) == 0 {
		return nil
	}
	local := make([]mergeRange, 0, len(ranges))
	for _, m := range ranges {
		if !present[m.sr] {
			continue
		}
		end := m.sr
		for row := m.sr; row <= m.er; row++ {
			if !present[row] {
				break
			}
			end = row
		}
		if end == m.sr && m.sc == m.ec {
			continue // one-row vertical span is not a merge
		}
		local = append(local, mergeRange{sr: m.sr, sc: m.sc, er: end, ec: m.ec})
	}
	return local
}

// mergeTouchesRow reports whether any merge rectangle covers row.
func mergeTouchesRow(ranges []mergeRange, row int) bool {
	for _, m := range ranges {
		if row >= m.sr && row <= m.er {
			return true
		}
	}
	return false
}

// mergeAt returns the merge rectangle covering (row, col), both 1-based, and
// whether that position is the rectangle's origin.
func mergeAt(ranges []mergeRange, row, col int) (*mergeRange, bool) {
	for i := range ranges {
		m := &ranges[i]
		if row >= m.sr && row <= m.er && col >= m.sc && col <= m.ec {
			return m, row == m.sr && col == m.sc
		}
	}
	return nil, false
}

// cutAfterMergeGroup moves a segment cut at index cut to the first row after
// any merge group it would split. The returned cut is always >= cut.
func cutAfterMergeGroup(cut int, rows []spreadsheetSegmentRow, ranges []mergeRange) int {
	if cut <= 0 || cut >= len(rows) {
		return cut
	}
	for cut < len(rows) {
		moved := false
		for _, m := range ranges {
			if m.sr <= rows[cut-1].rowNum && m.er >= rows[cut].rowNum {
				for cut < len(rows) && rows[cut].rowNum <= m.er {
					cut++
				}
				moved = true
				break
			}
		}
		if !moved {
			return cut
		}
	}
	return cut
}

// cutAfterRow returns the first row index whose absolute sheet row is greater
// than rowNum.
func cutAfterRow(rows []spreadsheetSegmentRow, rowNum int) int {
	cut := 0
	for cut < len(rows) && rows[cut].rowNum <= rowNum {
		cut++
	}
	return cut
}

// headerMergeEnd returns the furthest row covered by a merge mastered on the
// header row, or headerRow when the header has no vertical merge.
func headerMergeEnd(ranges []mergeRange, headerRow int) int {
	end := headerRow
	for _, m := range ranges {
		if m.sr == headerRow && m.er > end {
			end = m.er
		}
	}
	return end
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
// with a single space and trims invisible Unicode format characters from
// cell edges (see invisibleFormatCutset), which otherwise survive TrimSpace
// and pollute search text.
func cleanIllegalControlChars(records [][]string) [][]string {
	out := make([][]string, len(records))
	for i, row := range records {
		out[i] = make([]string, len(row))
		for j, cell := range row {
			cell = strings.Trim(cell, invisibleFormatCutset)
			out[i][j] = tableIllegalCharsRe.ReplaceAllString(cell, " ")
		}
	}
	return out
}

// trimTrailingEmptyColumns drops trailing columns that are blank in every
// row, header included. Only truncation happens here: remaining column
// indices are untouched, so row-aligned position tuples built downstream stay
// valid. A trailing column that carries a header label but no data (e.g. an
// unfilled score column) is intentionally kept — it is real schema, and its
// cells render empty. A header-only sheet is therefore left unchanged, and
// so is any sheet whose trailing columns all have labels.
func trimTrailingEmptyColumns(records [][]string) [][]string {
	if len(records) == 0 {
		return records
	}
	width := 0
	for _, row := range records {
		for c, cell := range row {
			if strings.TrimSpace(cell) != "" && c+1 > width {
				width = c + 1
			}
		}
	}
	if width == 0 {
		return records
	}
	for i, row := range records {
		if len(row) > width {
			records[i] = row[:width]
		}
	}
	return records
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
// strings. Only the header row is padded: merged-master
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

func readSpreadsheetRecords(f *excelize.File, sheet string) ([][]string, []int, int, []mergeRange, []string, error) {
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, nil, 0, nil, nil, fmt.Errorf("read XLSX sheet %q rows: %w", sheet, err)
	}
	if len(rows) == 0 {
		return nil, nil, 0, nil, nil, nil
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

	// Pad the header row to the furthest merged column when a merge touches
	// the header. excelize's GetRows truncates each row at its last valued
	// cell, so a merged slave beyond that point is absent from the slice; the
	// canonical grid builder needs the header width to include the merge
	// extent. Padding runs after detection so a wide merge (e.g. A1:Z1 title)
	// does not dilute the styled-majority signal of a narrow header.
	if mergeTouchesRow(ranges, headerRow) {
		padRowToWidth(&rows[headerRow-1], mergeExtentCol(ranges))
	}

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
	records = trimTrailingEmptyColumns(records)

	return records, absDataRows, headerRow, ranges, warnings, nil
}
