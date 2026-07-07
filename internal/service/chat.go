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
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	"strings"
	"unicode/utf8"

	"ragflow/internal/dao"
)

var DefaultRerankModels = map[string]struct{}{
	"BAAI/bge-reranker-v2-m3":           {},
	"maidalun1020/bce-reranker-base_v1": {},
}

var ReadOnlyFields = map[string]struct{}{
	"id":          {},
	"tenant_id":   {},
	"created_by":  {},
	"create_time": {},
	"create_date": {},
	"update_time": {},
	"update_date": {},
}

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
	*entity.Chat
	KBNames    []string `json:"kb_names"`
	DatasetIDs []string `json:"dataset_ids"`
}

// ListChatsResponse list chats response
type ListChatsResponse struct {
	Total int64              `json:"total"`
	Chats []*ChatWithKBNames `json:"chats"`
}

// ListChats list chats for a user
func (s *ChatService) ListChats(userID, status, keywords string, page, pageSize int, orderby string, desc bool) (*ListChatsResponse, error) {
	chats, total, err := s.chatDAO.ListByTenantIDs(
		nil,
		userID,
		page,
		pageSize,
		orderby,
		desc,
		keywords,
	)
	if err != nil {
		return nil, err
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
		Total: total,
		Chats: chatsWithKBNames,
	}, nil
}

type CreateChatRequest struct {
	Name                   string
	DatasetIDs             []string               `json:"dataset_ids"`
	KBIDs                  []string               `json:"kb_ids"`
	LLMID                  *string                `json:"llm_id"`
	LLMSetting             map[string]interface{} `json:"llm_setting"`
	RerankID               *string                `json:"rerank_id"`
	PromptConfig           map[string]interface{} `json:"prompt_config"`
	Description            *string
	TopN                   *int
	TopK                   *int
	SimilarityThreshold    *float64
	VectorSimilarityWeight *float64
	Icon                   *string
	TenantID               *string `json:"tenant_id"`
}

