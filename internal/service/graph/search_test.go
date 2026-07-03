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
	"testing"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	modelModule "ragflow/internal/entity/models"
)

// --- buildEntitySearchRequest ---

func TestBuildEntitySearchRequest_TextOnly(t *testing.T) {
	req := buildEntitySearchRequest([]string{"kb1"}, "Elon Musk", nil, 10)
	if req == nil {
		t.Fatal("expected non-nil request")
	}
	if req.Filter["knowledge_graph_kwd"] != "entity" {
		t.Errorf("expected 'entity' filter, got %v", req.Filter["knowledge_graph_kwd"])
	}
	if req.Limit != 10 {
		t.Errorf("expected limit 10, got %d", req.Limit)
	}
	if len(req.MatchExprs) != 1 {
		t.Fatalf("expected 1 MatchExpr (text only), got %d", len(req.MatchExprs))
	}
	if _, ok := req.MatchExprs[0].(*types.MatchTextExpr); !ok {
		t.Error("expected MatchTextExpr")
	}
}

func TestBuildEntitySearchRequest_EmptyQuestion(t *testing.T) {
	req := buildEntitySearchRequest([]string{"kb1"}, "", nil, 10)
	if len(req.MatchExprs) != 0 {
		t.Error("expected no MatchExprs for empty question")
	}
}

func TestBuildEntitySearchRequest_Hybrid(t *testing.T) {
	dense := &types.MatchDenseExpr{
		VectorColumnName:  "q_768_vec",
		EmbeddingData:     []float64{0.1, 0.2, 0.3},
		EmbeddingDataType: "float",
		DistanceType:      "cosine",
		TopN:              10,
	}
	req := buildEntitySearchRequest([]string{"kb1"}, "test question", dense, 10)
	if len(req.MatchExprs) != 3 {
		t.Fatalf("expected 3 MatchExprs (dense + text + fusion), got %d", len(req.MatchExprs))
	}
	if _, ok := req.MatchExprs[0].(*types.MatchDenseExpr); !ok {
		t.Error("expected MatchDenseExpr at [0]")
	}
	if _, ok := req.MatchExprs[1].(*types.MatchTextExpr); !ok {
		t.Error("expected MatchTextExpr at [1]")
	}
	fusion, ok := req.MatchExprs[2].(*types.FusionExpr)
	if !ok {
		t.Fatal("expected FusionExpr at [2]")
	}
	if fusion.Method != "weighted_sum" {
		t.Errorf("expected 'weighted_sum', got %q", fusion.Method)
	}
}

// --- buildEntityTypeSearchRequest ---

func TestBuildEntityTypeSearchRequest_Basic(t *testing.T) {
	req := buildEntityTypeSearchRequest([]string{"kb1"}, []string{"PERSON", "ORGANIZATION"}, 10)
	if req.Filter["knowledge_graph_kwd"] != "entity" {
		t.Errorf("expected 'entity' filter, got %v", req.Filter["knowledge_graph_kwd"])
	}
	filter, ok := req.Filter["entity_type_kwd"].([]interface{})
	if !ok || len(filter) != 2 {
		t.Errorf("expected 2 entity_type filters, got %v", filter)
	}
}

func TestBuildEntityTypeSearchRequest_EmptyTypes(t *testing.T) {
	req := buildEntityTypeSearchRequest([]string{"kb1"}, nil, 10)
	if _, ok := req.Filter["entity_type_kwd"]; ok {
		t.Error("expected no entity_type_kwd filter for empty types")
	}
}

// --- buildRelationSearchRequest ---

func TestBuildRelationSearchRequest_Basic(t *testing.T) {
	req := buildRelationSearchRequest([]string{"kb1"}, "test", nil, 10)
	if req.Filter["knowledge_graph_kwd"] != "relation" {
		t.Errorf("expected 'relation' filter, got %v", req.Filter["knowledge_graph_kwd"])
	}
}

func TestBuildRelationSearchRequest_EmptyQuestion(t *testing.T) {
	req := buildRelationSearchRequest([]string{"kb1"}, "", nil, 10)
	if len(req.MatchExprs) != 0 {
		t.Error("expected no MatchExprs for empty question")
	}
}

