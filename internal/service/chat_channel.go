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
	"strings"
	"sync"

	"go.uber.org/zap"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

type ChatChannelService struct {
	chatChannelDAO *dao.ChatChannelDAO
	chatDAO        *dao.ChatDAO
	chatSessionSvc *ChatSessionService
}

func NewChatChannelService() *ChatChannelService {
	return &ChatChannelService{
		chatChannelDAO: dao.NewChatChannel(),
		chatDAO:        dao.NewChatDAO(),
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

// chatChannelAgentService answers channel messages for channels bound to a flow
// Agent. The server wires the production AgentService at boot; a nil value makes
// the agent path report an error instead of silently answering nothing.
var (
	chatChannelAgentServiceMu sync.RWMutex
	chatChannelAgentService   *AgentService
)

// SetChatChannelAgentService installs the agent runner used by chat channels.
func SetChatChannelAgentService(agentService *AgentService) {
	chatChannelAgentServiceMu.Lock()
	defer chatChannelAgentServiceMu.Unlock()
	chatChannelAgentService = agentService
}

func getChatChannelAgentService() *AgentService {
	chatChannelAgentServiceMu.RLock()
	defer chatChannelAgentServiceMu.RUnlock()
	return chatChannelAgentService
}

func (s *ChatChannelService) List(ctx context.Context, userID string) ([]*entity.ChatChannelListResponse, error) {
	channelIDs, err := accessibleChatChannelIDs(ctx, userID, permission.OperationRead)
	if err != nil {
		return nil, err
	}
	return s.chatChannelDAO.ListByIDs(ctx, dao.DB, channelIDs)
}

func (s *ChatChannelService) CreateChatChannel(ctx context.Context, userID, name, channelType string, config entity.JSONMap, chatID, agentID *string) (*entity.ChatChannel, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, permission.ErrUnauthenticated
	}
	chatID, agentID, err := resolveChannelTarget(ctx, userID, chatID, agentID)
	if err != nil {
		return nil, err
	}
	row := &entity.ChatChannel{
		ID:       common.GenerateUUID(),
		TenantID: userID,
		Name:     name,
		Channel:  channelType,
		Config:   config,
		ChatID:   chatID,
		AgentID:  agentID,
		Status:   1,
	}

	if err := s.chatChannelDAO.Create(ctx, dao.DB, row); err != nil {
		return nil, err
	}

	created, err := s.chatChannelDAO.GetByID(ctx, dao.DB, row.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to load created chat channel: %w", err)
	}
	return created, nil
}

func (s *ChatChannelService) GetChatChannel(ctx context.Context, userID, channelID string) (*entity.ChatChannel, common.ErrorCode, error) {
	if err := checkChatChannelPermission(ctx, userID, channelID, permission.OperationRead); err != nil {
		code, permissionErr := normalizeChatChannelPermissionError(err)
		return nil, code, permissionErr
	}

	channel, err := s.chatChannelDAO.GetByID(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := normalizeChatChannelPermissionError(permission.ErrResourceNotFound)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, err
	}
	return channel, common.CodeSuccess, nil
}

func (s *ChatChannelService) UpdateChatChannel(ctx context.Context, userID, channelID string, req map[string]interface{}) (*entity.ChatChannel, common.ErrorCode, error) {
	if err := checkChatChannelPermission(ctx, userID, channelID, permission.OperationUpdate); err != nil {
		code, permissionErr := normalizeChatChannelPermissionError(err)
		return nil, code, permissionErr
	}
	channel, err := s.chatChannelDAO.GetByID(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := normalizeChatChannelPermissionError(permission.ErrResourceNotFound)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, err
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

	chatTarget, hasChatTarget, err := optionalChannelTarget(req, "chat_id")
	if err != nil {
		return nil, common.CodeDataError, err
	}
	agentTarget, hasAgentTarget, err := optionalChannelTarget(req, "agent_id")
	if err != nil {
		return nil, common.CodeDataError, err
	}
	if hasChatTarget && hasAgentTarget && chatTarget != nil && agentTarget != nil {
		return nil, common.CodeDataError, ErrChatChannelTargetConflict
	}

	// A channel binds at most one target: selecting one clears the other.
	if hasChatTarget {
		if chatTarget == nil {
			updates["chat_id"] = nil
		} else {
			if err := checkChatPermission(ctx, nil, userID, *chatTarget, permission.OperationUse); err != nil {
				code, normalizedErr := normalizeChatPermissionError(err)
				return nil, code, normalizedErr
			}
			if channel.TenantID != userID {
				if err := checkChatPermission(ctx, nil, channel.TenantID, *chatTarget, permission.OperationUse); err != nil {
					code, normalizedErr := normalizeChatPermissionError(err)
					return nil, code, normalizedErr
				}
			}
			updates["chat_id"] = *chatTarget
			updates["agent_id"] = nil
		}
	}

	if hasAgentTarget {
		if agentTarget == nil {
			updates["agent_id"] = nil
		} else {
			if err := validateChannelAgentTarget(ctx, userID, *agentTarget); err != nil {
				code, normalizedErr := normalizeChatChannelPermissionError(err)
				return nil, code, normalizedErr
			}
			if channel.TenantID != userID {
				if err := validateChannelAgentTarget(ctx, channel.TenantID, *agentTarget); err != nil {
					code, normalizedErr := normalizeChatChannelPermissionError(err)
					return nil, code, normalizedErr
				}
			}
			updates["agent_id"] = *agentTarget
			updates["chat_id"] = nil
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
			code, permissionErr := normalizeChatChannelPermissionError(permission.ErrResourceNotFound)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, err
	}
	return updated, common.CodeSuccess, nil
}

// GetChatChannelRuntime returns the live runtime metadata for a WhatsApp chat channel.
func (s *ChatChannelService) GetChatChannelRuntime(ctx context.Context, userID, channelID string) (map[string]any, common.ErrorCode, error) {
	if err := checkChatChannelPermission(ctx, userID, channelID, permission.OperationUse); err != nil {
		code, permissionErr := normalizeChatChannelPermissionError(err)
		return nil, code, permissionErr
	}
	conn, err := s.chatChannelDAO.GetByID(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := normalizeChatChannelPermissionError(permission.ErrResourceNotFound)
			return nil, code, permissionErr
		}
		return nil, common.CodeServerError, err
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
// HandleIncomingMessage resolves the channel's bound target and returns the
// answer for one inbound message. A channel bound to a flow Agent runs the agent
// canvas; otherwise it falls back to the connected Chat. notifyThinking, when
// supplied, is invoked right before the completion starts so a channel can post
// an interim status message.
func (s *ChatChannelService) HandleIncomingMessage(ctx context.Context, msg ChatChannelIncomingMessage, notifyThinking ...func() error) (string, error) {
	if strings.TrimSpace(msg.Text) == "" {
		return "", nil
	}

	channel, err := s.chatChannelDAO.GetByID(ctx, dao.DB, msg.AccountID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return "", nil
		}
		return "", err
	}

	if agentID := derefChannelTarget(channel.AgentID); agentID != "" {
		return s.runChannelAgent(ctx, channel, agentID, msg, notifyThinking...)
	}

	chatID := derefChannelTarget(channel.ChatID)
	if chatID == "" {
		return "", nil
	}

	dialog, err := s.chatDAO.GetByID(ctx, dao.DB, chatID)
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

	notifyThinkingMessage(notifyThinking)

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
		false,
		nil,
	)
	if err != nil {
		common.Error("chat channel completion failed", err, zap.String("channel_id", channel.ID))
		return "Sorry, I couldn't process your message right now.", nil
	}
	if result == nil {
		return "", nil
	}
	answer, _ := result["answer"].(string)
	return stripReasoningContent(answer), nil
}

func (s *ChatChannelService) DeleteChatChannel(ctx context.Context, userID, channelID string) (bool, common.ErrorCode, error) {
	if err := checkChatChannelPermission(ctx, userID, channelID, permission.OperationDelete); err != nil {
		code, permissionErr := normalizeChatChannelPermissionError(err)
		return false, code, permissionErr
	}
	channel, err := s.chatChannelDAO.GetByID(ctx, dao.DB, channelID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			code, permissionErr := normalizeChatChannelPermissionError(permission.ErrResourceNotFound)
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

// ErrChatChannelTargetConflict rejects binding a Chat and an Agent to one channel.
var ErrChatChannelTargetConflict = errors.New("choose either a chat assistant or an agent, not both")

// runChannelAgent answers one inbound message with the flow Agent bound to the channel.
func (s *ChatChannelService) runChannelAgent(ctx context.Context, channel *entity.ChatChannel, agentID string, msg ChatChannelIncomingMessage, notifyThinking ...func() error) (string, error) {
	agentService := getChatChannelAgentService()
	if agentService == nil {
		common.Warn("chat channel agent runtime is not configured", zap.String("channel_id", channel.ID))
		return "", nil
	}
	if err := validateChannelAgentTarget(ctx, channel.TenantID, agentID); err != nil {
		common.Warn("chat channel agent is unavailable", zap.String("channel_id", channel.ID), zap.String("agent_id", agentID), zap.Error(err))
		return "", nil
	}

	notifyThinkingMessage(notifyThinking)

	// Keep one stable agent session per (agent, channel, end user chat) so the
	// conversation history survives restarts and channel rebinding.
	sessionID := channelAgentSessionID(agentID, channel.ID, msg.ChatID)
	events, err := agentService.RunAgent(WithAgentSessionID(ctx, sessionID), channel.TenantID, agentID, sessionID, "", msg.Text, nil)
	if err != nil {
		common.Error("chat channel agent run failed", err, zap.String("channel_id", channel.ID), zap.String("agent_id", agentID))
		return "Sorry, I couldn't process your message right now.", nil
	}

	var answer strings.Builder
	for event := range events {
		if event.Type != "message" {
			continue
		}
		var data map[string]any
		if err := json.Unmarshal([]byte(event.Data), &data); err != nil {
			continue
		}
		if content, ok := data["content"].(string); ok {
			answer.WriteString(content)
		}
		// The canvas flags reasoning boundaries instead of inlining the markers;
		// wrap them the same way the Python bridge did so the block is stripped.
		if start, _ := data["start_to_think"].(bool); start {
			answer.WriteString("<think>")
		} else if end, _ := data["end_to_think"].(bool); end {
			answer.WriteString("</think>")
		}
	}
	return stripReasoningContent(answer.String()), nil
}

// notifyThinkingMessage posts the interim "thinking" message when the channel supplied one.
func notifyThinkingMessage(notifiers []func() error) {
	for _, notify := range notifiers {
		if notify == nil {
			continue
		}
		if err := notify(); err != nil {
			common.Warn("chat channel thinking placeholder failed", zap.Error(err))
		}
		return
	}
}

// stripReasoningContent mirrors the Python bridge: everything through the last
// </think> marker is model reasoning and must not reach the end user.
func stripReasoningContent(txt string) string {
	if txt == "" {
		return txt
	}
	first := strings.Index(txt, "<think>")
	if first < 0 {
		return txt
	}
	last := strings.LastIndex(txt, "</think>")
	if last < 0 || last < first {
		return txt
	}
	return txt[last+len("</think>"):]
}

// channelAgentSessionID derives the stable session id for one (agent, channel, chat).
func channelAgentSessionID(agentID, channelID, chatID string) string {
	sum := sha256.Sum256([]byte("agent:" + agentID + ":" + channelID + ":" + chatID))
	return hex.EncodeToString(sum[:])[:32]
}

func derefChannelTarget(value *string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(*value)
}

// optionalChannelTarget extracts an optional binding id from an update payload.
// A null or blank value clears the binding; a non-string value is rejected.
func optionalChannelTarget(req map[string]interface{}, key string) (*string, bool, error) {
	value, exists := req[key]
	if !exists {
		return nil, false, nil
	}
	if value == nil {
		return nil, true, nil
	}
	text, ok := value.(string)
	if !ok {
		return nil, true, fmt.Errorf("%s must be string or null", key)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, true, nil
	}
	return &text, true, nil
}

// resolveChannelTarget validates the create-time binding pair. A channel binds
// either a Chat or a flow Agent, never both.
func resolveChannelTarget(ctx context.Context, userID string, chatID, agentID *string) (*string, *string, error) {
	chatID = trimChannelTargetID(chatID)
	agentID = trimChannelTargetID(agentID)
	if chatID != nil && agentID != nil {
		return nil, nil, ErrChatChannelTargetConflict
	}
	if chatID != nil {
		if err := checkChatPermission(ctx, nil, userID, *chatID, permission.OperationUse); err != nil {
			return nil, nil, err
		}
	}
	if agentID != nil {
		if err := validateChannelAgentTarget(ctx, userID, *agentID); err != nil {
			return nil, nil, err
		}
	}
	return chatID, agentID, nil
}

func trimChannelTargetID(id *string) *string {
	if id == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*id)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

// validateChannelAgentTarget checks the subject may use the canvas and that the
// canvas really is a flow Agent; pipelines and other canvases cannot answer a channel.
func validateChannelAgentTarget(ctx context.Context, userID, agentID string) error {
	if err := CheckCanvasPermission(ctx, permission.Subject{UserID: userID}, agentID, permission.OperationUse); err != nil {
		return err
	}
	canvas, err := dao.NewUserCanvasDAO().GetByID(ctx, dao.DB, agentID)
	if err != nil {
		return err
	}
	if canvas == nil || canvas.CanvasCategory != "agent_canvas" {
		return dao.ErrUserCanvasNotFound
	}
	return nil
}
