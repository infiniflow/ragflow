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

package elasticsearch

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"ragflow/internal/common"

	"go.uber.org/zap"
)

// Field prefix in the doc-metadata ES index
const metaFieldsPrefix = "meta_fields"

// Date pattern for YYYY-MM-DD
var dateRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Supported operators
var supportedOperators = map[string]bool{
	"=":            true,
	"≠":            true,
	">":            true,
	"<":            true,
	"≥":            true,
	"≤":            true,
	"in":           true,
	"not in":       true,
	"contains":     true,
	"not contains": true,
	"start with":   true,
	"end with":     true,
	"empty":        true,
	"not empty":    true,
}

// Range operators mapping
var rangeOps = map[string]string{
	">": "gt",
	"<": "lt",
	"≥": "gte",
	"≤": "lte",
}

// Negative operators unsafe for multi-valued fields
var multivalueUnsafeNegativeOps = map[string]bool{
	"≠":      true,
	"not in": true,
}

// UnsupportedMetaFilterError is raised when a filter cannot be expressed as ES DSL
type UnsupportedMetaFilterError struct {
	Reason       string
	FilterClause map[string]interface{}
}

func (e *UnsupportedMetaFilterError) Error() string {
	return e.Reason
}

// TranslatedFilter represents a single filter rendered as ES bool clauses
type TranslatedFilter struct {
	Must    []map[string]interface{}
	MustNot []map[string]interface{}
}

// ToClauses converts to ES clauses
func (f *TranslatedFilter) ToClauses() []map[string]interface{} {
	if len(f.Must) == 0 && len(f.MustNot) == 0 {
		return []map[string]interface{}{}
	}
	if len(f.MustNot) == 0 {
		if len(f.Must) == 1 {
			return []map[string]interface{}{f.Must[0]}
		}
		return []map[string]interface{}{{"bool": map[string]interface{}{"must": f.Must}}}
	}
	return []map[string]interface{}{{"bool": map[string]interface{}{"must": f.Must, "must_not": f.MustNot}}}
}

// MetaFilterPushdownPlan represents composed ES bool query body
type MetaFilterPushdownPlan struct {
	Logic      string
	translated []*TranslatedFilter
}

// IsEmpty returns true if plan has no filters
func (p *MetaFilterPushdownPlan) IsEmpty() bool {
	return len(p.translated) == 0
}

// ToQuery renders the full ES query body scoped to given KB IDs
func (p *MetaFilterPushdownPlan) ToQuery(kbIDs []string) map[string]interface{} {
	kbClause := map[string]interface{}{"terms": map[string]interface{}{"kb_id": kbIDs}}

	if p.IsEmpty() {
		return map[string]interface{}{
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"filter": []map[string]interface{}{kbClause},
				},
			},
		}
	}

	var flatClauses []map[string]interface{}
	for _, t := range p.translated {
		clauses := t.ToClauses()
		flatClauses = append(flatClauses, clauses...)
	}

	var inner map[string]interface{}
	if p.Logic == "or" {
		inner = map[string]interface{}{
			"bool": map[string]interface{}{
				"should":               flatClauses,
				"minimum_should_match": 1,
			},
		}
	} else {
		inner = map[string]interface{}{
			"bool": map[string]interface{}{
				"must": flatClauses,
			},
		}
	}

	return map[string]interface{}{
		"query": map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": []map[string]interface{}{kbClause, inner},
			},
		},
	}
}

// MetaFilterTranslator translates filter clauses to ES DSL
type MetaFilterTranslator struct {
	prefix string
}

// NewMetaFilterTranslator creates a new translator
func NewMetaFilterTranslator() *MetaFilterTranslator {
	return &MetaFilterTranslator{prefix: metaFieldsPrefix}
}

func (t *MetaFilterTranslator) fieldName(key string) string {
	return t.prefix + "." + key
}

