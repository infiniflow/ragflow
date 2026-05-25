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

package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"ragflow/internal/common"

	"go.uber.org/zap"
)

// CreateMetadataStore creates the document metadata index
func (e *elasticsearchEngine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)

	// Check if index already exists
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if exists {
		return nil
	}

	// Index will be created with mapping from index template (ragflow_doc_meta_mapping)
	req := esapi.IndicesCreateRequest{
		Index: indexName,
	}
	res, err := req.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("failed to create metadata index: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return fmt.Errorf("elasticsearch returned error: %s, body: %s", res.Status(), string(bodyBytes))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	acknowledged, ok := result["acknowledged"].(bool)
	if !ok || !acknowledged {
		return fmt.Errorf("metadata index creation not acknowledged")
	}

	return nil
}

// InsertMetadata inserts documents into tenant's metadata index
// If a document with the same id and kb_id already exists, it will be updated with the new value
func (e *elasticsearchEngine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("ElasticsearchConnection.InsertMetadata called", zap.String("index_name", indexName), zap.String("tenant_id", tenantID), zap.Int("doc_count", len(metadata)))

	if len(metadata) == 0 {
		return []string{}, nil
	}

	if indexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}

	// Check if index exists, create if not
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		common.Error("Failed to check index existence", err)
		return nil, fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		// Create metadata index
		if createErr := e.CreateMetadataStore(ctx, tenantID); createErr != nil {
			return nil, fmt.Errorf("failed to create metadata index: %w", createErr)
		}
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

		// Action line: use json.Marshal to properly escape string values
		compositeID := fmt.Sprintf("%d:%s|%d:%s", len(docID), docID, len(kbID), kbID)
		action, err := json.Marshal(map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
				"_id":    compositeID,
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
	req := esapi.BulkRequest{
		Body:    bytes.NewReader(buf.Bytes()),
		Refresh: "false",
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		common.Error("Failed to execute bulk request", err)
		return nil, fmt.Errorf("failed to execute bulk request: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		common.Sugar.Errorw("Elasticsearch bulk request returned error", "status", res.Status(), "body", string(bodyBytes))
		return nil, fmt.Errorf("elasticsearch bulk request returned error: %s, body: %s", res.Status(), string(bodyBytes))
	}

	// Parse bulk response to check for errors
	var bulkResponse map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&bulkResponse); err != nil {
		common.Error("Failed to parse bulk response", err)
		return nil, fmt.Errorf("failed to parse bulk response: %w", err)
	}

	// Check for errors in bulk response
	if errors, ok := bulkResponse["errors"].(bool); ok && errors {
		common.Warn("Bulk request had some errors")
	}

	common.Info("ElasticsearchConnection.InsertMetadata result", zap.String("index_name", indexName), zap.Int("count", len(metadata)))
	return []string{}, nil
}

// UpdateMetadata updates document metadata in tenant's metadata index
// The metaFields map will fully replace the existing meta_fields
func (e *elasticsearchEngine) UpdateMetadata(ctx context.Context, docID string, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("ElasticsearchConnection.UpdateMetadata called", zap.String("index_name", indexName), zap.String("docID", docID), zap.String("datasetID", datasetID))

	// Check if index exists
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("index '%s' does not exist", indexName)
	}

	// Build the document ID for update
	docID = strings.ReplaceAll(docID, "'", "''")
	datasetIDStr := strings.ReplaceAll(datasetID, "'", "''")

	// Build update body - merge meta_fields with existing
	query := map[string]interface{}{
		"bool": map[string]interface{}{
			"must": []map[string]interface{}{
				{"term": map[string]interface{}{"id": docID}},
				{"term": map[string]interface{}{"kb_id": datasetIDStr}},
			},
		},
	}

	updateReq := map[string]interface{}{
		"query": query,
		"script": map[string]interface{}{
			"source": "ctx._source.meta_fields = params.meta_fields",
			"params": map[string]interface{}{
				"meta_fields": metaFields,
			},
		},
	}

	updateBytes, err := json.Marshal(updateReq)
	if err != nil {
		return fmt.Errorf("failed to marshal update request: %w", err)
	}

	req := esapi.UpdateByQueryRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(updateBytes),
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		common.Error("Failed to execute update by query", err)
		return fmt.Errorf("failed to execute update by query: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		common.Sugar.Errorw("Elasticsearch update by query returned error", "status", res.Status())
		return fmt.Errorf("elasticsearch update by query returned error: %s", res.Status())
	}

	common.Info("ElasticsearchConnection.UpdateMetadata completes", zap.String("index_name", indexName), zap.String("docID", docID))
	return nil
}

