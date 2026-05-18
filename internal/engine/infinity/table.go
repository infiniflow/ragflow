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
	"regexp"
	"strings"

	infinity "github.com/infiniflow/infinity-go-sdk"
	"ragflow/internal/common"
	"ragflow/internal/utility"

	"go.uber.org/zap"
)

// CreateChunkStore creates a chunk table in Infinity
// indexName is the table name prefix (e.g., "ragflow_<tenant_id>")
// The full table name is built as "{indexName}_{datasetID}"
// For skill index (datasetID="skill"), tableName is just indexName and uses skill_infinity_mapping.json
func (e *infinityEngine) CreateChunkStore(ctx context.Context, indexName, datasetID string, vectorSize int, parserID string) error {
	vecSize := vectorSize

	// Determine table name and mapping file based on index type
	var tableName string
	var mappingFile string

	if datasetID == "skill" {
		// Skill index: table name is just indexName (e.g., "skill_abc123_def456")
		tableName = indexName
		mappingFile = "skill_infinity_mapping.json"
		common.Info("Creating skill index table", zap.String("tableName", tableName), zap.String("mappingFile", mappingFile))
	} else {
		// Regular document index: table name is {indexName}_{datasetID}
		tableName = fmt.Sprintf("%s_%s", indexName, datasetID)
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
	exists, err := e.StoreExists(ctx, tableName)
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

// CreateMetadataStore creates a metadata table in Infinity
// tenantID is the tenant identifier used to build the table name
// The full table name is built as "ragflow_doc_meta_{tenantID}"
func (e *infinityEngine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	tableName := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)

	// Get database
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("Failed to get database: %w", err)
	}

	// Check if table already exists
	exists, err := e.StoreExists(ctx, tableName)
	if err != nil {
		return fmt.Errorf("Failed to check if table exists: %w", err)
	}
	if exists {
		return fmt.Errorf("metadata table '%s' already exists", tableName)
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
			Name:     fieldName,
			DataType: fieldInfo.Type,
			Default:  fieldInfo.Default,
		}
		columns = append(columns, &col)
	}

	// Create table
	_, err = db.CreateTable(tableName, columns, infinity.ConflictTypeIgnore)
	if err != nil {
		return fmt.Errorf("Failed to create doc meta table: %w", err)
	}
	common.Debug("Infinity created doc meta table", zap.String("tableName", tableName))

	// Get table for creating indexes
	table, err := db.GetTable(tableName)
	if err != nil {
		return fmt.Errorf("Failed to get table: %w", err)
	}

	// Create secondary index on id
	_, err = table.CreateIndex(
		fmt.Sprintf("idx_%s_id", tableName),
		infinity.NewIndexInfo("id", infinity.IndexTypeSecondary, nil),
		infinity.ConflictTypeIgnore,
		"",
	)
	if err != nil {
		return fmt.Errorf("Failed to create secondary index on id: %w", err)
	}

	// Create secondary index on kb_id
	_, err = table.CreateIndex(
		fmt.Sprintf("idx_%s_kb_id", tableName),
		infinity.NewIndexInfo("kb_id", infinity.IndexTypeSecondary, nil),
		infinity.ConflictTypeIgnore,
		"",
	)
	if err != nil {
		return fmt.Errorf("Failed to create secondary index on kb_id: %w", err)
	}

	return nil
}

// DropStore drops a table from Infinity
func (e *infinityEngine) DropStore(ctx context.Context, tableName string) error {
	if tableName == "" {
		return fmt.Errorf("table name cannot be empty")
	}

	// Check if table exists
	exists, err := e.StoreExists(ctx, tableName)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("table '%s' does not exist", tableName)
	}

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}

	_, err = db.DropTable(tableName, infinity.ConflictTypeError)
	if err != nil {
		return fmt.Errorf("failed to drop table: %w", err)
	}

	common.Info("Infinity dropped table", zap.String("tableName", tableName))
	return nil
}

// StoreExists checks if a table exists in Infinity
func (e *infinityEngine) StoreExists(ctx context.Context, tableName string) (bool, error) {
	if tableName == "" {
		return false, fmt.Errorf("table name cannot be empty")
	}

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return false, fmt.Errorf("failed to get database: %w", err)
	}

	// Try to get the table - if it exists, no error
	_, err = db.GetTable(tableName)
	if err != nil {
		errMsg := strings.ToLower(err.Error())
		if strings.Contains(errMsg, "not found") || strings.Contains(errMsg, "doesn't exist") {
			return false, nil
		}
		return false, fmt.Errorf("failed to check table existence: %w", err)
	}
	return true, nil
}
