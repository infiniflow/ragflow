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

package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"
	"ragflow/internal/service/kg"
	"ragflow/internal/service/nlp"

	"github.com/gin-gonic/gin"
)

// --- Interfaces (for testability) ---

// KBServiceIface abstracts KnowledgebaseService for the Dify handler.
type KBServiceIface interface {
	GetByID(kbID string) (*entity.Knowledgebase, error)
	Accessible(kbID, userID string) bool
}

// ModelServiceIface abstracts ModelProviderService for the Dify handler.
type ModelServiceIface interface {
	GetEmbeddingModel(tenantID, embdID string) (*modelModule.EmbeddingModel, error)
	GetChatModel(tenantID, compositeModelName string) (*modelModule.ChatModel, error)
}

// MetadataServiceIface abstracts MetadataService for the Dify handler.
type MetadataServiceIface interface {
	GetFlattedMetaByKBs(kbIDs []string) (common.MetaData, error)
	LabelQuestion(question string, kbs []*entity.Knowledgebase) map[string]float64
}

// RetrievalServiceIface abstracts RetrievalService for the Dify handler.
type RetrievalServiceIface interface {
	Retrieval(ctx context.Context, req *nlp.RetrievalRequest) (*nlp.RetrievalResult, error)
}

// DocumentDAOIface abstracts DocumentDAO for the Dify handler.
type DocumentDAOIface interface {
	GetByIDs(ids []string) ([]*entity.Document, error)
}

// --- Request / Response types ---

// difyRetrievalRequest is the JSON body / query params for the Dify retrieval endpoint.
type difyRetrievalRequest struct {
	KnowledgeID       string                 `json:"knowledge_id" form:"knowledge_id"`
	Query             string                 `json:"query" form:"query"`
	UseKG             bool                   `json:"use_kg" form:"use_kg"`
	RetrievalSetting  *difyRetrievalSetting  `json:"retrieval_setting"`
	MetadataCondition *difyMetadataCondition `json:"metadata_condition"`
}

type difyRetrievalSetting struct {
	TopK           *int     `json:"top_k" form:"top_k"`
	ScoreThreshold *float64 `json:"score_threshold" form:"score_threshold"`
}

// difyCondition is a Dify-format metadata filter condition.
// Dify uses "name"/"comparison_operator" instead of MetaFilterCondition's "key"/"op".
type difyCondition struct {
	Name               string      `json:"name"`
	ComparisonOperator string      `json:"comparison_operator"`
	Value              interface{} `json:"value"`
}

type difyMetadataCondition struct {
	Conditions []difyCondition `json:"conditions"`
	Logic      string          `json:"logic"`
}

// toMetaFilterConditions converts Dify-format conditions to internal MetaFilterConditions.
func (c difyMetadataCondition) toMetaFilterConditions() []service.MetaFilterCondition {
	if len(c.Conditions) == 0 {
		return nil
	}
	result := make([]service.MetaFilterCondition, len(c.Conditions))
	for i, dc := range c.Conditions {
		v := ""
		if dc.Value != nil {
			v = fmt.Sprint(dc.Value)
		}
		result[i] = service.MetaFilterCondition{
			Key:   dc.Name,
			Op:    dc.ComparisonOperator,
			Value: v,
		}
	}
	return result
}

// difyRecord is one item in the response records array.
type difyRecord struct {
	Content  string                 `json:"content"`
	Score    float64                `json:"score"`
	Title    string                 `json:"title"`
	Metadata map[string]interface{} `json:"metadata"`
}

// --- Handler ---

// DifyRetrievalHandler handles Dify-compatible retrieval requests.
type DifyRetrievalHandler struct {
	kbSvc        KBServiceIface
	modelSvc     ModelServiceIface
	metadataSvc  MetadataServiceIface
	retrievalSvc RetrievalServiceIface
	docDAO       DocumentDAOIface
	docEngine    engine.DocEngine
}

