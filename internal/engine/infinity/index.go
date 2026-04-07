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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	infinity "github.com/infiniflow/infinity-go-sdk"
	"ragflow/internal/logger"
	"ragflow/internal/utility"

	"go.uber.org/zap"
)

// fieldInfo represents a field in the infinity mapping schema
type fieldInfo struct {
	Type      string      `json:"type"`
	Default  interface{} `json:"default"`
	Analyzer interface{} `json:"analyzer"`  // string or []string
	IndexType interface{} `json:"index_type"` // string or map
	Comment  string      `json:"comment"`
}

// orderedFields preserves the order of fields as defined in JSON
type orderedFields struct {
	Keys   []string
	Fields map[string]fieldInfo
}

func (o *orderedFields) UnmarshalJSON(data []byte) error {
	// Parse JSON manually to preserve key order
	// Look for key names by scanning the JSON string
	// This is a simple approach: find {"key": value, "key2": value2...}
	o.Fields = make(map[string]fieldInfo)
	o.Keys = make([]string, 0)

	// Use a streaming JSON parser approach
	dec := json.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		return err
	}
	if delim, ok := tok.(json.Delim); ok && delim == '{' {
		for dec.More() {
			// Read key
			tok, err := dec.Token()
			if err != nil {
				return err
			}
			key, ok := tok.(string)
			if !ok {
				continue
			}
			o.Keys = append(o.Keys, key)

			// Read value into fieldInfo
			var field fieldInfo
			if err := dec.Decode(&field); err != nil {
				return err
			}
			o.Fields[key] = field
		}
	}
	return nil
}

// CreateIndex creates a table/index in Infinity
// indexName is the table name prefix (e.g., "ragflow_<tenant_id>")
// The full table name is built as "{indexName}_{datasetID}"
// For skill index (datasetID="skill"), tableName is just indexName and uses skill_infinity_mapping.json
func (e *infinityEngine) CreateIndex(ctx context.Context, indexName, datasetID string, vectorSize int, parserID string) error {
	vecSize := vectorSize

	// Determine table name and mapping file based on index type
	var tableName string
	var mappingFile string

	if datasetID == "skill" {
		// Skill index: table name is just indexName (e.g., "skill_abc123_def456")
		tableName = indexName
		mappingFile = "skill_infinity_mapping.json"
		logger.Info("Creating skill index table", zap.String("tableName", tableName), zap.String("mappingFile", mappingFile))
	} else {
		// Regular document index: table name is {indexName}_{datasetID}
		tableName = fmt.Sprintf("%s_%s", indexName, datasetID)
		mappingFile = e.mappingFileName
		logger.Info("Creating regular index table", zap.String("tableName", tableName), zap.String("mappingFile", mappingFile))
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

	// Build column definitions (preserving JSON order)
	var columns infinity.TableSchema
	for _, fieldName := range schema.Keys {
		fieldInfo := schema.Fields[fieldName]
		col := infinity.ColumnDefinition{
			Name:     fieldName,
			DataType: fieldInfo.Type,
			Default:  fieldInfo.Default,
			// Comment:  fieldInfo.Comment,
		}
		columns = append(columns, &col)
	}

	// Add vector column
	vectorColName := fmt.Sprintf("q_%d_vec", vecSize)
	logger.Info(fmt.Sprintf("Creating vector column: %s with dimension %d", vectorColName, vecSize))
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
	table, err := db.CreateTable(tableName, columns, infinity.ConflictTypeIgnore)
	if err != nil {
		return fmt.Errorf("Failed to create table: %w", err)
	}
	logger.Debug("Infinity created table", zap.String("tableName", tableName))

	// Create HNSW index on vector column
	_, err = table.CreateIndex(
		"q_vec_idx",
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
		return fmt.Errorf("Failed to create HNSW index: %w", err)
	}

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

	_ = table // suppress unused variable warning
	return nil
}

// DeleteIndex deletes a table/index
func (e *infinityEngine) DeleteIndex(ctx context.Context, indexName string) error {
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("Failed to get database: %w", err)
	}

	_, err = db.DropTable(indexName, infinity.ConflictTypeIgnore)
	if err != nil {
		return fmt.Errorf("Failed to drop table: %w", err)
	}
	logger.Debug("Infinity deleted table", zap.String("tableName", indexName))
	return nil
}

