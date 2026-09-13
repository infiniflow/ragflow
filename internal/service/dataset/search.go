package dataset

import (
	"context"
	"fmt"
	"ragflow/internal/dao"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"
	"ragflow/internal/service/nlp"
)

func (d *DatasetService) SearchDataset(ctx context.Context, datasetID, userID string, req *service.SearchDatasetRequest) (*service.SearchDatasetsResponse, error) {
	if datasetID == "" {
		return nil, fmt.Errorf("dataset_id is required")
	}
	return d.SearchDatasets(ctx, req.ToSearchDatasetsRequest(datasetID), userID)
}

// searchRunConfig holds the effective retrieval parameters after the
// request defaults are applied.
type searchRunConfig struct {
	rerankCandidatesCount  int
	metadataFilter         map[string]interface{}
	similarityThreshold    float64
	vectorSimilarityWeight float64
	knnTopK                int
	useKG                  bool
	crossLanguages         []string
	keyword                bool
	rerankID               string
	chatID                 string
}

// applySavedSearchConfig fills only the fields the request left unset from a
// saved search app's search_config. It mirrors the Python API's merge
// (req = {**search_config, **req}), where fields the caller sets explicitly
// always win over the saved configuration.
func (c *searchRunConfig) applySavedSearchConfig(req *service.SearchDatasetsRequest, searchConfig map[string]interface{}) {
	// Python: search_config.setdefault("rerank_candidates_count", 100) - the
	// saved-config default only applies when the request did not set it.
	if req.RerankCandidatesCount == nil {
		c.rerankCandidatesCount = 100
		if sc, ok := common.GetInt(searchConfig["rerank_candidates_count"]); ok {
			c.rerankCandidatesCount = sc
		}
	}
	if req.MetadataFilter == nil && req.MetadataCondition == nil {
		if sc, ok := searchConfig["meta_data_filter"].(map[string]interface{}); ok {
			c.metadataFilter = sc
		}
	}
	if req.SimilarityThreshold == nil {
		if sc, ok := searchConfig["similarity_threshold"].(float64); ok {
			c.similarityThreshold = sc
		}
	}
	if req.VectorSimilarityWeight == nil {
		if sc, ok := searchConfig["vector_similarity_weight"].(float64); ok {
			c.vectorSimilarityWeight = sc
		}
	}
	if req.KNNTopK == nil && req.TopK == nil {
		if sc, ok := searchConfig["top_k"].(float64); ok {
			c.knnTopK = int(sc)
			if c.knnTopK < 1 {
				c.knnTopK = 1
			} else if c.knnTopK > 2048 {
				c.knnTopK = 2048
			}
		}
	}
	if req.UseKG == nil {
		if sc, ok := searchConfig["use_kg"].(bool); ok {
			c.useKG = sc
		}
	}
	if req.CrossLanguages == nil {
		if sc, ok := searchConfig["cross_languages"].([]interface{}); ok {
			langs := make([]string, len(sc))
			for i, l := range sc {
				if str, ok := l.(string); ok {
					langs[i] = str
				}
			}
			c.crossLanguages = langs
		}
	}
	if req.Keyword == nil {
		if sc, ok := searchConfig["keyword"].(bool); ok {
			c.keyword = sc
		}
	}
	if req.RerankID == nil {
		if sc, ok := searchConfig["rerank_id"].(string); ok {
			c.rerankID = sc
		}
	}
	c.chatID, _ = searchConfig["chat_id"].(string)
}

