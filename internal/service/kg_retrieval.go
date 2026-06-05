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

// indexName builds the search index name from a tenant ID.
// Matches Python: rag/nlp/search.py::index_name()
func indexName(tenantID string) string {
	return "ragflow_" + tenantID
}

// Python alignment defaults — match rag/graphrag/search.py retrieval() params
const (
	defaultKGSimThreshold = 0.3  // Python: ent_sim_threshold, rel_sim_threshold
	defaultKGDenseTopK    = 1024 // Python: get_vector() topk
)

// kgEntityFromChunk parses a single entity chunk into a KGEntity.
func kgEntityFromChunk(name string, chunk map[string]interface{}) KGEntity {
	e := KGEntity{}
	if v, ok := chunk["_score"].(float64); ok {
		e.Similarity = v
	} else if v, ok := chunk["score"].(float64); ok {
		e.Similarity = v
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
				e.NhopEnts = append(e.NhopEnts, NhopEntity{
					Path:    item.Path,
					Weights: item.Weights,
				})
			}
		}
	}
	return e
}

// kgRelationFromChunk parses a single relation chunk into a KGRelation.
func kgRelationFromChunk(chunk map[string]interface{}) (Edge, KGRelation) {
	r := KGRelation{}
	r.Description, _ = chunk["content_with_weight"].(string)
	if v, ok := chunk["weight_int"].(float64); ok {
		r.PageRank = float64(v)
	} else if v, ok := chunk["weight_int"].(int); ok {
		r.PageRank = float64(v)
	}
	from, _ := chunk["from_entity_kwd"].(string)
	to, _ := chunk["to_entity_kwd"].(string)
	return Edge{From: from, To: to}, r
}

// KGSearchRetrieval performs a full knowledge graph retrieval and returns
// a synthetic chunk to be inserted into search results.
// Corresponds to Python: rag/graphrag/search.py::KGSearch.retrieval()
//
// This is a convenience wrapper around KGSearchPipeline.
func KGSearchRetrieval(
	ctx context.Context,
	docEngine engine.DocEngine,
	chatModel *modelModule.ChatModel,
	embModel *modelModule.EmbeddingModel,
	kbIDs []string,
	tenantIDs []string,
	question string,
) (map[string]interface{}, error) {
	p := &KGSearchPipeline{
		docEngine:       docEngine,
		chatModel:       chatModel,
		embModel:        embModel,
		kbIDs:           kbIDs,
		idxnms:          makeIndexNames(tenantIDs),
		question:        question,
		entSimThreshold: defaultKGSimThreshold,
		relSimThreshold: defaultKGSimThreshold,
		denseTopK:       defaultKGDenseTopK,
		entTopN:         6,
		relTopN:         6,
		commTopN:        1,
		maxToken:        8196,
	}
	return p.Retrieval(ctx)
}

// makeIndexNames converts tenant IDs to search index names.
func makeIndexNames(tenantIDs []string) []string {
	idxnms := make([]string, len(tenantIDs))
	for i, tid := range tenantIDs {
		idxnms[i] = indexName(tid)
	}
	return idxnms
}

