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
	"regexp"
	"strings"

	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// ErrUserCanvasNotFound is returned by GetByIDForUser when the canvas is
// missing or the caller has no read access. We deliberately do not
// distinguish "missing" from "forbidden" so the response cannot be used
// to enumerate other users' canvas ids — see plan §4.8 (IDOR mitigation).

// userCanvasOrderableColumns whitelists the columns that may appear in an
// ORDER BY clause. Keeps user-supplied `orderby` query params from being
// spliced straight into SQL.
var userCanvasOrderableColumns = map[string]struct{}{
	"id":              {},
	"user_id":         {},
	"title":           {},
	"permission":      {},
	"canvas_type":     {},
	"canvas_category": {},
	"create_time":     {},
	"create_date":     {},
	"update_time":     {},
	"update_date":     {},
}

func userCanvasOrderClause(orderby string, desc bool) string {
	if _, ok := userCanvasOrderableColumns[orderby]; !ok {
		orderby = "create_time"
	}
	if desc {
		return orderby + " DESC"
	}
	return orderby + " ASC"
}

func userCanvasQualifiedOrderClause(orderby string, desc bool) string {
	if _, ok := userCanvasOrderableColumns[orderby]; !ok {
		orderby = "create_time"
	}
	order := "user_canvas." + orderby
	if desc {
		return order + " DESC"
	}
	return order + " ASC"
}

func escapeSQLLike(s string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return replacer.Replace(s)
}

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

func applyUserCanvasTagFilter(query *gorm.DB, tags []string) *gorm.DB {
	if len(tags) == 0 {
		return query
	}
	tagQuery := DB.Session(&gorm.Session{NewDB: true})
	hasTag := false
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		pattern := "(^|,)[[:space:]]*" + regexp.QuoteMeta(tag) + "[[:space:]]*(,|$)"
		cond := DB.Where("user_canvas.tags REGEXP ?", pattern)
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

var ErrUserCanvasNotFound = errors.New("user_canvas: not found or access denied")

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
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserCanvasNotFound
		}
		return nil, err
	}
	return &canvas, nil
}

// GetByIDForUser fetches a canvas and enforces ownership visibility:
//
//   - canvases with permission="me" or owned by the requesting user are
//     always returned;
//   - canvases with permission="team" are returned when the canvas owner
//     is a tenant the requesting user belongs to (the team membership
//     predicate mirrors user_canvas.ListByTenantIDs).
//
// Any other case — missing row, foreign private canvas, foreign team
// canvas — yields ErrUserCanvasNotFound. The single error type stops
// callers from leaking "exists but not yours" vs "doesn't exist" via the
// HTTP status code.
func (dao *UserCanvasDAO) GetByIDForUser(canvasID, userID string, tenantIDs []string) (*entity.UserCanvas, error) {
	if canvasID == "" {
		return nil, ErrUserCanvasNotFound
	}
	if userID == "" {
		return nil, ErrUserCanvasNotFound
	}

	// owner=userID is allowed regardless of permission, matching the
	// ListByTenantIDs predicate used by GET /api/v1/agents.
	ownerOrTeam := DB.Where("user_id = ?", userID)
	if len(tenantIDs) > 0 {
		ownerOrTeam = ownerOrTeam.Or(
			"user_id IN ? AND permission = ?", tenantIDs, "team",
		)
	}

	var canvas entity.UserCanvas
	err := DB.Where("id = ?", canvasID).Where(ownerOrTeam).First(&canvas).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserCanvasNotFound
		}
		return nil, err
	}
	return &canvas, nil
}

// Update update user canvas
func (dao *UserCanvasDAO) Update(userCanvas *entity.UserCanvas) error {
	return DB.Save(userCanvas).Error
}

// Accessible reports whether canvasID is reachable by userID under
// the same owner-or-team rule used by GetByIDForUser. Used by
// downstream authorization gates (e.g. the sandbox-artifact
// download endpoint introduced by PR #16169) to confirm a caller
// may reach a given canvas before exposing its runtime artifacts.
// Returns false on any error (not found, DB failure, or empty
// inputs) so callers can treat a denial as a 404-equivalent and
// avoid leaking whether the canvas exists at all.
//
// Tenant scoping (PR review round 5, security review #1): unlike
// the previous form, a `permission = "team"` canvas is only
// reachable when userID is a member of one of the owner's tenants.
// Passing a nil/empty tenantIDs list effectively disables the
// team-canvas branch (no team canvas can match), which is the
// safe default — a caller that forgot to plumb the tenant list
// cannot accidentally bypass team-membership scoping.
//
// Callers that don't have a tenant list handy (rare; most
// handlers derive it from the user context) should call
// GetTenantIDsByUserID first and pass the result.
func (dao *UserCanvasDAO) Accessible(canvasID, userID string, tenantIDs []string) bool {
	if canvasID == "" || userID == "" {
		return false
	}
	// Owner can always access their own canvas regardless of permission.
	// Team-permission canvases are reachable only when the caller is a
	// member of one of the owner's tenants — mirrors the predicate in
	// GetByIDForUser / ListByTenantIDs.
	ownerOrTeam := DB.Where("user_id = ?", userID)
	if len(tenantIDs) > 0 {
		ownerOrTeam = ownerOrTeam.Or(
			"user_id IN ? AND permission = ?", tenantIDs, "team",
		)
	}
	var canvas entity.UserCanvas
	err := DB.Select("id").
		Where("id = ?", canvasID).
		Where(ownerOrTeam).
		First(&canvas).Error
	if err != nil {
		return false
	}
	return canvas.ID == canvasID
}

