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
)

// UserCanvasDAO user canvas data access object
type UserCanvasDAO struct{}

// NewUserCanvasDAO create user canvas DAO
func NewUserCanvasDAO() *UserCanvasDAO {
	return &UserCanvasDAO{}
}

// Create user canvas
func (dao *UserCanvasDAO) Create(userCanvas *entity.UserCanvas) error {
	return DB.Create(userCanvas).Error
}

// GetByID get user canvas by ID
func (dao *UserCanvasDAO) GetByID(id string) (*entity.UserCanvas, error) {
	var canvas entity.UserCanvas
	err := DB.Where("id = ?", id).First(&canvas).Error
	if err != nil {
		return nil, err
	}
	return &canvas, nil
}

// Update update user canvas
func (dao *UserCanvasDAO) Update(userCanvas *entity.UserCanvas) error {
	return DB.Save(userCanvas).Error
}

// Delete delete user canvas
func (dao *UserCanvasDAO) Delete(id string) error {
	return DB.Delete(&entity.UserCanvas{}, id).Error
}

// GetList get canvases list with pagination and filtering
// Similar to Python UserCanvasService.get_list
func (dao *UserCanvasDAO) GetList(tenantID string, pageNumber, itemsPerPage int, orderby string, desc bool, id, title string, canvasCategory string) ([]*entity.UserCanvas, error) {

	query := DB.Model(&entity.UserCanvas{}).
		Where("user_id = ?", tenantID)

	if id != "" {
		query = query.Where("id = ?", id)
	}
	if title != "" {
		query = query.Where("title = ?", title)
	}
	if canvasCategory != "" {
		query = query.Where("canvas_category = ?", canvasCategory)
	} else {
		// Default to agent category
		query = query.Where("canvas_category = ?", "agent_canvas")
	}

	// Order by
	if desc {
		query = query.Order(orderby + " DESC")
	} else {
		query = query.Order(orderby + " ASC")
	}

	// Pagination
	if pageNumber > 0 && itemsPerPage > 0 {
		offset := (pageNumber - 1) * itemsPerPage
		query = query.Offset(offset).Limit(itemsPerPage)
	}

	var canvases []*entity.UserCanvas
	err := query.Find(&canvases).Error
	return canvases, err
}

// GetAllCanvasesByTenantIDs get all permitted canvases by tenant IDs
// Similar to Python UserCanvasService.get_all_agents_by_tenant_ids
func (dao *UserCanvasDAO) GetAllCanvasesByTenantIDs(tenantIDs []string, userID string) ([]*CanvasBasicInfo, error) {

	query := DB.Model(&entity.UserCanvas{}).
		Select("id, avatar, title, permission, canvas_type, canvas_category").
		Where("user_id IN (?) AND permission = ?", tenantIDs, "team").
		Or("user_id = ?", userID).
		Order("create_time ASC")

	var results []*CanvasBasicInfo
	err := query.Scan(&results).Error
	return results, err
}

// ListByTenantIDs lists agent canvases accessible to the given owner IDs with optional
// keyword filter, pagination, and ordering.
// Mirrors Python UserCanvasService.get_by_tenant_ids (list route only).
func (dao *UserCanvasDAO) ListByTenantIDs(ownerIDs []string, userID string, page, pageSize int, orderby string, desc bool, keywords string, canvasCategory string) ([]*entity.UserCanvas, int64, error) {
	if len(ownerIDs) == 0 {
		return nil, 0, nil
	}

	// Canvases owned by any of the ownerIDs that are "team"-permission, plus all owned by userID.
	base := DB.Model(&entity.UserCanvas{}).
		Where(
			DB.Where("user_id IN ? AND permission = ?", ownerIDs, "team").
				Or("user_id = ?", userID),
		)

	if canvasCategory != "" {
		base = base.Where("canvas_category = ?", canvasCategory)
	} else {
		base = base.Where("canvas_category = ?", "agent_canvas")
	}

	if keywords != "" {
		like := "%" + keywords + "%"
		base = base.Where("title LIKE ?", like)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := orderby
	if desc {
		order += " DESC"
	} else {
		order += " ASC"
	}
	query := base.Order(order)

	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	var canvases []*entity.UserCanvas
	if err := query.Find(&canvases).Error; err != nil {
		return nil, 0, err
	}
	return canvases, total, nil
}

// GetByCanvasID get user canvas by canvas ID (alias for GetByID)
func (dao *UserCanvasDAO) GetByCanvasID(canvasID string) (*entity.UserCanvas, error) {
	return dao.GetByID(canvasID)
}

// CanvasBasicInfo basic canvas information for list responses
type CanvasBasicInfo struct {
	ID             string  `gorm:"column:id" json:"id"`
	Avatar         *string `gorm:"column:avatar" json:"avatar,omitempty"`
	Title          *string `gorm:"column:title" json:"title,omitempty"`
	Permission     string  `gorm:"column:permission" json:"permission"`
	CanvasType     *string `gorm:"column:canvas_type" json:"canvas_type,omitempty"`
	CanvasCategory string  `gorm:"column:canvas_category" json:"canvas_category"`
}

// DeleteByUserID deletes all canvases by user ID (hard delete)
func (dao *UserCanvasDAO) DeleteByUserID(userID string) (int64, error) {
	result := DB.Unscoped().Where("user_id = ?", userID).Delete(&entity.UserCanvas{})
	return result.RowsAffected, result.Error
}

// GetAllCanvasIDsByUserID gets all canvas IDs by user ID
func (dao *UserCanvasDAO) GetAllCanvasIDsByUserID(userID string) ([]string, error) {
	var canvasIDs []string
	err := DB.Model(&entity.UserCanvas{}).
		Where("user_id = ?", userID).
		Pluck("id", &canvasIDs).Error
	return canvasIDs, err
}

// UpdateDSL updates a canvas DSL by canvas ID.
func (dao *UserCanvasDAO) UpdateDSL(canvasID string, dsl entity.JSONMap) (int64, error) {
	result := DB.Model(&entity.UserCanvas{}).Where("id = ?", canvasID).Update("dsl", dsl)
	return result.RowsAffected, result.Error
}

// UpdateTags updates a canvas's comma-separated tags by canvas ID.
func (dao *UserCanvasDAO) UpdateTags(canvasID, tags string) (int64, error) {
	result := DB.Model(&entity.UserCanvas{}).Where("id = ?", canvasID).Update("tags", tags)
	return result.RowsAffected, result.Error
}
