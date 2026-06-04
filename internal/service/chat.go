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
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"unicode/utf8"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// ChatService chat service
type ChatService struct {
	chatDAO        *dao.ChatDAO
	chatSessionDAO *dao.ChatSessionDAO
	kbDAO          *dao.KnowledgebaseDAO
	userTenantDAO  *dao.UserTenantDAO
	tenantDAO      *dao.TenantDAO
}

// NewChatService create chat service
func NewChatService() *ChatService {
	return &ChatService{
		chatDAO:        dao.NewChatDAO(),
		chatSessionDAO: dao.NewChatSessionDAO(),
		kbDAO:          dao.NewKnowledgebaseDAO(),
		userTenantDAO:  dao.NewUserTenantDAO(),
		tenantDAO:      dao.NewTenantDAO(),
	}
}

// ChatWithKBNames chat with knowledge base names
type ChatWithKBNames struct {
	*entity.Chat
	KBNames    []string `json:"kb_names"`
	DatasetIDs []string `json:"dataset_ids"`
}

// ListChatsResponse list chats response
type ListChatsResponse struct {
	Chats []*ChatWithKBNames `json:"chats"`
}

// ListChats list chats for a user
func (s *ChatService) ListChats(userID, status, keywords string, page, pageSize int, orderby string, desc bool) (*ListChatsResponse, error) {
	// Get tenant IDs by user ID
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, err
	}

	// For now, use the first tenant ID (primary tenant)
	// This matches the Python implementation behavior
	var tenantID string
	if len(tenantIDs) > 0 {
		tenantID = tenantIDs[0]
	} else {
		tenantID = userID
	}

	// Query chats by tenant ID
	chats, err := s.chatDAO.ListByTenantID(tenantID, status)
	if err != nil {
		return nil, err
	}

	total := int64(len(chats))

	if page > 0 && pageSize > 0 {
		start := (page - 1) * pageSize
		end := start + pageSize
		if start < int(total) {
			if end > int(total) {
				end = int(total)
			}
			chats = chats[start:end]
		} else {
			chats = []*entity.Chat{}
		}
	}

	// Enrich with knowledge base names
	chatsWithKBNames := make([]*ChatWithKBNames, 0, len(chats))
	for _, chat := range chats {
		kbNames, datasetIDs := s.getDatasetNamesAndIDs(chat.KBIDs)
		chatsWithKBNames = append(chatsWithKBNames, &ChatWithKBNames{
			Chat:       chat,
			KBNames:    kbNames,
			DatasetIDs: datasetIDs,
		})
	}

	return &ListChatsResponse{
		Chats: chatsWithKBNames,
	}, nil
}

// ListChatsNextRequest list chats next request
type ListChatsNextRequest struct {
	OwnerIDs []string `json:"owner_ids,omitempty"`
}

// ListChatsNextResponse list chats next response
type ListChatsNextResponse struct {
	Chats []*ChatWithKBNames `json:"dialogs"`
	Total int64              `json:"total"`
}

// ListChatsNext list chats with advanced filtering (equivalent to list_dialogs_next)
func (s *ChatService) ListChatsNext(userID string, keywords string, page, pageSize int, orderby string, desc bool, ownerIDs []string) (*ListChatsNextResponse, error) {
	var chats []*entity.Chat
	var total int64
	var err error

	if len(ownerIDs) == 0 {
		// Get tenant IDs by user ID (joined tenants)
		tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
		if err != nil {
			return nil, err
		}

		// Use database pagination
		chats, total, err = s.chatDAO.ListByTenantIDs(tenantIDs, userID, page, pageSize, orderby, desc, keywords)
		if err != nil {
			return nil, err
		}
	} else {
		// Filter by owner IDs, manual pagination
		chats, total, err = s.chatDAO.ListByOwnerIDs(ownerIDs, userID, orderby, desc, keywords)
		if err != nil {
			return nil, err
		}

		// Manual pagination
		if page > 0 && pageSize > 0 {
			start := (page - 1) * pageSize
			end := start + pageSize
			if start < int(total) {
				if end > int(total) {
					end = int(total)
				}
				chats = chats[start:end]
			} else {
				chats = []*entity.Chat{}
			}
		}
	}

	// Enrich with knowledge base names
	chatsWithKBNames := make([]*ChatWithKBNames, 0, len(chats))
	for _, chat := range chats {
		kbNames, datasetIDs := s.getDatasetNamesAndIDs(chat.KBIDs)
		chatsWithKBNames = append(chatsWithKBNames, &ChatWithKBNames{
			Chat:       chat,
			KBNames:    kbNames,
			DatasetIDs: datasetIDs,
		})
	}

	return &ListChatsNextResponse{
		Chats: chatsWithKBNames,
		Total: total,
	}, nil
}

