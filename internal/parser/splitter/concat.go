// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package splitter

import (
	"errors"
	"maps"
	"strings"

	"ragflow/internal/entity"
	"ragflow/internal/parser/parser"
)

// OffsetPositions rewrites bounding boxes and page numbers in items by adding
// pageOffset to all page coordinates.
//
// When a sub-PDF is parsed by an OCR engine, the returned bounding boxes and page
// numbers are relative to the sub-PDF (e.g. pages 1..N). When stitching chapters
// back into the global document, adding chapter.StartPage restores global 1-indexed
// visual grounding coordinates for downstream chunking and frontend highlight boxes.
func OffsetPositions(items []parser.DocItem, pageOffset int) []parser.DocItem {
	if pageOffset == 0 || len(items) == 0 {
		return items
	}

	out := make([]parser.DocItem, 0, len(items))
	for _, it := range items {
		if it == nil {
			continue
		}
		out = append(out, offsetDocItem(it, pageOffset))
	}
	return out
}

func offsetDocItem(it parser.DocItem, pageOffset int) parser.DocItem {
	switch item := it.(type) {
	case parser.TableItem:
		newItem := parser.TableItem{
			Sheet:     item.Sheet,
			Positions: offsetTypedPositions(item.Positions, pageOffset),
			Extra:     offsetExtraMap(item.Extra, pageOffset),
		}
		if item.Table != nil {
			tableClone := *item.Table
			if tableClone.Page > 0 {
				tableClone.Page += pageOffset
			}
			newItem.Table = &tableClone
		}
		return newItem

	case parser.ImageItem:
		return parser.ImageItem{
			Text:      item.Text,
			Image:     item.Image,
			Media:     item.Media,
			Sheet:     item.Sheet,
			Positions: offsetTypedPositions(item.Positions, pageOffset),
			Extra:     offsetExtraMap(item.Extra, pageOffset),
		}

	case parser.TextItem:
		return parser.TextItem{
			Text:  item.Text,
			Extra: offsetExtraMap(item.Extra, pageOffset),
		}

	case parser.RawItem:
		return parser.RawItem{
			M: offsetMapVerbatim(item.M, pageOffset),
		}

	default:
		m := offsetMapVerbatim(it.ToMap(), pageOffset)
		return parser.DocItemFromMap(m)
	}
}

func offsetTypedPositions(pos parser.Positions, pageOffset int) parser.Positions {
	if len(pos) == 0 {
		return pos
	}
	newPos := make(parser.Positions, len(pos))
	for i, p := range pos {
		if box, ok := p.(parser.AbsoluteBox); ok {
			box.PageNumber += pageOffset
			newPos[i] = box
		} else {
			newPos[i] = p
		}
	}
	return newPos
}

func offsetExtraMap(extra map[string]any, pageOffset int) map[string]any {
	if len(extra) == 0 {
		return extra
	}
	return offsetMapVerbatim(extra, pageOffset)
}

func offsetMapVerbatim(m map[string]any, pageOffset int) map[string]any {
	if m == nil {
		return nil
	}
	clone := maps.Clone(m)

	if pg, ok := clone["page_number"]; ok {
		if offsetVal, converted := addOffsetToAny(pg, pageOffset); converted {
			clone["page_number"] = offsetVal
		}
	}

	for _, key := range []string{"positions", "_pdf_positions"} {
		if raw, ok := clone[key]; ok && raw != nil {
			clone[key] = offsetRawPositions(raw, pageOffset)
		}
	}

	if table, ok := clone["table"].(*entity.TableData); ok && table != nil {
		tableClone := *table
		if tableClone.Page > 0 {
			tableClone.Page += pageOffset
		}
		clone["table"] = &tableClone
	}

	return clone
}

