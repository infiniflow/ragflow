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
	"net/http"
	"ragflow/internal/common"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// ChatHandler chat handler
type ChatHandler struct {
	chatService *service.ChatService
	userService *service.UserService
	searchSvc   *service.SearchService
	tenantSvc   *service.TenantService
	llm         *service.ModelProviderService
	chunkSvc    service.Retriever
}

// NewChatHandler create chat handler
func NewChatHandler(chatService *service.ChatService, userService *service.UserService) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		userService: userService,
	}
}

// SetMindMapDependencies sets dependencies used by POST /api/v1/chat/mindmap.
func (h *ChatHandler) SetMindMapDependencies(searchSvc *service.SearchService, tenantSvc *service.TenantService, llm *service.ModelProviderService, chunkSvc service.Retriever) {
	h.searchSvc = searchSvc
	h.tenantSvc = tenantSvc
	h.llm = llm
	h.chunkSvc = chunkSvc
}

// ChatMindMapRequest is the request body for POST /api/v1/chat/mindmap.
type ChatMindMapRequest struct {
	Question string             `json:"question" binding:"required"`
	KbIDs    common.StringSlice `json:"kb_ids" binding:"required"`
	SearchID string             `json:"search_id,omitempty"`
}

// ListChats list chats
// @Summary List Chats
// @Description Get list of chats (dialogs) for the current user
// @Tags chat
// @Accept json
// @Produce json
// @Success 200 {object} service.ListChatsResponse
// @Router /api/v1/chats [get]
func (h *ChatHandler) ListChats(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	userID := user.ID

	// Parse query parameters
	keywords := c.Query("keywords")

	page := 0
	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	pageSize := 0
	if pageSizeStr := c.Query("page_size"); pageSizeStr != "" {
		if ps, err := strconv.Atoi(pageSizeStr); err == nil && ps > 0 {
			pageSize = ps
		}
	}

	orderby := c.DefaultQuery("orderby", "create_time")

	desc := true
	if descStr := c.Query("desc"); descStr != "" {
		desc = descStr != "false"
	}

	// List chats - default to valid status "1" (same as Python StatusEnum.VALID.value)
	result, err := h.chatService.ListChats(userID, "1", keywords, page, pageSize, orderby, desc)
	if err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusInternalServerError, 500, nil, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// Create creates a chat.
// @Summary Create Chat
// @Description Create a chat, aligned with Python POST /api/v1/chats.
// @Tags chat
// @Accept json
// @Produce json
// @Param request body service.CreateChatRequest true "chat configuration"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/chats [post]
func (h *ChatHandler) Create(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req map[string]interface{}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.UseNumber()
	if err := decoder.Decode(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeArgumentError, nil, err.Error())
		return
	}
	if req == nil {
		req = map[string]interface{}{}
	}

	result, code, err := h.chatService.Create(user.ID, req)
	if err != nil {
		common.ErrorWithCode(c, int(code), err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}

// MindMap generates a query mind map for chat search results.
// @Summary Generate Chat Mind Map
// @Description Retrieves related chunks and asks the configured chat model to summarize them into a mind map.
// @Tags chat
// @Accept json
// @Produce json
// @Param request body ChatMindMapRequest true "Mind map parameters"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/chat/mindmap [post]
func (h *ChatHandler) MindMap(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	var req ChatMindMapRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, err.Error())
		return
	}
	if strings.TrimSpace(req.Question) == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "kb_ids and question are required")
		return
	}

	searchConfig := map[string]interface{}{}
	modelTenantID := user.ID
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
		if tenantID, ok := detail["tenant_id"].(string); ok && tenantID != "" {
			modelTenantID = tenantID
		}
	}

	kbIDs := mergeMindMapKbIDs(stringSliceFromConfig(searchConfig, "kb_ids"), req.KbIDs)
	if len(kbIDs) == 0 {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeArgumentError, nil, "kb_ids and question are required")
		return
	}

	mindMap, err := runMindMap(mindMapRunConfig{
		Question:      req.Question,
		KbIDs:         kbIDs,
		SearchID:      req.SearchID,
		SearchConfig:  searchConfig,
		AuthUserID:    user.ID,
		ModelTenantID: modelTenantID,
		ChunkSvc:      h.chunkSvc,
		LLM:           h.llm,
		TenantSvc:     h.tenantSvc,
	})
	if err != nil {
		jsonInternalError(c, err)
		return
	}
	common.SuccessWithData(c, mindMap, "success")
}

func (h *ChatHandler) DeleteChat(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	userID := user.ID

	chatID := c.Param("chat_id")
	if chatID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "chat_id is required")
		return
	}

	if err := h.chatService.DeleteChat(userID, chatID); err != nil {
		if err.Error() == "no authorization" {
			common.ResponseWithCodeData(c, common.CodeDataError, false, "No authorization")
			return
		}
		common.ErrorWithCode(c, int(common.CodeDataError), err.Error())
		return
	}

	common.SuccessWithData(c, true, "success")
}

