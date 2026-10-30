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
	"encoding/json"
	"math"
	"slices"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/engine/types"
)

// TestDSL_HybridSearch_PipelineAndWeights verifies root hybrid assembly, weight normalization
// with residual absorption (summing strictly to 1.0), and pipeline strategy handling.
func TestDSL_HybridSearch_PipelineAndWeights(t *testing.T) {
	req := &types.SearchRequest{
		IndexNames: []string{"test_kb_123"},
		KbIDs:      []string{"kb_123"},
		Limit:      10,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				Fields:       []string{"content_ltks"},
				MatchingText: "database performance",
			},
			&types.MatchDenseExpr{
				VectorColumnName: "q_1024_vec",
				EmbeddingData:    make([]float64, 1024),
				TopN:             10,
			},
			&types.FusionExpr{
				Method: "weighted_sum",
				FusionParams: map[string]interface{}{
					"weights": "0.001,1",
				},
			},
		},
	}

	// 1. OpenSearch 2.19: Inline search pipeline with custom weights
	dsl219, useSearchAfter, err := buildSearchDSL(context.Background(), req)
	if err != nil {
		t.Fatalf("buildSearchDSL 2.19 failed: %v", err)
	}
	if useSearchAfter {
		t.Fatalf("expected useSearchAfter to be false")
	}

	// Verify hybrid structure
	query, ok := dsl219["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing query block")
	}
	hybrid, ok := query["hybrid"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected root query.hybrid block, got: %v", query)
	}
	queries, ok := hybrid["queries"].([]interface{})
	if !ok || len(queries) != 2 {
		t.Fatalf("expected 2 queries in hybrid, got: %v", queries)
	}
	if hybrid["pagination_depth"] != common.MAX_RESULT_WINDOW {
		t.Fatalf("expected pagination_depth=%d, got: %v", common.MAX_RESULT_WINDOW, hybrid["pagination_depth"])
	}

	// Verify inline search_pipeline in request body
	sp, ok := dsl219["search_pipeline"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected search_pipeline block in body for 2.19 inline strategy")
	}
	processors, ok := sp["phase_results_processors"].([]interface{})
	if !ok || len(processors) != 1 {
		t.Fatalf("expected 1 processor in search_pipeline, got: %v", processors)
	}
	normProc := processors[0].(map[string]interface{})["normalization-processor"].(map[string]interface{})
	comb := normProc["combination"].(map[string]interface{})
	params := comb["parameters"].(map[string]interface{})
	weights := params["weights"].([]float64)
	if len(weights) != 2 {
		t.Fatalf("expected 2 weights, got: %v", weights)
	}
	if math.Abs((weights[0]+weights[1])-1.0) > 1e-9 {
		t.Fatalf("weights must sum strictly to 1.0, got: %v (sum=%f)", weights, weights[0]+weights[1])
	}
	if math.Abs(weights[0]-0.001) > 1e-6 || math.Abs(weights[1]-0.999) > 1e-6 {
		t.Fatalf("expected weights [0.001, 0.999], got: %v", weights)
	}

	// 2. Default weights [0.5, 0.5] when FusionExpr is absent
	reqNoFusion := &types.SearchRequest{
		IndexNames: []string{"test_kb_123"},
		KbIDs:      []string{"kb_123"},
		Limit:      10,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				Fields:       []string{"content_ltks"},
				MatchingText: "database performance",
			},
			&types.MatchDenseExpr{
				VectorColumnName: "q_1024_vec",
				EmbeddingData:    make([]float64, 1024),
				TopN:             10,
			},
		},
	}
	dslDefault, _, err := buildSearchDSL(context.Background(), reqNoFusion)
	if err != nil {
		t.Fatalf("buildSearchDSL without fusion failed: %v", err)
	}
	spDefault, ok := dslDefault["search_pipeline"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected search_pipeline block in body for default weights")
	}
	procsDefault := spDefault["phase_results_processors"].([]interface{})
	normDefault := procsDefault[0].(map[string]interface{})["normalization-processor"].(map[string]interface{})
	combDefault := normDefault["combination"].(map[string]interface{})
	paramsDefault := combDefault["parameters"].(map[string]interface{})
	weightsDefault := paramsDefault["weights"].([]float64)
	if len(weightsDefault) != 2 || weightsDefault[0] != 0.5 || weightsDefault[1] != 0.5 {
		t.Fatalf("expected default weights [0.5, 0.5], got: %v", weightsDefault)
	}
}

