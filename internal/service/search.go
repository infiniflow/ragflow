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
	"fmt"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
)

// SearchService search service
type SearchService struct {
	searchDAO     *dao.SearchDAO
	datasetDAO    *dao.KnowledgebaseDAO
	tenantService *TenantService
}

// NewSearchService create search service
func NewSearchService() *SearchService {
	return &SearchService{
		searchDAO:     dao.NewSearchDAO(),
		datasetDAO:    dao.NewKnowledgebaseDAO(),
		tenantService: NewTenantService(),
	}
}

func (s *SearchService) SetTenantService(tenantService *TenantService) {
	if tenantService != nil {
		s.tenantService = tenantService
	}
}

// SearchWithTenantInfo search with tenant info
type SearchWithTenantInfo struct {
	*entity.Search
	Nickname     string `json:"nickname"`
	TenantAvatar string `json:"tenant_avatar,omitempty"`
}

// ListSearchAppsRequest list search apps request
type ListSearchAppsRequest struct {
	OwnerIDs []string `json:"owner_ids,omitempty"`
}

// ListSearchAppsResponse list search apps response
type ListSearchAppsResponse struct {
	SearchApps []map[string]interface{} `json:"search_apps"`
	Total      int64                    `json:"total"`
}

type SearchShareDetail struct {
	ID           string                 `json:"id"`
	Avatar       *string                `json:"avatar"`
	TenantID     string                 `json:"tenant_id"`
	Name         string                 `json:"name"`
	Description  *string                `json:"description,omitempty"`
	CreatedBy    string                 `json:"created_by"`
	SearchConfig map[string]interface{} `json:"search_config"`
	UpdateTime   *int64                 `json:"update_time,omitempty"`
}

// ListSearches list search apps with advanced filtering (equivalent to list_search_app)
func (s *SearchService) ListSearches(ctx context.Context, userID string, keywords string, page, pageSize int, terms []dao.OrderTerm, ownerIDs []string) (*ListSearchAppsResponse, error) {
	resourceIDs, err := accessibleSearchAppIDs(ctx, userID, permission.OperationRead)
	if err != nil {
		return nil, err
	}
	searches, total, err := s.searchDAO.ListByResourceIDs(ctx, dao.DB, resourceIDs, ownerIDs, page, pageSize, terms, keywords)
	if err != nil {
		return nil, err
	}

	// Convert to response format
	searchApps := make([]map[string]interface{}, len(searches))
	for i, search := range searches {
		searchApps[i] = s.toSearchAppResponse(search)
	}

	return &ListSearchAppsResponse{
		SearchApps: searchApps,
		Total:      total,
	}, nil
}

// toSearchAppResponse converts search model to response format
func (s *SearchService) toSearchAppResponse(search *entity.SearchListItem) map[string]interface{} {
	result := map[string]interface{}{
		"id":            search.ID,
		"tenant_id":     search.TenantID,
		"name":          search.Name,
		"description":   search.Description,
		"created_by":    search.CreatedBy,
		"status":        search.Status,
		"create_time":   search.CreateTime,
		"update_time":   search.UpdateTime,
		"search_config": BuildSearchConfigResponse(map[string]interface{}(search.SearchConfig)),
		"nickname":      ownerNickname(search.Nickname, search.TenantID),
	}

	if search.Avatar != nil {
		result["avatar"] = *search.Avatar
	}
	if search.TenantAvatar != nil {
		result["tenant_avatar"] = *search.TenantAvatar
	}

	return result
}

// CreateSearchResponse create search response
// Reference: api/apps/restful_apis/search_api.py::create - returns {"search_id": req["id"]}
type CreateSearchResponse struct {
	SearchID string `json:"search_id"` // UUID format
}

// CreateSearch creates a new search app
// Reference: api/apps/restful_apis/search_api.py::create
// Python implementation steps:
// 1. Get JSON request body with name (required) and description (optional)
// 2. Validate name is string, non-empty, and max 255 bytes
// 3. Generate unique name using duplicate_name(SearchService.query, name, tenant_id)
// 4. Generate UUID for search ID
// 5. Set fields: id, name, description, tenant_id, created_by
// 6. Save to database within DB.atomic() transaction
// 7. Return {search_id: id} on success
func (s *SearchService) CreateSearch(ctx context.Context, userID string, name string, description *string) (*CreateSearchResponse, error) {
	if err := common.ValidateName(name); err != nil {
		return nil, err
	}

	// Generate UUID for search ID (same as Python get_uuid())
	searchID := common.GenerateUUID()

	// Generate unique name (same as Python duplicate_name)
	// search.name is a 128-byte column; keep generated names within it.
	uniqueName, err := common.UniqueName(name, 128, func(candidate string) (bool, error) {
		existing, err := s.searchDAO.GetByNameAndTenant(ctx, dao.DB, candidate, userID)
		if err != nil {
			return false, err
		}
		return len(existing) > 0, nil
	})

	if err != nil {
		return nil, err
	}

	// Create search entity
	search := &entity.Search{
		ID:           searchID,
		TenantID:     userID,
		Name:         uniqueName,
		CreatedBy:    userID,
		SearchConfig: make(entity.JSONMap),
	}

	if description != nil {
		search.Description = description
	}

	// Set default status ("1" = valid/active, same as Python StatusEnum.VALID.value)
	status := "1"
	search.Status = &status

	// Save to database
	if err = s.searchDAO.Create(ctx, dao.DB, search); err != nil {
		return nil, fmt.Errorf("failed to create search: %w", err)
	}

	return &CreateSearchResponse{
		SearchID: searchID,
	}, nil
}

