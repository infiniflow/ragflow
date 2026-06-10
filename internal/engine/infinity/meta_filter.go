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

package infinity

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	infinity "github.com/infiniflow/infinity-go-sdk"
)

// Key pattern for validation
var keyPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)

// Supported operators
var supportedOperators = map[string]bool{
	"=":          true,
	"≠":          true,
	">":          true,
	"<":          true,
	"≥":          true,
	"≤":          true,
	"in":         true,
	"not in":     true,
	"contains":   true,
	"not contains": true,
	"start with": true,
	"end with":   true,
	"empty":      true,
	"not empty":  true,
}

// Range operators mapping
var rangeOps = map[string]string{
	">":  ">",
	"<":  "<",
	"≥":  ">=",
	"≤":  "<=",
}

// MetaFilterTranslator translates filter clauses to Infinity SQL
type MetaFilterTranslator struct{}

// NewMetaFilterTranslator creates a new translator
func NewMetaFilterTranslator() *MetaFilterTranslator {
	return &MetaFilterTranslator{}
}

// Translate translates a single filter dict into Infinity SQL filter string
func (t *MetaFilterTranslator) Translate(flt map[string]interface{}) (string, error) {
	op, _ := flt["op"].(string)
	key, _ := flt["key"].(string)
	value := flt["value"]

	if key == "" {
		return "", fmt.Errorf("filter is missing a string key")
	}
	if !keyPattern.MatchString(key) {
		return "", fmt.Errorf("invalid key format (must be identifier-like)")
	}
	if !supportedOperators[op] {
		return "", fmt.Errorf("unknown operator %q", op)
	}

	switch op {
	case "empty":
		return t.translateEmpty(key), nil
	case "not empty":
		return t.translateNotEmpty(key), nil
	case "=":
		return t.translateEqual(key, value, flt), nil
	case "≠":
		return t.translateNotEqual(key, value, flt), nil
	case ">", "<", "≥", "≤":
		return t.translateRange(key, op, value, flt), nil
	case "in":
		return t.translateIn(key, value, flt), nil
	case "not in":
		return t.translateNotIn(key, value, flt), nil
	case "contains":
		return t.translateContains(key, value, flt)
	case "not contains":
		return t.translateNotContains(key, value, flt), nil
	case "start with":
		return t.translateStartWith(key, value, flt), nil
	case "end with":
		return t.translateEndWith(key, value, flt), nil
	}

	return "", fmt.Errorf("no handler for operator %q", op)
}

func (t *MetaFilterTranslator) translateEmpty(key string) string {
	return fmt.Sprintf("JSON_EXTRACT_STRING(meta_fields, '$.%s') = '\"\"'", key)
}

func (t *MetaFilterTranslator) translateNotEmpty(key string) string {
	return fmt.Sprintf("JSON_EXTRACT_STRING(meta_fields, '$.%s') != '\"\"'", key)
}

func (t *MetaFilterTranslator) translateEqual(key string, value interface{}, flt map[string]interface{}) string {
	coerced := coerceScalar(value, flt)
	if s, ok := coerced.(string); ok {
		escaped := escapeSQLString(s)
		return fmt.Sprintf("JSON_CONTAINS(meta_fields, '$.%s', '\"%s\"')", key, escaped)
	}
	return fmt.Sprintf("JSON_CONTAINS(meta_fields, '$.%s', %v)", key, coerced)
}

func (t *MetaFilterTranslator) translateNotEqual(key string, value interface{}, flt map[string]interface{}) string {
	coerced := coerceScalar(value, flt)
	if s, ok := coerced.(string); ok {
		escaped := escapeSQLString(s)
		return fmt.Sprintf("NOT JSON_CONTAINS(meta_fields, '$.%s', '\"%s\"')", key, escaped)
	}
	return fmt.Sprintf("NOT JSON_CONTAINS(meta_fields, '$.%s', %v)", key, coerced)
}

