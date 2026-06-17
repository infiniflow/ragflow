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
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"github.com/gin-gonic/gin"
)

type ChatChannelHandler struct {
	chatChannelService ChatChannelService
}

type ChatChannelService interface {
	CreateChatChannel(tenantID, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error)
	List(tenantID string) ([]*entity.ChatChannelListResponse, error)
}

func NewChatChannel() *ChatChannelHandler {
	return &ChatChannelHandler{
		chatChannelService: service.NewChatChannelService(),
	}
}

type CreateChatChannelRequest struct {
	Name    string         `json:"name" binding:"required"`
	Channel string         `json:"channel" binding:"required"`
	Config  entity.JSONMap `json:"config" binding:"required"`
	ChatID  *string        `json:"chat_id"`
}

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
