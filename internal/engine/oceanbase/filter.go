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
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// chunkFilterUsesExtra takes a nil schema when the caller is deciding whether
// to look it up. In that case every optional field is potentially in extra.
func chunkFilterUsesExtra(condition map[string]interface{}, tableColumns map[string]bool) bool {
	for key, value := range condition {
		if isEmptyFilterValue(value) {
			continue
		}
		switch key {
		case "exists":
			key, _ = value.(string)
		case "must_not":
			object, _ := value.(map[string]interface{})
			key, _ = object["exists"].(string)
		}
		key = mapChunkField(key)
		if isOptionalChunkColumn(key) && !tableColumns[key] {
			return true
		}
	}
	return false
}

func chunkFieldInExtra(column string, tableColumns map[string]bool) bool {
	return tableColumns != nil && isOptionalChunkColumn(column) && !tableColumns[column]
}

func chunkExtraPath(column string) string {
	// The caller has already checked that column is a declared optional field.
	return "$." + column
}

func chunkExtraExtract(column string) string {
	return "JSON_EXTRACT(`extra`, '" + chunkExtraPath(column) + "')"
}

// filterColumn renders a column reference for a predicate or sort. Value
// comparisons decode JSON scalars. Existence checks only need SQL NULL.
func (schema chunkTableSchema) filterColumn(kind, column string, valueComparison bool) string {
	if kind != "chunk" || !chunkFieldInExtra(column, schema.columns) {
		return quoteIdentifier(column)
	}
	extracted := chunkExtraExtract(column)
	jsonType := "JSON_TYPE(" + extracted + ")"
	value := extracted
	if valueComparison && !jsonColumns[column] {
		value = "JSON_UNQUOTE(" + extracted + ")"
		if strings.HasPrefix(optionalChunkColumnTypes[column], "INTEGER ") {
			// SQL comparisons coerce text like "invalid" to zero. A numeric
			// condition must not match such rows in an update or delete.
			return "CASE WHEN " + jsonType + " IN ('INTEGER', 'DOUBLE', 'DECIMAL') THEN CAST(" + value + " AS DOUBLE) END"
		}
		return "(CASE WHEN " + jsonType + " <> 'NULL' THEN " + value + " END) COLLATE " + schema.collation
	}
	return "CASE WHEN " + jsonType + " <> 'NULL' THEN " + value + " END"
}

func buildFilter(condition map[string]interface{}, kind string, schema chunkTableSchema) (string, []interface{}, error) {
	valid := validColumns(kind)
	parts := make([]string, 0, len(condition))
	args := make([]interface{}, 0, len(condition))
	for rawKey, value := range condition {
		key := rawKey
		if kind == "memory" {
			key = mapMemoryField(key)
		} else if kind == "chunk" {
			key = mapChunkField(key)
		} else if kind == "skill" && key == "id" {
			key = "skill_id"
		} else if kind == "metadata" && key == "doc_id" {
			key = "id"
		}
		if isEmptyFilterValue(value) {
			continue
		}
		if kind == "memory" && key == "session_id_contains" {
			parts = append(parts, "INSTR(`session_id`, ?) > 0")
			args = append(args, value)
			continue
		}
		switch key {
		case "exists":
			column, ok := value.(string)
			if !ok {
				continue
			}
			if kind == "memory" {
				column = mapMemoryField(column)
			} else if kind == "chunk" {
				column = mapChunkField(column)
			}
			if valid[column] {
				parts = append(parts, schema.filterColumn(kind, column, false)+" IS NOT NULL")
			}
		case "must_not":
			object, ok := value.(map[string]interface{})
			if !ok {
				continue
			}
			column, ok := object["exists"].(string)
			if !ok {
				continue
			}
			if kind == "memory" {
				column = mapMemoryField(column)
			} else if kind == "chunk" {
				column = mapChunkField(column)
			}
			if valid[column] {
				parts = append(parts, schema.filterColumn(kind, column, false)+" IS NULL")
			}
		case "metadata_filtering_conditions":
			part, partArgs, err := buildMetadataFilter(value)
			if err != nil {
				return "", nil, err
			}
			if part != "" {
				parts = append(parts, part)
				args = append(args, partArgs...)
			}
		case "knowledge_graph_kwd":
			if kind != "chunk" {
				continue
			}
			values, isSlice := interfaceSlice(value)
			if !isSlice {
				values = []interface{}{value}
			}
			if len(values) == 0 {
				continue
			}
			placeholders := make([]string, len(values))
			for i := range values {
				placeholders[i] = "?"
			}
			predicate := "= ?"
			if len(values) > 1 {
				predicate = "IN (" + strings.Join(placeholders, ", ") + ")"
			}
			parts = append(parts, fmt.Sprintf("(%s %s OR %s %s)",
				quoteIdentifier("type_kwd"), predicate, quoteIdentifier("knowledge_graph_kwd"), predicate))
			args = append(args, values...)
			args = append(args, values...)
		default:
			if !valid[key] {
				continue
			}
			if kind == "chunk" && arrayColumns[key] {
				values, isSlice := interfaceSlice(value)
				if !isSlice {
					values = []interface{}{value}
				}
				arrayParts := make([]string, 0, len(values))
				for _, item := range values {
					if chunkFieldInExtra(key, schema.columns) {
						text, ok := item.(string)
						if !ok {
							return "", nil, fmt.Errorf("filter %s requires string array elements", key)
						}
						encoded, _ := json.Marshal(text)
						extracted := chunkExtraExtract(key)
						arrayParts = append(arrayParts, "(JSON_TYPE("+extracted+") = 'ARRAY' AND JSON_CONTAINS("+extracted+", ?))")
						item = string(encoded)
					} else {
						arrayParts = append(arrayParts, fmt.Sprintf("ARRAY_CONTAINS(%s, ?)", quoteIdentifier(key)))
					}
					args = append(args, item)
				}
				if len(arrayParts) == 1 {
					parts = append(parts, arrayParts[0])
				} else if len(arrayParts) > 1 {
					parts = append(parts, "("+strings.Join(arrayParts, " OR ")+")")
				}
				continue
			}
			expression := schema.filterColumn(kind, key, true)
			values, isSlice := interfaceSlice(value)
			if isSlice {
				if len(values) == 0 {
					continue
				}
				placeholders := make([]string, len(values))
				for i, item := range values {
					placeholders[i] = "?"
					args = append(args, item)
				}
				parts = append(parts, fmt.Sprintf("%s IN (%s)", expression, strings.Join(placeholders, ", ")))
			} else {
				parts = append(parts, expression+" = ?")
				args = append(args, value)
			}
		}
	}
	if len(parts) == 0 {
		return "1=1", args, nil
	}
	return strings.Join(parts, " AND "), args, nil
}