func (s *SearchService) GetSearchDetail(ctx context.Context, userID string, searchID string) (*entity.Search, error) {
	if err := checkSearchAppPermission(ctx, userID, searchID, permission.OperationRead); err != nil {
		return nil, err
	}
	search, err := s.searchDAO.GetByID(ctx, dao.DB, searchID)
	if err != nil {
		return nil, fmt.Errorf("can't find this Search App")
	}

	return search, nil
}

// GetSearchShareDetail returns the joined share-detail payload for public
// searchbot pages after verifying the caller can access the search app.
func (s *SearchService) GetSearchShareDetail(ctx context.Context, userID, searchID string) (*SearchShareDetail, error) {
	if _, err := s.GetSearchDetail(ctx, userID, searchID); err != nil {
		return nil, err
	}

	detail, err := s.searchDAO.GetDetailByID(ctx, dao.DB, searchID)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, fmt.Errorf("can't find this Search App")
	}

	return &SearchShareDetail{
		ID:           detail.ID,
		Avatar:       detail.Avatar,
		TenantID:     detail.TenantID,
		Name:         detail.Name,
		Description:  detail.Description,
		CreatedBy:    detail.CreatedBy,
		SearchConfig: BuildSearchConfigResponse(detail.SearchConfig),
		UpdateTime:   detail.UpdateTime,
	}, nil
}

// DeleteSearch deletes a search app by ID
func (s *SearchService) DeleteSearch(ctx context.Context, userID string, searchID string) error {
	if err := checkSearchAppPermission(ctx, userID, searchID, permission.OperationDelete); err != nil {
		return err
	}
	if err := s.searchDAO.DeleteByID(ctx, dao.DB, searchID); err != nil {
		return fmt.Errorf("failed to delete search App %s: %w", searchID, err)
	}

	return nil
}

type SearchCompletionPlan struct {
	UserID     string
	SearchID   string
	Question   string
	DatasetIDs []string
	ModelID    string
	Options    AskStreamOptions
}

func (s *SearchService) PrepareCompletion(ctx context.Context, userID, searchID string, req *SearchCompletionsRequest) (*SearchCompletionPlan, common.ErrorCode, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, common.CodeBadRequest, fmt.Errorf("user id is required")
	}
	searchID = strings.TrimSpace(searchID)
	if searchID == "" {
		return nil, common.CodeBadRequest, fmt.Errorf("search_id is required")
	}
	if req == nil {
		return nil, common.CodeArgumentError, fmt.Errorf("question is required")
	}
	question, err := ResolveCompletionQuestion(req.Question, req.Query, req.Messages)
	if err != nil {
		return nil, common.CodeArgumentError, err
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("question is required")
	}

	searchDetail, err := s.GetDetail(ctx, userID, searchID)
	if err != nil || searchDetail == nil {
		if permissionresponse.IsPermissionError(err) {
			code, permissionErr := permissionresponse.NormalizeHidden(err)
			return nil, code, permissionErr
		}
		return nil, common.CodeDataError, fmt.Errorf("cannot find search %s", searchID)
	}
	searchConfig := searchConfigMapFromValue(searchDetail["search_config"])

	datasetIDs := stringSliceFromSearchConfig(searchConfig["kb_ids"])
	if len(datasetIDs) == 0 {
		datasetIDs = stringSliceFromSearchConfig(req.KBIDs)
	}
	if len(datasetIDs) == 0 {
		return nil, common.CodeDataError, fmt.Errorf("`kb_ids` is required")
	}

	for _, datasetID := range datasetIDs {
		if err := CheckDatasetAccess(ctx, permission.Subject{UserID: userID}, datasetID, permission.OperationUse); err != nil {
			code, permissionErr := permissionresponse.Normalize(err)
			return nil, code, permissionErr
		}
	}

	modelID, _ := stringFromSearchConfig(searchConfig["chat_id"])
	if modelID == "" {
		tenantSvc := s.tenantService
		if tenantSvc == nil {
			tenantSvc = NewTenantService()
		}
		var defaultModelName string
		defaultModelName, err = tenantSvc.GetDefaultModelName(ctx, userID, entity.ModelTypeChat)
		if err == nil {
			modelID = strings.TrimSpace(defaultModelName)
		}
	}

	return &SearchCompletionPlan{
		UserID:     userID,
		SearchID:   searchID,
		Question:   question,
		DatasetIDs: datasetIDs,
		ModelID:    modelID,
		Options:    BuildAskStreamOptions(searchID, searchConfig),
	}, common.CodeSuccess, nil
}