// DeleteMetadata deletes metadata from tenant's metadata index by condition
// The condition is a map used to build an ES query (e.g., map["kb_id"]="xxx")
// Returns the number of deleted documents
func (e *elasticsearchEngine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("ElasticsearchConnection.DeleteMetadata called", zap.String("index_name", indexName), zap.Any("condition", condition))

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

	// Execute delete by query
	req := esapi.DeleteByQueryRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(bodyBytes),
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		common.Error("Failed to execute delete by query", err)
		return 0, fmt.Errorf("failed to execute delete by query: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		common.Sugar.Errorw("Elasticsearch delete by query returned error", "status", res.Status())
		return 0, fmt.Errorf("elasticsearch delete by query returned error: %s", res.Status())
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		common.Error("Failed to parse delete response", err)
		return 0, fmt.Errorf("failed to parse delete response: %w", err)
	}

	deleted := int64(0)
	if d, ok := result["deleted"].(float64); ok {
		deleted = int64(d)
	}

	common.Info("ElasticsearchConnection.DeleteMetadata completes", zap.String("index_name", indexName), zap.Int64("deleted_count", deleted))
	return deleted, nil
}

// DeleteMetadataKeys deletes specific metadata keys from a document's meta_fields.
// If deleting those keys leaves no metadata entries, the metadata document is removed.
func (e *elasticsearchEngine) DeleteMetadataKeys(ctx context.Context, docID string, datasetID string, keys []string, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("ElasticsearchConnection.DeleteMetadataKeys called", zap.String("index_name", indexName), zap.String("docID", docID), zap.Any("keys", keys))

	// Check if index exists
	exists, err := e.indexExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("index '%s' does not exist", indexName)
	}

	// Build the document ID for query (no escaping needed for ES term queries)
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
		"query": query,
		"_source": []string{"meta_fields"},
		"size": 1,
	}

	getBytes, err := json.Marshal(getReq)
	if err != nil {
		return fmt.Errorf("failed to marshal get request: %w", err)
	}

	// Use esapi.SearchRequest directly
	getSearchReq := esapi.SearchRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(getBytes),
	}

	getRes, err := getSearchReq.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("failed to get current metadata: %w", err)
	}
	defer getRes.Body.Close()

	if getRes.IsError() {
		return fmt.Errorf("elasticsearch get request returned error: %s", getRes.Status())
	}

	var getResult map[string]interface{}
	if err := json.NewDecoder(getRes.Body).Decode(&getResult); err != nil {
		return fmt.Errorf("failed to parse get response: %w", err)
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

	req := esapi.UpdateByQueryRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(updateBytes),
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		common.Error("Failed to execute update by query", err)
		return fmt.Errorf("failed to execute update by query: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		common.Sugar.Errorw("Elasticsearch update by query returned error", "status", res.Status())
		return fmt.Errorf("elasticsearch update by query returned error: %s", res.Status())
	}

	common.Info("ElasticsearchConnection.DeleteMetadataKeys completes", zap.String("index_name", indexName), zap.String("docID", docID))

	return nil
}

// DropMetadataStore drops a metadata index from Elasticsearch
func (e *elasticsearchEngine) DropMetadataStore(ctx context.Context, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	return e.dropIndex(ctx, indexName)
}

// MetadataStoreExists checks if a metadata index exists in Elasticsearch
func (e *elasticsearchEngine) MetadataStoreExists(ctx context.Context, tenantID string) (bool, error) {
	indexName := buildMetadataIndexName(tenantID)
	return e.indexExists(ctx, indexName)
}