// TestDSL_FilterIsolation_StrictlySeparatesTextFromKNN ensures structural filters
// (kb_id, doc_id, available_int) exist in both text bool and knn filter,
// while query_string is STRICTLY isolated from knn filter.
func TestDSL_FilterIsolation_StrictlySeparatesTextFromKNN(t *testing.T) {
	req := &types.SearchRequest{
		IndexNames: []string{"test_kb_456"},
		KbIDs:      []string{"kb_456"},
		Filter: map[string]interface{}{
			"doc_id":        "doc_abc",
			"available_int": 1,
		},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				MatchingText: "cloud security",
			},
			&types.MatchDenseExpr{
				VectorColumnName: "q_1024_vec",
				EmbeddingData:    make([]float64, 1024),
			},
		},
	}

	dsl, _, err := buildSearchDSL(context.Background(), req)
	if err != nil {
		t.Fatalf("buildSearchDSL failed: %v", err)
	}

	query := dsl["query"].(map[string]interface{})
	hybrid := query["hybrid"].(map[string]interface{})
	queries := hybrid["queries"].([]interface{})

	// Query 0: Text Bool
	textBool := queries[0].(map[string]interface{})["bool"].(map[string]interface{})
	must := textBool["must"].([]interface{})
	if len(must) != 1 {
		t.Fatalf("expected 1 must clause (query_string) in textBool, got: %v", must)
	}
	qs := must[0].(map[string]interface{})["query_string"]
	if qs == nil {
		t.Fatalf("expected query_string in textBool.must")
	}

	// Query 1: KNN
	knn := queries[1].(map[string]interface{})["knn"].(map[string]interface{})
	vecParams := knn["q_1024_vec"].(map[string]interface{})
	knnFilter, ok := vecParams["filter"].(map[string]interface{})
	if !ok || knnFilter == nil {
		t.Fatalf("expected filter inside knn query")
	}

	// Ensure knn filter has NO query_string and NO must clause containing query_string
	filterJSON, _ := json.Marshal(knnFilter)
	filterStr := string(filterJSON)
	if contains(filterStr, "query_string") || contains(filterStr, "cloud security") {
		t.Fatalf("CRITICAL: knn.filter leaked lexical query_string! filter: %s", filterStr)
	}

	// Ensure structural filters (kb_id, doc_id, available_int) exist in knn filter
	if !contains(filterStr, "kb_456") {
		t.Fatalf("expected kb_id in knn filter: %s", filterStr)
	}
	if !contains(filterStr, "doc_abc") {
		t.Fatalf("expected doc_id in knn filter: %s", filterStr)
	}
	if !contains(filterStr, "available_int") {
		t.Fatalf("expected available_int in knn filter: %s", filterStr)
	}
}

// TestDSL_NegativeRangeFilter_AvailableInt verifies negative range filtering logic.
func TestDSL_NegativeRangeFilter_AvailableInt(t *testing.T) {
	// available_int = 1 -> must_not lt 1
	req1 := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		Filter: map[string]interface{}{
			"available_int": 1,
		},
	}
	dsl1, _, err := buildSearchDSL(context.Background(), req1)
	if err != nil {
		t.Fatalf("buildSearchDSL req1 failed: %v", err)
	}
	bodyJSON1, _ := json.Marshal(dsl1)
	bodyStr1 := string(bodyJSON1)
	if !contains(bodyStr1, `"must_not":[{"range":{"available_int":{"lt":1}}}]`) {
		t.Fatalf("expected negative range for available_int=1, got: %s", bodyStr1)
	}

	// available_int = 0 -> range lt 1
	req0 := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		Filter: map[string]interface{}{
			"available_int": 0,
		},
	}
	dsl0, _, err := buildSearchDSL(context.Background(), req0)
	if err != nil {
		t.Fatalf("buildSearchDSL req0 failed: %v", err)
	}
	bodyJSON0, _ := json.Marshal(dsl0)
	bodyStr0 := string(bodyJSON0)
	if !contains(bodyStr0, `"range":{"available_int":{"lt":1}}`) || contains(bodyStr0, "must_not") {
		t.Fatalf("expected positive range lt 1 without must_not for available_int=0, got: %s", bodyStr0)
	}
}

