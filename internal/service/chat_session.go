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
	"ragflow/internal/engine"
	"ragflow/internal/service/nlp"
	"strings"
	"time"

	"go.uber.org/zap"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	modelModule "ragflow/internal/entity/models"
)

type chatKnowledgebaseStore interface {
	Accessible(kbID, userID string) bool
	GetByIDs(ids []string) ([]*entity.Knowledgebase, error)
}

type chatModelProvider interface {
	GetChatModel(tenantID, compositeModelName string) (*modelModule.ChatModel, error)
	GetEmbeddingModel(tenantID, compositeModelName string) (*modelModule.EmbeddingModel, error)
	GetRerankModel(tenantID, compositeModelName string) (*modelModule.RerankModel, error)
	GetModelConfigFromProviderInstance(tenantID string, modelType entity.ModelType, modelName string) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error)
	GetTenantDefaultModelByType(tenantID string, modelType entity.ModelType) (modelModule.ModelDriver, string, *modelModule.APIConfig, int, error)
}

type chatMetadataService interface {
	LabelQuestion(question string, kbs []*entity.Knowledgebase) map[string]float64
	GetFlattedMetaByKBs(kbIDs []string) (common.MetaData, error)
}

type chatRetrievalService interface {
	Retrieval(ctx context.Context, req *nlp.RetrievalRequest) (*nlp.RetrievalResult, error)
}

// ChatSessionService chat session (conversation) service
type ChatSessionService struct {
	chatSessionDAO   *dao.ChatSessionDAO
	chatDAO          *dao.ChatDAO
	userTenantDAO    *dao.UserTenantDAO
	kbDAO            chatKnowledgebaseStore
	docEngine        engine.DocEngine
	modelProviderSvc chatModelProvider
	metadataSvc      chatMetadataService
	retrievalSvc     chatRetrievalService
}

// NewChatSessionService create chat session service
func NewChatSessionService() *ChatSessionService {
	docEngine := engine.Get()
	return newChatSessionServiceWithRetrieval(docEngine, nlp.NewRetrievalService(docEngine, dao.NewDocumentDAO()))
}

// NewChatSessionServiceWithRetrieval creates a chat session service with a retrieval service.
func NewChatSessionServiceWithRetrieval(retrievalSvc chatRetrievalService) *ChatSessionService {
	return newChatSessionServiceWithRetrieval(engine.Get(), retrievalSvc)
}

