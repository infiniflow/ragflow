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
	"encoding/json"
	"fmt"
	"ragflow/internal/common"
	"regexp"
	"strconv"
	"strings"
	"ragflow/internal/utility"

	infinity "github.com/infiniflow/infinity-go-sdk"

	"go.uber.org/zap"
)

// InsertChunks inserts documents into a dataset table
// Table name format: {indexName}_{knowledgebaseID}
// Auto-create the table if it doesn't exist
// Delete existing rows with matching IDs before insert
func (e *infinityEngine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, indexName string, knowledgebaseID string) ([]string, error) {
	tableName := fmt.Sprintf("%s_%s", indexName, knowledgebaseID)
	common.Info("InfinityConnection.InsertChunks called", zap.String("tableName", tableName), zap.Int("chunkCount", len(chunks)))

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return nil, fmt.Errorf("Failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		// Table doesn't exist, try to create it
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "doesn't exist") {
			return nil, fmt.Errorf("Failed to get table %s: %w", tableName, err)
		}

		// Infer vector size from chunks
		vectorSize := 0
		vectorPattern := regexp.MustCompile(`q_(\d+)_vec`)
		for _, chunk := range chunks {
			for key := range chunk {
				matches := vectorPattern.FindStringSubmatch(key)
				if len(matches) >= 2 {
					vectorSize, _ = strconv.Atoi(matches[1])
					break
				}
			}
			if vectorSize > 0 {
				break
			}
		}
		if vectorSize == 0 {
			return nil, fmt.Errorf("cannot infer vector size from chunks")
		}

		// Determine parser_id from chunk structure
		parserID := ""
		if chunkData, ok := chunks[0]["chunk_data"].(map[string]interface{}); ok && chunkData != nil {
			parserID = "table"
		}

		// Create table
		if err := e.CreateChunkStore(ctx, indexName, knowledgebaseID, vectorSize, parserID); err != nil {
			return nil, fmt.Errorf("Failed to create table: %w", err)
		}

		table, err = db.GetTable(tableName)
		if err != nil {
			return nil, fmt.Errorf("Failed to get table after creation: %w", err)
		}
	}

	// Get embedding columns and their sizes
	var embeddingCols [][2]interface{}
	colsResp, err := table.ShowColumns()
	if err != nil {
		return nil, fmt.Errorf("Failed to get columns: %w", err)
	}
	result, ok := colsResp.(*infinity.QueryResult)
	if !ok {
		return nil, fmt.Errorf("unexpected response type: %T", colsResp)
	}

	// ShowColumns returns a result set where Data contains arrays of column values
	re := regexp.MustCompile(`Embedding\([a-z]+,(\d+)\)`)
	if nameArr, ok := result.Data["name"]; ok {
		if typeArr, ok := result.Data["type"]; ok {
			for i := 0; i < len(nameArr); i++ {
				colName, _ := nameArr[i].(string)
				colType, _ := typeArr[i].(string)
				matches := re.FindStringSubmatch(colType)
				if len(matches) >= 2 {
					size, _ := strconv.Atoi(matches[1])
					embeddingCols = append(embeddingCols, [2]interface{}{colName, size})
				}
			}
		}
	}

	// Transform chunks using helper function
	insertChunks := make([]map[string]interface{}, len(chunks))
	for i, chunk := range chunks {
		insertChunks[i] = transformChunkFields(chunk, embeddingCols)
	}

	// Delete existing rows with matching IDs
	if len(insertChunks) > 0 {
		idList := make([]string, len(insertChunks))
		for i, chunk := range insertChunks {
			idList[i] = fmt.Sprintf("'%v'", chunk["id"])
		}
		filter := fmt.Sprintf("id IN (%s)", strings.Join(idList, ", "))
		common.Debug(fmt.Sprintf("Deleting existing rows with filter: %s", filter))
		delResp, delErr := table.Delete(filter)
		if delErr != nil {
			common.Warn(fmt.Sprintf("Failed to delete existing rows: %v", delErr))
		} else {
			common.Info(fmt.Sprintf("Deleted %d existing rows", delResp.DeletedRows))
		}
	}

	// Insert chunks to dataset
	_, err = table.Insert(insertChunks)
	if err != nil {
		return nil, fmt.Errorf("Failed to insert chunks to dataset: %w", err)
	}

	common.Info("InfinityConnection.InsertChunks result", zap.String("tableName", tableName), zap.Int("count", len(insertChunks)))
	return []string{}, nil
}

