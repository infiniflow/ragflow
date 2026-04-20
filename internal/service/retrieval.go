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
	"fmt"
	"math"
	"sort"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/engine"
	"ragflow/internal/engine/infinity"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/logger"
	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
)

// RetrievalService provides retrieval search functionality (业务层: 粗召回 → 重排 → 精排)
// Corresponds to rag/nlp/search.py:retrieval()
type RetrievalService struct {
	docEngine engine.DocEngine
}

// NewRetrievalService creates a new RetrievalService with the given doc engine
func NewRetrievalService(docEngine engine.DocEngine) *RetrievalService {
	return &RetrievalService{docEngine: docEngine}
}

// GetVector computes query vector and returns MatchDenseExpr for hybrid search.
// Corresponds to Python's rag/nlp/search.py:get_vector() (L52-60).
func (s *RetrievalService) GetVector(txt string, embModel entity.EmbeddingModel, topk int, similarity float64) (*types.MatchDenseExpr, error) {
	vector, err := embModel.EncodeQuery(txt)
	if err != nil {
		return nil, err
	}

	vectorSize := len(vector)
	vectorColumnName := fmt.Sprintf("q_%d_vec", vectorSize)

	return &types.MatchDenseExpr{
		VectorColumnName:  vectorColumnName,
		EmbeddingData:     vector,
		EmbeddingDataType: "float",
		DistanceType:      "cosine",
		TopN:              topk,
		ExtraOptions:      map[string]interface{}{"similarity": similarity},
	}, nil
}

// RetrievalRequest request for retrieval search (业务层)
// This is the request struct for the RetrievalService.Search() method
// Corresponds to Python's rag/nlp/search.py:search() request parameters
type RetrievalRequest struct {
	Question               string
	TenantIDs              []string
	KbIDs                  []string
	DocIDs                 []string
	Page                   int
	PageSize               int
	Top                    *int
	SimilarityThreshold    *float64
	VectorSimilarityWeight *float64
	RankFeature            *map[string]float64
	RerankModel            nlp.RerankModel
	EmbeddingModel         entity.EmbeddingModel // For encoding query in Level 3
	Aggs                   *bool                 // Whether to build doc_aggs (matching Python L499)
	Highlight              *bool                 // Whether to include highlights
}

// RetrievalSearchRequest is the request struct for RetrievalService.Search()
// Contains fields needed to build the engine SearchRequest
type RetrievalSearchRequest struct {
	Question            string
	TenantIDs           []string
	KbIDs               []string
	DocIDs              []string
	Top                 int
	Page                int
	PageSize            int
	Sort                bool
	Highlight           *bool
	SimilarityThreshold float64
	RankFeature         map[string]float64
	Filter              map[string]interface{}
	EmbeddingModel      interface{}
}

// GetFilters builds metadata filter map from RetrievalSearchRequest
// Corresponds to Python's rag/nlp/search.py:get_filters()
func (r *RetrievalSearchRequest) GetFilters() map[string]interface{} {
	filters := make(map[string]interface{})

	if len(r.KbIDs) > 0 {
		filters["kb_id"] = r.KbIDs
	}
	if len(r.DocIDs) > 0 {
		filters["doc_id"] = r.DocIDs
	}
	for _, key := range []string{"knowledge_graph_kwd", "available_int", "entity_kwd", "from_entity_kwd", "to_entity_kwd", "removed_kwd"} {
		if val, ok := r.Filter[key]; ok && val != nil {
			filters[key] = val
		}
	}
	for key, val := range r.Filter {
		if _, exists := filters[key]; !exists && val != nil {
			filters[key] = val
		}
	}
	return filters
}

// RetrievalResult result from retrieval search
// Corresponds to Python's rag/nlp/search.py:retrieval() returning ranks dict
type RetrievalResult struct {
	Chunks  []map[string]interface{}
	DocAggs []map[string]interface{} // Aggregated document counts, sorted by count desc
}

