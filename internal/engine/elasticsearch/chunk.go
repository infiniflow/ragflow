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
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"github.com/json-iterator/go"
	"ragflow/internal/common"
	"ragflow/internal/engine/types"

	"go.uber.org/zap"
)

var jsonIterator = jsoniter.Config{
	SortMapKeys: false,
}.Froze()

var (
	elasticsearchHighlightEmTagRE     = regexp.MustCompile(`<em>[^<>]+</em>`)
	elasticsearchHighlightNewlineRE   = regexp.MustCompile(`[\r\n]`)
	elasticsearchHighlightDelimiterRE = regexp.MustCompile(`[.?!;\n]`)
	elasticsearchLetterRE             = regexp.MustCompile(`\pL`)
	elasticsearchEnglishLetterRE      = regexp.MustCompile(`[A-Za-z]`)
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

		// Action line: use json.Marshal to properly escape string values
		action, err := json.Marshal(map[string]interface{}{
			"index": map[string]interface{}{
				"_index": baseName,
				"_id":    chunkID,
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
		if err := jsonIterator.NewEncoder(&buf).Encode(docCopy); err != nil {
			return nil, fmt.Errorf("failed to encode document: %w", err)
		}
	}

	// Execute bulk request with refresh="wait_for"
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

// updateSingleChunk handles single document update
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
		return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
	}

	hitList, ok := hits["hits"].([]interface{})
	if !ok || len(hitList) == 0 {
		return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
	}

	firstHit, ok := hitList[0].(map[string]interface{})
	if !ok {
		return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
	}

	actualID, ok := firstHit["_id"].(string)
	if !ok {
		return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
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
			if res.StatusCode == http.StatusNotFound {
				return fmt.Errorf("%w: %s", types.ErrDocumentNotFound, chunkID)
			}
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
		Index:     []string{indexName},
		Body:      bytes.NewReader(bodyBytes),
		Refresh:   &refreshTrue,
		Slices:    5,
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
			ID        string                 `json:"_id"`
			Index     string                 `json:"_index"`
			Score     float64                `json:"_score"`
			Source    map[string]interface{} `json:"_source"`
			Fields    map[string]interface{} `json:"fields"` // ES 9.x stores dense_vector here
			Highlight map[string]interface{} `json:"highlight,omitempty"`
			// Sort is populated when the request body specifies a `sort`
			// clause. The last hit's Sort is the cursor for the next
			// search_after request — without it, deep pagination can't
			// advance.
			Sort []interface{} `json:"sort,omitempty"`
		} `json:"hits"`
	} `json:"hits"`
	Aggregations map[string]interface{} `json:"aggregations"`
}

