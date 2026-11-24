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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service/billing"

	"gorm.io/gorm"
)

type ChatChannelService struct {
	chatChannelDAO *dao.ChatChannelDAO
}

func NewChatChannelService() *ChatChannelService {
	return &ChatChannelService{
		chatChannelDAO: dao.NewChatChannel(),
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
type ChatChannelRuntimeProvider func(context.Context, *entity.ChatChannel) (map[string]any, bool)

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

func (s *ChatChannelService) List(ctx context.Context) ([]*entity.ChatChannelListResponse, error) {
	channelScope, err := permission.NewDatabaseChecker(dao.DB).Scope(ctx, permission.ScopeQuery{
		Kind:      permission.ResourceKindChatChannel,
		Operation: permission.OperationRead,
	})
	if err != nil {
		return nil, err
	}
	channelIDs := channelScope.ResourceIDs
	if channelScope.Mode != permission.ScopeIDs {
		channelIDs = []string{}
	}
	return s.chatChannelDAO.ListByIDs(ctx, dao.DB, channelIDs)
}

func (s *ChatChannelService) CreateChatChannel(ctx context.Context, name, channelType string, config entity.JSONMap, chatID, agentID *string) (*entity.ChatChannel, error) {
	if err := permission.NewDatabaseChecker(dao.DB).CheckFeature(ctx, "chat_channel", permission.ActionEnable); err != nil {
		return nil, err
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckFeature(ctx, permission.Feature("chat_channel::"+strings.TrimSpace(channelType)), permission.ActionEnable); err != nil {
		return nil, err
	}
	userID := permission.UserIDFromContext(ctx)
	tenantID, err := requireCurrentTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckFeature(ctx, "chat", permission.ActionWrite); err != nil {
		return nil, err
	}
	if allowed, err := billing.Get().APIAuthAllowed(ctx, tenantID); !allowed {
		if err == nil {
			err = errors.New("access is not allowed")
		}
		return nil, err
	}
	chatID, agentID = normalizedChannelTarget(chatID), normalizedChannelTarget(agentID)
	if err := validateChannelBinding(ctx, chatID, agentID); err != nil {
		return nil, err
	}
	if err := validateChannelBinding(permission.WithIdentity(ctx, tenantID, tenantID), chatID, agentID); err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if err := common.ValidateNameRunes(name, common.NameLimit128); err != nil {
		return nil, err
	}
	if _, err := FirstChatChannelAPIKey(ctx, tenantID); err != nil {
		return nil, err
	}
	row := &entity.ChatChannel{
		ID:       common.GenerateUUID(),
		UserID:   userID,
		TenantID: tenantID,
		Name:     name,
		Channel:  channelType,
		Config:   config,
		ChatID:   chatID,
		AgentID:  agentID,
		Status:   1,
	}

	if err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.chatChannelDAO.Create(ctx, tx, row); err != nil {
			return err
		}
		return permission.NewDatabaseChecker(tx).GrantResourceOwner(ctx, permission.ResourceRef{Kind: permission.ResourceKindChatChannel, ID: row.ID})
	}); err != nil {
		return nil, err
	}

	created, err := s.chatChannelDAO.GetByID(ctx, dao.DB, row.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load created chat channel: %w", err)
	}
	return created, nil
}

func (s *ChatChannelService) GetChatChannel(ctx context.Context, channelID string) (*entity.ChatChannel, common.ErrorCode, error) {
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindChatChannel, ID: channelID}, permission.OperationRead); err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		return nil, code, permissionErr
	}

	channel, err := s.chatChannelDAO.GetByID(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, err
	}
	return channel, common.CodeSuccess, nil
}

