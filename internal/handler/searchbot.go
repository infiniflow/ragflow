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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
	"ragflow/internal/service"

	"go.uber.org/zap"
)

// searchbotLLM is the interface for LLM calls used by SearchBotHandler.
type searchbotLLM interface {
	Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error)
}

// ChunkRetriever abstracts chunk retrieval for the searchbots handler.
type ChunkRetriever interface {
	RetrievalTest(req *service.RetrievalTestRequest, userID string) (*service.RetrievalTestResponse, error)
}

// streamingLLM abstracts streaming chat for the Ask endpoint.
// The returned channel delivers raw text deltas from the LLM.
// Implementations should respect ctx cancellation to prevent goroutine leaks.
type streamingLLM interface {
	ChatStream(ctx context.Context, tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, error)
}

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

// SearchBotRealLLM wraps ModelProviderService to implement searchbotLLM.
type SearchBotRealLLM struct {
	Svc *service.ModelProviderService
}

func (r *SearchBotRealLLM) Chat(tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (*modelModule.ChatResponse, error) {
	chatModel, err := r.Svc.GetChatModel(tenantID, modelID)
	if err != nil {
		return nil, err
	}
	return chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, messages, chatModel.APIConfig, config)
}

// ChatStream implements streamingLLM.
func (r *SearchBotRealLLM) ChatStream(ctx context.Context, tenantID, modelID string, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, error) {
	chatModel, err := r.Svc.GetChatModel(tenantID, modelID)
	if err != nil {
		return nil, err
	}
	return chatStreamWithContext(ctx, chatModel, messages, config), nil
}

