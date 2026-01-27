package service

import (
	"context"
	"fmt"

	"ragflow/internal/config"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/infinity"
	"ragflow/internal/model"
)

// ChunkService chunk service
type ChunkService struct {
	docEngine     engine.DocEngine
	engineType    config.EngineType
	modelProvider ModelProvider
	kbDAO         *dao.KnowledgebaseDAO
	userTenantDAO *dao.UserTenantDAO
}

// NewChunkService creates chunk service
func NewChunkService() *ChunkService {
	cfg := config.Get()
	return &ChunkService{
		docEngine:     engine.Get(),
		engineType:    cfg.DocEngine.Type,
		modelProvider: NewModelProvider(),
		kbDAO:         dao.NewKnowledgebaseDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
	}
}

// RetrievalTestRequest retrieval test request
type RetrievalTestRequest struct {
	KbID                  interface{} `json:"kb_id" binding:"required"` // string or []string
	Question              string      `json:"question" binding:"required"`
	Page                  *int        `json:"page,omitempty"`
	Size                  *int        `json:"size,omitempty"`
	DocIDs                []string    `json:"doc_ids,omitempty"`
	UseKG                 *bool       `json:"use_kg,omitempty"`
	TopK                  *int        `json:"top_k,omitempty"`
	CrossLanguages        []string    `json:"cross_languages,omitempty"`
	SearchID              *string     `json:"search_id,omitempty"`
	MetaDataFilter        map[string]interface{} `json:"meta_data_filter,omitempty"`
	RerankID              *string     `json:"rerank_id,omitempty"`
	Keyword               *bool       `json:"keyword,omitempty"`
	SimilarityThreshold   *float64    `json:"similarity_threshold,omitempty"`
	VectorSimilarityWeight *float64   `json:"vector_similarity_weight,omitempty"`
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
				tenantIDs = append(tenantIDs, tenant.TenantID)
				kbRecords = append(kbRecords, kb)
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("Only owner of dataset authorized for this operation.")
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
	
	// Choose target tenant: prioritize owner tenant if available in tenantIDs
	targetTenantID := tenantIDs[0]
	if len(ownerTenants) > 0 {
		// Create a set of tenantIDs for quick lookup
		tenantIDSet := make(map[string]bool)
		for _, tid := range tenantIDs {
			tenantIDSet[tid] = true
		}
		// Find first owner tenant that is in tenantIDs
		for _, owner := range ownerTenants {
			if tenantIDSet[owner.TenantID] {
				targetTenantID = owner.TenantID
				break
			}
		}
	}

	// Get embedding model for the target tenant
	// Note: embedding model name is taken from the kb record's embd_id
	// All kb records have the same embd_id (checked above)
	embeddingModel, err := s.modelProvider.GetEmbeddingModel(ctx, targetTenantID, kbRecords[0].EmbdID)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding model: %w", err)
	}
	vector, err := embeddingModel.EncodeQuery(req.Question)
	if err != nil {
		return nil, fmt.Errorf("failed to encode query: %w", err)
	}

	// Execute different retrieval logic based on engine type
	switch s.engineType {
	case config.EngineElasticsearch:
		return s.elasticsearchRetrieval(ctx, req, vector)
	case config.EngineInfinity:
		return s.infinityRetrieval(ctx, req, vector)
	default:
		return nil, fmt.Errorf("unsupported engine type: %s", s.engineType)
	}
}

