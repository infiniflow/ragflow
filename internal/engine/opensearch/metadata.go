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

package opensearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/internal/esoscommon"
	"ragflow/internal/engine/metafilter"
	"ragflow/internal/engine/types"
	"ragflow/internal/utility"

	"go.uber.org/zap"
)

// CreateMetadataStore creates the document metadata index
func (e *Engine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)

	// Check if index already exists
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if exists {
		return nil
	}

	mappingPath, err := utility.FindConfFileInProject("doc_meta_es_mapping.json")
	if err != nil {
		return err
	}
	data, err := os.ReadFile(*mappingPath)
	if err != nil {
		return fmt.Errorf("failed to read mapping file %q: %w", *mappingPath, err)
	}

	// OpenSearch does not support ES's "runtime" dynamic mapping mode; rewrite
	// it to a plain dynamic mapping before creating the index (mirrors Python's
	// opensearch_conn.create_doc_meta_idx).
	var mapping map[string]interface{}
	if err := json.Unmarshal(data, &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping file %q: %w", *mappingPath, err)
	}
	if mappings, ok := mapping["mappings"].(map[string]interface{}); ok {
		if dyn, ok := mappings["dynamic"].(string); ok && dyn == "runtime" {
			mappings["dynamic"] = true
		}
	}
	mappingBytes, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("failed to marshal mapping: %w", err)
	}

	result, err := e.doIndicesCreate(ctx, indexName, bytes.NewReader(mappingBytes))
	if err != nil {
		return fmt.Errorf("failed to create metadata index: %w", err)
	}

	acknowledged, ok := result["acknowledged"].(bool)
	if !ok || !acknowledged {
		return fmt.Errorf("metadata index creation not acknowledged")
	}

	return nil
}

// InsertMetadata inserts documents into tenant's metadata index
// If a document with the same id and kb_id already exists, it will be updated with the new value
func (e *Engine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("OpenSearchConnection.InsertMetadata called", zap.String("index_name", indexName), zap.String("tenant_id", tenantID), zap.Int("doc_count", len(metadata)))

	if len(metadata) == 0 {
		return []string{}, nil
	}

	if indexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}

	// Build bulk request body
	var buf bytes.Buffer
	for _, doc := range metadata {
		docIDRaw, hasID := doc["id"]
		kbIDRaw, hasKBID := doc["kb_id"]
		docID, idOK := docIDRaw.(string)
		kbID, kbOK := kbIDRaw.(string)
		if !hasID || !hasKBID || !idOK || !kbOK || strings.TrimSpace(docID) == "" || strings.TrimSpace(kbID) == "" {
			common.Warn("Skipping metadata document without id or kb_id")
			continue
		}

		// Action line: use the plain document id as _id, matching Python's
		// es_conn.insert (readers shim hit._id back into the "id" field).
		action, err := json.Marshal(map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
				"_id":    docID,
			},
		})
		if err != nil {
			common.Error("Failed to marshal bulk action", err)
			return nil, fmt.Errorf("failed to marshal bulk action: %w", err)
		}
		buf.Write(action)
		buf.WriteByte('\n')

		// Document line
		if err := json.NewEncoder(&buf).Encode(doc); err != nil {
			return nil, fmt.Errorf("failed to encode document: %w", err)
		}
	}

	// Execute bulk request
	bulkResponse, err := e.doBulk(ctx, bytes.NewReader(buf.Bytes()), "wait_for")
	if err != nil {
		common.Error("Failed to execute bulk request", err)
		return nil, fmt.Errorf("failed to execute bulk request: %w", err)
	}

	// A bulk request can index some documents and reject others, so a
	// top-level "errors": true must surface the failed items to the caller
	// instead of only producing a warning.
	if hasErrors, ok := bulkResponse["errors"].(bool); ok && hasErrors {
		items, _ := bulkResponse["items"].([]interface{})
		for _, item := range items {
			itemMap, _ := item.(map[string]interface{})
			for _, op := range itemMap {
				opMap, _ := op.(map[string]interface{})
				errInfo, ok := opMap["error"].(map[string]interface{})
				if !ok {
					continue
				}
				common.Error("Bulk metadata insert failed",
					fmt.Errorf("reason: %v", errInfo["reason"]),
					zap.Any("_id", opMap["_id"]),
					zap.String("index_name", indexName))
				return nil, fmt.Errorf("bulk metadata insert failed for _id %v: %v", opMap["_id"], errInfo["reason"])
			}
		}
		return nil, fmt.Errorf("bulk metadata insert reported errors")
	}

	common.Info("OpenSearchConnection.InsertMetadata result", zap.String("index_name", indexName), zap.Int("count", len(metadata)))
	return []string{}, nil
}

