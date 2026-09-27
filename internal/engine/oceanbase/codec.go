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
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
)

const importantKeywordMaxBytes = 256

var vectorColumnPattern = regexp.MustCompile(`^q_(\d+)_vec$`)

var arrayColumns = map[string]bool{
	"important_kwd": true, "question_kwd": true, "tag_kwd": true,
	"position_int": true, "page_num_int": true, "top_int": true,
	"source_id": true, "entities_kwd": true,
}

var jsonColumns = map[string]bool{
	"tag_feas": true, "chunk_data": true, "metadata": true, "extra": true, "meta_fields": true,
}

var knownChunkColumns = func() map[string]bool {
	known := make(map[string]bool, len(chunkColumns))
	for _, column := range chunkColumns {
		known[column.name] = true
	}
	return known
}()

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

func normalizeChunk(document map[string]interface{}) (map[string]interface{}, error) {
	document = normalizeImportantKeywordFields(document)
	result := make(map[string]interface{}, len(chunkColumns)+1)
	extra := make(map[string]interface{})
	if existing, ok := document["extra"].(map[string]interface{}); ok {
		for key, value := range existing {
			extra[key] = value
		}
	}
	for key, value := range document {
		key = mapChunkField(key)
		if vectorColumnPattern.MatchString(key) {
			encoded, err := encodeVector(value)
			if err != nil {
				return nil, fmt.Errorf("encode %s: %w", key, err)
			}
			result[key] = encoded
			continue
		}
		if !knownChunkColumns[key] {
			extra[key] = value
			continue
		}
		if value == nil {
			switch key {
			case "available_int":
				result[key] = 1
			case "removed_kwd":
				result[key] = "N"
			case "_order_id":
				result[key] = 0
			default:
				result[key] = nil
			}
			continue
		}
		encoded, err := encodeColumnValue(key, value)
		if err != nil {
			return nil, fmt.Errorf("encode %s: %w", key, err)
		}
		result[key] = encoded
	}
	for _, column := range chunkColumns {
		if _, ok := result[column.name]; ok {
			continue
		}
		switch column.name {
		case "available_int":
			result[column.name] = 1
		case "removed_kwd":
			result[column.name] = "N"
		case "_order_id":
			result[column.name] = 0
		default:
			result[column.name] = nil
		}
	}
	if len(extra) > 0 {
		encoded, err := json.Marshal(extra)
		if err != nil {
			return nil, err
		}
		result["extra"] = string(encoded)
	}
	metadata := asStringMap(document["metadata"])
	if docID := stringValue(document["doc_id"]); docID != "" {
		result["group_id"] = docID
		if groupID := stringValue(metadata["_group_id"]); groupID != "" {
			result["group_id"] = groupID
		}
		if title := stringValue(metadata["_title"]); title != "" {
			result["docnm_kwd"] = title
		}
	}
	return result, nil
}

func mapChunkField(field string) string {
	if field == "chunk_order_int" {
		return "_order_id"
	}
	return field
}

func normalizeMemory(document map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(memoryColumns)+1)
	for _, column := range memoryColumns {
		result[column.name] = nil
	}
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
		if _, ok := result[key]; !ok {
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
	if result["zone_id"] == nil {
		result["zone_id"] = 0
	}
	if content := stringValue(result["content_ltks"]); content != "" {
		result["tokenized_content_ltks"] = tokenizeMemoryContent(content)
	}
	return result, nil
}

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

