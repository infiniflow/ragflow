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

	"go.uber.org/zap"
	"ragflow/internal/logger"
)

// IndexDocument indexes a single document
// For skill index (tableName starts with "skill_"), uses InsertSkill
// For regular document index, returns not implemented error
func (e *infinityEngine) IndexDocument(ctx context.Context, tableName, docID string, doc interface{}) error {
	// Check if this is a skill index
	if strings.HasPrefix(tableName, "skill_") {
		return e.InsertSkill(ctx, tableName, docID, doc)
	}
	return fmt.Errorf("infinity insert not implemented for regular documents: waiting for official Go SDK")
}

// InsertSkill inserts a skill document into skill index
// Auto-creates the table if it doesn't exist
func (e *infinityEngine) InsertSkill(ctx context.Context, tableName, docID string, doc interface{}) error {
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		// Table doesn't exist, try to create it
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "doesn't exist") {
			return fmt.Errorf("failed to get table %s: %w", tableName, err)
		}

		// Cannot auto-create skill table without knowing the vector dimension
		// The table should be created by SkillIndexerService.EnsureIndex before calling this
		return fmt.Errorf("skill table %s does not exist, please ensure index is initialized first", tableName)
	}

	// Transform doc to map
	docMap, ok := doc.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid doc type, expected map[string]interface{}")
	}

	// Prepare insert data
	insertDoc := make(map[string]interface{})
	for k, v := range docMap {
		insertDoc[k] = v
	}
	// Ensure skill_id is set (schema uses skill_id, not id)
	insertDoc["skill_id"] = docID

	// Delete existing document with same skill_id
	filter := fmt.Sprintf("skill_id = '%s'", docID)
	delResp, delErr := table.Delete(filter)
	if delErr != nil {
		logger.Warn(fmt.Sprintf("Failed to delete existing skill document: %v", delErr))
	} else if delResp.DeletedRows > 0 {
		logger.Debug(fmt.Sprintf("Deleted %d existing skill document(s)", delResp.DeletedRows))
	}

	// Insert the document
	_, err = table.Insert([]map[string]interface{}{insertDoc})
	if err != nil {
		return fmt.Errorf("failed to insert skill document into %s: %w", tableName, err)
	}
	return nil
}

// BulkIndex indexes documents in bulk
// For skill index (tableName starts with "skill_"), uses BulkInsertSkill
// For regular document index, returns not implemented error
func (e *infinityEngine) BulkIndex(ctx context.Context, tableName string, docs []interface{}) (interface{}, error) {
	// Check if this is a skill index
	if strings.HasPrefix(tableName, "skill_") {
		inserted, err := e.BulkInsertSkill(ctx, tableName, docs)
		return &BulkResponse{Inserted: inserted}, err
	}
	return nil, fmt.Errorf("infinity bulk insert not implemented for regular documents: waiting for official Go SDK")
}

// BulkInsertSkill inserts multiple skill documents in bulk
func (e *infinityEngine) BulkInsertSkill(ctx context.Context, tableName string, docs []interface{}) (int, error) {
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return 0, fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		return 0, fmt.Errorf("failed to get table %s: %w", tableName, err)
	}

	// Transform docs to maps
	insertDocs := make([]map[string]interface{}, 0, len(docs))
	for _, doc := range docs {
		docMap, ok := doc.(map[string]interface{})
		if !ok {
			logger.Warn("Invalid doc type in bulk insert, expected map[string]interface{}")
			continue
		}
		// Ensure skill_id is set if id or skill_id exists in doc
		if docID, hasID := docMap["id"]; hasID {
			docMap["skill_id"] = docID
		}
		insertDocs = append(insertDocs, docMap)
	}

	if len(insertDocs) == 0 {
		return 0, nil
	}

	// Insert the documents
	_, err = table.Insert(insertDocs)
	if err != nil {
		return 0, fmt.Errorf("failed to bulk insert skill documents: %w", err)
	}

	logger.Debug("Bulk inserted skill documents",
		zap.String("tableName", tableName),
		zap.Int("count", len(insertDocs)))
	return len(insertDocs), nil
}

// BulkResponse bulk operation response
type BulkResponse struct {
	Inserted int
}

// GetDocument gets a document
func (e *infinityEngine) GetDocument(ctx context.Context, tableName, docID string) (interface{}, error) {
	return nil, fmt.Errorf("infinity get document not implemented: waiting for official Go SDK")
}

// DeleteDocument deletes a document by ID
func (e *infinityEngine) DeleteDocument(ctx context.Context, tableName, docID string) error {
	if tableName == "" {
		return fmt.Errorf("table name cannot be empty")
	}
	if docID == "" {
		return fmt.Errorf("document id cannot be empty")
	}

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		return fmt.Errorf("failed to get table: %w", err)
	}

	// Use filter to delete document by ID
	// Skill index uses 'skill_id', regular indices use 'id'
	idField := "id"
	if strings.HasPrefix(tableName, "skill_") {
		idField = "skill_id"
	}
	filter := fmt.Sprintf("%s = '%s'", idField, docID)
	resp, err := table.Delete(filter)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	logger.Debug("Deleted document from Infinity",
		zap.String("tableName", tableName),
		zap.String("docID", docID),
		zap.String("idField", idField),
		zap.Int64("deletedRows", resp.DeletedRows))

	return nil
}
