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
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/tokenizer"

	"github.com/lib/pq"
)

const keywordSeparator = "###"

var vectorColumnPattern = regexp.MustCompile(`^q_(\d+)_vec$`)

// integerArrayColumns are bound as PG integer[] via pq.Array.
var integerArrayColumns = map[string]bool{
	"position_int": true, "page_num_int": true, "top_int": true,
}

var jsonTextColumns = map[string]bool{
	"metadata": true, "extra": true, "meta_fields": true,
}

var memoryFieldToColumn = map[string]string{
	"message_type": "message_type_kwd",
	"status":       "status_int",
	"content":      "content_ltks",
}

var memoryColumnToField = map[string]string{
	"message_type_kwd": "message_type",
	"status_int":       "status",
	"content_ltks":     "content",
}

var knownMemoryColumns = func() map[string]bool {
	known := make(map[string]bool, len(memoryColumns))
	for _, column := range memoryColumns {
		known[column.name] = true
	}
	return known
}()

var knownSkillColumns = func() map[string]bool {
	known := make(map[string]bool, len(skillColumns))
	for _, column := range skillColumns {
		known[column.name] = true
	}
	return known
}()

// fieldKeyword reports whether a field stores its list values ###-joined in a
// single text column: source_id and *_kwd fields, except docnm_kwd and
// knowledge_graph_kwd which store plain text.
func fieldKeyword(field string) bool {
	if field == "source_id" {
		return true
	}
	return strings.HasSuffix(field, "_kwd") && field != "docnm_kwd" && field != "knowledge_graph_kwd"
}

// normalizeChunk encodes a chunk document for insert/update. Unknown fields
// are kept as-is: ensureDynamicColumns has (or will) materialize them as real
// varchar(256) columns — the ES dynamic-mapping equivalent. Unlike the
// OceanBase codec there is no extra folding and no whole-column padding; the
// table DEFAULTs fill the gaps so unwritten fields keep their ES-parity ”.
func normalizeChunk(document map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(document))
	for key, value := range document {
		if vectorColumnPattern.MatchString(key) {
			encoded, err := encodeVector(value)
			if err != nil {
				return nil, fmt.Errorf("encode %s: %w", key, err)
			}
			result[key] = encoded
			continue
		}
		encoded, err := encodeColumnValue(key, value)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", key, err)
		}
		result[key] = encoded
	}
	return result, nil
}

// encodeColumnValue converts one document value into its physical column
// representation: kb_id keeps only the first element of a list, integer
// arrays flatten, maps serialize as JSON, and keyword lists join.
func encodeColumnValue(columnName string, value interface{}) (interface{}, error) {
	if value == nil {
		return nil, nil
	}
	if columnName == "kb_id" {
		if values, ok := interfaceSlice(value); ok {
			if len(values) == 0 {
				return nil, nil
			}
			return values[0], nil
		}
	}
	if integerArrayColumns[columnName] {
		return encodeIntegerArray(columnName, value)
	}
	switch value.(type) {
	case map[string]interface{}:
		encoded, err := json.Marshal(value)
		return string(encoded), err
	}
	if fieldKeyword(columnName) {
		if values, ok := interfaceSlice(value); ok {
			parts := make([]string, 0, len(values))
			for _, item := range values {
				parts = append(parts, stringValue(item))
			}
			return strings.Join(parts, keywordSeparator), nil
		}
	}
	return value, nil
}

// encodeIntegerArray flattens position_int's two-dimensional [[page, x1, y1,
// x2, y2], ...] into the flat integer[] storage (regrouped on read) and binds
// page_num_int/top_int as plain integer arrays.
func encodeIntegerArray(columnName string, value interface{}) (interface{}, error) {
	values, ok := interfaceSlice(value)
	if !ok {
		return nil, fmt.Errorf("expected integer array for %s, got %T", columnName, value)
	}
	flat := make([]int64, 0, len(values))
	for _, item := range values {
		if nested, ok := interfaceSlice(item); ok {
			for _, inner := range nested {
				number, ok := numberToInt64(inner)
				if !ok {
					return nil, fmt.Errorf("non-integer element in %s", columnName)
				}
				flat = append(flat, number)
			}
			continue
		}
		number, ok := numberToInt64(item)
		if !ok {
			return nil, fmt.Errorf("non-integer element in %s", columnName)
		}
		flat = append(flat, number)
	}
	return pq.Int64Array(flat), nil
}

// normalizeMemory maps a memory document onto the memory columns, encoding
// content_embed into its q_N_vec column and deriving the tokenized content
// lookup column.
func normalizeMemory(document map[string]interface{}, datasetID string) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(memoryColumns)+1)
	for key, value := range document {
		if mapped, ok := memoryFieldToColumn[key]; ok {
			key = mapped
		}
		if key == "content_embed" {
			vector, ok := floatSlice(value)
			if !ok || len(vector) == 0 {
				continue
			}
			key = fmt.Sprintf("q_%d_vec", len(vector))
			value = vector
		}
		if vectorColumnPattern.MatchString(key) {
			encoded, err := encodeVector(value)
			if err != nil {
				return nil, fmt.Errorf("encode %s: %w", key, err)
			}
			result[key] = encoded
			continue
		}
		if !knownMemoryColumns[key] {
			continue
		}
		result[key] = value
	}
	if status, ok := result["status_int"].(bool); ok {
		if status {
			result["status_int"] = 1
		} else {
			result["status_int"] = 0
		}
	}
	if result["status_int"] == nil {
		result["status_int"] = 1
	}
	if stringValue(result["memory_id"]) == "" && datasetID != "" {
		result["memory_id"] = datasetID
	}
	if result["zone_id"] == nil {
		result["zone_id"] = 0
	}
	if content := stringValue(result["content_ltks"]); content != "" {
		result["tokenized_content_ltks"] = tokenizeMemoryContent(content)
	}
	return result, nil
}

