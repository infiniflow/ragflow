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
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/types"
	"ragflow/internal/utility"

	infinity "github.com/infiniflow/infinity-go-sdk"

	"go.uber.org/zap"
)

// CreateMetadataStore creates a metadata table in Infinity
// tenantID is the tenant identifier used to build the table name
func (e *infinityEngine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	tableName := buildMetadataTableName(tenantID)

	// Get database
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}

	// Check if table already exists
	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		return fmt.Errorf("failed to check if table exists: %w", err)
	}
	if exists {
		return fmt.Errorf("metadata table '%s' already exists", tableName)
	}

	fpMapping, err := utility.FindConfFileInProject(e.docMetaMappingFileName)
	if err != nil {
		return err
	}

	// Use configured doc_meta mapping file
	schemaData, err := os.ReadFile(*fpMapping)
	if err != nil {
		return fmt.Errorf("failed to read mapping file %q: %w", *fpMapping, err)
	}

	var schema map[string]fieldInfo
	if err = json.Unmarshal(schemaData, &schema); err != nil {
		return fmt.Errorf("failed to parse mapping file %q: %w", *fpMapping, err)
	}

	// Build column definitions
	var columns infinity.TableSchema
	for fieldName, fieldInfo := range schema {
		col := infinity.ColumnDefinition{
			Name:     fieldName,
			DataType: fieldInfo.Type,
			Default:  fieldInfo.Default,
		}
		columns = append(columns, &col)
	}

	// Create table
	_, err = db.CreateTable(tableName, columns, infinity.ConflictTypeIgnore)
	if err != nil {
		return fmt.Errorf("failed to create doc meta table: %w", err)
	}
	common.Debug("Infinity created doc meta table", zap.String("tableName", tableName))

	// Get table for creating indexes
	table, err := db.GetTable(tableName)
	if err != nil {
		return fmt.Errorf("failed to get table: %w", err)
	}

	// Create secondary index on id
	_, err = table.CreateIndex(
		fmt.Sprintf("idx_%s_id", tableName),
		infinity.NewIndexInfo("id", infinity.IndexTypeSecondary, nil),
		infinity.ConflictTypeIgnore,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to create secondary index on id: %w", err)
	}

	// Create secondary index on kb_id
	_, err = table.CreateIndex(
		fmt.Sprintf("idx_%s_kb_id", tableName),
		infinity.NewIndexInfo("kb_id", infinity.IndexTypeSecondary, nil),
		infinity.ConflictTypeIgnore,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to create secondary index on kb_id: %w", err)
	}

	// Create secondary index on meta_fields for metadata filter queries
	_, err = table.CreateIndex(
		fmt.Sprintf("idx_%s_meta_fields", tableName),
		infinity.NewIndexInfo("meta_fields", infinity.IndexTypeSecondary, nil),
		infinity.ConflictTypeIgnore,
		"",
	)
	if err != nil {
		return fmt.Errorf("failed to create secondary index on meta_fields: %w", err)
	}

	return nil
}