// getDatasetNamesAndIDs gets knowledge base names by IDs
func (s *ChatService) getDatasetNamesAndIDs(kbIDs entity.JSONSlice) ([]string, []string) {
	var names = make([]string, 0, 0)
	var ids = make([]string, 0, 0)
	for _, kbID := range kbIDs {
		kbIDStr, ok := kbID.(string)
		if !ok {
			continue
		}
		kb, err := s.kbDAO.GetByID(kbIDStr)
		if err != nil || kb == nil {
			continue
		}
		// Only include valid KBs
		if kb.Status != nil && *kb.Status == "1" {
			names = append(names, kb.Name)
			ids = append(ids, kbIDStr)
		}
	}
	return names, ids
}

// ParameterConfig parameter configuration in prompt_config
type ParameterConfig struct {
	Key      string `json:"key"`
	Optional bool   `json:"optional"`
}

// PromptConfig prompt configuration
type PromptConfig struct {
	System          string            `json:"system"`
	Prologue        string            `json:"prologue"`
	Parameters      []ParameterConfig `json:"parameters"`
	EmptyResponse   string            `json:"empty_response"`
	TavilyAPIKey    string            `json:"tavily_api_key,omitempty"`
	Keyword         bool              `json:"keyword,omitempty"`
	Quote           bool              `json:"quote,omitempty"`
	Reasoning       bool              `json:"reasoning,omitempty"`
	RefineMultiturn bool              `json:"refine_multiturn,omitempty"`
	TocEnhance      bool              `json:"toc_enhance,omitempty"`
	TTS             bool              `json:"tts,omitempty"`
	UseKG           bool              `json:"use_kg,omitempty"`
}

// SetDialogRequest set chat request
type SetDialogRequest struct {
	DialogID               string                 `json:"dialog_id,omitempty"`
	Name                   string                 `json:"name,omitempty"`
	Description            string                 `json:"description,omitempty"`
	Icon                   string                 `json:"icon,omitempty"`
	TopN                   int64                  `json:"top_n,omitempty"`
	TopK                   int64                  `json:"top_k,omitempty"`
	RerankID               string                 `json:"rerank_id,omitempty"`
	SimilarityThreshold    float64                `json:"similarity_threshold,omitempty"`
	VectorSimilarityWeight float64                `json:"vector_similarity_weight,omitempty"`
	LLMSetting             map[string]interface{} `json:"llm_setting,omitempty"`
	MetaDataFilter         map[string]interface{} `json:"meta_data_filter,omitempty"`
	PromptConfig           *PromptConfig          `json:"prompt_config" binding:"required"`
	KBIDs                  []string               `json:"kb_ids,omitempty"`
	LLMID                  string                 `json:"llm_id,omitempty"`
}

// SetDialogResponse set chat response
type SetDialogResponse struct {
	*entity.Chat
	KBNames []string `json:"kb_names"`
}

