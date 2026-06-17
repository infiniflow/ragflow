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

package service

import (
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// ChatChannelService mirrors the Python ChatChannelService/CommonService
// behavior needed by the chat-channel create/list APIs.
type ChatChannelService struct {
	chatChannelDAO *dao.ChatChannelDAO
	userTenantDAO  *dao.UserTenantDAO
}

// NewChatChannelService creates a chat channel service.
func NewChatChannelService() *ChatChannelService {
	return &ChatChannelService{
		chatChannelDAO: dao.NewChatChannel(),
		userTenantDAO:  dao.NewUserTenantDAO(),
	}
}

func (s *ChatChannelService) Insert(channel *entity.ChatChannel) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	if channel.ID == "" {
		channel.ID = common.GenerateUUID()
	}
	if channel.Status == 0 {
		channel.Status = 1
	}
	return s.chatChannelDAO.Create(channel)
}

func (s *ChatChannelService) GetByID(id string) (*entity.ChatChannel, error) {
	if id == "" {
		return nil, errors.New("id is empty")
	}

	var channel entity.ChatChannel
	if err := dao.DB.Where("id = ?", id).First(&channel).Error; err != nil {
		return nil, err
	}
	return &channel, nil
}

// List returns the chat channels owned by one tenant.
// ChatChannelService.list(current_user.id)
func (s *ChatChannelService) List(tenantID string) ([]*entity.ChatChannelListResponse, error) {
	return s.chatChannelDAO.ListByTenantID(tenantID)
}

// CreateChatChannel is a convenience wrapper for the Python create endpoint.
func (s *ChatChannelService) CreateChatChannel(tenantID string, name string, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error) {
	row := &entity.ChatChannel{
		ID:       common.GenerateUUID(),
		TenantID: tenantID,
		Name:     name,
		Channel:  channelType,
		Config:   config,
		ChatID:   chatID,
		Status:   1,
	}

	if err := s.Insert(row); err != nil {
		return nil, err
	}
	created, err := s.GetByID(row.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load created chat channel: %w", err)
	}
	return created, nil
}