// InsertMetadata inserts document metadata into tenant's metadata table
// Auto-create the table if it doesn't exist
// Replace existing metadata with same id and kb_id
func (e *infinityEngine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	tableName := buildMetadataTableName(tenantID)
	common.Info("InfinityConnection.InsertMetadata called", zap.String("tableName", tableName), zap.Int("metaCount", len(metadata)))

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		// Table doesn't exist, try to create it
		errMsg := strings.ToLower(err.Error())
		if !strings.Contains(errMsg, "not found") && !strings.Contains(errMsg, "doesn't exist") {
			return nil, fmt.Errorf("failed to get table %s: %w", tableName, err)
		}

		// Create metadata table
		if createErr := e.CreateMetadataStore(ctx, tenantID); createErr != nil {
			return nil, fmt.Errorf("failed to create metadata table: %w", createErr)
		}

		table, err = db.GetTable(tableName)
		if err != nil {
			return nil, fmt.Errorf("failed to get table after creation: %w", err)
		}
	}

	// Transform metadata - convert meta_fields map to JSON string
	insertMetadata := make([]map[string]interface{}, len(metadata))
	for i, m := range metadata {
		d := make(map[string]interface{})
		for k, v := range m {
			if k == "meta_fields" {
				d["meta_fields"] = utility.ConvertMapToJSONString(v)
			} else {
				d[k] = v
			}
		}
		insertMetadata[i] = d
	}

	// Delete existing metadata with same id and kb_id, then insert new
	if len(insertMetadata) > 0 {
		idList := make([]string, len(insertMetadata))
		for i, m := range insertMetadata {
			// Escape single quotes in values to prevent SQL injection
			docID := fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", m["id"]), "'", "''"))
			kbID := fmt.Sprintf("'%s'", strings.ReplaceAll(fmt.Sprintf("%v", m["kb_id"]), "'", "''"))
			idList[i] = fmt.Sprintf("(id = %s AND kb_id = %s)", docID, kbID)
		}
		filter := strings.Join(idList, " OR ")
		common.Debug(fmt.Sprintf("Deleting existing metadata with filter: %s", filter))
		delResp, delErr := table.Delete(filter)
		if delErr != nil {
			common.Warn(fmt.Sprintf("Failed to delete existing metadata: %v", delErr))
		} else if delResp.DeletedRows > 0 {
			common.Info(fmt.Sprintf("Deleted %d existing metadata entries", delResp.DeletedRows))
		}
	}

	// Insert metadata
	_, err = table.Insert(insertMetadata)
	if err != nil {
		return nil, fmt.Errorf("failed to insert metadata: %w", err)
	}

	common.Info("InfinityConnection.InsertMetadata result", zap.String("tableName", tableName), zap.Int("metaCount", len(metadata)))
	return []string{}, nil
}