// SetDialog create or update a chat
func (s *ChatService) SetDialog(userID string, req *SetDialogRequest) (*SetDialogResponse, error) {
	// Determine if this is a create or update operation
	isCreate := req.DialogID == ""

	// Validate and process name
	name := req.Name
	if name == "" {
		name = "New Chat"
	}

	// Validate name type and content
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("Chat name can't be empty")
	}

	// Check name length (UTF-8 byte length)
	if len(name) > 255 {
		return nil, fmt.Errorf("Chat name length is %d which is larger than 255", len(name))
	}

	name = strings.TrimSpace(name)

	// Get tenant ID (use userID as default tenant)
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, err
	}

	var tenantID string
	if len(tenantIDs) > 0 {
		tenantID = tenantIDs[0]
	} else {
		tenantID = userID
	}

	// For create: check for duplicate names and generate unique name
	if isCreate {
		existingNames, err := s.chatDAO.GetExistingNames(tenantID, "1")
		if err != nil {
			return nil, err
		}

		// Check if name exists (case-insensitive)
		nameLower := strings.ToLower(name)
		for _, existing := range existingNames {
			if strings.ToLower(existing) == nameLower {
				// Generate unique name
				name = s.generateUniqueName(name, existingNames)
				break
			}
		}
	}

	// Set default values
	description := req.Description
	if description == "" {
		description = "A helpful chat"
	}

	topN := req.TopN
	if topN == 0 {
		topN = 6
	}

	topK := req.TopK
	if topK == 0 {
		topK = 1024
	}

	rerankID := req.RerankID

	similarityThreshold := req.SimilarityThreshold
	if similarityThreshold == 0 {
		similarityThreshold = 0.1
	}

	vectorSimilarityWeight := req.VectorSimilarityWeight
	if vectorSimilarityWeight == 0 {
		vectorSimilarityWeight = 0.3
	}

	llmSetting := req.LLMSetting
	if llmSetting == nil {
		llmSetting = make(map[string]interface{})
	}

	metaDataFilter := req.MetaDataFilter
	if metaDataFilter == nil {
		metaDataFilter = make(map[string]interface{})
	}

	promptConfig := req.PromptConfig

	// Process kb_ids
	kbIDs := req.KBIDs
	if kbIDs == nil {
		kbIDs = []string{}
	}

	// Set default parameters for datasets with knowledge retrieval
	// Check if parameters is missing or empty and kb_ids is provided
	if len(kbIDs) > 0 && (promptConfig.Parameters == nil || len(promptConfig.Parameters) == 0) {
		// Check if system prompt uses {knowledge} placeholder
		if strings.Contains(promptConfig.System, "{knowledge}") {
			// Set default parameters for any dataset with knowledge placeholder
			promptConfig.Parameters = []ParameterConfig{
				{Key: "knowledge", Optional: false},
			}
		}
	}

	// For update: validate that {knowledge} is not used when no KBs or Tavily
	if !isCreate {
		if len(kbIDs) == 0 && promptConfig.TavilyAPIKey == "" && strings.Contains(promptConfig.System, "{knowledge}") {
			return nil, errors.New("Please remove `{knowledge}` in system prompt since no dataset / Tavily used here")
		}
	}

	// Validate parameters
	for _, p := range promptConfig.Parameters {
		if p.Optional {
			continue
		}
		placeholder := fmt.Sprintf("{%s}", p.Key)
		if !strings.Contains(promptConfig.System, placeholder) {
			return nil, fmt.Errorf("Parameter '%s' is not used", p.Key)
		}
	}

	// Check knowledge bases and their embedding models
	if len(kbIDs) > 0 {
		kbs, err := s.kbDAO.GetByIDs(kbIDs)
		if err != nil {
			return nil, err
		}

		// Check if all KBs use the same embedding model
		var embdID string
		for i, kb := range kbs {
			if i == 0 {
				embdID = kb.EmbdID
			} else {
				// Extract base model name (remove vendor suffix)
				embdBase := s.splitModelNameAndFactory(embdID)
				kbEmbdBase := s.splitModelNameAndFactory(kb.EmbdID)
				if embdBase != kbEmbdBase {
					return nil, fmt.Errorf("Datasets use different embedding models: %v", getEmbdIDs(kbs))
				}
			}
		}
	}

	// Get LLM ID (use tenant's default if not provided)
	llmID := req.LLMID
	if llmID == "" {
		tenant, err := s.tenantDAO.GetByID(tenantID)
		if err != nil {
			return nil, errors.New("Tenant not found")
		}
		llmID = tenant.LLMID
	}

	// Convert prompt config to JSONMap with all fields
	promptConfigMap := entity.JSONMap{
		"system":           promptConfig.System,
		"prologue":         promptConfig.Prologue,
		"empty_response":   promptConfig.EmptyResponse,
		"keyword":          promptConfig.Keyword,
		"quote":            promptConfig.Quote,
		"reasoning":        promptConfig.Reasoning,
		"refine_multiturn": promptConfig.RefineMultiturn,
		"toc_enhance":      promptConfig.TocEnhance,
		"tts":              promptConfig.TTS,
		"use_kg":           promptConfig.UseKG,
	}
	if promptConfig.TavilyAPIKey != "" {
		promptConfigMap["tavily_api_key"] = promptConfig.TavilyAPIKey
	}
	if len(promptConfig.Parameters) > 0 {
		params := make([]map[string]interface{}, len(promptConfig.Parameters))
		for i, p := range promptConfig.Parameters {
			params[i] = map[string]interface{}{
				"key":      p.Key,
				"optional": p.Optional,
			}
		}
		promptConfigMap["parameters"] = params
	}

	// Convert kbIDs to JSONSlice
	kbIDsJSON := make(entity.JSONSlice, len(kbIDs))
	for i, id := range kbIDs {
		kbIDsJSON[i] = id
	}

	if isCreate {
		// Generate UUID for new chat
		newID := common.GenerateUUID()

		// Set default language
		language := "English"

		// Create new chat
		chat := &entity.Chat{
			ID:                     newID,
			TenantID:               tenantID,
			Name:                   &name,
			Description:            &description,
			Icon:                   &req.Icon,
			Language:               &language,
			LLMID:                  llmID,
			LLMSetting:             llmSetting,
			PromptConfig:           promptConfigMap,
			MetaDataFilter:         (*entity.JSONMap)(&metaDataFilter),
			TopN:                   topN,
			TopK:                   topK,
			RerankID:               rerankID,
			SimilarityThreshold:    similarityThreshold,
			VectorSimilarityWeight: vectorSimilarityWeight,
			KBIDs:                  kbIDsJSON,
			Status:                 strPtr("1"),
		}

		if err := s.chatDAO.Create(chat); err != nil {
			return nil, errors.New("Fail to new a chat")
		}

		// Get KB names
		kbNames, _ := s.getDatasetNamesAndIDs(chat.KBIDs)

		return &SetDialogResponse{
			Chat:    chat,
			KBNames: kbNames,
		}, nil
	}

	updateData := map[string]interface{}{
		"name":                     name,
		"description":              description,
		"icon":                     req.Icon,
		"llm_id":                   llmID,
		"llm_setting":              llmSetting,
		"prompt_config":            promptConfigMap,
		"meta_data_filter":         metaDataFilter,
		"top_n":                    topN,
		"top_k":                    topK,
		"rerank_id":                rerankID,
		"similarity_threshold":     similarityThreshold,
		"vector_similarity_weight": vectorSimilarityWeight,
		"kb_ids":                   kbIDsJSON,
	}

	if err := s.chatDAO.UpdateByID(req.DialogID, updateData); err != nil {
		return nil, errors.New("Dialog not found")
	}

	// Get updated chat
	chat, err := s.chatDAO.GetByID(req.DialogID)
	if err != nil {
		return nil, errors.New("Fail to update a chat")
	}

	// Get KB names
	kbNames, _ := s.getDatasetNamesAndIDs(chat.KBIDs)

	return &SetDialogResponse{
		Chat:    chat,
		KBNames: kbNames,
	}, nil
}

