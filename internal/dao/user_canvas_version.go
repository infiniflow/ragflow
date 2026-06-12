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
	"errors"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// ErrUserCanvasVersionNotFound is returned when a version lookup by id or
// canvas id yields no rows. Service and handler layers map this to a 404.
var ErrUserCanvasVersionNotFound = errors.New("user_canvas_version: not found")

// UserCanvasVersionDAO persists and queries UserCanvasVersion rows.
//
// One UserCanvasVersion row is created on every agent publish (§2.9); rows
// are append-only and never updated. Cascade delete of a parent canvas
// removes all child versions via DeleteByCanvasID.
type UserCanvasVersionDAO struct{}

// NewUserCanvasVersionDAO returns a zero-value DAO. The struct is stateless
// so callers can share a single instance or create their own.
func NewUserCanvasVersionDAO() *UserCanvasVersionDAO {
	return &UserCanvasVersionDAO{}
}

// Create inserts a new version row. The caller assigns ID, UserCanvasID,
// Title, Description, DSL. CreateTime/UpdateTime are stamped by the
// BaseModel BeforeCreate hook.
func (dao *UserCanvasVersionDAO) Create(v *entity.UserCanvasVersion) error {
	return DB.Create(v).Error
}

// GetByID fetches a single version by primary key. Returns
// ErrUserCanvasVersionNotFound when the row is absent so callers can map
// to a 404 instead of inspecting gorm.ErrRecordNotFound directly.
func (dao *UserCanvasVersionDAO) GetByID(id string) (*entity.UserCanvasVersion, error) {
	var v entity.UserCanvasVersion
	err := DB.Where("id = ?", id).First(&v).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserCanvasVersionNotFound
		}
		return nil, err
	}
	return &v, nil
}

// ListByCanvasID returns every version of the given canvas, ordered by
// create_time DESC so the most recent publish appears first.
func (dao *UserCanvasVersionDAO) ListByCanvasID(canvasID string) ([]*entity.UserCanvasVersion, error) {
	var vs []*entity.UserCanvasVersion
	err := DB.Where("user_canvas_id = ?", canvasID).
		Order("create_time DESC").
		Find(&vs).Error
	return vs, err
}

// GetLatest returns the most recently created version of canvasID, or
// ErrUserCanvasVersionNotFound when the canvas has never been published.
func (dao *UserCanvasVersionDAO) GetLatest(canvasID string) (*entity.UserCanvasVersion, error) {
	var v entity.UserCanvasVersion
	err := DB.Where("user_canvas_id = ?", canvasID).
		Order("create_time DESC").
		First(&v).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserCanvasVersionNotFound
		}
		return nil, err
	}
	return &v, nil
}

// Delete removes a single version by id. No-op when the row is absent.
func (dao *UserCanvasVersionDAO) Delete(id string) error {
	return DB.Where("id = ?", id).Delete(&entity.UserCanvasVersion{}).Error
}

// DeleteTx is the transactional variant of Delete. Used by
// service.AgentService.DeleteVersion so the version-row removal and the
// (future) parent-canvas stat update land in one atomic write.
func (dao *UserCanvasVersionDAO) DeleteTx(tx *gorm.DB, id string) error {
	return tx.Where("id = ?", id).Delete(&entity.UserCanvasVersion{}).Error
}

// DeleteByCanvasID removes every version of the given canvas. Called from
// the service layer when the parent canvas is deleted to enforce the
// §2.9 cascade rule. Returns the number of rows actually deleted.
func (dao *UserCanvasVersionDAO) DeleteByCanvasID(canvasID string) (int64, error) {
	res := DB.Where("user_canvas_id = ?", canvasID).Delete(&entity.UserCanvasVersion{})
	return res.RowsAffected, res.Error
}

// DeleteByCanvasIDTx is the transactional variant of DeleteByCanvasID.
// Used by service.AgentService.DeleteAgent so the cascade runs atomically
// with the parent canvas row removal.
func (dao *UserCanvasVersionDAO) DeleteByCanvasIDTx(tx *gorm.DB, canvasID string) (int64, error) {
	res := tx.Where("user_canvas_id = ?", canvasID).Delete(&entity.UserCanvasVersion{})
	return res.RowsAffected, res.Error
}

// CreateTx is the transactional variant of Create. Used by
// service.AgentService.PublishAgent so the new version row and the
// parent canvas update land in one atomic write.
func (dao *UserCanvasVersionDAO) CreateTx(tx *gorm.DB, v *entity.UserCanvasVersion) error {
	return tx.Create(v).Error
}