// IndexExists checks if table/index exists
func (e *infinityEngine) IndexExists(ctx context.Context, indexName string) (bool, error) {
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return false, fmt.Errorf("Failed to get database: %w", err)
	}

	// Try to list all tables first for diagnostic purposes
	tables, listErr := db.ListTables()
	if listErr != nil {
		logger.Warn("Failed to list tables", zap.Error(listErr))
	} else {
		if tablesStr, ok := tables.([]string); ok {
			logger.Info("Infinity database tables", zap.Strings("tables", tablesStr), zap.String("checkingFor", indexName))
		} else {
			logger.Info("Infinity database tables", zap.Any("tables", tables), zap.String("checkingFor", indexName))
		}
	}

	_, err = db.GetTable(indexName)
	if err != nil {
		// Check if error is "table not found"
		errStr := err.Error()
		logger.Info("IndexExists GetTable error", zap.String("indexName", indexName), zap.String("error", errStr))
		// Add more error patterns that Infinity might return
		if strings.Contains(errStr, "not found") ||
			strings.Contains(errStr, "NotFound") ||
			strings.Contains(errStr, "doesn't exist") ||
			strings.Contains(errStr, "does not exist") ||
			strings.Contains(errStr, "not exist") ||
			strings.Contains(errStr, "Not exist") ||
			strings.Contains(errStr, "NOT_EXIST") ||
			(strings.Contains(errStr, "Table") && strings.Contains(errStr, "exist")) {
			return false, nil
		}
		return false, err
	}
	logger.Info("IndexExists: table found", zap.String("indexName", indexName))
	return true, nil
}

// CreateDocMetaIndex creates the document metadata table/index
func (e *infinityEngine) CreateDocMetaIndex(ctx context.Context, indexName string) error {
	// Get database
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("Failed to get database: %w", err)
	}

	// Use configured doc_meta mapping file
	fpMapping := filepath.Join(utility.GetProjectRoot(), "conf", e.docMetaMappingFileName)

	schemaData, err := os.ReadFile(fpMapping)
	if err != nil {
		return fmt.Errorf("Failed to read mapping file: %w", err)
	}

	var schema map[string]fieldInfo
	if err := json.Unmarshal(schemaData, &schema); err != nil {
		return fmt.Errorf("Failed to parse mapping file: %w", err)
	}

	// Build column definitions
	var columns infinity.TableSchema
	for fieldName, fieldInfo := range schema {
		col := infinity.ColumnDefinition{
			Name:    fieldName,
			DataType: fieldInfo.Type,
			Default: fieldInfo.Default,
			// Comment: fieldInfo.Comment,
		}
		columns = append(columns, &col)
	}

	// Create table
	_, err = db.CreateTable(indexName, columns, infinity.ConflictTypeIgnore)
	if err != nil {
		return fmt.Errorf("Failed to create doc meta table: %w", err)
	}
	logger.Debug("Infinity created doc meta table", zap.String("tableName", indexName))

	// Get table for creating indexes
	table, err := db.GetTable(indexName)
	if err != nil {
		return fmt.Errorf("Failed to get table: %w", err)
	}

	// Create secondary index on id
	_, err = table.CreateIndex(
		fmt.Sprintf("idx_%s_id", indexName),
		infinity.NewIndexInfo("id", infinity.IndexTypeSecondary, nil),
		infinity.ConflictTypeIgnore,
		"",
	)
	if err != nil {
		return fmt.Errorf("Failed to create secondary index on id: %w", err)
	}

	// Create secondary index on kb_id
	_, err = table.CreateIndex(
		fmt.Sprintf("idx_%s_kb_id", indexName),
		infinity.NewIndexInfo("kb_id", infinity.IndexTypeSecondary, nil),
		infinity.ConflictTypeIgnore,
		"",
	)
	if err != nil {
		return fmt.Errorf("Failed to create secondary index on kb_id: %w", err)
	}

	return nil
}