func normalizeSkill(document map[string]interface{}, documentID string) (map[string]interface{}, error) {
	result := make(map[string]interface{}, len(skillColumns)+1)
	for _, column := range skillColumns {
		result[column.name] = nil
	}
	for key, value := range document {
		if vectorColumnPattern.MatchString(key) {
			encoded, err := encodeVector(value)
			if err != nil {
				return nil, err
			}
			result[key] = encoded
			continue
		}
		if _, ok := result[key]; ok {
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
	if columnName == "content_with_weight" {
		if _, ok := value.(map[string]interface{}); ok {
			encoded, err := json.Marshal(value)
			return string(encoded), err
		}
	}
	if columnName == "important_kwd" {
		normalized, _, _ := normalizeImportantKeywords(value)
		// JSON escaping must not expand the VARCHAR value stored in each array element.
		encoded, err := json.Marshal(normalized)
		return string(encoded), err
	}
	if arrayColumns[columnName] {
		return encodeArray(value)
	}
	if jsonColumns[columnName] {
		if raw, ok := value.(string); ok {
			return raw, nil
		}
		encoded, err := json.Marshal(value)
		return string(encoded), err
	}
	return value, nil
}

func sanitizeImportantKeyword(keyword string) (string, bool) {
	keyword = strings.TrimSpace(keyword)
	if len(keyword) <= importantKeywordMaxBytes {
		return keyword, false
	}

	length := 0
	end := 0
	for _, character := range keyword {
		size := utf8.RuneLen(character)
		if length+size > importantKeywordMaxBytes {
			break
		}
		length += size
		end += size
	}
	truncated := strings.TrimRightFunc(keyword[:end], unicode.IsSpace)
	common.Warn(
		"Sanitizing oversized OceanBase/SeekDB important keyword",
		zap.Int("original_bytes", len(keyword)),
		zap.Int("stored_bytes", len(truncated)),
		zap.Int("limit_bytes", importantKeywordMaxBytes),
	)
	return truncated, true
}

func normalizeImportantKeywords(value interface{}) (interface{}, []string, bool) {
	values, ok := interfaceSlice(value)
	if !ok {
		return value, nil, false
	}

	normalized := make([]interface{}, len(values))
	textKeywords := make([]string, 0, len(values))
	truncated := false
	for i, item := range values {
		text, ok := item.(string)
		if !ok {
			normalized[i] = item
			continue
		}
		normalizedText, wasTruncated := sanitizeImportantKeyword(text)
		normalized[i] = normalizedText
		textKeywords = append(textKeywords, normalizedText)
		truncated = truncated || wasTruncated
	}
	return normalized, textKeywords, truncated
}

func normalizeImportantKeywordFields(fields map[string]interface{}) map[string]interface{} {
	keywords, ok := fields["important_kwd"]
	if !ok {
		return fields
	}

	normalizedKeywords, textKeywords, truncated := normalizeImportantKeywords(keywords)
	if !truncated {
		return fields
	}

	normalizedFields := copyMap(fields)
	normalizedFields["important_kwd"] = normalizedKeywords
	joinedKeywords := strings.Join(textKeywords, " ")
	tokenizedKeywords, err := tokenizer.Tokenize(joinedKeywords)
	if err != nil {
		tokenizedKeywords = joinedKeywords
	}
	normalizedFields["important_tks"] = tokenizedKeywords
	return normalizedFields
}

func encodeUpdateValue(kind, columnName string, value interface{}) (interface{}, error) {
	if vectorColumnPattern.MatchString(columnName) {
		return encodeVector(value)
	}
	if kind == "chunk" || kind == "metadata" {
		return encodeColumnValue(columnName, value)
	}
	return value, nil
}

func encodeArray(value interface{}) (string, error) {
	values, ok := interfaceSlice(value)
	if !ok {
		encoded, err := json.Marshal(value)
		return string(encoded), err
	}
	cleaned := make([]interface{}, len(values))
	for i, item := range values {
		if text, ok := item.(string); ok {
			text = strings.TrimSpace(text)
			text = strings.ReplaceAll(text, `\`, `\\`)
			text = strings.ReplaceAll(text, "\n", `\n`)
			text = strings.ReplaceAll(text, "\r", `\r`)
			text = strings.ReplaceAll(text, "\t", `\t`)
			cleaned[i] = text
		} else {
			cleaned[i] = item
		}
	}
	encoded, err := json.Marshal(cleaned)
	return string(encoded), err
}

func encodeVector(value interface{}) (string, error) {
	values, ok := floatSlice(value)
	if !ok {
		return "", fmt.Errorf("expected numeric vector, got %T", value)
	}
	parts := make([]string, len(values))
	for i, number := range values {
		// pyobvector converts every component to float32 before serializing.
		parts[i] = strconv.FormatFloat(float64(float32(number)), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}

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

func sortedColumns(document map[string]interface{}) []string {
	columns := make([]string, 0, len(document))
	for column := range document {
		columns = append(columns, column)
	}
	sort.Strings(columns)
	return columns
}

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

func asStringMap(value interface{}) map[string]interface{} {
	if result, ok := value.(map[string]interface{}); ok {
		return result
	}
	return map[string]interface{}{}
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return fmt.Sprint(value)
}