func (t *MetaFilterTranslator) translateRange(key string, op string, value interface{}, flt map[string]interface{}) string {
	coerced := coerceRangeValue(value, flt)
	sqlOp := rangeOps[op]
	if s, ok := coerced.(string); ok {
		escaped := escapeSQLString(s)
		return fmt.Sprintf("JSON_EXTRACT_STRING(meta_fields, '$.%s') %s '%s'", key, sqlOp, escaped)
	}
	return fmt.Sprintf("JSON_EXTRACT_DOUBLE(meta_fields, '$.%s') %s %v", key, sqlOp, coerced)
}

func (t *MetaFilterTranslator) translateIn(key string, value interface{}, flt map[string]interface{}) string {
	members := csvOrList(value, flt)

	var stringParts, numParts []string
	for _, m := range members {
		coerced := coerceRangeValue(m, flt)
		if num, ok := coerceToFloat(coerced); ok {
			numParts = append(numParts, fmt.Sprintf("JSON_CONTAINS(meta_fields, '$.%s', %v)", key, num))
		} else 		if s, ok := coerced.(string); ok {
			escaped := escapeSQLString(s)
			stringParts = append(stringParts, fmt.Sprintf("JSON_CONTAINS(meta_fields, '$.%s', '\"%s\"')", key, escaped))
		}
	}

	var conditions []string
	if len(stringParts) > 0 {
		conditions = append(conditions, "("+strings.Join(stringParts, " OR ")+")")
	}
	if len(numParts) > 0 {
		conditions = append(conditions, "("+strings.Join(numParts, " OR ")+")")
	}

	return "(" + strings.Join(conditions, " OR ") + ")"
}

func (t *MetaFilterTranslator) translateNotIn(key string, value interface{}, flt map[string]interface{}) string {
	members := csvOrList(value, flt)

	var stringParts, numParts []string
	for _, m := range members {
		coerced := coerceRangeValue(m, flt)
		if num, ok := coerceToFloat(coerced); ok {
			numParts = append(numParts, fmt.Sprintf("NOT JSON_CONTAINS(meta_fields, '$.%s', %v)", key, num))
		} else if s, ok := coerced.(string); ok {
			escaped := escapeSQLString(s)
			stringParts = append(stringParts, fmt.Sprintf("NOT JSON_CONTAINS(meta_fields, '$.%s', '\"%s\"')", key, escaped))
		}
	}

	var conditions []string
	if len(stringParts) > 0 {
		conditions = append(conditions, "("+strings.Join(stringParts, " AND ")+")")
	}
	if len(numParts) > 0 {
		conditions = append(conditions, "("+strings.Join(numParts, " AND ")+")")
	}

	return strings.Join(conditions, " AND ")
}

func (t *MetaFilterTranslator) translateContains(key string, value interface{}, flt map[string]interface{}) (string, error) {
	// Python guard: if not value and value != 0 -> raise ValueError.
	// Returning "" here would let the empty fragment slip into the
	// joined SQL (e.g. "( AND other_condition)"), so we surface the
	// error instead and let the caller decide how to respond.
	//
	// isEmptyValue mirrors Python's `not value` truthiness check so
	// nil, "", empty slices, and empty maps are all caught — a plain
	// fmt.Sprintf("%v", ...) == "" test misses those last two.
	if isEmptyValue(value) && !isNumericZero(value) {
		return "", fmt.Errorf("contains value is empty: %v", flt)
	}
	coerced := coerceRangeValue(value, flt)
	if num, ok := coerceToFloat(coerced); ok {
		return fmt.Sprintf("JSON_CONTAINS(meta_fields, '$.%s', %v)", key, num), nil
	}
	escaped := escapeSQLString(fmt.Sprintf("%v", value))
	return fmt.Sprintf("JSON_CONTAINS(meta_fields, '$.%s', '\"%s\"')", key, escaped), nil
}

func (t *MetaFilterTranslator) translateNotContains(key string, value interface{}, flt map[string]interface{}) string {
	text := coerceString(value, flt)
	escaped := escapeSQLString(text)
	return fmt.Sprintf("NOT JSON_CONTAINS(meta_fields, '$.%s', '\"%s\"')", key, escaped)
}

