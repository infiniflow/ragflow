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

package nlp

import (
	"context"
	"fmt"
	"maps"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity/models"
	"ragflow/internal/tokenizer"

	"go.uber.org/zap"
)

// RetrievalService provides retrieval search functionality
type RetrievalService struct {
	docEngine   engine.DocEngine
	documentDAO *dao.DocumentDAO
}

// NewRetrievalService creates a new RetrievalService with the given doc engine
func NewRetrievalService(docEngine engine.DocEngine, documentDAO *dao.DocumentDAO) *RetrievalService {
	return &RetrievalService{docEngine: docEngine, documentDAO: documentDAO}
}

// RetrievalRequest request for retrieval search
type RetrievalRequest struct {
	Question               string
	TenantIDs              []string
	KbIDs                  []string
	DocIDs                 []string
	Page                   int
	PageSize               int
	RerankCandidatesCount  *int
	KNNTopK                *int
	KNNNumCandidates       *int
	SimilarityThreshold    *float64
	VectorSimilarityWeight *float64
	RankFeature            *map[string]float64
	RerankModel            *models.RerankModel
	EmbeddingModel         *models.EmbeddingModel
	Aggs                   *bool
	Highlight              *bool
	AllowDenseFallback     *bool
	Filter                 map[string]interface{}
	// VectorOnly, when true, runs the dense leg alone: no text match expression
	// and no fusion expression reach the engine, so the search is a pure kNN
	// whose filter is the scope conditions only. Set by callers that need to
	// bridge a wording gap the query's own words cannot cross; the scoring pass
	// then uses the engine's kNN score directly instead of re-scoring tokens.
	VectorOnly bool
}

// RetrievalResult result from retrieval search
type RetrievalResult struct {
	Chunks  []map[string]interface{}
	DocAggs []map[string]interface{} // Aggregated document counts, sorted by count desc
	Total   int64                    // Threshold-valid matches across the retrieval candidate set
}

