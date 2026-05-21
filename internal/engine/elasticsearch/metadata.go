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
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"ragflow/internal/common"

	"go.uber.org/zap"
)

// CreateMetadataStore creates the document metadata index
func (e *elasticsearchEngine) CreateMetadataStore(ctx context.Context, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	req := esapi.IndicesCreateRequest{
		Index: indexName,
	}
	res, err := req.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("failed to create metadata index: %w", err)
	}
	defer res.Body.Close()
	if res.IsError() {
		return fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}
	return nil
}

// InsertMetadata inserts documents into tenant's metadata index
func (e *elasticsearchEngine) InsertMetadata(ctx context.Context, metadata []map[string]interface{}, tenantID string) ([]string, error) {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("Inserting metadata into Elasticsearch index", zap.String("index_name", indexName), zap.String("tenant_id", tenantID), zap.Int("doc_count", len(metadata)))

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
		// Action line - index operation
		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": indexName,
			},
		}
		actionBytes, err := json.Marshal(action)
		if err != nil {
			common.Error("Failed to marshal bulk action", err)
			return nil, fmt.Errorf("failed to marshal bulk action: %w", err)
		}
		buf.Write(actionBytes)
		buf.WriteByte('\n')

		// Document line - meta_fields is stored as-is (ES can handle nested objects)
		docBytes, err := json.Marshal(doc)
		if err != nil {
			common.Error("Failed to marshal document", err)
			return nil, fmt.Errorf("failed to marshal document: %w", err)
		}
		buf.Write(docBytes)
		buf.WriteByte('\n')
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
		common.Sugar.Errorw("Elasticsearch bulk request returned error", "status", res.Status())
		return nil, fmt.Errorf("elasticsearch bulk request returned error: %s", res.Status())
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

	common.Info("Successfully inserted metadata into Elasticsearch index", zap.String("index_name", indexName), zap.Int("doc_count", len(metadata)))
	return []string{}, nil
}

// UpdateMetadata updates document metadata in tenant's metadata index
func (e *elasticsearchEngine) UpdateMetadata(ctx context.Context, docID string, datasetID string, metaFields map[string]interface{}, tenantID string) error {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("Updating metadata in Elasticsearch index", zap.String("index_name", indexName), zap.String("docID", docID), zap.String("datasetID", datasetID))

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

	common.Info("Successfully updated metadata in Elasticsearch index", zap.String("index_name", indexName), zap.String("docID", docID))
	return nil
}

// DeleteMetadata deletes metadata from tenant's metadata index by condition
func (e *elasticsearchEngine) DeleteMetadata(ctx context.Context, condition map[string]interface{}, tenantID string) (int64, error) {
	indexName := buildMetadataIndexName(tenantID)
	common.Info("Deleting metadata from Elasticsearch index", zap.String("index_name", indexName), zap.Any("condition", condition))

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

	common.Info("Successfully deleted metadata", zap.String("index_name", indexName), zap.Int64("deleted_count", deleted))
	return deleted, nil
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