// generateUniqueName generates a unique name by appending a number
func (s *ChatService) generateUniqueName(name string, existingNames []string) string {
	baseName := name
	counter := 1

	// Check if name already has a suffix like "(1)"
	if idx := strings.LastIndex(name, "("); idx > 0 {
		if idx2 := strings.LastIndex(name, ")"); idx2 > idx {
			if num, err := fmt.Sscanf(name[idx+1:idx2], "%d", &counter); err == nil && num == 1 {
				baseName = strings.TrimSpace(name[:idx])
				counter++
			}
		}
	}

	existingMap := make(map[string]bool)
	for _, n := range existingNames {
		existingMap[strings.ToLower(n)] = true
	}

	newName := name
	for {
		if !existingMap[strings.ToLower(newName)] {
			return newName
		}
		newName = fmt.Sprintf("%s(%d)", baseName, counter)
		counter++
	}
}

// splitModelNameAndFactory extracts the base model name (removes vendor suffix)
func (s *ChatService) splitModelNameAndFactory(embdID string) string {
	// Remove vendor suffix (e.g., "model@openai" -> "model")
	if idx := strings.LastIndex(embdID, "@"); idx > 0 {
		return embdID[:idx]
	}
	return embdID
}

// getEmbdIDs extracts embedding IDs from knowledge bases
func getEmbdIDs(kbs []*entity.Knowledgebase) []string {
	ids := make([]string, len(kbs))
	for i, kb := range kbs {
		ids[i] = kb.EmbdID
	}
	return ids
}

// RemoveChats removes dialogs by setting their status to invalid (soft delete)
// Only the owner of the chat can perform this operation
func (s *ChatService) RemoveChats(userID string, chatIDs []string) error {
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
	// Also add userID itself as a tenant (for cases where tenant_id = user_id)
	tenantIDSet[userID] = true

	// Check each chat and build update list
	var updates []map[string]interface{}
	for _, chatID := range chatIDs {
		// Get the chat to check ownership
		chat, err := s.chatDAO.GetByID(chatID)
		if err != nil {
			return fmt.Errorf("chat not found: %s", chatID)
		}

		// Check if user is the owner (chat's tenant_id must be in user's tenants)
		if !tenantIDSet[chat.TenantID] {
			return errors.New("only owner of chat authorized for this operation")
		}

		// Add to update list (soft delete by setting status to "0")
		updates = append(updates, map[string]interface{}{
			"id":     chatID,
			"status": "0",
		})
	}

	// Batch update all dialogs
	if err := s.chatDAO.UpdateManyByID(updates); err != nil {
		return err
	}

	return nil
}

// strPtr returns a pointer to a string
func strPtr(s string) *string {
	return &s
}

// Helper to count UTF-8 characters (not bytes)
func (s *ChatService) countRunes(str string) int {
	return utf8.RuneCountInString(str)
}

// GetChatResponse get chat response with kb_names
// Reference: Python _build_chat_response
type GetChatResponse struct {
	*entity.Chat
	DatasetIDs []string `json:"dataset_ids"`
	KBNames    []string `json:"kb_names"`
}

