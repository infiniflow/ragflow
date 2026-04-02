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

// ListSearchApps list search apps with advanced filtering (equivalent to list_search_app)
func (s *SearchService) ListSearchApps(userID string, keywords string, page, pageSize int, orderby string, desc bool, ownerIDs []string) (*ListSearchAppsResponse, error) {
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
