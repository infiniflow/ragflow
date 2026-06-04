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
	"strconv"
	"strings"
	"time"

	"github.com/cespare/xxhash/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/server"
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
	Datasets               common.StringSlice      `json:"dataset_ids" binding:"required"` // string or []string
	Question               string                 `json:"question"`
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
	common.Info("RetrievalTest started", zap.String("userID", userID), zap.Any("kbID", req.Datasets), zap.String("question", req.Question))

	common.Debug(fmt.Sprintf("RetrievalTest request:\n"+
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
		req.Datasets, req.Question,
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

	tenantIDs, kbRecords, err := s.validateKBs(userID, req.Datasets)
	if err != nil {
		return nil, err
	}

	docIDs, err := s.resolveMetaFilter(ctx, req.SearchID, req.Filter, req.Question, req.DocIDs, req.Datasets, tenantIDs)
	if err != nil {
		return nil, err
	}

	modifiedQuestion, err := s.transformQuestion(ctx, req.Question, req.CrossLanguages, req.Keyword, tenantIDs)
	if err != nil {
		return nil, err
	}

	// Get tag-based rank features via LabelQuestion
	metadataSvc := NewMetadataService()
	labels := metadataSvc.LabelQuestion(modifiedQuestion, kbRecords)
	common.Debug("LabelQuestion result", zap.Any("labels", labels))

	embeddingModel, err := s.resolveEmbeddingModel(tenantIDs[0], kbRecords[0])
	if err != nil {
		return nil, err
	}

	rerankModel, err := s.resolveRerankModel(tenantIDs[0], req.TenantRerankID, req.RerankID)
	if err != nil {
		return nil, err
	}

	retrievalReq := &nlp.RetrievalRequest{
		TenantIDs:              tenantIDs,
		Question:               modifiedQuestion,
		KbIDs:                  []string(req.Datasets),
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
		common.Warn("use_kg is not yet implemented in Go - skipping KG retrieval")
	}

	// Apply retrieval_by_children - aggregate child chunks into parent chunks
	filteredChunks = nlp.RetrievalByChildren(filteredChunks, tenantIDs, s.docEngine, ctx)

	// Remove vector field from each chunk
	for i := range filteredChunks {
		delete(filteredChunks[i], "vector")
	}

	common.Info("RetrievalTest completed", zap.String("userID", userID), zap.Any("kbID", req.Datasets), zap.String("question", req.Question), zap.Int64("chunkCount", int64(len(filteredChunks))))

	return &RetrievalTestResponse{
		Chunks:  filteredChunks,
		DocAggs: retrievalResult.DocAggs,
		Labels:  &labels,
		Total:   int64(len(filteredChunks)),
	}, nil
}


// validateKBs resolves tenant IDs and KB records for the given dataset IDs.
func (s *ChunkService) validateKBs(userID string, datasetIDs []string) ([]string, []*entity.Knowledgebase, error) {
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil, nil, fmt.Errorf("user has no accessible tenants")
	}
	common.Debug("Retrieved user tenants from database", zap.String("userID", userID), zap.Int("tenantCount", len(tenants)))

	var tenantIDs []string
	var kbRecords []*entity.Knowledgebase
	for _, datasetID := range datasetIDs {
		found := false
		for _, tenant := range tenants {
			kb, err := s.kbDAO.GetByIDAndTenantID(datasetID, tenant.TenantID)
			if err == nil && kb != nil {
				common.Debug("Found knowledge base in database",
					zap.String("datasetID", datasetID),
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
			return nil, nil, fmt.Errorf("only owner of dataset is authorized for this operation")
		}
	}
	if len(kbRecords) > 1 {
		firstEmbdID := kbRecords[0].EmbdID
		for i := 1; i < len(kbRecords); i++ {
			if kbRecords[i].EmbdID != firstEmbdID {
				return nil, nil, fmt.Errorf("cannot retrieve across datasets with different embedding models")
			}
		}
	}
	return tenantIDs, kbRecords, nil
}

// resolveMetaFilter resolves a metadata filter from search_id and applies it.
func (s *ChunkService) resolveMetaFilter(ctx context.Context, searchID *string, initialFilter map[string]interface{}, question string, docIDs []string, datasetIDs []string, tenantIDs []string) ([]string, error) {
	var chatID string
	var chatModelForFilter *models.ChatModel
	filter := initialFilter

	if searchID != nil && *searchID != "" {
		searchDetail, err := s.searchService.GetDetail(*searchID)
		if err != nil {
			common.Warn("Failed to get search detail for search_id, proceeding without it", zap.String("searchID", *searchID), zap.Error(err))
		} else if searchConfig, ok := searchDetail["search_config"].(entity.JSONMap); ok && searchConfig != nil {
			if searchMetaFilter, ok := searchConfig["meta_data_filter"].(map[string]interface{}); ok {
				filter = searchMetaFilter
			}
			chatID, _ = searchConfig["chat_id"].(string)
		} else {
			common.Warn("No search_config found in search detail", zap.String("searchID", *searchID))
		}
	}
	if filter != nil {
		method, _ := filter["method"].(string)
		if method == "auto" || method == "semi_auto" {
			modelProviderSvc := NewModelProviderService()
			if chatID != "" {
				driver, mdlName, apiConfig, _, getErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeChat, chatID)
				if getErr != nil {
					common.Warn("Failed to get chat model from search_config chat_id, using tenant default", zap.String("chatID", chatID), zap.Error(getErr))
				} else {
					chatModelForFilter = models.NewChatModel(driver, &mdlName, apiConfig)
					common.Info("Fetched chat model (from search_config) for metadata filter",
						zap.String("chatID", chatID), zap.String("tenantID", tenantIDs[0]))
				}
			}
			if chatModelForFilter == nil {
				tenantSvc := NewTenantService()
				modelName, err := tenantSvc.GetDefaultModelName(tenantIDs[0], entity.ModelTypeChat)
				if err != nil || modelName == "" {
					common.Warn("Failed to get tenant default chat model name for meta_data_filter", zap.Error(err))
				} else {
					driver, mdlName, apiConfig, _, getErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeChat, modelName)
					if getErr != nil {
						common.Warn("Failed to get chat model for meta_data_filter", zap.Error(getErr))
					} else {
						chatModelForFilter = models.NewChatModel(driver, &mdlName, apiConfig)
						common.Info("Fetched chat model (tenant default) for metadata filter",
							zap.String("tenantID", tenantIDs[0]), zap.String("modelName", modelName))
					}
				}
			}
		}
	}
	out := make([]string, len(docIDs))
	copy(out, docIDs)
	if filter != nil {
		metadataSvc := NewMetadataService()
		flattedMeta, err := metadataSvc.GetFlattedMetaByKBs([]string(datasetIDs))
		if err != nil {
			common.Warn("Failed to get flatted metadata", zap.Error(err))
		} else {
			common.Info("metadata filter conditions", zap.Any("filter", filter))
			filteredDocIDs, _ := ApplyMetaDataFilter(ctx, filter, flattedMeta, question, chatModelForFilter, docIDs, []string(datasetIDs))
			out = filteredDocIDs
			common.Info("ApplyMetaDataFilter result", zap.Strings("docIDs", out))
		}
	}
	return out, nil
}

