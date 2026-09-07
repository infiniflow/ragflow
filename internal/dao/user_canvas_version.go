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

import "ragflow/internal/entity"

// UserCanvasVersionDAO user canvas version data access object
type UserCanvasVersionDAO struct{}

// NewUserCanvasVersionDAO create user canvas version DAO
func NewUserCanvasVersionDAO() *UserCanvasVersionDAO {
	return &UserCanvasVersionDAO{}
}

// ListByCanvasID returns all versions for a canvas, ordered by update_time DESC.
func (d *UserCanvasVersionDAO) ListByCanvasID(canvasID string) ([]*entity.UserCanvasVersion, error) {
	var versions []*entity.UserCanvasVersion
	err := DB.Select("id", "user_canvas_id", "title", "create_time", "update_time", "create_date", "update_date").
		Where("user_canvas_id = ?", canvasID).
		Order("update_time DESC").
		Find(&versions).Error
	return versions, err
}

// GetByID returns a version by its ID.
func (d *UserCanvasVersionDAO) GetByID(versionID string) (*entity.UserCanvasVersion, error) {
	var version entity.UserCanvasVersion
	err := DB.Where("id = ?", versionID).First(&version).Error
	if err != nil {
		return nil, err
	}
	return &version, nil
}