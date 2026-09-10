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
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	"ragflow/internal/utility"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type ChatChannelService struct {
	chatChannelDAO *dao.ChatChannelDAO
	chatDAO        *dao.ChatDAO
	userTenantDAO  *dao.UserTenantDAO
	chatSessionSvc *ChatSessionService
}

func NewChatChannelService() *ChatChannelService {
	return &ChatChannelService{
		chatChannelDAO: dao.NewChatChannel(),
		chatDAO:        dao.NewChatDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
		chatSessionSvc: NewChatSessionService(),
	}
}

type ChatChannelIncomingMessage struct {
	Channel   string
	AccountID string
	ChatID    string
	ChatType  string
	MessageID string
	SenderID  string
	Text      string
}

// ChatChannelRuntimeProvider reads runtime metadata for one live chat-channel instance.
type ChatChannelRuntimeProvider func(channelID string) (map[string]any, bool)

var (
	chatChannelRuntimeProviderMu sync.RWMutex
	chatChannelRuntimeProvider   ChatChannelRuntimeProvider
)

// SetChatChannelRuntimeProvider installs the runtime snapshot reader used by the chat-channel API.
func SetChatChannelRuntimeProvider(provider ChatChannelRuntimeProvider) {
	chatChannelRuntimeProviderMu.Lock()
	defer chatChannelRuntimeProviderMu.Unlock()
	chatChannelRuntimeProvider = provider
}

// getChatChannelRuntimeProvider returns the current runtime snapshot reader.
func getChatChannelRuntimeProvider() ChatChannelRuntimeProvider {
	chatChannelRuntimeProviderMu.RLock()
	defer chatChannelRuntimeProviderMu.RUnlock()
	return chatChannelRuntimeProvider
}

func (s *ChatChannelService) Insert(ctx context.Context, channel *entity.ChatChannel) error {
	if channel == nil {
		return errors.New("channel is nil")
	}
	if channel.ID == "" {
		channel.ID = utility.GenerateUUID()
	}
	if channel.Status == 0 {
		channel.Status = 1
	}
	return s.chatChannelDAO.Create(ctx, dao.DB, channel)
}

func (s *ChatChannelService) GetByID(ctx context.Context, id string) (*entity.ChatChannel, error) {
	if id == "" {
		return nil, errors.New("id is empty")
	}
	return s.chatChannelDAO.GetByIDOnly(ctx, dao.DB, id)
}

func (s *ChatChannelService) List(ctx context.Context, tenantID string) ([]*entity.ChatChannelListResponse, error) {
	return s.chatChannelDAO.ListByTenantID(ctx, dao.DB, tenantID)
}

