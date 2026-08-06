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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

const metadataPushdownMaxSize = 10000

func metadataTableName(tenantID string) string { return "ragflow_doc_meta_" + tenantID }

func validatedMetadataTableName(tenantID string) (string, error) {
	tableName := metadataTableName(tenantID)
	if err := validateIdentifier(tableName); err != nil {
		return "", err
	}
	return tableName, nil
}

// CreateMetadataStore creates the per-tenant metadata table.
func (e *Engine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return err
	}
	if err := e.ensureTableWithLock(ctx, tableName, metadataColumns, "ob_create_doc_meta_table_"+tableName); err != nil {
		return err
	}
	return e.ensureRegularIndex(ctx, tableName, "kb_id", "ob_")
}

// InsertMetadata stores metadata using the same REPLACE operation as Python.
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
	tx, err := e.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	for _, document := range metadata {
		metaFields := document["meta_fields"]
		var encoded string
		switch value := metaFields.(type) {
		case string:
			encoded = value
		case map[string]interface{}:
			data, marshalErr := json.Marshal(value)
			if marshalErr != nil {
				return nil, marshalErr
			}
			encoded = string(data)
		default:
			encoded = "{}"
		}
		row := map[string]interface{}{"id": document["id"], "kb_id": document["kb_id"], "meta_fields": encoded}
		if err := replaceRow(ctx, tx, tableName, row); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return []string{}, nil
}

// UpdateMetadata replaces the complete JSON object, inserting the row if it
// does not yet exist. This matches the service's replace_meta_fields contract.
func (e *Engine) UpdateMetadata(ctx context.Context, docID, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return err
	}
	encoded, err := json.Marshal(metaFields)
	if err != nil {
		return err
	}
	_, err = e.db.ExecContext(ctx, fmt.Sprintf("REPLACE INTO %s (id, kb_id, meta_fields) VALUES (?, ?, ?)", quoteIdentifier(tableName)), docID, datasetID, string(encoded))
	return err
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
	whereSQL, args, err := buildFilter(condition, "metadata")
	if err != nil {
		return 0, err
	}
	result, err := e.db.ExecContext(ctx, "DELETE FROM "+quoteIdentifier(tableName)+" WHERE "+whereSQL, args...)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteMetadataKeys removes selected JSON keys and deletes the row if no
// metadata remains.
func (e *Engine) DeleteMetadataKeys(ctx context.Context, docID, datasetID string, keys []string, tenantID string) error {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return err
	}
	var raw string
	if err := e.db.QueryRowContext(ctx, "SELECT meta_fields FROM "+quoteIdentifier(tableName)+" WHERE id = ? AND kb_id = ? LIMIT 1", docID, datasetID).Scan(&raw); err != nil {
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
		_, err := e.db.ExecContext(ctx, "DELETE FROM "+quoteIdentifier(tableName)+" WHERE id = ? AND kb_id = ?", docID, datasetID)
		return err
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return err
	}
	_, err = e.db.ExecContext(ctx, "UPDATE "+quoteIdentifier(tableName)+" SET meta_fields = ? WHERE id = ? AND kb_id = ?", string(encoded), docID, datasetID)
	return err
}

func (e *Engine) DropMetadataStore(ctx context.Context, tenantID string) error {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return err
	}
	_, err = e.db.ExecContext(ctx, "DROP TABLE IF EXISTS "+quoteIdentifier(tableName))
	return err
}

func (e *Engine) MetadataStoreExists(ctx context.Context, tenantID string) (bool, error) {
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		return false, err
	}
	return e.tableExists(ctx, tableName)
}

// SearchMetadata searches a tenant metadata table with exact total count.
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
	fieldsSQL, _, err := buildSelectFields(req.SelectFields, "metadata")
	if err != nil {
		return nil, err
	}
	whereSQL, args, err := buildFilter(req.Filter, "metadata")
	if err != nil {
		return nil, err
	}
	total, err := scanCount(e.db.QueryRowContext(ctx, "SELECT COUNT(id) FROM "+quoteIdentifier(tableName)+" WHERE "+whereSQL, args...))
	if err != nil || total == 0 {
		return &types.SearchMetadataResult{MetadataRecords: []map[string]interface{}{}, Total: total}, err
	}
	orderSQL, err := buildOrderBy(req.OrderBy, "metadata")
	if err != nil {
		return nil, err
	}
	limit := positiveOr(req.Limit, 30)
	query := fmt.Sprintf("SELECT %s FROM %s WHERE %s%s LIMIT %d, %d", fieldsSQL, quoteIdentifier(tableName), whereSQL, orderSQL, max(req.Offset, 0), limit)
	rows, err := e.queryRows(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return &types.SearchMetadataResult{MetadataRecords: decodeRows(rows, "metadata"), Total: total}, nil
}

