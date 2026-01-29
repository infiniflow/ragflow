package elasticsearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/elastic/go-elasticsearch/v8/esapi"
	"go.uber.org/zap"

	"ragflow/internal/logger"
)

// SearchRequest Elasticsearch search request
type SearchRequest struct {
	IndexNames []string
	Query      map[string]interface{}
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

// Search executes search
func (e *elasticsearchEngine) Search(ctx context.Context, req interface{}) (interface{}, error) {
	searchReq, ok := req.(*SearchRequest)
	if !ok {
		return nil, fmt.Errorf("invalid search request type")
	}

	if len(searchReq.IndexNames) == 0 {
		return nil, fmt.Errorf("index names cannot be empty")
	}

	// Build search query
	queryBody := make(map[string]interface{})
	if searchReq.Query != nil {
		// Create a copy of the query map to avoid modifying the original
		queryCopy := make(map[string]interface{})
		for k, v := range searchReq.Query {
			queryCopy[k] = v
		}

		// Check if this query contains a KNN component
		if knnValue, ok := queryCopy["knn"]; ok {
			// KNN queries should be at top level, not inside "query"
			queryBody["knn"] = knnValue
			// Remove the knn key from the copy
			delete(queryCopy, "knn")
		}

		// If there are remaining query components after removing knn, add them under "query"
		if len(queryCopy) > 0 {
			queryBody["query"] = queryCopy
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

	// Serialize query
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(queryBody); err != nil {
		return nil, fmt.Errorf("error encoding query: %w", err)
	}

	// query: {"query": {"bool": {"must": [{"query_string": {"fields": ["title_tks^10", "title_sm_tks^5", "important_kwd^30", "important_tks^20", "question_tks^20", "content_ltks^2", "content_sm_ltks"], "type": "best_fields", "query": "((((rag OR (torment \"call on the carpet\" annoy tabloid tease ragtime)^0.2))^1.0)^5 OR (\"torment\" OR \"call on the carpet\" OR \"annoy\" OR \"tabloid\" OR \"teas\" OR \"ragtim\")^0.7)", "minimum_should_match": "30%", "boost": 1}}], "filter": [{"terms": {"kb_id": ["633f5749f0f011f0b06038a74640adcc"]}}, {"bool": {"must_not": [{"range": {"available_int": {"lt": 1}}}]}}], "boost": 0.050000000000000044}}, "knn": {"field": "q_1024_vec", "k": 1024, "num_candidates": 2048, "query_vector": [], "filter": {"bool": {"must": [{"query_string": {"fields": ["title_tks^10", "title_sm_tks^5", "important_kwd^30", "important_tks^20", "question_tks^20", "content_ltks^2", "content_sm_ltks"], "type": "best_fields", "query": "((((rag OR (torment \"call on the carpet\" annoy tabloid tease ragtime)^0.2))^1.0)^5 OR (\"torment\" OR \"call on the carpet\" OR \"annoy\" OR \"tabloid\" OR \"teas\" OR \"ragtim\")^0.7)", "minimum_should_match": "30%", "boost": 1}}], "filter": [{"terms": {"kb_id": ["633f5749f0f011f0b06038a74640adcc"]}}, {"bool": {"must_not": [{"range": {"available_int": {"lt": 1}}}]}}], "boost": 0.050000000000000044}}, "similarity": 0.2}, "from": 0, "size": 90}

	// Log search details
	logger.Debug("Elasticsearch searching indices", zap.Strings("indices", searchReq.IndexNames))
	logger.Debug("Elasticsearch DSL", zap.Any("dsl", queryBody))

	// Build search request
	reqES := esapi.SearchRequest{
		Index: searchReq.IndexNames,
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
	var response SearchResponse
	if err := json.NewDecoder(res.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("error parsing response: %w", err)
	}

	return &response, nil
}

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
