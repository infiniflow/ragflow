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

package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/utility"

	"gorm.io/gorm"
)

// timeStr formats a Unix-millisecond timestamp for the read-side JSON payloads.
func timeStr(ms *int64) string {
	if ms == nil {
		return ""
	}
	return time.UnixMilli(*ms).UTC().Format("2006-01-02 15:04:05")
}

const (
	groupScopeFile    = "file"
	groupScopeDataset = "dataset"
	// maxGroupNameLen is the DB column size for compilation_template_group.name.
	maxGroupNameLen = 128
)

// GroupTemplate is a child-template entry in a group create/update payload.
type GroupTemplate struct {
	ID          string         `json:"id,omitempty"`
	Name        string         `json:"name,omitempty"`
	Description string         `json:"description,omitempty"`
	Kind        string         `json:"kind,omitempty"`
	Config      entity.JSONMap `json:"config,omitempty"`
}

// GroupRequest is the create/update payload for a compilation template group.
type GroupRequest struct {
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	Templates   []*GroupTemplate `json:"templates,omitempty"`
}

// CompilationTemplateGroupService implements the knowledge-compilation template
// group REST operations, mirroring the Python CompilationTemplateGroupService.
type CompilationTemplateGroupService struct {
	groupDAO    *dao.CompilationTemplateGroupDAO
	templateDAO *dao.CompilationTemplateDAO
	tenantDAO   *dao.TenantDAO
}

// NewCompilationTemplateGroupService creates a CompilationTemplateGroupService.
func NewCompilationTemplateGroupService() *CompilationTemplateGroupService {
	return &CompilationTemplateGroupService{
		groupDAO:    dao.NewCompilationTemplateGroupDAO(),
		templateDAO: dao.NewCompilationTemplateDAO(),
		tenantDAO:   dao.NewTenantDAO(),
	}
}

// GroupListItem is the read-side representation of a group with its nested
// templates, mirroring Python _group_to_dict.
type GroupListItem struct {
	ID          string              `json:"id"`
	Name        string              `json:"name"`
	Description string              `json:"description"`
	Scope       string              `json:"scope"`
	CreateTime  string              `json:"create_time,omitempty"`
	UpdateTime  string              `json:"update_time,omitempty"`
	Templates   []*TemplateListItem `json:"templates"`
}

// ListSaved returns the tenant's groups with nested templates. Mirrors Python
// list_saved(). total is derived from the full (unpaginated) result set.
func (s *CompilationTemplateGroupService) ListSaved(ctx context.Context, tenantID, keywords, scope, orderby string, desc bool) ([]*GroupListItem, error) {
	groups, err := s.groupDAO.ListSaved(ctx, dao.DB, tenantID, keywords, scope, orderby, desc)
	if err != nil {
		return nil, err
	}
	return s.buildGroupItems(ctx, tenantID, groups)
}

// GetSaved returns a single group with its nested templates, or nil. Mirrors
// Python get_saved().
func (s *CompilationTemplateGroupService) GetSaved(ctx context.Context, tenantID, groupID string) (*GroupListItem, error) {
	group, err := s.groupDAO.GetSaved(ctx, dao.DB, tenantID, groupID)
	if err != nil || group == nil {
		return nil, err
	}
	return s.buildGroupItem(ctx, tenantID, group)
}

// CreateGroup creates a group plus its child templates. Mirrors Python
// create_group(). It returns a GroupValidationError for payload/scope problems.
// The group and all child writes are committed atomically so a mid-way failure
// cannot leave a partially-populated group behind.
func (s *CompilationTemplateGroupService) CreateGroup(ctx context.Context, tenantID string, req *GroupRequest) (*GroupListItem, error) {
	if err := validateGroupPayload(req, true); err != nil {
		return nil, err
	}
	scope, err := s.deriveScope(req.Templates)
	if err != nil {
		return nil, err
	}

	groupID := utility.GenerateUUID()
	if err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group := &entity.CompilationTemplateGroup{
			ID:          groupID,
			TenantID:    tenantID,
			Name:        strings.TrimSpace(req.Name),
			Description: strptr(req.Description),
			Scope:       scope,
			Status:      strptr(string(entity.StatusValid)),
		}
		if cerr := s.groupDAO.Create(ctx, tx, group); cerr != nil {
			return cerr
		}
		return s.insertChildren(ctx, tx, tenantID, groupID, req.Templates, nil)
	}); err != nil {
		return nil, err
	}
	return s.GetSaved(ctx, tenantID, groupID)
}

