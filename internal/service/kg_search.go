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
	"fmt"

	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	modelModule "ragflow/internal/entity/models"
)

// KGEntity represents a knowledge graph entity.
type KGEntity struct {
	Name        string  // entity_kwd
	Type        string  // entity_type_kwd
	PageRank    float64 // rank_flt
	Similarity  float64 // _score
	Description string  // content_with_weight
}

// KGRelation represents a relation between two entities.
type KGRelation struct {
	From        string // from_entity_kwd
	To          string // to_entity_kwd
	Weight      int    // weight_int
	Description string // content_with_weight
}

// KGCommunityReport represents a community report.
type KGCommunityReport struct {
	Title    string  // docnm_kwd
	Content  string  // content_with_weight
	Weight   float64 // weight_flt
	Entities string  // entities_kwd
}

// buildKGDenseExpr computes the query vector and returns a MatchDenseExpr
// for KG hybrid search. Returns nil if embModel or question is empty.
func buildKGDenseExpr(embModel *modelModule.EmbeddingModel, question string, topN int) (*types.MatchDenseExpr, error) {
	if embModel == nil || question == "" {
		return nil, nil
	}
	embCfg := &modelModule.EmbeddingConfig{Dimension: 0}
	embeddings, err := embModel.ModelDriver.Embed(embModel.ModelName, []string{question}, embModel.APIConfig, embCfg)
	if err != nil {
		return nil, fmt.Errorf("KG entity embed failed: %w", err)
	}
	if len(embeddings) == 0 || len(embeddings[0].Embedding) == 0 {
		return nil, nil
	}
	vector := embeddings[0].Embedding
	return &types.MatchDenseExpr{
		VectorColumnName:  fmt.Sprintf("q_%d_vec", len(vector)),
		EmbeddingData:     vector,
		EmbeddingDataType: "float",
		DistanceType:      "cosine",
		TopN:              topN,
		ExtraOptions:      map[string]interface{}{"similarity": 0.3},
	}, nil
}

// buildHybridExpr returns MatchExprs for hybrid search (dense + text + fusion).
func buildHybridExpr(dense *types.MatchDenseExpr, text *types.MatchTextExpr, topN int) []interface{} {
	return []interface{}{
		dense,
		text,
		&types.FusionExpr{
			Method:       "weighted_sum",
			TopN:         topN,
			FusionParams: map[string]interface{}{"weights": "0.05,0.95"},
		},
	}
}

// buildEntitySearchRequest constructs a SearchRequest for KG entities.
// dense may be nil for text-only search.
func buildEntitySearchRequest(kbIDs []string, question string, dense *types.MatchDenseExpr, topN int) *types.SearchRequest {
	req := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"entity_kwd", "entity_type_kwd", "rank_flt", "content_with_weight"},
		Limit:        topN,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "entity"},
	}
	if question == "" {
		return req
	}
	textExpr := &types.MatchTextExpr{
		Fields:       []string{"entity_kwd^10", "content_ltks^2"},
		MatchingText: question,
		TopN:         topN,
	}
	if dense != nil {
		req.MatchExprs = buildHybridExpr(dense, textExpr, topN)
		req.RankFeature = map[string]float64{"pagerank_fea": 10.0}
	} else {
		req.MatchExprs = []interface{}{textExpr}
	}
	return req
}

// buildEntityTypeSearchRequest constructs a SearchRequest for KG entities by type.
func buildEntityTypeSearchRequest(kbIDs []string, typeKeywords []string, topN int) *types.SearchRequest {
	req := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"entity_kwd", "entity_type_kwd", "rank_flt", "content_with_weight"},
		Limit:        topN,
		Filter: map[string]interface{}{
			"knowledge_graph_kwd": "entity",
		},
	}
	if len(typeKeywords) > 0 {
		filters := make([]interface{}, len(typeKeywords))
		for i, t := range typeKeywords {
			filters[i] = t
		}
		req.Filter["entity_type_kwd"] = filters
	}
	return req
}