// UpdateMetadata updates or inserts document metadata in tenant's metadata table.
//
// "Updates" here means MERGE, not replace. The supplied metaFields are
// overlaid on top of the row's existing meta_fields map: keys already
// present are overwritten with the new value, keys not in the input
// are preserved, and brand-new keys are added. If no row exists for
// (docID, datasetID), one is inserted containing exactly metaFields.
//
// Examples (existing row → input → resulting meta_fields):
//
//	{character:["曹操","孙权"], year:2025}
//	  + {author:["John","Tom"], category:"tech"}
//	  = {character:["曹操","孙权"], year:2025, author:["John","Tom"], category:"tech"}
//
//	{character:["曹操","孙权"], year:2025}
//	  + {year:2025}
//	  = {character:["曹操","孙权"], year:2025}    // year value unchanged, character preserved
//
//	(empty / row absent) + {author:"Tom"} = {author:"Tom"}
//
// Note: this is at odds with the SET-METADATA CLI's name, which a
// reader naturally parses as "replace". The merge semantics exist so
// that user-driven metadata edits compose with auto-extracted fields
// produced by the LLM extraction pipeline. See the CLI parser in
// internal/cli/user_parser.go (parseDevSetMeta) for the user-facing
// surface that drives this engine method.
func (e *infinityEngine) UpdateMetadata(ctx context.Context, docID string, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	tableName := buildMetadataTableName(tenantID)
	common.Info("InfinityConnection.UpdateMetadata called", zap.String("tableName", tableName), zap.String("docID", docID), zap.String("datasetID", datasetID))

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		return fmt.Errorf("failed to get metadata table %s: %w", tableName, err)
	}

	// Build filter to find existing row by docID and datasetID
	escapedDocID := strings.ReplaceAll(docID, "'", "''")
	escapedDatasetID := strings.ReplaceAll(datasetID, "'", "''")
	filter := fmt.Sprintf("id = '%s' AND kb_id = '%s'", escapedDocID, escapedDatasetID)

	// Query existing metadata using the chainable API
	queryTable := table.Output([]string{"id", "kb_id", "meta_fields"}).Filter(filter).Limit(1).Offset(0)

	// Execute query to check if row exists
	result, err := queryTable.ToResult()
	rowExists := false
	if err != nil {
		common.Warn(fmt.Sprintf("Failed to query existing metadata: %v", err))
		// If query fails, treat as not exists and insert
	} else {
		// Get results - ToResult returns *infinity.QueryResult
		qr, ok := result.(*infinity.QueryResult)
		// Check if id column has any rows - len(qr.Data["id"]) > 0 means there are rows
		if ok && qr != nil && len(qr.Data["id"]) > 0 {
			rowExists = true
			// Get meta_fields from the first row
			if metaFieldsData, exists := qr.Data["meta_fields"]; exists && len(metaFieldsData) > 0 {
				existingMetaFieldsVal := metaFieldsData[0]

				// Parse existing meta_fields if it's a string or []uint8
				var existingMetaFields map[string]interface{}
				if existingMetaFieldsVal != nil {
					switch v := existingMetaFieldsVal.(type) {
					case string:
						if err := json.Unmarshal([]byte(v), &existingMetaFields); err != nil {
							common.Warn(fmt.Sprintf("Failed to parse existing meta_fields: %v", err))
							existingMetaFields = make(map[string]interface{})
						}
					case []uint8:
						// Handle raw bytes from Infinity - Infinity prefixes JSON with 4 bytes (likely length), skip them
						decoded := v
						if len(decoded) > 4 {
							decoded = decoded[4:]
						}
						if err := json.Unmarshal(decoded, &existingMetaFields); err != nil {
							common.Warn(fmt.Sprintf("Failed to parse existing meta_fields from []uint8: %v", err))
							existingMetaFields = make(map[string]interface{})
						}
					case map[string]interface{}:
						existingMetaFields = v
					}
				}

				// Merge new meta_fields with existing (new values override existing)
				if existingMetaFields == nil {
					existingMetaFields = make(map[string]interface{})
				}
				for k, v := range metaFields {
					existingMetaFields[k] = v
				}
				metaFields = existingMetaFields
			}
		}
	}

	// Prepare updated metadata as JSON string
	updatedFields := map[string]interface{}{
		"meta_fields": utility.ConvertMapToJSONString(metaFields),
	}

	if rowExists {
		// Row exists: update it with merged metadata
		common.Info(fmt.Sprintf("UpdateMetadata: updating existing row, table=%s, filter=%s, newValue=%v", tableName, filter, updatedFields))
		_, err = table.Update(filter, updatedFields)
		if err != nil {
			return fmt.Errorf("failed to update metadata: %w", err)
		}
	} else {
		// Row doesn't exist: insert new row
		insertFields := map[string]interface{}{
			"id":          docID,
			"kb_id":       datasetID,
			"meta_fields": utility.ConvertMapToJSONString(metaFields),
		}
		common.Info(fmt.Sprintf("UpdateMetadata: inserting new row, table=%s, newValue=%v", tableName, insertFields))
		_, err = table.Insert(insertFields)
		if err != nil {
			return fmt.Errorf("failed to insert metadata: %w", err)
		}
	}

	common.Info("InfinityConnection.UpdateMetadata completes", zap.String("tableName", tableName), zap.String("docID", docID))
	return nil
}

// DeleteMetadata deletes metadata from tenant's metadata table by condition
// Returns the number of deleted documents.
func (e *infinityEngine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	tableName := buildMetadataTableName(tenantID)

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return 0, fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		common.Warn(fmt.Sprintf("Metadata table %s does not exist, skipping delete", tableName))
		return 0, nil
	}

	// Get table columns for building filter
	clmns := make(map[string]struct {
		Type    string
		Default interface{}
	})
	colsResp, err := table.ShowColumns()
	if err != nil {
		return 0, fmt.Errorf("failed to get columns: %w", err)
	}
	result, ok := colsResp.(*infinity.QueryResult)
	if ok {
		if nameArr, ok := result.Data["name"]; ok {
			if typeArr, ok := result.Data["type"]; ok {
				if defArr, ok := result.Data["default"]; ok {
					for i := 0; i < len(nameArr); i++ {
						colName, _ := nameArr[i].(string)
						colType, _ := typeArr[i].(string)
						var colDefault interface{}
						if i < len(defArr) {
							colDefault = defArr[i]
						}
						clmns[colName] = struct {
							Type    string
							Default interface{}
						}{colType, colDefault}
					}
				}
			}
		}
	}

	// Build filter from condition
	filter := buildFilterFromCondition(condition, clmns)

	delResp, err := table.Delete(filter)
	if err != nil {
		return 0, fmt.Errorf("failed to delete metadata: %w", err)
	}

	return delResp.DeletedRows, nil
}

