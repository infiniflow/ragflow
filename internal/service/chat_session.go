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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/storage"
	"ragflow/internal/utility"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// Interfaces for testability — satisfied by the concrete DAO/pipeline types.

type chatSessionStore interface {
	GetByID(id string) (*entity.ChatSession, error)
	GetBySessionIDAndChatID(sessionID, chatID string) (*entity.ChatSession, error)
	Create(conv *entity.ChatSession) error
	UpdateByID(id string, updates map[string]interface{}) error
	DeleteByID(id string) error
	ListByChatID(chatID string) ([]*entity.ChatSession, error)
	GetDialogByID(chatID string) (*entity.Chat, error)
	CheckDialogExists(tenantID, chatID string) (bool, error)
}

type userTenantStore interface {
	GetTenantIDsByUserID(userID string) ([]string, error)
}

type chatPipelineRunner interface {
	AsyncChat(ctx context.Context, userID string, chat *entity.Chat, messages []map[string]interface{}, stream bool, kwargs map[string]interface{}) (<-chan AsyncChatResult, error)
}

// chunkFeedbackApplier is the dispatch seam for chunk-level feedback
// persistence. Mirrors the Python ChunkFeedbackService.apply_feedback
// (api/db/services/chunk_feedback_service.py) call site at
// api/apps/restful_apis/chat_api.py — that handler records the thumb
// vote against every chunk that produced the assistant message, in
// addition to the session-level thumbup field. Production uses
// *ChatSessionService itself via applyChunkFeedback; tests can swap
// in a fake by setting ChatSessionService.chunkFeedbackApplier.
type chunkFeedbackApplier interface {
	applyChunkFeedback(ctx context.Context, tenantID string, reference map[string]interface{}, isPositive bool) (map[string]interface{}, error)
}

type chunkPagerankAdjuster interface {
	AdjustChunkPagerank(ctx context.Context, indexName, chunkID, kbID string, delta, minWeight, maxWeight float64) error
}

// ChatSessionService chat session (conversation) service.
// The RAG pipeline is delegated to ChatPipelineService.
type ChatSessionService struct {
	chatSessionDAO       chatSessionStore
	userTenantDAO        userTenantStore
	pipeline             chatPipelineRunner
	chunkFeedbackApplier chunkFeedbackApplier
	docEngine            engine.DocEngine
}

// NewChatSessionService create chat session service
func NewChatSessionService() *ChatSessionService {
	return &ChatSessionService{
		chatSessionDAO: dao.NewChatSessionDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
		pipeline:       NewChatPipelineService(),
		docEngine:      engine.Get(),
	}
}

// SetChatSessionRequest set chat session request.
type SetChatSessionRequest struct {
	SessionID string `json:"conversation_id,omitempty"`
	DialogID  string `json:"dialog_id,omitempty"`
	Name      string `json:"name,omitempty"`
	IsNew     bool   `json:"is_new"`
}

// SetChatSessionResponse set chat session response.
type SetChatSessionResponse struct {
	*entity.ChatSession
}

// SetChatSession creates or updates a chat session.
// Kept as a compatibility entrypoint for older chat-session callers.
func (s *ChatSessionService) SetChatSession(userID string, req *SetChatSessionRequest) (*SetChatSessionResponse, error) {
	name := req.Name
	if name == "" {
		name = "New chat session"
	}
	if len(name) > 255 {
		name = name[:255]
	}

	if !req.IsNew {
		updates := map[string]interface{}{
			"name":    name,
			"user_id": userID,
		}
		if err := s.chatSessionDAO.UpdateByID(req.SessionID, updates); err != nil {
			return nil, errors.New("Chat session not found")
		}
		session, err := s.chatSessionDAO.GetByID(req.SessionID)
		if err != nil {
			return nil, errors.New("Fail to update a chat session")
		}
		return &SetChatSessionResponse{ChatSession: session}, nil
	}

	dialog, err := s.chatSessionDAO.GetDialogByID(req.DialogID)
	if err != nil {
		return nil, errors.New("Dialog not found")
	}

	prologue := "Hi! I'm your assistant. What can I do for you?"
	if dialog.PromptConfig != nil {
		if p, ok := dialog.PromptConfig["prologue"].(string); ok && p != "" {
			prologue = p
		}
	}
	messagesJSON, _ := json.Marshal([]map[string]interface{}{
		{
			"role":    "assistant",
			"content": prologue,
		},
	})
	referenceJSON, _ := json.Marshal([]interface{}{})

	session := &entity.ChatSession{
		ID:        utility.GenerateUUID(),
		DialogID:  req.DialogID,
		Name:      &name,
		Message:   messagesJSON,
		UserID:    &userID,
		Reference: referenceJSON,
	}
	if err := s.chatSessionDAO.Create(session); err != nil {
		return nil, errors.New("Fail to create a chat session")
	}

	return &SetChatSessionResponse{ChatSession: session}, nil
}

// RemoveChatSessions removes chat sessions.
// Kept as a compatibility entrypoint for older chat-session callers.
func (s *ChatSessionService) RemoveChatSessions(userID string, chatSessions []string) error {
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return err
	}

	tenantIDSet := make(map[string]bool)
	for _, tid := range tenantIDs {
		tenantIDSet[tid] = true
	}
	tenantIDSet[userID] = true

	for _, convID := range chatSessions {
		session, err := s.chatSessionDAO.GetByID(convID)
		if err != nil {
			return fmt.Errorf("Chat session not found: %s", convID)
		}

		isOwner := false
		for tenantID := range tenantIDSet {
			exists, err := s.chatSessionDAO.CheckDialogExists(tenantID, session.DialogID)
			if err != nil {
				return err
			}
			if exists {
				isOwner = true
				break
			}
		}
		if !isOwner {
			return errors.New("Only owner of chat session authorized for this operation")
		}

		if err := s.chatSessionDAO.DeleteByID(convID); err != nil {
			return err
		}
	}

	return nil
}

// ListChatSessionsRequest list chat sessions request.
type ListChatSessionsRequest struct {
	DialogID string `json:"dialog_id" binding:"required"`
}

// ListChatSessionsResponse list chat sessions response
type ListChatSessionsResponse struct {
	Sessions []*entity.ChatSession
}

type ChatSessionPayload struct {
	ID         string                   `json:"id"`
	ChatID     string                   `json:"chat_id"`
	Name       *string                  `json:"name,omitempty"`
	Messages   []map[string]interface{} `json:"messages"`
	Reference  []interface{}            `json:"reference"`
	UserID     *string                  `json:"user_id,omitempty"`
	Avatar     *string                  `json:"avatar,omitempty"`
	CreateDate *time.Time               `json:"create_date,omitempty"`
	UpdateDate *time.Time               `json:"update_date,omitempty"`
	CreateTime *int64                   `json:"create_time,omitempty"`
	UpdateTime *int64                   `json:"update_time,omitempty"`
}

// ListChatSessions lists chat sessions for a dialog
func (s *ChatSessionService) ListChatSessions(userID string, chatID string) (*ListChatSessionsResponse, error) {
	// Get user's tenants
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, err
	}

	// Check if user is the owner of the dialog
	isOwner := false
	for _, tenantID := range tenantIDs {
		var exists bool
		exists, err = s.chatSessionDAO.CheckDialogExists(tenantID, chatID)
		if err != nil {
			return nil, err
		}
		if exists {
			isOwner = true
			break
		}
	}

	// Also check with userID as tenant
	if !isOwner {
		var exists bool
		exists, err = s.chatSessionDAO.CheckDialogExists(userID, chatID)
		if err != nil {
			return nil, err
		}
		isOwner = exists
	}

	if !isOwner {
		return nil, errors.New("only owner of dialog authorized for this operation")
	}

	// List chat sessions
	sessions, err := s.chatSessionDAO.ListByChatID(chatID)
	if err != nil {
		return nil, err
	}

	return &ListChatSessionsResponse{Sessions: sessions}, nil
}

// GetSession returns one chat session after ownership validation.
func (s *ChatSessionService) GetSession(userID, chatID, sessionID string) (*ChatSessionPayload, common.ErrorCode, error) {
	ok, err := s.ensureOwnedChat(userID, chatID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}

	session, err := s.chatSessionDAO.GetByID(sessionID)
	if err != nil {
		if isChatSessionNotFound(err) {
			return nil, common.CodeDataError, errors.New("Session not found!")
		}
		return nil, common.CodeServerError, err
	}
	if session.DialogID != chatID {
		return nil, common.CodeDataError, errors.New("Session does not belong to this chat!")
	}

	dialog, err := s.chatSessionDAO.GetDialogByID(chatID)
	if err != nil && !isChatSessionNotFound(err) {
		return nil, common.CodeServerError, err
	}

	return s.buildSessionPayload(session, dialog, true), common.CodeSuccess, nil
}