// GetChat gets chat detail by ID with permission check
func (s *ChatService) GetChat(userID string, chatID string) (*GetChatResponse, error) {
	// Step 1: Get user tenants (same as Python UserTenantService.query(user_id=current_user.id))
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tenants: %w", err)
	}

	// Step 2: Check if user has permission to access this chat
	// Python: for tenant in tenants: if DialogService.query(tenant_id=tenant.tenant_id, id=chat_id, status=StatusEnum.VALID.value): break
	hasPermission := false
	for _, tenant := range tenants {
		chats, err := s.chatDAO.QueryByTenantIDAndID(tenant.TenantID, chatID, "1")
		if err != nil {
			continue // Try next tenant
		}
		if len(chats) > 0 {
			hasPermission = true
			break
		}
	}

	if !hasPermission {
		return nil, fmt.Errorf("no authorization")
	}

	// Step 3: Get chat detail (same as Python DialogService.get_by_id(chat_id))
	chat, err := s.chatDAO.GetByID(chatID)
	if err != nil {
		return nil, fmt.Errorf("chat not found")
	}

	// Step 4: Build response with kb_names (same as Python _build_chat_response)
	// Resolve kb_ids to kb_names
	kbNames, datasetIDs := s.getDatasetNamesAndIDs(chat.KBIDs)

	// Build dataset_ids from kb_ids (same as Python _resolve_kb_names returns ids)
	for _, kbID := range chat.KBIDs {
		datasetID, ok := kbID.(string)
		if !ok {
			continue
		}
		datasetIDs = append(datasetIDs, datasetID)
	}

	return &GetChatResponse{
		Chat:       chat,
		DatasetIDs: datasetIDs,
		KBNames:    kbNames,
	}, nil
}

// ── helpers ────────────────────────────────────────────────────────────────────

// buildChatResponseMap produces the dict that matches Python _build_chat_response:
// all chat columns, kb_ids replaced by dataset_ids, plus kb_names.
func (s *ChatService) buildChatResponseMap(chat *entity.Chat) map[string]interface{} {
	kbNames, datasetIDs := s.getDatasetNamesAndIDs(chat.KBIDs)
	return map[string]interface{}{
		"id":                       chat.ID,
		"tenant_id":                chat.TenantID,
		"name":                     chat.Name,
		"description":              chat.Description,
		"icon":                     chat.Icon,
		"language":                 chat.Language,
		"llm_id":                   chat.LLMID,
		"llm_setting":              chat.LLMSetting,
		"prompt_type":              chat.PromptType,
		"prompt_config":            chat.PromptConfig,
		"meta_data_filter":         chat.MetaDataFilter,
		"similarity_threshold":     chat.SimilarityThreshold,
		"vector_similarity_weight": chat.VectorSimilarityWeight,
		"top_n":                    chat.TopN,
		"top_k":                    chat.TopK,
		"do_refer":                 chat.DoRefer,
		"rerank_id":                chat.RerankID,
		"dataset_ids":              datasetIDs,
		"kb_names":                 kbNames,
		"status":                   chat.Status,
		"create_time":              chat.CreateTime,
		"create_date":              chat.CreateDate,
		"update_time":              chat.UpdateTime,
		"update_date":              chat.UpdateDate,
		"tenant_llm_id":            chat.TenantLLMID,
		"tenant_rerank_id":         chat.TenantRerankID,
	}
}

// validateName mirrors Python _validate_name.
// required=true is used for POST (name must be provided and non-empty);
// required=false is used for PATCH (name may be absent, but not empty string).
func validateName(name *string, required bool) (string, error) {
	if name == nil {
		if required {
			return "", fmt.Errorf("`name` is required.")
		}
		return "", nil
	}
	trimmed := strings.TrimSpace(*name)
	if trimmed == "" {
		if required {
			return "", fmt.Errorf("`name` is required.")
		}
		return "", fmt.Errorf("`name` cannot be empty.")
	}
	if len([]byte(trimmed)) > 255 {
		return "", fmt.Errorf("Chat name length is %d which is larger than 255.", len([]byte(trimmed)))
	}
	return trimmed, nil
}

// validateDatasetIDs validates that each dataset ID exists, is accessible to
// the user, has parsed files, and all use the same embedding model.
// Returns the resolved (valid) IDs or an error.
func (s *ChatService) validateDatasetIDs(datasetIDs []string, userID string) ([]string, error) {
	if len(datasetIDs) == 0 {
		return []string{}, nil
	}

	userTenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve user tenants: %w", err)
	}
	authorised := make(map[string]struct{}, len(userTenantIDs)+1)
	for _, tid := range userTenantIDs {
		authorised[tid] = struct{}{}
	}
	authorised[userID] = struct{}{}

	var validIDs []string
	var embdID string

	for _, id := range datasetIDs {
		if id == "" {
			continue
		}
		kb, err := s.kbDAO.GetByID(id)
		if err != nil || kb == nil {
			return nil, fmt.Errorf("You don't own the dataset %s", id)
		}
		if _, ok := authorised[kb.TenantID]; !ok {
			return nil, fmt.Errorf("You don't own the dataset %s", id)
		}
		if kb.ChunkNum == 0 {
			return nil, fmt.Errorf("The dataset %s doesn't own parsed file", id)
		}
		// Check embedding model consistency.
		base := embdModelBase(kb.EmbdID)
		if embdID == "" {
			embdID = base
		} else if embdID != base {
			return nil, fmt.Errorf("Datasets use different embedding models")
		}
		validIDs = append(validIDs, id)
	}
	return validIDs, nil
}

