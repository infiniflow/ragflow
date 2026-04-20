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

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// CreateDataset creates an index
func (e *elasticsearchEngine) CreateDataset(ctx context.Context, indexName, datasetID string, vectorSize int, parserID string) error {
	// Elasticsearch doesn't support vector_size or parser_id in the same way
	// Build mapping for ES (if needed)
	// TODO
	mapping := map[string]interface{}{
		"dataset_id": datasetID,
	}

	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	// Check if index already exists
	exists, err := e.TableExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if exists {
		return fmt.Errorf("index '%s' already exists", indexName)
	}

	// Prepare request body
	var body io.Reader
	if mapping != nil {
		data, err := json.Marshal(mapping)
		if err != nil {
			return fmt.Errorf("failed to marshal mapping: %w", err)
		}
		body = bytes.NewReader(data)
	}

	// Create index
	req := esapi.IndicesCreateRequest{
		Index: indexName,
		Body:  body,
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	// Parse response
	var result map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return fmt.Errorf("failed to parse response: %w", err)
	}

	acknowledged, ok := result["acknowledged"].(bool)
	if !ok || !acknowledged {
		return fmt.Errorf("index creation not acknowledged")
	}

	return nil
}

// DropTable deletes an index
func (e *elasticsearchEngine) DropTable(ctx context.Context, indexName string) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	// Check if index exists
	exists, err := e.TableExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("index '%s' does not exist", indexName)
	}

	// Delete index
	req := esapi.IndicesDeleteRequest{
		Index: []string{indexName},
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("failed to delete index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("elasticsearch returned error: %s", res.Status())
	}

	return nil
}

// TableExists checks if index exists
func (e *elasticsearchEngine) TableExists(ctx context.Context, indexName string) (bool, error) {
	if indexName == "" {
		return false, fmt.Errorf("index name cannot be empty")
	}

	req := esapi.IndicesExistsRequest{
		Index: []string{indexName},
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return false, fmt.Errorf("failed to check index existence: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode == 200 {
		return true, nil
	} else if res.StatusCode == 404 {
		return false, nil
	}

	return false, fmt.Errorf("elasticsearch returned error: %s", res.Status())
}

// CreateMetadata creates the document metadata index
func (e *elasticsearchEngine) CreateMetadata(ctx context.Context, indexName string) error {
	// TODO
	return nil
}

// InsertDataset inserts documents into a dataset index
func (e *elasticsearchEngine) InsertDataset(ctx context.Context, documents []map[string]interface{}, indexName string, knowledgebaseID string) ([]string, error) {
	// TODO
	return []string{}, nil
}

// InsertMetadata inserts documents into tenant's metadata index
func (e *elasticsearchEngine) InsertMetadata(ctx context.Context, documents []map[string]interface{}, tenantID string) ([]string, error) {
	// TODO
	return []string{}, nil
}

// UpdateDataset updates a chunk by condition
func (e *elasticsearchEngine) UpdateDataset(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, tableNamePrefix string, knowledgebaseID string) error {
	// TODO
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
