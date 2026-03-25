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
	"ragflow/internal/server"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/logger"
	"ragflow/internal/model"
	"ragflow/internal/service/nlp"
	"ragflow/internal/utility"
)

// ChunkService chunk service
type ChunkService struct {
	docEngine      engine.DocEngine
	engineType     server.EngineType
	modelProvider  ModelProvider
	embeddingCache *utility.EmbeddingLRU
	kbDAO          *dao.KnowledgebaseDAO
	userTenantDAO  *dao.UserTenantDAO
}

// NewChunkService creates chunk service
func NewChunkService() *ChunkService {
	cfg := server.GetConfig()
	return &ChunkService{
		docEngine:      engine.Get(),
		engineType:     cfg.DocEngine.Type,
		modelProvider:  NewModelProvider(),
		embeddingCache: utility.NewEmbeddingLRU(1000), // default capacity
		kbDAO:          dao.NewKnowledgebaseDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
	}
}

// RetrievalTestRequest retrieval test request
type RetrievalTestRequest struct {
	KbID                   interface{}            `json:"kb_id" binding:"required"` // string or []string
	Question               string                 `json:"question" binding:"required"`
	Page                   *int                   `json:"page,omitempty"`
	Size                   *int                   `json:"size,omitempty"`
	DocIDs                 []string               `json:"doc_ids,omitempty"`
	UseKG                  *bool                  `json:"use_kg,omitempty"`
	TopK                   *int                   `json:"top_k,omitempty"`
	CrossLanguages         []string               `json:"cross_languages,omitempty"`
	SearchID               *string                `json:"search_id,omitempty"`
	MetaDataFilter         map[string]interface{} `json:"meta_data_filter,omitempty"`
	RerankID               *string                `json:"rerank_id,omitempty"`
	Keyword                *bool                  `json:"keyword,omitempty"`
	SimilarityThreshold    *float64               `json:"similarity_threshold,omitempty"`
	VectorSimilarityWeight *float64               `json:"vector_similarity_weight,omitempty"`
	TenantIDs              []string               `json:"tenant_ids,omitempty"`
}

// RetrievalTestResponse retrieval test response
type RetrievalTestResponse struct {
	Chunks  []map[string]interface{} `json:"chunks"`
	DocAggs []map[string]interface{} `json:"doc_aggs"`
	Labels  *[]map[string]interface{} `json:"labels"`
	Total   int64                    `json:"total,omitempty"`
}

