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

package chunk

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"math"
	"math/rand"
	"path/filepath"
	"ragflow/internal/common"
	"ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	"ragflow/internal/entity/models"
	"ragflow/internal/server"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "image/gif"
	_ "image/png"

	"github.com/cespare/xxhash/v2"
	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/engine/types"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
	"ragflow/internal/storage"
	"ragflow/internal/tokenizer"
	"ragflow/internal/utility"
)

const (
	maximumPageNumber     = 100000
	maximumTaskPageNumber = maximumPageNumber * 1000
)

var chunkImageMergeLocks = struct {
	sync.Mutex
	locks map[string]*chunkImageMergeLock
}{locks: make(map[string]*chunkImageMergeLock)}

type chunkImageMergeLock struct {
	mu   sync.Mutex
	refs int
}

func searchConfigMap(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case entity.JSONMap:
		return map[string]interface{}(typed), true
	case map[string]interface{}:
		return typed, true
	default:
		return nil, false
	}
}

// ChunkService chunk service
type ChunkService struct {
	docEngine      engine.DocEngine
	engineType     server.EngineType
	embeddingCache *utility.EmbeddingLRU
	kbDAO          *dao.KnowledgebaseDAO
	userTenantDAO  *dao.UserTenantDAO
	documentDAO    *dao.DocumentDAO
	taskDAO        *dao.TaskDAO
	searchService  *service.SearchService

	accessibleFunc                func(string, string) bool
	getKnowledgebaseByIDFunc      func(string) (*entity.Knowledgebase, error)
	getDocumentsByIDsFunc         func([]string) ([]*entity.Document, error)
	getDocumentStorageAddressFunc func(*entity.Document) (string, string, error)
	queueParseTasksFunc           func(*entity.Document, string, string, int64) error
	beginParseDocumentFunc        func(string) error
	deleteTasksByDocIDsFunc       func([]string) (int64, error)
	getEmbeddingModelFunc         func(string, string) (*models.EmbeddingModel, error)
	incrementChunkStatsFunc       func(string, string, int64, int64, float64) error
	decrementChunkStatsFunc       func(string, string, int64, int64, float64) error
	storeChunkImageFunc           func(string, string, []byte) error
	tokenizeFunc                  func(string) (string, error)
	fineGrainedTokenizeFunc       func(string) (string, error)
	numTokensFunc                 func(string) int
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
		taskDAO:        dao.NewTaskDAO(),
		searchService:  service.NewSearchService(),
	}
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
func (s *ChunkService) RetrievalTest(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error) {
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
		common.PtrString(req.Page), common.PtrString(req.Size), req.DocIDs,
		common.PtrString(req.UseKG), common.PtrString(req.TopK), req.CrossLanguages, common.PtrString(req.SearchID),
		req.Filter,
		common.PtrString(req.TenantRerankID), common.PtrString(req.RerankID),
		common.PtrString(req.Keyword),
		common.PtrString(req.SimilarityThreshold), common.PtrString(req.VectorSimilarityWeight)))

	if req.Question == "" {
		return nil, fmt.Errorf("question is required")
	}
	if len(req.Datasets) == 0 {
		return nil, fmt.Errorf("dataset_ids is required")
	}

	ctx := context.Background()

	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tenants: %w", err)
	}
	if len(tenants) == 0 {
		return nil, fmt.Errorf("user has no accessible tenants")
	}
	common.Debug("Retrieved user tenants from database", zap.String("userID", userID), zap.Int("tenantCount", len(tenants)))

	var tenantIDs []string
	var kbRecords []*entity.Knowledgebase
	for _, datasetID := range req.Datasets {
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
			return nil, fmt.Errorf("only owner of dataset is authorized for this operation")
		}
	}

	// Check if all kbs have the same embedding model
	if len(kbRecords) > 1 {
		firstEmbeddingKey := knowledgebaseEmbeddingKey(kbRecords[0], tenantIDs[0])
		for i := 1; i < len(kbRecords); i++ {
			if knowledgebaseEmbeddingKey(kbRecords[i], tenantIDs[i]) != firstEmbeddingKey {
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
			common.Warn("Failed to get search detail for search_id, proceeding without it", zap.String("searchID", *req.SearchID), zap.Error(err))
		} else if searchConfig, ok := searchConfigMap(searchDetail["search_config"]); ok && searchConfig != nil {
			if searchMetaFilter, ok := searchConfigMap(searchConfig["meta_data_filter"]); ok {
				filter = searchMetaFilter
			}
			chatID, _ = searchConfig["chat_id"].(string)
		} else {
			common.Warn("No search_config found in search detail", zap.String("searchID", *req.SearchID))
		}
	}

	// If meta_data_filter method is auto/semi_auto, get chat model
	if filter != nil {
		method, _ := filter["method"].(string)
		if method == "auto" || method == "semi_auto" {
			modelProviderSvc := service.NewModelProviderService()
			if chatID != "" {
				// Use chat_id from search_config (it's actually the model name)
				driver, mdlName, apiConfig, _, getErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeChat, chatID)
				if getErr != nil {
					common.Warn("Failed to get chat model from search_config chat_id, using tenant default", zap.String("chatID", chatID), zap.Error(getErr))
				} else {
					chatModelForFilter = models.NewChatModel(driver, &mdlName, apiConfig)
					common.Info("Fetched chat model (from search_config) for metadata filter",
						zap.String("chatID", chatID),
						zap.String("tenantID", tenantIDs[0]))
				}

			}

			// If no chatID from search_config, or chatModel not found, use tenant default
			if chatModelForFilter == nil {
				tenantSvc := service.NewTenantService()
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
		metadataSvc := service.NewMetadataService()
		flattedMeta, err := metadataSvc.GetFlattedMetaByKBs([]string(req.Datasets))
		if err != nil {
			common.Warn("Failed to get flatted metadata", zap.Error(err))
		} else {
			common.Info("metadata filter conditions", zap.Any("filter", filter))
			filteredDocIDs, _ := service.ApplyMetaDataFilter(ctx, filter, flattedMeta, req.Question, chatModelForFilter, req.DocIDs, []string(req.Datasets))
			docIDs = filteredDocIDs
			common.Info("ApplyMetaDataFilter result", zap.Strings("docIDs", docIDs))
		}
	}

	// Apply cross_languages and keyword extraction with tenant default chat model
	modifiedQuestion := req.Question
	var chatModel *models.ChatModel

	// Get chat model for cross_languages and keyword_extraction
	var llmModelName string
	if len(req.CrossLanguages) > 0 || (req.Keyword != nil && *req.Keyword) {
		tenantSvc := service.NewTenantService()
		modelProviderSvc := service.NewModelProviderService()
		var err error
		llmModelName, err = tenantSvc.GetDefaultModelName(tenantIDs[0], "chat")
		if err != nil || llmModelName == "" {
			common.Warn("Failed to get default chat model name for LLM transformations", zap.Error(err))
		} else {
			driver, mdlName, apiConfig, _, getErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeChat, llmModelName)
			if getErr != nil {
				common.Warn("Failed to get chat model for LLM transformations", zap.Error(getErr))
			} else {
				chatModel = models.NewChatModel(driver, &mdlName, apiConfig)
				common.Info("Fetched chat model (tenant default) for cross_languages/keyword_extraction",
					zap.String("tenantID", tenantIDs[0]),
					zap.String("modelName", llmModelName))
			}
		}
	}

	// Apply cross_languages on the question (translate question)
	if len(req.CrossLanguages) > 0 {
		translated, err := service.CrossLanguages(ctx, tenantIDs[0], llmModelName, req.Question, req.CrossLanguages)
		if err != nil {
			common.Warn("Failed to translate question", zap.Error(err))
		} else {
			modifiedQuestion = translated
		}
	}

	// Apply keyword extraction on the question (append keywords to question)
	if chatModel != nil && req.Keyword != nil && *req.Keyword {
		extractedKeywords, err := service.KeywordExtraction(ctx, chatModel, modifiedQuestion, 3)
		if err != nil {
			common.Warn("Failed to extract keywords from question", zap.Error(err))
		} else if extractedKeywords != "" {
			modifiedQuestion = modifiedQuestion + " " + extractedKeywords
		}
	}

	if modifiedQuestion != req.Question {
		common.Info("Modified question after transformations",
			zap.String("originalQuestion", req.Question),
			zap.String("modifiedQuestion", modifiedQuestion),
			zap.Strings("crossLanguages", req.CrossLanguages),
			zap.Bool("keywordExtraction", req.Keyword != nil && *req.Keyword))
	}

	// Get tag-based rank features via LabelQuestion
	metadataSvc := service.NewMetadataService()
	labels := metadataSvc.LabelQuestion(modifiedQuestion, kbRecords)
	common.Debug("LabelQuestion result", zap.Any("labels", labels))

	// Determine embedding model.
	modelProviderSvc := service.NewModelProviderService()
	var embeddingModel *models.EmbeddingModel
	var embdID string
	if kbRecords[0].TenantEmbdID != nil && *kbRecords[0].TenantEmbdID > 0 {
		_, embdID, err = dao.LookupTenantLLMByID(dao.NewTenantLLMDAO(), *kbRecords[0].TenantEmbdID)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding model by tenant_embd_id: %w", err)
		}
		driver, modelName, apiConfig, maxTokens, getErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeEmbedding, embdID)
		if getErr != nil {
			return nil, fmt.Errorf("failed to get embedding model by tenant_embd_id: %w", getErr)
		}
		embeddingModel = models.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
	} else if kbRecords[0].EmbdID != "" {
		embdID = kbRecords[0].EmbdID
		driver, modelName, apiConfig, maxTokens, getErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeEmbedding, embdID)
		if getErr != nil {
			_, embdID, err = dao.LookupTenantLLMByName(dao.NewTenantLLMDAO(), tenantIDs[0], kbRecords[0].EmbdID, entity.ModelTypeEmbedding)
			if err != nil {
				return nil, fmt.Errorf("failed to get embedding model by embd_id: %w", getErr)
			}
			driver, modelName, apiConfig, maxTokens, getErr = modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeEmbedding, embdID)
			if getErr != nil {
				return nil, fmt.Errorf("failed to get embedding model by embd_id: %w", getErr)
			}
		}
		embeddingModel = models.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
	} else {
		driver, modelName, apiConfig, maxTokens, getErr := modelProviderSvc.GetTenantDefaultModelByType(tenantIDs[0], entity.ModelTypeEmbedding)
		if getErr != nil {
			return nil, fmt.Errorf("failed to get tenant default embedding model: %w", getErr)
		}
		embeddingModel = models.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
		embdID = fmt.Sprintf("%s@default", modelName)
	}

	if embeddingModel == nil {
		return nil, fmt.Errorf("no embedding model found for tenant %s", tenantIDs[0])
	}

	common.Info("Fetched embedding model for retrieval",
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
		rerankCompositeName = *req.RerankID
		if _, _, _, _, getErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeRerank, rerankCompositeName); getErr != nil {
			_, rerankCompositeName, err = dao.LookupTenantLLMByName(dao.NewTenantLLMDAO(), tenantIDs[0], *req.RerankID, entity.ModelTypeRerank)
			if err != nil {
				return nil, fmt.Errorf("failed to get rerank model by rerank_id: %w", getErr)
			}
		}
	}
	if rerankCompositeName != "" {
		driver, mdlName, apiConfig, _, getErr := modelProviderSvc.GetModelConfigFromProviderInstance(tenantIDs[0], entity.ModelTypeRerank, rerankCompositeName)
		if getErr != nil {
			return nil, fmt.Errorf("failed to get rerank model: %w", getErr)
		}
		rerankModel = models.NewRerankModel(driver, &mdlName, apiConfig)
	}

	if rerankModel != nil {
		common.Info("Fetched rerank model",
			zap.String("tenantID", tenantIDs[0]),
			zap.String("rerankCompositeName", rerankCompositeName))
	}

	retrievalReq := &nlp.RetrievalRequest{
		TenantIDs:              tenantIDs,
		Question:               modifiedQuestion,
		KbIDs:                  []string(req.Datasets),
		DocIDs:                 docIDs,
		Page:                   common.CoalesceInt(req.Page, 1),
		PageSize:               common.CoalesceInt(req.Size, 30),
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

	// Hydrate: ES returns zero vectors; replace with real vectors from FetchChunkVectors.
	// Infinity/OceanBase chunks already carry real vectors and are left unchanged.
	hydrateChunkVectors(ctx, s.docEngine, filteredChunks, req.Datasets, tenantIDs)

	common.Info("RetrievalTest completed", zap.String("userID", userID), zap.Any("kbID", req.Datasets), zap.String("question", req.Question), zap.Int64("chunkCount", int64(len(filteredChunks))))

	return &service.RetrievalTestResponse{
		Chunks:  filteredChunks,
		DocAggs: retrievalResult.DocAggs,
		Labels:  &labels,
		Total:   retrievalResult.Total,
	}, nil
}