// InsertMetadata inserts document metadata into tenant's metadata table
// Table name format: ragflow_doc_meta_{tenant_id}
// Auto-create the table if it doesn't exist
// Replace existing metadata with same id and kb_id
func (e *infinityEngine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	tableName := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
	common.Info("InfinityConnection.InsertMetadata called", zap.String("tableName", tableName), zap.Int("metaCount", len(metadata)))

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return nil, fmt.Errorf("Failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		// Table doesn't exist, try to create it
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "doesn't exist") {
			return nil, fmt.Errorf("Failed to get table %s: %w", tableName, err)
		}

		// Create metadata table
		if createErr := e.CreateMetadataStore(ctx, tableName); createErr != nil {
			return nil, fmt.Errorf("Failed to create metadata table: %w", createErr)
		}

		table, err = db.GetTable(tableName)
		if err != nil {
			return nil, fmt.Errorf("Failed to get table after creation: %w", err)
		}
	}

	// Transform metadata - convert meta_fields map to JSON string
	insertMetadata := make([]map[string]interface{}, len(metadata))
	for i, m := range metadata {
		d := make(map[string]interface{})
		for k, v := range m {
			if k == "meta_fields" {
				d["meta_fields"] = utility.ConvertMapToJSONString(v)
			} else {
				d[k] = v
			}
		}
		insertMetadata[i] = d
	}

	// Delete existing metadata with same id and kb_id, then insert new
	if len(insertMetadata) > 0 {
		idList := make([]string, len(insertMetadata))
		for i, m := range insertMetadata {
			// Escape single quotes in values to prevent SQL injection
			docID := fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", m["id"]), "'", "''"))
			kbID := fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", m["kb_id"]), "'", "''"))
			idList[i] = fmt.Sprintf("(id = %s AND kb_id = %s)", docID, kbID)
		}
		filter := strings.Join(idList, " OR ")
		common.Debug(fmt.Sprintf("Deleting existing metadata with filter: %s", filter))
		delResp, delErr := table.Delete(filter)
		if delErr != nil {
			common.Warn(fmt.Sprintf("Failed to delete existing metadata: %v", delErr))
		} else if delResp.DeletedRows > 0 {
			common.Info(fmt.Sprintf("Deleted %d existing metadata entries", delResp.DeletedRows))
		}
	}

	// Insert metadata
	_, err = table.Insert(insertMetadata)
	if err != nil {
		return nil, fmt.Errorf("Failed to insert metadata: %w", err)
	}

	common.Info("InfinityConnection.InsertMetadata result", zap.String("tableName", tableName), zap.Int("metaCount", len(metadata)))
	return []string{}, nil
}