// RetrievalTest performs retrieval test
func (s *ChunkService) RetrievalTest(req *RetrievalTestRequest, userID string) (*RetrievalTestResponse, error) {
	if s.docEngine == nil {
		return nil, fmt.Errorf("doc engine not initialized")
	}

	// Validate question is required
	if req.Question == "" {
		return nil, fmt.Errorf("question is required")
	}

	ctx := context.Background()

	// Get user's tenants
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil, fmt.Errorf("user has no accessible tenants")
	}
	logger.Debug("Retrieved user tenants from database", zap.String("userID", userID), zap.Int("tenantCount", len(tenants)))

	// Determine kb_id list
	var kbIDs []string
	switch v := req.KbID.(type) {
	case string:
		kbIDs = []string{v}
	case []interface{}:
		for _, item := range v {
			if str, ok := item.(string); ok {
				kbIDs = append(kbIDs, str)
			} else {
				return nil, fmt.Errorf("kb_id array must contain strings")
			}
		}
	case []string:
		kbIDs = v
	default:
		return nil, fmt.Errorf("kb_id must be string or array of strings")
	}

	if len(kbIDs) == 0 {
		return nil, fmt.Errorf("kb_id cannot be empty")
	}

	// Check permission for each kb_id
	var tenantIDs []string
	var kbRecords []*model.Knowledgebase

	for _, kbID := range kbIDs {
		found := false
		for _, tenant := range tenants {
			kb, err := s.kbDAO.GetByIDAndTenantID(kbID, tenant.TenantID)
			if err == nil && kb != nil {
				logger.Debug("Found knowledge base record in database",
					zap.String("kbID", kbID),
					zap.String("tenantID", tenant.TenantID),
					zap.String("kbName", kb.Name),
					zap.String("embdID", kb.EmbdID))
				tenantIDs = append(tenantIDs, tenant.TenantID)
				kbRecords = append(kbRecords, kb)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("only owner of dataset is authorized for this operation")
		}
	}

	// Check if all kb records have the same embedding model
	if len(kbRecords) > 1 {
		firstEmbdID := kbRecords[0].EmbdID
		for i := 1; i < len(kbRecords); i++ {
			if kbRecords[i].EmbdID != firstEmbdID {
				return nil, fmt.Errorf("cannot retrieve across datasets with different embedding models")
			}
		}
	}

	// Get user's owner tenants to prioritize
	ownerTenants, err := s.userTenantDAO.GetByUserIDAndRole(userID, "owner")
	if err != nil {
		return nil, fmt.Errorf("failed to get user owner tenants: %w", err)
	}
	logger.Debug("Retrieved owner tenants from database",
		zap.String("userID", userID),
		zap.Int("ownerTenantCount", len(ownerTenants)))

	req.TenantIDs = tenantIDs
	// Choose target tenant: prioritize owner tenant if available in tenantIDs
	targetTenantID := tenantIDs[0]

	// Get embedding model for the target tenant
	embeddingModel, err := s.modelProvider.GetEmbeddingModel(ctx, targetTenantID, kbRecords[0].EmbdID)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding model: %w", err)
	}
	logger.Debug("Retrieved embedding model from database",
		zap.String("targetTenantID", targetTenantID),
		zap.String("embdID", kbRecords[0].EmbdID))

	// Try to get embedding from cache first
	embdID := kbRecords[0].EmbdID
	var questionVector []float64

	if s.embeddingCache != nil {
		if cachedVector, ok := s.embeddingCache.Get(req.Question, embdID); ok {
			logger.Debug("Embedding cache hit",
				zap.String("question", req.Question),
				zap.String("embdID", embdID),
				zap.Int("cacheSize", s.embeddingCache.Len()))
			questionVector = cachedVector
		} else {
			// Cache miss, encode and store
			questionVector, err = embeddingModel.EncodeQuery(req.Question)
			if err != nil {
				return nil, fmt.Errorf("failed to encode query: %w", err)
			}
			s.embeddingCache.Put(req.Question, embdID, questionVector)
			logger.Debug("Embedding cache miss, stored",
				zap.String("question", req.Question),
				zap.String("embdID", embdID),
				zap.Int("vectorDim", len(questionVector)),
				zap.Int("cacheSize", s.embeddingCache.Len()))
		}
	} else {
		// No cache, just encode
		questionVector, err = embeddingModel.EncodeQuery(req.Question)
		if err != nil {
			return nil, fmt.Errorf("failed to encode query: %w", err)
		}
	}

	// Use global QueryBuilder to process question and get matchText and keywords
	// Reference: rag/nlp/search.py L115
	queryBuilder := nlp.GetQueryBuilder()
	if queryBuilder == nil {
		return nil, fmt.Errorf("query builder not initialized")
	}
	matchTextExpr, keywords := queryBuilder.Question(req.Question, "qa", 0.6)

	//if matchTextExpr == nil {
	//	return nil, fmt.Errorf("failed to process question")
	//}
	logger.Debug("QueryBuilder processed question",
		zap.String("original", req.Question),
		zap.String("matchingText", matchTextExpr.MatchingText),
		zap.Strings("keywords", keywords))

	// Build unified search request
	searchReq := &engine.SearchRequest{
		IndexNames:             buildIndexNames(tenantIDs),
		Question:               req.Question,
		MatchText:              matchTextExpr.MatchingText,
		Keywords:               keywords,
		Vector:                 questionVector,
		KbIDs:                  kbIDs,
		DocIDs:                 req.DocIDs,
		Page:                   getPageNum(req.Page),
		Size:                   getPageSize(req.Size),
		TopK:                   getTopK(req.TopK),
		KeywordOnly:            req.Keyword != nil && *req.Keyword,
		SimilarityThreshold:    getSimilarityThreshold(req.SimilarityThreshold),
		VectorSimilarityWeight: getVectorSimilarityWeight(req.VectorSimilarityWeight),
	}

	// Execute search through unified engine interface
	result, err := s.docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert result to unified response
	searchResp, ok := result.(*engine.SearchResponse)
	if !ok {
		return nil, fmt.Errorf("invalid search response type")
	}

	//return &RetrievalTestResponse{
	//	Chunks: searchResp.Chunks,
	//	Labels: []map[string]interface{}{}, // Empty labels for now
	//	Total:  searchResp.Total,
	//}, nil

	//// Build SearchResult for reranker
	//sres := buildSearchResult(searchResp, questionVector)
	//
	// Get rerank model if RerankID is specified (can be nil)
	var rerankModel nlp.RerankModel
	if req.RerankID != nil && *req.RerankID != "" {
		rerankModel, err = s.modelProvider.GetRerankModel(ctx, targetTenantID, *req.RerankID)
		if err != nil {
			logger.Warn("Failed to get rerank model, falling back to standard reranking", zap.Error(err))
			rerankModel = nil
		}
	}

	// Perform reranking
	// Reference: rag/nlp/search.py L404-L429
	vtWeight := getVectorSimilarityWeight(req.VectorSimilarityWeight)
	tkWeight := 1.0 - vtWeight
	useInfinity := s.engineType == server.EngineInfinity

	sim, term_similarity, vector_similarity := nlp.Rerank(
		rerankModel,
		searchResp,
		keywords,
		questionVector,
		nil,
		req.Question,
		tkWeight,
		vtWeight,
		useInfinity,
		"content_ltks",
		queryBuilder,
	)
	//
	// Apply similarity threshold and sort chunks
	similarityThreshold := getSimilarityThreshold(req.SimilarityThreshold)
	filteredChunks := applyRerankResults(searchResp.Chunks, sim, similarityThreshold)
	for idx, _ := range filteredChunks {
		filteredChunks[idx]["similarity"] = sim[idx]
		filteredChunks[idx]["term_similarity"] = term_similarity[idx]
		filteredChunks[idx]["vector_similarity"] = vector_similarity[idx]
	}

	convertedChunks := buildRetrievalTestResults(filteredChunks)

	// Build doc_aggs by aggregating chunks by docnm
	docAggsMap := make(map[string]struct {
		docID string
		count int
	})
	docNameOrder := []string{} // Track insertion order of doc names
	for _, chunk := range filteredChunks {
		docName := ""
		docID := ""
		if v, ok := chunk["docnm"].(string); ok {
			docName = v
		}
		if v, ok := chunk["doc_id"].(string); ok {
			docID = v
		}
		if docName == "" {
			continue
		}
		if entry, exists := docAggsMap[docName]; exists {
			entry.count++
			docAggsMap[docName] = entry
		} else {
			docAggsMap[docName] = struct {
				docID string
				count int
			}{docID: docID, count: 1}
			docNameOrder = append(docNameOrder, docName)
		}
	}

	// Convert to list maintaining insertion order
	type docAggEntry struct {
		docName string
		docID   string
		count   int
		order   int
	}
	docAggsList := make([]docAggEntry, 0, len(docAggsMap))
	for order, docName := range docNameOrder {
		entry := docAggsMap[docName]
		docAggsList = append(docAggsList, docAggEntry{docName: docName, docID: entry.docID, count: entry.count, order: order})
	}
	// Sort by count descending, then by order ascending (for tie-breaking)
	for i := 0; i < len(docAggsList)-1; i++ {
		for j := i + 1; j < len(docAggsList); j++ {
			if docAggsList[j].count > docAggsList[i].count ||
				(docAggsList[j].count == docAggsList[i].count && docAggsList[j].order < docAggsList[i].order) {
				docAggsList[i], docAggsList[j] = docAggsList[j], docAggsList[i]
			}
		}
	}
	docAggs := make([]map[string]interface{}, 0, len(docAggsList))
	for _, entry := range docAggsList {
		docAggs = append(docAggs, map[string]interface{}{
			"doc_name": entry.docName,
			"doc_id":   entry.docID,
			"count":    entry.count,
		})
	}

	return &RetrievalTestResponse{
		Chunks:  convertedChunks,
		DocAggs: docAggs,
		Labels:  nil,
		Total:   int64(len(convertedChunks)),
	}, nil
}