func knowledgebaseEmbeddingKey(kb *entity.Knowledgebase, tenantID string) string {
	if kb.TenantEmbdID != nil && *kb.TenantEmbdID > 0 {
		return fmt.Sprintf("tenant:%d", *kb.TenantEmbdID)
	}
	if kb.EmbdID == "" {
		return fmt.Sprintf("default:%s", tenantID)
	}
	return fmt.Sprintf("embd:%s", kb.EmbdID)
}

// hydrateChunkVectors replaces zero (placeholder) vectors in chunks with real
// vectors fetched from the engine.  Infinity and OceanBase already ship real
// vectors with chunks, so this is a no-op for those engines; for ES it queries
// the engine by chunk ID list.  No if/else on engine type — just replaces
// whatever is missing or zero.
func hydrateChunkVectors(ctx context.Context, engine engine.DocEngine, chunks []map[string]interface{}, kbIDs []string, tenantIDs []string) {
	if len(chunks) == 0 {
		return
	}

	// Collect chunk IDs whose vectors are missing or all-zero.
	var missingIDs []string
	missingIdx := make(map[string]int)
	for i, ck := range chunks {
		id, _ := ck["id"].(string)
		if id == "" {
			continue
		}
		v, _ := ck["vector"].([]float64)
		if len(v) == 0 || common.IsZeroVector(v) {
			missingIDs = append(missingIDs, id)
			missingIdx[id] = i
		}
	}
	if len(missingIDs) == 0 {
		return
	}

	dim := 0
	for _, ck := range chunks {
		if v, _ := ck["vector"].([]float64); len(v) > 0 {
			dim = len(v)
			break
		}
	}
	if dim == 0 {
		return
	}

	vectors := FetchChunkVectors(ctx, engine, missingIDs, tenantIDs, kbIDs, dim)
	for id, v := range vectors {
		if idx, ok := missingIdx[id]; ok && !common.IsZeroVector(v) {
			chunks[idx]["vector"] = v
		}
	}
}

