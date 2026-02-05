package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"go.uber.org/zap"

	"ragflow/internal/engine/types"
	"ragflow/internal/logger"
)

// SearchRequest Elasticsearch search request (legacy, kept for backward compatibility)
type SearchRequest struct {
	IndexNames []string
	Query      map[string]interface{}
	Filters    map[string]interface{} // Filter conditions (e.g., kb_id, doc_id, available_int)
	Size       int
	From       int
	Highlight  map[string]interface{}
	Source     []string
	Sort       []interface{}
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

// Search executes search (supports both unified engine.SearchRequest and legacy SearchRequest)
func (e *elasticsearchEngine) Search(ctx context.Context, req interface{}) (interface{}, error) {

	switch searchReq := req.(type) {
	case *types.SearchRequest:
		return e.searchUnified(ctx, searchReq)
	case *SearchRequest:
		return e.searchLegacy(ctx, searchReq)
	default:
		return nil, fmt.Errorf("invalid search request type: %T", req)
	}
}

// searchUnified handles the unified engine.SearchRequest
func (e *elasticsearchEngine) searchUnified(ctx context.Context, req *types.SearchRequest) (*types.SearchResponse, error) {
	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	// Build pagination parameters
	offset, limit := calculatePagination(req.Page, req.Size, req.TopK)

	// Build filter clauses (default: available=1, meaning available_int >= 1)
	// Reference: rag/utils/es_conn.py L60-L78
	filterClauses := buildFilterClauses(req.KbIDs, req.DocIDs, 1)

	// Build search query body
	queryBody := make(map[string]interface{})

	if req.KeywordOnly || len(req.Vector) == 0 {
		// Keyword-only search
		queryBody["query"] = buildESKeywordQuery(req.Question, filterClauses)
	} else {
		// Hybrid search: keyword + vector
		queryBody["query"] = buildESHybridQuery(req.Question, req.Vector, req.VectorSimilarityWeight, filterClauses)
	}

	queryBody["size"] = limit
	queryBody["from"] = offset

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
	chunks := convertESResponse(&esResp)
	return &types.SearchResponse{
		Chunks: chunks,
		Total:  esResp.Hits.Total.Value,
	}, nil
}

// searchLegacy handles the legacy elasticsearch.SearchRequest (backward compatibility)
func (e *elasticsearchEngine) searchLegacy(ctx context.Context, searchReq *SearchRequest) (*SearchResponse, error) {
	if len(searchReq.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	// Build search query
	queryBody := make(map[string]interface{})

	// Process Filters first - convert to Elasticsearch filter clauses
	var filterClauses []map[string]interface{}
	if searchReq.Filters != nil && len(searchReq.Filters) > 0 {
		for field, value := range searchReq.Filters {
			switch v := value.(type) {
			case map[string]interface{}:
				filterClauses = append(filterClauses, map[string]interface{}{
					field: v,
				})
			default:
				filterClauses = append(filterClauses, map[string]interface{}{
					"term": map[string]interface{}{
						field: v,
					},
				})
			}
		}
	}

	if searchReq.Query != nil {
		queryCopy := make(map[string]interface{})
		for k, v := range searchReq.Query {
			queryCopy[k] = v
		}

		if knnValue, ok := queryCopy["knn"]; ok {
			queryBody["knn"] = knnValue
			delete(queryCopy, "knn")
		}

		if len(queryCopy) > 0 {
			if len(filterClauses) > 0 {
				queryBody["query"] = map[string]interface{}{
					"bool": map[string]interface{}{
						"must":   queryCopy,
						"filter": filterClauses,
					},
				}
			} else {
				queryBody["query"] = queryCopy
			}
		} else if len(filterClauses) > 0 {
			queryBody["query"] = map[string]interface{}{
				"bool": map[string]interface{}{
					"filter": filterClauses,
				},
			}
		}
	} else if len(filterClauses) > 0 {
		queryBody["query"] = map[string]interface{}{
			"bool": map[string]interface{}{
				"filter": filterClauses,
			},
		}
	}
	if searchReq.Size > 0 {
		queryBody["size"] = searchReq.Size
	}
	if searchReq.From > 0 {
		queryBody["from"] = searchReq.From
	}
	if searchReq.Highlight != nil {
		queryBody["highlight"] = searchReq.Highlight
	}
	if len(searchReq.Source) > 0 {
		queryBody["_source"] = searchReq.Source
	}
	if len(searchReq.Sort) > 0 {
		queryBody["sort"] = searchReq.Sort
	}

	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(queryBody); err != nil {
		return nil, fmt.Errorf("error encoding query: %w", err)
	}

	logger.Debug("Elasticsearch searching indices", zap.Strings("indices", searchReq.IndexNames))
	logger.Debug("Elasticsearch DSL", zap.Any("dsl", queryBody))

	reqES := esapi.SearchRequest{
		Index: searchReq.IndexNames,
		Body:  &buf,
	}

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

	var response SearchResponse
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return &response, nil
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

// buildFilterClauses builds ES filter clauses from kb_ids, doc_ids and available_int
// Reference: rag/utils/es_conn.py L60-L78
// When available=0: available_int < 1
// When available!=0: NOT (available_int < 1)
func buildFilterClauses(kbIDs, docIDs []string, available int) []map[string]interface{} {
	var filters []map[string]interface{}

	if len(kbIDs) > 0 {
		filters = append(filters, map[string]interface{}{
			"terms": map[string]interface{}{"kb_id": kbIDs},
		})
	}

	if len(docIDs) > 0 {
		filters = append(filters, map[string]interface{}{
			"terms": map[string]interface{}{"doc_id": docIDs},
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

// buildESKeywordQuery builds keyword-only search query for ES
func buildESKeywordQuery(question string, filterClauses []map[string]interface{}) map[string]interface{} {
	mustClause := map[string]interface{}{
		"multi_match": map[string]interface{}{
			"query":  question,
			"fields": []string{"title_tks^2", "content_ltks", "question_kwd", "important_kwd^3"},
			"type":   "best_fields",
		},
	}

	return map[string]interface{}{
		"bool": map[string]interface{}{
			"must":                 mustClause,
			"filter":               filterClauses,
			"minimum_should_match": 1,
		},
	}
}

// buildESHybridQuery builds hybrid search query (keyword + vector) for ES
func buildESHybridQuery(question string, vector []float64, vectorWeight float64, filterClauses []map[string]interface{}) map[string]interface{} {
	textWeight := 1.0 - vectorWeight

	dimension := len(vector)
	var fieldBuilder strings.Builder
	fieldBuilder.WriteString("q_")
	fieldBuilder.WriteString(strconv.Itoa(dimension))
	fieldBuilder.WriteString("_vec")
	fieldName := fieldBuilder.String()

	shouldClauses := []map[string]interface{}{
		{
			"multi_match": map[string]interface{}{
				"query":  question,
				"fields": []string{"title_tks^2", "content_ltks", "question_kwd", "important_kwd^3"},
				"type":   "best_fields",
				"boost":  textWeight,
			},
		},
		{
			"script_score": map[string]interface{}{
				"query": map[string]interface{}{"match_all": map[string]interface{}{}},
				"script": map[string]interface{}{
					"source": fmt.Sprintf("cosineSimilarity(params.query_vector, '%s') + 1.0", fieldName),
					"params": map[string]interface{}{
						"query_vector": vector,
					},
				},
				"boost": vectorWeight,
			},
		},
	}

	return map[string]interface{}{
		"bool": map[string]interface{}{
			"should": shouldClauses,
			"filter": filterClauses,
		},
	}
}

// convertESResponse converts ES SearchResponse to unified chunks format
func convertESResponse(esResp *SearchResponse) []map[string]interface{} {
	if esResp == nil || esResp.Hits.Hits == nil {
		return []map[string]interface{}{}
	}

	chunks := make([]map[string]interface{}, len(esResp.Hits.Hits))
	for i, hit := range esResp.Hits.Hits {
		chunks[i] = hit.Source
		chunks[i]["_score"] = hit.Score
	}
	return chunks
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
