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
