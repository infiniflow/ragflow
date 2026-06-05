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

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	modelModule "ragflow/internal/entity/models"
)

// kgEntityFromChunk parses a single entity chunk into a common.KGEntity.
func kgEntityFromChunk(name string, chunk map[string]interface{}) common.KGEntity {
	e := common.KGEntity{}
	if v, ok := chunk["_score"].(float64); ok {
		e.Sim = v
	} else if v, ok := chunk["score"].(float64); ok {
		e.Sim = v
	}
	if v, ok := chunk["rank_flt"].(float64); ok {
		e.PageRank = v
	}
	e.Description, _ = chunk["content_with_weight"].(string)
	if raw, ok := chunk["n_hop_with_weight"].(string); ok && raw != "" {
		var nhopData []struct {
			Path    []string  `json:"path"`
			Weights []float64 `json:"weights"`
		}
		if err := json.Unmarshal([]byte(raw), &nhopData); err == nil {
			for _, item := range nhopData {
				e.NhopEnts = append(e.NhopEnts, common.NhopEntity{
					Path:    item.Path,
					Weights: item.Weights,
				})
			}
		}
	}
	return e
}

// kgRelationFromChunk parses a single relation chunk into a common.KGRelation.
func kgRelationFromChunk(chunk map[string]interface{}) (common.Edge, common.KGRelation) {
	r := common.KGRelation{}
	r.Description, _ = chunk["content_with_weight"].(string)
	if v, ok := chunk["weight_int"].(float64); ok {
		r.PageRank = float64(v)
	} else if v, ok := chunk["weight_int"].(int); ok {
		r.PageRank = float64(v)
	}
	from, _ := chunk["from_entity_kwd"].(string)
	to, _ := chunk["to_entity_kwd"].(string)
	return common.Edge{From: from, To: to}, r
}

// KGSearchRetrieval performs a full knowledge graph retrieval and returns
// a synthetic chunk to be inserted into search results.
// Corresponds to Python: rag/graphrag/search.py::KGSearch.retrieval()
func KGSearchRetrieval(
	ctx context.Context,
	docEngine engine.DocEngine,
	chatModel *modelModule.ChatModel,
	embModel *modelModule.EmbeddingModel,
	kbIDs []string,
	tenantIDs []string,
	question string,
) (map[string]interface{}, error) {
	// 1. Build index names from tenant IDs
	var idxnms []string
	for _, tid := range tenantIDs {
		idxnms = append(idxnms, fmt.Sprintf("ragflow_%s", tid))
	}

	// 2. Retrieve type samples (ty2ents)
	typeSamples, err := searchKGTypeSamples(ctx, docEngine, kbIDs)
	if err == nil && len(typeSamples) > 0 {
		_ = typeSamples // used in query_rewrite when available
	}

	// 3. Query rewrite (simplified: use question entities directly)
	typeKeywords := extractTypeKeywords(question)
	entities := extractEntities(question)

	// 4. Search entities by keywords
	entsReq := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"entity_kwd", "entity_type_kwd", "rank_flt", "content_with_weight", "n_hop_with_weight"},
		Limit:        50,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "entity"},
	}
	if len(entities) > 0 {
		entsReq.MatchExprs = []interface{}{
			&types.MatchTextExpr{
				Fields:       []string{"entity_kwd^10", "content_ltks^2"},
				MatchingText: entities[0],
				TopN:         50,
			},
		}
	}
	entsResult, err := docEngine.Search(ctx, entsReq)
	if err != nil {
		return nil, fmt.Errorf("KG entity search failed: %w", err)
	}
	entsFromQuery := make(map[string]*common.KGEntity)
	for _, chunk := range entsResult.Chunks {
		name, _ := chunk["entity_kwd"].(string)
		if name == "" {
			continue
		}
		e := kgEntityFromChunk(name, chunk)
		entsFromQuery[name] = &e
	}

	// 5. Search entities by types
	typesReq := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"entity_kwd", "entity_type_kwd"},
		Limit:        10000,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "entity"},
	}
	if len(typeKeywords) > 0 {
		typeFilters := make([]interface{}, len(typeKeywords))
		for i, t := range typeKeywords {
			typeFilters[i] = t
		}
		typesReq.Filter["entity_type_kwd"] = typeFilters
	}
	typesResult, err := docEngine.Search(ctx, typesReq)
	entsFromTypes := make(map[string]struct{})
	if err == nil {
		for _, chunk := range typesResult.Chunks {
			if name, ok := chunk["entity_kwd"].(string); ok {
				entsFromTypes[name] = struct{}{}
			}
		}
	}

	// 6. Search relations
	relsReq := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"from_entity_kwd", "to_entity_kwd", "weight_int", "content_with_weight"},
		Limit:        50,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "relation"},
	}
	if len(entities) > 0 {
		relsReq.MatchExprs = []interface{}{
			&types.MatchTextExpr{
				Fields:       []string{"content_ltks", "from_entity_kwd", "to_entity_kwd"},
				MatchingText: entities[0],
				TopN:         50,
			},
		}
	}
	relsResult, err := docEngine.Search(ctx, relsReq)
	relsFromText := make(map[common.Edge]*common.KGRelation)
	if err == nil {
		for _, chunk := range relsResult.Chunks {
			edge, rel := kgRelationFromChunk(chunk)
			if edge.From == "" || edge.To == "" {
				continue
			}
			relsFromText[edge] = &rel
		}
	}

	// 7. N-hop analysis + score fusion (from common/kg_scoring.go)
	nhopPathes := common.AnalyzeNHopPaths(entsFromQuery)
	common.DoubleHitBoost(entsFromQuery, entsFromTypes)
	common.FuseRelationScores(relsFromText, entsFromTypes, nhopPathes)

	// 8. Sort and trim
	scoredEnts := common.SortAndTrimEntities(entsFromQuery, 6)
	scoredRels := common.SortAndTrimRelations(relsFromText, 6)

	// 9. Search community reports and build content
	maxToken := 8196
	communityContent := searchKGCommunityContent(ctx, docEngine, kbIDs, scoredEnts, 1, &maxToken)
	kgContent := common.BuildKGContent(scoredEnts, scoredRels, maxToken-len(communityContent)/4)

	// 10. Build synthetic chunk
	return map[string]interface{}{
		"chunk_id":              "",
		"content_ltks":          "",
		"content_with_weight":   kgContent + communityContent,
		"doc_id":                "",
		"docnm_kwd":             "Related content in Knowledge Graph",
		"kb_id":                 kbIDs,
		"important_kwd":         []string{},
		"image_id":              "",
		"similarity":            1.0,
		"vector_similarity":     1.0,
		"term_similarity":       0,
		"vector":                []float64{},
		"positions":             []interface{}{},
	}, nil
}