// UpdateMetadata fully replaces the meta_fields for a document in the
// document engine. Callers must send the complete desired meta_fields map —
// unchanged keys are NOT preserved (unlike a merge). This mirrors Python's
// replace_meta_fields semantics: stale keys must not survive an update.
//
// The metadata index must already exist; the service layer is responsible
// for creating it before writing.
func (e *Engine) UpdateMetadata(ctx context.Context, docID string, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("OpenSearchConnection.UpdateMetadata called", zap.String("index_name", indexName), zap.String("docID", docID), zap.String("datasetID", datasetID))

	// Build the document ID for update
	docIDStr := strings.ReplaceAll(docID, "'", "''")
	datasetIDStr := strings.ReplaceAll(datasetID, "'", "''")

	// Build update body - merge meta_fields with existing
	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []map[string]interface{}{
				{"term": map[string]interface{}{"id": docIDStr}},
				{"term": map[string]interface{}{"kb_id": datasetIDStr}},
			},
		},
	}

	// Painless script: fully replace meta_fields (mirrors Python's
	// replace_meta_fields — stale keys must not survive the update).
	updateReq := map[string]interface{}{
		"query":     query,
		"conflicts": "proceed",
		"script": map[string]interface{}{
			"source": "ctx._source.meta_fields = params.meta_fields;",
			"lang":   "painless",
			"params": map[string]interface{}{
				"meta_fields": metaFields,
			},
		},
	}

	updateBytes, err := json.Marshal(updateReq)
	if err != nil {
		return fmt.Errorf("failed to marshal update request: %w", err)
	}

	updateResponse, err := e.doUpdateByQuery(ctx, []string{indexName}, bytes.NewReader(updateBytes))
	if err != nil {
		common.Error("Failed to execute update by query", err)
		return fmt.Errorf("failed to execute update by query: %w", err)
	}

	// A 200 response can still report that work timed out or individual
	// documents failed; do not fall back to an insert in that case, since the
	// document may exist and be partially updated.
	if err := checkUpdateByQueryResult(updateResponse, indexName, "update metadata by query"); err != nil {
		return err
	}

	if total, ok := updateResponse["total"].(float64); ok && total == 0 {
		_, err := e.InsertMetadata(ctx, []map[string]interface{}{
			{
				"id":          docID,
				"kb_id":       datasetID,
				"meta_fields": metaFields,
			},
		}, tenantID)
		if err != nil {
			return fmt.Errorf("failed to insert metadata: %w", err)
		}
	}

	common.Info("OpenSearchConnection.UpdateMetadata completes", zap.String("index_name", indexName), zap.String("docID", docID))
	return nil
}

