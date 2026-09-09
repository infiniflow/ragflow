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
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// CompilationTemplateGroupDAO is the data-access object for compilation template
// groups.
type CompilationTemplateGroupDAO struct{}

// NewCompilationTemplateGroupDAO creates a compilation template group DAO.
func NewCompilationTemplateGroupDAO() *CompilationTemplateGroupDAO {
	return &CompilationTemplateGroupDAO{}
}

// ListSaved returns the tenant's valid groups with optional keyword/scope
// filtering and ordering, mirroring Python list_saved(). Built-in groups
// (empty tenant) are included so every tenant sees the catalogue.
func (dao *CompilationTemplateGroupDAO) ListSaved(ctx context.Context, db *gorm.DB, tenantID, keywords, scope, orderby string, desc bool) ([]*entity.CompilationTemplateGroup, error) {
	q := db.WithContext(ctx).
		Where("(tenant_id = ? OR tenant_id = '') AND status = ?", tenantID, string(entity.StatusValid))
	if keywords != "" {
		q = q.Where("name LIKE ?", "%"+keywords+"%")
	}
	if scope != "" {
		q = q.Where("scope = ?", scope)
	}
	if orderby != "name" && orderby != "scope" && orderby != "create_time" && orderby != "update_time" {
		orderby = "create_time"
	}
	dir := "asc"
	if desc {
		dir = "desc"
	}
	var groups []*entity.CompilationTemplateGroup
	if err := q.Order(orderby + " " + dir).Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

// ListOwnedSaved returns only the tenant's own valid groups (built-in groups
// with empty tenant_id are excluded), mirroring the Python list_saved() query
// (cls.model.tenant_id == tenant_id). The merged /agents list uses this so
// built-in catalogue groups do not leak into a tenant's canvas list.
func (dao *CompilationTemplateGroupDAO) ListOwnedSaved(ctx context.Context, db *gorm.DB, tenantID, keywords, scope, orderby string, desc bool) ([]*entity.CompilationTemplateGroup, error) {
	q := db.WithContext(ctx).
		Where("tenant_id = ? AND status = ?", tenantID, string(entity.StatusValid))
	if keywords != "" {
		q = q.Where("name LIKE ?", "%"+keywords+"%")
	}
	if scope != "" {
		q = q.Where("scope = ?", scope)
	}
	if orderby != "name" && orderby != "scope" && orderby != "create_time" && orderby != "update_time" {
		orderby = "create_time"
	}
	dir := "asc"
	if desc {
		dir = "desc"
	}
	var groups []*entity.CompilationTemplateGroup
	if err := q.Order(orderby + " " + dir).Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

// CountSavedByTenant counts the tenant's own valid groups; built-in groups
// (empty tenant_id) are excluded, mirroring the group_count query in Python
// get_owner_filter / get_category_filter.
func (dao *CompilationTemplateGroupDAO) CountSavedByTenant(ctx context.Context, db *gorm.DB, tenantID string) (int64, error) {
	var count int64
	err := db.WithContext(ctx).Model(&entity.CompilationTemplateGroup{}).
		Where("tenant_id = ? AND status = ?", tenantID, string(entity.StatusValid)).
		Count(&count).Error
	return count, err
}

// GetSaved returns a single valid group for the tenant (or built-in), or nil.
func (dao *CompilationTemplateGroupDAO) GetSaved(ctx context.Context, db *gorm.DB, tenantID, groupID string) (*entity.CompilationTemplateGroup, error) {
	var g entity.CompilationTemplateGroup
	err := db.WithContext(ctx).
		Where("(tenant_id = ? OR tenant_id = '') AND id = ? AND status = ?",
			tenantID, groupID, string(entity.StatusValid)).
		First(&g).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &g, nil
}

// NameExists reports whether a valid group with the given name exists for the
// tenant, excluding excludeID. Mirrors Python name_exists().
func (dao *CompilationTemplateGroupDAO) NameExists(ctx context.Context, db *gorm.DB, tenantID, name, excludeID string) (bool, error) {
	q := db.WithContext(ctx).Model(&entity.CompilationTemplateGroup{}).
		Where("tenant_id = ? AND name = ? AND status = ?", tenantID, name, string(entity.StatusValid))
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// Create inserts a new group row.
func (dao *CompilationTemplateGroupDAO) Create(ctx context.Context, db *gorm.DB, g *entity.CompilationTemplateGroup) error {
	return db.WithContext(ctx).Create(g).Error
}

// UpdateFields persists the supplied columns for the group with the given id.
func (dao *CompilationTemplateGroupDAO) UpdateFields(ctx context.Context, db *gorm.DB, id string, m map[string]interface{}) error {
	return db.WithContext(ctx).Model(&entity.CompilationTemplateGroup{}).
		Where("id = ?", id).Updates(m).Error
}

// Delete soft-deletes a valid group, mirroring Python delete_group().
func (dao *CompilationTemplateGroupDAO) Delete(ctx context.Context, db *gorm.DB, tenantID, groupID string) error {
	return db.WithContext(ctx).Model(&entity.CompilationTemplateGroup{}).
		Where("id = ? AND tenant_id = ? AND status = ?", groupID, tenantID, string(entity.StatusValid)).
		Update("status", string(entity.StatusInvalid)).Error
}