// chatStreamWithContext creates a streaming LLM channel that stops sending
// when ctx is cancelled, preventing goroutine leaks on client disconnect.
func chatStreamWithContext(ctx context.Context, chatModel *modelModule.ChatModel, messages []modelModule.Message, config *modelModule.ChatConfig) <-chan string {
	ch := make(chan string, 256)
	go func() {
		defer close(ch)
		if err := chatModel.ModelDriver.ChatStreamlyWithSender(*chatModel.ModelName, messages, chatModel.APIConfig, config,
			func(delta *string, _ *string) error {
				if delta == nil {
					return nil
				}
				select {
				case ch <- *delta:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}); err != nil {
			if err == context.Canceled || err == context.DeadlineExceeded {
				return
			}
			common.Warn("ChatStreamlyWithSender returned error", zap.Error(err))
		}
	}()
	return ch
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
	llm       searchbotLLM
	streamLLM streamingLLM
	chunkSvc  ChunkRetriever
	askSvc    *service.AskService
	sseWriter SSEWriter
}

// NewSearchBotHandler creates a new SearchBotHandler.
func NewSearchBotHandler(searchSvc *service.SearchService, tenantSvc *service.TenantService, llm searchbotLLM, chunkSvc ChunkRetriever) *SearchBotHandler {
	return &SearchBotHandler{searchSvc: searchSvc, tenantSvc: tenantSvc, llm: llm, chunkSvc: chunkSvc, sseWriter: &ginSSEWriter{}}
}

// SetStreamLLM sets the streaming LLM for the Ask endpoint.
func (h *SearchBotHandler) SetStreamLLM(llm streamingLLM) { h.streamLLM = llm }

// SetAskService sets the AskService used by the Ask endpoint.
func (h *SearchBotHandler) SetAskService(svc *service.AskService) { h.askSvc = svc }

// askStreamAdapter adapts handler.streamingLLM to service.StreamingLLM.
type askStreamAdapter struct {
	llm      streamingLLM
	tenantID string
	modelID  string
}

func (a *askStreamAdapter) ChatStream(ctx context.Context, messages []modelModule.Message, config *modelModule.ChatConfig) (<-chan string, error) {
	if a.llm == nil {
		return nil, fmt.Errorf("streaming LLM not configured")
	}
	return a.llm.ChatStream(ctx, a.tenantID, a.modelID, messages, config)
}

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
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req SearchBotRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "question is required",
		})
		return
	}

	if req.Question == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "question is required",
		})
		return
	}

	// Resolve model ID from search config if provided
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

	messages := []modelModule.Message{
		{Role: "system", Content: relatedQuestionPrompt},
		{Role: "user", Content: "Keywords: " + req.Question + "\nRelated search terms:\n"},
	}

	genConf := &modelModule.ChatConfig{
		Temperature: ptrFloat64(0.9),
	}

	response, err := h.llm.Chat(user.ID, modelID, messages, genConf)
	if err != nil {
		common.Warn("searchbot LLM call failed", zap.String("error", err.Error()))
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeOperatingError,
			"data":    nil,
			"message": "LLM call failed",
		})
		return
	}

	var questions []string
	if response != nil && response.Answer != nil {
		questions = parseRelatedQuestions(*response.Answer)
	}
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    questions,
		"message": "success",
	})
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
		c.JSON(http.StatusUnauthorized, gin.H{"code": errorCode, "data": nil, "message": errorMessage})
		return
	}

	var req SearchBotRetrievalTestRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "data": nil, "message": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "data": nil, "message": "kb_id and question are required"})
		return
	}

	applyRetrievalDefaults(&req)

	if req.TopK != nil && *req.TopK <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "data": nil, "message": "top_k must be greater than 0"})
		return
	}

	svcReq := toRetrievalServiceRequest(&req)

	result, err := h.chunkSvc.RetrievalTest(svcReq, user.ID)
	if err != nil {
		common.Warn("searchbot retrieval test failed", zap.String("error", err.Error()))
		c.JSON(http.StatusInternalServerError, gin.H{"code": common.CodeServerError, "data": nil, "message": "retrieval test failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": int(common.CodeSuccess), "data": result, "message": "success"})
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
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req SearchBotAskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "data": nil, "message": err.Error()})
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
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "data": nil, "message": "kb_ids and question are required"})
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
	ctx := c.Request.Context()
	adapter := &askStreamAdapter{llm: h.streamLLM, tenantID: user.ID, modelID: modelID}
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
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req SearchBotMindMapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "data": nil, "message": err.Error()})
		return
	}

	filtered := make(common.StringSlice, 0, len(req.KbIDs))
	for _, id := range req.KbIDs {
		if strings.TrimSpace(id) != "" {
			filtered = append(filtered, id)
		}
	}
	if len(filtered) == 0 || strings.TrimSpace(req.Question) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": common.CodeArgumentError, "data": nil, "message": "kb_ids and question are required"})
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
	if req.SearchID != "" && h.searchSvc != nil {
		if detail, err := h.searchSvc.GetDetail(req.SearchID); err == nil {
			searchConfig = searchConfigFromDetail(detail)
		}
	}

	retrievalReq := mindMapRetrievalRequest(req.Question, filtered, req.SearchID, searchConfig)
	ranks, err := h.chunkSvc.RetrievalTest(retrievalReq, user.ID)
	if err != nil {
		common.Warn("searchbot mindmap retrieval failed", zap.String("error", err.Error()))
		jsonInternalError(c, err)
		return
	}
	sections := mindMapSections(ranks)
	if len(sections) == 0 {
		jsonResponse(c, common.CodeSuccess, gin.H{"id": "root", "children": []interface{}{}}, "success")
		return
	}

	modelID, _ := searchConfig["chat_id"].(string)
	if modelID == "" && h.tenantSvc != nil {
		defaultModel, err := h.tenantSvc.GetDefaultModelName(user.ID, entity.ModelTypeChat)
		if err == nil {
			modelID = defaultModel
		}
	}
	response, err := h.llm.Chat(user.ID, modelID, []modelModule.Message{{Role: "user", Content: mindMapPrompt(strings.Join(sections, "\n"))}, {Role: "user", Content: "Output:"}}, &modelModule.ChatConfig{})
	if err != nil {
		common.Warn("searchbot mindmap LLM call failed", zap.String("error", err.Error()))
		jsonInternalError(c, err)
		return
	}
	if response == nil || response.Answer == nil {
		jsonResponse(c, common.CodeSuccess, gin.H{"id": "root", "children": []interface{}{}}, "success")
		return
	}

	jsonResponse(c, common.CodeSuccess, parseMindMapMarkdown(*response.Answer), "success")
}

