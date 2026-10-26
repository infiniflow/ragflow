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
// See the License for the specific language governing permissions and
// limitations under the License.
//

package tableutil

import "ragflow/internal/entity"

// RowMode selects how ResolveRows interprets a TableData's merge geometry.
//
// The grid stores merge geometry once: an origin cell carries ColSpan/RowSpan
// and the positions it engulfs are Covered markers. How those Covered slots
// are projected into text is a consumer decision:
//
//   - RowsTable keeps every logical column filled. A data-origin span is
//     repeated across its covered slots so a row projected under a header stays
//     column-aligned; a header-origin span is repeated only inside the header
//     block and is never stamped into data rows.
//   - RowsData is the general-context view: data-origin row spans are resolved
//     at the origin column, header-origin spans stay out of data rows, and a
//     colspan is not repeated (its value already appears once).
//   - RowsQA treats every row as data. All row spans (including one mastered
//     on the first row) are projected at the origin column, while col spans are
//     not repeated: a question that covers two columns must still pair with the
//     answer in the next non-empty cell, not with a copy of itself.
type RowMode int

const (
	// RowsTable is the tabular view consumed by the table chunker.
	RowsTable RowMode = iota
	// RowsData is the data-row view consumed by the general chunker.
	RowsData
	// RowsQA is the all-rows-are-data view consumed by the QA chunker.
	RowsQA
)

// ResolveRows projects a TableData grid into row-major cell text according to
// mode. It deliberately derives spans from RowSpan/ColSpan/Covered alone so a
// caller does not depend on OriginRow/OriginCol bookkeeping; those fields are
// consumed by the table splitter when it clamps a span to a part.
func ResolveRows(td *entity.TableData, mode RowMode) [][]string {
	if td == nil {
		return nil
	}
	out := make([][]string, len(td.Grid))
	type activeSpan struct {
		text      string
		left      int
		originRow int
	}
	active := map[int]*activeSpan{}

	for r, row := range td.Grid {
		cells := make([]string, len(row))
		colspanText := ""
		colspanLeft := 0
		for c, cell := range row {
			if cell.Covered {
				if sp := active[c]; sp != nil && sp.left > 0 {
					// A header-origin span must not leak into a data row in
					// the table view: the header is rendered separately and
					// repeating its label would turn a column name into a
					// value. QA asks for the opposite projection.
					if mode == RowsQA || r < td.HeaderRows || sp.originRow >= td.HeaderRows {
						cells[c] = sp.text
					}
				} else if colspanLeft > 0 {
					// RowsTable keeps the full column count; RowsQA leaves the
					// engulfed slots empty so first-two-cell pairing is not
					// confused by a duplicated question.
					if mode == RowsTable {
						cells[c] = colspanText
					}
				}
				if colspanLeft > 0 {
					colspanLeft--
				}
				continue
			}

			cells[c] = cell.Text
			if cell.RowSpan > 1 {
				if mode == RowsTable {
					spanCols := cell.ColSpan
					if spanCols < 1 {
						spanCols = 1
					}
					for cc := c; cc < c+spanCols; cc++ {
						active[cc] = &activeSpan{text: cell.Text, left: cell.RowSpan, originRow: r}
					}
				} else {
					// One value belongs in the origin column. RowsData and
					// RowsQA must not duplicate it across a horizontal span:
					// general context would repeat the text, and QA would pair
					// a question with a copy of itself.
					active[c] = &activeSpan{text: cell.Text, left: cell.RowSpan, originRow: r}
				}
			}
			if cell.ColSpan > 1 {
				colspanText = cell.Text
				colspanLeft = cell.ColSpan
			}
			if colspanLeft > 0 {
				colspanLeft--
			}
		}
		for c, sp := range active {
			sp.left--
			if sp.left <= 0 {
				delete(active, c)
			}
		}
		out[r] = cells
	}
	return out
}
