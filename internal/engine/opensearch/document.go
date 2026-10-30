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

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/types"
	"ragflow/internal/utility"

	"github.com/opensearch-project/opensearch-go/v4/opensearchapi"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// metaPushdownMaxSize caps how many doc IDs the metadata push-down is
// willing to return in one shot, matching Python reference and ES/OS default max_result_window.
const metaPushdownMaxSize = 10000

// BulkResponse bulk operation response.
type BulkResponse struct {
	Took    int64 `json:"took"`
	Errors  bool  `json:"errors"`
	Indexed int   `json:"indexed"`
}

// CreateMetadataStore creates the document metadata index for a tenant in OpenSearch.
func (e *Engine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)

	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check metadata index existence: %w", err)
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

	var mapping map[string]interface{}
	if err = json.Unmarshal(data, &mapping); err != nil {
		return fmt.Errorf("failed to parse mapping file %q: %w", *mappingPath, err)
	}

	// In OpenSearch, top-level "dynamic": "runtime" is invalid; rewrite to boolean true.
	if m, ok := mapping["mappings"].(map[string]interface{}); ok {
		if dyn, ok := m["dynamic"].(string); ok && dyn == "runtime" {
			m["dynamic"] = true
		}
	}

	bodyBytes, err := json.Marshal(mapping)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata mapping: %w", err)
	}

	resp, err := e.apiClient.Indices.Create(ctx, opensearchapi.IndicesCreateReq{
		Index: indexName,
		Body:  bytes.NewReader(bodyBytes),
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Inspect().Response.Body)
			reason := extractErrorReason(body)
			if strings.Contains(reason, "resource_already_exists_exception") || strings.Contains(string(body), "resource_already_exists_exception") {
				return nil
			}
			if reason != "" {
				return fmt.Errorf("opensearch error: %s", reason)
			}
			return fmt.Errorf("opensearch returned error: %s, body: %s", resp.Inspect().Response.Status(), string(body))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to create metadata index: %w", err)
	}

	common.Info("Successfully created OpenSearch metadata index", zap.String("index_name", indexName))
	return nil
}

// InsertMetadata inserts documents into tenant's metadata index.
func (e *Engine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("OpenSearchConnection.InsertMetadata called", zap.String("index_name", indexName), zap.String("tenant_id", tenantID), zap.Int("doc_count", len(metadata)))

	if len(metadata) == 0 {
		return []string{}, nil
	}
	if indexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}

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

		action, err := json.Marshal(map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
				"_id":    docID,
			},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to marshal bulk action: %w", err)
		}
		buf.Write(action)
		buf.WriteByte('\n')

		if err := json.NewEncoder(&buf).Encode(doc); err != nil {
			return nil, fmt.Errorf("failed to encode metadata document: %w", err)
		}
	}

	if buf.Len() == 0 {
		return []string{}, nil
	}

	resp, err := e.apiClient.Bulk(ctx, opensearchapi.BulkReq{
		Body: bytes.NewReader(buf.Bytes()),
		Params: opensearchapi.BulkParams{
			Refresh: "wait_for",
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Inspect().Response.Body)
			return nil, fmt.Errorf("opensearch bulk request returned error: %s, body: %s", resp.Inspect().Response.Status(), string(body))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to execute bulk request: %w", err)
	}

	if resp.Errors {
		var reasons []string
		for _, itemMap := range resp.Items {
			for _, item := range itemMap {
				if item.Error != nil && item.Error.Reason != "" {
					reasons = append(reasons, item.Error.Reason)
				}
			}
		}
		totalErrors := len(reasons)
		displayReasons := reasons
		if len(displayReasons) > 5 {
			displayReasons = displayReasons[:5]
		}
		common.Error("OpenSearch metadata bulk request had item errors",
			fmt.Errorf("index %s: %d failed items; first reasons: %v", indexName, totalErrors, displayReasons))
		return nil, fmt.Errorf("opensearch metadata bulk request had %d item errors (index %s); first reasons: %v",
			totalErrors, indexName, displayReasons)
	}

	common.Info("OpenSearchConnection.InsertMetadata completed", zap.String("index_name", indexName), zap.Int("count", len(metadata)))
	return []string{}, nil
}

