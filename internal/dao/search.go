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

package dao

import (
	"ragflow/internal/entity"
	"strings"
)

// SearchDAO search data access object
type SearchDAO struct{}

// NewSearchDAO create search DAO
func NewSearchDAO() *SearchDAO {
	return &SearchDAO{}
}

// ListByTenantIDs list searches by tenant IDs with pagination and filtering
func (dao *SearchDAO) ListByTenantIDs(tenantIDs []string, userID string, page, pageSize int, orderby string, desc bool, keywords string) ([]*entity.Search, int64, error) {
	var searches []*entity.Search
	var total int64

	// Build query with join to user table for nickname and avatar
	query := DB.Model(&entity.Search{}).
		Select(`
			search.*,
			user.nickname,
			user.avatar as tenant_avatar
		`).
		Joins("LEFT JOIN user ON search.tenant_id = user.id").
		Where("(search.tenant_id IN ? OR search.tenant_id = ?) AND search.status = ?", tenantIDs, userID, "1")

	// Apply keyword filter
	if keywords != "" {
		query = query.Where("LOWER(search.name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	// Apply ordering (allowlist only; qualify columns for user JOIN)
	orderDirection := "ASC"
	if desc {
		orderDirection = "DESC"
	}
	orderCol := sanitizeSearchListOrderBy(orderby)
	query = query.Order(orderCol + " " + orderDirection)

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		if err := query.Offset(offset).Limit(pageSize).Find(&searches).Error; err != nil {
			return nil, 0, err
		}
	} else {
		if err := query.Find(&searches).Error; err != nil {
			return nil, 0, err
		}
	}

	return searches, total, nil
}

// ListByOwnerIDs list searches by owner IDs with filtering (manual pagination)
func (dao *SearchDAO) ListByOwnerIDs(ownerIDs []string, userID string, orderby string, desc bool, keywords string) ([]*entity.Search, int64, error) {
	var searches []*entity.Search

	// Build query with join to user table
	query := DB.Model(&entity.Search{}).
		Select(`
			search.*,
			user.nickname,
			user.avatar as tenant_avatar
		`).
		Joins("LEFT JOIN user ON search.tenant_id = user.id").
		Where("(search.tenant_id IN ? OR search.tenant_id = ?) AND search.status = ?", ownerIDs, userID, "1")

	// Apply keyword filter
	if keywords != "" {
		query = query.Where("LOWER(search.name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	// Filter by owner IDs (additional filter to ensure tenant_id is in ownerIDs)
	query = query.Where("search.tenant_id IN ?", ownerIDs)

	// Apply ordering (allowlist only; qualify columns for user JOIN)
	orderDirection := "ASC"
	if desc {
		orderDirection = "DESC"
	}
	orderCol := sanitizeSearchListOrderBy(orderby)
	query = query.Order(orderCol + " " + orderDirection)

	// Get all matching records
	if err := query.Find(&searches).Error; err != nil {
		return nil, 0, err
	}

	total := int64(len(searches))

	return searches, total, nil
}

// GetByID gets search by ID
func (dao *SearchDAO) GetByID(id string) (*entity.Search, error) {
	var search entity.Search
	err := DB.Where("id = ?", id).First(&search).Error
	if err != nil {
		return nil, err
	}
	return &search, nil
}

// GetByNameAndTenant gets search by name and tenant ID
func (dao *SearchDAO) GetByNameAndTenant(name string, tenantID string) ([]*entity.Search, error) {
	var searches []*entity.Search
	err := DB.Where("name = ? AND tenant_id = ? AND status = ?", name, tenantID, "1").Find(&searches).Error
	return searches, err
}

// Create creates a new search
func (dao *SearchDAO) Create(search *entity.Search) error {
	return DB.Create(search).Error
}

// QueryByTenantIDAndID checks if a search exists with given tenant_id and id
// Reference: Python SearchService.query(tenant_id=tenant.tenant_id, id=search_id)
// Used for permission verification in detail API
func (dao *SearchDAO) QueryByTenantIDAndID(tenantID string, searchID string) ([]*entity.Search, error) {
	var searches []*entity.Search
	err := DB.Where("tenant_id = ? AND id = ? AND status = ?", tenantID, searchID, "1").Find(&searches).Error
	return searches, err
}

// DeleteByID deletes a search by ID (soft delete by setting status to "0")
// Reference: Python common_service.py::delete_by_id
func (dao *SearchDAO) DeleteByID(id string) error {
	return DB.Model(&entity.Search{}).Where("id = ?", id).Update("status", "0").Error
}

// Accessible4Deletion checks if a search can be deleted by a specific user
// Reference: Python search_service.py::accessible4deletion
// Returns true if the search exists, is valid, and was created by the user
func (dao *SearchDAO) Accessible4Deletion(searchID string, userID string) (bool, error) {
	var search entity.Search
	err := DB.Where("id = ? AND created_by = ? AND status = ?", searchID, userID, "1").First(&search).Error
	return err == nil, err
}

// GetByTenantIDAndID gets search by tenant ID and search ID
// Reference: Python SearchService.query(tenant_id=tenant_id, id=search_id)
func (dao *SearchDAO) GetByTenantIDAndID(tenantID string, searchID string) (*entity.Search, error) {
	var search entity.Search
	err := DB.Where("tenant_id = ? AND id = ? AND status = ?", tenantID, searchID, "1").First(&search).Error
	if err != nil {
		return nil, err
	}
	return &search, nil
}

// UpdateByID updates search by ID
// Reference: Python common_service.py::update_by_id
func (dao *SearchDAO) UpdateByID(id string, updates map[string]interface{}) error {
	return DB.Model(&entity.Search{}).Where("id = ?", id).Updates(updates).Error
}