// TestDSL_SingleModal_DenseOnly_NoPipelineNoHybrid verifies single modal vector query.
func TestDSL_SingleModal_DenseOnly_NoPipelineNoHybrid(t *testing.T) {
	req := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				VectorColumnName: "q_1024_vec",
				EmbeddingData:    make([]float64, 1024),
				TopN:             15,
			},
		},
	}

	dsl, _, err := buildSearchDSL(context.Background(), req)
	if err != nil {
		t.Fatalf("buildSearchDSL failed: %v", err)
	}
	if dsl["search_pipeline"] != nil {
		t.Fatalf("expected no search_pipeline attached for dense-only query")
	}

	query := dsl["query"].(map[string]interface{})
	if query["hybrid"] != nil {
		t.Fatalf("dense-only query must not be wrapped in hybrid block")
	}
	knn, ok := query["knn"].(map[string]interface{})
	if !ok || knn["q_1024_vec"] == nil {
		t.Fatalf("expected root knn.q_1024_vec block, got: %v", query)
	}
}

// TestDSL_SingleModal_TextOnly_NoPipelineNoHybrid verifies single modal text query.
func TestDSL_SingleModal_TextOnly_NoPipelineNoHybrid(t *testing.T) {
	req := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				MatchingText: "artificial intelligence",
			},
		},
	}

	dsl, _, err := buildSearchDSL(context.Background(), req)
	if err != nil {
		t.Fatalf("buildSearchDSL failed: %v", err)
	}
	if dsl["search_pipeline"] != nil {
		t.Fatalf("expected no search_pipeline attached for text-only query")
	}

	query := dsl["query"].(map[string]interface{})
	if query["hybrid"] != nil {
		t.Fatalf("text-only query must not be wrapped in hybrid block")
	}
	boolQ, ok := query["bool"].(map[string]interface{})
	if !ok || boolQ["must"] == nil {
		t.Fatalf("expected root bool.must block, got: %v", query)
	}
}

// TestDSL_DeepPagination_AutoSort_SearchAfter verifies 10,000+ deep pagination auto-sort.
func TestDSL_DeepPagination_AutoSort_SearchAfter(t *testing.T) {
	// Case 1: deep offset without explicit sort -> auto-inject [{"_score": "desc"}, {"_id": "asc"}]
	req1 := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		Offset:     common.MAX_RESULT_WINDOW,
		Limit:      30,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "deep read"},
		},
	}

	dsl1, useSearchAfter1, err := buildSearchDSL(context.Background(), req1)
	if err != nil {
		t.Fatalf("buildSearchDSL req1 failed: %v", err)
	}
	if !useSearchAfter1 {
		t.Fatalf("expected useSearchAfter to be true for offset >= MAX_RESULT_WINDOW")
	}
	if dsl1["from"] != nil || dsl1["size"] != nil {
		t.Fatalf("dsl must not emit from/size when useSearchAfter is true, got from=%v, size=%v", dsl1["from"], dsl1["size"])
	}
	sort1, ok := dsl1["sort"].([]map[string]interface{})
	if !ok || len(sort1) != 2 {
		t.Fatalf("expected default 2 sort clauses for search_after, got: %v", sort1)
	}
	if sort1[0]["_score"] != "desc" || sort1[1]["_id"] != "asc" {
		t.Fatalf(`expected [{"_score": "desc"}, {"_id": "asc"}], got: %v`, sort1)
	}

	// Case 2: deep offset with explicit sort -> appends {"_id": "asc"} tie-breaker
	req2 := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		Offset:     common.MAX_RESULT_WINDOW + 10,
		Limit:      20,
		OrderBy:    (&types.OrderByExpr{}).Desc("page_num_int"),
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "deep read"},
		},
	}
	dsl2, useSearchAfter2, err := buildSearchDSL(context.Background(), req2)
	if err != nil {
		t.Fatalf("buildSearchDSL req2 failed: %v", err)
	}
	if !useSearchAfter2 {
		t.Fatalf("expected useSearchAfter to be true")
	}
	sort2, ok := dsl2["sort"].([]map[string]interface{})
	if !ok || len(sort2) != 2 {
		t.Fatalf("expected 2 sort clauses with tie-breaker, got: %v", sort2)
	}
	if sort2[1]["_id"] != "asc" {
		t.Fatalf("expected last sort clause to be _id: asc tie-breaker, got: %v", sort2[1])
	}
}

