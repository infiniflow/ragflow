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
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"ragflow/internal/common"
	"ragflow/internal/engine/types"

	"go.uber.org/zap"
)

// CreateChunkStore creates an index
func (e *elasticsearchEngine) CreateChunkStore(ctx context.Context, baseName, datasetID string, vectorSize int, parserID string) error {
	if baseName == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	// Check if index already exists
	exists, err := e.indexExists(ctx, baseName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if exists {
		return fmt.Errorf("index '%s' already exists", baseName)
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
		Index: baseName,
		Body:  body,
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		return fmt.Errorf("failed to create index: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		reason := extractErrorReason(bodyBytes)
		if reason != "" {
			return fmt.Errorf("elasticsearch error: %s", reason)
		}
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

// InsertChunks inserts documents into a dataset index
func (e *elasticsearchEngine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName string, datasetID string) ([]string, error) {
	fullIndexName := fmt.Sprintf("%s_%s", baseName, datasetID)
	common.Info("Inserting chunks into Elasticsearch index", zap.String("index_name", fullIndexName), zap.String("dataset_id", datasetID), zap.Int("doc_count", len(chunks)))

	if len(chunks) == 0 {
		return []string{}, nil
	}

	if fullIndexName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}

	// Build bulk request body
	var buf bytes.Buffer
	for _, doc := range chunks {
		// Action line - index operation
		action := map[string]interface{}{
			"index": map[string]interface{}{
				"_index": fullIndexName,
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

	common.Info("Successfully inserted chunks into Elasticsearch index", zap.String("index_name", fullIndexName), zap.Int("doc_count", len(chunks)))
	return []string{}, nil
}

// UpdateChunks updates chunks by condition
func (e *elasticsearchEngine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, baseName string, datasetID string) error {
	fullIndexName := fmt.Sprintf("%s_%s", baseName, datasetID)
	common.Info("Updating chunks in Elasticsearch index", zap.String("index_name", fullIndexName), zap.Any("condition", condition), zap.Any("new_value", newValue))

	if fullIndexName == "" {
		return fmt.Errorf("index name cannot be empty")
	}

	// Check if index exists
	exists, err := e.indexExists(ctx, fullIndexName)
	if err != nil {
		common.Error("Failed to check index existence", err)
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		return fmt.Errorf("index '%s' does not exist", fullIndexName)
	}

	// Build query from condition
	query := e.buildQueryFromCondition(condition)
	if query == nil {
		query = map[string]interface{}{"match_all": map[string]interface{}{}}
	}

	// Process remove operation if present
	var removeOperations []map[string]interface{}
	if removeData, ok := newValue["remove"].(map[string]interface{}); ok {
		removeOperations = e.buildRemoveOperations(removeData, query, fullIndexName)
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
		Index: []string{fullIndexName},
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
		common.Info("Successfully updated chunks", zap.String("index_name", fullIndexName), zap.Float64("updated_count", updated))
	}

	return nil
}

// DeleteChunks deletes chunks from a dataset index by condition
func (e *elasticsearchEngine) DeleteChunks(ctx context.Context, condition map[string]interface{}, indexName string, datasetID string) (int64, error) {
	fullIndexName := fmt.Sprintf("%s_%s", indexName, datasetID)
	common.Info("Deleting chunks from Elasticsearch index", zap.String("index_name", fullIndexName), zap.Any("condition", condition))

	// Check if index exists
	exists, err := e.indexExists(ctx, fullIndexName)
	if err != nil {
		return 0, fmt.Errorf("failed to check index existence: %w", err)
	}
	if !exists {
		common.Warn(fmt.Sprintf("Index %s does not exist, skipping delete", fullIndexName))
		return 0, nil
	}

	// Build query from condition
	query := e.buildQueryFromCondition(condition)
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
		Index: []string{fullIndexName},
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

	common.Info("Successfully deleted chunks", zap.String("index_name", fullIndexName), zap.Int64("deleted_count", deleted))
	return deleted, nil
}

// SearchResponse Elasticsearch search response
type SearchResponse struct {
	Hits struct {
		Total struct {
			Value int64 `json:"value"`
		} `json:"total"`
		Hits []struct {
			ID     string                 `json:"_id"`
			Score  float64                `json:"_score"`
			Source map[string]interface{} `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]interface{} `json:"aggregations"`
}

// Search executes search with unified types.SearchRequest
func (e *elasticsearchEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	return e.searchUnified(ctx, req)
}

// searchUnified handles the unified types.SearchRequest
func (e *elasticsearchEngine) searchUnified(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	// Build pagination parameters
	offset := req.Offset
	limit := req.Limit
	if limit <= 0 {
		limit = 30 // default ES size
	}

	// Check if this is a skill index
	isSkillIndex := len(req.IndexNames) > 0 && strings.HasPrefix(req.IndexNames[0], "skill_")

	// Build filter clauses
	var filterClauses []map[string]interface{}
	if isSkillIndex {
		filterClauses = buildSkillFilterClauses()
	} else {
		filterClauses = buildFilterClauses(req.KbIDs, 1)
	}

	// Add filters from req.Filter
	if req.Filter != nil && len(req.Filter) > 0 {
		filterClauses = append(filterClauses, buildFilterFromMap(req.Filter)...)
	}

	// Build search query body
	queryBody := make(map[string]interface{})

	// Determine search type from MatchExprs
	var matchText string
	var matchDense *types.MatchDenseExpr
	var hasVectorMatch bool

	for _, expr := range req.MatchExprs {
		if expr == nil {
			continue
		}
		switch e := expr.(type) {
		case string:
			matchText = e
		case *types.MatchTextExpr:
			matchText = e.MatchingText
		case *types.MatchDenseExpr:
			hasVectorMatch = true
			matchDense = e
		}
	}

	var vectorFieldName string
	if !hasVectorMatch || matchDense == nil {
		// Keyword-only search
		if isSkillIndex {
			queryBody["query"] = buildSkillKeywordQuery(matchText, filterClauses, 1.0)
		} else {
			queryBody["query"] = buildESKeywordQuery(matchText, filterClauses, 1.0)
		}
	} else {
		// Hybrid search: keyword + vector
		textWeight := 0.7 // default: vector weight = 0.3
		vectorWeight := 0.3
		if matchDense.ExtraOptions != nil {
			if vw, ok := matchDense.ExtraOptions["text_weight"].(float64); ok {
				textWeight = vw
			}
			if vw, ok := matchDense.ExtraOptions["vector_weight"].(float64); ok {
				vectorWeight = vw
			}
		}

		// Build boolean query for text match and filters
		var boolQuery map[string]interface{}
		if isSkillIndex {
			boolQuery = buildSkillKeywordQuery(matchText, filterClauses, 1.0)
		} else {
			boolQuery = buildESKeywordQuery(matchText, filterClauses, 1.0)
		}
		// Add boost to the bool query (as in Python code)
		if boolMap, ok := boolQuery["bool"].(map[string]interface{}); ok {
			boolMap["boost"] = textWeight
		}

		// Build kNN query
		vectorData := matchDense.EmbeddingData
		vectorFieldName = matchDense.VectorColumnName
		k := matchDense.TopN
		if k <= 0 {
			k = req.Limit
		}
		if k <= 0 {
			k = 1024
		}
		numCandidates := k * 2

		similarity := 0.0
		if matchDense.ExtraOptions != nil {
			if sim, ok := matchDense.ExtraOptions["similarity"].(float64); ok {
				similarity = sim
			}
		}

		knnQuery := map[string]interface{}{
			"field":          vectorFieldName,
			"query_vector":   vectorData,
			"k":              k,
			"num_candidates": numCandidates,
			"similarity":     similarity,
			"boost":          vectorWeight,
		}

		queryBody["knn"] = knnQuery
		queryBody["query"] = boolQuery

		// Add vector column to Source fields (matching Python ES: src.append(f"q_{len(q_vec)}_vec"))
		// Only modify Source if it was explicitly set by the caller
		if vectorFieldName != "" && len(req.SelectFields) > 0 {
			sourceFields := req.SelectFields
			found := false
			for _, f := range sourceFields {
				if f == vectorFieldName {
					found = true
					break
				}
			}
			if !found {
				sourceFields = append(sourceFields, vectorFieldName)
			}
			req.SelectFields = sourceFields
		}
	}

	queryBody["size"] = limit
	queryBody["from"] = offset

	// Add sorting if specified
	if req.OrderBy != nil {
		sort := parseOrderByExpr(req.OrderBy)
		if len(sort) > 0 {
			queryBody["sort"] = sort
		}
	}

	// Serialize query
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(queryBody); err != nil {
		return nil, fmt.Errorf("error encoding query: %w", err)
	}

	// Log search details
	common.Debug("Elasticsearch searching indices", zap.Strings("indices", req.IndexNames))
	common.Debug("Elasticsearch DSL", zap.Any("dsl", queryBody))

	// Build search request
	reqES := esapi.SearchRequest{
		Index: req.IndexNames,
		Body:  &buf,
	}

	// Execute search
	res, err := reqES.Do(ctx, e.client)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, err := io.ReadAll(res.Body)
		if err != nil {
			common.Error("Elasticsearch failed to read error response body", err)
		} else {
			common.Warn("Elasticsearch error response", zap.String("body", string(bodyBytes)))
		}
		return nil, fmt.Errorf("Elasticsearch returned error: %s", res.Status())
	}

	// Parse response
	var esResp SearchResponse
	if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	// Convert to unified response
	chunks := convertESResponse(&esResp, vectorFieldName)
	return &types.SearchResult{
		Chunks: chunks,
		Total:  esResp.Hits.Total.Value,
	}, nil
}

// GetChunk gets a chunk by ID
func (e *elasticsearchEngine) GetChunk(ctx context.Context, baseName, chunkID string, datasetIDs []string) (interface{}, error) {
	// Build unified search request to get the chunk by ID
	searchReq := &types.SearchRequest{
		IndexNames: []string{baseName},
		Limit:      1,
		Offset:     0,
		Filter: map[string]interface{}{
			"id": chunkID,
		},
	}

	// Execute search
	searchResp, err := e.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to search: %w", err)
	}

	if len(searchResp.Chunks) == 0 {
		return nil, nil
	}

	return searchResp.Chunks[0], nil
}

// GetFields is not implemented for Elasticsearch
func (e *elasticsearchEngine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	common.Warn("GetFields not implemented for Elasticsearch")
	return nil
}

// GetAggregation is not implemented for Elasticsearch
func (e *elasticsearchEngine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	common.Warn("GetAggregation not implemented for Elasticsearch")
	return nil
}

// GetHighlight is not implemented for Elasticsearch
func (e *elasticsearchEngine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	common.Warn("GetHighlight not implemented for Elasticsearch")
	return nil
}

// DropChunkStore deletes a chunk index
func (e *elasticsearchEngine) DropChunkStore(ctx context.Context, baseName, datasetID string) error {
	return e.dropIndex(ctx, baseName)
}

// ChunkStoreExists checks if a chunk index exists
func (e *elasticsearchEngine) ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error) {
	return e.indexExists(ctx, baseName)
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

// buildMetadataQueryFromCondition builds an ES query for metadata index
func (e *elasticsearchEngine) buildMetadataQueryFromCondition(condition map[string]interface{}) map[string]interface{} {
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
			"must": clauses,
		},
	}
}

// loadSkillMapping loads the skill index mapping from config file
func loadSkillMapping() (map[string]interface{}, error) {
	// Try multiple possible locations for the mapping file
	possiblePaths := []string{
		"conf/skill_es_mapping.json",
		"../conf/skill_es_mapping.json",
		"/app/conf/skill_es_mapping.json",
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
					"type":  "text",
					"index": false,
					"store": true,
				},
				"name_tks": map[string]interface{}{
					"type":     "text",
					"analyzer": "whitespace",
					"store":    true,
				},
				"tags": map[string]interface{}{
					"type":  "text",
					"index": false,
					"store": true,
				},
				"tags_tks": map[string]interface{}{
					"type":     "text",
					"analyzer": "whitespace",
					"store":    true,
				},
				"description": map[string]interface{}{
					"type":  "text",
					"index": false,
					"store": true,
				},
				"description_tks": map[string]interface{}{
					"type":     "text",
					"analyzer": "whitespace",
					"store":    true,
				},
				"content": map[string]interface{}{
					"type":  "text",
					"index": false,
					"store": true,
				},
				"content_tks": map[string]interface{}{
					"type":     "text",
					"analyzer": "whitespace",
					"store":    true,
				},
				"q_3072_vec": map[string]interface{}{
					"type":       "dense_vector",
					"dims":       3072,
					"index":      true,
					"similarity": "cosine",
				},
				"q_2560_vec": map[string]interface{}{
					"type":       "dense_vector",
					"dims":       2560,
					"index":      true,
					"similarity": "cosine",
				},
				"q_1536_vec": map[string]interface{}{
					"type":       "dense_vector",
					"dims":       1536,
					"index":      true,
					"similarity": "cosine",
				},
				"q_1024_vec": map[string]interface{}{
					"type":       "dense_vector",
					"dims":       1024,
					"index":      true,
					"similarity": "cosine",
				},
				"q_768_vec": map[string]interface{}{
					"type":       "dense_vector",
					"dims":       768,
					"index":      true,
					"similarity": "cosine",
				},
				"q_512_vec": map[string]interface{}{
					"type":       "dense_vector",
					"dims":       512,
					"index":      true,
					"similarity": "cosine",
				},
				"q_256_vec": map[string]interface{}{
					"type":       "dense_vector",
					"dims":       256,
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

// calculatePagination calculates offset and limit based on page, size and topK
func calculatePagination(page, size, topK int) (int, int) {
	if page < 1 {
		page = 1
	}
	if size <= 0 {
		size = 30
	}
	if topK <= 0 {
		topK = 1024
	}

	RERANK_LIMIT := max(30, (64/size)*size)
	if RERANK_LIMIT < size {
		RERANK_LIMIT = size
	}
	if RERANK_LIMIT > topK {
		RERANK_LIMIT = topK
	}

	offset := (page - 1) * RERANK_LIMIT
	if offset < 0 {
		offset = 0
	}

	return offset, RERANK_LIMIT
}

// buildFilterClauses builds ES filter clauses from kb_ids and available_int
// Reference: rag/utils/es_conn.py L60-L78
// When available=0: available_int < 1
// When available!=0: NOT (available_int < 1)
func buildFilterClauses(datasetIDs []string, available int) []map[string]interface{} {
	var filters []map[string]interface{}

	if len(datasetIDs) > 0 {
		filters = append(filters, map[string]interface{}{
			"terms": map[string]interface{}{"kb_id": datasetIDs},
		})
	}

	// Add available_int filter
	// Reference: rag/utils/es_conn.py L63-L68
	if available == 0 {
		// available_int < 1
		filters = append(filters, map[string]interface{}{
			"range": map[string]interface{}{
				"available_int": map[string]interface{}{
					"lt": 1,
				},
			},
		})
	} else {
		// must_not: available_int < 1 (i.e., available_int >= 1)
		filters = append(filters, map[string]interface{}{
			"bool": map[string]interface{}{
				"must_not": []map[string]interface{}{
					{
						"range": map[string]interface{}{
							"available_int": map[string]interface{}{
								"lt": 1,
							},
						},
					},
				},
			},
		})
	}

	return filters
}

// buildSkillFilterClauses builds ES filter clauses for skill index
// Skill index uses 'status' field instead of 'available_int'
func buildSkillFilterClauses() []map[string]interface{} {
	// Filter for active skills (status = "1")
	return []map[string]interface{}{
		{
			"term": map[string]interface{}{
				"status": "1",
			},
		},
	}
}

// buildFilterFromMap converts a generic filter map to ES filter clauses
func buildFilterFromMap(filter map[string]interface{}) []map[string]interface{} {
	var filters []map[string]interface{}
	for field, value := range filter {
		switch v := value.(type) {
		case []string:
			filters = append(filters, map[string]interface{}{
				"terms": map[string]interface{}{field: v},
			})
		case []interface{}:
			filters = append(filters, map[string]interface{}{
				"terms": map[string]interface{}{field: v},
			})
		default:
			filters = append(filters, map[string]interface{}{
				"term": map[string]interface{}{field: v},
			})
		}
	}
	return filters
}

// buildESKeywordQuery builds keyword-only search query for ES
// Uses query_string if matchText is in query_string format, otherwise uses multi_match
// boost is applied to the text match clause (query_string or multi_match)
func buildESKeywordQuery(matchText string, filterClauses []map[string]interface{}, boost float64) map[string]interface{} {
	var mustClause map[string]interface{}

	// Handle wildcard query (match all)
	if matchText == "*" || matchText == "" {
		mustClause = map[string]interface{}{
			"match_all": map[string]interface{}{},
		}
	} else {
		// Use query_string for complex queries
		queryString := map[string]interface{}{
			"query":                matchText,
			"fields":               []string{"title_tks^10", "title_sm_tks^5", "important_kwd^30", "important_tks^20", "question_tks^20", "content_ltks^2", "content_sm_ltks"},
			"type":                 "best_fields",
			"minimum_should_match": "30%",
			"boost":                boost,
		}
		mustClause = map[string]interface{}{
			"query_string": queryString,
		}
	}

	return map[string]interface{}{
		"bool": map[string]interface{}{
			"must":   mustClause,
			"filter": filterClauses,
		},
	}
}

// buildSkillKeywordQuery builds keyword-only search query for skill index
// Skill index uses different field names: name_tks, tags_tks, description_tks, content_tks
func buildSkillKeywordQuery(matchText string, filterClauses []map[string]interface{}, boost float64) map[string]interface{} {
	var mustClause map[string]interface{}

	// Handle wildcard query (match all)
	if matchText == "*" || matchText == "" {
		mustClause = map[string]interface{}{
			"match_all": map[string]interface{}{},
		}
	} else {
		// Use query_string for complex queries with skill-specific fields
		queryString := map[string]interface{}{
			"query":                matchText,
			"fields":               []string{"name_tks^10", "tags_tks^5", "description_tks^3", "content_tks^1"},
			"type":                 "best_fields",
			"minimum_should_match": "30%",
			"boost":                boost,
		}
		mustClause = map[string]interface{}{
			"query_string": queryString,
		}
	}

	return map[string]interface{}{
		"bool": map[string]interface{}{
			"must":   mustClause,
			"filter": filterClauses,
		},
	}
}

// convertESResponse converts ES SearchResponse to unified chunks format
func convertESResponse(esResp *SearchResponse, vectorFieldName string) []map[string]interface{} {
	if esResp == nil || esResp.Hits.Hits == nil {
		return []map[string]interface{}{}
	}

	chunks := make([]map[string]interface{}, len(esResp.Hits.Hits))
	for i, hit := range esResp.Hits.Hits {
		chunks[i] = hit.Source
		chunks[i]["_score"] = hit.Score
		chunks[i]["_id"] = hit.ID
	}
	return chunks
}

// parseOrderByExpr parses the OrderBy expression into ES sort format
func parseOrderByExpr(orderBy *types.OrderByExpr) []map[string]interface{} {
	if orderBy == nil || len(orderBy.Fields) == 0 {
		return nil
	}

	var result []map[string]interface{}
	for _, field := range orderBy.Fields {
		direction := "asc"
		if field.Type == types.SortDesc {
			direction = "desc"
		}

		if field.Field == "_score" || field.Field == "score" {
			result = append(result, map[string]interface{}{
				"_score": direction,
			})
		} else {
			result = append(result, map[string]interface{}{
				field.Field: direction,
			})
		}
	}

	return result
}

// Helper query builder functions (legacy)

// BuildMatchTextQuery builds a text match query
func BuildMatchTextQuery(fields []string, text string, fuzziness string) map[string]interface{} {
	query := map[string]interface{}{
		"multi_match": map[string]interface{}{
			"query":  text,
			"fields": fields,
		},
	}

	if fuzziness != "" {
		if multiMatch, ok := query["multi_match"].(map[string]interface{}); ok {
			multiMatch["fuzziness"] = fuzziness
		}
	}

	return query
}

// BuildTermQuery builds a term query
func BuildTermQuery(field string, value interface{}) map[string]interface{} {
	return map[string]interface{}{
		"term": map[string]interface{}{
			field: value,
		},
	}
}

// BuildRangeQuery builds a range query
func BuildRangeQuery(field string, from, to interface{}) map[string]interface{} {
	rangeQuery := make(map[string]interface{})
	if from != nil {
		rangeQuery["gte"] = from
	}
	if to != nil {
		rangeQuery["lte"] = to
	}

	return map[string]interface{}{
		"range": map[string]interface{}{
			field: rangeQuery,
		},
	}
}

// BuildBoolQuery builds a bool query
func BuildBoolQuery() map[string]interface{} {
	return map[string]interface{}{
		"bool": make(map[string]interface{}),
	}
}

// AddMust adds must clause to bool query
func AddMust(query map[string]interface{}, clauses ...map[string]interface{}) {
	if boolQuery, ok := query["bool"].(map[string]interface{}); ok {
		if _, exists := boolQuery["must"]; !exists {
			boolQuery["must"] = []map[string]interface{}{}
		}
		if must, ok := boolQuery["must"].([]map[string]interface{}); ok {
			boolQuery["must"] = append(must, clauses...)
		}
	}
}

// AddShould adds should clause to bool query
func AddShould(query map[string]interface{}, clauses ...map[string]interface{}) {
	if boolQuery, ok := query["bool"].(map[string]interface{}); ok {
		if _, exists := boolQuery["should"]; !exists {
			boolQuery["should"] = []map[string]interface{}{}
		}
		if should, ok := boolQuery["should"].([]map[string]interface{}); ok {
			boolQuery["should"] = append(should, clauses...)
		}
	}
}

// AddFilter adds filter clause to bool query
func AddFilter(query map[string]interface{}, clauses ...map[string]interface{}) {
	if boolQuery, ok := query["bool"].(map[string]interface{}); ok {
		if _, exists := boolQuery["filter"]; !exists {
			boolQuery["filter"] = []map[string]interface{}{}
		}
		if filter, ok := boolQuery["filter"].([]map[string]interface{}); ok {
			boolQuery["filter"] = append(filter, clauses...)
		}
	}
}

// AddMustNot adds must_not clause to bool query
func AddMustNot(query map[string]interface{}, clauses ...map[string]interface{}) {
	if boolQuery, ok := query["bool"].(map[string]interface{}); ok {
		if _, exists := boolQuery["must_not"]; !exists {
			boolQuery["must_not"] = []map[string]interface{}{}
		}
		if mustNot, ok := boolQuery["must_not"].([]map[string]interface{}); ok {
			boolQuery["must_not"] = append(mustNot, clauses...)
		}
	}
}

// GetDocIDs is not implemented for Elasticsearch
func (e *elasticsearchEngine) GetDocIDs(chunks []map[string]interface{}) []string {
	common.Warn("GetDocIDs not implemented for Elasticsearch")
	return nil
}