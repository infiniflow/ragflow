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

	"github.com/gin-gonic/gin"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
)

// ChatbotInfoResponse is the response for GET /api/v1/chatbots/<dialog_id>/info.
type ChatbotInfoResponse struct {
	Title        *string `json:"title"`
	Avatar       *string `json:"avatar"`
	Prologue     string  `json:"prologue"`
	HasTavilyKey bool    `json:"has_tavily_key"`
}

// GetChatbotInfo returns basic information about a chatbot dialog.
// @Summary Get Chatbot Info
// @Description Returns title, avatar, prologue and tavily key status for a chatbot.
// @Tags chatbots
// @Produce json
// @Param dialog_id path string true "Dialog ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/chatbots/{dialog_id}/info [get]
func GetChatbotInfo(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	dialogID := c.Param("dialog_id")
	if dialogID == "" {
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeArgumentError,
			"data":    nil,
			"message": "dialog_id is required",
		})
		return
	}

	chatDAO := dao.NewChatDAO()
	dialog, err := chatDAO.GetByIDAndStatus(dialogID, "1")
	if err != nil {
		common.Warn("chatbot info denied: dialog not found or invalid status",
			zap.String("dialog_id", dialogID))
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": "Authentication error: no access to this chatbot!",
		})
		return
	}

	if !hasTenantAccess(dialog.TenantID, user.ID) {
		common.Warn("chatbot info denied: tenant mismatch",
			zap.String("dialog_tenant", dialog.TenantID),
			zap.String("user", user.ID))
		c.JSON(http.StatusOK, gin.H{
			"code":    common.CodeDataError,
			"data":    nil,
			"message": "Authentication error: no access to this chatbot!",
		})
		return
	}

	prologue := ""
	hasTavilyKey := false
	if dialog.PromptConfig != nil {
		if p, ok := dialog.PromptConfig["prologue"].(string); ok {
			prologue = p
		}
		if k, ok := dialog.PromptConfig["tavily_api_key"].(string); ok {
			hasTavilyKey = len(k) > 0
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data": ChatbotInfoResponse{
			Title:        dialog.Name,
			Avatar:       dialog.Icon,
			Prologue:     prologue,
			HasTavilyKey: hasTavilyKey,
		},
		"message": "",
	})
}

// hasTenantAccess checks whether userID has access to the given tenantID.
// Returns true if userID == tenantID or if userID is a member of that tenant.
func hasTenantAccess(tenantID, userID string) bool {
	if tenantID == userID {
		return true
	}
	tenantDAO := dao.NewUserTenantDAO()
	tenantIDs, err := tenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return false
	}
	for _, tid := range tenantIDs {
		if tid == tenantID {
			return true
		}
	}
	return false
}