// Retrieval performs hybrid search + reranking + pagination
// - Retrieve candidates for reranking
// - Perform reranking via Rerank()
// - Sort indices by score descending and filter by threshold
// - Require Page * PageSize to fit within RerankCandidatesCount
// - Support only the first page when a rerank model is configured
// - Calculate pagination to extract actual page returned from reranked results
// - Build chunks
// - Build document aggregation if specified
func (s *RetrievalService) Retrieval(ctx context.Context, req *RetrievalRequest) (*RetrievalResult, error) {
	common.InfoCtx(ctx, "Retrieval START", zap.String("question", req.Question), zap.Int("page", req.Page), zap.Int("pageSize", req.PageSize))
	if req.Question == "" {
		return &RetrievalResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}, Total: 0}, nil
	}

	// Apply default values
	if req.KNNTopK == nil {
		req.KNNTopK = func() *int { v := 1024; return &v }()
	}
	if req.KNNNumCandidates == nil {
		req.KNNNumCandidates = func() *int { v := 2048; return &v }()
	}
	if req.SimilarityThreshold == nil {
		req.SimilarityThreshold = func() *float64 { v := 0.2; return &v }()
	}
	if req.VectorSimilarityWeight == nil {
		// A vector-only search is scored by vector similarity alone: no tokens
		// of the query are reliable signals there (the query is a description,
		// not a term list), so the default 0.3 vector weight would let token
		// coincidence re-rank a pure-vector result set.
		if req.VectorOnly {
			req.VectorSimilarityWeight = func() *float64 { v := 1.0; return &v }()
		} else {
			req.VectorSimilarityWeight = func() *float64 { v := 0.3; return &v }()
		}
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
	if req.RerankCandidatesCount == nil {
		req.RerankCandidatesCount = func() *int { v := 64; return &v }()
	}

	pageSize := req.PageSize
	rerankCandidatesCount := *req.RerankCandidatesCount
	if rerankCandidatesCount <= 0 || req.Page > rerankCandidatesCount/pageSize {
		return nil, fmt.Errorf("rerank_candidates_count(%d) must be greater than or equal to page(%d) * page_size(%d)", rerankCandidatesCount, req.Page, pageSize)
	}
	if req.RerankModel != nil && req.Page != 1 {
		return nil, fmt.Errorf("Pagination is not supported when rerank_mdl is specified. Please set page=1 to retrieve the top %d results.", pageSize)
	}
	// Request-scoped pool size (chat settings / tool arguments): without it a
	// shortfall is indistinguishable from a search that simply matched less.
	engineType := ""
	if s.docEngine != nil {
		engineType = s.docEngine.GetType()
	}
	common.InfoCtx(ctx, "Retrieval candidates",
		zap.Int("page", req.Page),
		zap.Int("pageSize", pageSize),
		zap.Int("rerankCandidatesCount", rerankCandidatesCount),
		zap.Int("knnTopK", *req.KNNTopK),
		zap.Int("knnNumCandidates", *req.KNNNumCandidates),
		zap.Float64("similarityThreshold", *req.SimilarityThreshold),
		zap.Float64("vectorSimilarityWeight", *req.VectorSimilarityWeight),
		zap.String("engine", engineType),
		zap.Bool("vectorOnly", req.VectorOnly))

	// Execute search via Search()
	searchReq := &RetrievalSearchRequest{
		TenantIDs:              req.TenantIDs,
		Question:               req.Question,
		KbIDs:                  req.KbIDs,
		DocIDs:                 req.DocIDs,
		Page:                   1,
		PageSize:               rerankCandidatesCount,
		KNNTopK:                *req.KNNTopK,
		KNNNumCandidates:       *req.KNNNumCandidates,
		RankFeature:            *req.RankFeature,
		EmbeddingModel:         req.EmbeddingModel,
		SimilarityThreshold:    *req.SimilarityThreshold,
		VectorSimilarityWeight: req.VectorSimilarityWeight,
		Highlight:              req.Highlight,
		AllowDenseFallback:     req.AllowDenseFallback,
		VectorOnly:             req.VectorOnly,
		Filter:                 req.Filter,
	}
	searchResult, err := s.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	// Prune deleted chunks
	searchResult, err = s.PruneDeletedChunks(ctx, searchResult)
	if err != nil {
		return nil, fmt.Errorf("PruneDeletedChunks failed: %w", err)
	}
	if searchResult.Total == 0 {
		return &RetrievalResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}, Total: 0}, nil
	}

	sim, termSimilarity, vectorSimilarity, err := s.scoreSearchResult(ctx, req, searchResult)
	if err != nil {
		return nil, err
	}
	if len(sim) == 0 {
		return &RetrievalResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}, Total: 0}, nil
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
	// Use SliceStable for deterministic ordering when scores are tied
	sort.SliceStable(idxScores, func(i, j int) bool {
		return idxScores[i].score > idxScores[j].score
	})

	// When vector_similarity_weight is 0, similarity_threshold is not meaningful for term-only scores
	postThreshold := *req.SimilarityThreshold
	if *req.VectorSimilarityWeight <= 0 {
		postThreshold = 0.0
	}
	// A VECTOR-ONLY search has already been floored by the engine's own kNN
	// similarity filter, so re-applying a threshold here buys nothing — and
	// costs everything when the score is the one this pipeline reads back: a
	// similarity that comes out 0 (absent from the hit's map, say) sends the
	// whole candidate set through the floor and the tool reports "0 hits" for a
	// corpus that certainly has neighbours. The engine's ranking is the truth
	// for this leg; keep it.
	if req.VectorOnly {
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
		return &RetrievalResult{Chunks: []map[string]interface{}{}, DocAggs: []map[string]interface{}{}, Total: 0}, nil
	}

	// Calculate pagination
	// begin and end define which of validIdx to return as the page
	begin := (req.Page - 1) * pageSize
	end := begin + pageSize

	// Get page indices
	var pageIdx []int
	if begin < len(validIdx) {
		if end > len(validIdx) {
			end = len(validIdx)
		}
		pageIdx = validIdx[begin:end]
	}
	common.InfoCtx(ctx, "Pagination result info", zap.Int("totalValid", len(validIdx)), zap.Int("begin", begin),
		zap.Int("end", end), zap.Int("chunkCount", len(pageIdx)), zap.Float64("postThreshold", postThreshold))

	total := int64(len(validIdx))

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
		if v, ok := chunk["img_id"]; ok {
			resultChunk["image_id"] = v
		} else {
			resultChunk["image_id"] = ""
		}
		if v, ok := chunk["position_int"]; ok && v != nil {
			resultChunk["positions"] = v
		} else {
			resultChunk["positions"] = []interface{}{}
		}
		if v, ok := chunk["doc_type_kwd"]; ok && v != nil {
			if s, ok := v.(string); ok {
				if s == "" {
					// Infinity's whitespace-# analyzer returns empty string as [] in Python SDK
					// but as "" in Go SDK. Both Infinity and Elasticsearch paths normalize
					// to None on the Python side (the test converts [] to None), so use nil
					// here for parity instead of []interface{}{}.
					resultChunk["doc_type_kwd"] = nil
				} else {
					resultChunk["doc_type_kwd"] = s
				}
			} else if sliceVal, ok := v.([]interface{}); ok {
				if len(sliceVal) == 0 {
					resultChunk["doc_type_kwd"] = nil
				} else {
					resultChunk["doc_type_kwd"] = sliceVal
				}
			} else {
				resultChunk["doc_type_kwd"] = nil
			}
		} else {
			resultChunk["doc_type_kwd"] = nil
		}
		// row_id: row identifier (for structured data like tables)
		if v, ok := chunk["row_id()"]; ok {
			resultChunk["row_id"] = v
		}
		resultChunk["similarity"] = sim[i]
		resultChunk["term_similarity"] = termSimilarity[i]
		resultChunk["vector_similarity"] = vectorSimilarity[i]

		// Always set these fields even if empty, to match Python response format
		if v, ok := chunk["important_kwd"]; ok {
			resultChunk["important_kwd"] = v
		} else {
			resultChunk["important_kwd"] = []string{}
		}
		if v, ok := chunk["mom_id"]; ok {
			resultChunk["mom_id"] = v
		} else {
			resultChunk["mom_id"] = ""
		}
		if v, ok := chunk["row_id()"]; ok {
			resultChunk["row_id"] = v
		} else {
			resultChunk["row_id"] = nil
		}
		// Mirrors rag/nlp/search.py's chunk.get("tag_kwd", []): neither side
		// selects tag_kwd (this file's src, search.py's default src), so the
		// engine never returns it even when a legacy chunk carries it.
		resultChunk["tag_kwd"] = []string{}

		vectorColumn := fmt.Sprintf("q_%d_vec", dim)
		if v, ok := chunk[vectorColumn]; ok {
			resultChunk["vector"] = v
		} else {
			resultChunk["vector"] = zeroVector
		}

		if searchResult.Highlight != nil {
			if highlightText, ok := searchResult.Highlight[chunkID]; ok {
				resultChunk["highlight"] = highlightText
			}
		}
		filteredChunks = append(filteredChunks, resultChunk)
	}

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
		Total:   total,
	}, nil
}