// DeleteMetadataKeys deletes specific metadata keys from a document's meta_fields.
// If deleting those keys leaves no metadata entries, the metadata row is removed.
func (e *infinityEngine) DeleteMetadataKeys(ctx context.Context, docID string, datasetID string, keys []string, tenantID string) error {
	tableName := buildMetadataTableName(tenantID)
	common.Info("InfinityConnection.DeleteMetadataKeys called", zap.String("tableName", tableName), zap.String("docID", docID), zap.Any("keys", keys))

	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return fmt.Errorf("failed to get database: %w", err)
	}

	table, err := db.GetTable(tableName)
	if err != nil {
		return fmt.Errorf("failed to get metadata table %s: %w", tableName, err)
	}

	// Build filter to find the document
	escapedDocID := strings.ReplaceAll(docID, "'", "''")
	escapedDatasetID := strings.ReplaceAll(datasetID, "'", "''")
	filter := fmt.Sprintf("id = '%s' AND kb_id = '%s'", escapedDocID, escapedDatasetID)

	// Query existing metadata to get current meta_fields
	queryTable := table.Output([]string{"id", "kb_id", "meta_fields"}).Filter(filter).Limit(1).Offset(0)
	result, err := queryTable.ToResult()
	if err != nil {
		return fmt.Errorf("failed to query existing metadata: %w", err)
	}

	qr, ok := result.(*infinity.QueryResult)
	if !ok || qr == nil || len(qr.Data["id"]) == 0 {
		return fmt.Errorf("document not found: %s", docID)
	}

	// Get existing meta_fields
	var existingMetaFields map[string]interface{}
	if metaFieldsData, exists := qr.Data["meta_fields"]; exists && len(metaFieldsData) > 0 {
		if metaFieldsData[0] != nil {
			switch v := metaFieldsData[0].(type) {
			case string:
				if err := json.Unmarshal([]byte(v), &existingMetaFields); err != nil {
					common.Warn("Failed to parse meta_fields JSON", zap.String("docID", docID), zap.Error(err))
					existingMetaFields = make(map[string]interface{})
				}
			case []uint8:
				// Handle raw bytes from Infinity - Infinity prefixes JSON with 4 bytes (likely length), skip them
				decoded := v
				if len(decoded) > 4 {
					decoded = decoded[4:]
				}
				if err := json.Unmarshal(decoded, &existingMetaFields); err != nil {
					common.Warn("Failed to parse meta_fields JSON from []uint8", zap.String("docID", docID), zap.String("err", err.Error()))
					existingMetaFields = make(map[string]interface{})
				}
			case map[string]interface{}:
				existingMetaFields = v
			default:
				common.Debug("meta_fields unexpected type", zap.String("type", fmt.Sprintf("%T", metaFieldsData[0])), zap.Any("value", metaFieldsData[0]))
			}
		}
	} else {
		common.Debug("meta_fields not found in qr.Data or empty", zap.Any("exists", exists))
	}

	if existingMetaFields == nil {
		existingMetaFields = make(map[string]interface{})
	}

	// Build set of keys to remove
	keysToRemove := make(map[string]bool)
	for _, k := range keys {
		keysToRemove[k] = true
	}

	// Check if any keys actually exist and would be removed
	hasKeysToRemove := false
	for k := range existingMetaFields {
		if keysToRemove[k] {
			hasKeysToRemove = true
			break
		}
	}

	if !hasKeysToRemove {
		common.Info(
			"No matching keys to delete from document",
			zap.String("docID", docID),
			zap.Int("existingMetaFieldCount", len(existingMetaFields)),
			zap.Int("keysCount", len(keys)),
		)
		return nil
	}

	// Count remaining keys after deletion (keys that are NOT being removed)
	remainingKeys := 0
	for k := range existingMetaFields {
		if !keysToRemove[k] {
			remainingKeys++
		}
	}

	// If no other keys would remain after deletion, delete the document directly
	if remainingKeys == 0 {
		common.Info("All metadata keys would be deleted, removing document from index", zap.String("docID", docID))

		// Use existing DeleteMetadata method which handles the deletion properly
		condition := map[string]interface{}{
			"id":    docID,
			"kb_id": datasetID,
		}
		_, err := e.DeleteMetadata(ctx, condition, tenantID)
		if err != nil {
			return fmt.Errorf("failed to delete document: %w", err)
		}

		common.Info("Successfully removed document with empty meta_fields", zap.String("docID", docID))
		return nil
	}

	// Some keys will remain, so remove only the specified keys
	for _, key := range keys {
		delete(existingMetaFields, key)
	}

	// Update with the modified metadata
	updatedFields := map[string]interface{}{
		"meta_fields": utility.ConvertMapToJSONString(existingMetaFields),
	}

	_, err = table.Update(filter, updatedFields)
	if err != nil {
		return fmt.Errorf("failed to delete metadata keys: %w", err)
	}

	common.Info("InfinityConnection.DeleteMetadataKeys completed", zap.String("tableName", tableName), zap.String("docID", docID))
	return nil
}

