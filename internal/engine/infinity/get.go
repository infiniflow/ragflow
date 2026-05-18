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
	"context"
	"fmt"
	"ragflow/internal/common"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"ragflow/internal/utility"

	infinity "github.com/infiniflow/infinity-go-sdk"

	"go.uber.org/zap"
)

// GetChunk gets a chunk by ID
func (e *infinityEngine) GetChunk(ctx context.Context, tableName, chunkID string, kbIDs []string) (interface{}, error) {
	if e.client == nil || e.client.conn == nil {
		return nil, fmt.Errorf("Infinity client not initialized")
	}

	// Build list of table names to search
	var tableNames []string
	if strings.HasPrefix(tableName, "ragflow_doc_meta_") {
		tableNames = []string{tableName}
	} else {
		// Search in tables like <tableName>_<kb_id> for each kbID
		if len(kbIDs) > 0 {
			for _, kbID := range kbIDs {
				tableNames = append(tableNames, fmt.Sprintf("%s_%s", tableName, kbID))
			}
		}
		// Also try the base tableName
		tableNames = append(tableNames, tableName)
	}

	// Try each table and collect results from all tables
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	// Collect chunks from all tables (same as Python's concat_dataframes)
	allChunks := make(map[string]map[string]interface{})

	for _, tblName := range tableNames {
		table, err := db.GetTable(tblName)
		if err != nil {
			continue
		}

		// Query with filter for the specific chunk ID
		filter := fmt.Sprintf("id = '%s'", chunkID)
		result, err := table.Output([]string{"*"}).Filter(filter).ToResult()
		if err != nil {
			continue
		}

		qr, ok := result.(*infinity.QueryResult)
		if !ok {
			continue
		}

		if len(qr.Data) == 0 {
			continue
		}

		// Convert to chunk format
		chunks := make([]map[string]interface{}, 0)
		for colName, colData := range qr.Data {
			for i, val := range colData {
				for len(chunks) <= i {
					chunks = append(chunks, make(map[string]interface{}))
				}
				chunks[i][colName] = val
			}
		}

		// Merge chunks into allChunks (by id), keeping first non-empty value
		for _, chunk := range chunks {
			if idVal, ok := chunk["id"].(string); ok {
				if existing, exists := allChunks[idVal]; exists {
					// Merge: keep first non-empty value for each field
					for k, v := range chunk {
						if _, has := existing[k]; !has || utility.IsEmpty(v) {
							existing[k] = v
						}
					}
				} else {
					allChunks[idVal] = chunk
				}
			}
		}
	}

	// Get the chunk by chunkID
	chunk, found := allChunks[chunkID]
	if !found {
		return nil, nil
	}

	common.Debug("infinity get chunk", zap.String("chunkID", chunkID), zap.Any("tables", tableNames))

	// Apply field mappings (same as in GetFields)
	// docnm -> docnm_kwd, title_tks, title_sm_tks
	if val, ok := chunk["docnm"].(string); ok {
		chunk["docnm_kwd"] = val
		chunk["title_tks"] = val
		chunk["title_sm_tks"] = val
	}

	// content -> content_with_weight, content_ltks, content_sm_ltks
	if val, ok := chunk["content"].(string); ok {
		chunk["content_with_weight"] = val
		chunk["content_ltks"] = val
		chunk["content_sm_ltks"] = val
	}

	// important_keywords -> important_kwd (split by comma), important_tks
	if val, ok := chunk["important_keywords"].(string); ok {
		if val == "" {
			chunk["important_kwd"] = []interface{}{}
		} else {
			parts := strings.Split(val, ",")
			chunk["important_kwd"] = parts
		}
		chunk["important_tks"] = val
	} else {
		chunk["important_kwd"] = []interface{}{}
		chunk["important_tks"] = []interface{}{}
	}

	// questions -> question_kwd (split by newline), question_tks
	if val, ok := chunk["questions"].(string); ok {
		if val == "" {
			chunk["question_kwd"] = []interface{}{}
		} else {
			parts := strings.Split(val, "\n")
			chunk["question_kwd"] = parts
		}
		chunk["question_tks"] = val
	} else {
		chunk["question_kwd"] = []interface{}{}
		chunk["question_tks"] = []interface{}{}
	}

	if posVal, ok := chunk["position_int"].(string); ok {
		chunk["position_int"] = utility.ConvertHexToPositionIntArray(posVal)
	} else {
		chunk["position_int"] = []interface{}{}
	}

	return chunk, nil
}

