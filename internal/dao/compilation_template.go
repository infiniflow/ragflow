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
	"fmt"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

// CompilationTemplateDAO is the data-access object for compilation templates and
// their groups.
type CompilationTemplateDAO struct{}

// NewCompilationTemplateDAO creates a compilation template DAO.
func NewCompilationTemplateDAO() *CompilationTemplateDAO {
	return &CompilationTemplateDAO{}
}

// ResolveGroupTemplateIDs resolves compilation-template-group ids to their child
// compilation_template ids for the given tenant. It mirrors the Python
// CompilationTemplateGroupService.resolve_template_ids: only VALID templates
// whose owning group is VALID and belongs to the tenant are returned, ordered by
// create_time within each group and by the requested group order across groups
// (so the output is deterministic).
//
// An error is returned if any requested group id does not resolve to at least
// one valid template (group missing, wrong tenant, or empty). This surfaces the
// misconfiguration instead of silently dropping the compilation_template_ids
// stamp — the data-loss path the caller is guarding against.
func (dao *CompilationTemplateDAO) ResolveGroupTemplateIDs(ctx context.Context, db *gorm.DB, tenantID string, groupIDs []string) ([]string, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}

	// Verify the requested groups exist and are valid. The built-in group
	// (compiler.json's default) is a global, tenant-agnostic catalogue, so it
	// resolves for every tenant; other groups are scoped to the caller's
	// tenant.
	isBuiltin := make(map[string]bool, len(groupIDs))
	for _, gid := range groupIDs {
		if gid == BuiltinCompilationTemplateGroupID {
			isBuiltin[gid] = true
		}
	}
	var validGroups []entity.CompilationTemplateGroup
	if err := db.WithContext(ctx).
		Where("id IN ? AND status = ?", groupIDs, string(entity.StatusValid)).
		Find(&validGroups).Error; err != nil {
		return nil, fmt.Errorf("resolve compilation template groups: %w", err)
	}
	valid := make(map[string]struct{}, len(validGroups))
	byID := make(map[string]entity.CompilationTemplateGroup, len(validGroups))
	for _, g := range validGroups {
		valid[g.ID] = struct{}{}
		byID[g.ID] = g
	}
	for _, gid := range groupIDs {
		if _, ok := valid[gid]; !ok {
			return nil, fmt.Errorf("compilation_template_group %q not found for tenant %q", gid, tenantID)
		}
		// Non-built-in groups must belong to the requesting tenant.
		if !isBuiltin[gid] {
			if byID[gid].TenantID != tenantID {
				return nil, fmt.Errorf("compilation_template_group %q not found for tenant %q", gid, tenantID)
			}
		}
	}

	var templates []entity.CompilationTemplate
	if err := db.WithContext(ctx).Model(&entity.CompilationTemplate{}).
		Where("group_id IN ? AND status = ?", groupIDs, string(entity.StatusValid)).
		Order("create_time asc").
		Find(&templates).Error; err != nil {
		return nil, fmt.Errorf("resolve compilation templates for groups: %w", err)
	}

	byGroup := make(map[string][]string, len(groupIDs))
	for _, t := range templates {
		if t.GroupID == nil {
			continue
		}
		byGroup[*t.GroupID] = append(byGroup[*t.GroupID], t.ID)
	}

	var out []string
	for _, gid := range groupIDs {
		ids := byGroup[gid]
		if len(ids) == 0 {
			return nil, fmt.Errorf("compilation_template_group %q resolved to no valid templates", gid)
		}
		out = append(out, ids...)
	}
	return out, nil
}

// GetTemplate loads a single, valid compilation template by id for the tenant.
// It backs the KnowledgeCompiler TemplateResolver: the template's kind selects
// the Go compile variant (see common.KindToVariant) and its config is the
// template "content" plumbed to the variant.
func (dao *CompilationTemplateDAO) GetTemplate(ctx context.Context, db *gorm.DB, tenantID, templateID string) (*entity.CompilationTemplate, error) {
	var t entity.CompilationTemplate
	if err := db.WithContext(ctx).
		Where("id = ? AND tenant_id = ? AND status = ?", templateID, tenantID, string(entity.StatusValid)).
		First(&t).Error; err != nil {
		return nil, fmt.Errorf("load compilation template %q: %w", templateID, err)
	}
	return &t, nil
}