func (s *RetrievalService) scoreSearchResult(ctx context.Context, req *RetrievalRequest, searchResult *RetrievalSearchResult) ([]float64, []float64, []float64, error) {
	// sim = tkWeight*tsim + vtWeight*vsim
	vtWeight := *req.VectorSimilarityWeight
	tkWeight := 1.0 - vtWeight
	qb := GetQueryBuilder()
	useInfinity := engine.GetEngineType() == "infinity"
	useOceanBase := engine.IsOceanBaseFamily(s.docEngine.GetType())

	if req.RerankModel != nil && searchResult.Total > 0 {
		return RerankByModel(
			ctx,
			req.RerankModel,
			searchResult.Chunks,
			searchResult.IDs,
			searchResult.Field,
			req.Question,
			tkWeight,
			vtWeight,
			"content_ltks",
			qb,
			*req.RankFeature,
		)
	}

	if useInfinity {
		sim := make([]float64, len(searchResult.IDs))
		for i, id := range searchResult.IDs {
			if chunk, ok := searchResult.Field[id]; ok {
				if score, ok := chunk["_score"].(float64); ok {
					sim[i] = score
				} else if score, ok := chunk["SCORE"].(float64); ok {
					sim[i] = score
				} else if score, ok := chunk["SIMILARITY"].(float64); ok {
					sim[i] = score
				}
			}
		}
		return sim, sim, sim, nil
	}

	if useOceanBase {
		sim, tsim, vsim := RerankStandard(
			searchResult.Chunks,
			nil,
			searchResult.QueryVector,
			req.Question,
			tkWeight,
			vtWeight,
			"content_ltks",
			qb,
			*req.RankFeature,
		)
		return sim, tsim, vsim, nil
	}

	if req.VectorOnly {
		// The engine's kNN search already ranked these chunks by vector
		// similarity, so the second-pass KNNScores round trip would recompute
		// exactly the same numbers — one extra search request per query for
		// nothing. Report the engine score as the similarity, term similarity
		// as zero (there is no term signal in a vector-only request), and let
		// the caller's sort keep the engine order.
		//
		// The score is read from the CHUNK maps, not from searchResult.Field:
		// Field carries only the requested SelectFields (the engine's GetFields
		// copies exactly those), so a "_score" lookup there yields nothing and
		// every similarity would come out 0 — which the caller's threshold then
		// filters away wholesale (observed as a 0-hit pure-vector search over a
		// corpus that certainly holds neighbours).
		scores := make(map[string]float64, len(searchResult.Chunks))
		for _, chunk := range searchResult.Chunks {
			id, _ := chunk["id"].(string)
			if id == "" {
				if alt, ok := chunk["_id"].(string); ok {
					id = alt
				}
			}
			if id == "" {
				continue
			}
			for _, key := range []string{"_score", "SCORE", "SIMILARITY"} {
				if score, ok := chunk[key].(float64); ok {
					scores[id] = score
					break
				}
			}
		}
		sim := make([]float64, len(searchResult.IDs))
		for i, id := range searchResult.IDs {
			sim[i] = scores[id]
		}
		return sim, make([]float64, len(sim)), sim, nil
	}

	knnResult, err := s.docEngine.KNNScores(ctx, searchResult.Chunks, searchResult.QueryVector, len(searchResult.IDs))
	if err != nil {
		common.Warn("KNNScores failed for ES, falling back to local computation", zap.Error(err))
		sim, tsim, vsim := RerankStandard(
			searchResult.Chunks,
			nil,
			searchResult.QueryVector,
			req.Question,
			tkWeight,
			vtWeight,
			"content_ltks",
			qb,
			*req.RankFeature,
		)
		return sim, tsim, vsim, nil
	}
	knnScores := s.docEngine.GetScores(knnResult)
	sim, tsim, vsim := RerankWithKNN(
		ctx,
		searchResult.Chunks,
		searchResult.IDs,
		searchResult.Field,
		knnScores,
		req.Question,
		tkWeight,
		vtWeight,
		"content_ltks",
		qb,
		*req.RankFeature,
	)
	return sim, tsim, vsim, nil
}

// RetrievalSearchRequest is the request struct for RetrievalService.Search()
type RetrievalSearchRequest struct {
	Question            string
	TenantIDs           []string
	KbIDs               []string
	DocIDs              []string
	KNNTopK             int
	KNNNumCandidates    int
	Page                int
	PageSize            int
	Sort                bool
	Highlight           *bool
	SimilarityThreshold float64
	// VectorOnly runs the dense leg alone (see RetrievalRequest.VectorOnly):
	// no text match expression and no fusion expression are sent, so the
	// engine's kNN filter carries the scope conditions only.
	VectorOnly             bool
	RankFeature            map[string]float64
	Filter                 map[string]interface{}
	EmbeddingModel         *models.EmbeddingModel
	VectorSimilarityWeight *float64
	AllowDenseFallback     *bool
}

func buildInfinityFusionExpr(topn int, vectorSimilarityWeight *float64) *types.FusionExpr {
	vectorWeight := 0.3
	if vectorSimilarityWeight != nil {
		vectorWeight = *vectorSimilarityWeight
	}
	termWeight := math.Round((1.0-vectorWeight)*10000) / 10000

	return &types.FusionExpr{
		Method: "weighted_sum",
		TopN:   topn,
		FusionParams: map[string]interface{}{
			"weights": fmt.Sprintf("%g,%g", termWeight, vectorWeight),
		},
	}
}