func newChatSessionServiceWithRetrieval(docEngine engine.DocEngine, retrievalSvc chatRetrievalService) *ChatSessionService {
	return &ChatSessionService{
		chatSessionDAO:   dao.NewChatSessionDAO(),
		chatDAO:          dao.NewChatDAO(),
		userTenantDAO:    dao.NewUserTenantDAO(),
		kbDAO:            dao.NewKnowledgebaseDAO(),
		docEngine:        docEngine,
		modelProviderSvc: NewModelProviderService(),
		metadataSvc:      NewMetadataService(),
		retrievalSvc:     retrievalSvc,
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
	result, err := s.asyncChat(userID, dialog, session, messages, chatModelConfig, messageID, reference, false)
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

	// Perform streaming chat completion with RAG
	resultChan, err := s.asyncChatStream(ctx, userID, dialog, session, messages, chatModelConfig, messageID, reference)
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
		"chunks":   []interface{}{},
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

// asyncChat performs chat with RAG support (non-streaming)
func (s *ChatSessionService) asyncChat(userID string, dialog *entity.Chat, session *entity.ChatSession, messages []map[string]interface{}, config map[string]interface{}, messageID string, reference []interface{}, stream bool) (map[string]interface{}, error) {
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

	if hasKB {
		return s.asyncChatWithRetrieval(context.Background(), userID, dialog, session, messages, config, messageID, reference, stream)
	}

	common.Warn("Tavily-backed chat retrieval is not implemented in Go; falling back to solo chat",
		zap.String("dialog_id", dialog.ID))
	return s.asyncChatSolo(dialog, session, messages, config, messageID, reference, stream)
}

// asyncChatStream performs streaming chat with RAG support
func (s *ChatSessionService) asyncChatStream(ctx context.Context, userID string, dialog *entity.Chat, session *entity.ChatSession, messages []map[string]interface{}, config map[string]interface{}, messageID string, reference []interface{}) (<-chan map[string]interface{}, error) {
	if ctx == nil {
		ctx = context.Background()
	}
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

		if hasKB {
			ragMessages, ragDialog, emptyResponse, err := s.messagesWithRetrievedKnowledge(ctx, userID, dialog, messages, reference)
			if err != nil {
				resultChan <- s.structureAnswer(session, "**ERROR**: "+err.Error(), messageID, session.ID, reference)
				return
			}
			if emptyResponse != nil {
				resultChan <- s.structureAnswer(session, *emptyResponse, messageID, session.ID, reference)
				return
			}
			s.asyncChatSoloStream(ragDialog, session, ragMessages, config, messageID, reference, resultChan)
			return
		}

		common.Warn("Tavily-backed streaming chat retrieval is not implemented in Go; falling back to solo chat",
			zap.String("dialog_id", dialog.ID))
		s.asyncChatSoloStream(dialog, session, messages, config, messageID, reference, resultChan)
	}()

	return resultChan, nil
}

func (s *ChatSessionService) asyncChatWithRetrieval(ctx context.Context, userID string, dialog *entity.Chat, session *entity.ChatSession, messages []map[string]interface{}, config map[string]interface{}, messageID string, reference []interface{}, stream bool) (map[string]interface{}, error) {
	ragMessages, ragDialog, emptyResponse, err := s.messagesWithRetrievedKnowledge(ctx, userID, dialog, messages, reference)
	if err != nil {
		return nil, err
	}
	if emptyResponse != nil {
		var lastRef interface{}
		if len(reference) > 0 {
			lastRef = reference[len(reference)-1]
		}
		ans := map[string]interface{}{
			"answer":    *emptyResponse,
			"reference": lastRef,
			"final":     true,
		}
		return s.structureAnswerWithConv(session, ans, messageID, session.ID, reference), nil
	}
	return s.asyncChatSolo(ragDialog, session, ragMessages, config, messageID, reference, stream)
}

func (s *ChatSessionService) messagesWithRetrievedKnowledge(ctx context.Context, userID string, dialog *entity.Chat, messages []map[string]interface{}, reference []interface{}) ([]map[string]interface{}, *entity.Chat, *string, error) {
	kbIDs := stringSliceFromJSON(dialog.KBIDs)
	if len(kbIDs) == 0 {
		return messages, dialog, nil, nil
	}
	if s.retrievalSvc == nil {
		return nil, nil, nil, errors.New("retrieval service is not configured")
	}

	question := latestUserQuestion(messages)
	if question == "" {
		return messages, dialog, nil, nil
	}

	kbs, err := s.kbDAO.GetByIDs(kbIDs)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to load knowledge bases: %w", err)
	}
	kbs, err = s.knowledgebasesForDialog(userID, dialog, kbIDs, kbs)
	if err != nil {
		return nil, nil, nil, err
	}
	embeddingTenantID, embeddingModelName, err := validateKnowledgebaseEmbeddingModels(kbs, dialog.TenantID, resolveEmbeddingModelName)
	if err != nil {
		return nil, nil, nil, err
	}

	embeddingModel, err := s.modelProviderSvc.GetEmbeddingModel(embeddingTenantID, embeddingModelName)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("failed to get embedding model: %w", err)
	}
	rerankModel, err := s.rerankModelForDialog(dialog)
	if err != nil {
		return nil, nil, nil, err
	}

	top := int(dialog.TopK)
	pageSize := int(dialog.TopN)
	if pageSize <= 0 {
		pageSize = 6
	}
	similarityThreshold := dialog.SimilarityThreshold
	vectorSimilarityWeight := dialog.VectorSimilarityWeight
	var rankFeature map[string]float64
	if s.metadataSvc != nil {
		rankFeature = s.metadataSvc.LabelQuestion(question, kbs)
	}
	baseDocIDs := docIDsFromMessages(messages)
	docIDs, err := s.filteredDocIDsForDialog(ctx, dialog, kbIDs, question, baseDocIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	tenantIDs := tenantIDsFromKnowledgebases(kbs, dialog.TenantID)

	retrievalResult, err := s.retrievalSvc.Retrieval(ctx, &nlp.RetrievalRequest{
		Question:               question,
		TenantIDs:              tenantIDs,
		KbIDs:                  kbIDs,
		DocIDs:                 docIDs,
		Page:                   1,
		PageSize:               pageSize,
		Top:                    &top,
		SimilarityThreshold:    &similarityThreshold,
		VectorSimilarityWeight: &vectorSimilarityWeight,
		RankFeature:            &rankFeature,
		EmbeddingModel:         embeddingModel,
		RerankModel:            rerankModel,
	})
	if err != nil {
		return nil, nil, nil, fmt.Errorf("retrieval search failed: %w", err)
	}
	if retrievalResult == nil {
		retrievalResult = &nlp.RetrievalResult{}
	}

	chunks := retrievalResult.Chunks
	if s.docEngine != nil {
		chunks = nlp.RetrievalByChildren(chunks, tenantIDs, s.docEngine, ctx)
	}
	setLatestReference(reference, chunks, retrievalResult.DocAggs)
	knowledge := buildKnowledgeBlock(chunks)
	if knowledge == "" {
		return messages, dialog, emptyResponseForDialog(dialog), nil
	}
	if ragDialog, ok := dialogWithInjectedKnowledgePrompt(dialog, knowledge); ok {
		return copyMessages(messages), ragDialog, nil, nil
	}

	return injectKnowledge(messages, knowledge), dialog, nil, nil
}