// Search executes search with unified types.SearchRequest
func (e *elasticsearchEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	types.LogSearchRequest("Elasticsearch", req)

	// Validate inputs and set defaults
	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	limit := req.Limit
	if limit <= 0 {
		limit = 30
	}

	// Detect index types
	isSkillIndex := false
	for _, idx := range req.IndexNames {
		if strings.HasPrefix(idx, "skill_") {
			isSkillIndex = true
			break
		}
	}

	// Build bool query from condition
	boolQuery := buildBoolQueryFromCondition(req.Filter, req.KbIDs, isSkillIndex)

	// Extract vector_similarity_weight from FusionExpr
	var matchText *types.MatchTextExpr
	var matchDense *types.MatchDenseExpr
	vectorSimilarityWeight := 0.5
	for _, expr := range req.MatchExprs {
		if expr == nil {
			continue
		}
		switch m := expr.(type) {
		case *types.FusionExpr:
			if m.Method == "weighted_sum" {
				if weights, ok := m.FusionParams["weights"].(string); ok {
					// Assert structure only when FusionExpr has weighted_sum with weights
					if len(req.MatchExprs) != 3 {
						return nil, fmt.Errorf("match_expressions must have exactly 3 elements with FusionExpr, got %d", len(req.MatchExprs))
					}
					if _, ok := req.MatchExprs[0].(*types.MatchTextExpr); !ok {
						return nil, fmt.Errorf("match_expressions[0] must be MatchTextExpr")
					}
					if _, ok := req.MatchExprs[1].(*types.MatchDenseExpr); !ok {
						return nil, fmt.Errorf("match_expressions[1] must be MatchDenseExpr")
					}
					if _, ok := req.MatchExprs[2].(*types.FusionExpr); !ok {
						return nil, fmt.Errorf("match_expressions[2] must be FusionExpr")
					}
					parts := strings.Split(weights, ",")
					if len(parts) == 2 {
						if w, err := strconv.ParseFloat(parts[1], 64); err == nil {
							vectorSimilarityWeight = w
						}
					}
				}
			}
		case *types.MatchTextExpr:
			matchText = m
		case *types.MatchDenseExpr:
			matchDense = m
		}
	}

	// Build query body with text match and/or knn match
	queryBody := make(map[string]interface{})

	if matchText != nil {
		textQuery := buildQueryStringQuery(matchText, vectorSimilarityWeight, isSkillIndex)
		if boolQuery != nil {
			if boolMap, ok := boolQuery["bool"].(map[string]interface{}); ok {
				if must, ok := boolMap["must"].([]interface{}); ok {
					must = append(must, textQuery)
					boolMap["must"] = must
				} else {
					boolMap["must"] = []interface{}{textQuery}
				}
				boolMap["boost"] = 1.0 - vectorSimilarityWeight
			}
		} else {
			boolQuery = textQuery
		}
	}

	hasVectorMatch := matchDense != nil && len(matchDense.EmbeddingData) > 0
	if hasVectorMatch {
		k := matchDense.TopN
		if k <= 0 {
			k = limit
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

		vectorFieldName := matchDense.VectorColumnName

		knnQuery := map[string]interface{}{
			"field":          vectorFieldName,
			"query_vector":   matchDense.EmbeddingData,
			"k":              k,
			"num_candidates": numCandidates,
			"similarity":     similarity,
			"filter":         boolQuery,
		}

		queryBody["knn"] = knnQuery
		if boolQuery != nil {
			queryBody["query"] = boolQuery
		}
	} else if boolQuery != nil {
		queryBody["query"] = boolQuery
	} else {
		queryBody["query"] = map[string]interface{}{
			"match_all": map[string]interface{}{},
		}
	}

	// Add rank_feature queries
	if req.RankFeature != nil && len(req.RankFeature) > 0 && !isSkillIndex {
		rankFeatureQuery := buildRankFeatureQuery(req.RankFeature)
		if rankFeatureQuery != nil {
			if boolQuery, ok := queryBody["query"].(map[string]interface{}); ok {
				if boolMap, ok := boolQuery["bool"].(map[string]interface{}); ok {
					if should, ok := boolMap["should"].([]interface{}); ok {
						for _, q := range rankFeatureQuery {
							boolMap["should"] = append(should, q)
						}
					} else {
						interfaceSlice := make([]interface{}, len(rankFeatureQuery))
						for i, q := range rankFeatureQuery {
							interfaceSlice[i] = q
						}
						boolMap["should"] = interfaceSlice
					}
				}
			}
		}
	}

	// Add sorting if order_by specified
	if req.OrderBy != nil && len(req.OrderBy.Fields) > 0 {
		sort := parseOrderByExpr(req.OrderBy)
		if len(sort) > 0 {
			queryBody["sort"] = sort
		}
	}

	// Determine use_search_after for deep pagination
	//
	// ES rejects from + size combinations where from + size > index.max_result_window
	// (default 10,000) — see https://www.elastic.co/guide/en/elasticsearch/reference/current/paginate-search-results.html.
	// For those requests we must drop `from` and walk the result set with
	// `search_after` instead. The preconditions mirror the Python reference
	// (rag/utils/es_conn.py):
	//   - explicit OrderBy is required (search_after needs a stable cursor,
	//     and _score / KNN-similarity sorts are not unique enough to be safe)
	//   - no dense/KNN match (knn queries do not honour `search_after` in the
	//     same way and the Python path explicitly disallows them here)
	hasDense := hasVectorMatch
	hasExplicitSort := req.OrderBy != nil && len(req.OrderBy.Fields) > 0
	useSearchAfter := limit > 0 && (offset+limit > common.MAX_RESULT_WINDOW) && hasExplicitSort && !hasDense

	// Apply offset/limit pagination. When useSearchAfter is true, the
	// caller is going to drive pagination via searchAfterCursor()
	// instead, so we must NOT emit from/size here — leaving them out
	// is the whole point of routing to the search_after path.
	if !useSearchAfter && limit > 0 {
		queryBody["size"] = limit
		queryBody["from"] = offset
	}

	// Set _source and fields for vector fields
	hasTextMatch := matchText != nil
	if len(req.SelectFields) > 0 {
		// Use caller-specified fields, add pagerank_fld/tag_fld if needed
		queryBody["_source"] = req.SelectFields
		if hasTextMatch || hasVectorMatch {
			if !isSkillIndex {
				if !slices.Contains(req.SelectFields, common.PAGERANK_FLD) {
					queryBody["_source"] = append(queryBody["_source"].([]string), common.PAGERANK_FLD)
				}
				if !slices.Contains(req.SelectFields, common.TAG_FLD) {
					queryBody["_source"] = append(queryBody["_source"].([]string), common.TAG_FLD)
				}
			}
		}
		var vectorFields []string
		for _, f := range req.SelectFields {
			if strings.HasSuffix(f, "_vec") {
				vectorFields = append(vectorFields, f)
			}
		}
		if len(vectorFields) > 0 {
			queryBody["fields"] = vectorFields
		}
	} else {
		// No explicit SelectFields - use match_all, but add pagerank_fld/tag_fld for scoring if needed
		if hasTextMatch || hasVectorMatch {
			if !isSkillIndex {
				queryBody["_source"] = []string{common.PAGERANK_FLD, common.TAG_FLD}
			}
		}
	}

	// Serialize query
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(queryBody); err != nil {
		return nil, fmt.Errorf("error encoding query: %w", err)
	}

	// Execute search. When useSearchAfter is true we must NOT send
	// from/size (we dropped them above) and instead walk the result set
	// page-by-page with the search_after cursor — ES otherwise returns
	// the first page and the caller gets the wrong page.
	var (
		totalHits  int64
		allResults []map[string]interface{}
		err        error
	)

	if useSearchAfter {
		allResults, totalHits, err = e.searchAfterCursor(ctx, req, queryBody, offset, limit)
		if err != nil {
			return nil, err
		}
	} else {
		// WithBody takes an io.Reader that the Go client streams
		// directly into the request. Reusing &buf across iterations
		// would drain it on the first request and leave the rest
		// with an empty body — so we copy the bytes once and hand
		// each iteration a fresh bytes.NewReader.
		payload := append([]byte(nil), buf.Bytes()...)
		for _, indexName := range req.IndexNames {
			res, err := e.client.Search(
				e.client.Search.WithContext(ctx),
				e.client.Search.WithIndex(indexName),
				e.client.Search.WithBody(bytes.NewReader(payload)),
				e.client.Search.WithTrackTotalHits(true),
			)
			if err != nil {
				common.Warn("Elasticsearch query failed", zap.String("index", indexName), zap.Error(err))
				continue
			}
			defer res.Body.Close()

			if res.IsError() {
				bodyBytes, _ := io.ReadAll(res.Body)
				common.Warn("Elasticsearch error response", zap.String("index", indexName), zap.String("body", string(bodyBytes)))
				continue
			}

			// Parse response and return results
			var esResp SearchResponse
			if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
				common.Warn("Elasticsearch failed to parse response", zap.String("index", indexName), zap.Error(err))
				continue
			}

			searchChunks := convertESResponse(&esResp, "")
			totalHits += esResp.Hits.Total.Value

			allResults = append(allResults, searchChunks...)
		}
	}

	// Post-processing: Sort results by score
	if len(allResults) > 0 && (matchText != nil || hasVectorMatch) {
		scoreColumn := "_score"
		if matchText != nil && hasVectorMatch {
			scoreColumn = "SCORE"
		}

		pagerankField := common.PAGERANK_FLD
		if isSkillIndex {
			pagerankField = ""
		}

		allResults = calculateScores(allResults, scoreColumn, pagerankField)
		allResults = sortByScore(allResults, limit)
	}

	common.Info("ES Search completed", zap.Int("returnedRows", len(allResults)), zap.Int64("totalHits", totalHits))

	return &types.SearchResult{
		Chunks: allResults,
		Total:  totalHits,
	}, nil
}

