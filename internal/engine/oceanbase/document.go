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

package oceanbase

import (
	"context"
	"fmt"
	"strings"
)

// IndexDocument indexes a skill document. Regular chunks use InsertChunks.
func (e *Engine) IndexDocument(ctx context.Context, indexName, docID string, doc interface{}) error {
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
	ready, err := e.ChunkStoreExists(ctx, indexName, "skill")
	if err != nil {
		return err
	}
	vectorSize := vectorDimension(document)
	if !ready {
		if err := e.CreateChunkStore(ctx, indexName, "skill", vectorSize, ""); err != nil {
			return err
		}
	} else if vectorSize > 0 {
		if err := e.ensureVectorColumnAndIndex(ctx, indexName, vectorSize, "ob_"); err != nil {
			return err
		}
	}
	normalized, err := normalizeSkill(document, docID)
	if err != nil {
		return err
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := replaceRow(ctx, tx, indexName, normalized); err != nil {
		return err
	}
	return tx.Commit()
}

// BulkIndex indexes skill documents in a single transaction.
func (e *Engine) BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error) {
	if err := validateIdentifier(indexName); err != nil {
		return nil, err
	}
	if !strings.HasPrefix(indexName, "skill_") {
		return nil, fmt.Errorf("BulkIndex is supported only for skill tables")
	}
	vectorSize := 0
	for _, raw := range docs {
		if document, ok := raw.(map[string]interface{}); ok {
			docID := stringValue(document["skill_id"])
			if docID == "" {
				docID = stringValue(document["id"])
			}
			if docID == "" {
				return nil, fmt.Errorf("document identifier cannot be empty")
			}
			if vectorSize == 0 {
				vectorSize = vectorDimension(document)
			}
		}
	}
	ready, err := e.ChunkStoreExists(ctx, indexName, "skill")
	if err != nil {
		return nil, err
	}
	if !ready {
		if err := e.CreateChunkStore(ctx, indexName, "skill", vectorSize, ""); err != nil {
			return nil, err
		}
	} else if vectorSize > 0 {
		if err := e.ensureVectorColumnAndIndex(ctx, indexName, vectorSize, "ob_"); err != nil {
			return nil, err
		}
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	inserted := 0
	for _, raw := range docs {
		document, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		docID := stringValue(document["skill_id"])
		if docID == "" {
			docID = stringValue(document["id"])
		}
		normalized, normalizeErr := normalizeSkill(document, docID)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		if err := replaceRow(ctx, tx, indexName, normalized); err != nil {
			return nil, err
		}
		inserted++
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return map[string]interface{}{"inserted": inserted}, nil
}

// DeleteDocument deletes a skill by primary key.
func (e *Engine) DeleteDocument(ctx context.Context, indexName, docID string) error {
	if err := validateIdentifier(indexName); err != nil {
		return err
	}
	if strings.HasPrefix(indexName, "skill_") {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE skill_id = ? OR REPLACE(skill_id, '/', '_') = ?", quoteIdentifier(indexName)), docID, docID)
		return err
	}
	primaryKey := "id"
	_, err := e.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s = ?", quoteIdentifier(indexName), quoteIdentifier(primaryKey)), docID)
	return err
}
