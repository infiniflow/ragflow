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
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/server"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/logger"

	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
)

// ChunkService chunk service
type ChunkService struct {
	docEngine      engine.DocEngine
	engineType     server.EngineType
	embeddingCache *utility.EmbeddingLRU
	kbDAO          *dao.KnowledgebaseDAO
	userTenantDAO  *dao.UserTenantDAO
	documentDAO    *dao.DocumentDAO
	searchService  *SearchService
}

// NewChunkService creates chunk service
func NewChunkService() *ChunkService {
	cfg := server.GetConfig()
	return &ChunkService{
		docEngine:      engine.Get(),
		engineType:     cfg.DocEngine.Type,
		embeddingCache: utility.NewEmbeddingLRU(1000), // default capacity
		kbDAO:          dao.NewKnowledgebaseDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
		documentDAO:    dao.NewDocumentDAO(),
		searchService:  NewSearchService(),
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
	Filter                 map[string]interface{} `json:"meta_data_filter,omitempty"`
	TenantRerankID         *string                `json:"tenant_rerank_id,omitempty"`
	RerankID               *string                `json:"rerank_id,omitempty"`
	Keyword                *bool                  `json:"keyword,omitempty"`
	SimilarityThreshold    *float64               `json:"similarity_threshold,omitempty"`
	VectorSimilarityWeight *float64               `json:"vector_similarity_weight,omitempty"`
}

// RetrievalTestResponse retrieval test response
type RetrievalTestResponse struct {
	Chunks  []map[string]interface{} `json:"chunks"`
	DocAggs []map[string]interface{} `json:"doc_aggs"`
	Labels  *map[string]float64      `json:"labels"`
	Total   int64                    `json:"total"`
}

// RetrievalTest performs retrieval test for a given question against specified knowledge bases.
// Corresponds to Python's api/apps/chunk_app.py:retrieval_test()
//
// Flow:
//  1. Validate kbs permissions and embedding model
//  2. Apply metadata filter if specified (auto/semi_auto uses LLM, manual uses provided conditions)
//  3. Apply cross_languages transformation if requested (translate question)
//  4. Apply keyword extraction if requested (append keywords to question)
//  5. Get rank features via LabelQuestion() - tag-based weights or pagerank_fld fallback
//  6. Call RetrievalService.Retrieval() which:
//     - Computes query embedding
//     - Performs hybrid search (text + vector) with rank features
//     - Reranks results
//     - Builds doc_aggs by aggregating chunks per document
//  7. knowledge graph retrieval (not implemented)
//  8. Apply retrieval by children to group child chunks under parent chunks
func (s *ChunkService) RetrievalTest(req *RetrievalTestRequest, userID string) (*RetrievalTestResponse, error) {
	logger.Info("RetrievalTest started", zap.String("userID", userID), zap.Any("kbID", req.KbID), zap.String("question", req.Question))

	logger.Debug(fmt.Sprintf("RetrievalTest request:\n"+
		"    kbID=%v\n"+
		"    question=%s\n"+
		"    page=%v, size=%v\n"+
		"    docIDs=%v\n"+
		"    useKG=%v, topK=%v\n"+
		"    crossLanguages=%v\n"+
		"    searchID=%v\n"+
		"    filter=%v\n"+
		"    tenantRerankID=%v\n"+
		"    rerankID=%v\n"+
		"    keyword=%v\n"+
		"    similarityThreshold=%v, vectorSimilarityWeight=%v",
		req.KbID, req.Question,
		ptrString(req.Page), ptrString(req.Size), req.DocIDs,
		ptrString(req.UseKG), ptrString(req.TopK), req.CrossLanguages, ptrString(req.SearchID),
		req.Filter,
		ptrString(req.TenantRerankID), ptrString(req.RerankID),
		ptrString(req.Keyword),
		ptrString(req.SimilarityThreshold), ptrString(req.VectorSimilarityWeight)))

	if req.Question == "" {
		return nil, fmt.Errorf("question is required")
	}

	ctx := context.Background()

	// Determine kb_id list and check permission for each kb_id
	var kbIDs []string
	switch v := req.KbID.(type) {
	case string:
		kbIDs = []string{v}
	case []string:
		kbIDs = v
	default:
		return nil, fmt.Errorf("kb_id must be string or array of strings")
	}
	if len(kbIDs) == 0 {
		return nil, fmt.Errorf("kb_id cannot be empty")
	}

	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil, fmt.Errorf("user has no accessible tenants")
	}
	logger.Debug("Retrieved user tenants from database", zap.String("userID", userID), zap.Int("tenantCount", len(tenants)))

	var tenantIDs []string
	var kbRecords []*entity.Knowledgebase
	for _, kbID := range kbIDs {
		found := false
		for _, tenant := range tenants {
			kb, err := s.kbDAO.GetByIDAndTenantID(kbID, tenant.TenantID)
			if err == nil && kb != nil {
				logger.Debug("Found knowledge base in database",
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

	// Check if all kbs have the same embedding model
	if len(kbRecords) > 1 {
		firstEmbdID := kbRecords[0].EmbdID
		for i := 1; i < len(kbRecords); i++ {
			if kbRecords[i].EmbdID != firstEmbdID {
				return nil, fmt.Errorf("cannot retrieve across datasets with different embedding models")
			}
		}
	}

	// Determine meta_data_filter
	var chatID string
	var chatModelForFilter *models.ChatModel
	filter := req.Filter

	if req.SearchID != nil && *req.SearchID != "" {
		// If search_id is set, get meta_data_filter and chat_id from search_config
		searchDetail, err := s.searchService.GetDetail(*req.SearchID)
		if err != nil {
			logger.Warn("Failed to get search detail for search_id, proceeding without it", zap.String("searchID", *req.SearchID), zap.Error(err))
		} else if searchConfig, ok := searchDetail["search_config"].(entity.JSONMap); ok && searchConfig != nil {
			if searchMetaFilter, ok := searchConfig["meta_data_filter"].(map[string]interface{}); ok {
				filter = searchMetaFilter
			}
			chatID, _ = searchConfig["chat_id"].(string)
		} else {
			logger.Warn("No search_config found in search detail", zap.String("searchID", *req.SearchID))
		}
	}

	// If meta_data_filter method is auto/semi_auto, get chat model
	if filter != nil {
		method, _ := filter["method"].(string)
		if method == "auto" || method == "semi_auto" {
			modelProviderSvc := NewModelProviderService()
			if chatID != "" {
				// Use chat_id from search_config (it's actually the model name)
				chatModelForFilter, err = modelProviderSvc.GetChatModel(tenantIDs[0], chatID)
				if err != nil {
					logger.Warn("Failed to get chat model from search_config chat_id, using tenant default", zap.String("chatID", chatID), zap.Error(err))
				} else {
					logger.Info("Fetched chat model (from search_config) for metadata filter",
						zap.String("chatID", chatID),
						zap.String("tenantID", tenantIDs[0]))
				}
			}
			// If no chatID from search_config, or chatModel not found, use tenant default
			if chatModelForFilter == nil {
				tenantSvc := NewTenantService()
				modelName, err := tenantSvc.GetDefaultModelName(tenantIDs[0], entity.ModelTypeChat)
				if err != nil || modelName == "" {
					logger.Warn("Failed to get tenant default chat model name for meta_data_filter", zap.Error(err))
				} else {
					chatModelForFilter, err = modelProviderSvc.GetChatModel(tenantIDs[0], modelName)
					if err != nil {
						logger.Warn("Failed to get chat model for meta_data_filter", zap.Error(err))
					} else {
						logger.Info("Fetched chat model (tenant default) for metadata filter",
							zap.String("tenantID", tenantIDs[0]),
							zap.String("modelName", modelName))
					}
				}
			}
		}
	}

	// Apply meta_data_filter to get filtered doc_ids (filter by metadata before retrieval)
	docIDs := make([]string, len(req.DocIDs))
	copy(docIDs, req.DocIDs)
	if filter != nil {
		// Get flattened metadata
		metadataSvc := NewMetadataService()
		flattedMeta, err := metadataSvc.GetFlattedMetaByKBs(kbIDs)
		if err != nil {
			logger.Warn("Failed to get flatted metadata", zap.Error(err))
		} else {
			logger.Info("metadata filter conditions", zap.Any("filter", filter))
			filteredDocIDs, _ := ApplyMetaDataFilter(ctx, filter, flattedMeta, req.Question, chatModelForFilter, req.DocIDs)
			docIDs = filteredDocIDs
			logger.Info("ApplyMetaDataFilter result", zap.Strings("docIDs", docIDs))
		}
	}

	// Apply cross_languages and keyword extraction with tenant default chat model
	modifiedQuestion := req.Question
	var chatModel *models.ChatModel

	// Get chat model for cross_languages and keyword_extraction
	if len(req.CrossLanguages) > 0 || (req.Keyword != nil && *req.Keyword) {
		tenantSvc := NewTenantService()
		modelProviderSvc := NewModelProviderService()

		modelName, err := tenantSvc.GetDefaultModelName(tenantIDs[0], "chat")
		if err != nil || modelName == "" {
			logger.Warn("Failed to get default chat model name for LLM transformations", zap.Error(err))
		} else {
			chatModel, err = modelProviderSvc.GetChatModel(tenantIDs[0], modelName)
			if err != nil {
				logger.Warn("Failed to get chat model for LLM transformations", zap.Error(err))
			} else {
				logger.Info("Fetched chat model (tenant default) for cross_languages/keyword_extraction",
					zap.String("tenantID", tenantIDs[0]),
					zap.String("modelName", modelName))
			}
		}
	}

	// Apply cross_languages on the question (translate question)
	if chatModel != nil && len(req.CrossLanguages) > 0 {
		translated, err := CrossLanguages(ctx, chatModel, req.Question, req.CrossLanguages)
		if err != nil {
			logger.Warn("Failed to translate question", zap.Error(err))
		} else {
			modifiedQuestion = translated
		}
	}

	// Apply keyword extraction on the question (append keywords to question)
	if chatModel != nil && req.Keyword != nil && *req.Keyword {
		extractedKeywords, err := KeywordExtraction(ctx, chatModel, modifiedQuestion, 3)
		if err != nil {
			logger.Warn("Failed to extract keywords from question", zap.Error(err))
		} else if extractedKeywords != "" {
			modifiedQuestion = modifiedQuestion + " " + extractedKeywords
		}
	}

	if modifiedQuestion != req.Question {
		logger.Info("Modified question after transformations",
			zap.String("originalQuestion", req.Question),
			zap.String("modifiedQuestion", modifiedQuestion),
			zap.Strings("crossLanguages", req.CrossLanguages),
			zap.Bool("keywordExtraction", req.Keyword != nil && *req.Keyword))
	}

	// Get tag-based rank features via LabelQuestion
	metadataSvc := NewMetadataService()
	labels := metadataSvc.LabelQuestion(modifiedQuestion, kbRecords)
	logger.Debug("LabelQuestion result", zap.Any("labels", labels))

	// Determine embedding model
	var embdID string
	var tenantLLM *entity.TenantLLM
	if kbRecords[0].TenantEmbdID != nil && *kbRecords[0].TenantEmbdID > 0 {
		tenantLLM, embdID, err = dao.LookupTenantLLMByID(dao.NewTenantLLMDAO(), *kbRecords[0].TenantEmbdID)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding model by tenant_embd_id: %w", err)
		}
	} else if kbRecords[0].EmbdID != "" {
		parts := strings.Split(kbRecords[0].EmbdID, "@")
		if len(parts) == 2 && parts[1] != "" {
			tenantLLM, embdID, err = dao.LookupTenantLLMByFactory(dao.NewTenantLLMDAO(), tenantIDs[0], parts[1], parts[0], entity.ModelTypeEmbedding)
		} else {
			tenantLLM, embdID, err = dao.LookupTenantLLMByName(dao.NewTenantLLMDAO(), tenantIDs[0], kbRecords[0].EmbdID, entity.ModelTypeEmbedding)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding model by embd_id: %w", err)
		}
	} else {
		tenantLLM, err = dao.NewTenantLLMDAO().GetByTenantAndType(tenantIDs[0], entity.ModelTypeEmbedding)
		if err != nil {
			return nil, fmt.Errorf("failed to get tenant default embedding model: %w", err)
		}
		if tenantLLM == nil || tenantLLM.LLMName == nil || *tenantLLM.LLMName == "" {
			return nil, fmt.Errorf("no default embedding model found for tenant %s", tenantIDs[0])
		}
		embdID = fmt.Sprintf("%s@%s", *tenantLLM.LLMName, tenantLLM.LLMFactory)
	}

	// Get embedding model for the tenant
	modelProviderSvc := NewModelProviderService()
	embeddingModel, err := modelProviderSvc.GetEmbeddingModel(tenantIDs[0], embdID)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding model: %w", err)
	}
	logger.Info("Fetched embedding model for retrieval",
		zap.String("tenantID", tenantIDs[0]),
		zap.String("embdID", embdID))

	// Get rerank model if RerankID is specified
	var rerankModel *models.RerankModel
	var rerankCompositeName string
	if req.TenantRerankID != nil && *req.TenantRerankID != "" {
		tenantRerankIDInt, parseErr := strconv.ParseInt(*req.TenantRerankID, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid tenant_rerank_id: %w", parseErr)
		}
		_, rerankCompositeName, err = dao.LookupTenantLLMByID(dao.NewTenantLLMDAO(), tenantRerankIDInt)
		if err != nil {
			return nil, fmt.Errorf("failed to get rerank model by tenant_rerank_id: %w", err)
		}
	} else if req.RerankID != nil && *req.RerankID != "" {
		_, rerankCompositeName, err = dao.LookupTenantLLMByName(dao.NewTenantLLMDAO(), tenantIDs[0], *req.RerankID, entity.ModelTypeRerank)
		if err != nil {
			return nil, fmt.Errorf("failed to get rerank model by rerank_id: %w", err)
		}
	}
	if rerankCompositeName != "" {
		rerankModel, err = modelProviderSvc.GetRerankModel(tenantIDs[0], rerankCompositeName)
		if err != nil {
			return nil, fmt.Errorf("failed to get rerank model: %w", err)
		}
	}

	if rerankModel != nil {
		logger.Info("Fetched rerank model",
			zap.String("tenantID", tenantIDs[0]),
			zap.String("rerankCompositeName", rerankCompositeName))
	}

	retrievalReq := &nlp.RetrievalRequest{
		TenantIDs:              tenantIDs,
		Question:               modifiedQuestion,
		KbIDs:                  kbIDs,
		DocIDs:                 docIDs,
		Page:                   getPageNum(req.Page, 1),
		PageSize:               getPageSize(req.Size, 30),
		Top:                    req.TopK,
		SimilarityThreshold:    req.SimilarityThreshold,
		VectorSimilarityWeight: req.VectorSimilarityWeight,
		RerankModel:            rerankModel,
		RankFeature:            &labels,
		EmbeddingModel:         embeddingModel,
	}

	// Call RetrievalService to perform retrieval
	retrievalResult, err := nlp.NewRetrievalService(s.docEngine, s.documentDAO).Retrieval(ctx, retrievalReq)
	if err != nil {
		return nil, fmt.Errorf("retrieval search failed: %w", err)
	}

	filteredChunks := retrievalResult.Chunks

	// Handle knowledge graph retrieval
	// TODO: KG retrieval requires GraphRAG infrastructure which is not yet implemented in Go
	if req.UseKG != nil && *req.UseKG {
		logger.Warn("use_kg is not yet implemented in Go - skipping KG retrieval")
	}

	// Apply retrieval_by_children - aggregate child chunks into parent chunks
	filteredChunks = nlp.RetrievalByChildren(filteredChunks, tenantIDs, s.docEngine, ctx)

	// Remove vector field from each chunk
	for i := range filteredChunks {
		delete(filteredChunks[i], "vector")
	}

	logger.Info("RetrievalTest completed", zap.String("userID", userID), zap.Any("kbID", req.KbID), zap.String("question", req.Question), zap.Int64("chunkCount", int64(len(filteredChunks))))

	return &RetrievalTestResponse{
		Chunks:  filteredChunks,
		DocAggs: retrievalResult.DocAggs,
		Labels:  &labels,
		Total:   int64(len(filteredChunks)),
	}, nil
}

// Helper functions

// ptrString converts a pointer to a formatted string
func ptrString[T any](p *T) string {
	if p == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%v", *p)
}

func getPageNum(page *int, defaultVal int) int {
	if page != nil && *page > 0 {
		return *page
	}
	return defaultVal
}

func getPageSize(size *int, defaultVal int) int {
	if size != nil && *size > 0 {
		return *size
	}
	return defaultVal
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
	DocID        string `json:"doc_id" binding:"required"`
	Page         *int   `json:"page,omitempty"`
	Size         *int   `json:"size,omitempty"`
	Keywords     string `json:"keywords,omitempty"`
	AvailableInt *int   `json:"available_int,omitempty"`
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

	page := getPageNum(req.Page, 1)
	size := getPageSize(req.Size, 30)
	keywords := req.Keywords

	// Build search request - same as retrieval test but filtered by doc_id
	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		MatchExprs: []interface{}{keywords},
		KbIDs:      kbIDs,
		Offset:     (page - 1) * size,
		Limit:      size,
		Filter: map[string]interface{}{
			"doc_id": req.DocID,
		},
	}

	// Add available_int filter if specified
	if req.AvailableInt != nil {
		searchReq.Filter["available_int"] = *req.AvailableInt
	}

	// Execute search through unified engine interface
	searchResp, err := s.docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

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

	// Build document info
	timeFormat := "2006-01-02T15:04:05"
	docInfo := map[string]interface{}{
		"id":               doc.ID,
		"thumbnail":        doc.Thumbnail,
		"kb_id":            doc.KbID,
		"parser_id":        doc.ParserID,
		"pipeline_id":      doc.PipelineID,
		"parser_config":    doc.ParserConfig,
		"source_type":      doc.SourceType,
		"type":             doc.Type,
		"created_by":       doc.CreatedBy,
		"name":             doc.Name,
		"location":         doc.Location,
		"size":             doc.Size,
		"token_num":        doc.TokenNum,
		"chunk_num":        doc.ChunkNum,
		"progress":         utility.JSONFloat64(doc.Progress),
		"progress_msg":     doc.ProgressMsg,
		"process_begin_at": utility.FormatTimeToString(doc.ProcessBeginAt, timeFormat),
		"process_duration": doc.ProcessDuration,
		"content_hash":     doc.ContentHash,
		"suffix":           doc.Suffix,
		"run":              doc.Run,
		"status":           doc.Status,
		"create_time":      doc.CreateTime,
		"create_date":      utility.FormatTimeToString(doc.CreateDate, timeFormat),
		"update_time":      doc.UpdateTime,
		"update_date":      utility.FormatTimeToString(doc.UpdateDate, timeFormat),
	}

	return &ListChunksResponse{
		Total:  searchResp.Total,
		Chunks: chunks,
		Doc:    docInfo,
	}, nil
}

// UpdateChunkRequest request for updating a chunk
type UpdateChunkRequest struct {
	DatasetID    string        `json:"dataset_id"`
	DocumentID   string        `json:"document_id"`
	ChunkID      string        `json:"chunk_id"`
	Content      *string       `json:"content,omitempty"`
	ImportantKwd []string      `json:"important_keywords,omitempty"`
	Questions    []string      `json:"questions,omitempty"`
	Available    *bool         `json:"available,omitempty"`
	Positions    []interface{} `json:"positions,omitempty"`
	TagKwd       []string      `json:"tag_kwd,omitempty"`
	TagFeas      interface{}   `json:"tag_feas,omitempty"`
}

// UpdateChunk updates a chunk fields
func (s *ChunkService) UpdateChunk(req *UpdateChunkRequest, userID string) error {
	if s.docEngine == nil {
		return fmt.Errorf("doc engine not initialized")
	}

	if req.ChunkID == "" {
		return fmt.Errorf("chunk_id is required")
	}

	ctx := context.Background()

	// Get user's tenants
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return fmt.Errorf("user has no accessible tenants")
	}

	// Find the tenant that owns this dataset
	var targetTenantID string
	for _, tenant := range tenants {
		kb, err := s.kbDAO.GetByIDAndTenantID(req.DatasetID, tenant.TenantID)
		if err == nil && kb != nil {
			targetTenantID = tenant.TenantID
			break
		}
	}
	if targetTenantID == "" {
		return fmt.Errorf("user does not have access to this dataset")
	}

	// Verify document belongs to dataset
	docDAO := dao.NewDocumentDAO()
	doc, err := docDAO.GetByID(req.DocumentID)
	if err != nil || doc == nil {
		return fmt.Errorf("document not found")
	}
	if doc.KbID != req.DatasetID {
		return fmt.Errorf("document does not belong to this dataset")
	}

	// Fetch existing chunk first
	indexName := fmt.Sprintf("ragflow_%s", targetTenantID)
	existingChunk, err := s.docEngine.GetChunk(ctx, indexName, req.ChunkID, []string{req.DatasetID})
	if err != nil {
		return fmt.Errorf("failed to get existing chunk: %w", err)
	}

	existing, ok := existingChunk.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid chunk format")
	}

	// Build update dict
	d := make(map[string]interface{})

	// Content - use new value or existing
	if req.Content != nil {
		d["content_with_weight"] = *req.Content
	} else {
		if v, ok := existing["content_with_weight"].(string); ok {
			d["content_with_weight"] = v
		} else if v, ok := existing["content"].(string); ok {
			d["content_with_weight"] = v
		} else {
			d["content_with_weight"] = ""
		}
	}

	// Tokenize content
	contentStr := d["content_with_weight"].(string)
	d["content_ltks"], _ = tokenizer.Tokenize(contentStr)
	d["content_sm_ltks"], _ = tokenizer.FineGrainedTokenize(d["content_ltks"].(string))

	// Important keywords - convert []string to []interface{} for transformChunkFields
	if req.ImportantKwd != nil {
		impKwd := make([]interface{}, len(req.ImportantKwd))
		for i, v := range req.ImportantKwd {
			impKwd[i] = v
		}
		d["important_kwd"] = impKwd
	}

	// Questions
	if req.Questions != nil {
		// Filter out empty questions and trim
		filteredQuestions := []string{}
		for _, q := range req.Questions {
			q = strings.TrimSpace(q)
			if q != "" {
				filteredQuestions = append(filteredQuestions, q)
			}
		}
		d["question_kwd"] = filteredQuestions
	}

	// Available
	if req.Available != nil {
		if *req.Available {
			d["available_int"] = 1
		} else {
			d["available_int"] = 0
		}
	}

	// Positions
	if req.Positions != nil {
		d["position_int"] = req.Positions
	}

	// Tag keywords
	if req.TagKwd != nil {
		d["tag_kwd"] = req.TagKwd
	}

	// Tag features
	if req.TagFeas != nil {
		d["tag_feas"] = req.TagFeas
	}

	// Always include id
	d["id"] = req.ChunkID

	// Call update
	condition := map[string]interface{}{
		"id": req.ChunkID,
	}

	err = s.docEngine.UpdateDataset(ctx, condition, d, indexName, req.DatasetID)
	if err != nil {
		return fmt.Errorf("failed to update chunk: %w", err)
	}

	return nil
}