// TransformChunkFields transforms chunk field name for insert/update
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
func TransformChunkFields(chunk map[string]interface{}, embeddingCols [][2]interface{}) map[string]interface{} {
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
		name, size := ec[0].(string), ec[1].(int)
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

// existsCondition builds a NOT EXISTS or field!='' condition
func existsCondition(field string, tableColumns map[string]struct {
	Type    string
	Default interface{}
}) string {
	col, colOk := tableColumns[field]
	if !colOk {
		logger.Warn(fmt.Sprintf("Column '%s' not found in table columns", field))
		return fmt.Sprintf("%s!=null", field)
	}
	if strings.Contains(strings.ToLower(col.Type), "char") {
		if col.Default != nil {
			return fmt.Sprintf(" %s!='%v' ", field, col.Default)
		}
		return fmt.Sprintf(" %s!='' ", field)
	}
	if col.Default != nil {
		return fmt.Sprintf("%s!=%v", field, col.Default)
	}
	return fmt.Sprintf("%s!=null", field)
}

func buildFilterFromCondition(condition map[string]interface{}, tableColumns map[string]struct {
	Type    string
	Default interface{}
}) string {
	var conditions []string

	for k, v := range condition {
		if v == nil || v == "" {
			continue
		}

		// Handle must_not conditions -> NOT (...)
		if k == "must_not" {
			if mustNotMap, ok := v.(map[string]interface{}); ok {
				for kk, vv := range mustNotMap {
					if kk == "exists" {
						if existsField, ok := vv.(string); ok {
							conditions = append(conditions, fmt.Sprintf("NOT (%s)", existsCondition(existsField, tableColumns)))
						}
					}
				}
			}
			continue
		}

		// Handle keyword fields -> filter_fulltext with converted field name
		if fieldKeyword(k) {
			if listVal, ok := v.([]interface{}); ok {
				var orConds []string
				for _, item := range listVal {
					if strItem, ok := item.(string); ok {
						strItem = strings.ReplaceAll(strItem, "'", "''")
						orConds = append(orConds, fmt.Sprintf("filter_fulltext('%s', '%s')", convertMatchingField(k), strItem))
					}
				}
				if len(orConds) > 0 {
					conditions = append(conditions, "("+strings.Join(orConds, " OR ")+")")
				}
			} else if strVal, ok := v.(string); ok {
				strVal = strings.ReplaceAll(strVal, "'", "''")
				conditions = append(conditions, fmt.Sprintf("filter_fulltext('%s', '%s')", convertMatchingField(k), strVal))
			}
			continue
		}

		// Handle list values (IN condition)
		if listVal, ok := v.([]interface{}); ok {
			var inVals []string
			for _, item := range listVal {
				if strItem, ok := item.(string); ok {
					strItem = strings.ReplaceAll(strItem, "'", "''")
					inVals = append(inVals, fmt.Sprintf("'%s'", strItem))
				} else {
					inVals = append(inVals, fmt.Sprintf("%v", item))
				}
			}
			if len(inVals) > 0 {
				conditions = append(conditions, fmt.Sprintf("%s IN (%s)", k, strings.Join(inVals, ", ")))
			}
			continue
		}

		// Handle exists condition
		if k == "exists" {
			if existsField, ok := v.(string); ok {
				conditions = append(conditions, existsCondition(existsField, tableColumns))
			}
			continue
		}

		// Handle string values
		if strVal, ok := v.(string); ok {
			strVal = strings.ReplaceAll(strVal, "'", "''")
			conditions = append(conditions, fmt.Sprintf("%s='%s'", k, strVal))
			continue
		}

		// Handle other values
		conditions = append(conditions, fmt.Sprintf("%s=%v", k, v))
	}

	if len(conditions) == 0 {
		return "1=1"
	}
	return strings.Join(conditions, " AND ")
}

// InsertDataset inserts chunks into a dataset table
// Table name format: {tableNamePrefix}_{knowledgebaseID}
// Auto-create the table if it doesn't exist
// Delete existing rows with matching IDs before insert
func (e *infinityEngine) InsertDataset(ctx context.Context, chunks []map[string]interface{}, tableNamePrefix string, knowledgebaseID string) ([]string, error) {
	tableName := fmt.Sprintf("%s_%s", tableNamePrefix, knowledgebaseID)
	logger.Info("InfinityConnection.InsertDataset called", zap.String("tableName", tableName), zap.Int("chunkCount", len(chunks)))

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
		if err := e.CreateIndex(ctx, tableNamePrefix, knowledgebaseID, vectorSize, parserID); err != nil {
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
		insertChunks[i] = TransformChunkFields(chunk, embeddingCols)
	}

	// Delete existing rows with matching IDs
	if len(insertChunks) > 0 {
		idList := make([]string, len(insertChunks))
		for i, chunk := range insertChunks {
			idList[i] = fmt.Sprintf("'%v'", chunk["id"])
		}
		filter := fmt.Sprintf("id IN (%s)", strings.Join(idList, ", "))
		logger.Debug(fmt.Sprintf("Deleting existing rows with filter: %s", filter))
		delResp, delErr := table.Delete(filter)
		if delErr != nil {
			logger.Warn(fmt.Sprintf("Failed to delete existing rows: %v", delErr))
		} else {
			logger.Info(fmt.Sprintf("Deleted %d existing rows", delResp.DeletedRows))
		}
	}

	// Insert chunks to dataset
	_, err = table.Insert(insertChunks)
	if err != nil {
		return nil, fmt.Errorf("Failed to insert chunks to dataset: %w", err)
	}

	logger.Info("InfinityConnection.InsertDataset result", zap.String("tableName", tableName), zap.Int("count", len(insertChunks)))
	return []string{}, nil
}

// UpdateDataset updates chunks in a dataset table
// Table name format: {tableNamePrefix}_{knowledgebaseID}
func (e *infinityEngine) UpdateDataset(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, tableNamePrefix string, knowledgebaseID string) error {
	tableName := fmt.Sprintf("%s_%s", tableNamePrefix, knowledgebaseID)
	logger.Info("InfinityConnection.UpdateDataset called", zap.String("tableName", tableName), zap.Any("condition", condition))

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
	transformed := TransformChunkFields(newValue, nil)
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
			logger.Warn(fmt.Sprintf("Failed to query rows for remove operation: %v", err))
		} else {
			qr, ok := queryResult.(*infinity.QueryResult)
			if ok && len(qr.Data) > 0 {
				// Get the id column and columns to remove
				idCol := qr.Data["id"]
				removeOpt := make(map[string]map[string][]string); // column -> value -> [ids]

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
						logger.Info(fmt.Sprintf("INFINITY remove update: table=%s, idFilter=%s, column=%s, newValue=%v", tableName, idFilter, colName, newVal))
						_, err := table.Update(idFilter, map[string]interface{}{colName: newVal})
						if err != nil {
							logger.Warn(fmt.Sprintf("Failed to remove value from column %s: %v", colName, err))
						}
					}
				}
			}
		}
	}

	// Execute the main update
	logger.Info(fmt.Sprintf("INFINITY update: table=%s, filter=%s, newValue=%v", tableName, filter, newValue))
	_, err = table.Update(filter, newValue)
	if err != nil {
		return fmt.Errorf("Failed to update chunks: %w", err)
	}

	logger.Info("InfinityConnection.UpdateDataset completes", zap.String("tableName", tableName))
	return nil
}