type embeddingModelNameResolver func(tenantID string, kb *entity.Knowledgebase) (string, error)

func validateKnowledgebaseEmbeddingModels(kbs []*entity.Knowledgebase, fallbackTenantID string, resolve embeddingModelNameResolver) (string, string, error) {
	if len(kbs) == 0 {
		return fallbackTenantID, "", nil
	}

	expected := ""
	expectedKBID := ""
	expectedTenantID := fallbackTenantID
	for _, kb := range kbs {
		if kb == nil {
			return "", "", errors.New("knowledge base is nil")
		}
		tenantID := kb.TenantID
		if tenantID == "" {
			tenantID = fallbackTenantID
		}
		modelName, err := resolve(tenantID, kb)
		if err != nil {
			return "", "", err
		}
		modelName = strings.TrimSpace(modelName)
		if modelName == "" {
			return "", "", fmt.Errorf("knowledge base %s has no embedding model", kb.ID)
		}
		if expected == "" {
			expected = modelName
			expectedKBID = kb.ID
			expectedTenantID = tenantID
			continue
		}
		if modelName != expected {
			return "", "", fmt.Errorf("knowledge bases must use the same embedding model: %s resolves to %q, expected %q from %s", kb.ID, modelName, expected, expectedKBID)
		}
	}
	return expectedTenantID, expected, nil
}

func (s *ChatSessionService) rerankModelForDialog(dialog *entity.Chat) (*modelModule.RerankModel, error) {
	compositeName, err := resolveRerankModelName(dialog)
	if err != nil {
		return nil, err
	}
	if compositeName == "" {
		return nil, nil
	}
	rerankModel, err := s.modelProviderSvc.GetRerankModel(dialog.TenantID, compositeName)
	if err != nil {
		return nil, fmt.Errorf("failed to get rerank model: %w", err)
	}
	return rerankModel, nil
}

func (s *ChatSessionService) filteredDocIDsForDialog(ctx context.Context, dialog *entity.Chat, kbIDs []string, question string, baseDocIDs []string) ([]string, error) {
	if dialog.MetaDataFilter == nil || len(*dialog.MetaDataFilter) == 0 {
		return baseDocIDs, nil
	}
	if s.metadataSvc == nil {
		return nil, errors.New("metadata service is not configured")
	}

	filter := make(map[string]interface{}, len(*dialog.MetaDataFilter))
	for key, value := range *dialog.MetaDataFilter {
		filter[key] = value
	}

	metaData, err := s.metadataSvc.GetFlattedMetaByKBs(kbIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get flattened metadata for chat retrieval: %w", err)
	}

	var filterChatModel *modelModule.ChatModel
	method, _ := filter["method"].(string)
	if method == "auto" || method == "semi_auto" {
		filterChatModel, err = s.modelProviderSvc.GetChatModel(dialog.TenantID, dialog.LLMID)
		if err != nil {
			common.Warn("Failed to get chat model for chat metadata filter", zap.Error(err))
		}
	}

	docIDs, empty := ApplyMetaDataFilter(ctx, filter, metaData, question, filterChatModel, baseDocIDs, kbIDs)
	if empty {
		return []string{NoMatchDocIDSentinel}, nil
	}
	return docIDs, nil
}