// CreateSession create a session in a dialog
func (s *ChatSessionService) CreateSession(userID, chatID string, req map[string]interface{}) (*ChatSessionPayload, common.ErrorCode, error) {
	ok, err := s.ensureOwnedChat(userID, chatID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}

	dialog, err := s.chatSessionDAO.GetDialogByID(chatID)
	if err != nil {
		if isChatSessionNotFound(err) {
			return nil, common.CodeDataError, errors.New("Chat not found!")
		}
		return nil, common.CodeServerError, err
	}

	name := "New session"
	if rawName, exists := req["name"]; exists {
		nameStr, ok := rawName.(string)
		if !ok || strings.TrimSpace(nameStr) == "" {
			return nil, common.CodeDataError, errors.New("`name` can not be empty.")
		}
		name = strings.TrimSpace(nameStr)
	}
	nameRunes := []rune(name)
	if len(nameRunes) > 255 {
		name = string(nameRunes[:255])
	}

	prologue := ""
	if dialog.PromptConfig != nil {
		if value, ok := dialog.PromptConfig["prologue"].(string); ok {
			prologue = value
		}
	}
	messagesJSON, _ := json.Marshal([]map[string]interface{}{
		{
			"role":    "assistant",
			"content": prologue,
		},
	})

	referenceJSON, _ := json.Marshal([]interface{}{})

	conv := &entity.ChatSession{
		ID:        utility.GenerateUUID(),
		DialogID:  chatID,
		Name:      &name,
		Message:   messagesJSON,
		UserID:    &userID,
		Reference: referenceJSON,
	}

	if err := s.chatSessionDAO.Create(conv); err != nil {
		return nil, common.CodeDataError, errors.New("Fail to create a session!")
	}

	session, err := s.chatSessionDAO.GetByID(conv.ID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("Fail to create a session!")
	}
	return s.buildSessionPayload(session, nil, false), common.CodeSuccess, nil
}

// DeleteSessions delete a session in a dialog
func (s *ChatSessionService) DeleteSessions(userID, chatID string, req map[string]interface{}) (interface{}, string, common.ErrorCode, error) {
	ok, err := s.ensureOwnedChat(userID, chatID)
	if err != nil {
		return nil, "", common.CodeServerError, err
	}
	if !ok {
		return false, "No authorization.", common.CodeAuthenticationError, errors.New("No authorization.")
	}

	if len(req) == 0 {
		return map[string]interface{}{}, "success", common.CodeSuccess, nil
	}

	sessionIDs, hasIDs := stringSliceFromValue(req["ids"])
	if !hasIDs || len(sessionIDs) == 0 {
		deleteAll, _ := req["delete_all"].(bool)
		if deleteAll {
			sessions, err := s.chatSessionDAO.ListByChatID(chatID)
			if err != nil {
				return nil, "", common.CodeServerError, err
			}
			for _, session := range sessions {
				sessionIDs = append(sessionIDs, session.ID)
			}
			if len(sessionIDs) == 0 {
				return map[string]interface{}{}, "success", common.CodeSuccess, nil
			}
		} else {
			return map[string]interface{}{}, "success", common.CodeSuccess, nil
		}
	}

	uniqueIDs, duplicateMessages := checkDuplicateChatSessionIDs(sessionIDs)

	errorsList := make([]string, 0)
	successCount := 0

	for _, sid := range uniqueIDs {
		session, err := s.chatSessionDAO.GetBySessionIDAndChatID(sid, chatID)
		if err != nil {
			errorsList = append(errorsList, fmt.Sprintf("The chat doesn't own the session %s", sid))
			continue
		}

		s.removeSessionUploadFiles(userID, session)

		if err := s.chatSessionDAO.DeleteByID(sid); err != nil {
			return nil, "", common.CodeServerError, err
		}

		successCount++
	}

	allErrors := append(errorsList, duplicateMessages...)

	if len(allErrors) > 0 {
		if successCount > 0 {
			return map[string]interface{}{
				"success_count": successCount,
				"errors":        allErrors,
			}, fmt.Sprintf("Partially deleted %d sessions with %d errors", successCount, len(allErrors)), common.CodeSuccess, nil
		}

		return nil, "", common.CodeDataError, errors.New(strings.Join(allErrors, "; "))
	}

	return true, "success", common.CodeSuccess, nil
}

func stringSliceFromValue(value interface{}) ([]string, bool) {
	var raw []interface{}
	switch typed := value.(type) {
	case []interface{}:
		raw = typed
	case []string:
		raw = make([]interface{}, 0, len(typed))
		for _, item := range typed {
			raw = append(raw, item)
		}
	default:
		return nil, false
	}

	ids := make([]string, 0, len(raw))
	for _, item := range raw {
		id, ok := item.(string)
		if !ok {
			continue
		}
		if strings.TrimSpace(id) == "" {
			continue
		}
		ids = append(ids, id)
	}
	return ids, true
}

func (s *ChatSessionService) removeSessionUploadFiles(userID string, session *entity.ChatSession) {
	messages := parseMessages(session.Message)
	bucket := fmt.Sprintf("%s-downloads", userID)
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		common.Warn("storage is not initialized; skip chat upload cleanup", zap.String("bucket", bucket))
		return
	}

	for _, msg := range messages {
		files, ok := msg["files"].([]interface{})
		if !ok {
			continue
		}

		for _, item := range files {
			file, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			fileID, ok := file["id"].(string)
			if !ok || fileID == "" {
				continue
			}

			if err := storageImpl.Remove(bucket, fileID); err != nil {
				common.Warn("Failed to delete chat upload blob",
					zap.String("bucket", bucket),
					zap.String("file_id", fileID),
					zap.Error(err),
				)
			}
		}
	}
}

func checkDuplicateChatSessionIDs(ids []string) ([]string, []string) {
	idCount := make(map[string]int, len(ids))
	uniqueIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		idCount[id]++
		if idCount[id] == 1 {
			uniqueIDs = append(uniqueIDs, id)
		}
	}

	duplicateMessages := make([]string, 0)
	for id, count := range idCount {
		if count > 1 {
			duplicateMessages = append(duplicateMessages, fmt.Sprintf("Duplicate session ids: %s", id))
		}
	}
	return uniqueIDs, duplicateMessages
}

// UpdateSession updates one chat session after Python-style field validation.
func (s *ChatSessionService) UpdateSession(userID, chatID, sessionID string, req map[string]interface{}) (*ChatSessionPayload, common.ErrorCode, error) {
	ok, err := s.ensureOwnedChat(userID, chatID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}
	if len(req) == 0 {
		return nil, common.CodeArgumentError, errors.New("Request body cannot be empty")
	}

	if _, err := s.chatSessionDAO.GetBySessionIDAndChatID(sessionID, chatID); err != nil {
		if isChatSessionNotFound(err) {
			return nil, common.CodeDataError, errors.New("Session not found!")
		}
		return nil, common.CodeServerError, err
	}

	if _, ok := req["message"]; ok {
		return nil, common.CodeDataError, errors.New("`messages` cannot be changed.")
	}
	if _, ok := req["messages"]; ok {
		return nil, common.CodeDataError, errors.New("`messages` cannot be changed.")
	}
	if _, ok := req["reference"]; ok {
		return nil, common.CodeDataError, errors.New("`reference` cannot be changed.")
	}

	if name, exists := req["name"]; exists && name != nil {
		nameStr, ok := name.(string)
		if !ok || strings.TrimSpace(nameStr) == "" {
			return nil, common.CodeDataError, errors.New("`name` can not be empty.")
		}
		req["name"] = strings.TrimSpace(nameStr)
		nameRunes := []rune(req["name"].(string))
		if len(nameRunes) > 255 {
			req["name"] = string(nameRunes[:255])
		}
	}

	updateFields := make(map[string]interface{})
	for k, v := range req {
		switch k {
		case "id", "dialog_id", "chat_id", "user_id":
			continue
		default:
			updateFields[k] = v
		}
	}

	if err := s.chatSessionDAO.UpdateByID(sessionID, updateFields); err != nil {
		if isChatSessionNotFound(err) {
			return nil, common.CodeDataError, errors.New("Session not found!")
		}
		return nil, common.CodeServerError, err
	}

	session, err := s.chatSessionDAO.GetByID(sessionID)
	if err != nil {
		if isChatSessionNotFound(err) {
			return nil, common.CodeDataError, errors.New("Fail to update a session!")
		}
		return nil, common.CodeServerError, err
	}

	return s.buildSessionPayload(session, nil, false), common.CodeSuccess, nil
}