func buildRetrievalFusionExpr(docEngineType string, topn int, vectorSimilarityWeight *float64) *types.FusionExpr {
	if docEngineType == string(engine.EngineInfinity) {
		return buildInfinityFusionExpr(topn, vectorSimilarityWeight)
	}

	// Matches rag/nlp/search.py:331 — every backend but Infinity takes this fixed
	// pair, so the first search is a vector recall pass and the caller's
	// vector_similarity_weight is applied afterwards, by RerankWithKNN
	// (tkWeight = 1-vw, vtWeight = vw). Feeding it in here instead ranks the
	// window by BM25 and cuts a different top-N than the reference does.
	return &types.FusionExpr{
		Method: "weighted_sum",
		TopN:   topn,
		FusionParams: map[string]interface{}{
			"weights": esFusionWeights,
		},
	}
}

// esFusionWeights is the pair the reference gives every non-Infinity backend
// (rag/nlp/search.py:331). 0.001 rather than 0 keeps a lexical leg for engines
// that reject a zero weight.
const esFusionWeights = "0.001,1"

// minMatch mirrors Python's `min_match = vector_similarity_weight < 0.8`
// (rag/nlp/search.py:773): a search that is almost entirely vector-weighted asks
// the text leg for nothing in particular, so its keyword query matches on any term
// instead of a share of them (0.3 first, 0.1 on the looser retry).
func minMatch(vectorSimilarityWeight *float64, withMatch float64) float64 {
	if vectorSimilarityWeight != nil && *vectorSimilarityWeight >= 0.8 {
		return 0.0
	}
	return withMatch
}

type RetrievalSearchResult struct {
	Chunks      []map[string]interface{}          // Search results
	Total       int64                             // Total number of matches
	QueryVector []float64                         // Query vector (for hybrid search, used in reranking)
	Highlight   map[string]string                 // Highlighted snippets (chunk_id -> highlighted text)
	Field       map[string]map[string]interface{} // ID -> chunk mapping
	IDs         []string                          // Ordered list of chunk IDs
	Keywords    []string                          // Keywords from query
	Aggregation []map[string]interface{}          // Doc aggregation by field
	Options     map[string]interface{}            // Engine-specific options (e.g., total from get_total)
	IndexNames  []string                          // Index names for second-pass queries (e.g., KNN scores)
}