// UpdateMetadata fully replaces meta_fields for a document in tenant's metadata index.
// If the document does not exist, it inserts it.
func (e *Engine) UpdateMetadata(ctx context.Context, docID string, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("OpenSearchConnection.UpdateMetadata called", zap.String("index_name", indexName), zap.String("docID", docID), zap.String("datasetID", datasetID))

	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []map[string]interface{}{
				{"term": map[string]interface{}{"id": docID}},
				{"term": map[string]interface{}{"kb_id": datasetID}},
			},
		},
	}

	updateReq := map[string]interface{}{
		"query": query,
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

	refreshTrue := true
	resp, err := e.apiClient.UpdateByQuery(ctx, opensearchapi.UpdateByQueryReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(updateBytes),
		Params: opensearchapi.UpdateByQueryParams{
			Refresh:   &refreshTrue,
			Conflicts: "proceed",
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Inspect().Response.Body)
			return fmt.Errorf("opensearch update by query returned error: %s, body: %s", resp.Inspect().Response.Status(), string(body))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to execute update by query: %w", err)
	}

	if resp.TimedOut {
		return fmt.Errorf("opensearch update metadata by query timed out on index %s", indexName)
	}
	if len(resp.Failures) > 0 {
		return fmt.Errorf("opensearch update metadata by query encountered %d failures on index %s", len(resp.Failures), indexName)
	}
	if resp.VersionConflicts > 0 {
		return fmt.Errorf("opensearch update metadata by query encountered %d version conflicts on index %s for doc %s", resp.VersionConflicts, indexName, docID)
	}

	// If no existing document matched the ID filter, insert it as new metadata doc.
	// Version conflicts are caught above and must never enter the insert branch.
	if resp.Total == 0 {
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

// DeleteMetadata deletes metadata from tenant's metadata index by condition.
func (e *Engine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("OpenSearchConnection.DeleteMetadata called", zap.String("index_name", indexName), zap.Any("condition", condition))

	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return 0, fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		common.Warn("Index does not exist, skipping delete", zap.String("index_name", indexName))
		return 0, nil
	}

	query := e.buildMetadataQueryFromCondition(condition)
	if query == nil {
		if len(condition) > 0 {
			return 0, fmt.Errorf("OpenSearch delete metadata aborted: non-empty condition yielded match_all query on index %s", indexName)
		}
		query = map[string]interface{}{"match_all": map[string]interface{}{}}
	}

	deleteBody := map[string]interface{}{
		"query": query,
	}

	bodyBytes, err := json.Marshal(deleteBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal delete body: %w", err)
	}

	refreshTrue := true
	resp, err := e.apiClient.Document.DeleteByQuery(ctx, opensearchapi.DocumentDeleteByQueryReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(bodyBytes),
		Params: opensearchapi.DocumentDeleteByQueryParams{
			Refresh: &refreshTrue,
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode == http.StatusNotFound {
			return 0, nil
		}
		if resp.Inspect().Response.StatusCode >= 400 {
			return 0, fmt.Errorf("opensearch delete by query returned error: %s", resp.Inspect().Response.Status())
		}
	}
	if err != nil {
		if isNotFoundError(err) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to execute delete by query: %w", err)
	}

	if resp.TimedOut {
		return int64(resp.Deleted), fmt.Errorf("opensearch delete metadata by query timed out on index %s (deleted %d of %d)", indexName, resp.Deleted, resp.Total)
	}
	if len(resp.Failures) > 0 {
		return int64(resp.Deleted), fmt.Errorf("opensearch delete metadata by query encountered %d failures on index %s", len(resp.Failures), indexName)
	}
	if resp.VersionConflicts > 0 {
		return int64(resp.Deleted), fmt.Errorf("opensearch delete metadata by query encountered %d version conflicts on index %s", resp.VersionConflicts, indexName)
	}

	deleted := int64(resp.Deleted)
	common.Info("OpenSearchConnection.DeleteMetadata completes", zap.String("index_name", indexName), zap.Int64("deleted_count", deleted))
	return deleted, nil
}

// DeleteMetadataKeys deletes specific metadata keys from a document's meta_fields.
// If deleting those keys leaves no metadata entries, the metadata document is removed completely.
func (e *Engine) DeleteMetadataKeys(ctx context.Context, docID string, datasetID string, keys []string, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("OpenSearchConnection.DeleteMetadataKeys called", zap.String("index_name", indexName), zap.String("docID", docID), zap.Any("keys", keys))

	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("%w: '%s'", types.ErrIndexNotFound, indexName)
	}

	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []map[string]interface{}{
				{"term": map[string]interface{}{"id": docID}},
				{"term": map[string]interface{}{"kb_id": datasetID}},
			},
		},
	}

	getReq := map[string]interface{}{
		"query":   query,
		"_source": []string{"meta_fields"},
		"size":    1,
	}

	getBytes, err := json.Marshal(getReq)
	if err != nil {
		return fmt.Errorf("failed to marshal get request: %w", err)
	}

	allowPartial := false
	ignoreUnavailable := true
	searchResp, err := e.apiClient.Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(getBytes),
		Params: opensearchapi.SearchParams{
			AllowPartialSearchResults: &allowPartial,
			IgnoreUnavailable:         &ignoreUnavailable,
		},
	})
	if searchResp != nil && searchResp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, searchResp.Inspect().Response.Body)
			_ = searchResp.Inspect().Response.Body.Close()
		}()
		if searchResp.Inspect().Response.StatusCode >= 400 {
			return fmt.Errorf("opensearch search request returned error: %s", searchResp.Inspect().Response.Status())
		}
	}
	if err != nil {
		return fmt.Errorf("failed to get current metadata: %w", err)
	}
	if searchResp.Timeout {
		return fmt.Errorf("opensearch get current metadata timed out on index %s", indexName)
	}
	if searchResp.Shards.Failed > 0 {
		return fmt.Errorf("opensearch get current metadata encountered %d failed shards on index %s", searchResp.Shards.Failed, indexName)
	}

	if len(searchResp.Hits.Hits) == 0 {
		return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, docID)
	}

	var source map[string]interface{}
	if err := json.Unmarshal(searchResp.Hits.Hits[0].Source, &source); err != nil {
		return fmt.Errorf("failed to unmarshal metadata source: %w", err)
	}

	var currentMetaFields map[string]interface{}
	if metaFieldsVal, hasMetaFields := source["meta_fields"]; hasMetaFields && metaFieldsVal != nil {
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

	if len(currentMetaFields) == 0 {
		common.Info("No metadata fields to delete from document", zap.String("docID", docID))
		return nil
	}

	keysToRemove := make(map[string]bool, len(keys))
	for _, k := range keys {
		keysToRemove[k] = true
	}

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

	remainingKeys := 0
	for k := range currentMetaFields {
		if !keysToRemove[k] {
			remainingKeys++
		}
	}

	// If no other keys remain, delete the entire document
	if remainingKeys == 0 {
		common.Info("All metadata keys removed, deleting document from index", zap.String("docID", docID))
		_, err := e.DeleteMetadata(ctx, map[string]interface{}{
			"id":    docID,
			"kb_id": datasetID,
		}, tenantID)
		if err != nil {
			return fmt.Errorf("failed to delete document: %w", err)
		}
		return nil
	}

	// Otherwise remove specified keys via Painless script
	scriptSource := "for(int i=0;i<params.keys.length;i++){if(ctx._source.meta_fields != null && ctx._source.meta_fields.containsKey(params.keys[i])){ctx._source.meta_fields.remove(params.keys[i]);}}"
	updateReq := map[string]interface{}{
		"query": query,
		"script": map[string]interface{}{
			"source": scriptSource,
			"params": map[string]interface{}{
				"keys": keys,
			},
		},
	}

	updateBytes, err := json.Marshal(updateReq)
	if err != nil {
		return fmt.Errorf("failed to marshal update request: %w", err)
	}

	refreshTrue := true
	uResp, err := e.apiClient.UpdateByQuery(ctx, opensearchapi.UpdateByQueryReq{
		Indices: []string{indexName},
		Body:    bytes.NewReader(updateBytes),
		Params: opensearchapi.UpdateByQueryParams{
			Refresh: &refreshTrue,
		},
	})
	if uResp != nil && uResp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, uResp.Inspect().Response.Body)
			_ = uResp.Inspect().Response.Body.Close()
		}()
		if uResp.Inspect().Response.StatusCode >= 400 {
			return fmt.Errorf("opensearch update by query returned error: %s", uResp.Inspect().Response.Status())
		}
	}
	if err != nil {
		return fmt.Errorf("failed to execute update by query: %w", err)
	}

	if uResp.TimedOut {
		return fmt.Errorf("opensearch update metadata keys by query timed out on index %s", indexName)
	}
	if len(uResp.Failures) > 0 {
		return fmt.Errorf("opensearch update metadata keys by query encountered %d failures on index %s", len(uResp.Failures), indexName)
	}
	if uResp.VersionConflicts > 0 {
		return fmt.Errorf("opensearch update metadata keys by query encountered %d version conflicts on index %s for doc %s", uResp.VersionConflicts, indexName, docID)
	}

	common.Info("OpenSearchConnection.DeleteMetadataKeys completes", zap.String("index_name", indexName), zap.String("docID", docID))
	return nil
}

