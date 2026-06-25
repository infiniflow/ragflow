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

package kg

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	modelModule "ragflow/internal/entity/models"
	"go.uber.org/zap"
)

// Pipeline encapsulates the knowledge graph retrieval pipeline.
// Matches Python: rag/graphrag/search.py::KGSearch
type Pipeline struct {
	docEngine engine.DocEngine
	chatModel *modelModule.ChatModel
	embModel  *modelModule.EmbeddingModel
	kbIDs     []string
	idxnms    []string
	question  string

	// Configurable parameters (defaults match Python)
	entSimThreshold float64
	relSimThreshold float64
	denseTopK       int
	entTopN         int
	relTopN         int
	commTopN        int
	maxToken        int
}

// Option configures a Pipeline.
type Option func(*Pipeline)

// WithSimThreshold sets the similarity threshold for entity and relation search.
// Default: 0.3 (matches Python ent_sim_threshold, rel_sim_threshold).
func WithSimThreshold(v float64) Option {
	return func(p *Pipeline) { p.entSimThreshold = v; p.relSimThreshold = v }
}

// WithDenseTopK sets the TopK for dense vector search.
// Default: 1024 (matches Python get_vector topk).
func WithDenseTopK(v int) Option {
	return func(p *Pipeline) { p.denseTopK = v }
}