// UpdateChunks updates chunks in a dataset table
// Table name format: {tblNamePrefix}_{knowledgebaseID}
func (e *infinityEngine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, tblNamePrefix string, knowledgebaseID string) error {
	tableName := fmt.Sprintf("%s_%s", tblNamePrefix, knowledgebaseID)
	common.Info("InfinityConnection.UpdateChunks called", zap.String("tableName", tableName), zap.Any("condition", condition))

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("Failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		return fmt.Errorf("Failed to get table %s: %w", tableName, err)
	}

	// Get table columns
	clmns := make(map[string]struct {
		Type    string
		Default interface{}
	})
	colsResp, err := table.ShowColumns()
	if err != nil {
		return fmt.Errorf("Failed to get columns: %w", err)
	}
	result, ok := colsResp.(*infinity.QueryResult)
	if ok {
		if nameArr, ok := result.Data["name"]; ok {
			if typeArr, ok := result.Data["type"]; ok {
				if defArr, ok := result.Data["default"]; ok {
					for i := 0; i < len(nameArr); i++ {
						colName, _ := nameArr[i].(string)
						colType, _ := typeArr[i].(string)
						var colDefault interface{}
						if i < len(defArr) {
							colDefault = defArr[i]
						}
						clmns[colName] = struct {
							Type    string
							Default interface{}
						}{colType, colDefault}
					}
				}
			}
		}
	}

	// Build filter string from condition
	filter := buildFilterFromCondition(condition, clmns)

	// Process remove operation first
	removeValue := make(map[string]interface{})
	if removeData, ok := newValue["remove"].(map[string]interface{}); ok {
		removeValue = removeData
	}
	delete(newValue, "remove")

	// Transform new_value fields using helper function (no embeddings needed for update)
	transformed := transformChunkFields(newValue, nil)
	for k, v := range transformed {
		newValue[k] = v
	}

	// Remove original fields that were transformed (they're now in transformed with new names/types)
	// Also remove intermediate token fields that shouldn't be stored in Infinity
	// This must match Python's delete list in infinity_conn.py
	for _, key := range []string{"docnm_kwd", "title_tks", "title_sm_tks", "important_kwd", "important_tks",
		"content_with_weight", "content_ltks", "content_sm_ltks", "authors_tks", "authors_sm_tks",
		"question_kwd", "question_tks"} {
		delete(newValue, key)
	}

	// Handle remove operations if any
	if len(removeValue) > 0 {
		colToRemove := make([]string, 0, len(removeValue))
		for k := range removeValue {
			colToRemove = append(colToRemove, k)
		}
		colToRemove = append(colToRemove, "id")

		// Query rows to be updated
		queryResult, err := table.Output(colToRemove).Filter(filter).ToResult()
		if err != nil {
			common.Warn(fmt.Sprintf("Failed to query rows for remove operation: %v", err))
		} else {
			qr, ok := queryResult.(*infinity.QueryResult)
			if ok && len(qr.Data) > 0 {
				// Get the id column and columns to remove
				idCol := qr.Data["id"]
				removeOpt := make(map[string]map[string][]string) // column -> value -> [ids]

				for colName, colData := range qr.Data {
					if colName == "id" {
						continue
					}
					removeVal := removeValue[colName]
					for i, id := range idCol {
						if i < len(colData) {
							existingVal := colData[i]
							if removeStr, ok := removeVal.(string); ok {
								// Split existing value by ### and remove the target value
								if existingStr, ok := existingVal.(string); ok {
									parts := strings.Split(existingStr, "###")
									var newParts []string
									for _, p := range parts {
										if p != removeStr {
											newParts = append(newParts, p)
										}
									}
									if len(newParts) != len(parts) {
										idStr := fmt.Sprintf("%v", id)
										if removeOpt[colName] == nil {
											removeOpt[colName] = make(map[string][]string)
										}
										removeOpt[colName][strings.Join(newParts, "###")] = append(removeOpt[colName][strings.Join(newParts, "###")], idStr)
									}
								}
							}
						}
					}
				}

				// Execute remove updates
				for colName, valueToIDs := range removeOpt {
					for newVal, ids := range valueToIDs {
						idFilter := filter + " AND id IN (" + strings.Join(ids, ", ") + ")"
						common.Info(fmt.Sprintf("INFINITY remove update: table=%s, idFilter=%s, column=%s, newValue=%v", tableName, idFilter, colName, newVal))
						_, err := table.Update(idFilter, map[string]interface{}{colName: newVal})
						if err != nil {
							common.Warn(fmt.Sprintf("Failed to remove value from column %s: %v", colName, err))
						}
					}
				}
			}
		}
	}

	// Execute the main update
	common.Info(fmt.Sprintf("INFINITY update: table=%s, filter=%s, newValue=%v", tableName, filter, newValue))
	_, err = table.Update(filter, newValue)
	if err != nil {
		return fmt.Errorf("Failed to update chunks: %w", err)
	}

	common.Info("InfinityConnection.UpdateChunks completes", zap.String("tableName", tableName))
	return nil
}

