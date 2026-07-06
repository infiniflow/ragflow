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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"go.uber.org/zap"
)

// SearchBotAskRequest is the request body for POST /api/v1/searchbots/ask.
type SearchBotAskRequest struct {
	Question string             `json:"question" binding:"required"`
	KbIDs    common.StringSlice `json:"kb_ids" binding:"required"`
	SearchID string             `json:"search_id,omitempty"`
}

// SearchBotMindMapRequest is the request body for POST /api/v1/searchbots/mindmap.
type SearchBotMindMapRequest struct {
	Question string             `json:"question" binding:"required"`
	KbIDs    common.StringSlice `json:"kb_ids" binding:"required"`
	SearchID string             `json:"search_id,omitempty"`
}

// SearchBotRetrievalTestRequest is the request body for POST /api/v1/searchbots/retrieval_test.
type SearchBotRetrievalTestRequest struct {
	KbIDs                  common.StringSlice     `json:"kb_ids" binding:"required"`
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
	// TODO: wire highlight to nlp Retrieval when engine supports highlightFields
	// Python: bot_api.py → retrieval(highlight=req.get("highlight"))
	//        → search.py highlightFields → ES get_highlight()
	// Issue: https://github.com/infiniflow/ragflow/issues/15712
	// Highlight           *bool                   `json:"highlight,omitempty"`
}

// UnmarshalJSON accepts both kb_id (Python API) and kb_ids (Go compatibility).
func (r *SearchBotRetrievalTestRequest) UnmarshalJSON(data []byte) error {
	type Alias SearchBotRetrievalTestRequest
	aux := struct {
		*Alias
		KbID common.StringSlice `json:"kb_id"`
	}{
		Alias: (*Alias)(r),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(r.KbIDs) == 0 && len(aux.KbID) > 0 {
		r.KbIDs = aux.KbID
	}
	return nil
}

// SearchBotRequest is the request body for POST /api/v1/searchbots/related_questions.
type SearchBotRequest struct {
	Question string `json:"question" binding:"required"`
	SearchID string `json:"search_id,omitempty"`
}

// SearchBotHandler handles searchbot endpoints:
//
//	POST /api/v1/searchbots/related_questions
//	POST /api/v1/searchbots/retrieval_test
//	POST /api/v1/searchbots/ask
//	POST /api/v1/searchbots/mindmap
type SearchBotHandler struct {
	searchSvc *service.SearchService
	tenantSvc *service.TenantService
	llm       *service.ModelProviderService
	streamLLM *service.ModelProviderService
	chunkSvc  service.Retriever
	askSvc    *service.AskService
	sseWriter SSEWriter
}

// NewSearchBotHandler creates a new SearchBotHandler.
func NewSearchBotHandler(searchSvc *service.SearchService, tenantSvc *service.TenantService, llm *service.ModelProviderService, chunkSvc service.Retriever) *SearchBotHandler {
	return &SearchBotHandler{searchSvc: searchSvc, tenantSvc: tenantSvc, llm: llm, chunkSvc: chunkSvc, sseWriter: &ginSSEWriter{}}
}

// SetStreamLLM sets the streaming LLM for the Ask endpoint.
func (h *SearchBotHandler) SetStreamLLM(llm *service.ModelProviderService) { h.streamLLM = llm }

// SetAskService sets the AskService used by the Ask endpoint.
func (h *SearchBotHandler) SetAskService(svc *service.AskService) { h.askSvc = svc }

// Handle generates related search questions based on a user query.
// @Summary Generate Related Questions
// @Description Generates 5-10 related search questions to expand the search scope.
// @Tags searchbots
// @Accept json
// @Produce json
// @Param request body SearchBotRequest true "Request body"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/searchbots/related_questions [post]
func (h *SearchBotHandler) Handle(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req SearchBotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}

	if req.Question == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "question is required")
		return
	}

	questions, err := service.GenerateRelatedQuestions(user.ID, req.Question, req.SearchID, h.searchSvc, h.tenantSvc, h.llm)
	if err != nil {
		common.Warn("searchbot related questions failed", zap.String("error", err.Error()))
		common.ResponseWithCodeData(c, common.CodeOperatingError, nil, "LLM call failed")
		return
	}

	common.SuccessWithData(c, questions, "success")
}

