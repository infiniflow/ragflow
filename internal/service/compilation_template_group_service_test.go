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
	"net/url"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

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