// Get retrieves a chunk by ID
func (s *ChunkService) Get(req *service.GetChunkRequest, userID string) (*service.GetChunkResponse, error) {
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
				return &service.GetChunkResponse{Chunk: result}, nil
			}
		}
	}

	if chunk == nil {
		return nil, fmt.Errorf("chunk not found")
	}

	return &service.GetChunkResponse{Chunk: chunk}, nil
}

const (
	docStopParsingInvalidStateMessage   = "Can't stop parsing document that has not started or already completed"
	docStopParsingInvalidStateErrorCode = "DOC_STOP_PARSING_INVALID_STATE"
)

func (s *ChunkService) cancelAllTasksOfDoc(docID string) error {
	tasks, err := s.taskDAO.GetByDocID(docID)
	if err != nil {
		return fmt.Errorf("failed to get tasks for document %s: %w", docID, err)
	}

	redisClient := redis.Get()
	if redisClient == nil {
		common.Warn(fmt.Sprintf("Redis unavailable; cannot cancel tasks for document %s", docID))
		return nil
	}

	for _, task := range tasks {
		if task == nil {
			continue
		}
		redisClient.Set(fmt.Sprintf("%s-cancel", task.ID), "x", 0)
	}

	return nil
}

func (s *ChunkService) StopParsing(userID, datasetID string, req service.StopParsingRequest) (*service.StopParsingResponse, common.ErrorCode, error) {
	if !s.kbDAO.Accessible(datasetID, userID) {
		return nil, common.CodeDataError, fmt.Errorf("You don't own the dataset %s", datasetID)
	}

	if len(req.DocumentIDs) == 0 {
		return nil, common.CodeDataError, fmt.Errorf("`document_ids` is required")
	}

	kb, err := s.kbDAO.GetByID(datasetID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("You don't own the dataset %s", datasetID)
	}

	docIDs, duplicateMessages := service.CheckDuplicateIDs(req.DocumentIDs, "document")
	successCount := 0
	ctx := context.Background()
	indexName := service.IndexName(kb.TenantID)

	for _, docID := range docIDs {
		doc, err := s.documentDAO.GetByDocumentIDAndDatasetID(docID, datasetID)
		if err != nil || doc == nil {
			return nil, common.CodeDataError, fmt.Errorf("You don't own the document %s", docID)
		}

		if doc.Run == nil || *doc.Run != string(entity.TaskStatusRunning) {
			return &service.StopParsingResponse{
				Data: map[string]interface{}{"error_code": docStopParsingInvalidStateErrorCode},
			}, common.CodeDataError, fmt.Errorf("%s", docStopParsingInvalidStateMessage)
		}

		if err := s.cancelAllTasksOfDoc(docID); err != nil {
			return nil, common.CodeServerError, err
		}

		updates := map[string]interface{}{
			"run":       string(entity.TaskStatusCancel),
			"progress":  0,
			"chunk_num": 0,
		}
		if err := s.documentDAO.UpdateByID(doc.ID, updates); err != nil {
			return nil, common.CodeServerError, fmt.Errorf("failed to update document %s: %w", doc.ID, err)
		}

		if s.docEngine != nil {
			exists, err := s.docEngine.ChunkStoreExists(ctx, indexName, datasetID)
			if err != nil {
				return nil, common.CodeServerError, fmt.Errorf("failed to check chunk store %s/%s: %w", indexName, datasetID, err)
			}
			if exists {
				if _, err := s.docEngine.DeleteChunks(ctx, map[string]interface{}{"doc_id": doc.ID}, indexName, datasetID); err != nil {
					return nil, common.CodeServerError, fmt.Errorf("failed to delete chunks for document %s: %w", doc.ID, err)
				}
			} else {
				common.Info(fmt.Sprintf("Skipping chunk delete during stop_parsing for doc %s: index %s/%s does not exist", doc.ID, indexName, datasetID))
			}
		} else {
			common.Info(fmt.Sprintf("Skipping chunk delete during stop_parsing for doc %s: index %s/%s does not exist", doc.ID, indexName, datasetID))
		}

		successCount++
	}

	if len(duplicateMessages) > 0 {
		if successCount > 0 {
			return &service.StopParsingResponse{
				Message: fmt.Sprintf("Partially stopped %d documents with %d errors", successCount, len(duplicateMessages)),
				Data: map[string]interface{}{
					"success_count": successCount,
					"errors":        duplicateMessages,
				},
			}, common.CodeSuccess, nil
		}
		return nil, common.CodeDataError, fmt.Errorf("%s", strings.Join(duplicateMessages, ";"))
	}

	return nil, common.CodeSuccess, nil
}

func checkDuplicateIDs(documentIDs []string, idTypes string) ([]string, []string) {
	idCount := make(map[string]int, len(documentIDs))
	duplicateMessages := make([]string, 0)
	uniqueDocIDs := make([]string, 0, len(documentIDs))

	for _, id := range documentIDs {
		idCount[id]++
	}
	for id, count := range idCount {
		if count > 1 {
			duplicateMessages = append(duplicateMessages, fmt.Sprintf("Duplicate %s ids: %s ", idTypes, id))
		}
		uniqueDocIDs = append(uniqueDocIDs, id)
	}
	return uniqueDocIDs, duplicateMessages
}

func (s *ChunkService) queueParseTasks(doc *entity.Document, bucket, objectName string, priority int64) error {
	if s.queueParseTasksFunc != nil {
		return s.queueParseTasksFunc(doc, bucket, objectName, priority)
	}
	tasks, err := s.buildParseTasks(doc, bucket, objectName, priority)
	if err != nil {
		return err
	}
	if len(tasks) == 0 {
		return nil
	}
	if err := dao.NewTaskDAO().CreateMany(tasks); err != nil {
		return err
	}

	queueName := s.parseQueueName(doc, priority)
	for _, task := range tasks {
		if task.Progress >= 1 {
			continue
		}
		message := parseTaskMessage(task)
		if ok := redis.Get().QueueProduct(queueName, message); !ok {
			if _, err := dao.NewTaskDAO().DeleteByDocIDs([]string{doc.ID}); err != nil {
				common.Warn("Failed to clean parse tasks after Redis enqueue failure",
					zap.String("docID", doc.ID),
					zap.Error(err))
			}
			return fmt.Errorf("Can't access Redis. Please check the Redis' status.")
		}
	}
	return nil
}