// RetrievalTest performs a retrieval test against specified knowledge bases.
// @Summary Retrieval Test
// @Description Test document retrieval across knowledge bases with optional filters, reranking, and KG search.
// @Tags searchbots
// @Accept json
// @Produce json
// @Param request body SearchBotRetrievalTestRequest true "Retrieval test parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/searchbots/retrieval_test [post]
func (h *SearchBotHandler) RetrievalTest(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ResponseWithHttpCodeData(c, http.StatusUnauthorized, errorCode, nil, errorMessage)
		return
	}

	var req SearchBotRetrievalTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}

	// Filter out empty strings from KbIDs before validation.
	filtered := make(common.StringSlice, 0, len(req.KbIDs))
	for _, id := range req.KbIDs {
		if strings.TrimSpace(id) != "" {
			filtered = append(filtered, id)
		}
	}
	req.KbIDs = filtered

	if len(req.KbIDs) == 0 || req.Question == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "kb_id and question are required")
		return
	}

	applyRetrievalDefaults(&req)

	if req.TopK != nil && *req.TopK <= 0 {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "top_k must be greater than 0")
		return
	}

	svcReq := toRetrievalServiceRequest(&req)

	result, err := h.chunkSvc.RetrievalTest(svcReq, user.ID)
	if err != nil {
		common.Warn("search bot retrieval test failed", zap.String("error", err.Error()))
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeServerError, nil, "retrieval test failed")
		return
	}

	common.SuccessWithData(c, result, "success")
}

// Ask performs a retrieval-augmented Q&A with streaming SSE response.
// @Summary Ask with Knowledge Bases
// @Description Retrieves chunks, builds prompt, and streams LLM answer with citations via SSE.
// @Tags searchbots
// @Accept json
// @Produce text/event-stream
// @Param request body SearchBotAskRequest true "Ask parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/searchbots/ask [post]
func (h *SearchBotHandler) Ask(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req SearchBotAskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}

	// Filter empty kb_ids.
	filtered := make(common.StringSlice, 0, len(req.KbIDs))
	for _, id := range req.KbIDs {
		if strings.TrimSpace(id) == "" {
			continue
		}
		filtered = append(filtered, id)
	}
	if len(filtered) == 0 || strings.TrimSpace(req.Question) == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "kb_ids and question are required")
		return
	}

	// Resolve chat model ID.
	modelID := ""
	if req.SearchID != "" && h.searchSvc != nil {
		if detail, err := h.searchSvc.GetDetail(req.SearchID); err == nil {
			if sc, ok := detail["search_config"].(map[string]interface{}); ok {
				if cid, ok := sc["chat_id"].(string); ok && cid != "" {
					modelID = cid
				}
			}
		}
	}
	if modelID == "" && h.tenantSvc != nil {
		defaultModel, err := h.tenantSvc.GetDefaultModelName(user.ID, entity.ModelTypeChat)
		if err == nil && defaultModel != "" {
			modelID = defaultModel
		}
	}
	if modelID == "" {
		h.sseWriter.Write(c, sseError("chat model not configured"))
		return
	}

	disableWriteDeadlineForSSE(c)
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	if h.askSvc == nil {
		h.sseWriter.Write(c, sseError("ask service not configured"))
		return
	}
	if h.streamLLM == nil {
		h.sseWriter.Write(c, sseError("streaming LLM not configured"))
		return
	}
	ctx := c.Request.Context()
	adapter := &service.TenantStreamAdapter{LLM: h.streamLLM, TenantID: user.ID, ModelID: modelID}
	for delta := range h.askSvc.Stream(ctx, adapter, user.ID, req.Question, filtered) {
		switch delta.Kind {
		case service.AskDeltaAnswer:
			h.sseWriter.Write(c, sseAnswer(delta.Value, nil, false))
		case service.AskDeltaMarker:
			h.sseWriter.Write(c, sseMarker(delta.Value))
		case service.AskDeltaError:
			h.sseWriter.Write(c, sseError(delta.Value))
		case service.AskDeltaFinal:
			h.sseWriter.Write(c, sseAnswer(delta.Value, delta.Refs, true))
		}
	}
	c.Stream(func(w io.Writer) bool {
		fmt.Fprintf(w, "data: {\"code\": 0, \"message\": \"\", \"data\": true}\n\n")
		return false
	})

}

