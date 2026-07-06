package graph

import (
	"context"
	"fmt"

	"encoding/json"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	modelModule "ragflow/internal/entity/models"
)

// NhopEntityNames extracts unique entity names from an n_hop_with_weight JSON string.
func NhopEntityNames(nHopJSON string) []string {
	if nHopJSON == "" {
		return nil
	}
	var nhopData []struct {
		Path    []string  `json:"path"`
		Weights []float64 `json:"weights"`
	}
	if err := json.Unmarshal([]byte(nHopJSON), &nhopData); err != nil {
		return nil
	}
	seen := make(map[string]struct{})
	for _, item := range nhopData {
		for _, name := range item.Path {
			seen[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(seen))
	for name := range seen {
		result = append(result, name)
	}
	return result
}

// SearchEntities searches for KG entities matching a question.
func SearchEntities(ctx context.Context, docEngine engine.DocEngine, kbIDs []string, question string, embModel *modelModule.EmbeddingModel, topN int) ([]KGEntity, error) {
	dense, err := buildDenseExpr(embModel, question, topN)
	if err != nil {
		return nil, err
	}
	searchReq := buildEntitySearchRequest(kbIDs, question, dense, topN)
	result, err := docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("KG entity search failed: %w", err)
	}
	return ParseEntityChunks(result.Chunks), nil
}

// SearchEntitiesByTypes searches for KG entities by type keywords.
func SearchEntitiesByTypes(ctx context.Context, docEngine engine.DocEngine, kbIDs []string, typeKeywords []string, topN int) ([]KGEntity, error) {
	searchReq := buildEntityTypeSearchRequest(kbIDs, typeKeywords, topN)
	result, err := docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("KG entity type search failed: %w", err)
	}
	return ParseEntityChunks(result.Chunks), nil
}

// SearchRelations searches for KG relations matching a question.
func SearchRelations(ctx context.Context, docEngine engine.DocEngine, kbIDs []string, question string, embModel *modelModule.EmbeddingModel, topN int) ([]KGRelation, error) {
	dense, err := buildDenseExpr(embModel, question, topN)
	if err != nil {
		return nil, err
	}
	searchReq := buildRelationSearchRequest(kbIDs, question, dense, topN)
	result, err := docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("KG relation search failed: %w", err)
	}
	return ParseRelationChunks(result.Chunks), nil
}

// SearchCommunityReports searches for community reports related to given entities.
func SearchCommunityReports(ctx context.Context, docEngine engine.DocEngine, kbIDs []string, entityNames []string, topN int) ([]KGCommunityReport, error) {
	searchReq := buildCommunitySearchRequest(kbIDs, entityNames, topN)
	result, err := docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("KG community search failed: %w", err)
	}
	return ParseCommunityReportChunks(result.Chunks), nil
}

// SearchTypeSamples retrieves the typeu2192entities mapping from ES.
func SearchTypeSamples(ctx context.Context, docEngine engine.DocEngine, kbIDs []string) (map[string][]string, error) {
	searchReq := buildTypeSamplesSearchRequest(kbIDs)
	result, err := docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, err
	}
	return ParseTypeSamplesChunks(result.Chunks), nil
}

// buildDenseExpr computes the query vector and returns a MatchDenseExpr.
func buildDenseExpr(embModel *modelModule.EmbeddingModel, question string, topN int) (*types.MatchDenseExpr, error) {
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
	if dense == nil {
		return []interface{}{text}
	}
	fusion := buildFusionExpr(defaultTextWeight, defaultVectorWeight, topN)
	return []interface{}{dense, text, fusion}
}

// buildEntitySearchRequest constructs a SearchRequest for KG entities.
func buildEntitySearchRequest(kbIDs []string, question string, dense *types.MatchDenseExpr, topN int) *types.SearchRequest {
	req := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"entity_kwd", "entity_type_kwd", "rank_flt", "content_with_weight", "n_hop_with_weight", "_score"},
		Limit:        topN,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "entity"},
	}
	if question != "" {
		textExpr := &types.MatchTextExpr{
			Fields:       []string{"entity_kwd^10", "content_ltks^2"},
			MatchingText: question,
			TopN:         topN,
		}
		req.MatchExprs = buildHybridExpr(dense, textExpr, topN)
	}
	return req
}