// elasticsearchRetrieval Elasticsearch retrieval implementation
func (s *ChunkService) elasticsearchRetrieval(ctx context.Context, req *RetrievalTestRequest, vector []float64) (*RetrievalTestResponse, error) {
	// Build index name list (based on kb_id)
	indexNames := buildIndexNames(req.KbID)

	// Build search request
	searchReq := &elasticsearch.SearchRequest{
		IndexNames: indexNames,
		Size:       getPageSize(req.Size),
		From:       getOffset(req.Page, req.Size),
	}

	// Build knn query using the pre-generated embedding vector
	topK := getTopK(req.TopK)
	searchReq.Query = map[string]interface{}{
		"knn": map[string]interface{}{
			"field":         "embedding",
			"query_vector":  vector,
			"k":             topK,
			"num_candidates": topK * 10, // Ensure enough candidates
		},
	}

	// Execute search
	result, err := s.docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert result
	esResp := result.(*elasticsearch.SearchResponse)
	chunks := convertESChunks(esResp)

	return &RetrievalTestResponse{
		Chunks: chunks,
		Labels: []map[string]interface{}{}, // Empty labels for now
		Total:  esResp.Hits.Total.Value,
	}, nil
}

// infinityRetrieval Infinity retrieval implementation
func (s *ChunkService) infinityRetrieval(ctx context.Context, req *RetrievalTestRequest, vector []float64) (*RetrievalTestResponse, error) {
	// Build table name (based on kb_id)
	tableName := buildTableName(req.KbID)

	// Build search request
	searchReq := &infinity.SearchRequest{
		TableName: tableName,
		Limit:     getPageSize(req.Size),
		Offset:    getOffset(req.Page, req.Size),
	}

	// Add text match (question is always required)
	searchReq.MatchText = &infinity.MatchTextExpr{
		Fields:       []string{"title", "content"},
		MatchingText: req.Question,
		TopN:        getTopK(req.TopK),
	}

	// Add vector match if vector is provided (for future support)
	if vector != nil && len(vector) > 0 {
		searchReq.MatchDense = &infinity.MatchDenseExpr{
			VectorColumnName:  "embedding",
			EmbeddingData:     vector,
			EmbeddingDataType: "float32",
			DistanceType:      "cosine",
			TopN:             getTopK(req.TopK),
			ExtraOptions:      nil,
		}
	}

	// Execute search
	result, err := s.docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert result
	infResp := result.(*infinity.SearchResponse)
	chunks := convertInfinityChunks(infResp.Rows)

	return &RetrievalTestResponse{
		Chunks: chunks,
		Labels: []map[string]interface{}{}, // Empty labels for now
		Total:  infResp.Total,
	}, nil
}

// buildIndexNames builds ES index name list
func buildIndexNames(kbID interface{}) []string {
	// TODO: Build actual index names based on kb_id
	// Simplified for now
	return []string{"chunks"}
}

// buildTableName builds Infinity table name
func buildTableName(kbID interface{}) string {
	// TODO: Build actual table name based on kb_id
	// Simplified for now
	return "chunks"
}

// getPageSize gets page size
func getPageSize(size *int) int {
	if size != nil && *size > 0 {
		return *size
	}
	return 30 // default value
}

// getOffset gets offset
func getOffset(page *int, size *int) int {
	if page != nil && *page > 0 {
		return (*page - 1) * getPageSize(size)
	}
	return 0
}

// getTopK gets top k value
func getTopK(topk *int) int {
	if topk != nil && *topk > 0 {
		return *topk
	}
	return 1024 // default value
}

// getUseKG gets use knowledge graph flag
func getUseKG(usekg *bool) bool {
	if usekg != nil {
		return *usekg
	}
	return false // default value
}

// convertESChunks converts ES returned chunks
func convertESChunks(esResp *elasticsearch.SearchResponse) []map[string]interface{} {
	if esResp == nil || esResp.Hits.Hits == nil {
		return []map[string]interface{}{}
	}

	chunks := make([]map[string]interface{}, len(esResp.Hits.Hits))
	for i, hit := range esResp.Hits.Hits {
		chunks[i] = hit.Source
		// Add score information
		chunks[i]["_score"] = hit.Score
	}
	return chunks
}

// convertInfinityChunks converts Infinity returned chunks
func convertInfinityChunks(rows []map[string]interface{}) []map[string]interface{} {
	if rows == nil {
		return []map[string]interface{}{}
	}
	return rows
}
