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
	"ragflow/internal/model"
)

// UserCanvasDAO user canvas data access object
type UserCanvasDAO struct{}

// NewUserCanvasDAO create user canvas DAO
func NewUserCanvasDAO() *UserCanvasDAO {
	return &UserCanvasDAO{}
}

// Create user canvas
func (dao *UserCanvasDAO) Create(userCanvas *model.UserCanvas) error {
	return DB.Create(userCanvas).Error
}

// GetByID get user canvas by ID
func (dao *UserCanvasDAO) GetByID(id string) (*model.UserCanvas, error) {
	var canvas model.UserCanvas
	err := DB.Where("id = ?", id).First(&canvas).Error
	if err != nil {
		return nil, err
	}
	return &canvas, nil
}

// Update update user canvas
func (dao *UserCanvasDAO) Update(userCanvas *model.UserCanvas) error {
	return DB.Save(userCanvas).Error
}

// Delete delete user canvas
func (dao *UserCanvasDAO) Delete(id string) error {
	return DB.Delete(&model.UserCanvas{}, id).Error
}

// GetList get canvases list with pagination and filtering
// Similar to Python UserCanvasService.get_list
func (dao *UserCanvasDAO) GetList(
	tenantID string,
	pageNumber, itemsPerPage int,
	orderby string,
	desc bool,
	id, title string,
	canvasCategory string,
) ([]*model.UserCanvas, error) {

	query := DB.Model(&model.UserCanvas{}).
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

	var canvases []*model.UserCanvas
	err := query.Find(&canvases).Error
	return canvases, err
}

// GetAllCanvasesByTenantIDs get all permitted canvases by tenant IDs
// Similar to Python UserCanvasService.get_all_agents_by_tenant_ids
func (dao *UserCanvasDAO) GetAllCanvasesByTenantIDs(tenantIDs []string, userID string) ([]*CanvasBasicInfo, error) {

	query := DB.Model(&model.UserCanvas{}).
		Select("id, avatar, title, permission, canvas_type, canvas_category").
		Where("user_id IN (?) AND permission = ?", tenantIDs, "team").
		Or("user_id = ?", userID).
		Order("create_time ASC")

	var results []*CanvasBasicInfo
	err := query.Scan(&results).Error
	return results, err
}

// GetByCanvasID get user canvas by canvas ID (alias for GetByID)
func (dao *UserCanvasDAO) GetByCanvasID(canvasID string) (*model.UserCanvas, error) {
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
