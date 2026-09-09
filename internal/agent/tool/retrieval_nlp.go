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

// retrieval_nlp.go — NLPRetrievalAdapter wiring.
//
// The agent tool layer (tool/retrieval_service.go) declares a
// minimal RetrievalService interface. Until this file landed, the
// only registered implementation was the stub that returns
// ErrRetrievalServiceMissing. NLPRetrievalAdapter bridges the
// agent-side interface to the production nlp.RetrievalService —
// the same service that powers chat / dataset search / chunk
// retrieval across the rest of the codebase.
//
// Translation rules:
//
//   tool.RetrievalRequest.Query      → nlp.RetrievalRequest.Question
//   tool.RetrievalRequest.DatasetIDs → nlp.RetrievalRequest.KbIDs
//   tool.RetrievalRequest.TopN       → nlp.RetrievalRequest.PageSize
//   tool.RetrievalRequest.TopK       → nlp.RetrievalRequest.KNNTopK
//                                       (fallback KNNTopK=TopN*4 so rerank
//                                        has headroom)
//   tool.RetrievalRequest.KeywordsSimilarityWeight
//                                    → nlp.RetrievalRequest.VectorSimilarityWeight
//                                       as 1-keyword weight
//   resolved knowledge-base model    → nlp.RetrievalRequest.EmbeddingModel
//   tool.RetrievalRequest.RerankID   → nlp.RetrievalRequest.RerankModel
//   tool.RetrievalRequest.UseKG      → ErrGraphRAGNotSupported (out of
//                                       scope for the Go Agent tool)
//
// Chunk shape translation: nlp's Chunks are []map[string]any with
// keys chunk_id, doc_id, docnm_kwd, content_with_weight,
// content_ltks, similarity, term_similarity, vector_similarity. The
// tool side wants a flat RetrievalChunk with the fields needed for both
// display and the frontend reference strip. We pick the most user-facing fields:
//   - ID         ← chunk_id
//   - Content    ← content_with_weight (fallback to content_ltks)
//   - DocumentID ← doc_id
//   - DocumentName ← docnm_kwd
//   - DatasetID  ← kb_id
//   - ImageID    ← image_id/img_id
//   - Positions  ← positions/position_int
//   - Score      ← similarity (fallback to avg of term+vector)
//
// Defensive defaults: missing or wrong-typed chunk fields become
// empty strings / 0.0 rather than panicking — a single malformed
// chunk from the doc engine shouldn't take down the whole
// retrieval call.

package tool

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service/nlp"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

var retrievalUserPrefixPattern = regexp.MustCompile(`(?i)^user[:：\s]*`)

// NLPRetrievalAdapter wraps *nlp.RetrievalService behind the
// agent-tool RetrievalService interface. The adapter is safe to
// share across goroutines — the wrapped service is stateless
// beyond its docEngine + documentDAO handles, both of which the
// nlp package treats as concurrent-safe.
type NLPRetrievalAdapter struct {
	svc           *nlp.RetrievalService
	kbDAO         knowledgebaseLookup
	modelResolver modelResolver
	enhancer      retrievalEnhancer
}

type knowledgebaseLookup interface {
	GetByIDs(ctx context.Context, sqlDB *gorm.DB, ids []string) ([]*entity.Knowledgebase, error)
	GetByName(ctx context.Context, sqlDB *gorm.DB, name, tenantID string) (*entity.Knowledgebase, error)
}

// modelResolver is the narrow model-provider surface needed by retrieval.
// Keeping this interface in the tool package avoids importing the parent
// internal/service package, which already depends on agent/tool.
type modelResolver interface {
	GetModelConfigByID(
		ctx context.Context,
		tenantID string,
		modelType entity.ModelType,
		modelID string,
	) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error)
	ResolveModelConfig(
		ctx context.Context,
		tenantID string,
		modelType entity.ModelType,
		modelRef string,
	) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error)
	GetTenantDefaultModelByType(
		ctx context.Context,
		tenantID string,
		modelType entity.ModelType,
	) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error)
}

// retrievalEnhancer exposes the service-layer query and result enhancements
// without making agent/tool import the parent internal/service package.
type retrievalEnhancer interface {
	CrossLanguages(
		ctx context.Context,
		tenantID, query string,
		languages []string,
	) (string, error)
	FilterDocuments(
		ctx context.Context,
		filter map[string]any,
		query string,
		chatModel *modelModule.ChatModel,
		baseDocIDs []string,
		kbIDs []string,
	) ([]string, error)
	LabelQuestion(
		ctx context.Context,
		question string,
		kbs []*entity.Knowledgebase,
	) map[string]float64
	EnhanceTOC(
		ctx context.Context,
		chatModel *modelModule.ChatModel,
		tenantIDs, kbIDs []string,
		question string,
		topN int,
		chunks []map[string]any,
	) ([]map[string]any, error)
	RetrieveByChildren(
		ctx context.Context,
		chunks []map[string]any,
		tenantIDs []string,
	) []map[string]any
}

