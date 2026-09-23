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

package common

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MetaCondition represents a single parsed filter condition.
type MetaCondition struct {
	Operator string      // "=", "≠", ">", "<", "≥", "≤", "contains", "not contains", "in", "not in", "start with", "end with", "empty", "not empty"
	Key      string      // metadata field name
	Value    interface{} // comparison value
}

// MetaValueDocs maps a metadata field value to the document IDs that have that value.
// Example: {"Zhang San": ["doc1", "doc2"], "Li Si": ["doc3"]}
type MetaValueDocs map[string][]string

// MetaData maps a metadata field name to its value→documents mapping.
// Example: {"author": {"Zhang San": ["doc1"]}, "year": {"2024": ["doc1", "doc2"]}}
type MetaData map[string]MetaValueDocs

// MetaFilterInput groups filter conditions with their logic operator.
type MetaFilterInput struct {
	Conditions []MetaCondition
	Logic      string // "and" | "or"
}

// operatorMapping translates Python-style operators to internal symbols.
var operatorMapping = map[string]string{
	"is":     "=",
	"not is": "≠",
	">=":     "≥",
	"<=":     "≤",
	"!=":     "≠",
	"==":     "=",
}

// ParseAndConvert converts raw API conditions into MetaFilterInput.
// Equivalent to Python: meta_filter(metas, convert_conditions(cond), cond.get("logic"))
func ParseAndConvert(metadataCondition map[string]interface{}) *MetaFilterInput {
	if metadataCondition == nil {
		return nil
	}

	logic, _ := metadataCondition["logic"].(string)
	if logic == "" {
		logic = "and"
	}

	rawConditions, ok := metadataCondition["conditions"].([]interface{})
	if !ok || len(rawConditions) == 0 {
		return nil
	}

	var conditions []MetaCondition
	for _, raw := range rawConditions {
		cond, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		name, _ := cond["name"].(string)
		if name == "" {
			name, _ = cond["key"].(string) // OpenAI API metadata_condition uses "key"
		}
		if name == "" {
			continue
		}
		op, _ := cond["comparison_operator"].(string)
		if op == "" {
			op, _ = cond["operator"].(string) // OpenAI API uses "operator"
		}
		op = convertOperator(op)
		conditions = append(conditions, MetaCondition{
			Operator: op,
			Key:      name,
			Value:    cond["value"],
		})
	}

	if len(conditions) == 0 {
		return nil
	}

	return &MetaFilterInput{
		Conditions: conditions,
		Logic:      logic,
	}
}

// convertOperator translates operator aliases to their canonical form.

func convertOperator(op string) string {
	if mapped, exists := operatorMapping[op]; exists {
		return mapped
	}
	return op
}

// NormalizeOperator is the exported equivalent of convertOperator.
func NormalizeOperator(op string) string { return convertOperator(op) }

// MetaFilter applies filter conditions against metadata and returns matching doc IDs.
// Python equivalent: common/metadata_utils.py::meta_filter()
func MetaFilter(metas MetaData, input *MetaFilterInput) []string {
	if input == nil || len(input.Conditions) == 0 {
		return nil
	}

	logic := input.Logic
	if logic == "" {
		logic = "and"
	}

	var docIDs *map[string]struct{}

	for _, f := range input.Conditions {
		v2docs, ok := metas[f.Key]
		if !ok {
			if logic == "and" {
				return []string{}
			}
			continue
		}

		matched := filterOut(v2docs, f.Operator, f.Value)

		if docIDs == nil {
			s := make(map[string]struct{}, len(matched))
			for _, id := range matched {
				s[id] = struct{}{}
			}
			docIDs = &s
		} else {
			if logic == "and" {
				s := make(map[string]struct{})
				for _, id := range matched {
					if _, exists := (*docIDs)[id]; exists {
						s[id] = struct{}{}
					}
				}
				docIDs = &s
				if len(*docIDs) == 0 {
					return []string{}
				}
			} else {
				for _, id := range matched {
					(*docIDs)[id] = struct{}{}
				}
			}
		}
	}

	if docIDs == nil {
		return []string{}
	}
	result := make([]string, 0, len(*docIDs))
	for id := range *docIDs {
		result = append(result, id)
	}
	return result
}

// filterOut returns matching doc IDs for a single (value → matchedDocs) map and operator.
// For "in" and "not in", it delegates to filterSet for O(n+m) hash-map-based filtering;
// all other operators use matchValue for per-element predicate evaluation.
func filterOut(v2docs MetaValueDocs, operator string, value interface{}) []string {
	if operator == "in" || operator == "not in" {
		return filterSet(v2docs, operator, value)
	}
	var ids []string
	for input, docids := range v2docs {
		if matchValue(input, operator, value) {
			ids = append(ids, docids...)
		}
	}
	return ids
}