// GetFields applies field mappings to chunks and returns a dict keyed by chunk ID.
// Equivalent to Python's get_fields() in infinity_conn.py.
// When fields is nil/empty, returns all fields from chunks.
func GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})
	if len(chunks) == 0 {
		return result
	}

	// If fields is provided, create a set for lookup
	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[f] = true
	}

	for _, chunk := range chunks {
		// Apply field mappings
		// docnm -> docnm_kwd, title_tks, title_sm_tks
		if val, ok := chunk["docnm"].(string); ok {
			chunk["docnm_kwd"] = val
			chunk["title_tks"] = val
			chunk["title_sm_tks"] = val
		}

		// important_keywords -> important_kwd (split by comma), important_tks
		if val, ok := chunk["important_keywords"].(string); ok {
			if val == "" {
				chunk["important_kwd"] = []interface{}{}
			} else {
				parts := strings.Split(val, ",")
				chunk["important_kwd"] = parts
			}
			chunk["important_tks"] = val
		} else {
			chunk["important_kwd"] = []interface{}{}
			chunk["important_tks"] = []interface{}{}
		}

		// questions -> question_kwd (split by newline), question_tks
		if val, ok := chunk["questions"].(string); ok {
			if val == "" {
				chunk["question_kwd"] = []interface{}{}
			} else {
				parts := strings.Split(val, "\n")
				chunk["question_kwd"] = parts
			}
			chunk["question_tks"] = val
		} else {
			chunk["question_kwd"] = []interface{}{}
			chunk["question_tks"] = []interface{}{}
		}

		// content -> content_with_weight, content_ltks, content_sm_ltks
		if val, ok := chunk["content"].(string); ok {
			chunk["content_with_weight"] = val
			chunk["content_ltks"] = val
			chunk["content_sm_ltks"] = val
		}

		// authors -> authors_tks, authors_sm_tks
		if val, ok := chunk["authors"].(string); ok {
			chunk["authors_tks"] = val
			chunk["authors_sm_tks"] = val
		}

		// position_int: convert from hex string to array format (grouped by 5)
		if val, ok := chunk["position_int"].(string); ok {
			chunk["position_int"] = utility.ConvertHexToPositionIntArray(val)
		}

		// Convert page_num_int and top_int from hex string to array
		for _, colName := range []string{"page_num_int", "top_int"} {
			if val, ok := chunk[colName].(string); ok && val != "" {
				chunk[colName] = utility.ConvertHexToIntArray(val)
			}
		}

		// Post-process: convert nil/empty values to empty slices for array-like fields
		// and split _kwd fields by "###" (except knowledge_graph_kwd, docnm_kwd, important_kwd, question_kwd)
		kwdNoSplit := map[string]bool{
			"knowledge_graph_kwd": true, "docnm_kwd": true,
			"important_kwd": true, "question_kwd": true,
		}
		arrayFields := []string{
			"doc_type_kwd", "important_kwd", "important_tks", "question_tks",
			"question_kwd", "authors_tks", "authors_sm_tks", "title_tks",
			"title_sm_tks", "content_ltks", "content_sm_ltks", "tag_kwd",
		}
		for _, colName := range arrayFields {
			val, ok := chunk[colName]
			if !ok || val == nil || val == "" {
				chunk[colName] = []interface{}{}
			} else if !kwdNoSplit[colName] {
				// Split by "###" for _kwd fields
				if strVal, ok := val.(string); ok && strings.Contains(strVal, "###") {
					parts := strings.Split(strVal, "###")
					var filtered []interface{}
					for _, p := range parts {
						if p != "" {
							filtered = append(filtered, p)
						}
					}
					chunk[colName] = filtered
				}
			}
		}

		// Handle row_id mapping - Infinity returns "ROW_ID" but we use "row_id()"
		if val, ok := chunk["ROW_ID"]; ok {
			chunk["row_id()"] = val
			delete(chunk, "ROW_ID")
		}

		// Build result map keyed by id
		if id, ok := chunk["id"].(string); ok {
			fieldMap := make(map[string]interface{})
			for field, value := range chunk {
				if len(fieldSet) == 0 || fieldSet[field] {
					fieldMap[field] = value
				}
			}
			result[id] = fieldMap
		}
	}

	return result
}

// GetFields is a method wrapper for infinityEngine to satisfy DocEngine interface
func (e *infinityEngine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	return GetFields(chunks, fields)
}

