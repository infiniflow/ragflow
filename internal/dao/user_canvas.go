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
	"errors"
	"regexp"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func splitUserCanvasTags(raw string) []string {
	parts := strings.Split(raw, ",")
	tags := make([]string, 0, len(parts))
	for _, tag := range parts {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	return tags
}

func applyUserCanvasTagFilter(ctx context.Context, db *gorm.DB, query *gorm.DB, tags []string) *gorm.DB {
	if len(tags) == 0 {
		return query
	}
	tagQuery := db.WithContext(ctx).Session(&gorm.Session{NewDB: true})
	hasTag := false
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		pattern := "(^|,)[[:space:]]*" + regexp.QuoteMeta(tag) + "[[:space:]]*(,|$)"
		cond := db.WithContext(ctx).Where("user_canvas.tags REGEXP ?", pattern)
		if !hasTag {
			tagQuery = tagQuery.Where(cond)
			hasTag = true
		} else {
			tagQuery = tagQuery.Or(cond)
		}
	}
	if !hasTag {
		return query
	}
	return query.Where(tagQuery)
}

var ErrUserCanvasNotFound = errors.New("agent: not found or access denied")

// UserCanvasDAO user canvas data access object
type UserCanvasDAO struct{}

// NewUserCanvasDAO create user canvas DAO
func NewUserCanvasDAO() *UserCanvasDAO {
	return &UserCanvasDAO{}
}

// Create user canvas
func (dao *UserCanvasDAO) Create(ctx context.Context, db *gorm.DB, userCanvas *entity.UserCanvas) error {
	return db.WithContext(ctx).Create(userCanvas).Error
}

// GetByID get user canvas by ID
func (dao *UserCanvasDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.UserCanvas, error) {
	var canvas entity.UserCanvas
	err := db.WithContext(ctx).Take(&canvas, "id = ?", id).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserCanvasNotFound
		}
		return nil, err
	}
	return &canvas, nil
}

// Update update user canvas
func (dao *UserCanvasDAO) Update(ctx context.Context, db *gorm.DB, userCanvas *entity.UserCanvas) error {
	return db.WithContext(ctx).Save(userCanvas).Error
}

// Delete delete user canvas
func (dao *UserCanvasDAO) Delete(ctx context.Context, db *gorm.DB, id string) error {
	// gorm v2 treats the first non-int inline arg as a column name, not a
	// primary-key value — passing `id` verbatim produced WHERE ID = ?
	// and made MySQL complain about an unknown "AGENT_ID" column. The
	// explicit Where+Delete form is the same pattern used by
	// API4ConversationDAO.Delete (see api_token.go:142-144).
	return db.WithContext(ctx).Where("id = ?", id).Delete(&entity.UserCanvas{}).Error
}

// UpdateTx is the transactional variant of Update. Callers wrap a sequence
// of *Tx calls in dao.DB.Transaction(func(tx *gorm.DB) error { ... }) so
// multi-step writes (e.g. publish-agent, delete-agent) are atomic.
func (dao *UserCanvasDAO) UpdateTx(ctx context.Context, tx *gorm.DB, userCanvas *entity.UserCanvas) error {
	return tx.WithContext(ctx).Save(userCanvas).Error
}

// DeleteTx is the transactional variant of Delete. The canvas must
// already be loaded and access-checked by the caller.
func (dao *UserCanvasDAO) DeleteTx(ctx context.Context, tx *gorm.DB, id string) error {
	// See Delete() above for the rationale on Where("id = ?", id).
	return tx.WithContext(ctx).Where("id = ?", id).Delete(&entity.UserCanvas{}).Error
}

// TitleExists reports whether the user already has a canvas with the given
// title in the category, comparing titles case-insensitively. excludeID, when
// non-empty, omits that canvas from the check so a rename can keep its own
// title while a move into another category is still validated.
func (dao *UserCanvasDAO) TitleExists(ctx context.Context, db *gorm.DB, userID, canvasCategory, title, excludeID string) (bool, error) {
	q := db.WithContext(ctx).Model(&entity.UserCanvas{}).
		Where("user_id = ? AND LOWER(title) = LOWER(?)", userID, title)
	if canvasCategory != "" {
		q = q.Where("canvas_category = ?", canvasCategory)
	}
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	err := q.Count(&count).Error
	return count > 0, err
}