// FilterDocIdsByMetaPushdown evaluates supported metadata filters in the
// legacy meta_fields JSON column. nil means the caller should fall back.
func (e *Engine) FilterDocIdsByMetaPushdown(ctx context.Context, sqlDB *gorm.DB, kbIDs []string, conditions []map[string]interface{}, logic string) []string {
	if len(kbIDs) == 0 || len(conditions) == 0 || (logic != "and" && logic != "or") {
		return nil
	}
	predicate, predicateArgs, err := buildMetaPushdownPredicate(conditions, logic)
	if err != nil {
		common.Debug("OceanBase metadata push-down is unsupported", zap.Error(err))
		return nil
	}
	tenantID, err := dao.GetTenantIDByKBID(ctx, sqlDB, kbIDs[0])
	if err != nil {
		return nil
	}
	tableName, err := validatedMetadataTableName(tenantID)
	if err != nil {
		common.Debug("OceanBase metadata table name is invalid", zap.Error(err))
		return nil
	}
	exists, err := e.tableExists(ctx, tableName)
	if err != nil || !exists {
		return nil
	}
	kbPlaceholders := make([]string, len(kbIDs))
	args := make([]interface{}, 0, len(kbIDs)+len(predicateArgs))
	for i, kbID := range kbIDs {
		kbPlaceholders[i] = "?"
		args = append(args, kbID)
	}
	whereSQL := "kb_id IN (" + strings.Join(kbPlaceholders, ", ") + ") AND (" + predicate + ")"
	args = append(args, predicateArgs...)
	total, err := scanCount(e.db.QueryRowContext(ctx, "SELECT COUNT(id) FROM "+quoteIdentifier(tableName)+" WHERE "+whereSQL, args...))
	if err != nil || total > metadataPushdownMaxSize {
		return nil
	}
	if total == 0 {
		return []string{}
	}
	rows, err := e.queryRows(ctx, fmt.Sprintf("SELECT id FROM %s WHERE %s LIMIT %d", quoteIdentifier(tableName), whereSQL, metadataPushdownMaxSize), args...)
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if id := stringValue(row["id"]); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func buildMetaPushdownPredicate(conditions []map[string]interface{}, logic string) (string, []interface{}, error) {
	logic = strings.ToLower(strings.TrimSpace(logic))
	if logic != "and" && logic != "or" {
		return "", nil, fmt.Errorf("unsupported metadata logic: %s", logic)
	}
	parts := make([]string, 0, len(conditions))
	args := make([]interface{}, 0, len(conditions)*4)
	for _, condition := range conditions {
		key := stringValue(condition["key"])
		op := stringValue(condition["op"])
		if key == "" || !metadataKeyPattern.MatchString(key) {
			return "", nil, fmt.Errorf("invalid metadata key")
		}
		path := "$." + key
		value := condition["value"]
		expression := "JSON_EXTRACT(meta_fields, ?)"
		if op == "≠" || op == "not in" {
			return "", nil, fmt.Errorf("metadata operator %s is unsafe for multi-valued fields", op)
		}
		switch op {
		case "=":
			candidate, err := encodeJSONCandidate(value)
			if err != nil {
				return "", nil, err
			}
			contains := "JSON_CONTAINS(" + expression + ", ?)"
			parts = append(parts, contains)
			args = append(args, path, candidate)
		case ">", "<", "≥", "≤":
			operator := map[string]string{">": ">", "<": "<", "≥": ">=", "≤": "<="}[op]
			coerced := coerceMetadataScalar(value)
			if _, numeric := numberToFloat(coerced); numeric {
				parts = append(parts, "CAST(JSON_UNQUOTE("+expression+") AS DECIMAL(65,20)) "+operator+" ?")
			} else {
				parts = append(parts, "LOWER(JSON_UNQUOTE("+expression+")) "+operator+" LOWER(?)")
			}
			args = append(args, path, coerced)
		case "in":
			values := metadataMembers(value)
			if len(values) == 0 {
				return "", nil, fmt.Errorf("metadata %s requires at least one value", op)
			}
			memberParts := make([]string, 0, len(values))
			for _, member := range values {
				candidate, err := encodeJSONCandidate(member)
				if err != nil {
					return "", nil, err
				}
				memberParts = append(memberParts, "JSON_CONTAINS("+expression+", ?)")
				args = append(args, path, candidate)
			}
			parts = append(parts, "("+strings.Join(memberParts, " OR ")+")")
		case "contains", "not contains", "start with", "end with":
			text := stringValue(value)
			if text == "" {
				return "", nil, fmt.Errorf("metadata %s requires a value", op)
			}
			like := "LOWER(JSON_UNQUOTE(" + expression + ")) LIKE "
			switch op {
			case "contains", "not contains":
				like += "CONCAT('%', LOWER(?), '%')"
			case "start with":
				like += "CONCAT(LOWER(?), '%')"
			case "end with":
				like += "CONCAT('%', LOWER(?))"
			}
			if op == "not contains" {
				like = "NOT (" + like + ")"
			}
			parts = append(parts, like)
			args = append(args, path, text)
		case "empty":
			parts = append(parts, "("+expression+" IS NULL OR JSON_TYPE("+expression+") = 'NULL' OR JSON_UNQUOTE("+expression+") = '' OR JSON_LENGTH("+expression+") = 0)")
			args = append(args, path, path, path, path)
		case "not empty":
			parts = append(parts, "NOT ("+expression+" IS NULL OR JSON_TYPE("+expression+") = 'NULL' OR JSON_UNQUOTE("+expression+") = '' OR JSON_LENGTH("+expression+") = 0)")
			args = append(args, path, path, path, path)
		default:
			return "", nil, fmt.Errorf("unsupported metadata operator: %s", op)
		}
	}
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("empty metadata predicate")
	}
	return strings.Join(parts, " "+strings.ToUpper(logic)+" "), args, nil
}

func encodeJSONCandidate(value interface{}) (string, error) {
	encoded, err := json.Marshal(coerceMetadataScalar(value))
	return string(encoded), err
}

func coerceMetadataScalar(value interface{}) interface{} {
	if value == nil {
		return nil
	}
	text := strings.TrimSpace(stringValue(value))
	if integer, err := strconv.ParseInt(text, 10, 64); err == nil {
		return integer
	}
	if number, err := strconv.ParseFloat(text, 64); err == nil {
		return number
	}
	return text
}

func metadataMembers(value interface{}) []interface{} {
	if values, ok := interfaceSlice(value); ok {
		return values
	}
	parts := strings.Split(stringValue(value), ",")
	result := make([]interface{}, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			result = append(result, part)
		}
	}
	return result
}
