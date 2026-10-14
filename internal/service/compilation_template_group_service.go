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

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"

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
	// maxTemplateNameLen is the DB column size for compilation_template.name.
	maxTemplateNameLen = 128
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
func (s *CompilationTemplateGroupService) ListSaved(ctx context.Context, tenantID, keywords, scope string, terms []dao.OrderTerm) ([]*GroupListItem, error) {
	groups, err := s.groupDAO.ListSaved(ctx, dao.DB, tenantID, keywords, scope, terms)
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
	// Fall back to a numbered name when the requested one is already taken.
	groupName, err := common.UniqueName(strings.TrimSpace(req.Name), maxGroupNameLen, func(candidate string) (bool, error) {
		return s.groupDAO.NameExists(ctx, dao.DB, tenantID, candidate, "")
	})
	if err != nil {
		return nil, err
	}
	scope, err := s.deriveScope(req.Templates)
	if err != nil {
		return nil, err
	}

	groupID := common.GenerateUUID()
	if err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		group := &entity.CompilationTemplateGroup{
			ID:          groupID,
			TenantID:    tenantID,
			Name:        groupName,
			Description: strptr(req.Description),
			Scope:       scope,
			Status:      strptr(string(entity.StatusValid)),
		}
		if cerr := s.groupDAO.Create(ctx, tx, group); cerr != nil {
			return cerr
		}
		return s.insertChildren(ctx, tx, tenantID, groupID, groupName, req.Templates)
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

// insertChildren inserts new child templates. A group that holds a single
// template mirrors the resolved group name onto that template so the two never
// diverge after the group name falls back to "name(N)"; otherwise a requested
// name already used in the group falls back to a numbered suffix.
func (s *CompilationTemplateGroupService) insertChildren(ctx context.Context, db *gorm.DB, tenantID, groupID, groupName string, templates []*GroupTemplate) error {
	for _, child := range templates {
		// The client submits the template name as the group name, so a single
		// child must share the (possibly deduped) group name or the update
		// endpoint would reject it as a duplicate group name.
		name := groupName
		if len(templates) != 1 {
			var err error
			name, err = common.UniqueName(strings.TrimSpace(child.Name), maxTemplateNameLen, func(candidate string) (bool, error) {
				return s.templateDAO.NameExistsInGroup(ctx, db, tenantID, groupID, candidate, "")
			})
			if err != nil {
				return err
			}
		}
		desc := child.Description
		config := fillConfigDefaultLLM(ctx, s.tenantDAO, child.Config, &tenantID)
		if config == nil {
			config = entity.JSONMap{}
		}
		valid := string(entity.StatusValid)
		tmpl := &entity.CompilationTemplate{
			ID:       common.GenerateUUID(),
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
//
// The reconciliation runs in four passes so a rename onto a name that this same
// update frees (a dropped child, swapped names) is not rejected as a conflict,
// while a rename onto a name held by a surviving child still fails:
//  1. resolve each submitted entry to its existing target,
//  2. soft-delete the children that leave the group,
//  3. apply the renames and inserts,
//  4. reject duplicated (case-insensitive) names among the surviving children.
func (s *CompilationTemplateGroupService) reconcileChildren(ctx context.Context, db *gorm.DB, tenantID, groupID string, templates []*GroupTemplate) error {
	current, err := s.templateDAO.ListByGroup(ctx, db, groupID)
	if err != nil {
		return err
	}
	currentByID := map[string]*entity.CompilationTemplate{}
	for _, c := range current {
		currentByID[c.ID] = c
	}

	type resolvedChild struct {
		child  *GroupTemplate
		target *entity.CompilationTemplate
		name   string
	}
	seenNames := map[string]struct{}{}
	retained := map[string]struct{}{}
	resolved := make([]resolvedChild, 0, len(templates))

	// Pass 1: resolve targets before any write so the soft-deletes in pass 2 can
	// run first.
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
		if target != nil {
			retained[target.ID] = struct{}{}
		}
		resolved = append(resolved, resolvedChild{child: child, target: target, name: name})
	}

	// Pass 2: drop the children that leave the group before applying renames, so
	// a rename onto one of their names is not reported as a conflict.
	for _, c := range current {
		if _, keep := retained[c.ID]; keep {
			continue
		}
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

	// Pass 3: apply updates and inserts.
	for _, item := range resolved {
		config := fillConfigDefaultLLM(ctx, s.tenantDAO, item.child.Config, &tenantID)
		if config == nil {
			config = entity.JSONMap{}
		}
		desc := item.child.Description
		if item.target != nil {
			updates := map[string]interface{}{
				"name": item.name, "kind": strings.TrimSpace(item.child.Kind), "config": config,
			}
			if desc != "" {
				updates["description"] = desc
			}
			if err := s.templateDAO.UpdateFields(ctx, db, item.target.ID, updates); err != nil {
				return err
			}
			continue
		}
		newName, err := common.UniqueName(item.name, maxTemplateNameLen, func(candidate string) (bool, error) {
			return s.templateDAO.NameExistsInGroup(ctx, db, tenantID, groupID, candidate, "")
		})
		if err != nil {
			return err
		}
		newID := common.GenerateUUID()
		valid := string(entity.StatusValid)
		tmpl := &entity.CompilationTemplate{
			ID:       newID,
			TenantID: &tenantID,
			GroupID:  &groupID,
			Name:     newName,
			Kind:     strings.TrimSpace(item.child.Kind),
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

	// Pass 4: reject the case-insensitive duplicates this update introduces.
	// Names that this update renames or creates are checked against the final
	// group, while pre-existing duplicates between untouched names are left alone
	// so a legacy group stays editable.
	oldNames := make(map[string]string, len(current))
	for _, c := range current {
		oldNames[c.ID] = c.Name
	}
	return s.assertUniqueChildNames(ctx, db, groupID, oldNames)
}

// assertUniqueChildNames rejects a group whose valid children collide on a
// case-insensitive name, matching the DAO's duplicate guard which ignores
// built-in templates. oldNames maps each child id to its name before the
// update; a collision is only an error when at least one of the colliding rows
// was renamed or newly created, so pre-existing duplicates between untouched
// names are tolerated.
func (s *CompilationTemplateGroupService) assertUniqueChildNames(ctx context.Context, db *gorm.DB, groupID string, oldNames map[string]string) error {
	children, err := s.templateDAO.ListByGroup(ctx, db, groupID)
	if err != nil {
		return err
	}
	type bucket struct {
		count      int
		anyChanged bool
		name       string
	}
	buckets := make(map[string]*bucket, len(children))
	for _, c := range children {
		if c.IsBuiltin {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(c.Name))
		b := buckets[key]
		if b == nil {
			b = &bucket{}
			buckets[key] = b
		}
		b.count++
		old, existed := oldNames[c.ID]
		if !existed || !strings.EqualFold(strings.TrimSpace(old), strings.TrimSpace(c.Name)) {
			b.anyChanged = true
			b.name = c.Name
		}
	}
	for _, b := range buckets {
		if b.count >= 2 && b.anyChanged {
			return groupValidationErrorf("template name '%s' already exists in this group.", b.name)
		}
	}
	return nil
}

func strptr(s string) *string { return &s }
