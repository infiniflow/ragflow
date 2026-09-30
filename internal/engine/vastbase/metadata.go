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
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"ragflow/internal/engine/types"

	"gorm.io/gorm"
)

func metadataTableName(tenantID string) string { return "ragflow_doc_meta_" + tenantID }

func validatedMetadataTableName(tenantID string) (string, error) {
	tableName := metadataTableName(tenantID)
	if err := validateIdentifier(tableName); err != nil {
		return "", err
	}
	return tableName, nil
}

// CreateMetadataStore creates the per-tenant metadata table with its kb_id
// index. Tables are per-tenant, so kb_id is a plain attribute column here,
// not a shared-table discriminator.
func (e *Engine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return err
	}
	if err := e.ensureTable(ctx, tableName, metadataColumns); err != nil {
		return err
	}
	return e.ensureRegularIndex(ctx, tableName, "kb_id")
}

// InsertMetadata stores metadata rows with the same delete-then-insert
// replacement the chunk path uses (id is the primary key).
func (e *Engine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	if len(metadata) == 0 {
		return []string{}, nil
	}
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return nil, err
	}
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := e.CreateMetadataStore(ctx, tenantID); err != nil {
			return nil, err
		}
	}
	rows := make([]map[string]interface{}, 0, len(metadata))
	ids := make([]string, 0, len(metadata))
	for _, document := range metadata {
		row := make(map[string]interface{}, len(metadataColumns))
		for _, column := range metadataColumns {
			value, present := document[column.name]
			if !present {
				continue
			}
			if column.name == "meta_fields" {
				encoded, err := encodeMetaFields(value)
				if err != nil {
					return nil, err
				}
				row[column.name] = encoded
				continue
			}
			encoded, err := encodeColumnValue(column.name, value)
			if err != nil {
				return nil, err
			}
			row[column.name] = encoded
		}
		if stringValue(row["id"]) == "" {
			return nil, fmt.Errorf("metadata document is missing id")
		}
		ids = append(ids, stringValue(row["id"]))
		rows = append(rows, row)
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if len(ids) > 0 {
		if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)",
			quoteIdent(tableName), placeholderList(len(ids), 0)), stringArgs(ids)...); err != nil {
			return nil, fmt.Errorf("vastbase: delete before metadata insert: %w", err)
		}
	}
	for _, group := range groupByColumnSet(rows) {
		if err := insertGroup(ctx, tx, tableName, group); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return []string{}, nil
}

func encodeMetaFields(value interface{}) (string, error) {
	switch typed := value.(type) {
	case string:
		return typed, nil
	case map[string]interface{}:
		encoded, err := json.Marshal(typed)
		if err != nil {
			return "", err
		}
		return string(encoded), nil
	default:
		return "{}", nil
	}
}

// UpdateMetadata replaces the complete meta_fields JSON object, inserting the
// row when it does not yet exist — the service's replace_meta_fields contract.
func (e *Engine) UpdateMetadata(ctx context.Context, docID, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return err
	}
	encoded, err := encodeMetaFields(metaFields)
	if err != nil {
		return err
	}
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1", quoteIdent(tableName)), docID); err != nil {
		return fmt.Errorf("vastbase: replace metadata: %w", err)
	}
	if _, err := tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO %s (id, kb_id, meta_fields) VALUES ($1, $2, $3)", quoteIdent(tableName)), docID, datasetID, encoded); err != nil {
		return fmt.Errorf("vastbase: replace metadata: %w", err)
	}
	return tx.Commit()
}

// DeleteMetadata deletes matching metadata rows.
func (e *Engine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return 0, err
	}
	exists, err := e.tableExists(ctx, tableName)
	if err != nil || !exists {
		return 0, err
	}
	columns, err := e.listTableColumns(ctx, tableName)
	if err != nil {
		return 0, err
	}
	whereSQL, args, err := buildFilter(condition, "metadata", columns)
	if err != nil {
		return 0, err
	}
	result, err := e.db.ExecContext(ctx, "DELETE FROM "+quoteIdent(tableName)+" WHERE "+whereSQL, args...)
	if err != nil {
		return 0, fmt.Errorf("vastbase: delete metadata: %w", err)
	}
	return result.RowsAffected()
}