// Translate translates a single filter dict into ES bool clauses
func (t *MetaFilterTranslator) Translate(flt map[string]interface{}) (*TranslatedFilter, error) {
	op, _ := flt["op"].(string)
	key, _ := flt["key"].(string)
	value := flt["value"]

	if key == "" {
		return nil, &UnsupportedMetaFilterError{Reason: "filter is missing a string key", FilterClause: flt}
	}
	if !supportedOperators[op] {
		return nil, &UnsupportedMetaFilterError{Reason: fmt.Sprintf("unknown operator %q", op), FilterClause: flt}
	}
	if op != "empty" && op != "not empty" {
		switch v := value.(type) {
		case nil:
			return nil, &UnsupportedMetaFilterError{Reason: fmt.Sprintf("operator %q requires a value", op), FilterClause: flt}
		case string:
			if strings.TrimSpace(v) == "" {
				return nil, &UnsupportedMetaFilterError{Reason: fmt.Sprintf("operator %q requires a non-empty value", op), FilterClause: flt}
			}
		case []interface{}:
			if len(v) == 0 {
				return nil, &UnsupportedMetaFilterError{Reason: fmt.Sprintf("operator %q requires at least one value", op), FilterClause: flt}
			}
		}
	}

	fieldPath := t.fieldName(key)

	switch op {
	case "empty":
		return t.translateEmpty(fieldPath), nil
	case "not empty":
		return t.translateNotEmpty(fieldPath), nil
	case "=":
		return t.translateEqual(fieldPath, value, flt), nil
	case "≠":
		return t.translateNotEqual(fieldPath, value, flt), nil
	case ">", "<", "≥", "≤":
		return t.translateRange(fieldPath, op, value, flt), nil
	case "in":
		return t.translateIn(fieldPath, value, flt), nil
	case "not in":
		return t.translateNotIn(fieldPath, value, flt), nil
	case "contains":
		return t.translateContains(fieldPath, value, flt), nil
	case "not contains":
		return t.translateNotContains(fieldPath, value, flt), nil
	case "start with":
		return t.translateStartWith(fieldPath, value, flt), nil
	case "end with":
		return t.translateEndWith(fieldPath, value, flt), nil
	}

	return nil, &UnsupportedMetaFilterError{Reason: fmt.Sprintf("no handler for operator %q", op), FilterClause: flt}
}

func (t *MetaFilterTranslator) translateEmpty(fieldPath string) *TranslatedFilter {
	keywordPath := keywordPath(fieldPath)
	return &TranslatedFilter{
		Must: []map[string]interface{}{
			{
				"bool": map[string]interface{}{
					"should": []map[string]interface{}{
						{"bool": map[string]interface{}{"must_not": []map[string]interface{}{{"exists": map[string]interface{}{"field": fieldPath}}}}},
						{"term": map[string]interface{}{keywordPath: ""}},
					},
					"minimum_should_match": 1,
				},
			},
		},
	}
}

func (t *MetaFilterTranslator) translateNotEmpty(fieldPath string) *TranslatedFilter {
	keywordPath := keywordPath(fieldPath)
	return &TranslatedFilter{
		Must:    []map[string]interface{}{{"exists": map[string]interface{}{"field": fieldPath}}},
		MustNot: []map[string]interface{}{{"term": map[string]interface{}{keywordPath: ""}}},
	}
}

func (t *MetaFilterTranslator) translateEqual(fieldPath string, value interface{}, flt map[string]interface{}) *TranslatedFilter {
	coerced := coerceScalar(value, flt)
	return &TranslatedFilter{
		Must: []map[string]interface{}{termOrMatch(fieldPath, coerced)},
	}
}

func (t *MetaFilterTranslator) translateNotEqual(fieldPath string, value interface{}, flt map[string]interface{}) *TranslatedFilter {
	coerced := coerceScalar(value, flt)
	return &TranslatedFilter{
		Must:    []map[string]interface{}{{"exists": map[string]interface{}{"field": fieldPath}}},
		MustNot: []map[string]interface{}{termOrMatch(fieldPath, coerced)},
	}
}

func (t *MetaFilterTranslator) translateRange(fieldPath string, op string, value interface{}, flt map[string]interface{}) *TranslatedFilter {
	coerced := coerceRangeValue(value, flt)
	sqlOp := rangeOps[op]
	return &TranslatedFilter{
		Must: []map[string]interface{}{
			{"exists": map[string]interface{}{"field": fieldPath}},
			{"range": map[string]interface{}{fieldPath: map[string]interface{}{sqlOp: coerced}}},
		},
	}
}