func (s *ChatService) Create(userID string, req map[string]interface{}) (map[string]interface{}, common.ErrorCode, error) {
	tenant, err := s.tenantDAO.GetByID(userID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("Tenant not found!")
	}

	if tenantValue, ok := req["tenant_id"]; ok && isTruthy(tenantValue) {
		return nil, common.CodeDataError, errors.New("`tenant_id` must not be provided.")
	}

	name, err := validateCreateChatName(req["name"])
	if err != nil {
		return nil, common.CodeDataError, err
	}
	req["name"] = name

	if datasetIDsValue, ok := req["dataset_ids"]; ok {
		kbIDs, err := s.validateCreateDatasetIDs(datasetIDsValue, userID)
		if err != nil {
			return nil, common.CodeDataError, err
		}
		req["kb_ids"] = kbIDs
		delete(req, "dataset_ids")
	}

	if llmIDValue, ok := req["llm_id"]; ok {
		llmID := stringFromValue(llmIDValue)
		llmSetting, _ := mapFromValue(req["llm_setting"])
		if err = validateCreateLLMID(llmID, userID, llmSetting); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if rerankIDValue, ok := req["rerank_id"]; ok {
		rerankID := stringFromValue(rerankIDValue)
		if err = validateCreateRerankID(rerankID, userID); err != nil {
			return nil, common.CodeDataError, err
		}
	}

	if promptConfigValue, ok := req["prompt_config"]; ok {
		if _, ok := mapFromValue(promptConfigValue); !ok {
			return nil, common.CodeDataError, errors.New("`prompt_config` should be an object.")
		}
	}

	if metaDataFilterValue, ok := req["meta_data_filter"]; ok && metaDataFilterValue != nil {
		if _, ok := mapFromValue(metaDataFilterValue); !ok {
			return nil, common.CodeDataError, errors.New("`meta_data_filter` should be an object.")
		}
	}

	if _, ok := req["kb_ids"]; !ok {
		req["kb_ids"] = []string{}
	}
	if _, ok := req["llm_id"]; !ok || req["llm_id"] == nil {
		req["llm_id"] = tenant.LLMID
	}
	if _, ok := req["llm_setting"]; !ok || req["llm_setting"] == nil {
		req["llm_setting"] = map[string]interface{}{}
	}
	if _, ok := req["description"]; !ok {
		req["description"] = "A helpful Assistant"
	}
	if _, ok := req["top_n"]; !ok {
		req["top_n"] = 6
	}
	if _, ok := req["top_k"]; !ok {
		req["top_k"] = 1024
	}
	if _, ok := req["rerank_id"]; !ok {
		req["rerank_id"] = ""
	}
	if _, ok := req["similarity_threshold"]; !ok {
		req["similarity_threshold"] = 0.1
	}
	if _, ok := req["vector_similarity_weight"]; !ok {
		req["vector_similarity_weight"] = 0.3
	}
	if _, ok := req["icon"]; !ok {
		req["icon"] = ""
	}
	if _, ok := req["meta_data_filter"]; !ok || req["meta_data_filter"] == nil {
		req["meta_data_filter"] = map[string]interface{}{}
	}

	applyCreatePromptDefaults(req)
	filterCreateChatPersistedFields(req)

	exists, err := s.chatDAO.ExistsByNameTenantStatus(name, userID, string(entity.StatusValid))
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if exists {
		return nil, common.CodeDataError, errors.New("Duplicated chat name in creating chat.")
	}

	chat := buildCreateChatEntity(req, userID)
	if err = s.chatDAO.Create(chat); err != nil {
		return nil, common.CodeDataError, errors.New("Failed to create chat.")
	}

	chat, err = s.chatDAO.GetByID(chat.ID)
	if err != nil {
		return nil, common.CodeDataError, errors.New("Failed to retrieve created chat.")
	}

	response, err := s.buildCreateChatResponse(chat)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	return response, common.CodeSuccess, nil
}

func validateCreateChatName(value interface{}) (string, error) {
	if value == nil {
		return "", errors.New("`name` is required.")
	}
	name, ok := value.(string)
	if !ok {
		return "", errors.New("Chat name must be a string.")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("`name` is required.")
	}
	if len([]byte(name)) > 255 {
		return "", fmt.Errorf("Chat name length is %d which is larger than 255.", len([]byte(name)))
	}
	return name, nil
}

func (s *ChatService) validateCreateDatasetIDs(value interface{}, tenantID string) ([]string, error) {
	if value == nil {
		return []string{}, nil
	}
	values, ok := listFromValue(value)
	if !ok {
		return nil, errors.New("`dataset_ids` should be a list.")
	}

	normalizedIDs := make([]string, 0, len(values))
	kbs := make([]*entity.Knowledgebase, 0, len(values))
	for _, item := range values {
		if !isTruthy(item) {
			continue
		}
		datasetID := stringFromValue(item)
		normalizedIDs = append(normalizedIDs, datasetID)
	}

	for _, datasetID := range normalizedIDs {
		if !s.kbDAO.Accessible(datasetID, tenantID) {
			return nil, fmt.Errorf("You don't own the dataset %s", datasetID)
		}
		kb, err := s.kbDAO.GetByID(datasetID)
		if err != nil {
			return nil, fmt.Errorf("You don't own the dataset %s", datasetID)
		}
		if kb.ChunkNum == 0 {
			return nil, fmt.Errorf("The dataset %s doesn't own parsed file", datasetID)
		}
		kbs = append(kbs, kb)
	}

	embedIDs := make(map[string]struct{}, len(kbs))
	for _, kb := range kbs {
		embedIDs[s.splitModelNameAndFactory(kb.EmbdID)] = struct{}{}
	}
	if len(embedIDs) > 1 {
		return nil, fmt.Errorf("Datasets use different embedding models: %v", getEmbdIDs(kbs))
	}
	return normalizedIDs, nil
}

func validateCreateLLMID(llmID, tenantID string, llmSetting map[string]interface{}) error {
	if llmID == "" {
		return nil
	}
	modelType := entity.ModelTypeChat
	switch confModelType := llmSetting["model_type"].(type) {
	case string:
		if confModelType == string(entity.ModelTypeImage2Text) {
			modelType = entity.ModelTypeImage2Text
		}
	case []interface{}:
		for _, item := range confModelType {
			if item == string(entity.ModelTypeImage2Text) {
				modelType = entity.ModelTypeImage2Text
				break
			}
		}
	case []string:
		for _, item := range confModelType {
			if item == string(entity.ModelTypeImage2Text) {
				modelType = entity.ModelTypeImage2Text
				break
			}
		}
	}
	if _, _, _, _, err := NewModelProviderService().GetModelConfigFromProviderInstance(tenantID, modelType, llmID); err != nil {
		return fmt.Errorf("`llm_id` %s doesn't exist", llmID)
	}
	return nil
}

func validateCreateRerankID(rerankID, tenantID string) error {
	if rerankID == "" {
		return nil
	}
	llmName := strings.Split(rerankID, "@")[0]
	if _, ok := DefaultRerankModels[llmName]; ok {
		return nil
	}
	if _, _, _, _, err := NewModelProviderService().GetModelConfigFromProviderInstance(tenantID, entity.ModelTypeRerank, rerankID); err != nil {
		return fmt.Errorf("`rerank_id` %s doesn't exist", rerankID)
	}
	return nil
}

func applyCreatePromptDefaults(req map[string]interface{}) {
	promptConfig, _ := mapFromValue(req["prompt_config"])
	if promptConfig == nil {
		promptConfig = map[string]interface{}{}
	}
	if system, ok := promptConfig["system"]; !ok || !isTruthy(system) {
		promptConfig["system"] = pyDefaultSystemPrompt
	}
	if _, ok := promptConfig["prologue"]; !ok {
		promptConfig["prologue"] = pyDefaultPrologue
	}
	if _, ok := promptConfig["parameters"]; !ok {
		promptConfig["parameters"] = []interface{}{map[string]interface{}{"key": "knowledge", "optional": false}}
	}
	if _, ok := promptConfig["empty_response"]; !ok {
		promptConfig["empty_response"] = pyDefaultEmptyResponse
	}
	if _, ok := promptConfig["quote"]; !ok {
		promptConfig["quote"] = true
	}
	if _, ok := promptConfig["tts"]; !ok {
		promptConfig["tts"] = false
	}
	if _, ok := promptConfig["refine_multiturn"]; !ok {
		promptConfig["refine_multiturn"] = true
	}

	kbIDs, _ := listFromValue(req["kb_ids"])
	system, _ := promptConfig["system"].(string)
	if len(kbIDs) > 0 && !isTruthy(promptConfig["parameters"]) && strings.Contains(system, "{knowledge}") {
		promptConfig["parameters"] = []interface{}{map[string]interface{}{"key": "knowledge", "optional": false}}
	}
	req["prompt_config"] = promptConfig
}

func filterCreateChatPersistedFields(req map[string]interface{}) {
	persisted := map[string]struct{}{
		"name": {}, "description": {}, "icon": {}, "language": {}, "llm_id": {}, "tenant_llm_id": {},
		"llm_setting": {}, "prompt_type": {}, "prompt_config": {}, "meta_data_filter": {},
		"similarity_threshold": {}, "vector_similarity_weight": {}, "top_n": {}, "top_k": {},
		"do_refer": {}, "rerank_id": {}, "tenant_rerank_id": {}, "kb_ids": {}, "status": {},
	}
	for key := range req {
		if _, ok := persisted[key]; !ok {
			delete(req, key)
		}
	}
	for key := range ReadOnlyFields {
		delete(req, key)
	}
}

func buildCreateChatEntity(req map[string]interface{}, tenantID string) *entity.Chat {
	name := stringFromValue(req["name"])
	description := stringFromValue(req["description"])
	icon := stringFromValue(req["icon"])
	llmID := stringFromValue(req["llm_id"])
	rerankID := stringFromValue(req["rerank_id"])
	llmSetting, _ := mapFromValue(req["llm_setting"])
	promptConfig, _ := mapFromValue(req["prompt_config"])
	kbIDs, _ := stringListFromValue(req["kb_ids"])
	kbIDsJSON := make(entity.JSONSlice, 0, len(kbIDs))
	for _, id := range kbIDs {
		kbIDsJSON = append(kbIDsJSON, id)
	}
	status, hasStatus := req["status"]
	statusValue := string(entity.StatusValid)
	if hasStatus {
		statusValue = stringFromValue(status)
	}

	chat := &entity.Chat{
		ID:                     utility.GenerateUUID(),
		TenantID:               tenantID,
		Name:                   &name,
		Description:            &description,
		Icon:                   &icon,
		LLMID:                  llmID,
		LLMSetting:             entity.JSONMap(llmSetting),
		PromptType:             stringFromValue(req["prompt_type"]),
		PromptConfig:           entity.JSONMap(promptConfig),
		SimilarityThreshold:    floatFromValue(req["similarity_threshold"]),
		VectorSimilarityWeight: floatFromValue(req["vector_similarity_weight"]),
		TopN:                   int64FromValue(req["top_n"]),
		TopK:                   int64FromValue(req["top_k"]),
		DoRefer:                stringFromValue(req["do_refer"]),
		RerankID:               rerankID,
		KBIDs:                  kbIDsJSON,
		Status:                 &statusValue,
	}
	if chat.PromptType == "" {
		chat.PromptType = "simple"
	}
	if chat.DoRefer == "" {
		chat.DoRefer = "1"
	}
	if language := stringFromValue(req["language"]); language != "" {
		chat.Language = &language
	}
	if metaDataFilter, ok := mapFromValue(req["meta_data_filter"]); ok {
		metaDataFilterJSON := entity.JSONMap(metaDataFilter)
		chat.MetaDataFilter = &metaDataFilterJSON
	} else {
		metaDataFilterJSON := entity.JSONMap{}
		chat.MetaDataFilter = &metaDataFilterJSON
	}
	return chat
}

func (s *ChatService) buildCreateChatResponse(chat *entity.Chat) (map[string]interface{}, error) {
	data, err := structToMap(chat)
	if err != nil {
		return nil, err
	}
	kbNames, datasetIDs := s.getDatasetNamesAndIDs(chat.KBIDs)
	data["dataset_ids"] = datasetIDs
	delete(data, "kb_ids")
	data["kb_names"] = kbNames
	data["meta_data_filter"] = normalizeMetaDataFilter(chat.MetaDataFilter)
	return data, nil
}

func structToMap(value interface{}) (map[string]interface{}, error) {
	bytes, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	result := map[string]interface{}{}
	if err = json.Unmarshal(bytes, &result); err != nil {
		return nil, err
	}
	return result, nil
}

func stringFromValue(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func mapFromValue(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case map[string]interface{}:
		return typed, true
	case entity.JSONMap:
		return map[string]interface{}(typed), true
	default:
		return nil, false
	}
}

func normalizeMetaDataFilter(value *entity.JSONMap) entity.JSONMap {
	if value == nil || *value == nil {
		return entity.JSONMap{}
	}
	return *value
}

func listFromValue(value interface{}) ([]interface{}, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case []interface{}:
		return typed, true
	case []string:
		result := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			result = append(result, item)
		}
		return result, true
	case entity.JSONSlice:
		return []interface{}(typed), true
	default:
		return nil, false
	}
}

