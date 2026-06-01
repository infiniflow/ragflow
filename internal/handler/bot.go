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
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/service"
)

type botServiceIface interface {
	AuthByBetaToken(token string) (string, error)
	GetChatbotInfo(tenantID, dialogID string) (map[string]interface{}, error)
	GetSearchbotDetail(tenantID, searchID string) (map[string]interface{}, error)
}

// BotHandler serves the public chatbot/searchbot endpoints, which authenticate
// with an SDK "beta" token rather than a session.
type BotHandler struct {
	botService botServiceIface
}

// NewBotHandler create bot handler
func NewBotHandler(botService *service.BotService) *BotHandler {
	return &BotHandler{botService: botService}
}

// resolveBetaTenant extracts the SDK beta token from the Authorization header
// and resolves it to a tenant ID. It writes the error response and returns
// false when authentication fails. Mirrors _get_sdk_authorization_token plus
// the APIToken.query(beta=token) lookup in bot_api.py.
func (h *BotHandler) resolveBetaTenant(c *gin.Context) (string, bool) {
	parts := strings.Fields(c.GetHeader("Authorization"))
	if len(parts) != 2 {
		jsonError(c, common.CodeDataError, "Authorization is not valid!")
		return "", false
	}

	tenantID, err := h.botService.AuthByBetaToken(parts[1])
	if errors.Is(err, service.ErrBotInvalidAPIKey) {
		jsonError(c, common.CodeDataError, err.Error())
		return "", false
	}
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return "", false
	}
	return tenantID, true
}

// ChatbotInfo returns public chatbot metadata for a dialog.
// @Summary Get Chatbot Info
// @Description Get public chatbot metadata (title, avatar, prologue) for a dialog
// @Tags bot
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param dialog_id path string true "dialog ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/chatbots/{dialog_id}/info [get]
func (h *BotHandler) ChatbotInfo(c *gin.Context) {
	tenantID, ok := h.resolveBetaTenant(c)
	if !ok {
		return
	}

	info, err := h.botService.GetChatbotInfo(tenantID, c.Param("dialog_id"))
	if err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	jsonResponse(c, common.CodeSuccess, info, "success")
}

// SearchbotDetail returns search-app detail for an embedded/shared search bot.
// @Summary Get Searchbot Detail
// @Description Get search-app detail for a search the tenant can access
// @Tags bot
// @Accept json
// @Produce json
// @Security ApiKeyAuth
// @Param search_id query string true "search ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/searchbots/detail [get]
func (h *BotHandler) SearchbotDetail(c *gin.Context) {
	tenantID, ok := h.resolveBetaTenant(c)
	if !ok {
		return
	}

	searchID := c.Query("search_id")
	if searchID == "" {
		jsonError(c, common.CodeArgumentError, "search_id is required")
		return
	}

	detail, err := h.botService.GetSearchbotDetail(tenantID, searchID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrBotNoSearchPermission):
			c.JSON(http.StatusOK, gin.H{
				"code":    common.CodeOperatingError,
				"data":    false,
				"message": err.Error(),
			})
		case errors.Is(err, service.ErrBotNoTenant), errors.Is(err, service.ErrBotSearchNotFound):
			jsonError(c, common.CodeDataError, err.Error())
		default:
			jsonError(c, common.CodeServerError, err.Error())
		}
		return
	}

	jsonResponse(c, common.CodeSuccess, detail, "success")
}