// Helper functions

func getPageNum(page *int) int {
	if page != nil && *page > 0 {
		return *page
	}
	return 1
}

func getPageSize(size *int) int {
	if size != nil && *size > 0 {
		return *size
	}
	return 30
}

func getTopK(topk *int) int {
	if topk != nil && *topk > 0 {
		return *topk
	}
	return 1024
}

func getSimilarityThreshold(threshold *float64) float64 {
	if threshold != nil && *threshold >= 0 {
		return *threshold
	}
	return 0.1
}

func getVectorSimilarityWeight(weight *float64) float64 {
	if weight != nil && *weight >= 0 && *weight <= 1 {
		return *weight
	}
	return 0.3
}

func buildIndexNames(tenantIDs []string) []string {
	indexNames := make([]string, len(tenantIDs))
	for i, tenantID := range tenantIDs {
		indexNames[i] = fmt.Sprintf("ragflow_%s", tenantID)
	}
	return indexNames
}

// buildSearchResult converts engine.SearchResponse to nlp.SearchResult for reranking
func buildSearchResult(resp *engine.SearchResponse, queryVector []float64) *nlp.SearchResult {
	field := make(map[string]map[string]interface{})
	ids := make([]string, 0, len(resp.Chunks))

	for i, chunk := range resp.Chunks {
		// Extract ID from chunk
		id := ""
		if idVal, ok := chunk["_id"].(string); ok {
			id = idVal
		} else {
			id = fmt.Sprintf("chunk_%d", i)
		}
		ids = append(ids, id)

		// Store fields by id
		field[id] = chunk
	}

	return &nlp.SearchResult{
		Total:       len(resp.Chunks),
		IDs:         ids,
		QueryVector: queryVector,
		Field:       field,
	}
}

