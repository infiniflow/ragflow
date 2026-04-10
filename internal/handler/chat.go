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
	"net/http"
	"ragflow/internal/common"
	"strconv"

	"github.com/gin-gonic/gin"

	"ragflow/internal/service"
)

// ChatHandler chat handler
type ChatHandler struct {
	chatService *service.ChatService
	userService *service.UserService
}

// NewChatHandler create chat handler
func NewChatHandler(chatService *service.ChatService, userService *service.UserService) *ChatHandler {
	return &ChatHandler{
		chatService: chatService,
		userService: userService,
	}
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
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// List chats - default to valid status "1" (same as Python StatusEnum.VALID.value)
	result, err := h.chatService.ListChats(userID, "1")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result,
		"message": "success",
	})
}

// ListChatsNext list chats with advanced filtering and pagination
// @Summary List Chats Next
// @Description Get list of chats with filtering, pagination and sorting (equivalent to list_dialogs_next)
// @Tags chat
// @Accept json
// @Produce json
// @Param keywords query string false "search keywords"
// @Param page query int false "page number"
// @Param page_size query int false "items per page"
// @Param orderby query string false "order by field (default: create_time)"
// @Param desc query bool false "descending order (default: true)"
// @Param request body service.ListChatsNextRequest true "filter options including owner_ids"
// @Success 200 {object} service.ListChatsNextResponse
// @Router /v1/dialog/next [post]
func (h *ChatHandler) ListChatsNext(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
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

	// Parse request body for owner_ids
	var req service.ListChatsNextRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": err.Error(),
			})
			return
		}
	}

	// List chats with advanced filtering
	result, err := h.chatService.ListChatsNext(userID, keywords, page, pageSize, orderby, desc, req.OwnerIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result,
		"message": "success",
	})
}

// SetDialog create or update a dialog
// @Summary Set Dialog
// @Description Create or update a dialog (chat). If dialog_id is provided, updates existing dialog; otherwise creates new one.
// @Tags chat
// @Accept json
// @Produce json
// @Param request body service.SetDialogRequest true "dialog configuration"
// @Success 200 {object} service.SetDialogResponse
// @Router /v1/dialog/set [post]
func (h *ChatHandler) SetDialog(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Parse request body
	var req service.SetDialogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// Validate required field: prompt_config
	if req.PromptConfig == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "prompt_config is required",
		})
		return
	}

	// Call service to set dialog
	result, err := h.chatService.SetDialog(userID, &req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    result,
		"message": "success",
	})
}

// RemoveDialogsRequest remove dialogs request
type RemoveDialogsRequest struct {
	DialogIDs []string `json:"dialog_ids" binding:"required"`
}

// RemoveChats remove/delete dialogs (soft delete by setting status to invalid)
// @Summary Remove Dialogs
// @Description Remove dialogs by setting their status to invalid. Only the owner of the dialog can perform this operation.
// @Tags chat
// @Accept json
// @Produce json
// @Param request body RemoveDialogsRequest true "dialog IDs to remove"
// @Success 200 {object} map[string]interface{}
// @Router /v1/dialog/rm [post]
func (h *ChatHandler) RemoveChats(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Parse request body
	var req RemoveDialogsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// Call service to remove dialogs
	if err := h.chatService.RemoveChats(userID, req.DialogIDs); err != nil {
		// Check if it's an authorization error
		if err.Error() == "only owner of chat authorized for this operation" {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"data":    false,
				"message": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    0,
		"data":    true,
		"message": "success",
	})
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
		jsonError(c, errorCode, errorMessage)
		return
	}
	userID := user.ID

	// Get chat_id from path parameter (same as Python <chat_id>)
	chatID := c.Param("chat_id")
	if chatID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    common.CodeBadRequest,
			"data":    nil,
			"message": "chat_id is required",
		})
		return
	}

	// Get chat detail with permission check
	chat, err := h.chatService.GetChat(userID, chatID)
	if err != nil {
		errMsg := err.Error()
		// Check if it's an authorization error
		if errMsg == "no authorization" {
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeAuthenticationError,
				"data":    false,
				"message": "No authorization.",
			})
			return
		}
		// Not found error
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": err.Error(),
		})
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
		"kb_ids":                   chat.KBIDs,
		"dataset_ids":              chat.DatasetIDs,
		"kb_names":                 chat.KBNames,
		"status":                   chat.Status,
		"create_time":              chat.CreateTime,
		"update_time":              chat.UpdateTime,
	}

	// Return success response
	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": "success",
	})
}
