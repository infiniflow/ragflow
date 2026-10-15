//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package chunker

import (
	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// splitLargeTable enforces the ChunkTokenSize budget on a structured table,
// splitting it into sub-tables that fit. Splitting is row-granular: every
// emitted sub-table is a whole number of complete rows (never a partial row),
// so the per-row chunk produced downstream keeps full row semantics. The token
// budget is counted on the HTML rendering of each row — the text actually
// stored on the chunk — so a rendered part never exceeds the budget.
//
// Splitting is also span-safe: a cut never falls inside a merged cell's
// rowspan, so an origin and every row it covers stay in the same part, and a
// header cell whose rowspan reached into data rows is re-derived per part so
// it never claims rows the part does not contain. Every part therefore
// satisfies the same grid contract as a freshly parsed table.
//
// Alongside the parts it returns dataRanges and headerRows: for part i, the
// half-open range [start, end) of DATA rows (rows below the header) the part
// contains, and the number of leading header rows every part replicates. A
// caller carrying a per-row position matrix slices
// matrix[:headerRows] + matrix[headerRows+start : headerRows+end] to stay
// aligned with the sub-table. headerRows and ranges are 0/nil when the table
// fits in a single chunk (the caller then emits the whole table unchanged).
func splitLargeTable(td *entity.TableData, maxTokens int, countTokens func(string) int) (parts []entity.TableData, dataRanges [][2]int, headerRows int) {
	if td == nil || len(td.Grid) == 0 {
		return nil, nil, 0
	}
	headerRows = td.HeaderRows
	if headerRows > len(td.Grid) {
		headerRows = len(td.Grid)
	}
	dataRows := td.Grid[headerRows:]
	if len(dataRows) == 0 {
		return nil, nil, 0
	}

	headerGrid := append([][]entity.TableCell{}, td.Grid[:headerRows]...)

	// Base tokens: header rows + caption, replicated into every sub-table,
	// counted on the HTML rendering — the text actually stored on the chunk —
	// so a rendered part never exceeds the budget. The <table> wrapper + caption
	// appear once per part; emptyWrapper isolates them so each data row below is
	// charged only its own <tr> contribution.
	base := &entity.TableData{Grid: headerGrid, Caption: td.Caption}
	baseTokens := countTokens(tableutil.RenderTableHTML(base))
	emptyWrapper := countTokens(tableutil.RenderTableHTML(&entity.TableData{Caption: td.Caption}))

	// spanThrough[i] is the last data-row index any rowspan originating at or
	// before row i still covers (-1 while no span is active). Header-origin
	// spans need no entry here: every part carries the whole header block.
	spanThrough := make([]int, len(dataRows))
	maxEnd := -1
	for i, row := range dataRows {
		for _, cell := range row {
			if !cell.Covered && cell.RowSpan > 1 {
				if end := i + cell.RowSpan - 1; end > maxEnd {
					maxEnd = end
				}
			}
		}
		spanThrough[i] = maxEnd
	}
	// safeCut moves a desired cut at j right to the end of the span group it
	// would otherwise split. Span-group integrity wins over the budget, the
	// same way single-row integrity already does.
	safeCut := func(j int) int {
		if j <= 0 || j >= len(dataRows) {
			return j
		}
		if end := spanThrough[j-1]; end >= j {
			if end+1 > len(dataRows) {
				return len(dataRows)
			}
			return end + 1
		}
		return j
	}

	var start int
	var currentTokens int
	flush := func(end int) {
		if end <= start {
			return
		}
		part := entity.TableData{
			Caption:    td.Caption,
			HeaderRows: headerRows,
			Grid: append(cloneHeaderGridForPart(headerGrid, dataRows, start, end, headerRows),
				partDataRows(dataRows, start, end, headerRows)...),
		}
		parts = append(parts, part)
		dataRanges = append(dataRanges, [2]int{start, end})
		start = end
		currentTokens = baseTokens
	}

	currentTokens = baseTokens
	for i, row := range dataRows {
		if i < start {
			continue // already belongs to a flushed part
		}
		rowTokens := countTokens(tableutil.RenderTableHTML(&entity.TableData{Grid: [][]entity.TableCell{row}, Caption: td.Caption})) - emptyWrapper
		// Flush the running batch before this row if adding it would exceed the
		// budget. A single row that alone exceeds the budget still becomes its
		// own chunk (row integrity wins over the budget).
		if i > start && currentTokens+rowTokens > maxTokens {
			end := safeCut(i)
			flush(end)
			if i < end {
				continue // this row is inside the flushed part
			}
		}
		currentTokens += rowTokens
	}
	flush(len(dataRows))

	if len(parts) <= 1 {
		return nil, nil, 0
	}
	return parts, dataRanges, headerRows
}

// cloneHeaderGridForPart copies the header grid for one split part and
// re-derives each header origin's RowSpan from the data rows the part
// actually contains. A header cell whose rowspan reached into data rows must
// not claim rows outside the part: the part keeps the full span when all
// spanned rows travel with it, shrinks it when only some do, and degrades it
// to a plain cell when none do. ColSpan is row-local and never changes.
func cloneHeaderGridForPart(headerGrid, dataRows [][]entity.TableCell, start, end, headerRows int) [][]entity.TableCell {
	cloned := make([][]entity.TableCell, len(headerGrid))
	for r, row := range headerGrid {
		cloned[r] = append([]entity.TableCell{}, row...)
		for c := range cloned[r] {
			cell := &cloned[r][c]
			if cell.Covered || cell.RowSpan <= 1 {
				continue
			}
			// Rows below the origin inside the header block are in every
			// part; count those first, then the part's own covered rows.
			below := headerRows - 1 - r
			if below > cell.RowSpan-1 {
				below = cell.RowSpan - 1
			}
			span := 1 + below
			for i := start; i < end && span < cell.RowSpan; i++ {
				if c >= len(dataRows[i]) {
					break
				}
				covered := dataRows[i][c]
				if !covered.Covered || covered.OriginRow != r || covered.OriginCol != c {
					break
				}
				span++
			}
			cell.RowSpan = span
		}
	}
	return cloned
}

// partDataRows clones the data rows [start,end) for one part and shifts the
// OriginRow back-pointer of every Covered slot whose origin is a data-row
// cell (OriginRow >= headerRows) so it names the origin's row inside the
// part; data rows move down by start. Header-origin markers keep their index
// because the header block is identical in every part. The source grid is
// never mutated.
func partDataRows(dataRows [][]entity.TableCell, start, end, headerRows int) [][]entity.TableCell {
	out := make([][]entity.TableCell, 0, end-start)
	for i := start; i < end; i++ {
		row := make([]entity.TableCell, len(dataRows[i]))
		copy(row, dataRows[i])
		for c := range row {
			if row[c].Covered && row[c].OriginRow >= headerRows {
				row[c].OriginRow -= start
			}
		}
		out = append(out, row)
	}
	return out
}