func buildMetadataFilter(raw interface{}) (string, []interface{}, error) {
	filter, ok := raw.(map[string]interface{})
	if !ok {
		return "", nil, nil
	}
	conditions, ok := interfaceSlice(filter["conditions"])
	if !ok || len(conditions) == 0 {
		return "", nil, nil
	}
	logic := strings.ToUpper(stringValue(filter["logical_operator"]))
	if logic == "" {
		logic = "AND"
	}
	if logic != "AND" && logic != "OR" {
		return "", nil, fmt.Errorf("unsupported metadata logical operator: %s", logic)
	}

	parts := make([]string, 0, len(conditions))
	args := make([]interface{}, 0, len(conditions)*2)
	for _, item := range conditions {
		condition, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		name := stringValue(condition["name"])
		operator := stringValue(condition["comparison_operator"])
		if name == "" || operator == "" || strings.ContainsAny(name, `"\\`) {
			continue
		}
		path := "$." + name
		expression := "JSON_EXTRACT(`metadata`, ?)"
		value := condition["value"]
		switch operator {
		case "is", "is not", "=", "≠", ">", "<", "≥", "≤", "before", "after":
			left := expression
			comparison := operator
			switch operator {
			case "is":
				comparison = "="
			case "is not":
				comparison = "!="
			case "=", "≠", ">", "<", "≥", "≤":
				left = "CAST(" + expression + " AS DECIMAL(20,10))"
				comparison = map[string]string{"=": "=", "≠": "!=", ">": ">", "<": "<", "≥": ">=", "≤": "<="}[operator]
			case "before":
				left = "CAST(" + expression + " AS DATETIME)"
				comparison = "<"
			case "after":
				left = "CAST(" + expression + " AS DATETIME)"
				comparison = ">"
			}
			parts = append(parts, left+" "+comparison+" ?")
			args = append(args, path, value)
		case "contains", "not contains":
			prefix := ""
			if operator == "not contains" {
				prefix = "NOT "
			}
			parts = append(parts, prefix+"JSON_CONTAINS("+expression+", ?)")
			args = append(args, path, value)
		case "start with":
			parts = append(parts, expression+" LIKE CONCAT(?, '%')")
			args = append(args, path, value)
		case "end with":
			parts = append(parts, expression+" LIKE CONCAT('%', ?)")
			args = append(args, path, value)
		case "empty":
			parts = append(parts, "("+expression+" IS NULL OR "+expression+" = '' OR "+expression+" = '[]' OR "+expression+" = '{}')")
			args = append(args, path, path, path, path)
		case "not empty":
			parts = append(parts, "("+expression+" IS NOT NULL AND "+expression+" != '' AND "+expression+" != '[]' AND "+expression+" != '{}')")
			args = append(args, path, path, path, path)
		}
	}
	if len(parts) == 0 {
		return "", nil, nil
	}
	return "(" + strings.Join(parts, " "+logic+" ") + ")", args, nil
}

func validColumns(kind string) map[string]bool {
	result := map[string]bool{"_score": true}
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
	for _, column := range columns {
		result[column.name] = true
	}
	return result
}

func isEmptyFilterValue(value interface{}) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.String, reflect.Array, reflect.Slice, reflect.Map:
		return rv.Len() == 0
	case reflect.Bool:
		return !rv.Bool()
	}
	return false
}