// buildEntityTypeSearchRequest constructs a SearchRequest for KG entities by type.
func buildEntityTypeSearchRequest(kbIDs []string, typeKeywords []string, topN int) *types.SearchRequest {
	req := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"entity_kwd", "entity_type_kwd"},
		Limit:        topN,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "entity"},
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
func buildRelationSearchRequest(kbIDs []string, question string, dense *types.MatchDenseExpr, topN int) *types.SearchRequest {
	req := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"from_entity_kwd", "to_entity_kwd", "weight_int", "content_with_weight", "_score"},
		Limit:        topN,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "relation"},
	}
	if question != "" {
		textExpr := &types.MatchTextExpr{
			Fields:       []string{"content_ltks", "from_entity_kwd", "to_entity_kwd"},
			MatchingText: question,
			TopN:         topN,
		}
		req.MatchExprs = buildHybridExpr(dense, textExpr, topN)
	}
	return req
}

// buildCommunitySearchRequest constructs a SearchRequest for KG community reports.
func buildCommunitySearchRequest(kbIDs []string, entityNames []string, topN int) *types.SearchRequest {
	req := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"docnm_kwd", "content_with_weight", "weight_flt", "entities_kwd"},
		Limit:        topN,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "community_report"},
		OrderBy:      (&types.OrderByExpr{}).Desc("weight_flt"),
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

// buildTypeSamplesSearchRequest constructs a SearchRequest for type samples.
func buildTypeSamplesSearchRequest(kbIDs []string) *types.SearchRequest {
	return &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"content_with_weight"},
		Limit:        10000,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "ty2ents"},
	}
}

// ParseEntityChunks converts raw search result chunks into KGEntity slices.
func ParseEntityChunks(chunks []map[string]interface{}) []KGEntity {
	var entities []KGEntity
	for _, chunk := range chunks {
		name, _ := chunk["entity_kwd"].(string)
		if name == "" {
			// Try extracting from list
			if list, ok := chunk["entity_kwd"].([]interface{}); ok && len(list) > 0 {
				name, _ = list[0].(string)
			}
		}
		if name == "" {
			continue
		}
		typ, _ := chunk["entity_type_kwd"].(string)
		e := KGEntity{Name: name, Type: typ}
		if v, ok := chunk["rank_flt"].(float64); ok {
			e.PageRank = v
		}
		if v, ok := chunk["_score"].(float64); ok {
			e.Similarity = v
		} else if v, ok := chunk["score"].(float64); ok {
			e.Similarity = v
		}
		e.Description, _ = chunk["content_with_weight"].(string)
		entities = append(entities, e)
	}
	return entities
}

// ParseRelationChunks converts raw search result chunks into KGRelation slices.
func ParseRelationChunks(chunks []map[string]interface{}) []KGRelation {
	var relations []KGRelation
	for _, chunk := range chunks {
		from, _ := chunk["from_entity_kwd"].(string)
		to, _ := chunk["to_entity_kwd"].(string)
		if from == "" || to == "" {
			continue
		}
		r := KGRelation{From: from, To: to}
		if v, ok := chunk["_score"].(float64); ok {
			r.Sim = v
		} else if v, ok := chunk["score"].(float64); ok {
			r.Sim = v
		}
		if v, ok := chunk["weight_int"].(float64); ok {
			r.PageRank = v
		} else if v, ok := chunk["weight_int"].(int); ok {
			r.PageRank = float64(v)
		}
		r.Description, _ = chunk["content_with_weight"].(string)
		relations = append(relations, r)
	}
	return relations
}

// ParseCommunityReportChunks converts raw search result chunks into KGCommunityReport slices.
func ParseCommunityReportChunks(chunks []map[string]interface{}) []KGCommunityReport {
	var reports []KGCommunityReport
	for _, chunk := range chunks {
		title, _ := chunk["docnm_kwd"].(string)
		content, _ := chunk["content_with_weight"].(string)
		if title == "" && content == "" {
			continue
		}
		r := KGCommunityReport{Title: title, Content: content}
		if v, ok := chunk["weight_flt"].(float64); ok {
			r.Weight = v
		}
		r.Entities, _ = chunk["entities_kwd"].(string)
		reports = append(reports, r)
	}
	return reports
}

// ParseTypeSamplesChunks converts raw search result chunks into a typeu2192entities map.
func ParseTypeSamplesChunks(chunks []map[string]interface{}) map[string][]string {
	typeMap := make(map[string][]string)
	for _, chunk := range chunks {
		content, ok := chunk["content_with_weight"].(string)
		if !ok || content == "" {
			continue
		}
		var parsed map[string][]string
		if err := json.Unmarshal([]byte(content), &parsed); err != nil {
			continue
		}
		for typ, entities := range parsed {
			typeMap[typ] = append(typeMap[typ], entities...)
		}
	}
	return typeMap
}