func offsetRawPositions(raw any, pageOffset int) any {
	switch v := raw.(type) {
	case [][]any:
		out := make([][]any, len(v))
		for i, row := range v {
			if len(row) > 0 {
				newRow := make([]any, len(row))
				copy(newRow, row)
				if off, ok := addOffsetToAny(newRow[0], pageOffset); ok {
					newRow[0] = off
				}
				out[i] = newRow
			} else {
				out[i] = row
			}
		}
		return out

	case [][]float64:
		out := make([][]float64, len(v))
		for i, row := range v {
			if len(row) > 0 {
				newRow := make([]float64, len(row))
				copy(newRow, row)
				newRow[0] += float64(pageOffset)
				out[i] = newRow
			} else {
				out[i] = row
			}
		}
		return out

	case []float64:
		if len(v) >= 5 {
			newTuple := make([]float64, len(v))
			copy(newTuple, v)
			newTuple[0] += float64(pageOffset)
			return newTuple
		}
		return v

	default:
		return raw
	}
}

func addOffsetToAny(val any, offset int) (any, bool) {
	switch n := val.(type) {
	case int:
		return n + offset, true
	case int64:
		return n + int64(offset), true
	case float64:
		return n + float64(offset), true
	default:
		return val, false
	}
}

// ConcatenateResults losslessly aggregates chapter parse results into a single ParseResult.
// It applies coordinate offsets, stitches continuous DocItems sequentially across chapter cuts,
// and updates total page_count metadata (document outlines are stripped here and authoritatively
// attached by the caller's normalize stage).
func ConcatenateResults(chapterResults []parser.ParseResult, plans []ChapterPlan) parser.ParseResult {
	if len(chapterResults) == 0 {
		return parser.ParseResult{}
	}
	if len(chapterResults) != len(plans) {
		return parser.ParseResult{
			Err: errors.New("splitter: chapter results count does not match plans count"),
		}
	}

	var combinedErrors error
	var allWarnings []string
	var allItems []parser.DocItem
	var baseFile map[string]any

	totalPages := 0
	for i, res := range chapterResults {
		if res.Err != nil {
			combinedErrors = errors.Join(combinedErrors, res.Err)
		}
		if res.File != nil && baseFile == nil {
			baseFile = maps.Clone(res.File)
		}
		if len(res.Warnings) > 0 {
			allWarnings = append(allWarnings, res.Warnings...)
		}

		plan := plans[i]
		if plan.EndPage+1 > totalPages {
			totalPages = plan.EndPage + 1
		}

		offsetItems := OffsetPositions(res.JSON, plan.StartPage)
		if len(allItems) > 0 && len(offsetItems) > 0 {
			if merged, ok := tryStitchBoundaryItems(allItems[len(allItems)-1], offsetItems[0]); ok {
				allItems[len(allItems)-1] = merged
				offsetItems = offsetItems[1:]
			}
		}
		allItems = append(allItems, offsetItems...)
	}

	if combinedErrors != nil {
		return parser.ParseResult{
			Err:      combinedErrors,
			Warnings: allWarnings,
		}
	}

	if baseFile == nil {
		baseFile = make(map[string]any)
	}
	if totalPages > 0 {
		baseFile["page_count"] = totalPages
	}
	// Purge local chapter outline so the canonical normalize stage attaches full document outline.
	delete(baseFile, "outline")

	return parser.ParseResult{
		File:     baseFile,
		JSON:     allItems,
		Warnings: allWarnings,
	}
}

// tryStitchBoundaryItems stitches cross-chapter tables spanning across consecutive
// page boundaries (e.g. table continuing from chapter i into chapter i+1).
// Images are discrete page-bound media assets and are not merged across boundaries.
func tryStitchBoundaryItems(prev, next parser.DocItem) (parser.DocItem, bool) {
	t1, ok1 := prev.(parser.TableItem)
	t2, ok2 := next.(parser.TableItem)
	if ok1 && ok2 && t1.Table != nil && t2.Table != nil && len(t1.Table.Grid) > 0 && len(t2.Table.Grid) > 0 {
		_, lastP1 := tableFirstAndLastPage(t1)
		firstP2, _ := tableFirstAndLastPage(t2)
		if lastP1 > 0 && firstP2 > 0 && firstP2 == lastP1+1 {
			if merged, ok := stitchTables(t1, t2); ok {
				return merged, true
			}
		}
	}
	return nil, false
}

