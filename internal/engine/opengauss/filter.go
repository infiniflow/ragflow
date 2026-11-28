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
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"ragflow/internal/engine/types"
)

var placeholderRegex = regexp.MustCompile(`\$(\d+)`)

// shiftPlaceholders renumbers the $n placeholders of a built clause so it can
// be embedded after clauses that already consumed the first offset args.
func shiftPlaceholders(clause string, offset int) string {
	if offset == 0 {
		return clause
	}
	return placeholderRegex.ReplaceAllStringFunc(clause, func(match string) string {
		number, _ := strconv.Atoi(match[1:])
		return fmt.Sprintf("$%d", number+offset)
	})
}

// filterBuilder accumulates WHERE parts with a running $n counter; args order
// is bound to placeholder order by construction.
type filterBuilder struct {
	parts []string
	args  []interface{}
}

// raw appends a pre-built SQL fragment to the conjunction.
func (b *filterBuilder) raw(part string) {
	b.parts = append(b.parts, part)
}

// placeholder binds a value and returns its $n reference.
func (b *filterBuilder) placeholder(value interface{}) string {
	b.args = append(b.args, value)
	return fmt.Sprintf("$%d", len(b.args))
}

// join renders the accumulated parts as one AND chain, with a trivially
// true predicate for the empty case.
func (b *filterBuilder) join() string {
	if len(b.parts) == 0 {
		return "1=1"
	}
	return strings.Join(b.parts, " AND ")
}

// buildFilter binds terms, nested conditions and metadata filters to a SQL predicate.
func (e *Engine) buildFilter(condition map[string]interface{}, kind string, columns map[string]columnMeta) (string, []interface{}, error) {
	if _, ok := condition["_id"]; ok {
		return "", nil, fmt.Errorf("opengauss: _id is not a filterable column, use id")
	}
	filter := &filterBuilder{}
	for _, rawKey := range slices.Sorted(maps.Keys(condition)) {
		key := rawKey
		if key == "doc_ids" {
			key = "doc_id"
		}
		if kind == "memory" {
			key = mapMemoryField(key)
		} else if kind == "skill" && key == "id" {
			key = "skill_id"
		}
		value := condition[rawKey]
		// An empty terms filter matches no rows. False and zero remain values
		// (for example status=false and available_int=0).
		if value == nil {
			filter.raw("1=0")
			continue
		}
		if key == "or" || key == "and" {
			children := types.FilterClauses(value)
			items, ok := interfaceSlice(value)
			if !ok || len(children) != len(items) {
				return "", nil, fmt.Errorf("opengauss: %s requires a list of filter objects", key)
			}
			if len(children) == 0 {
				if key == "or" {
					filter.raw("1=0")
				}
				continue
			}
			parts := make([]string, 0, len(children))
			for _, child := range children {
				clause, args, err := e.buildFilter(child, kind, columns)
				if err != nil {
					return "", nil, err
				}
				parts = append(parts, "("+shiftPlaceholders(clause, len(filter.args))+")")
				filter.args = append(filter.args, args...)
			}
			filter.raw("(" + strings.Join(parts, " "+strings.ToUpper(key)+" ") + ")")
			continue
		}
		rv := reflect.ValueOf(value)
		if (rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array) && rv.Len() == 0 {
			filter.raw("1=0")
			continue
		}
		if kind == "memory" && key == "status_int" {
			if active, ok := value.(bool); ok {
				value = 0
				if active {
					value = 1
				}
			}
		}
		if kind != "memory" && (fieldKeyword(key) || (kind == "chunk" && chunkKeywordField(key))) {
			filter.appendKeywordTerms(key, value, columns, kind == "chunk")
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
			filter.appendExists(column, columns)
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
			meta, known := columns[column]
			if !known {
				// A column the table lacks is NULL on every row, so the
				// must_not clause holds everywhere.
				continue
			}
			quoted := quoteIdent(column)
			if isTextColumn(meta.dataType) {
				filter.raw(filter.emptyText(quoted))
			} else {
				filter.raw(quoted + " IS NULL")
			}
		case "metadata_filtering_conditions":
			if err := filter.appendMetadataFilteringConditions(value); err != nil {
				return "", nil, err
			}
		default:
			if columns != nil {
				if _, known := columns[key]; !known {
					filter.raw("1=0")
					continue
				}
			}
			if values, ok := interfaceSlice(value); ok {
				placeholders := make([]string, 0, len(values))
				for _, item := range values {
					placeholders = append(placeholders, filter.placeholder(item))
				}
				filter.raw(quoteIdent(key) + " IN (" + strings.Join(placeholders, ", ") + ")")
			} else {
				filter.raw(quoteIdent(key) + " = " + filter.placeholder(value))
			}
		}
	}
	return filter.join(), filter.args, nil
}