// DropMetadataStore drops a metadata index from OpenSearch.
func (e *Engine) DropMetadataStore(ctx context.Context, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	return e.dropIndex(ctx, indexName)
}

// MetadataStoreExists checks if a metadata index exists in OpenSearch.
func (e *Engine) MetadataStoreExists(ctx context.Context, tenantID string) (bool, error) {
	indexName := buildMetadataIndexName(tenantID)
	return e.indexExists(ctx, indexName)
}

// SearchMetadata executes search specifically for metadata indices (ragflow_doc_meta_*).
func (e *Engine) SearchMetadata(ctx context.Context, req *types.SearchMetadataRequest) (*types.SearchMetadataResult, error) {
	tenantID := req.TenantID
	common.Debug("SearchMetadata in OpenSearch started", zap.String("tenantID", tenantID))

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

	queryBody := make(map[string]interface{})
	queryBody["query"] = e.buildMetadataQueryFromCondition(req.Filter)
	if queryBody["query"] == nil {
		queryBody["query"] = map[string]interface{}{"match_all": map[string]interface{}{}}
	}

	if req.OrderBy != nil && len(req.OrderBy.Fields) > 0 {
		sort := parseOrderByExpr(req.OrderBy)
		if len(sort) > 0 {
			queryBody["sort"] = sort
		}
	}

	if len(req.SelectFields) > 0 {
		queryBody["_source"] = req.SelectFields
	}

	queryBody["size"] = limit
	queryBody["from"] = offset
	queryBody["track_total_hits"] = true

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(queryBody); err != nil {
		return nil, fmt.Errorf("error encoding query: %w", err)
	}

	common.Info("OpenSearch SearchMetadata", zap.String("indexName", indexName), zap.Any("dsl", queryBody))

	allowPartial := false
	ignoreUnavailable := true
	resp, err := e.apiClient.Search(ctx, &opensearchapi.SearchReq{
		Indices: []string{indexName},
		Body:    &buf,
		Params: opensearchapi.SearchParams{
			AllowPartialSearchResults: &allowPartial,
			IgnoreUnavailable:         &ignoreUnavailable,
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Inspect().Response.Body)
			return nil, fmt.Errorf("opensearch search returned error: %s, body: %s", resp.Inspect().Response.Status(), string(body))
		}
	}
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	if resp.Timeout {
		return nil, fmt.Errorf("opensearch metadata search timed out on index %s", indexName)
	}
	if resp.Shards.Failed > 0 {
		return nil, fmt.Errorf("opensearch metadata search encountered %d failed shards on index %s", resp.Shards.Failed, indexName)
	}

	records := make([]map[string]interface{}, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		var src map[string]interface{}
		if len(hit.Source) > 0 {
			_ = json.Unmarshal(hit.Source, &src)
		}
		if src == nil {
			src = make(map[string]interface{})
		}
		src["_id"] = hit.ID
		src["id"] = hit.ID
		src["_score"] = hit.Score
		src["_index"] = hit.Index
		records = append(records, src)
	}

	totalHits := int64(resp.Hits.Total.Value)
	common.Debug("SearchMetadata in OpenSearch completed", zap.Int("returnedRows", len(records)), zap.Int64("totalHits", totalHits))

	return &types.SearchMetadataResult{
		MetadataRecords: records,
		Total:           totalHits,
	}, nil
}