// DeleteMetadata deletes metadata from tenant's metadata index by condition
// The condition is a map used to build a query (e.g., map["kb_id"]="xxx")
// Returns the number of deleted documents
func (e *Engine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("OpenSearchConnection.DeleteMetadata called", zap.String("index_name", indexName), zap.Any("condition", condition))

	// Check if index exists
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return 0, fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		common.Warn(fmt.Sprintf("Index %s does not exist, skipping delete", indexName))
		return 0, nil
	}

	// Build query from condition
	query := e.buildMetadataQueryFromCondition(condition)
	if query == nil {
		query = map[string]interface{}{"match_all": map[string]interface{}{}}
	}

	// Build delete by query body
	deleteBody := map[string]interface{}{
		"query": query,
	}

	bodyBytes, err := json.Marshal(deleteBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal delete body: %w", err)
	}

	// Execute delete by query. Refresh so follow-up reads/writes do not see
	// the stale pre-delete state (mirrors Python's refresh semantics).
	result, err := e.doDeleteByQuery(ctx, []string{indexName}, bytes.NewReader(bodyBytes))
	if err != nil {
		common.Error("Failed to execute delete by query", err)
		return 0, fmt.Errorf("failed to execute delete by query: %w", err)
	}

	deleted := int64(0)
	if d, ok := result["deleted"].(float64); ok {
		deleted = int64(d)
	}

	// delete_by_query can answer 200 while only part of the work was done.
	// Surface that so callers do not treat leftover metadata as deleted.
	if timedOut, _ := result["timed_out"].(bool); timedOut {
		return deleted, fmt.Errorf("opensearch delete metadata by query timed out after deleting %d metadata documents from %s", deleted, indexName)
	}
	if failures, ok := result["failures"].([]interface{}); ok && len(failures) > 0 {
		return deleted, fmt.Errorf("opensearch delete metadata by query reported %d failures after deleting %d metadata documents from %s", len(failures), deleted, indexName)
	}

	common.Info("OpenSearchConnection.DeleteMetadata completes", zap.String("index_name", indexName), zap.Int64("deleted_count", deleted))
	return deleted, nil
}

