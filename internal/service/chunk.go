package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"ragflow/internal/config"
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
	engineType     config.EngineType
	modelProvider  ModelProvider
	embeddingCache *utility.EmbeddingLRU
	kbDAO          *dao.KnowledgebaseDAO
	userTenantDAO  *dao.UserTenantDAO
}

// NewChunkService creates chunk service
func NewChunkService() *ChunkService {
	cfg := config.Get()
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
	Chunks []map[string]interface{} `json:"chunks"`
	Labels []map[string]interface{} `json:"labels"`
	Total  int64                    `json:"total,omitempty"`
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

	return &RetrievalTestResponse{
		Chunks: searchResp.Chunks,
		Labels: []map[string]interface{}{}, // Empty labels for now
		Total:  searchResp.Total,
	}, nil

	//// Build SearchResult for reranker
	//sres := buildSearchResult(searchResp, questionVector)
	//
	//// Get rerank model if RerankID is specified (can be nil)
	//var rerankModel nlp.RerankModel
	//if req.RerankID != nil && *req.RerankID != "" {
	//	rerankModel, err = s.modelProvider.GetRerankModel(ctx, targetTenantID, *req.RerankID)
	//	if err != nil {
	//		logger.Warn("Failed to get rerank model, falling back to standard reranking", zap.Error(err))
	//		rerankModel = nil
	//	}
	//}
	//
	//// Perform reranking
	//// Reference: rag/nlp/search.py L404-L429
	//tkWeight := 1.0 - getVectorSimilarityWeight(req.VectorSimilarityWeight)
	//vtWeight := getVectorSimilarityWeight(req.VectorSimilarityWeight)
	//useInfinity := s.engineType == config.EngineInfinity
	//
	//sim, _, _ := nlp.Rerank(
	//	rerankModel,
	//	searchResp,
	//	keywords,
	//	questionVector,
	//	sres,
	//	req.Question,
	//	tkWeight,
	//	vtWeight,
	//	useInfinity,
	//	"content_ltks",
	//	queryBuilder,
	//)
	//
	//// Apply similarity threshold and sort chunks
	//similarityThreshold := getSimilarityThreshold(req.SimilarityThreshold)
	//filteredChunks := applyRerankResults(searchResp.Chunks, sim, similarityThreshold)
	//
	//return &RetrievalTestResponse{
	//	Chunks: filteredChunks,
	//	Labels: []map[string]interface{}{}, // Empty labels for now
	//	Total:  int64(len(filteredChunks)),
	//}, nil
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
	//if weight != nil && *weight >= 0 && *weight <= 1 {
	//	return *weight
	//}
	return 0.95
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