// Search performs search based on question and EmbeddingModel:
// - Empty question: list data matching filters, optionally sorted
// - Non-empty question, no EmbeddingModel: fulltext search only
// - Non-empty question, with EmbeddingModel: hybrid search (fulltext + vector + fusion)
//
// Hybrid search path retries with lower thresholds if no results found.
func (s *RetrievalService) Search(ctx context.Context, req *RetrievalSearchRequest) (*RetrievalSearchResult, error) {
	if req.Highlight == nil {
		req.Highlight = func() *bool { v := false; return &v }()
	}
	if req.AllowDenseFallback == nil {
		req.AllowDenseFallback = new(true)
	}
	filters := req.GetFilters()
	if _, ok := filters["available_int"]; !ok {
		filters["available_int"] = 1
	}
	pg := max(req.Page-1, 0)
	knnTopK := req.KNNTopK
	if knnTopK <= 0 {
		knnTopK = 1024
	}
	numCandidates := req.KNNNumCandidates
	if numCandidates <= 0 {
		numCandidates = 2048
	}
	// Result pagination is independent of the KNN candidate pool size.
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = 30
	}
	limit := pageSize

	// Build Source field list
	src := []string{
		"docnm_kwd", "content_ltks", "kb_id", "img_id", "title_tks", "important_kwd", "position_int",
		"doc_id", "chunk_order_int", "page_num_int", "top_int", "create_timestamp_flt", "knowledge_graph_kwd",
		// Fields the projection below reads off each chunk. This list IS the ES
		// _source filter, so omitting one returns it empty even though the
		// document carries it: content_with_weight is the text the caller
		// displays, doc_type_kwd marks image/table chunks, mom_id is what promotes
		// child fragments to their parent, row_id() the table row identity.
		"content_with_weight", "doc_type_kwd", "mom_id", "row_id()",
		"_score",
	}

	kwds := make(map[string]struct{})

	// Build base engine request with common fields.
	searchRequest := &types.SearchRequest{
		IndexNames:   buildIndexNames(req.TenantIDs),
		KbIDs:        req.KbIDs,
		Offset:       pg * pageSize,
		Limit:        limit,
		Filter:       filters,
		SelectFields: src,
	}

	// queryVector tracks the query vector for reranking
	var engineResult *types.SearchResult
	var queryVector []float64
	var err error

	if req.Question == "" {
		// Empty question
		if req.Sort {
			searchRequest.OrderBy = &types.OrderByExpr{}
			searchRequest.OrderBy.Asc("chunk_order_int").Asc("page_num_int").Asc("top_int").Desc("create_timestamp_flt")
		}
		searchRequest.MatchExprs = []interface{}{}
		engineResult, err = s.docEngine.Search(ctx, searchRequest)
		if err != nil {
			return nil, fmt.Errorf("search failed: %w", err)
		}
	} else {
		// Non-empty question

		// Compute keywords via QueryBuilder
		matchText, keywords := GetQueryBuilder().Question(req.Question, "", minMatch(req.VectorSimilarityWeight, 0.3))
		for _, k := range keywords {
			kwds[k] = struct{}{}
		}

		// Check if EmbeddingModel is available
		if req.EmbeddingModel == nil {
			// Keyword-only search
			searchRequestWithRank := *searchRequest
			searchRequestWithRank.MatchExprs = []interface{}{matchText}
			searchRequestWithRank.RankFeature = req.RankFeature

			engineResult, err = s.docEngine.Search(ctx, &searchRequestWithRank)
			if err != nil {
				return nil, fmt.Errorf("search failed: %w", err)
			}
			queryVector = nil
		} else {
			// Compute question vector via GetVector
			similarityForGetVector := req.SimilarityThreshold
			if similarityForGetVector <= 0 {
				similarityForGetVector = 0.1
			}
			matchDense, err := s.GetVector(ctx, req.Question, req.EmbeddingModel, knnTopK, numCandidates, similarityForGetVector)
			if err != nil {
				return nil, fmt.Errorf("GetVector failed: %w", err)
			}
			denseTemplate := cloneDenseExpr(matchDense)

			// Execute search with fusion — unless this is a VECTOR-ONLY
			// request, which must reach the engine as a dense-only search: an
			// empty text expression is what keeps the BM25 clause out of the
			// kNN's filter (see the engine's query builder), and no fusion
			// expression means no weighted-sum step over a leg that does not
			// exist.
			var fusionExpr *types.FusionExpr
			if !req.VectorOnly {
				fusionExpr = buildRetrievalFusionExpr(s.docEngine.GetType(), knnTopK, req.VectorSimilarityWeight)
			}

			// Build source with vector column for ES
			searchSrc := make([]string, len(searchRequest.SelectFields))
			copy(searchSrc, searchRequest.SelectFields)
			if engine.GetEngineType() == "elasticsearch" || engine.IsOceanBaseFamily(engine.GetEngineType()) {
				searchSrc = append(searchSrc, matchDense.VectorColumnName)
			}

			searchRequest.SelectFields = searchSrc
			if req.VectorOnly || matchText == nil {
				// Dense leg alone. The engine then builds a kNN query whose
				// filter is the scope conditions only — no BM25 clause, so the
				// result set is not restricted to chunks that contain the
				// query's words.
				searchRequest.MatchExprs = []interface{}{matchDense}
			} else {
				searchRequest.MatchExprs = []interface{}{matchText, matchDense, fusionExpr}
			}
			searchRequest.RankFeature = req.RankFeature

			engineResult, err = s.docEngine.Search(ctx, searchRequest)
			if err != nil {
				return nil, fmt.Errorf("search failed: %w", err)
			}
			// If result is empty, retry with relaxed conditions
			if engineResult.Total == 0 {
				_, hasDocIDFilter := filters["doc_id"]
				if req.VectorOnly || matchText == nil {
					if *req.AllowDenseFallback {
						common.Debug("Retrieval dense-only fallback after empty initial search")
						matchDense = cloneDenseExpr(denseTemplate)
						matchDense.ExtraOptions["similarity"] = 0.17
						searchRequest.MatchExprs = []any{matchDense}
						engineResult, err = s.docEngine.Search(ctx, searchRequest)
						if err != nil {
							return nil, fmt.Errorf("dense-only fallback failed: %w", err)
						}
					}
				} else if hasDocIDFilter {
					// When a doc_id filter is present (e.g. from metadata filter like era=960)
					// and the hybrid search returns no results, fall back to a filter-only
					// search (no text match, no vector match). This ensures that when a
					// metadata filter restricts the search to a specific set of documents
					// that happen to have no relevant content for the query, we still
					// return those documents' chunks (ordered by the request's sort/order).
					//
					// Example: searching "打虎" with metadata filter era≠960 limits the
					// search to Three Kingdoms documents (era=220). Since "打虎" only
					// appears in Water Margin (era=960), the hybrid search returns 0
					// results. This fallback returns all chunks from Three Kingdoms
					// documents instead of returning an empty result.
					searchRequest.SelectFields = src
					searchRequest.MatchExprs = []interface{}{}
					searchRequest.RankFeature = nil

					engineResult, err = s.docEngine.Search(ctx, searchRequest)
					if err != nil {
						return nil, fmt.Errorf("search retry failed: %w", err)
					}
				} else {
					// No doc_id filter — retry with lower min_match (0.1 vs default 0.3)
					// and lower vector similarity threshold (0.17 vs default 0.1-0.2).
					// This provides a second chance for queries that were too strict
					// on the first attempt.
					matchText, _ := GetQueryBuilder().Question(req.Question, "qa", minMatch(req.VectorSimilarityWeight, 0.1))
					matchDense = cloneDenseExpr(denseTemplate)
					matchDense.ExtraOptions["similarity"] = 0.17
					if req.VectorOnly || matchText == nil {
						searchRequest.MatchExprs = []interface{}{matchDense}
					} else {
						searchRequest.MatchExprs = []interface{}{matchText, matchDense, fusionExpr}
					}
					searchRequest.RankFeature = req.RankFeature

					engineResult, err = s.docEngine.Search(ctx, searchRequest)
					if err != nil {
						return nil, fmt.Errorf("search retry failed: %w", err)
					}
					// Zero-only by design: any lexical hit keeps the existing hybrid candidate semantics.
					if engineResult.Total == 0 && matchText != nil && *req.AllowDenseFallback {
						common.Debug("Retrieval dense-only fallback after empty hybrid retries")
						matchDense = cloneDenseExpr(denseTemplate)
						matchDense.ExtraOptions["similarity"] = 0.17
						searchRequest.MatchExprs = []any{matchDense}
						engineResult, err = s.docEngine.Search(ctx, searchRequest)
						if err != nil {
							return nil, fmt.Errorf("dense-only fallback failed: %w", err)
						}
					}
				}
			}

			queryVector = matchDense.EmbeddingData
		}

		// Build kwds from keywords with fine-grained tokenization
		for _, k := range keywords {
			kwds[k] = struct{}{}
			fgToken, _ := tokenizer.FineGrainedTokenize(k)
			for kk := range strings.FieldsSeq(fgToken) {
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

	searchResult := engineResult
	// The window the engine was asked for vs what it returned, together with what
	// it was asked FOR: a shortfall is indistinguishable from a narrower query
	// without both, and it is what a caller sees as a thin candidate pool (the ES
	// backend is asked for rerankCandidatesCount candidates, see "Retrieval
	// candidates"). The query is read off MatchExprs — the expressions that
	// actually went to the engine — so the retries that re-ask with a lower
	// minimum_should_match are reported as issued.
	if searchResult != nil {
		engineQuery := ""
		if len(searchRequest.MatchExprs) > 0 {
			if mt, ok := searchRequest.MatchExprs[0].(*types.MatchTextExpr); ok && mt != nil {
				engineQuery = mt.MatchingText
			}
		}
		knnTopN, knnExtra := 0, map[string]interface{}(nil)
		if len(searchRequest.MatchExprs) > 1 {
			if de, ok := searchRequest.MatchExprs[1].(*types.MatchDenseExpr); ok && de != nil {
				knnTopN, knnExtra = de.TopN, de.ExtraOptions
			}
		}
		common.InfoCtx(ctx, "Search window",
			zap.Int("limit", limit), zap.Int("offset", pg*pageSize),
			zap.Int("engineChunks", len(searchResult.Chunks)), zap.Int64("engineTotal", searchResult.Total),
			zap.Strings("indexNames", searchRequest.IndexNames),
			zap.Int("matchExprs", len(searchRequest.MatchExprs)),
			zap.String("query", engineQuery),
			zap.Any("searchFilters", filters),
			zap.Any("requestFilter", req.Filter),
			zap.Strings("docIDs", req.DocIDs),
			zap.Int("knnTopN", knnTopN), zap.Any("knnExtra", knnExtra))
	}
	ids := s.docEngine.GetChunkIDs(searchResult.Chunks)
	common.Debug("GetChunkIDs result", zap.Int("count", len(ids)), zap.Strings("ids", ids))

	// Build Keywords list from kwds set
	keywordsList := make([]string, 0, len(kwds))
	for k := range kwds {
		keywordsList = append(keywordsList, k)
	}

	fieldMap := s.docEngine.GetFields(searchResult.Chunks, src)
	common.Debug("GetFields result", zap.Int("count", len(fieldMap)), zap.Strings("keys", func() []string {
		keys := make([]string, 0, len(fieldMap))
		for k := range fieldMap {
			keys = append(keys, k)
		}
		return keys
	}()), zap.Strings("ids_from_GetDocIDs", ids))

	// Build Aggregation
	aggregation := s.docEngine.GetAggregation(searchResult.Chunks, "docnm_kwd")

	// Build Highlight using GetHighlight
	highlight := make(map[string]string)
	if *req.Highlight {
		highlight = s.docEngine.GetHighlight(searchResult.Chunks, keywordsList, "content_with_weight")
	}

	return &RetrievalSearchResult{
		Chunks:      searchResult.Chunks,
		Total:       searchResult.Total,
		QueryVector: queryVector,
		Highlight:   highlight,
		Field:       fieldMap,
		IDs:         ids,
		Keywords:    keywordsList,
		Aggregation: aggregation,
		IndexNames:  searchRequest.IndexNames,
	}, nil
}

func cloneDenseExpr(source *types.MatchDenseExpr) *types.MatchDenseExpr {
	clone := *source
	clone.EmbeddingData = slices.Clone(source.EmbeddingData)
	clone.ExtraOptions = maps.Clone(source.ExtraOptions)
	return &clone
}

// GetVector computes query vector and returns MatchDenseExpr for hybrid search
func (s *RetrievalService) GetVector(ctx context.Context, txt string, embModel *models.EmbeddingModel, knnTopK, numCandidates int, similarity float64) (*types.MatchDenseExpr, error) {
	embeddingConfig := &models.EmbeddingConfig{
		Dimension: 0,
	}
	// Query: true mirrors Python Dealer.get_vector (rag/nlp/search.py:75-82),
	// which embeds the search text with emb_mdl.encode_queries — the asymmetric
	// query encoding (Cohere search_query / Voyage query / Jina retrieval.query
	// / NVIDIA query). Embedding it as a document would put the query vector in
	// the wrong space for those providers.
	embeddings, err := embModel.ModelDriver.Embed(ctx, embModel.ModelName, models.EmbedRequest{Texts: []string{txt}, Query: true}, embModel.APIConfig, embeddingConfig, nil)
	if err != nil {
		return nil, err
	}

	vector := embeddings[0].Embedding
	vectorSize := len(vector)
	vectorColumnName := fmt.Sprintf("q_%d_vec", vectorSize)

	return &types.MatchDenseExpr{
		VectorColumnName:  vectorColumnName,
		EmbeddingData:     vector,
		EmbeddingDataType: "float",
		DistanceType:      "cosine",
		TopN:              knnTopK,
		ExtraOptions:      map[string]interface{}{"similarity": similarity, "num_candidates": numCandidates},
	}, nil
}

// GetFilters builds metadata filter map from RetrievalSearchRequest
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

// RetrievalByChildren aggregates child chunks into parent chunks
func RetrievalByChildren(chunks []map[string]interface{}, tenantIDs []string, docEngine engine.DocEngine, ctx context.Context) []map[string]interface{} {
	common.Info("RetrievalByChildren started", zap.Int("chunks", len(chunks)), zap.Strings("tenantIDs", tenantIDs))

	indexNames := buildIndexNames(tenantIDs)
	if len(chunks) == 0 || len(indexNames) == 0 {
		return chunks
	}

	// Group child chunks by parent identity and document. The document scope is
	// required both for new parent IDs and for safe fallback when reading legacy
	// rows that used hash(mom) without document scope.
	type childChunk struct {
		chunk map[string]interface{}
	}
	type parentKey struct {
		momID string
		docID string
		kbID  string
	}
	momChunks := make(map[parentKey][]childChunk)
	remainingChunks := make([]map[string]interface{}, 0, len(chunks))

	for _, ck := range chunks {
		momID, ok := ck["mom_id"].(string)
		if !ok || momID == "" {
			remainingChunks = append(remainingChunks, ck)
			continue
		}
		kbID, _ := ck["kb_id"].(string)
		docID, _ := ck["doc_id"].(string)
		if docID == "" {
			remainingChunks = append(remainingChunks, ck)
			continue
		}
		key := parentKey{momID: momID, docID: docID, kbID: kbID}
		momChunks[key] = append(momChunks[key], childChunk{chunk: ck})
	}

	if len(momChunks) == 0 {
		common.Info("RetrievalByChildren finished", zap.Int("momChunks", len(momChunks)), zap.Int("resultChunks", len(chunks)))
		return chunks
	}

	// Fetch parent chunks and aggregate
	vectorSize := 1024
	for key, childList := range momChunks {
		momID := key.momID
		parentResult, err := docEngine.Search(ctx, &types.SearchRequest{
			IndexNames:         []string{indexNames[0]},
			KbIDs:              []string{key.kbID},
			Limit:              1,
			IncludeUnavailable: true,
			Filter: map[string]interface{}{
				"id":     momID,
				"doc_id": key.docID,
			},
		})
		if err != nil {
			common.Warn("Failed to get parent chunk", zap.String("momID", momID), zap.Error(err))
			for _, child := range childList {
				remainingChunks = append(remainingChunks, child.chunk)
			}
			continue
		}
		if parentResult == nil || len(parentResult.Chunks) == 0 {
			for _, child := range childList {
				remainingChunks = append(remainingChunks, child.chunk)
			}
			continue
		}
		parentMap := parentResult.Chunks[0]

		// Calculate average similarity
		simBuf := make([]float64, 0, len(childList))
		for _, c := range childList {
			if sim, ok := c.chunk["similarity"].(float64); ok {
				simBuf = append(simBuf, sim)
			}
		}
		totalSim := common.PairwiseSum(simBuf)
		avgSim := totalSim / float64(len(childList))

		// Collect content_ltks from children
		var contentParts []string
		for _, c := range childList {
			if ltks, ok := c.chunk["content_ltks"].(string); ok {
				contentParts = append(contentParts, ltks)
			}
		}
		contentLTKS := strings.Join(contentParts, " ")

		// Collect important_kwd from children
		allImportantKwd := []string{}
		for _, c := range childList {
			if kwd, ok := c.chunk["important_kwd"].([]interface{}); ok {
				for _, k := range kwd {
					if ks, ok := k.(string); ok {
						allImportantKwd = append(allImportantKwd, ks)
					}
				}
			}
		}

		// Build aggregated chunk
		docTypeKwd := ""
		if v, ok := parentMap["doc_type_kwd"].(string); ok {
			docTypeKwd = v
		}
		imgID := parentMap["img_id"]
		if imgID == nil || imgID == "" {
			imgID = ""
		}
		aggregated := map[string]interface{}{
			"chunk_id":            momID,
			"content_ltks":        contentLTKS,
			"content_with_weight": parentMap["content_with_weight"],
			"doc_id":              parentMap["doc_id"],
			"docnm_kwd":           parentMap["docnm_kwd"],
			"kb_id":               parentMap["kb_id"],
			"important_kwd":       allImportantKwd,
			"image_id":            imgID,
			"similarity":          avgSim,
			"vector_similarity":   avgSim,
			"term_similarity":     avgSim,
			"vector":              make([]float64, vectorSize),
			"positions":           parentMap["position_int"],
			"doc_type_kwd":        docTypeKwd,
		}

		// Get vector from first child if available
	childVecLoop:
		for _, c := range childList {
			for k := range c.chunk {
				if strings.HasSuffix(k, "_vec") {
					if vec, ok := c.chunk[k].([]float64); ok {
						aggregated["vector"] = vec
						vectorSize = len(vec)
						break childVecLoop
					}
				}
			}
		}

		remainingChunks = append(remainingChunks, aggregated)
	}

	// Sort by similarity descending
	for i := 0; i < len(remainingChunks); i++ {
		for j := i + 1; j < len(remainingChunks); j++ {
			simI, _ := remainingChunks[i]["similarity"].(float64)
			simJ, _ := remainingChunks[j]["similarity"].(float64)
			if simJ > simI {
				remainingChunks[i], remainingChunks[j] = remainingChunks[j], remainingChunks[i]
			}
		}
	}

	common.Info("RetrievalByChildren finished", zap.Int("momChunks", len(momChunks)), zap.Int("resultChunks", len(remainingChunks)))
	return remainingChunks
}

// PruneDeletedChunks removes chunks whose documents no longer exist
func (s *RetrievalService) PruneDeletedChunks(ctx context.Context, result *RetrievalSearchResult) (*RetrievalSearchResult, error) {
	if s.documentDAO == nil {
		return nil, fmt.Errorf("documentDAO is not initialized")
	}
	// Collect all doc_ids from chunks
	chunkDocIDs := make([]string, 0, len(result.Field))
	for _, chunk := range result.Field {
		if docID, ok := chunk["doc_id"].(string); ok && docID != "" {
			chunkDocIDs = append(chunkDocIDs, docID)
		}
	}

	if len(chunkDocIDs) == 0 {
		return result, nil
	}

	// Deduplicate chunkDocIDs for correct comparison with existingDocIDs
	uniqueDocIDs := make([]string, 0, len(chunkDocIDs))
	seen := make(map[string]struct{}, len(chunkDocIDs))
	for _, id := range chunkDocIDs {
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			uniqueDocIDs = append(uniqueDocIDs, id)
		}
	}

	// Get existing document IDs
	docs, err := s.documentDAO.GetByIDs(ctx, dao.DB, uniqueDocIDs)
	if err != nil {
		return nil, fmt.Errorf("GetByIDs failed: %w", err)
	}

	existingDocIDs := make(map[string]struct{}, len(docs))
	for _, doc := range docs {
		existingDocIDs[doc.ID] = struct{}{}
	}

	// Early return if all docs exist
	if len(existingDocIDs) == len(uniqueDocIDs) {
		return result, nil
	}

	// Filter out chunks with deleted documents
	filteredIDs := make([]string, 0, len(result.IDs))
	filteredChunks := make([]map[string]interface{}, 0, len(result.IDs))
	filteredField := make(map[string]map[string]interface{}, len(result.IDs))
	filteredHighlight := make(map[string]string)
	removed := 0

	for _, chunkID := range result.IDs {
		chunk, exists := result.Field[chunkID]
		if !exists {
			continue
		}
		docID, ok := chunk["doc_id"].(string)
		if !ok || docID == "" {
			// Keep chunks without doc_id
			filteredIDs = append(filteredIDs, chunkID)
			filteredChunks = append(filteredChunks, chunk)
			filteredField[chunkID] = chunk
			if result.Highlight != nil {
				if hl, ok := result.Highlight[chunkID]; ok {
					filteredHighlight[chunkID] = hl
				}
			}
			continue
		}
		if _, docExists := existingDocIDs[docID]; !docExists {
			removed++
			continue
		}
		filteredIDs = append(filteredIDs, chunkID)
		filteredChunks = append(filteredChunks, chunk)
		filteredField[chunkID] = chunk
		if result.Highlight != nil {
			if hl, ok := result.Highlight[chunkID]; ok {
				filteredHighlight[chunkID] = hl
			}
		}
	}

	if removed > 0 {
		common.Warn("Pruned stale chunks whose documents no longer exist", zap.Int("removed", removed))
	}

	return &RetrievalSearchResult{
		Chunks:      filteredChunks,
		Total:       int64(len(filteredIDs)),
		QueryVector: result.QueryVector,
		Highlight:   filteredHighlight,
		Field:       filteredField,
		IDs:         filteredIDs,
		Keywords:    result.Keywords,
		Aggregation: result.Aggregation,
		Options:     result.Options,
	}, nil
}

// buildIndexNames creates index names for the given tenant IDs.
// Each tenantID may be a comma-separated list.
func buildIndexNames(tenantIDs []string) []string {
	var indexNames []string
	for _, tid := range tenantIDs {
		for part := range strings.SplitSeq(tid, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				indexNames = append(indexNames, fmt.Sprintf("ragflow_%s", part))
			}
		}
	}
	return indexNames
}