// TestDSL_DeepPagination_VectorOffsetGuard ensures vector queries short-circuit on offset >= 10,000.
func TestDSL_DeepPagination_VectorOffsetGuard(t *testing.T) {
	req := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		Offset:     common.MAX_RESULT_WINDOW,
		Limit:      10,
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				VectorColumnName: "q_1024_vec",
				EmbeddingData:    make([]float64, 1024),
			},
		},
	}

	_, _, err := buildSearchDSL(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error for vector search with offset >= 10000, got nil")
	}
}

// TestDSL_RankFeatures_KeptInLexicalQuery_OutOfKNN verifies rank features stay out of KNN.
func TestDSL_RankFeatures_KeptInLexicalQuery_OutOfKNN(t *testing.T) {
	req := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "search"},
			&types.MatchDenseExpr{
				VectorColumnName: "q_1024_vec",
				EmbeddingData:    make([]float64, 1024),
			},
		},
		RankFeature: map[string]float64{
			common.PAGERANK_FLD: 1.5,
			"custom_weight":     2.0,
		},
	}

	dsl, _, err := buildSearchDSL(context.Background(), req)
	if err != nil {
		t.Fatalf("buildSearchDSL failed: %v", err)
	}

	query := dsl["query"].(map[string]interface{})
	hybrid := query["hybrid"].(map[string]interface{})
	queries := hybrid["queries"].([]interface{})

	// Lexical query has rank features in should
	textBool := queries[0].(map[string]interface{})["bool"].(map[string]interface{})
	should, ok := textBool["should"].([]interface{})
	if !ok || len(should) != 2 {
		t.Fatalf("expected 2 should clauses in textBool, got: %v", should)
	}

	// Deterministic sorting check: "custom_weight" -> "tag_feas.custom_weight" comes first or second
	shouldJSON, _ := json.Marshal(should)
	shouldStr := string(shouldJSON)
	if !contains(shouldStr, `"field":"pagerank_fea"`) {
		t.Fatalf("missing pagerank_fea in should: %s", shouldStr)
	}
	if !contains(shouldStr, `"field":"tag_feas.custom_weight"`) {
		t.Fatalf("missing tag_feas.custom_weight in should: %s", shouldStr)
	}

	// KNN query has NO rank features
	knn := queries[1].(map[string]interface{})["knn"].(map[string]interface{})
	knnJSON, _ := json.Marshal(knn)
	knnStr := string(knnJSON)
	if contains(knnStr, "rank_feature") || contains(knnStr, "pagerank_fea") {
		t.Fatalf("CRITICAL: rank_feature leaked into knn query: %s", knnStr)
	}
}