// GetList get canvases list with pagination and filtering
// Similar to Python UserCanvasService.get_list
func (dao *UserCanvasDAO) GetList(ctx context.Context, db *gorm.DB, tenantID string, pageNumber, itemsPerPage int, terms []OrderTerm, id, title string, canvasCategory, canvasType string) ([]*entity.UserCanvas, error) {

	query := db.WithContext(ctx).Model(&entity.UserCanvas{}).
		Where("user_id = ?", tenantID)

	if id != "" {
		query = query.Where("id = ?", id)
	}
	if title != "" {
		query = query.Where("title = ?", title)
	}
	if canvasCategory != "" {
		query = query.Where("canvas_category = ?", canvasCategory)
	}

	if canvasType != "" {
		query = query.Where("canvas_type = ?", canvasType)
	}

	// Order by
	// Route the requested terms through userCanvasOrderClause above so user-supplied
	// query params can never reach Order() verbatim. The helper validates
	// against userCanvasOrderableColumns (a closed allowlist) and falls
	// back to "create_time" on any miss, so the string spliced into the
	// SQL fragment is always one of a fixed set of column names.
	query = query.Order(userCanvasOrderClause(terms))

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
func (dao *UserCanvasDAO) GetAllCanvasesByTenantIDs(ctx context.Context, db *gorm.DB, tenantIDs []string, userID string) ([]*CanvasBasicInfo, error) {

	query := db.WithContext(ctx).Model(&entity.UserCanvas{}).
		Select("id, avatar, title, permission, canvas_type, canvas_category").
		Where("user_id IN (?) AND permission = ?", tenantIDs, "team").
		Or("user_id = ?", userID).
		Order("create_time ASC")

	var results []*CanvasBasicInfo
	err := query.Scan(&results).Error
	return results, err
}

// UserCanvasListItem is the joined row returned by ListByIDs.
type UserCanvasListItem struct {
	ID             string  `gorm:"column:id"`
	Avatar         *string `gorm:"column:avatar"`
	Title          *string `gorm:"column:title"`
	Description    *string `gorm:"column:description"`
	Permission     string  `gorm:"column:permission"`
	UserID         string  `gorm:"column:user_id"`
	TenantID       string  `gorm:"column:tenant_id"`
	Nickname       *string `gorm:"column:nickname"`
	TenantAvatar   *string `gorm:"column:tenant_avatar"`
	CanvasType     *string `gorm:"column:canvas_type"`
	CanvasCategory string  `gorm:"column:canvas_category"`
	Tags           string  `gorm:"column:tags"`
	CreateTime     *int64  `gorm:"column:create_time"`
	UpdateTime     *int64  `gorm:"column:update_time"`
}

// ListByIDs lists permission-filtered canvases with optional metadata filters.
func (dao *UserCanvasDAO) ListByIDs(ctx context.Context, db *gorm.DB, canvasIDs, ownerIDs []string, page, pageSize int, terms []OrderTerm, keywords string, canvasCategories []string, canvasType string, tags []string) ([]*UserCanvasListItem, int64, error) {
	if len(canvasIDs) == 0 {
		return nil, 0, nil
	}

	base := db.WithContext(ctx).Model(&entity.UserCanvas{}).
		Select(`user_canvas.id,
		user_canvas.avatar,
		user_canvas.title,
		user_canvas.description,
		user_canvas.permission,
		user_canvas.user_id,
		user_canvas.user_id AS tenant_id,
		user.nickname,
		user.avatar AS tenant_avatar,
		user_canvas.canvas_type,
		user_canvas.canvas_category,
		user_canvas.tags,
		user_canvas.create_time,
		user_canvas.update_time`).
		Joins("LEFT JOIN user ON user_canvas.user_id = user.id").
		Where("user_canvas.id IN ?", canvasIDs)
	if len(ownerIDs) > 0 {
		base = base.Where("user_canvas.user_id IN ?", ownerIDs)
	}

	if len(canvasCategories) > 0 {
		base = base.Where("user_canvas.canvas_category IN ?", canvasCategories)
	}

	if canvasType != "" {
		base = base.Where("canvas_type = ?", canvasType)
	}

	if keywords != "" {
		like := "%" + keywords + "%"
		base = base.Where("user_canvas.title LIKE ? OR user_canvas.tags LIKE ?", like, like)
	}
	base = applyUserCanvasTagFilter(ctx, db, base, tags)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := userCanvasQualifiedOrderClause(terms)
	// codeql[go/sql-injection] False positive: `order` was just derived
	// from userCanvasQualifiedOrderClause above, which validates `orderby`
	// against userCanvasOrderableColumns (a closed allowlist) and
	// defaults to "create_time" on miss. The string spliced into
	// Order() is always one of a fixed set of qualified column names.
	query := base.Order(order)

	if page > 0 && pageSize > 0 {
		query = query.Offset((page - 1) * pageSize).Limit(pageSize)
	}

	var canvases []*UserCanvasListItem
	if err := query.Scan(&canvases).Error; err != nil {
		return nil, 0, err
	}
	return canvases, total, nil
}

// OwnerFilterItem is one row of the owner aggregation backing the
// ?type=filter branch of the agents list endpoint.
type OwnerFilterItem struct {
	ID    string  `gorm:"column:id"`
	Label *string `gorm:"column:label"`
	Count int64   `gorm:"column:count"`
}

// CategoryFilterItem is one row of the canvas_category aggregation backing
// the ?type=filter branch of the agents list endpoint.
type CategoryFilterItem struct {
	ID    string `gorm:"column:id"`
	Count int64  `gorm:"column:count"`
}

// GetOwnerFilterByCanvasIDs aggregates permission-filtered canvases by owner.
func (dao *UserCanvasDAO) GetOwnerFilterByCanvasIDs(ctx context.Context, db *gorm.DB, canvasIDs []string) ([]*OwnerFilterItem, error) {
	if len(canvasIDs) == 0 {
		return nil, nil
	}
	var items []*OwnerFilterItem
	err := db.WithContext(ctx).Model(&entity.UserCanvas{}).
		Where("user_canvas.id IN ?", canvasIDs).
		Select("user_canvas.user_id AS id, user.nickname AS label, COUNT(user_canvas.id) AS count").
		Joins("LEFT JOIN user ON user_canvas.user_id = user.id").
		Group("user_canvas.user_id, user.nickname").
		Scan(&items).Error
	return items, err
}

// GetCategoryFilterByCanvasIDs aggregates permission-filtered canvases by category.
func (dao *UserCanvasDAO) GetCategoryFilterByCanvasIDs(ctx context.Context, db *gorm.DB, canvasIDs []string) ([]*CategoryFilterItem, error) {
	if len(canvasIDs) == 0 {
		return nil, nil
	}
	var items []*CategoryFilterItem
	err := db.WithContext(ctx).Model(&entity.UserCanvas{}).
		Where("user_canvas.id IN ?", canvasIDs).
		Select("user_canvas.canvas_category AS id, COUNT(user_canvas.id) AS count").
		Group("user_canvas.canvas_category").
		Scan(&items).Error
	return items, err
}

// ListTagsByCanvasIDs returns tag usage counts across permission-filtered canvases.
func (dao *UserCanvasDAO) ListTagsByCanvasIDs(ctx context.Context, db *gorm.DB, canvasIDs []string, canvasCategory string) (map[string]int, error) {
	if len(canvasIDs) == 0 {
		return map[string]int{}, nil
	}

	query := db.WithContext(ctx).Model(&entity.UserCanvas{}).
		Select("user_canvas.tags").
		Where("user_canvas.id IN ?", canvasIDs)

	if canvasCategory != "" {
		query = query.Where("user_canvas.canvas_category = ?", canvasCategory)
	}

	var rows []struct {
		Tags string `gorm:"column:tags"`
	}
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, row := range rows {
		for _, tag := range splitUserCanvasTags(row.Tags) {
			counts[tag]++
		}
	}
	return counts, nil
}

// GetByCanvasID get user canvas by canvas ID (alias for GetByID)
func (dao *UserCanvasDAO) GetByCanvasID(ctx context.Context, db *gorm.DB, canvasID string) (*entity.UserCanvas, error) {
	return dao.GetByID(ctx, db, canvasID)
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
func (dao *UserCanvasDAO) DeleteByUserID(ctx context.Context, db *gorm.DB, userID string) (int64, error) {
	result := db.WithContext(ctx).Unscoped().Where("user_id = ?", userID).Delete(&entity.UserCanvas{})
	return result.RowsAffected, result.Error
}

// GetAllCanvasIDsByUserID gets all canvas IDs by user ID
func (dao *UserCanvasDAO) GetAllCanvasIDsByUserID(ctx context.Context, db *gorm.DB, userID string) ([]string, error) {
	var canvasIDs []string
	err := db.WithContext(ctx).Model(&entity.UserCanvas{}).
		Where("user_id = ?", userID).
		Pluck("id", &canvasIDs).Error
	return canvasIDs, err
}

// UpdateDSL updates a canvas DSL by canvas ID.
func (dao *UserCanvasDAO) UpdateDSL(ctx context.Context, db *gorm.DB, canvasID string, dsl entity.JSONMap) (int64, error) {
	result := db.WithContext(ctx).Model(&entity.UserCanvas{}).Where("id = ?", canvasID).Update("dsl", dsl)
	return result.RowsAffected, result.Error
}

// UpdateFields updates only the supplied user_canvas columns.
func (dao *UserCanvasDAO) UpdateFields(ctx context.Context, db *gorm.DB, canvasID string, fields map[string]interface{}) (int64, error) {
	result := db.WithContext(ctx).Model(&entity.UserCanvas{}).Where("id = ?", canvasID).Updates(fields)
	return result.RowsAffected, result.Error
}

// UpdateFieldsTx is the transactional variant of UpdateFields. Used by
// service.AgentService.UpdateAgent so the canvas row update and the
// version-row save commit atomically in one transaction.
func (dao *UserCanvasDAO) UpdateFieldsTx(tx *gorm.DB, canvasID string, fields map[string]interface{}) (int64, error) {
	result := tx.Model(&entity.UserCanvas{}).Where("id = ?", canvasID).Updates(fields)
	return result.RowsAffected, result.Error
}

// UpdateTags updates a canvas's comma-separated tags by canvas ID.
func (dao *UserCanvasDAO) UpdateTags(ctx context.Context, db *gorm.DB, canvasID, tags string) (int64, error) {
	result := db.WithContext(ctx).Model(&entity.UserCanvas{}).Where("id = ?", canvasID).Update("tags", tags)
	return result.RowsAffected, result.Error
}
