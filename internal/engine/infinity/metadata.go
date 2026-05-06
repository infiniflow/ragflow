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
	"strings"

	"ragflow/internal/utility"

	infinity "github.com/infiniflow/infinity-go-sdk"

	"go.uber.org/zap"
)

// CreateMetadata creates the document metadata table/index
func (e *infinityEngine) CreateMetadata(ctx context.Context, indexName string) error {
	// Get database
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("Failed to get database: %w", err)
	}

	// Check if table already exists
	exists, err := e.TableExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("Failed to check if table exists: %w", err)
	}
	if exists {
		return fmt.Errorf("metadata table '%s' already exists", indexName)
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
			// Comment: fieldInfo.Comment,
		}
		columns = append(columns, &col)
	}

	// Create table
	_, err = db.CreateTable(indexName, columns, infinity.ConflictTypeIgnore)
	if err != nil {
		return fmt.Errorf("Failed to create doc meta table: %w", err)
	}
	common.Debug("Infinity created doc meta table", zap.String("tableName", indexName))

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
		if createErr := e.CreateMetadata(ctx, tableName); createErr != nil {
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