func (d *DatasetService) SearchDatasets(ctx context.Context, req *service.SearchDatasetsRequest, userID string) (*service.SearchDatasetsResponse, error) {
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
	if req.PageSize != nil {
		pageSize = *req.PageSize
	} else if req.Size != nil {
		pageSize = *req.Size
	}
	rerankCandidatesCount := 64
	if req.RerankCandidatesCount != nil {
		rerankCandidatesCount = *req.RerankCandidatesCount
	}
	useKG := false
	if req.UseKG != nil {
		useKG = *req.UseKG
	}
	similarityThreshold := 0.2
	if req.SimilarityThreshold != nil {
		similarityThreshold = *req.SimilarityThreshold
	}
	vectorSimilarityWeight := 0.3
	if req.VectorSimilarityWeight != nil {
		vectorSimilarityWeight = *req.VectorSimilarityWeight
	}
	knnTopK := 1024
	if req.KNNTopK != nil {
		knnTopK = *req.KNNTopK
	} else if req.TopK != nil {
		knnTopK = *req.TopK
	}
	if knnTopK < 1 {
		knnTopK = 1
	} else if knnTopK > 2048 {
		knnTopK = 2048
	}
	knnNumCandidates := 2048
	if req.KNNNumCandidates != nil {
		knnNumCandidates = *req.KNNNumCandidates
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
	hasMetadataCondition := req.MetadataCondition != nil
	if req.MetadataCondition != nil {
		manual := make([]interface{}, 0)
		if conditions, ok := req.MetadataCondition["conditions"].([]interface{}); ok {
			for _, item := range conditions {
				if condition, ok := item.(map[string]interface{}); ok {
					manual = append(manual, map[string]interface{}{"key": condition["name"], "op": condition["comparison_operator"], "value": condition["value"]})
				}
			}
		}
		metadataFilter = map[string]interface{}{"method": "manual", "logic": req.MetadataCondition["logic"], "manual": manual}
	}
	crossLanguages := req.CrossLanguages
	documentIDs := req.DocumentIDs
	if documentIDs == nil {
		documentIDs = req.DocIDs
	}

	modelProviderSvc := service.NewModelProviderService()

	// Access check for all datasets
	var tenantIDs []string
	var kbRecords []*entity.Knowledgebase
	seenTenants := make(map[string]bool)
	for _, datasetID := range datasetIDs {
		if !d.kbDAO.Accessible(ctx, dao.DB, datasetID, userID) {
			common.Warn("SearchDatasets access denied", zap.String("datasetID", datasetID), zap.String("userID", userID))
			return nil, fmt.Errorf("only owner of dataset %s is authorized for this operation", datasetID)
		}

		kb, err := d.kbDAO.GetByID(ctx, dao.DB, datasetID)
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
	if err := service.ValidateDatasetEmbeddingModels(ctx, dao.DB, kbRecords); err != nil {
		return nil, err
	}

	// Override request fields with values from saved search config
	var chatID string
	if searchID != "" {
		if d.searchService == nil {
			common.Warn("Search service is not initialized for search_id", zap.String("searchID", searchID))
			return nil, fmt.Errorf("invalid search_id")
		}
		searchDetail, err := d.searchService.GetDetail(ctx, searchID)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			common.Warn("Invalid search_id", zap.String("searchID", searchID), zap.Error(err))
			return nil, fmt.Errorf("invalid search_id")
		}
		if searchDetail == nil || len(searchDetail) == 0 {
			common.Warn("Invalid search_id", zap.String("searchID", searchID))
			return nil, fmt.Errorf("invalid search_id")
		}
		searchTenantID, ok := searchDetail["tenant_id"].(string)
		if !ok || searchTenantID != userID {
			common.Warn("Invalid search_id", zap.String("searchID", searchID))
			return nil, fmt.Errorf("invalid search_id")
		}

		if searchConfig, ok := searchDetail["search_config"].(map[string]interface{}); ok && searchConfig != nil {
			rc := searchRunConfig{
				rerankCandidatesCount:  rerankCandidatesCount,
				metadataFilter:         metadataFilter,
				similarityThreshold:    similarityThreshold,
				vectorSimilarityWeight: vectorSimilarityWeight,
				knnTopK:                knnTopK,
				useKG:                  useKG,
				crossLanguages:         crossLanguages,
				keyword:                keyword,
				rerankID:               rerankID,
			}
			rc.applySavedSearchConfig(req, searchConfig)
			rerankCandidatesCount = rc.rerankCandidatesCount
			metadataFilter = rc.metadataFilter
			similarityThreshold = rc.similarityThreshold
			vectorSimilarityWeight = rc.vectorSimilarityWeight
			knnTopK = rc.knnTopK
			useKG = rc.useKG
			crossLanguages = rc.crossLanguages
			keyword = rc.keyword
			rerankID = rc.rerankID
			chatID = rc.chatID
		} else {
			common.Warn("Invalid search_id: search_config missing or invalid", zap.String("searchID", searchID))
			return nil, fmt.Errorf("invalid search_id")
		}
	}
	knnNumCandidates = max(knnNumCandidates, knnTopK)

	// If meta_data_filter method is auto/semi_auto, get chat model
	var chatModelForFilter *modelModule.ChatModel
	if metadataFilter != nil {
		method, _ := metadataFilter["method"].(string)
		if method == "auto" || method == "semi_auto" {
			if chatID != "" {
				driver, modelName, apiConfig, _, err := modelProviderSvc.ResolveModelConfig(ctx, tenantIDs[0], entity.ModelTypeChat, chatID)
				if err != nil {
					common.Warn("Failed to get chat model config from search_config chat_id, using tenant default", zap.String("chatID", chatID), zap.Error(err))
				} else {
					chatModelForFilter = modelModule.NewChatModel(driver, &modelName, apiConfig)
				}
			}

			if chatModelForFilter == nil {
				driver, modelName, apiConfig, _, err := modelProviderSvc.GetTenantDefaultModelByType(ctx, tenantIDs[0], entity.ModelTypeChat)
				if err != nil {
					common.Warn("Failed to get tenant default chat model for meta_data_filter", zap.Error(err))
				} else {
					chatModelForFilter = modelModule.NewChatModel(driver, &modelName, apiConfig)
				}
			}
		}
	}

	// Apply meta_data_filter to get filtered doc_ids
	docIDs := make([]string, len(documentIDs))
	copy(docIDs, documentIDs)
	if len(metadataFilter) > 0 {
		metadataSvc := service.NewMetadataService()
		flattedMeta, err := metadataSvc.GetFlattedMetaByKBs(ctx, datasetIDs)
		if err != nil {
			common.Warn("Failed to get flatted metadata, using empty metadata for filter", zap.Error(err))
			flattedMeta = make(common.MetaData)
		}
		if hasMetadataCondition {
			filteredDocIDs, _ := service.ApplyMetaDataFilter(ctx, metadataFilter, flattedMeta, question, chatModelForFilter, documentIDs, datasetIDs)
			docIDs = filteredDocIDs
		} else {
			filteredDocIDs, filterReturnedEmpty := service.ApplyMetaDataFilter(ctx, metadataFilter, flattedMeta, question, chatModelForFilter, nil, datasetIDs)
			if !filterReturnedEmpty {
				docIDs = append(docIDs, filteredDocIDs...)
			}
		}
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
		driver, modelName, apiConfig, _, err := modelProviderSvc.GetTenantDefaultModelByType(ctx, tenantIDs[0], entity.ModelTypeChat)
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
	labels := metadataSvc.LabelQuestion(ctx, modifiedQuestion, kbRecords)

	// Determine embedding model
	var embeddingModel *modelModule.EmbeddingModel
	if kbRecords[0].EmbdID != "" {
		driver, modelName, apiConfig, maxTokens, embErr := modelProviderSvc.ResolveModelConfig(ctx, tenantIDs[0], entity.ModelTypeEmbedding, kbRecords[0].EmbdID)
		if embErr != nil {
			return nil, fmt.Errorf("failed to get embedding model by embd_id: %w", embErr)
		}
		embeddingModel = modelModule.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens)
	}

	// Get rerank model if rerankID is specified
	var rerankModel *modelModule.RerankModel
	if rerankID != "" {
		driver, modelName, apiConfig, maxTokens, rErr := modelProviderSvc.ResolveModelConfig(ctx, tenantIDs[0], entity.ModelTypeRerank, rerankID)
		if rErr != nil {
			return nil, fmt.Errorf("failed to get rerank model by rerank_id: %w", rErr)
		}
		rerankModel = modelModule.NewRerankModel(driver, &modelName, apiConfig, maxTokens)
	}

	retrievalReq := &nlp.RetrievalRequest{
		TenantIDs:              tenantIDs,
		Question:               modifiedQuestion,
		KbIDs:                  datasetIDs,
		DocIDs:                 docIDs,
		Page:                   page,
		PageSize:               pageSize,
		RerankCandidatesCount:  &rerankCandidatesCount,
		KNNTopK:                &knnTopK,
		KNNNumCandidates:       &knnNumCandidates,
		SimilarityThreshold:    &similarityThreshold,
		VectorSimilarityWeight: &vectorSimilarityWeight,
		RerankModel:            rerankModel,
		RankFeature:            &labels,
		EmbeddingModel:         embeddingModel,
		Highlight:              req.Highlight,
	}
	if req.IncludeCompiledChunks != nil && !*req.IncludeCompiledChunks {
		retrievalReq.Filter = map[string]interface{}{"must_not": map[string]interface{}{"exists": "compile_kwd"}}
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

	keyMapping := map[string]string{
		"chunk_id":            "id",
		"content_with_weight": "content",
		"doc_id":              "document_id",
		"important_kwd":       "important_keywords",
		"question_kwd":        "questions",
		"docnm_kwd":           "document_keyword",
		"kb_id":               "dataset_id",
	}
	for i := range filteredChunks {
		delete(filteredChunks[i], "vector")
		for oldKey, newKey := range keyMapping {
			if value, ok := filteredChunks[i][oldKey]; ok {
				filteredChunks[i][newKey] = value
				delete(filteredChunks[i], oldKey)
			}
		}
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