// NewDifyRetrievalHandler creates a new DifyRetrievalHandler.
// The KG pipeline is created inline when use_kg=true to avoid injecting
// a pipeline that depends on per-request model configuration.
func NewDifyRetrievalHandler(
	kbSvc KBServiceIface,
	modelSvc ModelServiceIface,
	metadataSvc MetadataServiceIface,
	retrievalSvc RetrievalServiceIface,
	docDAO DocumentDAOIface,
	docEngine engine.DocEngine,
) *DifyRetrievalHandler {
	return &DifyRetrievalHandler{
		kbSvc:        kbSvc,
		modelSvc:     modelSvc,
		metadataSvc:  metadataSvc,
		retrievalSvc: retrievalSvc,
		docDAO:       docDAO,
		docEngine:    docEngine,
	}
}

// Retrieval handles POST/GET /api/v1/dify/retrieval.
// Matches Python: api/apps/restful_apis/dify_retrieval_api.py::retrieval()
func (h *DifyRetrievalHandler) Retrieval(c *gin.Context) {
	user, errCode, errMsg := GetUser(c)
	if errCode != common.CodeSuccess {
		c.JSON(http.StatusUnauthorized, gin.H{"code": errCode, "message": errMsg})
		return
	}

	var req difyRetrievalRequest
	if c.Request.Method == http.MethodGet {
		if err := c.ShouldBindQuery(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "message": "invalid query parameters"})
			return
		}
		// Manually extract top_k and score_threshold from query (flat params, not nested)
		if v := c.Query("top_k"); v != "" {
			if parsed, err := strconv.Atoi(v); err == nil {
				if req.RetrievalSetting == nil {
					req.RetrievalSetting = &difyRetrievalSetting{}
				}
				req.RetrievalSetting.TopK = &parsed
			}
		}
		if v := c.Query("score_threshold"); v != "" {
			if parsed, err := strconv.ParseFloat(v, 64); err == nil {
				if req.RetrievalSetting == nil {
					req.RetrievalSetting = &difyRetrievalSetting{}
				}
				req.RetrievalSetting.ScoreThreshold = &parsed
			}
		}
	} else {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "message": "invalid request body"})
			return
		}
	}

	if req.KnowledgeID == "" || req.Query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "message": "knowledge_id and query are required"})
		return
	}

	kb, err := h.kbSvc.GetByID(req.KnowledgeID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"code": common.CodeNotFound, "message": "Knowledgebase not found!"})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "message": "failed to query knowledgebase"})
		}
		return
	}

	if !h.kbSvc.Accessible(req.KnowledgeID, user.ID) {
		c.JSON(http.StatusUnauthorized, gin.H{"code": common.CodeAuthenticationError, "message": "No authorization."})
		return
	}

	// Parse retrieval options (nil means service uses defaults)
	var topK *int
	if req.RetrievalSetting != nil && req.RetrievalSetting.TopK != nil {
		topK = req.RetrievalSetting.TopK
	}
	var scoreThreshold *float64
	if req.RetrievalSetting != nil && req.RetrievalSetting.ScoreThreshold != nil {
		scoreThreshold = req.RetrievalSetting.ScoreThreshold
	}
	pageSize := 1024
	if topK != nil {
		pageSize = *topK
	}

	// Get embedding model
	embModel, err := h.modelSvc.GetEmbeddingModel(kb.TenantID, kb.EmbdID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "message": fmt.Sprintf("failed to get embedding model: %v", err)})
		return
	}

	// Metadata filter
	metas, metaErr := h.metadataSvc.GetFlattedMetaByKBs([]string{req.KnowledgeID})
	docIDs := make([]string, 0)
	if metaErr == nil && req.MetadataCondition != nil {
		logic := req.MetadataCondition.Logic
		if logic == "" {
			logic = "and"
		}
		filteredIDs := service.ApplyMetaFilter(metas, req.MetadataCondition.toMetaFilterConditions(), logic)
		docIDs = append(docIDs, filteredIDs...)
	}
	if len(docIDs) == 0 && req.MetadataCondition != nil {
		docIDs = []string{service.NoMatchDocIDSentinel}
	}

	// Label question for rank features
	kbs := []*entity.Knowledgebase{kb}
	rankFeature := h.metadataSvc.LabelQuestion(req.Query, kbs)

	// Chunk retrieval
	sr := &nlp.RetrievalRequest{
		Question:            req.Query,
		TenantIDs:           []string{kb.TenantID},
		KbIDs:               []string{req.KnowledgeID},
		DocIDs:              docIDs,
		Page:                1,
		PageSize:            pageSize,
		Top:                 topK,
		SimilarityThreshold: scoreThreshold,
		EmbeddingModel:      embModel,
	}
	if rankFeature != nil {
		sr.RankFeature = &rankFeature
	}

	result, err := h.retrievalSvc.Retrieval(c.Request.Context(), sr)
	if err != nil {
		if strings.Contains(err.Error(), "not_found") {
			c.JSON(http.StatusNotFound, gin.H{"code": common.CodeNotFound, "message": "No chunk found! Check the chunk status please!"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "message": err.Error()})
		return
	}

	// Enrich with child chunks
	chunks := nlp.RetrievalByChildren(result.Chunks, []string{kb.TenantID}, h.docEngine, c.Request.Context())

	// KG retrieval (optional)
	if req.UseKG {
		chatModel, kgErr := h.modelSvc.GetChatModel(kb.TenantID, "")
		if kgErr != nil {
			common.Warn("KG retrieval: failed to get chat model", zap.String("kbID", req.KnowledgeID), zap.Error(kgErr))
		} else if chatModel != nil {
			kgPipeline := kg.NewPipeline(
				h.docEngine,
				[]string{req.KnowledgeID},
				[]string{kb.TenantID},
				req.Query,
			)
			kgPipeline.SetChatModel(chatModel)
			kgPipeline.SetEmbModel(embModel)
			if kgResult, kgErr := kgPipeline.Retrieval(c.Request.Context()); kgErr == nil {
				if content, ok := kgResult["content_with_weight"].(string); ok && content != "" {
					chunks = append([]map[string]interface{}{kgResult}, chunks...)
				}
			}
		}
	}

	// Collect doc IDs and fetch documents
	docIDSet := make(map[string]struct{})
	for _, ch := range chunks {
		if docID, ok := ch["doc_id"].(string); ok && docID != "" {
			docIDSet[docID] = struct{}{}
		}
	}
	allDocIDs := make([]string, 0, len(docIDSet))
	for id := range docIDSet {
		allDocIDs = append(allDocIDs, id)
	}

	docMap := make(map[string]*entity.Document)
	if len(allDocIDs) > 0 {
		docs, err := h.docDAO.GetByIDs(allDocIDs)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "message": fmt.Sprintf("failed to load documents: %v", err)})
			return
		}
		for _, d := range docs {
			docMap[d.ID] = d
		}
	}

	// Build response
	records := make([]difyRecord, 0, len(chunks))
	for _, ch := range chunks {
		docID, _ := ch["doc_id"].(string)
		doc := docMap[docID]
		if doc == nil {
			continue
		}

		// Remove vector to reduce response size
		delete(ch, "vector")

		meta := make(map[string]interface{})
		if doc.MetaFields != nil {
			for k, v := range *doc.MetaFields {
				meta[k] = v
			}
		}
		meta["doc_id"] = docID
		meta["document_id"] = docID

		score, _ := ch["similarity"].(float64)
		title, _ := ch["docnm_kwd"].(string)
		content, _ := ch["content_with_weight"].(string)

		records = append(records, difyRecord{
			Content:  content,
			Score:    score,
			Title:    title,
			Metadata: meta,
		})
	}

	c.JSON(http.StatusOK, gin.H{"records": records})
}

// HealthCheck returns a simple health check response.
func (h *DifyRetrievalHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"code": 0, "data": true})
}