func (s *ChatSessionService) DeleteSessionMessage(userID, chatID, sessionID, msgID string) (*ChatSessionPayload, common.ErrorCode, error) {
	ok, err := s.ensureOwnedChat(userID, chatID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !ok {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}

	session, err := s.chatSessionDAO.GetByID(sessionID)
	if err != nil || session.DialogID != chatID {
		if err != nil && !isChatSessionNotFound(err) {
			return nil, common.CodeServerError, err
		}
		return nil, common.CodeDataError, errors.New("Session not found!")
	}

	// parseMessages / parseReferenceList return nil for
	// malformed input, so the existing `nil` guards below are
	// the single source of truth for "this blob is corrupt".
	messages := parseMessages(session.Message)
	if len(session.Message) > 0 && messages == nil {
		return nil, common.CodeDataError, errors.New("Invalid session messages")
	}
	references := parseReferenceList(session.Reference)
	if len(session.Reference) > 0 && references == nil {
		return nil, common.CodeDataError, errors.New("Invalid session reference")
	}
	for i, msg := range messages {
		if msgID != stringValue(msg["id"]) {
			continue
		}
		if i+1 >= len(messages) || stringValue(messages[i+1]["id"]) != msgID {
			return nil, common.CodeServerError, errors.New("message pair assertion failed")
		}
		messages = append(messages[:i], messages[i+2:]...)
		refIndex := (i - 1) / 2
		if refIndex < 0 {
			refIndex = 0
		}
		if refIndex < len(references) {
			references = append(references[:refIndex], references[refIndex+1:]...)
		}
		break
	}

	messageRaw, err := json.Marshal(messages)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	referenceRaw, err := json.Marshal(references)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if err := s.chatSessionDAO.UpdateByID(session.ID, map[string]interface{}{
		"message":   messageRaw,
		"reference": referenceRaw,
	}); err != nil {
		return nil, common.CodeServerError, err
	}
	session.Message = messageRaw
	session.Reference = referenceRaw

	return s.buildSessionPayload(session, nil, false), common.CodeSuccess, nil
}

func (s *ChatSessionService) UpdateMessageFeedback(ctx context.Context, userID, chatID, sessionID, msgID string, req map[string]interface{}) (*ChatSessionPayload, common.ErrorCode, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ownerTenantID := ""
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	for _, tenantID := range tenantIDs {
		exists, err := s.chatSessionDAO.CheckDialogExists(tenantID, chatID)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if exists {
			ownerTenantID = tenantID
			break
		}
	}
	if ownerTenantID == "" {
		exists, err := s.chatSessionDAO.CheckDialogExists(userID, chatID)
		if err != nil {
			return nil, common.CodeServerError, err
		}
		if exists {
			ownerTenantID = userID
		}
	}
	ok := ownerTenantID != ""
	if !ok {
		return nil, common.CodeAuthenticationError, errors.New("No authorization.")
	}

	session, err := s.chatSessionDAO.GetByID(sessionID)
	if err != nil || session.DialogID != chatID {
		if err != nil && !isChatSessionNotFound(err) {
			return nil, common.CodeServerError, err
		}
		return nil, common.CodeDataError, errors.New("Session not found!")
	}

	thumbRaw, ok := req["thumbup"]
	if !ok {
		return nil, common.CodeDataError, errors.New("thumbup must be a boolean")
	}
	thumbup, ok := thumbRaw.(bool)
	if !ok {
		return nil, common.CodeDataError, errors.New("thumbup must be a boolean")
	}

	messages := parseMessages(session.Message)
	if len(session.Message) > 0 && messages == nil {
		return nil, common.CodeDataError, errors.New("Invalid session messages")
	}
	// References are only used later in this function but a
	// malformed blob must surface immediately, not silently
	// collapse to an empty slice.
	references := parseReferenceList(session.Reference)
	if len(session.Reference) > 0 && references == nil {
		return nil, common.CodeDataError, errors.New("Invalid session reference")
	}
	messageIndex := -1
	var priorThumb interface{}
	applyChunkFeedback := false
	var feedbackReference map[string]interface{}
	for i, msg := range messages {
		if msgID != stringValue(msg["id"]) || stringValue(msg["role"]) != "assistant" {
			continue
		}
		priorThumb = msg["thumbup"]
		priorThumbBool, priorThumbIsBool := priorThumb.(bool)
		if thumbup {
			msg["thumbup"] = true
			delete(msg, "feedback")
			applyChunkFeedback = !priorThumbIsBool || !priorThumbBool
		} else {
			msg["thumbup"] = false
			if feedback, exists := req["feedback"]; exists && isTruthy(feedback) {
				msg["feedback"] = feedback
			}
			applyChunkFeedback = !priorThumbIsBool || priorThumbBool
		}
		messages[i] = msg
		messageIndex = i
		break
	}

	if messageIndex != -1 && applyChunkFeedback {
		references := parseReferenceList(session.Reference)
		if len(session.Reference) > 0 && references == nil {
			return nil, common.CodeDataError, errors.New("Invalid session reference")
		}
		refIndex := (messageIndex - 1) / 2
		if refIndex >= 0 && refIndex < len(references) {
			if reference, ok := references[refIndex].(map[string]interface{}); ok && len(reference) > 0 {
				feedbackReference = reference
			}
		}
	}

	messageRaw, err := json.Marshal(messages)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if err := s.chatSessionDAO.UpdateByID(session.ID, map[string]interface{}{"message": messageRaw}); err != nil {
		return nil, common.CodeServerError, err
	}
	session.Message = messageRaw

	if feedbackReference != nil {
		applier := s.chunkFeedbackApplier
		if applier == nil {
			applier = s
		}
		if priorThumbBool, ok := priorThumb.(bool); ok && priorThumbBool != thumbup {
			result, _ := applier.applyChunkFeedback(ctx, ownerTenantID, feedbackReference, !priorThumbBool)
			if result != nil {
				common.Debug("Chunk feedback undo applied",
					zap.Any("success_count", result["success_count"]),
					zap.Any("fail_count", result["fail_count"]),
				)
			}
		}
		result, _ := applier.applyChunkFeedback(ctx, ownerTenantID, feedbackReference, thumbup)
		if result != nil {
			common.Debug("Chunk feedback applied",
				zap.Any("success_count", result["success_count"]),
				zap.Any("fail_count", result["fail_count"]),
			)
		}
	}

	return s.buildSessionPayload(session, nil, false), common.CodeSuccess, nil
}

const (
	upvoteWeightIncrement   = 1
	downvoteWeightDecrement = 1
	minPagerankWeight       = 0.0
	maxPagerankWeight       = 100.0
	chunkFeedbackTimeout    = 30 * time.Second
)

type feedbackChunkRow struct {
	chunkID string
	kbID    string
	chunk   map[string]interface{}
}

// applyChunkFeedback records a thumb vote against the chunks that produced a
// session message. It mirrors Python's ChunkFeedbackService.apply_feedback:
// feature-flagged by CHUNK_FEEDBACK_ENABLED, split by relevance unless
// CHUNK_FEEDBACK_WEIGHTING=uniform, and clamped through the document engine.
func (s *ChatSessionService) applyChunkFeedback(ctx context.Context, tenantID string, reference map[string]interface{}, isPositive bool) (map[string]interface{}, error) {
	if !chunkFeedbackEnabled() {
		common.Debug("Chunk feedback feature is disabled")
		return map[string]interface{}{
			"success_count": 0,
			"fail_count":    0,
			"chunk_ids":     []string{},
			"disabled":      true,
		}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, chunkFeedbackTimeout)
		defer cancel()
	}

	rows := feedbackRowsFromReference(reference)
	chunkIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		chunkIDs = append(chunkIDs, row.chunkID)
	}
	if len(rows) == 0 {
		common.Debug("No chunk IDs found in reference for feedback")
		return map[string]interface{}{
			"success_count": 0,
			"fail_count":    0,
			"chunk_ids":     chunkIDs,
		}, nil
	}

	signedBudget := float64(upvoteWeightIncrement)
	if !isPositive {
		signedBudget = -float64(downvoteWeightDecrement)
	}
	deltas := allocateFeedbackDeltas(rows, signedBudget, chunkFeedbackWeighting())

	successCount := 0
	failCount := 0
	for _, delta := range deltas {
		if delta.delta == 0 {
			continue
		}
		if s.updateChunkWeight(ctx, tenantID, delta.chunkID, delta.kbID, delta.delta) {
			successCount++
		} else {
			failCount++
		}
	}

	common.Info("Applied chunk feedback",
		zap.Bool("is_positive", isPositive),
		zap.String("weighting", chunkFeedbackWeighting()),
		zap.Int("success_count", successCount),
		zap.Int("chunk_count", len(chunkIDs)),
	)
	return map[string]interface{}{
		"success_count": successCount,
		"fail_count":    failCount,
		"chunk_ids":     chunkIDs,
	}, nil
}

type feedbackDelta struct {
	chunkID string
	kbID    string
	delta   float64
}

func chunkFeedbackEnabled() bool {
	return strings.ToLower(os.Getenv("CHUNK_FEEDBACK_ENABLED")) == "true"
}