// BuildAskStreamOptions maps a saved search configuration to Ask retrieval options.
func BuildAskStreamOptions(searchID string, searchConfig map[string]interface{}) AskStreamOptions {
	opts := AskStreamOptions{
		SearchID:       searchID,
		DocIDs:         stringSliceFromSearchConfig(searchConfig["doc_ids"]),
		CrossLanguages: stringSliceFromSearchConfig(searchConfig["cross_languages"]),
	}
	if value, ok := searchConfig["use_kg"].(bool); ok {
		opts.UseKG = &value
	}
	if value, ok := intFromSearchConfig(searchConfig["top_k"]); ok {
		opts.TopK = &value
	}
	if value, ok := intFromSearchConfig(searchConfig["rerank_candidates_count"]); ok {
		opts.RerankCandidatesCount = &value
	}
	if value, ok := searchConfigMapValue(searchConfig["meta_data_filter"]); ok {
		opts.Filter = value
	}
	if value, ok := stringFromSearchConfig(searchConfig["tenant_rerank_id"]); ok {
		opts.TenantRerankID = &value
	}
	if value, ok := stringFromSearchConfig(searchConfig["rerank_id"]); ok {
		opts.RerankID = &value
	}
	if value, ok := searchConfig["keyword"].(bool); ok {
		opts.Keyword = &value
	}
	if value, ok := floatFromSearchConfig(searchConfig["similarity_threshold"]); ok {
		opts.SimilarityThreshold = &value
	}
	keywordsSimilarityWeight, _ := similarityWeightFromMap(searchConfig, "keywords_similarity_weight")
	vectorSimilarityWeight, _ := similarityWeightFromMap(searchConfig, "vector_similarity_weight")
	if value, err := ResolveVectorSimilarityWeight(keywordsSimilarityWeight, vectorSimilarityWeight); err == nil && value != nil {
		opts.VectorSimilarityWeight = value
	}
	if llmSetting, ok := searchConfigMapValue(searchConfig["llm_setting"]); ok {
		opts.Temperature = generationFloat(llmSetting, "temperature", DefaultAskTemperature)
		opts.TopP = generationFloat(llmSetting, "top_p", DefaultAskTopP)
	}
	return opts
}

// generationFloat resolves a chat-generation parameter from the llm_setting
// saved in search_config, matching Python's resolve_llm_setting: the
// user-configured value is kept only when the {key}_enabled flag is enabled
// (absent flags count as enabled) and the value is present; otherwise the
// parameter's LLM_SETTING_DEFAULTS fallback is substituted.
func generationFloat(llmSetting map[string]interface{}, key string, fallback float64) *float64 {
	if enabled, ok := llmSetting[key+"_enabled"].(bool); ok && !enabled {
		return &fallback
	}
	if v, ok := floatFromSearchConfig(llmSetting[key]); ok {
		return &v
	}
	return &fallback
}

func searchConfigMapFromValue(value interface{}) map[string]interface{} {
	if result, ok := searchConfigMapValue(value); ok {
		return result
	}
	return map[string]interface{}{}
}

func searchConfigMapValue(value interface{}) (map[string]interface{}, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case map[string]interface{}:
		return typed, true
	case entity.JSONMap:
		return typed, true
	default:
		return nil, false
	}
}

func stringSliceFromSearchConfig(value interface{}) []string {
	switch typed := value.(type) {
	case nil:
		return nil
	case []string:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if item = strings.TrimSpace(item); item != "" {
				result = append(result, item)
			}
		}
		return result
	case common.StringSlice:
		return stringSliceFromSearchConfig([]string(typed))
	case []interface{}:
		result := make([]string, 0, len(typed))
		for _, item := range typed {
			if value, ok := stringFromSearchConfig(item); ok {
				result = append(result, value)
			}
		}
		return result
	default:
		if value, ok := stringFromSearchConfig(value); ok {
			return []string{value}
		}
		return nil
	}
}