func resolveEmbeddingModelName(tenantID string, kb *entity.Knowledgebase) (string, error) {
	if kb.TenantEmbdID != nil && *kb.TenantEmbdID > 0 {
		_, compositeName, err := dao.LookupTenantLLMByID(dao.NewTenantLLMDAO(), *kb.TenantEmbdID)
		if err != nil {
			return "", fmt.Errorf("failed to get embedding model by tenant_embd_id: %w", err)
		}
		return compositeName, nil
	}
	if kb.EmbdID != "" {
		if strings.Contains(kb.EmbdID, "@") {
			return kb.EmbdID, nil
		}
		_, compositeName, err := dao.LookupTenantLLMByName(dao.NewTenantLLMDAO(), tenantID, kb.EmbdID, entity.ModelTypeEmbedding)
		if err != nil {
			return "", fmt.Errorf("failed to get embedding model by embd_id: %w", err)
		}
		return compositeName, nil
	}

	tenantLLM, err := dao.NewTenantLLMDAO().GetByTenantAndType(tenantID, entity.ModelTypeEmbedding)
	if err != nil {
		return "", fmt.Errorf("failed to get tenant default embedding model: %w", err)
	}
	if tenantLLM == nil || tenantLLM.LLMName == nil || *tenantLLM.LLMName == "" {
		return "", fmt.Errorf("no default embedding model found for tenant %s", tenantID)
	}
	return fmt.Sprintf("%s@%s", *tenantLLM.LLMName, tenantLLM.LLMFactory), nil
}

func resolveRerankModelName(dialog *entity.Chat) (string, error) {
	if dialog.TenantRerankID != nil && *dialog.TenantRerankID > 0 {
		_, compositeName, err := dao.LookupTenantLLMByID(dao.NewTenantLLMDAO(), *dialog.TenantRerankID)
		if err != nil {
			return "", fmt.Errorf("failed to get rerank model by tenant_rerank_id: %w", err)
		}
		return compositeName, nil
	}
	if dialog.RerankID == "" {
		return "", nil
	}
	if strings.Contains(dialog.RerankID, "@") {
		return dialog.RerankID, nil
	}
	_, compositeName, err := dao.LookupTenantLLMByName(dao.NewTenantLLMDAO(), dialog.TenantID, dialog.RerankID, entity.ModelTypeRerank)
	if err != nil {
		return "", fmt.Errorf("failed to get rerank model by rerank_id: %w", err)
	}
	return compositeName, nil
}

func stringSliceFromJSON(values entity.JSONSlice) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		str, ok := value.(string)
		if !ok || str == "" {
			continue
		}
		if _, exists := seen[str]; exists {
			continue
		}
		seen[str] = struct{}{}
		result = append(result, str)
	}
	return result
}

func tenantIDsFromKnowledgebases(kbs []*entity.Knowledgebase, fallback string) []string {
	seen := make(map[string]struct{}, len(kbs)+1)
	var tenantIDs []string
	for _, kb := range kbs {
		if kb == nil || kb.TenantID == "" {
			continue
		}
		if _, exists := seen[kb.TenantID]; exists {
			continue
		}
		seen[kb.TenantID] = struct{}{}
		tenantIDs = append(tenantIDs, kb.TenantID)
	}
	if len(tenantIDs) == 0 && fallback != "" {
		tenantIDs = append(tenantIDs, fallback)
	}
	return tenantIDs
}

func (s *ChatSessionService) knowledgebasesForDialog(userID string, dialog *entity.Chat, kbIDs []string, loaded []*entity.Knowledgebase) ([]*entity.Knowledgebase, error) {
	byID := make(map[string]*entity.Knowledgebase, len(loaded))
	for _, kb := range loaded {
		if kb != nil {
			byID[kb.ID] = kb
		}
	}

	kbs := make([]*entity.Knowledgebase, 0, len(kbIDs))
	for _, kbID := range kbIDs {
		kb := byID[kbID]
		if kb == nil {
			return nil, fmt.Errorf("knowledge base %s not found", kbID)
		}
		if userID != "" && !s.kbDAO.Accessible(kbID, userID) {
			return nil, fmt.Errorf("knowledge base %s is not authorized for user", kbID)
		}
		if userID == "" && kb.TenantID != dialog.TenantID {
			return nil, fmt.Errorf("knowledge base %s is not authorized for dialog tenant", kbID)
		}
		kbs = append(kbs, kb)
	}
	if len(kbs) == 0 {
		return nil, errors.New("no valid knowledge bases found")
	}
	return kbs, nil
}