// filterSet handles "in" and "not in" operators using O(1) hash map lookups.
//
// Instead of the O(n×m) linear scan that matchValue performs for these operators
// (n = distinct metadata values, m = filter list size), filterSet builds a lookup
// map from the filter value list once (O(m)) then tests each metadata entry in
// O(1) time (O(n)), yielding O(n+m) overall.
//
// Case sensitivity follows the same contract as matchValue:
//   - "in":      case-sensitive  (exact match via toString(item) == input)
//   - "not in":  case-insensitive (strings.ToLower on both sides)
//
// When value is not a []interface{} (should not happen in normal call paths),
// filterSet returns nil — no metadata values match "in", and for "not in" it
// defensively returns nil as well (rather than returning all entries, which could
// silently bypass a misconfigured filter).
func filterSet(v2docs MetaValueDocs, operator string, value interface{}) []string {
	list, ok := value.([]interface{})
	if !ok {
		return nil
	}

	if operator == "not in" {
		// Build case-insensitive exclusion set.
		lookup := make(map[string]bool, len(list))
		for _, item := range list {
			lookup[strings.ToLower(toString(item))] = true
		}
		var ids []string
		for input, docids := range v2docs {
			if !lookup[strings.ToLower(input)] {
				ids = append(ids, docids...)
			}
		}
		return ids
	}

	// "in": build case-sensitive inclusion set.
	lookup := make(map[string]bool, len(list))
	for _, item := range list {
		lookup[toString(item)] = true
	}
	var ids []string
	for input, docids := range v2docs {
		if lookup[input] {
			ids = append(ids, docids...)
		}
	}
	return ids
}

// matchValue checks if a single metadata value matches the operator+value.
func matchValue(input string, operator string, value interface{}) bool {
	switch operator {
	case "empty":
		return input == ""
	case "not empty":
		return input != ""
	}

	valStr := toString(value)

	switch operator {
	case "contains":
		return strings.Contains(strings.ToLower(input), strings.ToLower(valStr))
	case "not contains":
		return !strings.Contains(strings.ToLower(input), strings.ToLower(valStr))
	case "start with":
		return strings.HasPrefix(strings.ToLower(input), strings.ToLower(valStr))
	case "end with":
		return strings.HasSuffix(strings.ToLower(input), strings.ToLower(valStr))

		// "in" and "not in" are intentionally omitted from matchValue.
		// filterOut (line 177) intercepts these operators and delegates
		// them to filterSet for O(n+m) hash-map-based filtering, so they
		// never reach this function through normal call paths.
	}

	// Comparison operators: =, ≠, >, <, ≥, ≤
	return compareValues(input, valStr, operator)
}

// compareValues handles numeric/date/string comparison.
func compareValues(a, b, operator string) bool {
	// If filter value (b) is a date, only compare if data (a) is also a date.
	// Non-date values should not be compared against date filters (matching Python behavior).
	if isDate(b) {
		if !isDate(a) {
			return operator == "≠"
		}
		return compareString(a, b, operator)
	}

	// Try numeric comparison
	af, errA := strconv.ParseFloat(a, 64)
	bf, errB := strconv.ParseFloat(b, 64)
	if errA == nil && errB == nil {
		return compareFloat(af, bf, operator)
	}

	// Fall back to case-insensitive string comparison
	return compareString(strings.ToLower(a), strings.ToLower(b), operator)
}

func compareFloat(a, b float64, operator string) bool {
	switch operator {
	case "=":
		return a == b
	case "≠":
		return a != b
	case ">":
		return a > b
	case "<":
		return a < b
	case "≥":
		return a >= b
	case "≤":
		return a <= b
	}
	return false
}

func compareString(a, b string, operator string) bool {
	switch operator {
	case "=":
		return a == b
	case "≠":
		return a != b
	case ">":
		return a > b
	case "<":
		return a < b
	case "≥":
		return a >= b
	case "≤":
		return a <= b
	}
	return false
}

// isDate checks if a string is in YYYY-MM-DD format.
func isDate(s string) bool {
	if len(s) != 10 {
		return false
	}
	if s[4] != '-' || s[7] != '-' {
		return false
	}
	for i := 0; i < 10; i++ {
		if i == 4 || i == 7 {
			continue
		}
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

// toString converts a value to string for comparison.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch s := v.(type) {
	case string:
		return s
	case float64:
		return strconv.FormatFloat(s, 'f', -1, 64)
	case bool:
		if s {
			return "true"
		}
		return "false"
	default:
		return ""
	}
}

