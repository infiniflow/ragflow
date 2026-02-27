package service

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"ragflow/internal/dao"
	"ragflow/internal/model"
)

// ChatService chat service
type ChatService struct {
	chatDAO       *dao.ChatDAO
	kbDAO         *dao.KnowledgebaseDAO
	userTenantDAO *dao.UserTenantDAO
	tenantDAO     *dao.TenantDAO
}

// NewChatService create chat service
func NewChatService() *ChatService {
	return &ChatService{
		chatDAO:       dao.NewChatDAO(),
		kbDAO:         dao.NewKnowledgebaseDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
		tenantDAO:     dao.NewTenantDAO(),
	}
}

// ChatWithKBNames chat with knowledge base names
type ChatWithKBNames struct {
	*model.Chat
	KBNames []string `json:"kb_names"`
}

// ListChatsResponse list chats response
type ListChatsResponse struct {
	Chats []*ChatWithKBNames `json:"chats"`
}

// ListChats list chats for a user
func (s *ChatService) ListChats(userID string, status string) (*ListChatsResponse, error) {
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

	// Enrich with knowledge base names
	var chatsWithKBNames []*ChatWithKBNames
	for _, chat := range chats {
		kbNames := s.getKBNames(chat.KBIDs)
		chatsWithKBNames = append(chatsWithKBNames, &ChatWithKBNames{
			Chat:    chat,
			KBNames: kbNames,
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
	var chats []*model.Chat
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
				chats = []*model.Chat{}
			}
		}
	}

	// Enrich with knowledge base names
	var chatsWithKBNames []*ChatWithKBNames
	for _, chat := range chats {
		kbNames := s.getKBNames(chat.KBIDs)
		chatsWithKBNames = append(chatsWithKBNames, &ChatWithKBNames{
			Chat:    chat,
			KBNames: kbNames,
		})
	}

	return &ListChatsNextResponse{
		Chats: chatsWithKBNames,
		Total: total,
	}, nil
}

// getKBNames gets knowledge base names by IDs
func (s *ChatService) getKBNames(kbIDs model.JSONSlice) []string {
	var names []string
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
		}
	}
	return names
}

// ParameterConfig parameter configuration in prompt_config
type ParameterConfig struct {
	Key      string `json:"key"`
	Optional bool   `json:"optional"`
}

// PromptConfig prompt configuration
type PromptConfig struct {
	System           string            `json:"system"`
	Prologue         string            `json:"prologue"`
	Parameters       []ParameterConfig `json:"parameters"`
	EmptyResponse    string            `json:"empty_response"`
	TavilyAPIKey     string            `json:"tavily_api_key,omitempty"`
	Keyword          bool              `json:"keyword,omitempty"`
	Quote            bool              `json:"quote,omitempty"`
	Reasoning        bool              `json:"reasoning,omitempty"`
	RefineMultiturn  bool              `json:"refine_multiturn,omitempty"`
	TocEnhance       bool              `json:"toc_enhance,omitempty"`
	TTS              bool              `json:"tts,omitempty"`
	UseKG            bool              `json:"use_kg,omitempty"`
}

// SetDialogRequest set dialog request
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

// SetDialogResponse set dialog response
type SetDialogResponse struct {
	*model.Chat
	KBNames []string `json:"kb_names"`
}

// SetDialog create or update a dialog
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
		description = "A helpful dialog"
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
	promptConfigMap := model.JSONMap{
		"system":            promptConfig.System,
		"prologue":          promptConfig.Prologue,
		"empty_response":    promptConfig.EmptyResponse,
		"keyword":           promptConfig.Keyword,
		"quote":             promptConfig.Quote,
		"reasoning":         promptConfig.Reasoning,
		"refine_multiturn":  promptConfig.RefineMultiturn,
		"toc_enhance":       promptConfig.TocEnhance,
		"tts":               promptConfig.TTS,
		"use_kg":            promptConfig.UseKG,
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
	kbIDsJSON := make(model.JSONSlice, len(kbIDs))
	for i, id := range kbIDs {
		kbIDsJSON[i] = id
	}

	if isCreate {
		// Generate UUID for new dialog
		newID := uuid.New().String()
		newID = strings.ReplaceAll(newID, "-", "")
		if len(newID) > 32 {
			newID = newID[:32]
		}

		// Get current time
		now := time.Now()
		createTime := now.UnixMilli()

		// Set default language
		language := "English"

		// Create new dialog
		chat := &model.Chat{
			ID:                     newID,
			TenantID:               tenantID,
			Name:                   &name,
			Description:            &description,
			Icon:                   &req.Icon,
			Language:               &language,
			LLMID:                  llmID,
			LLMSetting:             llmSetting,
			PromptConfig:           promptConfigMap,
			MetaDataFilter:         (*model.JSONMap)(&metaDataFilter),
			TopN:                   topN,
			TopK:                   topK,
			RerankID:               rerankID,
			SimilarityThreshold:    similarityThreshold,
			VectorSimilarityWeight: vectorSimilarityWeight,
			KBIDs:                  kbIDsJSON,
			Status:                 strPtr("1"),
		}
		chat.CreateTime = createTime
		chat.CreateDate = &now
		chat.UpdateTime = &createTime
		chat.UpdateDate = &now

		if err := s.chatDAO.Create(chat); err != nil {
			return nil, errors.New("Fail to new a dialog")
		}

		// Get KB names
		kbNames := s.getKBNames(chat.KBIDs)

		return &SetDialogResponse{
			Chat:    chat,
			KBNames: kbNames,
		}, nil
	}

	// Update existing dialog - also update update_time
	now := time.Now()
	updateTime := now.UnixMilli()
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
		"update_time":              updateTime,
		"update_date":              now,
	}

	if err := s.chatDAO.UpdateByID(req.DialogID, updateData); err != nil {
		return nil, errors.New("Dialog not found")
	}

	// Get updated dialog
	chat, err := s.chatDAO.GetByID(req.DialogID)
	if err != nil {
		return nil, errors.New("Fail to update a dialog")
	}

	// Get KB names
	kbNames := s.getKBNames(chat.KBIDs)

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
func getEmbdIDs(kbs []*model.Knowledgebase) []string {
	ids := make([]string, len(kbs))
	for i, kb := range kbs {
		ids[i] = kb.EmbdID
	}
	return ids
}

// strPtr returns a pointer to a string
func strPtr(s string) *string {
	return &s
}

// Helper to count UTF-8 characters (not bytes)
func (s *ChatService) countRunes(str string) int {
	return utf8.RuneCountInString(str)
}
