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

// ConcatenateResults lossless aggregates chapter parse results into a single ParseResult.
// It applies coordinate offsets, stitches DocItems sequentially, attaches authoritative
// original document outlines, and updates total page_count metadata.
func ConcatenateResults(originalPDF []byte, chapterResults []parser.ParseResult, plans []ChapterPlan) parser.ParseResult {
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
	// Purge local chapter outline so AttachPDFOutlines will populate full document outline.
	delete(baseFile, "outline")

	result := parser.ParseResult{
		File:     baseFile,
		JSON:     allItems,
		Warnings: allWarnings,
	}

	if len(originalPDF) > 0 {
		result = parser.AttachPDFOutlines(result, originalPDF)
	}

	return result
}
