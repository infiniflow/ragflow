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
	"os"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/engine/types"
	"ragflow/internal/utility"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	infinity "github.com/infiniflow/infinity-go-sdk"
	"go.uber.org/zap"
)

// ChinesePunctRegex splits on comma, semicolon, Chinese punctuations, and newlines
var ChinesePunctRegex = regexp.MustCompile(`[,，;；、\r\n]+`)

// CreateChunkStore creates a chunk table in Infinity
// baseName is the table name prefix (e.g., "ragflow_<tenant_id>")
// The full table name is built as "{baseName}_{datasetID}"
// For skill index (datasetID="skill"), tableName is just baseName and uses skill_infinity_mapping.json
func (e *infinityEngine) CreateChunkStore(ctx context.Context, baseName, datasetID string, vectorSize int, parserID string) error {
	vecSize := vectorSize

	// Determine table name and mapping file based on index type
	var tableName string
	var mappingFile string

	tableName = buildChunkTableName(baseName, datasetID)
	if datasetID == "skill" {
		mappingFile = "skill_infinity_mapping.json"
		common.Info("Creating skill index table", zap.String("tableName", tableName), zap.String("mappingFile", mappingFile))
	} else {
		mappingFile = e.mappingFileName
		common.Info("Creating regular index table", zap.String("tableName", tableName), zap.String("mappingFile", mappingFile))
	}

	// Use configured schema
	fpMapping := filepath.Join(utility.GetProjectRoot(), "conf", mappingFile)

	schemaData, err := os.ReadFile(fpMapping)
	if err != nil {
		return fmt.Errorf("Failed to read mapping file: %w", err)
	}

	var schema orderedFields
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		return fmt.Errorf("Failed to parse mapping file: %w", err)
	}

	// Get database
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("Failed to get database: %w", err)
	}

	// Determine vector column name
	vectorColName := fmt.Sprintf("q_%d_vec", vecSize)

	// Check if table already exists
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return fmt.Errorf("Failed to check if table exists: %w", err)
	}

	var table *infinity.Table
	if exists {
		// Table exists, open it and check if vector column needs to be added
		common.Info("Table already exists, checking for vector column", zap.String("tableName", tableName))
		table, err = db.GetTable(tableName)
		if err != nil {
			return fmt.Errorf("Failed to open existing table %s: %w", tableName, err)
		}

		// Check if vector column exists (for embedding model changes)
		colExists, err := e.columnExists(table, vectorColName)
		if err != nil {
			common.Warn("Failed to check column existence", zap.String("column", vectorColName), zap.Error(err))
		}

		// Add new vector column if it doesn't exist (handles embedding model change)
		if !colExists {
			common.Info("Adding new vector column for embedding model change", zap.String("column", vectorColName), zap.Int("size", vecSize))
			addColSchema := infinity.TableSchema{
				&infinity.ColumnDefinition{
					Name:     vectorColName,
					DataType: fmt.Sprintf("vector,%d,float", vecSize),
				},
			}
			if _, err := table.AddColumns(addColSchema); err != nil {
				common.Error("Failed to add vector column "+vectorColName, err)
				return fmt.Errorf("Failed to add vector column %s: %w", vectorColName, err)
			}
			common.Info("Successfully added vector column", zap.String("column", vectorColName))
		}
	} else {
		// Table doesn't exist, create it with vector column in the initial schema
		common.Info(fmt.Sprintf("Creating table with vector column: %s with dimension %d", vectorColName, vecSize))

		// Build column definitions (preserving JSON order)
		var columns infinity.TableSchema
		for _, fieldName := range schema.Keys {
			fieldInfo := schema.Fields[fieldName]
			col := infinity.ColumnDefinition{
				Name:     fieldName,
				DataType: fieldInfo.Type,
				Default:  fieldInfo.Default,
			}
			columns = append(columns, &col)
		}

		// Add vector column
		columns = append(columns, &infinity.ColumnDefinition{
			Name:     vectorColName,
			DataType: fmt.Sprintf("vector,%d,float", vecSize),
		})

		// Add chunk_data column for table parser
		if parserID == "table" {
			columns = append(columns, &infinity.ColumnDefinition{
				Name:     "chunk_data",
				DataType: "json",
				Default:  "{}",
			})
		}

		// Create table
		table, err = db.CreateTable(tableName, columns, infinity.ConflictTypeIgnore)
		if err != nil {
			return fmt.Errorf("Failed to create table: %w", err)
		}
		common.Debug("Infinity created table", zap.String("tableName", tableName))
	}

	// Create HNSW index on vector column with unique name based on vector size
	// Use unique index name to avoid conflict when embedding model changes
	vectorIndexName := fmt.Sprintf("q_%d_vec_idx", vecSize)
	_, err = table.CreateIndex(
		vectorIndexName,
		infinity.NewIndexInfo(vectorColName, infinity.IndexTypeHnsw, map[string]string{
			"M":               "16",
			"ef_construction": "50",
			"metric":          "cosine",
			"encode":          "lvq",
		}),
		infinity.ConflictTypeIgnore,
		"",
	)
	if err != nil {
		return fmt.Errorf("Failed to create HNSW index %s: %w", vectorIndexName, err)
	}
	common.Info("Created vector index", zap.String("indexName", vectorIndexName), zap.String("column", vectorColName))

	// Create full-text indexes for varchar fields with analyzers
	for _, fieldName := range schema.Keys {
		fieldInfo := schema.Fields[fieldName]
		if fieldInfo.Type != "varchar" || fieldInfo.Analyzer == nil {
			continue
		}

		analyzers := []string{}
		switch a := fieldInfo.Analyzer.(type) {
		case string:
			analyzers = []string{a}
		case []interface{}:
			for _, v := range a {
				if s, ok := v.(string); ok {
					analyzers = append(analyzers, s)
				}
			}
		}

		for _, analyzer := range analyzers {
			indexNameFt := fmt.Sprintf("ft_%s_%s",
				regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(fieldName, "_"),
				regexp.MustCompile(`[^a-zA-Z0-9]`).ReplaceAllString(analyzer, "_"),
			)
			_, err = table.CreateIndex(
				indexNameFt,
				infinity.NewIndexInfo(fieldName, infinity.IndexTypeFullText, map[string]string{"ANALYZER": analyzer}),
				infinity.ConflictTypeIgnore,
				"",
			)
			if err != nil {
				return fmt.Errorf("Failed to create fulltext index %s: %w", indexNameFt, err)
			}
		}
	}

	// Create secondary indexes for fields with index_type
	for _, fieldName := range schema.Keys {
		fieldInfo := schema.Fields[fieldName]
		if fieldInfo.IndexType == nil {
			continue
		}

		indexTypeStr := ""
		params := map[string]string{}

		switch it := fieldInfo.IndexType.(type) {
		case string:
			indexTypeStr = it
		case map[string]interface{}:
			if t, ok := it["type"].(string); ok {
				indexTypeStr = t
			}
			if card, ok := it["cardinality"].(string); ok {
				params["cardinality"] = card
			}
		}

		if indexTypeStr == "secondary" {
			indexNameSec := fmt.Sprintf("sec_%s", fieldName)
			_, err = table.CreateIndex(
				indexNameSec,
				infinity.NewIndexInfo(fieldName, infinity.IndexTypeSecondary, params),
				infinity.ConflictTypeIgnore,
				"",
			)
			if err != nil {
				return fmt.Errorf("Failed to create secondary index %s: %w", indexNameSec, err)
			}
		}
	}

	return nil
}

