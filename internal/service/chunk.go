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
	"ragflow/internal/server"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/logger"

	"ragflow/internal/service/nlp"
	"ragflow/internal/tokenizer"
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
	searchService  *SearchService
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
	MetaDataFilter         map[string]interface{} `json:"meta_data_filter,omitempty"`
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

// lookupTenantLLMByID looks up a TenantLLM record by ID and returns the record plus composite model name.
// Corresponds to Python's get_model_config_by_id().
func lookupTenantLLMByID(tenantLLMDao *dao.TenantLLMDAO, id int64) (*entity.TenantLLM, string, error) {
	tenantLLM, err := tenantLLMDao.GetByID(id)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get tenant_llm by id %d: %w", id, err)
	}
	if tenantLLM == nil || tenantLLM.LLMName == nil || *tenantLLM.LLMName == "" {
		return nil, "", fmt.Errorf("tenant_llm record not found for id %d", id)
	}
	compositeName := fmt.Sprintf("%s@%s", *tenantLLM.LLMName, tenantLLM.LLMFactory)
	return tenantLLM, compositeName, nil
}

// lookupTenantLLMByName looks up a TenantLLM record by tenant name and model type.
// Corresponds to Python's get_model_config_by_type_and_name() when no factory is provided.
func lookupTenantLLMByName(tenantLLMDao *dao.TenantLLMDAO, tenantID, name string, modelType entity.ModelType) (*entity.TenantLLM, string, error) {
	tenantLLM, err := tenantLLMDao.GetByTenantNameAndType(tenantID, name, modelType)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get tenant_llm by name %s: %w", name, err)
	}
	if tenantLLM == nil || tenantLLM.LLMName == nil || *tenantLLM.LLMName == "" {
		return nil, "", fmt.Errorf("tenant_llm record not found for name %s", name)
	}
	compositeName := fmt.Sprintf("%s@%s", *tenantLLM.LLMName, tenantLLM.LLMFactory)
	return tenantLLM, compositeName, nil
}

// lookupTenantLLMByFactory looks up a TenantLLM record by tenant, factory, and model name.
// Corresponds to Python's get_model_config_by_type_and_name() with factory suffix.
func lookupTenantLLMByFactory(tenantLLMDao *dao.TenantLLMDAO, tenantID, factory, name string, modelType entity.ModelType) (*entity.TenantLLM, string, error) {
	tenantLLM, err := tenantLLMDao.GetByTenantFactoryAndModelName(tenantID, factory, name)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get tenant_llm by factory %s and name %s: %w", factory, name, err)
	}
	if tenantLLM == nil || tenantLLM.LLMName == nil || *tenantLLM.LLMName == "" {
		return nil, "", fmt.Errorf("tenant_llm record not found for factory %s and name %s", factory, name)
	}
	compositeName := fmt.Sprintf("%s@%s", *tenantLLM.LLMName, tenantLLM.LLMFactory)
	return tenantLLM, compositeName, nil
}