func stringListFromValue(value interface{}) ([]string, bool) {
	values, ok := listFromValue(value)
	if !ok {
		return nil, false
	}
	result := make([]string, 0, len(values))
	for _, item := range values {
		if !isTruthy(item) {
			continue
		}
		result = append(result, stringFromValue(item))
	}
	return result, true
}

func int64FromValue(value interface{}) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case json.Number:
		n, err := typed.Int64()
		if err == nil {
			return n
		}
		f, _ := typed.Float64()
		return int64(f)
	default:
		return 0
	}
}

func floatFromValue(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		n, _ := typed.Float64()
		return n
	default:
		return 0
	}
}

func isTruthy(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case bool:
		return typed
	case string:
		return typed != ""
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case json.Number:
		n, err := typed.Float64()
		return err != nil || n != 0
	case []interface{}:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case map[string]interface{}:
		return len(typed) > 0
	default:
		return true
	}
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

const (
	pyDefaultSystemPrompt = "You are an intelligent assistant. Please summarize the content of the dataset to answer the question. " +
		"Please list the data in the dataset and answer in detail. " +
		"When all dataset content is irrelevant to the question, your answer must include the sentence " +
		`"The answer you are looking for is not found in the dataset!" ` +
		"Answers need to consider chat history.\n" +
		"      Here is the knowledge base:\n" +
		"      {knowledge}\n" +
		"      The above is the knowledge base."

	pyDefaultPrologue      = "Hi! I'm your assistant. What can I do for you?"
	pyDefaultEmptyResponse = "Sorry! No relevant content was found in the knowledge base!"
)

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