func (s *ChunkService) buildParseTasks(doc *entity.Document, bucket, objectName string, priority int64) ([]*entity.Task, error) {
	now := time.Now()
	ranges, err := s.parseTaskRanges(doc, bucket, objectName)
	if err != nil {
		return nil, err
	}
	tasks := make([]*entity.Task, 0, len(ranges))
	for _, pageRange := range ranges {
		taskID := utility.GenerateUUID()
		progressMsg := ""
		digest := s.parseTaskDigest(doc, pageRange.from, pageRange.to)
		chunkIDs := ""
		tasks = append(tasks, &entity.Task{
			ID:          taskID,
			DocID:       doc.ID,
			FromPage:    pageRange.from,
			ToPage:      pageRange.to,
			TaskType:    "",
			Priority:    priority,
			BeginAt:     &now,
			Progress:    0,
			ProgressMsg: &progressMsg,
			Digest:      &digest,
			ChunkIDs:    &chunkIDs,
		})
	}
	return tasks, nil
}

type parsePageRange struct {
	from int64
	to   int64
}

func (s *ChunkService) parseTaskRanges(doc *entity.Document, bucket, objectName string) ([]parsePageRange, error) {
	if doc.Type == "pdf" {
		return s.pdfParseTaskRanges(doc, bucket, objectName)
	}
	if doc.ParserID == string(entity.ParserTypeTable) {
		return s.tableParseTaskRanges(doc, bucket, objectName)
	}
	return []parsePageRange{{from: 0, to: maximumTaskPageNumber}}, nil
}

func (s *ChunkService) pdfParseTaskRanges(doc *entity.Document, bucket, objectName string) ([]parsePageRange, error) {
	binary, err := s.getStorageBinary(bucket, objectName)
	if err != nil {
		return nil, err
	}
	pages := estimatePDFPageCount(binary)
	pageSize := int64(parserConfigInt(doc.ParserConfig, "task_page_size", 12))
	if doc.ParserID == string(entity.ParserTypePaper) {
		pageSize = int64(parserConfigInt(doc.ParserConfig, "task_page_size", 22))
	}
	if doc.ParserID == string(entity.ParserTypeOne) ||
		doc.ParserID == string(entity.ParserTypeKG) ||
		parserConfigString(doc.ParserConfig, "layout_recognize", "DeepDOC") != "DeepDOC" ||
		parserConfigBool(doc.ParserConfig, "toc_extraction", false) {
		pageSize = maximumTaskPageNumber
	}
	if pageSize <= 0 {
		pageSize = 12
	}

	pageRanges := parserConfigPageRanges(doc.ParserConfig)
	ranges := make([]parsePageRange, 0)
	for _, configuredRange := range pageRanges {
		start := configuredRange.from - 1
		if start < 0 {
			start = 0
		}
		end := configuredRange.to - 1
		if pages >= 0 && end > pages {
			end = pages
		}
		for page := start; page < end; page += pageSize {
			to := page + pageSize
			if to > end {
				to = end
			}
			ranges = append(ranges, parsePageRange{from: page, to: to})
		}
	}
	if len(ranges) == 0 {
		ranges = append(ranges, parsePageRange{from: 0, to: maximumTaskPageNumber})
	}
	return ranges, nil
}

func (s *ChunkService) tableParseTaskRanges(doc *entity.Document, bucket, objectName string) ([]parsePageRange, error) {
	binary, err := s.getStorageBinary(bucket, objectName)
	if err != nil {
		return nil, err
	}
	rows := estimateTableRowCount(docName(doc), binary)
	if rows <= 0 {
		return []parsePageRange{{from: 0, to: maximumTaskPageNumber}}, nil
	}
	ranges := make([]parsePageRange, 0, (rows+2999)/3000)
	for row := int64(0); row < int64(rows); row += 3000 {
		to := row + 3000
		if to > int64(rows) {
			to = int64(rows)
		}
		ranges = append(ranges, parsePageRange{from: row, to: to})
	}
	return ranges, nil
}

func (s *ChunkService) getStorageBinary(bucket, objectName string) ([]byte, error) {
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}
	return storageImpl.Get(bucket, objectName)
}

func (s *ChunkService) beginParseDocument(docID string) error {
	if s.beginParseDocumentFunc != nil {
		return s.beginParseDocumentFunc(docID)
	}
	now := time.Now()
	return dao.GetDB().Model(&entity.Document{}).Where("id = ?", docID).Updates(map[string]interface{}{
		"progress_msg":     "Task is queued...",
		"process_begin_at": now,
		"progress":         rand.Float64() * 0.01,
		"run":              string(entity.TaskStatusRunning),
		"chunk_num":        0,
		"token_num":        0,
	}).Error
}

func (s *ChunkService) getDocumentStorageAddress(doc *entity.Document) (string, string, error) {
	if s.getDocumentStorageAddressFunc != nil {
		return s.getDocumentStorageAddressFunc(doc)
	}
	return service.NewDocumentService().GetDocumentStorageAddress(doc)
}

func (s *ChunkService) deleteTasksByDocIDs(docIDs []string) (int64, error) {
	if s.deleteTasksByDocIDsFunc != nil {
		return s.deleteTasksByDocIDsFunc(docIDs)
	}
	return dao.NewTaskDAO().DeleteByDocIDs(docIDs)
}

func (s *ChunkService) accessible(datasetID, userID string) bool {
	if s.accessibleFunc != nil {
		return s.accessibleFunc(datasetID, userID)
	}
	return s.kbDAO.Accessible(datasetID, userID)
}

func (s *ChunkService) getKnowledgebaseByID(datasetID string) (*entity.Knowledgebase, error) {
	if s.getKnowledgebaseByIDFunc != nil {
		return s.getKnowledgebaseByIDFunc(datasetID)
	}
	return s.kbDAO.GetByID(datasetID)
}

func (s *ChunkService) getDocumentsByIDs(docIDs []string) ([]*entity.Document, error) {
	if s.getDocumentsByIDsFunc != nil {
		return s.getDocumentsByIDsFunc(docIDs)
	}
	return s.documentDAO.GetByIDs(docIDs)
}

func (s *ChunkService) parseQueueName(doc *entity.Document, priority int64) string {
	suffix := "common"
	if doc.ParserID == string(entity.ParserTypeResume) {
		suffix = "resume"
	}
	return fmt.Sprintf("te.%d.%s", priority, suffix)
}