// DeleteMetadataKeys removes selected JSON keys and deletes the row when no
// metadata remains.
func (e *Engine) DeleteMetadataKeys(ctx context.Context, docID, datasetID string, keys []string, tenantID string) error {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return err
	}
	var raw string
	if err := e.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT meta_fields FROM %s WHERE id = $1 AND kb_id = $2 LIMIT 1", quoteIdent(tableName)),
		docID, datasetID).Scan(&raw); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, docID)
		}
		return err
	}
	fields := make(map[string]interface{})
	if err := json.Unmarshal([]byte(raw), &fields); err != nil {
		return fmt.Errorf("decode metadata for document %s: %w", docID, err)
	}
	for _, key := range keys {
		delete(fields, key)
	}
	if len(fields) == 0 {
		_, err := e.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s WHERE id = $1 AND kb_id = $2", quoteIdent(tableName)), docID, datasetID)
		return err
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = e.db.ExecContext(ctx, fmt.Sprintf("UPDATE %s SET meta_fields = $1 WHERE id = $2 AND kb_id = $3", quoteIdent(tableName)), string(encoded), docID, datasetID)
	return err
}

func (e *Engine) DropMetadataStore(ctx context.Context, tenantID string) error {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return err
	}
	_, err = e.db.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoteIdent(tableName))
	return err
}

func (e *Engine) MetadataStoreExists(ctx context.Context, tenantID string) (bool, error) {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return false, err
	}
	return e.tableExists(ctx, tableName)
}

// SearchMetadata searches a tenant metadata table with an exact total count.
func (e *Engine) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	if req == nil || req.TenantID == "" {
		return nil, fmt.Errorf("tenantID cannot be empty")
	}
	tableName, err := validatedMetadataTableName(req.TenantID)
	if err != nil {
		return nil, err
	}
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return &types.SearchMetadataResult{MetadataRecords: []map[string]interface{}{}}, nil
	}
	columns, err := e.listTableColumns(ctx, tableName)
	if err != nil {
		return nil, err
	}
	output := buildMetadataOutput(req.SelectFields, columns)
	whereSQL, args, err := buildFilter(req.Filter, "metadata", columns)
	if err != nil {
		return nil, err
	}
	var total int64
	if err := e.db.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COUNT(id) FROM %s WHERE %s", quoteIdent(tableName), whereSQL),
		args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("vastbase: count metadata %s: %w", tableName, err)
	}
	if total == 0 {
		return &types.SearchMetadataResult{MetadataRecords: []map[string]interface{}{}, Total: total}, nil
	}
	orderSQL, err := buildOrderBy(req.OrderBy, "metadata", columns)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s%s LIMIT %d OFFSET %d",
		selectFieldsSQL(output), quoteIdent(tableName), whereSQL, orderSQL, max(req.Offset, 0), positiveOr(req.Limit, 30))
	rows, err := e.queryRows(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("vastbase: search metadata %s: %w", tableName, err)
	}
	return &types.SearchMetadataResult{MetadataRecords: decodeRows(rows, "metadata"), Total: total}, nil
}

// buildMetadataOutput projects the requested fields onto the live columns; a
// nil field list selects every column.
func buildMetadataOutput(selectFields []string, columns map[string]columnMeta) []string {
	if len(selectFields) == 0 {
		names := make([]string, 0, len(columns))
		for column := range columns {
			names = append(names, column)
		}
		sort.Strings(names)
		return names
	}
	output := make([]string, 0, len(selectFields))
	for _, field := range selectFields {
		if _, exists := columns[field]; exists && !containsString(output, field) {
			output = append(output, field)
		}
	}
	return output
}

// FilterDocIdsByMetaPushdown is not implemented: meta_fields is stored as
// text, so a push-down predicate would cast the whole table to jsonb. The
// caller falls back to in-memory filtering, which is correct and bounded by
// the metadata page size.
func (e *Engine) FilterDocIdsByMetaPushdown(ctx context.Context, sqlDB *gorm.DB, kbIDs []string, conditions []map[string]interface{}, logic string) []string {
	return nil
}