// applyRerankResults sorts and filters chunks based on reranking results
// Reference: rag/nlp/search.py L430-L439
func applyRerankResults(chunks []map[string]interface{}, sim []float64, threshold float64) []map[string]interface{} {
	if len(chunks) == 0 || len(sim) == 0 {
		return chunks
	}

	// Get sorted indices (descending by similarity)
	sortedIndices := nlp.ArgsortDescending(sim)

	// Sort and filter chunks based on reranking results
	var filteredChunks []map[string]interface{}
	for _, idx := range sortedIndices {
		if idx < 0 || idx >= len(chunks) {
			continue
		}
		if sim[idx] >= threshold {
			chunk := chunks[idx]
			// Add similarity score to chunk
			chunk["_score"] = sim[idx]
			filteredChunks = append(filteredChunks, chunk)
		}
	}

	return filteredChunks
}

// buildRetrievalTestResults converts filtered chunks to retrieval test results with renamed keys
func buildRetrievalTestResults(filteredChunks []map[string]interface{}) []map[string]interface{} {
	results := make([]map[string]interface{}, 0, len(filteredChunks))

	for _, chunk := range filteredChunks {
		result := make(map[string]interface{})

		// Key mappings
		if v, ok := chunk["id"]; ok {
			result["chunk_id"] = v
		} else if v, ok := chunk["_id"]; ok {
			result["chunk_id"] = v
		}
		if v, ok := chunk["content"]; ok {
			result["content_ltks"] = v
			result["content_with_weight"] = v
		} else {
			if v, ok := chunk["content_ltks"]; ok {
				result["content_ltks"] = v
			}
			if v, ok := chunk["content_with_weight"]; ok {
				result["content_with_weight"] = v
			}
		}
		if v, ok := chunk["doc_id"]; ok {
			result["doc_id"] = v
		}
		if v, ok := chunk["docnm"]; ok {
			result["docnm_kwd"] = v
		} else if v, ok := chunk["docnm_kwd"]; ok {
			result["docnm_kwd"] = v
		}
		if v, ok := chunk["img_id"]; ok {
			result["image_id"] = v
		}
		if v, ok := chunk["kb_id"]; ok {
			result["kb_id"] = v
		}
		if v, ok := chunk["position_int"]; ok {
			result["positions"] = v
		}
		if v, ok := chunk["doc_type_kwd"]; ok {
			result["doc_type_kwd"] = v
		}
		if v, ok := chunk["mom_id"]; ok {
			result["mom_id"] = v
		}
		if v, ok := chunk["important_kwd"]; ok {
			result["important_kwd"] = v
		} else if v, ok := chunk["important_keywords"]; ok {
			result["important_kwd"] = v
		}
		if v, ok := chunk["similarity"]; ok {
			result["similarity"] = v
		}
		if v, ok := chunk["term_similarity"]; ok {
			result["term_similarity"] = v
		}
		if v, ok := chunk["vector_similarity"]; ok {
			result["vector_similarity"] = v
		}

		results = append(results, result)
	}

	return results
}

// GetChunkRequest request for getting a chunk by ID
type GetChunkRequest struct {
	ChunkID string `json:"chunk_id"`
}

// GetChunkResponse response for getting a chunk
type GetChunkResponse struct {
	Chunk map[string]interface{} `json:"chunk"`
}