// InsertMetadata inserts document metadata into tenant's metadata table
// Table name format: ragflow_doc_meta_{tenant_id}
// Auto-create the table if it doesn't exist
// Replace existing metadata with same id and kb_id
func (e *infinityEngine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	tableName := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
	logger.Info("InfinityConnection.InsertMetadata called", zap.String("tableName", tableName), zap.Int("metaCount", len(metadata)))

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
		if createErr := e.CreateDocMetaIndex(ctx, tableName); createErr != nil {
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
			docID := fmt.Sprintf("'%v'", m["id"])
			kbID := fmt.Sprintf("'%v'", m["kb_id"])
			idList[i] = fmt.Sprintf("(id = %s AND kb_id = %s)", docID, kbID)
		}
		filter := strings.Join(idList, " OR ")
		logger.Debug(fmt.Sprintf("Deleting existing metadata with filter: %s", filter))
		delResp, delErr := table.Delete(filter)
		if delErr != nil {
			logger.Warn(fmt.Sprintf("Failed to delete existing metadata: %v", delErr))
		} else if delResp.DeletedRows > 0 {
			logger.Info(fmt.Sprintf("Deleted %d existing metadata entries", delResp.DeletedRows))
		}
	}

	// Insert metadata
	_, err = table.Insert(insertMetadata)
	if err != nil {
		return nil, fmt.Errorf("Failed to insert metadata: %w", err)
	}

	logger.Info("InfinityConnection.InsertMetadata result", zap.String("tableName", tableName), zap.Int("metaCount", len(metadata)))
	return []string{}, nil
}

