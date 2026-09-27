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

package serenedb

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ragflow/internal/engine/types"

	"gorm.io/gorm"
)

// metadataTableDDL creates the per-tenant metadata table and its lookup
// indexes. Pure for tests.
func metadataTableDDL(tableName string) []string {
	cols := make([]string, 0, len(docMetaColumnOrder))
	for _, c := range docMetaColumnOrder {
		cols = append(cols, fmt.Sprintf("%s %s", c, docMetaDDL[c]))
	}
	return []string{
		fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)", tableName, strings.Join(cols, ", ")),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS idx_%s_kb_id ON %s (kb_id)", tableName, tableName),
	}
}

// CreateMetadataStore creates the tenant's document metadata table.
func (e *serenedbEngine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	tableName := buildMetadataTableName(tenantID)
	for _, stmt := range metadataTableDDL(tableName) {
		if err := e.exec(ctx, stmt); err != nil {
			return fmt.Errorf("serenedb: create metadata store %s: %w", tableName, err)
		}
	}
	return nil
}

// DropMetadataStore drops the tenant metadata table.
func (e *serenedbEngine) DropMetadataStore(ctx context.Context, tenantID string) error {
	return e.exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s", buildMetadataTableName(tenantID)))
}

// MetadataStoreExists reports whether the tenant metadata table exists.
func (e *serenedbEngine) MetadataStoreExists(ctx context.Context, tenantID string) (bool, error) {
	return e.tableExists(ctx, buildMetadataTableName(tenantID))
}

func metaFieldsJSON(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// InsertMetadata upserts metadata records by id. meta_fields is stored as a
// JSON string.
func (e *serenedbEngine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	if len(metadata) == 0 {
		return []string{}, nil
	}
	tableName := buildMetadataTableName(tenantID)
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := e.CreateMetadataStore(ctx, tenantID); err != nil {
			return nil, err
		}
	}
	query := fmt.Sprintf("INSERT INTO %s (id, kb_id, meta_fields) VALUES ($1, $2, $3) "+
		"ON CONFLICT (id) DO UPDATE SET kb_id = EXCLUDED.kb_id, meta_fields = EXCLUDED.meta_fields", tableName)
	for _, rec := range metadata {
		if err := e.exec(ctx, query, rec["id"], rec["kb_id"], metaFieldsJSON(rec["meta_fields"])); err != nil {
			return nil, fmt.Errorf("serenedb: insert metadata into %s: %w", tableName, err)
		}
	}
	return []string{}, nil
}

// UpdateMetadata merges metaFields into the stored record, preserving keys the
// caller did not send. Missing rows are inserted.
func (e *serenedbEngine) UpdateMetadata(ctx context.Context, docID, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	tableName := buildMetadataTableName(tenantID)
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return err
	}
	if !exists {
		if err := e.CreateMetadataStore(ctx, tenantID); err != nil {
			return err
		}
	}
	existing, err := e.loadMetaFields(ctx, tableName, docID, datasetID)
	if err != nil {
		return err
	}
	if existing == nil {
		return e.exec(ctx,
			fmt.Sprintf("INSERT INTO %s (id, kb_id, meta_fields) VALUES ($1, $2, $3)", tableName),
			docID, datasetID, metaFieldsJSON(metaFields))
	}
	for k, v := range metaFields {
		existing[k] = v
	}
	return e.exec(ctx,
		fmt.Sprintf("UPDATE %s SET meta_fields = $1 WHERE id = $2 AND kb_id = $3", tableName),
		metaFieldsJSON(existing), docID, datasetID)
}

