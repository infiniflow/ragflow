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
	"database/sql"
	"fmt"
	"strings"

	"ragflow/internal/engine/types"
)

// InsertChunks writes chunks or memory messages with legacy REPLACE semantics.
func (e *Engine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName, datasetID string) ([]string, error) {
	if len(chunks) == 0 {
		return []string{}, nil
	}
	if err := validateIdentifier(baseName); err != nil {
		return nil, err
	}
	vectorSize := 0
	for _, chunk := range chunks {
		vectorSize = vectorDimension(chunk)
		if vectorSize > 0 {
			break
		}
	}
	exists, err := e.ChunkStoreExists(ctx, baseName, datasetID)
	if err != nil {
		return nil, err
	}
	if !exists {
		if vectorSize == 0 {
			return nil, fmt.Errorf("cannot infer vector size from documents")
		}
		if err := e.CreateChunkStore(ctx, baseName, datasetID, vectorSize, ""); err != nil {
			return nil, err
		}
	} else if vectorSize > 0 {
		if err := e.ensureVectorColumnAndIndex(ctx, baseName, vectorSize, lockPrefix(baseName)); err != nil {
			return nil, err
		}
	}

	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, chunk := range chunks {
		var normalized map[string]interface{}
		switch {
		case strings.HasPrefix(baseName, "memory_"):
			normalized, err = normalizeMemory(chunk)
			if err != nil {
				return nil, err
			}
			if normalized["memory_id"] == nil {
				normalized["memory_id"] = datasetID
			}
		case strings.HasPrefix(baseName, "skill_") || datasetID == "skill":
			documentID := stringValue(chunk["skill_id"])
			if documentID == "" {
				documentID = stringValue(chunk["id"])
			}
			normalized, err = normalizeSkill(chunk, documentID)
			if err != nil {
				return nil, err
			}
		default:
			normalized, err = normalizeChunk(chunk)
			if err != nil {
				return nil, err
			}
			if normalized["kb_id"] == nil || normalized["kb_id"] == "" {
				normalized["kb_id"] = datasetID
			}
		}
		if err := replaceRow(ctx, tx, baseName, normalized); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	if vectorSize > 0 {
		if err := e.waitForIndexRefresh(ctx); err != nil {
			return nil, err
		}
	}
	return []string{}, nil
}

func (e *Engine) waitForIndexRefresh(ctx context.Context) error {
	if !e.indexRefreshEnabled {
		return nil
	}
	if _, err := e.db.ExecContext(ctx, "CALL DBMS_INDEX_MANAGER.REFRESH()"); err != nil {
		return fmt.Errorf("wait for SeekDB index refresh: %w", err)
	}
	return nil
}

func replaceRow(ctx context.Context, tx *sql.Tx, tableName string, document map[string]interface{}) error {
	columns := sortedColumns(document)
	quoted := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	values := make([]interface{}, len(columns))
	for i, column := range columns {
		if err := validateIdentifier(column); err != nil {
			return err
		}
		quoted[i] = quoteIdentifier(column)
		placeholders[i] = "?"
		values[i] = document[column]
	}
	query := fmt.Sprintf("REPLACE INTO %s (%s) VALUES (%s)", quoteIdentifier(tableName),
		strings.Join(quoted, ", "), strings.Join(placeholders, ", "))
	if _, err := tx.ExecContext(ctx, query, values...); err != nil {
		return fmt.Errorf("replace row in %s: %w", tableName, err)
	}
	return nil
}

// GetChunk loads one row, scoped by the requested datasets.
func (e *Engine) GetChunk(ctx context.Context, baseName, chunkID string, datasetIDs []string) (interface{}, error) {
	if err := validateIdentifier(baseName); err != nil {
		return nil, err
	}
	kind := tableKind(baseName, datasetIDs...)
	exists, err := e.tableExists(ctx, baseName)
	if err != nil {
		return nil, err
	}
	if !exists {
		if kind == "memory" {
			return nil, fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
		}
		return nil, nil
	}
	identifier := "id"
	if kind == "skill" {
		identifier = "skill_id"
	}
	condition := map[string]interface{}{identifier: chunkID}
	if kind == "memory" {
		condition["memory_id"] = datasetIDs
	} else if kind == "chunk" {
		condition["kb_id"] = datasetIDs
	}
	whereSQL, args, err := buildFilter(condition, kind)
	if err != nil {
		return nil, err
	}
	rows, err := e.queryRows(ctx, "SELECT * FROM "+quoteIdentifier(baseName)+" WHERE "+whereSQL+" LIMIT 1", args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		if kind == "memory" {
			return nil, fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
		}
		return nil, nil
	}
	return decodeLogicalRow(rows[0], kind), nil
}

// UpdateChunks updates rows while retaining the dataset-level scope used by
// the Python connector.
func (e *Engine) UpdateChunks(ctx context.Context, condition, newValue map[string]interface{}, baseName, datasetID string) error {
	if err := validateIdentifier(baseName); err != nil {
		return err
	}
	kind := tableKind(baseName, datasetID)
	condition = copyMap(condition)
	_, scopedByDocument := condition["doc_id"]
	if kind == "memory" {
		condition["memory_id"] = datasetID
	} else if kind == "chunk" {
		condition["kb_id"] = datasetID
	}
	exists, err := e.tableExists(ctx, baseName)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: '%s'", types.ErrIndexNotFound, baseName)
	}
	if kind == "chunk" {
		newValue = normalizeImportantKeywordFields(newValue)
	}
	whereSQL, args, err := buildFilter(condition, kind)
	if err != nil {
		return err
	}
	setParts := make([]string, 0, len(newValue))
	setArgs := make([]interface{}, 0, len(newValue))
	for key, value := range newValue {
		if kind == "memory" {
			key = mapMemoryField(key)
		} else if kind == "chunk" {
			key = mapChunkField(key)
		} else if kind == "skill" && key == "id" {
			key = "skill_id"
		}
		switch key {
		case "id":
			continue
		case "remove":
			switch remove := value.(type) {
			case string:
				if kind == "chunk" {
					remove = mapChunkField(remove)
				}
				if err := validateIdentifier(remove); err != nil {
					return err
				}
				setParts = append(setParts, quoteIdentifier(remove)+" = NULL")
			case map[string]interface{}:
				for column, item := range remove {
					if kind != "chunk" || !arrayColumns[column] {
						return fmt.Errorf("column %s is not an array column", column)
					}
					setParts = append(setParts, fmt.Sprintf("%s = ARRAY_REMOVE(%s, ?)", quoteIdentifier(column), quoteIdentifier(column)))
					setArgs = append(setArgs, item)
				}
			default:
				return fmt.Errorf("remove must be a field name or object")
			}
		case "add":
			items, ok := value.(map[string]interface{})
			if !ok {
				return fmt.Errorf("add must be an object")
			}
			for column, item := range items {
				if kind != "chunk" || !arrayColumns[column] {
					return fmt.Errorf("column %s is not an array column", column)
				}
				if column == "important_kwd" {
					if text, ok := item.(string); ok {
						item, _ = sanitizeImportantKeyword(text)
					}
				}
				setParts = append(setParts, fmt.Sprintf("%s = ARRAY_APPEND(%s, ?)", quoteIdentifier(column), quoteIdentifier(column)))
				setArgs = append(setArgs, item)
			}
		default:
			if err := validateIdentifier(key); err != nil {
				return err
			}
			encoded, encodeErr := encodeUpdateValue(kind, key, value)
			if encodeErr != nil {
				return encodeErr
			}
			setParts = append(setParts, quoteIdentifier(key)+" = ?")
			setArgs = append(setArgs, encoded)
			if kind == "memory" && key == "content_ltks" {
				setParts = append(setParts, quoteIdentifier("tokenized_content_ltks")+" = ?")
				setArgs = append(setArgs, tokenizeMemoryContent(stringValue(value)))
			}
			if key == "metadata" && scopedByDocument {
				metadata := asStringMap(value)
				if groupID := stringValue(metadata["_group_id"]); groupID != "" {
					setParts = append(setParts, quoteIdentifier("group_id")+" = ?")
					setArgs = append(setArgs, groupID)
				}
				if title := stringValue(metadata["_title"]); title != "" {
					setParts = append(setParts, quoteIdentifier("docnm_kwd")+" = ?")
					setArgs = append(setArgs, title)
				}
			}
		}
	}
	if len(setParts) == 0 {
		return nil
	}
	setArgs = append(setArgs, args...)
	_, err = e.db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET %s WHERE %s", quoteIdentifier(baseName), strings.Join(setParts, ", "), whereSQL), setArgs...)
	return err
}