// searchKGTypeSamples searches for ty2ents data.
func searchKGTypeSamples(ctx context.Context, docEngine engine.DocEngine, kbIDs []string) (map[string][]string, error) {
	req := &types.SearchRequest{
		KbIDs:        kbIDs,
		SelectFields: []string{"content_with_weight"},
		Limit:        10000,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "ty2ents"},
	}
	result, err := docEngine.Search(ctx, req)
	if err != nil {
		return nil, err
	}
	typeMap := make(map[string][]string)
	for _, chunk := range result.Chunks {
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
	return typeMap, nil
}

// searchKGCommunityContent searches for community reports and formats them.
func searchKGCommunityContent(ctx context.Context, docEngine engine.DocEngine, kbIDs []string, scoredEnts []common.ScoredEntity, topN int, maxToken *int) string {
	if len(scoredEnts) == 0 || *maxToken <= 0 {
		return ""
	}
	entityNames := make([]string, len(scoredEnts))
	for i, e := range scoredEnts {
		entityNames[i] = e.Entity
	}
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
	result, err := docEngine.Search(ctx, req)
	if err != nil || len(result.Chunks) == 0 || *maxToken <= 0 {
		return ""
	}

	var bld string
	for _, chunk := range result.Chunks {
		title, _ := chunk["docnm_kwd"].(string)
		content, _ := chunk["content_with_weight"].(string)
		if title == "" && content == "" {
			continue
		}
		section := fmt.Sprintf("\n# %s\n## Content\n%s\n", title, content)
		tokens := common.NumTokensFromString(section)
		if *maxToken-tokens <= 0 {
			break
		}
		bld += section
		*maxToken -= tokens
	}
	return bld
}

// extractTypeKeywords extracts type-related keywords from a question.
// Simplified version; full implementation would use LLM query_rewrite.
func extractTypeKeywords(question string) []string {
	if question == "" {
		return nil
	}
	return nil
}

// extractEntities extracts entity names from a question.
// Simplified version; full implementation would use LLM query_rewrite.
func extractEntities(question string) []string {
	if question == "" {
		return nil
	}
	return []string{question}
}