func chunkFeedbackWeighting() string {
	weighting := strings.ToLower(strings.TrimSpace(os.Getenv("CHUNK_FEEDBACK_WEIGHTING")))
	if weighting == "uniform" || weighting == "relevance" {
		return weighting
	}
	return "relevance"
}

func feedbackRowsFromReference(reference map[string]interface{}) []feedbackChunkRow {
	if len(reference) == 0 {
		return nil
	}
	rawChunks, ok := reference["chunks"].([]interface{})
	if !ok {
		if chunks, ok := reference["chunks"].([]map[string]interface{}); ok {
			rows := make([]feedbackChunkRow, 0, len(chunks))
			for _, chunk := range chunks {
				if row, ok := feedbackRowFromChunk(chunk); ok {
					rows = append(rows, row)
				}
			}
			return rows
		}
		return nil
	}
	rows := make([]feedbackChunkRow, 0, len(rawChunks))
	for _, raw := range rawChunks {
		chunk, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if row, ok := feedbackRowFromChunk(chunk); ok {
			rows = append(rows, row)
		}
	}
	return rows
}

func feedbackRowFromChunk(chunk map[string]interface{}) (feedbackChunkRow, bool) {
	chunkID := stringValue(chunk["id"])
	if chunkID == "" {
		chunkID = stringValue(chunk["chunk_id"])
	}
	kbID := stringValue(chunk["dataset_id"])
	if kbID == "" {
		kbID = stringValue(chunk["kb_id"])
	}
	if chunkID == "" || kbID == "" {
		return feedbackChunkRow{}, false
	}
	return feedbackChunkRow{chunkID: chunkID, kbID: kbID, chunk: chunk}, true
}

func allocateFeedbackDeltas(rows []feedbackChunkRow, signedBudget float64, weighting string) []feedbackDelta {
	if len(rows) == 0 {
		return nil
	}
	if weighting == "uniform" {
		step := signedBudget / float64(len(rows))
		deltas := make([]feedbackDelta, 0, len(rows))
		for _, row := range rows {
			deltas = append(deltas, feedbackDelta{chunkID: row.chunkID, kbID: row.kbID, delta: step})
		}
		return deltas
	}

	magnitudes := make([]float64, 0, len(rows))
	for _, row := range rows {
		signal := retrievalSignal(row.chunk)
		if signal <= 0 {
			signal = 1
		}
		magnitudes = append(magnitudes, signal)
	}
	parts := splitFloatBudget(magnitudes, math.Abs(signedBudget))
	sign := math.Copysign(1, signedBudget)
	deltas := make([]feedbackDelta, 0, len(rows))
	for i, row := range rows {
		deltas = append(deltas, feedbackDelta{chunkID: row.chunkID, kbID: row.kbID, delta: sign * parts[i]})
	}
	return deltas
}

func retrievalSignal(chunk map[string]interface{}) float64 {
	best := 0.0
	for _, key := range []string{"similarity", "vector_similarity", "term_similarity"} {
		val, ok := floatValue(chunk[key])
		if ok && !math.IsInf(val, 0) && !math.IsNaN(val) && val > best {
			best = val
		}
	}
	return best
}

func splitFloatBudget(magnitudes []float64, budget float64) []float64 {
	n := len(magnitudes)
	out := make([]float64, n)
	if n == 0 || budget == 0 {
		return out
	}
	total := 0.0
	for _, magnitude := range magnitudes {
		total += magnitude
	}
	if total <= 0 {
		base := budget / float64(n)
		for i := range out {
			out[i] = base
		}
		return out
	}

	for i, magnitude := range magnitudes {
		out[i] = budget * magnitude / total
	}
	return out
}

func (s *ChatSessionService) updateChunkWeight(ctx context.Context, tenantID, chunkID, kbID string, delta float64) bool {
	docEngine := s.docEngine
	if docEngine == nil {
		docEngine = engine.Get()
	}
	if docEngine == nil {
		common.Warn("Document engine is not initialized; chunk feedback skipped",
			zap.String("chunk_id", chunkID),
			zap.String("kb_id", kbID),
		)
		return false
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	if adjuster, ok := docEngine.(chunkPagerankAdjuster); ok {
		if err := adjuster.AdjustChunkPagerank(ctx, indexName, chunkID, kbID, delta, minPagerankWeight, maxPagerankWeight); err != nil {
			common.Warn("Failed atomic pagerank adjust for chunk",
				zap.String("chunk_id", chunkID),
				zap.Error(err),
			)
			return false
		}
		return true
	}

	rawChunk, err := docEngine.GetChunk(ctx, indexName, chunkID, []string{kbID})
	if err != nil {
		common.Warn("Chunk not found for feedback",
			zap.String("chunk_id", chunkID),
			zap.String("index", indexName),
			zap.Error(err),
		)
		return false
	}
	chunk, ok := rawChunk.(map[string]interface{})
	if !ok {
		common.Warn("Unexpected chunk shape for feedback",
			zap.String("chunk_id", chunkID),
			zap.String("kb_id", kbID),
			zap.String("index", indexName),
			zap.String("chunk_type", fmt.Sprintf("%T", rawChunk)),
		)
		return false
	}

	currentWeight, _ := floatValue(chunk[common.PAGERANK_FLD])
	nextWeight := currentWeight + delta
	if nextWeight < minPagerankWeight {
		nextWeight = minPagerankWeight
	}
	if nextWeight > maxPagerankWeight {
		nextWeight = maxPagerankWeight
	}

	newValue := map[string]interface{}{common.PAGERANK_FLD: nextWeight}
	if nextWeight <= 0 && strings.ToLower(docEngine.GetType()) == string(engine.EngineElasticsearch) {
		newValue = map[string]interface{}{"remove": common.PAGERANK_FLD}
	}
	if err := docEngine.UpdateChunks(ctx, map[string]interface{}{"id": chunkID}, newValue, indexName, kbID); err != nil {
		common.Warn("Failed to update chunk pagerank",
			zap.String("chunk_id", chunkID),
			zap.Error(err),
		)
		return false
	}
	return true
}

func floatValue(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case int32:
		return float64(v), true
	case json.Number:
		f, err := v.Float64()
		return f, err == nil
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

func (s *ChatSessionService) ensureOwnedChat(userID, chatID string) (bool, error) {
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return false, err
	}

	for _, tenantID := range tenantIDs {
		exists, err := s.chatSessionDAO.CheckDialogExists(tenantID, chatID)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}

	exists, err := s.chatSessionDAO.CheckDialogExists(userID, chatID)
	if err != nil {
		return false, err
	}
	return exists, nil
}

func (s *ChatSessionService) buildSessionPayload(session *entity.ChatSession, dialog *entity.Chat, includeAvatar bool) *ChatSessionPayload {
	var avatar *string
	if includeAvatar {
		value := ""
		if dialog != nil && dialog.Icon != nil {
			value = *dialog.Icon
		}
		avatar = &value
	}

	references := parseReferenceList(session.Reference)
	for index, ref := range references {
		refMap, ok := ref.(map[string]interface{})
		if !ok {
			continue
		}
		refMap["chunks"] = formatReferenceChunks(refMap)
		references[index] = refMap
	}

	return &ChatSessionPayload{
		ID:         session.ID,
		ChatID:     session.DialogID,
		Name:       session.Name,
		Messages:   parseMessages(session.Message),
		Reference:  references,
		UserID:     session.UserID,
		Avatar:     avatar,
		CreateDate: session.CreateDate,
		UpdateDate: session.UpdateDate,
		CreateTime: session.CreateTime,
		UpdateTime: session.UpdateTime,
	}
}

// parseMessages decodes a session.Message blob. Returns:
//   - nil                  — input was non-empty but malformed JSON;
//     callers should reject with "Invalid session messages".
//   - non-nil empty slice  — input was empty (no messages stored yet).
//   - non-nil slice        — successful decode.
//
// The nil-on-malformed contract is what makes the
// `if len(raw) > 0 && messages == nil` checks in
// DeleteSessionMessage / UpdateMessageFeedback fire — without it,
// a corrupted blob silently parses to an empty slice and the
// message-repair logic no-ops. PR review round 7 (CI red): the
// helpers previously returned `make([]T, 0)` on parse failure,
// so the upstream-introduced guards in a55388698 never matched.
func parseMessages(raw json.RawMessage) []map[string]interface{} {
	messages := make([]map[string]interface{}, 0)
	if len(raw) == 0 {
		return messages
	}
	if err := json.Unmarshal(raw, &messages); err == nil {
		if messages == nil {
			return make([]map[string]interface{}, 0)
		}
		return messages
	}

	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil
	}
	wrappedMessages, ok := wrapped["messages"]
	if !ok {
		// Object without a "messages" key — wrong schema, not empty.
		return nil
	}
	if len(wrappedMessages) == 0 || string(wrappedMessages) == "null" {
		// Empty / explicit-null wrapped messages is a legitimate
		// empty form, not a parse failure. Return non-nil empty
		// so the service layer's `messages == nil` check
		// correctly distinguishes "no messages yet" from "corrupt
		// blob". (Upstream fix: the original code returned nil
		// here too, which made the test
		// TestParseCollections_ReturnEmptySlicesForMissingOrNull
		// fail — `{"messages":null}` is a valid empty form, not
		// a malformed blob.)
		return make([]map[string]interface{}, 0)
	}
	if err := json.Unmarshal(wrappedMessages, &messages); err != nil {
		return nil
	}
	if messages == nil {
		return make([]map[string]interface{}, 0)
	}
	return messages
}

