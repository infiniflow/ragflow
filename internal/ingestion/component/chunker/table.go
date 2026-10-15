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

// TableChunker emits one chunk per upstream table row. It is the faithful
// Go port of the Python `table` chunk method, whose docstring states:
// "Every row in table will be treated as a chunk."
//
// Unlike TokenChunker (which token-shreds a row's text into multiple
// pieces) or OneChunker (which merges many rows into a single chunk),
// TableChunker keeps the row as the unit of chunking. Every table arrives as
// structured TableData on ChunkDoc.TableData — the parsers emit captioned
// table segments — and each data row becomes one chunk whose text repeats the
// column names in the Python "- field: value" line format, carrying its own
// position tuple when the item is a spreadsheet item with a row-aligned
// matrix. A table whose only row is the header stays one whole chunk.
package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/component/schema"
	"ragflow/internal/parser/tableutil"

	"gorm.io/gorm"
)

const ComponentNameTableChunker = "TableChunker"

type tableChunkerParam struct{}

func (p *tableChunkerParam) Update(conf map[string]any) {}

func (tableChunkerParam) Defaults() tableChunkerParam { return tableChunkerParam{} }

func (tableChunkerParam) Validate() error { return nil }

type TableChunkerComponent struct {
	name  string
	param tableChunkerParam
}

func NewTableChunker(params map[string]any) (runtime.Component, error) {
	p := tableChunkerParam{}.Defaults()
	(&p).Update(params)
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &TableChunkerComponent{
		name:  ComponentNameTableChunker,
		param: p,
	}, nil
}
func (c *TableChunkerComponent) Inputs() map[string]string { return ChunkerInputs }

func (c *TableChunkerComponent) Outputs() map[string]string { return ChunkerOutputs }

func (c *TableChunkerComponent) Invoke(ctx context.Context, db *gorm.DB, inputs map[string]any) (map[string]any, error) {
	return c.invoke(ctx, inputs)
}

func (c *TableChunkerComponent) invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		return emptyOutputs(), nil
	}
	upstream, err := decodeChunkerFromUpstream(inputs)
	if err != nil {
		return map[string]any{
			"output_format": "chunks",
			"chunks":        []map[string]any{},
			"_ERROR":        fmt.Sprintf("Input error: %v", err),
		}, nil
	}

	switch upstream.OutputFormat {
	case schema.PayloadFormatMarkdown:
		if upstream.MarkdownResult == nil {
			return emptyOutputs(), nil
		}
		return emitOne(*upstream.MarkdownResult, "text"), nil
	case schema.PayloadFormatText:
		if upstream.TextResult == nil {
			return emptyOutputs(), nil
		}
		return emitOne(*upstream.TextResult, "text"), nil
	case schema.PayloadFormatHTML:
		if upstream.HTMLResult == nil {
			return emptyOutputs(), nil
		}
		return emitOne(*upstream.HTMLResult, "text"), nil
	default:
		// Row-structured payload: one chunk per upstream record.
		items := tableItems(upstream.JSONResult, upstream.Chunks)
		if len(items) == 0 {
			return emptyOutputs(), nil
		}
		return chunkOutputs(items), nil
	}
}

// tableItems returns the per-row records, preferring JSONResult and
// falling back to Chunks. Each table row becomes exactly one chunk; every
// other payload record passes through as one chunk — including pre-upgrade
// row-IR records (ck_type: table_row/table_header with cells), which hold no
// markup and therefore keep no per-row positions; documents from before this
// wire must be re-parsed rather than re-chunked.
func tableItems(items, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	source := items
	if len(source) == 0 {
		source = chunks
	}
	if len(source) == 0 {
		return nil
	}
	filtered := make([]schema.ChunkDoc, 0, len(source))
	for _, item := range source {
		filtered = append(filtered, expandHTMLTableRows(item)...)
	}
	return filtered
}

// expandHTMLTableRows turns one table payload into one chunk per data row.
// tableChunkRows projects a structured TableData to row-major cell texts while
// preserving one value per logical column. Unlike CellTexts it does NOT drop
// Covered slots: a slot engulfed by a span is resolved back to the spanning
// origin's text so each projected row keeps the same column count as the header
// and stays aligned with it. CellTexts would drop a spanned continuation column
// and shift the surviving cells under the wrong headers — exactly the mis-match
// a merged HTML table hits when it is expanded per row.
//
// Resolution works from the grid alone. As rows are scanned top-to-bottom a
// per-column "active rowspan" remembers the text each ancestor rowspan still
// contributes, and within a row a running colspan remembers the text of a just
// seen wide origin. This deliberately avoids OriginRow/OriginCol, which are
// json:"-" and therefore absent after the TableData is serialized to JSON and
// read back — the path expandHTMLTableRows actually consumes.
func tableChunkRows(td *entity.TableData) [][]string {
	if td == nil {
		return nil
	}
	out := make([][]string, 0, len(td.Grid))
	rowspanText := []string{}
	rowspanLeft := []int{}
	for _, row := range td.Grid {
		texts := make([]string, 0, len(row))
		colspanLeft := 0
		colspanText := ""
		for c, cell := range row {
			for len(rowspanText) <= c {
				rowspanText = append(rowspanText, "")
				rowspanLeft = append(rowspanLeft, 0)
			}
			if cell.Covered {
				if rowspanLeft[c] > 0 {
					texts = append(texts, rowspanText[c])
				} else {
					texts = append(texts, colspanText)
				}
			} else {
				texts = append(texts, cell.Text)
				// A merged cell that spans rows AND columns must register its
				// active rowspan across the full [c, c+ColSpan) span, not just
				// the origin column. Its continuation rows own every covered
				// column, so each must resolve back to the origin text; had we
				// only registered the start column, the extra covered columns
				// would read as blank and the row would lose those fields.
				// An unset ColSpan (0) normalizes to 1 so a row-only span still
				// registers its single start column.
				if cell.RowSpan > 1 {
					colSpan := cell.ColSpan
					if colSpan < 1 {
						colSpan = 1
					}
					for cc := c; cc < c+colSpan; cc++ {
						for len(rowspanText) <= cc {
							rowspanText = append(rowspanText, "")
							rowspanLeft = append(rowspanLeft, 0)
						}
						rowspanText[cc] = cell.Text
						rowspanLeft[cc] = cell.RowSpan
					}
				}
				if cell.ColSpan > 1 {
					colspanLeft = cell.ColSpan
					colspanText = cell.Text
				}
			}
			if colspanLeft > 0 {
				colspanLeft--
			}
		}
		for c := range rowspanLeft {
			if rowspanLeft[c] > 0 {
				rowspanLeft[c]--
			}
		}
		out = append(out, texts)
	}
	return out
}