// transformQuestion applies cross-languages translation and keyword extraction.
func (s *ChunkService) transformQuestion(ctx context.Context, question string, crossLanguages []string, keyword *bool, tenantIDs []string) (string, error) {
	modifiedQuestion := question
	if len(crossLanguages) == 0 && (keyword == nil || !*keyword) {
		return modifiedQuestion, nil
	}
	tenantSvc := NewTenantService()
	modelProviderSvc := NewModelProviderService()
	modelName, err := tenantSvc.GetDefaultModelName(tenantIDs[0], "chat")
	if err != nil || modelName == "" {
		common.Warn("Failed to get default chat model name for LLM transformations", zap.Error(err))
		return question, nil
	}
	driver, mdlName, apiConfig, _, getErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeChat, modelName)
	if getErr != nil {
		common.Warn("Failed to get chat model for LLM transformations", zap.Error(getErr))
		return question, nil
	}
	chatModel := models.NewChatModel(driver, &mdlName, apiConfig)
	common.Info("Fetched chat model (tenant default) for cross_languages/keyword_extraction",
		zap.String("tenantID", tenantIDs[0]), zap.String("modelName", modelName))
	if len(crossLanguages) > 0 {
		translated, err := CrossLanguages(ctx, tenantIDs[0], modelName, question, crossLanguages)
		if err != nil {
			common.Warn("Failed to translate question", zap.Error(err))
		} else {
			modifiedQuestion = translated
		}
	}
	if keyword != nil && *keyword {
		extractedKeywords, err := KeywordExtraction(ctx, chatModel, modifiedQuestion, 3)
		if err != nil {
			common.Warn("Failed to extract keywords from question", zap.Error(err))
		} else if extractedKeywords != "" {
			modifiedQuestion = modifiedQuestion + " " + extractedKeywords
		}
	}
	if modifiedQuestion != question {
		common.Info("Modified question after transformations",
			zap.String("originalQuestion", question),
			zap.String("modifiedQuestion", modifiedQuestion),
			zap.Strings("crossLanguages", crossLanguages),
			zap.Bool("keywordExtraction", keyword != nil && *keyword))
	}
	return modifiedQuestion, nil
}