func (s *ChunkService) parseTaskDigest(doc *entity.Document, fromPage, toPage int64) string {
	hasher := xxhash.New()
	config := chunkingConfigForDigest(doc)
	keys := make([]string, 0, len(config))
	for key := range config {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		hasher.WriteString(stableString(config[key]))
	}
	hasher.WriteString(doc.ID)
	hasher.WriteString(strconv.FormatInt(fromPage, 10))
	hasher.WriteString(strconv.FormatInt(toPage, 10))
	return fmt.Sprintf("%x", hasher.Sum64())
}

func parseTaskMessage(task *entity.Task) map[string]interface{} {
	beginAt := ""
	if task.BeginAt != nil {
		beginAt = task.BeginAt.Format("2006-01-02 15:04:05")
	}
	digest := ""
	if task.Digest != nil {
		digest = *task.Digest
	}
	return map[string]interface{}{
		"id":        task.ID,
		"doc_id":    task.DocID,
		"from_page": task.FromPage,
		"to_page":   task.ToPage,
		"progress":  task.Progress,
		"priority":  task.Priority,
		"begin_at":  beginAt,
		"digest":    digest,
	}
}

func chunkingConfigForDigest(doc *entity.Document) map[string]interface{} {
	return map[string]interface{}{
		"doc_id":        doc.ID,
		"kb_id":         doc.KbID,
		"parser_id":     doc.ParserID,
		"parser_config": copyParserConfigForDigest(doc.ParserConfig),
	}
}

func copyParserConfigForDigest(config map[string]interface{}) map[string]interface{} {
	copied := make(map[string]interface{}, len(config))
	for key, value := range config {
		if key == "raptor" || key == "graphrag" {
			continue
		}
		copied[key] = value
	}
	return copied
}

func stableString(value interface{}) string {
	binary, err := json.Marshal(value)
	if err != nil {
		return fmt.Sprint(value)
	}
	return string(binary)
}

func parserConfigInt(config map[string]interface{}, key string, fallback int) int {
	value, ok := config[key]
	if !ok || value == nil {
		return fallback
	}
	switch typedValue := value.(type) {
	case int:
		return typedValue
	case int64:
		return int(typedValue)
	case float64:
		return int(typedValue)
	case json.Number:
		if intValue, err := typedValue.Int64(); err == nil {
			return int(intValue)
		}
	case string:
		if intValue, err := strconv.Atoi(strings.TrimSpace(typedValue)); err == nil {
			return intValue
		}
	}
	return fallback
}

func parserConfigString(config map[string]interface{}, key, fallback string) string {
	value, ok := config[key]
	if !ok || value == nil {
		return fallback
	}
	if stringValue, ok := value.(string); ok {
		return stringValue
	}
	return fmt.Sprint(value)
}

func parserConfigBool(config map[string]interface{}, key string, fallback bool) bool {
	value, ok := config[key]
	if !ok || value == nil {
		return fallback
	}
	switch typedValue := value.(type) {
	case bool:
		return typedValue
	case string:
		switch strings.ToLower(strings.TrimSpace(typedValue)) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return fallback
}

func parserConfigPageRanges(config map[string]interface{}) []parsePageRange {
	defaultRanges := []parsePageRange{{from: 1, to: maximumPageNumber}}
	raw, ok := config["pages"]
	if !ok || raw == nil {
		return defaultRanges
	}
	rawRanges, ok := raw.([]interface{})
	if !ok || len(rawRanges) == 0 {
		return defaultRanges
	}

	ranges := make([]parsePageRange, 0, len(rawRanges))
	for _, rawRange := range rawRanges {
		rangeValues, ok := rawRange.([]interface{})
		if !ok || len(rangeValues) < 2 {
			continue
		}
		from, okFrom := toInt64(rangeValues[0])
		to, okTo := toInt64(rangeValues[1])
		if okFrom && okTo && to > from {
			ranges = append(ranges, parsePageRange{from: from, to: to})
		}
	}
	if len(ranges) == 0 {
		return defaultRanges
	}
	return ranges
}

func toInt64(value interface{}) (int64, bool) {
	switch typedValue := value.(type) {
	case int:
		return int64(typedValue), true
	case int64:
		return typedValue, true
	case float64:
		return int64(typedValue), true
	case json.Number:
		intValue, err := typedValue.Int64()
		return intValue, err == nil
	case string:
		intValue, err := strconv.ParseInt(strings.TrimSpace(typedValue), 10, 64)
		return intValue, err == nil
	default:
		return 0, false
	}
}

var pdfPagePattern = regexp.MustCompile(`/Type\s*/Page\b`)

func estimatePDFPageCount(binary []byte) int64 {
	if len(binary) == 0 {
		return 0
	}
	return int64(len(pdfPagePattern.FindAll(binary, -1)))
}

func estimateTableRowCount(name string, binary []byte) int {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx":
		if rows, err := countXLSXRows(binary); err == nil {
			return rows
		}
	case ".csv", ".tsv", ".txt":
		return countDelimitedRows(name, binary)
	}
	return 0
}

func countDelimitedRows(name string, binary []byte) int {
	reader := csv.NewReader(bytes.NewReader(binary))
	reader.FieldsPerRecord = -1
	reader.ReuseRecord = true
	if strings.EqualFold(filepath.Ext(name), ".tsv") {
		reader.Comma = '\t'
	}
	rows := 0
	for {
		_, err := reader.Read()
		if err == nil {
			rows++
			continue
		}
		if err == io.EOF {
			break
		}
		rows += bytes.Count(binary, []byte{'\n'})
		if len(binary) > 0 && binary[len(binary)-1] != '\n' {
			rows++
		}
		break
	}
	return rows
}

func countXLSXRows(binary []byte) (int, error) {
	zipReader, err := zip.NewReader(bytes.NewReader(binary), int64(len(binary)))
	if err != nil {
		return 0, err
	}
	maxRows := 0
	for _, file := range zipReader.File {
		if !strings.HasPrefix(file.Name, "xl/worksheets/") || !strings.HasSuffix(file.Name, ".xml") {
			continue
		}
		rows, err := countWorksheetRows(file)
		if err != nil {
			return 0, err
		}
		if rows > maxRows {
			maxRows = rows
		}
	}
	return maxRows, nil
}

func countWorksheetRows(file *zip.File) (int, error) {
	reader, err := file.Open()
	if err != nil {
		return 0, err
	}
	defer reader.Close()

	decoder := xml.NewDecoder(reader)
	rows := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, err
		}
		start, ok := token.(xml.StartElement)
		if ok && start.Name.Local == "row" {
			rows++
		}
	}
	return rows, nil
}

func docName(doc *entity.Document) string {
	if doc.Name == nil {
		return ""
	}
	return *doc.Name
}