// InsertChunks inserts documents into a dataset table
// Table name format: {baseName}_{datasetID}
// Auto-create the table if it doesn't exist
// Delete existing rows with matching IDs before insert
func (e *infinityEngine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName string, datasetID string) ([]string, error) {
	tableName := buildChunkTableName(baseName, datasetID)
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
		if err := e.CreateChunkStore(ctx, baseName, datasetID, vectorSize, parserID); err != nil {
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

// UpdateChunks updates chunks in a dataset table
// Table name format: {baseName}_{datasetID}
func (e *infinityEngine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, baseName string, datasetID string) error {
	tableName := buildChunkTableName(baseName, datasetID)
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
										idStr := fmt.Sprintf("'%s'", escapeFilterValue(fmt.Sprintf("%v", id)))
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

// DeleteChunks deletes chunks from a dataset table
// Table name format: {baseName}_{datasetID}
// condition specifies which chunks to delete
func (e *infinityEngine) DeleteChunks(ctx context.Context, condition map[string]interface{}, baseName string, datasetID string) (int64, error) {
	tableName := buildChunkTableName(baseName, datasetID)

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

// Search searches the Infinity engine for matching chunks.
// It supports three matching types: MatchTextExpr (full-text), MatchDenseExpr (vector), and FusionExpr (combined).
// If no match expressions are provided, Search relies solely on filter (e.g., doc_id, available_int) to find results.
func (e *infinityEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	types.LogSearchRequest("Infinity", req)

	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	// Get retrieval parameters with defaults
	pageSize := req.Limit
	if pageSize <= 0 {
		pageSize = 30
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	isSkillIndex := false
	for _, idx := range req.IndexNames {
		if strings.HasPrefix(idx, "skill_") {
			isSkillIndex = true
			break
		}
	}

	var outputColumns []string
	if isSkillIndex {
		outputColumns = []string{
			"skill_id", "space_id", "folder_id", "name", "tags", "description", "content",
			"version", "status", "create_time", "update_time",
		}
		outputColumns = convertSelectFields(outputColumns, true)
	} else {
		outputColumns = []string{
			"id", "doc_id", "kb_id", "content_ltks", "content_with_weight",
			"title_tks", "docnm_kwd", "img_id", "available_int", "important_kwd",
			"position_int", "page_num_int", "top_int", "chunk_order_int",
			"create_timestamp_flt", "knowledge_graph_kwd", "question_kwd", "question_tks",
			"doc_type_kwd", "mom_id", "tag_kwd", "pagerank_fea", "tag_feas",
		}
		outputColumns = convertSelectFields(outputColumns)
	}

	// Allow caller to override output columns (used by KG search, etc.)
	if len(req.SelectFields) > 0 {
		outputColumns = convertSelectFields(req.SelectFields)
	}

	hasTextMatch := false
	hasVectorMatch := false
	var matchText *types.MatchTextExpr
	var matchDense *types.MatchDenseExpr
	if req.MatchExprs != nil && len(req.MatchExprs) > 0 {
		for _, expr := range req.MatchExprs {
			if expr == nil {
				continue
			}
			switch e := expr.(type) {
			case string:
				if e != "" {
					hasTextMatch = true
					matchText = &types.MatchTextExpr{
						MatchingText: e,
						TopN:         pageSize,
					}
				}
			case *types.MatchTextExpr:
				if e.MatchingText != "" {
					hasTextMatch = true
					matchText = e
				}
			case *types.MatchDenseExpr:
				if len(e.EmbeddingData) > 0 {
					hasVectorMatch = true
					matchDense = e
				}
			}
		}
	}

	if hasTextMatch || hasVectorMatch {
		if hasTextMatch {
			outputColumns = append(outputColumns, "score()")
		}
		// similarity() is only allowed by Infinity when there is ONLY MATCH VECTOR.
		// When both text and vector matches exist (hybrid search with Fusion),
		// only score() is valid — Fusion produces a unified SCORE column.
		if hasVectorMatch && !hasTextMatch {
			outputColumns = append(outputColumns, "similarity()")
		}
		// Skill index does not have pagerank_fea and tag_feas columns
		if !isSkillIndex {
			if !slices.Contains(outputColumns, common.PAGERANK_FLD) {
				outputColumns = append(outputColumns, common.PAGERANK_FLD)
			}
			if !slices.Contains(outputColumns, common.TAG_FLD) {
				outputColumns = append(outputColumns, common.TAG_FLD)
			}
		}
	}

	if !slices.Contains(outputColumns, "row_id") && !slices.Contains(outputColumns, "row_id()") {
		outputColumns = append(outputColumns, "row_id()")
	}

	// Strip score pseudo-columns when there's no match expression — Infinity
	// rejects SCORE()/SCORE_FACTORS() without MATCH TEXT/TENSOR/Fusion with
	// "InfinityException(3013)". This protects callers (e.g. the no-match
	// fallback in retrieval.go) that reuse a SelectFields list containing
	// "_score" across both matched and unmatched queries.
	if !hasTextMatch && !hasVectorMatch {
		filtered := outputColumns[:0]
		for _, c := range outputColumns {
			switch c {
			case "_score", "SCORE", "score()", "similarity()":
				continue
			}
			filtered = append(filtered, c)
		}
		outputColumns = filtered
	}

	outputColumns = convertSelectFields(outputColumns, isSkillIndex)
	if hasVectorMatch && matchDense != nil && matchDense.VectorColumnName != "" {
		outputColumns = append(outputColumns, matchDense.VectorColumnName)
	}

	var filterParts []string

	if hasTextMatch || hasVectorMatch {
		if req.Filter != nil {
			if availInt, ok := req.Filter["available_int"]; ok {
				filterParts = append(filterParts, fmt.Sprintf("available_int=%v", availInt))
			} else if status, ok := req.Filter["status"]; ok {
				filterParts = append(filterParts, fmt.Sprintf("status='%s'", status))
			} else {
				if isSkillIndex {
					filterParts = append(filterParts, "status='1'")
				} else {
					filterParts = append(filterParts, "available_int=1")
				}
			}
		} else {
			if isSkillIndex {
				filterParts = append(filterParts, "status='1'")
			} else {
				filterParts = append(filterParts, "available_int=1")
			}
		}
	}

	// Build filter string from req.Filter
	if req.Filter != nil {
		filterCopy := make(map[string]interface{})
		for k, v := range req.Filter {
			if k != "kb_id" {
				filterCopy[k] = v
			}
		}

		condStr := equivalentConditionToStr(filterCopy)
		if condStr != "" {
			filterParts = append(filterParts, condStr)
		}
	}
	filterStr := strings.Join(filterParts, " AND ")

	orderBy := req.OrderBy
	var rankFeature map[string]float64
	if req.RankFeature != nil {
		rankFeature = req.RankFeature
	}

	var fusionExpr *types.FusionExpr
	if len(req.MatchExprs) > 2 {
		if fe, ok := req.MatchExprs[2].(*types.FusionExpr); ok {
			fusionExpr = fe
		}
	}

	var allResults []map[string]interface{}
	totalHits := int64(0)

	for _, indexName := range req.IndexNames {
		var tableNames []string
		if strings.HasPrefix(indexName, "ragflow_doc_meta_") {
			tableNames = []string{indexName}
		} else {
			kbIDs := req.KbIDs
			if len(kbIDs) == 0 {
				kbIDs = []string{""}
			}
			for _, kbID := range kbIDs {
				if kbID == "" {
					tableNames = append(tableNames, indexName)
				} else {
					tableNames = append(tableNames, fmt.Sprintf("%s_%s", indexName, kbID))
				}
			}
		}

		// minMatch comes from matchText.ExtraOptions when set (Python parity).
		// Mirrors rag/utils/infinity_conn.py which reads
		// matchExpr.extra_options.get("minimum_should_match", 0.0) — for the
		// English (non-Chinese) path, the Go Question() builder omits
		// minimum_should_match, so the default is 0.0 to match Python's
		// effective 0% threshold for English queries.
		minMatch := 0.0
		var questionText string
		var vectorData []float64
		textTopN := pageSize
		var originalQuery string
		if matchText != nil {
			questionText = matchText.MatchingText
			textTopN = int(matchText.TopN)
			if matchText.ExtraOptions != nil {
				if oq, ok := matchText.ExtraOptions["original_query"].(string); ok {
					originalQuery = oq
				}
				if v, ok := matchText.ExtraOptions["minimum_should_match"]; ok {
					switch x := v.(type) {
					case float64:
						minMatch = x
					case int:
						minMatch = float64(x)
					case string:
						s := strings.TrimSuffix(x, "%")
						if pct, err := strconv.Atoi(s); err == nil {
							minMatch = float64(pct) / 100
						}
					}
				}
			}
		}
		if matchDense != nil {
			vectorData = matchDense.EmbeddingData
		}

		for _, tableName := range tableNames {
			tbl, err := db.GetTable(tableName)
			if err != nil {
				continue
			}
			table := tbl.Output(outputColumns)

			var textFields []string
			if matchText != nil && len(matchText.Fields) > 0 {
				textFields = matchText.Fields
			} else if isSkillIndex {
				textFields = []string{
					"name^10",
					"tags^5",
					"description^3",
					"content^1",
				}
			} else {
				textFields = []string{
					"title_tks^10",
					"title_sm_tks^5",
					"important_kwd^30",
					"important_tks^20",
					"question_tks^20",
					"content_ltks^2",
					"content_sm_ltks",
				}
			}

			// Convert field names for Infinity
			var convertedFields []string
			for _, f := range textFields {
				cf := convertMatchingField(f)
				convertedFields = append(convertedFields, cf)
			}
			fields := strings.Join(convertedFields, ",")

			hasTextMatch := questionText != ""
			hasVectorMatch := len(vectorData) > 0
			// Add text match if question is provided
			if hasTextMatch {
				extraOptions := map[string]string{
					"minimum_should_match": fmt.Sprintf("%d%%", int(minMatch*100)),
				}

				if filterStr != "" {
					extraOptions["filter"] = filterStr
				}

				if rankFeature != nil {
					var rankFeaturesList []string
					for featureName, weight := range rankFeature {
						rankFeaturesList = append(rankFeaturesList, fmt.Sprintf("%s^%s^%.0f", common.TAG_FLD, featureName, weight))
					}
					if len(rankFeaturesList) > 0 {
						extraOptions["rank_features"] = strings.Join(rankFeaturesList, ",")
					}
				}

				if originalQuery != "" {
					extraOptions["original_query"] = originalQuery
				}

				table = table.MatchText(fields, questionText, textTopN, extraOptions)

				common.Debug(fmt.Sprintf(
					"MatchTextExpr:\n"+
						"    fields=%s\n"+
						"    matching_text=%s\n"+
						"    topn=%d\n"+
						"    extra_options=%v",
					fields, questionText, textTopN, extraOptions,
				))
			}

			// Add vector match if provided
			if hasVectorMatch {
				vecFieldName := fmt.Sprintf("q_%d_vec", len(vectorData))
				dataType := "float"
				distanceType := "cosine"

				if matchDense != nil {
					if matchDense.VectorColumnName != "" {
						vecFieldName = matchDense.VectorColumnName
					}
					if matchDense.EmbeddingDataType != "" {
						dataType = matchDense.EmbeddingDataType
					}
					if matchDense.DistanceType != "" {
						distanceType = matchDense.DistanceType
					}
				}

				vectorTopN := pageSize
				if matchDense != nil && matchDense.TopN > 0 {
					vectorTopN = int(matchDense.TopN)
				}

				denseFilterStr := filterStr
				if denseFilterStr == "" {
					if isSkillIndex {
						denseFilterStr = "status='1'"
					} else {
						denseFilterStr = "available_int=1"
					}
				}

				if hasTextMatch {
					fieldsStr := strings.Join(convertedFields, ",")
					filterFulltext := fmt.Sprintf("filter_fulltext('%s', '%s')", fieldsStr, questionText)
					denseFilterStr = fmt.Sprintf("(%s) AND %s", denseFilterStr, filterFulltext)
				}
				threshold := "0.0"
				if matchDense != nil && matchDense.ExtraOptions != nil {
					if sim, ok := matchDense.ExtraOptions["similarity"].(float64); ok {
						threshold = fmt.Sprintf("%g", sim)
					} else if s, ok := matchDense.ExtraOptions["threshold"].(string); ok {
						threshold = s
					}
				}
				extraOptions := map[string]string{
					"threshold": threshold,
					"filter":    denseFilterStr,
				}

				common.Debug("MatchDense for hybrid search",
					zap.String("fieldName", vecFieldName),
					zap.String("distanceType", distanceType),
					zap.Int("topN", vectorTopN),
					zap.Bool("hasFusion", fusionExpr != nil))

				table = table.MatchDense(vecFieldName, vectorData, dataType, distanceType, vectorTopN, extraOptions)
			}

			// Add fusion (for text + vector combination)
			if hasTextMatch && hasVectorMatch && fusionExpr != nil {
				fusionMethod := fusionExpr.Method
				fusionTopK := fusionExpr.TopN
				if fusionTopK == 0 {
					fusionTopK = pageSize
				}
				fusionParams := map[string]interface{}{
					"normalize": "atan",
				}
				if fusionExpr.FusionParams != nil {
					for k, v := range fusionExpr.FusionParams {
						fusionParams[k] = v
					}
				}

				common.Debug("Applying Fusion for hybrid search",
					zap.String("method", fusionMethod),
					zap.Int("topN", fusionTopK),
					zap.Any("params", fusionParams))

				table = table.Fusion(fusionMethod, fusionTopK, fusionParams)
			}

			// Add order_by if provided
			if orderBy != nil && len(orderBy.Fields) > 0 {
				var sortFields [][2]interface{}
				for _, orderField := range orderBy.Fields {
					sortType := infinity.SortTypeAsc
					if orderField.Type == types.SortDesc {
						sortType = infinity.SortTypeDesc
					}
					sortFields = append(sortFields, [2]interface{}{orderField.Field, sortType})
				}
				table = table.Sort(sortFields)
			}

			// Add filter when there's no text/vector match (like metadata queries)
			if !hasTextMatch && !hasVectorMatch && filterStr != "" {
				common.Debug(fmt.Sprintf("Adding filter for no-match query: %s", filterStr))
				table = table.Filter(filterStr)
			}

			// Set limit and offset
			table = table.Limit(pageSize)
			if offset > 0 {
				table = table.Offset(offset)
			}

			// Request total_hits_count from Infinity
			table = table.Option(map[string]interface{}{"total_hits_count": true})

			// Execute query
			df, err := table.ToDataFrame()
			if err != nil {
				common.Warn("Infinity query failed",
					zap.String("tableName", tableName),
					zap.Bool("hasTextMatch", hasTextMatch),
					zap.Bool("hasVectorMatch", hasVectorMatch),
					zap.Bool("hasFusion", fusionExpr != nil),
					zap.Error(err))
				continue
			}

			// Convert DataFrame to chunks format (column-oriented to row-oriented)
			searchChunks := make([]map[string]interface{}, 0)
			for colName, colData := range df.ColumnData {
				for i, val := range colData {
					for len(searchChunks) <= i {
						searchChunks = append(searchChunks, make(map[string]interface{}))
					}
					searchChunks[i][colName] = val
				}
			}

			// Apply field name mapping and row_id handling
			// Skill index uses different schema
			// so we skip the document-specific field mappings
			if !isSkillIndex {
				applyFieldMappings(searchChunks)
			} else {
				// For skill index, only handle ROW_ID -> row_id() mapping
				for _, chunk := range searchChunks {
					if val, ok := chunk["ROW_ID"]; ok {
						chunk["row_id()"] = val
						delete(chunk, "ROW_ID")
					}
				}
			}

			// Parse total_hits_count from ExtraInfo
			var tableTotal int64
			if df.ExtraInfo != "" {
				var extraResult map[string]interface{}
				if err := json.Unmarshal([]byte(df.ExtraInfo), &extraResult); err == nil {
					if count, ok := extraResult["total_hits_count"].(float64); ok {
						tableTotal = int64(count)
					}
				}
			}

			searchResult := &types.SearchResult{
				Chunks: searchChunks,
				Total:  tableTotal,
			}

			allResults = append(allResults, searchResult.Chunks...)
			totalHits += searchResult.Total
		}
	}

	if hasTextMatch || hasVectorMatch {
		scoreColumn := ""
		if hasTextMatch && hasVectorMatch {
			scoreColumn = "SCORE"
		} else if hasTextMatch {
			scoreColumn = "SCORE"
		} else if hasVectorMatch {
			scoreColumn = "SIMILARITY"
		}
		pagerankField := common.PAGERANK_FLD
		if isSkillIndex {
			pagerankField = "" // Skill index has no pagerank field
		}

		allResults = calculateScores(allResults, scoreColumn, pagerankField)
		allResults = sortByScore(allResults, len(allResults))
	}

	if len(allResults) > pageSize {
		allResults = allResults[:pageSize]
	}

	common.Debug("Search in Infinity completed", zap.Int("returnedRows", len(allResults)), zap.Int64("totalHits", totalHits))

	return &types.SearchResult{
		Chunks: allResults,
		Total:  totalHits,
	}, nil
}

// GetChunk gets a chunk by ID
func (e *infinityEngine) GetChunk(ctx context.Context, tableName, chunkID string, datasetIDs []string) (interface{}, error) {
	if e.client == nil || e.client.conn == nil {
		return nil, fmt.Errorf("Infinity client not initialized")
	}

	common.Info("Infinity get chunk start",
		zap.String("chunkID", chunkID),
		zap.String("tableName", tableName),
		zap.Strings("datasetIDs", datasetIDs))

	// Build list of table names to search
	tableNames := make([]string, 0, len(datasetIDs))
	for _, datasetID := range datasetIDs {
		tableNames = append(tableNames, fmt.Sprintf("%s_%s", tableName, datasetID))
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
						if _, has := existing[k]; !has || (utility.IsEmpty(existing[k]) && !utility.IsEmpty(v)) {
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

// applyFieldMappings applies field mappings to chunks (side-effect only).
// Used by Search() to mutate chunks with derived fields before returning.
func applyFieldMappings(chunks []map[string]interface{}) {
	for _, chunk := range chunks {
		// docnm -> docnm_kwd, title_tks, title_sm_tks
		if val, ok := chunk["docnm"].(string); ok {
			chunk["docnm_kwd"] = val
			chunk["title_tks"] = val
			chunk["title_sm_tks"] = val
		}

		// important_keywords -> important_kwd (split by comma/semicolon/Chinese punctuations), important_tks
		if val, ok := chunk["important_keywords"].(string); ok {
			if val == "" {
				chunk["important_kwd"] = []interface{}{}
			} else {
				parts := ChinesePunctRegex.Split(val, -1)
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
			"important_kwd", "important_tks", "question_tks",
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
	}
}

// GetFields extracts the requested fields from Infinity search results
func (e *infinityEngine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})

	// Python: if not fields, return {}
	if len(fields) == 0 {
		return result
	}

	if len(chunks) == 0 {
		return result
	}

	// Build field set for lookup (Python lines 713-715)
	fieldsAll := make(map[string]bool)
	for _, f := range fields {
		fieldsAll[f] = true
	}
	fieldsAll["id"] = true

	// noneColumns is rebuilt per chunk inside the loop below. The
	// per-chunk "missing → nil" map MUST be fresh for every iteration; if
	// it's reused, the first chunk that contains a field removes it from
	// the shared set, and later chunks missing that same field silently
	// stop getting the nil placeholder, producing inconsistent shapes
	// per document.

	// Check if important_kwd is needed (for empty_count handling)
	needImportantKwdEmptyCount := fieldsAll["important_kwd"]

	for _, chunk := range chunks {
		// Build column map for case-insensitive lookup (Python line 747)
		columnMap := make(map[string]string)
		for k := range chunk {
			columnMap[strings.ToLower(k)] = k
		}

		// Apply field mappings first (to get derived fields)
		// docnm -> docnm_kwd, title_tks, title_sm_tks (Python lines 716-719)
		// Note: Python checks "docnm" in res.columns regardless of whether fields were requested
		if val, ok := chunk["docnm"].(string); ok {
			if fieldsAll["docnm_kwd"] {
				chunk["docnm_kwd"] = val
			}
			if fieldsAll["title_tks"] {
				chunk["title_tks"] = val
			}
			if fieldsAll["title_sm_tks"] {
				chunk["title_sm_tks"] = val
			}
		}

		// important_keywords -> important_kwd (split by comma), important_tks (Python lines 720-732)
		// Python: v.split(",") if v else [] — empty string yields empty list
		if fieldsAll["important_kwd"] || fieldsAll["important_tks"] {
			if val, ok := chunk["important_keywords"].(string); ok && val != "" {
				if fieldsAll["important_kwd"] {
					if needImportantKwdEmptyCount {
						// Check for important_kwd_empty_count (Python lines 722-728)
						if emptyCountVal, hasEmptyCount := chunk["important_kwd_empty_count"]; hasEmptyCount {
							tokens := strings.Split(val, ",")
							var emptyCount int
							switch v := emptyCountVal.(type) {
							case float64:
								emptyCount = int(v)
							case int:
								emptyCount = v
							case string:
								emptyCount, _ = strconv.Atoi(v)
							}
							kwdList := make([]interface{}, 0, len(tokens)+emptyCount)
							for _, t := range tokens {
								kwdList = append(kwdList, t)
							}
							for i := 0; i < emptyCount; i++ {
								kwdList = append(kwdList, "")
							}
							chunk["important_kwd"] = kwdList
						} else {
							parts := strings.Split(val, ",")
							kwdList := make([]interface{}, len(parts))
							for i, p := range parts {
								kwdList[i] = p
							}
							chunk["important_kwd"] = kwdList
						}
					} else {
						parts := strings.Split(val, ",")
						kwdList := make([]interface{}, len(parts))
						for i, p := range parts {
							kwdList[i] = p
						}
						chunk["important_kwd"] = kwdList
					}
				}
				if fieldsAll["important_tks"] {
					chunk["important_tks"] = val
				}
			} else {
				if fieldsAll["important_kwd"] {
					chunk["important_kwd"] = []interface{}{}
				}
				if fieldsAll["important_tks"] {
					chunk["important_tks"] = []interface{}{}
				}
			}
		}

		// questions -> question_kwd (split by newline), question_tks (Python lines 733-737)
		// Python: v.splitlines() — empty string yields empty list
		if fieldsAll["question_kwd"] || fieldsAll["question_tks"] {
			if val, ok := chunk["questions"].(string); ok && val != "" {
				if fieldsAll["question_kwd"] {
					parts := strings.Split(val, "\n")
					qList := make([]interface{}, len(parts))
					for i, p := range parts {
						qList[i] = p
					}
					chunk["question_kwd"] = qList
				}
				if fieldsAll["question_tks"] {
					chunk["question_tks"] = val
				}
			} else {
				if fieldsAll["question_kwd"] {
					chunk["question_kwd"] = []interface{}{}
				}
				if fieldsAll["question_tks"] {
					chunk["question_tks"] = []interface{}{}
				}
			}
		}

		// content -> content_with_weight, content_ltks, content_sm_ltks (Python lines 738-741)
		if fieldsAll["content_with_weight"] || fieldsAll["content_ltks"] || fieldsAll["content_sm_ltks"] {
			if val, ok := chunk["content"].(string); ok {
				if fieldsAll["content_with_weight"] {
					chunk["content_with_weight"] = val
				}
				if fieldsAll["content_ltks"] {
					chunk["content_ltks"] = val
				}
				if fieldsAll["content_sm_ltks"] {
					chunk["content_sm_ltks"] = val
				}
			}
		}

		// authors -> authors_tks, authors_sm_tks (Python lines 742-745)
		if fieldsAll["authors_tks"] || fieldsAll["authors_sm_tks"] {
			if val, ok := chunk["authors"].(string); ok {
				if fieldsAll["authors_tks"] {
					chunk["authors_tks"] = val
				}
				if fieldsAll["authors_sm_tks"] {
					chunk["authors_sm_tks"] = val
				}
			}
		}

		// Post-process fields matching Python lines 758-780
		// This single loop processes all column transformations in Python order
		kwdNoSplit := map[string]bool{
			"knowledge_graph_kwd": true, "docnm_kwd": true,
			"important_kwd": true, "question_kwd": true,
		}
		for field, val := range chunk {
			fieldLower := strings.ToLower(field)

			// field_keyword: split by "###" (Python lines 760-761)
			needsSplit := false
			if fieldLower == "source_id" {
				needsSplit = true
			} else if strings.HasSuffix(fieldLower, "_kwd") && !kwdNoSplit[fieldLower] {
				needsSplit = true
			}
			if needsSplit {
				if strVal, ok := val.(string); ok && strings.Contains(strVal, "###") {
					parts := strings.Split(strVal, "###")
					var filtered []interface{}
					for _, p := range parts {
						if p != "" {
							filtered = append(filtered, p)
						}
					}
					chunk[field] = filtered
				}
				continue
			}

			// _feas: JSON parse (Python lines 762-763)
			if strings.HasSuffix(fieldLower, "_feas") {
				if strVal, ok := val.(string); ok && strVal != "" {
					var parsed interface{}
					if err := json.Unmarshal([]byte(strVal), &parsed); err == nil {
						chunk[field] = parsed
					}
				} else {
					chunk[field] = map[string]interface{}{}
				}
				continue
			}

			// chunk_data: JSON parse (Python lines 764-766)
			if fieldLower == "chunk_data" {
				if strVal, ok := val.(string); ok && strVal != "" {
					var parsed interface{}
					if err := json.Unmarshal([]byte(strVal), &parsed); err == nil {
						chunk[field] = parsed
					}
				} else if val == nil {
					// Keep nil
				}
				continue
			}

			// position_int: hex decode with grouping by 5 (Python lines 767-776)
			if fieldLower == "position_int" && fieldsAll[fieldLower] {
				// If already converted to slice by applyFieldMappings, skip
				if _, isSlice := val.([]interface{}); isSlice {
					continue
				}
				// applyFieldMappings returns [][]int, check that too
				if _, isIntSlice := val.([][]int); isIntSlice {
					continue
				}
				if strVal, ok := val.(string); ok && strVal != "" {
					chunk[field] = utility.ConvertHexToPositionIntArray(strVal)
				} else {
					chunk[field] = []interface{}{}
				}
				continue
			}

			// page_num_int, top_int: hex decode (Python lines 777-778)
			if (fieldLower == "page_num_int" || fieldLower == "top_int") && fieldsAll[fieldLower] {
				// If already converted to slice by applyFieldMappings, skip
				if _, isSlice := val.([]interface{}); isSlice {
					continue
				}
				// applyFieldMappings returns []int, check that too
				if _, isIntSlice := val.([]int); isIntSlice {
					continue
				}
				if strVal, ok := val.(string); ok && strVal != "" {
					chunk[field] = utility.ConvertHexToIntArray(strVal)
				} else {
					chunk[field] = []interface{}{}
				}
				continue
			}
		}

		// Handle row_id mapping (Python lines 748-750)
		if fieldsAll["row_id()"] {
			if lowerKey, ok := columnMap["row_id"]; ok {
				chunk["row_id()"] = chunk[lowerKey]
			}
		}

		// Delete base columns after mapping (Python lines 781-783)
		for _, col := range []string{"docnm", "important_keywords", "questions", "content", "authors"} {
			delete(chunk, col)
		}

		// Build result map keyed by id
		if idVal, ok := chunk["id"].(string); ok {
			fieldMap := make(map[string]interface{})
			// Rebuild noneColumns for this chunk so that fields missing
			// from THIS chunk get a nil placeholder. Reusing a set across
			// chunks would let the first chunk's contents permanently
			// remove keys, leaving later chunks with inconsistent shapes.
			noneColumns := make(map[string]bool, len(fieldsAll))
			for f := range fieldsAll {
				noneColumns[strings.ToLower(f)] = true
			}
			for field, value := range chunk {
				if fieldsAll[field] {
					fieldMap[field] = value
					delete(noneColumns, strings.ToLower(field))
				}
			}
			// Set none_columns to None (Python lines 784-785)
			for col := range noneColumns {
				fieldMap[col] = nil
			}
			result[idVal] = fieldMap
		}
	}

	return result
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

// GetChunkIDs extracts chunk IDs from Infinity search results.
func (e *infinityEngine) GetChunkIDs(chunks []map[string]interface{}) []string {
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

	// For Infinity, scores are already returned in search results (_score column)
	// So GetScores just extracts scores from chunks, mimicking Python's approach
	return result
}

// KNNScores for Infinity - since Infinity normalizes scores during fusion,
// we just need to return a result structure that GetScores can parse.
// This matches Python's approach where Infinity doesn't use the two-pass KNN.
func (e *infinityEngine) KNNScores(ctx context.Context, chunks []map[string]interface{}, queryVector []float64, topK int) (map[string]interface{}, error) {
	if len(chunks) == 0 {
		return nil, nil
	}

	// Build a result structure that GetScores can parse
	// For Infinity, scores are already in _score field from the first search
	result := make(map[string]interface{})
	hitList := make([]interface{}, 0, len(chunks))
	for _, chunk := range chunks {
		if id, ok := chunk["id"].(string); ok {
			hit := map[string]interface{}{
				"_id":    id,
				"_score": chunk["_score"],
			}
			hitList = append(hitList, hit)
		}
	}
	result["hits"] = map[string]interface{}{
		"hits": hitList,
	}
	return result, nil
}

// GetScores extracts similarity scores from KNN search result.
// For Infinity, it parses the result from KNNScores and extracts _score values.
func (e *infinityEngine) GetScores(knnResult map[string]interface{}) map[string]float64 {
	scores := make(map[string]float64)
	hits, ok := knnResult["hits"].(map[string]interface{})
	if !ok {
		return scores
	}
	hitList, ok := hits["hits"].([]interface{})
	if !ok {
		return scores
	}
	for _, h := range hitList {
		hit, ok := h.(map[string]interface{})
		if !ok {
			continue
		}
		docID, ok := hit["_id"].(string)
		if !ok {
			continue
		}
		scoreVal := hit["_score"]
		if scoreVal == nil {
			scores[docID] = 0.0
			continue
		}
		score, ok := scoreVal.(float64)
		if !ok {
			scores[docID] = 0.0
			continue
		}
		scores[docID] = score
	}
	return scores
}

// convertSelectFields converts field names to Infinity format
// isSkillIndex indicates if this is a skill index (uses skill_id instead of id)
//
// Does NOT mutate the input slice — callers (e.g. retrieval.go) reuse the same
// SelectFields list both for Search() and GetFields(); mutating it would
// replace logical names like "content_with_weight" with their Infinity column
// names ("content"), breaking GetFields's field-presence checks.
func convertSelectFields(output []string, isSkillIndex ...bool) []string {
	fieldMapping := map[string]string{
		"docnm_kwd":           "docnm",
		"title_tks":           "docnm",
		"title_sm_tks":        "docnm",
		"important_kwd":       "important_keywords",
		"important_tks":       "important_keywords",
		"question_kwd":        "questions",
		"question_tks":        "questions",
		"content_with_weight": "content",
		"content_ltks":        "content",
		"content_sm_ltks":     "content",
		"authors_tks":         "authors",
		"authors_sm_tks":      "authors",
	}

	skillIndex := false
	if len(isSkillIndex) > 0 {
		skillIndex = isSkillIndex[0]
	}

	// Copy + map without mutating the caller's slice.
	mapped := make([]string, len(output))
	needEmptyCount := false
	for i, field := range output {
		if field == "important_kwd" {
			needEmptyCount = true
		}
		if newField, ok := fieldMapping[field]; ok {
			mapped[i] = newField
		} else {
			mapped[i] = field
		}
	}

	// Remove duplicates
	seen := make(map[string]bool)
	result := []string{}
	for _, f := range mapped {
		if f != "" && !seen[f] {
			seen[f] = true
			result = append(result, f)
		}
	}

	// Add id and empty count if needed
	// For skill index, use skill_id instead of id
	hasID := false
	idField := "id"
	if skillIndex {
		idField = "skill_id"
	}
	for _, f := range result {
		if f == idField {
			hasID = true
			break
		}
	}
	if !hasID {
		result = append([]string{idField}, result...)
	}

	if needEmptyCount {
		result = append(result, "important_kwd_empty_count")
	}

	return result
}

// convertMatchingField converts field names for matching
// For regular document indices: maps _tks/_kwd fields to column@index_name format
// For skill indices: maps raw field names to column@index_name format
// Infinity requires column@index_name when a column has multiple full-text indexes
func convertMatchingField(fieldWeightStr string) string {
	// Split on ^ to get field name
	parts := strings.Split(fieldWeightStr, "^")
	field := parts[0]

	// Field name conversion
	fieldMapping := map[string]string{
		"docnm_kwd":           "docnm@ft_docnm_rag_coarse",
		"title_tks":           "docnm@ft_docnm_rag_coarse",
		"title_sm_tks":        "docnm@ft_docnm_rag_fine",
		"important_kwd":       "important_keywords@ft_important_keywords_rag_coarse",
		"important_tks":       "important_keywords@ft_important_keywords_rag_fine",
		"question_kwd":        "questions@ft_questions_rag_coarse",
		"question_tks":        "questions@ft_questions_rag_fine",
		"content_with_weight": "content@ft_content_rag_coarse",
		"content_ltks":        "content@ft_content_rag_coarse",
		"content_sm_ltks":     "content@ft_content_rag_fine",
		"authors_tks":         "authors@ft_authors_rag_coarse",
		"authors_sm_tks":      "authors@ft_authors_rag_fine",
		"tag_kwd":             "tag_kwd@ft_tag_kwd_whitespace__",
		// Skill index fields
		"name":        "name@ft_name_rag_coarse",
		"tags":        "tags@ft_tags_rag_coarse",
		"description": "description@ft_description_rag_coarse",
		"content":     "content@ft_content_rag_coarse",
	}

	if newField, ok := fieldMapping[field]; ok {
		parts[0] = newField
	}

	return strings.Join(parts, "^")
}

// escapeFilterValue escapes single quotes for filter values
func escapeFilterValue(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// equivalentConditionToStr converts a condition map to an Infinity filter string
func equivalentConditionToStr(condition map[string]interface{}) string {
	if len(condition) == 0 {
		return ""
	}

	var cond []string

	for k, v := range condition {
		if k == "_id" || utility.IsEmpty(v) {
			continue
		}

		// Handle must_not specially
		if k == "must_not" {
			if m, ok := v.(map[string]interface{}); ok {
				for kk, vv := range m {
					if kk == "exists" {
						// For must_not exists, use !='' since we don't have table schema
						cond = append(cond, fmt.Sprintf("NOT (%v!='')", vv))
					}
				}
			}
			continue
		}

		// Handle exists specially (without table schema, use string comparison)
		if k == "exists" {
			cond = append(cond, fmt.Sprintf("%v!=''", v))
			continue
		}

		// Handle keyword fields (using full-text filter)
		if fieldKeyword(k) {
			// For keyword fields, values are always treated as strings for filter_fulltext
			switch val := v.(type) {
			case []string:
				var inCond []string
				for _, item := range val {
					inCond = append(inCond, fmt.Sprintf("filter_fulltext('%s', '%s')",
						convertMatchingField(k), escapeFilterValue(item)))
				}
				if len(inCond) > 0 {
					cond = append(cond, "("+strings.Join(inCond, " or ")+")")
				}
			case []interface{}:
				var inCond []string
				for _, item := range val {
					if s, ok := item.(string); ok {
						inCond = append(inCond, fmt.Sprintf("filter_fulltext('%s', '%s')",
							convertMatchingField(k), escapeFilterValue(s)))
					} else {
						inCond = append(inCond, fmt.Sprintf("filter_fulltext('%s', '%s')",
							convertMatchingField(k), escapeFilterValue(fmt.Sprintf("%v", item))))
					}
				}
				if len(inCond) > 0 {
					cond = append(cond, "("+strings.Join(inCond, " or ")+")")
				}
			case string:
				cond = append(cond, fmt.Sprintf("filter_fulltext('%s', '%s')",
					convertMatchingField(k), escapeFilterValue(val)))
			default:
				cond = append(cond, fmt.Sprintf("filter_fulltext('%s', '%s')",
					convertMatchingField(k), escapeFilterValue(fmt.Sprintf("%v", v))))
			}
			continue
		}

		// Handle list values (mixed types - strings get quotes, numbers don't)
		if list, ok := v.([]interface{}); ok && len(list) > 0 {
			var strItems, numItems []string
			for _, item := range list {
				if s, ok := item.(string); ok {
					strItems = append(strItems, fmt.Sprintf("'%s'", escapeFilterValue(s)))
				} else if n, ok := item.(int); ok {
					numItems = append(numItems, strconv.Itoa(n))
				} else if n, ok := item.(int64); ok {
					numItems = append(numItems, strconv.FormatInt(n, 10))
				} else if f, ok := item.(float64); ok {
					numItems = append(numItems, strconv.FormatFloat(f, 'f', -1, 64))
				} else if s, ok := item.(fmt.Stringer); ok {
					strItems = append(strItems, fmt.Sprintf("'%s'", escapeFilterValue(s.String())))
				} else {
					strItems = append(strItems, fmt.Sprintf("'%s'", escapeFilterValue(fmt.Sprintf("%v", item))))
				}
			}
			if len(strItems) > 0 {
				if len(strItems) == 1 {
					cond = append(cond, fmt.Sprintf("%s=%s", k, strItems[0]))
				} else {
					cond = append(cond, fmt.Sprintf("%s IN (%s)", k, strings.Join(strItems, ", ")))
				}
			}
			if len(numItems) > 0 {
				if len(numItems) == 1 {
					cond = append(cond, fmt.Sprintf("%s=%s", k, numItems[0]))
				} else {
					cond = append(cond, fmt.Sprintf("%s IN (%s)", k, strings.Join(numItems, ", ")))
				}
			}
			continue
		}

		if list, ok := v.([]string); ok && len(list) > 0 {
			if len(list) == 1 {
				cond = append(cond, fmt.Sprintf("%s='%s'", k, escapeFilterValue(list[0])))
			} else {
				var items []string
				for _, item := range list {
					items = append(items, fmt.Sprintf("'%s'", escapeFilterValue(item)))
				}
				cond = append(cond, fmt.Sprintf("%s IN (%s)", k, strings.Join(items, ", ")))
			}
			continue
		}

		if list, ok := v.([]int); ok && len(list) > 0 {
			if len(list) == 1 {
				cond = append(cond, fmt.Sprintf("%s=%d", k, list[0]))
			} else {
				var strs []string
				for _, n := range list {
					strs = append(strs, strconv.Itoa(n))
				}
				cond = append(cond, fmt.Sprintf("%s IN (%s)", k, strings.Join(strs, ", ")))
			}
			continue
		}

		// Handle numeric values (no quotes)
		if utility.IsNumericValue(v) {
			cond = append(cond, fmt.Sprintf("%s=%v", k, v))
			continue
		}

		// Handle string values (with quotes and escaping)
		if str, ok := v.(string); ok {
			cond = append(cond, fmt.Sprintf("%s='%s'", k, escapeFilterValue(str)))
			continue
		}

		// Fallback: treat as string
		cond = append(cond, fmt.Sprintf("%s='%s'", k, escapeFilterValue(fmt.Sprintf("%v", v))))
	}

	if len(cond) == 0 {
		return ""
	}
	return strings.Join(cond, " AND ")
}

// calculateScores calculates _score = score_column + pagerank
func calculateScores(chunks []map[string]interface{}, scoreColumn, pagerankField string) []map[string]interface{} {
	for i := range chunks {
		score := 0.0
		if scoreVal, ok := chunks[i][scoreColumn]; ok {
			if f, ok := utility.ToFloat64(scoreVal); ok {
				score += f
			}
		}
		if pagerankField != "" {
			if prVal, ok := chunks[i][pagerankField]; ok {
				if f, ok := utility.ToFloat64(prVal); ok {
					score += f
				}
			}
		}
		chunks[i]["_score"] = score
	}
	return chunks
}

// sortByScore sorts by _score descending and limits
func sortByScore(chunks []map[string]interface{}, limit int) []map[string]interface{} {
	if len(chunks) == 0 {
		return chunks
	}

	// Sort by _score descending
	sort.Slice(chunks, func(i, j int) bool {
		scoreI := getChunkScore(chunks[i])
		scoreJ := getChunkScore(chunks[j])
		return scoreI > scoreJ
	})

	// Limit
	if len(chunks) > limit && limit > 0 {
		chunks = chunks[:limit]
	}

	return chunks
}

// getChunkScore extracts the score from a chunk
func getChunkScore(chunk map[string]interface{}) float64 {
	if v, ok := chunk["_score"].(float64); ok {
		return v
	}
	if v, ok := chunk["SCORE"].(float64); ok {
		return v
	}
	if v, ok := chunk["SIMILARITY"].(float64); ok {
		return v
	}
	return 0.0
}

// transformChunkFields converts chunk field names to Infinity format.
// Converts internal field names (like docnm_kwd) to Infinity column names (docnm).
// Also handles:
// - kb_id: extracts first element if it's a list
// - position_int, page_num_int, top_int: converts arrays to hex strings
// - tag_kwd: joins with ### separator
// - question_kwd: joins with newline separator
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

// DropChunkStore drops a chunk table from Infinity
func (e *infinityEngine) DropChunkStore(ctx context.Context, baseName, datasetID string) error {
	return e.dropTable(ctx, buildChunkTableName(baseName, datasetID))
}

// ChunkStoreExists checks if a chunk table exists in Infinity
func (e *infinityEngine) ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error) {
	return e.tableExists(ctx, buildChunkTableName(baseName, datasetID))
}