// DeleteMetadataKeys deletes specific metadata keys from a document's meta_fields.
// If deleting those keys leaves no metadata entries, the metadata document is removed.
func (e *Engine) DeleteMetadataKeys(ctx context.Context, docID string, datasetID string, keys []string, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("OpenSearchConnection.DeleteMetadataKeys called", zap.String("index_name", indexName), zap.String("docID", docID), zap.Any("keys", keys))

	// Check if index exists
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: '%s'", types.ErrIndexNotFound, indexName)
	}

	// Build the document ID for query (no escaping needed for term queries)
	docIDTerm := docID
	datasetIDTerm := datasetID

	// Build query to find the document
	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []map[string]interface{}{
				{"term": map[string]interface{}{"id": docIDTerm}},
				{"term": map[string]interface{}{"kb_id": datasetIDTerm}},
			},
		},
	}

	// First, get the current meta_fields to check if it will be empty after deletion
	getReq := map[string]interface{}{
		"query":   query,
		"_source": []string{"meta_fields"},
		"size":    1,
	}

	getBytes, err := json.Marshal(getReq)
	if err != nil {
		return fmt.Errorf("failed to marshal get request: %w", err)
	}

	getResult, err := e.doPlainSearch(ctx, []string{indexName}, bytes.NewReader(getBytes), nil)
	if err != nil {
		return fmt.Errorf("failed to get current metadata: %w", err)
	}

	hits, ok := getResult["hits"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid get response format")
	}
	hitsArray, ok := hits["hits"].([]interface{})
	if !ok || len(hitsArray) == 0 {
		return fmt.Errorf("document not found: %s", docID)
	}

	// Check current meta_fields
	firstHit, ok := hitsArray[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid hit format")
	}
	source, ok := firstHit["_source"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid source format")
	}
	metaFieldsVal, hasMetaFields := source["meta_fields"]

	var currentMetaFields map[string]interface{}
	if hasMetaFields && metaFieldsVal != nil {
		switch v := metaFieldsVal.(type) {
		case map[string]interface{}:
			currentMetaFields = v
		case string:
			if unmarshalErr := json.Unmarshal([]byte(v), &currentMetaFields); unmarshalErr != nil {
				common.Warn("Failed to parse meta_fields JSON", zap.String("docID", docID), zap.Error(unmarshalErr))
				currentMetaFields = make(map[string]interface{})
			}
		}
	}

	// If no current meta_fields or already empty, nothing to delete
	if currentMetaFields == nil || len(currentMetaFields) == 0 {
		common.Info("No metadata fields to delete from document", zap.String("docID", docID))
		return nil
	}

	// Calculate which keys will be removed
	keysToRemove := make(map[string]bool)
	for _, k := range keys {
		keysToRemove[k] = true
	}

	// Check if any keys actually exist and would be removed
	hasKeysToRemove := false
	for k := range currentMetaFields {
		if keysToRemove[k] {
			hasKeysToRemove = true
			break
		}
	}

	if !hasKeysToRemove {
		common.Info("No matching keys to delete from document", zap.String("docID", docID))
		return nil
	}

	// Count remaining keys after deletion (keys that are NOT being removed)
	remainingKeys := 0
	for k := range currentMetaFields {
		if !keysToRemove[k] {
			remainingKeys++
		}
	}

	// If no other keys would remain after deletion, delete the document directly
	if remainingKeys == 0 {
		common.Info("All metadata keys would be deleted, removing document from index", zap.String("docID", docID))

		// Build condition for deletion using docID and datasetID
		condition := map[string]interface{}{
			"id":    docIDTerm,
			"kb_id": datasetIDTerm,
		}

		// Use existing DeleteMetadata method which handles the deletion properly
		_, err := e.DeleteMetadata(ctx, condition, tenantID)
		if err != nil {
			return fmt.Errorf("failed to delete document: %w", err)
		}

		common.Info("Successfully removed document with empty meta_fields", zap.String("docID", docID))
		return nil
	}

	// Some keys will remain, so remove only the specified keys
	keysParam := make([]string, len(keys))
	for i, k := range keys {
		keysParam[i] = k
	}

	// Build update script that removes keys from meta_fields map
	scriptSource := "for(int i=0;i<params.keys.length;i++){if(ctx._source.meta_fields.containsKey(params.keys[i])){ctx._source.meta_fields.remove(params.keys[i])}}"

	updateReq := map[string]interface{}{
		"query": query,
		"script": map[string]interface{}{
			"source": scriptSource,
			"params": map[string]interface{}{
				"keys": keysParam,
			},
		},
	}

	updateBytes, err := json.Marshal(updateReq)
	if err != nil {
		return fmt.Errorf("failed to marshal update request: %w", err)
	}

	updateResponse, err := e.doUpdateByQuery(ctx, []string{indexName}, bytes.NewReader(updateBytes))
	if err != nil {
		common.Error("Failed to execute update by query", err)
		return fmt.Errorf("failed to execute update by query: %w", err)
	}
	// Returning success while the keys remain is worse than failing: the
	// caller would believe the metadata was removed.
	if err := checkUpdateByQueryResult(updateResponse, indexName, "delete metadata keys"); err != nil {
		return err
	}

	common.Info("OpenSearchConnection.DeleteMetadataKeys completes", zap.String("index_name", indexName), zap.String("docID", docID))

	return nil
}

// checkUpdateByQueryResult rejects a 200 update_by_query response that still
// signals a timeout or per-document failures. Without this the caller sees
// success while documents keep their previous values.
func checkUpdateByQueryResult(result map[string]interface{}, indexName, operation string) error {
	if timedOut, _ := result["timed_out"].(bool); timedOut {
		return fmt.Errorf("opensearch %s timed out on %s", operation, indexName)
	}
	if failures, ok := result["failures"].([]interface{}); ok && len(failures) > 0 {
		return fmt.Errorf("opensearch %s reported %d failures on %s", operation, len(failures), indexName)
	}
	return nil
}