// resolveEmbeddingModel resolves the embedding model for a KB record.
func (s *ChunkService) resolveEmbeddingModel(tenantID string, kbRecord *entity.Knowledgebase) (*models.EmbeddingModel, error) {
	var embdID string
	var err error
	if kbRecord.TenantEmbdID != nil && *kbRecord.TenantEmbdID > 0 {
		_, embdID, err = dao.LookupTenantLLMByID(dao.NewTenantLLMDAO(), *kbRecord.TenantEmbdID)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding model by tenant_embd_id: %w", err)
		}
	} else if kbRecord.EmbdID != "" {
		parts := strings.Split(kbRecord.EmbdID, "@")
		if len(parts) == 2 && parts[1] != "" {
			_, embdID, err = dao.LookupTenantLLMByFactory(dao.NewTenantLLMDAO(), tenantID, parts[1], parts[0], entity.ModelTypeEmbedding)
		} else {
			_, embdID, err = dao.LookupTenantLLMByName(dao.NewTenantLLMDAO(), tenantID, kbRecord.EmbdID, entity.ModelTypeEmbedding)
		}
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding model by embd_id: %w", err)
		}
	} else {
		tenantLLM, err := dao.NewTenantLLMDAO().GetByTenantAndType(tenantID, entity.ModelTypeEmbedding)
		if err != nil {
			return nil, fmt.Errorf("failed to get tenant default embedding model: %w", err)
		}
		if tenantLLM == nil || tenantLLM.LLMName == nil || *tenantLLM.LLMName == "" {
			return nil, fmt.Errorf("no default embedding model found for tenant %s", tenantID)
		}
		embdID = fmt.Sprintf("%s@%s", *tenantLLM.LLMName, tenantLLM.LLMFactory)
	}
	modelProviderSvc := NewModelProviderService()
	embeddingModel, err := modelProviderSvc.GetEmbeddingModel(tenantID, embdID)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding model: %w", err)
	}
	common.Info("Fetched embedding model for retrieval",
		zap.String("tenantID", tenantID), zap.String("embdID", embdID))
	return embeddingModel, nil
}