// searchKGTypeSamples searches for ty2ents data.
func searchKGTypeSamples(ctx context.Context, docEngine engine.DocEngine, idxnms []string, kbIDs []string) (map[string][]string, error) {
	req := &types.SearchRequest{
		IndexNames:   idxnms,
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
func searchKGCommunityContent(ctx context.Context, docEngine engine.DocEngine, idxnms []string, kbIDs []string, scoredEnts []ScoredEntity, topN int, maxToken *int) string {
	if maxToken == nil || len(scoredEnts) == 0 || *maxToken <= 0 {
		return ""
	}
	entityNames := make([]string, len(scoredEnts))
	for i, e := range scoredEnts {
		entityNames[i] = e.Entity
	}
	req := &types.SearchRequest{
		IndexNames:   idxnms,
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
	for idx, chunk := range result.Chunks {
		title, _ := chunk["docnm_kwd"].(string)
		raw, _ := chunk["content_with_weight"].(string)
		if title == "" && raw == "" {
			continue
		}
		// Parse JSON for nested report/evidences fields (Python: json.loads)
		report := raw
		evidence := ""
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			if r, ok := parsed["report"].(string); ok {
				report = r
			}
			if e, ok := parsed["evidences"].(string); ok {
				evidence = e
			}
		}
		section := fmt.Sprintf("\n# %d. %s\n## Content\n%s\n## Evidences\n%s\n", idx+1, title, report, evidence)
		tokens := NumTokensFromString(section)
		if *maxToken-tokens <= 0 {
			break
		}
		bld += section
		*maxToken -= tokens
	}
	return bld
}

// buildMatchDenseExpr constructs a MatchDenseExpr from an embedding vector.
// This is a pure function — no I/O, no external dependencies.
func buildMatchDenseExpr(vector []float64, topN int, similarity float64) *types.MatchDenseExpr {
	vectorColumnName := fmt.Sprintf("q_%d_vec", len(vector))
	return &types.MatchDenseExpr{
		VectorColumnName:  vectorColumnName,
		EmbeddingData:     vector,
		EmbeddingDataType: "float",
		DistanceType:      "cosine",
		TopN:              topN,
		ExtraOptions:      map[string]interface{}{"similarity": similarity},
	}
}

// buildFusionExpr constructs a FusionExpr for weighted-sum hybrid search.
// This is a pure function — no I/O, no external dependencies.
func buildFusionExpr(textWeight, vectorWeight float64, topN int) *types.FusionExpr {
	return &types.FusionExpr{
		Method: "weighted_sum",
		TopN:   topN,
		FusionParams: map[string]interface{}{
			"weights": fmt.Sprintf("%.2f,%.2f", textWeight, vectorWeight),
		},
	}
}

// buildSearchExprs constructs MatchExprs for KG entity/relation search.
// When embModel is nil, returns text-only match expression.
// When embModel is non-nil, embeds the question and returns hybrid
// (text + dense + fusion) expressions for vector+keyword search.
func buildSearchExprs(embModel *modelModule.EmbeddingModel, matchText *types.MatchTextExpr, simThreshold float64, denseTopK int) []interface{} {
	if embModel == nil || embModel.ModelDriver == nil {
		return []interface{}{matchText}
	}
	embeddingConfig := &modelModule.EmbeddingConfig{Dimension: 0}
	embeddings, err := embModel.ModelDriver.Embed(embModel.ModelName, []string{matchText.MatchingText}, embModel.APIConfig, embeddingConfig)
	if err != nil || len(embeddings) == 0 {
		return []interface{}{matchText}
	}
	denseExpr := buildMatchDenseExpr(embeddings[0].Embedding, denseTopK, simThreshold)
	fusionExpr := buildFusionExpr(0.5, 0.5, matchText.TopN)
	return []interface{}{matchText, denseExpr, fusionExpr}
}

// queryRewrite attempts LLM-based query rewrite, falling back to raw question.
// ty2entsJSON is the JSON-encoded type→entities mapping for prompt context.
func queryRewrite(chatModel *modelModule.ChatModel, question string, ty2entsJSON string) (typeKeywords, entities []string) {
	if question == "" {
		return nil, nil
	}
	if chatModel != nil && chatModel.ModelName != nil && chatModel.APIConfig != nil {
		prompt := common.BuildQueryRewritePrompt(question, ty2entsJSON)
		messages := []modelModule.Message{
			{Role: "system", Content: prompt},
			{Role: "user", Content: "Output:"},
		}
		response, err := chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, messages, chatModel.APIConfig, nil)
		if err == nil && response != nil && response.Answer != nil {
			result, parseErr := common.ParseQueryRewriteResponse(*response.Answer)
			if parseErr == nil && result != nil {
				return result.TypeKeywords, result.Entities
			}
		}
	}
	// Fallback: use raw question as single entity
	return nil, []string{question}
}
