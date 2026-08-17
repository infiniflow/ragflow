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

package oceanbase

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

func (e *Engine) queryRows(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error) {
	rows, err := e.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]interface{}, 0)
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

func decodeLogicalRow(row map[string]interface{}, kind string) map[string]interface{} {
	result := make(map[string]interface{}, len(row))
	for rawColumn, value := range row {
		column := rawColumn
		if kind == "memory" {
			if vectorColumnPattern.MatchString(column) {
				column = "content_embed"
			} else if mapped, ok := memoryColumnToField[column]; ok {
				column = mapped
			}
		}
		if kind == "chunk" && column == "_order_id" {
			column = "chunk_order_int"
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
		storedColumn := rawColumn
		if (kind == "chunk" && arrayColumns[storedColumn]) || jsonColumns[storedColumn] || vectorColumnPattern.MatchString(storedColumn) {
			if text, ok := value.(string); ok {
				var decoded interface{}
				if err := json.Unmarshal([]byte(text), &decoded); err == nil {
					value = decoded
				}
			}
		}
		result[column] = value
	}
	return result
}

func scanCount(row *sql.Row) (int64, error) {
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

type selectField struct {
	column string
	alias  string
}

func (field selectField) expression(tableAlias string) string {
	prefix := ""
	if tableAlias != "" {
		prefix = quoteIdentifier(tableAlias) + "."
	}
	expression := prefix + quoteIdentifier(field.column)
	if field.alias != field.column {
		expression += " AS " + quoteIdentifier(field.alias)
	}
	return expression
}

func parseSelectField(field, kind string) (*selectField, error) {
	if field == "_score" {
		return nil, nil
	}
	if field == "row_id()" || field == "row_id" {
		return &selectField{column: identifierField(kind), alias: "row_id"}, nil
	}
	column := field
	if kind == "memory" {
		column = mapMemoryField(field)
	}
	if kind == "chunk" && field == "chunk_order_int" {
		column = "_order_id"
	}
	if field == "content_embed" && kind == "memory" {
		return nil, nil
	}
	if !vectorColumnPattern.MatchString(column) && !validColumns(kind)[column] {
		return nil, fmt.Errorf("unknown %s field: %s", kind, field)
	}
	return &selectField{column: column, alias: field}, nil
}

func selectExpression(field, kind string) (string, string, error) {
	parsed, err := parseSelectField(field, kind)
	if err != nil || parsed == nil {
		return "", "", err
	}
	return parsed.expression(""), parsed.alias, nil
}

func buildSelectFields(fields []string, kind string) (string, []string, error) {
	return buildSelectFieldsWithAlias(fields, kind, "")
}

func buildQualifiedSelectFields(fields []string, kind, tableAlias string) (string, []string, error) {
	return buildSelectFieldsWithAlias(fields, kind, tableAlias)
}

func buildSelectFieldsWithAlias(fields []string, kind, tableAlias string) (string, []string, error) {
	if len(fields) == 0 || containsString(fields, "*") {
		fields = defaultFields(kind)
	}
	if !containsString(fields, identifierField(kind)) {
		fields = append([]string{identifierField(kind)}, fields...)
	}
	expressions := make([]string, 0, len(fields))
	aliases := make([]string, 0, len(fields))
	seen := make(map[string]bool)
	for _, field := range fields {
		parsed, err := parseSelectField(field, kind)
		if err != nil {
			return "", nil, err
		}
		if parsed == nil || seen[parsed.alias] {
			continue
		}
		seen[parsed.alias] = true
		expressions = append(expressions, parsed.expression(tableAlias))
		aliases = append(aliases, parsed.alias)
	}
	return strings.Join(expressions, ", "), aliases, nil
}

func defaultFields(kind string) []string {
	var columns []columnDefinition
	switch kind {
	case "memory":
		columns = memoryColumns
	case "metadata":
		columns = metadataColumns
	case "skill":
		columns = skillColumns
	default:
		columns = chunkColumns
	}
	fields := make([]string, 0, len(columns))
	for _, column := range columns {
		field := column.name
		if kind == "memory" {
			if mapped, ok := memoryColumnToField[field]; ok {
				field = mapped
			}
		}
		if kind == "chunk" && field == "_order_id" {
			field = "chunk_order_int"
		}
		fields = append(fields, field)
	}
	return fields
}

func identifierField(kind string) string {
	if kind == "skill" {
		return "skill_id"
	}
	return "id"
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