// appendExists renders an exists/does-not-exist filter against a column,
// treating never-stored fields as absent on every row (ES dynamic-mapping
// parity).
func (b *filterBuilder) appendExists(column string, columns map[string]columnMeta) {
	meta, known := columns[column]
	if !known {
		// ES dynamic-mapping parity: a field this table never stored exists
		// on no row.
		b.raw("1=0")
		return
	}
	quoted := quoteIdent(column)
	if isTextColumn(meta.dataType) {
		b.raw(b.nonemptyText(quoted))
		return
	}
	b.raw(quoted + " IS NOT NULL")
}

// A-mode DataVec treats an empty string literal as NULL.
func (b *filterBuilder) emptyText(column string) string {
	return "COALESCE(length(" + column + "), 0) = 0"
}

func (b *filterBuilder) nonemptyText(column string) string {
	return "length(" + column + ") > 0"
}

// likeEscaper neutralizes LIKE metacharacters so a bound value matches
// literally; backslash is the default LIKE escape character.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// appendKeywordTerms matches whole elements of the stored keyword list.
func (b *filterBuilder) appendKeywordTerms(column string, value interface{}, columns map[string]columnMeta, arrayEncoded bool) {
	if columns != nil {
		if _, known := columns[column]; !known {
			// ES dynamic-mapping parity: a field this table never stored exists
			// on no row.
			b.raw("1=0")
			return
		}
	}
	values := []interface{}{value}
	if list, ok := interfaceSlice(value); ok {
		values = list
	}
	padded := "('" + keywordSeparator + "' || " + quoteIdent(column) + " || '" + keywordSeparator + "')"
	parts := make([]string, 0, len(values))
	for _, item := range values {
		text := stringValue(item)
		if text == "" {
			continue
		}
		if arrayEncoded {
			parts = append(parts, b.placeholder(text)+" = ANY("+keywordArraySQL(column)+")")
			continue
		}
		pattern := "%" + keywordSeparator + likeEscaper.Replace(text) + keywordSeparator + "%"
		parts = append(parts, padded+" LIKE "+b.placeholder(pattern))
	}
	if len(parts) == 0 {
		// Only empty-string values remain. The decode side drops empty
		// elements, so no stored row can ever contain one.
		b.raw("1=0")
		return
	}
	b.raw("(" + strings.Join(parts, " OR ") + ")")
}

// metadataJSON reads the metadata text column as jsonb, mapping rows the
// writer left at DEFAULT ” to an empty object so the cast never fails the
// whole query.
const metadataJSON = `CASE WHEN "metadata" LIKE '{%' THEN "metadata"::jsonb ELSE '{}'::jsonb END`

// metadataText builds the jsonb string extraction for one metadata key.
func (b *filterBuilder) metadataText(path string) string {
	return "(" + metadataJSON + ") ->> " + b.placeholder(path)
}