// DropMetadataStore drops a metadata index
func (e *Engine) DropMetadataStore(ctx context.Context, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	return e.dropIndex(ctx, indexName)
}

// MetadataStoreExists checks if a metadata index exists
func (e *Engine) MetadataStoreExists(ctx context.Context, tenantID string) (bool, error) {
	indexName := buildMetadataIndexName(tenantID)
	return e.indexExists(ctx, indexName)
}

// SearchMetadata executes search specifically for metadata indices (ragflow_doc_meta_*)
func (e *Engine) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	tenantID := req.TenantID
	common.Debug("SearchMetadata in OpenSearch started", zap.String("tenantID", tenantID))

	// Validate inputs
	if tenantID == "" {
		return nil, fmt.Errorf("tenantID cannot be empty")
	}

	indexName := buildMetadataIndexName(tenantID)

	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		common.Warn("OpenSearch SearchMetadata index existence check failed", zap.String("index", indexName), zap.Error(err))
		return nil, fmt.Errorf("failed to check metadata index existence: %w", err)
	}
	if !exists {
		common.Debug("OpenSearch SearchMetadata index absent, returning empty result", zap.String("index", indexName))
		return &types.SearchMetadataResult{
			MetadataRecords: []map[string]interface{}{},
			Total:           0,
		}, nil
	}

	offset := max(req.Offset, 0)
	limit := req.Limit
	if limit <= 0 {
		limit = 30
	}

	// Build query for metadata using buildMetadataQueryFromCondition
	queryBody := make(map[string]interface{})
	queryBody["query"] = e.buildMetadataQueryFromCondition(req.Filter)
	if queryBody["query"] == nil {
		queryBody["query"] = map[string]interface{}{"match_all": map[string]interface{}{}}
	}

	// Add sorting if order_by specified
	if req.OrderBy != nil && len(req.OrderBy.Fields) > 0 {
		sort := esoscommon.ParseOrderByExpr(req.OrderBy)
		if len(sort) > 0 {
			queryBody["sort"] = sort
		}
	}

	// Add _source for field filtering if specified
	if len(req.SelectFields) > 0 {
		queryBody["_source"] = req.SelectFields
	}

	// Apply offset/limit
	queryBody["size"] = limit
	queryBody["from"] = offset

	// Serialize query
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(queryBody); err != nil {
		return nil, fmt.Errorf("error encoding query: %w", err)
	}

	common.Info("OpenSearch SearchMetadata", zap.String("indexName", indexName), zap.Any("dsl", queryBody))

	// Execute search (no hybrid search pipeline: this is a plain bool query)
	searchResult, err := e.doPlainSearch(ctx, []string{indexName}, bytes.NewReader(buf.Bytes()), map[string]string{"track_total_hits": "true"})
	if err != nil {
		common.Warn("OpenSearch SearchMetadata query failed", zap.String("index", indexName), zap.Error(err))
		return nil, fmt.Errorf("search failed: %w", err)
	}

	esResp, err := mapToSearchResponse(searchResult)
	if err != nil {
		common.Warn("OpenSearch SearchMetadata failed to parse response", zap.String("index", indexName), zap.Error(err))
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	searchChunks := convertESResponse(&esResp, "")
	totalHits := esResp.Hits.Total.Value

	common.Debug("SearchMetadata in OpenSearch completed", zap.Int("returnedRows", len(searchChunks)), zap.Int64("totalHits", totalHits))

	return &types.SearchMetadataResult{
		MetadataRecords: searchChunks,
		Total:           totalHits,
	}, nil
}