// Get retrieves a chunk by ID
func (s *ChunkService) Get(req *GetChunkRequest, userID string) (*GetChunkResponse, error) {
	if s.docEngine == nil {
		return nil, fmt.Errorf("doc engine not initialized")
	}

	if req.ChunkID == "" {
		return nil, fmt.Errorf("chunk_id is required")
	}

	ctx := context.Background()

	// Get user's tenants
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil, fmt.Errorf("user has no accessible tenants")
	}

	// Try each tenant to find the chunk
	var chunk map[string]interface{}
	for _, tenant := range tenants {
		// Get kbIDs for this tenant
		kbIDs, err := s.kbDAO.GetKBIDsByTenantID(tenant.TenantID)
		if err != nil {
			continue
		}

		indexName := fmt.Sprintf("ragflow_%s", tenant.TenantID)

		doc, err := s.docEngine.GetChunk(ctx, indexName, req.ChunkID, kbIDs)
		if err != nil {
			continue
		}

		if doc != nil {
			chunk, ok := doc.(map[string]interface{})
			if ok {
				// Format to match Python output
				result := make(map[string]interface{})
				skipFields := map[string]bool{
					"id": true, "authors": true, "_score": true, "SCORE": true,
				}
				for k, v := range chunk {
					if skipFields[k] || strings.HasSuffix(k, "_vec") || strings.Contains(k, "_sm_") || strings.HasSuffix(k, "_tks") || strings.HasSuffix(k, "_ltks") {
						continue
					}
					switch k {
					case "content":
						result["content_with_weight"] = v
					case "docnm":
						result["docnm_kwd"] = v
					case "important_keywords":
						utility.SetFieldArray(result, "important_kwd", v)
					case "questions":
						utility.SetFieldArray(result, "question_kwd", v)
					case "entities_kwd", "entity_kwd", "entity_type_kwd", "from_entity_kwd",
						"name_kwd", "raptor_kwd", "removed_kwd", "source_id", "tag_kwd",
						"to_entity_kwd", "toc_kwd", "authors_tks", "doc_type_kwd":
						if utility.IsEmpty(v) {
							result[k] = []interface{}{}
						} else {
							result[k] = v
						}
					case "tag_feas":
						if utility.IsEmpty(v) {
							result[k] = map[string]interface{}{}
						} else {
							result[k] = v
						}
					case "create_timestamp_flt", "rank_flt", "weight_flt":
						if floatVal, ok := utility.ToFloat64(v); ok {
							result[k] = utility.JSONFloat64(floatVal)
						}
					default:
						result[k] = v
					}
				}
				return &GetChunkResponse{Chunk: result}, nil
			}
		}
	}

	if chunk == nil {
		return nil, fmt.Errorf("chunk not found")
	}

	return &GetChunkResponse{Chunk: chunk}, nil
}

// ListChunksRequest request for listing chunks
type ListChunksRequest struct {
	DocID       string `json:"doc_id" binding:"required"`
	Page        *int   `json:"page,omitempty"`
	Size        *int   `json:"size,omitempty"`
	Keywords    string `json:"keywords,omitempty"`
	AvailableInt *int  `json:"available_int,omitempty"`
}

// ListChunksResponse response for listing chunks
type ListChunksResponse struct {
	Chunks []map[string]interface{} `json:"chunks"`
	Doc    map[string]interface{}   `json:"doc"`
	Total  int64                    `json:"total"`
}