func (s *ChatService) getOwnedValidChat(userID, chatID string) (*entity.Chat, error) {
	chat, err := s.chatDAO.GetByIDAndStatus(chatID, string(entity.StatusValid))
	if err != nil {
		return nil, errors.New("no authorization")
	}
	if chat.TenantID != userID {
		return nil, errors.New("no authorization")
	}
	return chat, nil
}

var chatPersistedFields = map[string]struct{}{
	"name":                     {},
	"description":              {},
	"icon":                     {},
	"language":                 {},
	"llm_id":                   {},
	"tenant_llm_id":            {},
	"llm_setting":              {},
	"prompt_type":              {},
	"prompt_config":            {},
	"meta_data_filter":         {},
	"similarity_threshold":     {},
	"vector_similarity_weight": {},
	"top_n":                    {},
	"top_k":                    {},
	"do_refer":                 {},
	"rerank_id":                {},
	"tenant_rerank_id":         {},
	"kb_ids":                   {},
	"status":                   {},
}

var chatReadonlyFields = map[string]struct{}{
	"id":          {},
	"tenant_id":   {},
	"created_by":  {},
	"create_time": {},
	"create_date": {},
	"update_time": {},
	"update_date": {},
}

var defaultRerankModels = map[string]struct{}{
	"BAAI/bge-reranker-v2-m3":           {},
	"maidalun1020/bce-reranker-base_v1": {},
}