func (t *MetaFilterTranslator) translateStartWith(key string, value interface{}, flt map[string]interface{}) string {
	text := coerceString(value, flt)
	escaped := escapeSQLString(escapeLikeWildcards(text))
	return fmt.Sprintf("JSON_EXTRACT_STRING(meta_fields, '$.%s') LIKE '%s%%'", key, escaped)
}

func (t *MetaFilterTranslator) translateEndWith(key string, value interface{}, flt map[string]interface{}) string {
	text := coerceString(value, flt)
	escaped := escapeSQLString(escapeLikeWildcards(text))
	return fmt.Sprintf("JSON_EXTRACT_STRING(meta_fields, '$.%s') LIKE '%%%s'", key, escaped)
}

// PlanPushdown translates every filter
func PlanPushdown(filters []map[string]interface{}, logic string) ([]string, error) {
	if logic != "and" && logic != "or" {
		return nil, fmt.Errorf("unknown logic %q", logic)
	}

	translator := NewMetaFilterTranslator()
	var result []string
	for _, flt := range filters {
		translated, err := translator.Translate(flt)
		if err != nil {
			return nil, err
		}
		result = append(result, translated)
	}
	return result, nil
}

// BuildInfinityFilter builds the full WHERE clause
func BuildInfinityFilter(filters []map[string]interface{}, logic string) (string, error) {
	if len(filters) == 0 {
		return "1=1", nil
	}

	fragments, err := PlanPushdown(filters, logic)
	if err != nil {
		return "", err
	}

	joiner := " AND "
	if logic == "or" {
		joiner = " OR "
	}

	return "(" + strings.Join(fragments, joiner) + ")", nil
}

// IsPushdownSupported checks if all filters can be pushed down
func IsPushdownSupported(filters []map[string]interface{}) bool {
	for _, flt := range filters {
		op, _ := flt["op"].(string)
		if !supportedOperators[op] {
			return false
		}
		key, _ := flt["key"].(string)
		if key == "" || !keyPattern.MatchString(key) {
			return false
		}
	}
	return true
}

// ExtractDocIDs extracts doc IDs from Infinity result
func ExtractDocIDs(result interface{}) []string {
	var docIDs []string

	// Try to handle different result types from Infinity SDK
	switch v := result.(type) {
	case map[string]interface{}:
		if idData, ok := v["id"].([]interface{}); ok {
			for _, id := range idData {
				if idStr, ok := id.(string); ok {
					docIDs = append(docIDs, idStr)
				}
			}
		}
	case *infinity.QueryResult:
		if v == nil {
			break
		}
		if data, ok := v.Data["id"]; ok {
			for _, id := range data {
				if idStr, ok := id.(string); ok {
					docIDs = append(docIDs, idStr)
				}
			}
		}
	}

	return docIDs
}

// coerceScalar handles scalar comparison values.
// Mirrors Python's ast.literal_eval: tries int first, then float, then string.
func coerceScalar(value interface{}, flt map[string]interface{}) interface{} {
	if value == nil {
		return nil
	}

	s := strings.TrimSpace(fmt.Sprintf("%v", value))

	// Try to parse as int first (Python ast.literal_eval preserves int vs float)
	if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
		return parsed
	}
	// Then try float
	if parsed, err := strconv.ParseFloat(s, 64); err == nil {
		return parsed
	}

	return s
}

// coerceRangeValue handles range comparison values.
// Mirrors Python: tries int first, then float, then string.
func coerceRangeValue(value interface{}, flt map[string]interface{}) interface{} {
	if value == nil {
		return nil
	}

	s := strings.TrimSpace(fmt.Sprintf("%v", value))

	// Try to parse as int first
	if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
		return parsed
	}
	// Then try float
	if parsed, err := strconv.ParseFloat(s, 64); err == nil {
		return parsed
	}

	return s
}

// coerceString ensures value is a non-empty string
func coerceString(value interface{}, flt map[string]interface{}) string {
	if value == nil {
		return ""
	}
	s := fmt.Sprintf("%v", value)
	if s == "" {
		return ""
	}
	return s
}

