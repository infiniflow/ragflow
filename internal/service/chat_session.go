package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// ChatSessionService chat session (conversation) service
type ChatSessionService struct {
	chatSessionDAO *dao.ChatSessionDAO
	chatDAO        *dao.ChatDAO
	userTenantDAO  *dao.UserTenantDAO
}

// NewChatSessionService create chat session service
func NewChatSessionService() *ChatSessionService {
	return &ChatSessionService{
		chatSessionDAO: dao.NewChatSessionDAO(),
		chatDAO:        dao.NewChatDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
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
	*model.ChatSession
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
			"name":        name,
			"user_id":     userID,
			"update_time": time.Now().UnixMilli(),
			"update_date": time.Now(),
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
	newID := uuid.New().String()
	newID = strings.ReplaceAll(newID, "-", "")
	if len(newID) > 32 {
		newID = newID[:32]
	}

	// Get prologue from dialog's prompt_config
	prologue := "Hi! I'm your assistant. What can I do for you?"
	if dialog.PromptConfig != nil {
		if p, ok := dialog.PromptConfig["prologue"].(string); ok && p != "" {
			prologue = p
		}
	}

	now := time.Now()
	createTime := now.UnixMilli()

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
	session := &model.ChatSession{
		ID:        newID,
		DialogID:  req.DialogID,
		Name:      &name,
		Message:   messagesJSON,
		UserID:    &userID,
		Reference: referenceJSON,
	}
	session.CreateTime = createTime
	session.CreateDate = &now
	session.UpdateTime = &createTime
	session.UpdateDate = &now

	if err := s.chatSessionDAO.Create(session); err != nil {
		return nil, errors.New("Fail to create a chat session")
	}

	return &SetChatSessionResponse{ChatSession: session}, nil
}

// RemoveChatSessionRequest remove chat sessions request
type RemoveChatSessionRequest struct {
	ChatSessions []string `json:"conversation_ids" binding:"required"`
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
	Sessions []*model.ChatSession
}

// ListChatSessions lists chat sessions for a dialog
func (s *ChatSessionService) ListChatSessions(userID string, dialogID string) (*ListChatSessionsResponse, error) {
	// Get user's tenants
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, err
	}

	// Check if user is the owner of the dialog
	isOwner := false
	for _, tenantID := range tenantIDs {
		exists, err := s.chatSessionDAO.CheckDialogExists(tenantID, dialogID)
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
		exists, err := s.chatSessionDAO.CheckDialogExists(userID, dialogID)
		if err != nil {
			return nil, err
		}
		isOwner = exists
	}

	if !isOwner {
		return nil, errors.New("Only owner of dialog authorized for this operation")
	}

	// List chat sessions
	sessions, err := s.chatSessionDAO.ListByDialogID(dialogID)
	if err != nil {
		return nil, err
	}

	return &ListChatSessionsResponse{Sessions: sessions}, nil
}

// Completion performs chat completion with full RAG support
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

	// Perform chat completion with RAG
	result, err := s.asyncChat(dialog, session, messages, chatModelConfig, messageID, reference, false)
	if err != nil {
		return nil, err
	}

	// Update conversation if not embedded
	if !isEmbedded {
		s.updateSessionMessages(session, sessionMessages, reference)
	}

	return result, nil
}