// parseReferenceList decodes a session.Reference blob. Same
// nil-on-malformed contract as parseMessages — callers gate on
// `if len(raw) > 0 && references == nil` to reject corruption.
func parseReferenceList(raw json.RawMessage) []interface{} {
	references := make([]interface{}, 0)
	if len(raw) == 0 {
		return references
	}
	if err := json.Unmarshal(raw, &references); err != nil {
		return nil
	}
	if references == nil {
		return make([]interface{}, 0)
	}
	return references
}

func formatReferenceChunks(reference map[string]interface{}) []FormattedChunk {
	rawChunks, ok := reference["chunks"].([]interface{})
	if !ok {
		return []FormattedChunk{}
	}

	chunks := make([]map[string]interface{}, 0, len(rawChunks))
	for _, item := range rawChunks {
		chunk, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		chunks = append(chunks, chunk)
	}
	return formatChunks(chunks)
}

func isChatSessionNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// Completion performs chat completion with full RAG support via ChatPipelineService.
// Kept as a compatibility entrypoint for callers that still use the pre-ChatCompletions API.
func (s *ChatSessionService) Completion(userID string, conversationID string, messages []map[string]interface{}, llmID string, chatModelConfig map[string]interface{}, messageID string) (map[string]interface{}, error) {
	if len(messages) == 0 {
		return nil, errors.New("messages cannot be empty")
	}
	lastRole, _ := messages[len(messages)-1]["role"].(string)
	if lastRole != "user" {
		return nil, errors.New("the last content of this conversation is not from user")
	}

	session, err := s.chatSessionDAO.GetByID(conversationID)
	if err != nil {
		return nil, errors.New("Conversation not found")
	}

	dialog, err := s.chatSessionDAO.GetDialogByID(session.DialogID)
	if err != nil {
		return nil, errors.New("Dialog not found")
	}

	sessionMessages := s.buildSessionMessages(session, messages)
	reference := s.initializeReference(session)

	isEmbedded := llmID != ""
	if llmID != "" {
		hasKey, err := s.checkTenantLLMAPIKey(dialog.TenantID, llmID)
		if err != nil || !hasKey {
			return nil, fmt.Errorf("Cannot use specified model %s", llmID)
		}
		dialog.LLMID = llmID
		if chatModelConfig != nil {
			dialog.LLMSetting = chatModelConfig
		}
	}

	kwargs := chatModelConfig
	if kwargs == nil {
		kwargs = map[string]interface{}{}
	}
	resultChan, err := s.pipeline.AsyncChat(context.Background(), userID, dialog, messages, false, kwargs)
	if err != nil {
		return nil, err
	}

	var answer strings.Builder
	var finalRef map[string]interface{}
	for result := range resultChan {
		if result.Answer != "" {
			answer.WriteString(result.Answer)
		}
		if result.Reference != nil {
			finalRef = result.Reference
		}
	}

	ans := map[string]interface{}{
		"answer":    answer.String(),
		"reference": finalRef,
		"final":     true,
	}
	result := s.structureAnswerWithConv(session, ans, messageID, session.ID, reference)

	if !isEmbedded {
		sessionMessages = append(sessionMessages, map[string]interface{}{
			"role":       "assistant",
			"content":    answer.String(),
			"id":         messageID,
			"created_at": float64(time.Now().Unix()),
		})
		s.updateSessionMessages(session, sessionMessages, reference)
	}

	return result, nil
}

// CompletionStream performs streaming chat completion with full RAG support via ChatPipelineService.
// Kept as a compatibility entrypoint for callers that still use the pre-ChatCompletions API.
func (s *ChatSessionService) CompletionStream(ctx context.Context, userID string, conversationID string, messages []map[string]interface{}, llmID string, chatModelConfig map[string]interface{}, messageID string, streamChan chan<- string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	if len(messages) == 0 {
		streamChan <- fmt.Sprintf("data: %s\n\n", `{"code": 500, "message": "messages cannot be empty", "data": {"answer": "**ERROR**: messages cannot be empty", "reference": []}}`)
		return errors.New("messages cannot be empty")
	}
	lastRole, _ := messages[len(messages)-1]["role"].(string)
	if lastRole != "user" {
		streamChan <- fmt.Sprintf("data: %s\n\n", `{"code": 500, "message": "the last content of this conversation is not from user", "data": {"answer": "**ERROR**: the last content of this conversation is not from user", "reference": []}}`)
		return errors.New("the last content of this conversation is not from user")
	}

	session, err := s.chatSessionDAO.GetByID(conversationID)
	if err != nil {
		streamChan <- fmt.Sprintf("data: %s\n\n", `{"code": 500, "message": "Conversation not found", "data": {"answer": "**ERROR**: Conversation not found", "reference": []}}`)
		return errors.New("Conversation not found")
	}

	dialog, err := s.chatSessionDAO.GetDialogByID(session.DialogID)
	if err != nil {
		streamChan <- fmt.Sprintf("data: %s\n\n", `{"code": 500, "message": "Dialog not found", "data": {"answer": "**ERROR**: Dialog not found", "reference": []}}`)
		return errors.New("Dialog not found")
	}

	sessionMessages := s.buildSessionMessages(session, messages)
	reference := s.initializeReference(session)

	isEmbedded := llmID != ""
	if llmID != "" {
		hasKey, err := s.checkTenantLLMAPIKey(dialog.TenantID, llmID)
		if err != nil || !hasKey {
			errMsg := fmt.Sprintf(`{"code": 500, "message": "Cannot use specified model %s", "data": {"answer": "**ERROR**: Cannot use specified model", "reference": []}}`, llmID)
			streamChan <- fmt.Sprintf("data: %s\n\n", errMsg)
			return fmt.Errorf("Cannot use specified model %s", llmID)
		}
		dialog.LLMID = llmID
		if chatModelConfig != nil {
			dialog.LLMSetting = chatModelConfig
		}
	}

	kwargs := chatModelConfig
	if kwargs == nil {
		kwargs = map[string]interface{}{}
	}
	resultChan, err := s.pipeline.AsyncChat(ctx, userID, dialog, messages, true, kwargs)
	if err != nil {
		streamChan <- fmt.Sprintf("data: %s\n\n", fmt.Sprintf(`{"code": 500, "message": "%s", "data": {"answer": "**ERROR**: %s", "reference": []}}`, err.Error(), err.Error()))
		return err
	}

	var fullAnswer strings.Builder
	for result := range resultChan {
		if result.Reference != nil && len(reference) > 0 {
			reference[len(reference)-1] = result.Reference
		}
		if result.Final {
			if result.Answer != "" {
				fullAnswer.Reset()
				fullAnswer.WriteString(result.Answer)
			}
		} else if result.Answer != "" {
			fullAnswer.WriteString(result.Answer)
		}
		ans := s.structureAnswer(session, fullAnswer.String(), messageID, session.ID, reference)
		data, _ := json.Marshal(map[string]interface{}{
			"code":    0,
			"message": "",
			"data":    ans,
		})
		streamChan <- fmt.Sprintf("data: %s\n\n", string(data))
	}

	finalData, _ := json.Marshal(map[string]interface{}{
		"code":    0,
		"message": "",
		"data":    true,
	})
	streamChan <- fmt.Sprintf("data: %s\n\n", string(finalData))

	if !isEmbedded {
		sessionMessages = append(sessionMessages, map[string]interface{}{
			"role":       "assistant",
			"content":    fullAnswer.String(),
			"id":         messageID,
			"created_at": float64(time.Now().Unix()),
		})
		s.updateSessionMessages(session, sessionMessages, reference)
	}

	return nil
}