// NewNLPRetrievalAdapter wraps an already-constructed
// *nlp.RetrievalService.
func NewNLPRetrievalAdapter(
	svc *nlp.RetrievalService,
	resolver modelResolver,
	enhancer retrievalEnhancer,
) *NLPRetrievalAdapter {
	return &NLPRetrievalAdapter{
		svc:           svc,
		kbDAO:         dao.NewKnowledgebaseDAO(),
		modelResolver: resolver,
		enhancer:      enhancer,
	}
}

// NewNLPRetrievalAdapterFromDeps is the convenience constructor
// for the common boot path:
//
// The boot path supplies the model provider and service enhancement bridge so
// the adapter can build a complete hybrid-retrieval request.
func NewNLPRetrievalAdapterFromDeps(
	docEngine engine.DocEngine,
	documentDAO *dao.DocumentDAO,
	resolver modelResolver,
	enhancer retrievalEnhancer,
) *NLPRetrievalAdapter {
	return &NLPRetrievalAdapter{
		svc:           nlp.NewRetrievalService(docEngine, documentDAO),
		kbDAO:         dao.NewKnowledgebaseDAO(),
		modelResolver: resolver,
		enhancer:      enhancer,
	}
}

// Search implements RetrievalService. The translation rules live
// at the top of this file.
func (a *NLPRetrievalAdapter) Search(ctx context.Context, db *gorm.DB, req RetrievalRequest) ([]RetrievalChunk, error) {
	if a == nil || a.svc == nil {
		return nil, ErrRetrievalServiceMissing
	}
	if req.UseKG {
		// Keep direct adapter callers consistent with RetrievalTool.
		return nil, ErrGraphRAGNotSupported
	}
	if req.Query == "" {
		return nil, nil
	}
	topN := req.TopN
	if topN <= 0 {
		topN = 8
	}

	datasets, err := a.resolveDatasets(ctx, db, req)
	if err != nil {
		return nil, err
	}
	if len(datasets.tenantIDs) != 1 {
		return nil, fmt.Errorf("retrieval: datasets span multiple tenants")
	}
	if err := validateEmbeddingModels(ctx, db, datasets.kbs); err != nil {
		return nil, err
	}
	embeddingModel, err := a.resolveEmbeddingModel(ctx, datasets.kbs[0])
	if err != nil {
		return nil, err
	}

	chatModel, err := a.resolveChatModel(ctx, req, datasets.kbs[0].TenantID)
	if err != nil {
		return nil, err
	}
	query := req.Query
	docIDs := compactStrings(req.DocScope)
	if len(req.MetaDataFilter) > 0 {
		if a.enhancer == nil {
			return nil, fmt.Errorf("retrieval: metadata filter service is not configured")
		}
		docIDs, err = a.enhancer.FilterDocuments(
			ctx, req.MetaDataFilter, query, chatModel, docIDs, datasets.kbIDs,
		)
		if err != nil {
			return nil, fmt.Errorf("retrieval: filter documents: %w", err)
		}
	}
	if len(req.CrossLanguages) > 0 {
		if a.enhancer == nil {
			return nil, fmt.Errorf("retrieval: cross-language service is not configured")
		}
		translated, translateErr := a.enhancer.CrossLanguages(
			ctx, datasets.kbs[0].TenantID, query, req.CrossLanguages,
		)
		if translateErr != nil {
			common.Warn("agent retrieval: cross-language query failed; using original query", zap.Error(translateErr))
		} else if strings.TrimSpace(translated) != "" {
			query = translated
		}
	}
	query = retrievalUserPrefixPattern.ReplaceAllString(query, "")
	var rankFeature map[string]float64
	if a.enhancer != nil {
		rankFeature = a.enhancer.LabelQuestion(ctx, query, datasets.kbs)
	}
	rerankModel, err := a.resolveRerankModel(ctx, req, datasets.kbs[0].TenantID)
	if err != nil {
		return nil, err
	}
	preparedReq := req
	preparedReq.Query = query
	preparedReq.DocScope = docIDs
	preparedReq.DatasetIDs = append([]string(nil), datasets.kbIDs...)
	nlpReq := nlpRequestFromRetrieval(preparedReq, datasets.tenantIDs, topN, embeddingModel)
	nlpReq.RerankModel = rerankModel
	if rankFeature != nil {
		nlpReq.RankFeature = &rankFeature
	}

	res, err := a.svc.Retrieval(ctx, nlpReq)
	if err != nil {
		return nil, err
	}
	if res == nil || len(res.Chunks) == 0 {
		return []RetrievalChunk{}, nil
	}
	rawChunks := res.Chunks
	if req.TOCEnhance {
		if a.enhancer == nil {
			return nil, fmt.Errorf("retrieval: TOC enhancement service is not configured")
		}
		rawChunks, err = a.enhancer.EnhanceTOC(
			ctx,
			chatModel,
			datasets.tenantIDs,
			datasets.kbIDs,
			query,
			topN,
			rawChunks,
		)
		if err != nil {
			common.Warn("agent retrieval: TOC enhancement failed; using retrieval results", zap.Error(err))
			rawChunks = res.Chunks
		}
	}
	if a.enhancer != nil {
		rawChunks = a.enhancer.RetrieveByChildren(ctx, rawChunks, datasets.tenantIDs)
	}
	out := make([]RetrievalChunk, 0, len(rawChunks))
	for _, raw := range rawChunks {
		out = append(out, translateChunk(raw))
	}
	return out, nil
}

