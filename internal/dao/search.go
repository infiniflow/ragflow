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
	"context"
	"strings"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// SearchDAO search data access object
type SearchDAO struct{}

// NewSearchDAO create search DAO
func NewSearchDAO() *SearchDAO {
	return &SearchDAO{}
}

// SearchDetailRow represents the joined detail payload used by the
// share-detail endpoint.
type SearchDetailRow struct {
	ID           string         `gorm:"column:id"`
	Avatar       *string        `gorm:"column:avatar"`
	TenantID     string         `gorm:"column:tenant_id"`
	Name         string         `gorm:"column:name"`
	Description  *string        `gorm:"column:description"`
	CreatedBy    string         `gorm:"column:created_by"`
	SearchConfig entity.JSONMap `gorm:"column:search_config"`
	UpdateTime   *int64         `gorm:"column:update_time"`
	Nickname     *string        `gorm:"column:nickname"`
	TenantAvatar *string        `gorm:"column:tenant_avatar"`
}

// ListByResourceIDs lists only the authorized Search Apps, with optional
// tenant-owner filtering and pagination.
func (dao *SearchDAO) ListByResourceIDs(ctx context.Context, db *gorm.DB, resourceIDs, ownerIDs []string, page, pageSize int, terms []OrderTerm, keywords string) ([]*entity.SearchListItem, int64, error) {
	var searches []*entity.SearchListItem
	var total int64
	if len(resourceIDs) == 0 {
		return []*entity.SearchListItem{}, 0, nil
	}

	// Build query with join to user table for nickname and avatar
	query := db.WithContext(ctx).Model(&entity.Search{}).
		Select(`
			search.*,
			user.nickname,
			user.avatar as tenant_avatar
		`).
		Joins("LEFT JOIN user ON search.tenant_id = user.id")

	query = query.Where("search.id IN ? AND search.status = ?", resourceIDs, "1")
	if len(ownerIDs) > 0 {
		query = query.Where("search.tenant_id IN ?", ownerIDs)
	}

	// Apply keyword filter
	if keywords != "" {
		query = query.Where("LOWER(search.name) LIKE ?", "%"+strings.ToLower(keywords)+"%")
	}

	// Apply ordering. Route the requested terms through searchOrderClause so a
	// user-supplied query param can never reach Order() verbatim: the helper
	// validates against searchOrderableColumns (a closed allowlist) and falls
	// back to "create_time" on a miss.
	// codeql[go/sql-injection] False positive: searchOrderClause
	query = query.Order(searchOrderClause(terms))

	// Count total
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination
	if page > 0 && pageSize > 0 {
		offset := (page - 1) * pageSize
		if err := query.Offset(offset).Limit(pageSize).Scan(&searches).Error; err != nil {
			return nil, 0, err
		}
	} else {
		if err := query.Scan(&searches).Error; err != nil {
			return nil, 0, err
		}
	}

	return searches, total, nil
}

// GetByID gets search by ID
func (dao *SearchDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.Search, error) {
	var search entity.Search
	err := db.WithContext(ctx).Where("id = ?", id).First(&search).Error
	if err != nil {
		return nil, err
	}
	return &search, nil
}

// GetDetailByID retrieves the share-detail payload by joining the search app
// with its owner profile, matching Python SearchService.get_detail.
func (dao *SearchDAO) GetDetailByID(ctx context.Context, db *gorm.DB, searchID string) (*SearchDetailRow, error) {
	var detail SearchDetailRow
	err := db.WithContext(ctx).Table("search").
		Select(`
			search.id,
			search.avatar,
			search.tenant_id,
			search.name,
			search.description,
			search.created_by,
			search.search_config,
			search.update_time,
			user.nickname,
			user.avatar AS tenant_avatar
		`).
		Joins("JOIN user ON user.id = search.tenant_id AND user.status = ?", "1").
		Where("search.id = ? AND search.status = ?", searchID, "1").
		Scan(&detail).Error
	if err != nil {
		return nil, err
	}
	if detail.ID == "" {
		return nil, nil
	}
	return &detail, nil
}

// GetByNameAndTenant gets search by name and tenant ID
func (dao *SearchDAO) GetByNameAndTenant(ctx context.Context, db *gorm.DB, name string, tenantID string) ([]*entity.Search, error) {
	var searches []*entity.Search
	err := db.WithContext(ctx).Where("LOWER(name) = LOWER(?) AND tenant_id = ? AND status = ?", name, tenantID, "1").Find(&searches).Error
	return searches, err
}

// Create creates a new search
func (dao *SearchDAO) Create(ctx context.Context, db *gorm.DB, search *entity.Search) error {
	return db.WithContext(ctx).Create(search).Error
}

// DeleteByID deletes a search by ID (soft delete by setting status to "0")
func (dao *SearchDAO) DeleteByID(ctx context.Context, db *gorm.DB, id string) error {
	return db.WithContext(ctx).Model(&entity.Search{}).Where("id = ? AND status = ?", id, "1").Update("status", "0").Error
}

// UpdateByID updates search by ID
// Reference: Python common_service.py::update_by_id
func (dao *SearchDAO) UpdateByID(ctx context.Context, db *gorm.DB, id string, updates map[string]interface{}) error {
	return db.WithContext(ctx).Model(&entity.Search{}).Where("id = ?", id).Updates(updates).Error
}