// ChatCompletions handles chat completion matching Python's session_completion.
// When stream=true, returns nil result and streams SSE via streamChan.
// When stream=false, returns the structured answer map.
func (s *ChatSessionService) ChatCompletions(
	ctx context.Context,
	userID string,
	chatID string, sessionID string,
	messages []map[string]interface{}, question string, files []interface{},
	llmID string, genConfig map[string]interface{}, kwargs map[string]interface{},
	passAllHistory bool, legacy bool,
	stream bool, streamChan chan<- string,
) (map[string]interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	fail := func(err error) (map[string]interface{}, error) {
		if stream && streamChan != nil {
			s.sendSSEError(streamChan, err.Error())
		}
		return nil, err
	}

	sendOrCancel := func(data string) bool {
		select {
		case streamChan <- data:
			return true
		case <-ctx.Done():
			return false
		}
	}

	common.Info("ChatCompletions started")

	// --- 1. Normalize messages ---
	requestMessages, requestMsg, messageID, err := s.normalizeCompletionMessages(messages, question, files)
	if err != nil {
		return fail(err)
	}

	// --- 2. Validate ---
	if sessionID != "" && chatID == "" {
		return fail(errors.New("`chat_id` is required when `session_id` is provided."))
	}

	// --- 3. Resolve dialog and session ---
	var dialog *entity.Chat
	var session *entity.ChatSession
	if chatID != "" {
		if err := s.checkDialogOwnership(userID, chatID); err != nil {
			return fail(err)
		}
		dialog, err = s.chatSessionDAO.GetDialogByID(chatID)
		if err != nil {
			return fail(errors.New("Chat not found!"))
		}
		if sessionID != "" {
			session, err = s.chatSessionDAO.GetByID(sessionID)
			if err != nil {
				return fail(errors.New("Session not found!"))
			}
			if session.DialogID != chatID {
				return fail(errors.New("Session does not belong to this chat!"))
			}
		} else {
			session, err = s.createSessionForCompletion(chatID, dialog, userID)
			if err != nil {
				return fail(err)
			}
			sessionID = session.ID
		}

		if passAllHistory {
			session.Message, _ = json.Marshal(requestMessages)
		} else {
			session = s.appendSessionMessage(session, requestMsg)
		}
		requestMsg = s.filterSystemAndLeadingAssistant(session)
		_ = messageID
	} else {
		dialog = s.buildDefaultCompletionDialog(userID)
		if !stream {
			genConfig["stream"] = false
		}
	}

	// --- 4. Initialize reference ---
	var reference []interface{}
	if session != nil {
		reference = s.initializeReference(session)
	}

	// --- 5. LLM override ---
	if genConfig == nil {
		genConfig = map[string]interface{}{}
	}
	if llmID != "" {
		hasKey, err := s.checkTenantLLMAPIKey(dialog.TenantID, llmID)
		if err != nil || !hasKey {
			return fail(fmt.Errorf("Cannot use specified model %s", llmID))
		}
		dialog.LLMID = llmID
		dialog.LLMSetting = genConfig
	} else if dialog.LLMID == "" {
		tenant, err := dao.NewTenantDAO().GetByID(dialog.TenantID)
		if err != nil || tenant.LLMID == "" {
			return fail(errors.New("No default chat model for tenant."))
		}
		dialog.LLMID = tenant.LLMID
		if dialog.LLMSetting == nil {
			dialog.LLMSetting = entity.JSONMap{}
		}
		for k, v := range genConfig {
			dialog.LLMSetting[k] = v
		}
	}

	if kwargs == nil {
		kwargs = map[string]interface{}{}
	}
	for k, v := range genConfig {
		kwargs[k] = v
	}

	// --- 6. Run pipeline ---
	resultChan, err := s.pipeline.AsyncChat(ctx, userID, dialog, requestMsg, stream, kwargs)
	if err != nil {
		return fail(err)
	}

	if stream && streamChan != nil {
		var fullAnswer strings.Builder
		var finalLegacyAnswer map[string]interface{}

		for result := range resultChan {
			if result.Reference != nil && len(reference) > 0 {
				reference[len(reference)-1] = result.Reference
			}

			if legacy {
				if result.Final {
					if strings.Contains(result.Answer, "**ERROR**") {
						ans := s.structureAnswer(session, result.Answer, messageID, sessionID, reference)
						if chatID != "" {
							ans["chat_id"] = chatID
						}
						sendOrCancel(fmt.Sprintf("data:%s\n\n", sseMarshalChunk(sanitizeJSONFloats(ans).(map[string]interface{}), chatID)))
					}
					finalLegacyAnswer = s.structureAnswer(session, result.Answer, messageID, sessionID, reference)
					continue
				}
				if result.StartToThink {
					fullAnswer.WriteString("<think>")
				} else if result.EndToThink {
					fullAnswer.WriteString("</think>")
				} else if result.Answer != "" {
					fullAnswer.WriteString(result.Answer)
				}
				if session != nil {
					s.appendAssistantToSession(session, fullAnswer.String(), messageID)
				}
				ans := s.structureAnswer(session, fullAnswer.String(), messageID, sessionID, reference)
				ans["start_to_think"] = nil
				ans["end_to_think"] = nil
				delete(ans, "start_to_think")
				delete(ans, "end_to_think")
				if chatID != "" {
					ans["chat_id"] = chatID
				}
				sendOrCancel(fmt.Sprintf("data:%s\n\n", sseMarshalChunk(sanitizeJSONFloats(ans).(map[string]interface{}), chatID)))
			} else {
				if result.Final {
					if strings.Contains(result.Answer, "**ERROR**") {
						ans := s.structureAnswer(session, result.Answer, messageID, sessionID, reference)
						if chatID != "" {
							ans["chat_id"] = chatID
						}
						sendOrCancel(fmt.Sprintf("data:%s\n\n", sseMarshalChunk(sanitizeJSONFloats(ans).(map[string]interface{}), chatID)))
					} else {
						ans := s.structureAnswer(session, "", messageID, sessionID, reference)
						ans["final"] = true
						if chatID != "" {
							ans["chat_id"] = chatID
						}
						sendOrCancel(fmt.Sprintf("data:%s\n\n", sseMarshalChunk(sanitizeJSONFloats(ans).(map[string]interface{}), chatID)))
					}
					continue
				}
				deltaAnswer := ""
				if result.StartToThink {
					fullAnswer.WriteString("<think>")
				} else if result.EndToThink {
					fullAnswer.WriteString("</think>")
				} else if result.Answer != "" {
					fullAnswer.WriteString(result.Answer)
					deltaAnswer = result.Answer
				}
				if session != nil {
					s.appendAssistantToSession(session, fullAnswer.String(), messageID)
				}
				ans := s.structureAnswer(session, deltaAnswer, messageID, sessionID, reference)
				ans["start_to_think"] = result.StartToThink
				ans["end_to_think"] = result.EndToThink
				if chatID != "" {
					ans["chat_id"] = chatID
				}
				sendOrCancel(fmt.Sprintf("data:%s\n\n", sseMarshalChunk(sanitizeJSONFloats(ans).(map[string]interface{}), chatID)))
			}
		}
		if legacy && finalLegacyAnswer != nil {
			finalLegacyAnswer["answer"] = fullAnswer.String()
			delete(finalLegacyAnswer, "start_to_think")
			delete(finalLegacyAnswer, "end_to_think")
			finalChunk := sseWrapper{Code: 0, Message: "", Data: sanitizeJSONFloats(finalLegacyAnswer)}
			sendOrCancel(fmt.Sprintf("data:%s\n\n", marshalJSONWithSpaces(finalChunk)))
		}

		wrapper := sseWrapper{Code: 0, Message: "", Data: true}
		sendOrCancel(fmt.Sprintf("data:%s\n\n", marshalJSONWithSpaces(wrapper)))

		// Persist session state (matches Python's update_by_id after loop)
		if session != nil {
			s.updateSessionMessages(session, s.getSessionMessagesAsSlice(session), reference)
		}
	} else {
		var answer strings.Builder
		var finalRef map[string]interface{}
		for result := range resultChan {
			if result.Answer != "" {
				answer.WriteString(result.Answer)
			}
			if result.Reference != nil {
				finalRef = result.Reference
			}
		}
		ans := map[string]interface{}{
			"answer":    answer.String(),
			"reference": finalRef,
			"final":     true,
		}
		if session != nil {
			result := s.structureAnswerWithConv(session, ans, messageID, sessionID, reference)
			if chatID != "" {
				result["chat_id"] = chatID
			}
			s.updateSessionMessages(session, s.getSessionMessagesAsSlice(session), reference)
			return sanitizeJSONFloats(result).(map[string]interface{}), nil
		}
		ans["id"] = messageID
		ans["session_id"] = sessionID
		if chatID != "" {
			ans["chat_id"] = chatID
		}
		return sanitizeJSONFloats(ans).(map[string]interface{}), nil
	}

	return nil, nil
}

// --- Helpers for ChatCompletions ---