// resolveRerankModel resolves the rerank model from tenant_rerank_id or rerank_id.
func (s *ChunkService) resolveRerankModel(tenantID string, tenantRerankID, rerankID *string) (*models.RerankModel, error) {
	var rerankCompositeName string
	var err error
	if tenantRerankID != nil && *tenantRerankID != "" {
		tenantRerankIDInt, parseErr := strconv.ParseInt(*tenantRerankID, 10, 64)
		if parseErr != nil {
			return nil, fmt.Errorf("invalid tenant_rerank_id: %w", parseErr)
		}
		_, rerankCompositeName, err = dao.LookupTenantLLMByID(dao.NewTenantLLMDAO(), tenantRerankIDInt)
		if err != nil {
			return nil, fmt.Errorf("failed to get rerank model by tenant_rerank_id: %w", err)
		}
	} else if rerankID != nil && *rerankID != "" {
		_, rerankCompositeName, err = dao.LookupTenantLLMByName(dao.NewTenantLLMDAO(), tenantID, *rerankID, entity.ModelTypeRerank)
		if err != nil {
			return nil, fmt.Errorf("failed to get rerank model by rerank_id: %w", err)
		}
	}
	if rerankCompositeName == "" {
		return nil, nil
	}
	modelProviderSvc := NewModelProviderService()
	driver, mdlName, apiConfig, _, getErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantID, entity.ModelTypeRerank, rerankCompositeName)
	if getErr != nil {
		return nil, fmt.Errorf("failed to get rerank model: %w", getErr)
	}
	rerankModel := models.NewRerankModel(driver, &mdlName, apiConfig)
	common.Info("Fetched rerank model",
		zap.String("tenantID", tenantID), zap.String("rerankCompositeName", rerankCompositeName))
	return rerankModel, nil
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

	err = s.docEngine.UpdateChunks(ctx, condition, d, indexName, req.DatasetID)
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

	deletedCount, err := s.docEngine.DeleteChunks(ctx, condition, indexName, doc.KbID)
	if err != nil {
		return 0, fmt.Errorf("failed to delete chunks: %w", err)
	}

	return deletedCount, nil
}

// ── REST chunk helpers ────────────────────────────────────────────────────────

// resolveDatasetAccess validates user access to a dataset and returns the
// dataset's owning tenantID plus the Knowledgebase record.
// It also optionally validates that a document belongs to the dataset.
func (s *ChunkService) resolveDatasetAccess(userID, datasetID string) (tenantID string, kb *entity.Knowledgebase, err error) {
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get user tenants: %w", err)
	}
	for _, t := range tenants {
		kb, err = s.kbDAO.GetByIDAndTenantID(datasetID, t.TenantID)
		if err == nil && kb != nil {
			return t.TenantID, kb, nil
		}
	}
	return "", nil, fmt.Errorf("You don't own the dataset %s.", datasetID)
}