// RemoveChunksRequest request for removing chunks
type RemoveChunksRequest struct {
	DocID     string   `json:"doc_id"`
	ChunkIDs  []string `json:"chunk_ids,omitempty"`
	DeleteAll bool     `json:"delete_all,omitempty"`
}

// RemoveChunks removes chunks from the dataset table.
// If ChunkIDs is empty and DeleteAll is true, removes all chunks for the document.
// Otherwise removes only the specified chunks.
func (s *ChunkService) RemoveChunks(req *RemoveChunksRequest, userID string) (int64, error) {
	if s.docEngine == nil {
		return 0, fmt.Errorf("doc engine not initialized")
	}

	if req.DocID == "" {
		return 0, fmt.Errorf("doc_id is required")
	}

	ctx := context.Background()

	// Get user's tenants
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return 0, fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return 0, fmt.Errorf("user has no accessible tenants")
	}

	// Verify document exists and belongs to a dataset (do this first to get doc.KbID)
	docDAO := dao.NewDocumentDAO()
	doc, err := docDAO.GetByID(req.DocID)
	if err != nil || doc == nil {
		return 0, fmt.Errorf("document not found")
	}

	// Find the tenant that owns this document
	var targetTenantID string
	for _, tenant := range tenants {
		kb, err := s.kbDAO.GetByIDAndTenantID(doc.KbID, tenant.TenantID)
		if err == nil && kb != nil {
			targetTenantID = tenant.TenantID
			break
		}
	}
	if targetTenantID == "" {
		return 0, fmt.Errorf("user does not have access to this document")
	}

	indexName := fmt.Sprintf("ragflow_%s", targetTenantID)

	// Build condition
	condition := make(map[string]interface{})
	switch {
	case len(req.ChunkIDs) > 0 && req.DeleteAll:
		return 0, fmt.Errorf("chunk_ids and delete_all are mutually exclusive")
	case len(req.ChunkIDs) > 0:
		// Delete specific chunks - convert []string to []interface{} for buildFilterFromCondition
		chunkIDsIf := make([]interface{}, len(req.ChunkIDs))
		for i, id := range req.ChunkIDs {
			chunkIDsIf[i] = id
		}
		condition["id"] = chunkIDsIf
		condition["doc_id"] = req.DocID
	case req.DeleteAll:
		// Delete all chunks for this document
		condition["doc_id"] = req.DocID
	default:
		return 0, fmt.Errorf("either chunk_ids or delete_all must be provided")
	}

	deletedCount, err := s.docEngine.Delete(ctx, condition, indexName, doc.KbID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete chunks: %w", err)
	}

	return deletedCount, nil
}
