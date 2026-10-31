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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"ragflow/internal/entity"
)

// ErrUserCanvasVersionNotFound is returned when a version lookup by id or
// canvas id yields no rows. Service and handler layers map this to a 404.
var ErrUserCanvasVersionNotFound = errors.New("user_canvas_version: not found")

// ErrPipelineVersionMissing indicates the requested pipeline_version_id does
// not exist or does not belong to the bound canvas. Callers treat this as a
// Missing state: the file cannot be parsed until a valid version is (re)selected.
// It is defined once here in the dao layer so that the service and task layers
// (which re-export it) share a single sentinel and error checks match across
// all layers (no duplicate errors.New with the same message but different
// identity).
var ErrPipelineVersionMissing = errors.New("pipeline version missing or not owned by canvas")

// ErrPipelineVersionEmpty is returned when a dataset or document is saved in
// Pipeline (dataflow canvas) mode but carries no concrete pinned version
// (pipeline_version_id is empty / not selected). Per design C5/G3 the empty
// version is no longer auto-backfilled to the canvas latest; instead the Save is
// rejected so the UI can surface the red "Missing" badge and the
// "version information is missing" error. BuiltIn mode (no canvas bound) is exempt.
var ErrPipelineVersionEmpty = errors.New("version information is missing")

// userFacingVersionHint is appended to every Missing-state error so the runtime
// parse path can surface a human-readable instruction (design section 6.2, C4).
const userFacingVersionHint = "pipeline version does not exist. Please select a version again"

// MaxCanvasVersions is the per-canvas cap on the number of DSL versions (K=64,
// design: limit the number of versions of a DSL). When a canvas already holds
// this many versions, any write that would create a NEW version row is rejected
// so the user is forced to delete old unused versions first. The cap counts
// every version row (published or not); in-place DSL refreshes never consume
// the budget because they do not insert a row.
const MaxCanvasVersions = 64

// ErrCanvasVersionLimitExceeded is returned by SaveOrReplaceLatestTx when a new
// version write would exceed MaxCanvasVersions. The handler surfaces it to the
// frontend as a data error with this user-facing hint.
var ErrCanvasVersionLimitExceeded = errors.New("version limit reached (maximum 64). Please delete old versions that are no longer in use")

// LoadCanvasVersionDSL resolves the DSL for a dataflow canvas version. When
// versionID is non-empty it pins that exact version; when empty it resolves the
// latest published version. A missing, mis-owned, or empty-DSL version returns
// ErrPipelineVersionMissing so callers can surface a Missing (unparseable) state.
// Both the service (component-param resolution) and task (runtime parse) layers
// delegate here, so the resolution logic and its error contract live in one place.
func LoadCanvasVersionDSL(ctx context.Context, db *gorm.DB, canvasID, versionID string) ([]byte, error) {
	versionDAO := NewUserCanvasVersionDAO()
	if versionID != "" {
		v, err := versionDAO.GetByID(ctx, db, versionID)
		if err != nil {
			if errors.Is(err, ErrUserCanvasVersionNotFound) {
				return nil, fmt.Errorf("%s (version %s): %w", userFacingVersionHint, versionID, ErrPipelineVersionMissing)
			}
			return nil, fmt.Errorf("load pipeline version %s: %w", versionID, err)
		}
		if v.UserCanvasID != canvasID {
			return nil, fmt.Errorf("%s (version %s does not belong to canvas %s): %w", userFacingVersionHint, versionID, canvasID, ErrPipelineVersionMissing)
		}
		if len(v.DSL) == 0 {
			return nil, fmt.Errorf("%s (version %s has no DSL): %w", userFacingVersionHint, versionID, ErrPipelineVersionMissing)
		}
		return marshalCanvasVersionDSL(versionID, v.DSL)
	}
	v, err := versionDAO.GetLatest(ctx, db, canvasID)
	if err != nil {
		if errors.Is(err, ErrUserCanvasVersionNotFound) {
			return nil, fmt.Errorf("%s (canvas %s has no published version): %w", userFacingVersionHint, canvasID, ErrPipelineVersionMissing)
		}
		return nil, fmt.Errorf("load latest version for canvas %s: %w", canvasID, err)
	}
	if len(v.DSL) == 0 {
		return nil, fmt.Errorf("%s (latest version %s has no DSL): %w", userFacingVersionHint, v.ID, ErrPipelineVersionMissing)
	}
	return marshalCanvasVersionDSL(v.ID, v.DSL)
}