// DropMetadataStore drops a metadata table from Infinity
func (e *infinityEngine) DropMetadataStore(ctx context.Context, tenantID string) error {
	tableName := buildMetadataTableName(tenantID)
	return e.dropTable(ctx, tableName)
}

// MetadataStoreExists checks if a metadata table exists in Infinity
func (e *infinityEngine) MetadataStoreExists(ctx context.Context, tenantID string) (bool, error) {
	tableName := buildMetadataTableName(tenantID)
	return e.tableExists(ctx, tableName)
}

// SearchMetadata executes search specifically for metadata tables
// This is separate from Search() which handles only chunk tables
func (e *infinityEngine) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	tenantID := req.TenantID
	common.Debug("SearchMetadata in Infinity started", zap.String("tenantID", tenantID))

	// Validate inputs
	if tenantID == "" {
		return nil, fmt.Errorf("tenantID cannot be empty")
	}

	// Build table name from tenantID
	tableName := buildMetadataTableName(tenantID)

	exists, err := e.tableExists(ctx, tableName)
	if err != nil {
		common.Warn("Infinity SearchMetadata table existence check failed", zap.String("table", tableName), zap.Error(err))
		return nil, fmt.Errorf("failed to check metadata table existence: %w", err)
	}
	if !exists {
		common.Debug("Infinity SearchMetadata table absent, returning empty result", zap.String("table", tableName))
		// Return an empty (non-nil) slice — Python returns `[]`, and a
		// nil slice is read by callers as "fall back to in-memory". A
		// zero-match against an absent table is a definitive answer,
		// not a missing-data condition.
		return &types.SearchMetadataResult{
			MetadataRecords: []map[string]interface{}{},
			Total:           0,
		}, nil
	}

	// Build output columns: use caller-specified fields, or "*" for all columns
	var outputColumns []string
	if len(req.SelectFields) > 0 {
		outputColumns = req.SelectFields
	} else {
		outputColumns = []string{"*"}
	}

	// Pagination defaults
	pageSize := req.Limit
	if pageSize <= 0 {
		pageSize = 30
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	// Build filter from req.Filter
	var filterStr string
	if req.Filter != nil {
		filterStr = equivalentConditionToStr(req.Filter)
	}

	// Get database and table
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil {
		return nil, fmt.Errorf("failed to get database: %w", err)
	}

	tbl, err := db.GetTable(tableName)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata table %s: %w", tableName, err)
	}

	// Build Infinity query (chainable API)
	table := tbl.Output(outputColumns)
	if filterStr != "" {
		table = table.Filter(filterStr)
	}

	// Add order_by if provided
	if req.OrderBy != nil && len(req.OrderBy.Fields) > 0 {
		var sortFields [][2]interface{}
		for _, orderField := range req.OrderBy.Fields {
			sortType := infinity.SortTypeAsc
			if orderField.Type == types.SortDesc {
				sortType = infinity.SortTypeDesc
			}
			sortFields = append(sortFields, [2]interface{}{orderField.Field, sortType})
		}
		table = table.Sort(sortFields)
	}

	table = table.Limit(pageSize)
	if offset > 0 {
		table = table.Offset(offset)
	}
	table = table.Option(map[string]interface{}{"total_hits_count": true})

	// Execute query
	df, err := table.ToDataFrame()
	if err != nil {
		common.Warn("Infinity SearchMetadata query failed",
			zap.String("tableName", tableName),
			zap.Error(err))
		return nil, fmt.Errorf("metadata query failed: %w", err)
	}

	// Convert column-oriented DataFrame to row-oriented records
	records := make([]map[string]interface{}, 0)
	for colName, colData := range df.ColumnData {
		for i, val := range colData {
			for len(records) <= i {
				records = append(records, make(map[string]interface{}))
			}
			records[i][colName] = val
		}
	}

	// Handle ROW_ID -> row_id() mapping (Infinity internal column)
	for _, rec := range records {
		if val, ok := rec["ROW_ID"]; ok {
			rec["row_id()"] = val
			delete(rec, "ROW_ID")
		}
	}

	// Realign meta_fields column for multi-row queries (Infinity may
	// concatenate values into one blob with 4-byte length prefix)
	realignMetaFieldsColumn(records)

	// Parse total_hits_count from ExtraInfo
	var totalHits int64
	if df.ExtraInfo != "" {
		if t, ok := totalHitsFromInfinityExtraInfo(df.ExtraInfo); ok {
			totalHits = t
		}
	}

	common.Debug("SearchMetadata in Infinity completed",
		zap.Int("rows", len(records)),
		zap.Int64("total", totalHits))

	return &types.SearchMetadataResult{
		MetadataRecords: records,
		Total:           totalHits,
	}, nil
}