func (t *MetaFilterTranslator) translateIn(fieldPath string, value interface{}, flt map[string]interface{}) *TranslatedFilter {
	members := csvOrList(value, flt)
	return &TranslatedFilter{
		Must: []map[string]interface{}{termsStringOrNumeric(fieldPath, members)},
	}
}

func (t *MetaFilterTranslator) translateNotIn(fieldPath string, value interface{}, flt map[string]interface{}) *TranslatedFilter {
	members := csvOrList(value, flt)
	return &TranslatedFilter{
		Must:    []map[string]interface{}{{"exists": map[string]interface{}{"field": fieldPath}}},
		MustNot: []map[string]interface{}{termsStringOrNumeric(fieldPath, members)},
	}
}

func (t *MetaFilterTranslator) translateContains(fieldPath string, value interface{}, flt map[string]interface{}) *TranslatedFilter {
	text := coerceString(value, flt)
	return &TranslatedFilter{
		Must: []map[string]interface{}{wildcard(fieldPath, "*"+escapeWildcard(text)+"*")},
	}
}

func (t *MetaFilterTranslator) translateNotContains(fieldPath string, value interface{}, flt map[string]interface{}) *TranslatedFilter {
	text := coerceString(value, flt)
	return &TranslatedFilter{
		Must:    []map[string]interface{}{{"exists": map[string]interface{}{"field": fieldPath}}},
		MustNot: []map[string]interface{}{wildcard(fieldPath, "*"+escapeWildcard(text)+"*")},
	}
}

func (t *MetaFilterTranslator) translateStartWith(fieldPath string, value interface{}, flt map[string]interface{}) *TranslatedFilter {
	text := coerceString(value, flt)
	keywordPath := keywordPath(fieldPath)
	return &TranslatedFilter{
		Must: []map[string]interface{}{
			{"prefix": map[string]interface{}{keywordPath: map[string]interface{}{"value": text, "case_insensitive": true}}},
		},
	}
}

func (t *MetaFilterTranslator) translateEndWith(fieldPath string, value interface{}, flt map[string]interface{}) *TranslatedFilter {
	text := coerceString(value, flt)
	return &TranslatedFilter{
		Must: []map[string]interface{}{wildcard(fieldPath, "*"+escapeWildcard(text))},
	}
}

// BuildMetaFilterQuery translates filters and renders ES query body
func BuildMetaFilterQuery(filters []map[string]interface{}, logic string, kbIDs []string) (map[string]interface{}, error) {
	plan, err := planPushdown(filters, logic)
	if err != nil {
		return nil, err
	}
	return plan.ToQuery(kbIDs), nil
}

// PlanPushdown translates every filter and builds a composed plan
func planPushdown(filters []map[string]interface{}, logic string) (*MetaFilterPushdownPlan, error) {
	if logic != "and" && logic != "or" {
		return nil, fmt.Errorf("unsupported logic %q", logic)
	}

	translator := NewMetaFilterTranslator()
	plan := &MetaFilterPushdownPlan{Logic: logic}
	for _, flt := range filters {
		translated, err := translator.Translate(flt)
		if err != nil {
			common.Warn("plan_pushdown failed", zap.String("error", err.Error()))
			return nil, err
		}
		plan.translated = append(plan.translated, translated)
	}
	return plan, nil
}

// IsPushdownSupported checks if all filters can be pushed down
func IsPushdownSupported(filters []map[string]interface{}) bool {
	for _, flt := range filters {
		op, _ := flt["op"].(string)
		if !supportedOperators[op] {
			return false
		}
		if multivalueUnsafeNegativeOps[op] {
			return false
		}
		key, _ := flt["key"].(string)
		if key == "" {
			return false
		}
	}
	return true
}

