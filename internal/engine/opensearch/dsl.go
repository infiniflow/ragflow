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
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

var queryStringOperators = map[string]bool{"AND": true, "OR": true, "NOT": true}

// queryVectorField matches serialized query vector floats for log redaction.
var queryVectorField = regexp.MustCompile(`"query_vector":\s*\[[^\]]*\]|"vector":\s*\[[^\]]*\]`)

// elideQueryVector drops long vector float arrays from payload before logging.
func elideQueryVector(body []byte) string {
	return queryVectorField.ReplaceAllString(string(body), `"vector":"<elided>"`)
}

// BuildSearchDSL compiles a unified types.SearchRequest into an OpenSearch DSL search body,
// along with any pipeline parameter and a flag indicating whether search_after pagination should be used.
// It implements:
//  1. Root hybrid query assembly ({"query": {"hybrid": {"queries": [bool_query, knn_query]}}})
//  2. Four-decimal weight normalization summing strictly to 1.0
//  3. Bi-modal guard (hybrid and search pipeline attached only when text and vector match both exist)
//  4. Strict filter isolation: structural filters (kb_id, available_int, doc_id) are placed into
//     bool.filter and knn.filter; lexical query_string is never placed into knn.filter.
//  5. Negative range filtering for available_int (must_not lt 1).
//  6. Deep pagination support (10,000+ auto sort with _id:asc tie-breaker, vector deep pagination short-circuit).
func buildSearchDSL(ctx context.Context, req *types.SearchRequest) (queryBody map[string]interface{}, useSearchAfter bool, err error) {
	if req == nil {
		return nil, false, fmt.Errorf("search request cannot be nil")
	}
	if ctx != nil && ctx.Err() != nil {
		return nil, false, ctx.Err()
	}
	if len(req.IndexNames) == 0 {
		return nil, false, fmt.Errorf("index names cannot be empty")
	}

	offset := max(req.Offset, 0)
	limit := req.Limit
	if limit <= 0 {
		limit = 30
	}

	isSkillIndex := false
	isMemoryIndex := false
	for _, idx := range req.IndexNames {
		if strings.HasPrefix(idx, "skill_") {
			isSkillIndex = true
		}
		if strings.HasPrefix(idx, "memory_") {
			isMemoryIndex = true
		}
	}

	// Extract match expressions
	var matchText *types.MatchTextExpr
	var matchDense *types.MatchDenseExpr
	for _, expr := range req.MatchExprs {
		if expr == nil {
			continue
		}
		switch m := expr.(type) {
		case *types.MatchTextExpr:
			matchText = m
		case *types.MatchDenseExpr:
			matchDense = m
		}
	}

	hasTextMatch := matchText != nil && strings.TrimSpace(matchText.MatchingText) != ""
	hasVectorMatch := matchDense != nil && len(matchDense.EmbeddingData) > 0

	// Requirement 5: Vector deep pagination guard
	if hasVectorMatch && offset+limit > common.MAX_RESULT_WINDOW {
		return nil, false, fmt.Errorf("vector search does not support offset + limit %d beyond max_result_window (%d)", offset+limit, common.MAX_RESULT_WINDOW)
	}

	// Build base structural filter query
	filterQuery := buildBoolQueryFromCondition(req.Filter, req.KbIDs, isSkillIndex, isMemoryIndex)

	queryBody = make(map[string]interface{})

	// Bi-modal guard: only assemble hybrid query and attach search pipeline when both legs exist
	if hasTextMatch && hasVectorMatch {
		textQuery := buildQueryStringQuery(matchText, isSkillIndex, isMemoryIndex)
		var textBoolQuery map[string]interface{}
		if filterQuery != nil {
			textBoolQuery = cloneFilterSnapshot(filterQuery)
			innerBool := textBoolQuery["bool"].(map[string]interface{})
			must, _ := innerBool["must"].([]interface{})
			must = append(must, textQuery)
			innerBool["must"] = must
		} else {
			textBoolQuery = map[string]interface{}{
				"bool": map[string]interface{}{
					"must": []interface{}{textQuery},
				},
			}
		}

		// Inject rank features into lexical bool query only (never into knn)
		if req.RankFeature != nil && len(req.RankFeature) > 0 && !isSkillIndex && !isMemoryIndex {
			rankQueries := buildRankFeatureQuery(req.RankFeature)
			if len(rankQueries) > 0 {
				innerBool := textBoolQuery["bool"].(map[string]interface{})
				should, _ := innerBool["should"].([]interface{})
				for _, rq := range rankQueries {
					should = append(should, rq)
				}
				innerBool["should"] = should
			}
		}

		candidateDepth := common.MAX_RESULT_WINDOW

		// KNN query gets structural filters only (strictly isolated from query_string).
		// In hybrid search, pass candidateDepth so default k matches pagination_depth,
		// keeping vector candidate set stable across paginated requests.
		knnQuery, knnErr := buildKNNQuery(matchDense, filterQuery, candidateDepth)
		if knnErr != nil {
			return nil, false, knnErr
		}

		weights, hasFusion := extractFusionWeights(req)
		if !hasFusion {
			weights = []float64{0.5, 0.5}
		}
		queryBody["query"] = map[string]interface{}{
			"hybrid": map[string]interface{}{
				"queries": []interface{}{
					textBoolQuery,
					knnQuery,
				},
				"pagination_depth": candidateDepth,
			},
		}
		queryBody["search_pipeline"] = BuildInlinePipeline(weights)
	} else if hasVectorMatch {
		// Single-modal dense vector query
		singleDepth := common.MAX_RESULT_WINDOW
		knnQuery, knnErr := buildKNNQuery(matchDense, filterQuery, singleDepth)
		if knnErr != nil {
			return nil, false, knnErr
		}
		queryBody["query"] = knnQuery
	} else if hasTextMatch {
		// Single-modal text query
		textQuery := buildQueryStringQuery(matchText, isSkillIndex, isMemoryIndex)
		var textBoolQuery map[string]interface{}
		if filterQuery != nil {
			textBoolQuery = cloneFilterSnapshot(filterQuery)
			innerBool := textBoolQuery["bool"].(map[string]interface{})
			must, _ := innerBool["must"].([]interface{})
			must = append(must, textQuery)
			innerBool["must"] = must
		} else {
			textBoolQuery = map[string]interface{}{
				"bool": map[string]interface{}{
					"must": []interface{}{textQuery},
				},
			}
		}

		if req.RankFeature != nil && len(req.RankFeature) > 0 && !isSkillIndex && !isMemoryIndex {
			rankQueries := buildRankFeatureQuery(req.RankFeature)
			if len(rankQueries) > 0 {
				innerBool := textBoolQuery["bool"].(map[string]interface{})
				should, _ := innerBool["should"].([]interface{})
				for _, rq := range rankQueries {
					should = append(should, rq)
				}
				innerBool["should"] = should
			}
		}
		queryBody["query"] = textBoolQuery
	} else {
		// Neither text nor vector match: filter or match_all
		if filterQuery != nil {
			queryBody["query"] = filterQuery
		} else {
			queryBody["query"] = map[string]interface{}{
				"match_all": map[string]interface{}{},
			}
		}
	}

	// Deep pagination and sorting
	hasExplicitSort := req.OrderBy != nil && len(req.OrderBy.Fields) > 0
	useSearchAfter = limit > 0 && (offset+limit > common.MAX_RESULT_WINDOW) && !hasVectorMatch

	if useSearchAfter {
		var sort []map[string]interface{}
		if hasExplicitSort {
			sort = ensureSearchAfterSort(parseOrderByExpr(req.OrderBy))
		} else {
			sort = buildDefaultSortForSearchAfter()
		}
		queryBody["sort"] = sort
		queryBody["track_total_hits"] = true
	} else {
		if hasExplicitSort {
			sort := parseOrderByExpr(req.OrderBy)
			if len(sort) > 0 {
				queryBody["sort"] = sort
			}
		}
		if limit > 0 {
			queryBody["size"] = limit
			queryBody["from"] = offset
		}
		queryBody["track_total_hits"] = true
	}

	// Select fields and vector fields
	var sourceFields []string
	if len(req.SelectFields) > 0 {
		if isMemoryIndex {
			sourceFields = mapMemoryMessageOSFields(req.SelectFields, false)
		} else {
			sourceFields = slices.Clone(req.SelectFields)
		}
		if (hasTextMatch || hasVectorMatch) && !isSkillIndex && !isMemoryIndex {
			if !slices.Contains(sourceFields, common.PAGERANK_FLD) {
				sourceFields = append(sourceFields, common.PAGERANK_FLD)
			}
			if !slices.Contains(sourceFields, common.TAG_FLD) {
				sourceFields = append(sourceFields, common.TAG_FLD)
			}
		}
		queryBody["_source"] = sourceFields

		var vectorFields []string
		for _, f := range sourceFields {
			if strings.HasSuffix(f, "_vec") {
				vectorFields = append(vectorFields, f)
			}
		}
		if len(vectorFields) > 0 {
			queryBody["fields"] = vectorFields
		}
	} else {
		if (hasTextMatch || hasVectorMatch) && !isSkillIndex && !isMemoryIndex {
			queryBody["_source"] = []string{common.PAGERANK_FLD, common.TAG_FLD}
		}
	}

	return queryBody, useSearchAfter, nil
}