// searchAfterFetcher issues one ES search request with the given batch
// size and search_after cursor, returning the decoded response. Defined
// as a function type so the pagination logic below can be unit-tested
// with a mock fetcher instead of a real Elasticsearch client.
type searchAfterFetcher func(
	ctx context.Context,
	baseQuery map[string]interface{},
	batch int,
	cursor []interface{},
	trackTotalHits bool,
) (SearchResponse, error)

// searchAfterCursor walks ES with the search_after pagination protocol,
// returning the page [offset, offset+limit) of an explicitly-sorted
// result set. Used when offset+limit exceeds common.MAX_RESULT_WINDOW
// and ES would otherwise reject the from/size combination.
//
// Mirrors rag/utils/es_conn.py:ESConnection._search_with_search_after:
//
//  1. Drop from/size from the base query (the caller has already omitted
//     them on this path; this is a defensive no-op).
//  2. Skip phase: discard hits until we have skipped `offset` of them.
//  3. Take phase: collect hits until we have `limit` of them, or the
//     index is exhausted.
//  4. After each batch, advance the cursor with the last hit's `sort`
//     field. If `sort` is missing or unchanged, the index is exhausted.
//
// The first request carries trackTotalHits=true so the caller still
// gets an accurate total; subsequent requests skip it for efficiency.
// Returns the (possibly empty) collected hits and the total hit count
// from the first response.
func (e *elasticsearchEngine) searchAfterCursor(
	ctx context.Context,
	req *types.SearchRequest,
	baseQuery map[string]interface{},
	offset, limit int,
) ([]map[string]interface{}, int64, error) {
	// Defensive: strip from/size if the caller left them in. In the
	// current code path they are never set when useSearchAfter is true,
	// but the base query is a shared map and future callers may forget.
	delete(baseQuery, "from")
	delete(baseQuery, "size")

	return searchAfterPaginate(ctx, baseQuery, offset, limit, e.buildSearchAfterFetcher(req))
}

// buildSearchAfterFetcher returns a fetcher that delegates each
// iteration to executeSearchRequest, which talks to the real ES client.
func (e *elasticsearchEngine) buildSearchAfterFetcher(req *types.SearchRequest) searchAfterFetcher {
	return func(
		ctx context.Context,
		baseQuery map[string]interface{},
		batch int,
		cursor []interface{},
		trackTotalHits bool,
	) (SearchResponse, error) {
		return e.executeSearchRequest(ctx, req, baseQuery, batch, cursor, trackTotalHits)
	}
}