// UpdateGroup applies a partial update to a group and reconciles its child
// templates. Mirrors Python update_group(). The field update and child
// reconciliation are committed atomically.
func (s *CompilationTemplateGroupService) UpdateGroup(ctx context.Context, tenantID, groupID string, req *GroupRequest) (*GroupListItem, error) {
	existing, err := s.groupDAO.GetSaved(ctx, dao.DB, tenantID, groupID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, nil
	}

	if err = validateGroupPayload(req, false); err != nil {
		return nil, err
	}

	if err = dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{}
		if strings.TrimSpace(req.Name) != "" {
			updates["name"] = strings.TrimSpace(req.Name)
		}
		if req.Description != "" {
			updates["description"] = req.Description
		}
		if req.Templates != nil {
			scope, serr := s.deriveScope(req.Templates)
			if serr != nil {
				return serr
			}
			updates["scope"] = scope
		}
		if len(updates) > 0 {
			if uerr := s.groupDAO.UpdateFields(ctx, tx, groupID, updates); uerr != nil {
				return uerr
			}
		}
		if req.Templates != nil {
			return s.reconcileChildren(ctx, tx, tenantID, groupID, req.Templates)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s.GetSaved(ctx, tenantID, groupID)
}

// DeleteGroup soft-deletes a group and its valid child templates. Mirrors
// Python delete_group(). Returns (false, nil) when the group is missing.
func (s *CompilationTemplateGroupService) DeleteGroup(ctx context.Context, tenantID, groupID string) (bool, error) {
	existing, err := s.groupDAO.GetSaved(ctx, dao.DB, tenantID, groupID)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return false, nil
	}
	if err = s.templateDAO.UpdateStatusByGroup(ctx, dao.DB, groupID, string(entity.StatusInvalid)); err != nil {
		return false, err
	}
	if err = s.groupDAO.Delete(ctx, dao.DB, tenantID, groupID); err != nil {
		return false, err
	}
	return true, nil
}

// GroupValidationError is returned for group payload/scope problems so handlers
// can map it to a 400 without an HTTP 500.
type GroupValidationError struct{ msg string }

func (e *GroupValidationError) Error() string { return e.msg }

func groupValidationErrorf(format string, a ...interface{}) error {
	return &GroupValidationError{msg: fmt.Sprintf(format, a...)}
}

// NameExists reports whether a valid group with the given name exists for the
// tenant (optionally excluding excludeID), mirroring Python name_exists().
func (s *CompilationTemplateGroupService) NameExists(ctx context.Context, tenantID, name string, excludeIDs ...string) (bool, error) {
	excludeID := ""
	if len(excludeIDs) > 0 {
		excludeID = excludeIDs[0]
	}
	return s.groupDAO.NameExists(ctx, dao.DB, tenantID, name, excludeID)
}

// ValidateGroupRequest validates the fields present in a group create/update
// payload before any DB write, mirroring the Python blueprint
// _validate_group_payload. Required-field enforcement for create happens inside
// CreateGroup (require_all=True).
func ValidateGroupRequest(req *GroupRequest) error {
	return validateGroupPayload(req, false)
}

