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
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// setupCompilationGroupTestDB migrates the minimal schema for the group
// service and pushes it onto dao.DB.
func setupCompilationGroupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	// A shared-cache memory database keeps the transaction's connection and the
	// pool's other connections on the same schema.
	dsn := "file:" + url.QueryEscape(t.Name()) + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	if err = db.AutoMigrate(
		&entity.CompilationTemplateGroup{},
		&entity.CompilationTemplate{},
		&entity.Tenant{},
	); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	pushServiceDB(t, db)
	return db
}

func seedCompilationGroup(t *testing.T, svc *CompilationTemplateGroupService, tenantID, groupName string, templateNames ...string) *GroupListItem {
	t.Helper()

	templates := make([]*GroupTemplate, 0, len(templateNames))
	for _, name := range templateNames {
		templates = append(templates, &GroupTemplate{Name: name, Kind: "text", Config: entity.JSONMap{}})
	}
	group, err := svc.CreateGroup(t.Context(), tenantID, &GroupRequest{Name: groupName, Templates: templates})
	if err != nil {
		t.Fatalf("seed group %q: %v", groupName, err)
	}
	return group
}

func templateByName(t *testing.T, group *GroupListItem, name string) *TemplateListItem {
	t.Helper()

	for _, item := range group.Templates {
		if item.Name == name {
			return item
		}
	}
	t.Fatalf("template %q not found in group %#v", name, group.Templates)
	return nil
}

// seedLegacyCompilationGroup writes a group and its children straight to the
// database, bypassing the service's case-insensitive dedupe so tests can model
// rows created before that guard existed.
func seedLegacyCompilationGroup(t *testing.T, tenantID, groupID string, names ...string) map[string]string {
	t.Helper()

	valid := string(entity.StatusValid)
	if err := dao.DB.Create(&entity.CompilationTemplateGroup{
		ID:       groupID,
		TenantID: tenantID,
		Name:     "Legacy " + groupID,
		Status:   &valid,
	}).Error; err != nil {
		t.Fatalf("seed legacy group: %v", err)
	}

	ids := make(map[string]string, len(names))
	for i, name := range names {
		id := fmt.Sprintf("%s-c%d", groupID, i)
		ids[name] = id
		if err := dao.DB.Create(&entity.CompilationTemplate{
			ID:       id,
			TenantID: &tenantID,
			GroupID:  &groupID,
			Name:     name,
			Kind:     "text",
			Config:   entity.JSONMap{},
			Status:   &valid,
		}).Error; err != nil {
			t.Fatalf("seed legacy template %q: %v", name, err)
		}
	}
	return ids
}

func templateByID(t *testing.T, group *GroupListItem, id string) *TemplateListItem {
	t.Helper()

	for _, item := range group.Templates {
		if item.ID == id {
			return item
		}
	}
	t.Fatalf("template %s not found in group %#v", id, group.Templates)
	return nil
}

// CreateGroup must not write the deduped name back into the caller's request,
// otherwise a reused request would create "Group A(1)(1)" style names.
func TestCreateGroupDoesNotMutateRequestName(t *testing.T) {
	setupCompilationGroupTestDB(t)
	svc := NewCompilationTemplateGroupService()
	ctx := t.Context()

	req := &GroupRequest{
		Name:      "Group A",
		Templates: []*GroupTemplate{{Name: "tpl", Kind: "text", Config: entity.JSONMap{}}},
	}

	first, err := svc.CreateGroup(ctx, "tenant-1", req)
	if err != nil {
		t.Fatalf("first CreateGroup: %v", err)
	}
	if first.Name != "Group A" {
		t.Fatalf("first group name = %q, want %q", first.Name, "Group A")
	}
	if req.Name != "Group A" {
		t.Fatalf("CreateGroup mutated req.Name to %q", req.Name)
	}

	second, err := svc.CreateGroup(ctx, "tenant-1", req)
	if err != nil {
		t.Fatalf("second CreateGroup: %v", err)
	}
	if second.Name != "Group A(1)" {
		t.Fatalf("second group name = %q, want %q", second.Name, "Group A(1)")
	}
	if req.Name != "Group A" {
		t.Fatalf("CreateGroup mutated req.Name to %q", req.Name)
	}
}

