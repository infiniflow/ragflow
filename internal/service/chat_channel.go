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

package service

import (
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type ChatChannelService struct {
	chatChannelDao *dao.ChatChannelDAO
	chatDao        *dao.ChatDAO
	userTenantDao  *dao.UserTenantDAO
}

func NewChatChannelService() *ChatChannelService {
	return &ChatChannelService{
		chatChannelDao: dao.NewChatChannel(),
		chatDao:        dao.NewChatDAO(),
		userTenantDao:  dao.NewUserTenantDAO(),
	}
}

// accessible Return whether the user can access the chat channel's tenant.
func (s *ChatChannelService) accessible(userID, channelID string) (*entity.ChatChannel, bool, error) {
	channel, err := s.chatChannelDao.GetByIDOnly(channelID)
	if err != nil {
		if !dao.IsNotFoundErr(err) {
			return nil, false, err
		}
		return nil, false, nil
	}

	if channel.TenantID == userID {
		return channel, true, nil
	}

	tenantIDs, err := s.userTenantDao.GetTenantIDsByUserID(userID)
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

// GetChatChannel Return a chat channel bot's details when the current user can access it.
func (s *ChatChannelService) GetChatChannel(userID, channelID string) (*entity.ChatChannel, common.ErrorCode, error) {
	chatChannel, ok, err := s.accessible(userID, channelID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}
	if chatChannel == nil {
		return nil, common.CodeDataError, fmt.Errorf("Can't find this chat channel!")
	}
	return chatChannel, common.CodeSuccess, nil
}

// UpdateChatChannel Update an accessible chat channel bot's name/config/status.
func (s *ChatChannelService) UpdateChatChannel(userID, channelID string, req map[string]interface{}) (*entity.ChatChannel, common.ErrorCode, error) {
	channel, ok, err := s.accessible(userID, channelID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}
	if channel == nil {
		return nil, common.CodeDataError, fmt.Errorf("Can't find this chat channel!")
	}

	updates := map[string]interface{}{}

	if value, exists := req["name"]; exists {
		name, ok := value.(string)
		if !ok {
			return nil, common.CodeDataError, fmt.Errorf("name must be string")
		}
		updates["name"] = name
	}

	if value, exists := req["config"]; exists {
		if value == nil {
			updates["config"] = nil
		} else {
			config, ok := value.(map[string]interface{})
			if !ok {
				return nil, common.CodeDataError, fmt.Errorf("config must be object")
			}
			updates["config"] = config
		}
	}

	// Validate the connected dialog (if provided) belongs to the channel's tenant.
	if value, exists := req["chat_id"]; exists {
		if value == nil {
			updates["chat_id"] = nil
		} else {
			chatID, ok := value.(string)
			if !ok {
				return nil, common.CodeDataError, fmt.Errorf("chat_id must be string or null")
			}
			if chatID != "" {
				dialog, err := s.chatDao.GetByID(chatID)
				if err != nil {
					return nil, common.CodeDataError, fmt.Errorf("Can't find this chat assistant!")
				}
				if dialog.TenantID != channel.TenantID {
					return nil, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
				}
			}
			updates["chat_id"] = chatID
		}
	}

	if len(updates) > 0 {
		if err = s.chatChannelDao.UpdateByID(channelID, channel.TenantID, updates); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	updated, err := s.chatChannelDao.GetByIDOnly(channelID)
	if err != nil {
		return nil, common.CodeDataError, fmt.Errorf("Can't find this chat channel!")
	}
	return updated, common.CodeSuccess, nil
}

// DeleteChatChannel Delete an accessible chat channel bot.
func (s *ChatChannelService) DeleteChatChannel(userID, channelID string) (bool, common.ErrorCode, error) {
	channel, ok, err := s.accessible(userID, channelID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if !ok {
		return false, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}
	if channel == nil {
		return false, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}

	if err := s.chatChannelDao.DeleteByID(channelID, channel.TenantID); err != nil {
		return false, common.CodeDataError, err
	}
	return true, common.CodeSuccess, nil
}