func nlpRequestFromRetrieval(
	req RetrievalRequest,
	tenantIDs []string,
	topN int,
	embeddingModel *modelModule.EmbeddingModel,
) *nlp.RetrievalRequest {
	nlpReq := &nlp.RetrievalRequest{
		Question:       req.Query,
		TenantIDs:      append([]string(nil), tenantIDs...),
		KbIDs:          append([]string(nil), req.DatasetIDs...),
		DocIDs:         append([]string(nil), compactStrings(req.DocScope)...),
		Page:           1,
		PageSize:       topN,
		EmbeddingModel: embeddingModel,
		Aggs:           boolPtr(false),
		Highlight:      boolPtr(false),
	}
	if req.RerankCandidatesCount != 0 {
		nlpReq.RerankCandidatesCount = &req.RerankCandidatesCount
	}
	if req.TopK > 0 {
		nlpReq.KNNTopK = &req.TopK
	} else if topN > 0 {
		rerankBudget := topN * 4
		nlpReq.KNNTopK = &rerankBudget
	}
	if req.SimilarityThreshold != nil {
		nlpReq.SimilarityThreshold = req.SimilarityThreshold
	}
	if req.KeywordsSimilarityWeight != nil {
		vectorSimilarityWeight := 1 - *req.KeywordsSimilarityWeight
		nlpReq.VectorSimilarityWeight = &vectorSimilarityWeight
	}
	return nlpReq
}

type resolvedDatasets struct {
	kbs       []*entity.Knowledgebase
	kbIDs     []string
	tenantIDs []string
}

func (a *NLPRetrievalAdapter) resolveDatasets(
	ctx context.Context,
	db *gorm.DB,
	req RetrievalRequest,
) (*resolvedDatasets, error) {
	seen := map[string]struct{}{}
	tenantIDs := make([]string, 0, 1)
	appendTenantID := func(tenantID string) {
		tenantID = strings.TrimSpace(tenantID)
		if tenantID == "" {
			return
		}
		if _, ok := seen[tenantID]; ok {
			return
		}
		seen[tenantID] = struct{}{}
		tenantIDs = append(tenantIDs, tenantID)
	}

	datasetIDs := compactStrings(req.DatasetIDs)
	if len(datasetIDs) == 0 {
		return nil, fmt.Errorf("retrieval: dataset_ids is required")
	}
	if a == nil || a.kbDAO == nil {
		return nil, fmt.Errorf("retrieval: knowledge base lookup is not configured")
	}

	foundKBs, err := a.kbDAO.GetByIDs(ctx, db, datasetIDs)
	if err != nil {
		return nil, fmt.Errorf("retrieval: resolve dataset tenants: %w", err)
	}
	kbsByID := make(map[string]*entity.Knowledgebase, len(foundKBs))
	for _, kb := range foundKBs {
		if kb != nil {
			kbsByID[kb.ID] = kb
		}
	}
	kbs := make([]*entity.Knowledgebase, 0, len(datasetIDs))
	resolvedKBIDs := make([]string, 0, len(datasetIDs))
	for _, datasetID := range datasetIDs {
		kb := kbsByID[datasetID]
		if kb == nil && strings.TrimSpace(req.TenantID) != "" {
			kb, err = a.kbDAO.GetByName(ctx, db, datasetID, req.TenantID)
			if err != nil && err != gorm.ErrRecordNotFound {
				return nil, fmt.Errorf("retrieval: resolve dataset %q by name: %w", datasetID, err)
			}
		}
		if kb == nil {
			return nil, fmt.Errorf("retrieval: dataset %q was not found", datasetID)
		}
		kbs = append(kbs, kb)
		resolvedKBIDs = append(resolvedKBIDs, kb.ID)
	}
	for _, kb := range kbs {
		if kb == nil {
			continue
		}
		appendTenantID(kb.TenantID)
	}
	if len(tenantIDs) == 0 {
		return nil, fmt.Errorf("retrieval: no valid knowledge bases found for dataset_ids %v", datasetIDs)
	}
	return &resolvedDatasets{kbs: kbs, kbIDs: resolvedKBIDs, tenantIDs: tenantIDs}, nil
}