// SearchbotDetail returns the public share-page bootstrap payload for a
// search app. The route is mounted under apiNoAuth but still requires a beta
// token, matching Python's AUTH_BETA flow.
func (h *SearchBotHandler) SearchbotDetail(c *gin.Context) {
	searchID := strings.TrimSpace(c.Query("search_id"))
	if searchID == "" {
		jsonError(c, common.CodeArgumentError, "search_id is required")
		return
	}

	userSvc := service.NewUserService()
	user, code, err := userSvc.GetUserByBetaAPIToken(c.GetHeader("Authorization"))
	if err != nil {
		jsonError(c, code, "Authentication error: API key is invalid!")
		return
	}

	detail, err := h.searchSvc.GetSearchShareDetail(user.ID, searchID)
	if err != nil {
		switch err.Error() {
		case "has no permission for this operation":
			jsonError(c, common.CodeOperatingError, "Has no permission for this operation.")
		case "can't find this Search App!":
			jsonError(c, common.CodeDataError, "Can't find this Search App!")
		default:
			jsonInternalError(c, err)
		}
		return
	}

	jsonResponse(c, common.CodeSuccess, detail, "success")
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

// SSEWriter writes an SSE event to the client.
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

var relatedQuestionLineRe = regexp.MustCompile(`^\d+\.\s`)

// parseRelatedQuestions extracts numbered list items from an LLM response.
// Lines matching "^N. " are extracted and the number prefix is stripped.
func parseRelatedQuestions(text string) []string {
	var result []string
	for _, line := range strings.Split(text, "\n") {
		if relatedQuestionLineRe.MatchString(line) {
			result = append(result, relatedQuestionLineRe.ReplaceAllString(line, ""))
		}
	}
	if result == nil {
		return []string{}
	}
	return result
}

func searchConfigFromDetail(detail map[string]interface{}) map[string]interface{} {
	if sc, ok := detail["search_config"].(map[string]interface{}); ok && sc != nil {
		return sc
	}
	if sc, ok := detail["search_config"].(entity.JSONMap); ok && sc != nil {
		return map[string]interface{}(sc)
	}
	return map[string]interface{}{}
}

func mindMapRetrievalRequest(question string, kbIDs common.StringSlice, searchID string, searchConfig map[string]interface{}) *service.RetrievalTestRequest {
	page := 1
	size := 12
	topK := intFromConfig(searchConfig, "top_k", 1024)
	similarityThreshold := floatFromConfig(searchConfig, "similarity_threshold", 0.2)
	vectorSimilarityWeight := floatFromConfig(searchConfig, "vector_similarity_weight", 0.3)
	req := &service.RetrievalTestRequest{
		Datasets:               kbIDs,
		Question:               question,
		Page:                   &page,
		Size:                   &size,
		TopK:                   &topK,
		SimilarityThreshold:    &similarityThreshold,
		VectorSimilarityWeight: &vectorSimilarityWeight,
		DocIDs:                 stringSliceFromConfig(searchConfig, "doc_ids"),
		Filter:                 mapFromConfig(searchConfig, "meta_data_filter"),
	}
	if searchID != "" {
		req.SearchID = &searchID
	}
	if rerankID, _ := searchConfig["rerank_id"].(string); rerankID != "" {
		req.RerankID = &rerankID
	}
	return req
}

func mindMapSections(ranks *service.RetrievalTestResponse) []string {
	if ranks == nil {
		return nil
	}
	sections := make([]string, 0, len(ranks.Chunks))
	for _, chunk := range ranks.Chunks {
		if content, ok := chunk["content_with_weight"].(string); ok && strings.TrimSpace(content) != "" {
			sections = append(sections, content)
		}
	}
	return sections
}

func intFromConfig(config map[string]interface{}, key string, fallback int) int {
	switch v := config[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	}
	return fallback
}

func floatFromConfig(config map[string]interface{}, key string, fallback float64) float64 {
	switch v := config[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case json.Number:
		if n, err := v.Float64(); err == nil {
			return n
		}
	}
	return fallback
}

func stringSliceFromConfig(config map[string]interface{}, key string) []string {
	switch v := config[key].(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func mapFromConfig(config map[string]interface{}, key string) map[string]interface{} {
	if m, ok := config[key].(map[string]interface{}); ok {
		return m
	}
	if m, ok := config[key].(entity.JSONMap); ok {
		return map[string]interface{}(m)
	}
	return nil
}

func mindMapPrompt(inputText string) string {
	return `- Role: You're a talent text processor to summarize a piece of text into a mind map.

- Step of task:
  1. Generate a title for user's 'TEXT'.
  2. Classify the 'TEXT' into sections of a mind map.
  3. If the subject matter is really complex, split them into sub-sections and sub-subsections.
  4. Add a shot content summary of the bottom level section.

- Output requirement:
  - Generate at least 4 levels.
  - Always try to maximize the number of sub-sections.
  - In language of 'Text'
  - MUST IN FORMAT OF MARKDOWN

-TEXT-
` + inputText + "\n"
}

type mindMapNode struct {
	ID       string        `json:"id"`
	Children []mindMapNode `json:"children"`
}

var mindMapHeadingRe = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)
var mindMapListRe = regexp.MustCompile(`^(\s*)(?:[-*+]|\d+\.)\s+(.+)$`)

func parseMindMapMarkdown(text string) mindMapNode {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	root := mindMapNode{ID: "root", Children: []mindMapNode{}}
	stack := []*mindMapNode{&root}
	inFence := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence || trimmed == "" {
			continue
		}
		level := 0
		title := ""
		if m := mindMapHeadingRe.FindStringSubmatch(trimmed); len(m) == 3 {
			level = len(m[1])
			title = cleanMindMapText(m[2])
		} else if m := mindMapListRe.FindStringSubmatch(line); len(m) == 3 {
			level = len(m[1])/2 + 1
			title = cleanMindMapText(m[2])
		}
		if title == "" {
			continue
		}
		for len(stack) > level {
			stack = stack[:len(stack)-1]
		}
		parent := stack[len(stack)-1]
		parent.Children = append(parent.Children, mindMapNode{ID: title, Children: []mindMapNode{}})
		stack = append(stack, &parent.Children[len(parent.Children)-1])
	}
	if len(root.Children) == 1 {
		return root.Children[0]
	}
	return root
}

func cleanMindMapText(text string) string {
	text = strings.TrimSpace(text)
	text = strings.Trim(text, "`")
	text = strings.Trim(text, "*_ ")
	return strings.TrimSpace(text)
}

// relatedQuestionPrompt is the system prompt for generating related search questions.
// Matches Python rag/prompts/related_question.md
const relatedQuestionPrompt = `# Role
You are an AI language model assistant tasked with generating **5-10 related questions** based on a user's original query.
These questions should help **expand the search query scope** and **improve search relevance**.

---

## Instructions

**Input:**
You are provided with a **user's question**.

**Output:**
Generate **5-10 alternative questions** that are **related** to the original user question.
These alternatives should help retrieve a **broader range of relevant documents** from a vector database.

**Context:**
Focus on **rephrasing** the original question in different ways, ensuring the alternative questions are **diverse but still connected** to the topic of the original query.
Do **not** create overly obscure, irrelevant, or unrelated questions.

**Fallback:**
If you cannot generate any relevant alternatives, do **not** return any questions.

---

## Guidance

1. Each alternative should be **unique** but still **relevant** to the original query.
2. Keep the phrasing **clear, concise, and easy to understand**.
3. Avoid overly technical jargon or specialized terms **unless directly relevant**.
4. Ensure that each question **broadens** the search angle, **not narrows** it.

---

## Example

**Original Question:**
> What are the benefits of electric vehicles?

**Alternative Questions:**
1. How do electric vehicles impact the environment?
2. What are the advantages of owning an electric car?
3. What is the cost-effectiveness of electric vehicles?
4. How do electric vehicles compare to traditional cars in terms of fuel efficiency?
5. What are the environmental benefits of switching to electric cars?
6. How do electric vehicles help reduce carbon emissions?
7. Why are electric vehicles becoming more popular?
8. What are the long-term savings of using electric vehicles?
9. How do electric vehicles contribute to sustainability?
10. What are the key benefits of electric vehicles for consumers?

---

## Reason
Rephrasing the original query into multiple alternative questions helps the user explore **different aspects** of their search topic, improving the **quality of search results**.
These questions guide the search engine to provide a **more comprehensive set** of relevant documents.`
