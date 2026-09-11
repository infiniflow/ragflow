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

// DeduplicateColumnNames ports Python's _deduplicate_column_names (rag/app/table.py:43-60).
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

// RenderRowsToJSONChunks converts table rows into structured row chunks (one chunk per row),
// respecting column_mode and column_roles.
// Text lines are formatted as "- col: val" and stored in the "text" field.
// Structured metadata fields are stored in the "chunk_data" map.
// Auto mode (the default, matching Python's table chunker where every column
// defaults to "both") indexes every column into text and stores nothing in
// chunk_data. Manual mode honors column_roles; columns without an explicit
// role default to "both". Empty headers are named Column_N, matching Python's
// _parse_simple_headers fallback.
func RenderRowsToJSONChunks(rows [][]string, sheetName string, columnMode string, columnRoles map[string]string) ([]map[string]any, []string) {
	if len(rows) == 0 {
		return nil, nil
	}

	rawHeaders := make([]string, len(rows[0]))
	for i, h := range rows[0] {
		rawHeaders[i] = strings.TrimSpace(h)
		if rawHeaders[i] == "" {
			rawHeaders[i] = fmt.Sprintf("Column_%d", i+1)
		}
	}
	headers := DeduplicateColumnNames(rawHeaders)

	isManual := strings.EqualFold(strings.TrimSpace(columnMode), "manual")
	items := make([]map[string]any, 0, len(rows)-1)

	for r := 1; r < len(rows); r++ {
		row := rows[r]
		textLines := make([]string, 0, len(headers))
		chunkData := make(map[string]any)

		for j := 0; j < len(headers); j++ {
			col := headers[j]
			var val string
			if j < len(row) {
				val = strings.TrimSpace(row[j])
			}
			if val == "" {
				continue
			}

			role := "both"
			if isManual {
				if rVal, ok := columnRoles[col]; ok && strings.TrimSpace(rVal) != "" {
					role = strings.ToLower(strings.TrimSpace(rVal))
				}
				if role == "vectorize" {
					role = "indexing"
				}
			}

			if role == "indexing" || role == "both" {
				textLines = append(textLines, fmt.Sprintf("- %s: %s", col, val))
			}
			if role == "metadata" || role == "both" {
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