// getEmbeddingModelForKB resolves the embedding model for a knowledge base.
func (s *ChunkService) getEmbeddingModelForKB(kb *entity.Knowledgebase, tenantID string) (*models.EmbeddingModel, error) {
	tenantLLMDAO := dao.NewTenantLLMDAO()
	modelProviderSvc := NewModelProviderService()

	var embdID string
	var err error
	if kb.TenantEmbdID != nil && *kb.TenantEmbdID > 0 {
		_, embdID, err = dao.LookupTenantLLMByID(tenantLLMDAO, *kb.TenantEmbdID)
	} else if kb.EmbdID != "" {
		// Mirror Python TenantLLMService.split_model_name_and_factory: the factory
		// is the segment after the LAST "@", so model names that themselves contain
		// "@" (e.g. "Qwen/Qwen3-Embedding-8B@test@SILICONFLOW") resolve correctly.
		name, factory := kb.EmbdID, ""
		if idx := strings.LastIndex(kb.EmbdID, "@"); idx >= 0 {
			name, factory = kb.EmbdID[:idx], kb.EmbdID[idx+1:]
		}
		if factory != "" {
			_, embdID, err = dao.LookupTenantLLMByFactory(tenantLLMDAO, tenantID, factory, name, entity.ModelTypeEmbedding)
		} else {
			_, embdID, err = dao.LookupTenantLLMByName(tenantLLMDAO, tenantID, name, entity.ModelTypeEmbedding)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("failed to resolve embedding model: %w", err)
	}
	if embdID == "" {
		return nil, fmt.Errorf("no embedding model configured for dataset")
	}
	return modelProviderSvc.GetEmbeddingModel(tenantID, embdID)
}

// embedTexts calls the embedding model and returns the raw float64 slices plus
// an approximate token count for the embedded input.
//
// The embedding driver does not surface provider token usage (EmbeddingData has
// no token field), so the count is derived from the input text via the project
// tokenizer rather than from the embedding vector dimensionality (which is a
// fixed model property unrelated to consumption).
func embedTexts(em *models.EmbeddingModel, texts []string) ([][]float64, int, error) {
	resp, err := em.ModelDriver.Embed(em.ModelName, texts, em.APIConfig, nil)
	if err != nil {
		return nil, 0, err
	}
	vecs := make([][]float64, len(resp))
	for i, d := range resp {
		vecs[i] = d.Embedding
	}
	tokenCount := 0
	for _, t := range texts {
		tokenCount += estimateTokenCount(t)
	}
	return vecs, tokenCount, nil
}

// estimateTokenCount approximates the number of tokens in text. It tokenizes the
// text with the project tokenizer (which segments CJK and splits terms) and
// counts the resulting terms; on failure it falls back to a rune-based estimate
// of roughly one token per four characters.
func estimateTokenCount(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	if toks, err := tokenizer.Tokenize(text); err == nil && toks != "" {
		return len(strings.Fields(toks))
	}
	return (len([]rune(text)) + 3) / 4
}

// derefString safely dereferences a *string, returning "" when nil.
func derefString(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

// mapDocRun maps a document's run-status code to its label, mirroring Python's
// run_mapping in chunk_api._map_doc. Unknown/nil codes map to an empty string.
func mapDocRun(run *string) string {
	if run == nil {
		return ""
	}
	switch strings.TrimSpace(*run) {
	case "0":
		return "UNSTART"
	case "1":
		return "RUNNING"
	case "2":
		return "CANCEL"
	case "3":
		return "DONE"
	case "4":
		return "FAIL"
	default:
		return ""
	}
}

// weightedVec returns 0.1*a + 0.9*b (doc_name weight vs content weight).
func weightedVec(a, b []float64) []float64 {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	out := make([]float64, n)
	for i := range out {
		out[i] = 0.1*a[i] + 0.9*b[i]
	}
	return out
}

// ── ListChunksREST ────────────────────────────────────────────────────────────

// ListChunksREST mirrors Python GET /datasets/:dataset_id/documents/:document_id/chunks.
// dataset_id and document_id are path params; validation is ownership-based.
func (s *ChunkService) ListChunksREST(datasetID, documentID, userID string, page, pageSize int, keywords string, available *bool) (*ListChunksResponse, error) {
	if s.docEngine == nil {
		return nil, fmt.Errorf("doc engine not initialized")
	}

	tenantID, _, err := s.resolveDatasetAccess(userID, datasetID)
	if err != nil {
		return nil, err
	}

	// Verify document belongs to dataset.
	doc, err := s.documentDAO.GetByID(documentID)
	if err != nil || doc == nil {
		return nil, fmt.Errorf("You don't own the document %s.", documentID)
	}
	if doc.KbID != datasetID {
		return nil, fmt.Errorf("You don't own the document %s.", documentID)
	}

	ctx := context.Background()
	indexName := fmt.Sprintf("ragflow_%s", tenantID)

	searchReq := &types.SearchRequest{
		IndexNames: []string{indexName},
		MatchExprs: []interface{}{keywords},
		KbIDs:      []string{datasetID},
		Offset:     (page - 1) * pageSize,
		Limit:      pageSize,
		Filter:     map[string]interface{}{"doc_id": documentID},
	}
	if available != nil {
		avInt := 0
		if *available {
			avInt = 1
		}
		searchReq.Filter["available_int"] = avInt
	}

	searchResp, err := s.docEngine.Search(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	chunks := make([]map[string]interface{}, 0, len(searchResp.Chunks))
	for _, chunk := range searchResp.Chunks {
		result := map[string]interface{}{
			"id":                 chunk["id"],
			"content":            chunk["content_with_weight"],
			"document_id":        chunk["doc_id"],
			"docnm_kwd":          chunk["docnm_kwd"],
			"important_keywords": orSlice(chunk["important_kwd"]),
			"questions":          orSlice(chunk["question_kwd"]),
			"tag_kwd":            orSlice(chunk["tag_kwd"]),
			"dataset_id":         datasetID,
			"image_id":           orStr(chunk["img_id"]),
			"available":          intToBool(chunk["available_int"]),
			"positions":          orSlice(chunk["position_int"]),
		}
		chunks = append(chunks, result)
	}

	timeFormat := "2006-01-02T15:04:05"
	// Mirror Python chunk_api._map_doc: return the full document with the SDK key
	// renames (kb_id→dataset_id, chunk_num→chunk_count, token_num→token_count,
	// parser_id→chunk_method) and the run-status label mapping, so the frontend
	// receives every field it expects.
	docInfo := map[string]interface{}{
		"id":               doc.ID,
		"thumbnail":        doc.Thumbnail,
		"dataset_id":       doc.KbID,
		"chunk_method":     doc.ParserID,
		"pipeline_id":      doc.PipelineID,
		"parser_config":    doc.ParserConfig,
		"source_type":      doc.SourceType,
		"type":             doc.Type,
		"created_by":       doc.CreatedBy,
		"name":             doc.Name,
		"location":         doc.Location,
		"size":             doc.Size,
		"token_count":      doc.TokenNum,
		"chunk_count":      doc.ChunkNum,
		"progress":         utility.JSONFloat64(doc.Progress),
		"progress_msg":     doc.ProgressMsg,
		"process_begin_at": utility.FormatTimeToString(doc.ProcessBeginAt, timeFormat),
		"process_duration": doc.ProcessDuration,
		"content_hash":     doc.ContentHash,
		"meta_fields":      doc.MetaFields,
		"suffix":           doc.Suffix,
		"run":              mapDocRun(doc.Run),
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

func orSlice(v interface{}) interface{} {
	if v == nil {
		return []interface{}{}
	}
	return v
}

func orStr(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intToBool(v interface{}) bool {
	switch t := v.(type) {
	case int:
		return t != 0
	case int64:
		return t != 0
	case float64:
		return t != 0
	case string:
		return t != "0" && t != ""
	}
	return true // default available
}

// ── AddChunk ─────────────────────────────────────────────────────────────────

// AddChunkRequest mirrors the Python add_chunk body.
type AddChunkRequest struct {
	Content           string                 `json:"content"`
	ImportantKeywords []string               `json:"important_keywords"`
	Questions         []string               `json:"questions"`
	TagKwd            []string               `json:"tag_kwd"`
	TagFeas           map[string]interface{} `json:"tag_feas"`
}

// AddChunk mirrors Python POST /datasets/:dataset_id/documents/:document_id/chunks.
func (s *ChunkService) AddChunk(datasetID, documentID, userID string, req *AddChunkRequest) (map[string]interface{}, error) {
	if s.docEngine == nil {
		return nil, fmt.Errorf("doc engine not initialized")
	}
	if strings.TrimSpace(req.Content) == "" {
		return nil, fmt.Errorf("`content` is required")
	}

	tenantID, kb, err := s.resolveDatasetAccess(userID, datasetID)
	if err != nil {
		return nil, err
	}

	doc, err := s.documentDAO.GetByID(documentID)
	if err != nil || doc == nil || doc.KbID != datasetID {
		return nil, fmt.Errorf("You don't own the document %s.", documentID)
	}
	docName := derefString(doc.Name)

	// Deterministic chunk ID: xxhash64(content + document_id).
	chunkID := fmt.Sprintf("%x", xxhash.Sum64String(req.Content+documentID))

	// Tokenize content.
	contentLtks, _ := tokenizer.Tokenize(req.Content)
	contentSmLtks, _ := tokenizer.FineGrainedTokenize(contentLtks)

	// Build questions list (trimmed, non-empty).
	questions := make([]string, 0, len(req.Questions))
	for _, q := range req.Questions {
		if q = strings.TrimSpace(q); q != "" {
			questions = append(questions, q)
		}
	}
	importantKwd := req.ImportantKeywords
	if importantKwd == nil {
		importantKwd = []string{}
	}

	now := time.Now()
	d := map[string]interface{}{
		"id":                   chunkID,
		"content_ltks":         contentLtks,
		"content_sm_ltks":      contentSmLtks,
		"content_with_weight":  req.Content,
		"important_kwd":        importantKwd,
		"important_tks":        strings.Join(importantKwd, " "),
		"question_kwd":         questions,
		"question_tks":         strings.Join(questions, "\n"),
		"create_time":          now.Format("2006-01-02 15:04:05"),
		"create_timestamp_flt": float64(now.Unix()),
		"kb_id":                datasetID,
		"docnm_kwd":            docName,
		"doc_id":               documentID,
		"available_int":        1,
	}
	if len(req.TagKwd) > 0 {
		d["tag_kwd"] = req.TagKwd
	}
	if req.TagFeas != nil {
		d["tag_feas"] = req.TagFeas
	}

	// Compute embedding: 0.1 * embed(doc.name) + 0.9 * embed(content).
	em, err := s.getEmbeddingModelForKB(kb, tenantID)
	if err != nil {
		return nil, fmt.Errorf("failed to get embedding model: %w", err)
	}
	embedInput := req.Content
	if len(questions) > 0 {
		embedInput = strings.Join(questions, "\n")
	}
	vecs, tokenCount, err := embedTexts(em, []string{docName, embedInput})
	if err != nil {
		return nil, fmt.Errorf("embedding failed: %w", err)
	}
	if len(vecs) >= 2 {
		vec := weightedVec(vecs[0], vecs[1])
		d[fmt.Sprintf("q_%d_vec", len(vec))] = vec
	}

	// Insert into document store.
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	if _, err := s.docEngine.InsertChunks(ctx_bg(), []map[string]interface{}{d}, indexName, datasetID); err != nil {
		return nil, fmt.Errorf("failed to insert chunk: %w", err)
	}

	// Increment document chunk_num and token_num.
	_ = s.documentDAO.UpdateByID(documentID, map[string]interface{}{
		"chunk_num": gorm.Expr("chunk_num + 1"),
		"token_num": gorm.Expr("token_num + ?", tokenCount),
	})

	// Build response matching Python key_mapping.
	renamed := map[string]interface{}{
		"id":               chunkID,
		"content":          req.Content,
		"document_id":      documentID,
		"important_keywords": importantKwd,
		"questions":        questions,
		"dataset_id":       datasetID,
		"create_timestamp": float64(now.Unix()),
		"create_time":      d["create_time"],
	}
	if len(req.TagKwd) > 0 {
		renamed["tag_kwd"] = req.TagKwd
	}
	return map[string]interface{}{"chunk": renamed}, nil
}

// ctx_bg returns a background context (avoids referencing context.Background everywhere).
func ctx_bg() context.Context { return context.Background() }

// ── UpdateChunkREST ───────────────────────────────────────────────────────────

// UpdateChunkRESTRequest mirrors the Python PATCH body.
type UpdateChunkRESTRequest struct {
	Content           *string                `json:"content"`
	ImportantKeywords []string               `json:"important_keywords"`
	Questions         []string               `json:"questions"`
	Available         *bool                  `json:"available"`
	Positions         []interface{}          `json:"positions"`
	TagKwd            []string               `json:"tag_kwd"`
	TagFeas           map[string]interface{} `json:"tag_feas"`
}

// UpdateChunkREST mirrors Python PATCH /datasets/:dataset_id/documents/:document_id/chunks/:chunk_id.
// Like the existing UpdateChunk but with re-embedding on content change.
func (s *ChunkService) UpdateChunkREST(datasetID, documentID, chunkID, userID string, req *UpdateChunkRESTRequest) error {
	if s.docEngine == nil {
		return fmt.Errorf("doc engine not initialized")
	}

	tenantID, kb, err := s.resolveDatasetAccess(userID, datasetID)
	if err != nil {
		return err
	}

	doc, err := s.documentDAO.GetByID(documentID)
	if err != nil || doc == nil || doc.KbID != datasetID {
		return fmt.Errorf("You don't own the document %s.", documentID)
	}
	docName := derefString(doc.Name)

	ctx := context.Background()
	indexName := fmt.Sprintf("ragflow_%s", tenantID)

	// Get existing chunk.
	rawChunk, err := s.docEngine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
	if err != nil || rawChunk == nil {
		return fmt.Errorf("Can't find this chunk %s", chunkID)
	}
	existing, ok := rawChunk.(map[string]interface{})
	if !ok {
		return fmt.Errorf("Can't find this chunk %s", chunkID)
	}
	existingDocID := ""
	if v, ok := existing["doc_id"].(string); ok {
		existingDocID = v
	} else if v, ok := existing["document_id"].(string); ok {
		existingDocID = v
	}
	if existingDocID != documentID {
		return fmt.Errorf("Can't find this chunk %s", chunkID)
	}

	// Determine content.
	content := ""
	if req.Content != nil {
		if strings.TrimSpace(*req.Content) == "" {
			return fmt.Errorf("`content` is required")
		}
		content = *req.Content
	} else {
		if v, ok := existing["content_with_weight"].(string); ok {
			content = v
		} else if v, ok := existing["content"].(string); ok {
			content = v
		}
	}

	// Tokenize.
	contentLtks, _ := tokenizer.Tokenize(content)
	contentSmLtks, _ := tokenizer.FineGrainedTokenize(contentLtks)

	d := map[string]interface{}{
		"id":                  chunkID,
		"content_with_weight": content,
		"content_ltks":        contentLtks,
		"content_sm_ltks":     contentSmLtks,
	}

	if req.ImportantKeywords != nil {
		d["important_kwd"] = req.ImportantKeywords
		d["important_tks"] = strings.Join(req.ImportantKeywords, " ")
	}

	questions := []string{}
	if req.Questions != nil {
		for _, q := range req.Questions {
			if q = strings.TrimSpace(q); q != "" {
				questions = append(questions, q)
			}
		}
		d["question_kwd"] = questions
		d["question_tks"] = strings.Join(questions, "\n")
	}

	if req.Available != nil {
		avInt := 0
		if *req.Available {
			avInt = 1
		}
		d["available_int"] = avInt
	}
	if req.Positions != nil {
		d["position_int"] = req.Positions
	}
	if req.TagKwd != nil {
		d["tag_kwd"] = req.TagKwd
	}
	if req.TagFeas != nil {
		d["tag_feas"] = req.TagFeas
	}

	// Re-embed when content or questions changed.
	if req.Content != nil || req.Questions != nil {
		em, err := s.getEmbeddingModelForKB(kb, tenantID)
		if err != nil {
			return fmt.Errorf("failed to get embedding model: %w", err)
		}
		embedInput := content
		if len(questions) > 0 {
			embedInput = strings.Join(questions, "\n")
		}
		vecs, _, err := embedTexts(em, []string{docName, embedInput})
		if err != nil {
			return fmt.Errorf("embedding failed: %w", err)
		}
		if len(vecs) >= 2 {
			vec := weightedVec(vecs[0], vecs[1])
			d[fmt.Sprintf("q_%d_vec", len(vec))] = vec
		}
	}

	condition := map[string]interface{}{"id": chunkID}
	return s.docEngine.UpdateChunks(ctx, condition, d, indexName, datasetID)
}

// ── SwitchChunks ─────────────────────────────────────────────────────────────

// SwitchChunks mirrors Python PATCH /datasets/:dataset_id/documents/:document_id/chunks
// (without chunk_id) — bulk toggle of available_int.
func (s *ChunkService) SwitchChunks(datasetID, documentID, userID string, chunkIDs []string, available bool) error {
	if s.docEngine == nil {
		return fmt.Errorf("doc engine not initialized")
	}
	if len(chunkIDs) == 0 {
		return fmt.Errorf("`chunk_ids` is required.")
	}

	tenantID, _, err := s.resolveDatasetAccess(userID, datasetID)
	if err != nil {
		return err
	}

	// Mirror Python: verify the document belongs to the dataset before touching
	// the index.
	doc, err := s.documentDAO.GetByID(documentID)
	if err != nil || doc == nil || doc.KbID != datasetID {
		return fmt.Errorf("Document not found!")
	}

	ctx := context.Background()
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	availInt := 0
	if available {
		availInt = 1
	}

	// Update each chunk's available_int. Python's docStoreConn.update returns False
	// for a non-existent chunk id, surfacing as "Index updating failure" (code 102);
	// a blind update would otherwise report a false-positive success. Confirm the
	// chunk exists, then update.
	for _, chunkID := range chunkIDs {
		existing, gerr := s.docEngine.GetChunk(ctx, indexName, chunkID, []string{datasetID})
		if gerr != nil || existing == nil {
			return fmt.Errorf("Index updating failure")
		}
		condition := map[string]interface{}{"id": chunkID}
		update := map[string]interface{}{"available_int": availInt}
		if err := s.docEngine.UpdateChunks(ctx, condition, update, indexName, datasetID); err != nil {
			common.Warn("SwitchChunks: failed to update chunk", zap.String("chunkID", chunkID), zap.Error(err))
			return fmt.Errorf("Index updating failure")
		}
	}
	return nil
}

