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

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// T35 (v5.2 alias write, design G2/§4.4): PATCH /versions/{id} with alias
// updates that version's alias metadata, does NOT create a new version, and
// does NOT change the DSL.
func TestUpdateVersion_PatchesAliasWithoutNewVersion(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	userID := "user-1"
	canvasID := "0123456789abcdef0123456789abcdef"
	versionID := "version0000000000000000000000cc"
	seedVersionedDataflowCanvas(t, db, userID, canvasID, versionID)

	alias := "release-candidate"
	ctx := serviceTestContext(t, userID, userID)
	svc := NewAgentService()
	patch := dao.UserCanvasVersionPatch{Alias: &alias}
	if err := svc.UpdateVersion(ctx, userID, canvasID, versionID, patch); err != nil {
		t.Fatalf("UpdateVersion alias: %v", err)
	}

	updated, err := dao.NewUserCanvasVersionDAO().GetByID(ctx, dao.DB, versionID)
	if err != nil {
		t.Fatalf("reload version: %v", err)
	}
	if updated.Alias == nil || *updated.Alias != alias {
		t.Fatalf("alias not updated: got %v", updated.Alias)
	}
	if g, _ := updated.DSL["graph"].(string); g != "versioned" {
		t.Fatalf("DSL changed unexpectedly: %v", updated.DSL)
	}
	all, err := dao.NewUserCanvasVersionDAO().ListByCanvasID(ctx, dao.DB, canvasID)
	if err != nil {
		t.Fatalf("list versions: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("expected exactly 1 version, got %d", len(all))
	}
}

// T35 ownership: patching a version that belongs to a different canvas is
// rejected (cannot cross-canvas edit alias).
func TestUpdateVersion_CrossCanvasRejected(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	userID := "user-1"
	canvasID := "0123456789abcdef0123456789abcdef"
	otherCanvasID := "fedcba9876543210fedcba9876543210"
	versionID := "version0000000000000000000000dd"
	seedVersionedDataflowCanvas(t, db, userID, canvasID, versionID)

	if err := db.AutoMigrate(&entity.UserCanvas{}); err != nil {
		t.Fatalf("migrate canvas: %v", err)
	}
	other := &entity.UserCanvas{ID: otherCanvasID, UserID: userID, CanvasCategory: "dataflow_canvas", Permission: "me", DSL: entity.JSONMap{"graph": "legacy"}}
	if err := db.Create(other).Error; err != nil {
		t.Fatalf("seed other canvas: %v", err)
	}

	alias := "hijack"
	ctx := serviceTestContext(t, userID, userID)
	svc := NewAgentService()
	patch := dao.UserCanvasVersionPatch{Alias: &alias}
	err := svc.UpdateVersion(ctx, userID, otherCanvasID, versionID, patch)
	if !errors.Is(err, dao.ErrUserCanvasVersionNotFound) {
		t.Fatalf("expected ErrUserCanvasVersionNotFound, got %v", err)
	}
}
