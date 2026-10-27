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

package entity

// TableData is the canonical structured table contract shared by every parser
// (office / markdown / html / pdf) and consumed by the ingestion chunkers.
//
// The table is stored as a dense, normalized grid (BuildGrid in
// internal/parser/tableutil enforces this): every logical position appears
// exactly once. A merged cell keeps its text on the origin slot and sets
// ColSpan/RowSpan > 1; the positions it covers are Covered slots that carry no
// text of their own. This replaces the previous parallel Spans list, which was
// never populated, so merge geometry is now a single source of truth instead
// of two fields that could drift apart.
//
// Coordinates and merge geometry are optional so coordinate-free tables
// (spreadsheet / markdown / html) and coordinate-bearing tables (pdf) share
// one type. The cell text in Grid is already normalized the same way the
// legacy chunker normalized HTML <td>/<th> text (TrimSpace, <br> -> newline,
// inert markup skipped, nested table text folded), so consumers can render
// chunk text directly from Grid without re-parsing any markup.
type TableData struct {
	// Grid holds the cell text, row-major and dense (every row has the same
	// width). Grid[0:HeaderRows] are header rows. A Covered slot contributes
	// no text; its content lives at the origin slot named by OriginRow/
	// OriginCol.
	Grid [][]TableCell `json:"grid"`
	// HeaderRows is the number of leading header rows. The legacy rule is:
	// rows carrying a <th> cell count as headers; when no row uses <th> at
	// all the first row is treated as the header (HeaderRows == 1).
	HeaderRows int `json:"header_rows"`
	// Caption is the table caption (pdf emits it; office uses the sheet name).
	Caption string `json:"caption,omitempty"`
	// Page is the pdf 0-based page number, used for layout retrieval.
	Page int `json:"page,omitempty"`
}

// TableCell is one position in TableData.Grid.
type TableCell struct {
	// Text is the cell's content (already normalized like the legacy <td>
	// text: TrimSpace, <br> -> newline, inert markup skipped).
	Text string `json:"text"`
	// ColSpan / RowSpan are the extents a merged cell covers. 0 or 1 means the
	// cell is not merged. Only meaningful on non-Covered origin slots.
	ColSpan int `json:"col_span,omitempty"`
	RowSpan int `json:"row_span,omitempty"`
	// Covered marks a position swallowed by an origin cell's span. Its Text is
	// empty; readers must skip it (the real content is at the origin slot).
	Covered bool `json:"covered,omitempty"`
	// OriginRow / OriginCol name the slot whose span produced this Covered
	// marker. They are serialized because the table splitter clamps a header
	// span after the parser payload has round-tripped through JSON; zero is a
	// valid origin, so omitempty is safe (it decodes back to zero).
	OriginRow int `json:"origin_row,omitempty"`
	OriginCol int `json:"origin_col,omitempty"`
}

// CellTexts returns the raw row-major cell texts, skipping Covered positions
// (their content lives at the origin). For a grid with no merges this is
// exactly the legacy Rows: same strings, same row lengths. Consumers that need
// merged values projected into the covered slots (the table chunker's
// column-aligned view, or QA's all-rows-are-data view) should call
// tableutil.ResolveRows with the appropriate mode instead.
func (td *TableData) CellTexts() [][]string {
	if td == nil {
		return nil
	}
	out := make([][]string, 0, len(td.Grid))
	for _, row := range td.Grid {
		cells := make([]string, 0, len(row))
		for _, c := range row {
			if c.Covered {
				continue
			}
			cells = append(cells, c.Text)
		}
		out = append(out, cells)
	}
	return out
}
