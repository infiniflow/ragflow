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
)

// IndexDocument indexes a single document
func (e *elasticsearchEngine) IndexDocument(ctx context.Context, indexName, docID string, doc interface{}) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}
	if docID == "" {
		return fmt.Errorf("document id cannot be empty")
	}
	if doc == nil {
		return fmt.Errorf("document cannot be nil")
	}

	// Serialize document
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	// Index document
	req := esapi.IndexRequest{
		Index:      indexName,
		DocumentID: docID,
		Body:       bytes.NewReader(data),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	return nil
}

// BulkIndex indexes documents in bulk
func (e *elasticsearchEngine) BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error) {
	if indexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("documents cannot be empty")
	}

	// Build bulk request
	var buf bytes.Buffer
	for _, doc := range docs {
		docMap, ok := doc.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("document must be map[string]interface{}")
		}

		docID, hasID := docMap["_id"]
		if !hasID {
			return nil, fmt.Errorf("document missing _id field")
		}

		// Delete _id field to avoid duplication
		delete(docMap, "_id")

		// Add index operation
		meta := map[string]interface{}{
			"_index": indexName,
			"_id":    docID,
		}
		metaData, _ := json.Marshal(meta)
		docData, _ := json.Marshal(docMap)

		buf.Write(metaData)
		buf.WriteByte('\n')
		buf.Write(docData)
		buf.WriteByte('\n')
	}

	// Execute bulk request
	req := esapi.BulkRequest{
		Body:    &buf,
		Refresh: "true",
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return nil, fmt.Errorf("bulk index failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for errors
	if errors, ok := result["errors"].(bool); ok && errors {
		// Get error details
		if items, ok := result["items"].([]interface{}); ok && len(items) > 0 {
			for _, item := range items {
				if itemMap, ok := item.(map[string]interface{}); ok {
					for _, op := range itemMap {
						if opMap, ok := op.(map[string]interface{}); ok {
							if errInfo, ok := opMap["error"].(map[string]interface{}); ok {
								if reason, ok := errInfo["reason"].(string); ok {
									return nil, fmt.Errorf("bulk index error: %s", reason)
								}
							}
						}
					}
				}
			}
		}
		return nil, fmt.Errorf("bulk index has errors")
	}

	response := &BulkResponse{
		Took:    int64(result["took"].(float64)),
		Errors:  result["errors"].(bool),
		Indexed: len(docs),
	}

	return response, nil
}

// BulkResponse bulk operation response
type BulkResponse struct {
	Took    int64
	Errors  bool
	Indexed int
}

// GetDocument gets a document
func (e *elasticsearchEngine) GetDocument(ctx context.Context, indexName, docID string) (interface{}, error) {
	if indexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}
	if docID == "" {
		return nil, fmt.Errorf("document id cannot be empty")
	}

	// Get document
	req := esapi.GetRequest{
		Index:      indexName,
		DocumentID: docID,
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return nil, fmt.Errorf("failed to get document: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 404 {
		return nil, fmt.Errorf("document not found")
	}

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if found, ok := result["found"].(bool); !ok || !found {
		return nil, fmt.Errorf("document not found")
	}

	return result["_source"], nil
}

// DeleteDocument deletes a document
func (e *elasticsearchEngine) DeleteDocument(ctx context.Context, indexName, docID string) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}
	if docID == "" {
		return fmt.Errorf("document id cannot be empty")
	}

	// Delete document
	req := esapi.DeleteRequest{
		Index:      indexName,
		DocumentID: docID,
		Refresh:    "true",
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 404 {
		return fmt.Errorf("document not found")
	}

	if res.IsError() {
		return fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	return nil
}

// UpdateDocument updates a document (partial update)
//
// Parameters:
//   - ctx: Context for the operation
//   - indexName: The name of the index
//   - docID: The document ID to update
//   - doc: The document fields to update (partial update)
//
// Returns:
//   - error: Error if the operation fails
//
// Example:
//
//	err := engine.UpdateDocument(ctx, "memory_tenant123", "mem456_1", map[string]interface{}{"status": 1})
func (e *elasticsearchEngine) UpdateDocument(ctx context.Context, indexName, docID string, doc interface{}) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}
	if docID == "" {
		return fmt.Errorf("document id cannot be empty")
	}
	if doc == nil {
		return fmt.Errorf("document cannot be nil")
	}

	// Wrap the document in a "doc" field for partial update
	updateBody := map[string]interface{}{
		"doc": doc,
	}

	data, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("failed to marshal update body: %w", err)
	}

	// Update document
	req := esapi.UpdateRequest{
		Index:      indexName,
		DocumentID: docID,
		Body:       bytes.NewReader(data),
		Refresh:    "true",
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("failed to update document: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 404 {
		return fmt.Errorf("document not found")
	}

	if res.IsError() {
		return fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	return nil
}

// DeleteByQuery deletes documents matching a query
//
// Parameters:
//   - ctx: Context for the operation
//   - indexName: The name of the index
//   - query: The query to match documents for deletion
//
// Returns:
//   - int64: Number of documents deleted
//   - error: Error if the operation fails
//
// Example:
//
//	count, err := engine.DeleteByQuery(ctx, "memory_tenant123", map[string]interface{}{
//	    "term": map[string]interface{}{"memory_id": "mem456"},
//	})
func (e *elasticsearchEngine) DeleteByQuery(ctx context.Context, indexName string, query map[string]interface{}) (int64, error) {
	if indexName == "" {
		return 0, fmt.Errorf("index name cannot be empty")
	}
	if query == nil {
		return 0, fmt.Errorf("query cannot be nil")
	}

	queryBody := map[string]interface{}{
		"query": query,
	}

	data, err := json.Marshal(queryBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal query body: %w", err)
	}

	req := esapi.DeleteByQueryRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(data),
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return 0, fmt.Errorf("failed to delete by query: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return 0, fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	// Get deleted count
	var deleted int64
	if deletedFloat, ok := result["deleted"].(float64); ok {
		deleted = int64(deletedFloat)
	}

	return deleted, nil
}

// UpdateByQuery updates documents matching a query
//
// Parameters:
//   - ctx: Context for the operation
//   - indexName: The name of the index
//   - query: The query to match documents for update
//   - updateDoc: The fields to update
//
// Returns:
//   - int64: Number of documents updated
//   - error: Error if the operation fails
//
// Example:
//
//	count, err := engine.UpdateByQuery(ctx, "memory_tenant123",
//	    map[string]interface{}{"term": map[string]interface{}{"memory_id": "mem456"}},
//	    map[string]interface{}{"status": 1},
//	)
func (e *elasticsearchEngine) UpdateByQuery(ctx context.Context, indexName string, query map[string]interface{}, updateDoc map[string]interface{}) (int64, error) {
	if indexName == "" {
		return 0, fmt.Errorf("index name cannot be empty")
	}
	if query == nil {
		return 0, fmt.Errorf("query cannot be nil")
	}
	if updateDoc == nil {
		return 0, fmt.Errorf("update document cannot be nil")
	}

	queryBody := map[string]interface{}{
		"query": query,
		"script": map[string]interface{}{
			"source": buildUpdateScript(updateDoc),
			"lang":   "painless",
		},
	}

	data, err := json.Marshal(queryBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal query body: %w", err)
	}

	req := esapi.UpdateByQueryRequest{
		Index: []string{indexName},
		Body:  bytes.NewReader(data),
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return 0, fmt.Errorf("failed to update by query: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return 0, fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return 0, fmt.Errorf("failed to parse response: %w", err)
	}

	// Get updated count
	var updated int64
	if updatedFloat, ok := result["updated"].(float64); ok {
		updated = int64(updatedFloat)
	}

	return updated, nil
}

// buildUpdateScript builds a painless script for updating documents
func buildUpdateScript(updateDoc map[string]interface{}) string {
	var scriptParts []string
	for key, value := range updateDoc {
		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = fmt.Sprintf("'%s'", v)
		default:
			valueBytes, _ := json.Marshal(v)
			valueStr = string(valueBytes)
		}
		scriptParts = append(scriptParts, fmt.Sprintf("ctx._source.%s = %s", key, valueStr))
	}
	return strings.Join(scriptParts, "; ")
}