// buildRelationSearchRequest constructs a SearchRequest for KG relations.
// dense may be nil for text-only search.
func buildRelationSearchRequest(kbIDs []string, question string, dense *types.MatchDenseExpr, topN int) *types.SearchRequest {
	req := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"from_entity_kwd", "to_entity_kwd", "weight_int", "content_with_weight"},
		Limit:        topN,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "relation"},
	}
	if question != "" {
		textExpr := &types.MatchTextExpr{
			Fields:       []string{"content_ltks"},
			MatchingText: question,
			TopN:         topN,
		}
		if dense != nil {
			req.MatchExprs = buildHybridExpr(dense, textExpr, topN)
		} else {
			req.MatchExprs = []interface{}{textExpr}
		}
	}
	return req
}

// buildCommunitySearchRequest constructs a SearchRequest for KG community reports.
// Matches community reports whose entities_kwd contains any of the given entity names.
func buildCommunitySearchRequest(kbIDs []string, entityNames []string, topN int) *types.SearchRequest {
	req := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"docnm_kwd", "content_with_weight", "weight_flt", "entities_kwd"},
		Limit:        topN,
		Filter: map[string]interface{}{
			"knowledge_graph_kwd": "community_report",
		},
		OrderBy: (&types.OrderByExpr{}).Desc("weight_flt"),
	}
	if len(entityNames) > 0 {
		filters := make([]interface{}, len(entityNames))
		for i, name := range entityNames {
			filters[i] = name
		}
		req.Filter["entities_kwd"] = filters
	}
	return req
}

// buildTypeSamplesSearchRequest constructs a SearchRequest for ty2ents data.
func buildTypeSamplesSearchRequest(kbIDs []string) *types.SearchRequest {
	return &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"content_with_weight"},
		Limit:        10000,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "ty2ents"},
	}
}

// ParseKGEntityChunks converts raw search result chunks into KGEntity slices.
func ParseKGEntityChunks(chunks []map[string]interface{}) []KGEntity {
	var entities []KGEntity
	for _, chunk := range chunks {
		e := KGEntity{}
		if v, ok := chunk["entity_kwd"].(string); ok {
			e.Name = v
		} else if list, ok := chunk["entity_kwd"].([]interface{}); ok && len(list) > 0 {
			e.Name, _ = list[0].(string)
		}
		if e.Name == "" {
			continue
		}
		e.Type, _ = chunk["entity_type_kwd"].(string)
		e.Description, _ = chunk["content_with_weight"].(string)
		if v, ok := chunk["rank_flt"].(float64); ok {
			e.PageRank = v
		}
		if v, ok := chunk["_score"].(float64); ok {
			e.Similarity = v
		} else if v, ok := chunk["score"].(float64); ok {
			e.Similarity = v
		}
		entities = append(entities, e)
	}
	return entities
}

// ParseKGRelationChunks converts raw search result chunks into KGRelation slices.
func ParseKGRelationChunks(chunks []map[string]interface{}) []KGRelation {
	var relations []KGRelation
	for _, chunk := range chunks {
		r := KGRelation{}
		r.From, _ = chunk["from_entity_kwd"].(string)
		r.To, _ = chunk["to_entity_kwd"].(string)
		r.Description, _ = chunk["content_with_weight"].(string)
		if v, ok := chunk["weight_int"].(float64); ok {
			r.Weight = int(v)
		} else if v, ok := chunk["weight_int"].(int); ok {
			r.Weight = v
		}
		if r.From == "" || r.To == "" {
			continue
		}
		relations = append(relations, r)
	}
	return relations
}

// ParseKGCommunityReportChunks converts raw search result chunks into KGCommunityReport slices.
func ParseKGCommunityReportChunks(chunks []map[string]interface{}) []KGCommunityReport {
	var reports []KGCommunityReport
	for _, chunk := range chunks {
		r := KGCommunityReport{}
		r.Title, _ = chunk["docnm_kwd"].(string)
		r.Content, _ = chunk["content_with_weight"].(string)
		r.Entities, _ = chunk["entities_kwd"].(string)
		if v, ok := chunk["weight_flt"].(float64); ok {
			r.Weight = v
		}
		if r.Title == "" && r.Content == "" {
			continue
		}
		reports = append(reports, r)
	}
	return reports
}