func marshalCanvasVersionDSL(versionID string, dsl entity.JSONMap) ([]byte, error) {
	raw, err := json.Marshal(dsl)
	if err != nil {
		return nil, fmt.Errorf("marshal canvas version %s DSL: %w", versionID, err)
	}
	return raw, nil
}

// UserCanvasVersionDAO persists and queries UserCanvasVersion rows.
type UserCanvasVersionDAO struct{}

// SaveOrReplaceLatestVersionOptions controls a version-history save.
type SaveOrReplaceLatestVersionOptions struct {
	NewID        string
	UserCanvasID string
	Title        *string
	Description  *string
	Alias        *string
	DSL          entity.JSONMap
	Release      bool
	SameDSL      func(entity.JSONMap) bool
	// ForceNew bypasses the same-DSL in-place refresh so a manual Save always
	// INSERTs a fresh version row (design §5.1: every manual Save yields a new
	// version, even when the DSL is byte-identical to the previous one). When
	// false, an unchanged DSL refreshes the latest row in place as before.
	ForceNew bool
}

// NewUserCanvasVersionDAO returns a zero-value DAO. The struct is stateless
// so callers can share a single instance or create their own.
func NewUserCanvasVersionDAO() *UserCanvasVersionDAO {
	return &UserCanvasVersionDAO{}
}

