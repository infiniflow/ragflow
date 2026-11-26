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
	"database/sql"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"ragflow/internal/engine/types"
)

// InsertChunks upserts document rows atomically using their primary keys.
func (e *Engine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName, datasetID, language string) ([]string, error) {
	baseName = physicalTableName(baseName)
	if len(chunks) == 0 {
		return []string{}, nil
	}
	if err := validateIdentifier(baseName); err != nil {
		return nil, err
	}
	kind := tableKind(baseName, datasetID)
	var err error
	fieldNames := map[string]bool{}
	normalized := make([]map[string]interface{}, 0, len(chunks))
	for _, chunk := range chunks {
		var document map[string]interface{}
		switch kind {
		case "memory":
			document, err = normalizeMemory(chunk, datasetID)
		case "skill":
			documentID := stringValue(chunk["skill_id"])
			if documentID == "" {
				documentID = stringValue(chunk["id"])
			}
			if documentID == "" {
				return nil, fmt.Errorf("skill identifier cannot be empty")
			}
			document, err = normalizeSkill(chunk, documentID)
		default:
			document, err = e.normalizeChunk(chunk)
			// normalizeChunk returns a nil map on encode errors; checking
			// err first keeps the kb_id backfill from panicking on it.
			if err == nil && stringValue(document["kb_id"]) == "" {
				document["kb_id"] = datasetID
			}
		}
		if err != nil {
			return nil, err
		}
		if kind == "chunk" {
			kb := stringValue(document["kb_id"])
			if kb == "" || (datasetID != "" && kb != datasetID) || stringValue(document["id"]) == "" {
				return nil, fmt.Errorf("opengauss chunk requires id and matching kb_id")
			}
		}
		for field := range document {
			fieldNames[field] = true
		}
		normalized = append(normalized, document)
	}
	ready, err := e.ChunkStoreExists(ctx, baseName, datasetID)
	if err != nil {
		return nil, err
	}
	if !ready {
		if err := e.CreateChunkStore(ctx, baseName, datasetID, 0, "", ""); err != nil {
			return nil, err
		}
	}
	if err := e.ensureDynamicColumns(ctx, baseName, kind, slices.Sorted(maps.Keys(fieldNames))); err != nil {
		return nil, err
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := e.insertGroups(ctx, tx, baseName, kind, normalized); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return []string{}, nil
}

// groupByColumnSet buckets documents by their sorted column set so each bucket
// forms one multi-row INSERT.
func groupByColumnSet(documents []map[string]interface{}) [][]map[string]interface{} {
	groups := map[string][]map[string]interface{}{}
	keys := make([]string, 0)
	for _, document := range documents {
		key := strings.Join(sortedColumns(document), "\x00")
		if _, ok := groups[key]; !ok {
			keys = append(keys, key)
		}
		groups[key] = append(groups[key], document)
	}
	sort.Strings(keys)
	result := make([][]map[string]interface{}, 0, len(keys))
	for _, key := range keys {
		result = append(result, groups[key])
	}
	return result
}

// insertGroups replaces each row, including fields omitted by the new document.
func (e *Engine) insertGroups(ctx context.Context, tx *sql.Tx, table, kind string, documents []map[string]interface{}) error {
	var onDuplicate string
	if len(documents) > 0 {
		rows, err := queryRowsTx(ctx, tx, "SELECT column_name FROM information_schema.columns WHERE table_schema=current_schema() AND table_name=$1 ORDER BY ordinal_position", table)
		if err != nil {
			return err
		}
		var sets []string
		for _, row := range rows {
			name := stringValue(row["column_name"])
			if name == identifierColumn(kind) || (kind == "chunk" && name == "kb_id") {
				continue
			}
			col := quoteIdent(name)
			// VALUES uses the column default when the INSERT omits the field.
			sets = append(sets, col+"=VALUES("+col+")")
		}
		onDuplicate = " ON DUPLICATE KEY UPDATE " + strings.Join(sets, ", ")
	}
	for _, group := range groupByColumnSet(documents) {
		if err := insertGroup(ctx, tx, table, onDuplicate, group); err != nil {
			return err
		}
	}
	return nil
}

// insertGroup writes one same-shape batch of documents as a single
// multi-row INSERT inside the caller's transaction.
func insertGroup(ctx context.Context, tx *sql.Tx, tableName, onDuplicate string, documents []map[string]interface{}) error {
	columns := sortedColumns(documents[0])
	quoted := make([]string, len(columns))
	for i, column := range columns {
		if err := validateIdentifier(column); err != nil {
			return err
		}
		quoted[i] = quoteIdent(column)
	}
	args := make([]interface{}, 0, len(columns)*len(documents))
	placeholders := make([]string, 0, len(documents))
	for _, document := range documents {
		row := make([]string, len(columns))
		for i, column := range columns {
			args = append(args, document[column])
			row[i] = fmt.Sprintf("$%d", len(args))
		}
		placeholders = append(placeholders, "("+strings.Join(row, ",")+")")
	}
	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		quoteIdent(tableName), strings.Join(quoted, ", "), strings.Join(placeholders, ","))
	if _, err := tx.ExecContext(ctx, query+onDuplicate, args...); err != nil {
		return fmt.Errorf("opengauss: insert into %s: %w", tableName, err)
	}
	return nil
}

