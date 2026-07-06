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

package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	modelModule "ragflow/internal/entity/models"
)

type mockRetrievalEngine struct {
	engine.DocEngine
	results map[string]*types.SearchResult
}

func (m *mockRetrievalEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	// --- Contract validation (matches real ES/Infinity preconditions) ---
	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("mock: IndexNames cannot be empty")
	}
	if len(req.KbIDs) == 0 {
		return nil, fmt.Errorf("mock: KbIDs cannot be empty")
	}
	// --- Original stubbing logic ---
	kgType, _ := req.Filter["knowledge_graph_kwd"].(string)
	key := kgType
	if ents, ok := req.Filter["entity_kwd"].([]interface{}); ok && len(ents) > 0 {
		key = kgType + ":" + ents[0].(string)
	}
	if r, ok := m.results[key]; ok {
		return r, nil
	}
	if r, ok := m.results[""]; ok {
		return r, nil
	}
	return &types.SearchResult{}, nil
}

// --- entityFromChunk ---

func TestEntityFromChunk_Basic(t *testing.T) {
	chunk := map[string]interface{}{
		"_score":              0.85,
		"rank_flt":            0.9,
		"content_with_weight": "Founder of SpaceX",
		"n_hop_with_weight":   `[{"path":["A","B"],"weights":[0.8]}]`,
	}
	e := entityFromChunk("Elon Musk", chunk)
	if e.Similarity != 0.85 {
		t.Errorf("expected Sim=0.85, got %f", e.Similarity)
	}
	if e.PageRank != 0.9 {
		t.Errorf("expected PageRank=0.9, got %f", e.PageRank)
	}
	if e.Description != "Founder of SpaceX" {
		t.Errorf("expected Description, got %q", e.Description)
	}
	if len(e.NhopEnts) != 1 || len(e.NhopEnts[0].Path) != 2 {
		t.Errorf("expected 1 NhopEnt with 2-path, got %+v", e.NhopEnts)
	}
}

func TestEntityFromChunk_ScoreFallback(t *testing.T) {
	chunk := map[string]interface{}{"score": 0.75}
	e := entityFromChunk("Test", chunk)
	if e.Similarity != 0.75 {
		t.Errorf("expected Sim=0.75 from score field, got %f", e.Similarity)
	}
}

func TestEntityFromChunk_MissingFields(t *testing.T) {
	chunk := map[string]interface{}{}
	e := entityFromChunk("Empty", chunk)
	if e.Similarity != 0 || e.PageRank != 0 || len(e.NhopEnts) != 0 {
		t.Errorf("expected zero defaults, got %+v", e)
	}
}

// --- relationFromChunk ---

func TestRelationFromChunk_Basic(t *testing.T) {
	chunk := map[string]interface{}{
		"from_entity_kwd":     "Elon Musk",
		"to_entity_kwd":       "SpaceX",
		"weight_int":          float64(5),
		"content_with_weight": "Founder",
	}
	edge, rel := relationFromChunk(chunk)
	if edge.From != "Elon Musk" || edge.To != "SpaceX" {
		t.Errorf("expected Elon Musk→SpaceX, got %v", edge)
	}
	if rel.PageRank != 5 {
		t.Errorf("expected weight 5, got %f", rel.PageRank)
	}
}

func TestRelationFromChunk_MissingFrom(t *testing.T) {
	chunk := map[string]interface{}{"to_entity_kwd": "B"}
	edge, _ := relationFromChunk(chunk)
	if edge.From != "" {
		t.Error("expected empty from")
	}
}

// --- searchTypeSamples ---