// searchAfterPaginate is the pure, callback-driven pagination loop
// shared by the engine and the unit tests. See searchAfterCursor for
// the semantics.
func searchAfterPaginate(
	ctx context.Context,
	baseQuery map[string]interface{},
	offset, limit int,
	fetch searchAfterFetcher,
) ([]map[string]interface{}, int64, error) {
	var (
		cursor        []interface{}
		totalHits     int64
		collected     []map[string]interface{}
		collectedTake int
		firstCall     = true
	)

	// Skip phase: walk past `offset` hits without retaining them.
	remainingSkip := offset
	for remainingSkip > 0 {
		batch := remainingSkip
		if batch > common.SearchAfterBatchSize {
			batch = common.SearchAfterBatchSize
		}

		resp, err := fetch(ctx, baseQuery, batch, cursor, firstCall)
		firstCall = false
		if err != nil {
			return nil, 0, err
		}
		if totalHits == 0 {
			totalHits = resp.Hits.Total.Value
		}
		if len(resp.Hits.Hits) == 0 {
			break
		}
		nextCursor := resp.Hits.Hits[len(resp.Hits.Hits)-1].Sort
		if len(nextCursor) == 0 || sortValuesEqual(nextCursor, cursor) {
			// ES returned hits but no usable cursor (e.g. sort field
			// missing or unchanged). The index is exhausted from our
			// point of view.
			break
		}
		cursor = nextCursor
		remainingSkip -= len(resp.Hits.Hits)
		if len(resp.Hits.Hits) < batch {
			// Short batch — we asked for more than was available, so
			// the cursor is at the end of the index.
			break
		}
	}

	// Take phase: collect up to `limit` hits. ES may return up to
	// `batch` hits per request, but we stop at `limit` (the absolute
	// target) regardless of how many we asked for in this iteration.
	for collectedTake < limit {
		want := limit - collectedTake
		batch := want
		if batch > common.SearchAfterBatchSize {
			batch = common.SearchAfterBatchSize
		}

		resp, err := fetch(ctx, baseQuery, batch, cursor, firstCall)
		firstCall = false
		if err != nil {
			return nil, 0, err
		}
		if totalHits == 0 {
			totalHits = resp.Hits.Total.Value
		}
		if len(resp.Hits.Hits) == 0 {
			break
		}

		// Convert and append. We could parallelize the conversion with
		// the next request, but conversion is cheap relative to the
		// ES round-trip, so keep the loop straightforward.
		for _, hit := range resp.Hits.Hits {
			if collectedTake >= limit {
				break
			}
			chunk := hit.Source
			if chunk == nil {
				chunk = map[string]interface{}{}
			}
			chunk["_score"] = hit.Score
			chunk["_id"] = hit.ID
			chunk["_index"] = hit.Index
			collected = append(collected, chunk)
			collectedTake++
		}

		// Reached the absolute limit — stop without advancing the
		// cursor (we already have what was asked for).
		if collectedTake >= limit {
			break
		}

		nextCursor := resp.Hits.Hits[len(resp.Hits.Hits)-1].Sort
		if len(nextCursor) == 0 || sortValuesEqual(nextCursor, cursor) {
			break
		}
		cursor = nextCursor
		if len(resp.Hits.Hits) < batch {
			break
		}
	}

	// If we never sent a request (e.g. offset == 0 and limit == 0) we
	// still need a total. Issue one count-only request.
	if totalHits == 0 {
		resp, err := fetch(ctx, baseQuery, 0, nil, true)
		if err != nil {
			return nil, 0, err
		}
		totalHits = resp.Hits.Total.Value
	}

	return collected, totalHits, nil
}

// executeSearchRequest sends one ES search request with the given
// batch size and search_after cursor. If trackTotalHits is true the
// request asks ES to compute an exact total (cheap to omit on
// pagination iterations after the first).
func (e *elasticsearchEngine) executeSearchRequest(
	ctx context.Context,
	req *types.SearchRequest,
	baseQuery map[string]interface{},
	batch int,
	cursor []interface{},
	trackTotalHits bool,
) (SearchResponse, error) {
	queryBody := make(map[string]interface{}, len(baseQuery)+2)
	for k, v := range baseQuery {
		queryBody[k] = v
	}
	if batch > 0 {
		queryBody["size"] = batch
	}
	if len(cursor) > 0 {
		queryBody["search_after"] = cursor
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(queryBody); err != nil {
		return SearchResponse{}, fmt.Errorf("error encoding query: %w", err)
	}

	res, err := e.client.Search(
		e.client.Search.WithContext(ctx),
		e.client.Search.WithIndex(req.IndexNames...),
		e.client.Search.WithBody(&buf),
		e.client.Search.WithTrackTotalHits(trackTotalHits),
	)
	if err != nil {
		return SearchResponse{}, fmt.Errorf("elasticsearch search failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return SearchResponse{}, fmt.Errorf("elasticsearch error response: %s", string(bodyBytes))
	}

	var esResp SearchResponse
	if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
		return SearchResponse{}, fmt.Errorf("elasticsearch failed to parse response: %w", err)
	}
	return esResp, nil
}

// sortValuesEqual reports whether two sort cursors are identical.
// ES guarantees that successive requests with `search_after: <cursor>`
// advance strictly past the cursor, so an unchanged cursor between
// iterations means the index is exhausted.
func sortValuesEqual(a, b []interface{}) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// buildBoolQueryFromCondition builds an ES bool query from condition map
// For skill index, uses 'status' field instead of 'available_int'
func buildBoolQueryFromCondition(filter map[string]interface{}, kbIDs []string, isSkillIndex bool) map[string]interface{} {
	var mustClauses []interface{}
	var filterClauses []interface{}
	var shouldClauses []interface{}

	// Add kb_id to condition
	if kbIDs != nil && len(kbIDs) > 0 {
		filterClauses = append(filterClauses, map[string]interface{}{
			"terms": map[string]interface{}{"kb_id": kbIDs},
		})
	}

	// For skill index, add status = "1" filter by default (active skills)
	if isSkillIndex {
		filterClauses = append(filterClauses, map[string]interface{}{
			"term": map[string]interface{}{
				"status": "1",
			},
		})
	}

	if filter == nil {
		filter = make(map[string]interface{})
	}

	for k, v := range filter {
		// For skill index, handle 'status' field instead of 'available_int'
		if isSkillIndex && k == "status" {
			if v == nil || v == "" {
				continue
			}
			if listVal, ok := v.([]interface{}); ok && len(listVal) > 0 {
				filterClauses = append(filterClauses, map[string]interface{}{
					"terms": map[string]interface{}{"status": listVal},
				})
			} else if strVal, ok := v.(string); ok && strVal != "" {
				filterClauses = append(filterClauses, map[string]interface{}{
					"term": map[string]interface{}{"status": strVal},
				})
			}
			continue
		}
		if k == "available_int" {
			var numVal float64
			switch val := v.(type) {
			case float64:
				numVal = val
			case int:
				numVal = float64(val)
			case int64:
				numVal = float64(val)
			default:
				continue
			}
			if numVal == 0 {
				filterClauses = append(filterClauses, map[string]interface{}{
					"range": map[string]interface{}{"available_int": map[string]interface{}{"lt": 1}},
				})
			} else {
				filterClauses = append(filterClauses, map[string]interface{}{
					"bool": map[string]interface{}{
						"must_not": []map[string]interface{}{
							{"range": map[string]interface{}{"available_int": map[string]interface{}{"lt": 1}}},
						},
					},
				})
			}
			continue
		}
		if k == "id" {
			if v == nil || v == "" {
				continue
			}
			if listVal, ok := v.([]interface{}); ok && len(listVal) > 0 {
				shouldClauses = append(shouldClauses,
					map[string]interface{}{"terms": map[string]interface{}{"id": listVal}},
					map[string]interface{}{"terms": map[string]interface{}{"_id": listVal}},
				)
			} else if strVal, ok := v.(string); ok && strVal != "" {
				shouldClauses = append(shouldClauses,
					map[string]interface{}{"term": map[string]interface{}{"id": strVal}},
					map[string]interface{}{"term": map[string]interface{}{"_id": strVal}},
				)
			} else if intVal, ok := v.(int); ok && intVal != 0 {
				shouldClauses = append(shouldClauses,
					map[string]interface{}{"term": map[string]interface{}{"id": intVal}},
					map[string]interface{}{"term": map[string]interface{}{"_id": intVal}},
				)
			}
			continue
		}
		if v == nil || v == "" {
			continue
		}
		if listVal, ok := v.([]interface{}); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"terms": map[string]interface{}{k: listVal},
			})
		} else if strListVal, ok := v.([]string); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"terms": map[string]interface{}{k: strListVal},
			})
		} else if strVal, ok := v.(string); ok && strVal != "" {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{k: strVal},
			})
		} else if intVal, ok := v.(int); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{k: intVal},
			})
		} else if floatVal, ok := v.(float64); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{k: floatVal},
			})
		}
	}

	// Build the bool query
	boolQuery := make(map[string]interface{})
	if len(mustClauses) > 0 {
		boolQuery["must"] = mustClauses
	}
	if len(filterClauses) > 0 {
		boolQuery["filter"] = filterClauses
	}
	if len(shouldClauses) > 0 {
		boolQuery["should"] = shouldClauses
		boolQuery["minimum_should_match"] = 1
	}

	if len(boolQuery) == 0 {
		return nil
	}

	return map[string]interface{}{"bool": boolQuery}
}

