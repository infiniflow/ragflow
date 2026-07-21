package dataset

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
)

func (d *DatasetService) SearchDataset(datasetID, userID string, req *service.SearchDatasetRequest) (*service.SearchDatasetsResponse, error) {
	if datasetID == "" {
		return nil, fmt.Errorf("dataset_id is required")
	}
	return d.SearchDatasets(req.ToSearchDatasetsRequest(datasetID), userID)
}

func (d *DatasetService) SearchDatasets(req *service.SearchDatasetsRequest, userID string) (*service.SearchDatasetsResponse, error) {
	if req.Question == "" {
		return nil, fmt.Errorf("question is required")
	}
	if len(req.DatasetIDs) == 0 {
		return nil, fmt.Errorf("dataset_ids is required")
	}
	common.Info("SearchDatasets started", zap.String("userID", userID), zap.Any("datasets", req.DatasetIDs), zap.String("question", req.Question))

	page := 1
	if req.Page != nil {
		page = *req.Page
	}
	pageSize := 30
	if req.Size != nil {
		pageSize = *req.Size
	}
	useKG := false
	if req.UseKG != nil {
		useKG = *req.UseKG
	}
	similarityThreshold := 0.0
	if req.SimilarityThreshold != nil {
		similarityThreshold = *req.SimilarityThreshold
	}
	vectorSimilarityWeight := 0.3
	if req.VectorSimilarityWeight != nil {
		vectorSimilarityWeight = *req.VectorSimilarityWeight
	}
	topK := 1024
	if req.TopK != nil {
		topK = *req.TopK
	}
	if topK < 1 {
		topK = 1
	} else if topK > 2048 {
		topK = 2048
	}
	keyword := false
	if req.Keyword != nil {
		keyword = *req.Keyword
	}
	searchID := ""
	if req.SearchID != nil {
		searchID = *req.SearchID
	}
	rerankID := ""
	if req.RerankID != nil {
		rerankID = *req.RerankID
	}

	question := req.Question
	datasetIDs := req.DatasetIDs
	metadataFilter := req.MetadataFilter
	crossLanguages := req.CrossLanguages

	ctx := context.Background()
	modelProviderSvc := service.NewModelProviderService()

	// Access check for all datasets
	var tenantIDs []string
	var kbRecords []*entity.Knowledgebase
	seenTenants := make(map[string]bool)
	for _, datasetID := range datasetIDs {
		if !d.kbDAO.Accessible(datasetID, userID) {
			common.Warn("SearchDatasets access denied", zap.String("datasetID", datasetID), zap.String("userID", userID))
			return nil, fmt.Errorf("only owner of dataset %s is authorized for this operation", datasetID)
		}

		kb, err := d.kbDAO.GetByID(datasetID)
		if err != nil || kb == nil {
			common.Warn("SearchDatasets dataset not found", zap.String("datasetID", datasetID))
			return nil, fmt.Errorf("dataset %s not found", datasetID)
		}
		if !seenTenants[kb.TenantID] {
			seenTenants[kb.TenantID] = true
			tenantIDs = append(tenantIDs, kb.TenantID)
		}
		kbRecords = append(kbRecords, kb)
	}

	// Check if all kbs have the same embedding model
	if err := service.ValidateDatasetEmbeddingModels(kbRecords); err != nil {
		return nil, err
	}

	// Override request fields with values from saved search config
	var chatID string
	if searchID != "" {
		if d.searchService == nil {
			common.Warn("Search service is not initialized for search_id", zap.String("searchID", searchID))
			return nil, fmt.Errorf("Invalid search_id")
		}
		searchDetail, err := d.searchService.GetDetail(searchID)
		if err != nil || searchDetail == nil || len(searchDetail) == 0 {
			common.Warn("Invalid search_id", zap.String("searchID", searchID), zap.Error(err))
			return nil, fmt.Errorf("Invalid search_id")
		} else if searchConfig, ok := searchDetail["search_config"].(map[string]interface{}); ok && searchConfig != nil {
			if scMetadataFilter, ok := searchConfig["meta_data_filter"].(map[string]interface{}); ok {
				metadataFilter = scMetadataFilter
			}
			if scST, ok := searchConfig["similarity_threshold"].(float64); ok {
				similarityThreshold = scST
			}
			if scVSW, ok := searchConfig["vector_similarity_weight"].(float64); ok {
				vectorSimilarityWeight = scVSW
			}
			if scTopK, ok := searchConfig["top_k"].(float64); ok {
				topK = int(scTopK)
				if topK < 1 {
					topK = 1
				} else if topK > 2048 {
					topK = 2048
				}
			}
			if scUseKG, ok := searchConfig["use_kg"].(bool); ok {
				useKG = scUseKG
			}
			if scLangs, ok := searchConfig["cross_languages"].([]interface{}); ok {
				crossLanguages = make([]string, len(scLangs))
				for i, l := range scLangs {
					if s, ok := l.(string); ok {
						crossLanguages[i] = s
					}
				}
			}
			if scKeyword, ok := searchConfig["keyword"].(bool); ok {
				keyword = scKeyword
			}
			if scRerankID, ok := searchConfig["rerank_id"].(string); ok {
				rerankID = scRerankID
			}
			chatID, _ = searchConfig["chat_id"].(string)
		} else {
			common.Warn("Invalid search_id: search_config missing or invalid", zap.String("searchID", searchID))
			return nil, fmt.Errorf("Invalid search_id")
		}
	}

	// If meta_data_filter method is auto/semi_auto, get chat model
	var chatModelForFilter *modelModule.ChatModel
	if metadataFilter != nil {
		method, _ := metadataFilter["method"].(string)
		if method == "auto" || method == "semi_auto" {
			if chatID != "" {
				driver, modelName, apiConfig, _, err := modelProviderSvc.ResolveModelConfig(tenantIDs[0], entity.ModelTypeChat, chatID)
				if err != nil {
					common.Warn("Failed to get chat model config from search_config chat_id, using tenant default", zap.String("chatID", chatID), zap.Error(err))
				} else {
					chatModelForFilter = modelModule.NewChatModel(driver, &modelName, apiConfig)
				}
			}

			if chatModelForFilter == nil {
				driver, modelName, apiConfig, _, err := modelProviderSvc.GetTenantDefaultModelByType(tenantIDs[0], entity.ModelTypeChat)
				if err != nil {
					common.Warn("Failed to get tenant default chat model for meta_data_filter", zap.Error(err))
				} else {
					chatModelForFilter = modelModule.NewChatModel(driver, &modelName, apiConfig)
				}
			}
		}
	}

	// Apply meta_data_filter to get filtered doc_ids
	docIDs := make([]string, len(req.DocIDs))
	copy(docIDs, req.DocIDs)
	if len(metadataFilter) > 0 {
		metadataSvc := service.NewMetadataService()
		flattedMeta, err := metadataSvc.GetFlattedMetaByKBs(datasetIDs)
		if err != nil {
			common.Warn("Failed to get flatted metadata, using empty metadata for filter", zap.Error(err))
			flattedMeta = make(common.MetaData)
		}
		filteredDocIDs, _ := service.ApplyMetaDataFilter(ctx, metadataFilter, flattedMeta, question, chatModelForFilter, req.DocIDs, datasetIDs)
		docIDs = filteredDocIDs
	}

	// Apply cross_languages and keyword extraction
	modifiedQuestion := question
	if len(crossLanguages) > 0 {
		translated, err := service.CrossLanguages(ctx, tenantIDs[0], "", question, crossLanguages)
		if err != nil {
			common.Warn("Failed to translate question", zap.String("llmID", ""), zap.Error(err))
		} else {
			modifiedQuestion = translated
		}
	}
	if keyword {
		driver, modelName, apiConfig, _, err := modelProviderSvc.GetTenantDefaultModelByType(tenantIDs[0], entity.ModelTypeChat)
		if err != nil {
			common.Warn("Failed to get default chat model for LLM transformations", zap.Error(err))
		} else {
			chatModel := modelModule.NewChatModel(driver, &modelName, apiConfig)
			extractedKeywords, err := service.KeywordExtraction(ctx, chatModel, modifiedQuestion, 3)
			if err != nil {
				common.Warn("Failed to extract keywords from question", zap.Error(err))
			} else if extractedKeywords != "" {
				modifiedQuestion = modifiedQuestion + extractedKeywords
			}
		}
	}

	// Get tag-based rank features via LabelQuestion
	metadataSvc := service.NewMetadataService()
	labels := metadataSvc.LabelQuestion(modifiedQuestion, kbRecords)

	// Determine embedding model
	var embeddingModel *modelModule.EmbeddingModel
	if kbRecords[0].EmbdID != "" {
		driver, modelName, apiConfig, maxTokens, embErr := modelProviderSvc.ResolveModelConfig(tenantIDs[0], entity.ModelTypeEmbedding, kbRecords[0].EmbdID)
		if embErr != nil {
			return nil, fmt.Errorf("failed to get embedding model by embd_id: %w", embErr)
		}
		embeddingModel = modelModule.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
	}

	// Get rerank model if rerankID is specified
	var rerankModel *modelModule.RerankModel
	if rerankID != "" {
		driver, modelName, apiConfig, _, rErr := modelProviderSvc.ResolveModelConfig(tenantIDs[0], entity.ModelTypeRerank, rerankID)
		if rErr != nil {
			return nil, fmt.Errorf("failed to get rerank model by rerank_id: %w", rErr)
		}
		rerankModel = modelModule.NewRerankModel(driver, &modelName, apiConfig)
	}

	retrievalReq := &nlp.RetrievalRequest{
		TenantIDs:              tenantIDs,
		Question:               modifiedQuestion,
		KbIDs:                  datasetIDs,
		DocIDs:                 docIDs,
		Page:                   page,
		PageSize:               pageSize,
		Top:                    &topK,
		SimilarityThreshold:    &similarityThreshold,
		VectorSimilarityWeight: &vectorSimilarityWeight,
		RerankModel:            rerankModel,
		RankFeature:            &labels,
		EmbeddingModel:         embeddingModel,
	}

	retrievalResult, err := nlp.NewRetrievalService(d.docEngine, d.documentDAO).Retrieval(ctx, retrievalReq)
	if err != nil {
		return nil, fmt.Errorf("retrieval search failed: %w", err)
	}

	filteredChunks := retrievalResult.Chunks

	if useKG {
		common.Warn("use_kg is not yet implemented in Go - skipping KG retrieval")
	}

	filteredChunks = nlp.RetrievalByChildren(filteredChunks, tenantIDs, d.docEngine, ctx)

	for i := range filteredChunks {
		delete(filteredChunks[i], "vector")
	}

	common.Info("SearchDatasets completed", zap.String("userID", userID), zap.Any("kbID", datasetIDs), zap.String("question", question), zap.Int64("chunkCount", int64(len(filteredChunks))))

	pyChunks := common.ConvertFloatsToPyFormat(filteredChunks).([]map[string]interface{})

	return &service.SearchDatasetsResponse{
		Chunks:  pyChunks,
		DocAggs: retrievalResult.DocAggs,
		Labels:  &labels,
		Total:   retrievalResult.Total,
	}, nil
}