// MindMap generates a query mind map for a shared search bot.
// @Summary Generate Mind Map
// @Description Retrieves related chunks and asks the configured chat model to summarize them into a mind map.
// @Tags searchbots
// @Accept json
// @Produce json
// @Param request body SearchBotMindMapRequest true "Mind map parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/searchbots/mindmap [post]
func (h *SearchBotHandler) MindMap(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req SearchBotMindMapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}

	filtered := make(common.StringSlice, 0, len(req.KbIDs))
	for _, id := range req.KbIDs {
		if strings.TrimSpace(id) != "" {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 || strings.TrimSpace(req.Question) == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "kb_ids and question are required")
		return
	}
	if h.chunkSvc == nil {
		jsonInternalError(c, fmt.Errorf("chunk service not configured"))
		return
	}
	if h.llm == nil {
		jsonInternalError(c, fmt.Errorf("LLM not configured"))
		return
	}

	searchConfig := map[string]interface{}{}
	if req.SearchID != "" {
		if h.searchSvc == nil {
			jsonInternalError(c, fmt.Errorf("search service not configured"))
			return
		}
		detail, err := h.searchSvc.GetDetail(req.SearchID)
		if err != nil {
			jsonInternalError(c, err)
			return
		}
		searchConfig = searchConfigFromDetail(detail)
	}

	mindMap, err := runMindMap(mindMapRunConfig{
		Question:      req.Question,
		KbIDs:         filtered,
		SearchID:      req.SearchID,
		SearchConfig:  searchConfig,
		AuthUserID:    user.ID,
		ModelTenantID: user.ID,
		ChunkSvc:      h.chunkSvc,
		LLM:           h.llm,
		TenantSvc:     h.tenantSvc,
	})
	if err != nil {
		common.Warn("searchbot mindmap failed", zap.String("error", err.Error()))
		jsonInternalError(c, err)
		return
	}
	common.SuccessWithData(c, mindMap, "success")
}

// SearchbotDetail returns the public share-page bootstrap payload for a
// search app. The route is mounted under apiNoAuth but still requires a beta
// token, matching Python's AUTH_BETA flow.
func (h *SearchBotHandler) SearchbotDetail(c *gin.Context) {
	searchID := strings.TrimSpace(c.Query("search_id"))
	if searchID == "" {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, "search_id is required")
		return
	}

	userSvc := service.NewUserService()
	user, code, err := userSvc.GetUserByBetaAPIToken(c.GetHeader("Authorization"))
	if err != nil {
		common.ResponseWithCodeData(c, code, nil, "Authentication error: API key is invalid!")
		return
	}

	detail, err := h.searchSvc.GetSearchShareDetail(user.ID, searchID)
	if err != nil {
		switch err.Error() {
		case "has no permission for this operation":
			common.ResponseWithCodeData(c, common.CodeOperatingError, nil, "Has no permission for this operation.")
		case "can't find this Search App!":
			common.ResponseWithCodeData(c, common.CodeDataError, nil, "Can't find this Search App!")
		default:
			jsonInternalError(c, err)
		}
		return
	}

	common.SuccessWithData(c, detail, "success")
}

// ---- SSE helpers ----

type ssePayload struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// askSSEData is the inner data object for SSE events, matching Python bot_api.py.
// The Reference field is always present (non-nil) so the frontend can safely
// access .chunks or .reduce without a null guard.
type askSSEData struct {
	Answer       string      `json:"answer"`
	Reference    interface{} `json:"reference"`
	Final        bool        `json:"final"`
	StartToThink bool        `json:"start_to_think,omitempty"`
	EndToThink   bool        `json:"end_to_think,omitempty"`
}

