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

package opengauss

import (
	"context"
	"fmt"
	"strings"
)

// IndexDocument indexes a skill document. Regular chunks use InsertChunks.
func (e *Engine) IndexDocument(ctx context.Context, indexName, docID string, doc interface{}) error {
	indexName = physicalTableName(indexName)
	if err := validateIdentifier(indexName); err != nil {
		return err
	}
	if !strings.HasPrefix(indexName, "skill_") {
		return fmt.Errorf("IndexDocument is supported only for skill tables")
	}
	document, ok := doc.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid document type %T", doc)
	}
	if stringValue(document["skill_id"]) == "" {
		document = copyMap(document)
		document["skill_id"] = docID
	}
	_, err := e.InsertChunks(ctx, []map[string]interface{}{document}, indexName, "skill", "")
	return err
}

// BulkIndex indexes skill documents in a single transaction.
func (e *Engine) BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error) {
	indexName = physicalTableName(indexName)
	if err := validateIdentifier(indexName); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(indexName, "skill_") {
		return nil, fmt.Errorf("BulkIndex is supported only for skill tables")
	}
	rows := make([]map[string]interface{}, 0, len(docs))
	for _, raw := range docs {
		row, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid skill document type %T", raw)
		}
		rows = append(rows, row)
	}
	if _, err := e.InsertChunks(ctx, rows, indexName, "skill", ""); err != nil {
		return nil, err
	}
	return map[string]interface{}{"inserted": len(rows)}, nil
}

// DeleteDocument deletes a document by its exact primary key.
func (e *Engine) DeleteDocument(ctx context.Context, indexName, docID string) error {
	indexName = physicalTableName(indexName)
	if err := validateIdentifier(indexName); err != nil {
		return err
	}
	if strings.HasPrefix(indexName, "skill_") {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE skill_id = $1", quoteIdent(indexName)), docID)
		return err
	}
	_, err := e.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1", quoteIdent(indexName)), docID)
	return err
}