func (s *ChunkService) Parse(userID, datasetID string, req *service.ParseFileRequest) (map[string]interface{}, common.ErrorCode, error) {
	if !s.accessible(datasetID, userID) {
		return nil, common.CodeOperatingError, fmt.Errorf("You don't own the dataset %s.", datasetID)
	}
	if req == nil || len(req.DocumentIDs) == 0 {
		return nil, common.CodeDataError, fmt.Errorf("`document_ids` is required")
	}

	kb, err := s.getKnowledgebaseByID(datasetID)
	if err != nil || kb == nil {
		return nil, common.CodeDataError, fmt.Errorf("dataset not found")
	}

	docIDs, duplicateMessages := checkDuplicateIDs(req.DocumentIDs, "document")
	notFound := make([]string, 0)

	docs, err := s.getDocumentsByIDs(docIDs)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	docByID := make(map[string]*entity.Document, len(docs))
	for _, doc := range docs {
		docByID[doc.ID] = doc
	}
	for _, docID := range docIDs {
		doc := docByID[docID]
		if doc == nil || doc.KbID != datasetID {
			notFound = append(notFound, docID)
		}
	}
	if len(notFound) > 0 {
		return nil, common.CodeDataError, fmt.Errorf("Documents not found: %v", notFound)
	}
	for _, docID := range docIDs {
		doc := docByID[docID]
		if doc.Run != nil && *doc.Run == string(entity.TaskStatusRunning) {
			return nil, common.CodeDataError, fmt.Errorf("Can't parse document that is currently being processed")
		}
	}

	successCount := 0

	for _, docID := range docIDs {
		doc := docByID[docID]

		if s.docEngine != nil {
			indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
			if _, err := s.docEngine.DeleteChunks(context.Background(), map[string]interface{}{"doc_id": docID}, indexName, datasetID); err != nil {
				return nil, common.CodeServerError, err
			}
		}
		if _, err := s.deleteTasksByDocIDs([]string{docID}); err != nil {
			return nil, common.CodeServerError, err
		}

		bucket, objectName, err := s.getDocumentStorageAddress(doc)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if err := s.queueParseTasks(doc, bucket, objectName, 0); err != nil {
			return nil, common.CodeServerError, err
		}
		if err := s.beginParseDocument(doc.ID); err != nil {
			if _, delErr := s.deleteTasksByDocIDs([]string{doc.ID}); delErr != nil {
				common.Warn("Failed to clean parse tasks after document state update failure",
					zap.String("docID", doc.ID),
					zap.Error(delErr))
			}
			return nil, common.CodeServerError, err
		}
		successCount++
	}

	if len(duplicateMessages) > 0 {
		if successCount > 0 {
			return map[string]interface{}{
				"success_count": successCount,
				"errors":        duplicateMessages,
			}, common.CodeSuccess, fmt.Errorf("Partially parsed %d documents with %d errors", successCount, len(duplicateMessages))
		}
		return nil, common.CodeDataError, fmt.Errorf("%s", strings.Join(duplicateMessages, ";"))
	}
	return nil, common.CodeSuccess, nil
}

// List retrieves chunks for a document
func (s *ChunkService) List(req *service.ListChunksRequest, userID string) (*service.ListChunksResponse, error) {
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
	if req.DatasetID != "" && doc.KbID != req.DatasetID {
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

	page := common.CoalesceInt(req.Page, 1)
	size := common.CoalesceInt(req.Size, 30)
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

	return &service.ListChunksResponse{
		Total:  searchResp.Total,
		Chunks: chunks,
		Doc:    docInfo,
	}, nil
}

func (s *ChunkService) SwitchChunks(userID, datasetID, documentID string, availableInt int, chunkIDs []string) error {
	if s.docEngine == nil {
		return fmt.Errorf("doc engine not initialized")
	}

	if availableInt != 0 && availableInt != 1 {
		return fmt.Errorf("available_int should be 0 or 1")
	}

	if chunkIDs == nil || len(chunkIDs) == 0 {
		return fmt.Errorf("req is null")
	}

	ctx := context.Background()
	defer ctx.Done()

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
		kb, err := s.kbDAO.GetByIDAndTenantID(datasetID, tenant.TenantID)
		if err == nil && kb != nil {
			targetTenantID = tenant.TenantID
			break
		}
	}
	if targetTenantID == "" {
		return fmt.Errorf("user does not have access to this dataset")
	}

	docDAO := dao.NewDocumentDAO()
	doc, err := docDAO.GetByID(documentID)
	if err != nil || doc == nil {
		return fmt.Errorf("document not found")
	}
	if doc.KbID != datasetID {
		return fmt.Errorf("document does not belong to this dataset")
	}

	for _, cid := range chunkIDs {
		indexName := fmt.Sprintf("ragflow_%s", targetTenantID)

		if err = s.docEngine.UpdateChunks(ctx, map[string]interface{}{
			"id":     cid,
			"doc_id": documentID,
		}, map[string]interface{}{
			"id":            cid,
			"available_int": availableInt,
		}, indexName, datasetID); err != nil {
			return err
		}
	}

	return nil
}

