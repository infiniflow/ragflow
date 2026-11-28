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

package opengauss

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"ragflow/internal/common"

	pq "gitcode.com/opengauss/openGauss-connector-go-pq"
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

// decodeLogicalRow maps database columns to chunk, skill or memory fields.
func (e *Engine) decodeLogicalRow(row map[string]interface{}, kind, preferredVector string) map[string]interface{} {
	result := make(map[string]interface{}, len(row))
	// Without a requested dimension prefer a populated nonzero vector, then column name.
	var selectedVector string
	var selectedNonzero bool
	for rawColumn, value := range row {
		column := rawColumn
		if kind == "memory" {
			if vectorColumnPattern.MatchString(column) {
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
		if text, ok := value.(string); ok && kind == "chunk" && chunkKeywordField(rawColumn) {
			if strings.HasPrefix(text, keywordArrayPrefix+"{") {
				var keywords pq.StringArray
				if err := keywords.Scan(strings.TrimPrefix(text, keywordArrayPrefix)); err == nil {
					result[column] = []string(keywords)
					continue
				}
			}
			result[column] = splitKeywords(text)
			continue
		}
		value = decodeColumnValue(kind, rawColumn, value)
		if kind == "memory" && column == "content_embed" {
			vector, _ := value.([]float64)
			nonzero := !common.IsZeroVector(vector)
			if selectedVector != "" && rawColumn != preferredVector &&
				(selectedVector == preferredVector || (selectedNonzero && !nonzero) || (nonzero == selectedNonzero && rawColumn > selectedVector)) {
				continue
			}
			selectedVector, selectedNonzero = rawColumn, nonzero
		}
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
	// scalars.
	case fieldKeyword(column) && kind != "memory":
		if isText {
			return splitKeywords(text)
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

func splitKeywords(text string) []string {
	parts := strings.Split(text, keywordSeparator)
	keywords := make([]string, 0, len(parts))
	for _, part := range parts {
		if part != "" {
			keywords = append(keywords, part)
		}
	}
	return keywords
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

// parseFloatVectorText decodes "[0.1,0.2,...]" vector text.
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

// parsePgIntArray decodes the integer array returned by the connector.
func parsePgIntArray(literal string) []int64 {
	var values pq.Int64Array
	if err := values.Scan(literal); err != nil {
		return nil
	}
	return []int64(values)
}
