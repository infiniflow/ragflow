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
	"strings"

	"ragflow/internal/logger"
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

	logger.Debug("infinity get chunk", zap.String("chunkID", chunkID), zap.Any("tables", tableNames))

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
		posArr, err := utility.ConvertHexToPositionIntArray(posVal)
		if err != nil {
			return nil, fmt.Errorf("failed to convert position_int for chunk %s: %w", chunkID, err)
		}
		chunk["position_int"] = posArr
	} else {
		chunk["position_int"] = []interface{}{}
	}

	return chunk, nil
}

// GetFields applies field mappings to chunks and returns a dict keyed by chunk ID.
// Equivalent to Python's get_fields() in infinity_conn.py.
// When fields is nil/empty, returns all fields from chunks.
func GetFields(chunks []map[string]interface{}, fields []string) (map[string]map[string]interface{}, error) {
	result := make(map[string]map[string]interface{})
	if len(chunks) == 0 {
		return result, nil
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
			posArr, err := utility.ConvertHexToPositionIntArray(val)
			if err != nil {
				return nil, fmt.Errorf("failed to convert position_int for chunk %v: %w", chunk["id"], err)
			}
			chunk["position_int"] = posArr
		}

		// Convert page_num_int and top_int from hex string to array
		for _, colName := range []string{"page_num_int", "top_int"} {
			if val, ok := chunk[colName].(string); ok && val != "" {
				intArr, err := utility.ConvertHexToIntArray(val)
				if err != nil {
					return nil, fmt.Errorf("failed to convert %s for chunk %v: %w", colName, chunk["id"], err)
				}
				chunk[colName] = intArr
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

	return result, nil
}

// GetFields is a method wrapper for infinityEngine to satisfy DocEngine interface
func (e *infinityEngine) GetFields(chunks []map[string]interface{}, fields []string) (map[string]map[string]interface{}, error) {
	return GetFields(chunks, fields)
}