// MetadataFieldDef describes one auto-metadata field, mirroring the
// {key, type, description, enum} shape stored in parser_config.metadata /
// built_in_metadata and the Python metadata_utils.py field contract.
type MetadataFieldDef struct {
	Key         string   `json:"key"`
	Type        string   `json:"type,omitempty"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
}

// Turn2JSONSchema converts a metadata field list into a JSON-Schema object,
// mirroring Python common/metadata_utils.py::turn2jsonschema /
// metadata_schema. The result is rendered into the auto-metadata extraction
// prompt (rag/prompts/meta_data.md equivalent) so the LLM knows the exact
// keys, descriptions and allowed enum values to extract.
func Turn2JSONSchema(fields []MetadataFieldDef) map[string]any {
	properties := make(map[string]any, len(fields))
	for _, f := range fields {
		if strings.TrimSpace(f.Key) == "" {
			continue
		}
		prop := map[string]any{}
		if f.Description != "" {
			prop["description"] = f.Description
		}
		if len(f.Enum) > 0 {
			prop["enum"] = f.Enum
			prop["type"] = "string"
		} else if f.Type != "" {
			prop["type"] = f.Type
		}
		properties[f.Key] = prop
	}
	if len(properties) == 0 {
		return map[string]any{}
	}
	return map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
}

// ParserConfigBool coerces a parser_config boolean-like value (bool / number) to bool,
// mirroring the frontend's metadata enable flag.
func ParserConfigBool(v any) bool {
	switch typed := v.(type) {
	case bool:
		return typed
	case float64:
		return typed > 0
	case int:
		return typed > 0
	}
	return false
}

// extractorComponentIDs returns the Extractor component ids of a parser_config in
// sorted key order, so two extractor nodes resolve deterministically.
func extractorComponentIDs(parserConfig map[string]any) []string {
	var extractorKeys []string
	for k := range parserConfig {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "extractor:") || strings.HasPrefix(lower, "extractor_") {
			extractorKeys = append(extractorKeys, k)
		}
	}
	sort.Strings(extractorKeys)
	return extractorKeys
}

// ExtractorMetadataConfig returns the modular metadata config
// ({"enabled", "metadata", "built_in_metadata"}) of the FIRST Extractor component node, in
// sorted key order so two extractor nodes resolve deterministically.
//
// The Extractor node is the only place a dataset's modular metadata config is stored: the
// top-level parser_config["metadata"] key is input-only and is stripped on write (see
// ApplyComponentScopedParserConfig). Flat legacy fields (enable_metadata / metadata_config /
// built_in_metadata at the top level or on the node) are deliberately not supported.
func ExtractorMetadataConfig(parserConfig map[string]any) (map[string]any, bool) {
	if parserConfig == nil {
		return nil, false
	}
	for _, k := range extractorComponentIDs(parserConfig) {
		node, ok := parserConfig[k].(map[string]any)
		if !ok {
			continue
		}
		if metaObj, ok := node["metadata"].(map[string]any); ok {
			return metaObj, true
		}
	}
	return nil, false
}

// SetExtractorMetadataConfig writes a modular metadata config
// ({"enabled", "metadata", "built_in_metadata"}) onto every Extractor component node of
// parser_config. It is the single write path now that the top-level
// parser_config["metadata"] key is gone, and it reports false when there is no Extractor
// node to carry the config.
func SetExtractorMetadataConfig(parserConfig map[string]any, metaObj map[string]any) bool {
	if parserConfig == nil || metaObj == nil {
		return false
	}
	written := false
	for _, k := range extractorComponentIDs(parserConfig) {
		node, ok := parserConfig[k].(map[string]any)
		if !ok {
			// A non-object node cannot carry params; skip it instead of
			// overwriting whatever the caller stored there.
			continue
		}
		node["metadata"] = copyAnyMap(metaObj)
		written = true
	}
	return written
}

// copyAnyMap shallow-copies a JSON object so every Extractor node owns its own map.
func copyAnyMap(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// IsModularMetadataConfig reports whether a parser_config metadata value is the modular
// dataset-level config ({enabled, metadata, built_in_metadata}) rather than a
// document-level metadata map of plain values. The two share the "metadata" key but are
// different things: only the modular one is scoped onto the Extractor node and stripped
// from the top level; a document-level map is left untouched.
func IsModularMetadataConfig(value any) bool {
	metaObj, ok := value.(map[string]any)
	if !ok {
		return false
	}
	_, hasEnabled := metaObj["enabled"]
	_, hasBuiltIn := metaObj["built_in_metadata"]
	return hasEnabled || hasBuiltIn
}

// MetadataRawFieldList normalizes a metadata field list that may arrive as []any (the DB
// round-trip) or []map[string]any (in-memory construction) into a []any.
func MetadataRawFieldList(value any) []any {
	switch list := value.(type) {
	case []any:
		return list
	case []map[string]any:
		out := make([]any, 0, len(list))
		for _, item := range list {
			out = append(out, item)
		}
		return out
	}
	return nil
}

// MetadataFieldDefsFromRaw parses a raw {key, type, description, enum} field list into
// typed defs. Entries without a key are dropped, so a malformed entry cannot become a
// field nobody can filter on.
func MetadataFieldDefsFromRaw(value any) []MetadataFieldDef {
	if value == nil {
		return nil
	}
	if defs, ok := value.([]MetadataFieldDef); ok {
		return defs
	}
	arr := MetadataRawFieldList(value)
	if arr == nil {
		return nil
	}
	fields := make([]MetadataFieldDef, 0, len(arr))
	for _, f := range arr {
		m, ok := f.(map[string]any)
		if !ok {
			continue
		}
		key, _ := m["key"].(string)
		if key = strings.TrimSpace(key); key == "" {
			continue
		}
		def := MetadataFieldDef{Key: key}
		if t, ok := m["type"].(string); ok {
			def.Type = t
		}
		if d, ok := m["description"].(string); ok {
			def.Description = d
		}
		if e, ok := m["enum"].([]any); ok {
			for _, ev := range e {
				if s, ok := ev.(string); ok {
					def.Enum = append(def.Enum, s)
				}
			}
		}
		fields = append(fields, def)
	}
	return fields
}

// DeclaredMetadataFieldsFromParserConfig returns the metadata fields a dataset DECLARES for
// extraction, read from the first Extractor component node — the single place the modular
// metadata config is stored.
//
// This is the DECLARATIVE source of a dataset's metadata: unlike the doc-metadata index it
// exists BEFORE anything is indexed, and it carries each field's meaning (description) and
// allowed values (enum) — what a model needs to fill a filter correctly rather than guess.
// Fields declared in both lists are de-duplicated, first occurrence winning.
func DeclaredMetadataFieldsFromParserConfig(parserConfig map[string]any) []MetadataFieldDef {
	metaObj, ok := ExtractorMetadataConfig(parserConfig)
	if !ok {
		return nil
	}
	var out []MetadataFieldDef
	seen := map[string]bool{}
	for _, raw := range []any{metaObj["metadata"], metaObj["built_in_metadata"]} {
		for _, def := range MetadataFieldDefsFromRaw(raw) {
			if seen[def.Key] {
				continue
			}
			seen[def.Key] = true
			out = append(out, def)
		}
	}
	return out
}

// combinedValueDelim splits a single combined metadata value into its parts.
// Mirrors Python doc_metadata_service.py _split_combined_values regex
// r"[、,，;；|]+" (Chinese comma 、, ASCII comma ,, full-width comma ，,
// semicolon ;, full-width semicolon ；, pipe |).
var combinedValueDelim = regexp.MustCompile(`[、,，;；|]+`)

// SplitCombinedMetadataValues post-processes a metadata map by splitting
// combined values written to the doc-metadata index, mirroring Python
// api/db/services/doc_metadata_service.py:_split_combined_values (applied at
// both insert_document_metadata:383 and update_document_metadata:468).
//
// Only list-typed values are split (each string element is split on the
// delimiter, trimmed, empties dropped, then flattened and de-duplicated);
// scalar string values are left untouched, matching the Python implementation.
// Non-string / non-list values pass through unchanged.
func SplitCombinedMetadataValues(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return meta
	}
	out := make(map[string]any, len(meta))
	for k, v := range meta {
		switch val := v.(type) {
		case []string:
			out[k] = splitCombinedStringList(val)
		case []any:
			strs, allStr := toStringSlice(val)
			if !allStr {
				out[k] = v
				continue
			}
			out[k] = splitCombinedStringList(strs)
		default:
			out[k] = v
		}
	}
	return out
}

// splitCombinedStringList splits each element on the combined-value delimiter,
// trims, drops empties, keeps single elements intact, and de-duplicates.
func splitCombinedStringList(items []string) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		parts := combinedValueDelim.Split(strings.TrimSpace(item), -1)
		got := false
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p == "" {
				continue
			}
			out = append(out, p)
			got = true
		}
		if !got {
			out = append(out, item)
		}
	}
	return dedupeMetaStrings(out)
}

// toStringSlice converts a []any to []string when every element is a string,
// reporting allStr=false otherwise.
func toStringSlice(val []any) ([]string, bool) {
	strs := make([]string, 0, len(val))
	for _, e := range val {
		s, ok := e.(string)
		if !ok {
			return nil, false
		}
		strs = append(strs, s)
	}
	return strs, true
}

// dedupeMetaStrings removes duplicates while preserving order.
func dedupeMetaStrings(input []string) []string {
	seen := make(map[string]struct{}, len(input))
	out := make([]string, 0, len(input))
	for _, s := range input {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}