// parseLengthPrefixedJSON parses Infinity's length-prefixed JSON format
// (a sequence of [4-byte little-endian length][JSON] records) and returns
// each parsed JSON object. This is the same on-the-wire format that the
// service layer's ParseAllLengthPrefixedJSON understands; duplicated here
// to keep the engine package free of service-layer dependencies.
//
// The format is what Infinity's SDK returns for VARCHAR/TEXT columns
// when a query matches multiple rows: instead of giving us a list of
// per-row byte arrays, it concatenates all rows' values into a single
// blob, prefixing each with a 4-byte little-endian length.
//
// Returns nil if `data` is too short to be valid, or if no JSON
// objects could be extracted.
func parseLengthPrefixedJSON(data []byte) []map[string]interface{} {
	if len(data) < 4 {
		return nil
	}
	var results []map[string]interface{}
	offset := 0
	for offset+4 <= len(data) {
		// Read 4-byte length (little-endian)
		length := uint32(data[offset]) |
			uint32(data[offset+1])<<8 |
			uint32(data[offset+2])<<16 |
			uint32(data[offset+3])<<24
		if length == 0 || offset+4+int(length) > len(data) {
			// Length invalid; bail out.
			break
		}
		jsonStart := offset + 4
		jsonEnd := jsonStart + int(length)
		var result map[string]interface{}
		if err := json.Unmarshal(data[jsonStart:jsonEnd], &result); err == nil {
			results = append(results, result)
			offset = jsonEnd
			continue
		}
		break
	}
	return results
}

