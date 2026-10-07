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

package vastbase

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/engine/types"
)

// InsertChunks writes chunks or memory messages with the Python connector's
// delete-then-insert upsert: legacy tables carry no PRIMARY KEY, so ON
// CONFLICT is not an option. The delete and the grouped inserts share one
// transaction, keeping a failed batch from leaving half-replaced rows.
func (e *Engine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName, datasetID string) ([]string, error) {
	if len(chunks) == 0 {
		return []string{}, nil
	}
	if err := validateIdentifier(baseName); err != nil {
		return nil, err
	}
	kind := tableKind(baseName, datasetID)
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
		if err := e.ensureVectorColumnAndIndex(ctx, baseName, vectorSize); err != nil {
			return nil, err
		}
	}

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
			document, err = normalizeSkill(chunk, documentID)
		default:
			document, err = normalizeChunk(chunk)
			// normalizeChunk returns a nil map on encode errors; checking
			// err first keeps the kb_id backfill from panicking on it.
			if err == nil && stringValue(document["kb_id"]) == "" {
				document["kb_id"] = datasetID
			}
		}
		if err != nil {
			return nil, err
		}
		for field := range document {
			fieldNames[field] = true
		}
		normalized = append(normalized, document)
	}
	if err := e.ensureDynamicColumns(ctx, baseName, sortedSet(fieldNames)); err != nil {
		return nil, err
	}
	if err := e.backfillVectorColumns(ctx, baseName, normalized); err != nil {
		return nil, err
	}

	identifier := identifierColumn(kind)
	ids := make([]string, 0, len(normalized))
	for _, document := range normalized {
		if id := stringValue(document[identifier]); id != "" {
			ids = append(ids, id)
		}
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if len(ids) > 0 {
		// Python-parity IN form ("DELETE ... WHERE id IN (...)"): plain
		// placeholders avoid asking the server to infer an array parameter
		// type for ANY($n).
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE %s IN (%s)",
			quoteIdent(baseName), quoteIdent(identifier), placeholderList(len(ids), 0)),
			stringArgs(ids)...); err != nil {
			return nil, fmt.Errorf("vastbase: delete before insert in %s: %w", baseName, err)
		}
	}
	for _, group := range groupByColumnSet(normalized) {
		if err := insertGroup(ctx, tx, baseName, group); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return []string{}, nil
}

