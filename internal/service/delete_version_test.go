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

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

// seedVersionedDataflowCanvas inserts a dataflow canvas owned by userID with a
// single version row and a knowledgebase pinning that version. The canvas is
// owned by the requesting user so the DeleteVersion permission gate passes.
func seedVersionedDataflowCanvas(t *testing.T, db *gorm.DB, userID, canvasID, versionID string) {
	t.Helper()
	if err := db.AutoMigrate(&entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate version table: %v", err)
	}
	canvas := &entity.UserCanvas{
		ID:             canvasID,
		UserID:         userID,
		CanvasCategory: "dataflow_canvas",
		Permission:     "me",
		DSL:            entity.JSONMap{"graph": "legacy"},
	}
	if err := db.Create(canvas).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
	ver := &entity.UserCanvasVersion{
		ID:           versionID,
		UserCanvasID: canvasID,
		DSL:          entity.JSONMap{"graph": "versioned"},
	}
	if err := db.Create(ver).Error; err != nil {
		t.Fatalf("seed version: %v", err)
	}
	kb := &entity.Knowledgebase{
		ID:                "kb-" + canvasID,
		TenantID:          "tenant-1",
		Name:              "kb-" + canvasID,
		EmbdID:            "BAAI/bge-large-zh-v1.5@Builtin",
		CreatedBy:         userID,
		Permission:        string(entity.TenantPermissionMe),
		ParserID:          string(entity.ParserTypeNaive),
		ParserConfig:      entity.JSONMap{},
		Status:            sptr(string(entity.StatusValid)),
		PipelineID:        &canvasID,
		PipelineVersionID: &versionID,
	}
	if err := db.Create(kb).Error; err != nil {
		t.Fatalf("seed kb: %v", err)
	}
}

// T9: DeleteVersion reference scan. When a version is referenced by a binding,
// deletion is still allowed (design A4); the referencing binding enters the
// Missing state (design section 6.2) because the version row is gone.
func TestDeleteVersion_ReferencedVersionStillDeleted(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	userID := "user-1"
	canvasID := "0123456789abcdef0123456789abcdef"
	versionID := "version0000000000000000000000aa"
	seedVersionedDataflowCanvas(t, db, userID, canvasID, versionID)

	ctx := serviceTestContext(t, userID, userID)
	svc := NewAgentService()
	if err := svc.DeleteVersion(ctx, userID, canvasID, versionID); err != nil {
		t.Fatalf("DeleteVersion referenced version: %v", err)
	}

	// The version row must actually be removed.
	if _, err := dao.NewUserCanvasVersionDAO().GetByID(ctx, dao.DB, versionID); !errors.Is(err, dao.ErrUserCanvasVersionNotFound) {
		t.Fatalf("expected version row removed, got err=%v", err)
	}

	// The referencing binding now resolves to Missing (design section 6.2).
	if _, err := loadCanvasDSLJSON(ctx, canvasID, &versionID); !errors.Is(err, ErrPipelineVersionMissing) {
		t.Fatalf("expected Missing after referenced version delete, got err=%v", err)
	}
}

// T9b: DeleteVersion with no references deletes normally.
func TestDeleteVersion_UnreferencedVersionDeleted(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	userID := "user-1"
	canvasID := "0123456789abcdef0123456789abcdef"
	versionID := "version0000000000000000000000bb"
	// Seed canvas + version without any KB pinning it.
	if err := db.AutoMigrate(&entity.UserCanvasVersion{}); err != nil {
		t.Fatalf("migrate version table: %v", err)
	}
	canvas := &entity.UserCanvas{
		ID:             canvasID,
		UserID:         userID,
		CanvasCategory: "dataflow_canvas",
		Permission:     "me",
		DSL:            entity.JSONMap{"graph": "legacy"},
	}
	if err := db.Create(canvas).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
	ver := &entity.UserCanvasVersion{
		ID:           versionID,
		UserCanvasID: canvasID,
		DSL:          entity.JSONMap{"graph": "versioned"},
	}
	if err := db.Create(ver).Error; err != nil {
		t.Fatalf("seed version: %v", err)
	}

	ctx := serviceTestContext(t, userID, userID)
	svc := NewAgentService()
	if err := svc.DeleteVersion(ctx, userID, canvasID, versionID); err != nil {
		t.Fatalf("DeleteVersion unreferenced version: %v", err)
	}
	if _, err := dao.NewUserCanvasVersionDAO().GetByID(ctx, dao.DB, versionID); !errors.Is(err, dao.ErrUserCanvasVersionNotFound) {
		t.Fatalf("expected version row removed, got err=%v", err)
	}
}