// tableFirstAndLastPage returns the 1-indexed inclusive page range [first, last] for a table item.
func tableFirstAndLastPage(t parser.TableItem) (int, int) {
	minP, maxP := 0, 0
	for _, p := range t.Positions {
		if box, ok := p.(parser.AbsoluteBox); ok && box.PageNumber > 0 {
			if minP == 0 || box.PageNumber < minP {
				minP = box.PageNumber
			}
			if box.PageNumber > maxP {
				maxP = box.PageNumber
			}
		}
	}
	if minP > 0 {
		return minP, maxP
	}
	if t.Table != nil && t.Table.Page > 0 {
		return t.Table.Page, t.Table.Page
	}
	if pg, ok := t.Extra["page_number"].(int); ok && pg > 0 {
		return pg, pg
	}
	return 0, 0
}

func isContinuationCaption(c1, c2 string) bool {
	if c1 == "" || c2 == "" {
		return false
	}
	c1Lower := strings.ToLower(strings.TrimSpace(c1))
	c2Lower := strings.ToLower(strings.TrimSpace(c2))
	if strings.Contains(c2Lower, "continued") || strings.Contains(c2Lower, "cont.") ||
		strings.Contains(c2Lower, "(续)") || strings.Contains(c2Lower, "续表") {
		cleanC1 := strings.TrimSpace(c1Lower)
		if strings.HasPrefix(c2Lower, cleanC1) {
			return true
		}
	}
	return false
}

func stitchTables(t1, t2 parser.TableItem) (parser.TableItem, bool) {
	grid1 := t1.Table.Grid
	grid2 := t2.Table.Grid
	if len(grid1) == 0 || len(grid2) == 0 {
		return t1, false
	}

	w1 := maxRowWidth(grid1)
	w2 := maxRowWidth(grid2)
	if w1 == 0 || w2 == 0 {
		return t1, false
	}

	// Columns must match exactly for continuation stitching
	if w1 != w2 {
		return t1, false
	}

	// Continuation evidence check: require matching repeated header signature or continuation caption
	hasContinuationEvidence := false
	startRow := 0

	if t1.Table.HeaderRows > 0 && len(grid2) > t1.Table.HeaderRows {
		repeatsHeader := true
		for r := 0; r < t1.Table.HeaderRows; r++ {
			if !cellsEqual(grid1[r], grid2[r]) {
				repeatsHeader = false
				break
			}
		}
		if repeatsHeader {
			hasContinuationEvidence = true
			startRow = t1.Table.HeaderRows
		}
	} else if len(grid2) > 1 && cellsEqual(grid1[0], grid2[0]) {
		hasContinuationEvidence = true
		startRow = 1
	}

	if !hasContinuationEvidence && isContinuationCaption(t1.Table.Caption, t2.Table.Caption) {
		hasContinuationEvidence = true
	}

	if !hasContinuationEvidence {
		return t1, false
	}

	newGrid := make([][]entity.TableCell, 0, len(grid1)+len(grid2)-startRow)
	for _, row := range grid1 {
		newGrid = append(newGrid, padRow(row, w1))
	}
	for _, row := range grid2[startRow:] {
		newGrid = append(newGrid, padRow(row, w1))
	}

	mergedTable := *t1.Table
	mergedTable.Grid = newGrid
	if mergedTable.Caption == "" && t2.Table.Caption != "" {
		mergedTable.Caption = t2.Table.Caption
	}

	combinedPositions := append(append(parser.Positions(nil), t1.Positions...), t2.Positions...)
	combinedExtra := maps.Clone(t1.Extra)
	if combinedExtra == nil {
		combinedExtra = make(map[string]any)
	}

	return parser.TableItem{
		Table:     &mergedTable,
		Sheet:     t1.Sheet,
		Positions: combinedPositions,
		Extra:     combinedExtra,
	}, true
}

func maxRowWidth(grid [][]entity.TableCell) int {
	m := 0
	for _, r := range grid {
		if len(r) > m {
			m = len(r)
		}
	}
	return m
}

func padRow(row []entity.TableCell, targetWidth int) []entity.TableCell {
	if len(row) >= targetWidth {
		return row
	}
	padded := make([]entity.TableCell, targetWidth)
	copy(padded, row)
	return padded
}

func cellsEqual(r1, r2 []entity.TableCell) bool {
	if len(r1) != len(r2) {
		return false
	}
	for i := range r1 {
		if strings.TrimSpace(r1[i].Text) != strings.TrimSpace(r2[i].Text) {
			return false
		}
	}
	return true
}