func TestBuildRelationSearchRequest_Hybrid(t *testing.T) {
	dense := &types.MatchDenseExpr{
		VectorColumnName: "q_768_vec",
		EmbeddingData:    []float64{0.1, 0.2},
		TopN:             5,
	}
	req := buildRelationSearchRequest([]string{"kb1"}, "test", dense, 5)
	if len(req.MatchExprs) != 3 {
		t.Fatalf("expected 3 MatchExprs (dense + text + fusion), got %d", len(req.MatchExprs))
	}
}

// --- buildCommunitySearchRequest ---

func TestBuildCommunitySearchRequest_Basic(t *testing.T) {
	req := buildCommunitySearchRequest([]string{"kb1"}, []string{"Elon Musk"}, 5)
	if req.Filter["knowledge_graph_kwd"] != "community_report" {
		t.Errorf("expected 'community_report' filter, got %v", req.Filter["knowledge_graph_kwd"])
	}
	if req.OrderBy == nil {
		t.Error("expected OrderBy")
	}
}

func TestBuildCommunitySearchRequest_EmptyNames(t *testing.T) {
	req := buildCommunitySearchRequest([]string{"kb1"}, nil, 5)
	if _, ok := req.Filter["entities_kwd"]; ok {
		t.Error("expected no entities_kwd filter for empty names")
	}
}

// --- buildTypeSamplesSearchRequest ---

func TestBuildTypeSamplesSearchRequest(t *testing.T) {
	req := buildTypeSamplesSearchRequest([]string{"kb1"})
	if req.Filter["knowledge_graph_kwd"] != "ty2ents" {
		t.Errorf("expected 'ty2ents' filter, got %v", req.Filter["knowledge_graph_kwd"])
	}
	if req.Limit != 10000 {
		t.Errorf("expected 10000, got %d", req.Limit)
	}
}

// --- ParseEntityChunks ---

func TestParseEntityChunks_Basic(t *testing.T) {
	chunks := []map[string]interface{}{
		{"entity_kwd": "Elon Musk", "entity_type_kwd": "PERSON", "rank_flt": 0.9, "_score": 0.85, "content_with_weight": "Founder of SpaceX"},
	}
	entities := ParseEntityChunks(chunks)
	if len(entities) != 1 {
		t.Fatalf("expected 1, got %d", len(entities))
	}
	if entities[0].Name != "Elon Musk" || entities[0].Type != "PERSON" || entities[0].PageRank != 0.9 || entities[0].Similarity != 0.85 {
		t.Errorf("unexpected entity fields: %+v", entities[0])
	}
}

func TestParseEntityChunks_List(t *testing.T) {
	chunks := []map[string]interface{}{
		{"entity_kwd": []interface{}{"Elon Musk", "elon_musk"}},
	}
	entities := ParseEntityChunks(chunks)
	if len(entities) != 1 || entities[0].Name != "Elon Musk" {
		t.Errorf("expected first list element, got %q", entities[0].Name)
	}
}

func TestParseEntityChunks_EmptyName(t *testing.T) {
	chunks := []map[string]interface{}{{"entity_type_kwd": "PERSON"}}
	if len(ParseEntityChunks(chunks)) != 0 {
		t.Error("expected 0 for missing name")
	}
}

func TestParseEntityChunks_ScoreFallback(t *testing.T) {
	chunks := []map[string]interface{}{{"entity_kwd": "Test", "score": 0.75}}
	if ParseEntityChunks(chunks)[0].Similarity != 0.75 {
		t.Error("expected 0.75 from score field")
	}
}

func TestParseEntityChunks_NilInput(t *testing.T) {
	if len(ParseEntityChunks(nil)) != 0 {
		t.Error("expected 0 for nil input")
	}
}

// --- ParseRelationChunks ---

func TestParseRelationChunks_Basic(t *testing.T) {
	chunks := []map[string]interface{}{
		{"from_entity_kwd": "Elon Musk", "to_entity_kwd": "SpaceX", "weight_int": float64(5), "content_with_weight": "Founder"},
	}
	relations := ParseRelationChunks(chunks)
	if len(relations) != 1 || relations[0].From != "Elon Musk" || relations[0].PageRank != 5 {
		t.Errorf("unexpected: %+v", relations[0])
	}
}