// embdModelBase strips the @vendor suffix so model names can be compared.
func embdModelBase(embdID string) string {
	if idx := strings.LastIndex(embdID, "@"); idx > 0 {
		return embdID[:idx]
	}
	return embdID
}

// ensureOwnedChat checks that userID is the owner of a valid chat.
// Mirrors Python _ensure_owned_chat.
// ErrChatNoAuth mirrors Python _ensure_owned_chat, which queries by
// (tenant_id, id, status=VALID) and collapses every failure mode — chat missing,
// not active, or owned by another tenant — into a single "No authorization."
// (code 109). Handlers detect it with errors.Is to return the right code instead
// of leaking a 500 or a misleading "Chat not found!" (code 102).
var ErrChatNoAuth = errors.New("No authorization.")

func (s *ChatService) ensureOwnedChat(userID, chatID string) (*entity.Chat, error) {
	chat, err := s.chatDAO.GetByID(chatID)
	if err != nil {
		return nil, ErrChatNoAuth
	}
	if chat.Status == nil || *chat.Status != "1" {
		return nil, ErrChatNoAuth
	}
	if chat.TenantID != userID {
		return nil, ErrChatNoAuth
	}
	return chat, nil
}

// EnsureOwnedChat verifies the user owns the active chat, returning ErrChatNoAuth
// otherwise. Exposed so handlers can authorize before parsing a request body
// (parity with Python, which calls _ensure_owned_chat before reading the payload).
func (s *ChatService) EnsureOwnedChat(userID, chatID string) error {
	_, err := s.ensureOwnedChat(userID, chatID)
	return err
}

// ── CreateChat ─────────────────────────────────────────────────────────────────

// CreateChatRequest mirrors Python POST /chats body.
type CreateChatRequest struct {
	Name                   *string                `json:"name"`
	Description            string                 `json:"description"`
	Icon                   string                 `json:"icon"`
	DatasetIDs             []string               `json:"dataset_ids"`
	LLMID                  string                 `json:"llm_id"`
	LLMSetting             map[string]interface{} `json:"llm_setting"`
	RerankID               string                 `json:"rerank_id"`
	PromptConfig           map[string]interface{} `json:"prompt_config"`
	SimilarityThreshold    *float64               `json:"similarity_threshold"`
	VectorSimilarityWeight *float64               `json:"vector_similarity_weight"`
	TopN                   *int64                 `json:"top_n"`
	TopK                   *int64                 `json:"top_k"`
	TenantID               *string                `json:"tenant_id"`
}

// CreateChat creates a new chat dialog, mirroring Python POST /api/v1/chats.
func (s *ChatService) CreateChat(userID string, req *CreateChatRequest) (map[string]interface{}, error) {
	if req.TenantID != nil {
		return nil, fmt.Errorf("`tenant_id` must not be provided.")
	}

	name, err := validateName(req.Name, true)
	if err != nil {
		return nil, err
	}

	kbIDs, err := s.validateDatasetIDs(req.DatasetIDs, userID)
	if err != nil {
		return nil, err
	}

	// Resolve tenant / default LLM.
	tenant, tenantErr := s.tenantDAO.GetByID(userID)
	if tenantErr != nil {
		return nil, fmt.Errorf("Tenant not found!")
	}
	llmID := req.LLMID
	if llmID == "" && tenant != nil {
		llmID = tenant.LLMID
	}

	// Apply prompt defaults.
	promptConfig := applyPromptDefaults(req.PromptConfig, kbIDs)

	// Validate parameters vs. system prompt.
	if err := validatePromptParams(promptConfig); err != nil {
		return nil, err
	}

	// Duplicate name check.
	exists, err := s.chatDAO.NameConflictExists(userID, name, "", "1")
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, fmt.Errorf("Duplicated chat name in creating chat.")
	}

	description := req.Description
	if description == "" {
		description = "A helpful Assistant"
	}
	topN := int64(6)
	if req.TopN != nil {
		topN = *req.TopN
	}
	topK := int64(1024)
	if req.TopK != nil {
		topK = *req.TopK
	}
	simThreshold := 0.1
	if req.SimilarityThreshold != nil {
		simThreshold = *req.SimilarityThreshold
	}
	vecWeight := 0.3
	if req.VectorSimilarityWeight != nil {
		vecWeight = *req.VectorSimilarityWeight
	}
	llmSetting := req.LLMSetting
	if llmSetting == nil {
		llmSetting = map[string]interface{}{}
	}
	kbIDsJSON := make(entity.JSONSlice, len(kbIDs))
	for i, id := range kbIDs {
		kbIDsJSON[i] = id
	}
	// Mirror Python's Dialog.language default, which is locale-driven:
	// "Chinese" when the server LANG is zh_CN, otherwise "English".
	lang := "English"
	if strings.Contains(os.Getenv("LANG"), "zh_CN") {
		lang = "Chinese"
	}
	status := "1"
	chat := &entity.Chat{
		ID:                     common.GenerateUUID(),
		TenantID:               userID,
		Name:                   &name,
		Description:            &description,
		Icon:                   &req.Icon,
		Language:               &lang,
		LLMID:                  llmID,
		LLMSetting:             llmSetting,
		PromptConfig:           promptConfig,
		TopN:                   topN,
		TopK:                   topK,
		RerankID:               req.RerankID,
		SimilarityThreshold:    simThreshold,
		VectorSimilarityWeight: vecWeight,
		KBIDs:                  kbIDsJSON,
		Status:                 &status,
	}
	if err := s.chatDAO.Create(chat); err != nil {
		return nil, fmt.Errorf("Failed to create chat.")
	}
	created, err := s.chatDAO.GetByID(chat.ID)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve created chat.")
	}
	return s.buildChatResponseMap(created), nil
}