func sseAnswer(answer string, refs interface{}, final bool) string {
	if refs == nil {
		refs = map[string]interface{}{}
	}
	payload := ssePayload{
		Code:    0,
		Message: "",
		Data: askSSEData{
			Answer:    answer,
			Reference: refs,
			Final:     final,
		},
	}
	b, _ := json.Marshal(payload)
	return fmt.Sprintf("data: %s\n\n", string(b))
}

// sseError matches Python bot_api.py error format:
//
//	{"code": 500, "message": "...", "data": {"answer": "**ERROR**: ...", "reference": []}}
func sseError(message string) string {
	payload := ssePayload{
		Code:    int(common.CodeServerError),
		Message: message,
		Data: askSSEData{
			Answer:    "**ERROR**: " + message,
			Reference: []map[string]interface{}{},
		},
	}
	b, _ := json.Marshal(payload)
	return fmt.Sprintf("data: %s\n\n", string(b))
}

// sseMarker matches Python dialog_service.py think-tag marker format:
//
//	{"answer": "", "reference": {}, "final": false, "start_to_think": true}
func sseMarker(marker string) string {
	d := askSSEData{
		Answer:    "",
		Reference: map[string]interface{}{},
	}
	if marker == "<think>" {
		d.StartToThink = true
	} else {
		d.EndToThink = true
	}
	payload := ssePayload{Code: 0, Message: "", Data: d}
	b, _ := json.Marshal(payload)
	return fmt.Sprintf("data: %s\n\n", string(b))
}

type SSEWriter interface {
	Write(c *gin.Context, data string)
}

// ginSSEWriter is the production SSEWriter backed by gin.Context.Stream.
type ginSSEWriter struct{}

func (w *ginSSEWriter) Write(c *gin.Context, data string) {
	c.Stream(func(w io.Writer) bool {
		fmt.Fprint(w, data)
		return false
	})
}

// toRetrievalServiceRequest maps the handler DTO to the service DTO.
// The two structs differ in KbIDs (StringSlice → []string) and
// MetaDataFilter (→ Filter) to maintain Python API compatibility.
func toRetrievalServiceRequest(h *SearchBotRetrievalTestRequest) *service.RetrievalTestRequest {
	return &service.RetrievalTestRequest{
		Datasets:               common.StringSlice(h.KbIDs),
		Question:               h.Question,
		Page:                   h.Page,
		Size:                   h.Size,
		DocIDs:                 h.DocIDs,
		UseKG:                  h.UseKG,
		TopK:                   h.TopK,
		CrossLanguages:         h.CrossLanguages,
		SearchID:               h.SearchID,
		Filter:                 h.MetaDataFilter,
		TenantRerankID:         h.TenantRerankID,
		RerankID:               h.RerankID,
		Keyword:                h.Keyword,
		SimilarityThreshold:    h.SimilarityThreshold,
		VectorSimilarityWeight: h.VectorSimilarityWeight,
	}
}

// ptrFloat64 returns a pointer to a float64 value.
func ptrFloat64(v float64) *float64 { return &v }

func intPtr(v int) *int           { return &v }
func floatPtr(v float64) *float64 { return &v }

// applyRetrievalDefaults fills in default values for optional fields,
// matching Python bot_api.py retrieval_test endpoint.
func applyRetrievalDefaults(req *SearchBotRetrievalTestRequest) {
	if req.Page == nil {
		v := 1
		req.Page = &v
	}
	if req.Size == nil {
		v := 30
		req.Size = &v
	}
	if req.TopK == nil {
		v := 1024
		req.TopK = &v
	}
	if req.UseKG == nil {
		v := false
		req.UseKG = &v
	}
	if req.Keyword == nil {
		v := false
		req.Keyword = &v
	}
	if req.SimilarityThreshold == nil {
		v := 0.0
		req.SimilarityThreshold = &v
	}
	if req.VectorSimilarityWeight == nil {
		v := 0.3
		req.VectorSimilarityWeight = &v
	}
}