// ExtractDocIDs extracts doc IDs from ES search response
func ExtractDocIDs(esResponse map[string]interface{}) []string {
	var docIDs []string

	hitsRoot, ok := esResponse["hits"].(map[string]interface{})
	if !ok {
		return docIDs
	}

	rawHits, ok := hitsRoot["hits"].([]interface{})
	if !ok {
		return docIDs
	}

	for _, hit := range rawHits {
		hitMap, ok := hit.(map[string]interface{})
		if !ok {
			continue
		}
		docID := ""
		if id, ok := hitMap["_id"].(string); ok {
			docID = id
		} else {
			source, _ := hitMap["_source"].(map[string]interface{})
			if source != nil {
				if id, ok := source["id"].(string); ok {
					docID = id
				} else if docID, ok = source["doc_id"].(string); ok {
					// already set
				}
			}
		}
		if docID != "" {
			docIDs = append(docIDs, docID)
		}
	}

	return docIDs
}

// coerceScalar mirrors ast.literal_eval then str.lower() flow
func coerceScalar(value interface{}, flt map[string]interface{}) interface{} {
	if value == nil {
		return nil
	}

	s := strings.TrimSpace(fmt.Sprintf("%v", value))
	if dateRegex.MatchString(s) {
		return s
	}

	// Try to parse as int first (like Python's ast.literal_eval preserves int vs float)
	if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
		return parsed
	}
	if parsed, err := strconv.ParseFloat(s, 64); err == nil {
		return parsed
	}

	// It's a string - lower case it
	return strings.ToLower(s)
}

// coerceRangeValue handles range comparison values
func coerceRangeValue(value interface{}, flt map[string]interface{}) interface{} {
	if value == nil {
		return nil
	}

	s := strings.TrimSpace(fmt.Sprintf("%v", value))
	if dateRegex.MatchString(s) {
		return s
	}

	// Try to parse as number
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
		// Try to parse as JSON array or split by comma
		trimmed := strings.TrimSpace(v)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			// JSON array - parse it
			parsed := parseJSONArray(trimmed)
			if parsed != nil {
				members = parsed
			}
		} else {
			// Comma-separated
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

	// Normalize strings to lowercase
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

// keywordPath returns .keyword sub-field path
func keywordPath(fieldPath string) string {
	return fieldPath + ".keyword"
}

// termOrMatch creates exact-match clause
func termOrMatch(fieldPath string, value interface{}) map[string]interface{} {
	if s, ok := value.(string); ok {
		return map[string]interface{}{
			"term": map[string]interface{}{
				keywordPath(fieldPath): map[string]interface{}{
					"value":            s,
					"case_insensitive": true,
				},
			},
		}
	}
	return map[string]interface{}{
		"term": map[string]interface{}{fieldPath: value},
	}
}

// termsStringOrNumeric creates terms query for in/not in
func termsStringOrNumeric(fieldPath string, members []interface{}) map[string]interface{} {
	allNumeric := true
	for _, m := range members {
		if _, ok := m.(string); ok {
			allNumeric = false
			break
		}
	}

	if allNumeric {
		return map[string]interface{}{
			"terms": map[string]interface{}{fieldPath: members},
		}
	}

	var shouldClauses []map[string]interface{}
	for _, m := range members {
		shouldClauses = append(shouldClauses, termOrMatch(fieldPath, m))
	}

	return map[string]interface{}{
		"bool": map[string]interface{}{
			"should":               shouldClauses,
			"minimum_should_match": 1,
		},
	}
}

// wildcard creates wildcard query
func wildcard(fieldPath string, pattern string) map[string]interface{} {
	return map[string]interface{}{
		"wildcard": map[string]interface{}{
			keywordPath(fieldPath): map[string]interface{}{
				"value":            pattern,
				"case_insensitive": true,
			},
		},
	}
}

// escapeWildcard escapes wildcard metacharacters
func escapeWildcard(text string) string {
	text = strings.ReplaceAll(text, "\\", "\\\\")
	text = strings.ReplaceAll(text, "*", "\\*")
	text = strings.ReplaceAll(text, "?", "\\?")
	return text
}

// parseJSONArray parses a simple JSON array string
func parseJSONArray(s string) []interface{} {
	// Remove brackets
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
		// Remove quotes if present
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
