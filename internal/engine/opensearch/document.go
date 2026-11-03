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
	"net/url"
)

// IndexDocument indexes a single document
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

	// Serialize document
	data, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("failed to marshal document: %w", err)
	}

	// Index document
	if _, err := e.doIndex(ctx, indexName, docID, bytes.NewReader(data), "true"); err != nil {
		return fmt.Errorf("failed to index document: %w", err)
	}

	return nil
}

// BulkIndex indexes documents in bulk
func (e *Engine) BulkIndex(ctx context.Context, indexName string, docs []interface{}) (interface{}, error) {
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

		// Copy without _id so the caller's map stays intact for a retry.
		body := make(map[string]interface{}, len(docMap))
		for k, v := range docMap {
			if k != "_id" {
				body[k] = v
			}
		}

		// Add index operation
		meta := map[string]interface{}{
			"_index": indexName,
			"_id":    docID,
		}
		metaData, _ := json.Marshal(meta)
		docData, _ := json.Marshal(body)

		buf.Write(metaData)
		buf.WriteByte('\n')
		buf.Write(docData)
		buf.WriteByte('\n')
	}

	// Execute bulk request
	result, err := e.doBulk(ctx, &buf, "true")
	if err != nil {
		return nil, err
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
									return nil, fmt.Errorf("bulk index error for _id %v: %s", opMap["_id"], reason)
								}
							}
						}
					}
				}
			}
		}
		return nil, fmt.Errorf("bulk index has errors")
	}

	took, _ := result["took"].(float64)
	hasErrors, _ := result["errors"].(bool)
	response := &BulkResponse{
		Took:    int64(took),
		Errors:  hasErrors,
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
func (e *Engine) GetDocument(ctx context.Context, indexName, docID string) (interface{}, error) {
	if indexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}
	if docID == "" {
		return nil, fmt.Errorf("document id cannot be empty")
	}

	// Get document
	path := fmt.Sprintf("/%s/_doc/%s", url.PathEscape(indexName), url.PathEscape(docID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := e.client.Perform(req)
	if err != nil {
		return nil, fmt.Errorf("get document request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("document not found")
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("get document error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse get document response: %w", err)
	}

	if found, ok := result["found"].(bool); !ok || !found {
		return nil, fmt.Errorf("document not found")
	}

	return result["_source"], nil
}

// DeleteDocument deletes a document
func (e *Engine) DeleteDocument(ctx context.Context, indexName, docID string) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}
	if docID == "" {
		return fmt.Errorf("document id cannot be empty")
	}

	// Delete document
	path := fmt.Sprintf("/%s/_doc/%s", url.PathEscape(indexName), url.PathEscape(docID))
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}

	resp, err := e.client.Perform(req)
	if err != nil {
		return fmt.Errorf("delete document request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("document not found")
	}
	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete document error (status %d): %s", resp.StatusCode, string(respBody))
	}

	return nil
}