// buildBoolQueryFromCondition builds an OpenSearch bool query from condition map.
func buildBoolQueryFromCondition(filter map[string]interface{}, kbIDs []string, isSkillIndex, isMemoryIndex bool) map[string]interface{} {
	return buildBoolQueryFromConditionRaw(types.CompilationFilter(filter), kbIDs, isSkillIndex, isMemoryIndex)
}

func buildBoolQueryFromConditionRaw(filter map[string]interface{}, kbIDs []string, isSkillIndex, isMemoryIndex bool) map[string]interface{} {
	var mustClauses []interface{}
	var mustNotClauses []interface{}
	var filterClauses []interface{}

	if kbIDs != nil && len(kbIDs) > 0 {
		fieldName := "kb_id"
		if isMemoryIndex {
			fieldName = "memory_id"
		}
		filterClauses = append(filterClauses, map[string]interface{}{
			"terms": map[string]interface{}{fieldName: kbIDs},
		})
	}

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

	keys := make([]string, 0, len(filter))
	for k := range filter {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		v := filter[k]
		if k == "and" || k == "or" {
			var children []interface{}
			for _, child := range types.FilterClauses(v) {
				if childQuery := buildBoolQueryFromConditionRaw(child, nil, false, isMemoryIndex); childQuery != nil {
					children = append(children, childQuery)
				}
			}
			if len(children) > 0 {
				clause := map[string]interface{}{"filter": children}
				if k == "or" {
					clause = map[string]interface{}{"should": children, "minimum_should_match": 1}
				}
				filterClauses = append(filterClauses, map[string]interface{}{"bool": clause})
			}
			continue
		}
		if isMemoryIndex {
			k = mapMemoryMessageESField(k, false)
		}
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
		if k == "must_not" {
			for _, clause := range mustNotExistsClauses(v) {
				mustNotClauses = append(mustNotClauses, clause)
			}
			continue
		}
		if k == "exists" {
			filterClauses = append(filterClauses, map[string]interface{}{"exists": map[string]interface{}{"field": v}})
			continue
		}
		if k == "knowledge_graph_kwd" {
			if kgClause := buildKnowledgeGraphFilterClause(v); kgClause != nil {
				filterClauses = append(filterClauses, kgClause)
			}
			continue
		}
		if k == "id" {
			if idClauses := idTermClauses(v); len(idClauses) > 0 {
				filterClauses = append(filterClauses, map[string]interface{}{
					"bool": map[string]interface{}{
						"should":               idClauses,
						"minimum_should_match": 1,
					},
				})
			}
			continue
		}
		if v == nil || v == "" {
			continue
		}
		if isMemoryIndex && k == "session_id_contains" {
			if text, ok := v.(string); ok {
				pattern := strings.NewReplacer(`\`, `\\`, "*", `\*`, "?", `\?`).Replace(text)
				filterClauses = append(filterClauses, map[string]interface{}{"wildcard": map[string]interface{}{"session_id": "*" + pattern + "*"}})
			}
			continue
		}
		if isMemoryIndex && k == "session_id" {
			if strVal, ok := v.(string); ok && strVal != "" {
				filterClauses = append(filterClauses, map[string]interface{}{
					"query_string": map[string]interface{}{
						"query":            fmt.Sprintf("*%s*", strVal),
						"fields":           []string{"session_id"},
						"analyze_wildcard": true,
					},
				})
				continue
			}
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
		} else if int64Val, ok := v.(int64); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{k: int64Val},
			})
		} else if floatVal, ok := v.(float64); ok {
			filterClauses = append(filterClauses, map[string]interface{}{
				"term": map[string]interface{}{k: floatVal},
			})
		}
	}

	boolQuery := make(map[string]interface{})
	if len(mustClauses) > 0 {
		boolQuery["must"] = mustClauses
	}
	if len(mustNotClauses) > 0 {
		boolQuery["must_not"] = mustNotClauses
	}
	if len(filterClauses) > 0 {
		boolQuery["filter"] = filterClauses
	}

	if len(boolQuery) == 0 {
		return nil
	}

	return map[string]interface{}{"bool": boolQuery}
}

func idTermClauses(value interface{}) []interface{} {
	switch val := value.(type) {
	case nil:
		return nil
	case string:
		if val == "" {
			return nil
		}
		return []interface{}{
			map[string]interface{}{"term": map[string]interface{}{"id": val}},
			map[string]interface{}{"term": map[string]interface{}{"_id": val}},
		}
	case int:
		if val == 0 {
			return nil
		}
		return []interface{}{
			map[string]interface{}{"term": map[string]interface{}{"id": val}},
			map[string]interface{}{"term": map[string]interface{}{"_id": val}},
		}
	case int64:
		if val == 0 {
			return nil
		}
		return []interface{}{
			map[string]interface{}{"term": map[string]interface{}{"id": val}},
			map[string]interface{}{"term": map[string]interface{}{"_id": val}},
		}
	default:
		items, ok := conditionTermsList(value)
		if !ok || len(items) == 0 {
			return nil
		}
		return []interface{}{
			map[string]interface{}{"terms": map[string]interface{}{"id": items}},
			map[string]interface{}{"terms": map[string]interface{}{"_id": items}},
		}
	}
}

func mustNotExistsClauses(value interface{}) []map[string]interface{} {
	exclusions, ok := value.(map[string]interface{})
	if !ok {
		return nil
	}
	if field, ok := exclusions["exists"]; ok && len(exclusions) == 1 {
		return []map[string]interface{}{{"exists": map[string]interface{}{"field": field}}}
	}
	if query := buildBoolQueryFromConditionRaw(exclusions, nil, false, false); query != nil {
		return []map[string]interface{}{query}
	}
	return nil
}

// cloneFilterSnapshot deep-copies the bool query structure so that passing it to
// knn.filter does not share references with the scored lexical query.
func cloneFilterSnapshot(q map[string]interface{}) map[string]interface{} {
	if q == nil {
		return nil
	}
	clone := make(map[string]interface{}, len(q))
	for k, v := range q {
		clone[k] = v
	}
	inner, ok := q["bool"].(map[string]interface{})
	if !ok {
		return clone
	}
	innerClone := make(map[string]interface{}, len(inner))
	for k, v := range inner {
		innerClone[k] = v
	}
	for _, key := range []string{"must", "should", "filter", "must_not"} {
		if clauses, ok := inner[key].([]interface{}); ok {
			innerClone[key] = slices.Clone(clauses)
		}
	}
	clone["bool"] = innerClone
	return clone
}

// buildQueryStringQuery builds a query_string query from MatchTextExpr.
func buildQueryStringQuery(matchText *types.MatchTextExpr, isSkillIndex, isMemoryIndex bool) map[string]interface{} {
	if matchText == nil {
		return nil
	}

	minimumShouldMatch := "0%"
	if matchText.ExtraOptions != nil {
		if msm, ok := matchText.ExtraOptions["minimum_should_match"].(float64); ok {
			minimumShouldMatch = common.FormatMinimumShouldMatchPercent(msm)
		}
	}

	fields := matchText.Fields
	if len(fields) == 0 {
		if isSkillIndex {
			fields = []string{"name_tks^10", "tags_tks^5", "description_tks^3", "content_tks^1"}
		} else if isMemoryIndex {
			fields = []string{"tokenized_content_ltks"}
		} else {
			fields = []string{"title_tks^10", "title_sm_tks^5", "important_kwd^30", "important_tks^20", "question_tks^20", "content_ltks^2", "content_sm_ltks"}
		}
	}
	if isSkillIndex {
		fields = mapSkillSearchFields(fields)
	}
	if isMemoryIndex {
		fields = mapMemoryMessageOSFields(fields, true)
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
			"query":                lowerCaseQueryText(matchText.MatchingText),
			"minimum_should_match": minimumShouldMatch,
			"boost":                boost,
		},
	}
}

// buildKNNQuery builds an OpenSearch knn query block for the given dense vector match expression.
func buildKNNQuery(matchDense *types.MatchDenseExpr, filterQuery map[string]interface{}, defaultLimit int) (map[string]interface{}, error) {
	if matchDense == nil || len(matchDense.EmbeddingData) == 0 {
		return nil, fmt.Errorf("matchDense or embedding data is empty")
	}

	k := matchDense.TopN
	if k <= 0 {
		k = defaultLimit
	}
	if k <= 0 {
		k = 1024
	}
	k = min(k, 10000)

	knnParams := map[string]interface{}{
		"vector": matchDense.EmbeddingData,
		"k":      k,
	}
	if filterQuery != nil {
		knnParams["filter"] = cloneFilterSnapshot(filterQuery)
	}

	vectorField := matchDense.VectorColumnName
	if vectorField == "" {
		vectorField = fmt.Sprintf("q_%d_vec", len(matchDense.EmbeddingData))
	}

	return map[string]interface{}{
		"knn": map[string]interface{}{
			vectorField: knnParams,
		},
	}, nil
}

// buildRankFeatureQuery builds rank_feature queries for learning to rank.
func buildRankFeatureQuery(rankFeature map[string]float64) []map[string]interface{} {
	if len(rankFeature) == 0 {
		return nil
	}

	keys := make([]string, 0, len(rankFeature))
	for k := range rankFeature {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var queries []map[string]interface{}
	for _, fld := range keys {
		sc := rankFeature[fld]
		field := fmt.Sprintf("%s.%s", common.TAG_FLD, fld)
		if fld == common.PAGERANK_FLD {
			field = common.PAGERANK_FLD
		}
		queries = append(queries, map[string]interface{}{
			"rank_feature": map[string]interface{}{
				"field":  field,
				"linear": map[string]interface{}{},
				"boost":  sc,
			},
		})
	}
	return queries
}

// parseOrderByExpr parses OrderBy expression into OpenSearch sort clauses.
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

		if field.Field == "id" {
			continue
		}

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

// ensureSearchAfterSort ensures the sort array ends with a unique tie-breaker field ({"_id": "asc"}).
func ensureSearchAfterSort(sort []map[string]interface{}) []map[string]interface{} {
	if len(sort) == 0 {
		return buildDefaultSortForSearchAfter()
	}
	lastClause := sort[len(sort)-1]
	if dir, ok := lastClause["_id"]; ok && dir == "asc" {
		return sort
	}
	copied := make([]map[string]interface{}, len(sort), len(sort)+1)
	copy(copied, sort)
	return append(copied, map[string]interface{}{"_id": "asc"})
}

// buildDefaultSortForSearchAfter returns the default tie-breaker sort for search_after cursor.
func buildDefaultSortForSearchAfter() []map[string]interface{} {
	return []map[string]interface{}{
		{"_score": "desc"},
		{"_id": "asc"},
	}
}

// extractFusionWeights parses weights from FusionExpr (e.g. "0.001,1"), normalizes them to sum strictly to 1.0.
func extractFusionWeights(req *types.SearchRequest) ([]float64, bool) {
	if req == nil {
		return NormalizeWeights([]float64{0.5, 0.5}), false
	}
	for _, expr := range req.MatchExprs {
		if fusion, ok := expr.(*types.FusionExpr); ok && fusion != nil {
			if fusion.Method == "weighted_sum" && fusion.FusionParams != nil {
				switch val := fusion.FusionParams["weights"].(type) {
				case string:
					parts := strings.Split(val, ",")
					if len(parts) == 2 {
						w0, err0 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
						w1, err1 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
						if err0 == nil && err1 == nil {
							return NormalizeWeights([]float64{w0, w1}), true
						}
					}
				case []float64:
					if len(val) == 2 {
						return NormalizeWeights(val), true
					}
				case []interface{}:
					if len(val) == 2 {
						w0, ok0 := parseFloat(val[0])
						w1, ok1 := parseFloat(val[1])
						if ok0 && ok1 {
							return NormalizeWeights([]float64{w0, w1}), true
						}
					}
				}
			}
		}
	}
	return NormalizeWeights([]float64{0.5, 0.5}), false
}

func parseFloat(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func mapSkillSearchFields(fields []string) []string {
	mapped := make([]string, 0, len(fields))
	for _, field := range fields {
		name, boost, hasBoost := strings.Cut(field, "^")
		switch name {
		case "name":
			name = "name_tks"
		case "tags":
			name = "tags_tks"
		case "description":
			name = "description_tks"
		case "content":
			name = "content_tks"
		}
		if hasBoost {
			mapped = append(mapped, name+"^"+boost)
		} else {
			mapped = append(mapped, name)
		}
	}
	return mapped
}

func mapMemoryMessageOSFields(fields []string, useTokenizedContent bool) []string {
	if len(fields) == 0 {
		return fields
	}
	mapped := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		mappedField := mapMemoryMessageESField(field, useTokenizedContent)
		if _, ok := seen[mappedField]; ok {
			continue
		}
		seen[mappedField] = struct{}{}
		mapped = append(mapped, mappedField)
	}
	return mapped
}

func lowerCaseQueryText(text string) string {
	var b strings.Builder
	b.Grow(len(text))
	inQuote := false
	escaped := false
	for i := 0; i < len(text); {
		c := text[i]
		if escaped {
			b.WriteByte(c)
			escaped = false
			i++
			continue
		}
		switch {
		case c == '\\':
			b.WriteByte(c)
			escaped = true
			i++
		case c == '"':
			inQuote = !inQuote
			b.WriteByte(c)
			i++
		case isASCIILetter(c):
			j := i
			for j < len(text) && isASCIILetter(text[j]) {
				j++
			}
			word := text[i:j]
			if !inQuote && queryStringOperators[word] {
				b.WriteString(word)
			} else {
				b.WriteString(strings.ToLower(word))
			}
			i = j
		default:
			b.WriteByte(c)
			i++
		}
	}
	return b.String()
}

func isASCIILetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}
