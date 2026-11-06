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

package vastbase

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"
)

// queryRows runs a query and shapes each row into a map keyed by column name.
// NULL columns are omitted, matching the ES _source behavior of not
// returning fields that were never written.
func (e *Engine) queryRows(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

// queryRowsTx is queryRows bound to an open transaction.
func queryRowsTx(ctx context.Context, tx *sql.Tx, query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

// scanRows drains a result set into per-row maps keyed by column name.
func scanRows(rows *sql.Rows) ([]map[string]interface{}, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var result []map[string]interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		destinations := make([]interface{}, len(columns))
		for i := range values {
			destinations[i] = &values[i]
		}
		if err := rows.Scan(destinations...); err != nil {
			return nil, err
		}
		row := make(map[string]interface{}, len(columns))
		for i, column := range columns {
			if values[i] == nil {
				continue
			}
			if raw, ok := values[i].([]byte); ok {
				row[column] = string(raw)
			} else {
				row[column] = values[i]
			}
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

// decodeLogicalRow converts a physical row into domain values: memory columns
// are renamed to their logical fields, keyword columns are split on ###,
// position_int is regrouped into 5-tuples, integer arrays and JSON text are
// parsed, and floatvector text becomes a float slice (lib/pq returns unknown
// OIDs as text, so decoding rides on the q_(\d+)_vec column-name pattern).
func decodeLogicalRow(row map[string]interface{}, kind string) map[string]interface{} {
	result := make(map[string]interface{}, len(row))
	for rawColumn, value := range row {
		column := rawColumn
		if kind == "memory" {
			if vectorColumnPattern.MatchString(column) {
				// Multiple q_N_vec columns coexist after an embedding-model
				// change; backfillVectorColumns zero-fills the stale ones, and
				// GetChunk's SELECT * reads them all. A zero placeholder must
				// not overwrite a real embedding: keep the first non-zero
				// vector and skip zeros once content_embed is set.
				if text, ok := value.(string); ok {
					if vector := parseFloatVectorText(text); !isZeroVector(vector) {
						result["content_embed"] = vector
						continue
					} else if _, set := result["content_embed"]; set {
						continue
					}
				}
				column = "content_embed"
			} else if mapped, ok := memoryColumnToField[column]; ok {
				column = mapped
			}
		}
		if kind == "memory" && column == "status" {
			switch status := value.(type) {
			case int64:
				value = status != 0
			case int:
				value = status != 0
			case string:
				value = status != "" && status != "0"
			}
		}
		value = decodeColumnValue(kind, rawColumn, value)
		result[column] = value
	}
	return result
}

// decodeColumnValue converts one physical column value to its logical form:
// vector text to float slices, keyword joins back to lists, JSON columns to
// maps.
func decodeColumnValue(kind, column string, value interface{}) interface{} {
	if value == nil {
		return nil
	}
	text, isText := value.(string)
	switch {
	case vectorColumnPattern.MatchString(column):
		if isText {
			return parseFloatVectorText(text)
		}
	// Memory keyword-named columns (message_type_kwd, source_id) store plain
	// scalars — the Python memory connector read them back without the ###
	// split reserved for chunk keyword lists.
	case fieldKeyword(column) && kind != "memory":
		if isText {
			parts := strings.Split(text, keywordSeparator)
			keywords := make([]string, 0, len(parts))
			for _, part := range parts {
				if part != "" {
					keywords = append(keywords, part)
				}
			}
			return keywords
		}
	case column == "position_int":
		if isText {
			return regroupPositions(parsePgIntArray(text))
		}
	case integerArrayColumns[column]:
		if isText {
			return parsePgIntArray(text)
		}
	case jsonTextColumns[column] || strings.HasSuffix(column, "_feas"):
		if isText && strings.HasPrefix(strings.TrimSpace(text), "{") {
			var decoded interface{}
			if err := json.Unmarshal([]byte(text), &decoded); err == nil {
				return decoded
			}
		}
	}
	return value
}

// regroupPositions converts the flat integer[] storage back into the original
// [[page, x1, y1, x2, y2], ...] five-element groups.
func regroupPositions(flat []int64) [][]int64 {
	groups := make([][]int64, 0, (len(flat)+4)/5)
	for start := 0; start < len(flat); start += 5 {
		end := start + 5
		if end > len(flat) {
			end = len(flat)
		}
		groups = append(groups, flat[start:end])
	}
	return groups
}

// parseFloatVectorText decodes "[0.1,0.2,...]" floatvector text.
func parseFloatVectorText(text string) []float64 {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "[")
	text = strings.TrimSuffix(text, "]")
	if text == "" {
		return nil
	}
	parts := strings.Split(text, ",")
	values := make([]float64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		number, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return nil
		}
		values = append(values, number)
	}
	return values
}

// isZeroVector reports whether a decoded vector is the all-zero placeholder
// backfillVectorColumns writes into stale dimension columns.
func isZeroVector(values []float64) bool {
	for _, value := range values {
		if value != 0 {
			return false
		}
	}
	return true
}

// parsePgIntArray decodes a PostgreSQL integer[] text literal ("{1,2,3}").
func parsePgIntArray(literal string) []int64 {
	parts := parsePgArray(literal)
	values := make([]int64, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if number, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil {
			values = append(values, number)
		}
	}
	return values
}

// parsePgArray decodes a PostgreSQL text-array literal (e.g. {a,"b,c"}) into a
// slice. lib/pq returns array columns as this literal when scanned dynamically.
func parsePgArray(literal string) []string {
	if len(literal) < 2 || literal[0] != '{' || literal[len(literal)-1] != '}' {
		return nil
	}
	body := literal[1 : len(literal)-1]
	if body == "" {
		return []string{}
	}
	var out []string
	var buf strings.Builder
	inQuote := false
	for i := 0; i < len(body); i++ {
		c := body[i]
		switch {
		case c == '"':
			if inQuote && i+1 < len(body) && body[i+1] == '"' {
				buf.WriteByte('"')
				i++
				continue
			}
			inQuote = !inQuote
		case c == '\\' && i+1 < len(body):
			buf.WriteByte(body[i+1])
			i++
		case c == ',' && !inQuote:
			out = append(out, buf.String())
			buf.Reset()
		default:
			buf.WriteByte(c)
		}
	}
	out = append(out, buf.String())
	return out
}

// containsString reports whether target appears in values.
func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