// CompletionStream performs streaming chat completion with full RAG support
func (s *ChatSessionService) CompletionStream(userID string, conversationID string, messages []map[string]interface{}, llmID string, chatModelConfig map[string]interface{}, messageID string, streamChan chan<- string) error {
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

	// Perform streaming chat completion with RAG
	resultChan, err := s.asyncChatStream(dialog, session, messages, chatModelConfig, messageID, reference)
	if err != nil {
		streamChan <- fmt.Sprintf("data: %s\n\n", fmt.Sprintf(`{"code": 500, "message": "%s", "data": {"answer": "**ERROR**: %s", "reference": []}}`, err.Error(), err.Error()))
		return err
	}

	// Stream results
	for result := range resultChan {
		data, _ := json.Marshal(map[string]interface{}{
			"code":    0,
			"message": "",
			"data":    result,
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

func (s *ChatSessionService) buildSessionMessages(session *model.ChatSession, messages []map[string]interface{}) []map[string]interface{} {
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

func (s *ChatSessionService) initializeReference(session *model.ChatSession) []interface{} {
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
		"chunks":   []interface{}{},
		"doc_aggs": []interface{}{},
	})
	return filtered
}

func (s *ChatSessionService) checkTenantLLMAPIKey(tenantID, modelName string) (bool, error) {
	// Simplified check - in real implementation, check if tenant has API key for this model
	return true, nil
}

func (s *ChatSessionService) performChat(dialog *model.Chat, messages []map[string]interface{}, config map[string]interface{}) (string, error) {
	// Get system prompt from dialog
	systemPrompt := ""
	if dialog.PromptConfig != nil {
		if sys, ok := dialog.PromptConfig["system"].(string); ok {
			systemPrompt = sys
		}
	}

	// Convert messages to history format
	history := make([]map[string]string, 0)
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if role != "" && content != "" {
			history = append(history, map[string]string{
				"role":    role,
				"content": content,
			})
		}
	}

	// Use ModelBundle to perform chat
	bundle, err := NewModelBundle(dialog.TenantID, model.ModelTypeChat, dialog.LLMID)
	if err != nil {
		return "", err
	}

	// Merge dialog's LLM setting with request config
	genConf := make(map[string]interface{})
	if dialog.LLMSetting != nil {
		for k, v := range dialog.LLMSetting {
			genConf[k] = v
		}
	}
	for k, v := range config {
		genConf[k] = v
	}

	response, _, err := bundle.Chat(systemPrompt, history, genConf)
	return response, err
}

func (s *ChatSessionService) performChatStream(dialog *model.Chat, messages []map[string]interface{}, config map[string]interface{}) (<-chan string, error) {
	// Get system prompt from dialog
	systemPrompt := ""
	if dialog.PromptConfig != nil {
		if sys, ok := dialog.PromptConfig["system"].(string); ok {
			systemPrompt = sys
		}
	}

	// Convert messages to history format
	history := make([]map[string]string, 0)
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if role != "" && content != "" {
			history = append(history, map[string]string{
				"role":    role,
				"content": content,
			})
		}
	}

	// Use ModelBundle to perform streaming chat
	bundle, err := NewModelBundle(dialog.TenantID, model.ModelTypeChat, dialog.LLMID)
	if err != nil {
		return nil, err
	}

	// Merge dialog's LLM setting with request config
	genConf := make(map[string]interface{})
	if dialog.LLMSetting != nil {
		for k, v := range dialog.LLMSetting {
			genConf[k] = v
		}
	}
	for k, v := range config {
		genConf[k] = v
	}

	// Get chat model and call ChatStreamly
	chatModel, ok := bundle.GetModel().(model.ChatModel)
	if !ok {
		return nil, fmt.Errorf("model is not a chat model")
	}

	return chatModel.ChatStreamly(systemPrompt, history, genConf)
}

func (s *ChatSessionService) structureAnswer(session *model.ChatSession, answer string, messageID, conversationID string, reference []interface{}) map[string]interface{} {
	return map[string]interface{}{
		"answer":          answer,
		"reference":       reference,
		"conversation_id": conversationID,
		"message_id":      messageID,
	}
}

func (s *ChatSessionService) updateSessionMessages(session *model.ChatSession, messages []map[string]interface{}, reference []interface{}) {
	// Update session with new messages and reference
	messagesJSON, _ := json.Marshal(map[string]interface{}{
		"messages": messages,
	})
	referenceJSON, _ := json.Marshal(reference)

	updates := map[string]interface{}{
		"message":     messagesJSON,
		"reference":   referenceJSON,
		"update_time": time.Now().UnixMilli(),
		"update_date": time.Now(),
	}
	s.chatSessionDAO.UpdateByID(session.ID, updates)
}

// asyncChat performs chat with RAG support (non-streaming)
func (s *ChatSessionService) asyncChat(dialog *model.Chat, session *model.ChatSession, messages []map[string]interface{}, config map[string]interface{}, messageID string, reference []interface{}, stream bool) (map[string]interface{}, error) {
	// Check if we need RAG (knowledge base or tavily)
	hasKB := len(dialog.KBIDs) > 0
	hasTavily := false
	if dialog.PromptConfig != nil {
		if tavilyKey, ok := dialog.PromptConfig["tavily_api_key"].(string); ok && tavilyKey != "" {
			hasTavily = true
		}
	}

	if !hasKB && !hasTavily {
		// Simple chat without RAG
		return s.asyncChatSolo(dialog, session, messages, config, messageID, reference, stream)
	}

	// TODO: Full RAG implementation with knowledge base retrieval
	// This would include:
	// 1. Get embedding model and rerank model
	// 2. Extract questions from messages
	// 3. Retrieve chunks from knowledge bases
	// 4. Rerank chunks
	// 5. Build prompt with context
	// 6. Call LLM

	// For now, fall back to solo chat
	return s.asyncChatSolo(dialog, session, messages, config, messageID, reference, stream)
}

// asyncChatStream performs streaming chat with RAG support
func (s *ChatSessionService) asyncChatStream(dialog *model.Chat, session *model.ChatSession, messages []map[string]interface{}, config map[string]interface{}, messageID string, reference []interface{}) (<-chan map[string]interface{}, error) {
	resultChan := make(chan map[string]interface{})

	go func() {
		defer close(resultChan)

		// Check if we need RAG
		hasKB := len(dialog.KBIDs) > 0
		hasTavily := false
		if dialog.PromptConfig != nil {
			if tavilyKey, ok := dialog.PromptConfig["tavily_api_key"].(string); ok && tavilyKey != "" {
				hasTavily = true
			}
		}

		if !hasKB && !hasTavily {
			// Simple chat without RAG
			s.asyncChatSoloStream(dialog, session, messages, config, messageID, reference, resultChan)
			return
		}

		// TODO: Full RAG streaming implementation
		// For now, fall back to solo chat
		s.asyncChatSoloStream(dialog, session, messages, config, messageID, reference, resultChan)
	}()

	return resultChan, nil
}

// asyncChatSolo performs simple chat without RAG (non-streaming)
func (s *ChatSessionService) asyncChatSolo(dialog *model.Chat, session *model.ChatSession, messages []map[string]interface{}, config map[string]interface{}, messageID string, reference []interface{}, stream bool) (map[string]interface{}, error) {
	// Get system prompt
	systemPrompt := s.buildSystemPrompt(dialog)

	// Process messages - handle attachments and image files
	processedMessages := s.processMessages(messages, dialog)

	// Get LLM type
	llmType := s.getLLMType(dialog.LLMID)

	// Build generation config
	genConf := s.buildGenConf(dialog, config)

	// Create ModelBundle for chat
	var bundle *ModelBundle
	var err error
	if llmType == "image2text" {
		bundle, err = NewModelBundle(dialog.TenantID, model.ModelTypeImage2Text, dialog.LLMID)
	} else {
		bundle, err = NewModelBundle(dialog.TenantID, model.ModelTypeChat, dialog.LLMID)
	}
	if err != nil {
		return nil, err
	}

	// Convert messages to history format
	history := s.convertToHistory(processedMessages)

	// Perform chat
	response, _, err := bundle.Chat(systemPrompt, history, genConf)
	if err != nil {
		return nil, err
	}

	// Structure the answer
	ans := map[string]interface{}{
		"answer":    response,
		"reference": reference[len(reference)-1],
		"final":     true,
	}

	return s.structureAnswerWithConv(session, ans, messageID, session.ID, reference), nil
}

// asyncChatSoloStream performs simple streaming chat without RAG
func (s *ChatSessionService) asyncChatSoloStream(dialog *model.Chat, session *model.ChatSession, messages []map[string]interface{}, config map[string]interface{}, messageID string, reference []interface{}, resultChan chan<- map[string]interface{}) {
	// Get system prompt
	systemPrompt := s.buildSystemPrompt(dialog)

	// Process messages
	processedMessages := s.processMessages(messages, dialog)

	// Get LLM type
	llmType := s.getLLMType(dialog.LLMID)

	// Build generation config
	genConf := s.buildGenConf(dialog, config)

	// Create ModelBundle
	var bundle *ModelBundle
	var err error
	if llmType == "image2text" {
		bundle, err = NewModelBundle(dialog.TenantID, model.ModelTypeImage2Text, dialog.LLMID)
	} else {
		bundle, err = NewModelBundle(dialog.TenantID, model.ModelTypeChat, dialog.LLMID)
	}
	if err != nil {
		resultChan <- s.structureAnswer(session, "**ERROR**: "+err.Error(), messageID, session.ID, reference)
		return
	}

	// Convert messages to history
	history := s.convertToHistory(processedMessages)

	// Get chat model
	chatModel, ok := bundle.GetModel().(model.ChatModel)
	if !ok {
		resultChan <- s.structureAnswer(session, "**ERROR**: model is not a chat model", messageID, session.ID, reference)
		return
	}

	// Perform streaming chat
	streamChan, err := chatModel.ChatStreamly(systemPrompt, history, genConf)
	if err != nil {
		resultChan <- s.structureAnswer(session, "**ERROR**: "+err.Error(), messageID, session.ID, reference)
		return
	}

	// Stream results
	fullAnswer := ""
	for chunk := range streamChan {
		fullAnswer += chunk
		// Clean up reasoning content
		fullAnswer = s.removeReasoningContent(fullAnswer)
		ans := s.structureAnswer(session, fullAnswer, messageID, session.ID, reference)
		resultChan <- ans
	}
}

// buildSystemPrompt builds the system prompt from dialog configuration
func (s *ChatSessionService) buildSystemPrompt(dialog *model.Chat) string {
	if dialog.PromptConfig == nil {
		return ""
	}

	system, _ := dialog.PromptConfig["system"].(string)
	return system
}

// processMessages processes messages and handles attachments
func (s *ChatSessionService) processMessages(messages []map[string]interface{}, dialog *model.Chat) []map[string]interface{} {
	// Process each message
	processed := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		processed[i] = make(map[string]interface{})
		for k, v := range msg {
			processed[i][k] = v
		}

		// Clean content - remove file markers
		if content, ok := msg["content"].(string); ok {
			content = s.cleanContent(content)
			processed[i]["content"] = content
		}
	}

	return processed
}