// labelQuestion returns rank features for a question based on KB's tag configuration.
// Corresponds to rag/app/tag.py:label_question()
// labelQuestion matches and returns the most relevant tags from a specified tag library based on the user question.
// labelQuestion extracts relevant tags from a question using tag-based knowledgebases.
// This is used in RAG retrieval to boost or filter results based on tag relevance.
//
// The function works in 4 steps:
//  1. Extract tag_kb_ids (tag library IDs) from KBs' parser_config
//     - These KBs contain tag definitions (not actual documents)
//  2. Get all tags from the tag library
//     - First tries cache, computes and caches if miss
//  3. Query the tag library with the question to find matching tags
//     - Uses vector similarity to score each tag against the question
//  4. Return topN tags with highest relevance scores
//
// Parameters:
//   - question: User query text
//   - kbs: Knowledgebase list, from which tag_kb_ids are extracted as tag library
//
// Returns:
//   - map[string]float64: Mapping of tag name to weight score, returns at most topn_tags tags
//   - Returns nil if no tag_kb_ids configured or no matching tags found
func labelQuestion(question string, kbs []*entity.Knowledgebase) map[string]float64 {
	if len(kbs) == 0 {
		return nil
	}

	// Collect tag_kb_ids from KBs' parser_config and track last KB (matching Python L130-132)
	var tagKBBIDs []string
	var lastKB *entity.Knowledgebase
	for _, kb := range kbs {
		if kb.ParserConfig == nil {
			continue
		}
		lastKB = kb // Python uses kb from last iteration for tenant_id
		if tagKBIDs, ok := kb.ParserConfig["tag_kb_ids"].([]interface{}); ok {
			for _, id := range tagKBIDs {
				if idStr, ok := id.(string); ok {
					tagKBBIDs = append(tagKBBIDs, idStr)
				}
			}
		}
	}

	if len(tagKBBIDs) == 0 {
		return nil
	}

	logger.Debug("tag_kb_ids found in parser_config", zap.Strings("tagKBBIDs", tagKBBIDs))

	// Get all tags from cache or compute and cache (matching Python L134-139)
	allTags, err := GetTagsFromCache(tagKBBIDs)
	if err != nil {
		logger.Warn("Failed to get tags from cache", zap.Error(err))
	}
	if allTags == nil {
		// Cache miss - compute all_tags_in_portion
		metadataSvc := NewMetadataService()
		allTags, err = metadataSvc.GetAllTagsInPortion(lastKB.TenantID, tagKBBIDs)
		if err != nil {
			logger.Warn("Failed to get all tags in portion", zap.Error(err))
			return nil
		}
		// Store in cache for future lookups
		if err := SetTagsToCache(tagKBBIDs, allTags); err != nil {
			logger.Warn("Failed to set tags cache", zap.Error(err))
		}
	}

	// Get tag_kbs by IDs (matching Python L140)
	kbDAO := dao.NewKnowledgebaseDAO()
	tagKBs, err := kbDAO.GetByIDs(tagKBBIDs)
	if err != nil || len(tagKBs) == 0 {
		// Return nil if no tag_kbs found (matching Python L141-142)
		return nil
	}

	// Get unique tenant IDs from tag_kbs (matching Python L144: list(set([kb.tenant_id for kb in tag_kbs])))
	tenantIDSet := make(map[string]bool)
	for _, kb := range tagKBs {
		tenantIDSet[kb.TenantID] = true
	}
	var uniqueTenantIDs []string
	for tid := range tenantIDSet {
		uniqueTenantIDs = append(uniqueTenantIDs, tid)
	}
	if len(uniqueTenantIDs) == 0 {
		return nil
	}

	// Get topn_tags from last KB's parser_config (matching Python L147: kb.parser_config.get("topn_tags", 3))
	topnTags := 3
	if lastKB != nil && lastKB.ParserConfig != nil {
		if topn, ok := lastKB.ParserConfig["topn_tags"].(int); ok {
			topnTags = topn
		}
	}

	// Query tags for the question using unique tenant IDs (matching Python L143-148)
	metadataSvc := NewMetadataService()
	tagFeatures, err := metadataSvc.TagQuery(question, uniqueTenantIDs, tagKBBIDs, allTags, topnTags)
	if err != nil {
		return nil
	}
	if len(tagFeatures) == 0 {
		// Tag kb exists but returned no matching tags - return empty map (not nil)
		// so caller knows tag kb was configured vs not configured at all
		return make(map[string]float64)
	}

	return tagFeatures
}