// GetByID fetches a single version by primary key. Returns
// ErrUserCanvasVersionNotFound when the row is absent so callers can map
// to a 404 instead of inspecting gorm.ErrRecordNotFound directly.
func (dao *UserCanvasVersionDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.UserCanvasVersion, error) {
	var v entity.UserCanvasVersion
	err := db.WithContext(ctx).Where("id = ?", id).First(&v).Error
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
func (dao *UserCanvasVersionDAO) ListByCanvasID(ctx context.Context, db *gorm.DB, canvasID string) ([]*entity.UserCanvasVersion, error) {
	var vs []*entity.UserCanvasVersion
	err := db.WithContext(ctx).Where("user_canvas_id = ?", canvasID).
		Order("create_time DESC").
		Find(&vs).Error
	return vs, err
}

// GetLatest returns the most recently created version of canvasID, or
// ErrUserCanvasVersionNotFound when the canvas has never been published.
func (dao *UserCanvasVersionDAO) GetLatest(ctx context.Context, db *gorm.DB, canvasID string) (*entity.UserCanvasVersion, error) {
	var v entity.UserCanvasVersion
	err := db.WithContext(ctx).Where("user_canvas_id = ?", canvasID).
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

// LatestIDsByCanvasIDs returns the latest version id (by create_time DESC,
// matching GetLatest) for each of the given canvas ids in a single query.
// Canvases with no version rows are absent from the map. List endpoints use it
// to resolve every row's "is the pinned version still the latest" flag without
// an N+1 of per-row GetLatest calls.
func (dao *UserCanvasVersionDAO) LatestIDsByCanvasIDs(ctx context.Context, db *gorm.DB, canvasIDs []string) (map[string]string, error) {
	out := make(map[string]string, len(canvasIDs))
	if len(canvasIDs) == 0 {
		return out, nil
	}
	type row struct {
		UserCanvasID string
		ID           string
	}
	var rows []row
	if err := db.WithContext(ctx).Model(&entity.UserCanvasVersion{}).
		Select("user_canvas_id, id").
		Where("user_canvas_id IN ?", canvasIDs).
		Order("create_time DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	// Rows arrive newest-first globally, so the first row seen per canvas is its
	// latest version.
	for _, r := range rows {
		if _, ok := out[r.UserCanvasID]; !ok {
			out[r.UserCanvasID] = r.ID
		}
	}
	return out, nil
}

// GetLatestReleased returns the most recently updated released version of
// canvasID, or ErrUserCanvasVersionNotFound when nothing was ever published.
// Mirrors the released-version lookup in Python's get_agent handler.
func (dao *UserCanvasVersionDAO) GetLatestReleased(ctx context.Context, db *gorm.DB, canvasID string) (*entity.UserCanvasVersion, error) {
	var v entity.UserCanvasVersion
	err := db.WithContext(ctx).Where("user_canvas_id = ? AND `release` = ?", canvasID, true).
		Order("update_time DESC").
		First(&v).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserCanvasVersionNotFound
		}
		return nil, err
	}
	return &v, nil
}

// GetLatestReleaseTimes returns MAX(create_time) of released versions per
// canvas for the given canvas IDs. Mirrors the release_time aggregation in
// Python's UserCanvasService.get_list.
func (dao *UserCanvasVersionDAO) GetLatestReleaseTimes(ctx context.Context, db *gorm.DB, canvasIDs []string) (map[string]int64, error) {
	result := make(map[string]int64, len(canvasIDs))
	if len(canvasIDs) == 0 {
		return result, nil
	}
	type row struct {
		UserCanvasID string
		ReleaseTime  int64
	}
	var rows []row
	err := db.WithContext(ctx).Model(&entity.UserCanvasVersion{}).
		Select("user_canvas_id, MAX(create_time) AS release_time").
		Where("user_canvas_id IN ? AND `release` = ?", canvasIDs, true).
		Group("user_canvas_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		result[r.UserCanvasID] = r.ReleaseTime
	}
	return result, nil
}

// DeleteTx is the transactional variant of Delete. Used by
// service.AgentService.DeleteVersion so the version-row removal and the
// (future) parent-canvas stat update land in one atomic write.
func (dao *UserCanvasVersionDAO) DeleteTx(ctx context.Context, tx *gorm.DB, id string) error {
	return tx.WithContext(ctx).Where("id = ?", id).Delete(&entity.UserCanvasVersion{}).Error
}

// pipelineVersionReferenceTables are the tables that can pin a canvas
// version through their pipeline_version_id column (design B4 sticky
// snapshot). The reference scan on delete is scoped to exactly these
// (design section 12 A4).
var pipelineVersionReferenceTables = []string{"knowledgebase", "document"}

// CountPipelineVersionReferences returns how many rows across the version
// reference tables pin the given version. Used by DeleteVersion so callers
// can warn that deleting the version will leave those bindings in a Missing
// state (design section 6.2 / A4).
func (dao *UserCanvasVersionDAO) CountPipelineVersionReferences(ctx context.Context, db *gorm.DB, versionID string) (int64, error) {
	var total int64
	for _, table := range pipelineVersionReferenceTables {
		var n int64
		if err := db.WithContext(ctx).
			Table(table).
			Where("pipeline_version_id = ?", versionID).
			Count(&n).Error; err != nil {
			return 0, fmt.Errorf("count %s references for version %s: %w", table, versionID, err)
		}
		total += n
	}
	return total, nil
}

// DeleteByCanvasIDTx is the transactional variant of DeleteByCanvasID.
// Used by service.AgentService.DeleteAgent so the cascade runs atomically
// with the parent canvas row removal.
func (dao *UserCanvasVersionDAO) DeleteByCanvasIDTx(ctx context.Context, tx *gorm.DB, canvasID string) (int64, error) {
	res := tx.WithContext(ctx).Where("user_canvas_id = ?", canvasID).Delete(&entity.UserCanvasVersion{})
	return res.RowsAffected, res.Error
}

// CreateTx inserts a version using the caller's transaction.
func (dao *UserCanvasVersionDAO) CreateTx(ctx context.Context, tx *gorm.DB, v *entity.UserCanvasVersion) error {
	return tx.WithContext(ctx).Create(v).Error
}

// UserCanvasVersionPatch carries the mutable metadata fields a version PATCH
// may change. All fields are optional; only non-nil fields are written, so a
// caller may update just the alias while leaving title/description untouched.
type UserCanvasVersionPatch struct {
	Alias       *string
	Title       *string
	Description *string
}

// UpdateTx updates mutable metadata fields of an existing version (alias,
// title, description) without touching its DSL or creating a new version. It
// is the backing store for the version PATCH endpoint (design G2/§4.4): alias
// editing must never produce a new version row or alter the pinned DSL. When
// the patch is empty it is a no-op. Returns ErrUserCanvasVersionNotFound when
// the target row does not exist.
func (dao *UserCanvasVersionDAO) UpdateTx(ctx context.Context, tx *gorm.DB, id string, patch UserCanvasVersionPatch) error {
	updates := map[string]interface{}{}
	if patch.Alias != nil {
		updates["alias"] = patch.Alias
	}
	if patch.Title != nil {
		updates["title"] = patch.Title
	}
	if patch.Description != nil {
		updates["description"] = patch.Description
	}
	if len(updates) == 0 {
		return nil
	}
	res := tx.WithContext(ctx).Model(&entity.UserCanvasVersion{}).Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrUserCanvasVersionNotFound
	}
	return nil
}

