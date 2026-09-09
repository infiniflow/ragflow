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
	"fmt"
	"reflect"
	"strings"
)

func buildFilter(condition map[string]interface{}, kind string) (string, []interface{}, error) {
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
		switch key {
		case "exists":
			column, ok := value.(string)
			if !ok {
				continue
			}
			if kind == "memory" {
				column = mapMemoryField(column)
			}
			if valid[column] {
				parts = append(parts, quoteIdentifier(column)+" IS NOT NULL")
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
			}
			if valid[column] {
				parts = append(parts, quoteIdentifier(column)+" IS NULL")
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
		default:
			if !valid[key] {
				continue
			}
			if kind == "chunk" && arrayColumns[key] {
				values, isSlice := interfaceSlice(value)
				if !isSlice {
					parts = append(parts, fmt.Sprintf("ARRAY_CONTAINS(%s, ?)", quoteIdentifier(key)))
					args = append(args, value)
					continue
				}
				arrayParts := make([]string, 0, len(values))
				for _, item := range values {
					arrayParts = append(arrayParts, fmt.Sprintf("ARRAY_CONTAINS(%s, ?)", quoteIdentifier(key)))
					args = append(args, item)
				}
				if len(arrayParts) > 0 {
					parts = append(parts, "("+strings.Join(arrayParts, " OR ")+")")
				}
				continue
			}
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
				parts = append(parts, fmt.Sprintf("%s IN (%s)", quoteIdentifier(key), strings.Join(placeholders, ", ")))
			} else {
				parts = append(parts, quoteIdentifier(key)+" = ?")
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