// TestDSL_SkillIndex_And_MemoryIndex_Mappings tests skill and memory index mappings.
func TestDSL_SkillIndex_And_MemoryIndex_Mappings(t *testing.T) {
	// 1. Skill Index
	reqSkill := &types.SearchRequest{
		IndexNames: []string{"skill_index"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "weather"},
		},
	}
	dslSkill, _, err := buildSearchDSL(context.Background(), reqSkill)
	if err != nil {
		t.Fatalf("buildSearchDSL skill failed: %v", err)
	}
	skillJSON, _ := json.Marshal(dslSkill)
	skillStr := string(skillJSON)
	if !contains(skillStr, `"term":{"status":"1"}`) {
		t.Fatalf("expected status=1 term filter for skill index: %s", skillStr)
	}
	if !contains(skillStr, "name_tks^10") {
		t.Fatalf("expected skill fields in query_string: %s", skillStr)
	}

	// 2. Memory Index
	reqMemory := &types.SearchRequest{
		IndexNames: []string{"memory_index"},
		KbIDs:      []string{"mem_123"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "hello"},
		},
	}
	dslMemory, _, err := buildSearchDSL(context.Background(), reqMemory)
	if err != nil {
		t.Fatalf("buildSearchDSL memory failed: %v", err)
	}
	memoryJSON, _ := json.Marshal(dslMemory)
	memoryStr := string(memoryJSON)
	if !contains(memoryStr, `"terms":{"memory_id":["mem_123"]}`) {
		t.Fatalf("expected memory_id terms filter for memory index: %s", memoryStr)
	}
	if !contains(memoryStr, "tokenized_content_ltks") {
		t.Fatalf("expected tokenized_content_ltks in query_string for memory index: %s", memoryStr)
	}
}

// TestDSL_LowerCaseQueryText_PreservesOperators verifies lowercasing preserving operators.
func TestDSL_LowerCaseQueryText_PreservesOperators(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"Cat AND Dog", "cat AND dog"},
		{"Apple OR Orange", "apple OR orange"},
		{"NOT Banana", "NOT banana"},
		{"\"Quoted Phrase\"", "\"quoted phrase\""},
		{"Mixed AND \"Phrase With AND Inside\"", "mixed AND \"phrase with and inside\""},
	}

	for _, c := range cases {
		got := lowerCaseQueryText(c.input)
		if got != c.expected {
			t.Errorf("lowerCaseQueryText(%q) = %q, expected %q", c.input, got, c.expected)
		}
	}
}

func TestDSL_NilRequest_ReturnsError(t *testing.T) {
	_, _, err := buildSearchDSL(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error on nil request, got nil")
	}
	weights, ok := extractFusionWeights(nil)
	if ok || weights[0] != 0.5 || weights[1] != 0.5 {
		t.Fatalf("expected default [0.5, 0.5], false on nil request, got: %v, %v", weights, ok)
	}
}

func TestDSL_SelectFields_Immutability_NoDataRace(t *testing.T) {
	origFields := []string{"content"}
	req := &types.SearchRequest{
		IndexNames:   []string{"kb_test"},
		SelectFields: origFields,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "test"},
		},
	}

	dsl, _, err := buildSearchDSL(context.Background(), req)
	if err != nil {
		t.Fatalf("buildSearchDSL failed: %v", err)
	}

	// Verify original slice was not mutated in place
	if len(origFields) != 1 || origFields[0] != "content" {
		t.Fatalf("original req.SelectFields was mutated! len=%d, val=%v", len(origFields), origFields)
	}

	// Verify dsl _source contains pagerank_fea and tag_feas
	source, ok := dsl["_source"].([]string)
	if !ok || len(source) != 3 {
		t.Fatalf("expected 3 fields in _source, got: %v", source)
	}
	if !slices.Contains(source, common.PAGERANK_FLD) || !slices.Contains(source, common.TAG_FLD) {
		t.Fatalf("expected pagerank_fea and tag_feas in source: %v", source)
	}
}

