package service

import (
	"context"
	"fmt"

	"ragflow/internal/config"
	"ragflow/internal/engine"
	"ragflow/internal/engine/elasticsearch"
	"ragflow/internal/engine/infinity"
)

// ChunkService chunk service
type ChunkService struct {
	docEngine    engine.DocEngine
	engineType   config.EngineType
	modelProvider ModelProvider
}

// NewChunkService creates chunk service
func NewChunkService() *ChunkService {
	cfg := config.Get()
	return &ChunkService{
		docEngine:    engine.Get(),
		engineType:   cfg.DocEngine.Type,
		modelProvider: NewModelProvider(),
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
func (s *ChunkService) RetrievalTest(req *RetrievalTestRequest) (*RetrievalTestResponse, error) {
	if s.docEngine == nil {
		return nil, fmt.Errorf("doc engine not initialized")
	}

	ctx := context.Background()

	// Execute different retrieval logic based on engine type
	switch s.engineType {
	case config.EngineElasticsearch:
		return s.elasticsearchRetrieval(ctx, req)
	case config.EngineInfinity:
		return s.infinityRetrieval(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported engine type: %s", s.engineType)
	}
}

// elasticsearchRetrieval Elasticsearch retrieval implementation
func (s *ChunkService) elasticsearchRetrieval(ctx context.Context, req *RetrievalTestRequest) (*RetrievalTestResponse, error) {
	// Build index name list (based on kb_id)
	indexNames := buildIndexNames(req.KbID)

	// Build search request
	searchReq := &elasticsearch.SearchRequest{
		IndexNames: indexNames,
		Size:       getPageSize(req.Size),
		From:       getOffset(req.Page),
	}

	// If there's a question, build vector search query
	if req.Question != "" {
		// Get embedding model
		embeddingModel, err := s.modelProvider.GetEmbeddingModel(ctx, "")
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding model: %w", err)
		}

		// Generate vector for the question
		vector, err := embeddingModel.EncodeQuery(req.Question)
		if err != nil {
			return nil, fmt.Errorf("failed to encode query: %w", err)
		}

		// Build knn query
		searchReq.Query = map[string]interface{}{
			"knn": map[string]interface{}{
				"field":         "embedding",
				"query_vector":  vector,
				"k":             10,
				"num_candidates": 100,
			},
		}
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
		Total:  esResp.Hits.Total.Value,
	}, nil
}

// infinityRetrieval Infinity retrieval implementation
func (s *ChunkService) infinityRetrieval(ctx context.Context, req *RetrievalTestRequest) (*RetrievalTestResponse, error) {
	// Build table name (based on kb_id)
	tableName := buildTableName(req.KbID)

	// Build search request
	searchReq := &infinity.SearchRequest{
		TableName: tableName,
		Limit:     getPageSize(req.Size),
		Offset:    getOffset(req.Page),
	}

	// If there's a question, add text match
	if req.Question != "" {
		searchReq.MatchText = &infinity.MatchTextExpr{
			Fields:       []string{"title", "content"},
			MatchingText: req.Question,
			TopN:        getPageSize(req.Size),
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
	return 10 // default value
}

// getOffset gets offset
func getOffset(page *int) int {
	if page != nil && *page > 0 {
		return (*page - 1) * getPageSize(page)
	}
	return 0
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