func docIDsFromMessages(messages []map[string]interface{}) []string {
	for i := len(messages) - 1; i >= 0; i-- {
		if role, _ := messages[i]["role"].(string); role != "user" {
			continue
		}
		return stringSliceFromValue(messages[i]["doc_ids"])
	}
	return nil
}

func latestUserQuestion(messages []map[string]interface{}) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if role, _ := messages[i]["role"].(string); role != "user" {
			continue
		}
		return textFromMessageContent(messages[i]["content"])
	}
	return ""
}

func stringSliceFromValue(value interface{}) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []string:
		return uniqueNonEmptyStrings(typed)
	case []interface{}:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			if str, ok := item.(string); ok {
				values = append(values, str)
			}
		}
		return uniqueNonEmptyStrings(values)
	default:
		return nil
	}
}

func uniqueNonEmptyStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func emptyResponseForDialog(dialog *entity.Chat) *string {
	if dialog.PromptConfig == nil {
		return nil
	}
	emptyResponse, ok := dialog.PromptConfig["empty_response"].(string)
	if !ok || emptyResponse == "" {
		return nil
	}
	return &emptyResponse
}

func buildKnowledgeBlock(chunks []map[string]interface{}) string {
	var builder strings.Builder
	for i, chunk := range chunks {
		content := chunkText(chunk)
		if content == "" {
			continue
		}
		if builder.Len() > 0 {
			builder.WriteString("\n\n")
		}
		builder.WriteString(fmt.Sprintf("[%d]", i+1))
		if docName, ok := chunk["docnm_kwd"].(string); ok && docName != "" {
			builder.WriteString(" ")
			builder.WriteString(docName)
		}
		builder.WriteString("\n")
		builder.WriteString(content)
	}
	return builder.String()
}