func TestDSL_ZeroWeights_ClampedAboveZero(t *testing.T) {
	// Zero weight: "0,1" -> clamped to [0.0001, 0.9999]
	weights := NormalizeWeights([]float64{0.0, 1.0})
	if weights[0] <= 0.0 || weights[1] <= 0.0 {
		t.Fatalf("weights must be strictly > 0: %v", weights)
	}
	if math.Abs((weights[0]+weights[1])-1.0) > 1e-9 {
		t.Fatalf("weights must sum to 1.0: %v", weights)
	}
	if weights[0] != 0.0001 || weights[1] != 0.9999 {
		t.Fatalf("expected [0.0001, 0.9999], got: %v", weights)
	}
}

func TestDSL_EmptyVectorColumn_Fallback(t *testing.T) {
	req := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				VectorColumnName: "",
				EmbeddingData:    make([]float64, 512),
			},
		},
	}

	dsl, _, err := buildSearchDSL(context.Background(), req)
	if err != nil {
		t.Fatalf("buildSearchDSL failed: %v", err)
	}
	query := dsl["query"].(map[string]interface{})
	knn := query["knn"].(map[string]interface{})
	if knn["q_512_vec"] == nil {
		t.Fatalf("expected fallback vector column q_512_vec, got: %v", knn)
	}
}

func TestDSL_EnsureSearchAfterSort_IdVsUnderscoreId(t *testing.T) {
	// 1. Sort with {"id": "asc"} should still append {"_id": "asc"}
	sortWithDocID := []map[string]interface{}{
		{"id": "asc"},
	}
	result := ensureSearchAfterSort(sortWithDocID)
	if len(result) != 2 || result[1]["_id"] != "asc" {
		t.Fatalf("expected _id: asc appended after id, got: %v", result)
	}

	// 2. Sort already ending with {"_id": "asc"} remains unchanged
	sortWithMetaID := []map[string]interface{}{
		{"_id": "asc"},
	}
	result2 := ensureSearchAfterSort(sortWithMetaID)
	if len(result2) != 1 {
		t.Fatalf("expected sort with _id: asc unchanged, got: %v", result2)
	}
}

func TestDSL_EmptyChildFilter_NoNull(t *testing.T) {
	req := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		Filter: map[string]interface{}{
			"and": []interface{}{
				map[string]interface{}{},
				map[string]interface{}{"doc_id": "123"},
			},
		},
	}

	dsl, _, err := buildSearchDSL(context.Background(), req)
	if err != nil {
		t.Fatalf("buildSearchDSL failed: %v", err)
	}
	bodyJSON, _ := json.Marshal(dsl)
	bodyStr := string(bodyJSON)
	if contains(bodyStr, "null") {
		t.Fatalf("DSL contains null child filter clause: %s", bodyStr)
	}
}

func TestDSL_ExtractFusionWeights_SliceVariants(t *testing.T) {
	// Float slice
	reqFloat := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		MatchExprs: []interface{}{
			&types.FusionExpr{
				Method: "weighted_sum",
				FusionParams: map[string]interface{}{
					"weights": []float64{0.2, 0.8},
				},
			},
		},
	}
	w1, ok1 := extractFusionWeights(reqFloat)
	if !ok1 || math.Abs(w1[0]-0.2) > 1e-4 || math.Abs(w1[1]-0.8) > 1e-4 {
		t.Fatalf("expected [0.2, 0.8], got: %v, %v", w1, ok1)
	}

	// Any slice
	reqAny := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		MatchExprs: []interface{}{
			&types.FusionExpr{
				Method: "weighted_sum",
				FusionParams: map[string]interface{}{
					"weights": []interface{}{0.3, 0.7},
				},
			},
		},
	}
	w2, ok2 := extractFusionWeights(reqAny)
	if !ok2 || math.Abs(w2[0]-0.3) > 1e-4 || math.Abs(w2[1]-0.7) > 1e-4 {
		t.Fatalf("expected [0.3, 0.7], got: %v, %v", w2, ok2)
	}
}