// DeleteMetadataKeys removes specific keys from a record's meta_fields, dropping
// the whole row if none remain.
func (e *serenedbEngine) DeleteMetadataKeys(ctx context.Context, docID, datasetID string, keys []string, tenantID string) error {
	tableName := buildMetadataTableName(tenantID)
	existing, err := e.loadMetaFields(ctx, tableName, docID, datasetID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("serenedb: metadata document not found: %s", docID)
	}
	changed := false
	for _, k := range keys {
		if _, ok := existing[k]; ok {
			delete(existing, k)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	if len(existing) == 0 {
		return e.exec(ctx,
			fmt.Sprintf("DELETE FROM %s WHERE id = $1 AND kb_id = $2", tableName), docID, datasetID)
	}
	return e.exec(ctx,
		fmt.Sprintf("UPDATE %s SET meta_fields = $1 WHERE id = $2 AND kb_id = $3", tableName),
		metaFieldsJSON(existing), docID, datasetID)
}

// DeleteMetadata removes records matching condition. A missing table is not an
// error.
func (e *serenedbEngine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	tableName := buildMetadataTableName(tenantID)
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return 0, err
	}
	if !exists {
		return 0, nil
	}
	if unknown := unrecognizedFilterKeys(condition); len(unknown) > 0 {
		return 0, fmt.Errorf("serenedb: refusing to delete metadata from %s with unrecognized filter keys %v", tableName, unknown)
	}
	filters := buildFilters(condition)
	if len(filters) == 0 {
		return 0, nil
	}
	res, err := e.db.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE %s", tableName, strings.Join(filters, " AND ")))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// SearchMetadata returns metadata records matching the request. A missing table
// yields a non-nil empty result so callers do not fall back to in-memory scans.
func (e *serenedbEngine) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	if req.TenantID == "" {
		return nil, fmt.Errorf("serenedb: SearchMetadata requires a tenant id")
	}
	tableName := buildMetadataTableName(req.TenantID)
	empty := &types.SearchMetadataResult{MetadataRecords: []map[string]interface{}{}, Total: 0}
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return nil, err
	}
	if !exists {
		return empty, nil
	}
	// SelectFields is interpolated into the projection, so keep only real
	// metadata columns.
	fields := "*"
	if len(req.SelectFields) > 0 {
		valid := make([]string, 0, len(req.SelectFields))
		for _, f := range req.SelectFields {
			if _, ok := docMetaDDL[f]; ok {
				valid = append(valid, f)
			}
		}
		if len(valid) > 0 {
			fields = strings.Join(valid, ", ")
		}
	}
	filters := buildFilters(req.Filter)
	where := filtersExpr(filters)
	limit := req.Limit
	if limit <= 0 {
		limit = defaultPageSize
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	query := buildFilterSQL(tableName, fields, where, req.OrderBy, limit, offset)
	records, err := e.queryMaps(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("serenedb: search metadata %s: %w", tableName, err)
	}
	total, err := e.countRows(ctx, tableName, where)
	if err != nil {
		return nil, err
	}
	if records == nil {
		records = []map[string]interface{}{}
	}
	return &types.SearchMetadataResult{MetadataRecords: records, Total: total}, nil
}

// FilterDocIdsByMetaPushdown returns nil, which tells the caller to filter
// document metadata in memory. Pushing metadata predicates into SQL is a
// deferred optimization for this engine; nil is the interface's defined
// fall-back path and is always correct.
func (e *serenedbEngine) FilterDocIdsByMetaPushdown(ctx context.Context, sqlDB *gorm.DB, kbIDs []string, conditions []map[string]interface{}, logic string) []string {
	return nil
}

// loadMetaFields returns the parsed meta_fields map for a record, or nil when
// the record does not exist.
func (e *serenedbEngine) loadMetaFields(ctx context.Context, tableName, docID, datasetID string) (map[string]interface{}, error) {
	rows, err := e.queryMaps(ctx,
		fmt.Sprintf("SELECT meta_fields FROM %s WHERE id = $1 AND kb_id = $2", tableName), docID, datasetID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	switch mf := rows[0]["meta_fields"].(type) {
	case map[string]interface{}:
		return mf, nil
	case nil:
		return map[string]interface{}{}, nil
	default:
		return map[string]interface{}{}, nil
	}
}