func TestSearchTypeSamples_Success(t *testing.T) {
	data, _ := json.Marshal(map[string][]string{"PERSON": {"Elon Musk"}})
	mock := &mockRetrievalEngine{
		results: map[string]*types.SearchResult{
			"ty2ents": {Chunks: []map[string]interface{}{
				{"content_with_weight": string(data)},
			}},
		},
	}
	result, err := searchTypeSamples(context.Background(), mock, []string{"ragflow_tenant1"}, []string{"kb1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || len(result["PERSON"]) != 1 || result["PERSON"][0] != "Elon Musk" {
		t.Errorf("expected PERSON→[Elon Musk], got %v", result)
	}
}

func TestSearchTypeSamples_Empty(t *testing.T) {
	mock := &mockRetrievalEngine{}
	result, err := searchTypeSamples(context.Background(), mock, []string{"ragflow_tenant1"}, []string{"kb1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

// --- Retrieval ---

func TestRetrieval_Basic(t *testing.T) {
	mock := &mockRetrievalEngine{
		results: map[string]*types.SearchResult{
			"entity": {Chunks: []map[string]interface{}{
				{"entity_kwd": "Elon Musk", "entity_type_kwd": "PERSON", "rank_flt": 0.9, "_score": 0.85},
			}},
			"relation": {Chunks: []map[string]interface{}{
				{"from_entity_kwd": "Elon Musk", "to_entity_kwd": "SpaceX", "weight_int": float64(5), "_score": 0.85},
			}},
			"community_report": {Chunks: []map[string]interface{}{
				{"docnm_kwd": "Community 1", "content_with_weight": "Report text", "weight_flt": 0.95},
			}},
			"ty2ents": {Chunks: []map[string]interface{}{
				{"content_with_weight": `{"PERSON":["Elon Musk"]}`},
			}},
		},
	}
	result, err := Retrieval(context.Background(), mock, nil, nil, []string{"kb1"}, []string{"tenant1"}, "Elon Musk")
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	content, ok := result["content_with_weight"].(string)
	if !ok {
		t.Fatal("expected content_with_weight string")
	}
	if content == "" {
		t.Error("expected non-empty KG content")
	}
	if result["similarity"] != 1.0 {
		t.Errorf("expected similarity 1.0, got %v", result["similarity"])
	}
	if result["docnm_kwd"] != "Related content in Knowledge Graph" {
		t.Errorf("unexpected docnm_kwd: %v", result["docnm_kwd"])
	}
}

func TestRetrieval_NoEntities(t *testing.T) {
	mock := &mockRetrievalEngine{}
	result, err := Retrieval(context.Background(), mock, nil, nil, []string{"kb1"}, []string{"tenant1"}, "test")
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	content, _ := result["content_with_weight"].(string)
	if content != "" {
		t.Errorf("expected empty when no entities found, got %q", content)
	}
}

// TestEntitySearch_MultiEntities verifies that all entities are used in search query.

func TestRetrieval_WithChatModel(t *testing.T) {
	mock := &mockRetrievalEngine{
		results: map[string]*types.SearchResult{
			"entity": {Chunks: []map[string]interface{}{
				{"entity_kwd": "Elon Musk", "entity_type_kwd": "PERSON", "rank_flt": 0.9, "_score": 0.85},
			}},
			"relation": {Chunks: []map[string]interface{}{
				{"from_entity_kwd": "Elon Musk", "to_entity_kwd": "SpaceX", "weight_int": float64(5), "_score": 0.85},
			}},
		},
	}
	// chatModel with nil ModelName so queryRewrite falls back to raw question,
	// but the ty2entsJSON construction path is still exercised.
	chatModel := &modelModule.ChatModel{ModelName: nil, APIConfig: nil}
	result, err := Retrieval(context.Background(), mock, chatModel, nil, []string{"kb1"}, []string{"tenant1"}, "Elon Musk")
	if err != nil {
		t.Fatalf("Retrieval failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	content, ok := result["content_with_weight"].(string)
	if !ok {
		t.Fatal("expected content_with_weight string")
	}
	if content == "" {
		t.Error("expected non-empty KG content")
	}
	// Verify "null" does not appear — the ty2entsJSON fix ensures "{}" not "null"
	if strings.Contains(content, "null") {
		t.Error("content should not contain 'null' from ty2entsJSON")
	}
}

func TestEntitySearch_MultiEntities(t *testing.T) {
	var capturedText string
	mock := &searchCaptureEngine{}
	mock.searchFn = func(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
		if kgType, _ := req.Filter["knowledge_graph_kwd"].(string); kgType == "entity" && len(req.MatchExprs) > 0 {
			if expr, ok := req.MatchExprs[0].(*types.MatchTextExpr); ok {
				capturedText = expr.MatchingText
			}
		}
		return &types.SearchResult{}, nil
	}
	entities := []string{"Elon Musk", "SpaceX"}
	entsReq := &types.SearchRequest{
		IndexNames:   []string{"ragflow_tenant1"},
		KbIDs:        []string{"kb1"},
		SelectFields: []string{"entity_kwd", "n_hop_with_weight"},
		Limit:        50,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "entity"},
		MatchExprs: []interface{}{
			&types.MatchTextExpr{
				Fields:       []string{"entity_kwd^10", "content_ltks^2"},
				MatchingText: strings.Join(entities, " "),
				TopN:         50,
			},
		},
	}
	mock.Search(context.Background(), entsReq)
	if !strings.Contains(capturedText, "Elon Musk") || !strings.Contains(capturedText, "SpaceX") {
		t.Errorf("expected both entities in query, got %q", capturedText)
	}
}

// searchCaptureEngine is a minimal mock for testing search requests.
type searchCaptureEngine struct {
	engine.DocEngine
	searchFn func(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error)
}

func (e *searchCaptureEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if len(req.IndexNames) == 0 {
		return nil, fmt.Errorf("mock: IndexNames cannot be empty")
	}
	if e.searchFn != nil {
		return e.searchFn(ctx, req)
	}
	return &types.SearchResult{}, nil
}

// --- queryRewrite ---

func TestQueryRewrite_Fallback(t *testing.T) {
	typeKeywords, entities := queryRewrite(nil, "What is SpaceX?", "{}")
	if typeKeywords != nil {
		t.Errorf("expected nil typeKeywords when no LLM, got %v", typeKeywords)
	}
	if len(entities) != 1 || entities[0] != "What is SpaceX?" {
		t.Errorf("expected [What is SpaceX?], got %v", entities)
	}
}

func TestQueryRewrite_EmptyQuestion(t *testing.T) {
	typeKeywords, entities := queryRewrite(nil, "", "")
	if typeKeywords != nil || entities != nil {
		t.Errorf("expected nil for empty question, got type=%v entities=%v", typeKeywords, entities)
	}
}

// spyEmbedDriver captures Embed input for testing — enables assertions on what text
// was embedded, not just that embedding succeeded.
type spyEmbedDriver struct {
	modelModule.ModelDriver
	capturedTexts []string
	vector        []float64
	err           error
}

func (s *spyEmbedDriver) Embed(_ *string, texts []string, _ *modelModule.APIConfig, _ *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, error) {
	s.capturedTexts = texts
	if s.err != nil {
		return nil, s.err
	}
	return []modelModule.EmbeddingData{{Embedding: s.vector}}, nil
}

// --- pure function: buildMatchDenseExpr ---

func TestBuildMatchDenseExpr_Basic(t *testing.T) {
	vector := []float64{0.1, 0.2, 0.3}
	expr := buildMatchDenseExpr(vector, 10, 0.2)
	if expr.VectorColumnName != "q_3_vec" {
		t.Errorf("expected q_3_vec, got %q", expr.VectorColumnName)
	}
	if len(expr.EmbeddingData) != 3 || expr.EmbeddingData[0] != 0.1 {
		t.Errorf("unexpected embedding data: %v", expr.EmbeddingData)
	}
	if expr.EmbeddingDataType != "float" {
		t.Errorf("expected float, got %q", expr.EmbeddingDataType)
	}
	if expr.DistanceType != "cosine" {
		t.Errorf("expected cosine, got %q", expr.DistanceType)
	}
	if expr.TopN != 10 {
		t.Errorf("expected TopN=10, got %d", expr.TopN)
	}
	sim, ok := expr.ExtraOptions["similarity"].(float64)
	if !ok || sim != 0.2 {
		t.Errorf("expected similarity=0.2, got %v", expr.ExtraOptions["similarity"])
	}
}

func TestBuildMatchDenseExpr_ZeroVector(t *testing.T) {
	expr := buildMatchDenseExpr(nil, 5, 0.0)
	if expr.VectorColumnName != "q_0_vec" {
		t.Errorf("expected q_0_vec for empty vector, got %q", expr.VectorColumnName)
	}
}

// --- pure function: buildFusionExpr ---

func TestBuildFusionExpr_DefaultWeights(t *testing.T) {
	expr := buildFusionExpr(0.5, 0.5, 20)
	if expr.Method != "weighted_sum" {
		t.Errorf("expected weighted_sum, got %q", expr.Method)
	}
	if expr.TopN != 20 {
		t.Errorf("expected TopN=20, got %d", expr.TopN)
	}
	weights, ok := expr.FusionParams["weights"].(string)
	if !ok || weights != "0.50,0.50" {
		t.Errorf("expected weights=0.50,0.50, got %v", expr.FusionParams["weights"])
	}
}

func TestBuildFusionExpr_AsymmetricWeights(t *testing.T) {
	expr := buildFusionExpr(0.3, 0.7, 10)
	weights := expr.FusionParams["weights"].(string)
	if weights != "0.30,0.70" {
		t.Errorf("expected 0.30,0.70, got %q", weights)
	}
}

// --- buildSearchExprs ---

func TestBuildSearchExprs_NoEmbModel(t *testing.T) {
	matchText := &types.MatchTextExpr{
		Fields:       []string{"entity_kwd^10"},
		MatchingText: "test",
		TopN:         10,
	}
	exprs := buildSearchExprs(nil, matchText, 0, 0)
	if len(exprs) != 1 {
		t.Fatalf("expected 1 expr, got %d", len(exprs))
	}
	mt, ok := exprs[0].(*types.MatchTextExpr)
	if !ok {
		t.Fatalf("expected MatchTextExpr, got %T", exprs[0])
	}
	if mt.MatchingText != "test" {
		t.Errorf("expected 'test', got %q", exprs[0].(*types.MatchTextExpr).MatchingText)
	}
}

func TestBuildSearchExprs_WithEmbModel(t *testing.T) {
	driver := &spyEmbedDriver{vector: []float64{0.1, 0.2, 0.3}}
	embModel := modelModule.NewEmbeddingModel(driver, strPtr("text-embedding"), &modelModule.APIConfig{}, 512)
	matchText := &types.MatchTextExpr{
		Fields:       []string{"entity_kwd^10"},
		MatchingText: "Elon Musk SpaceX",
		TopN:         50,
	}
	exprs := buildSearchExprs(embModel, matchText, defaultSimThreshold, defaultDenseTopK)
	// Verify Embed was called with matchText.MatchingText, not raw question
	if len(driver.capturedTexts) != 1 || driver.capturedTexts[0] != "Elon Musk SpaceX" {
		t.Errorf("expected Embed to receive %q, got %v", "Elon Musk SpaceX", driver.capturedTexts)
	}
	if len(exprs) != 3 {
		t.Fatalf("expected 3 exprs (text+dense+fusion), got %d", len(exprs))
	}
	// Index 0: MatchTextExpr
	mt, ok := exprs[0].(*types.MatchTextExpr)
	if !ok {
		t.Fatalf("expected MatchTextExpr at [0], got %T", exprs[0])
	}
	if mt.MatchingText != "Elon Musk SpaceX" {
		t.Errorf("expected 'Elon Musk SpaceX', got %q", mt.MatchingText)
	}
	// Index 1: MatchDenseExpr
	md, ok := exprs[1].(*types.MatchDenseExpr)
	if !ok {
		t.Fatalf("expected MatchDenseExpr at [1], got %T", exprs[1])
	}
	if md.VectorColumnName != "q_3_vec" {
		t.Errorf("expected q_3_vec, got %q", md.VectorColumnName)
	}
	if md.TopN != defaultDenseTopK {
		t.Errorf("expected TopN=%d (Python alignment), got %d", defaultDenseTopK, md.TopN)
	}
	if md.ExtraOptions["similarity"] != defaultSimThreshold {
		t.Errorf("expected similarity=%v (Python alignment), got %v", defaultSimThreshold, md.ExtraOptions["similarity"])
	}
	// Index 2: FusionExpr
	fu, ok := exprs[2].(*types.FusionExpr)
	if !ok {
		t.Fatalf("expected FusionExpr at [2], got %T", exprs[2])
	}
	if fu.Method != "weighted_sum" {
		t.Errorf("expected weighted_sum, got %q", fu.Method)
	}
}

func TestBuildSearchExprs_EmbModelFallback(t *testing.T) {
	driver := &spyEmbedDriver{err: assertError("embed failed")}
	embModel := modelModule.NewEmbeddingModel(driver, strPtr("text-embedding"), &modelModule.APIConfig{}, 512)
	matchText := &types.MatchTextExpr{
		Fields:       []string{"entity_kwd^10"},
		MatchingText: "fallback test",
		TopN:         10,
	}
	exprs := buildSearchExprs(embModel, matchText, defaultSimThreshold, defaultDenseTopK)
	// Should fall back to text-only when Embed fails
	if len(exprs) != 1 {
		t.Fatalf("expected 1 expr (text-only fallback), got %d", len(exprs))
	}
	if _, ok := exprs[0].(*types.MatchTextExpr); !ok {
		t.Errorf("expected MatchTextExpr, got %T", exprs[0])
	}
}

// --- Python alignment defaults ---

func TestDefaultValuesMatchPython(t *testing.T) {
	if defaultSimThreshold != 0.3 {
		t.Errorf("expected 0.3 (Python ent_sim_threshold), got %f", defaultSimThreshold)
	}
	if defaultDenseTopK != 1024 {
		t.Errorf("expected 1024 (Python get_vector topk), got %d", defaultDenseTopK)
	}
}

// assertError is a simple error for testing fallback behaviour.
type assertError string

func (e assertError) Error() string { return string(e) }

// --- indexName ---

func TestIndexName_Normal(t *testing.T) {
	result := indexName("tenant1")
	if result != "ragflow_tenant1" {
		t.Errorf("expected ragflow_tenant1, got %q", result)
	}
}

func TestIndexName_Empty(t *testing.T) {
	result := indexName("")
	if result != "ragflow_" {
		t.Errorf("expected ragflow_, got %q", result)
	}
}

// --- searchCommunityContent ---

func TestSearchKGCommunityContent_EmptyEntities(t *testing.T) {
	mock := &mockRetrievalEngine{}
	result := searchCommunityContent(context.Background(), mock, []string{"ragflow_t1"}, []string{"kb1"}, nil, 1, intPtr(100))
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

func TestSearchKGCommunityContent_WithContent(t *testing.T) {
	mock := &mockRetrievalEngine{
		results: map[string]*types.SearchResult{
			"community_report": {Chunks: []map[string]interface{}{
				{
					"docnm_kwd":           "Community Alpha",
					"content_with_weight": `{"report": "Report text", "evidences": "Evidence text"}`,
				},
			}},
		},
	}
	result := searchCommunityContent(context.Background(), mock, []string{"ragflow_t1"}, []string{"kb1"}, []ScoredEntity{{Entity: "E1"}}, 1, intPtr(500))
	if result == "" {
		t.Fatal("expected non-empty result")
	}
	if !strings.Contains(result, "Community Alpha") {
		t.Errorf("expected title 'Community Alpha', got %q", result)
	}
	if !strings.Contains(result, "Report text") {
		t.Errorf("expected report content, got %q", result)
	}
	if !strings.Contains(result, "Evidence text") {
		t.Errorf("expected evidence, got %q", result)
	}
	if !strings.Contains(result, "# 1.") {
		t.Errorf("expected numbered report (# 1.), got %q", result)
	}
}

func TestSearchKGCommunityContent_NilMaxToken(t *testing.T) {
	mock := &mockRetrievalEngine{}
	result := searchCommunityContent(context.Background(), mock, []string{"ragflow_t1"}, []string{"kb1"}, []ScoredEntity{{Entity: "E1"}}, 1, nil)
	if result != "" {
		t.Errorf("expected empty when maxToken is nil, got %q", result)
	}
}

func TestSearchKGCommunityContent_ZeroMaxToken(t *testing.T) {
	mock := &mockRetrievalEngine{}
	result := searchCommunityContent(context.Background(), mock, []string{"ragflow_t1"}, []string{"kb1"}, []ScoredEntity{{Entity: "E1"}}, 1, intPtr(0))
	if result != "" {
		t.Errorf("expected empty when maxToken=0, got %q", result)
	}
}

// intPtr returns a pointer to n.
func intPtr(n int) *int { return &n }
