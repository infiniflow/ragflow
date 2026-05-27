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
	"slices"
	"sort"
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

	// Check if index already exists (matches Python create_idx behavior)
	exists, err := e.indexExists(ctx, baseName)
	if err != nil {
		return fmt.Errorf("failed to check index existence: %w", err)
	}
	if exists {
		common.Info("Index already exists, skipping creation", zap.String("index_name", baseName))
		return nil
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

	common.Info("Successfully created Elasticsearch index", zap.String("index_name", baseName))
	return nil
}

// InsertChunks inserts chunks into a chunk index
// If a chunk with the same id + doc_id + kb_id already exists, it will be updated with the new value
func (e *elasticsearchEngine) InsertChunks(ctx context.Context, chunks []map[string]interface{}, baseName string, datasetID string) ([]string, error) {
	common.Info("ElasticsearchConnection.InsertChunks called", zap.String("index_name", baseName), zap.Int("chunkCount", len(chunks)))

	if len(chunks) == 0 {
		return []string{}, nil
	}

	if baseName == "" {
		return nil, fmt.Errorf("index name cannot be empty")
	}

	// Build bulk request body with index operations (upsert behavior: insert if not exists, update if exists)
	var buf bytes.Buffer
	for _, doc := range chunks {
		docID, _ := doc["doc_id"].(string)
		chunkID, _ := doc["id"].(string)
		if docID == "" || chunkID == "" {
			common.Warn("Skipping chunk without doc_id or id")
			continue
		}

		compositeID := fmt.Sprintf("%s_%s_%s", docID, datasetID, chunkID)

		// Action line: use json.Marshal to properly escape string values
		action, err := json.Marshal(map[string]interface{}{
			"index": map[string]interface{}{
				"_index": baseName,
				"_id":    compositeID,
			},
		})
		if err != nil {
			common.Error("Failed to marshal bulk action", err)
			return nil, fmt.Errorf("failed to marshal bulk action: %w", err)
		}
		buf.Write(action)
		buf.WriteByte('\n')

		// Document line: work with a copy to avoid mutating the original
		docCopy := copyFields(doc)
		docCopy["kb_id"] = datasetID
		if err := json.NewEncoder(&buf).Encode(docCopy); err != nil {
			return nil, fmt.Errorf("failed to encode document: %w", err)
		}
	}

	// Execute bulk request with refresh="wait_for" (matches Python behavior)
	req := esapi.BulkRequest{
		Body:    bytes.NewReader(buf.Bytes()),
		Refresh: "wait_for",
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

	// Parse bulk response
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

	common.Info("ElasticsearchConnection.InsertChunks result", zap.String("index_name", baseName), zap.Int("count", len(chunks)))
	return []string{}, nil
}

// UpdateChunks updates chunks by condition
func (e *elasticsearchEngine) UpdateChunks(ctx context.Context, condition map[string]interface{}, newValue map[string]interface{}, baseName string, datasetID string) error {
	fullIndexName := baseName
	common.Info("ElasticsearchConnection.UpdateChunks called", zap.String("index_name", fullIndexName), zap.Any("condition", condition), zap.Any("new_value", newValue))

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

	// Add kb_id to condition
	condition["kb_id"] = datasetID

	// Case 1: Single document update (when condition["id"] is a string)
	if chunkID, ok := condition["id"].(string); ok {
		return e.updateSingleChunk(ctx, fullIndexName, chunkID, newValue)
	}

	// Case 2: Multi-document update via UpdateByQuery
	return e.updateChunksByQuery(ctx, fullIndexName, condition, newValue)
}

// updateSingleChunk handles single document update (matches Python lines 350-398)
func (e *elasticsearchEngine) updateSingleChunk(ctx context.Context, indexName, chunkID string, newValue map[string]interface{}) error {
	common.Debug("ElasticsearchConnection.updateSingleChunk called", zap.String("indexName", indexName), zap.String("chunkID", chunkID))

	// First find the document by id field to get the actual _id
	searchReq := map[string]interface{}{
		"query": map[string]interface{}{
			"term": map[string]interface{}{"id": chunkID},
		},
	}

	body, err := json.Marshal(searchReq)
	if err != nil {
		return fmt.Errorf("failed to marshal search request: %w", err)
	}

	res, err := e.client.Search(
		e.client.Search.WithContext(ctx),
		e.client.Search.WithIndex(indexName),
		e.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return fmt.Errorf("failed to search for chunk: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return fmt.Errorf("failed to search for chunk: %s", res.Status())
	}

	var searchResult map[string]interface{}
	if err := json.NewDecoder(res.Body).Decode(&searchResult); err != nil {
		return fmt.Errorf("failed to parse search response: %w", err)
	}

	hits, ok := searchResult["hits"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("elasticsearch update error: 404 Not Found")
	}

	hitList, ok := hits["hits"].([]interface{})
	if !ok || len(hitList) == 0 {
		return fmt.Errorf("elasticsearch update error: 404 Not Found")
	}

	firstHit, ok := hitList[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("elasticsearch update error: 404 Not Found")
	}

	actualID, ok := firstHit["_id"].(string)
	if !ok {
		return fmt.Errorf("elasticsearch update error: 404 Not Found")
	}

	doc := copyFields(newValue)
	delete(doc, "id")

	removeValue, _ := doc["remove"]
	delete(doc, "remove")
	removeField, _ := removeValue.(string)
	removeDict, _ := removeValue.(map[string]interface{})

	// Remove *_feas fields
	var feasFields []string
	for k := range doc {
		if strings.HasSuffix(k, "feas") {
			feasFields = append(feasFields, k)
		}
	}
	for _, k := range feasFields {
		scriptBody := map[string]interface{}{
			"script": map[string]interface{}{
				"source": fmt.Sprintf("ctx._source.remove(\"%s\");", k),
			},
		}
		body, _ := json.Marshal(scriptBody)
		req := esapi.UpdateRequest{
			Index:      indexName,
			DocumentID: actualID,
			Body:       bytes.NewReader(body),
		}
		res, err := req.Do(ctx, e.client)
		if err != nil {
			common.Warn("Failed to remove feas field", zap.String("field", k), zap.Error(err))
		} else {
			res.Body.Close()
		}
	}

	// Remove specific field if removeField is set
	if removeField != "" {
		scriptBody := map[string]interface{}{
			"script": map[string]interface{}{
				"source": fmt.Sprintf("ctx._source.remove('%s');", removeField),
			},
		}
		body, _ := json.Marshal(scriptBody)
		req := esapi.UpdateRequest{
			Index:      indexName,
			DocumentID: actualID,
			Body:       bytes.NewReader(body),
		}
		res, err := req.Do(ctx, e.client)
		if err != nil {
			common.Warn("Failed to remove field", zap.String("field", removeField), zap.Error(err))
		} else {
			res.Body.Close()
		}
	}

	// Remove specific values from array fields (removeDict)
	if removeDict != nil {
		scripts := []string{}
		params := make(map[string]interface{})
		for kk, vv := range removeDict {
			scripts = append(scripts,
				fmt.Sprintf("if (ctx._source.containsKey('%s') && ctx._source.%s != null) { int i = ctx._source.%s.indexOf(params.p_%s); if (i >= 0) { ctx._source.%s.remove(i); }}",
					kk, kk, kk, kk, kk))
			params[fmt.Sprintf("p_%s", kk)] = vv
		}
		if scripts != nil {
			scriptBody := map[string]interface{}{
				"script": map[string]interface{}{
					"source": strings.Join(scripts, ""),
					"params": params,
				},
			}
			body, _ := json.Marshal(scriptBody)
			req := esapi.UpdateRequest{
				Index:      indexName,
				DocumentID: actualID,
				Body:       bytes.NewReader(body),
			}
			res, err := req.Do(ctx, e.client)
			if err != nil {
				common.Warn("Failed to remove dict fields", zap.Error(err))
			} else {
				res.Body.Close()
			}
		}
	}

	// Update document fields if any remain
	if len(doc) > 0 {
		updateBody := map[string]interface{}{"doc": doc}
		body, _ := json.Marshal(updateBody)
		req := esapi.UpdateRequest{
			Index:      indexName,
			DocumentID: actualID,
			Body:       bytes.NewReader(body),
		}
		res, err := req.Do(ctx, e.client)
		if err != nil {
			return fmt.Errorf("failed to update document: %w", err)
		}
		defer res.Body.Close()
		if res.IsError() {
			return fmt.Errorf("elasticsearch update error: %s", res.Status())
		}
	}

	common.Debug("ElasticsearchConnection.updateSingleChunk completed", zap.String("indexName", indexName), zap.String("chunkID", chunkID))
	return nil
}

// updateChunksByQuery handles multi-document update
func (e *elasticsearchEngine) updateChunksByQuery(ctx context.Context, indexName string, condition map[string]interface{}, newValue map[string]interface{}) error {
	common.Debug("ElasticsearchConnection.updateChunksByQuery called", zap.String("indexName", indexName))

	// Build bool query from condition
	var mustClauses []map[string]interface{}
	for k, v := range condition {
		if k == "exists" {
			mustClauses = append(mustClauses, map[string]interface{}{
				"exists": map[string]interface{}{"field": v},
			})
			continue
		}
		if v == nil || v == "" {
			continue
		}
		if listVal, ok := v.([]interface{}); ok {
			mustClauses = append(mustClauses, map[string]interface{}{
				"terms": map[string]interface{}{k: listVal},
			})
		} else if _, ok := v.(string); ok {
			mustClauses = append(mustClauses, map[string]interface{}{
				"term": map[string]interface{}{k: v},
			})
		} else if _, ok := v.(int); ok {
			mustClauses = append(mustClauses, map[string]interface{}{
				"term": map[string]interface{}{k: v},
			})
		}
	}

	boolQuery := map[string]interface{}{
		"bool": map[string]interface{}{
			"filter": mustClauses,
		},
	}

	// Build painless scripts from newValue
	var scripts []string
	params := make(map[string]interface{})

	for k, v := range newValue {
		if k == "remove" {
			if removeStr, ok := v.(string); ok {
				scripts = append(scripts, fmt.Sprintf("ctx._source.remove('%s');", removeStr))
				continue
			}
			if removeDict, ok := v.(map[string]interface{}); ok {
				for kk, vv := range removeDict {
					scripts = append(scripts,
						fmt.Sprintf("if (ctx._source.containsKey('%s') && ctx._source.%s != null) { int i = ctx._source.%s.indexOf(params.p_%s); if (i >= 0) { ctx._source.%s.remove(i); }}",
							kk, kk, kk, kk, kk))
					params[fmt.Sprintf("p_%s", kk)] = vv
				}
			}
			continue
		}
		if k == "add" {
			if addDict, ok := v.(map[string]interface{}); ok {
				for kk, vv := range addDict {
					vvStr, ok := vv.(string)
					if ok {
						vvStr = strings.TrimSpace(vvStr)
						scripts = append(scripts, fmt.Sprintf("ctx._source.%s.add(params.pp_%s);", kk, kk))
						params[fmt.Sprintf("pp_%s", kk)] = vvStr
					}
				}
			}
			continue
		}
		if (k == "" || v == nil) && k != "available_int" {
			continue
		}

		switch val := v.(type) {
		case string:
			// Sanitize: replace ' \n \r with space
			sanitized := sanitizeString(val)
			params[fmt.Sprintf("pp_%s", k)] = sanitized
			scripts = append(scripts, fmt.Sprintf("ctx._source.%s=params.pp_%s;", k, k))
		case int, float64:
			scripts = append(scripts, fmt.Sprintf("ctx._source.%s=%v;", k, val))
		case []interface{}:
			params[fmt.Sprintf("pp_%s", k)] = val
			scripts = append(scripts, fmt.Sprintf("ctx._source.%s=params.pp_%s;", k, k))
		}
	}

	scriptSource := strings.Join(scripts, "")

	// Build update by query body
	updateBody := map[string]interface{}{
		"query": boolQuery,
		"script": map[string]interface{}{
			"source": scriptSource,
			"params": params,
		},
	}

	bodyBytes, err := json.Marshal(updateBody)
	if err != nil {
		return fmt.Errorf("failed to marshal update body: %w", err)
	}

	// Execute update by query with refresh=true, slices=5, conflicts="proceed"
	refreshTrue := true
	req := esapi.UpdateByQueryRequest{
		Index:    []string{indexName},
		Body:     bytes.NewReader(bodyBytes),
		Refresh:  &refreshTrue,
		Slices:   5,
		Conflicts: "proceed",
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		common.Error("Failed to execute update by query", err)
		return fmt.Errorf("failed to execute update by query: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return fmt.Errorf("elasticsearch update by query error: %s, body: %s", res.Status(), string(bodyBytes))
	}

	common.Debug("ElasticsearchConnection.updateChunksByQuery completed", zap.String("indexName", indexName))
	return nil
}

// sanitizeString replaces ' \n \r with space
func sanitizeString(s string) string {
	s = strings.ReplaceAll(s, "'", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.TrimSpace(s)
}

// copyFields creates a shallow copy of a map
func copyFields(m map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range m {
		result[k] = v
	}
	return result
}

// DeleteChunks deletes chunks from a dataset index by condition
func (e *elasticsearchEngine) DeleteChunks(ctx context.Context, condition map[string]interface{}, indexName string, datasetID string) (int64, error) {
	// For ES, index name is just indexName (e.g., "ragflow_{tenantID}"), not indexName_datasetID
	fullIndexName := indexName
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

	// Build bool query from condition
	var mustClauses []map[string]interface{}
	var filterClauses []map[string]interface{}
	var mustNotClauses []map[string]interface{}

	// Handle chunk IDs - use terms query on "id" field instead of ids query on _id
	if idVal, ok := condition["id"]; ok && idVal != nil {
		switch v := idVal.(type) {
		case []interface{}:
			ids := make([]string, 0, len(v))
			for _, id := range v {
				if s, ok := id.(string); ok {
					ids = append(ids, s)
				}
			}
			if len(ids) > 0 {
				mustClauses = append(mustClauses, map[string]interface{}{
					"terms": map[string]interface{}{"id": ids},
				})
			}
		case string:
			mustClauses = append(mustClauses, map[string]interface{}{
				"term": map[string]interface{}{"id": v},
			})
		}
	}

	// Handle kb_id - add as term filter
	if kbID, ok := condition["kb_id"].(string); ok && kbID != "" {
		filterClauses = append(filterClauses, map[string]interface{}{
			"term": map[string]interface{}{"kb_id": kbID},
		})
	}

	// Add all other conditions as filters/must/must_not
	for k, v := range condition {
		if k == "id" || k == "kb_id" {
			continue // Already handled above
		}
		if k == "exists" {
			filterClauses = append(filterClauses, map[string]interface{}{
				"exists": map[string]interface{}{"field": v},
			})
		} else if k == "must_not" {
			if m, ok := v.(map[string]interface{}); ok {
				for kk, vv := range m {
					if kk == "exists" {
						mustNotClauses = append(mustNotClauses, map[string]interface{}{
							"exists": map[string]interface{}{"field": vv},
						})
					}
				}
			}
		} else if v != nil {
			if listVal, ok := v.([]interface{}); ok {
				mustClauses = append(mustClauses, map[string]interface{}{
					"terms": map[string]interface{}{k: listVal},
				})
			} else if _, ok := v.(string); ok {
				mustClauses = append(mustClauses, map[string]interface{}{
					"term": map[string]interface{}{k: v},
				})
			} else if _, ok := v.(int); ok {
				mustClauses = append(mustClauses, map[string]interface{}{
					"term": map[string]interface{}{k: v},
				})
			}
		}
	}

	// Build the query
	var qry map[string]interface{}
	if len(filterClauses) == 0 && len(mustClauses) == 0 && len(mustNotClauses) == 0 {
		qry = map[string]interface{}{"match_all": map[string]interface{}{}}
	} else {
		boolMap := map[string]interface{}{}
		if len(filterClauses) > 0 {
			boolMap["filter"] = filterClauses
		}
		if len(mustClauses) > 0 {
			boolMap["must"] = mustClauses
		}
		if len(mustNotClauses) > 0 {
			boolMap["must_not"] = mustNotClauses
		}
		qry = map[string]interface{}{"bool": boolMap}
	}

	// Build delete by query body
	deleteBody := map[string]interface{}{
		"query": qry,
	}

	bodyBytes, err := json.Marshal(deleteBody)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal delete body: %w", err)
	}

	// Execute delete by query with refresh=true
	refreshTrue := true
	req := esapi.DeleteByQueryRequest{
		Index:   []string{fullIndexName},
		Body:    bytes.NewReader(bodyBytes),
		Refresh: &refreshTrue,
	}

	res, err := req.Do(ctx, e.client)
	if err != nil {
		common.Error("Failed to execute delete by query", err)
		if strings.Contains(err.Error(), "not_found") {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to execute delete by query: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		errStr := string(bodyBytes)
		if strings.Contains(errStr, "not_found") {
			return 0, nil
		}
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
// Matches the behavior of Infinity's Search() method
func (e *elasticsearchEngine) searchUnified(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	common.Debug("Search in Elasticsearch started", zap.Any("indexNames", req.IndexNames))

	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	// Get retrieval parameters with defaults
	pageSize := req.Limit
	if pageSize <= 0 {
		pageSize = 30
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	isMetadataTable := false
	isSkillIndex := false
	for _, idx := range req.IndexNames {
		if strings.HasPrefix(idx, "ragflow_doc_meta_") {
			isMetadataTable = true
			break
		}
		if strings.HasPrefix(idx, "skill_") {
			isSkillIndex = true
			break
		}
	}

	var outputColumns []string
	if isMetadataTable {
		outputColumns = []string{"id", "kb_id", "meta_fields"}
	} else if isSkillIndex {
		outputColumns = []string{
			"skill_id", "space_id", "folder_id", "name", "tags", "description", "content",
			"version", "status", "create_time", "update_time",
		}
	} else {
		outputColumns = []string{
			"id", "doc_id", "kb_id", "content_ltks", "content_with_weight",
			"title_tks", "docnm_kwd", "img_id", "available_int", "important_kwd",
			"position_int", "page_num_int", "top_int", "chunk_order_int",
			"create_timestamp_flt", "knowledge_graph_kwd", "question_kwd", "question_tks",
			"doc_type_kwd", "mom_id", "tag_kwd", "pagerank_fea", "tag_feas",
		}
	}

	hasTextMatch := false
	hasVectorMatch := false
	var matchText *types.MatchTextExpr
	var matchDense *types.MatchDenseExpr
	if req.MatchExprs != nil && len(req.MatchExprs) > 0 {
		for _, expr := range req.MatchExprs {
			if expr == nil {
				continue
			}
			switch e := expr.(type) {
			case string:
				if e != "" {
					hasTextMatch = true
					matchText = &types.MatchTextExpr{
						MatchingText: e,
						TopN:         pageSize,
					}
				}
			case *types.MatchTextExpr:
				if e.MatchingText != "" {
					hasTextMatch = true
					matchText = e
				}
			case *types.MatchDenseExpr:
				if len(e.EmbeddingData) > 0 {
					hasVectorMatch = true
					matchDense = e
				}
			}
		}
	}

	// Extract FusionExpr if present (used for hybrid search fusion)
	var fusionExpr *types.FusionExpr
	if len(req.MatchExprs) > 2 {
		if fe, ok := req.MatchExprs[2].(*types.FusionExpr); ok {
			fusionExpr = fe
		}
	}
	_ = fusionExpr // TODO: implement fusion for ES hybrid search

	if hasTextMatch || hasVectorMatch {
		if !isSkillIndex {
			if !slices.Contains(outputColumns, common.PAGERANK_FLD) {
				outputColumns = append(outputColumns, common.PAGERANK_FLD)
			}
			if !slices.Contains(outputColumns, common.TAG_FLD) {
				outputColumns = append(outputColumns, common.TAG_FLD)
			}
		}
	}

	if hasVectorMatch && matchDense != nil && matchDense.VectorColumnName != "" {
		outputColumns = append(outputColumns, matchDense.VectorColumnName)
	}

	// Build filter string
	var filterParts []string

	if !isMetadataTable && (hasTextMatch || hasVectorMatch) {
		if req.Filter != nil {
			if availInt, ok := req.Filter["available_int"]; ok {
				filterParts = append(filterParts, fmt.Sprintf("available_int=%v", availInt))
			} else if status, ok := req.Filter["status"]; ok {
				filterParts = append(filterParts, fmt.Sprintf("status='%s'", status))
			} else {
				if isSkillIndex {
					filterParts = append(filterParts, "status='1'")
				} else {
					filterParts = append(filterParts, "available_int=1")
				}
			}
		} else {
			if isSkillIndex {
				filterParts = append(filterParts, "status='1'")
			} else {
				filterParts = append(filterParts, "available_int=1")
			}
		}
	}

	// Build filter string from req.Filter
	if req.Filter != nil {
		filterCopy := req.Filter
		if !isMetadataTable {
			filterCopy = make(map[string]interface{})
			for k, v := range req.Filter {
				if k != "kb_id" {
					filterCopy[k] = v
				}
			}
		}

		condStr := equivalentConditionToStr(filterCopy)
		if condStr != "" {
			filterParts = append(filterParts, condStr)
		}
	}
	filterStr := strings.Join(filterParts, " AND ")

	orderBy := req.OrderBy
	_ = orderBy // TODO: implement rank feature for ES

	var allResults []map[string]interface{}
	totalHits := int64(0)

	for _, indexName := range req.IndexNames {
		var indexNames []string
		if strings.HasPrefix(indexName, "ragflow_doc_meta_") {
			indexNames = []string{indexName}
		} else {
			indexNames = []string{indexName}
		}

		for _, fullIndexName := range indexNames {
			// Build search query body
			queryBody := make(map[string]interface{})

			// Determine text fields for the query (used indirectly via buildESKeywordQuery)
			if matchText != nil && len(matchText.Fields) > 0 {
				// Use matchText.Fields for text matching
			} else if isSkillIndex {
				// Use skill-specific fields in buildSkillKeywordQuery
			} else {
				// Use default fields in buildESKeywordQuery
			}

			var vectorFieldName string
			if !hasVectorMatch || matchDense == nil {
				// Keyword-only search (no vector match)
				queryBody["query"] = map[string]interface{}{
					"match_all": map[string]interface{}{},
				}
				if hasTextMatch && matchText != nil {
					if isSkillIndex {
						queryBody["query"] = buildSkillKeywordQuery(matchText.MatchingText, nil, 1.0)
					} else {
						queryBody["query"] = buildESKeywordQuery(matchText.MatchingText, nil, 1.0)
					}
					// Add filter if present
					if filterStr != "" {
						if boolQuery, ok := queryBody["query"].(map[string]interface{}); ok {
							if boolMap, ok := boolQuery["bool"].(map[string]interface{}); ok {
								filterClauses := buildFilterClausesFromStr(filterStr)
								if existingFilter, ok := boolMap["filter"].([]map[string]interface{}); ok {
									boolMap["filter"] = append(existingFilter, filterClauses...)
								} else {
									boolMap["filter"] = filterClauses
								}
							}
						}
					}
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
				matchingText := ""
				if matchText != nil {
					matchingText = matchText.MatchingText
				}
				if isSkillIndex {
					boolQuery = buildSkillKeywordQuery(matchingText, nil, textWeight)
				} else {
					boolQuery = buildESKeywordQuery(matchingText, nil, textWeight)
				}

				// Add filter to bool query
				if filterStr != "" {
					if boolMap, ok := boolQuery["bool"].(map[string]interface{}); ok {
						filterClauses := buildFilterClausesFromStr(filterStr)
						if existingFilter, ok := boolMap["filter"].([]map[string]interface{}); ok {
							boolMap["filter"] = append(existingFilter, filterClauses...)
						} else {
							boolMap["filter"] = filterClauses
						}
					}
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
					"query_vector":    vectorData,
					"k":               k,
					"num_candidates":  numCandidates,
					"similarity":      similarity,
					"boost":           vectorWeight,
				}

				queryBody["knn"] = knnQuery
				queryBody["query"] = boolQuery

				// Add vector column to output columns
				if vectorFieldName != "" {
					outputColumns = append(outputColumns, vectorFieldName)
				}
			}

			queryBody["size"] = pageSize
			queryBody["from"] = offset

			// Add sorting if specified
			if orderBy != nil && len(orderBy.Fields) > 0 {
				sort := parseOrderByExpr(orderBy)
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
			common.Debug("Elasticsearch searching index", zap.String("index", fullIndexName))
			common.Debug("Elasticsearch DSL", zap.Any("dsl", queryBody))

			// Build search request
			reqES := esapi.SearchRequest{
				Index: []string{fullIndexName},
				Body:  &buf,
			}

			// Execute search
			res, err := reqES.Do(ctx, e.client)
			if err != nil {
				common.Warn("Elasticsearch query failed", zap.String("index", fullIndexName), zap.Error(err))
				continue
			}

			if res.IsError() {
				bodyBytes, err := io.ReadAll(res.Body)
				res.Body.Close()
				if err != nil {
					common.Error("Elasticsearch failed to read error response body", err)
				} else {
					common.Warn("Elasticsearch error response", zap.String("index", fullIndexName), zap.String("body", string(bodyBytes)))
				}
				continue
			}

			// Parse response
			var esResp SearchResponse
			if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
				res.Body.Close()
				common.Warn("Elasticsearch failed to parse response", zap.String("index", fullIndexName), zap.Error(err))
				continue
			}

			res.Body.Close()

			// Convert to unified response
			searchChunks := convertESResponse(&esResp, vectorFieldName)
			totalHits += esResp.Hits.Total.Value

			// Apply field name mapping and row_id handling
			if !isSkillIndex {
				GetFields(searchChunks, nil)
			}

			allResults = append(allResults, searchChunks...)
		}
	}

	// Calculate scores and sort
	if hasTextMatch || hasVectorMatch {
		scoreColumn := "_score"
		if hasTextMatch && hasVectorMatch {
			scoreColumn = "SCORE"
		}

		pagerankField := common.PAGERANK_FLD
		if isSkillIndex {
			pagerankField = ""
		}

		allResults = calculateScores(allResults, scoreColumn, pagerankField)
		allResults = sortByScore(allResults, len(allResults))
	}

	// Limit results
	if len(allResults) > pageSize {
		allResults = allResults[:pageSize]
	}

	common.Debug("Search in Elasticsearch completed", zap.Int("returnedRows", len(allResults)), zap.Int64("totalHits", totalHits))

	return &types.SearchResult{
		Chunks: allResults,
		Total:  totalHits,
	}, nil
}

// buildFilterClausesFromStr converts a filter string to ES filter clauses
func buildFilterClausesFromStr(filterStr string) []map[string]interface{} {
	if filterStr == "" {
		return nil
	}
	return []map[string]interface{}{
		{"query_string": map[string]interface{}{
			"query": filterStr,
		}},
	}
}

// GetChunk gets a chunk by ID using ES search API
// _id in ES is composite: {doc_id}_{kb_id}_{chunk_id}
func (e *elasticsearchEngine) GetChunk(ctx context.Context, baseName, chunkID string, datasetIDs []string) (interface{}, error) {
	// Try search by doc_id field (which is stored in the document)
	for _, datasetID := range datasetIDs {
		searchReq := map[string]interface{}{
			"query": map[string]interface{}{
				"bool": map[string]interface{}{
					"must": []map[string]interface{}{
						{"term": map[string]interface{}{"id": chunkID}},
						{"term": map[string]interface{}{"kb_id": datasetID}},
					},
				},
			},
		}

		body, err := json.Marshal(searchReq)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal search request: %w", err)
		}

		res, err := e.client.Search(
			e.client.Search.WithContext(ctx),
			e.client.Search.WithIndex(baseName),
			e.client.Search.WithBody(bytes.NewReader(body)),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to search for chunk: %w", err)
		}

		if res.IsError() {
			res.Body.Close()
			return nil, fmt.Errorf("failed to search for chunk: %s", res.Status())
		}

		var searchResult map[string]interface{}
		if err := json.NewDecoder(res.Body).Decode(&searchResult); err != nil {
			res.Body.Close()
			return nil, fmt.Errorf("failed to parse search response: %w", err)
		}
		res.Body.Close()

		hits, ok := searchResult["hits"].(map[string]interface{})
		if !ok {
			continue
		}

		hitList, ok := hits["hits"].([]interface{})
		if !ok || len(hitList) == 0 {
			continue
		}

		firstHit, ok := hitList[0].(map[string]interface{})
		if !ok {
			continue
		}

		source, ok := firstHit["_source"].(map[string]interface{})
		if !ok {
			continue
		}

		common.Info("GetChunk found hit", zap.String("baseName", baseName), zap.String("chunkID", chunkID))
		source["id"] = chunkID
		return source, nil
	}

	common.Info("GetChunk no hits found", zap.String("baseName", baseName), zap.String("chunkID", chunkID))
	return nil, nil
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

// equivalentConditionToStr converts a condition map to a filter string (for ES query_string)
func equivalentConditionToStr(condition map[string]interface{}) string {
	if len(condition) == 0 {
		return ""
	}

	var cond []string

	for k, v := range condition {
		if k == "_id" {
			continue
		}
		if v == nil || v == "" {
			continue
		}

		// Handle list values
		if list, ok := v.([]interface{}); ok && len(list) > 0 {
			var items []string
			for _, item := range list {
				if s, ok := item.(string); ok {
					items = append(items, fmt.Sprintf("%s:'%s'", k, strings.ReplaceAll(s, "'", "\\'")))
				} else {
					items = append(items, fmt.Sprintf("%s:%v", k, item))
				}
			}
			if len(items) > 0 {
				cond = append(cond, "("+strings.Join(items, " OR ")+")")
			}
			continue
		}

		if list, ok := v.([]string); ok && len(list) > 0 {
			var items []string
			for _, item := range list {
				items = append(items, fmt.Sprintf("%s:'%s'", k, strings.ReplaceAll(item, "'", "\\'")))
			}
			if len(items) > 0 {
				cond = append(cond, "("+strings.Join(items, " OR ")+")")
			}
			continue
		}

		// Handle numeric values (no quotes)
		if isNumericValue(v) {
			cond = append(cond, fmt.Sprintf("%s:%v", k, v))
			continue
		}

		// Handle string values (with quotes and escaping)
		if str, ok := v.(string); ok {
			cond = append(cond, fmt.Sprintf("%s:'%s'", k, strings.ReplaceAll(str, "'", "\\'")))
			continue
		}

		// Fallback: treat as string
		cond = append(cond, fmt.Sprintf("%s:'%v'", k, v))
	}

	if len(cond) == 0 {
		return ""
	}
	return strings.Join(cond, " AND ")
}

// isNumericValue checks if a value is numeric
func isNumericValue(v interface{}) bool {
	switch v.(type) {
	case int, int8, int16, int32, int64:
		return true
	case uint, uint8, uint16, uint32, uint64:
		return true
	case float32, float64:
		return true
	}
	return false
}

// calculateScores calculates _score for chunks
func calculateScores(chunks []map[string]interface{}, scoreColumn, pagerankField string) []map[string]interface{} {
	for i := range chunks {
		score := 0.0
		if scoreVal, ok := chunks[i][scoreColumn]; ok {
			if f, ok := toFloat64(scoreVal); ok {
				score += f
			}
		}
		if pagerankField != "" {
			if prVal, ok := chunks[i][pagerankField]; ok {
				if f, ok := toFloat64(prVal); ok {
					score += f
				}
			}
		}
		chunks[i]["_score"] = score
	}
	return chunks
}

// toFloat64 converts a value to float64
func toFloat64(v interface{}) (float64, bool) {
	switch val := v.(type) {
	case float64:
		return val, true
	case float32:
		return float64(val), true
	case int:
		return float64(val), true
	case int64:
		return float64(val), true
	}
	return 0, false
}

// sortByScore sorts chunks by _score descending and limits
func sortByScore(chunks []map[string]interface{}, limit int) []map[string]interface{} {
	if len(chunks) == 0 {
		return chunks
	}

	// Sort by _score descending
	sort.Slice(chunks, func(i, j int) bool {
		scoreI := getChunkScore(chunks[i])
		scoreJ := getChunkScore(chunks[j])
		return scoreI > scoreJ
	})

	// Limit
	if len(chunks) > limit && limit > 0 {
		chunks = chunks[:limit]
	}

	return chunks
}

// getChunkScore extracts the score from a chunk
func getChunkScore(chunk map[string]interface{}) float64 {
	if v, ok := chunk["_score"].(float64); ok {
		return v
	}
	if v, ok := chunk["SCORE"].(float64); ok {
		return v
	}
	if v, ok := chunk["SIMILARITY"].(float64); ok {
		return v
	}
	return 0.0
}

// GetFields applies field mappings to chunks and returns a dict keyed by chunk ID.
// This mirrors the Infinity GetFields function behavior.
func GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	result := make(map[string]map[string]interface{})
	if len(chunks) == 0 {
		return result
	}

	// If fields is provided, create a set for lookup
	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[f] = true
	}

	for _, chunk := range chunks {
		// Apply field mappings
		// docnm -> docnm_kwd, title_tks, title_sm_tks
		if val, ok := chunk["docnm"].(string); ok {
			chunk["docnm_kwd"] = val
			chunk["title_tks"] = val
			chunk["title_sm_tks"] = val
		}

		// important_keywords -> important_kwd (split by comma), important_tks
		if val, ok := chunk["important_keywords"].(string); ok {
			if val == "" {
				chunk["important_kwd"] = []interface{}{}
			} else {
				parts := strings.Split(val, ",")
				chunk["important_kwd"] = parts
			}
			chunk["important_tks"] = val
		} else {
			chunk["important_kwd"] = []interface{}{}
			chunk["important_tks"] = []interface{}{}
		}

		// questions -> question_kwd (split by newline), question_tks
		if val, ok := chunk["questions"].(string); ok {
			if val == "" {
				chunk["question_kwd"] = []interface{}{}
			} else {
				parts := strings.Split(val, "\n")
				chunk["question_kwd"] = parts
			}
			chunk["question_tks"] = val
		} else {
			chunk["question_kwd"] = []interface{}{}
			chunk["question_tks"] = []interface{}{}
		}

		// content -> content_with_weight, content_ltks, content_sm_ltks
		if val, ok := chunk["content"].(string); ok {
			chunk["content_with_weight"] = val
			chunk["content_ltks"] = val
			chunk["content_sm_ltks"] = val
		}

		// authors -> authors_tks, authors_sm_tks
		if val, ok := chunk["authors"].(string); ok {
			chunk["authors_tks"] = val
			chunk["authors_sm_tks"] = val
		}

		// Build result map keyed by id
		if id, ok := chunk["id"].(string); ok {
			fieldMap := make(map[string]interface{})
			for field, value := range chunk {
				if len(fieldSet) == 0 || fieldSet[field] {
					fieldMap[field] = value
				}
			}
			result[id] = fieldMap
		}
	}

	return result
}