// buildGroupItems loads child templates for many groups at once.
func (s *CompilationTemplateGroupService) buildGroupItems(ctx context.Context, tenantID string, groups []*entity.CompilationTemplateGroup) ([]*GroupListItem, error) {
	items := make([]*GroupListItem, 0, len(groups))
	for _, g := range groups {
		item, err := s.buildGroupItem(ctx, tenantID, g)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *CompilationTemplateGroupService) buildGroupItem(ctx context.Context, tenantID string, group *entity.CompilationTemplateGroup) (*GroupListItem, error) {
	children, err := s.templateDAO.ListByGroup(ctx, dao.DB, group.ID)
	if err != nil {
		return nil, err
	}
	items := make([]*TemplateListItem, 0, len(children))
	for _, c := range children {
		items = append(items, &TemplateListItem{
			ID:          c.ID,
			Name:        c.Name,
			Description: derefString(c.Description),
			Kind:        c.Kind,
			Config:      fillConfigDefaultLLM(ctx, s.tenantDAO, c.Config, c.TenantID),
			CreateTime:  timeStr(c.CreateTime),
			UpdateTime:  timeStr(c.UpdateTime),
		})
	}
	return &GroupListItem{
		ID:          group.ID,
		Name:        group.Name,
		Description: derefString(group.Description),
		Scope:       group.Scope,
		CreateTime:  timeStr(group.CreateTime),
		UpdateTime:  timeStr(group.UpdateTime),
		Templates:   items,
	}, nil
}

// deriveScope mirrors Python _derive_scope: one wiki child => dataset scope,
// otherwise file scope (with a wiki-combination guard and re-chunk tree guard).
func (s *CompilationTemplateGroupService) deriveScope(templates []*GroupTemplate) (string, error) {
	if len(templates) == 0 {
		return "", groupValidationErrorf("a template group must contain at least one template.")
	}
	artifactCount := 0
	for _, t := range templates {
		if strings.TrimSpace(t.Kind) == "wiki" {
			artifactCount++
		}
	}
	if artifactCount > 0 {
		if artifactCount != 1 || len(templates) != 1 {
			return "", groupValidationErrorf("a wiki template cannot be combined with other templates in the same group.")
		}
		return groupScopeDataset, nil
	}
	if err := s.enforceSingleRechunkTree(templates); err != nil {
		return "", err
	}
	return groupScopeFile, nil
}

// enforceSingleRechunkTree mirrors Python _enforce_single_rechunk_tree.
func (s *CompilationTemplateGroupService) enforceSingleRechunkTree(templates []*GroupTemplate) error {
	rechunkTrees := 0
	for _, t := range templates {
		if strings.TrimSpace(t.Kind) != "tree" {
			continue
		}
		raptor, _ := t.Config["raptor"].(map[string]interface{})
		if b, ok := raptor["rechunk"].(bool); ok && b {
			rechunkTrees++
		}
	}
	if rechunkTrees > 1 {
		return groupValidationErrorf("only one tree template in a group may enable re-chunking.")
	}
	return nil
}

// validateGroupPayload mirrors Python _validate_group_payload in the group
// blueprint.
func validateGroupPayload(req *GroupRequest, requireAll bool) error {
	if requireAll {
		if strings.TrimSpace(req.Name) == "" {
			return groupValidationErrorf("missing required field: name.")
		}
		if len(req.Templates) == 0 {
			return groupValidationErrorf("missing required field: templates.")
		}
	}
	if strings.TrimSpace(req.Name) != "" {
		if len([]byte(req.Name)) > maxGroupNameLen {
			return groupValidationErrorf("template group name is too long.")
		}
	}
	if len(req.Description) > 1024 {
		return groupValidationErrorf("invalid template group description.")
	}
	if req.Templates != nil {
		if len(req.Templates) == 0 {
			return groupValidationErrorf("a template group must contain at least one template.")
		}
		seen := map[string]struct{}{}
		for _, child := range req.Templates {
			if child == nil {
				return groupValidationErrorf("invalid template entry in group.")
			}
			payload := map[string]interface{}{
				"name": child.Name, "description": child.Description,
				"kind": child.Kind, "config": child.Config,
			}
			if err := ValidateTemplatePayload(payload, true); err != nil {
				return err
			}
			name := strings.TrimSpace(child.Name)
			if _, dup := seen[name]; dup {
				return groupValidationErrorf("template name '%s' is duplicated in this group.", name)
			}
			seen[name] = struct{}{}
		}
	}
	return nil
}

// insertChildren inserts new child templates. usedForReconcile controls whether
// the duplicate-name guard applies (create always; reconcile passes seen set).
func (s *CompilationTemplateGroupService) insertChildren(ctx context.Context, db *gorm.DB, tenantID, groupID string, templates []*GroupTemplate, seen map[string]struct{}) error {
	for _, child := range templates {
		name := strings.TrimSpace(child.Name)
		if exists, err := s.templateDAO.NameExistsInGroup(ctx, dao.DB, tenantID, groupID, name, ""); err != nil {
			return err
		} else if exists {
			return groupValidationErrorf("template name '%s' already exists in this group.", name)
		}
		desc := child.Description
		config := fillConfigDefaultLLM(ctx, s.tenantDAO, child.Config, &tenantID)
		if config == nil {
			config = entity.JSONMap{}
		}
		valid := string(entity.StatusValid)
		tmpl := &entity.CompilationTemplate{
			ID:       utility.GenerateUUID(),
			TenantID: &tenantID,
			GroupID:  &groupID,
			Name:     name,
			Kind:     strings.TrimSpace(child.Kind),
			Config:   config,
			Status:   &valid,
		}
		if desc != "" {
			tmpl.Description = &desc
		}
		if err := s.templateDAO.Save(ctx, db, tmpl); err != nil {
			return err
		}
	}
	return nil
}

// reconcileChildren mirrors the Python update_group child reconciliation:
// existing children (matched by id, or by submitted order for legacy clients)
// are updated in place, new ones inserted, and removed ones soft-deleted.
func (s *CompilationTemplateGroupService) reconcileChildren(ctx context.Context, db *gorm.DB, tenantID, groupID string, templates []*GroupTemplate) error {
	current, err := s.templateDAO.ListByGroup(ctx, db, groupID)
	if err != nil {
		return err
	}
	currentByID := map[string]*entity.CompilationTemplate{}
	for _, c := range current {
		currentByID[c.ID] = c
	}
	seenNames := map[string]struct{}{}
	retained := map[string]struct{}{}

	for index, child := range templates {
		name := strings.TrimSpace(child.Name)
		if _, dup := seenNames[name]; dup {
			return groupValidationErrorf("template name '%s' is duplicated in this group.", name)
		}
		seenNames[name] = struct{}{}

		var target *entity.CompilationTemplate
		if child.ID != "" {
			target = currentByID[child.ID]
			if target == nil {
				return groupValidationErrorf("template %s does not belong to this group.", child.ID)
			}
		} else if index < len(current) {
			// Positional fallback for legacy clients that omit IDs. Skip rows
			// already retained by an explicit ID above so a mixed payload cannot
			// bind two submitted entries to one existing row.
			if _, taken := retained[current[index].ID]; !taken {
				target = current[index]
			}
		}

		config := fillConfigDefaultLLM(ctx, s.tenantDAO, child.Config, &tenantID)
		if config == nil {
			config = entity.JSONMap{}
		}
		desc := child.Description
		if target != nil {
			updates := map[string]interface{}{
				"name": name, "kind": strings.TrimSpace(child.Kind), "config": config,
			}
			if desc != "" {
				updates["description"] = desc
			}
			if err := s.templateDAO.UpdateFields(ctx, db, target.ID, updates); err != nil {
				return err
			}
			retained[target.ID] = struct{}{}
			continue
		}
		newID := utility.GenerateUUID()
		valid := string(entity.StatusValid)
		tmpl := &entity.CompilationTemplate{
			ID:       newID,
			TenantID: &tenantID,
			GroupID:  &groupID,
			Name:     name,
			Kind:     strings.TrimSpace(child.Kind),
			Config:   config,
			Status:   &valid,
		}
		if desc != "" {
			tmpl.Description = &desc
		}
		if err := s.templateDAO.Save(ctx, db, tmpl); err != nil {
			return err
		}
		retained[newID] = struct{}{}
	}

	for _, c := range current {
		if _, keep := retained[c.ID]; !keep {
			if err := s.templateDAO.UpdateStatusByID(ctx, db, c.ID, string(entity.StatusInvalid)); err != nil {
				return err
			}
			// Mirror Python _purge_stale_invalid_children: permanently drop
			// any stale invalid non-builtin template of the same name, scoped to
			// this group so a same-named template in another group is untouched.
			if c.Name != "" && !c.IsBuiltin {
				if err := s.templateDAO.HardDeleteOrphansByName(ctx, db, tenantID, groupID, c.Name); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func strptr(s string) *string { return &s }