// metadataCast wraps the extracted text in a guarded cast: rows whose value
// does not parse as the target type yield NULL (no match) instead of failing
// the query, matching the silent-coercion behavior of the MySQL JSON path.
func (b *filterBuilder) metadataCast(path, castType, pattern string) string {
	text := b.metadataText(path)
	return "(CASE WHEN " + text + " ~ '" + pattern + "' THEN (" + text + ")::" + castType + " ELSE NULL END)"
}

// Validate the calendar day against the last day of its month before casting.
func (b *filterBuilder) metadataTimestamp(path string) string {
	text := "(" + b.metadataText(path) + ")"
	const shape = "^[0-9]{4}-(0[1-9]|1[0-2])-(0[1-9]|[12][0-9]|3[01])([ T]([01][0-9]|2[0-3]):[0-5][0-9](:[0-5][0-9])?)?$"
	return "(CASE WHEN " + text + " ~ '" + shape + "' AND left(" + text + ",4) <> '0000' THEN CASE WHEN substring(" +
		text + ",9,2)::int <= extract(day from last_day((left(" + text + ",7) || '-01')::date)) THEN " +
		text + "::timestamp END END)"
}

// appendMetadataFilteringConditions adds the post-filter conditions from
// the metadata filtering option, joined as a conjunction.
func (b *filterBuilder) appendMetadataFilteringConditions(raw interface{}) error {
	filter, ok := raw.(map[string]interface{})
	if !ok {
		return nil
	}
	conditions, ok := interfaceSlice(filter["conditions"])
	if !ok || len(conditions) == 0 {
		return nil
	}
	logic := strings.ToUpper(stringValue(filter["logical_operator"]))
	if logic == "" {
		logic = "AND"
	}
	if logic != "AND" && logic != "OR" {
		return fmt.Errorf("opengauss: unsupported metadata logical operator: %s", logic)
	}
	parts := make([]string, 0, len(conditions))
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
		// A plain top-level key, not a MySQL "$.name" JSON path: the jsonb
		// ->>/-> text operand is a literal key lookup, and no metadata object
		// carries a key literally named "$.name".
		path := name
		value := condition["value"]
		switch operator {
		case "is", "is not":
			comparison := "="
			if operator == "is not" {
				comparison = "<>"
			}
			parts = append(parts, b.metadataText(path)+" "+comparison+" "+b.placeholder(value))
		case "=", "≠", ">", "<", "≥", "≤":
			comparison := map[string]string{"=": "=", "≠": "<>", ">": ">", "<": "<", "≥": ">=", "≤": "<="}[operator]
			parts = append(parts, b.metadataCast(path, "numeric", `^-?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?$`)+
				" "+comparison+" "+b.placeholder(value))
		case "before", "after":
			comparison := "<"
			if operator == "after" {
				comparison = ">"
			}
			parts = append(parts, b.metadataTimestamp(path)+
				" "+comparison+" "+b.placeholder(value))
		case "contains", "not contains":
			encoded, err := json.Marshal(value)
			if err != nil {
				continue
			}
			part := "(" + metadataJSON + ") -> " + b.placeholder(path) + " @> " + b.placeholder(string(encoded)) + "::jsonb"
			if operator == "not contains" {
				part = "NOT " + part
			}
			parts = append(parts, part)
		case "start with":
			parts = append(parts, b.metadataText(path)+" LIKE "+b.placeholder(likeEscaper.Replace(stringValue(value))+"%"))
		case "end with":
			parts = append(parts, b.metadataText(path)+" LIKE "+b.placeholder("%"+likeEscaper.Replace(stringValue(value))))
		case "empty", "not empty":
			text := b.metadataText(path)
			empty := "(" + b.emptyText(text) + " OR " + text + " = '[]' OR " + text + " = '{}')"
			if operator == "not empty" {
				empty = "(" + b.nonemptyText(text) + " AND " + text + " != '[]' AND " + text + " != '{}')"
			}
			parts = append(parts, empty)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	b.raw("(" + strings.Join(parts, " "+logic+" ") + ")")
	return nil
}
