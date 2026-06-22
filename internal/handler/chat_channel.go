// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type ChatChannelService interface {
	CreateChatChannel(tenantID, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error)
	List(tenantID string) ([]*entity.ChatChannelListResponse, error)
	GetChatChannel(userID, channelID string) (*entity.ChatChannel, common.ErrorCode, error)
	UpdateChatChannel(userID, channelID string, req map[string]interface{}) (*entity.ChatChannel, common.ErrorCode, error)
	DeleteChatChannel(userID, channelID string) (bool, common.ErrorCode, error)
}

type ChatChannelHandler struct {
	chatChannelService ChatChannelService
}

func NewChatChannelHandler(chatChannelService ChatChannelService) *ChatChannelHandler {
	return &ChatChannelHandler{chatChannelService: chatChannelService}
}

// NewChatChannel keeps the existing constructor shape used by boot code.
func NewChatChannel() *ChatChannelHandler {
	return NewChatChannelHandler(service.NewChatChannelService())
}

type CreateChatChannelRequest struct {
	Name    string         `json:"name" binding:"required"`
	Channel string         `json:"channel" binding:"required"`
	Config  entity.JSONMap `json:"config" binding:"required"`
	ChatID  *string        `json:"chat_id"`
}

// CreateChatChannel handles POST /chat-channels.
func (h *ChatChannelHandler) CreateChatChannel(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	var req CreateChatChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		jsonError(c, common.CodeDataError, "Invalid request: "+err.Error())
		return
	}

	row, err := h.chatChannelService.CreateChatChannel(
		user.ID,
		req.Name,
		req.Channel,
		req.Config,
		req.ChatID,
	)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	jsonResponse(c, common.CodeSuccess, row, "success")
}

// ListChatChannel handles GET /chat-channels.
func (h *ChatChannelHandler) ListChatChannel(c *gin.Context) {
	user, errorCode, errorMessage := GetUser(c)
	if errorCode != common.CodeSuccess {
		jsonError(c, errorCode, errorMessage)
		return
	}

	rows, err := h.chatChannelService.List(user.ID)
	if err != nil {
		jsonError(c, common.CodeServerError, err.Error())
		return
	}
	jsonResponse(c, common.CodeSuccess, rows, "success")
}

// GetChatChannel handles GET /chat-channels/:channel_id.
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

	channel, code, err := h.chatChannelService.GetChatChannel(userID, channelID)
	if code != common.CodeSuccess || err != nil {
		writeChatChannelError(c, code, chatChannelErrMsg(code, err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    channel,
		"message": "success",
	})
}

// UpdateChatChannel handles PATCH /chat-channels/:channel_id.
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

	result, code, err := h.chatChannelService.UpdateChatChannel(userID, channelID, unwrapChatChannelPayload(request))
	if code != common.CodeSuccess || err != nil {
		writeChatChannelError(c, code, chatChannelErrMsg(code, err))
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    common.CodeSuccess,
		"data":    result,
		"message": "success",
	})
}

// DeleteChatChannel handles DELETE /chat-channels/:channel_id.
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

	result, code, err := h.chatChannelService.DeleteChatChannel(userID, channelID)
	if code != common.CodeSuccess || err != nil {
		writeChatChannelError(c, code, chatChannelErrMsg(code, err))
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

func chatChannelErrMsg(code common.ErrorCode, err error) string {
	if err != nil {
		return err.Error()
	}
	return code.Message()
}
