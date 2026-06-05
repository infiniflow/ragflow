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

package service

import (
	"context"
	"encoding/json"
	"testing"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
)

type mockRetrievalEngine struct {
	engine.DocEngine
	results map[string]*types.SearchResult
}

func (m *mockRetrievalEngine) Search(ctx context.Context, req *types.SearchRequest) (*types.SearchResult, error) {
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

// --- kgEntityFromChunk ---

func TestKgEntityFromChunk_Basic(t *testing.T) {
	chunk := map[string]interface{}{
		"_score":              0.85,
		"rank_flt":            0.9,
		"content_with_weight": "Founder of SpaceX",
		"n_hop_with_weight":   `[{"path":["A","B"],"weights":[0.8]}]`,
	}
	e := kgEntityFromChunk("Elon Musk", chunk)
	if e.Sim != 0.85 {
		t.Errorf("expected Sim=0.85, got %f", e.Sim)
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

func TestKgEntityFromChunk_ScoreFallback(t *testing.T) {
	chunk := map[string]interface{}{"score": 0.75}
	e := kgEntityFromChunk("Test", chunk)
	if e.Sim != 0.75 {
		t.Errorf("expected Sim=0.75 from score field, got %f", e.Sim)
	}
}

func TestKgEntityFromChunk_MissingFields(t *testing.T) {
	chunk := map[string]interface{}{}
	e := kgEntityFromChunk("Empty", chunk)
	if e.Sim != 0 || e.PageRank != 0 || len(e.NhopEnts) != 0 {
		t.Errorf("expected zero defaults, got %+v", e)
	}
}

// --- kgRelationFromChunk ---

func TestKgRelationFromChunk_Basic(t *testing.T) {
	chunk := map[string]interface{}{
		"from_entity_kwd":    "Elon Musk",
		"to_entity_kwd":      "SpaceX",
		"weight_int":         float64(5),
		"content_with_weight": "Founder",
	}
	edge, rel := kgRelationFromChunk(chunk)
	if edge.From != "Elon Musk" || edge.To != "SpaceX" {
		t.Errorf("expected Elon Musk→SpaceX, got %v", edge)
	}
	if rel.PageRank != 5 {
		t.Errorf("expected weight 5, got %f", rel.PageRank)
	}
}

func TestKgRelationFromChunk_MissingFrom(t *testing.T) {
	chunk := map[string]interface{}{"to_entity_kwd": "B"}
	edge, _ := kgRelationFromChunk(chunk)
	if edge.From != "" {
		t.Error("expected empty from")
	}
}

// --- searchKGTypeSamples ---

func TestSearchKGTypeSamples_Success(t *testing.T) {
	data, _ := json.Marshal(map[string][]string{"PERSON": {"Elon Musk"}})
	mock := &mockRetrievalEngine{
		results: map[string]*types.SearchResult{
			"ty2ents": {Chunks: []map[string]interface{}{
				{"content_with_weight": string(data)},
			}},
		},
	}
	result, err := searchKGTypeSamples(context.Background(), mock, []string{"kb1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 || len(result["PERSON"]) != 1 || result["PERSON"][0] != "Elon Musk" {
		t.Errorf("expected PERSON→[Elon Musk], got %v", result)
	}
}

func TestSearchKGTypeSamples_Empty(t *testing.T) {
	mock := &mockRetrievalEngine{}
	result, err := searchKGTypeSamples(context.Background(), mock, []string{"kb1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty, got %v", result)
	}
}

// --- KGSearchRetrieval ---

func TestKGSearchRetrieval_Basic(t *testing.T) {
	mock := &mockRetrievalEngine{
		results: map[string]*types.SearchResult{
			"entity": {Chunks: []map[string]interface{}{
				{"entity_kwd": "Elon Musk", "entity_type_kwd": "PERSON", "rank_flt": 0.9, "_score": 0.85},
			}},
			"relation": {Chunks: []map[string]interface{}{
				{"from_entity_kwd": "Elon Musk", "to_entity_kwd": "SpaceX", "weight_int": float64(5)},
			}},
			"community_report": {Chunks: []map[string]interface{}{
				{"docnm_kwd": "Community 1", "content_with_weight": "Report text", "weight_flt": 0.95},
			}},
			"ty2ents": {Chunks: []map[string]interface{}{
				{"content_with_weight": `{"PERSON":["Elon Musk"]}`},
			}},
		},
	}
	result, err := KGSearchRetrieval(context.Background(), mock, nil, nil, []string{"kb1"}, []string{"tenant1"}, "Elon Musk")
	if err != nil {
		t.Fatalf("KGSearchRetrieval failed: %v", err)
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

func TestKGSearchRetrieval_NoEntities(t *testing.T) {
	mock := &mockRetrievalEngine{}
	result, err := KGSearchRetrieval(context.Background(), mock, nil, nil, []string{"kb1"}, []string{"tenant1"}, "test")
	if err != nil {
		t.Fatalf("KGSearchRetrieval failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	content, _ := result["content_with_weight"].(string)
	if content != "" {
		t.Errorf("expected empty when no entities found, got %q", content)
	}
}

// --- queryRewrite ---

func TestQueryRewrite_Fallback(t *testing.T) {
	typeKeywords, entities := queryRewrite(nil, "What is SpaceX?", func() string { return "{}" })
	if typeKeywords != nil {
		t.Errorf("expected nil typeKeywords when no LLM, got %v", typeKeywords)
	}
	if len(entities) != 1 || entities[0] != "What is SpaceX?" {
		t.Errorf("expected [What is SpaceX?], got %v", entities)
	}
}

func TestQueryRewrite_EmptyQuestion(t *testing.T) {
	typeKeywords, entities := queryRewrite(nil, "", nil)
	if typeKeywords != nil || entities != nil {
		t.Errorf("expected nil for empty question, got type=%v entities=%v", typeKeywords, entities)
	}
}