// UpdateChat mirrors PUT /api/v1/chats/<chat_id> in the Python REST API.
func (s *ChatService) UpdateChat(userID, chatID string, req map[string]interface{}) (map[string]interface{}, error) {
	return s.updateChatREST(userID, chatID, req, false)
}

// PatchChat mirrors PATCH /api/v1/chats/<chat_id> in the Python REST API.
func (s *ChatService) PatchChat(userID, chatID string, req map[string]interface{}) (map[string]interface{}, error) {
	return s.updateChatREST(userID, chatID, req, true)
}

func (s *ChatService) updateChatREST(userID, chatID string, req map[string]interface{}, patch bool) (map[string]interface{}, error) {
	currentChat, err := s.getOwnedValidChat(userID, chatID)
	if err != nil {
		return nil, err
	}
	if _, err := s.tenantDAO.GetByID(userID); err != nil {
		return nil, errors.New("Tenant not found!")
	}

	if !patch && isTruthy(req["tenant_id"]) {
		return nil, errors.New("`tenant_id` must not be provided.")
	}

	if value, ok := req["name"]; ok {
		name, shouldSet, err := validateRESTChatName(value, !patch)
		if err != nil {
			return nil, err
		}
		if shouldSet {
			req["name"] = name
		} else {
			delete(req, "name")
		}
	}

	if value, ok := req["dataset_ids"]; ok {
		kbIDs, err := s.validateRESTDatasetIDs(value, userID)
		if err != nil {
			return nil, err
		}
		req["kb_ids"] = kbIDs
		delete(req, "dataset_ids")
	}

	var llmSetting map[string]interface{}
	llmSettingProvided := false
	if value, ok := req["llm_setting"]; ok {
		llmSettingProvided = true
		setting, ok := mapFromValue(value)
		if !ok {
			return nil, errors.New("`llm_setting` should be an object.")
		}
		llmSetting = setting
	}

	if value, ok := req["llm_id"]; ok {
		llmID := fmt.Sprint(value)
		if err := s.validateRESTLLMID(llmID, userID, llmSetting); err != nil {
			return nil, err
		}
	}

	if value, ok := req["rerank_id"]; ok {
		rerankID := fmt.Sprint(value)
		if err := s.validateRESTRerankID(rerankID, userID); err != nil {
			return nil, err
		}
	}

	if value, ok := req["prompt_config"]; ok {
		promptConfig, ok := mapFromValue(value)
		if !ok {
			return nil, errors.New("`prompt_config` should be an object.")
		}
		if patch {
			req["prompt_config"] = mergeJSONMap(currentChat.PromptConfig, promptConfig)
		} else {
			req["prompt_config"] = entity.JSONMap(promptConfig)
		}
	}

	if llmSettingProvided {
		if patch {
			req["llm_setting"] = mergeJSONMap(currentChat.LLMSetting, llmSetting)
		} else {
			req["llm_setting"] = entity.JSONMap(llmSetting)
		}
	}

	if value, ok := req["meta_data_filter"]; ok {
		if value == nil {
			req["meta_data_filter"] = entity.JSONMap{}
		} else {
			metaDataFilter, ok := mapFromValue(value)
			if !ok {
				return nil, errors.New("`meta_data_filter` should be an object.")
			}
			req["meta_data_filter"] = entity.JSONMap(metaDataFilter)
		}
	} else if currentChat.MetaDataFilter == nil || *currentChat.MetaDataFilter == nil {
		req["meta_data_filter"] = entity.JSONMap{}
	}

	updates := filterRESTChatUpdates(req)
	if value, ok := updates["name"]; ok {
		name := value.(string)
		currentName := ""
		if currentChat.Name != nil {
			currentName = *currentChat.Name
		}
		if strings.ToLower(name) != strings.ToLower(currentName) {
			existingNames, err := s.chatDAO.GetExistingNames(userID, string(entity.StatusValid))
			if err != nil {
				return nil, err
			}
			for _, existingName := range existingNames {
				if existingName == name {
					return nil, errors.New("Duplicated chat name.")
				}
			}
		}
	}

	if len(updates) > 0 {
		if err := s.chatDAO.UpdateByID(chatID, updates); err != nil {
			if patch {
				return nil, errors.New("Failed to update chat.")
			}
			return nil, errors.New("Chat not found!")
		}
	}

	updatedChat, err := s.chatDAO.GetByID(chatID)
	if err != nil {
		return nil, errors.New("Failed to retrieve updated chat.")
	}
	return s.buildRESTChatResponse(updatedChat), nil
}