// RetrievalTest performs retrieval test for a given question against specified knowledge bases.
// Corresponds to Python's api/apps/chunk_app.py:retrieval_test()
//
// Flow:
//  1. Validate kbs permissions and embedding model
//  2. Apply metadata filter if specified (auto/semi_auto uses LLM, manual uses provided conditions)
//  3. Apply cross_languages transformation if requested (translate question)
//  4. Apply keyword extraction if requested (append keywords to question)
//  5. Get rank features via labelQuestion() - tag-based weights or pagerank_fld fallback
//  6. Call RetrievalService.Retrieval() which:
//     - Computes query embedding
//     - Performs hybrid search (text + vector) with rank features
//     - Reranks results
//     - Builds doc_aggs by aggregating chunks per document
//  7. knowledge graph retrieval (not implemented)
//  8. Apply retrieval by children to group child chunks under parent chunks
func (s *ChunkService) RetrievalTest(req *RetrievalTestRequest, userID string) (*RetrievalTestResponse, error) {
	logger.Info("RetrievalTest called", zap.String("userID", userID), zap.Any("kbID", req.KbID), zap.String("question", req.Question))
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
	var creds *entity.ModelCredentials
	metaDataFilter := req.MetaDataFilter

	if req.SearchID != nil && *req.SearchID != "" {
		// If search_id is set, get meta_data_filter and chat_id from search_config
		searchDetail, err := s.searchService.GetDetail(*req.SearchID)
		if err != nil {
			logger.Warn("Failed to get search detail for search_id, proceeding without it", zap.String("searchID", *req.SearchID), zap.Error(err))
		} else {
			searchConfig := searchDetail["search_config"].(entity.JSONMap)
			if searchConfig != nil {
				if searchMetaFilter, ok := searchConfig["meta_data_filter"].(map[string]interface{}); ok {
					metaDataFilter = searchMetaFilter
				}
				chatID, _ = searchConfig["chat_id"].(string)
			} else {
				logger.Warn("No search_config found in search detail", zap.String("searchID", *req.SearchID))
			}
		}
	}

	// If meta_data_filter method is auto/semi_auto, get chat model
	if metaDataFilter != nil {
		method, _ := metaDataFilter["method"].(string)
		if method == "auto" || method == "semi_auto" {
			modelProviderSvc := NewModelProviderService()
			if chatID != "" {
				// Use chat_id from search_config
				creds, err = modelProviderSvc.GetModelByName(chatID, tenantIDs[0])
				if err != nil {
					logger.Warn("Failed to get chat model from search_config chat_id, using tenant default", zap.String("chatID", chatID), zap.Error(err))
				} else {
					logger.Info("GetModelByName for meta_data_filter chat model (from search_config)",
						zap.String("chatID", chatID),
						zap.String("tenantIDs[0]", tenantIDs[0]),
						zap.String("providerName", creds.ProviderName),
						zap.String("modelName", creds.ModelName),
						zap.Bool("hasApiKey", creds.APIKey != ""))
				}
			}
			// If no chatID from search_config, or creds not found, use tenant default
			if creds == nil {
				creds, err = modelProviderSvc.GetDefaultModel(entity.ModelTypeChat, tenantIDs[0])
				if err != nil {
					logger.Warn("Failed to get tenant default chat model for meta_data_filter", zap.Error(err))
				} else {
					logger.Info("GetDefaultModel for meta_data_filter chat model (tenant default)",
						zap.String("tenantID", tenantIDs[0]),
						zap.String("providerName", creds.ProviderName),
						zap.String("modelName", creds.ModelName),
						zap.Bool("hasApiKey", creds.APIKey != ""))
				}
			}
		}
	}

	// Apply meta_data_filter to get filtered doc_ids (filter by metadata before retrieval)
	docIDs := make([]string, len(req.DocIDs))
	copy(docIDs, req.DocIDs)
	if metaDataFilter != nil {
		// Get flattened metadata
		metadataSvc := NewMetadataService()
		flattedMeta, err := metadataSvc.GetFlattedMetaByKBs(kbIDs)
		if err != nil {
			logger.Warn("Failed to get flatted metadata", zap.Error(err))
		} else {
			filteredDocIDs, _ := ApplyMetaDataFilter(ctx, metaDataFilter, flattedMeta, req.Question, creds, req.DocIDs)
			docIDs = filteredDocIDs
		}
	}

	// Apply cross_languages and keyword extraction with tenant default chat model
	modifiedQuestion := req.Question

	// Get chat model for cross_languages and keyword_extraction
	if len(req.CrossLanguages) > 0 || (req.Keyword != nil && *req.Keyword) {
		modelProviderSvc := NewModelProviderService()
		creds, err = modelProviderSvc.GetDefaultModel(entity.ModelTypeChat, tenantIDs[0])
		if err != nil {
			logger.Warn("Failed to get default chat model for LLM transformations", zap.Error(err))
		} else {
			logger.Info("GetDefaultModel for chat model (cross_languages/keyword_extraction)",
				zap.String("tenantID", tenantIDs[0]),
				zap.String("providerName", creds.ProviderName),
				zap.String("modelName", creds.ModelName),
				zap.Bool("hasApiKey", creds.APIKey != ""))
		}
	}

	// Apply cross_languages on the question (translate question)
	if creds != nil && len(req.CrossLanguages) > 0 {
		translated, err := CrossLanguages(ctx, creds, req.Question, req.CrossLanguages)
		if err != nil {
			logger.Warn("Failed to translate question", zap.Error(err))
		} else {
			modifiedQuestion = translated
		}
	}

	// Apply keyword extraction on the question (append keywords to question)
	if creds != nil && req.Keyword != nil && *req.Keyword {
		extractedKeywords, err := KeywordExtraction(ctx, creds, modifiedQuestion, 3)
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

	// Get tag-based rank features via labelQuestion
	labels := labelQuestion(modifiedQuestion, kbRecords)
	logger.Debug("labelQuestion result", zap.Any("labels", labels))

	// Determine embedding model
	var embdID string
	var tenantLLM *entity.TenantLLM
	if kbRecords[0].TenantEmbdID != nil && *kbRecords[0].TenantEmbdID > 0 {
		tenantLLM, embdID, err = lookupTenantLLMByID(dao.NewTenantLLMDAO(), *kbRecords[0].TenantEmbdID)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding model by tenant_embd_id: %w", err)
		}
	} else if kbRecords[0].EmbdID != "" {
		parts := strings.Split(kbRecords[0].EmbdID, "@")
		if len(parts) == 2 && parts[1] != "" {
			tenantLLM, embdID, err = lookupTenantLLMByFactory(dao.NewTenantLLMDAO(), tenantIDs[0], parts[1], parts[0], entity.ModelTypeEmbedding)
		} else {
			tenantLLM, embdID, err = lookupTenantLLMByName(dao.NewTenantLLMDAO(), tenantIDs[0], kbRecords[0].EmbdID, entity.ModelTypeEmbedding)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding model by embd_id: %w", err)
		}
	} else {
		modelProviderSvc := NewModelProviderService()
		creds, err := modelProviderSvc.GetDefaultModel(entity.ModelTypeEmbedding, tenantIDs[0])
		if err != nil {
			return nil, fmt.Errorf("failed to get tenant default embedding model: %w", err)
		}
		logger.Info("GetDefaultModel for tenant default embedding model",
			zap.String("tenantIDs[0]", tenantIDs[0]),
			zap.String("providerName", creds.ProviderName),
			zap.String("modelName", creds.ModelName),
			zap.Bool("hasApiKey", creds.APIKey != ""))

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
	var embeddingModel entity.EmbeddingModel
	embeddingModel, err = s.modelProvider.GetEmbeddingModel(ctx, tenantIDs[0], embdID)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding model: %w", err)
	}
	logger.Info("Retrieved embedding model for retrieval",
		zap.String("tenantIDs[0]", tenantIDs[0]),
		zap.String("embdID", embdID),
		zap.Any("embeddingModel", embeddingModel))

	// Get rerank model if RerankID is specified
	var rerankModel nlp.RerankModel
	var rerankCompositeName string
	if req.TenantRerankID != nil && *req.TenantRerankID != "" {
		tenantRerankIDInt, parseErr := strconv.ParseInt(*req.TenantRerankID, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid tenant_rerank_id: %w", parseErr)
		}
		_, rerankCompositeName, err = lookupTenantLLMByID(dao.NewTenantLLMDAO(), tenantRerankIDInt)
		if err != nil {
			return nil, fmt.Errorf("failed to get rerank model by tenant_rerank_id: %w", err)
		}
		rerankModel, err = s.modelProvider.GetRerankModel(ctx, tenantIDs[0], rerankCompositeName)
		if err != nil {
			return nil, fmt.Errorf("failed to get rerank model by tenant_rerank_id: %w", err)
		}
	} else if req.RerankID != nil && *req.RerankID != "" {
		var err error
		_, rerankCompositeName, err = lookupTenantLLMByName(dao.NewTenantLLMDAO(), tenantIDs[0], *req.RerankID, entity.ModelTypeRerank)
		if err != nil {
			return nil, fmt.Errorf("failed to get rerank model by rerank_id: %w", err)
		}
		rerankModel, err = s.modelProvider.GetRerankModel(ctx, tenantIDs[0], rerankCompositeName)
		if err != nil {
			return nil, fmt.Errorf("failed to get rerank model by rerank_id: %w", err)
		}
	}

	if rerankModel != nil {
		logger.Info("Retrieved rerank model",
			zap.String("tenantID", tenantIDs[0]),
			zap.String("rerankCompositeName", rerankCompositeName))
	}

	retrievalReq := &RetrievalRequest{
		TenantIDs:              tenantIDs,
		Question:               modifiedQuestion,
		KbIDs:                  kbIDs,
		DocIDs:                 req.DocIDs,
		Page:                   getPageNum(req.Page, 1),
		PageSize:               getPageSize(req.Size, 30),
		Top:                    req.TopK,
		SimilarityThreshold:    req.SimilarityThreshold,
		VectorSimilarityWeight: req.VectorSimilarityWeight,
		RerankModel:            rerankModel,
		RankFeature:            &labels,
		EmbeddingModel:         embeddingModel,
	}

	// Call RetrievalService to perform Level 2: hybrid similarity + reranking
	// Level 3 (Search) and Level 4 (searchUnified) are called internally
	retrievalResult, err := NewRetrievalService(s.docEngine).Retrieval(ctx, retrievalReq)
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
	filteredChunks = retrievalByChildren(filteredChunks, tenantIDs, s.docEngine, ctx)

	// Remove vector field from each chunk
	for i := range filteredChunks {
		delete(filteredChunks[i], "vector")
	}

	return &RetrievalTestResponse{
		Chunks:  filteredChunks,
		DocAggs: retrievalResult.DocAggs,
		Labels:  &labels,
		Total:   int64(len(filteredChunks)),
	}, nil
}