func TestParseRelationChunks_IntWeight(t *testing.T) {
	chunks := []map[string]interface{}{{"from_entity_kwd": "A", "to_entity_kwd": "B", "weight_int": 3}}
	if ParseRelationChunks(chunks)[0].PageRank != 3 {
		t.Error("expected weight 3")
	}
}

func TestParseRelationChunks_EmptyFrom(t *testing.T) {
	if len(ParseRelationChunks([]map[string]interface{}{{"to_entity_kwd": "B"}})) != 0 {
		t.Error("expected 0 for missing from")
	}
}

func TestParseRelationChunks_NilInput(t *testing.T) {
	if len(ParseRelationChunks(nil)) != 0 {
		t.Error("expected 0 for nil")
	}
}

// --- ParseCommunityReportChunks ---

func TestParseCommunityReportChunks_Basic(t *testing.T) {
	chunks := []map[string]interface{}{
		{"docnm_kwd": "Report 1", "content_with_weight": "content", "weight_flt": 0.95, "entities_kwd": "A, B"},
	}
	reports := ParseCommunityReportChunks(chunks)
	if len(reports) != 1 || reports[0].Title != "Report 1" || reports[0].Weight != 0.95 {
		t.Errorf("unexpected: %+v", reports[0])
	}
}

func TestParseCommunityReportChunks_EmptyTitle(t *testing.T) {
	if len(ParseCommunityReportChunks([]map[string]interface{}{{"weight_flt": 0.5}})) != 0 {
		t.Error("expected 0 for empty title and content")
	}
}

func TestParseCommunityReportChunks_NilInput(t *testing.T) {
	if len(ParseCommunityReportChunks(nil)) != 0 {
		t.Error("expected 0 for nil")
	}
}

// --- ParseTypeSamplesChunks ---

func TestParseTypeSamplesChunks_ValidJSON(t *testing.T) {
	chunks := []map[string]interface{}{
		{"content_with_weight": `{"PERSON": ["Elon Musk", "Einstein"], "ORGANIZATION": ["SpaceX"]}`},
	}
	result := ParseTypeSamplesChunks(chunks)
	if len(result) != 2 {
		t.Fatalf("expected 2 types, got %d: %v", len(result), result)
	}
	if len(result["PERSON"]) != 2 || result["PERSON"][0] != "Elon Musk" {
		t.Errorf("expected PERSON entities, got %v", result["PERSON"])
	}
	if len(result["ORGANIZATION"]) != 1 || result["ORGANIZATION"][0] != "SpaceX" {
		t.Errorf("expected ORGANIZATION entities, got %v", result["ORGANIZATION"])
	}
}

func TestParseTypeSamplesChunks_InvalidJSON(t *testing.T) {
	chunks := []map[string]interface{}{
		{"content_with_weight": "not json"},
	}
	result := ParseTypeSamplesChunks(chunks)
	if len(result) != 0 {
		t.Error("expected empty for invalid JSON")
	}
}

func TestParseTypeSamplesChunks_Empty(t *testing.T) {
	result := ParseTypeSamplesChunks(nil)
	if len(result) != 0 {
		t.Error("expected empty for nil")
	}
}

// --- NhopEntityNames ---

func TestNhopEntityNames_ValidJSON(t *testing.T) {
	input := `[{"path": ["A", "B", "C"], "weights": [0.8, 0.5]}, {"path": ["C", "D"], "weights": [0.3]}]`
	names := NhopEntityNames(input)
	if len(names) != 4 {
		t.Fatalf("expected 4 unique names, got %d: %v", len(names), names)
	}
}

func TestNhopEntityNames_Dedup(t *testing.T) {
	input := `[{"path": ["A", "B"], "weights": [0.9]}, {"path": ["A", "C"], "weights": [0.8]}]`
	names := NhopEntityNames(input)
	if len(names) != 3 {
		t.Errorf("expected 3 unique names (A,B,C), got %d: %v", len(names), names)
	}
}

func TestNhopEntityNames_InvalidJSON(t *testing.T) {
	result := NhopEntityNames("not json")
	if result != nil {
		t.Error("expected nil for invalid JSON")
	}
}

// --- Mock engine ---

type mockKGEngine struct {
	engine.DocEngine
	searchFunc func(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error)
}