// backfillVectorColumns writes the zero-vector placeholder for every
// floatvector column the table already has but a document does not carry, the
// Python connector's backfill contract for multi-dimension tables.
func (e *Engine) backfillVectorColumns(ctx context.Context, tableName string, documents []map[string]interface{}) error {
	columns, err := e.listTableColumns(ctx, tableName)
	if err != nil || len(columns) == 0 {
		return err
	}
	for column := range columns {
		match := vectorDimRegex.FindStringSubmatch(column)
		if match == nil {
			continue
		}
		dimension, _ := strconv.Atoi(match[1])
		zero := zeroVector(dimension)
		for _, document := range documents {
			if _, ok := document[column]; !ok {
				document[column] = zero
			}
		}
	}
	return nil
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

// insertGroup writes one same-shape batch of documents as a single
// multi-row INSERT inside the caller's transaction.
func insertGroup(ctx context.Context, tx *sql.Tx, tableName string, documents []map[string]interface{}) error {
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
	if _, err := tx.ExecContext(ctx, query, args...); err != nil {
		return fmt.Errorf("vastbase: insert into %s: %w", tableName, err)
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
	columns, err := e.listTableColumns(ctx, baseName)
	if err != nil {
		return nil, err
	}
	condition := map[string]interface{}{identifierColumn(kind): chunkID}
	if kind == "memory" {
		condition["memory_id"] = datasetIDs
	} else if kind == "chunk" {
		condition["kb_id"] = datasetIDs
	}
	whereSQL, args, err := buildFilter(condition, kind, columns)
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
	return decodeLogicalRow(rows[0], kind), nil
}

// UpdateChunks updates rows while retaining the dataset-level scope the shared
// table needs. remove as a field name resets the column to its DEFAULT; remove
// as an object drops values from ###-joined keyword lists via read-modify-write.
func (e *Engine) UpdateChunks(ctx context.Context, condition, newValue map[string]interface{}, baseName, datasetID string) error {
	if err := validateIdentifier(baseName); err != nil {
		return err
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
		return err
	}
	if !exists {
		return fmt.Errorf("%w: '%s'", types.ErrIndexNotFound, baseName)
	}
	columns, err := e.listTableColumns(ctx, baseName)
	if err != nil {
		return err
	}
	whereSQL, args, err := buildFilter(condition, kind, columns)
	if err != nil {
		return err
	}
	setParts := make([]string, 0, len(newValue))
	setArgs := make([]interface{}, 0, len(newValue))
	removeFields := map[string]interface{}{}
	for _, rawKey := range sortedKeys(newValue) {
		key := rawKey
		if kind == "memory" {
			key = mapMemoryField(rawKey)
		} else if kind == "skill" && key == "id" {
			key = "skill_id"
		}
		value := newValue[rawKey]
		switch key {
		case "id":
			continue
		case "remove":
			switch remove := value.(type) {
			case string:
				if kind == "memory" {
					remove = mapMemoryField(remove)
				}
				if _, known := columns[remove]; !known {
					return fmt.Errorf("vastbase: cannot remove unknown column %q", remove)
				}
				setParts = append(setParts, quoteIdent(remove)+" = DEFAULT")
			case map[string]interface{}:
				for column, item := range remove {
					if _, known := columns[column]; !known {
						return fmt.Errorf("vastbase: cannot remove from unknown column %q", column)
					}
					removeFields[column] = item
				}
			default:
				return fmt.Errorf("vastbase: remove must be a field name or object")
			}
		default:
			if err := validateIdentifier(key); err != nil {
				return err
			}
			encoded, err := encodeUpdateValue(kind, key, value)
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
	if err := e.applyKeywordRemovals(ctx, baseName, whereSQL, args, removeFields); err != nil {
		return err
	}
	if len(setParts) == 0 {
		return nil
	}
	query := fmt.Sprintf("UPDATE %s SET %s WHERE %s",
		quoteIdent(baseName), strings.Join(setParts, ", "), shiftPlaceholders(whereSQL, len(setArgs)))
	setArgs = append(setArgs, args...)
	_, err = e.db.ExecContext(ctx, query, setArgs...)
	if err != nil {
		return fmt.Errorf("vastbase: update %s: %w", baseName, err)
	}
	return nil
}

// applyKeywordRemovals implements the Python connector's read-modify-write for
// remove objects: keyword columns are ###-joined text, so affected rows are
// read, the value is dropped from the decoded list, and rows landing on the
// same resulting list are updated in one statement per (column, list) pair.
func (e *Engine) applyKeywordRemovals(ctx context.Context, tableName, whereSQL string, args []interface{}, removeFields map[string]interface{}) error {
	if len(removeFields) == 0 {
		return nil
	}
	removeColumns := sortedMapKeys(removeFields)
	selectColumns := make([]string, 0, len(removeColumns)+1)
	for _, column := range removeColumns {
		selectColumns = append(selectColumns, quoteIdent(column))
	}
	selectColumns = append(selectColumns, quoteIdent("id"))
	rows, err := e.queryRows(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s",
		strings.Join(selectColumns, ", "), quoteIdent(tableName), whereSQL), args...)
	if err != nil {
		return fmt.Errorf("vastbase: read %s for removal: %w", tableName, err)
	}
	updates := map[string]map[string][]string{}
	for _, row := range rows {
		id := stringValue(row["id"])
		if id == "" {
			continue
		}
		for _, column := range removeColumns {
			current, _ := decodeColumnValue("chunk", column, row[column]).([]string)
			removeValue := stringValue(removeFields[column])
			if !containsString(current, removeValue) {
				continue
			}
			updated := make([]string, 0, len(current))
			for _, item := range current {
				if item != removeValue {
					updated = append(updated, item)
				}
			}
			if updates[column] == nil {
				updates[column] = map[string][]string{}
			}
			updates[column][strings.Join(updated, keywordSeparator)] = append(updates[column][strings.Join(updated, keywordSeparator)], id)
		}
	}
	for _, column := range sortedMapKeys(updates) {
		for joined, ids := range updates[column] {
			// The filter args keep their original $1..$n numbering (they are
			// bound first); the SET value and the id list continue past them.
			query := fmt.Sprintf("UPDATE %s SET %s = $%d WHERE %s AND id IN (%s)",
				quoteIdent(tableName), quoteIdent(column), len(args)+1, whereSQL,
				placeholderList(len(ids), len(args)+1))
			execArgs := append(append([]interface{}{}, args...), joined)
			execArgs = append(execArgs, stringArgs(ids)...)
			if _, err := e.db.ExecContext(ctx, query, execArgs...); err != nil {
				return fmt.Errorf("vastbase: remove from %s: %w", tableName, err)
			}
		}
	}
	return nil
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
	columns, err := e.listTableColumns(ctx, baseName)
	if err != nil {
		return 0, err
	}
	whereSQL, args, err := buildFilter(condition, kind, columns)
	if err != nil {
		return 0, err
	}
	result, err := e.db.ExecContext(ctx, "DELETE FROM "+quoteIdent(baseName)+" WHERE "+whereSQL, args...)
	if err != nil {
		return 0, fmt.Errorf("vastbase: delete from %s: %w", baseName, err)
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

// stringArgs widens a string slice to the interface slice database/sql
// variadic calls need.
func stringArgs(values []string) []interface{} {
	args := make([]interface{}, len(values))
	for i, value := range values {
		args[i] = value
	}
	return args
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
	result := make(map[string]interface{}, len(source)+1)
	for key, value := range source {
		result[key] = value
	}
	return result
}

// sortedSet lists a set's members in sorted order.
func sortedSet(source map[string]bool) []string {
	values := make([]string, 0, len(source))
	for value := range source {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

// sortedMapKeys lists a map's keys in sorted order.
func sortedMapKeys[V any](source map[string]V) []string {
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
