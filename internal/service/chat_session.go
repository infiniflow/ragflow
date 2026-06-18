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
	"strings"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// Interfaces for testability — satisfied by the concrete DAO/pipeline types.

type chatSessionStore interface {
	GetByID(id string) (*entity.ChatSession, error)
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

// ChatSessionService chat session (conversation) service.
// The RAG pipeline is delegated to ChatPipelineService.
type ChatSessionService struct {
	chatSessionDAO chatSessionStore
	userTenantDAO  userTenantStore
	pipeline       chatPipelineRunner
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

	// Create initial message - store as JSON object with messages array
	messagesObj := map[string]interface{}{
		"messages": []map[string]interface{}{
			{
				"role":    "assistant",
				"content": prologue,
			},
		},
	}
	messagesJSON, _ := json.Marshal(messagesObj)

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

	// Deep copy messages to session
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

	// Deep copy messages to session
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
		s.updateSessionMessages(session, sessionMessages, reference)
	}

	return nil
}

// Helper methods

func (s *ChatSessionService) buildSessionMessages(session *entity.ChatSession, messages []map[string]interface{}) []map[string]interface{} {
	// Deep copy messages to session
	sessionMessages := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		sessionMessages[i] = make(map[string]interface{})
		for k, v := range msg {
			sessionMessages[i][k] = v
		}
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
	messagesJSON, _ := json.Marshal(map[string]interface{}{
		"messages": messages,
	})
	referenceJSON, _ := json.Marshal(reference)

	updates := map[string]interface{}{
		"message":   messagesJSON,
		"reference": referenceJSON,
	}
	s.chatSessionDAO.UpdateByID(session.ID, updates)
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

	// Parse existing messages
	var messagesObj map[string]interface{}
	if len(session.Message) > 0 {
		json.Unmarshal(session.Message, &messagesObj)
	}
	messages, _ := messagesObj["messages"].([]interface{})

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
		lastMsg, _ := messages[lastIdx].(map[string]interface{})
		if lastMsg != nil {
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
	}

	// Update reference
	if len(reference) > 0 {
		reference[len(reference)-1] = ref
	}

	return ans
}

// getLastRole gets the role of the last message
func (s *ChatSessionService) getLastRole(messages []interface{}) string {
	if len(messages) == 0 {
		return ""
	}
	lastMsg, _ := messages[len(messages)-1].(map[string]interface{})
	if lastMsg != nil {
		role, _ := lastMsg["role"].(string)
		return role
	}
	return ""
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