// cleanContent removes file markers from content
func (s *ChatSessionService) cleanContent(content string) string {
	// Remove ##N$$ markers
	// This is a simplified version - full implementation would use regex
	return content
}

// convertToHistory converts messages to history format for LLM
func (s *ChatSessionService) convertToHistory(messages []map[string]interface{}) []map[string]string {
	history := make([]map[string]string, 0)
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		content, _ := msg["content"].(string)
		if role != "" && content != "" && role != "system" {
			history = append(history, map[string]string{
				"role":    role,
				"content": content,
			})
		}
	}
	return history
}

// buildGenConf builds generation config from dialog and request
func (s *ChatSessionService) buildGenConf(dialog *model.Chat, config map[string]interface{}) map[string]interface{} {
	genConf := make(map[string]interface{})

	// Start with dialog's LLM setting
	if dialog.LLMSetting != nil {
		for k, v := range dialog.LLMSetting {
			genConf[k] = v
		}
	}

	// Override with request config
	for k, v := range config {
		genConf[k] = v
	}

	return genConf
}

// getLLMType gets the LLM type from model ID
func (s *ChatSessionService) getLLMType(llmID string) string {
	// Simplified - would need to query TenantLLMService
	if strings.Contains(llmID, "image") || strings.Contains(llmID, "vision") {
		return "image2text"
	}
	return "chat"
}

// removeReasoningContent removes reasoning/thinking content from answer
func (s *ChatSessionService) removeReasoningContent(answer string) string {
	// Remove </think> tags
	if strings.HasSuffix(answer, "</think>") {
		answer = answer[:len(answer)-len("</think>")]
	}
	return answer
}

// structureAnswerWithConv structures the answer with conversation update (like Python's structure_answer)
func (s *ChatSessionService) structureAnswerWithConv(session *model.ChatSession, ans map[string]interface{}, messageID, conversationID string, reference []interface{}) map[string]interface{} {
	// Extract reference from answer
	ref, _ := ans["reference"].(map[string]interface{})
	if ref == nil {
		ref = map[string]interface{}{
			"chunks":   []interface{}{},
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
				lastMsg["content"] = (lastMsg["content"].(string)) + content
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
func (s *ChatSessionService) chunksFormat(reference map[string]interface{}) []interface{} {
	chunks, _ := reference["chunks"].([]interface{})
	if chunks == nil {
		return []interface{}{}
	}

	// Format each chunk
	formatted := make([]interface{}, len(chunks))
	for i, chunk := range chunks {
		formatted[i] = chunk
	}
	return formatted
}
