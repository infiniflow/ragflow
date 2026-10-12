package component

import (
	"errors"
	"net/url"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	"ragflow/internal/permission"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCheckCanvasRetrievalDependencies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.Knowledgebase{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	valid := string(entity.StatusValid)
	canvas := entity.UserCanvas{ID: "canvas-1", UserID: "canvas-owner", Permission: string(entity.TenantPermissionTeam)}
	dataset := entity.Knowledgebase{
		ID: "dataset-1", TenantID: "canvas-owner", CreatedBy: "canvas-owner",
		Permission: string(entity.TenantPermissionTeam), Status: &valid,
	}
	privateDataset := entity.Knowledgebase{
		ID: "private-dataset", TenantID: "other-owner", CreatedBy: "other-owner",
		Permission: string(entity.TenantPermissionMe), Status: &valid,
	}
	for _, row := range []any{&canvas, &dataset, &privateDataset} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create permission resource: %v", err)
		}
	}
	memberships := []entity.UserTenant{
		{ID: "canvas-owner-membership", UserID: "canvas-owner", TenantID: "canvas-owner", Role: string(permission.RoleOwner), Status: &valid},
		{ID: "caller-membership", UserID: "caller", TenantID: "canvas-owner", Role: string(permission.RoleNormal), Status: &valid},
		{ID: "dataset-owner-membership", UserID: "other-owner", TenantID: "other-owner", Role: string(permission.RoleOwner), Status: &valid},
	}
	for i := range memberships {
		if err := db.Create(&memberships[i]).Error; err != nil {
			t.Fatalf("create membership: %v", err)
		}
	}

	state := runtime.NewCanvasState("run-1", "session-1")
	state.Sys["canvas_id"] = canvas.ID
	state.Sys["user_id"] = "caller"
	ctx := runtime.WithState(t.Context(), state)

	if err := checkCanvasRetrievalDependencies(ctx, db, map[string]any{"dataset_ids": []string{dataset.ID}}); err != nil {
		t.Fatalf("check accessible dataset dependency: %v", err)
	}
	if err := checkCanvasRetrievalDependencies(ctx, db, map[string]any{"dataset_ids": []string{privateDataset.ID}}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("check private dataset dependency error = %v, want permission denied", err)
	}

	missingCanvasState := runtime.NewCanvasState("run-2", "session-2")
	missingCanvasState.Sys["user_id"] = "caller"
	if err := checkCanvasRetrievalDependencies(runtime.WithState(t.Context(), missingCanvasState), db, nil); !errors.Is(err, permission.ErrInvalidPermission) {
		t.Fatalf("check missing canvas identity error = %v, want invalid permission", err)
	}
}