// FilterDocIdsByMetaPushdown runs a metadata filter directly against OpenSearch doc metadata index.
//
// Return value convention (parity with ES and Python reference):
//
//	nil        -> push-down was not viable / errored / result overflowed cap (fallback to in-memory)
//	[]string{} -> push-down succeeded but found 0 matching docs (definitive answer)
//	[...]      -> push-down succeeded with matching doc IDs
func (e *Engine) FilterDocIdsByMetaPushdown(ctx context.Context, sqlDB *gorm.DB, kbIDs []string, conditions []map[string]interface{}, logic string) []string {
	if sqlDB == nil || len(conditions) == 0 || len(kbIDs) == 0 {
		return nil
	}

	if !elasticsearch.IsPushdownSupported(conditions) {
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

	if len(indexNames) == 0 {
		return []string{}
	}

	queryBody, err := elasticsearch.BuildMetaFilterQuery(conditions, logic, kbIDs)
	if err != nil {
		common.Debug("FilterDocIdsByMetaPushdown: build query failed", zap.String("error", err.Error()))
		return nil
	}

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

	common.Debug("FilterDocIdsByMetaPushdown executing OpenSearch query", zap.Any("query", requestBody))

	allowPartial := false
	resp, err := e.apiClient.Search(ctx, &opensearchapi.SearchReq{
		Indices: indexNames,
		Body:    &buf,
		Params: opensearchapi.SearchParams{
			AllowPartialSearchResults: &allowPartial,
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			return nil
		}
	}
	if err != nil || resp.Timeout || resp.Shards.Failed > 0 {
		return nil
	}

	total := int64(resp.Hits.Total.Value)
	if total > int64(metaPushdownMaxSize) {
		common.Warn("FilterDocIdsByMetaPushdown: result exceeds push-down cap, falling back to in-memory",
			zap.Int64("total", total),
			zap.Int("cap", metaPushdownMaxSize),
			zap.Strings("kbIDs", kbIDs),
		)
		return nil
	}

	docIDs := make([]string, 0, len(resp.Hits.Hits))
	for _, hit := range resp.Hits.Hits {
		var src map[string]interface{}
		if len(hit.Source) > 0 {
			_ = json.Unmarshal(hit.Source, &src)
		}
		if id, ok := src["id"].(string); ok && id != "" {
			docIDs = append(docIDs, id)
		} else if hit.ID != "" {
			docIDs = append(docIDs, hit.ID)
		}
	}

	uniqueDocIDs := dedupeStrings(docIDs)
	if total == 0 && len(uniqueDocIDs) >= metaPushdownMaxSize {
		common.Warn("FilterDocIdsByMetaPushdown: OpenSearch total is unavailable at cap, falling back to in-memory",
			zap.Int("uniqueDocCount", len(uniqueDocIDs)),
			zap.Int("cap", metaPushdownMaxSize),
			zap.Strings("kbIDs", kbIDs),
		)
		return nil
	}

	return uniqueDocIDs
}

// IndexDocument indexes a single document into OpenSearch (used by skill indexing).
func (e *Engine) IndexDocument(ctx context.Context, indexName, docID string, doc interface{}) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}
	if docID == "" {
		return fmt.Errorf("document id cannot be empty")
	}
	if doc == nil {
		return fmt.Errorf("document cannot be nil")
	}

	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	resp, err := e.apiClient.Index(ctx, opensearchapi.IndexReq{
		Index:      indexName,
		DocumentID: docID,
		Body:       bytes.NewReader(data),
		Params: opensearchapi.IndexParams{
			Refresh: "true",
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Inspect().Response.Body)
			reason := extractErrorReason(body)
			if reason != "" {
				return fmt.Errorf("opensearch error: %s", reason)
			}
			return fmt.Errorf("opensearch returned error: %s, body: %s", resp.Inspect().Response.Status(), string(body))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}

	return nil
}

// DeleteDocument deletes a single document from OpenSearch.
func (e *Engine) DeleteDocument(ctx context.Context, indexName, docID string) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}
	if docID == "" {
		return fmt.Errorf("document id cannot be empty")
	}

	resp, err := e.apiClient.Document.Delete(ctx, opensearchapi.DocumentDeleteReq{
		Index:      indexName,
		DocumentID: docID,
		Params: opensearchapi.DocumentDeleteParams{
			Refresh: "true",
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode == http.StatusNotFound {
			return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, docID)
		}
		if resp.Inspect().Response.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Inspect().Response.Body)
			reason := extractErrorReason(body)
			if reason != "" {
				return fmt.Errorf("opensearch error: %s", reason)
			}
			return fmt.Errorf("opensearch returned error: %s, body: %s", resp.Inspect().Response.Status(), string(body))
		}
	}
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	return nil
}

