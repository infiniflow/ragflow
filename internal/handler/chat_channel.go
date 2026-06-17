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
	"ragflow/internal/service"
	"strings"

	"github.com/gin-gonic/gin"
)

type ChatChannelHandler struct {
	chatChannelService *service.ChatChannelService
}

func NewChatChannelHandler(chatChannelService *service.ChatChannelService) *ChatChannelHandler {
	return &ChatChannelHandler{chatChannelService: chatChannelService}
}

// GetChatChannel Return a chat channel bot's details when the current user can access it.
func (h *ChatChannelHandler) GetChatChannel(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		jsonError(c, common.CodeArgumentError, "user_id is required")
		return
	}

	channelID := strings.TrimSpace(c.Param("channel_id"))
	if channelID == "" {
		jsonError(c, common.CodeArgumentError, "channel_id is required")
		return
	}

	channel, errorCode, err := h.chatChannelService.GetChatChannel(userID, channelID)
	if errorCode != common.CodeSuccess || err != nil {
		writeChatChannelError(c, errorCode, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    channel,
		"message": "success",
	})
}

// UpdateChatChannel Update an accessible chat channel bot's name/config/status.
func (h *ChatChannelHandler) UpdateChatChannel(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		jsonError(c, common.CodeArgumentError, "user_id is required")
		return
	}

	channelID := strings.TrimSpace(c.Param("channel_id"))
	if channelID == "" {
		jsonError(c, common.CodeArgumentError, "channel_id is required")
		return
	}

	var request map[string]interface{}
	if err := c.ShouldBindJSON(&request); err != nil {
		jsonError(c, common.CodeDataError, err.Error())
		return
	}

	reqPayload := unwrapChatChannelPayload(request)

	result, errorCode, err := h.chatChannelService.UpdateChatChannel(userID, channelID, reqPayload)
	if errorCode != common.CodeSuccess || err != nil {
		writeChatChannelError(c, errorCode, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": "success",
	})
}

// DeleteChatChannel Delete an accessible chat channel bot.
func (h *ChatChannelHandler) DeleteChatChannel(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	userID := strings.TrimSpace(user.ID)
	if userID == "" {
		jsonError(c, common.CodeArgumentError, "user_id is required")
		return
	}

	channelID := strings.TrimSpace(c.Param("channel_id"))
	if channelID == "" {
		jsonError(c, common.CodeArgumentError, "channel_id is required")
		return
	}

	result, errorCode, err := h.chatChannelService.DeleteChatChannel(userID, channelID)
	if errorCode != common.CodeSuccess || err != nil {
		writeChatChannelError(c, errorCode, err.Error())
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": "success",
	})
}

func unwrapChatChannelPayload(payload map[string]interface{}) map[string]interface{} {
	if data, ok := payload["data"].(map[string]interface{}); ok {
		return data
	}
	return payload
}

func writeChatChannelError(c *gin.Context, code common.ErrorCode, message string) {
	if code == common.CodeAuthenticationError && message == "No authorization." {
		c.JSON(http.StatusOK, gin.H{
			"code":    code,
			"data":    false,
			"message": message,
		})
		return
	}
	jsonError(c, code, message)
}