// UpdateMetadata updates or inserts document metadata in tenant's metadata table.
// If a row with the given docID and kbID exists, it merges the new metadata with existing.
// If no row exists, it inserts a new row.
// Table name format: ragflow_doc_meta_{tenant_id}
func (e *infinityEngine) UpdateMetadata(ctx context.Context, docID string, kbID string, metaFields map[string]interface{}, tenantID string) error {
	tableName := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
	common.Info("InfinityConnection.UpdateMetadata called", zap.String("tableName", tableName), zap.String("docID", docID), zap.String("kbID", kbID))

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		return fmt.Errorf("failed to get metadata table %s: %w", tableName, err)
	}

	// Build filter to find existing row by docID and kbID
	escapedDocID := strings.ReplaceAll(docID, "'", "''")
	escapedKbID := strings.ReplaceAll(kbID, "'", "''")
	filter := fmt.Sprintf("id = '%s' AND kb_id = '%s'", escapedDocID, escapedKbID)

	// Query existing metadata using the chainable API
	queryTable := table.Output([]string{"id", "kb_id", "meta_fields"}).Filter(filter).Limit(1).Offset(0)

	// Execute query to check if row exists
	result, err := queryTable.ToResult()
	rowExists := false
	if err != nil {
		common.Warn(fmt.Sprintf("Failed to query existing metadata: %v", err))
		// If query fails, treat as not exists and insert
	} else {
		// Get results - ToResult returns *infinity.QueryResult
		qr, ok := result.(*infinity.QueryResult)
		// Check if id column has any rows - len(qr.Data["id"]) > 0 means there are rows
		if ok && qr != nil && len(qr.Data["id"]) > 0 {
			rowExists = true
			// Get meta_fields from the first row
			if metaFieldsData, exists := qr.Data["meta_fields"]; exists && len(metaFieldsData) > 0 {
				existingMetaFieldsVal := metaFieldsData[0]

				// Parse existing meta_fields if it's a string
				var existingMetaFields map[string]interface{}
				if existingMetaFieldsVal != nil {
					switch v := existingMetaFieldsVal.(type) {
					case string:
						if err := json.Unmarshal([]byte(v), &existingMetaFields); err != nil {
							common.Warn(fmt.Sprintf("Failed to parse existing meta_fields: %v", err))
							existingMetaFields = make(map[string]interface{})
						}
					case map[string]interface{}:
						existingMetaFields = v
					}
				}

				// Merge new meta_fields with existing (new values override existing)
				if existingMetaFields == nil {
					existingMetaFields = make(map[string]interface{})
				}
				for k, v := range metaFields {
					existingMetaFields[k] = v
				}
				metaFields = existingMetaFields
			}
		}
	}

	// Prepare updated metadata as JSON string
	updatedFields := map[string]interface{}{
		"meta_fields": utility.ConvertMapToJSONString(metaFields),
	}

	if rowExists {
		// Row exists: update it with merged metadata
		common.Info(fmt.Sprintf("UpdateMetadata: updating existing row, table=%s, filter=%s, newValue=%v", tableName, filter, updatedFields))
		_, err = table.Update(filter, updatedFields)
		if err != nil {
			return fmt.Errorf("failed to update metadata: %w", err)
		}
	} else {
		// Row doesn't exist: insert new row
		insertFields := map[string]interface{}{
			"id":          docID,
			"kb_id":       kbID,
			"meta_fields": utility.ConvertMapToJSONString(metaFields),
		}
		common.Info(fmt.Sprintf("UpdateMetadata: inserting new row, table=%s, newValue=%v", tableName, insertFields))
		_, err = table.Insert(insertFields)
		if err != nil {
			return fmt.Errorf("failed to insert metadata: %w", err)
		}
	}

	common.Info("InfinityConnection.UpdateMetadata completes", zap.String("tableName", tableName), zap.String("docID", docID))
	return nil
}

// If indexName starts with "ragflow_doc_meta_", it's a metadata table.
// Otherwise, it's a dataset table: {indexName}_{datasetID}
func (e *infinityEngine) Delete(ctx context.Context, condition map[string]interface{}, indexName string, datasetID string) (int64, error) {
	var tableName string
	if strings.HasPrefix(indexName, "ragflow_doc_meta_") {
		tableName = indexName
	} else {
		tableName = fmt.Sprintf("%s_%s", indexName, datasetID)
	}

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return 0, fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		common.Warn(fmt.Sprintf("Table %s does not exist, skipping delete", tableName))
		return 0, nil
	}

	// Get table columns for building filter
	clmns := make(map[string]struct {
		Type    string
		Default interface{}
	})
	colsResp, err := table.ShowColumns()
	if err != nil {
		return 0, fmt.Errorf("failed to get columns: %w", err)
	}
	result, ok := colsResp.(*infinity.QueryResult)
	if ok {
		if nameArr, ok := result.Data["name"]; ok {
			if typeArr, ok := result.Data["type"]; ok {
				if defArr, ok := result.Data["default"]; ok {
					for i := 0; i < len(nameArr); i++ {
						colName, _ := nameArr[i].(string)
						colType, _ := typeArr[i].(string)
						var colDefault interface{}
						if i < len(defArr) {
							colDefault = defArr[i]
						}
						clmns[colName] = struct {
							Type    string
							Default interface{}
						}{colType, colDefault}
					}
				}
			}
		}
	}

	// Build filter from condition
	filter := buildFilterFromCondition(condition, clmns)

	delResp, err := table.Delete(filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete: %w", err)
	}

	return delResp.DeletedRows, nil
}