// BulkIndex indexes documents in bulk in OpenSearch.
func (e *Engine) BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error) {
	if indexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("documents cannot be empty")
	}

	var buf bytes.Buffer
	for _, doc := range docs {
		docMap, ok := doc.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("document must be map[string]interface{}")
		}

		docID, hasID := docMap["_id"]
		if !hasID || docID == nil || docID == "" {
			return nil, fmt.Errorf("document missing _id field")
		}

		// Delete _id field from doc body
		docCopy := make(map[string]interface{}, len(docMap))
		for k, v := range docMap {
			if k != "_id" {
				docCopy[k] = v
			}
		}

		meta := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
				"_id":    fmt.Sprintf("%v", docID),
			},
		}
		metaData, _ := json.Marshal(meta)
		docData, _ := json.Marshal(docCopy)

		buf.Write(metaData)
		buf.WriteByte('\n')
		buf.Write(docData)
		buf.WriteByte('\n')
	}

	resp, err := e.apiClient.Bulk(ctx, opensearchapi.BulkReq{
		Body: &buf,
		Params: opensearchapi.BulkParams{
			Refresh: "true",
		},
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Inspect().Response.Body)
			reason := extractErrorReason(body)
			if reason != "" {
				return nil, fmt.Errorf("opensearch error: %s", reason)
			}
			return nil, fmt.Errorf("opensearch returned error: %s", resp.Inspect().Response.Status())
		}
	}
	if err != nil {
		return nil, fmt.Errorf("bulk index failed: %w", err)
	}

	if resp.Errors {
		for _, itemMap := range resp.Items {
			for _, item := range itemMap {
				if item.Error != nil && item.Error.Reason != "" {
					return nil, fmt.Errorf("bulk index error: %s", item.Error.Reason)
				}
			}
		}
		return nil, fmt.Errorf("bulk index has errors")
	}

	return &BulkResponse{
		Took:    int64(resp.Took),
		Errors:  resp.Errors,
		Indexed: len(docs),
	}, nil
}