// normalizeCompletionMessages mirrors Python _normalize_completion_messages.
func (s *ChatSessionService) normalizeCompletionMessages(
	messages []map[string]interface{}, question string, files []interface{},
) (requestMessages []map[string]interface{}, requestMsg []map[string]interface{}, messageID string, err error) {
	if len(messages) == 0 {
		if question == "" {
			return nil, nil, "", errors.New("required argument are missing: messages")
		}
		messages = []map[string]interface{}{{"role": "user", "content": question}}
		if len(files) > 0 {
			messages[0]["files"] = files
		}
	}

	requestMessages = make([]map[string]interface{}, len(messages))
	for i, m := range messages {
		requestMessages[i] = make(map[string]interface{})
		for k, v := range m {
			requestMessages[i][k] = v
		}
	}

	// Filter system and leading assistant messages
	requestMsg = make([]map[string]interface{}, 0, len(messages))
	for _, m := range messages {
		role, _ := m["role"].(string)
		if role == "system" {
			continue
		}
		if role == "assistant" && len(requestMsg) == 0 {
			continue
		}
		requestMsg = append(requestMsg, m)
	}

	if len(requestMsg) == 0 {
		return nil, nil, "", errors.New("`messages` must contain a user message.")
	}
	lastRole, _ := requestMsg[len(requestMsg)-1]["role"].(string)
	if lastRole != "user" {
		return nil, nil, "", errors.New("The last content of this conversation is not from user.")
	}

	// Generate message ID if missing — matches Python's get_uuid() in _normalize_completion_messages.
	lastUserMsg := requestMsg[len(requestMsg)-1]
	if id, ok := lastUserMsg["id"].(string); ok && id != "" {
		messageID = id
	} else {
		messageID = utility.GenerateToken()
		lastUserMsg["id"] = messageID
		for i := len(requestMessages) - 1; i >= 0; i-- {
			if role, _ := requestMessages[i]["role"].(string); role == "user" {
				requestMessages[i]["id"] = messageID
				break
			}
		}
	}
	return requestMessages, requestMsg, messageID, nil
}

// checkDialogOwnership checks if the user owns the dialog.
func (s *ChatSessionService) checkDialogOwnership(userID, chatID string) error {
	ok, err := s.ensureOwnedChat(userID, chatID)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("No authorization.")
	}
	return nil
}

// buildDefaultCompletionDialog mirrors Python _build_default_completion_dialog.
func (s *ChatSessionService) buildDefaultCompletionDialog(tenantID string) *entity.Chat {
	return &entity.Chat{
		TenantID:               tenantID,
		LLMID:                  "",
		LLMSetting:             entity.JSONMap{},
		PromptConfig:           entity.JSONMap{},
		KBIDs:                  entity.JSONSlice{},
		TopN:                   6,
		TopK:                   1024,
		RerankID:               "",
		SimilarityThreshold:    0.1,
		VectorSimilarityWeight: 0.3,
	}
}

// createSessionForCompletion mirrors Python _create_session_for_completion.
func (s *ChatSessionService) createSessionForCompletion(chatID string, dialog *entity.Chat, userID string) (*entity.ChatSession, error) {
	newID := utility.GenerateUUID()
	name := "New session"

	prologue := "Hi! I'm your assistant. What can I do for you?"
	if dialog.PromptConfig != nil {
		if p, ok := dialog.PromptConfig["prologue"].(string); ok && p != "" {
			prologue = p
		}
	}

	msg := []map[string]interface{}{
		{"role": "assistant", "content": prologue},
	}
	msgJSON, _ := json.Marshal(msg)
	refJSON, _ := json.Marshal([]interface{}{})

	session := &entity.ChatSession{
		ID:        newID,
		DialogID:  chatID,
		Name:      &name,
		Message:   msgJSON,
		UserID:    &userID,
		Reference: refJSON,
	}
	if err := s.chatSessionDAO.Create(session); err != nil {
		return nil, err
	}
	return session, nil
}

// appendSessionMessage appends the last user message to the session's message history.
func (s *ChatSessionService) appendSessionMessage(session *entity.ChatSession, requestMsg []map[string]interface{}) *entity.ChatSession {
	msgs := parseMessages(session.Message)
	msgs = append(msgs, requestMsg[len(requestMsg)-1])
	session.Message, _ = json.Marshal(msgs)
	return session
}

// filterSystemAndLeadingAssistant filters system messages and leading assistant messages from session history.
func (s *ChatSessionService) filterSystemAndLeadingAssistant(session *entity.ChatSession) []map[string]interface{} {
	messages := parseMessages(session.Message)
	var result []map[string]interface{}
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role == "system" {
			continue
		}
		if role == "assistant" && len(result) == 0 {
			continue
		}
		result = append(result, msg)
	}
	return result
}

// appendAssistantToSession appends or updates the assistant message in session.Message.
func (s *ChatSessionService) appendAssistantToSession(session *entity.ChatSession, content string, messageID string) {
	messages := parseMessages(session.Message)
	if len(messages) == 0 || s.getLastRole(messages) != "assistant" {
		messages = append(messages, map[string]interface{}{
			"role":       "assistant",
			"content":    content,
			"created_at": float64(time.Now().Unix()),
			"id":         messageID,
		})
	} else {
		lastIdx := len(messages) - 1
		messages[lastIdx]["content"] = content
		messages[lastIdx]["created_at"] = float64(time.Now().Unix())
		messages[lastIdx]["id"] = messageID
	}
	session.Message, _ = json.Marshal(messages)
}

// getSessionMessagesAsSlice returns the session's messages as a slice of maps.
func (s *ChatSessionService) getSessionMessagesAsSlice(session *entity.ChatSession) []map[string]interface{} {
	if session == nil {
		return nil
	}
	return parseMessages(session.Message)
}

// sendSSEError sends an error in SSE format through the stream channel.
func (s *ChatSessionService) sendSSEError(streamChan chan<- string, errMsg string) {
	wrapper := sseWrapper{
		Code:    500,
		Message: errMsg,
		Data: map[string]interface{}{
			"answer":    "**ERROR**: " + errMsg,
			"reference": []interface{}{},
		},
	}
	streamChan <- fmt.Sprintf("data:%s\n\n", marshalJSONWithSpaces(wrapper))
}

// Helper methods

func (s *ChatSessionService) buildSessionMessages(session *entity.ChatSession, messages []map[string]interface{}) []map[string]interface{} {
	prefix := make([]map[string]interface{}, 0, 1)
	existingMessages := parseMessages(session.Message)
	if len(existingMessages) > 0 {
		if role, _ := existingMessages[0]["role"].(string); role == "assistant" {
			firstIncomingRole := ""
			if len(messages) > 0 {
				firstIncomingRole, _ = messages[0]["role"].(string)
			}
			if firstIncomingRole != "assistant" {
				prologue := make(map[string]interface{}, len(existingMessages[0]))
				for k, v := range existingMessages[0] {
					prologue[k] = v
				}
				prefix = append(prefix, prologue)
			}
		}
	}

	sessionMessages := make([]map[string]interface{}, 0, len(prefix)+len(messages))
	sessionMessages = append(sessionMessages, prefix...)
	for _, msg := range messages {
		cloned := make(map[string]interface{}, len(msg))
		for k, v := range msg {
			cloned[k] = v
		}
		sessionMessages = append(sessionMessages, cloned)
	}
	return sessionMessages
}

func (s *ChatSessionService) initializeReference(session *entity.ChatSession) []interface{} {
	var reference []interface{}
	if len(session.Reference) > 0 {
		json.Unmarshal(session.Reference, &reference)
	}
	// Filter out nil entries and append new reference
	var filtered []interface{}
	for _, r := range reference {
		if r != nil {
			filtered = append(filtered, r)
		}
	}
	filtered = append(filtered, map[string]interface{}{
		"chunks":   []map[string]interface{}{},
		"doc_aggs": []interface{}{},
	})
	return filtered
}

func (s *ChatSessionService) checkTenantLLMAPIKey(tenantID, modelName string) (bool, error) {
	_, err := NewTenantLLMService().GetAPIKeyFromInstance(tenantID, modelName)
	if err != nil {
		return false, err
	}
	return true, nil
}

// sseAnswerChunk has deterministic JSON field order matching Python's structure_answer output.
type sseAnswerChunk struct {
	Answer       string                 `json:"answer"`
	Reference    map[string]interface{} `json:"reference"`
	AudioBinary  interface{}            `json:"audio_binary"`
	Prompt       string                 `json:"prompt"`
	CreatedAt    float64                `json:"created_at"`
	Final        bool                   `json:"final"`
	ID           string                 `json:"id"`
	SessionID    string                 `json:"session_id"`
	ChatID       string                 `json:"chat_id,omitempty"`
	StartToThink bool                   `json:"start_to_think,omitempty"`
	EndToThink   bool                   `json:"end_to_think,omitempty"`
}

// sseWrapper wraps the SSE response with deterministic field order matching Python:
//
//	{"code": 0, "message": "", "data": ...}
type sseWrapper struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

