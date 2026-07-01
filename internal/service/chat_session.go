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
	"ragflow/internal/common"
	"ragflow/internal/storage"
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
	AsyncChat(ctx context.Context, chat *entity.Chat, messages []map[string]interface{}, stream bool, kwargs map[string]interface{}) (<-chan AsyncChatResult, error)
}

// chunkFeedbackApplier is the dispatch seam for chunk-level feedback
// persistence. Mirrors the Python ChunkFeedbackService.apply_feedback
// (api/db/services/chunk_feedback_service.py) call site at
// api/apps/restful_apis/chat_api.py — that handler records the thumb
// vote against every chunk that produced the assistant message, in
// addition to the session-level thumbup field. The Go stack does not
// yet have a chunk-feedback DAO, so this interface is the seam where
// one will plug in. Production uses *ChatSessionService itself via
// applyChunkFeedback (which currently no-ops with a debug log) so the
// handler can still update the session-level thumbup without crashing;
// tests can swap in a fake by setting ChatSessionService.chunkFeedbackApplier.
type chunkFeedbackApplier interface {
	applyChunkFeedback(tenantID string, reference map[string]interface{}, isPositive bool) (map[string]interface{}, error)
}

// ChatSessionService chat session (conversation) service.
// The RAG pipeline is delegated to ChatPipelineService.
type ChatSessionService struct {
	chatSessionDAO       chatSessionStore
	userTenantDAO        userTenantStore
	pipeline             chatPipelineRunner
	chunkFeedbackApplier chunkFeedbackApplier
}

// NewChatSessionService create chat session service
func NewChatSessionService() *ChatSessionService {
	return &ChatSessionService{
		chatSessionDAO: dao.NewChatSessionDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
		pipeline:       NewChatPipelineService(),
	}
}

// SetChatSessionRequest set chat session request
type SetChatSessionRequest struct {
	SessionID string `json:"conversation_id,omitempty"`
	DialogID  string `json:"dialog_id,omitempty"`
	Name      string `json:"name,omitempty"`
	IsNew     bool   `json:"is_new"`
}

// SetChatSessionResponse set chat session response
type SetChatSessionResponse struct {
	*entity.ChatSession
}

// SetChatSession create or update a chat session
func (s *ChatSessionService) SetChatSession(userID string, req *SetChatSessionRequest) (*SetChatSessionResponse, error) {
	name := req.Name
	if name == "" {
		name = "New chat session"
	}
	// Limit name length to 255 characters
	if len(name) > 255 {
		name = name[:255]
	}

	if !req.IsNew {
		// Update existing chat session
		updates := map[string]interface{}{
			"name":    name,
			"user_id": userID,
		}

		if err := s.chatSessionDAO.UpdateByID(req.SessionID, updates); err != nil {
			return nil, errors.New("Chat session not found")
		}

		// Get updated chat session
		session, err := s.chatSessionDAO.GetByID(req.SessionID)
		if err != nil {
			return nil, errors.New("Fail to update a chat session")
		}

		return &SetChatSessionResponse{ChatSession: session}, nil
	}

	// Create new chat session
	// Check if dialog exists
	dialog, err := s.chatSessionDAO.GetDialogByID(req.DialogID)
	if err != nil {
		return nil, errors.New("Dialog not found")
	}

	// Generate UUID for new chat session
	newID := common.GenerateUUID()

	// Get prologue from dialog's prompt_config
	prologue := "Hi! I'm your assistant. What can I do for you?"
	if dialog.PromptConfig != nil {
		if p, ok := dialog.PromptConfig["prologue"].(string); ok && p != "" {
			prologue = p
		}
	}

	// Store messages in the same list shape as Python Conversation.message.
	messagesJSON, _ := json.Marshal([]map[string]interface{}{
		{
			"role":    "assistant",
			"content": prologue,
		},
	})

	// Create reference - store as JSON array
	referenceJSON, _ := json.Marshal([]interface{}{})

	// Create chat session
	session := &entity.ChatSession{
		ID:        newID,
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

// RemoveChatSessions removes chat sessions (hard delete)
func (s *ChatSessionService) RemoveChatSessions(userID string, chatSessions []string) error {
	// Get user's tenants
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return err
	}

	// Build a set of user's tenant IDs for quick lookup
	tenantIDSet := make(map[string]bool)
	for _, tid := range tenantIDs {
		tenantIDSet[tid] = true
	}
	tenantIDSet[userID] = true

	// Check each chat session
	for _, convID := range chatSessions {
		// Get the chat session
		session, err := s.chatSessionDAO.GetByID(convID)
		if err != nil {
			return fmt.Errorf("Chat session not found: %s", convID)
		}

		// Check if user is the owner by checking dialog ownership
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

		// Delete the chat session
		if err := s.chatSessionDAO.DeleteByID(convID); err != nil {
			return err
		}
	}

	return nil
}

// ListChatSessionsRequest list chat sessions request
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
		ID:        common.GenerateUUID(),
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

func (s *ChatSessionService) UpdateMessageFeedback(userID, chatID, sessionID, msgID string, req map[string]interface{}) (*ChatSessionPayload, common.ErrorCode, error) {
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
			result, _ := applier.applyChunkFeedback(ownerTenantID, feedbackReference, !priorThumbBool)
			if result != nil {
				common.Debug("Chunk feedback undo applied",
					zap.Any("success_count", result["success_count"]),
					zap.Any("fail_count", result["fail_count"]),
				)
			}
		}
		result, _ := applier.applyChunkFeedback(ownerTenantID, feedbackReference, thumbup)
		if result != nil {
			common.Debug("Chunk feedback applied",
				zap.Any("success_count", result["success_count"]),
				zap.Any("fail_count", result["fail_count"]),
			)
		}
	}

	return s.buildSessionPayload(session, nil, false), common.CodeSuccess, nil
}