// buildMetadataQueryFromCondition builds a query for metadata index
func (e *Engine) buildMetadataQueryFromCondition(condition map[string]interface{}) map[string]interface{} {
	if len(condition) == 0 {
		return nil
	}

	var clauses []map[string]interface{}

	for k, v := range condition {
		if v == nil {
			continue
		}

		switch k {
		case "kb_id":
			if listVal, ok := v.([]interface{}); ok {
				clauses = append(clauses, map[string]interface{}{
					"terms": map[string]interface{}{k: listVal},
				})
			} else if strListVal, ok := v.([]string); ok {
				clauses = append(clauses, map[string]interface{}{
					"terms": map[string]interface{}{k: strListVal},
				})
			} else {
				clauses = append(clauses, map[string]interface{}{
					"term": map[string]interface{}{k: v},
				})
			}
		case "id":
			if listVal, ok := v.([]interface{}); ok {
				clauses = append(clauses, map[string]interface{}{
					"terms": map[string]interface{}{k: listVal},
				})
			} else if strListVal, ok := v.([]string); ok {
				clauses = append(clauses, map[string]interface{}{
					"terms": map[string]interface{}{k: strListVal},
				})
			} else {
				clauses = append(clauses, map[string]interface{}{
					"term": map[string]interface{}{k: v},
				})
			}
		default:
			clauses = append(clauses, map[string]interface{}{
				"term": map[string]interface{}{k: v},
			})
		}
	}

	if len(clauses) == 0 {
		return nil
	}

	if len(clauses) == 1 {
		return clauses[0]
	}

	return map[string]interface{}{
		"bool": map[string]interface{}{
			"filter": clauses,
		},
	}
}

// metaPushdownMaxSize caps how many doc IDs the metadata push-down is
// willing to return in one shot. Matches the Python reference
// (DocMetadataService.filter_doc_ids_by_meta_pushdown, default limit=10000)
// and the engine's default index.max_result_window.
//
// When the underlying query matches more than this, the push-down
// returns nil and the caller falls back to the in-memory meta_filter,
// which is correct (just slower for very large result sets). Returning
// a truncated slice as a definitive answer would silently drop docs.
const metaPushdownMaxSize = 10000