// DeleteChunks deletes rows under the requested dataset scope.
func (e *Engine) DeleteChunks(ctx context.Context, condition map[string]interface{}, baseName, datasetID string) (int64, error) {
	if err := validateIdentifier(baseName); err != nil {
		return 0, err
	}
	kind := tableKind(baseName, datasetID)
	condition = copyMap(condition)
	if kind == "memory" {
		condition["memory_id"] = datasetID
	} else if kind == "chunk" {
		condition["kb_id"] = datasetID
	}
	exists, err := e.tableExists(ctx, baseName)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	whereSQL, args, err := buildFilter(condition, kind)
	if err != nil {
		return 0, err
	}
	result, err := e.db.ExecContext(ctx, "DELETE FROM "+quoteIdentifier(baseName)+" WHERE "+whereSQL, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func tableKind(tableName string, datasetIDs ...string) string {
	for _, datasetID := range datasetIDs {
		if datasetID == "skill" {
			return "skill"
		}
	}
	switch {
	case strings.HasPrefix(tableName, "memory_"):
		return "memory"
	case strings.HasPrefix(tableName, "skill_"):
		return "skill"
	case strings.HasPrefix(tableName, "ragflow_doc_meta_"):
		return "metadata"
	default:
		return "chunk"
	}
}

func mapMemoryField(field string) string {
	if mapped, ok := memoryFieldToColumn[field]; ok {
		return mapped
	}
	return field
}

func copyMap(source map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}