func validateEmbeddingModels(ctx context.Context, db *gorm.DB, kbs []*entity.Knowledgebase) error {
	if len(kbs) == 0 {
		return fmt.Errorf("retrieval: no datasets selected")
	}
	for _, kb := range kbs {
		if kb == nil {
			return fmt.Errorf("retrieval: dataset record is nil")
		}
	}
	embdNameCache := make(map[string]string)
	firstKey := knowledgebaseEmbeddingKey(ctx, db, kbs[0], embdNameCache)
	for _, kb := range kbs[1:] {
		if knowledgebaseEmbeddingKey(ctx, db, kb, embdNameCache) != firstKey {
			return fmt.Errorf("retrieval: datasets use different embedding models")
		}
	}
	return nil
}

// knowledgebaseEmbeddingKey groups datasets by their resolved base embedding
// model name (e.g. "BAAI/bge-m3"), matching the chat/dataset-search
// validation, so datasets pointing at the same model through different
// provider instances or storage forms (tenant_model id vs legacy composite
// name) retrieve together.
func knowledgebaseEmbeddingKey(ctx context.Context, db *gorm.DB, kb *entity.Knowledgebase, cache map[string]string) string {
	if strings.TrimSpace(kb.EmbdID) == "" && (kb.TenantEmbdID == nil || strings.TrimSpace(*kb.TenantEmbdID) == "") {
		return "default:" + strings.TrimSpace(kb.TenantID)
	}
	return "embedding:" + dao.NewKnowledgebaseDAO().EmbeddingBaseName(ctx, db, kb, cache)
}