func (s *ChatChannelService) CreateChatChannel(ctx context.Context, tenantID, name, channelType string, config entity.JSONMap, chatID *string) (*entity.ChatChannel, error) {
	if chatID != nil && *chatID != "" {
		dialog, err := s.chatDAO.GetByID(ctx, dao.DB, *chatID)
		if err != nil {
			if dao.IsNotFoundErr(err) {
				return nil, errors.New("can't find this chat assistant")
			}
			return nil, err
		}
		if dialog.TenantID != tenantID {
			return nil, errors.New("no authorization")
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

	if err := s.Insert(ctx, row); err != nil {
		return nil, err
	}

	created, err := s.GetByID(ctx, row.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load created chat channel: %w", err)
	}
	return created, nil
}

func (s *ChatChannelService) accessible(ctx context.Context, userID, channelID string) (*entity.ChatChannel, bool, error) {
	channel, err := s.chatChannelDAO.GetByIDOnly(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	if channel.TenantID == userID {
		return channel, true, nil
	}

	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(ctx, dao.DB, userID)
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

func (s *ChatChannelService) GetChatChannel(ctx context.Context, userID, channelID string) (*entity.ChatChannel, common.ErrorCode, error) {
	_, ok, err := s.accessible(ctx, userID, channelID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, errors.New("no authorization")
	}

	channel, err := s.chatChannelDAO.GetByIDOnly(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("can't find this chat channel")
		}
		return nil, common.CodeServerError, err
	}
	return channel, common.CodeSuccess, nil
}

func (s *ChatChannelService) UpdateChatChannel(ctx context.Context, userID, channelID string, req map[string]interface{}) (*entity.ChatChannel, common.ErrorCode, error) {
	channel, ok, err := s.accessible(ctx, userID, channelID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, errors.New("no authorization")
	}
	if channel == nil {
		return nil, common.CodeDataError, errors.New("can't find this chat channel")
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
				dialog, err := s.chatDAO.GetByID(ctx, dao.DB, chatID)
				if err != nil {
					if dao.IsNotFoundErr(err) {
						return nil, common.CodeDataError, errors.New("can't find this chat assistant")
					}
					return nil, common.CodeServerError, err
				}
				if dialog.TenantID != channel.TenantID {
					return nil, common.CodeAuthenticationError, errors.New("no authorization")
				}
			}
			updates["chat_id"] = chatID
		}
	}

	if len(updates) > 0 {
		if err = s.chatChannelDAO.UpdateByID(ctx, dao.DB, channelID, channel.TenantID, updates); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	updated, err := s.chatChannelDAO.GetByIDOnly(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, common.CodeDataError, errors.New("can't find this chat channel")
		}
		return nil, common.CodeServerError, err
	}
	return updated, common.CodeSuccess, nil
}

// GetChatChannelRuntime returns the live runtime metadata for a WhatsApp chat channel.
func (s *ChatChannelService) GetChatChannelRuntime(ctx context.Context, userID, channelID string) (map[string]any, common.ErrorCode, error) {
	conn, ok, err := s.accessible(ctx, userID, channelID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, errors.New("no authorization")
	}
	if conn == nil {
		return nil, common.CodeDataError, errors.New("can't find this chat channel")
	}

	if conn.Channel != "whatsapp" {
		return nil, common.CodeDataError, errors.New("runtime snapshot is only available for WhatsApp")
	}

	if provider := getChatChannelRuntimeProvider(); provider != nil {
		if snapshot, ok := provider(channelID); ok {
			return snapshot, common.CodeSuccess, nil
		}
	}
	return waitingRuntimeSnapshotMap(channelID), common.CodeSuccess, nil
}

// HandleIncomingMessage routes one external channel message through the bound RAGFlow dialog.
func (s *ChatChannelService) HandleIncomingMessage(ctx context.Context, msg ChatChannelIncomingMessage) (string, error) {
	if strings.TrimSpace(msg.Text) == "" {
		return "", nil
	}

	channel, err := s.chatChannelDAO.GetByIDOnly(ctx, dao.DB, msg.AccountID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return "", nil
		}
		return "", err
	}
	if channel.ChatID == nil || strings.TrimSpace(*channel.ChatID) == "" {
		return "", nil
	}

	dialog, err := s.chatDAO.GetByID(ctx, dao.DB, *channel.ChatID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return "", nil
		}
		return "", err
	}

	session, err := s.chatSessionSvc.GetOrCreateForChannel(ctx, dialog.ID, channel.ID, msg.ChatID)
	if err != nil {
		return "", err
	}

	result, err := s.chatSessionSvc.ChatCompletions(
		ctx,
		dialog.TenantID,
		dialog.ID,
		session.ID,
		[]map[string]interface{}{{"role": "user", "content": msg.Text}},
		"",
		nil,
		"",
		nil,
		map[string]interface{}{"quote": false},
		false,
		true,
		false,
		false,
		nil,
	)
	if err != nil {
		log.Printf("chat channel %s completion failed: %v", channel.ID, err)
		return "Sorry, I couldn't process your message right now.", nil
	}
	if result == nil {
		return "", nil
	}
	answer, _ := result["answer"].(string)
	return answer, nil
}

func (s *ChatChannelService) DeleteChatChannel(ctx context.Context, userID, channelID string) (bool, common.ErrorCode, error) {
	channel, ok, err := s.accessible(ctx, userID, channelID)
	if err != nil {
		return false, common.CodeServerError, err
	}
	if !ok {
		return false, common.CodeAuthenticationError, errors.New("no authorization")
	}
	if channel == nil {
		return false, common.CodeAuthenticationError, errors.New("no authorization")
	}

	if err = s.chatChannelDAO.DeleteByID(ctx, dao.DB, channelID, channel.TenantID); err != nil {
		return false, common.CodeDataError, err
	}
	return true, common.CodeSuccess, nil
}

// waitingRuntimeSnapshotMap returns the API fallback payload before a runtime instance starts.
func waitingRuntimeSnapshotMap(channelID string) map[string]any {
	return map[string]any{
		"account_id":       channelID,
		"session_key":      channelID,
		"status":           "waiting",
		"connected_at":     nil,
		"qr_updated_at":    nil,
		"qr_data_url":      nil,
		"last_error":       nil,
		"session_id":       nil,
		"last_snapshot_at": nil,
		"gateway_base_url": nil,
		"event_cursor":     int64(0),
	}
}