// expandHTMLTableRows turns one table payload into one chunk per data row.
// It reads the structured TableData contract the parser emits on
// ChunkDoc.TableData; items without TableData (pre-upgrade HTML markup or
// non-table records) pass through unchanged as a single chunk. Every emitted
// chunk carries its content rendered as an HTML <table> on Text, so the
// persisted content and the chunk id (a hash of Text) describe the same bytes
// and the UI/index can display a real table; the structured TableData is
// dropped here and never serialized to the index. Header-only and fallback
// chunks keep the whole table's HTML (merge geometry intact); per-row chunks
// carry a flat header+row sub-table (merge-free, see cellGrid).
func expandHTMLTableRows(item schema.ChunkDoc) []schema.ChunkDoc {
	td := item.TableData
	if td == nil {
		return []schema.ChunkDoc{item}
	}
	rows, headerCount := tableChunkRows(td), td.HeaderRows
	if len(rows) <= headerCount {
		out := item
		out.Text = tableutil.RenderTableHTML(td)
		out.TableData = nil
		return []schema.ChunkDoc{out}
	}
	names := rows[0]
	// R1: a row chunk must carry only its own tuple. That is only sound when
	// the item carries spreadsheet identity and its matrix was built
	// row-aligned (one five-field tuple per row, header included); a
	// whole-table tuple must not be copied onto every row, and PDF items write
	// layout boxes into the same field, so misaligned or non-spreadsheet
	// payloads get no positions at all.
	var matrix [][]float64
	aligned := false
	if item.SheetIndex != nil && len(item.Positions) > 0 {
		if err := json.Unmarshal(item.Positions, &matrix); err == nil && len(matrix) == len(rows) {
			aligned = true
			for _, tuple := range matrix {
				if len(tuple) != 5 {
					aligned = false
					break
				}
			}
		}
	}
	out := make([]schema.ChunkDoc, 0, len(rows)-headerCount)
	for i, row := range rows[headerCount:] {
		// Skip empty rows (no cell content); the HTML render of an empty row
		// would be a header-only table that duplicates the header-only chunk.
		if tableRowRecordText(names, row) == "" {
			continue
		}
		doc := item
		// Render this row as its own HTML table: every projected header row
		// plus this data row, so the displayed table keeps the full header.
		doc.Text = tableutil.RenderTableHTML(&entity.TableData{
			Grid:       cellGrid(append(append([][]string{}, rows[:headerCount]...), row)),
			HeaderRows: headerCount,
			Caption:    td.Caption,
		})
		doc.TKNums = nil
		doc.Positions = nil
		doc.TableData = nil
		if aligned {
			if tuple, err := json.Marshal([][]float64{matrix[headerCount+i]}); err == nil {
				doc.Positions = tuple
			}
		}
		out = append(out, doc)
	}
	if len(out) == 0 {
		single := item
		single.Text = tableutil.RenderTableHTML(td)
		single.TableData = nil
		return []schema.ChunkDoc{single}
	}
	return out
}

// cellGrid builds a merge-free dense Grid from row-major cell strings. It is
// used to render a per-row sub-table (the projected header rows plus one data
// row) as HTML; merged-cell geometry is intentionally not reconstructed here,
// so the sub-table shows each cell plainly rather than with colspan/rowspan.
func cellGrid(rows [][]string) [][]entity.TableCell {
	grid := make([][]entity.TableCell, len(rows))
	for i, r := range rows {
		grid[i] = make([]entity.TableCell, len(r))
		for j, c := range r {
			grid[i][j] = entity.TableCell{Text: c}
		}
	}
	return grid
}

// tableDataOf returns the structured TableData for an item. The parser emits
// it on ChunkDoc.TableData; items without it (pre-upgrade HTML markup or
// non-table records) return nil and pass through unchanged. Re-chunk always
// re-parses from the source document, so a payload never arrives with HTML
// but no TableData, and no HTML fallback is performed (decision §9.1).
func tableDataOf(item schema.ChunkDoc) *entity.TableData {
	return item.TableData
}

// tableRowRecordText renders one row in the Python table chunker's line format:
// "- {field}: {value}" per non-empty cell, newline joined. A cell whose
// column has no header name keeps its value alone rather than being dropped.
func tableRowRecordText(names, cells []string) string {
	lines := make([]string, 0, len(cells))
	for j, cell := range cells {
		value := strings.TrimSpace(cell)
		if value == "" {
			continue
		}
		name := ""
		if j < len(names) {
			name = strings.TrimSpace(names[j])
		}
		if name != "" {
			lines = append(lines, "- "+name+": "+value)
			continue
		}
		lines = append(lines, "- "+value)
	}
	return strings.Join(lines, "\n")
}

func init() {
	MustRegisterChunker(ComponentNameTableChunker)
}