// A child may take the name of a child that leaves the group in the same
// update; the removed row must not be treated as a surviving conflict.
func TestReconcileChildrenAllowsRenameOntoRemovedName(t *testing.T) {
	setupCompilationGroupTestDB(t)
	svc := NewCompilationTemplateGroupService()
	ctx := t.Context()

	group := seedCompilationGroup(t, svc, "tenant-1", "Group A", "A", "B")
	b := templateByName(t, group, "B")

	updated, err := svc.UpdateGroup(ctx, "tenant-1", group.ID, &GroupRequest{
		Templates: []*GroupTemplate{{ID: b.ID, Name: "A", Kind: "text", Config: entity.JSONMap{}}},
	})
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if len(updated.Templates) != 1 {
		t.Fatalf("templates = %#v, want exactly one", updated.Templates)
	}
	if updated.Templates[0].Name != "A" {
		t.Fatalf("template name = %q, want %q", updated.Templates[0].Name, "A")
	}
}

// Swapping two child names in one update settles on unique names, so it must
// not be rejected by the duplicate-name guard.
func TestReconcileChildrenAllowsNameSwap(t *testing.T) {
	setupCompilationGroupTestDB(t)
	svc := NewCompilationTemplateGroupService()
	ctx := t.Context()

	group := seedCompilationGroup(t, svc, "tenant-1", "Group A", "A", "B")
	a := templateByName(t, group, "A")
	b := templateByName(t, group, "B")

	updated, err := svc.UpdateGroup(ctx, "tenant-1", group.ID, &GroupRequest{
		Templates: []*GroupTemplate{
			{ID: a.ID, Name: "B", Kind: "text", Config: entity.JSONMap{}},
			{ID: b.ID, Name: "A", Kind: "text", Config: entity.JSONMap{}},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGroup: %v", err)
	}
	if got := templateByID(t, updated, a.ID).Name; got != "B" {
		t.Fatalf("template %s name = %q, want %q", a.ID, got, "B")
	}
	if got := templateByID(t, updated, b.ID).Name; got != "A" {
		t.Fatalf("template %s name = %q, want %q", b.ID, got, "A")
	}
}

// Two surviving children must not end up with names that differ only by case.
func TestReconcileChildrenRejectsDuplicateSurvivingName(t *testing.T) {
	setupCompilationGroupTestDB(t)
	svc := NewCompilationTemplateGroupService()
	ctx := t.Context()

	group := seedCompilationGroup(t, svc, "tenant-1", "Group A", "A", "B")
	a := templateByName(t, group, "A")
	b := templateByName(t, group, "B")

	_, err := svc.UpdateGroup(ctx, "tenant-1", group.ID, &GroupRequest{
		Templates: []*GroupTemplate{
			{ID: a.ID, Name: "b", Kind: "text", Config: entity.JSONMap{}},
			{ID: b.ID, Name: "B", Kind: "text", Config: entity.JSONMap{}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "already exists in this group") {
		t.Fatalf("UpdateGroup error = %v, want duplicate-name error", err)
	}
}

// A group that already carries case-variant duplicate child names (created
// before the case-insensitive guard, on a case-sensitive collation) must stay
// editable as long as the update does not touch those names.
func TestReconcileChildrenAllowsLegacyCaseVariantDuplicates(t *testing.T) {
	setupCompilationGroupTestDB(t)
	svc := NewCompilationTemplateGroupService()
	ctx := t.Context()

	ids := seedLegacyCompilationGroup(t, "tenant-1", "legacy-group", "A", "a")

	updated, err := svc.UpdateGroup(ctx, "tenant-1", "legacy-group", &GroupRequest{
		Templates: []*GroupTemplate{
			{ID: ids["A"], Name: "A", Kind: "text", Config: entity.JSONMap{}},
			{ID: ids["a"], Name: "a", Kind: "text", Config: entity.JSONMap{}},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGroup with legacy case-variant names: %v", err)
	}
	if len(updated.Templates) != 2 {
		t.Fatalf("templates = %#v, want two", updated.Templates)
	}
}

// A rename that lands on an untouched legacy name is a real collision and must
// still be rejected.
func TestReconcileChildrenRejectsRenameOntoUnchangedLegacyName(t *testing.T) {
	setupCompilationGroupTestDB(t)
	svc := NewCompilationTemplateGroupService()
	ctx := t.Context()

	ids := seedLegacyCompilationGroup(t, "tenant-1", "legacy-group-2", "A", "a", "B")

	_, err := svc.UpdateGroup(ctx, "tenant-1", "legacy-group-2", &GroupRequest{
		Templates: []*GroupTemplate{
			{ID: ids["A"], Name: "b", Kind: "text", Config: entity.JSONMap{}},
			{ID: ids["a"], Name: "a", Kind: "text", Config: entity.JSONMap{}},
			{ID: ids["B"], Name: "B", Kind: "text", Config: entity.JSONMap{}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "already exists in this group") {
		t.Fatalf("UpdateGroup error = %v, want duplicate-name error", err)
	}
}

// A single-template group mirrors the fallback group name onto its child, so the
// client-submitted template name (which it also sends as the group name) does not
// collide with a sibling group on update.
func TestCreateGroupSingleTemplateSharesFallbackName(t *testing.T) {
	setupCompilationGroupTestDB(t)
	svc := NewCompilationTemplateGroupService()
	ctx := t.Context()

	first := seedCompilationGroup(t, svc, "tenant-1", "Group A", "tpl")
	if first.Templates[0].Name != "Group A" {
		t.Fatalf("first template name = %q, want %q", first.Templates[0].Name, "Group A")
	}

	second := seedCompilationGroup(t, svc, "tenant-1", "Group A", "tpl")
	if second.Name != "Group A(1)" {
		t.Fatalf("second group name = %q, want %q", second.Name, "Group A(1)")
	}
	if second.Templates[0].Name != "Group A(1)" {
		t.Fatalf("second template name = %q, want %q", second.Templates[0].Name, "Group A(1)")
	}

	updated, err := svc.UpdateGroup(ctx, "tenant-1", second.ID, &GroupRequest{
		Name: "Group A(1)",
		Templates: []*GroupTemplate{
			{ID: second.Templates[0].ID, Name: "Group A(1)", Kind: "text", Config: entity.JSONMap{}},
		},
	})
	if err != nil {
		t.Fatalf("UpdateGroup after fallback: %v", err)
	}
	if updated.Name != "Group A(1)" {
		t.Fatalf("updated group name = %q, want %q", updated.Name, "Group A(1)")
	}
	if updated.Templates[0].Name != "Group A(1)" {
		t.Fatalf("updated template name = %q, want %q", updated.Templates[0].Name, "Group A(1)")
	}
}

// Multi-template groups keep their own per-group deduped child names; only a
// single-template group mirrors the group name.
func TestCreateGroupMultiTemplateKeepsChildNames(t *testing.T) {
	setupCompilationGroupTestDB(t)
	svc := NewCompilationTemplateGroupService()

	group := seedCompilationGroup(t, svc, "tenant-1", "Group A", "A", "B")
	if group.Templates[0].Name != "A" {
		t.Fatalf("first template name = %q, want %q", group.Templates[0].Name, "A")
	}
	if group.Templates[1].Name != "B" {
		t.Fatalf("second template name = %q, want %q", group.Templates[1].Name, "B")
	}
}
