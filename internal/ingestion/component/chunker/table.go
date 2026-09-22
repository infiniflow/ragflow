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
// Go port of the Python `table` chunk method (rag/app/table.py), whose
// docstring states: "Every row in table will be treated as a chunk."
//
// Unlike TokenChunker (which token-shreds a row's text into multiple
// pieces) or OneChunker (which merges many rows into a single chunk),
// TableChunker keeps the row as the unit of chunking. Row-IR records from
// spreadsheet parsers pass through unchanged — the table parser produced
// one structured record per row already. HTML table payloads are expanded
// here: each data row becomes one chunk whose text repeats the column names
// in the Python "- field: value" line format, carrying its own position
// tuple when the item's position matrix is row-aligned.
package chunker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/ingestion/component/schema"

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
// falling back to Chunks. Each record becomes exactly one chunk.
func tableItems(items, chunks []schema.ChunkDoc) []schema.ChunkDoc {
	source := items
	if len(source) == 0 {
		source = chunks
	}
	if len(source) == 0 {
		return nil
	}
	dataTables := make(map[string]struct{})
	for _, item := range source {
		if item.CKType == "table_row" {
			dataTables[spreadsheetTableKey(item)] = struct{}{}
		}
	}
	filtered := make([]schema.ChunkDoc, 0, len(source))
	for _, item := range source {
		// Spreadsheet parsers expose the header as schema metadata. It is
		// already represented in each table_row and must not become a data
		// chunk of its own. A table with no data rows still needs its header
		// as the only searchable representation.
		if item.CKType == "table_header" {
			if _, hasRows := dataTables[spreadsheetTableKey(item)]; hasRows {
				continue
			}
		}
		filtered = append(filtered, expandHTMLTableRows(item)...)
	}
	return filtered
}

// expandHTMLTableRows turns one HTML <table> payload into one chunk per data
// row. Row-IR records (which are already per-row) and non-table payloads
// pass through unchanged. A table whose only row is the header keeps the
// whole markup as its single chunk: the header line is then the only
// searchable representation.
func expandHTMLTableRows(item schema.ChunkDoc) []schema.ChunkDoc {
	if item.CKType == "table_row" || item.CKType == "table_header" || !isTableHTML(item.Text) {
		return []schema.ChunkDoc{item}
	}
	rows, headerCount := tableRowsWithHeader(item.Text)
	if len(rows) <= headerCount {
		return []schema.ChunkDoc{item}
	}
	names := rows[0]
	// R1: a row chunk must carry only its own tuple. That is only sound when
	// the item's matrix was built row-aligned (one tuple per <tr>, header
	// included); a whole-table tuple must not be copied onto every row, so
	// misaligned payloads get no positions at all.
	var matrix [][]float64
	aligned := false
	if len(item.Positions) > 0 {
		if err := json.Unmarshal(item.Positions, &matrix); err == nil && len(matrix) == len(rows) {
			aligned = true
		}
	}
	out := make([]schema.ChunkDoc, 0, len(rows)-headerCount)
	for i, row := range rows[headerCount:] {
		text := tableRowRecordText(names, row)
		if text == "" {
			continue
		}
		doc := item
		doc.Text = text
		doc.TKNums = nil
		doc.Positions = nil
		if aligned {
			if tuple, err := json.Marshal([][]float64{matrix[headerCount+i]}); err == nil {
				doc.Positions = tuple
			}
		}
		out = append(out, doc)
	}
	if len(out) == 0 {
		return []schema.ChunkDoc{item}
	}
	return out
}

// tableRowRecordText renders one row in rag/app/table.py's line format:
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

func spreadsheetTableKey(item schema.ChunkDoc) string {
	if item.TableID != "" {
		return "table:" + item.TableID
	}
	if item.SheetIndex != nil {
		return fmt.Sprintf("sheet-index:%d", *item.SheetIndex)
	}
	if item.Sheet != "" {
		return "sheet:" + item.Sheet
	}
	if sheet, ok := spreadsheetPositionSheet(item); ok {
		return fmt.Sprintf("position-sheet:%g", sheet)
	}
	return "unknown"
}

func init() {
	MustRegisterChunker(ComponentNameTableChunker)
}