// transformChunkFields transforms chunk field name for insert/update
// It handles field name conversions and value transformations:
// - docnm_kwd -> docnm
// - title_kwd/title_sm_tks -> docnm (if docnm_kwd not set)
// - important_kwd -> important_keywords (+ important_kwd_empty_count)
// - content_with_weight/content_ltks/content_sm_ltks -> content
// - authors_tks/authors_sm_tks -> authors
// - question_kwd -> questions (joined with \n), question_tks -> questions (if question_kwd not set)
// - kb_id: list -> str (first element)
// - position_int: list -> hex_joined string
// - page_num_int, top_int: list -> hex string
// - *_feas fields -> JSON string
// - keyword fields with list values -> ### joined string
// - chunk_data: dict -> JSON string
// - Missing embeddings filled with zeros if embeddingCols provided
func transformChunkFields(chunk map[string]interface{}, embeddingCols [][2]interface{}) map[string]interface{} {
	d := make(map[string]interface{})

	for k, v := range chunk {
		switch k {
		case "docnm_kwd":
			d["docnm"] = v
		case "title_kwd":
			if _, exists := chunk["docnm_kwd"]; !exists {
				d["docnm"] = utility.ConvertToString(v)
			}
		case "title_sm_tks":
			if _, exists := chunk["docnm_kwd"]; !exists {
				d["docnm"] = utility.ConvertToString(v)
			}
		case "important_kwd":
			if list, ok := v.([]interface{}); ok {
				emptyCount := 0
				tokens := make([]string, 0)
				for _, item := range list {
					if str, ok := item.(string); ok {
						if str == "" {
							emptyCount++
						} else {
							tokens = append(tokens, str)
						}
					}
				}
				d["important_keywords"] = strings.Join(tokens, ",")
				d["important_kwd_empty_count"] = emptyCount
			} else {
				d["important_keywords"] = utility.ConvertToString(v)
			}
		case "important_tks":
			if _, exists := chunk["important_kwd"]; !exists {
				d["important_keywords"] = v
			}
		case "content_with_weight":
			d["content"] = v
		case "content_ltks":
			if _, exists := chunk["content_with_weight"]; !exists {
				d["content"] = v
			}
		case "content_sm_ltks":
			if _, exists := chunk["content_with_weight"]; !exists {
				d["content"] = v
			}
		case "authors_tks":
			d["authors"] = v
		case "authors_sm_tks":
			if _, exists := chunk["authors_tks"]; !exists {
				d["authors"] = v
			}
		case "question_kwd":
			d["questions"] = strings.Join(utility.ConvertToStringSlice(v), "\n")
		case "tag_kwd":
			d["tag_kwd"] = strings.Join(utility.ConvertToStringSlice(v), "###")
		case "question_tks":
			if _, exists := chunk["question_kwd"]; !exists {
				d["questions"] = utility.ConvertToString(v)
			}
		case "kb_id":
			if list, ok := v.([]interface{}); ok && len(list) > 0 {
				d["kb_id"] = list[0]
			} else {
				d["kb_id"] = v
			}
		case "position_int":
			if list, ok := v.([]interface{}); ok {
				d["position_int"] = utility.ConvertPositionIntArrayToHex(list)
			} else {
				d["position_int"] = v
			}
		case "page_num_int", "top_int":
			if list, ok := v.([]interface{}); ok {
				d[k] = utility.ConvertIntArrayToHex(list)
			} else {
				d[k] = v
			}
		case "chunk_data":
			d["chunk_data"] = utility.ConvertMapToJSONString(v)
		default:
			// Check for *_feas fields
			if strings.HasSuffix(k, "_feas") {
				jsonBytes, _ := json.Marshal(v)
				d[k] = string(jsonBytes)
			} else if fieldKeyword(k) {
				// keyword fields with list values -> ### joined
				if list, ok := v.([]interface{}); ok {
					d[k] = strings.Join(utility.ConvertToStringSlice(list), "###")
				} else {
					d[k] = v
				}
			} else {
				d[k] = v
			}
		}
	}

	// Remove intermediate token fields
	for _, key := range []string{"docnm_kwd", "title_tks", "title_sm_tks", "important_kwd", "important_tks",
		"content_with_weight", "content_ltks", "content_sm_ltks", "authors_tks", "authors_sm_tks",
		"question_kwd", "question_tks"} {
		delete(d, key)
	}

	// Fill missing embedding columns with zeros if embedding info provided
	for _, ec := range embeddingCols {
		name, ok1 := ec[0].(string)
		size, ok2 := ec[1].(int)
		if !ok1 || !ok2 {
			continue
		}
		if _, exists := d[name]; !exists {
			zeros := make([]float64, size)
			for i := range zeros {
				zeros[i] = 0
			}
			d[name] = zeros
		}
	}

	return d
}