func (a *NLPRetrievalAdapter) resolveEmbeddingModel(
	ctx context.Context,
	kb *entity.Knowledgebase,
) (*modelModule.EmbeddingModel, error) {
	if a == nil || a.modelResolver == nil {
		return nil, fmt.Errorf("retrieval: embedding model resolver is not configured")
	}

	var (
		driver    modelModule.ModelDriver
		modelName string
		apiConfig *modelModule.APIConfig
		maxTokens int
		err       error
	)
	switch {
	case kb.TenantEmbdID != nil && strings.TrimSpace(*kb.TenantEmbdID) != "":
		driver, modelName, apiConfig, maxTokens, err = a.modelResolver.GetModelConfigByID(
			ctx, kb.TenantID, entity.ModelTypeEmbedding, *kb.TenantEmbdID,
		)
	case strings.TrimSpace(kb.EmbdID) != "":
		driver, modelName, apiConfig, maxTokens, err = a.modelResolver.ResolveModelConfig(
			ctx, kb.TenantID, entity.ModelTypeEmbedding, kb.EmbdID,
		)
	default:
		driver, modelName, apiConfig, maxTokens, err = a.modelResolver.GetTenantDefaultModelByType(
			ctx, kb.TenantID, entity.ModelTypeEmbedding,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("retrieval: resolve embedding model for dataset %s: %w", kb.ID, err)
	}
	return modelModule.NewEmbeddingModel(driver, &modelName, apiConfig, maxTokens), nil
}

func (a *NLPRetrievalAdapter) resolveChatModel(
	ctx context.Context,
	req RetrievalRequest,
	tenantID string,
) (*modelModule.ChatModel, error) {
	method, _ := req.MetaDataFilter["method"].(string)
	needsChatModel := req.TOCEnhance || method == "auto" || method == "semi_auto"
	if !needsChatModel {
		return nil, nil
	}
	if a == nil || a.modelResolver == nil {
		return nil, fmt.Errorf("retrieval: model resolver is not configured")
	}
	driver, modelName, apiConfig, _, err := a.modelResolver.GetTenantDefaultModelByType(
		ctx, tenantID, entity.ModelTypeChat,
	)
	if err != nil {
		return nil, fmt.Errorf("retrieval: resolve default chat model: %w", err)
	}
	return modelModule.NewChatModel(driver, &modelName, apiConfig), nil
}

func (a *NLPRetrievalAdapter) resolveRerankModel(
	ctx context.Context,
	req RetrievalRequest,
	tenantID string,
) (*modelModule.RerankModel, error) {
	if req.RerankID == "" {
		return nil, nil
	}
	if a == nil || a.modelResolver == nil {
		return nil, fmt.Errorf("retrieval: model resolver is not configured")
	}
	var (
		driver    modelModule.ModelDriver
		modelName string
		apiConfig *modelModule.APIConfig
		err       error
	)
	driver, modelName, apiConfig, _, err = a.modelResolver.ResolveModelConfig(
		ctx, tenantID, entity.ModelTypeRerank, req.RerankID,
	)
	if err != nil {
		return nil, fmt.Errorf("retrieval: resolve rerank model: %w", err)
	}
	return modelModule.NewRerankModel(driver, &modelName, apiConfig), nil
}

// translateChunk converts one nlp chunk map into a RetrievalChunk.
// Tolerates missing fields (returns zero values) and wrong types
// (returns zero values) so a single bad chunk from the doc engine
// can't break the whole result list.
func translateChunk(raw map[string]any) RetrievalChunk {
	return RetrievalChunk{
		ID:               stringFromMap(raw, "chunk_id"),
		Content:          contentFromMap(raw),
		DocumentID:       stringFromMap(raw, "doc_id"),
		DocumentName:     stringFromMap(raw, "docnm_kwd"),
		DatasetID:        stringFromMap(raw, "kb_id"),
		ImageID:          firstStringFromMap(raw, "image_id", "img_id"),
		URL:              firstStringFromMap(raw, "url", "document_url", "doc_url"),
		Positions:        firstValueFromMap(raw, "positions", "position_int"),
		Score:            scoreFromMap(raw),
		TermSimilarity:   scoreValueFromMap(raw, "term_similarity"),
		VectorSimilarity: scoreValueFromMap(raw, "vector_similarity"),
	}
}

// stringFromMap returns raw[key].(string) or "" if missing / wrong
// type. Keeps the translator compact.
func stringFromMap(raw map[string]any, key string) string {
	if v, ok := raw[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func firstStringFromMap(raw map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := stringFromMap(raw, key); value != "" {
			return value
		}
	}
	return ""
}

func firstValueFromMap(raw map[string]any, keys ...string) any {
	for _, key := range keys {
		if value, ok := raw[key]; ok && value != nil {
			return value
		}
	}
	return nil
}

// contentFromMap picks the most user-facing content field. nlp
// chunks carry content_with_weight (the highlightable string) and
// content_ltks (the tokenised form). content_with_weight is what
// the model sees in Python; we use it here too. Empty / missing →
// fall back to content_ltks; both empty → empty string.
func contentFromMap(raw map[string]any) string {
	if v := stringFromMap(raw, "content_with_weight"); v != "" {
		return v
	}
	return stringFromMap(raw, "content_ltks")
}

// scoreFromMap returns the chunk's similarity score. nlp populates
// three fields — similarity (combined), term_similarity (BM25),
// vector_similarity (cosine). We prefer similarity; if absent or
// zero, average the two sub-scores. Wrong-type values → fall through
// to sub-scores; missing sub-scores → 0.
func scoreFromMap(raw map[string]any) float64 {
	if f, ok := numberFromMap(raw, "similarity"); ok {
		return f
	}
	term, termOK := numberFromMap(raw, "term_similarity")
	vec, vecOK := numberFromMap(raw, "vector_similarity")
	if termOK && vecOK {
		return (term + vec) / 2
	}
	if termOK {
		return term
	}
	if vecOK {
		return vec
	}
	return 0
}

func scoreValueFromMap(raw map[string]any, key string) float64 {
	value, _ := numberFromMap(raw, key)
	return value
}

// numberFromMap returns raw[key].(float64) with a tolerant path
// for ints. JSON unmarshaling can produce either.
func numberFromMap(raw map[string]any, key string) (float64, bool) {
	v, ok := raw[key]
	if !ok {
		return 0, false
	}
	switch x := v.(type) {
	case float64:
		return x, true
	case float32:
		return float64(x), true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	}
	return 0, false
}

func boolPtr(b bool) *bool { return &b }