// ── PatchChat ──────────────────────────────────────────────────────────────────

// PatchChatRequest mirrors Python PATCH /chats/<chat_id> body (all fields optional).
type PatchChatRequest struct {
	Name                   *string                `json:"name"`
	Description            *string                `json:"description"`
	Icon                   *string                `json:"icon"`
	DatasetIDs             []string               `json:"dataset_ids"`
	LLMID                  *string                `json:"llm_id"`
	LLMSetting             map[string]interface{} `json:"llm_setting"`
	RerankID               *string                `json:"rerank_id"`
	PromptConfig           map[string]interface{} `json:"prompt_config"`
	SimilarityThreshold    *float64               `json:"similarity_threshold"`
	VectorSimilarityWeight *float64               `json:"vector_similarity_weight"`
	TopN                   *int64                 `json:"top_n"`
	TopK                   *int64                 `json:"top_k"`
}

// PatchChat partially updates a chat, mirroring Python PATCH /api/v1/chats/<chat_id>.
func (s *ChatService) PatchChat(userID, chatID string, req *PatchChatRequest) (map[string]interface{}, error) {
	current, err := s.ensureOwnedChat(userID, chatID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}

	if req.Name != nil {
		name, err := validateName(req.Name, false)
		if err != nil {
			return nil, err
		}
		if name != "" && !strings.EqualFold(name, derefStr(current.Name)) {
			exists, err := s.chatDAO.NameConflictExists(userID, name, chatID, "1")
			if err != nil {
				return nil, err
			}
			if exists {
				return nil, fmt.Errorf("Duplicated chat name.")
			}
			updates["name"] = name
		}
	}

	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Icon != nil {
		updates["icon"] = *req.Icon
	}

	if req.DatasetIDs != nil {
		kbIDs, err := s.validateDatasetIDs(req.DatasetIDs, userID)
		if err != nil {
			return nil, err
		}
		kbIDsJSON := make(entity.JSONSlice, len(kbIDs))
		for i, id := range kbIDs {
			kbIDsJSON[i] = id
		}
		updates["kb_ids"] = kbIDsJSON
	}

	if req.LLMID != nil {
		updates["llm_id"] = *req.LLMID
	}

	// Merge llm_setting with existing.
	if req.LLMSetting != nil {
		existing := map[string]interface{}{}
		for k, v := range current.LLMSetting {
			existing[k] = v
		}
		for k, v := range req.LLMSetting {
			existing[k] = v
		}
		updates["llm_setting"] = existing
	}

	// Merge prompt_config with existing (PATCH semantics).
	if req.PromptConfig != nil {
		existing := map[string]interface{}{}
		for k, v := range current.PromptConfig {
			existing[k] = v
		}
		for k, v := range req.PromptConfig {
			existing[k] = v
		}
		updates["prompt_config"] = existing
	}

	if req.RerankID != nil {
		updates["rerank_id"] = *req.RerankID
	}
	if req.SimilarityThreshold != nil {
		updates["similarity_threshold"] = *req.SimilarityThreshold
	}
	if req.VectorSimilarityWeight != nil {
		updates["vector_similarity_weight"] = *req.VectorSimilarityWeight
	}
	if req.TopN != nil {
		updates["top_n"] = *req.TopN
	}
	if req.TopK != nil {
		updates["top_k"] = *req.TopK
	}

	if len(updates) > 0 {
		if err := s.chatDAO.UpdateByID(chatID, updates); err != nil {
			return nil, fmt.Errorf("Failed to update chat.")
		}
	}

	updated, err := s.chatDAO.GetByID(chatID)
	if err != nil {
		return nil, fmt.Errorf("Failed to retrieve updated chat.")
	}
	return s.buildChatResponseMap(updated), nil
}

// ── DeleteChatByID ─────────────────────────────────────────────────────────────

// DeleteChatByID soft-deletes a single chat (status → "0"), mirroring Python DELETE /chats/<chat_id>.
func (s *ChatService) DeleteChatByID(userID, chatID string) error {
	if _, err := s.ensureOwnedChat(userID, chatID); err != nil {
		return err
	}
	if err := s.chatDAO.UpdateByID(chatID, map[string]interface{}{"status": "0"}); err != nil {
		return fmt.Errorf("Failed to delete chat %s", chatID)
	}
	return nil
}

