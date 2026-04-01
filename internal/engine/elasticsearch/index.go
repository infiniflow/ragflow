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
	"os"

	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// CreateIndex creates an index
func (e *elasticsearchEngine) CreateIndex(ctx context.Context, indexName, datasetID string, vectorSize int, parserID string) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	// Check if index already exists
	exists, err := e.IndexExists(ctx, indexName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if exists {
		return fmt.Errorf("index '%s' already exists", indexName)
	}

	// Load mapping based on index type
	var mapping map[string]interface{}
	if datasetID == "skill" {
		// Load skill-specific mapping
		skillMapping, err := loadSkillMapping()
		if err != nil {
			return fmt.Errorf("failed to load skill mapping: %w", err)
		}
		mapping = skillMapping
	} else {
		// Default mapping for dataset
		mapping = map[string]interface{}{
			"settings": map[string]interface{}{
				"number_of_shards":   1,
				"number_of_replicas": 0,
			},
		}
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
		return fmt.Errorf("index creation not acknowledged")
	}

	return nil
}

// loadSkillMapping loads the skill index mapping from config file
func loadSkillMapping() (map[string]interface{}, error) {
	// Try multiple possible locations for the mapping file
	possiblePaths := []string{
		"conf/skill_es_mapping.json",
		"../conf/skill_es_mapping.json",
		"/app/conf/skill_es_mapping.json",
		"/home/infominer/codebase/ragflow/conf/skill_es_mapping.json",
	}

	var data []byte
	var err error
	for _, path := range possiblePaths {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
	}

	if err != nil {
		// Fallback to default skill mapping if file not found
		return getDefaultSkillMapping(), nil
	}

	var mapping map[string]interface{}
	if err := json.Unmarshal(data, &mapping); err != nil {
		return nil, fmt.Errorf("failed to parse skill mapping: %w", err)
	}

	return mapping, nil
}

// getDefaultSkillMapping returns the default skill index mapping
func getDefaultSkillMapping() map[string]interface{} {
	return map[string]interface{}{
		"settings": map[string]interface{}{
			"index": map[string]interface{}{
				"number_of_shards":   1,
				"number_of_replicas": 0,
				"refresh_interval":   "1000ms",
			},
		},
		"mappings": map[string]interface{}{
			"dynamic": false,
			"properties": map[string]interface{}{
				"skill_id": map[string]interface{}{
					"type":  "keyword",
					"store": true,
				},
				"name": map[string]interface{}{
					"type":   "text",
					"index":  false,
					"store":  true,
				},
				"name_tks": map[string]interface{}{
					"type":      "text",
					"analyzer":  "whitespace",
					"store":     true,
				},
				"tags": map[string]interface{}{
					"type":   "text",
					"index":  false,
					"store":  true,
				},
				"tags_tks": map[string]interface{}{
					"type":      "text",
					"analyzer":  "whitespace",
					"store":     true,
				},
				"description": map[string]interface{}{
					"type":   "text",
					"index":  false,
					"store":  true,
				},
				"description_tks": map[string]interface{}{
					"type":      "text",
					"analyzer":  "whitespace",
					"store":     true,
				},
				"content": map[string]interface{}{
					"type":   "text",
					"index":  false,
					"store":  true,
				},
				"content_tks": map[string]interface{}{
					"type":      "text",
					"analyzer":  "whitespace",
					"store":     true,
				},
				"q_1024_vec": map[string]interface{}{
					"type":       "dense_vector",
					"dims":       1024,
					"index":      true,
					"similarity": "cosine",
				},
				"version": map[string]interface{}{
					"type":  "keyword",
					"store": true,
				},
				"status": map[string]interface{}{
					"type":  "keyword",
					"store": true,
				},
				"create_time": map[string]interface{}{
					"type":  "long",
					"store": true,
				},
				"update_time": map[string]interface{}{
					"type":  "long",
					"store": true,
				},
			},
		},
	}
}

// DeleteIndex deletes an index
func (e *elasticsearchEngine) DeleteIndex(ctx context.Context, indexName string) error {
	if indexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	// Check if index exists
	exists, err := e.IndexExists(ctx, indexName)
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

// IndexExists checks if index exists
func (e *elasticsearchEngine) IndexExists(ctx context.Context, indexName string) (bool, error) {
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

// CreateDocMetaIndex creates the document metadata index
func (e *elasticsearchEngine) CreateDocMetaIndex(ctx context.Context, indexName string) error {
	// TODO
	return nil
}
