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

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"ragflow/internal/common"

	"go.uber.org/zap"
)

// InsertChunks inserts documents into a dataset index
func (e *elasticsearchEngine) InsertChunks(ctx context.Context, documents []map[string]interface{}, indexName string, knowledgebaseID string) ([]string, error) {
	common.Info("Inserting chunks into Elasticsearch index", zap.String("index_name", indexName), zap.String("knowledgebase_id", knowledgebaseID), zap.Int("doc_count", len(documents)))

	if len(documents) == 0 {
		return []string{}, nil
	}

	if indexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}

	// Build bulk request body
	var buf bytes.Buffer
	for _, doc := range documents {
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

		// Document line
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
		// Could iterate through items to find specific errors if needed
	}

	common.Info("Successfully inserted chunks into Elasticsearch index", zap.String("index_name", indexName), zap.Int("doc_count", len(documents)))
	return []string{}, nil
}

// InsertMetadata inserts documents into tenant's metadata index
func (e *elasticsearchEngine) InsertMetadata(ctx context.Context, documents []map[string]interface{}, tenantID string) ([]string, error) {
	indexName := fmt.Sprintf("ragflow_doc_meta_%s", tenantID)
	common.Info("Inserting metadata into Elasticsearch index", zap.String("index_name", indexName), zap.String("tenant_id", tenantID), zap.Int("doc_count", len(documents)))

	if len(documents) == 0 {
		return []string{}, nil
	}

	if indexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}

	// Check if index exists, create if not
	exists, err := e.StoreExists(ctx, indexName)
	if err != nil {
		common.Error("Failed to check index existence", err)
		return nil, fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		// Create metadata index
		if createErr := e.CreateMetadataStore(ctx, indexName); createErr != nil {
			return nil, fmt.Errorf("failed to create metadata index: %w", createErr)
		}
	}

	// Build bulk request body
	var buf bytes.Buffer
	for _, doc := range documents {
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

	common.Info("Successfully inserted metadata into Elasticsearch index", zap.String("index_name", indexName), zap.Int("doc_count", len(documents)))
	return []string{}, nil
}

// UpdateChunks updates chunks by condition
func (e *elasticsearchEngine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, tableNamePrefix string, knowledgebaseID string) error {
	indexName := fmt.Sprintf("%s_%s", tableNamePrefix, knowledgebaseID)
	common.Info("Updating chunks in Elasticsearch index", zap.String("index_name", indexName), zap.Any("condition", condition), zap.Any("new_value", newValue))

	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	// Check if index exists
	exists, err := e.StoreExists(ctx, indexName)
	if err != nil {
		common.Error("Failed to check index existence", err)
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("index '%s' does not exist", indexName)
	}

	// Build query from condition
	query := e.buildQueryFromCondition(condition)
	if query == nil {
		query = map[string]interface{}{"match_all": map[string]interface{}{}}
	}

	// Process remove operation if present
	var removeOperations []map[string]interface{}
	if removeData, ok := newValue["remove"].(map[string]interface{}); ok {
		removeOperations = e.buildRemoveOperations(removeData, query, indexName)
	}
	delete(newValue, "remove")

	// Build update body
	updateBody := map[string]interface{}{
		"query": query,
	}

	// Handle script-based update if needed (for remove operations or transformations)
	if len(removeOperations) > 0 || e.needsScriptUpdate(newValue) {
		script := e.buildUpdateScript(newValue, removeOperations)
		updateBody["script"] = script
	} else {
		updateBody["doc"] = newValue
	}

	bodyBytes, err := json.Marshal(updateBody)
	if err != nil {
		common.Error("Failed to marshal update body", err)
		return fmt.Errorf("failed to marshal update body: %w", err)
	}

	// Execute update by query
	req := esapi.UpdateByQueryRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(bodyBytes),
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

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		common.Error("Failed to parse update response", err)
		return fmt.Errorf("failed to parse update response: %w", err)
	}

	if updated, ok := result["updated"].(float64); ok {
		common.Info("Successfully updated chunks", zap.String("index_name", indexName), zap.Float64("updated_count", updated))
	}

	return nil
}

// UpdateMetadata updates document metadata in tenant's metadata index
func (e *elasticsearchEngine) UpdateMetadata(ctx context.Context, docID string, kbID string, metaFields map[string]interface{}, tenantID string) error {
	// TODO
	return nil
}

// Delete deletes rows from either a dataset index or metadata index
func (e *elasticsearchEngine) Delete(ctx context.Context, condition map[string]interface{}, indexName string, datasetID string) (int64, error) {
	// TODO
	return 0, nil
}

// buildQueryFromCondition builds an ES query from condition map
func (e *elasticsearchEngine) buildQueryFromCondition(condition map[string]interface{}) map[string]interface{} {
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
			// Handle kb_id as terms query
			if listVal, ok := v.([]interface{}); ok {
				clauses = append(clauses, map[string]interface{}{
					"terms": map[string]interface{}{k: listVal},
				})
			} else {
				clauses = append(clauses, map[string]interface{}{
					"term": map[string]interface{}{k: v},
				})
			}
		case "id":
			// Handle id as terms or term query
			if listVal, ok := v.([]interface{}); ok {
				clauses = append(clauses, map[string]interface{}{
					"terms": map[string]interface{}{k: listVal},
				})
			} else {
				clauses = append(clauses, map[string]interface{}{
					"term": map[string]interface{}{k: v},
				})
			}
		case "available_int":
			// Handle available_int as term query
			clauses = append(clauses, map[string]interface{}{
				"term": map[string]interface{}{k: v},
			})
		default:
			// Default: treat as term query
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
			"must": clauses,
		},
	}
}

// buildRemoveOperations builds ES script operations for remove
func (e *elasticsearchEngine) buildRemoveOperations(removeData map[string]interface{}, query map[string]interface{}, indexName string) []map[string]interface{} {
	// For ES, we handle removals differently - they are typically done via separate update operations
	// This is a simplified implementation
	return nil
}

// needsScriptUpdate checks if the update requires a script (more complex operations)
func (e *elasticsearchEngine) needsScriptUpdate(newValue map[string]interface{}) bool {
	// Check if any values contain operations that need scripts
	return false
}

// buildUpdateScript builds an ES script for updates
func (e *elasticsearchEngine) buildUpdateScript(newValue map[string]interface{}, removeOperations []map[string]interface{}) map[string]interface{} {
	script := map[string]interface{}{
		"source": "ctx._source.putAll(params.doc)",
		"params": map[string]interface{}{
			"doc": newValue,
		},
	}
	return script
}