// UpdateMetadata updates document metadata in tenant's metadata table
// Table name format: ragflow_doc_meta_{tenant_id}
func (e *infinityEngine) UpdateMetadata(ctx context.Context, docID string, kbID string, metaFields map[string]interface{}, tenantID string) error {
	tableName := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
	logger.Info("InfinityConnection.UpdateMetadata called", zap.String("tableName", tableName), zap.String("docID", docID), zap.String("kbID", kbID))

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		return fmt.Errorf("failed to get metadata table %s: %w", tableName, err)
	}

	// Query existing metadata using the chainable API
	filter := fmt.Sprintf("id = '%s' AND kb_id = '%s'", docID, kbID)

	// Use chainable API: Output().Filter().Limit().Offset()
	queryTable := table.Output([]string{"id", "kb_id", "meta_fields"}).Filter(filter).Limit(1).Offset(0)

	// Execute query
	result, err := queryTable.ToResult()
	if err != nil {
		logger.Warn(fmt.Sprintf("Failed to query existing metadata: %v", err))
		// If query fails, just insert new metadata
	} else {
		// Get results
		rows, ok := result.([]map[string]interface{})
		if ok && len(rows) > 0 {
			existingMetaFieldsVal := rows[0]["meta_fields"]

			// Parse existing meta_fields if it's a string
			var existingMetaFields map[string]interface{}
			if existingMetaFieldsVal != nil {
				switch v := existingMetaFieldsVal.(type) {
				case string:
					if err := json.Unmarshal([]byte(v), &existingMetaFields); err != nil {
						logger.Warn(fmt.Sprintf("Failed to parse existing meta_fields: %v", err))
						existingMetaFields = make(map[string]interface{})
					}
				case map[string]interface{}:
					existingMetaFields = v
				}
			}

			// Merge new meta_fields with existing
			if existingMetaFields == nil {
				existingMetaFields = make(map[string]interface{})
			}
			for k, v := range metaFields {
				existingMetaFields[k] = v
			}
			metaFields = existingMetaFields
		}
	}

	// Prepare updated metadata
	updatedFields := map[string]interface{}{
		"meta_fields": utility.ConvertMapToJSONString(metaFields),
	}

	// Update metadata
	logger.Info(fmt.Sprintf("INFINITY metadata update: table=%s, filter=%s, newValue=%v", tableName, filter, updatedFields))
	_, err = table.Update(filter, updatedFields)
	if err != nil {
		return fmt.Errorf("failed to update metadata: %w", err)
	}

	logger.Info("InfinityConnection.UpdateMetadata completes", zap.String("tableName", tableName), zap.String("docID", docID))
	return nil
}