func stringFromSearchConfig(value interface{}) (string, bool) {
	typed, ok := value.(string)
	if !ok {
		return "", false
	}
	typed = strings.TrimSpace(typed)
	return typed, typed != ""
}

func intFromSearchConfig(value interface{}) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case float32:
		return int(typed), true
	default:
		return 0, false
	}
}

func floatFromSearchConfig(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	default:
		return 0, false
	}
}

// BuildSearchConfigResponse returns the public search configuration without
// exposing the internally persisted vector weight.
func BuildSearchConfigResponse(searchConfig map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{}, len(searchConfig))
	for key, value := range searchConfig {
		result[key] = value
	}
	if _, exists := result["keywords_similarity_weight"]; !exists {
		if vectorWeight, ok := floatFromSearchConfig(result["vector_similarity_weight"]); ok {
			result["keywords_similarity_weight"] = 1 - vectorWeight
		}
	}
	delete(result, "vector_similarity_weight")
	return result
}

// UpdateSearchRequest update search request
// Reference: api/apps/restful_apis/search_api.py::update
// Required fields: name, search_config
// Optional fields: description
// Immutable fields: search_id, tenant_id, created_by, update_time, id (will be removed)
type UpdateSearchRequest struct {
	Name         string                 `json:"name" binding:"required"`
	Description  *string                `json:"description,omitempty"`
	SearchConfig map[string]interface{} `json:"search_config" binding:"required"`
	Avatar       *string                `json:"avatar,omitempty"`
}

func (s *SearchService) UpdateSearch(ctx context.Context, userID string, searchID string, req *UpdateSearchRequest) (*entity.Search, error) {
	if err := checkSearchAppPermission(ctx, userID, searchID, permission.OperationUpdate); err != nil {
		return nil, err
	}

	search, err := s.searchDAO.GetByID(ctx, dao.DB, searchID)
	if err != nil {
		return nil, fmt.Errorf("cannot find search %s", searchID)
	}

	// Check for duplicate names within the Search App's tenant.
	trimmedName := req.Name
	available, err := common.NameAvailable(search.Name, trimmedName, func(candidate string) (bool, error) {
		existing, err := s.searchDAO.GetByNameAndTenant(ctx, dao.DB, candidate, search.TenantID)
		if err != nil {
			return false, err
		}
		return len(existing) > 0, nil
	})
	if err != nil {
		return nil, err
	}
	if !available {
		return nil, fmt.Errorf("duplicated search name")
	}
	if err := NormalizeSimilarityWeights(req.SearchConfig); err != nil {
		return nil, err
	}

	// Merge the provided search configuration into the stored configuration.
	currentConfig := search.SearchConfig
	if currentConfig == nil {
		currentConfig = make(entity.JSONMap)
	}
	mergedConfig := make(entity.JSONMap)
	// Copy current config
	for k, v := range currentConfig {
		mergedConfig[k] = v
	}
	// Merge new config
	for k, v := range req.SearchConfig {
		mergedConfig[k] = v
	}

	// Keep ownership and identity fields immutable.
	updates := map[string]interface{}{
		"name":          trimmedName,
		"search_config": mergedConfig,
	}

	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Avatar != nil {
		updates["avatar"] = *req.Avatar
	}

	// Persist the update and reload the resource.
	if err = s.searchDAO.UpdateByID(ctx, dao.DB, searchID, updates); err != nil {
		return nil, fmt.Errorf("failed to update search: %w", err)
	}

	updatedSearch, err := s.searchDAO.GetByID(ctx, dao.DB, searchID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated search: %w", err)
	}

	return updatedSearch, nil
}

// GetDetail returns Search App configuration to a caller that is executing it.
func (s *SearchService) GetDetail(ctx context.Context, userID, searchID string) (map[string]interface{}, error) {
	if err := checkSearchAppPermission(ctx, userID, searchID, permission.OperationRun); err != nil {
		return nil, err
	}
	search, err := s.searchDAO.GetByID(ctx, dao.DB, searchID)
	if err != nil {
		return nil, err
	}

	result := map[string]interface{}{
		"id":            search.ID,
		"tenant_id":     search.TenantID,
		"name":          search.Name,
		"description":   search.Description,
		"created_by":    search.CreatedBy,
		"status":        search.Status,
		"create_time":   search.CreateTime,
		"update_time":   search.UpdateTime,
		"search_config": map[string]interface{}(search.SearchConfig),
	}

	if search.Avatar != nil {
		result["avatar"] = *search.Avatar
	}

	return result, nil
}

type SearchCompletionsRequest struct {
	Query    string                   `json:"query,omitempty"`
	Messages []map[string]interface{} `json:"messages,omitempty"`
	Question string                   `json:"question"`
	KBIDs    []string                 `json:"kb_ids,omitempty"`
}
