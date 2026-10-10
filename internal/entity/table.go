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
// Coordinates and merge geometry are optional so coordinate-free tables
// (spreadsheet / markdown / html) and coordinate-bearing tables (pdf) share
// one type. The cell text in Rows is already normalized the same way the
// legacy chunker normalized HTML <td>/<th> text (TrimSpace, <br> -> newline,
// inert markup skipped, nested table text folded), so consumers can render
// chunk text directly from Rows without re-parsing any markup.
type TableData struct {
	// Rows holds the cell text, row-major. rows[0:HeaderRows] are header rows.
	Rows [][]string `json:"rows"`
	// HeaderRows is the number of leading header rows. The legacy rule is:
	// rows carrying a <th> cell count as headers; when no row uses <th> at
	// all the first row is treated as the header (HeaderRows == 1).
	HeaderRows int `json:"header_rows"`
	// Caption is the table caption (pdf emits it; office uses the sheet name).
	Caption string `json:"caption,omitempty"`
	// Spans records merged cells. Recovered information: pdf already emits
	// colspan/rowspan in its HTML and office loses merged-cell geometry today;
	// consumers do not use Spans yet (preserved for a later follow-up).
	Spans []CellSpan `json:"spans,omitempty"`
	// Page is the pdf 0-based page number, used for layout retrieval.
	Page int `json:"page,omitempty"`
	// Cells carries optional per-cell coordinates (pdf only); currently lost
	// in the legacy HTML serialization and preserved here for later use.
	Cells []CellCoord `json:"cells,omitempty"`
}

// CellSpan identifies a merged cell: the top-left anchor at (Row, Col) spans
// RowSpan rows and ColSpan columns.
type CellSpan struct {
	Row, Col, RowSpan, ColSpan int
}

// CellCoord is the bounding box of a single cell in pdf page coordinates.
type CellCoord struct {
	Row, Col       int
	X0, Y0, X1, Y1 float64
}