func validateRESTChatName(value interface{}, required bool) (string, bool, error) {
	if value == nil {
		if required {
			return "", false, errors.New("`name` is required.")
		}
		return "", false, nil
	}
	name, ok := value.(string)
	if !ok {
		return "", false, errors.New("Chat name must be a string.")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		if required {
			return "", false, errors.New("`name` is required.")
		}
		return "", false, errors.New("`name` cannot be empty.")
	}
	if len([]byte(name)) > 255 {
		return "", false, fmt.Errorf("Chat name length is %d which is larger than 255.", len([]byte(name)))
	}
	return name, true, nil
}

func (s *ChatService) validateRESTDatasetIDs(value interface{}, userID string) (entity.JSONSlice, error) {
	if value == nil {
		return entity.JSONSlice{}, nil
	}
	items, ok := value.([]interface{})
	if !ok {
		return nil, errors.New("`dataset_ids` should be a list.")
	}

	var kbs []*entity.Knowledgebase
	kbIDs := make(entity.JSONSlice, 0, len(items))
	for _, item := range items {
		if !isTruthy(item) {
			continue
		}
		datasetID := fmt.Sprint(item)
		if !s.kbDAO.Accessible(datasetID, userID) {
			return nil, fmt.Errorf("You don't own the dataset %s", datasetID)
		}
		kb, err := s.kbDAO.GetByID(datasetID)
		if err != nil || kb == nil {
			return nil, fmt.Errorf("You don't own the dataset %s", datasetID)
		}
		if kb.ChunkNum == 0 {
			return nil, fmt.Errorf("The dataset %s doesn't own parsed file", datasetID)
		}
		kbs = append(kbs, kb)
		kbIDs = append(kbIDs, datasetID)
	}

	embdIDs := make([]string, 0, len(kbs))
	seenEmbdIDs := make(map[string]struct{})
	for _, kb := range kbs {
		embdIDs = append(embdIDs, kb.EmbdID)
		seenEmbdIDs[s.splitModelNameAndFactory(kb.EmbdID)] = struct{}{}
	}
	if len(seenEmbdIDs) > 1 {
		return nil, fmt.Errorf("Datasets use different embedding models: %v", embdIDs)
	}
	return kbIDs, nil
}