// tokenizeMemoryContent produces the fine-grained token string the memory
// full-text lookup queries, falling back to coarser inputs on tokenizer
// failure.
func tokenizeMemoryContent(content string) string {
	tokens, err := tokenizer.Tokenize(content)
	if err != nil {
		return content
	}
	fineTokens, err := tokenizer.FineGrainedTokenize(tokens)
	if err != nil {
		return tokens
	}
	return fineTokens
}

// normalizeSkill maps a skill document onto the skill columns, passing
// vector fields through encoded and dropping unknown keys.
func normalizeSkill(document map[string]interface{}, documentID string) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(skillColumns)+1)
	for key, value := range document {
		if vectorColumnPattern.MatchString(key) {
			encoded, err := encodeVector(value)
			if err != nil {
				return nil, err
			}
			result[key] = encoded
			continue
		}
		if knownSkillColumns[key] {
			result[key] = value
		}
	}
	if stringValue(result["skill_id"]) == "" {
		result["skill_id"] = documentID
	}
	for _, pair := range [][2]string{{"name", "name_tks"}, {"tags", "tags_tks"}, {"description", "description_tks"}, {"content", "content_tks"}} {
		if result[pair[1]] != nil {
			continue
		}
		original := stringValue(result[pair[0]])
		tokens, err := tokenizer.Tokenize(original)
		if err != nil {
			tokens = original
		}
		result[pair[1]] = tokens
	}
	return result, nil
}

// encodeUpdateValue converts one field for an UPDATE, applying the same
// physical representations as encodeColumnValue (vectors, memory status
// bools, JSON maps).
func encodeUpdateValue(kind, columnName string, value interface{}) (interface{}, error) {
	if vectorColumnPattern.MatchString(columnName) {
		return encodeVector(value)
	}
	if kind == "memory" {
		if columnName == "status_int" {
			if status, ok := value.(bool); ok {
				if status {
					return 1, nil
				}
				return 0, nil
			}
		}
		return value, nil
	}
	return encodeColumnValue(columnName, value)
}

// encodeVector renders a vector as the floatvector text form "[0.1,0.2,...]",
// converting every component to float32 first — the format the Python
// psycopg2 connector wrote, kept byte-compatible for shared tables.
func encodeVector(value interface{}) (string, error) {
	values, ok := floatSlice(value)
	if !ok {
		return "", fmt.Errorf("expected numeric vector, got %T", value)
	}
	parts := make([]string, len(values))
	for i, number := range values {
		parts[i] = strconv.FormatFloat(float64(float32(number)), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}

// zeroVector is the placeholder written when a document lacks a vector column
// the table already has, mirroring the Python connector's zero backfill.
func zeroVector(dimension int) string {
	return "[" + strings.Repeat("0,", dimension-1) + "0]"
}

// vectorDimension reports the embedding size a document carries, from its
// q_N_vec column or content_embed length.
func vectorDimension(document map[string]interface{}) int {
	for key, value := range document {
		if matches := vectorColumnPattern.FindStringSubmatch(key); len(matches) == 2 {
			dimension, _ := strconv.Atoi(matches[1])
			return dimension
		}
		if key == "content_embed" {
			if vector, ok := floatSlice(value); ok {
				return len(vector)
			}
		}
	}
	return 0
}

// sortedColumns lists a document's columns in sorted order, keeping INSERT
// and UPDATE column ordering deterministic.
func sortedColumns(document map[string]interface{}) []string {
	columns := make([]string, 0, len(document))
	for column := range document {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

// interfaceSlice converts any slice or array value to []interface{}.
func interfaceSlice(value interface{}) ([]interface{}, bool) {
	if value == nil {
		return nil, false
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	result := make([]interface{}, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		result[i] = rv.Index(i).Interface()
	}
	return result, true
}

// floatSlice converts a value to []float64 when every element is numeric.
func floatSlice(value interface{}) ([]float64, bool) {
	values, ok := interfaceSlice(value)
	if !ok {
		return nil, false
	}
	result := make([]float64, len(values))
	for i, item := range values {
		number, ok := numberToFloat(item)
		if !ok {
			return nil, false
		}
		result[i] = number
		if math.IsNaN(result[i]) || math.IsInf(result[i], 0) {
			return nil, false
		}
	}
	return result, true
}

// numberToFloat coerces the numeric kinds database/sql and JSON decode to
// into a float64.
func numberToFloat(value interface{}) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case json.Number:
		parsed, err := number.Float64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseFloat(number, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

// numberToInt64 coerces the numeric kinds database/sql and JSON decode to
// into an int64.
func numberToInt64(value interface{}) (int64, bool) {
	switch number := value.(type) {
	case int:
		return int64(number), true
	case int32:
		return int64(number), true
	case int64:
		return number, true
	case float64:
		return int64(number), true
	case float32:
		return int64(number), true
	case json.Number:
		parsed, err := number.Int64()
		return parsed, err == nil
	case string:
		parsed, err := strconv.ParseInt(number, 10, 64)
		return parsed, err == nil
	default:
		return 0, false
	}
}

// stringValue renders a value as the empty string for nil, itself for a
// string, and fmt's default otherwise.
func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}