// Delete delete user canvas
func (dao *UserCanvasDAO) Delete(id string) error {
	// gorm v2 treats the first non-int inline arg as a column name, not a
	// primary-key value — passing `id` verbatim produced WHERE ID = ?
	// and made MySQL complain about an unknown "AGENT_ID" column. The
	// explicit Where+Delete form is the same pattern used by
	// API4ConversationDAO.Delete (see api_token.go:142-144).
	return DB.Where("id = ?", id).Delete(&entity.UserCanvas{}).Error
}

// UpdateTx is the transactional variant of Update. Callers wrap a sequence
// of *Tx calls in dao.DB.Transaction(func(tx *gorm.DB) error { ... }) so
// multi-step writes (e.g. publish-agent, delete-agent) are atomic.
func (dao *UserCanvasDAO) UpdateTx(tx *gorm.DB, userCanvas *entity.UserCanvas) error {
	return tx.Save(userCanvas).Error
}

// DeleteTx is the transactional variant of Delete. The canvas must
// already be loaded and access-checked by the caller.
func (dao *UserCanvasDAO) DeleteTx(tx *gorm.DB, id string) error {
	// See Delete() above for the rationale on Where("id = ?", id).
	return tx.Where("id = ?", id).Delete(&entity.UserCanvas{}).Error
}

// GetByUserAndTitle returns the canvas matching user_id + title (and
// optional canvas_category), or (nil, nil) when no such canvas exists.
// Used by service.AgentService.CreateAgent to enforce the "title
// already exists" rule that the Python agent API mirrors with
// UserCanvasService.query(user_id=..., title=...).
func (dao *UserCanvasDAO) GetByUserAndTitle(userID, title, canvasCategory string) (*entity.UserCanvas, error) {
	q := DB.Where("user_id = ? AND title = ?", userID, title)
	if canvasCategory != "" {
		q = q.Where("canvas_category = ?", canvasCategory)
	}
	var row entity.UserCanvas
	if err := q.First(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &row, nil
}

// GetList get canvases list with pagination and filtering
// Similar to Python UserCanvasService.get_list
func (dao *UserCanvasDAO) GetList(tenantID string, pageNumber, itemsPerPage int, orderby string, desc bool, id, title string, canvasCategory, canvasType string) ([]*entity.UserCanvas, error) {

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
	}

	if canvasType != "" {
		query = query.Where("canvas_type = ?", canvasType)
	}

	// Order by
	// Route orderby through userCanvasOrderClause above so user-supplied
	// query params can never reach Order() verbatim. The helper validates
	// against userCanvasOrderableColumns (a closed allowlist) and falls
	// back to "create_time" on any miss, so the string spliced into the
	// SQL fragment is always one of a fixed set of column names.
	query = query.Order(userCanvasOrderClause(orderby, desc))

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

// UserCanvasListItem is the joined row returned by ListByTenantIDs.
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

// ListByTenantIDs lists agent canvases accessible to the given owner IDs with optional
// keyword filter, tag filter, pagination, and ordering.
// Mirrors Python UserCanvasService.get_by_tenant_ids (list route only).
func (dao *UserCanvasDAO) ListByTenantIDs(ownerIDs []string, userID string, page, pageSize int, orderby string, desc bool, keywords, canvasCategory, canvasType string, tags []string) ([]*UserCanvasListItem, int64, error) {
	if len(ownerIDs) == 0 {
		return nil, 0, nil
	}

	// Canvases owned by any of the ownerIDs that are "team"-permission, plus all owned by userID.
	base := DB.Model(&entity.UserCanvas{}).
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
		Where(
			DB.Where("user_canvas.user_id IN ? AND user_canvas.permission = ?", ownerIDs, "team").
				Or("user_canvas.user_id = ?", userID),
			"user_canvas.user_id IN ?",
			ownerIDs,
		).Where(
		DB.Where("user_canvas.permission = ?", "team").
			Or("user_canvas.user_id = ?", userID))

	if canvasCategory != "" {
		base = base.Where("user_canvas.canvas_category = ?", canvasCategory)
	}

	if canvasType != "" {
		base = base.Where("canvas_type = ?", canvasType)
	}

	if keywords != "" {
		like := "%" + keywords + "%"
		base = base.Where("user_canvas.title LIKE ?", like)
	}
	base = applyUserCanvasTagFilter(base, tags)

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	order := userCanvasQualifiedOrderClause(orderby, desc)
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

// ListTags returns tag usage counts across canvases visible to userID.
func (dao *UserCanvasDAO) ListTags(ownerIDs []string, userID string, canvasCategory string) (map[string]int, error) {
	if len(ownerIDs) == 0 {
		return map[string]int{}, nil
	}

	query := DB.Model(&entity.UserCanvas{}).
		Select("user_canvas.tags").
		Where(
			DB.Where("user_canvas.user_id IN ? AND user_canvas.permission = ?", ownerIDs, "team").
				Or("user_canvas.user_id = ?", userID),
		)

	if canvasCategory != "" {
		query = query.Where("user_canvas.canvas_category = ?", canvasCategory)
	} else {
		query = query.Where("user_canvas.canvas_category = ?", "agent_canvas")
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

// UpdateFields updates only the supplied user_canvas columns.
func (dao *UserCanvasDAO) UpdateFields(canvasID string, fields map[string]interface{}) (int64, error) {
	result := DB.Model(&entity.UserCanvas{}).Where("id = ?", canvasID).Updates(fields)
	return result.RowsAffected, result.Error
}

// UpdateTags updates a canvas's comma-separated tags by canvas ID.
func (dao *UserCanvasDAO) UpdateTags(canvasID, tags string) (int64, error) {
	result := DB.Model(&entity.UserCanvas{}).Where("id = ?", canvasID).Update("tags", tags)
	return result.RowsAffected, result.Error
}