// realignMetaFieldsColumn fixes a column-oriented data-frame
// misalignment that happens when Infinity's SDK returns the
// `meta_fields` column for a multi-row query as a single
// length-prefixed byte array instead of one entry per row. After the
// column→row loop has run, the first matching chunk holds the entire
// concatenated blob and the rest are missing the field. This function
// splits the blob into per-row JSON objects and reattaches them in
// order to the chunks that need them.
//
// Safe no-op when:
//   - there are no chunks
//   - the `meta_fields` column is already aligned (one byte array per
//     chunk), so a length-prefixed parse of any single value yields
//     exactly one object
//   - the byte array doesn't parse as length-prefixed JSON
func realignMetaFieldsColumn(chunks []map[string]interface{}) {
	if len(chunks) < 2 {
		return
	}
	firstVal, ok := chunks[0]["meta_fields"]
	if !ok {
		return
	}
	firstBytes, ok := firstVal.([]byte)
	if !ok {
		return
	}
	parsed := parseLengthPrefixedJSON(firstBytes)
	if len(parsed) != len(chunks) {
		// Either the blob didn't parse as length-prefixed, or it
		// parsed to a different count than the number of chunks we
		// built. In either case, don't risk misattributing data.
		return
	}
	for i, meta := range parsed {
		chunks[i]["meta_fields"] = meta
	}
}

// metaPushdownMaxSize caps how many doc IDs the metadata push-down is
// willing to return in one shot. Matches the Python reference
// (DocMetadataService.filter_doc_ids_by_meta_pushdown, default limit=10000)
// and ES's default index.max_result_window.
//
// When the underlying query matches more than this, the push-down
// returns nil and the caller falls back to the in-memory meta_filter,
// which is correct (just slower for very large result sets). Returning
// a truncated slice as a definitive answer would silently drop docs.
const metaPushdownMaxSize = 10000