// buildQueryStringQuery builds a query_string query from MatchTextExpr
// When isSkillIndex is true, uses skill-specific fields (name_tks, tags_tks, etc.)
// Otherwise uses document fields (title_tks, content_ltks, etc.)
func buildQueryStringQuery(matchText *types.MatchTextExpr, vectorSimilarityWeight float64, isSkillIndex bool) map[string]interface{} {
	if matchText == nil {
		return nil
	}

	minimumShouldMatch := "0%"
	if matchText.ExtraOptions != nil {
		if msm, ok := matchText.ExtraOptions["minimum_should_match"].(float64); ok {
			minimumShouldMatch = fmt.Sprintf("%d%%", int(msm*100))
		}
	}

	fields := matchText.Fields
	if fields == nil || len(fields) == 0 {
		if isSkillIndex {
			fields = []string{"name_tks^10", "tags_tks^5", "description_tks^3", "content_tks^1"}
		} else {
			fields = []string{"title_tks^10", "title_sm_tks^5", "important_kwd^30", "important_tks^20", "question_tks^20", "content_ltks^2", "content_sm_ltks"}
		}
	}

	boost := 1.0
	if matchText.ExtraOptions != nil {
		if b, ok := matchText.ExtraOptions["boost"].(float64); ok {
			boost = b
		}
	}

	return map[string]interface{}{
		"query_string": map[string]interface{}{
			"fields":               fields,
			"type":                 "best_fields",
			"query":                matchText.MatchingText,
			"minimum_should_match": minimumShouldMatch,
			"boost":                boost,
		},
	}
}

