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
func (dao *CompilationTemplateDAO) ResolveGroupTemplateIDs(ctx context.Context, tenantID string, groupIDs []string) ([]string, error) {
	if len(groupIDs) == 0 {
		return nil, nil
	}

	db := DB.WithContext(ctx)

	// Verify the requested groups exist, are valid, and belong to the tenant.
	var validGroups []entity.CompilationTemplateGroup
	if err := db.
		Where("id IN ? AND tenant_id = ? AND status = ?", groupIDs, tenantID, string(entity.StatusValid)).
		Find(&validGroups).Error; err != nil {
		return nil, fmt.Errorf("resolve compilation template groups: %w", err)
	}
	valid := make(map[string]struct{}, len(validGroups))
	for _, g := range validGroups {
		valid[g.ID] = struct{}{}
	}
	for _, gid := range groupIDs {
		if _, ok := valid[gid]; !ok {
			return nil, fmt.Errorf("compilation_template_group %q not found for tenant %q", gid, tenantID)
		}
	}

	var templates []entity.CompilationTemplate
	if err := db.Model(&entity.CompilationTemplate{}).
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