// List retrieves chunks for a document
func (s *ChunkService) List(req *ListChunksRequest, userID string) (*ListChunksResponse, error) {
	if s.docEngine == nil {
		return nil, fmt.Errorf("doc engine not initialized")
	}

	if req.DocID == "" {
		return nil, fmt.Errorf("doc_id is required")
	}

	ctx := context.Background()

	// Get user's tenants
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil, fmt.Errorf("user has no accessible tenants")
	}

	// Get document to find its tenant
	docDAO := dao.NewDocumentDAO()
	doc, err := docDAO.GetByID(req.DocID)
	if err != nil || doc == nil {
		return nil, fmt.Errorf("document not found")
	}

	// Get knowledge base to find tenant
	kb, err := s.kbDAO.GetByID(doc.KbID)
	if err != nil || kb == nil {
		return nil, fmt.Errorf("knowledge base not found")
	}

	// Find which tenant this document belongs to
	var targetTenantID string
	for _, tenant := range tenants {
		if tenant.TenantID == kb.TenantID {
			targetTenantID = tenant.TenantID
			break
		}
	}
	if targetTenantID == "" {
		return nil, fmt.Errorf("user does not have access to this document")
	}

	// Get kbIDs for this tenant
	kbIDs, err := s.kbDAO.GetKBIDsByTenantID(targetTenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get kb ids: %w", err)
	}

	indexName := fmt.Sprintf("ragflow_%s", targetTenantID)

	page := getPageNum(req.Page)
	size := getPageSize(req.Size)
	keywords := req.Keywords

	// Build search request - same as retrieval test but filtered by doc_id
	searchReq := &engine.SearchRequest{
		IndexNames: []string{indexName},
		Question:   keywords,
		KbIDs:      kbIDs,
		DocIDs:     []string{req.DocID},
		Page:       page,
		Size:       size,
		TopK:       size,
	}

	// Add available_int filter if specified
	if req.AvailableInt != nil {
		searchReq.AvailableInt = req.AvailableInt
	}

	// Execute search through unified engine interface
	result, err := s.docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert result to unified response
	searchResp, ok := result.(*engine.SearchResponse)
	if !ok {
		return nil, fmt.Errorf("invalid search response type")
	}

	// Format output to match Python
	chunks := make([]map[string]interface{}, 0, len(searchResp.Chunks))
	for _, chunk := range searchResp.Chunks {
		// Inline formatChunkForList
		result := make(map[string]interface{})
		skipFields := map[string]bool{
			"_id": true, "authors": true, "_score": true, "SCORE": true,
			"important_kwd_empty_count": true, "kb_id": true, "mom_id": true, "page_num_int": true,
		}
		for k, v := range chunk {
			if skipFields[k] || strings.HasSuffix(k, "_vec") || strings.Contains(k, "_sm_") || strings.HasSuffix(k, "_ltks") || strings.HasSuffix(k, "_tks") {
				continue
			}
			switch k {
			case "img_id":
				if strVal, ok := v.(string); ok {
					result["image_id"] = strVal
				} else {
					result["image_id"] = ""
				}
			case "position_int":
                result["positions"] = v
			case "id":
				result["chunk_id"] = v
			case "content":
				result["content_with_weight"] = v
			case "docnm":
				result["docnm_kwd"] = v
			case "important_keywords":
				utility.SetFieldArray(result, "important_kwd", v)
			case "questions":
				utility.SetFieldArray(result, "question_kwd", v)
			case "entities_kwd", "entity_kwd", "entity_type_kwd", "from_entity_kwd",
				"name_kwd", "raptor_kwd", "removed_kwd",
				"source_id", "tag_kwd", "to_entity_kwd", "toc_kwd", "doc_type_kwd":
				if utility.IsEmpty(v) {
					result[k] = []interface{}{}
				} else {
					result[k] = v
				}
			default:
				// Handle _kwd fields that need "###" splitting
				if strings.HasSuffix(k, "_kwd") && k != "knowledge_graph_kwd" {
					if strVal, ok := v.(string); ok && strings.Contains(strVal, "###") {
						parts := strings.Split(strVal, "###")
						var filtered []interface{}
						for _, p := range parts {
							if p != "" {
								filtered = append(filtered, p)
							}
						}
						result[k] = filtered
					} else {
						result[k] = v
					}
				} else {
					result[k] = v
				}
			}
		}
		chunks = append(chunks, result)
	}

	// Build document info (matching Python doc.to_dict())
	timeFormat := "2006-01-02T15:04:05"
	docInfo := map[string]interface{}{
		"id":                doc.ID,
		"thumbnail":         doc.Thumbnail,
		"kb_id":             doc.KbID,
		"parser_id":         doc.ParserID,
		"pipeline_id":       doc.PipelineID,
		"parser_config":     doc.ParserConfig,
		"source_type":       doc.SourceType,
		"type":              doc.Type,
		"created_by":        doc.CreatedBy,
		"name":              doc.Name,
		"location":          doc.Location,
		"size":              doc.Size,
		"token_num":         doc.TokenNum,
		"chunk_num":         doc.ChunkNum,
		"progress":          utility.JSONFloat64(doc.Progress),
		"progress_msg":      doc.ProgressMsg,
		"process_begin_at":  utility.FormatTimeToString(doc.ProcessBeginAt, timeFormat),
		"process_duration":  doc.ProcessDuration,
		"content_hash":      doc.ContentHash,
		"suffix":            doc.Suffix,
		"run":               doc.Run,
		"status":            doc.Status,
		"create_time":       doc.CreateTime,
		"create_date":       utility.FormatTimeToString(doc.CreateDate, timeFormat),
		"update_time":       doc.UpdateTime,
		"update_date":       utility.FormatTimeToString(doc.UpdateDate, timeFormat),
	}

	return &ListChunksResponse{
		Total:  searchResp.Total,
		Chunks: chunks,
		Doc:    docInfo,
	}, nil
}