// ParseKGTypeSamplesChunks converts raw search result chunks into a type→entities map.
func ParseKGTypeSamplesChunks(chunks []map[string]interface{}) map[string][]string {
	result := make(map[string][]string)
	for _, chunk := range chunks {
		content, ok := chunk["content_with_weight"].(string)
		if !ok || content == "" {
			continue
		}
		var typeMap map[string][]string
		if err := json.Unmarshal([]byte(content), &typeMap); err != nil {
			continue
		}
		for typ, entities := range typeMap {
			result[typ] = append(result[typ], entities...)
		}
	}
	return result
}

// NhopEntityNames extracts unique entity names from n_hop_with_weight JSON string.
// The JSON format is: [{"path": ["A", "B", "C"], "weights": [0.8, 0.5]}, ...]
// Returns entity names in order of first appearance, with duplicates removed.
func NhopEntityNames(nHopJSON string) []string {
	type nhopItem struct {
		Path    []string  `json:"path"`
		Weights []float64 `json:"weights"`
	}
	var data []nhopItem
	if err := json.Unmarshal([]byte(nHopJSON), &data); err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	var names []string
	for _, item := range data {
		for _, name := range item.Path {
			if _, ok := seen[name]; !ok {
				seen[name] = struct{}{}
				names = append(names, name)
			}
		}
	}
	return names
}

// SearchKGEntities searches for KG entities matching a question.
func SearchKGEntities(ctx context.Context, docEngine engine.DocEngine, kbIDs []string, question string, embModel *modelModule.EmbeddingModel, topN int) ([]KGEntity, error) {
	dense, err := buildKGDenseExpr(embModel, question, topN)
	if err != nil {
		return nil, err
	}
	req := buildEntitySearchRequest(kbIDs, question, dense, topN)
	result, err := docEngine.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("KG entity search failed: %w", err)
	}
	return ParseKGEntityChunks(result.Chunks), nil
}

// SearchKGEntitiesByTypes searches for KG entities by type keywords.
func SearchKGEntitiesByTypes(ctx context.Context, docEngine engine.DocEngine, kbIDs []string, typeKeywords []string, topN int) ([]KGEntity, error) {
	req := buildEntityTypeSearchRequest(kbIDs, typeKeywords, topN)
	result, err := docEngine.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("KG entity type search failed: %w", err)
	}
	return ParseKGEntityChunks(result.Chunks), nil
}

// SearchKGRelations searches for KG relations matching a question.
func SearchKGRelations(ctx context.Context, docEngine engine.DocEngine, kbIDs []string, question string, embModel *modelModule.EmbeddingModel, topN int) ([]KGRelation, error) {
	dense, err := buildKGDenseExpr(embModel, question, topN)
	if err != nil {
		return nil, err
	}
	req := buildRelationSearchRequest(kbIDs, question, dense, topN)
	result, err := docEngine.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("KG relation search failed: %w", err)
	}
	return ParseKGRelationChunks(result.Chunks), nil
}

// SearchKGCommunityReports searches for community reports related to given entities.
func SearchKGCommunityReports(ctx context.Context, docEngine engine.DocEngine, kbIDs []string, entityNames []string, topN int) ([]KGCommunityReport, error) {
	req := buildCommunitySearchRequest(kbIDs, entityNames, topN)
	result, err := docEngine.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("KG community search failed: %w", err)
	}
	return ParseKGCommunityReportChunks(result.Chunks), nil
}

// SearchKGTypeSamples retrieves the type→entities mapping from ES.
func SearchKGTypeSamples(ctx context.Context, docEngine engine.DocEngine, kbIDs []string) (map[string][]string, error) {
	req := buildTypeSamplesSearchRequest(kbIDs)
	result, err := docEngine.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("KG type samples search failed: %w", err)
	}
	return ParseKGTypeSamplesChunks(result.Chunks), nil
}