// ── CreateChatSession ──────────────────────────────────────────────────────────

// CreateChatSession creates a new conversation for a chat, mirroring Python POST /chats/<chat_id>/sessions.
func (s *ChatService) CreateChatSession(userID, chatID, name string) (map[string]interface{}, error) {
	chat, err := s.ensureOwnedChat(userID, chatID)
	if err != nil {
		return nil, err
	}

	name = strings.TrimSpace(name)
	if name == "" {
		name = "New session"
	}
	if len([]rune(name)) > 255 {
		name = string([]rune(name)[:255])
	}

	prologue := ""
	if p, ok := chat.PromptConfig["prologue"]; ok {
		if ps, ok := p.(string); ok {
			prologue = ps
		}
	}

	initMsg, _ := json.Marshal([]map[string]interface{}{
		{"role": "assistant", "content": prologue},
	})
	refJSON, _ := json.Marshal([]interface{}{})

	session := &entity.ChatSession{
		ID:        common.GenerateUUID(),
		DialogID:  chatID,
		Name:      &name,
		Message:   initMsg,
		Reference: refJSON,
		UserID:    &userID,
	}
	if err := s.chatSessionDAO.Create(session); err != nil {
		return nil, fmt.Errorf("Fail to create a session!")
	}
	created, err := s.chatSessionDAO.GetByID(session.ID)
	if err != nil {
		return nil, fmt.Errorf("Fail to create a session!")
	}
	return buildSessionResponseMap(created), nil
}

// buildSessionResponseMap mirrors Python _build_session_response:
// renames dialog_id → chat_id and message → messages.
func buildSessionResponseMap(s *entity.ChatSession) map[string]interface{} {
	var messages interface{}
	if len(s.Message) > 0 {
		_ = json.Unmarshal(s.Message, &messages)
	}
	if messages == nil {
		messages = []interface{}{}
	}
	var reference interface{}
	if len(s.Reference) > 0 {
		_ = json.Unmarshal(s.Reference, &reference)
	}
	if reference == nil {
		reference = []interface{}{}
	}
	return map[string]interface{}{
		"id":          s.ID,
		"chat_id":     s.DialogID,
		"name":        s.Name,
		"messages":    messages,
		"reference":   reference,
		"user_id":     s.UserID,
		"create_time": s.CreateTime,
		"create_date": s.CreateDate,
		"update_time": s.UpdateTime,
		"update_date": s.UpdateDate,
	}
}

// ── internal helpers ───────────────────────────────────────────────────────────

// applyPromptDefaults mirrors Python _apply_prompt_defaults.
func applyPromptDefaults(pc map[string]interface{}, kbIDs []string) entity.JSONMap {
	defaults := map[string]interface{}{
		"system": ("You are an intelligent assistant. Please summarize the content of the dataset to answer the question. " +
			"Please list the data in the dataset and answer in detail. When all dataset content is irrelevant to the " +
			"question, your answer must include the sentence \"The answer you are looking for is not found in the dataset!\" " +
			"Answers need to consider chat history.\n      Here is the knowledge base:\n      {knowledge}\n      The above is the knowledge base."),
		"prologue":        "Hi! I'm your assistant. What can I do for you?",
		"parameters":      []interface{}{map[string]interface{}{"key": "knowledge", "optional": false}},
		"empty_response":  "Sorry! No relevant content was found in the knowledge base!",
		"quote":           true,
		"tts":             false,
		"refine_multiturn": true,
	}
	result := entity.JSONMap{}
	for k, v := range defaults {
		result[k] = v
	}
	if pc != nil {
		for k, v := range pc {
			result[k] = v
		}
	}
	// If no datasets and parameters reference {knowledge}, keep the defaults as-is.
	// If datasets provided but parameters missing, add the knowledge parameter.
	if len(kbIDs) > 0 {
		if params, ok := result["parameters"]; !ok || params == nil {
			sys, _ := result["system"].(string)
			if strings.Contains(sys, "{knowledge}") {
				result["parameters"] = []interface{}{map[string]interface{}{"key": "knowledge", "optional": false}}
			}
		}
	}
	return result
}

// validatePromptParams checks that every non-optional parameter has a placeholder in the system prompt.
func validatePromptParams(pc entity.JSONMap) error {
	sys, _ := pc["system"].(string)
	params, _ := pc["parameters"].([]interface{})
	for _, p := range params {
		pm, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		optional, _ := pm["optional"].(bool)
		if optional {
			continue
		}
		key, _ := pm["key"].(string)
		if key != "" && !strings.Contains(sys, fmt.Sprintf("{%s}", key)) {
			return fmt.Errorf("Parameter '%s' is not used", key)
		}
	}
	return nil
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
