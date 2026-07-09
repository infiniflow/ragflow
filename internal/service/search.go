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
	"errors"
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"
	"strings"

	"gorm.io/gorm"
)

// SearchService search service
type SearchService struct {
	searchDAO     *dao.SearchDAO
	userTenantDAO *dao.UserTenantDAO
	tenantService *TenantService
}

// NewSearchService create search service
func NewSearchService() *SearchService {
	return &SearchService{
		searchDAO:     dao.NewSearchDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
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
func (s *SearchService) ListSearches(userID string, keywords string, page, pageSize int, orderby string, desc bool, ownerIDs []string) (*ListSearchAppsResponse, error) {
	var searches []*entity.Search
	var total int64
	var err error

	if len(ownerIDs) == 0 {
		searches, total, err = s.searchDAO.ListByTenantIDs(nil, userID, page, pageSize, orderby, desc, keywords)
		if err != nil {
			return nil, err
		}
	} else {
		ownerIDs, err = s.filterAccessibleSearchOwnerIDs(userID, ownerIDs)
		if err != nil {
			return nil, err
		}
		if len(ownerIDs) == 0 {
			return &ListSearchAppsResponse{
				SearchApps: []map[string]interface{}{},
				Total:      0,
			}, nil
		}

		searches, total, err = s.searchDAO.ListByOwnerIDs(ownerIDs, userID, orderby, desc, keywords)
		if err != nil {
			return nil, err
		}

		if page > 0 && pageSize > 0 {
			start := (page - 1) * pageSize
			end := start + pageSize
			if start < int(total) {
				if end > int(total) {
					end = int(total)
				}
				searches = searches[start:end]
			} else {
				searches = []*entity.Search{}
			}
		}
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

func (s *SearchService) filterAccessibleSearchOwnerIDs(userID string, ownerIDs []string) ([]string, error) {
	tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
	if err != nil {
		return nil, err
	}

	allowed := map[string]struct{}{userID: {}}
	for _, tenantID := range tenantIDs {
		tenantID = strings.TrimSpace(tenantID)
		if tenantID != "" {
			allowed[tenantID] = struct{}{}
		}
	}

	filtered := make([]string, 0, len(ownerIDs))
	seen := make(map[string]struct{}, len(ownerIDs))
	for _, ownerID := range ownerIDs {
		ownerID = strings.TrimSpace(ownerID)
		if ownerID == "" {
			continue
		}
		if _, ok := allowed[ownerID]; !ok {
			continue
		}
		if _, ok := seen[ownerID]; ok {
			continue
		}
		seen[ownerID] = struct{}{}
		filtered = append(filtered, ownerID)
	}
	return filtered, nil
}

// toSearchAppResponse converts search model to response format
func (s *SearchService) toSearchAppResponse(search *entity.Search) map[string]interface{} {
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

	// Add joined fields from user table
	// Note: These fields are populated by the DAO query with Select clause
	// but GORM will map them to the model's embedded fields if available
	// We need to handle the extra fields manually

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
func (s *SearchService) CreateSearch(userID string, name string, description *string) (*CreateSearchResponse, error) {
	// Generate UUID for search ID (same as Python get_uuid())
	searchID := utility.GenerateUUID()

	// Generate unique name (same as Python duplicate_name)
	uniqueName, err := common.DuplicateName(func(name string, tid string) bool {
		existing, _ := s.searchDAO.GetByNameAndTenant(name, tid)
		return len(existing) > 0
	}, name, userID)

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
	if err := s.searchDAO.Create(search); err != nil {
		return nil, fmt.Errorf("failed to create search: %w", err)
	}

	return &CreateSearchResponse{
		SearchID: searchID,
	}, nil
}

func (s *SearchService) GetSearchDetail(userID string, searchID string) (*entity.Search, error) {
	// Step 1: Get user tenants (same as Python UserTenantService.query(user_id=current_user.id))
	tenants, err := s.userTenantDAO.GetByUserID(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user tenants: %w", err)
	}

	// Step 2: Check if user has permission to access this search
	// Python: for tenant in tenants: if SearchService.query(tenant_id=tenant.tenant_id, id=search_id): break
	hasPermission := false
	for _, tenant := range tenants {
		searches, err := s.searchDAO.QueryByTenantIDAndID(tenant.TenantID, searchID)
		if err != nil {
			continue // Try next tenant
		}
		if len(searches) > 0 {
			hasPermission = true
			break
		}
	}

	if !hasPermission {
		return nil, fmt.Errorf("has no permission for this operation")
	}

	// Step 3: Get search detail (same as Python SearchService.get_detail(search_id))
	search, err := s.searchDAO.GetByID(searchID)
	if err != nil {
		return nil, fmt.Errorf("can't find this Search App!")
	}

	return search, nil
}

// GetSearchShareDetail returns the joined share-detail payload for public
// searchbot pages after verifying the caller can access the search app.
func (s *SearchService) GetSearchShareDetail(userID, searchID string) (*SearchShareDetail, error) {
	if _, err := s.GetSearchDetail(userID, searchID); err != nil {
		return nil, err
	}

	detail, err := s.searchDAO.GetDetailByID(searchID)
	if err != nil {
		return nil, err
	}
	if detail == nil {
		return nil, fmt.Errorf("can't find this Search App!")
	}

	return &SearchShareDetail{
		ID:           detail.ID,
		Avatar:       detail.Avatar,
		TenantID:     detail.TenantID,
		Name:         detail.Name,
		Description:  detail.Description,
		CreatedBy:    detail.CreatedBy,
		SearchConfig: map[string]interface{}(detail.SearchConfig),
		UpdateTime:   detail.UpdateTime,
	}, nil
}

// DeleteSearch deletes a search app by ID
func (s *SearchService) DeleteSearch(userID string, searchID string) error {
	// Step 1: Check deletion permission (same as Python SearchService.accessible4deletion)
	// Python: cls.model.select().where(cls.model.id == search_id, cls.model.created_by == user_id, cls.model.status == StatusEnum.VALID.value).first()

	status, err := s.searchDAO.Accessible4Deletion(searchID, userID)
	if err != nil {
		return fmt.Errorf("failed to check deletion permission: %w", err)
	}

	if !status {
		return fmt.Errorf("no authorization")
	}

	// Step 2: Execute delete (same as Python SearchService.delete_by_id)
	// Python: cls.model.delete().where(cls.model.id == pid).execute()
	if err = s.searchDAO.DeleteByID(searchID); err != nil {
		return fmt.Errorf("failed to delete search App %s: %w", searchID, err)
	}

	return nil
}

// AccessibleForCompletion check if it is accessible
func (s *SearchService) AccessibleForCompletion(userID string, searchID string) (bool, error) {
	ok, err := s.searchDAO.Accessible4Deletion(searchID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return ok, nil
}

type SearchCompletionPlan struct {
	UserID   string
	SearchID string
	Question string
	KBIDs    []string
	ModelID  string
	Options  AskStreamOptions
}

func (s *SearchService) PrepareCompletion(userID, searchID string, req *SearchCompletionsRequest) (*SearchCompletionPlan, common.ErrorCode, error) {
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
	question := strings.TrimSpace(req.Question)
	if question == "" {
		return nil, common.CodeArgumentError, fmt.Errorf("question is required")
	}

	accessible, err := s.AccessibleForCompletion(userID, searchID)
	if err != nil {
		return nil, common.CodeServerError, err
	}
	if !accessible {
		return nil, common.CodeAuthenticationError, fmt.Errorf("No authorization.")
	}

	searchDetail, err := s.GetDetail(searchID)
	if err != nil || searchDetail == nil {
		return nil, common.CodeDataError, fmt.Errorf("Cannot find search %s", searchID)
	}
	searchConfig := searchConfigMapFromValue(searchDetail["search_config"])

	kbIDs := stringSliceFromSearchConfig(searchConfig["kb_ids"])
	if len(kbIDs) == 0 {
		kbIDs = stringSliceFromSearchConfig(req.KBIDs)
	}
	if len(kbIDs) == 0 {
		return nil, common.CodeDataError, fmt.Errorf("`kb_ids` is required.")
	}

	modelID, _ := stringFromSearchConfig(searchConfig["chat_id"])
	if modelID == "" {
		tenantSvc := s.tenantService
		if tenantSvc == nil {
			tenantSvc = NewTenantService()
		}
		defaultModel, err := tenantSvc.GetDefaultModelName(userID, entity.ModelTypeChat)
		if err == nil {
			modelID = strings.TrimSpace(defaultModel)
		}
	}

	return &SearchCompletionPlan{
		UserID:   userID,
		SearchID: searchID,
		Question: question,
		KBIDs:    kbIDs,
		ModelID:  modelID,
		Options:  askOptionsFromSearchConfig(searchID, searchConfig),
	}, common.CodeSuccess, nil
}

func askOptionsFromSearchConfig(searchID string, searchConfig map[string]interface{}) AskStreamOptions {
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
	if value, ok := floatFromSearchConfig(searchConfig["vector_similarity_weight"]); ok {
		opts.VectorSimilarityWeight = &value
	}
	return opts
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
		return map[string]interface{}(typed), true
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

func (s *SearchService) UpdateSearch(userID string, searchID string, req *UpdateSearchRequest) (*entity.Search, error) {
	// Step 1: Check update permission (same as delete - uses accessible4deletion)
	// Only creator can update

	status, err := s.searchDAO.Accessible4Deletion(searchID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to check deletion permission: %w", err)
	}

	if !status {
		return nil, fmt.Errorf("no authorization")
	}

	// Step 2: Get existing search
	// Python: search_app = SearchService.query(tenant_id=current_user.id, id=search_id)[0]
	search, err := s.searchDAO.GetByTenantIDAndID(userID, searchID)
	if err != nil {
		return nil, fmt.Errorf("cannot find search %s", searchID)
	}

	// Step 3: Check for duplicate name (if name changed)
	// Python: if req["name"].lower() != search_app.name.lower() and len(SearchService.query(...)) >= 1
	trimmedName := req.Name
	if search.Name != trimmedName {
		existing, _ := s.searchDAO.GetByNameAndTenant(trimmedName, userID)
		if len(existing) > 0 {
			return nil, fmt.Errorf("duplicated search name")
		}
	}

	// Step 4: Merge search_config
	// Python: req["search_config"] = {**current_config, **new_config}
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

	// Step 5: Prepare updates (excluding immutable fields)
	// Python removes: search_id, tenant_id, created_by, update_time, id
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

	// Step 6: Execute update
	// Python: SearchService.update_by_id(search_id, req)
	if err = s.searchDAO.UpdateByID(searchID, updates); err != nil {
		return nil, fmt.Errorf("failed to update search: %w", err)
	}

	// Step 7: Fetch updated search
	// Python: e, updated_search = SearchService.get_by_id(search_id)
	updatedSearch, err := s.searchDAO.GetByID(searchID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated search: %w", err)
	}

	return updatedSearch, nil
}

// GetDetail gets search details by ID including search_config
func (s *SearchService) GetDetail(searchID string) (map[string]interface{}, error) {
	search, err := s.searchDAO.GetByID(searchID)

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
	Question string   `json:"question" binding:"required"`
	KBIDs    []string `json:"kb_ids,omitempty"`
}