// GetChunk loads one row, scoped by the requested datasets.
func (e *Engine) GetChunk(ctx context.Context, baseName, chunkID string, datasetIDs []string) (interface{}, error) {
	baseName = physicalTableName(baseName)
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
	columns, err := e.listTableColumns(ctx, baseName)
	if err != nil {
		return nil, err
	}
	condition := map[string]interface{}{identifierColumn(kind): chunkID}
	if kind == "memory" && len(datasetIDs) > 0 {
		condition["memory_id"] = datasetIDs
	} else if kind == "chunk" && len(datasetIDs) > 0 {
		condition["kb_id"] = datasetIDs
	}
	whereSQL, args, err := e.buildFilter(condition, kind, columns)
	if err != nil {
		return nil, err
	}
	rows, err := e.queryRows(ctx, "SELECT * FROM "+quoteIdent(baseName)+" WHERE "+whereSQL+" LIMIT 1", args...)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		if kind == "memory" {
			return nil, fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
		}
		return nil, nil
	}
	return e.decodeLogicalRow(rows[0], kind, ""), nil
}

// UpdateChunks updates rows while retaining the dataset-level scope the shared
// table needs. remove as a field name resets the column to its DEFAULT; remove
// as an object edits chunk keyword lists within the scoped UPDATE.
func (e *Engine) UpdateChunks(ctx context.Context, condition, newValue map[string]interface{}, baseName, datasetID string) error {
	baseName = physicalTableName(baseName)
	if err := validateIdentifier(baseName); err != nil {
		return err
	}
	kind := tableKind(baseName, datasetID)
	if kind != "skill" && datasetID == "" {
		return fmt.Errorf("opengauss: dataset is required for scoped updates")
	}
	condition = copyMap(condition)
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
	columns, err := e.listTableColumns(ctx, baseName)
	if err != nil {
		return err
	}
	whereSQL, args, err := e.buildFilter(condition, kind, columns)
	if err != nil {
		return err
	}
	setParts := make([]string, 0, len(newValue))
	setArgs := make([]interface{}, 0, len(newValue))
	if kind == "chunk" {
		setParts, setArgs, err = keywordUpdates(newValue, columns)
		if err != nil {
			return err
		}
	}
	for _, rawKey := range slices.Sorted(maps.Keys(newValue)) {
		key := rawKey
		if kind == "memory" {
			key = mapMemoryField(rawKey)
		} else if kind == "skill" && key == "id" {
			key = "skill_id"
		}
		value := newValue[rawKey]
		if kind == "chunk" && (key == "add" || key == "remove") {
			if _, ok := value.(map[string]interface{}); ok {
				continue
			}
		}
		if (kind == "chunk" && key == "kb_id") || (kind == "memory" && key == "memory_id") {
			return fmt.Errorf("opengauss: cannot update dataset field %q", key)
		}
		switch key {
		case "id":
			continue
		case "remove":
			switch remove := value.(type) {
			case string:
				if kind == "memory" {
					remove = mapMemoryField(remove)
				}
				if (kind == "chunk" && remove == "kb_id") || (kind == "memory" && remove == "memory_id") {
					return fmt.Errorf("opengauss: cannot reset dataset field %q", remove)
				}
				if kind == "memory" && remove == "content_ltks" {
					setParts = append(setParts, "tokenized_content_ltks = DEFAULT")
				}
				if _, known := columns[remove]; !known {
					return fmt.Errorf("opengauss: cannot remove unknown column %q", remove)
				}
				setParts = append(setParts, quoteIdent(remove)+" = DEFAULT")
			default:
				return fmt.Errorf("opengauss: remove must be a field name or object")
			}
		default:
			if err := validateIdentifier(key); err != nil {
				return err
			}
			encoded, err := e.encodeUpdateValue(kind, key, value)
			if err != nil {
				return err
			}
			setArgs = append(setArgs, encoded)
			setParts = append(setParts, fmt.Sprintf("%s = $%d", quoteIdent(key), len(setArgs)))
			if kind == "memory" && key == "content_ltks" {
				setArgs = append(setArgs, tokenizeMemoryContent(stringValue(value)))
				setParts = append(setParts, fmt.Sprintf("%s = $%d", quoteIdent("tokenized_content_ltks"), len(setArgs)))
			}
		}
	}
	if len(setParts) == 0 {
		return nil
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		quoteIdent(baseName), strings.Join(setParts, ", "), shiftPlaceholders(whereSQL, len(setArgs)))
	setArgs = append(setArgs, args...)
	_, err = e.db.ExecContext(ctx, query, setArgs...)
	if err != nil {
		return fmt.Errorf("opengauss: update %s: %w", baseName, err)
	}
	return nil
}