func (m *mockKGEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
	if m.searchFunc != nil {
		return m.searchFunc(ctx, req)
	}
	return &types.SearchResult{}, nil
}

// --- Mock model driver for Embed tests ---

type fakeEmbedDriver struct {
	modelModule.ModelDriver
	name   string
	vector []float64
}

func (f *fakeEmbedDriver) Name() string { return f.name }

func (f *fakeEmbedDriver) Embed(modelName *string, texts []string, apiConfig *modelModule.APIConfig, config *modelModule.EmbeddingConfig) ([]modelModule.EmbeddingData, error) {
	return []modelModule.EmbeddingData{{Embedding: f.vector}}, nil
}

func TestBuildKGDenseExpr_WithModel(t *testing.T) {
	embModel := &modelModule.EmbeddingModel{
		ModelName: strPtr("test-model"),
		ModelDriver: &fakeEmbedDriver{
			name:   "test",
			vector: []float64{0.1, 0.2, 0.3},
		},
		APIConfig: &modelModule.APIConfig{},
	}
	dense, err := buildDenseExpr(embModel, "test question", 10)
	if err != nil {
		t.Fatalf("buildDenseExpr failed: %v", err)
	}
	if dense == nil {
		t.Fatal("expected non-nil MatchDenseExpr")
	}
	if dense.VectorColumnName != "q_3_vec" {
		t.Errorf("expected 'q_3_vec', got %q", dense.VectorColumnName)
	}
	if dense.TopN != 10 {
		t.Errorf("expected TopN 10, got %d", dense.TopN)
	}
}

func TestBuildKGDenseExpr_NilModel(t *testing.T) {
	dense, err := buildDenseExpr(nil, "test", 10)
	if dense != nil || err != nil {
		t.Errorf("expected nil,nil for nil model, got dense=%v err=%v", dense, err)
	}
}

func TestBuildKGDenseExpr_EmptyQuestion(t *testing.T) {
	dense, err := buildDenseExpr(&modelModule.EmbeddingModel{}, "", 10)
	if dense != nil || err != nil {
		t.Errorf("expected nil,nil for empty question, got dense=%v err=%v", dense, err)
	}
}

// --- Search integration with mock ---

func TestSearchEntities_WithMock(t *testing.T) {
	mock := &mockKGEngine{
		searchFunc: func(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
			if req.Filter["knowledge_graph_kwd"] != "entity" {
				t.Error("expected entity filter")
			}
			return &types.SearchResult{
				Chunks: []map[string]interface{}{
					{"entity_kwd": "Elon Musk", "entity_type_kwd": "PERSON"},
				},
			}, nil
		},
	}
	entities, err := SearchEntities(context.Background(), mock, []string{"kb1"}, "Elon", nil, 10)
	if err != nil {
		t.Fatalf("SearchEntities failed: %v", err)
	}
	if len(entities) != 1 || entities[0].Name != "Elon Musk" {
		t.Errorf("expected [Elon Musk], got %v", entities)
	}
}

func TestSearchEntitiesByTypes_WithMock(t *testing.T) {
	mock := &mockKGEngine{
		searchFunc: func(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
			return &types.SearchResult{
				Chunks: []map[string]interface{}{
					{"entity_kwd": "SpaceX", "entity_type_kwd": "ORGANIZATION"},
				},
			}, nil
		},
	}
	entities, err := SearchEntitiesByTypes(context.Background(), mock, []string{"kb1"}, []string{"ORGANIZATION"}, 10)
	if err != nil {
		t.Fatalf("SearchEntitiesByTypes failed: %v", err)
	}
	if len(entities) != 1 || entities[0].Type != "ORGANIZATION" {
		t.Errorf("expected ORGANIZATION, got %v", entities)
	}
}

func TestSearchTypeSamples_WithMock(t *testing.T) {
	mock := &mockKGEngine{}
	samples, err := SearchTypeSamples(context.Background(), mock, []string{"kb1"})
	if err != nil {
		t.Fatalf("SearchTypeSamples failed: %v", err)
	}
	if samples == nil {
		samples = map[string][]string{}
	}
	if len(samples) != 0 {
		t.Errorf("expected empty, got %d", len(samples))
	}
}