func (s *ChunkService) UpdateChunk(req *service.UpdateChunkRequest, userID string) error {
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
		tagFeas, err := validateTagFeatures(req.TagFeas)
		if err != nil {
			return updateChunkError{code: common.CodeArgumentError, message: "`tag_feas` " + err.Error()}
		}
		d["tag_feas"] = tagFeas
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
func (s *ChunkService) RemoveChunks(req *service.RemoveChunksRequest, userID string) (int64, error) {
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

	if deletedCount > 0 {
		if err := s.decrementChunkStats(req.DocID, doc.KbID, 0, deletedCount, 0); err != nil {
			return deletedCount, fmt.Errorf("failed to update chunk stats: %w", err)
		}
	}

	return deletedCount, nil
}

func (s *ChunkService) AddChunk(req *service.AddChunkRequest, userID string) (*service.AddChunkResponse, error) {
	if s.docEngine == nil {
		return nil, addChunkError{code: common.CodeServerError, message: "doc engine not initialized"}
	}
	if req == nil {
		return nil, addChunkError{code: common.CodeDataError, message: "invalid request payload"}
	}
	if !s.accessible(req.DatasetID, userID) {
		return nil, addChunkError{code: common.CodeDataError, message: fmt.Sprintf("You don't own the dataset %s.", req.DatasetID)}
	}

	kb, err := s.getKnowledgebaseByID(req.DatasetID)
	if err != nil || kb == nil {
		return nil, addChunkError{code: common.CodeDataError, message: fmt.Sprintf("You don't own the dataset %s.", req.DatasetID)}
	}

	doc, err := s.documentDAO.GetByDocumentIDAndDatasetID(req.DocumentID, req.DatasetID)
	if err != nil || doc == nil {
		return nil, addChunkError{code: common.CodeDataError, message: fmt.Sprintf("You don't own the document %s.", req.DocumentID)}
	}

	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, addChunkError{code: common.CodeDataError, message: "`content` is required"}
	}

	var tagFeas map[string]float64
	if req.TagFeas != nil {
		tagFeas, err = validateTagFeatures(req.TagFeas)
		if err != nil {
			return nil, addChunkError{code: common.CodeDataError, message: "`tag_feas` " + err.Error()}
		}
	}

	chunkID := strconv.FormatUint(xxhash.Sum64([]byte(req.Content+req.DocumentID)), 16)
	indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
	contentLtks, err := s.tokenize(req.Content)
	if err != nil {
		return nil, addChunkError{code: common.CodeServerError, message: fmt.Sprintf("tokenize content: %v", err)}
	}
	contentSmLtks, err := s.fineGrainedTokenize(contentLtks)
	if err != nil {
		return nil, addChunkError{code: common.CodeServerError, message: fmt.Sprintf("tokenize content fine-grained: %v", err)}
	}
	importantTks, err := s.tokenize(strings.Join(req.ImportantKeywords, " "))
	if err != nil {
		return nil, addChunkError{code: common.CodeServerError, message: fmt.Sprintf("tokenize important keywords: %v", err)}
	}
	questionKwd := filterTrimmedStrings(req.Questions)
	questionTks, err := s.tokenize(strings.Join(req.Questions, "\n"))
	if err != nil {
		return nil, addChunkError{code: common.CodeServerError, message: fmt.Sprintf("tokenize questions: %v", err)}
	}

	now := time.Now()
	docName := ""
	if doc.Name != nil {
		docName = *doc.Name
	}
	importantKeywords := req.ImportantKeywords
	if importantKeywords == nil {
		importantKeywords = []string{}
	}

	chunkData := map[string]interface{}{
		"id":                   chunkID,
		"content_with_weight":  req.Content,
		"content_ltks":         contentLtks,
		"content_sm_ltks":      contentSmLtks,
		"important_kwd":        importantKeywords,
		"important_tks":        importantTks,
		"question_kwd":         questionKwd,
		"question_tks":         questionTks,
		"create_time":          now.Format("2006-01-02 15:04:05"),
		"create_timestamp_flt": float64(now.UnixNano()) / float64(time.Second),
		"kb_id":                req.DatasetID,
		"docnm_kwd":            docName,
		"doc_id":               req.DocumentID,
	}
	if req.TagKwd != nil {
		chunkData["tag_kwd"] = req.TagKwd
	}
	if tagFeas != nil {
		chunkData["tag_feas"] = tagFeas
	}

	if req.ImageBase64 != nil {
		imageBinary, err := decodeChunkImageBase64(*req.ImageBase64)
		if err != nil {
			return nil, addChunkError{code: common.CodeDataError, message: err.Error()}
		}
		if err := s.storeChunkImage(req.DatasetID, chunkID, imageBinary); err != nil {
			return nil, addChunkError{code: common.CodeDataError, message: "Failed to store chunk image"}
		}
		chunkData["img_id"] = fmt.Sprintf("%s-%s", req.DatasetID, chunkID)
		chunkData["doc_type_kwd"] = "image"
	}

	embeddingModel, err := s.getEmbeddingModel(kb.TenantID, kb.EmbdID)
	if err != nil {
		return nil, addChunkError{code: common.CodeServerError, message: fmt.Sprintf("get embedding model: %v", err)}
	}
	embeddingText := req.Content
	if len(questionKwd) > 0 {
		embeddingText = strings.Join(questionKwd, "\n")
	}
	embeddings, err := embeddingModel.ModelDriver.Embed(embeddingModel.ModelName, []string{docName, embeddingText}, embeddingModel.APIConfig, &models.EmbeddingConfig{Dimension: 0})
	if err != nil {
		return nil, addChunkError{code: common.CodeServerError, message: fmt.Sprintf("encode chunk embedding: %v", err)}
	}
	if len(embeddings) != 2 {
		return nil, addChunkError{code: common.CodeServerError, message: fmt.Sprintf("unexpected embedding count: %d", len(embeddings))}
	}
	mergedVec, err := mergeChunkEmbeddings(embeddings[0].Embedding, embeddings[1].Embedding)
	if err != nil {
		return nil, addChunkError{code: common.CodeServerError, message: err.Error()}
	}
	chunkData[fmt.Sprintf("q_%d_vec", len(mergedVec))] = mergedVec

	ctx, cancel := context.WithTimeout(context.Background(), 600*time.Second)
	defer cancel()
	if _, err := s.docEngine.InsertChunks(ctx, []map[string]interface{}{chunkData}, indexName, req.DatasetID); err != nil {
		return nil, addChunkError{code: common.CodeServerError, message: fmt.Sprintf("insert chunk: %v", err)}
	}

	tokenNum := int64(s.numTokens(req.Content))
	if err := s.incrementChunkStats(req.DocumentID, req.DatasetID, tokenNum, 1, 0); err != nil {
		return nil, addChunkError{code: common.CodeServerError, message: fmt.Sprintf("increment chunk stats: %v", err)}
	}

	renamedChunk := map[string]interface{}{
		"id":                 chunkID,
		"content":            req.Content,
		"document_id":        req.DocumentID,
		"document":           docName,
		"important_keywords": importantKeywords,
		"questions":          questionKwd,
		"dataset_id":         req.DatasetID,
		"create_timestamp":   chunkData["create_timestamp_flt"],
		"create_time":        chunkData["create_time"],
	}
	if req.TagKwd != nil {
		renamedChunk["tag_kwd"] = req.TagKwd
	}
	if imgID, ok := chunkData["img_id"]; ok {
		renamedChunk["image_id"] = imgID
	}

	return &service.AddChunkResponse{Chunk: renamedChunk}, nil
}

type addChunkError struct {
	code    common.ErrorCode
	message string
}

type updateChunkError struct {
	code    common.ErrorCode
	message string
}

func (e updateChunkError) Error() string {
	return e.message
}

func (e updateChunkError) Code() common.ErrorCode {
	return e.code
}

func (e addChunkError) Error() string {
	return e.message
}

func (e addChunkError) Code() common.ErrorCode {
	return e.code
}

func validateTagFeatures(raw interface{}) (map[string]float64, error) {
	parsed, ok := raw.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("must be an object mapping string tags to finite numeric scores")
	}
	cleaned := make(map[string]float64, len(parsed))
	for key, value := range parsed {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("keys must be non-empty strings")
		}
		switch typed := value.(type) {
		case float64:
			if math.IsNaN(typed) || math.IsInf(typed, 0) || typed <= 0 {
				return nil, fmt.Errorf("values must be finite numbers greater than 0")
			}
			cleaned[key] = typed
		case float32:
			if math.IsNaN(float64(typed)) || math.IsInf(float64(typed), 0) || typed <= 0 {
				return nil, fmt.Errorf("values must be finite numbers greater than 0")
			}
			cleaned[key] = float64(typed)
		case int:
			if typed <= 0 {
				return nil, fmt.Errorf("values must be finite numbers greater than 0")
			}
			cleaned[key] = float64(typed)
		case int8:
			if typed <= 0 {
				return nil, fmt.Errorf("values must be finite numbers greater than 0")
			}
			cleaned[key] = float64(typed)
		case int16:
			if typed <= 0 {
				return nil, fmt.Errorf("values must be finite numbers greater than 0")
			}
			cleaned[key] = float64(typed)
		case int32:
			if typed <= 0 {
				return nil, fmt.Errorf("values must be finite numbers greater than 0")
			}
			cleaned[key] = float64(typed)
		case int64:
			if typed <= 0 {
				return nil, fmt.Errorf("values must be finite numbers greater than 0")
			}
			cleaned[key] = float64(typed)
		default:
			return nil, fmt.Errorf("values must be finite numbers greater than 0")
		}
	}
	return cleaned, nil
}