// FilterDocIdsByMetaPushdown runs a metadata filter directly against the Infinity table.
//
// Return value convention (matching Python's filter_doc_ids_by_meta_pushdown):
//
//	nil        -> push-down was not viable / errored / result overflowed the
//	              push-down cap (caller should fall back to in-memory)
//	[]string{} -> push-down succeeded but found 0 matching docs (empty result is definitive)
func (e *infinityEngine) FilterDocIdsByMetaPushdown(ctx context.Context, kbIDs []string, conditions []map[string]interface{}, logic string) []string {
	if len(conditions) == 0 || len(kbIDs) == 0 {
		return nil
	}

	// Check if push-down is supported
	if !IsPushdownSupported(conditions) {
		common.Debug("FilterDocIdsByMetaPushdown: push-down not supported for some filters")
		return nil
	}

	// Get tenant ID from first KB
	tenantID, err := dao.GetTenantIDByKBID(kbIDs[0])
	if err != nil {
		common.Warn("FilterDocIdsByMetaPushdown: failed to get tenant for KB", zap.String("kbID", kbIDs[0]), zap.Error(err))
		return nil
	}

	tableName := buildMetadataTableName(tenantID)

	// Build SQL WHERE clause using the full meta_filter logic
	whereClause, err := BuildInfinityFilter(conditions, logic)
	if err != nil {
		common.Debug("FilterDocIdsByMetaPushdown: build filter failed", zap.String("error", err.Error()))
		return nil
	}

	// Add KB filter using IN clause. Escape any single quotes in the IDs
	// defensively — KB IDs are normally UUIDs, but malformed input must
	// not be able to break out of the literal and alter the query.
	quotedKBIDs := make([]string, len(kbIDs))
	for i, kbID := range kbIDs {
		quotedKBIDs[i] = "'" + strings.ReplaceAll(kbID, "'", "''") + "'"
	}
	kbFilter := "kb_id IN (" + strings.Join(quotedKBIDs, ", ") + ")"
	// Wrap the translated predicate in parens so the AND with the KB clause
	// doesn't get re-grouped by an internal OR. Without the parens,
	// `kbFilter AND a OR b` parses as `(kbFilter AND a) OR b`, which can
	// match rows in other KBs.
	whereClause = kbFilter + " AND (" + whereClause + ")"

	// Use Infinity connection to execute query
	db, err := e.client.conn.GetDatabase(e.client.dbName)
	if err != nil || db == nil {
		return nil
	}

	table, err := db.GetTable(tableName)
	if err != nil || table == nil {
		return nil
	}

	// Execute query using chainable API: Output(...).Filter(...)
	// .Limit(metaPushdownMaxSize) caps the page size, and
	// .Option({total_hits_count: true}) makes the exact match count
	// available in QueryResult.ExtraInfo so we can detect overflow and
	// fall back to the in-memory meta_filter rather than silently
	// returning a truncated slice (which the caller treats as definitive).
	common.Debug("FilterDocIdsByMetaPushdown executing Infinity query", zap.String("whereClause", whereClause))
	queryTable := table.Output([]string{"id"}).Filter(whereClause)
	queryTable = queryTable.Limit(metaPushdownMaxSize)
	queryTable = queryTable.Option(map[string]interface{}{"total_hits_count": true})
	result, err := queryTable.ToResult()
	if err != nil {
		return nil
	}

	qr, ok := result.(*infinity.QueryResult)
	if !ok || qr == nil {
		return nil
	}

	// Detect overflow via the SDK's ExtraInfo payload (a JSON string set
	// when total_hits_count is requested). If we can't parse it, log
	// and fall through — the in-memory path is still correct, just
	// slower.
	if total, parsed := totalHitsFromInfinityExtraInfo(qr.ExtraInfo); parsed {
		if total > int64(metaPushdownMaxSize) {
			common.Warn("FilterDocIdsByMetaPushdown: result exceeds push-down cap, falling back to in-memory",
				zap.Int64("total", total),
				zap.Int("cap", metaPushdownMaxSize),
				zap.Strings("kbIDs", kbIDs),
			)
			return nil
		}
	} else if qr.ExtraInfo != "" {
		// ExtraInfo was non-empty but didn't carry total_hits_count in the
		// expected shape — unusual, but worth flagging so we don't quietly
		// lose the overflow signal if Infinity changes its payload.
		common.Debug("FilterDocIdsByMetaPushdown: Infinity ExtraInfo present but total_hits_count missing",
			zap.String("extraInfo", qr.ExtraInfo),
		)
	}

	// Extract doc IDs from the result.
	docIDs := make([]string, 0)
	if idData, exists := qr.Data["id"]; exists {
		for _, id := range idData {
			if idStr, ok := id.(string); ok {
				docIDs = append(docIDs, idStr)
			}
		}
	}

	common.Debug("FilterDocIdsByMetaPushdown returned doc IDs", zap.Int("count", len(docIDs)))
	return docIDs
}

// totalHitsFromInfinityExtraInfo parses the JSON blob Infinity returns
// in QueryResult.ExtraInfo when the total_hits_count option is set. The
// shape is not part of the public SDK contract today (it's a string
// field with an undocumented layout), so we accept several common
// spellings and stay tolerant of future changes.
//
// Returns (total, true) when a non-negative integer is found, otherwise
// (0, false) so the caller can decide how to react.
func totalHitsFromInfinityExtraInfo(extraInfo string) (int64, bool) {
	if extraInfo == "" {
		return 0, false
	}
	// Try a permissive decode first — Infinity has historically
	// returned things like {"total_hits_count": 42} but we don't want
	// to bind to that exact shape forever.
	var generic map[string]interface{}
	if err := json.Unmarshal([]byte(extraInfo), &generic); err != nil {
		return 0, false
	}
	for _, key := range []string{"total_hits_count", "totalHitsCount", "total"} {
		raw, ok := generic[key]
		if !ok {
			continue
		}
		switch v := raw.(type) {
		case float64:
			if v < 0 {
				return 0, false
			}
			return int64(v), true
		case int64:
			if v < 0 {
				return 0, false
			}
			return v, true
		case int:
			if v < 0 {
				return 0, false
			}
			return int64(v), true
		case json.Number:
			n, err := v.Int64()
			if err == nil && n >= 0 {
				return n, true
			}
		}
	}
	return 0, false
}