// ListByGroup returns the valid compilation templates belonging to a group,
// ordered by create_time asc, mirroring Python
// CompilationTemplateGroupService child loading.
func (dao *CompilationTemplateDAO) ListByGroup(ctx context.Context, db *gorm.DB, groupID string) ([]*entity.CompilationTemplate, error) {
	var templates []*entity.CompilationTemplate
	if err := db.WithContext(ctx).
		Where("group_id = ? AND status = ?", groupID, string(entity.StatusValid)).
		Order("create_time asc").
		Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

// ListBuiltins returns the valid, built-in (is_builtin) compilation templates,
// ordered by create_time then name. Mirrors Python list_builtins().
func (dao *CompilationTemplateDAO) ListBuiltins(ctx context.Context, db *gorm.DB) ([]*entity.CompilationTemplate, error) {
	var templates []*entity.CompilationTemplate
	if err := db.WithContext(ctx).
		Where("is_builtin = ? AND status = ?", true, string(entity.StatusValid)).
		Order("create_time asc, name asc").
		Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

// NameExistsInGroup reports whether a valid, non-built-in template with the
// given name already exists inside the group, excluding excludeID. Mirrors the
// Python duplicate-child guard.
func (dao *CompilationTemplateDAO) NameExistsInGroup(ctx context.Context, db *gorm.DB, tenantID, groupID, name, excludeID string) (bool, error) {
	q := db.WithContext(ctx).Model(&entity.CompilationTemplate{}).
		Where("tenant_id = ? AND group_id = ? AND name = ? AND is_builtin = ? AND status = ?",
			tenantID, groupID, name, false, string(entity.StatusValid))
	if excludeID != "" {
		q = q.Where("id <> ?", excludeID)
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetByID returns a single template row by id regardless of tenant scope
// (used for reconciling group children).
func (dao *CompilationTemplateDAO) GetByID(ctx context.Context, db *gorm.DB, id string) (*entity.CompilationTemplate, error) {
	var t entity.CompilationTemplate
	if err := db.WithContext(ctx).Where("id = ?", id).First(&t).Error; err != nil {
		return nil, err
	}
	return &t, nil
}

// Save inserts a new template row.
func (dao *CompilationTemplateDAO) Save(ctx context.Context, db *gorm.DB, t *entity.CompilationTemplate) error {
	return db.WithContext(ctx).Create(t).Error
}

// UpdateFields persists the supplied columns for the template with the given id.
func (dao *CompilationTemplateDAO) UpdateFields(ctx context.Context, db *gorm.DB, id string, m map[string]interface{}) error {
	return db.WithContext(ctx).Model(&entity.CompilationTemplate{}).
		Where("id = ?", id).Updates(m).Error
}

// UpdateStatusByGroup flips the status of every valid template in a group
func (dao *CompilationTemplateDAO) UpdateStatusByGroup(ctx context.Context, db *gorm.DB, groupID, status string) error {
	return db.WithContext(ctx).Model(&entity.CompilationTemplate{}).
		Where("group_id = ? AND status = ?", groupID, string(entity.StatusValid)).
		Update("status", status).Error
}

// UpdateStatusByID flips a single template's status.
func (dao *CompilationTemplateDAO) UpdateStatusByID(ctx context.Context, db *gorm.DB, id, status string) error {
	return db.WithContext(ctx).Model(&entity.CompilationTemplate{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// HardDeleteOrphansByName physically removes stale, invalid, non-built-in
// templates of the given name within the group, mirroring Python
// _purge_stale_invalid_children (which Deletes orphaned duplicate names after a
// group child is soft-deleted). The deletion is scoped to the group so a
// same-named template in another group is never affected. These rows were
// soft-deleted in a prior operation but must be permanently purged to keep the
// table clean.
func (dao *CompilationTemplateDAO) HardDeleteOrphansByName(ctx context.Context, db *gorm.DB, tenantID, groupID, name string) error {
	return db.WithContext(ctx).Where(
		"tenant_id = ? AND group_id = ? AND name = ? AND is_builtin = ? AND status = ?",
		tenantID, groupID, name, false, string(entity.StatusInvalid),
	).Delete(&entity.CompilationTemplate{}).Error
}