func decodeChunkImageBase64(raw string) ([]byte, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("`image_base64` must be a non-empty string")
	}
	imageBinary, err := base64.StdEncoding.Strict().DecodeString(raw)
	if err != nil {
		return nil, fmt.Errorf("Invalid `image_base64`")
	}
	if len(imageBinary) == 0 {
		return nil, fmt.Errorf("`image_base64` is empty")
	}
	return imageBinary, nil
}

func mergeChunkEmbeddings(a, b []float64) ([]float64, error) {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return nil, fmt.Errorf("unexpected embedding dimensions")
	}
	merged := make([]float64, len(a))
	for i := range a {
		merged[i] = 0.1*a[i] + 0.9*b[i]
	}
	return merged, nil
}

func filterTrimmedStrings(values []string) []string {
	filtered := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}

func (s *ChunkService) tokenize(text string) (string, error) {
	if s.tokenizeFunc != nil {
		return s.tokenizeFunc(text)
	}
	return tokenizer.Tokenize(text)
}

func (s *ChunkService) fineGrainedTokenize(text string) (string, error) {
	if s.fineGrainedTokenizeFunc != nil {
		return s.fineGrainedTokenizeFunc(text)
	}
	return tokenizer.FineGrainedTokenize(text)
}

func (s *ChunkService) numTokens(text string) int {
	if s.numTokensFunc != nil {
		return s.numTokensFunc(text)
	}
	return tokenizer.NumTokensFromString(text)
}

func (s *ChunkService) getEmbeddingModel(tenantID, embdID string) (*models.EmbeddingModel, error) {
	if s.getEmbeddingModelFunc != nil {
		return s.getEmbeddingModelFunc(tenantID, embdID)
	}
	return service.NewModelProviderService().GetEmbeddingModel(tenantID, embdID)
}

func (s *ChunkService) incrementChunkStats(docID, kbID string, tokenNum, chunkNum int64, duration float64) error {
	if s.incrementChunkStatsFunc != nil {
		return s.incrementChunkStatsFunc(docID, kbID, tokenNum, chunkNum, duration)
	}
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&entity.Document{}).
			Where("id = ? AND kb_id = ?", docID, kbID).
			Updates(map[string]interface{}{
				"token_num":        gorm.Expr("token_num + ?", tokenNum),
				"chunk_num":        gorm.Expr("chunk_num + ?", chunkNum),
				"process_duration": gorm.Expr("process_duration + ?", duration),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("document not found")
		}

		result = tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", kbID).
			Updates(map[string]interface{}{
				"token_num": gorm.Expr("token_num + ?", tokenNum),
				"chunk_num": gorm.Expr("chunk_num + ?", chunkNum),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("knowledgebase not found")
		}
		return nil
	})
}

func (s *ChunkService) decrementChunkStats(docID, kbID string, tokenNum, chunkNum int64, duration float64) error {
	if s.decrementChunkStatsFunc != nil {
		return s.decrementChunkStatsFunc(docID, kbID, tokenNum, chunkNum, duration)
	}
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&entity.Document{}).
			Where("id = ? AND kb_id = ?", docID, kbID).
			Updates(map[string]interface{}{
				"token_num":        gorm.Expr("CASE WHEN token_num - ? >= 0 THEN token_num - ? ELSE 0 END", tokenNum, tokenNum),
				"chunk_num":        gorm.Expr("CASE WHEN chunk_num - ? >= 0 THEN chunk_num - ? ELSE 0 END", chunkNum, chunkNum),
				"process_duration": gorm.Expr("CASE WHEN process_duration + ? >= 0 THEN process_duration + ? ELSE 0 END", duration, duration),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("document not found")
		}

		result = tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", kbID).
			Updates(map[string]interface{}{
				"token_num": gorm.Expr("CASE WHEN token_num - ? >= 0 THEN token_num - ? ELSE 0 END", tokenNum, tokenNum),
				"chunk_num": gorm.Expr("CASE WHEN chunk_num - ? >= 0 THEN chunk_num - ? ELSE 0 END", chunkNum, chunkNum),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("knowledgebase not found")
		}
		return nil
	})
}

func (s *ChunkService) storeChunkImage(bucket, chunkID string, imageBinary []byte) error {
	if s.storeChunkImageFunc != nil {
		return s.storeChunkImageFunc(bucket, chunkID, imageBinary)
	}
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return fmt.Errorf("storage not initialized")
	}
	lockKey := bucket + "/" + chunkID
	lock := acquireChunkImageMergeLock(lockKey)
	lock.mu.Lock()
	defer func() {
		lock.mu.Unlock()
		releaseChunkImageMergeLock(lockKey)
	}()

	if !storageImpl.ObjExist(bucket, chunkID) {
		return storageImpl.Put(bucket, chunkID, imageBinary)
	}

	oldBinary, err := storageImpl.Get(bucket, chunkID)
	if err != nil {
		return err
	}
	oldImage, _, err := image.Decode(bytes.NewReader(oldBinary))
	if err != nil {
		return err
	}
	newImage, _, err := image.Decode(bytes.NewReader(imageBinary))
	if err != nil {
		return err
	}
	oldBounds, newBounds := oldImage.Bounds(), newImage.Bounds()
	width := oldBounds.Dx()
	if newBounds.Dx() > width {
		width = newBounds.Dx()
	}
	height := oldBounds.Dy() + newBounds.Dy()
	combined := image.NewRGBA(image.Rect(0, 0, width, height))
	draw.Draw(combined, combined.Bounds(), &image.Uniform{C: color.White}, image.Point{}, draw.Src)
	draw.Draw(combined, oldBounds, oldImage, oldBounds.Min, draw.Src)
	draw.Draw(combined, image.Rect(0, oldBounds.Dy(), newBounds.Dx(), oldBounds.Dy()+newBounds.Dy()), newImage, newBounds.Min, draw.Src)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, combined, nil); err != nil {
		return err
	}
	return storageImpl.Put(bucket, chunkID, buf.Bytes())
}

func acquireChunkImageMergeLock(key string) *chunkImageMergeLock {
	chunkImageMergeLocks.Lock()
	defer chunkImageMergeLocks.Unlock()

	lock := chunkImageMergeLocks.locks[key]
	if lock == nil {
		lock = &chunkImageMergeLock{}
		chunkImageMergeLocks.locks[key] = lock
	}
	lock.refs++
	return lock
}

func releaseChunkImageMergeLock(key string) {
	chunkImageMergeLocks.Lock()
	defer chunkImageMergeLocks.Unlock()

	lock := chunkImageMergeLocks.locks[key]
	if lock == nil {
		return
	}
	lock.refs--
	if lock.refs == 0 {
		delete(chunkImageMergeLocks.locks, key)
	}
}
