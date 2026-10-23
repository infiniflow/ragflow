//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package service

import (
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestSkillSpacePermissionsUsePolicyAcrossServiceOperations(t *testing.T) {
	db := setupServiceTestDB(t)
	if err := db.AutoMigrate(&entity.SkillSpace{}); err != nil {
		t.Fatalf("migrate skill spaces: %v", err)
	}
	pushServiceDB(t, db)

	active := string(entity.StatusValid)
	for _, membership := range []entity.UserTenant{
		{ID: "skill-owner-membership", UserID: "owner", TenantID: "team", Role: string(permission.RoleOwner), Status: &active},
		{ID: "skill-member-membership", UserID: "member", TenantID: "team", Role: string(permission.RoleNormal), InvitedBy: "owner", Status: &active},
		{ID: "skill-member-personal-owner", UserID: "member", TenantID: "member", Role: string(permission.RoleOwner), InvitedBy: "member", Status: &active},
		{ID: "skill-outsider-membership", UserID: "outsider", TenantID: "other", Role: string(permission.RoleOwner), Status: &active},
	} {
		if err := db.Create(&membership).Error; err != nil {
			t.Fatalf("create membership %q: %v", membership.ID, err)
		}
	}
	folder := entity.File{
		ID: "skill-folder", ParentID: "root", TenantID: "team", CreatedBy: "owner",
		Name: "shared", Type: "folder", SourceType: "skill_space",
	}
	if err := db.Create(&folder).Error; err != nil {
		t.Fatalf("create skill folder: %v", err)
	}
	space := entity.SkillSpace{
		ID: "shared-space", TenantID: "team", Name: "shared", FolderID: folder.ID,
		Status: entity.SpaceStatusActive,
	}
	if err := db.Create(&space).Error; err != nil {
		t.Fatalf("create skill space: %v", err)
	}

	service := NewSkillSpaceService(nil)
	ctx := t.Context()

	listed, code, err := service.ListSpaces(ctx, "member")
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("member list: code=%d err=%v", code, err)
	}
	spaces, ok := listed["spaces"].([]map[string]interface{})
	if !ok || len(spaces) != 1 || spaces[0]["id"] != space.ID {
		t.Fatalf("member list result = %#v, want only %q", listed["spaces"], space.ID)
	}

	if _, code, err := service.GetSpace(ctx, "member", space.ID); err != nil || code != common.CodeSuccess {
		t.Fatalf("member read: code=%d err=%v", code, err)
	}
	if _, code, err := service.UpdateSpace(ctx, "member", space.ID, &UpdateSpaceRequest{Description: "forbidden"}); err == nil || code != common.CodeNotFound || !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("member update should be hidden: code=%d err=%v", code, err)
	}
	if code, err := service.DeleteSpace(ctx, "member", space.ID, nil); err == nil || code != common.CodeNotFound || !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("member delete should be hidden: code=%d err=%v", code, err)
	}
	if _, code, err := service.GetSpace(ctx, "outsider", space.ID); err == nil || code != common.CodeNotFound || !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("outsider read should be hidden: code=%d err=%v", code, err)
	}

	created, code, err := service.CreateSpace(ctx, "member", &CreateSpaceRequest{Name: "personal"})
	if err != nil || code != common.CodeSuccess || created["tenant_id"] != "member" {
		t.Fatalf("member should create a personal skill space without switching tenants: result=%#v code=%d err=%v", created, code, err)
	}
	if tenantID, err := resolveSkillSpaceTenantID(ctx, "owner", "default", permission.OperationUse); err != nil || tenantID != "owner" {
		t.Fatalf("default skill space tenant = %q, err=%v; want owner's personal tenant", tenantID, err)
	}

	updated, code, err := service.UpdateSpace(ctx, "owner", space.ID, &UpdateSpaceRequest{Description: "updated"})
	if err != nil || code != common.CodeSuccess || updated["description"] != "updated" {
		t.Fatalf("owner update: result=%#v code=%d err=%v", updated, code, err)
	}

	indexer := NewSkillIndexerService()
	if err := indexer.BatchIndexSkills(ctx, "member", space.ID, nil, nil, ""); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("member indexing should be denied before engine access, got %v", err)
	}
	if err := indexer.DeleteSkillIndex(ctx, "member", space.ID, "skill", nil); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("member index deletion should be denied before engine access, got %v", err)
	}
	if _, _, err := NewSkillSearchService().UpdateConfig(ctx, "member", &UpdateConfigRequest{
		SpaceID: space.ID, EmbdID: "model", VectorSimilarityWeight: 0.5,
		SimilarityThreshold: 0.2, TopK: 10,
	}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("member config update should be denied, got %v", err)
	}
	if _, _, err := NewSkillSearchService().Search(ctx, "outsider", &SearchRequest{SpaceID: space.ID}, nil); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("outsider search should be denied before engine access, got %v", err)
	}
}