// Retrieval performs hybrid search + reranking + pagination
// - Calculate rerank limit and call Search() to fetch rerankLimit candidates for reranking
// - Perform reranking via Rerank()
// - Sort indices by sim descending and filter by threshold
// - Calculate pagination to extract actual page returned from reranked results
// - Build chunks
// - Build document aggregation if specified
func (s *RetrievalService) Retrieval(ctx context.Context, req *RetrievalRequest) (*RetrievalResult, error) {
	if req.Question == "" {
		return &RetrievalResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}}, nil
	}

	// Apply default values
	if req.Top == nil {
		req.Top = func() *int { v := 1024; return &v }()
	}
	if req.SimilarityThreshold == nil {
		req.SimilarityThreshold = func() *float64 { v := 0.0; return &v }()
	}
	if req.VectorSimilarityWeight == nil {
		req.VectorSimilarityWeight = func() *float64 { v := 0.3; return &v }()
	}
	if req.RankFeature == nil {
		req.RankFeature = &map[string]float64{"pagerank_fea": 10.0}
	}
	if req.Aggs == nil {
		req.Aggs = func() *bool { v := true; return &v }()
	}

	if req.Page <= 0 {
		req.Page = 1
	}
	if req.PageSize <= 0 {
		req.PageSize = 1
	}

	// Calculate rerank limit to ensure we get enough results for proper pagination
	pageSize := req.PageSize
	rerankLimit := pageSize
	if pageSize > 1 {
		rerankLimit = int(math.Ceil(64.0/float64(pageSize))) * pageSize
	} else {
		rerankLimit = 1
	}
	if rerankLimit < 30 {
		rerankLimit = 30
	}
	// Cap rerank limit when external rerank model is used
	if req.RerankModel != nil && *req.Top > 0 {
		if rerankLimit > *req.Top {
			rerankLimit = *req.Top
		}
		if rerankLimit > 64 {
			rerankLimit = 64
		}
	}

	// Execute search via Search()
	page := req.Page
	globalOffset := (page - 1) * pageSize
	searchPage := globalOffset/rerankLimit + 1
	similarityThreshold := *req.SimilarityThreshold
	searchReq := &RetrievalSearchRequest{
		TenantIDs:           req.TenantIDs,
		Question:            req.Question,
		KbIDs:               req.KbIDs,
		DocIDs:              req.DocIDs,
		Page:                searchPage,
		PageSize:            rerankLimit,
		Top:                 *req.Top,
		SimilarityThreshold: similarityThreshold,
		RankFeature:         *req.RankFeature,
		EmbeddingModel:      req.EmbeddingModel,
	}
	searchResult, err := s.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("Search failed: %w", err)
	}

	// Perform reranking
	vtWeight := *req.VectorSimilarityWeight
	tkWeight := 1.0 - vtWeight
	sim, term_similarity, vector_similarity := nlp.Rerank(
		req.RerankModel,
		searchResult.Chunks,
		int(searchResult.Total),
		nil,
		searchResult.QueryVector,
		req.Question,
		tkWeight,
		vtWeight,
		true,
		"content_ltks",
		nil,
		*req.RankFeature,
	)
	if len(sim) == 0 {
		return &RetrievalResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}}, nil
	}

	// Sort indices (positions into search results) by score descending
	// After sorting by score descending, we process chunks in relevance order
	type idxScore struct {
		idx   int
		score float64
	}
	idxScores := make([]idxScore, 0, len(sim))
	for i, s := range sim {
		idxScores = append(idxScores, idxScore{idx: i, score: s})
	}
	sort.Slice(idxScores, func(i, j int) bool {
		return idxScores[i].score > idxScores[j].score
	})

	// When vector_similarity_weight is 0, similarity_threshold is not meaningful for term-only scores
	// When doc_ids is explicitly provided (metadata or document filtering), bypass threshold
	// User wants those specific documents regardless of their relevance score
	postThreshold := *req.SimilarityThreshold
	if *req.VectorSimilarityWeight <= 0 || len(req.DocIDs) > 0 {
		postThreshold = 0.0
	}

	// Get valid indices where score >= postThreshold
	validIdx := make([]int, 0)
	for _, is := range idxScores {
		if is.score >= postThreshold {
			validIdx = append(validIdx, is.idx)
		}
	}
	if len(validIdx) == 0 {
		return &RetrievalResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}}, nil
	}

	// Calculate pagination
	// begin and end define which of validIdx to return as the page
	begin := globalOffset % rerankLimit
	end := begin + pageSize

	// Get page indices
	var pageIdx []int
	if begin < len(validIdx) {
		if end > len(validIdx) {
			end = len(validIdx)
		}
		pageIdx = validIdx[begin:end]
	}

	// Build chunks for pageIdx, transforms raw search results into the API response format
	var filteredChunks []map[string]interface{}
	dim := 0
	if searchResult.QueryVector != nil {
		dim = len(searchResult.QueryVector)
	}
	zeroVector := make([]float64, dim)
	for j := 0; j < dim; j++ {
		zeroVector[j] = 0.0
	}

	for _, i := range pageIdx {
		if i < 0 || i >= len(searchResult.IDs) {
			continue
		}
		chunkID := searchResult.IDs[i]
		chunk, exists := searchResult.Field[chunkID]
		if !exists {
			continue
		}

		resultChunk := make(map[string]interface{})
		resultChunk["chunk_id"] = chunkID
		if v, ok := chunk["content_ltks"]; ok {
			resultChunk["content_ltks"] = v
		}
		if v, ok := chunk["content_with_weight"]; ok {
			resultChunk["content_with_weight"] = v
		}
		if v, ok := chunk["doc_id"]; ok {
			resultChunk["doc_id"] = v
		}
		if v, ok := chunk["docnm_kwd"]; ok {
			resultChunk["docnm_kwd"] = v
		}
		if v, ok := chunk["kb_id"]; ok {
			resultChunk["kb_id"] = v
		}
		if v, ok := chunk["important_kwd"]; ok {
			resultChunk["important_kwd"] = v
		}
		if v, ok := chunk["tag_kwd"]; ok {
			resultChunk["tag_kwd"] = v
		}
		if v, ok := chunk["img_id"]; ok {
			resultChunk["image_id"] = v
		}
		if v, ok := chunk["position_int"]; ok {
			resultChunk["positions"] = v
		}
		if v, ok := chunk["doc_type_kwd"]; ok {
			resultChunk["doc_type_kwd"] = v
		}
		if v, ok := chunk["mom_id"]; ok {
			resultChunk["mom_id"] = v
		}
		// row_id: row identifier (for structured data like tables)
		if v, ok := chunk["row_id()"]; ok {
			resultChunk["row_id"] = v
		}
		resultChunk["similarity"] = sim[i]
		resultChunk["term_similarity"] = term_similarity[i]
		resultChunk["vector_similarity"] = vector_similarity[i]
		vectorColumn := fmt.Sprintf("q_%d_vec", dim)
		if v, ok := chunk[vectorColumn]; ok {
			resultChunk["vector"] = v
		} else {
			resultChunk["vector"] = zeroVector
		}

		highlightEnabled := false
		if req.Highlight != nil && *req.Highlight {
			highlightEnabled = true
		}
		if highlightEnabled && searchResult.Highlight != nil {
			if highlightText, ok := searchResult.Highlight[chunkID]; ok {
				resultChunk["highlight"] = nlp.RemoveRedundantSpaces(highlightText)
			} else if contentWithWeight, ok := chunk["content_with_weight"].(string); ok {
				resultChunk["highlight"] = nlp.RemoveRedundantSpaces(contentWithWeight)
			}
		}
		filteredChunks = append(filteredChunks, resultChunk)
	}

	logger.Debug("Retrieval completed", zap.Int("totalChunks", len(filteredChunks)))

	// Build document aggregation, aggregates document-level statistics across all valid chunks
	// This is useful for showing users which documents are most relevant to their query.
	var docAggs []map[string]interface{}
	if req.Aggs != nil && *req.Aggs {
		docAggsMap := make(map[string]struct {
			docID string
			count int
		})
		for _, i := range validIdx {
			if i < 0 || i >= len(searchResult.IDs) {
				continue
			}
			chunkID := searchResult.IDs[i]
			chunk, exists := searchResult.Field[chunkID]
			if !exists {
				continue
			}
			docName := ""
			docID := ""
			if v, ok := chunk["docnm_kwd"].(string); ok {
				docName = v
			}
			if v, ok := chunk["doc_id"].(string); ok {
				docID = v
			}
			if entry, exists := docAggsMap[docName]; exists {
				entry.count++
				docAggsMap[docName] = entry
			} else {
				docAggsMap[docName] = struct {
					docID string
					count int
				}{docID: docID, count: 1}
			}
		}

		// Sort by count descending
		type docAggEntry struct {
			docName string
			docID   string
			count   int
		}
		docAggsList := make([]docAggEntry, 0, len(docAggsMap))
		for docName, entry := range docAggsMap {
			docAggsList = append(docAggsList, docAggEntry{docName: docName, docID: entry.docID, count: entry.count})
		}
		sort.Slice(docAggsList, func(i, j int) bool {
			return docAggsList[i].count > docAggsList[j].count
		})

		docAggs = make([]map[string]interface{}, 0, len(docAggsList))
		for _, entry := range docAggsList {
			docAggs = append(docAggs, map[string]interface{}{
				"doc_name": entry.docName,
				"doc_id":   entry.docID,
				"count":    entry.count,
			})
		}
	} else {
		docAggs = []map[string]interface{}{}
	}

	return &RetrievalResult{
		Chunks:  filteredChunks,
		DocAggs: docAggs,
	}, nil
}