// csvOrList handles in/not in values
func csvOrList(value interface{}, flt map[string]interface{}) []interface{} {
	if value == nil {
		return nil
	}

	var members []interface{}

	switch v := value.(type) {
	case []interface{}:
		members = v
	case string:
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			parsed := parseJSONArray(trimmed)
			if parsed != nil {
				members = parsed
			}
		} else {
			parts := strings.Split(v, ",")
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					members = append(members, strings.ToLower(trimmed))
				}
			}
		}
	default:
		members = []interface{}{v}
	}

	if len(members) == 0 {
		return nil
	}

	result := make([]interface{}, len(members))
	for i, m := range members {
		if s, ok := m.(string); ok {
			result[i] = strings.ToLower(strings.TrimSpace(s))
		} else {
			result[i] = m
		}
	}

	return result
}

// escapeSQLString escapes SQL string
func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// escapeLikeWildcards escapes LIKE wildcards
func escapeLikeWildcards(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "%", "\\%")
	text = strings.ReplaceAll(text, "_", "\\_")
	return text
}

// coerceToFloat tries to convert interface{} to float64
func coerceToFloat(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// isNumericZero checks if value is numeric zero (0, 0.0, etc.)
func isNumericZero(value interface{}) bool {
	switch v := value.(type) {
	case int:
		return v == 0
	case int64:
		return v == 0
	case float64:
		return v == 0
	case float32:
		return v == 0
	default:
		return false
	}
}

// isEmptyValue mirrors Python's `not value` truthiness for the small
// set of types we receive in filter dicts. nil, empty strings, and
// zero-length slices/maps are all considered "empty" — calling
// fmt.Sprintf("%v", ...) on an empty slice or map produces "[]" or
// "map[]", neither of which is the empty string, so a stringly-typed
// guard would miss them.
func isEmptyValue(value interface{}) bool {
	if value == nil {
		return true
	}
	switch v := value.(type) {
	case string:
		return v == ""
	case []string:
		return len(v) == 0
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	}
	return false
}

// parseJSONArray parses a simple JSON array string
func parseJSONArray(s string) []interface{} {
	s = strings.TrimSpace(s)
	if len(s) < 2 {
		return nil
	}
	s = s[1 : len(s)-1]

	var result []interface{}
	parts := splitJSONParts(s)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) >= 2 {
			if (p[0] == '"' && p[len(p)-1] == '"') || (p[0] == '\'' && p[len(p)-1] == '\'') {
				p = p[1 : len(p)-1]
			}
		}
		result = append(result, p)
	}
	return result
}

// splitJSONParts splits JSON array parts. It tracks the actual quote
// character that opened the current quoted region so a double-quoted
// string isn't terminated by a stray single quote inside it (e.g. an
// apostrophe) and a single-quoted string isn't split by a comma inside
// a double-quoted neighbour. Naive `inQuote = !inQuote` was wrong on
// both counts.
//
// Quotes preceded by an odd number of backslashes are treated as
// escaped literals (JSON `\"` / `\'`) so the comma inside a string like
// `"a\"b,c"` doesn't trigger a spurious split.
func splitJSONParts(s string) []string {
	var parts []string
	var current strings.Builder
	var quoteChar rune
	depth := 0

	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		if c == '\'' || c == '"' {
			// Inside a quoted string, count the consecutive backslashes
			// immediately before this rune. An odd count means this
			// quote is escaped (e.g. JSON `\"`); an even count (incl. 0)
			// means it really does toggle the quote state.
			if quoteChar != 0 {
				bs := 0
				for j := i - 1; j >= 0 && runes[j] == '\\'; j-- {
					bs++
				}
				if bs%2 == 1 {
					current.WriteRune(c)
					continue
				}
			}
			if quoteChar == 0 {
				quoteChar = c
			} else if c == quoteChar {
				quoteChar = 0
			}
			current.WriteRune(c)
			continue
		}
		switch c {
		case '[', '{':
			if quoteChar == 0 {
				depth++
			}
			current.WriteRune(c)
		case ']', '}':
			if quoteChar == 0 {
				depth--
			}
			current.WriteRune(c)
		case ',':
			if quoteChar == 0 && depth == 0 {
				parts = append(parts, current.String())
				current.Reset()
			} else {
				current.WriteRune(c)
			}
		default:
			current.WriteRune(c)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}