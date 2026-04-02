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

	"go.uber.org/zap"
	"ragflow/internal/logger"
)

// IndexDocument indexes a single document
func (e *infinityEngine) IndexDocument(ctx context.Context, tableName, docID string, doc interface{}) error {
	return fmt.Errorf("infinity insert not implemented: waiting for official Go SDK")
}

// BulkIndex indexes documents in bulk
func (e *infinityEngine) BulkIndex(ctx context.Context, tableName string, docs []interface{}) (interface{}, error) {
	return nil, fmt.Errorf("infinity bulk insert not implemented: waiting for official Go SDK")
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
	filter := fmt.Sprintf("id = '%s'", docID)
	resp, err := table.Delete(filter)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	logger.Debug("Deleted document from Infinity",
		zap.String("tableName", tableName),
		zap.String("docID", docID),
		zap.Uint64("deletedRows", resp.DeletedRows))

	return nil
}
