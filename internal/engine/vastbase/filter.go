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
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
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

func (b *filterBuilder) raw(part string) {
	b.parts = append(b.parts, part)
}

func (b *filterBuilder) placeholder(value interface{}) string {
	b.args = append(b.args, value)
	return fmt.Sprintf("$%d", len(b.args))
}

func (b *filterBuilder) join() string {
	if len(b.parts) == 0 {
		return "1=1"
	}
	return strings.Join(b.parts, " AND ")
}

// buildFilter translates an ES-style condition map into a parameterized WHERE
// clause, porting the Python connector's equivalent_condition_to_str:
//
//   - falsy values are dropped (the Python code skipped them too),
//   - keyword columns (source_id, *_kwd except docnm_kwd/knowledge_graph_kwd)
//     store ###-joined lists, so their term/terms conditions become LIKE
//     containment tests on the separator-padded column. Dropping them instead
//     would collapse a write filter to its dataset scope and let an update or
//     delete hit every row of the dataset,
//   - a column missing from the live schema matches no row (1=0), like an
//     unmapped ES field; a nil columns map means "unknown schema" and allows
//     every column,
//   - exists on a text column excludes DEFAULT ” rows, because unwritten text
//     columns hold ” rather than NULL; must_not.exists is the mirror image.
//
// Memory tables skip the keyword translation: their keyword-named columns
// (message_type_kwd, source_id) store plain scalars.
//
// Unlike the Python connector, kb_id is NOT skipped: Go tables are shared per
// baseName with kb_id/memory_id as row-level dataset discriminators, so the
// scope condition must reach the SQL.
func buildFilter(condition map[string]interface{}, kind string, columns map[string]columnMeta) (string, []interface{}, error) {
	if _, ok := condition["_id"]; ok {
		return "", nil, fmt.Errorf("vastbase: _id is not a filterable column, use id")
	}
	filter := &filterBuilder{}
	for _, rawKey := range sortedKeys(condition) {
		key := rawKey
		if kind == "memory" {
			key = mapMemoryField(key)
		} else if kind == "skill" && key == "id" {
			key = "skill_id"
		}
		value := condition[rawKey]
		if isEmptyFilterValue(value) {
			continue
		}
		if fieldKeyword(key) && kind != "memory" {
			filter.appendKeywordTerms(key, value, columns)
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
				filter.raw("(" + quoted + " IS NULL OR " + quoted + " = '')")
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
		b.raw("(" + quoted + " IS NOT NULL AND " + quoted + " <> '')")
		return
	}
	b.raw(quoted + " IS NOT NULL")
}

// likeEscaper neutralizes LIKE metacharacters so a bound value matches
// literally; backslash is the default LIKE escape character.
var likeEscaper = strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)

// appendKeywordTerms translates a term/terms condition on a ###-joined keyword
// column into LIKE containment tests: padding the column with separators makes
// first and last list elements match and prevents substring false positives
// ("nav_doc" must not match a stored "nav_doc_2" element). A terms list ORs
// one test per value, matching the ES any-of semantics. The pattern is bound
// as a parameter, so only the value's own LIKE metacharacters need escaping.
func (b *filterBuilder) appendKeywordTerms(column string, value interface{}, columns map[string]columnMeta) {
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
		return fmt.Errorf("vastbase: unsupported metadata logical operator: %s", logic)
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
			parts = append(parts, b.metadataCast(path, "timestamp", `^[0-9]{4}-[0-9]{2}-[0-9]{2}([ T][0-9]{2}:[0-9]{2}(:[0-9]{2})?)?$`)+
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
			parts = append(parts, b.metadataText(path)+" LIKE "+b.placeholder(stringValue(value)+"%"))
		case "end with":
			parts = append(parts, b.metadataText(path)+" LIKE "+b.placeholder("%"+stringValue(value)))
		case "empty", "not empty":
			text := b.metadataText(path)
			empty := "(" + text + " IS NULL OR " + text + " = '' OR " + text + " = '[]' OR " + text + " = '{}')"
			if operator == "not empty" {
				empty = "(" + text + " IS NOT NULL AND " + text + " != '' AND " + text + " != '[]' AND " + text + " != '{}')"
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
	// Python truthiness also drops numeric zero: the connector's
	// `if ... or not v: continue` skipped 0 and 0.0 conditions.
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return rv.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return rv.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return rv.Float() == 0
	}
	return false
}

func sortedKeys(source map[string]interface{}) []string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