// applyChunkFeedback records a thumb vote against the chunks that
// produced a session message. Mirrors Python's
// ChunkFeedbackService.apply_feedback side effect (called from
// api/apps/restful_apis/chat_api.py when a user toggles a thumb on
// an assistant message). The Go persistence port for chunk feedback
// is intentionally not yet landed — the call here is a documented
// no-op so the session-level thumbup flow (the user-visible behavior)
// keeps working while a future PR ports the Python DAO. The returned
// counts let the caller log a "Chunk feedback applied: N succeeded,
// M failed" line consistent with the Python equivalent, so log
// scrapers don't see a regression in success/fail rates.
//
// Production callers should always go through the chunkFeedbackApplier
// field; this method is the default implementation used when that
// field is nil.
func (s *ChatSessionService) applyChunkFeedback(tenantID string, reference map[string]interface{}, isPositive bool) (map[string]interface{}, error) {
	common.Debug("chunk feedback persistence not yet ported; dropping vote",
		zap.String("tenant_id", tenantID),
		zap.Bool("is_positive", isPositive),
	)
	return map[string]interface{}{
		"success_count": 0,
		"fail_count":    0,
	}, nil
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
func (s *ChatSessionService) Completion(userID string, conversationID string, messages []map[string]interface{}, llmID string, chatModelConfig map[string]interface{}, messageID string) (map[string]interface{}, error) {
	// Validate the last message is from user
	if len(messages) == 0 {
		return nil, errors.New("messages cannot be empty")
	}
	lastRole, _ := messages[len(messages)-1]["role"].(string)
	if lastRole != "user" {
		return nil, errors.New("the last content of this conversation is not from user")
	}

	// Get conversation
	session, err := s.chatSessionDAO.GetByID(conversationID)
	if err != nil {
		return nil, errors.New("Conversation not found")
	}

	// Get dialog
	dialog, err := s.chatSessionDAO.GetDialogByID(session.DialogID)
	if err != nil {
		return nil, errors.New("Dialog not found")
	}

	// Deep copy messages to session, preserving the stored prologue that handler strips from requests.
	sessionMessages := s.buildSessionMessages(session, messages)

	// Initialize reference if empty
	reference := s.initializeReference(session)

	// Check if custom LLM is specified and validate API key
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

	// Perform chat completion via shared RAG pipeline
	ctx := context.Background()
	kwargs := chatModelConfig
	if kwargs == nil {
		kwargs = map[string]interface{}{}
	}
	resultChan, err := s.pipeline.AsyncChat(ctx, dialog, messages, false, kwargs)
	if err != nil {
		return nil, err
	}

	// Collect results from the pipeline
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

	// Structure the answer
	ans := map[string]interface{}{
		"answer":    answer.String(),
		"reference": finalRef,
		"final":     true,
	}
	result := s.structureAnswerWithConv(session, ans, messageID, session.ID, reference)

	// Update conversation if not embedded
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
func (s *ChatSessionService) CompletionStream(ctx context.Context, userID string, conversationID string, messages []map[string]interface{}, llmID string, chatModelConfig map[string]interface{}, messageID string, streamChan chan<- string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Validate the last message is from user
	if len(messages) == 0 {
		streamChan <- fmt.Sprintf("data: %s\n\n", `{"code": 500, "message": "messages cannot be empty", "data": {"answer": "**ERROR**: messages cannot be empty", "reference": []}}`)
		return errors.New("messages cannot be empty")
	}
	lastRole, _ := messages[len(messages)-1]["role"].(string)
	if lastRole != "user" {
		streamChan <- fmt.Sprintf("data: %s\n\n", `{"code": 500, "message": "the last content of this conversation is not from user", "data": {"answer": "**ERROR**: the last content of this conversation is not from user", "reference": []}}`)
		return errors.New("the last content of this conversation is not from user")
	}

	// Get conversation
	session, err := s.chatSessionDAO.GetByID(conversationID)
	if err != nil {
		streamChan <- fmt.Sprintf("data: %s\n\n", `{"code": 500, "message": "Conversation not found", "data": {"answer": "**ERROR**: Conversation not found", "reference": []}}`)
		return errors.New("Conversation not found")
	}

	// Get dialog
	dialog, err := s.chatSessionDAO.GetDialogByID(session.DialogID)
	if err != nil {
		streamChan <- fmt.Sprintf("data: %s\n\n", `{"code": 500, "message": "Dialog not found", "data": {"answer": "**ERROR**: Dialog not found", "reference": []}}`)
		return errors.New("Dialog not found")
	}

	// Deep copy messages to session, preserving the stored prologue that handler strips from requests.
	sessionMessages := s.buildSessionMessages(session, messages)

	// Initialize reference if empty
	reference := s.initializeReference(session)

	// Check if custom LLM is specified and validate API key
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

	// Perform streaming chat via shared RAG pipeline
	kwargs := chatModelConfig
	if kwargs == nil {
		kwargs = map[string]interface{}{}
	}
	resultChan, err := s.pipeline.AsyncChat(ctx, dialog, messages, true, kwargs)
	if err != nil {
		streamChan <- fmt.Sprintf("data: %s\n\n", fmt.Sprintf(`{"code": 500, "message": "%s", "data": {"answer": "**ERROR**: %s", "reference": []}}`, err.Error(), err.Error()))
		return err
	}

	// Stream results, accumulating the answer
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

	// Send final completion signal
	finalData, _ := json.Marshal(map[string]interface{}{
		"code":    0,
		"message": "",
		"data":    true,
	})
	streamChan <- fmt.Sprintf("data: %s\n\n", string(finalData))

	// Update conversation if not embedded
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
	// Simplified check - in real implementation, check if tenant has API key for this model
	return true, nil
}

func (s *ChatSessionService) structureAnswer(session *entity.ChatSession, answer string, messageID, conversationID string, reference []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"answer":          answer,
		"reference":       reference,
		"conversation_id": conversationID,
		"message_id":      messageID,
	}
}

func (s *ChatSessionService) updateSessionMessages(session *entity.ChatSession, messages []map[string]interface{}, reference []interface{}) {
	// Update session with new messages and reference
	messagesJSON, _ := json.Marshal(messages)
	referenceJSON, _ := json.Marshal(reference)

	updates := map[string]interface{}{
		"message":   messagesJSON,
		"reference": referenceJSON,
	}
	s.chatSessionDAO.UpdateByID(session.ID, updates)
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

// chunksFormat formats chunks for reference (simplified version)
func (s *ChatSessionService) chunksFormat(reference map[string]interface{}) []map[string]interface{} {
	switch c := reference["chunks"].(type) {
	case []map[string]interface{}:
		formatted := make([]map[string]interface{}, len(c))
		copy(formatted, c)
		return formatted
	case []interface{}:
		formatted := make([]map[string]interface{}, 0, len(c))
		for _, item := range c {
			if m, ok := item.(map[string]interface{}); ok {
				formatted = append(formatted, m)
			}
		}
		return formatted
	default:
		return []map[string]interface{}{}
	}
}