// GetAggregation aggregates chunk values by field name.
// Input: [{"docnm_kwd": "docA"}, {"docnm_kwd": "docA"}, {"docnm_kwd": "docB"}]
//
// GetAggregation(chunks, "docnm_kwd") returns:
//
//	[{"key": "docA", "count": 2}, {"key": "docB", "count": 1}]
//
// For tag_kwd field, splits values by "###" separator.
// For other fields, uses comma separation.
func (e *infinityEngine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	if len(chunks) == 0 {
		return []map[string]interface{}{}
	}

	// Check if field exists in first chunk
	hasField := false
	for _, chunk := range chunks {
		if _, ok := chunk[fieldName]; ok {
			hasField = true
			break
		}
	}
	if !hasField {
		return []map[string]interface{}{}
	}

	// Count occurrences
	tagCounts := make(map[string]int)
	for _, chunk := range chunks {
		value, ok := chunk[fieldName]
		if !ok || value == nil {
			continue
		}

		// Handle string value
		if valueStr, ok := value.(string); ok {
			if valueStr == "" {
				continue
			}

			var tags []string
			// Split by "###" for tag_kwd field
			if fieldName == "tag_kwd" && strings.Contains(valueStr, "###") {
				for _, tag := range strings.Split(valueStr, "###") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tags = append(tags, tag)
					}
				}
			} else {
				// Fallback to comma separation
				for _, tag := range strings.Split(valueStr, ",") {
					tag = strings.TrimSpace(tag)
					if tag != "" {
						tags = append(tags, tag)
					}
				}
			}

			for _, tag := range tags {
				tagCounts[tag]++
			}
			continue
		}

		// Handle list value
		if valueList, ok := value.([]interface{}); ok {
			for _, item := range valueList {
				if itemStr, ok := item.(string); ok {
					tag := strings.TrimSpace(itemStr)
					if tag != "" {
						tagCounts[tag]++
					}
				}
			}
		}
	}

	if len(tagCounts) == 0 {
		return []map[string]interface{}{}
	}

	// Convert to slice and sort by count descending
	type tagCountPair struct {
		tag   string
		count int
	}
	pairs := make([]tagCountPair, 0, len(tagCounts))
	for tag, count := range tagCounts {
		pairs = append(pairs, tagCountPair{tag, count})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].count > pairs[j].count
	})

	// Convert to []map[string]interface{} directly
	result := make([]map[string]interface{}, len(pairs))
	for i, p := range pairs {
		result[i] = map[string]interface{}{"key": p.tag, "count": p.count}
	}

	return result
}

// GetDocIDs extracts document IDs from search results.
// Extracts "id" field from each chunk and returns as a list.
func (e *infinityEngine) GetDocIDs(chunks []map[string]interface{}) []string {
	if len(chunks) == 0 {
		return nil
	}
	ids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if id, ok := chunk["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// GetHighlight generates highlighted text snippets for search results.
// Matches keywords in text and wraps them with <em> tags.
func (e *infinityEngine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	result := make(map[string]string)
	if len(chunks) == 0 || len(keywords) == 0 {
		return result
	}

	// Check if field exists
	hasField := false
	for _, chunk := range chunks {
		if _, ok := chunk[fieldName]; ok {
			hasField = true
			break
		}
	}
	if !hasField {
		// Try alternative field names
		if fieldName == "content_with_weight" {
			if _, ok := chunks[0]["content"]; ok {
				fieldName = "content"
				hasField = true
			}
		}
	}
	if !hasField {
		return result
	}

	emTag := regexp.MustCompile(`<em>[^<>]+</em>`)

	for _, chunk := range chunks {
		id := ""
		if idVal, ok := chunk["id"].(string); ok {
			id = idVal
		}

		txt, ok := chunk[fieldName].(string)
		if !ok || txt == "" {
			continue
		}

		// Check if already highlighted
		if emTag.MatchString(txt) {
			result[id] = txt
			continue
		}

		// Replace newlines with spaces
		txt = regexp.MustCompile(`[\r\n]`).ReplaceAllString(txt, " ")

		// Split by sentence delimiters
		delimiters := regexp.MustCompile(`[.?!;\n]`)
		segments := delimiters.Split(txt, -1)

		var highlightedSegments []string
		for _, segment := range segments {
			// Check if segment is English or contains keywords
			englishCount := 0
			totalCount := 0
			for _, r := range segment {
				if unicode.IsLetter(r) {
					totalCount++
					if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
						englishCount++
					}
				}
			}
			isEnglish := totalCount > 0 && float64(englishCount)/float64(totalCount) > 0.5
			segmentToCheck := segment
			if isEnglish {
				// For English: match whole words with boundaries
				for _, kw := range keywords {
					re := regexp.MustCompile(`(^|[ .?/'\"\(\)!,:;-])` + regexp.QuoteMeta(kw) + `([ .?/'\"\(\)!,:;-]|$)`)
					segmentToCheck = re.ReplaceAllString(segmentToCheck, "$1<em>"+kw+"</em>$2")
				}
			} else {
				// For non-English: simple substring match
				for _, kw := range keywords {
					segmentToCheck = strings.ReplaceAll(segmentToCheck, kw, "<em>"+kw+"</em>")
				}
			}
			if strings.Contains(segmentToCheck, "<em>") {
				highlightedSegments = append(highlightedSegments, segmentToCheck)
			}
		}

		if len(highlightedSegments) > 0 {
			result[id] = strings.Join(highlightedSegments, "...")
		}
	}

	return result
}
