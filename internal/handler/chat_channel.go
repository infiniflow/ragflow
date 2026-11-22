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
	"context"
	"strings"

	"github.com/gin-gonic/gin"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
)

type ChatChannelService interface {
	CreateChatChannel(ctx context.Context, name, channelType string, config entity.JSONMap, chatID, agentID *string) (*entity.ChatChannel, error)
	List(ctx context.Context) ([]*entity.ChatChannelListResponse, error)
	GetChatChannel(ctx context.Context, channelID string) (*entity.ChatChannel, common.ErrorCode, error)
	UpdateChatChannel(ctx context.Context, channelID string, req map[string]interface{}) (*entity.ChatChannel, common.ErrorCode, error)
	GetChatChannelRuntime(ctx context.Context, channelID string) (map[string]any, common.ErrorCode, error)
	DeleteChatChannel(ctx context.Context, channelID string) (bool, common.ErrorCode, error)
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
	Config  entity.JSONMap `json:"config"`
	ChatID  *string        `json:"chat_id"`
	AgentID *string        `json:"agent_id"`
}

// CreateChatChannel handles POST /chat-channels.
func (h *ChatChannelHandler) CreateChatChannel(c *gin.Context) {
	common.StartOperationLog(c, "create_chat_channel")
	defer common.FinishOperationLog(c)

	var req CreateChatChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		common.RespondError(c, common.CodeDataError, "Invalid request: "+err.Error())
		return
	}
	if req.Config == nil {
		req.Config = entity.JSONMap{}
	}
	req.Name = strings.TrimSpace(req.Name)
	if err := common.ValidateNameRunes(req.Name, common.NameLimit128); err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}
	if req.ChatID != nil && strings.TrimSpace(*req.ChatID) != "" && req.AgentID != nil && strings.TrimSpace(*req.AgentID) != "" {
		common.RespondError(c, common.CodeDataError, "chat_id and agent_id are mutually exclusive")
		return
	}

	ctx := c.Request.Context()

	row, err := h.chatChannelService.CreateChatChannel(
		ctx,
		req.Name,
		req.Channel,
		req.Config,
		req.ChatID,
		req.AgentID,
	)
	if err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}
	common.SetOperationResource(c, "channel_id", row.ID)
	common.RespondSuccess(c, row, "success")
}

// ListChatChannel handles GET /chat-channels.
func (h *ChatChannelHandler) ListChatChannel(c *gin.Context) {
	ctx := c.Request.Context()
	rows, err := h.chatChannelService.List(ctx)
	if err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		common.RespondError(c, common.CodeServerError, err.Error())
		return
	}
	common.RespondSuccessList(c, rows, "success")
}

// GetChatChannel handles GET /chat-channels/:channel_id.
func (h *ChatChannelHandler) GetChatChannel(c *gin.Context) {
	channelID := strings.TrimSpace(c.Param("channel_id"))
	if channelID == "" {
		common.RespondError(c, common.CodeArgumentError, "channel_id is required")
		return
	}

	ctx := c.Request.Context()
	channel, code, err := h.chatChannelService.GetChatChannel(ctx, channelID)
	if code != common.CodeSuccess || err != nil {
		writeChatChannelError(c, code, chatChannelErrMsg(code, err))
		return
	}

	common.RespondSuccess(c, channel, "success")
}

// UpdateChatChannel handles PATCH /chat-channels/:channel_id.
func (h *ChatChannelHandler) UpdateChatChannel(c *gin.Context) {
	common.StartOperationLog(c, "update_chat_channel")
	defer common.FinishOperationLog(c)

	channelID := strings.TrimSpace(c.Param("channel_id"))
	if channelID == "" {
		common.RespondError(c, common.CodeArgumentError, "channel_id is required")
		return
	}

	common.SetOperationResource(c, "channel_id", channelID)
	var request map[string]interface{}
	if err := c.ShouldBindJSON(&request); err != nil {
		common.RespondError(c, common.CodeDataError, err.Error())
		return
	}

	ctx := c.Request.Context()
	result, code, err := h.chatChannelService.UpdateChatChannel(ctx, channelID, unwrapChatChannelPayload(request))
	if code != common.CodeSuccess || err != nil {
		if respondPermissionErrorIf(c, err, true) {
			return
		}
		writeChatChannelError(c, code, chatChannelErrMsg(code, err))
		return
	}

	common.RespondSuccess(c, result, "success")
}

// GetChatChannelRuntime returns live runtime metadata for a running chat channel.
func (h *ChatChannelHandler) GetChatChannelRuntime(c *gin.Context) {
	channelID := strings.TrimSpace(c.Param("channel_id"))
	if channelID == "" {
		common.RespondError(c, common.CodeArgumentError, "channel_id is required")
		return
	}

	ctx := c.Request.Context()
	result, code, err := h.chatChannelService.GetChatChannelRuntime(ctx, channelID)
	if code != common.CodeSuccess || err != nil {
		writeChatChannelError(c, code, chatChannelErrMsg(code, err))
		return
	}
	common.RespondSuccess(c, result, "success")
}

// DeleteChatChannel handles DELETE /chat-channels/:channel_id.
func (h *ChatChannelHandler) DeleteChatChannel(c *gin.Context) {
	common.StartOperationLog(c, "delete_chat_channel")
	defer common.FinishOperationLog(c)

	channelID := strings.TrimSpace(c.Param("channel_id"))
	if channelID == "" {
		common.RespondError(c, common.CodeArgumentError, "channel_id is required")
		return
	}

	common.SetOperationResource(c, "channel_id", channelID)
	ctx := c.Request.Context()
	_, code, err := h.chatChannelService.DeleteChatChannel(ctx, channelID)
	if code != common.CodeSuccess || err != nil {
		writeChatChannelError(c, code, chatChannelErrMsg(code, err))
		return
	}

	common.RespondSuccess(c, nil, "success")
}

func unwrapChatChannelPayload(payload map[string]interface{}) map[string]interface{} {
	if data, ok := payload["data"].(map[string]interface{}); ok {
		return data
	}
	return payload
}

func writeChatChannelError(c *gin.Context, code common.ErrorCode, message string) {
	common.RespondError(c, code, message)
}

func chatChannelErrMsg(code common.ErrorCode, err error) string {
	if err != nil {
		return err.Error()
	}
	return code.Message()
}