// NewPipeline creates a KG search pipeline with the given dependencies.
//
//	docEngine: search engine backend
//	kbIDs:     knowledge base IDs to search
//	tenantIDs: tenant IDs (converted to index names internally)
//	question:  user query string
//	opts:      optional configuration (WithSimThreshold, WithDenseTopK)
//
// chatModel and embModel should be set via WithChatModel/WithEmbModel setters
// or passed directly after construction.
func NewPipeline(
	docEngine engine.DocEngine,
	kbIDs []string,
	tenantIDs []string,
	question string,
	opts ...Option,
) *Pipeline {
	idxnms := make([]string, len(tenantIDs))
	for i, tid := range tenantIDs {
		idxnms[i] = indexName(tid)
	}
	p := &Pipeline{
		docEngine: docEngine,
		kbIDs:     kbIDs,
		idxnms:    idxnms,
		question:  question,

		entSimThreshold: defaultSimThreshold,
		relSimThreshold: defaultSimThreshold,
		denseTopK:       defaultDenseTopK,
		entTopN:         6,
		relTopN:         6,
		commTopN:        1,
		maxToken:        8196,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// SetChatModel sets the chat model for LLM-based query rewrite.
func (p *Pipeline) SetChatModel(chatModel *modelModule.ChatModel) {
	p.chatModel = chatModel
}

// SetEmbModel sets the embedding model for dense/hybrid search.
func (p *Pipeline) SetEmbModel(embModel *modelModule.EmbeddingModel) {
	p.embModel = embModel
}

// Retrieval runs the full KG retrieval pipeline and returns a synthetic chunk.
func (p *Pipeline) Retrieval(ctx context.Context) (map[string]interface{}, error) {
	// 1. Query rewrite via LLM, or fall back to raw question
	ty2entsJSON := ""
	if p.chatModel != nil {
		typeSamples, err := searchTypeSamples(ctx, p.docEngine, p.idxnms, p.kbIDs)
		if err != nil {
			common.Warn("KG type samples search failed", zap.String("kbIDs", fmt.Sprint(p.kbIDs)))
		}
		if typeSamples == nil {
			typeSamples = make(map[string][]string)
		}
		data, _ := json.Marshal(typeSamples)
		ty2entsJSON = string(data)
	}
	typeKeywords, entities := queryRewrite(p.chatModel, p.question, ty2entsJSON)

	// 2-4. Search entities, types, and relations in parallel (mutually independent)
	var (
		entsFromQuery map[string]*KGEntity
		entsFromTypes map[string]struct{}
		relsFromText  map[Edge]*KGRelation
		entsErr       error
	)
	var wg sync.WaitGroup
	wg.Add(3)
	go func() {
		defer wg.Done()
		entsFromQuery, entsErr = p.searchEntities(ctx, entities)
	}()
	go func() {
		defer wg.Done()
		entsFromTypes = p.searchEntityTypes(ctx, typeKeywords)
	}()
	go func() {
		defer wg.Done()
		relsFromText = p.searchRelations(ctx, entities)
	}()
	wg.Wait()
	if entsErr != nil {
		return nil, entsErr
	}// 5. N-hop analysis + score fusion
	nhopPathes := AnalyzeNHopPaths(entsFromQuery)
	DoubleHitBoost(entsFromQuery, entsFromTypes)
	FuseRelationScores(relsFromText, entsFromTypes, nhopPathes)

	// 6. Sort and trim
	scoredEnts := SortAndTrimEntities(entsFromQuery, p.entTopN)
	scoredRels := SortAndTrimRelations(relsFromText, p.relTopN)

	// 7. Build KG content with token budget
	entsRelsContent := BuildContent(scoredEnts, scoredRels, p.maxToken)
	used := NumTokensFromString(entsRelsContent)
	remaining := p.maxToken - used
	// 8. Search community reports with remaining token budget
	communityContent := searchCommunityContent(ctx, p.docEngine, p.idxnms, p.kbIDs, scoredEnts, p.commTopN, &remaining)

	// 9. Build synthetic chunk
	return map[string]interface{}{
		"chunk_id":              "",
		"content_ltks":          "",
		"content_with_weight":   entsRelsContent + communityContent,
		"doc_id":                "",
		"docnm_kwd":             "Related content in Knowledge Graph",
		"kb_id":                 p.kbIDs,
		"important_kwd":         []string{},
		"image_id":              "",
		"similarity":            1.0,
		"vector_similarity":     1.0,
		"term_similarity":       0,
		"vector":                []float64{},
		"positions":             []interface{}{},
	}, nil
}

// searchEntities searches KG entities by keyword text and optional dense vector.
func (p *Pipeline) searchEntities(ctx context.Context, entities []string) (map[string]*KGEntity, error) {
	entsReq := &types.SearchRequest{
		IndexNames:   p.idxnms,
		KbIDs:        p.kbIDs,
		SelectFields: []string{"entity_kwd", "entity_type_kwd", "rank_flt", "content_with_weight", "n_hop_with_weight"},
		Limit:        50,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "entity"},
	}
	if len(entities) > 0 {
		entsReq.MatchExprs = buildSearchExprs(p.embModel, &types.MatchTextExpr{
			Fields:       []string{"entity_kwd^10", "content_ltks^2"},
			MatchingText: strings.Join(entities, " "),
			TopN:         50,
		}, p.entSimThreshold, p.denseTopK)
	}
	entsResult, err := p.docEngine.Search(ctx, entsReq)
	if err != nil {
		return nil, fmt.Errorf("KG entity search failed: %w", err)
	}
	result := make(map[string]*KGEntity)
	for _, chunk := range FilterChunksByScore(entsResult.Chunks, p.entSimThreshold) {
		name, _ := chunk["entity_kwd"].(string)
		if name == "" {
			continue
		}
		e := entityFromChunk(name, chunk)
		result[name] = &e
	}
	return result, nil
}

// searchEntityTypes searches KG entities by type keywords.
func (p *Pipeline) searchEntityTypes(ctx context.Context, typeKeywords []string) map[string]struct{} {
	typesReq := &types.SearchRequest{
		IndexNames:   p.idxnms,
		KbIDs:        p.kbIDs,
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
	typesResult, err := p.docEngine.Search(ctx, typesReq)
	result := make(map[string]struct{})
	if err != nil {
		common.Warn("KG types search failed", zap.String("kbIDs", fmt.Sprint(p.kbIDs)))
	} else {
		for _, chunk := range typesResult.Chunks {
			if name, ok := chunk["entity_kwd"].(string); ok {
				result[name] = struct{}{}
			}
		}
	}
	return result
}

// searchRelations searches KG relations by entity text and optional dense vector.
func (p *Pipeline) searchRelations(ctx context.Context, entities []string) map[Edge]*KGRelation {
	relsReq := &types.SearchRequest{
		IndexNames:   p.idxnms,
		KbIDs:        p.kbIDs,
		SelectFields: []string{"from_entity_kwd", "to_entity_kwd", "weight_int", "content_with_weight"},
		Limit:        50,
		Filter:       map[string]interface{}{"knowledge_graph_kwd": "relation"},
	}
	if len(entities) > 0 {
		relsReq.MatchExprs = buildSearchExprs(p.embModel, &types.MatchTextExpr{
			Fields:       []string{"content_ltks", "from_entity_kwd", "to_entity_kwd"},
			MatchingText: strings.Join(entities, " "),
			TopN:         50,
		}, p.relSimThreshold, p.denseTopK)
	}
	relsResult, err := p.docEngine.Search(ctx, relsReq)
	result := make(map[Edge]*KGRelation)
	if err != nil {
		common.Warn("KG relations search failed", zap.String("kbIDs", fmt.Sprint(p.kbIDs)))
	} else {
		for _, chunk := range FilterChunksByScore(relsResult.Chunks, p.relSimThreshold) {
			edge, rel := relationFromChunk(chunk)
			if edge.From == "" || edge.To == "" {
				continue
			}
			result[edge] = &rel
		}
	}
	return result
}
