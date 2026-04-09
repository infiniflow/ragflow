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
	"fmt"
	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// SearchService search service
type SearchService struct {
	searchDAO     *dao.SearchDAO
	userTenantDAO *dao.UserTenantDAO
}

// NewSearchService create search service
func NewSearchService() *SearchService {
	return &SearchService{
		searchDAO:     dao.NewSearchDAO(),
		userTenantDAO: dao.NewUserTenantDAO(),
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

// ListSearches list search apps with advanced filtering (equivalent to list_search_app)
func (s *SearchService) ListSearches(userID string, keywords string, page, pageSize int, orderby string, desc bool, ownerIDs []string) (*ListSearchAppsResponse, error) {
	var searches []*entity.Search
	var total int64
	var err error

	if len(ownerIDs) == 0 {
		// Get tenant IDs by user ID (joined tenants)
		tenantIDs, err := s.userTenantDAO.GetTenantIDsByUserID(userID)
		if err != nil {
			return nil, err
		}

		// Use database pagination
		searches, total, err = s.searchDAO.ListByTenantIDs(tenantIDs, userID, page, pageSize, orderby, desc, keywords)
		if err != nil {
			return nil, err
		}
	} else {
		// Filter by owner IDs, manual pagination
		searches, total, err = s.searchDAO.ListByOwnerIDs(ownerIDs, userID, orderby, desc, keywords)
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
		"search_config": search.SearchConfig,
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
//
// Error handling from Python:
// - Name not string: "Search name must be string."
// - Name empty: "Search name can't be empty."
// - Name too long: "Search name length is X which is large than 255."
// - Tenant not found: "Authorized identity."
// - Save failure: generic get_data_error_result()
//
// Note: Go implementation validates these in handler layer for cleaner separation
// Note: Similar pattern in: CreateMemory (memory.go), CreateDataset (datasets.go)
func (s *SearchService) CreateSearch(userID string, name string, description *string) (*CreateSearchResponse, error) {
	// Generate UUID for search ID (same as Python get_uuid())
	searchID := common.GenerateUUID()

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
