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

package parser

import (
	"fmt"
	"strings"

	"ragflow/internal/common"
)

// DecodeTableColumnConfig extracts column_mode and column_roles from a setup map.
func DecodeTableColumnConfig(setup map[string]any) (string, map[string]string) {
	if setup == nil {
		return "", nil
	}
	var mode string
	if v, ok := setup["column_mode"].(string); ok && v != "" {
		mode = v
	}
	var roles map[string]string
	if v, ok := setup["column_roles"].(map[string]any); ok {
		roles = make(map[string]string, len(v))
		for k, val := range v {
			if s, ok := val.(string); ok {
				roles[k] = s
			}
		}
	} else if v, ok := setup["column_roles"].(map[string]string); ok {
		roles = v
	}
	return mode, roles
}

// DeduplicateColumnNames ports Python's _deduplicate_column_names (rag/app/table.py:43-63).
// Ensures all column header names are unique by appending _2, _3, etc.,
// avoiding collisions with both already-used and existing reserved headers.
func DeduplicateColumnNames(columns []string) []string {
	reserved := make(map[string]struct{}, len(columns))
	for _, col := range columns {
		reserved[col] = struct{}{}
	}
	used := make(map[string]struct{}, len(columns))
	counts := make(map[string]int, len(columns))
	unique := make([]string, 0, len(columns))
	for _, col := range columns {
		counts[col]++
		if _, ok := used[col]; !ok {
			unique = append(unique, col)
			used[col] = struct{}{}
			continue
		}
		suffix := counts[col]
		newName := fmt.Sprintf("%s_%d", col, suffix)
		for {
			_, isUsed := used[newName]
			_, isReserved := reserved[newName]
			if !isUsed && !isReserved {
				break
			}
			suffix++
			newName = fmt.Sprintf("%s_%d", col, suffix)
		}
		counts[col] = suffix
		used[newName] = struct{}{}
		unique = append(unique, newName)
	}
	return unique
}

// TableRowHasContent reports whether a row carries a non-blank cell, the rule
// both the renderer and the schema probe use to find the header row of a table.
func TableRowHasContent(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return true
		}
	}
	return false
}

// tableBookkeepingColumns are the columns Python deletes from every table
// before rendering (`TABLE_BOOKKEEPING_COLUMNS` at rag/app/table.py:70, dropped
// at rag/app/table.py:614-616). They carry no content, and keeping them would
// index the row's primary key into the chunk text and into chunk_data. The
// schema probe drops them too, so the columns it reports are the columns
// ingestion can index.
var tableBookkeepingColumns = map[string]struct{}{
	"id": {}, "_id": {}, "index": {}, "idx": {},
}

// TableHeaderRule selects the header rules of a file family. The table parser
// does not read a spreadsheet and a delimited file the same way: Excel headers
// go through _parse_simple_headers, which trims each cell and names an empty
// one Column_<position> (rag/app/table.py:280-302), while a CSV/TSV header is
// the first record as read and is only deduplicated
// (rag/app/table.py:582, deduplicated at :594). The schema probe applies the
// rule of the file it is shown, so the columns it offers for configuration are
// the columns ingestion indexes.
type TableHeaderRule int

const (
	// TableHeaderRuleSpreadsheet trims cells and renames an empty header to
	// Column_<position>. It is the zero value, which is also the rule the
	// shared header helper had before the file kinds were separated.
	TableHeaderRuleSpreadsheet TableHeaderRule = iota
	// TableHeaderRuleDelimited takes the header cells as read: a padded name
	// stays padded and an empty name stays empty, because that is what the
	// column is called in the index.
	TableHeaderRuleDelimited
)

// TableColumnHeaderNames turns a header row into the column names the table
// parser indexes: the row-bookkeeping columns are dropped and the survivors
// are deduplicated, under the cell rules of rule. sourceIndexes carries each
// surviving name's position in headerRow, which is how a data row is mapped
// onto the columns. The schema probe uses the same function with the same rule,
// so what it offers for configuration is what ingestion produces.
func TableColumnHeaderNames(headerRow []string, rule TableHeaderRule) (names []string, sourceIndexes []int) {
	raw := make([]string, 0, len(headerRow))
	indexes := make([]int, 0, len(headerRow))
	for i, h := range headerRow {
		name := h
		if rule == TableHeaderRuleSpreadsheet {
			name = strings.TrimSpace(h)
			if name == "" {
				name = fmt.Sprintf("Column_%d", i+1)
			}
		}
		if _, reserved := tableBookkeepingColumns[name]; reserved {
			continue
		}
		raw = append(raw, name)
		indexes = append(indexes, i)
	}
	return DeduplicateColumnNames(raw), indexes
}

// RenderRowsToJSONChunks converts table rows into structured row chunks (one chunk per row),
// respecting column_mode and column_roles.
// Text lines are formatted as "- col: val" and stored in the "text" field.
// Structured metadata fields are stored in the "chunk_data" map.
// Auto mode (the default, matching Python's table chunker) gives every column
// the "both" role, so all columns land in text and in chunk_data. Manual mode
// honors column_roles; a column the roles map does not carry is "both".
// headerRule is the header rule of the file being rendered: a caller reading a
// spreadsheet passes TableHeaderRuleSpreadsheet, a CSV/TSV reader passes
// TableHeaderRuleDelimited.
func RenderRowsToJSONChunks(rows [][]string, sheetName string, columnMode string, columnRoles map[string]string, headerRule TableHeaderRule) ([]map[string]any, []string) {
	if len(rows) == 0 {
		return nil, nil
	}

	headerRowIdx := -1
	for rIdx, r := range rows {
		if TableRowHasContent(r) {
			headerRowIdx = rIdx
			break
		}
	}
	if headerRowIdx == -1 {
		return nil, nil
	}

	headers, headerIndexes := TableColumnHeaderNames(rows[headerRowIdx], headerRule)

	isManual := common.NormalizeTableColumnMode(columnMode) == common.TableColumnModeManual
	items := make([]map[string]any, 0, len(rows)-headerRowIdx-1)

	for r := headerRowIdx + 1; r < len(rows); r++ {
		row := rows[r]
		textLines := make([]string, 0, len(headers))
		chunkData := make(map[string]any)

		for j := 0; j < len(headers); j++ {
			col := headers[j]
			var val string
			if src := headerIndexes[j]; src < len(row) {
				val = strings.TrimSpace(row[src])
			}
			if val == "" {
				continue
			}

			// The "both" default belongs to the lookup, not to the normalizer:
			// Python's `column_roles.get(col, "both")` (rag/app/table.py:701)
			// defaults only a column the map does not carry, so a column
			// carried with a blank or unknown value is excluded. common owns
			// that classification, which the indexdoc aggregation shares.
			role := common.ColumnRoleBoth
			if isManual {
				if configured, ok := columnRoles[col]; ok {
					role = common.NormalizeColumnRole(configured)
				}
			}

			if role == common.ColumnRoleIndexing || role == common.ColumnRoleBoth {
				textLines = append(textLines, fmt.Sprintf("- %s: %s", col, val))
			}
			if role == common.ColumnRoleMetadata || role == common.ColumnRoleBoth {
				chunkData[col] = val
			}
		}

		if len(textLines) == 0 && len(chunkData) == 0 {
			continue
		}

		item := map[string]any{
			"text":         strings.Join(textLines, "\n"),
			"doc_type_kwd": "table",
			"ck_type":      "table",
		}
		if sheetName != "" {
			item["sheet"] = sheetName
		}
		if len(chunkData) > 0 {
			item["chunk_data"] = chunkData
		}
		items = append(items, item)
	}

	return items, headers
}
