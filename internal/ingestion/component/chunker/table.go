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

// invoke expands the decoded upstream tables into one chunk per data row.
func (c *TableChunkerComponent) invoke(_ context.Context, inputs map[string]any) (map[string]any, error) {
	if inputs == nil {
		return emptyOutputs(), nil
	}
	upstream, err := decodeChunkerFromUpstream(inputs)
	if err != nil {
		return inputErrorOutput(err), nil
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
// pre-upgrade row-IR records, which hold no
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
// It reads the table's canonical merge geometry through tableutil.RowsTable:
// data-origin spans are repeated across their covered slots so each projected
// row keeps the same column count as the header, while a header-origin span
// stays inside the header block and is not stamped into a data row.
//
// Items without TableData pass through unchanged as a single chunk. A
// header-only or fallback chunk keeps the whole table's canonical HTML; each
// per-row chunk carries a flat header+row sub-table (see cellGrid).
func expandHTMLTableRows(item schema.ChunkDoc) []schema.ChunkDoc {
	td := item.TableData
	if td == nil {
		return []schema.ChunkDoc{item}
	}
	rows, headerCount := tableutil.ResolveRows(td, tableutil.RowsTable), td.HeaderRows
	if len(rows) <= headerCount {
		out := item
		out.Text = tableutil.RenderTableHTML(td)
		out.TableData = nil
		return []schema.ChunkDoc{out}
	}
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
		hasText := false
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				hasText = true
				break
			}
		}
		if !hasText {
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

func init() {
	MustRegisterChunker(ComponentNameTableChunker)
}
