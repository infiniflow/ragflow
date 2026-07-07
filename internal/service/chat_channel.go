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
	"ragflow/internal/utility"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type ChatChannelService struct {
	chatChannelDAO *dao.ChatChannelDAO
	chatDAO        *dao.ChatDAO
	userTenantDAO  *dao.UserTenantDAO
}

func NewChatChannelService() *ChatChannelService {
	return &ChatChannelService{
		chatChannelDAO: dao.NewChatChannel(),
		chatDAO:        dao.NewChatDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
	}
}

func (s *ChatChannelService) Insert(channel *entity.ChatChannel) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	if channel.ID == "" {
		channel.ID = utility.GenerateUUID()
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
	return s.chatChannelDAO.GetByIDOnly(id)
}

func (s *ChatChannelService) List(tenantID string) ([]*entity.ChatChannelListResponse, error) {
	return s.chatChannelDAO.ListByTenantID(tenantID)
}

func (s *ChatChannelService) CreateChatChannel(tenantID, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error) {
	if chatID != nil && *chatID != "" {
		dialog, err := s.chatDAO.GetByID(*chatID)
		if err != nil {
			if dao.IsNotFoundErr(err) {
				return nil, errors.New("Can't find this chat assistant!")
			}
			return nil, err
		}
		if dialog.TenantID != tenantID {
			return nil, errors.New("No authorization.")
		}
	}
	row := &entity.ChatChannel{
		ID:       utility.GenerateUUID(),
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

func (s *ChatChannelService) accessible(userID, channelID string) (*entity.ChatChannel, bool, error) {
	channel, err := s.chatChannelDAO.GetByIDOnly(channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	if channel.TenantID == userID {
		return channel, true, nil
	}

	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, false, err
	}
	for _, tenantID := range tenantIDs {
		if tenantID == channel.TenantID {
			return channel, true, nil
		}
	}

	return channel, false, nil
}

func (s *ChatChannelService) GetChatChannel(userID, channelID string) (*entity.ChatChannel, common.ErrorCode, error) {
	_, ok, err := s.accessible(userID, channelID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}

	channel, err := s.chatChannelDAO.GetByIDOnly(channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Can't find this chat channel!")
		}
		return nil, common.CodeServerError, err
	}
	return channel, common.CodeSuccess, nil
}

func (s *ChatChannelService) UpdateChatChannel(userID, channelID string, req map[string]interface{}) (*entity.ChatChannel, common.ErrorCode, error) {
	channel, ok, err := s.accessible(userID, channelID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}
	if channel == nil {
		return nil, common.CodeDataError, errors.New("Can't find this chat channel!")
	}

	updates := map[string]interface{}{}

	if value, exists := req["name"]; exists {
		name, ok := value.(string)
		if !ok {
			return nil, common.CodeDataError, errors.New("name must be string")
		}
		updates["name"] = name
	}

	if value, exists := req["config"]; exists {
		if value == nil {
			updates["config"] = nil
		} else {
			config, ok := value.(map[string]interface{})
			if !ok {
				return nil, common.CodeDataError, errors.New("config must be object")
			}
			updates["config"] = entity.JSONMap(config)
		}
	}

	if value, exists := req["chat_id"]; exists {
		if value == nil {
			updates["chat_id"] = nil
		} else {
			chatID, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, errors.New("chat_id must be string or null")
			}
			if chatID != "" {
				dialog, err := s.chatDAO.GetByID(chatID)
				if err != nil {
					if dao.IsNotFoundErr(err) {
						return nil, common.CodeDataError, errors.New("Can't find this chat assistant!")
					}
					return nil, common.CodeServerError, err
				}
				if dialog.TenantID != channel.TenantID {
					return nil, common.CodeAuthenticationError, errors.New("No authorization.")
				}
			}
			updates["chat_id"] = chatID
		}
	}

	if len(updates) > 0 {
		if err := s.chatChannelDAO.UpdateByID(channelID, channel.TenantID, updates); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	updated, err := s.chatChannelDAO.GetByIDOnly(channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("Can't find this chat channel!")
		}
		return nil, common.CodeServerError, err
	}
	return updated, common.CodeSuccess, nil
}

func (s *ChatChannelService) DeleteChatChannel(userID, channelID string) (bool, common.ErrorCode, error) {
	channel, ok, err := s.accessible(userID, channelID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if !ok {
		return false, common.CodeAuthenticationError, errors.New("No authorization.")
	}
	if channel == nil {
		return false, common.CodeAuthenticationError, errors.New("No authorization.")
	}

	if err := s.chatChannelDAO.DeleteByID(channelID, channel.TenantID); err != nil {
		return false, common.CodeDataError, err
	}
	return true, common.CodeSuccess, nil
}