func chunkText(chunk map[string]interface{}) string {
	for _, key := range []string{"content_with_weight", "content_ltks", "content"} {
		if value, ok := chunk[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func injectKnowledge(messages []map[string]interface{}, knowledge string) []map[string]interface{} {
	copied := copyMessages(messages)
	if len(copied) == 0 {
		return copied
	}

	knowledgePrompt := fmt.Sprintf("Use the following knowledge snippets to answer the user's question. If the snippets do not contain the answer, say that the knowledge base does not provide enough information.\n\n%s", knowledge)
	for i := len(copied) - 1; i >= 0; i-- {
		if role, _ := copied[i]["role"].(string); role != "user" {
			continue
		}
		copied[i]["content"] = injectKnowledgeIntoContent(copied[i]["content"], knowledgePrompt)
		return copied
	}

	copied = append(copied, map[string]interface{}{
		"role":    "system",
		"content": knowledgePrompt,
	})
	return copied
}

func injectKnowledgeIntoContent(content interface{}, knowledgePrompt string) interface{} {
	switch typed := content.(type) {
	case []interface{}:
		injected := make([]interface{}, 0, len(typed)+1)
		injected = append(injected, knowledgeTextBlock(knowledgePrompt))
		injected = append(injected, typed...)
		return injected
	case []map[string]interface{}:
		injected := make([]interface{}, 0, len(typed)+1)
		injected = append(injected, knowledgeTextBlock(knowledgePrompt))
		for _, block := range typed {
			injected = append(injected, block)
		}
		return injected
	default:
		contentText := ""
		if content != nil {
			contentText = fmt.Sprint(content)
		}
		return strings.TrimSpace(knowledgePrompt + "\n\nQuestion:\n" + contentText)
	}
}

func knowledgeTextBlock(knowledgePrompt string) map[string]interface{} {
	return map[string]interface{}{
		"type": "text",
		"text": knowledgePrompt + "\n\nQuestion:",
	}
}

func textFromMessageContent(content interface{}) string {
	switch typed := content.(type) {
	case string:
		return strings.TrimSpace(typed)
	case []interface{}:
		return strings.TrimSpace(strings.Join(textsFromContentBlocks(typed), "\n"))
	case []map[string]interface{}:
		blocks := make([]interface{}, 0, len(typed))
		for _, block := range typed {
			blocks = append(blocks, block)
		}
		return strings.TrimSpace(strings.Join(textsFromContentBlocks(blocks), "\n"))
	default:
		if content == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprint(content))
	}
}

func textsFromContentBlocks(blocks []interface{}) []string {
	texts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		switch typed := block.(type) {
		case string:
			if text := strings.TrimSpace(typed); text != "" {
				texts = append(texts, text)
			}
		case map[string]interface{}:
			if text, ok := typed["text"].(string); ok && strings.TrimSpace(text) != "" {
				texts = append(texts, strings.TrimSpace(text))
			}
		}
	}
	return texts
}

func dialogWithInjectedKnowledgePrompt(dialog *entity.Chat, knowledge string) (*entity.Chat, bool) {
	if dialog.PromptConfig == nil {
		return dialog, false
	}
	systemPrompt, ok := dialog.PromptConfig["system"].(string)
	if !ok || !strings.Contains(systemPrompt, "{knowledge}") {
		return dialog, false
	}

	copied := cloneJSONMap(dialog.PromptConfig)
	copied["system"] = strings.ReplaceAll(systemPrompt, "{knowledge}", knowledge)
	dialogCopy := *dialog
	dialogCopy.PromptConfig = copied
	return &dialogCopy, true
}

func cloneJSONMap(values entity.JSONMap) entity.JSONMap {
	copied := make(entity.JSONMap, len(values))
	for key, value := range values {
		copied[key] = value
	}
	return copied
}

func copyMessages(messages []map[string]interface{}) []map[string]interface{} {
	copied := make([]map[string]interface{}, len(messages))
	for i, msg := range messages {
		copied[i] = make(map[string]interface{}, len(msg))
		for key, value := range msg {
			copied[i][key] = value
		}
	}
	return copied
}

func setLatestReference(reference []interface{}, chunks []map[string]interface{}, docAggs []map[string]interface{}) {
	ref := map[string]interface{}{
		"chunks":   chunksForReference(chunks),
		"doc_aggs": mapsForReference(docAggs),
	}
	if len(reference) == 0 {
		return
	}
	reference[len(reference)-1] = ref
}

func chunksForReference(chunks []map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(chunks))
	for _, chunk := range chunks {
		copied := make(map[string]interface{}, len(chunk))
		for key, value := range chunk {
			if key == "vector" {
				continue
			}
			copied[key] = value
		}
		result = append(result, copied)
	}
	return result
}

