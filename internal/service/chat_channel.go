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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"gorm.io/gorm"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
)

type ChatChannelService struct {
	chatChannelDAO *dao.ChatChannelDAO
	chatDAO        *dao.ChatDAO
	chatSessionSvc *ChatSessionService
	agentRunner    chatChannelAgentRunner
}

func NewChatChannelService() *ChatChannelService {
	return &ChatChannelService{
		chatChannelDAO: dao.NewChatChannel(),
		chatDAO:        dao.NewChatDAO(),
		chatSessionSvc: NewChatSessionService(),
		agentRunner:    NewAgentService(),
	}
}

// NewChatChannelServiceWithAgentService shares the API's agent run infrastructure.
func NewChatChannelServiceWithAgentService(agentService *AgentService) *ChatChannelService {
	s := NewChatChannelService()
	s.agentRunner = agentService
	return s
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
	userID := permission.UserIDFromContext(ctx)
	tenantID, err := requireCurrentTenantID(ctx)
	if err != nil {
		return nil, err
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckFeature(ctx, "chat", permission.ActionWrite); err != nil {
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

	if provider := getChatChannelRuntimeProvider(); provider != nil {
		if snapshot, ok := provider(channelID); ok {
			return snapshot, common.CodeSuccess, nil
		}
	}
	return waitingRuntimeSnapshotMap(channelID), common.CodeSuccess, nil
}

// HandleIncomingMessage routes one external channel message through the bound RAGFlow dialog.
// When onDelta is non-nil the answer is generated as a stream and onDelta receives the
// cumulative display text as it grows; the returned string is always the complete answer.
func (s *ChatChannelService) HandleIncomingMessage(ctx context.Context, msg ChatChannelIncomingMessage, onDelta func(string)) (string, error) {
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
	if channel.Status != 1 {
		return "", nil
	}
	ownerCtx := permission.WithIdentity(ctx, channel.TenantID, channel.TenantID)
	if err := validateChannelBinding(ownerCtx, channel.ChatID, channel.AgentID); err != nil {
		return "", err
	}
	if channel.AgentID != nil {
		ctx, cancel := context.WithTimeout(ownerCtx, 5*time.Minute)
		defer cancel()
		return s.handleChannelAgentMessage(ctx, channel, msg)
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

	session, err := s.chatSessionSvc.GetOrCreateForChannel(ctx, dialog.ID, channel.ID, msg.ChatID, msg.Text, channel.Name)
	if err != nil {
		return "", err
	}

	stream := onDelta != nil
	var streamChan chan string
	var streamed chan channelStreamResult
	if stream {
		streamChan = make(chan string, 64)
		streamed = make(chan channelStreamResult, 1)
		go func() {
			streamed <- consumeChannelStream(streamChan, onDelta)
		}()
	}

	// Channel chat sessions are owned by the dialog tenant; authorization above
	// separately enforces the channel owner's current access to that dialog.
	completionCtx := permission.WithIdentity(ctx, dialog.TenantID, dialog.TenantID)
	result, err := s.chatSessionSvc.ChatCompletions(
		completionCtx,
		dialog.ID,
		session.ID,
		[]map[string]interface{}{{"role": "user", "content": msg.Text}},
		"",
		nil,
		"",
		nil,
		map[string]interface{}{"quote": false},
		false,
		stream,
		streamChan,
	)
	if stream {
		close(streamChan)
		consumed := <-streamed
		if err != nil {
			common.Error("chat channel completion failed", err, zap.String("channel_id", channel.ID))
			return "Sorry, I couldn't process your message right now.", nil
		}
		if consumed.err != nil {
			common.Warn("chat channel stream reported an error", zap.Error(consumed.err), zap.String("channel_id", channel.ID))
		}
		return consumed.answer, nil
	}
	if err != nil {
		common.Error("chat channel completion failed", err, zap.String("channel_id", channel.ID))
		return "Sorry, I couldn't process your message right now.", nil
	}
	if result == nil {
		return "", nil
	}
	answer, _ := result["answer"].(string)
	return answer, nil
}

// channelStreamResult is what consumeChannelStream reports back to its caller.
type channelStreamResult struct {
	answer string
	err    error
}

// consumeChannelStream drains the SSE frames ChatCompletions writes on frames and reports
// the cumulative display text through onDelta. Reasoning wrapped in <think>...</think> and
// the markers themselves are dropped, so the streamed text matches what the non-stream
// answer contains.
func consumeChannelStream(frames <-chan string, onDelta func(string)) channelStreamResult {
	var answer strings.Builder
	inThink := false
	for frame := range frames {
		payload, ok := strings.CutPrefix(frame, "data:")
		if !ok {
			continue
		}
		var wrapper struct {
			Code    int             `json:"code"`
			Message string          `json:"message"`
			Data    json.RawMessage `json:"data"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(payload)), &wrapper); err != nil {
			continue
		}
		if wrapper.Code != 0 {
			return channelStreamResult{answer: answer.String(), err: errors.New(wrapper.Message)}
		}
		var chunk map[string]interface{}
		if err := json.Unmarshal(wrapper.Data, &chunk); err != nil {
			// `data: true` closes the stream.
			continue
		}
		text, _ := chunk["answer"].(string)
		if final, _ := chunk["final"].(bool); final {
			// Adopt the final answer only when it extends the text already shown; a rewrite
			// would make a streaming channel repaint the whole message in one block.
			if text != "" && strings.HasPrefix(text, answer.String()) {
				answer.Reset()
				answer.WriteString(text)
				onDelta(answer.String())
			}
			continue
		}
		text = stripChannelThink(text, &inThink)
		if text == "" {
			continue
		}
		answer.WriteString(text)
		onDelta(answer.String())
	}
	return channelStreamResult{answer: answer.String()}
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