// BulkDeleteChats soft deletes multiple chats owned by the current user.
func (h *ChatHandler) BulkDeleteChats(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	userID := user.ID
	if c.Request.Body == nil || c.Request.ContentLength == 0 {
		common.SuccessWithData(c, map[string]interface{}{}, "success")
		return
	}

	var req service.BulkDeleteChatsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "Invalid request body: "+err.Error())
		return
	}

	if len(req.IDs) == 0 && !req.DeleteAll {
		if req.ChatID != "" {
			if err := h.chatService.DeleteChat(userID, req.ChatID); err != nil {
				if err.Error() == "no authorization" {
					common.ResponseWithCodeData(c, common.CodeAuthenticationError, false, "No authorization.")
					return
				}
				common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
				return
			}

			common.SuccessWithData(c, true, "success")
			return
		}

		common.SuccessWithData(c, map[string]interface{}{}, "success")
		return
	}

	result, err := h.chatService.BulkDeleteChats(userID, &req)
	if err != nil {
		common.ErrorWithCode(c, int(common.CodeDataError), err.Error())
		return
	}

	message := "success"
	if errorsList, ok := result["errors"].([]string); ok && len(errorsList) > 0 {
		if successCount, ok := result["success_count"].(int); ok {
			message = "Partially deleted " + strconv.Itoa(successCount) + " chats with " + strconv.Itoa(len(errorsList)) + " errors"
		}
	}

	common.SuccessWithData(c, result, message)
}

// GetChat get chat detail
// @Summary Get Chat Detail
// @Description Get detail of a chat by ID
// @Tags chat
// @Accept json
// @Produce json
// @Param chat_id path string true "chat ID"
// @Success 200 {object} service.GetChatResponse
// @Router /api/v1/chats/{chat_id} [get]
// Reference: api/apps/restful_apis/chat_api.py::get_chat
// Python implementation details:
// - Route: @manager.route("/chats/<chat_id>", methods=["GET"])
func (h *ChatHandler) GetChat(c *gin.Context) {
	// Get current user from context (same as Python current_user)
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}
	userID := user.ID

	// Get chat_id from path parameter (same as Python <chat_id>)
	chatID := c.Param("chat_id")
	if chatID == "" {
		common.ResponseWithHttpCodeData(c, http.StatusBadRequest, common.CodeBadRequest, nil, "chat_id is required")
		return
	}

	// Get chat detail with permission check
	chat, err := h.chatService.GetChat(userID, chatID)
	if err != nil {
		errMsg := err.Error()
		// Check if it's an authorization error
		if errMsg == "no authorization" {
			common.ResponseWithCodeData(c, common.CodeDataError, false, "No authorization")
			return
		}
		// Not found error
		common.ErrorWithCode(c, int(common.CodeDataError), err.Error())
		return
	}

	// Build response (same as Python _build_chat_response)
	// The service already returns GetChatResponse with DatasetIDs and KBNames
	result := map[string]interface{}{
		"id":                       chat.ID,
		"tenant_id":                chat.TenantID,
		"name":                     chat.Name,
		"description":              chat.Description,
		"icon":                     chat.Icon,
		"language":                 chat.Language,
		"llm_id":                   chat.LLMID,
		"llm_setting":              chat.LLMSetting,
		"prompt_type":              chat.PromptType,
		"prompt_config":            chat.PromptConfig,
		"meta_data_filter":         chat.MetaDataFilter,
		"similarity_threshold":     chat.SimilarityThreshold,
		"vector_similarity_weight": chat.VectorSimilarityWeight,
		"top_n":                    chat.TopN,
		"top_k":                    chat.TopK,
		"do_refer":                 chat.DoRefer,
		"rerank_id":                chat.RerankID,
		"dataset_ids":              chat.DatasetIDs,
		"kb_names":                 chat.KBNames,
		"status":                   chat.Status,
		"create_time":              chat.CreateTime,
		"create_date":              chat.CreateDate,
		"update_time":              chat.UpdateTime,
		"update_date":              chat.UpdateDate,
		"tenant_llm_id":            chat.TenantLLMID,
		"tenant_rerank_id":         chat.TenantRerankID,
	}

	// Return success response
	common.SuccessWithData(c, result, "success")
}

// UpdateChat updates a chat by ID using REST PUT semantics.
func (h *ChatHandler) UpdateChat(c *gin.Context) {
	h.updateChatByMethod(c, false)
}

// PatchChat updates a chat by ID using REST PATCH semantics.
func (h *ChatHandler) PatchChat(c *gin.Context) {
	h.updateChatByMethod(c, true)
}

func (h *ChatHandler) updateChatByMethod(c *gin.Context, patch bool) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		common.ErrorWithCode(c, int(errorCode), errorMessage)
		return
	}

	chatID := c.Param("chat_id")
	if chatID == "" {
		common.ResponseWithCodeData(c, common.CodeBadRequest, nil, "chat_id is required")
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	var (
		result map[string]interface{}
		err    error
	)
	if patch {
		result, err = h.chatService.PatchChat(user.ID, chatID, req)
	} else {
		result, err = h.chatService.UpdateChat(user.ID, chatID, req)
	}
	if err != nil {
		if err.Error() == "no authorization" {
			common.ResponseWithCodeData(c, common.CodeDataError, false, "No authorization")
			return
		}
		common.ResponseWithCodeData(c, common.CodeDataError, nil, err.Error())
		return
	}

	common.SuccessWithData(c, result, "success")
}