// buildRankFeatureQuery builds rank_feature queries for learning to rank
func buildRankFeatureQuery(rankFeature map[string]float64) []map[string]interface{} {
	if rankFeature == nil || len(rankFeature) == 0 {
		return nil
	}

	// Sort keys for deterministic query order (Go map iteration is randomized)
	keys := make([]string, 0, len(rankFeature))
	for k := range rankFeature {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queries []map[string]interface{}
	for _, fld := range keys {
		if fld == common.PAGERANK_FLD {
			continue
		}
		sc := rankFeature[fld]
		tagField := fmt.Sprintf("%s.%s", common.TAG_FLD, fld)
		queries = append(queries, map[string]interface{}{
			"rank_feature": map[string]interface{}{
				"field":  tagField,
				"linear": map[string]interface{}{},
				"boost":  sc,
			},
		})
	}
	return queries
}

// GetChunk gets a chunk by ID using ES search API
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

// GetFields extracts the requested fields from ES search response chunks
//
// Unlike Infinity, Elasticsearch does NOT use convertSelectFields before querying.
// The original requested field names ARE the database column names:
//   - "content_with_weight" is stored and returned as "content_with_weight"
//   - No field name mapping is needed in GetFields
func (e *elasticsearchEngine) GetFields(chunks []map[string]interface{}, fields []string) map[string]map[string]interface{} {
	common.Info("GetFields called", zap.Int("chunkCount", len(chunks)), zap.Strings("fields", fields))
	result := make(map[string]map[string]interface{})

	if len(fields) == 0 || len(chunks) == 0 {
		return result
	}

	// Build field set for lookup
	fieldSet := make(map[string]bool)
	for _, f := range fields {
		fieldSet[f] = true
	}

	for _, chunk := range chunks {
		docID, ok := elasticsearchChunkID(chunk)
		if !ok {
			continue
		}
		if id, ok := chunk["id"].(string); !ok || id == "" {
			chunk["id"] = docID
		}

		m := make(map[string]interface{})
		for field := range fieldSet {
			val := chunk[field]

			if val == nil {
				continue
			}

			if listVal, ok := val.([]interface{}); ok {
				if len(listVal) == 1 {
					if _, isArray := listVal[0].([]interface{}); !isArray {
						val = listVal[0]
					}
				}
			}

			if _, ok := val.([]interface{}); ok {
				m[field] = val
				continue
			}

			if field == "available_int" {
				if _, ok := val.(int); ok {
					m[field] = val
					continue
				}
				if _, ok := val.(float64); ok {
					m[field] = val
					continue
				}
			}

			if _, ok := val.(string); !ok {
				val = fmt.Sprintf("%v", val)
			}
			m[field] = val
		}

		if len(m) > 0 {
			result[docID] = m
		}
	}

	common.Info("GetFields result", zap.Int("resultCount", len(result)), zap.Strings("keys", func() []string {
		keys := make([]string, 0, len(result))
		for k := range result {
			keys = append(keys, k)
		}
		return keys
	}()))
	return result
}

// GetAggregation aggregates chunk values by field name
// Input: [{"docnm_kwd": "docA"}, {"docnm_kwd": "docA"}, {"docnm_kwd": "docB"}]
// Returns: [{"key": "docA", "count": 2}, {"key": "docB", "count": 1}]
func (e *elasticsearchEngine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	if len(chunks) == 0 || fieldName == "" {
		return []map[string]interface{}{}
	}

	tagCounts := make(map[string]int)
	for _, chunk := range chunks {
		value, ok := chunk[fieldName]
		if !ok || value == nil {
			continue
		}

		if valueStr, ok := value.(string); ok {
			if valueStr == "" {
				continue
			}
			separator := ","
			if fieldName == "tag_kwd" && strings.Contains(valueStr, "###") {
				separator = "###"
			}
			for _, tag := range strings.Split(valueStr, separator) {
				countElasticsearchAggregationTag(tagCounts, tag)
			}
			continue
		}

		if valueList, ok := value.([]interface{}); ok {
			for _, item := range valueList {
				if itemStr, ok := item.(string); ok {
					countElasticsearchAggregationTag(tagCounts, itemStr)
				}
			}
		}
	}

	if len(tagCounts) == 0 {
		return []map[string]interface{}{}
	}

	tags := make([]string, 0, len(tagCounts))
	for tag := range tagCounts {
		tags = append(tags, tag)
	}
	slices.SortFunc(tags, func(a, b string) int {
		if byCount := cmp.Compare(tagCounts[b], tagCounts[a]); byCount != 0 {
			return byCount
		}
		return cmp.Compare(a, b)
	})

	result := make([]map[string]interface{}, len(tags))
	for i, tag := range tags {
		result[i] = map[string]interface{}{"key": tag, "count": tagCounts[tag]}
	}

	return result
}

// GetChunkIDs extracts chunk IDs from ES search response chunks.
// Uses _id field (composite: {doc_id}_{kb_id}_{chunk_id}).
func (e *elasticsearchEngine) GetChunkIDs(chunks []map[string]interface{}) []string {
	ids := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if id, ok := elasticsearchChunkID(chunk); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// GetHighlight returns highlighted text for matching keywords
func (e *elasticsearchEngine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	result := make(map[string]string)
	if len(chunks) == 0 || len(keywords) == 0 {
		return result
	}

	normalizedKeywords := normalizeElasticsearchHighlightKeywords(keywords)
	englishPatterns := compileElasticsearchHighlightPatterns(normalizedKeywords)
	nonEnglishPattern := compileElasticsearchNonEnglishHighlightPattern(normalizedKeywords)

	for _, chunk := range chunks {
		docID, ok := elasticsearchChunkID(chunk)
		if !ok {
			continue
		}

		if highlightText := firstElasticsearchHighlight(chunk); highlightText != "" {
			result[docID] = highlightText
			continue
		}

		txt, ok := chunk[fieldName].(string)
		if fieldName == "content_with_weight" && (!ok || txt == "") {
			txt, ok = chunk["content"].(string)
		}
		if !ok || txt == "" {
			continue
		}

		if elasticsearchHighlightEmTagRE.MatchString(txt) {
			result[docID] = txt
			continue
		}

		txt = elasticsearchHighlightNewlineRE.ReplaceAllString(txt, " ")
		segments := elasticsearchHighlightDelimiterRE.Split(txt, -1)

		var highlightedSegments []string
		for _, segment := range segments {
			segmentToCheck := segment
			if isMostlyEnglishElasticsearchSegment(segment) {
				for _, pattern := range englishPatterns {
					segmentToCheck = pattern.ReplaceAllString(segmentToCheck, "$1<em>$2</em>$3")
				}
			} else if nonEnglishPattern != nil {
				segmentToCheck = nonEnglishPattern.ReplaceAllStringFunc(segmentToCheck, func(match string) string {
					return "<em>" + match + "</em>"
				})
			}
			if segmentToCheck != segment {
				highlightedSegments = append(highlightedSegments, strings.TrimSpace(segmentToCheck))
			}
		}

		if len(highlightedSegments) > 0 {
			result[docID] = strings.Join(highlightedSegments, "... ")
		}
	}
	return result
}

func elasticsearchChunkID(chunk map[string]interface{}) (string, bool) {
	if id, ok := chunk["id"].(string); ok && id != "" {
		return id, true
	}
	if id, ok := chunk["_id"].(string); ok && id != "" {
		return id, true
	}
	return "", false
}

func firstElasticsearchHighlight(chunk map[string]interface{}) string {
	highlight, ok := chunk["highlight"].(map[string]interface{})
	if !ok || len(highlight) == 0 {
		return ""
	}

	for _, vals := range highlight {
		if arr, ok := vals.([]interface{}); ok && len(arr) > 0 {
			if str, ok := arr[0].(string); ok {
				return str
			}
		}
	}
	return ""
}

func countElasticsearchAggregationTag(counts map[string]int, tag string) {
	if tag = strings.TrimSpace(tag); tag != "" {
		counts[tag]++
	}
}

func isMostlyEnglishElasticsearchSegment(segment string) bool {
	totalCount := len(elasticsearchLetterRE.FindAllString(segment, -1))
	return totalCount > 0 && float64(len(elasticsearchEnglishLetterRE.FindAllString(segment, -1)))/float64(totalCount) > 0.5
}

func compileElasticsearchHighlightPatterns(keywords []string) []*regexp.Regexp {
	patterns := make([]*regexp.Regexp, 0, len(keywords))
	for _, kw := range keywords {
		patterns = append(patterns, regexp.MustCompile(`(?i)(^|[ .?/'\"\(\)!,:;-])(`+regexp.QuoteMeta(kw)+`)([ .?/'\"\(\)!,:;-]|$)`))
	}
	return patterns
}

func compileElasticsearchNonEnglishHighlightPattern(keywords []string) *regexp.Regexp {
	if len(keywords) == 0 {
		return nil
	}
	parts := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		parts = append(parts, regexp.QuoteMeta(kw))
	}
	return regexp.MustCompile(strings.Join(parts, "|"))
}

func normalizeElasticsearchHighlightKeywords(keywords []string) []string {
	seen := make(map[string]struct{}, len(keywords))
	normalized := make([]string, 0, len(keywords))
	for _, kw := range keywords {
		if kw == "" {
			continue
		}
		if _, ok := seen[kw]; !ok {
			seen[kw] = struct{}{}
			normalized = append(normalized, kw)
		}
	}
	slices.SortStableFunc(normalized, func(a, b string) int {
		return cmp.Compare(len(b), len(a))
	})
	return normalized
}

// DropChunkStore deletes a chunk index
func (e *elasticsearchEngine) DropChunkStore(ctx context.Context, baseName, datasetID string) error {
	return e.dropIndex(ctx, baseName)
}

// ChunkStoreExists checks if a chunk index exists
func (e *elasticsearchEngine) ChunkStoreExists(ctx context.Context, baseName, datasetID string) (bool, error) {
	return e.indexExists(ctx, baseName)
}

// KNNScores performs a second-pass KNN search to get clean cosine similarities for ES.
// This keeps chunk vectors in the index and asks ES to compute the cosine similarity.
func (e *elasticsearchEngine) KNNScores(ctx context.Context, chunks []map[string]interface{}, queryVector []float64, topK int) (map[string]interface{}, error) {
	if len(chunks) == 0 || len(queryVector) == 0 {
		return nil, nil
	}

	// Extract chunk IDs from first search results
	chunkIDs := make([]string, 0, len(chunks))
	for _, chunk := range chunks {
		if id, ok := chunk["_id"].(string); ok {
			chunkIDs = append(chunkIDs, id)
		}
	}
	if len(chunkIDs) == 0 {
		return nil, nil
	}

	common.Info("KNNScores starting", zap.Int("chunkCount", len(chunkIDs)), zap.Strings("chunkIDs", chunkIDs), zap.Int("vectorSize", len(queryVector)))

	// Build KNN-only query filtered by chunk IDs
	vectorSize := len(queryVector)
	k := len(chunkIDs)
	knnQuery := map[string]interface{}{
		"field":          fmt.Sprintf("q_%d_vec", vectorSize),
		"query_vector":   queryVector,
		"k":              k,
		"num_candidates": k * 2,
		"similarity":     0.0, // No threshold - get all
		"filter": map[string]interface{}{
			"terms": map[string]interface{}{"id": chunkIDs},
		},
	}

	queryBody := map[string]interface{}{
		"knn":     knnQuery,
		"size":    k,
		"_source": false, // Don't need source fields, only need _id and _score
	}

	body, err := json.Marshal(queryBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal KNN query: %w", err)
	}

	//common.Info("KNNScores query body", zap.String("body", string(body)))

	// Execute search - use first index name from chunks if available
	indexName := ""
	if len(chunks) > 0 {
		if idx, ok := chunks[0]["_index"].(string); ok {
			indexName = idx
		}
	}

	res, err := e.client.Search(
		e.client.Search.WithContext(ctx),
		e.client.Search.WithIndex(indexName),
		e.client.Search.WithBody(bytes.NewReader(body)),
	)
	if err != nil {
		return nil, fmt.Errorf("KNN scores search failed: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		bodyBytes, _ := io.ReadAll(res.Body)
		return nil, fmt.Errorf("KNN scores search returned error: %s, body: %s", res.Status(), string(bodyBytes))
	}

	var esResp SearchResponse
	if err := json.NewDecoder(res.Body).Decode(&esResp); err != nil {
		return nil, fmt.Errorf("failed to parse KNN scores response: %w", err)
	}

	common.Info("KNNScores ES response", zap.Int("hitCount", len(esResp.Hits.Hits)), zap.Any("firstHit", func() interface{} {
		if len(esResp.Hits.Hits) > 0 {
			return esResp.Hits.Hits[0]
		}
		return nil
	}()))

	// Return raw ES response
	// Caller will pass to GetScores to extract scores
	knnResult := make(map[string]interface{})
	knnResult["hits"] = map[string]interface{}{
		"hits": esResp.Hits.Hits,
	}
	return knnResult, nil
}

// GetScores extracts similarity scores from KNN search result
func (e *elasticsearchEngine) GetScores(knnResult map[string]interface{}) map[string]float64 {
	scores := make(map[string]float64)
	hits, ok := knnResult["hits"].(map[string]interface{})
	if !ok {
		return scores
	}
	hitsList, ok := hits["hits"]
	if !ok {
		return scores
	}

	switch v := hitsList.(type) {
	case []interface{}:
		for _, h := range v {
			if hit, ok := h.(map[string]interface{}); ok {
				if docID, ok := hit["_id"].(string); ok && docID != "" {
					if scoreVal := hit["_score"]; scoreVal != nil {
						if score, ok := scoreVal.(float64); ok {
							scores[docID] = score
						}
					}
				}
			}
		}
	case []map[string]interface{}:
		for _, hit := range v {
			if docID, ok := hit["_id"].(string); ok && docID != "" {
				if scoreVal := hit["_score"]; scoreVal != nil {
					if score, ok := scoreVal.(float64); ok {
						scores[docID] = score
					}
				}
			}
		}
	default:
		// Handle slice of structs via reflection
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice {
			for i := 0; i < rv.Len(); i++ {
				elem := rv.Index(i)
				idField := elem.FieldByName("ID")
				if !idField.IsValid() {
					idField = elem.FieldByName("Id")
				}
				if !idField.IsValid() || idField.Kind() != reflect.String {
					continue
				}
				docID := idField.String()
				if docID == "" {
					continue
				}
				scoreField := elem.FieldByName("Score")
				if !scoreField.IsValid() || scoreField.Kind() != reflect.Float64 {
					continue
				}
				scores[docID] = scoreField.Float()
			}
		}
	}

	return scores
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

// rerankWindow returns the candidate-window size shared by retrieval's
// block fetch and slice. Mirrors Dealer._rerank_window in rag/nlp/search.py.
//
// `size` is the per-page size; the window MUST be an exact multiple of it,
// otherwise the block fetched (offset // window) and the in-block page slice
// (offset % window) drift apart and deep pagination silently drops results.
//
// The window targets a provider-friendly pool of ~64 candidates, bounded by
// `topK` when given (i.e. when an external reranker is active), and is always
// rounded UP to a whole number of pages to preserve the alignment invariant.
func rerankWindow(size, topK int) int {
	if size <= 1 {
		if topK > 0 {
			return min(30, topK)
		}
		return 30
	}
	window := ((64 + size - 1) / size) * size // ceil(64/size) * size
	if topK > 0 {
		if aligned := ((topK + size - 1) / size) * size; window > aligned {
			window = aligned
		}
	}
	return window
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

	window := rerankWindow(size, topK)

	offset := (page - 1) * window
	if offset < 0 {
		offset = 0
	}

	return offset, window
}

// convertESResponse converts ES SearchResponse to unified chunks format
func convertESResponse(esResp *SearchResponse, vectorFieldName string) []map[string]interface{} {
	if esResp == nil || esResp.Hits.Hits == nil {
		return []map[string]interface{}{}
	}

	chunks := make([]map[string]interface{}, len(esResp.Hits.Hits))
	for i, hit := range esResp.Hits.Hits {
		chunks[i] = hit.Source
		if chunks[i] == nil {
			chunks[i] = make(map[string]interface{})
		}
		chunks[i]["_score"] = hit.Score
		chunks[i]["_id"] = hit.ID
		chunks[i]["_index"] = hit.Index
		if len(hit.Highlight) > 0 {
			chunks[i]["highlight"] = hit.Highlight
		}
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

		// Skip id field (cannot order by text field)
		if field.Field == "id" {
			continue
		}

		// Special handling for page_num_int and top_int
		if field.Field == "page_num_int" || field.Field == "top_int" {
			result = append(result, map[string]interface{}{
				field.Field: map[string]interface{}{
					"order":         direction,
					"unmapped_type": "float",
					"mode":          "avg",
					"numeric_type":  "double",
				},
			})
		} else if strings.HasSuffix(field.Field, "_int") || strings.HasSuffix(field.Field, "_flt") {
			// Fields ending with _int or _flt
			result = append(result, map[string]interface{}{
				field.Field: map[string]interface{}{
					"order":         direction,
					"unmapped_type": "float",
				},
			})
		} else if field.Field == "_score" || field.Field == "score" {
			result = append(result, map[string]interface{}{
				"_score": direction,
			})
		} else {
			// Default: unmapped_type = keyword
			result = append(result, map[string]interface{}{
				field.Field: map[string]interface{}{
					"order":         direction,
					"unmapped_type": "keyword",
				},
			})
		}
	}

	return result
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