func (s *ChatService) validateRESTLLMID(llmID, tenantID string, llmSetting map[string]interface{}) error {
	if llmID == "" {
		return nil
	}
	modelType := entity.ModelTypeChat
	if rawModelType, ok := llmSetting["model_type"]; ok {
		switch typedModelType := rawModelType.(type) {
		case string:
			if typedModelType == string(entity.ModelTypeImage2Text) {
				modelType = entity.ModelTypeImage2Text
			}
		case []interface{}:
			for _, item := range typedModelType {
				if fmt.Sprint(item) == string(entity.ModelTypeImage2Text) {
					modelType = entity.ModelTypeImage2Text
					break
				}
			}
		}
	}
	if _, _, _, _, err := NewModelProviderService().GetModelConfigFromProviderInstance(tenantID, modelType, llmID); err != nil {
		return fmt.Errorf("`llm_id` %s doesn't exist", llmID)
	}
	return nil
}

func (s *ChatService) validateRESTRerankID(rerankID, tenantID string) error {
	if rerankID == "" {
		return nil
	}
	baseName := s.splitModelNameAndFactory(rerankID)
	if _, ok := defaultRerankModels[baseName]; ok {
		return nil
	}
	if _, _, _, _, err := NewModelProviderService().GetModelConfigFromProviderInstance(tenantID, entity.ModelTypeRerank, rerankID); err != nil {
		return fmt.Errorf("`rerank_id` %s doesn't exist", rerankID)
	}
	return nil
}

func filterRESTChatUpdates(req map[string]interface{}) map[string]interface{} {
	updates := make(map[string]interface{})
	for field, value := range req {
		if _, ok := chatPersistedFields[field]; !ok {
			continue
		}
		if _, ok := chatReadonlyFields[field]; ok {
			continue
		}
		updates[field] = value
	}
	return updates
}

func mergeJSONMap(base entity.JSONMap, patch map[string]interface{}) entity.JSONMap {
	merged := entity.JSONMap{}
	for key, value := range base {
		merged[key] = value
	}
	for key, value := range patch {
		merged[key] = value
	}
	return merged
}