func mapsForReference(values []map[string]interface{}) []interface{} {
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

// asyncChatSolo performs simple chat without RAG (non-streaming)
func (s *ChatSessionService) asyncChatSolo(dialog *entity.Chat, session *entity.ChatSession, messages []map[string]interface{}, config map[string]interface{}, messageID string, reference []interface{}, stream bool) (map[string]interface{}, error) {
	common.Info("asyncChatSolo started",
		zap.String("tenant_id", dialog.TenantID),
		zap.String("llm_id", dialog.LLMID),
		zap.String("dialog_id", dialog.ID),
		zap.Int("message_count", len(messages)))

	// Get system prompt
	systemPrompt := s.buildSystemPrompt(dialog)

	// Process messages - handle attachments and image files
	processedMessages := s.processMessages(messages, dialog)

	var (
		driver    modelModule.ModelDriver
		modelName string
		apiConfig *modelModule.APIConfig
		err       error
	)
	if dialog.LLMID != "" {
		driver, modelName, apiConfig, _, err = s.modelProviderSvc.GetModelConfigFromProviderInstance(
			dialog.TenantID, entity.ModelTypeChat, dialog.LLMID,
		)
	} else {
		driver, modelName, apiConfig, _, err = s.modelProviderSvc.GetTenantDefaultModelByType(
			dialog.TenantID, entity.ModelTypeChat,
		)
	}
	if err != nil {
		common.Error("asyncChatSolo failed to get chat model", err)
		return nil, err
	}
	chatModel := modelModule.NewChatModel(driver, &modelName, apiConfig)

	// Convert messages to Message format
	var msgs []modelModule.Message
	if systemPrompt != "" {
		msgs = append(msgs, modelModule.Message{Role: "system", Content: systemPrompt})
	}
	for _, msg := range processedMessages {
		role, _ := msg["role"].(string)
		if role == "" || role == "system" {
			continue
		}

		if msg["content"] != nil {
			msgs = append(msgs, modelModule.Message{Role: role, Content: msg["content"]})
		}
	}

	// Get ChatConfig directly from dialog and config
	chatConfig := s.buildChatConfig(dialog, config)

	// Perform chat
	response, err := chatModel.ModelDriver.ChatWithMessages(*chatModel.ModelName, msgs, chatModel.APIConfig, chatConfig)
	if err != nil {
		common.Error("asyncChatSolo chat failed", err)
		return nil, err
	}

	common.Info("asyncChatSolo completed",
		zap.String("tenant_id", dialog.TenantID),
		zap.String("llm_id", dialog.LLMID),
		zap.Int("response_length", len(*response.Answer)))

	// Structure the answer
	ans := map[string]interface{}{
		"answer":    *response.Answer,
		"reference": reference[len(reference)-1],
		"final":     true,
	}

	return s.structureAnswerWithConv(session, ans, messageID, session.ID, reference), nil
}

// asyncChatSoloStream performs simple streaming chat without RAG
func (s *ChatSessionService) asyncChatSoloStream(dialog *entity.Chat, session *entity.ChatSession, messages []map[string]interface{}, config map[string]interface{}, messageID string, reference []interface{}, resultChan chan<- map[string]interface{}) {
	common.Info("asyncChatSoloStream started",
		zap.String("tenant_id", dialog.TenantID),
		zap.String("llm_id", dialog.LLMID),
		zap.String("dialog_id", dialog.ID),
		zap.Int("message_count", len(messages)))

	// Get system prompt
	systemPrompt := s.buildSystemPrompt(dialog)

	// Process messages
	processedMessages := s.processMessages(messages, dialog)

	var (
		driver    modelModule.ModelDriver
		modelName string
		apiConfig *modelModule.APIConfig
		err       error
	)
	if dialog.LLMID != "" {
		driver, modelName, apiConfig, _, err = s.modelProviderSvc.GetModelConfigFromProviderInstance(
			dialog.TenantID, entity.ModelTypeChat, dialog.LLMID,
		)
	} else {
		driver, modelName, apiConfig, _, err = s.modelProviderSvc.GetTenantDefaultModelByType(
			dialog.TenantID, entity.ModelTypeChat,
		)
	}
	if err != nil {
		common.Error("asyncChatSoloStream failed to get chat model", err)
		resultChan <- s.structureAnswer(session, "**ERROR**: "+err.Error(), messageID, session.ID, reference)
		return
	}
	chatModel := modelModule.NewChatModel(driver, &modelName, apiConfig)

	// Convert messages to []modelModule.Message for ChatStreamlyWithSender
	var chatMessages []modelModule.Message
	if systemPrompt != "" {
		chatMessages = append(chatMessages, modelModule.Message{
			Role:    "system",
			Content: systemPrompt,
		})
	}
	for _, msg := range processedMessages {
		role, _ := msg["role"].(string)
		content := msg["content"]
		if role != "" && content != nil && role != "system" {
			chatMessages = append(chatMessages, modelModule.Message{
				Role:    role,
				Content: content,
			})
		}
	}

	// Get ChatConfig directly from dialog and config
	chatConfig := s.buildChatConfig(dialog, config)

	// Perform streaming chat using ChatStreamlyWithSender
	fullAnswer := ""
	err = chatModel.ModelDriver.ChatStreamlyWithSender(*chatModel.ModelName, chatMessages, chatModel.APIConfig, chatConfig, func(answer *string, reason *string) error {
		if reason != nil && *reason != "" {
			fullAnswer += *reason
			ans := s.structureAnswer(session, fullAnswer, messageID, session.ID, reference)
			resultChan <- ans
		}
		if answer != nil && *answer != "" {
			fullAnswer += *answer
			fullAnswer = s.removeReasoningContent(fullAnswer)
			ans := s.structureAnswer(session, fullAnswer, messageID, session.ID, reference)
			resultChan <- ans
		}
		return nil
	})
	if err != nil {
		resultChan <- s.structureAnswer(session, "**ERROR**: "+err.Error(), messageID, session.ID, reference)
		return
	}

	common.Info("asyncChatSoloStream completed",
		zap.String("tenant_id", dialog.TenantID),
		zap.String("llm_id", dialog.LLMID),
		zap.Int("response_length", len(fullAnswer)))
}

// buildSystemPrompt builds the system prompt from dialog configuration
func (s *ChatSessionService) buildSystemPrompt(dialog *entity.Chat) string {
	if dialog.PromptConfig == nil {
		return ""
	}

	system, _ := dialog.PromptConfig["system"].(string)
	return system
}

// processMessages processes messages and handles attachments
func (s *ChatSessionService) processMessages(messages []map[string]interface{}, dialog *entity.Chat) []map[string]interface{} {
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

// removeReasoningContent removes reasoning/thinking content from answer
func (s *ChatSessionService) removeReasoningContent(answer string) string {
	// Remove </think> tags
	if strings.HasSuffix(answer, "</think>") {
		answer = answer[:len(answer)-len("</think>")]
	}
	return answer
}

// structureAnswerWithConv structures the answer with conversation update (like Python's structure_answer)
func (s *ChatSessionService) structureAnswerWithConv(session *entity.ChatSession, ans map[string]interface{}, messageID, conversationID string, reference []interface{}) map[string]interface{} {
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

// buildChatConfig builds ChatConfig directly from dialog.LLMSetting and config
func (s *ChatSessionService) buildChatConfig(dialog *entity.Chat, config map[string]interface{}) *modelModule.ChatConfig {
	cfg := &modelModule.ChatConfig{}

	// Start with dialog's LLM setting
	if dialog.LLMSetting != nil {
		if v, ok := dialog.LLMSetting["stream"].(bool); ok {
			cfg.Stream = &v
		}
		if v, ok := dialog.LLMSetting["thinking"].(bool); ok {
			cfg.Thinking = &v
		}
		if v, ok := dialog.LLMSetting["max_tokens"].(float64); ok {
			intVal := int(v)
			cfg.MaxTokens = &intVal
		}
		if v, ok := dialog.LLMSetting["temperature"].(float64); ok {
			cfg.Temperature = &v
		}
		if v, ok := dialog.LLMSetting["top_p"].(float64); ok {
			cfg.TopP = &v
		}
		if v, ok := dialog.LLMSetting["do_sample"].(bool); ok {
			cfg.DoSample = &v
		}
		if v, ok := dialog.LLMSetting["stop"].([]interface{}); ok {
			stopStrs := make([]string, 0, len(v))
			for _, s := range v {
				if str, ok := s.(string); ok {
					stopStrs = append(stopStrs, str)
				}
			}
			cfg.Stop = &stopStrs
		}
		if v, ok := dialog.LLMSetting["model_class"].(string); ok {
			cfg.ModelClass = &v
		}
		if v, ok := dialog.LLMSetting["effort"].(string); ok {
			cfg.Effort = &v
		}
		if v, ok := dialog.LLMSetting["verbosity"].(string); ok {
			cfg.Verbosity = &v
		}
	}

	// Override with request config
	if config != nil {
		if v, ok := config["stream"].(bool); ok {
			cfg.Stream = &v
		}
		if v, ok := config["thinking"].(bool); ok {
			cfg.Thinking = &v
		}
		if v, ok := config["max_tokens"].(float64); ok {
			intVal := int(v)
			cfg.MaxTokens = &intVal
		}
		if v, ok := config["temperature"].(float64); ok {
			cfg.Temperature = &v
		}
		if v, ok := config["top_p"].(float64); ok {
			cfg.TopP = &v
		}
		if v, ok := config["do_sample"].(bool); ok {
			cfg.DoSample = &v
		}
		if v, ok := config["stop"].([]interface{}); ok {
			stopStrs := make([]string, 0, len(v))
			for _, s := range v {
				if str, ok := s.(string); ok {
					stopStrs = append(stopStrs, str)
				}
			}
			cfg.Stop = &stopStrs
		}
		if v, ok := config["model_class"].(string); ok {
			cfg.ModelClass = &v
		}
		if v, ok := config["effort"].(string); ok {
			cfg.Effort = &v
		}
		if v, ok := config["verbosity"].(string); ok {
			cfg.Verbosity = &v
		}
	}

	return cfg
}
