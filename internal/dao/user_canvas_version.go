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
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ragflow/internal/entity"
)

// ErrUserCanvasVersionNotFound is returned when a version lookup by id or
// canvas id yields no rows. Service and handler layers map this to a 404.
var ErrUserCanvasVersionNotFound = errors.New("user_canvas_version: not found")

// UserCanvasVersionDAO persists and queries UserCanvasVersion rows.
type UserCanvasVersionDAO struct{}

// SaveOrReplaceLatestVersionOptions controls a version-history save.
type SaveOrReplaceLatestVersionOptions struct {
	NewID           string
	UserCanvasID    string
	Title           *string
	Description     *string
	DSL             entity.JSONMap
	Release         bool
	KeepUnpublished int
	SameDSL         func(entity.JSONMap) bool
}

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

// CreateTx is the transactional variant of Create.
func (dao *UserCanvasVersionDAO) CreateTx(tx *gorm.DB, v *entity.UserCanvasVersion) error {
	return tx.Create(v).Error
}

// SaveOrReplaceLatest inserts a new version or refreshes the latest matching
// draft in place. If the latest matching version is released and the current
// save is a draft, it creates a new draft to preserve the released snapshot.
func (dao *UserCanvasVersionDAO) SaveOrReplaceLatest(opts SaveOrReplaceLatestVersionOptions) (*entity.UserCanvasVersion, error) {
	if opts.KeepUnpublished <= 0 {
		opts.KeepUnpublished = 20
	}
	var saved *entity.UserCanvasVersion
	if err := DB.Transaction(func(tx *gorm.DB) error {
		var parent struct {
			ID string
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Table((&entity.UserCanvas{}).TableName()).
			Select("id").
			Where("id = ?", opts.UserCanvasID).
			Take(&parent).Error; err != nil {
			return err
		}

		var latest entity.UserCanvasVersion
		err := tx.Where("user_canvas_id = ?", opts.UserCanvasID).
			Order("create_time DESC, id DESC").
			First(&latest).Error
		if err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
		} else if opts.sameDSL(latest.DSL) {
			if !latest.Release || opts.Release {
				updates := map[string]interface{}{
					"dsl":     opts.DSL,
					"release": opts.Release,
				}
				if opts.Title != nil {
					updates["title"] = opts.Title
				}
				if opts.Description != nil {
					updates["description"] = opts.Description
				}
				if err := tx.Model(&entity.UserCanvasVersion{}).
					Where("id = ?", latest.ID).
					Updates(updates).Error; err != nil {
					return err
				}
				latest.DSL = opts.DSL
				latest.Release = opts.Release
				if opts.Title != nil {
					latest.Title = opts.Title
				}
				if opts.Description != nil {
					latest.Description = opts.Description
				}
				saved = &latest
				return dao.deleteAllUnpublishedExcessTx(tx, opts.UserCanvasID, opts.KeepUnpublished)
			}
		}
		row := &entity.UserCanvasVersion{
			ID:           opts.NewID,
			UserCanvasID: opts.UserCanvasID,
			Title:        opts.Title,
			Description:  opts.Description,
			Release:      opts.Release,
			DSL:          opts.DSL,
		}
		if err := tx.Create(row).Error; err != nil {
			return err
		}
		saved = row
		return dao.deleteAllUnpublishedExcessTx(tx, opts.UserCanvasID, opts.KeepUnpublished)
	}); err != nil {
		return nil, err
	}
	return saved, nil
}

func (opts SaveOrReplaceLatestVersionOptions) sameDSL(dsl entity.JSONMap) bool {
	if opts.SameDSL != nil {
		return opts.SameDSL(dsl)
	}
	return reflect.DeepEqual(dsl, opts.DSL)
}

// DeleteAllUnpublishedExcess keeps the newest keep unpublished versions for a
// canvas and deletes older unpublished rows. Released versions are never
// removed by this cleanup.
func (dao *UserCanvasVersionDAO) DeleteAllUnpublishedExcess(canvasID string, keep int) error {
	return dao.deleteAllUnpublishedExcessTx(DB, canvasID, keep)
}

func (dao *UserCanvasVersionDAO) deleteAllUnpublishedExcessTx(tx *gorm.DB, canvasID string, keep int) error {
	if keep < 0 {
		keep = 0
	}
	var ids []string
	if err := tx.Model(&entity.UserCanvasVersion{}).
		Where(map[string]interface{}{"user_canvas_id": canvasID, "release": false}).
		Order("create_time DESC").
		Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) <= keep {
		return nil
	}
	ids = ids[keep:]
	return tx.Where("id IN ?", ids).Delete(&entity.UserCanvasVersion{}).Error
}
