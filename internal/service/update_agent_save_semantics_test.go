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
	"testing"

	"gorm.io/gorm"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func seedSaveCanvas(t *testing.T, db *gorm.DB, userID, canvasID string) {
	t.Helper()
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate canvas tables: %v", err)
	}
	if err := db.Create(&entity.User{ID: userID, Nickname: "owner", Email: "owner@test.com"}).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.Create(&entity.UserCanvas{
		ID: canvasID, UserID: userID, Title: sptr("Save Agent"), CanvasCategory: "agent_canvas", DSL: entity.JSONMap{},
	}).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
}

func sampleDSL() map[string]interface{} {
	return map[string]interface{}{
		"graph": map[string]interface{}{
			"nodes": []interface{}{map[string]interface{}{"id": "begin"}},
			"edges": []interface{}{},
		},
		"components": map[string]interface{}{
			"begin": map[string]interface{}{"obj": map[string]interface{}{"component_name": "Begin"}},
		},
	}
}

// T32 (v5.2 §5.1): autosave updates the live draft (user_canvas.dsl) but does
// NOT create a new version.
func TestUpdateAgent_AutosaveUpdatesDraftOnly(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	userID := "user-1"
	canvasID := "canvas-autosave"
	seedSaveCanvas(t, db, userID, canvasID)
	svc := NewAgentService()
	ctx := serviceTestContext(t, userID, userID)

	if err := svc.UpdateAgent(ctx, userID, canvasID, map[string]interface{}{"dsl": sampleDSL()}); err != nil {
		t.Fatalf("manual save: %v", err)
	}
	before, err := dao.NewUserCanvasVersionDAO().ListByCanvasID(ctx, dao.DB, canvasID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(before) != 1 {
		t.Fatalf("want 1 version after manual save, got %d", len(before))
	}

	autoDSL := sampleDSL()
	autoDSL["autosave_marker"] = "yes"
	if err := svc.UpdateAgent(ctx, userID, canvasID, map[string]interface{}{"dsl": autoDSL, "autosave": true}); err != nil {
		t.Fatalf("autosave: %v", err)
	}
	after, err := dao.NewUserCanvasVersionDAO().ListByCanvasID(ctx, dao.DB, canvasID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(after) != 1 {
		t.Fatalf("autosave must not create a version, got %d", len(after))
	}
	reloaded, err := dao.NewUserCanvasDAO().GetByID(ctx, dao.DB, canvasID)
	if err != nil {
		t.Fatalf("reload canvas: %v", err)
	}
	if _, ok := reloaded.DSL["autosave_marker"]; !ok {
		t.Fatalf("autosave did not refresh the live draft: %v", reloaded.DSL)
	}
}

// T33 (v5.2 §5.1): a manual Save with the SAME DSL as the previous version
// still INSERTs a new version (no same-DSL in-place dedup).
func TestUpdateAgent_ManualSaveInsertsNewVersion(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	userID := "user-1"
	canvasID := "canvas-manual"
	seedSaveCanvas(t, db, userID, canvasID)
	svc := NewAgentService()
	ctx := serviceTestContext(t, userID, userID)

	if err := svc.UpdateAgent(ctx, userID, canvasID, map[string]interface{}{"dsl": sampleDSL()}); err != nil {
		t.Fatalf("first save: %v", err)
	}
	if err := svc.UpdateAgent(ctx, userID, canvasID, map[string]interface{}{"dsl": sampleDSL()}); err != nil {
		t.Fatalf("second save: %v", err)
	}
	versions, err := dao.NewUserCanvasVersionDAO().ListByCanvasID(ctx, dao.DB, canvasID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(versions) != 2 {
		t.Fatalf("manual saves must each mint a version, got %d", len(versions))
	}
}

// T36 (v5.2 §4.4 carry-on): a manual Save carrying an alias writes it onto the
// newly created version.
func TestUpdateAgent_ManualSaveCarriesAlias(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	userID := "user-1"
	canvasID := "canvas-alias-carry"
	seedSaveCanvas(t, db, userID, canvasID)
	svc := NewAgentService()
	ctx := serviceTestContext(t, userID, userID)

	alias := "carried-alias"
	if err := svc.UpdateAgent(ctx, userID, canvasID, map[string]interface{}{"dsl": sampleDSL(), "alias": alias}); err != nil {
		t.Fatalf("save with alias: %v", err)
	}
	latest, err := dao.NewUserCanvasVersionDAO().GetLatest(ctx, dao.DB, canvasID)
	if err != nil {
		t.Fatalf("get latest: %v", err)
	}
	if latest.Alias == nil || *latest.Alias != alias {
		t.Fatalf("carry-on alias not written: got %v", latest.Alias)
	}
}