func (s *ChatService) buildRESTChatResponse(chat *entity.Chat) map[string]interface{} {
	kbNames, datasetIDs := s.getDatasetNamesAndIDs(chat.KBIDs)
	return map[string]interface{}{
		"id":                       chat.ID,
		"tenant_id":                chat.TenantID,
		"name":                     chat.Name,
		"description":              chat.Description,
		"icon":                     chat.Icon,
		"language":                 chat.Language,
		"llm_id":                   chat.LLMID,
		"tenant_llm_id":            chat.TenantLLMID,
		"llm_setting":              chat.LLMSetting,
		"prompt_type":              chat.PromptType,
		"prompt_config":            chat.PromptConfig,
		"meta_data_filter":         normalizeMetaDataFilter(chat.MetaDataFilter),
		"similarity_threshold":     chat.SimilarityThreshold,
		"vector_similarity_weight": chat.VectorSimilarityWeight,
		"top_n":                    chat.TopN,
		"top_k":                    chat.TopK,
		"do_refer":                 chat.DoRefer,
		"rerank_id":                chat.RerankID,
		"tenant_rerank_id":         chat.TenantRerankID,
		"dataset_ids":              datasetIDs,
		"kb_names":                 kbNames,
		"status":                   chat.Status,
		"create_time":              chat.CreateTime,
		"create_date":              chat.CreateDate,
		"update_time":              chat.UpdateTime,
		"update_date":              chat.UpdateDate,
	}
}

// DeleteChat soft deletes a single chat owned by the current user.
func (s *ChatService) DeleteChat(userID, chatID string) error {
	if _, err := s.getOwnedValidChat(userID, chatID); err != nil {
		return err
	}
	if err := s.chatDAO.UpdateByID(chatID, map[string]interface{}{
		"status": string(entity.StatusInvalid),
	}); err != nil {
		return fmt.Errorf("Failed to delete chat %s", chatID)
	}

	return nil
}

// BulkDeleteChatsRequest matches DELETE /api/v1/chats request semantics.
type BulkDeleteChatsRequest struct {
	IDs       []string `json:"ids,omitempty"`
	DeleteAll bool     `json:"delete_all,omitempty"`
	ChatID    string   `json:"chat_id,omitempty"`
}

// checkDuplicateChatIDs
func checkDuplicateChatIDs(ids []string) ([]string, []string) {
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
			duplicateMessages = append(duplicateMessages, fmt.Sprintf("Duplicate chat ids: %s", id))
		}
	}
	return uniqueIDs, duplicateMessages
}

// BulkDeleteChats soft deletes chats owned by the current user with partial success semantics.
func (s *ChatService) BulkDeleteChats(userID string, req *BulkDeleteChatsRequest) (map[string]interface{}, error) {
	ids := req.IDs
	if len(ids) == 0 && req.DeleteAll {
		chats, err := s.chatDAO.ListByTenantID(userID, string(entity.StatusValid))
		if err != nil {
			return nil, err
		}
		for _, chat := range chats {
			ids = append(ids, chat.ID)
		}
		if len(ids) == 0 {
			return map[string]interface{}{}, nil
		}
	}

	uniqueIDs, duplicateMessages := checkDuplicateChatIDs(ids)
	errorsList := make([]string, 0, len(duplicateMessages))
	errorsList = append(errorsList, duplicateMessages...)
	successCount := 0

	for _, chatID := range uniqueIDs {
		if _, err := s.getOwnedValidChat(userID, chatID); err != nil {
			errorsList = append(errorsList, fmt.Sprintf("Chat(%s) not found.", chatID))
			continue
		}
		if err := s.chatDAO.UpdateByID(chatID, map[string]interface{}{
			"status": string(entity.StatusInvalid),
		}); err != nil {
			errorsList = append(errorsList, fmt.Sprintf("Failed to delete chat %s", chatID))
			continue
		}
		successCount++
	}

	if len(errorsList) == 0 {
		return map[string]interface{}{"success_count": successCount}, nil
	}
	if successCount > 0 {
		return map[string]interface{}{
			"success_count": successCount,
			"errors":        errorsList,
		}, nil
	}

	return nil, errors.New(strings.Join(errorsList, "; "))
}

// strPtr returns a pointer to a string
func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
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

	return &GetChatResponse{
		Chat:       chat,
		DatasetIDs: datasetIDs,
		KBNames:    kbNames,
	}, nil
}
