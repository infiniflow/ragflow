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
	"strings"

	"ragflow/internal/entity"
	"ragflow/internal/parser/tableutil"
)

// splitLargeTable enforces the ChunkTokenSize budget on a structured table,
// splitting it into sub-tables that fit. Splitting is row-granular: every
// emitted sub-table is a whole number of complete rows (never a partial row),
// so the per-row chunk produced downstream keeps full row semantics. The token
// budget is counted on the real cell text of each row, not on any HTML markup.
//
// Alongside the parts it returns dataRanges and headerRows: for part i, the
// half-open range [start, end) of DATA rows (rows below the header) the part
// contains, and the number of leading header rows every part replicates. A
// caller carrying a per-row position matrix slices
// matrix[:headerRows] + matrix[headerRows+start : headerRows+end] to stay
// aligned with the sub-table. headerRows and ranges are 0/nil when the table
// fits in a single chunk (the caller then emits the whole table unchanged).
func splitLargeTable(td *entity.TableData, maxTokens int, countTokens func(string) int) (parts []entity.TableData, dataRanges [][2]int, headerRows int) {
	if td == nil || len(td.Rows) == 0 {
		return nil, nil, 0
	}
	headerRows = td.HeaderRows
	if headerRows > len(td.Rows) {
		headerRows = len(td.Rows)
	}
	dataRows := td.Rows[headerRows:]
	if len(dataRows) == 0 {
		return nil, nil, 0
	}

	// Base tokens: header rows + caption, replicated into every sub-table.
	base := &entity.TableData{Rows: td.Rows[:headerRows], Caption: td.Caption}
	baseTokens := countTokens(tableutil.RenderTableText(base))

	var start int
	var currentTokens int
	flush := func(end int) {
		if end <= start {
			return
		}
		part := entity.TableData{
			Caption:    td.Caption,
			HeaderRows: headerRows,
			Rows:       append(append([][]string{}, td.Rows[:headerRows]...), dataRows[start:end]...),
		}
		parts = append(parts, part)
		dataRanges = append(dataRanges, [2]int{start, end})
		start = end
		currentTokens = baseTokens
	}

	currentTokens = baseTokens
	for i, row := range dataRows {
		rowTokens := countTokens(strings.Join(row, " "))
		// Flush the running batch before this row if adding it would exceed the
		// budget. A single row that alone exceeds the budget still becomes its
		// own chunk (row integrity wins over the budget).
		if i > start && currentTokens+rowTokens > maxTokens {
			flush(i)
		}
		currentTokens += rowTokens
	}
	flush(len(dataRows))

	if len(parts) <= 1 {
		return nil, nil, 0
	}
	return parts, dataRanges, headerRows
}
