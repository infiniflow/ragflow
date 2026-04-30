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
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"go.uber.org/zap"

	"ragflow/internal/engine/types"
	"ragflow/internal/logger"
)

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
	logger.Debug("Elasticsearch searching indices", zap.Strings("indices", req.IndexNames))
	logger.Debug("Elasticsearch DSL", zap.Any("dsl", queryBody))

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
			logger.Error("Elasticsearch failed to read error response body", err)
		} else {
			logger.Warn("Elasticsearch error response", zap.String("body", string(bodyBytes)))
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
func buildFilterClauses(kbIDs []string, available int) []map[string]interface{} {
	var filters []map[string]interface{}

	if len(kbIDs) > 0 {
		filters = append(filters, map[string]interface{}{
			"terms": map[string]interface{}{"kb_id": kbIDs},
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

// GetFields is not implemented for Elasticsearch
func (e *elasticsearchEngine) GetFields(chunks []map[string]interface{}, fields []string) (map[string]map[string]interface{}, error) {
	logger.Warn("GetFields not implemented for Elasticsearch")
	return nil, nil
}

// GetAggregation is not implemented for Elasticsearch
func (e *elasticsearchEngine) GetAggregation(chunks []map[string]interface{}, fieldName string) []map[string]interface{} {
	logger.Warn("GetAggregation not implemented for Elasticsearch")
	return nil
}

// GetHighlight is not implemented for Elasticsearch
func (e *elasticsearchEngine) GetHighlight(chunks []map[string]interface{}, keywords []string, fieldName string) map[string]string {
	logger.Warn("GetHighlight not implemented for Elasticsearch")
	return nil
}

// GetDocIDs is not implemented for Elasticsearch
func (e *elasticsearchEngine) GetDocIDs(chunks []map[string]interface{}) []string {
	logger.Warn("GetDocIDs not implemented for Elasticsearch")
	return nil
}