// DeleteChunks deletes rows under the requested dataset scope.
func (e *Engine) DeleteChunks(ctx context.Context, condition map[string]interface{}, baseName, datasetID string) (int64, error) {
	baseName = physicalTableName(baseName)
	if err := validateIdentifier(baseName); err != nil {
		return 0, err
	}
	kind := tableKind(baseName, datasetID)
	if kind != "skill" && datasetID == "" {
		return 0, fmt.Errorf("opengauss: dataset is required for scoped deletes")
	}
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
	columns, err := e.listTableColumns(ctx, baseName)
	if err != nil {
		return 0, err
	}
	whereSQL, args, err := e.buildFilter(condition, kind, columns)
	if err != nil {
		return 0, err
	}
	result, err := e.db.ExecContext(ctx, "DELETE FROM "+quoteIdent(baseName)+" WHERE "+whereSQL, args...)
	if err != nil {
		return 0, fmt.Errorf("opengauss: delete from %s: %w", baseName, err)
	}
	return result.RowsAffected()
}

// placeholderList renders "offset+1, offset+2, ..." for an IN clause with the
// given number of bound arguments.
func placeholderList(count, offset int) string {
	placeholders := make([]string, count)
	for i := range placeholders {
		placeholders[i] = fmt.Sprintf("$%d", offset+i+1)
	}
	return strings.Join(placeholders, ", ")
}

// tableKind classifies a table row set: skill tables carry skill_id, memory_
// tables hold memory rows, everything else is a chunk table.
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

// identifierColumn is the primary-key column name for a row kind.
func identifierColumn(kind string) string {
	if kind == "skill" {
		return "skill_id"
	}
	return "id"
}

// mapMemoryField translates a logical memory field to its column name,
// passing unknown fields through.
func mapMemoryField(field string) string {
	if mapped, ok := memoryFieldToColumn[field]; ok {
		return mapped
	}
	return field
}

// copyMap shallow-copies a filter map so per-table discriminators can be
// added without aliasing the request's filter.
func copyMap(source map[string]interface{}) map[string]interface{} {
	if source == nil {
		return map[string]interface{}{}
	}
	return maps.Clone(source)
}