// FetchChunkVectors returns q_{dim}_vec for the given chunk IDs.
// Missing or wrong-dimension chunks get a zero vector.
func (s *RetrievalService) FetchChunkVectors(ctx context.Context, chunkIDs []string, tenantIDs []string, kbIDs []string, dim int) (map[string][]float64, error) {
	if dim <= 0 {
		return nil, fmt.Errorf("FetchChunkVectors: dim must be > 0, got %d", dim)
	}
	if len(chunkIDs) == 0 {
		return map[string][]float64{}, nil
	}

	vecField := fmt.Sprintf("q_%d_vec", dim)
	idxNames := buildIndexNames(tenantIDs)

	req := &types.SearchRequest{
		IndexNames:   idxNames,
		KbIDs:        kbIDs,
		Limit:        len(chunkIDs),
		Offset:       0,
		SelectFields: []string{"id", vecField},
		Filter:       map[string]interface{}{"id": chunkIDs},
		MatchExprs:   []interface{}{},
	}

	result, err := s.docEngine.Search(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("FetchChunkVectors: engine search failed: %w", err)
	}

	out := make(map[string][]float64, len(chunkIDs))
	for _, cid := range chunkIDs {
		out[cid] = make([]float64, dim)
	}

	for _, chunk := range result.Chunks {
		cid, _ := chunk["id"].(string)
		if cid == "" {
			continue
		}
		var vec []float64
		switch v := chunk[vecField].(type) {
		case []float64:
			vec = v
		case []interface{}:
			vec = make([]float64, len(v))
			for i, val := range v {
				if f, ok := val.(float64); ok {
					vec[i] = f
				} else if f32, ok := val.(float32); ok {
					vec[i] = float64(f32)
				}
			}
		case string:
			// Tab-separated floats (mirrors Python's split("\t") in
			// search.py:435-437 when Infinity returns vectors as a string).
			parts := strings.Split(v, "\t")
			vec = make([]float64, 0, len(parts))
			for _, p := range parts {
				if f, err := strconv.ParseFloat(strings.TrimSpace(p), 64); err == nil {
					vec = append(vec, f)
				}
			}
		}
		if len(vec) != dim {
			vec = make([]float64, dim)
		}
		out[cid] = vec
	}

	return out, nil
}