// marshalJSONWithSpaces marshals v to JSON and adds spaces after ':' and ','
// to match Python's json.dumps format.
func marshalJSONWithSpaces(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return addJSONSpacesOutsideStrings(data)
}

func addJSONSpacesOutsideStrings(data []byte) string {
	var b strings.Builder
	b.Grow(len(data) + 16)
	inString := false
	escaped := false
	for _, c := range data {
		b.WriteByte(c)
		if escaped {
			escaped = false
			continue
		}
		if inString && c == '\\' {
			escaped = true
			continue
		}
		if c == '"' {
			inString = !inString
			continue
		}
		if !inString && (c == ':' || c == ',') {
			b.WriteByte(' ')
		}
	}
	return b.String()
}

// sanitizeJSONFloats recursively replaces NaN/Infinity with nil.
// Matches Python's _sanitize_json_floats in chat_api.py.
func sanitizeJSONFloats(v interface{}) interface{} {
	switch val := v.(type) {
	case float64:
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil
		}
		return val
	case float32:
		if math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			return nil
		}
		return val
	case map[string]interface{}:
		out := make(map[string]interface{}, len(val))
		for k, vv := range val {
			out[k] = sanitizeJSONFloats(vv)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(val))
		for i, vv := range val {
			out[i] = sanitizeJSONFloats(vv)
		}
		return out
	case []map[string]interface{}:
		out := make([]map[string]interface{}, len(val))
		for i, item := range val {
			sanitized, _ := sanitizeJSONFloats(item).(map[string]interface{})
			out[i] = sanitized
		}
		return out
	default:
		return v
	}
}

// sseMarshalChunk converts an answer map to the ordered sseAnswerChunk struct
// and marshals it with Python-compatible JSON formatting (spaces, field order).
func sseMarshalChunk(ans map[string]interface{}, chatID string) string {
	ref, _ := ans["reference"].(map[string]interface{})
	if ref == nil {
		ref = map[string]interface{}{"chunks": []interface{}{}}
	}
	answer, _ := ans["answer"].(string)
	prompt, _ := ans["prompt"].(string)
	id, _ := ans["id"].(string)
	sessionID, _ := ans["session_id"].(string)
	createdAt, _ := ans["created_at"].(float64)
	final, _ := ans["final"].(bool)

	startToThink, _ := ans["start_to_think"].(bool)
	endToThink, _ := ans["end_to_think"].(bool)

	chunk := sseAnswerChunk{
		Answer:       answer,
		Reference:    ref,
		AudioBinary:  ans["audio_binary"],
		Prompt:       prompt,
		CreatedAt:    createdAt,
		Final:        final,
		ID:           id,
		SessionID:    sessionID,
		ChatID:       chatID,
		StartToThink: startToThink,
		EndToThink:   endToThink,
	}
	wrapper := sseWrapper{Code: 0, Message: "", Data: chunk}
	return marshalJSONWithSpaces(wrapper)
}

func (s *ChatSessionService) structureAnswer(session *entity.ChatSession, answer string, messageID, conversationID string, reference []interface{}) map[string]interface{} {
	// Match Python's structure_answer output:
	// {"answer", "reference": {"chunks": [...]}, "audio_binary": null, "prompt": "",
	//  "created_at": ..., "final": false, "id": "...", "session_id": "..."}
	refMap := map[string]interface{}{
		"chunks":   []interface{}{},
		"doc_aggs": []interface{}{},
	}
	if len(reference) > 0 {
		if latest, ok := reference[len(reference)-1].(map[string]interface{}); ok && latest != nil {
			refMap = latest
			if _, ok := refMap["chunks"]; !ok {
				refMap["chunks"] = []interface{}{}
			}
		}
	}
	return map[string]interface{}{
		"answer":       answer,
		"reference":    refMap,
		"audio_binary": nil,
		"prompt":       "",
		"created_at":   float64(time.Now().UnixNano()) / 1e9,
		"final":        false,
		"id":           messageID,
		"session_id":   conversationID,
	}
}

func (s *ChatSessionService) updateSessionMessages(session *entity.ChatSession, messages []map[string]interface{}, reference []interface{}) {
	messagesJSON, err := json.Marshal(messages)
	if err != nil {
		common.Warn("updateSessionMessages: failed to marshal messages", zap.Error(err))
		return
	}
	referenceJSON, err := json.Marshal(reference)
	if err != nil {
		common.Warn("updateSessionMessages: failed to marshal reference", zap.Error(err))
		return
	}

	updates := map[string]interface{}{
		"message":   messagesJSON,
		"reference": referenceJSON,
	}
	if err := s.chatSessionDAO.UpdateByID(session.ID, updates); err != nil {
		common.Warn("updateSessionMessages: DAO update failed", zap.Error(err))
		return
	}
	session.Message = messagesJSON
	session.Reference = referenceJSON
}

// structureAnswerWithConv structures the answer with conversation update (like Python's structure_answer)
func (s *ChatSessionService) structureAnswerWithConv(session *entity.ChatSession, ans map[string]interface{}, messageID, conversationID string, reference []interface{}) map[string]interface{} {
	// Extract reference from answer
	ref, _ := ans["reference"].(map[string]interface{})
	if ref == nil {
		ref = map[string]interface{}{
			"chunks":   []map[string]interface{}{},
			"doc_aggs": []interface{}{},
		}
		ans["reference"] = ref
	}

	// Format chunks
	chunkList := s.chunksFormat(ref)
	ref["chunks"] = chunkList

	// Add message ID and session ID
	ans["id"] = messageID
	ans["session_id"] = conversationID

	// Update session message
	content, _ := ans["answer"].(string)
	if ans["start_to_think"] != nil {
		content = "<think>"
	} else if ans["end_to_think"] != nil {
		content = "</think>"
	}

	// Parse existing messages. Keep backward compatibility with wrapped legacy rows.
	messages := parseMessages(session.Message)

	// Update or append assistant message
	if len(messages) == 0 || s.getLastRole(messages) != "assistant" {
		messages = append(messages, map[string]interface{}{
			"role":       "assistant",
			"content":    content,
			"created_at": float64(time.Now().Unix()),
			"id":         messageID,
		})
	} else {
		lastIdx := len(messages) - 1
		lastMsg := messages[lastIdx]
		if ans["final"] == true && ans["answer"] != nil {
			lastMsg["content"] = ans["answer"]
		} else {
			existing, _ := lastMsg["content"].(string)
			lastMsg["content"] = existing + content
		}
		lastMsg["created_at"] = float64(time.Now().Unix())
		lastMsg["id"] = messageID
		messages[lastIdx] = lastMsg
	}

	session.Message, _ = json.Marshal(messages)

	// Update reference
	if len(reference) > 0 {
		reference[len(reference)-1] = ref
	}

	return ans
}

// getLastRole gets the role of the last message
func (s *ChatSessionService) getLastRole(messages []map[string]interface{}) string {
	if len(messages) == 0 {
		return ""
	}
	role, _ := messages[len(messages)-1]["role"].(string)
	return role
}

// chunksFormat normalizes chunk fields to a canonical schema (matching
// formatChunks in openai_chat.go and Python's chunks_format), returning
// []map[string]interface{} for JSON serialization.
func (s *ChatSessionService) chunksFormat(reference map[string]interface{}) []map[string]interface{} {
	raw, ok := reference["chunks"].([]map[string]interface{})
	if !ok {
		// Coerce []interface{} → []map[string]interface{}
		if ifaces, ok2 := reference["chunks"].([]interface{}); ok2 {
			raw = make([]map[string]interface{}, 0, len(ifaces))
			for _, item := range ifaces {
				if m, ok3 := item.(map[string]interface{}); ok3 {
					raw = append(raw, m)
				}
			}
		}
	}
	if len(raw) == 0 {
		return []map[string]interface{}{}
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, chunk := range raw {
		out = append(out, map[string]interface{}{
			"id":                getValue(chunk, "chunk_id", "id"),
			"content":           getValue(chunk, "content_with_weight", "content"),
			"document_id":       getValue(chunk, "doc_id", "document_id"),
			"document_name":     getValue(chunk, "docnm_kwd", "document_name"),
			"dataset_id":        getValue(chunk, "kb_id", "dataset_id"),
			"image_id":          getValue(chunk, "image_id", "img_id"),
			"positions":         getValue(chunk, "positions", "position_int"),
			"url":               chunk["url"],
			"similarity":        sanitizeJSONFloats(chunk["similarity"]),
			"vector_similarity": sanitizeJSONFloats(chunk["vector_similarity"]),
			"term_similarity":   sanitizeJSONFloats(chunk["term_similarity"]),
			"row_id":            chunk["row_id"],
			"doc_type":          getValue(chunk, "doc_type_kwd", "doc_type"),
			"document_metadata": chunk["document_metadata"],
		})
	}
	return out
}
