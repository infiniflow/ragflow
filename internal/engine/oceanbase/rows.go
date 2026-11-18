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
	"strconv"
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
				// Keep the column so decoding can tell a NULL column from one
				// the table does not have. Decoding drops the key again, but
				// the presence still blocks a stale value in extra.
				row[column] = nil
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

func decodeLogicalRow(row map[string]interface{}, kind string, extra *chunkExtraSelection) map[string]interface{} {
	result := make(map[string]interface{}, len(row))
	if kind == "memory" {
		result["forget_at"] = nil
	}
	nullColumns := make([]string, 0, len(row))
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
		if kind == "memory" && column == "message_id" {
			if text, ok := value.(string); ok {
				if id, err := strconv.ParseInt(text, 10, 64); err == nil {
					value = id
				}
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
		storedColumn := rawColumn
		if value == nil {
			nullColumns = append(nullColumns, column)
		}
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
	if kind == "chunk" {
		if extra == nil {
			// SELECT * carries every physical column, so a field the row lacks
			// is one this table stores in extra.
			expandChunkExtra(result)
		} else {
			liftChunkExtra(result, extra.fields)
			if !extra.keepExtra {
				delete(result, "extra")
			}
		}
	}
	// A NULL column stays absent from the decoded row, as it always was, but it
	// still counted as present for the extra expansion above.
	for _, column := range nullColumns {
		if kind == "memory" && column == "forget_at" {
			continue
		}
		delete(result, column)
	}
	return result
}

// chunkExtraSelection describes a projection that reads chunk fields the table
// stores in its extra JSON column. fields names the declared columns to lift;
// keepExtra retains the extra object itself because the caller asked for it.
type chunkExtraSelection struct {
	fields    map[string]bool
	keepExtra bool
}

// liftChunkExtra copies the requested fields out of the decoded extra object.
// The projection reads extra as JSON rather than JSON_UNQUOTE(JSON_EXTRACT(...)),
// so a stored integer stays a JSON number and a JSON null stays nil instead of
// turning into the string "null".
func liftChunkExtra(row map[string]interface{}, fields map[string]bool) {
	if len(fields) == 0 {
		return
	}
	extra, ok := row["extra"].(map[string]interface{})
	if !ok {
		return
	}
	for field := range fields {
		value, ok := extra[field]
		if !ok {
			continue
		}
		if _, present := row[field]; present {
			continue
		}
		row[field] = value
	}
}

// expandChunkExtra lifts declared fields that an existing chunk table stores
// in its extra JSON column back onto the chunk document. A field the table has
// as a column wins: the row then carries the physical value, which is the
// current one.
func expandChunkExtra(row map[string]interface{}) {
	extra, ok := row["extra"].(map[string]interface{})
	if !ok {
		return
	}
	for column, value := range extra {
		if !knownChunkColumns[column] {
			continue
		}
		logical := column
		if logical == "_order_id" {
			logical = "chunk_order_int"
		}
		if _, present := row[logical]; present {
			continue
		}
		row[logical] = value
	}
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

func parseSelectField(field, kind string, tableColumns map[string]bool) (*selectField, error) {
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
	// Only columns the engine added after the first chunk schema can be absent
	// from an existing table; those are read from extra instead. The projection
	// selects the extra object once and decoding lifts the requested fields, so
	// the value keeps its stored JSON type.
	if kind == "chunk" && optionalChunkColumns[column] && tableColumns != nil && !tableColumns[column] {
		return &selectField{column: "extra", alias: "extra"}, nil
	}
	if !vectorColumnPattern.MatchString(column) && !validColumns(kind)[column] {
		return nil, fmt.Errorf("unknown %s field: %s", kind, field)
	}
	return &selectField{column: column, alias: field}, nil
}

func selectExpression(field, kind string) (string, string, error) {
	parsed, err := parseSelectField(field, kind, nil)
	if err != nil || parsed == nil {
		return "", "", err
	}
	return parsed.expression(""), parsed.alias, nil
}

func buildSelectFields(fields []string, kind string) (string, []string, error) {
	return buildSelectFieldsWithAlias(fields, kind, "", nil)
}

func buildQualifiedSelectFields(fields []string, kind, tableAlias string) (string, []string, error) {
	return buildSelectFieldsWithAlias(fields, kind, tableAlias, nil)
}

// chunkSelectionNeedsSchema reports whether an explicit chunk projection names
// a column an existing table may lack. Only then is the table's column set
// worth a metadata read; the default projection and base columns are not.
// A wildcard projection already reads every physical column, and an extra
// request next to it ("*", "compile_kwd") must not turn the row into a partial
// projection that drops the rest of extra.
func chunkSelectionNeedsSchema(fields []string) bool {
	if containsString(fields, "*") {
		return false
	}
	for _, field := range fields {
		if field == "" || field == "*" {
			continue
		}
		if optionalChunkColumns[mapChunkField(field)] {
			return true
		}
	}
	return false
}

// chunkFieldsFromExtra names the requested chunk fields the table stores in
// its extra JSON column: optional columns the live schema lacks.
func chunkFieldsFromExtra(fields []string, tableColumns map[string]bool) map[string]bool {
	lifted := make(map[string]bool)
	for _, field := range fields {
		if field == "" || field == "*" {
			continue
		}
		column := mapChunkField(field)
		if optionalChunkColumns[column] && !tableColumns[column] {
			lifted[column] = true
		}
	}
	return lifted
}

func buildSelectFieldsWithAlias(fields []string, kind, tableAlias string, tableColumns map[string]bool) (string, []string, error) {
	if len(fields) == 0 || containsString(fields, "*") {
		// Chunk columns grow additively on existing tables, so a static default
		// list breaks a read that runs before CreateChunkStore. SELECT * follows
		// the table's real schema.
		if kind == "chunk" {
			prefix := ""
			if tableAlias != "" {
				prefix = quoteIdentifier(tableAlias) + "."
			}
			return prefix + "*", []string{"*"}, nil
		}
		fields = defaultFields(kind)
	}
	if !containsString(fields, identifierField(kind)) {
		fields = append([]string{identifierField(kind)}, fields...)
	}
	expressions := make([]string, 0, len(fields))
	aliases := make([]string, 0, len(fields))
	seen := make(map[string]bool)
	for _, field := range fields {
		parsed, err := parseSelectField(field, kind, tableColumns)
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