// GetDocument gets a document from OpenSearch by ID.
func (e *Engine) GetDocument(ctx context.Context, indexName, docID string) (interface{}, error) {
	if indexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}
	if docID == "" {
		return nil, fmt.Errorf("document id cannot be empty")
	}

	resp, err := e.apiClient.Document.Get(ctx, opensearchapi.DocumentGetReq{
		Index:      indexName,
		DocumentID: docID,
	})
	if resp != nil && resp.Inspect().Response != nil {
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Inspect().Response.Body)
			_ = resp.Inspect().Response.Body.Close()
		}()
		if resp.Inspect().Response.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("%w: %s", types.ErrDocumentNotFound, docID)
		}
		if resp.Inspect().Response.StatusCode >= 400 {
			body, _ := io.ReadAll(resp.Inspect().Response.Body)
			reason := extractErrorReason(body)
			if reason != "" {
				return nil, fmt.Errorf("opensearch error: %s", reason)
			}
			return nil, fmt.Errorf("opensearch returned error: %s", resp.Inspect().Response.Status())
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}

	if !resp.Found {
		return nil, fmt.Errorf("%w: %s", types.ErrDocumentNotFound, docID)
	}

	var source map[string]interface{}
	if err := json.Unmarshal(resp.Source, &source); err != nil {
		return nil, fmt.Errorf("failed to parse document source: %w", err)
	}

	return source, nil
}

// Helper methods

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
		case "kb_id", "id":
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