func TestDSL_HybridPaginationDepth_And_KNN_K(t *testing.T) {
	req := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		Offset:     20,
		Limit:      10,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				MatchingText: "database query",
				Fields:       []string{"content_ltks"},
			},
			&types.MatchDenseExpr{
				EmbeddingData: []float64{0.1, 0.2, 0.3},
			},
		},
	}

	dsl, _, err := buildSearchDSL(context.Background(), req)
	if err != nil {
		t.Fatalf("buildSearchDSL failed: %v", err)
	}

	query, ok := dsl["query"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected query object: %v", dsl)
	}
	hybrid, ok := query["hybrid"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected hybrid query: %v", query)
	}
	expectedDepth := common.MAX_RESULT_WINDOW
	if depth, ok := hybrid["pagination_depth"].(int); !ok || depth != expectedDepth {
		t.Fatalf("expected pagination_depth=%d for 2.19+ when offset>0, got %v", expectedDepth, hybrid["pagination_depth"])
	}

	queries, ok := hybrid["queries"].([]interface{})
	if !ok || len(queries) != 2 {
		t.Fatalf("expected 2 subqueries, got %v", queries)
	}
	knnQuery, ok := queries[1].(map[string]interface{})
	if !ok {
		t.Fatalf("expected knn query at queries[1]: %v", queries[1])
	}
	knnClause, ok := knnQuery["knn"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected knn wrapper in knnQuery: %v", knnQuery)
	}
	qVec, ok := knnClause["q_3_vec"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected q_3_vec in knnQuery: %v", knnClause)
	}
	// Default k in hybrid search should match expectedDepth to keep candidate pool stable across pages
	if k, ok := qVec["k"].(int); !ok || k != expectedDepth {
		t.Fatalf("expected knn k=expectedDepth (%d), got %v", expectedDepth, qVec["k"])
	}

	// Page 1 (offset=0): pagination_depth should be set consistently across all pages per OpenSearch hybrid pagination docs
	reqPage1 := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		Offset:     0,
		Limit:      10,
		MatchExprs: req.MatchExprs,
	}
	dslPage1, _, err := buildSearchDSL(context.Background(), reqPage1)
	if err != nil {
		t.Fatalf("buildSearchDSL page 1 failed: %v", err)
	}
	query1 := dslPage1["query"].(map[string]interface{})
	hybrid1 := query1["hybrid"].(map[string]interface{})
	expectedPage1Depth := common.MAX_RESULT_WINDOW
	if hybrid1["pagination_depth"] != expectedPage1Depth {
		t.Fatalf("page 1 (offset=0) should set pagination_depth=%d: %v", expectedPage1Depth, hybrid1)
	}
	// Verify pagination depth is stable across pages (page 1 and page 3 use the same depth)
	if hybrid1["pagination_depth"] != hybrid["pagination_depth"] {
		t.Fatalf("expected stable pagination_depth across pages, got page1=%v, page3=%v", hybrid1["pagination_depth"], hybrid["pagination_depth"])
	}

	// Reviewer case: TopN=10, limit=10: Page 1 and Page 2 must have identical pagination_depth
	reqReviewerP1 := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		Offset:     0,
		Limit:      10,
		MatchExprs: []interface{}{
			req.MatchExprs[0],
			&types.MatchDenseExpr{
				VectorColumnName: "q_3_vec",
				EmbeddingData:    []float64{0.1, 0.2, 0.3},
				TopN:             10,
			},
		},
	}
	reqReviewerP2 := &types.SearchRequest{
		IndexNames: []string{"kb_test"},
		Offset:     10,
		Limit:      10,
		MatchExprs: reqReviewerP1.MatchExprs,
	}
	dslRevP1, _, errRevP1 := buildSearchDSL(context.Background(), reqReviewerP1)
	if errRevP1 != nil {
		t.Fatalf("buildSearchDSL reviewer page 1 failed: %v", errRevP1)
	}
	dslRevP2, _, errRevP2 := buildSearchDSL(context.Background(), reqReviewerP2)
	if errRevP2 != nil {
		t.Fatalf("buildSearchDSL reviewer page 2 failed: %v", errRevP2)
	}
	depthRevP1 := dslRevP1["query"].(map[string]interface{})["hybrid"].(map[string]interface{})["pagination_depth"]
	depthRevP2 := dslRevP2["query"].(map[string]interface{})["hybrid"].(map[string]interface{})["pagination_depth"]
	if depthRevP1 != depthRevP2 {
		t.Fatalf("expected identical pagination_depth between page 1 and page 2, got p1=%v, p2=%v", depthRevP1, depthRevP2)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || (len(s) > 0 && len(substr) > 0 && (s[:len(substr)] == substr || (len(s) > len(substr) && searchSubstr(s, substr)))))
}

func searchSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestBuildSearchDSL_VectorOutOfBoundsRejected(t *testing.T) {
	// 1. Vector query within window: offset 9990, limit 10 -> sum 10000 <= 10000 -> succeeds
	reqOK := &types.SearchRequest{
		IndexNames: []string{"test_idx"},
		Offset:     9990,
		Limit:      10,
		MatchExprs: []interface{}{
			&types.MatchDenseExpr{
				EmbeddingData: []float64{0.1, 0.2, 0.3},
			},
		},
	}
	dslOK, useSearchAfter, err := buildSearchDSL(context.Background(), reqOK)
	if err != nil {
		t.Fatalf("expected vector query within window to succeed, got: %v", err)
	}
	if dslOK == nil || useSearchAfter {
		t.Fatalf("unexpected dslOK=%v, useSearchAfter=%v", dslOK, useSearchAfter)
	}

	// 2. Vector query beyond window: offset 9995, limit 10 -> sum 10005 > 10000 -> fails immediately
	reqBeyond := &types.SearchRequest{
		IndexNames: []string{"test_idx"},
		Offset:     9995,
		Limit:      10,
		MatchExprs: reqOK.MatchExprs,
	}
	_, _, errBeyond := buildSearchDSL(context.Background(), reqBeyond)
	if errBeyond == nil {
		t.Fatalf("expected vector query beyond window to fail immediately, got nil")
	}
	if !strings.Contains(errBeyond.Error(), "vector search does not support offset + limit 10005 beyond max_result_window (10000)") {
		t.Errorf("unexpected error message: %v", errBeyond)
	}

	// 3. Hybrid query beyond window: offset 9995, limit 10 -> sum 10005 > 10000 -> fails
	reqHybridBeyond := &types.SearchRequest{
		IndexNames: []string{"test_idx"},
		Offset:     9995,
		Limit:      10,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "hello"},
			&types.MatchDenseExpr{EmbeddingData: []float64{0.1, 0.2}},
		},
	}
	_, _, errHybridBeyond := buildSearchDSL(context.Background(), reqHybridBeyond)
	if errHybridBeyond == nil {
		t.Fatalf("expected hybrid query beyond window to fail immediately, got nil")
	}
	if !strings.Contains(errHybridBeyond.Error(), "vector search does not support offset + limit 10005 beyond max_result_window (10000)") {
		t.Errorf("unexpected error message: %v", errHybridBeyond)
	}
}

func TestBuildSearchDSL_TextOutOfBoundsUsesSearchAfter(t *testing.T) {
	// Non-vector (text) query beyond window: offset 9995, limit 10 -> useSearchAfter = true
	req := &types.SearchRequest{
		IndexNames: []string{"test_idx"},
		Offset:     9995,
		Limit:      10,
		MatchExprs: []interface{}{
			&types.MatchTextExpr{MatchingText: "database query"},
		},
	}
	dsl, useSearchAfter, err := buildSearchDSL(context.Background(), req)
	if err != nil {
		t.Fatalf("expected non-vector query beyond window to succeed, got: %v", err)
	}
	if !useSearchAfter {
		t.Fatalf("expected useSearchAfter=true for non-vector query beyond window")
	}
	if dsl["sort"] == nil {
		t.Fatalf("expected sort clause to be attached for search_after, got nil")
	}
}