type RetrievalSearchResult struct {
	Chunks      []map[string]interface{}          // Search results
	Total       int64                             // Total number of matches
	QueryVector []float64                         // Query vector (for hybrid search, used in reranking)
	Highlight   map[string]string                 // Highlighted snippets (chunk_id -> highlighted text)
	Field       map[string]map[string]interface{} // ID -> chunk mapping (like Python's sres.field)
	IDs         []string                          // Ordered list of chunk IDs (like Python's sres.ids)
	Keywords    []string                          // Keywords from query (like Python's sres.keywords)
	Aggregation []map[string]interface{}          // Doc aggregation by field (like Python's sres.aggregation)
	Options     map[string]interface{}            // Engine-specific options (e.g., total from get_total)
}

// Search performs search based on question and EmbeddingModel:
// - Empty question: list data matching filters, optionally sorted
// - Non-empty question, no EmbeddingModel: fulltext search only
// - Non-empty question, with EmbeddingModel: hybrid search (fulltext + vector + fusion)
//
// Hybrid search path retries with lower thresholds if no results found.
func (s *RetrievalService) Search(ctx context.Context, req *RetrievalSearchRequest) (*RetrievalSearchResult, error) {

	// Normalize highlight to bool (matching Python L80-81: if highlight is None: highlight = False)
	if req.Highlight == nil {
		req.Highlight = func() *bool { v := false; return &v }()
	}

	// Filters from request (matching Python L83)
	filters := req.GetFilters()

	// Pagination (matching Python L86-89)
	pg := req.Page - 1
	if pg < 0 {
		pg = 0
	}
	topk := req.Top
	if topk <= 0 {
		topk = 1024
	}
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = topk
	}
	offset := pg * pageSize
	limit := pageSize

	// Build Source field list (matching Python L91-95)
	src := []string{
		"docnm_kwd", "content_ltks", "kb_id", "img_id", "title_tks", "important_kwd", "position_int",
		"doc_id", "chunk_order_int", "page_num_int", "top_int", "create_timestamp_flt", "knowledge_graph_kwd",
		"question_kwd", "question_tks", "doc_type_kwd",
		"available_int", "content_with_weight", "mom_id", "pagerank_fea", "tag_feas", "row_id()",
	}

	// Keywords set (matching Python L96)
	kwds := make(map[string]struct{})

	// Build base engine request with common fields (matching Python L83-89)
	// Note: RankFeature is NOT set here; it's set per-call where needed
	searchRequest := &types.SearchRequest{
		IndexNames:          buildIndexNames(req.TenantIDs),
		KbIDs:               req.KbIDs,
		TopK:                topk,
		Offset:              offset,
		Limit:               limit,
		MetaDataFilter:      filters,
		Source:              src,
		SimilarityThreshold: req.SimilarityThreshold,
	}

	// engineResult holds the result from docEngine.Search() (types.SearchResult)
	// queryVector tracks the query vector for reranking
	var engineResult *types.SearchResult
	var queryVector []float64
	var err error

	if req.Question == "" {
		// Empty question path (L100-106): list/retrieve documents matching filters without keyword/vector search
		// rank_feature is NOT passed (matching Python L106)
		// If req.Sort is true, apply sort order; otherwise use default engine order
		if req.Sort {
			searchRequest.OrderBy = &types.OrderByExpr{}
			searchRequest.OrderBy.Asc("chunk_order_int").Asc("page_num_int").Asc("top_int").Desc("create_timestamp_flt")
		}
		// Explicitly set empty highlightFields and matchExprs (matching Python L106: passing [] for both)
		searchRequest.HighlightFields = []string{}
		searchRequest.MatchExprs = []interface{}{}

		engineResult, err = s.docEngine.Search(ctx, searchRequest)
		if err != nil {
			return nil, fmt.Errorf("Search failed: %w", err)
		}
	} else {
		// Non-empty question path (matching Python L108-158)

		// Determine highlightFields (matching Python L108-112)
		var highlightFields []string
		if req.Highlight != nil && *req.Highlight {
			highlightFields = []string{"content_ltks", "title_tks"}
		} else {
			highlightFields = []string{}
		}

		// Compute keywords via QueryBuilder (matching Python L115)
		// In Python: matchText, keywords = self.qryr.question(qst, min_match=0.3)
		matchText, keywords := nlp.GetQueryBuilder().Question(req.Question, "qa", 0.3)
		for _, k := range keywords {
			kwds[k] = struct{}{}
		}

		// Check if EmbeddingModel is available (matching Python L116 vs L123)
		if req.EmbeddingModel == nil {
			// Keyword-only search (matching Python L116-122)
			// rank_feature is passed (matching Python L119)
			searchRequestWithRank := *searchRequest
			searchRequestWithRank.MatchExprs = []interface{}{matchText}
			searchRequestWithRank.HighlightFields = highlightFields
			searchRequestWithRank.RankFeature = req.RankFeature

			engineResult, err = s.docEngine.Search(ctx, &searchRequestWithRank)
			if err != nil {
				return nil, fmt.Errorf("Search failed: %w", err)
			}
			queryVector = nil
		} else {
			// Compute question vector via GetVector (matching Python L123-125)
			// In Python: matchDense = await self.get_vector(qst, emb_mdl, topk, req.get("similarity", 0.1))
			// Note: Python defaults similarity to 0.1, not 0.2
			similarityForGetVector := req.SimilarityThreshold
			if similarityForGetVector <= 0 {
				similarityForGetVector = 0.1
			}
			matchDense, err := s.GetVector(req.Question, req.EmbeddingModel.(entity.EmbeddingModel), topk, similarityForGetVector)
			if err != nil {
				return nil, fmt.Errorf("GetVector failed: %w", err)
			}

			// Execute search with fusion (matching Python L128-131)
			// matchDense = MatchDenseExpr(vector_column, embedding_data, 'float', 'cosine', topk, {"similarity": similarity})
			// fusionExpr = FusionExpr("weighted_sum", topk, {"weights": "0.05,0.95"})
			// matchExprs = [matchText, matchDense, fusionExpr]
			fusionExpr := &types.FusionExpr{
				Method:       "weighted_sum",
				TopN:         topk,
				FusionParams: map[string]interface{}{"weights": "0.05,0.95"},
			}

			// Build source with vector column for ES (matching Python L125-126)
			searchSrc := make([]string, len(searchRequest.Source))
			copy(searchSrc, searchRequest.Source)
			if engine.GetEngineType() == engine.EngineElasticsearch {
				searchSrc = append(searchSrc, matchDense.VectorColumnName)
			}

			// rank_feature is passed (matching Python L132)
			searchRequestWithRank := &types.SearchRequest{
				IndexNames:          buildIndexNames(req.TenantIDs),
				KbIDs:               req.KbIDs,
				TopK:                topk,
				Offset:              offset,
				Limit:               limit,
				MetaDataFilter:      filters,
				Source:              searchSrc,
				MatchExprs:          []interface{}{matchText, matchDense, fusionExpr},
				HighlightFields:     highlightFields,
				RankFeature:         req.RankFeature,
				SimilarityThreshold: req.SimilarityThreshold,
			}

			engineResult, err = s.docEngine.Search(ctx, searchRequestWithRank)
			if err != nil {
				return nil, fmt.Errorf("Search failed: %w", err)
			}
			// If result is empty, retry with lower min_match (matching Python L133-145)
			if engineResult.Total == 0 {
				// Check if filters contains doc_id key (matching Python: filters.get("doc_id"))
				_, hasDocIDFilter := filters["doc_id"]

				if hasDocIDFilter {
					// Fallback without vector query when doc_id filter is present (matching Python L138-140)
					// Python L139: rank_feature is NOT passed in this fallback
					searchRequestWithoutRank := &types.SearchRequest{
						IndexNames:      buildIndexNames(req.TenantIDs),
						KbIDs:           req.KbIDs,
						TopK:            topk,
						Offset:          offset,
						Limit:           limit,
						MetaDataFilter:  filters,
						Source:          src,
						HighlightFields: []string{},
						MatchExprs:      []interface{}{},
					}
					engineResult, err = s.docEngine.Search(ctx, searchRequestWithoutRank)
					if err != nil {
						return nil, fmt.Errorf("Search retry failed: %w", err)
					}
				} else {
					// Retry with lower min_match via QueryBuilder (matching Python L142-147)
					// rank_feature is passed (matching Python L146)
					matchText, _ := nlp.GetQueryBuilder().Question(req.Question, "qa", 0.1)
					matchDense.ExtraOptions["similarity"] = 0.17
					searchRequestWithRank := &types.SearchRequest{
						IndexNames:          buildIndexNames(req.TenantIDs),
						KbIDs:               req.KbIDs,
						TopK:                topk,
						Offset:              offset,
						Limit:               limit,
						MetaDataFilter:      filters,
						Source:              searchSrc,
						MatchExprs:          []interface{}{matchText, matchDense, fusionExpr},
						HighlightFields:     highlightFields,
						RankFeature:         req.RankFeature,
						SimilarityThreshold: req.SimilarityThreshold,
					}
					engineResult, err = s.docEngine.Search(ctx, searchRequestWithRank)
					if err != nil {
						return nil, fmt.Errorf("Search retry failed: %w", err)
					}
				}
			}

			// Set QueryVector in response
			queryVector = matchDense.EmbeddingData
		}

		// Build kwds from keywords with fine-grained tokenization (matching Python L150-157)
		for _, k := range keywords {
			kwds[k] = struct{}{}
			fgToken, _ := tokenizer.FineGrainedTokenize(k)
			for _, kk := range strings.Fields(fgToken) {
				if len(kk) < 2 {
					continue
				}
				if _, ok := kwds[kk]; ok {
					continue
				}
				kwds[kk] = struct{}{}
			}
		}
	}

	// Build IDs from chunks (matching Python L160)
	ids := infinity.GetDocIDs(engineResult.Chunks)

	// Build Keywords list from kwds set (matching Python L161)
	keywordsList := make([]string, 0, len(kwds))
	for k := range kwds {
		keywordsList = append(keywordsList, k)
	}

	// Build Field map (matching Python: field=self.dataStore.get_fields(res, src + ["_score"]))
	fieldMap := infinity.GetFields(engineResult.Chunks, nil)

	// Build Aggregation (matching Python L163)
	aggregation := infinity.GetAggregation(engineResult.Chunks, "docnm_kwd")

	// Build Highlight using GetHighlight (matching Python L162)
	var highlight map[string]string
	if len(keywordsList) > 0 {
		highlight = infinity.GetHighlight(engineResult.Chunks, keywordsList, "content_with_weight")
	}

	// Build and return RetrievalSearchResult
	return &RetrievalSearchResult{
		Chunks:      engineResult.Chunks,
		Total:       engineResult.Total,
		QueryVector: queryVector,
		Highlight:   highlight,
		Field:       fieldMap,
		IDs:         ids,
		Keywords:    keywordsList,
		Aggregation: aggregation,
	}, nil
}
