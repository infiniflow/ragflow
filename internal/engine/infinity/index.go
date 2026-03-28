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
func (e *infinityEngine) CreateIndex(ctx context.Context, indexName, datasetID string, vectorSize int, parserID string) error {
	vecSize := vectorSize

	// Build full table name: {indexName}_{datasetID}
	tableName := fmt.Sprintf("%s_%s", indexName, datasetID)

	// Use configured schema
	fpMapping := filepath.Join(utility.GetProjectRoot(), "conf", e.mappingFileName)

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

	_, err = db.GetTable(indexName)
	if err != nil {
		// Check if error is "table not found"
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}
		return false, err
	}
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