func (s *ChatChannelService) UpdateChatChannel(ctx context.Context, channelID string, req map[string]interface{}) (*entity.ChatChannel, common.ErrorCode, error) {
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindChatChannel, ID: channelID}, permission.OperationUpdate); err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		return nil, code, permissionErr
	}
	channel, err := s.chatChannelDAO.GetByID(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, err
	}

	if allowed, err := billing.Get().APIAuthAllowed(ctx, channel.TenantID); !allowed {
		if err == nil {
			err = errors.New("access is not allowed")
		}
		return nil, common.CodeForbidden, err
	}

	updates := map[string]interface{}{}

	if value, exists := req["name"]; exists {
		name, ok := value.(string)
		if !ok {
			return nil, common.CodeDataError, errors.New("name must be string")
		}
		name = strings.TrimSpace(name)
		if err := common.ValidateNameRunes(name, common.NameLimit128); err != nil {
			return nil, common.CodeDataError, err
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

	chatID, agentID := channel.ChatID, channel.AgentID
	bindingChanged := false
	for _, field := range []string{"chat_id", "agent_id"} {
		if value, exists := req[field]; exists {
			bindingChanged = true
			var id *string
			if value != nil {
				text, ok := value.(string)
				if !ok {
					return nil, common.CodeDataError, fmt.Errorf("%s must be string or null", field)
				}
				id = normalizedChannelTarget(&text)
			}
			if field == "chat_id" {
				chatID = id
			} else {
				agentID = id
			}
		}
	}
	if bindingChanged {
		if chatID != nil && agentID != nil {
			return nil, common.CodeDataError, errors.New("chat_id and agent_id are mutually exclusive")
		}
		for _, bindingCtx := range []context.Context{ctx, permission.WithIdentity(ctx, channel.TenantID, channel.TenantID)} {
			if err := validateChannelBinding(bindingCtx, chatID, agentID); err != nil {
				code, normalizedErr := permissionresponse.NormalizeHidden(err)
				return nil, code, normalizedErr
			}
		}
		updates["chat_id"], updates["agent_id"] = chatID, agentID
	}

	if channel.Status == 1 {
		if _, err := FirstChatChannelAPIKey(ctx, channel.TenantID); err != nil {
			return nil, common.CodeDataError, err
		}
	}
	if len(updates) > 0 {
		if err = s.chatChannelDAO.UpdateByID(ctx, dao.DB, channelID, channel.TenantID, updates); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	updated, err := s.chatChannelDAO.GetByID(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, err
	}
	return updated, common.CodeSuccess, nil
}

// GetChatChannelRuntime returns the live runtime metadata for a WhatsApp chat channel.
func (s *ChatChannelService) GetChatChannelRuntime(ctx context.Context, channelID string) (map[string]any, common.ErrorCode, error) {
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindChatChannel, ID: channelID}, permission.OperationUse); err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		return nil, code, permissionErr
	}
	conn, err := s.chatChannelDAO.GetByID(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, err
	}

	if conn.Channel != "whatsapp" {
		return nil, common.CodeDataError, errors.New("runtime snapshot is only available for WhatsApp")
	}

	if conn.Status != 1 {
		return waitingRuntimeSnapshotMap(channelID), common.CodeSuccess, nil
	}
	if provider := getChatChannelRuntimeProvider(); provider != nil {
		if snapshot, ok := provider(ctx, conn); ok {
			return snapshot, common.CodeSuccess, nil
		}
	}
	return waitingRuntimeSnapshotMap(channelID), common.CodeSuccess, nil
}

// ChatChannelRuntimeFingerprint identifies the configuration that owns a runtime snapshot.
func ChatChannelRuntimeFingerprint(channel string, credential map[string]any) string {
	payload, _ := json.Marshal(map[string]any{"channel": channel, "credential": credential})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// Channels cannot render references. Filter every cumulative snapshot as well as
// the final answer so split markers never reach a platform or its fallback send.
var channelCitationPattern = regexp.MustCompile(`(?i)[\[(【]\s*\**\s*ID\s*[:：][^\[\]()【】\n]*(?:[\])】]|$)|\[[0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]+\]`)
var channelCitationTailPattern = regexp.MustCompile(`(?i)[\[(【]\s*\**\s*(?:I(?:D\s*)?|S(?:L(?:O(?:T\s*[0-9]*)?)?)?|[0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]*)$|#{1,2}(?:[0-9]+\$?)?$|\bref\s*[0-9\x{0660}-\x{0669}\x{06F0}-\x{06F9}]*$`)

var channelCitationWordTailPattern = regexp.MustCompile(`(?i)\br(?:e)?$`)

func stripChannelCitations(text string, streaming bool) string {
	if streaming {
		text = channelCitationWordTailPattern.ReplaceAllString(text, "")
	}
	text = channelCitationTailPattern.ReplaceAllString(text, "")
	text = stripCitations(text)
	return channelCitationPattern.ReplaceAllString(text, "")
}

// stripChannelThink removes the <think>...</think> reasoning block from a delta. The
// closing marker can share a delta with the answer text that follows it.
func stripChannelThink(text string, inThink *bool) string {
	for {
		if *inThink {
			idx := strings.Index(text, "</think>")
			if idx < 0 {
				return ""
			}
			text = text[idx+len("</think>"):]
			*inThink = false
			continue
		}
		idx := strings.Index(text, "<think>")
		if idx < 0 {
			break
		}
		text = text[idx+len("<think>"):]
		*inThink = true
	}
	if *inThink {
		return ""
	}
	return text
}

func (s *ChatChannelService) DeleteChatChannel(ctx context.Context, channelID string) (bool, common.ErrorCode, error) {
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindChatChannel, ID: channelID}, permission.OperationDelete); err != nil {
		code, permissionErr := permissionresponse.NormalizeHidden(err)
		return false, code, permissionErr
	}
	channel, err := s.chatChannelDAO.GetByID(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := permissionresponse.NormalizeHidden(permission.ErrResourceNotFound)
			return false, code, permissionErr
		}
		return false, common.CodeServerError, err
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