// Helper functions

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

func getTopK(topk *int, defaultVal int) int {
	if topk != nil && *topk > 0 {
		return *topk
	}
	return defaultVal
}

func getSimilarityThreshold(threshold *float64) float64 {
	if threshold != nil && *threshold >= 0 {
		return *threshold
	}
	return 0.0
}

func getVectorSimilarityWeight(weight *float64) float64 {
	if weight != nil && *weight >= 0 && *weight <= 1 {
		return *weight
	}
	return 0.3
}

const (
	PAGERANK_FLD = "pagerank_fea"
)

// retrievalByChildren aggregates child chunks into parent chunks
// Reference: rag/nlp/search.py:retrieval_by_children()
func retrievalByChildren(chunks []map[string]interface{}, tenantIDs []string, docEngine engine.DocEngine, ctx context.Context) []map[string]interface{} {
	indexNames := buildIndexNames(tenantIDs)
	if len(chunks) == 0 {
		return chunks
	}

	// Group child chunks by mom_id
	type childChunk struct {
		chunk map[string]interface{}
		kbID  string
	}
	momChunks := make(map[string][]childChunk)
	remainingChunks := make([]map[string]interface{}, 0, len(chunks))

	for _, ck := range chunks {
		momID, ok := ck["mom_id"].(string)
		if !ok || momID == "" {
			remainingChunks = append(remainingChunks, ck)
			continue
		}
		kbID, _ := ck["kb_id"].(string)
		momChunks[momID] = append(momChunks[momID], childChunk{chunk: ck, kbID: kbID})
	}

	if len(momChunks) == 0 {
		return chunks
	}

	// Fetch parent chunks and aggregate
	vectorSize := 1024
	for momID, childList := range momChunks {
		kbIDs := make([]string, 0, len(childList))
		for _, c := range childList {
			if c.kbID != "" {
				kbIDs = append(kbIDs, c.kbID)
			}
		}
		if len(kbIDs) == 0 {
			kbIDs = append(kbIDs, "")
		}

		parent, err := docEngine.GetChunk(ctx, indexNames[0], momID, kbIDs)
		if err != nil {
			logger.Warn("Failed to get parent chunk", zap.String("momID", momID), zap.Error(err))
			continue
		}
		parentMap, ok := parent.(map[string]interface{})
		if !ok {
			continue
		}

		// Calculate average similarity
		var totalSim float64
		for _, c := range childList {
			if sim, ok := c.chunk["similarity"].(float64); ok {
				totalSim += sim
			}
		}
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
		aggregated := map[string]interface{}{
			"chunk_id":            momID,
			"content_ltks":        contentLTKS,
			"content_with_weight": parentMap["content_with_weight"],
			"doc_id":              parentMap["doc_id"],
			"docnm_kwd":           parentMap["docnm_kwd"],
			"kb_id":               parentMap["kb_id"],
			"important_kwd":       allImportantKwd,
			"image_id":            parentMap["img_id"],
			"similarity":          avgSim,
			"vector_similarity":   avgSim,
			"term_similarity":     avgSim,
			"vector":              make([]float64, vectorSize),
			"positions":           parentMap["position_int"],
			"doc_type_kwd":        parentMap["doc_type_kwd"],
		}

		// Get vector from first child if available
		for _, c := range childList {
			for k := range c.chunk {
				if strings.HasSuffix(k, "_vec") {
					if vec, ok := c.chunk[k].([]float64); ok {
						aggregated["vector"] = vec
						vectorSize = len(vec)
						break
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

	return remainingChunks
}

func buildIndexNames(tenantIDs []string) []string {
	indexNames := make([]string, len(tenantIDs))
	for i, tenantID := range tenantIDs {
		indexNames[i] = fmt.Sprintf("ragflow_%s", tenantID)
	}
	return indexNames
}

// buildSearchResult converts engine.SearchResult to nlp.SearchResult for reranking
func buildSearchResult(resp *engine.SearchResult, queryVector []float64) *nlp.SearchResult {
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
	var docTenantID string
	for _, tenant := range tenants {
		if tenant.TenantID == kb.TenantID {
			docTenantID = tenant.TenantID
			break
		}
	}
	if docTenantID == "" {
		return nil, fmt.Errorf("user does not have access to this document")
	}

	// Get kbIDs for this tenant
	kbIDs, err := s.kbDAO.GetKBIDsByTenantID(docTenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get kb ids: %w", err)
	}

	indexName := fmt.Sprintf("ragflow_%s", docTenantID)

	page := getPageNum(req.Page, 1)
	size := getPageSize(req.Size, 30)
	keywords := req.Keywords

	// Build search request - same as retrieval test but filtered by doc_id
	searchReq := &engine.SearchRequest{
		IndexNames:     []string{indexName},
		MatchExprs:     []interface{}{keywords},
		KbIDs:          kbIDs,
		DocIDs:         []string{req.DocID},
		Offset:         (page - 1) * size,
		Limit:          size,
		TopK:           size,
		MetaDataFilter: map[string]interface{}{},
	}

	// Add available_int filter if specified
	if req.AvailableInt != nil {
		searchReq.MetaDataFilter["available_int"] = *req.AvailableInt
	}

	// Execute search through unified engine interface
	searchResp, err := s.docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
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
	var dsTenantID string
	for _, tenant := range tenants {
		kb, err := s.kbDAO.GetByIDAndTenantID(req.DatasetID, tenant.TenantID)
		if err == nil && kb != nil {
			dsTenantID = tenant.TenantID
			break
		}
	}
	if dsTenantID == "" {
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

	// Fetch existing chunk first (like Python does)
	indexName := fmt.Sprintf("ragflow_%s", dsTenantID)
	existingChunk, err := s.docEngine.GetChunk(ctx, indexName, req.ChunkID, []string{req.DatasetID})
	if err != nil {
		return fmt.Errorf("failed to get existing chunk: %w", err)
	}

	existing, ok := existingChunk.(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid chunk format")
	}

	// Build update dict like Python does (doc.py:1476-1523)
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
	var docTenantID string
	for _, tenant := range tenants {
		kb, err := s.kbDAO.GetByIDAndTenantID(doc.KbID, tenant.TenantID)
		if err == nil && kb != nil {
			docTenantID = tenant.TenantID
			break
		}
	}
	if docTenantID == "" {
		return 0, fmt.Errorf("user does not have access to this document")
	}

	indexName := fmt.Sprintf("ragflow_%s", docTenantID)

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