// SaveOrReplaceLatestTx inserts or refreshes a version using the caller's
// transaction.
func (dao *UserCanvasVersionDAO) SaveOrReplaceLatestTx(ctx context.Context, tx *gorm.DB, opts SaveOrReplaceLatestVersionOptions) (*entity.UserCanvasVersion, error) {
	var parent struct {
		ID string
	}
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Table((&entity.UserCanvas{}).TableName()).
		Select("id").
		Where("id = ?", opts.UserCanvasID).
		Take(&parent).Error; err != nil {
		return nil, err
	}

	var latest entity.UserCanvasVersion
	err := tx.WithContext(ctx).Where("user_canvas_id = ?", opts.UserCanvasID).
		Order("create_time DESC, id DESC").
		First(&latest).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, err
		}
	} else if !opts.ForceNew && opts.sameDSL(latest.DSL) {
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
			if opts.Alias != nil {
				updates["alias"] = opts.Alias
			}
			if err = tx.WithContext(ctx).Model(&entity.UserCanvasVersion{}).
				Where("id = ?", latest.ID).
				Updates(updates).Error; err != nil {
				return nil, err
			}
			latest.DSL = opts.DSL
			latest.Release = opts.Release
			if opts.Title != nil {
				latest.Title = opts.Title
			}
			if opts.Description != nil {
				latest.Description = opts.Description
			}
			return &latest, nil
		}
	}
	// Reaching this point means we are about to INSERT a NEW version row (the
	// in-place refresh branch above never gets here). Enforce the per-canvas
	// version cap: reject the write once the canvas already holds MaxCanvasVersions
	// rows so the user must delete old unused versions first. Counting all rows
	// (published and unpublished) keeps the budget honest -- the unpublished
	// excess trim below only ever removes unpublished rows and never frees the cap.
	var existing int64
	if err = tx.WithContext(ctx).Model(&entity.UserCanvasVersion{}).
		Where("user_canvas_id = ?", opts.UserCanvasID).
		Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing >= MaxCanvasVersions {
		return nil, ErrCanvasVersionLimitExceeded
	}
	row := &entity.UserCanvasVersion{
		ID:           opts.NewID,
		UserCanvasID: opts.UserCanvasID,
		Title:        opts.Title,
		Description:  opts.Description,
		Alias:        opts.Alias,
		Release:      opts.Release,
		DSL:          opts.DSL,
	}
	if err = tx.WithContext(ctx).Create(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (opts SaveOrReplaceLatestVersionOptions) sameDSL(dsl entity.JSONMap) bool {
	if opts.SameDSL != nil {
		return opts.SameDSL(dsl)
	}
	return reflect.DeepEqual(dsl, opts.DSL)
}