// FilterDocIdsByMetaPushdown runs a metadata filter directly against the doc metadata index.
//
// Return value convention (matching Python's filter_doc_ids_by_meta_pushdown):
//
//	nil        -> push-down was not viable / errored / result overflowed the
//	              push-down cap (caller should fall back to in-memory)
//	[]string{} -> push-down succeeded but found 0 matching docs (empty result is definitive)
func (e *Engine) FilterDocIdsByMetaPushdown(ctx context.Context, sqlDB *gorm.DB, kbIDs []string, conditions []map[string]interface{}, logic string) []string {
	if len(conditions) == 0 || len(kbIDs) == 0 {
		return nil
	}

	// Check if push-down is supported
	if !metafilter.IsPushdownSupported(conditions) {
		common.Debug("FilterDocIdsByMetaPushdown: push-down not supported for some filters")
		return nil
	}

	groups, err := dao.GroupKBIDsByTenant(ctx, sqlDB, kbIDs)
	if err != nil {
		common.Warn("FilterDocIdsByMetaPushdown: failed to resolve KB tenants", zap.Strings("kbIDs", kbIDs), zap.Error(err))
		return nil
	}
	indexNames := make([]string, 0, len(groups))
	for _, group := range groups {
		indexName := buildMetadataIndexName(group.TenantID)
		exists, err := e.indexExists(ctx, indexName)
		if err != nil || !exists {
			return nil
		}
		indexNames = append(indexNames, indexName)
	}

	// Build the filter query using the full meta_filter logic
	queryBody, err := metafilter.BuildMetaFilterQuery(conditions, logic, kbIDs)
	if err != nil {
		common.Debug("FilterDocIdsByMetaPushdown: build query failed", zap.String("error", err.Error()))
		return nil
	}

	// Use the push-down cap. track_total_hits=true makes hits.total.value
	// exact (the engine otherwise caps the tracked total at 10,000 with
	// relation=gte, which would let overflow slip through undetected).
	requestBody := make(map[string]interface{})
	for k, v := range queryBody {
		requestBody[k] = v
	}
	requestBody["size"] = metaPushdownMaxSize
	requestBody["track_total_hits"] = true
	requestBody["_source"] = []string{"id"}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(requestBody); err != nil {
		return nil
	}

	common.Debug("FilterDocIdsByMetaPushdown executing search query", zap.Any("query", requestBody))

	// A partial answer would look like a definitive (possibly too small) ID
	// set, so require every shard to respond (mirrors the Elasticsearch
	// engine's AllowPartialSearchResults(false)).
	res, err := e.doPlainSearch(ctx, indexNames, bytes.NewReader(buf.Bytes()), map[string]string{
		"allow_partial_search_results": "false",
	})
	if err != nil {
		return nil
	}

	// Extract doc IDs before the cap check so we can use uniqueDocIDs to
	// detect silent truncation when the total isn't available.
	docIDs := metafilter.ExtractDocIDs(res)
	uniqueDocIDs := dedupeStrings(docIDs)

	// Detect silent truncation: the push-down is a fast path, not the
	// system of record. If the query matched more than metaPushdownMaxSize
	// docs, the slice we can build here is necessarily a strict subset of
	// the truth, and the caller treats non-nil as definitive. Surface a
	// warning (parity with the Python reference) AND bail out so the
	// caller falls back to the in-memory meta_filter, which is correct.
	total, totalOK := totalHitsFromESResponse(res)
	switch {
	case !totalOK && len(uniqueDocIDs) >= metaPushdownMaxSize:
		common.Warn("FilterDocIdsByMetaPushdown: total is unavailable at cap, falling back to in-memory",
			zap.Int("uniqueDocCount", len(uniqueDocIDs)),
			zap.Int("cap", metaPushdownMaxSize),
			zap.Strings("kbIDs", kbIDs),
		)
		return nil
	case totalOK && total > int64(metaPushdownMaxSize):
		common.Warn("FilterDocIdsByMetaPushdown: result exceeds push-down cap, falling back to in-memory",
			zap.Int64("total", total),
			zap.Int("cap", metaPushdownMaxSize),
			zap.Strings("kbIDs", kbIDs),
		)
		return nil
	}

	return docIDs
}

// dedupeStrings returns the unique values of s, preserving first-seen order.
func dedupeStrings(s []string) []string {
	if len(s) == 0 {
		return s
	}
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// totalHitsFromESResponse extracts the exact total hit count from a search
// response. Returns ok=false when the field is missing or in an unexpected
// shape; callers should treat that as "cannot verify" rather than "no overflow".
func totalHitsFromESResponse(esResponse map[string]interface{}) (int64, bool) {
	hitsRoot, ok := esResponse["hits"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	totalRoot, ok := hitsRoot["total"].(map[string]interface{})
	if !ok {
		return 0, false
	}
	// JSON numeric values decode as float64 in Go's default decoder.
	if v, ok := totalRoot["value"].(float64); ok {
		return int64(v), true
	}
	// Some clients / older versions return int64 directly.
	if v, ok := totalRoot["value"].(int64); ok {
		return v, true
	}
	if v, ok := totalRoot["value"].(json.Number); ok {
		n, err := v.Int64()
		if err == nil {
			return n, true
		}
	}
	return 0, false
}

// doPlainSearch runs a plain search request without the hybrid search
// pipeline. Metadata queries are ordinary bool queries and must not be
// routed through the BM25+KNN normalization pipeline.
func (e *Engine) doPlainSearch(ctx context.Context, indices []string, body io.Reader, params map[string]string) (map[string]interface{}, error) {
	path := "/" + strings.Join(indices, ",") + "/_search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	q := req.URL.Query()
	for k, v := range params {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read search response: %w", err)
	}
	if resp.StatusCode == http.StatusNotFound && isIndexNotFoundBody(respBody) {
		return nil, &indexNotFoundError{index: strings.Join(indices, ",")}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("search error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse search response: %w", err)
	}
	return result, nil
}
