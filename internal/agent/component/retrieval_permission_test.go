package component

import (
	"errors"
	"net/url"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/permission/testsupport"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCheckCanvasRetrievalDependencies(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+url.QueryEscape(t.Name())+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserCanvas{}, &entity.Knowledgebase{}, &entity.Memory{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate permission tables: %v", err)
	}

	valid := string(entity.StatusValid)
	canvas := entity.UserCanvas{ID: "canvas-1", UserID: "canvas-owner", Permission: string(entity.TenantPermissionTeam)}
	dataset := entity.Knowledgebase{
		ID: "dataset-1", TenantID: "canvas-owner", CreatedBy: "canvas-owner",
		Permission: string(entity.TenantPermissionTeam), Status: &valid,
	}
	privateDataset := entity.Knowledgebase{
		ID: "private-dataset", TenantID: "canvas-owner", CreatedBy: "other-owner",
		Permission: string(entity.TenantPermissionMe), Status: &valid,
	}
	sharedMemory := entity.Memory{
		ID: "shared-memory", Name: "Shared", TenantID: "canvas-owner",
		Permissions: string(entity.TenantPermissionTeam),
	}
	callerOnlyMemory := entity.Memory{
		ID: "caller-only-memory", Name: "Caller only", TenantID: "canvas-owner",
		Permissions: string(entity.TenantPermissionMe),
	}
	ownerOnlyMemory := entity.Memory{
		ID: "owner-only-memory", Name: "Owner only", TenantID: "canvas-owner",
		Permissions: string(entity.TenantPermissionMe),
	}
	for _, row := range []any{&canvas, &dataset, &privateDataset, &sharedMemory, &callerOnlyMemory, &ownerOnlyMemory} {
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
	if err := testsupport.PrepareEnterprisePermissionFixture(db, "canvas-owner", "canvas-owner", "caller", "other-owner"); err != nil {
		t.Fatalf("prepare permission fixture: %v", err)
	}

	state := runtime.NewCanvasState("run-1", "session-1")
	state.Sys["canvas_id"] = canvas.ID
	state.Sys["user_id"] = "caller"
	state.Sys["tenant_id"] = "canvas-owner"
	ctx := permission.WithSubject(runtime.WithState(t.Context(), state), permission.Subject{UserID: "caller", TenantID: "canvas-owner"})

	if err := checkCanvasRetrievalDependencies(ctx, db, map[string]any{"dataset_ids": []string{dataset.ID}}); err != nil {
		t.Fatalf("check accessible dataset dependency: %v", err)
	}
	if err := checkCanvasRetrievalDependencies(ctx, db, map[string]any{"dataset_ids": []string{privateDataset.ID}}); !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("check private dataset dependency error = %v, want permission denied", err)
	}
	if err := checkCanvasRetrievalDependencies(ctx, db, map[string]any{"memory_ids": []string{sharedMemory.ID}}); err != nil {
		t.Fatalf("check shared memory dependency: %v", err)
	}
	if err := checkCanvasRetrievalDependencies(ctx, db, map[string]any{
		"retrieval_from": "memory",
		"dataset_ids":    []string{privateDataset.ID},
		"memory_ids":     []string{sharedMemory.ID},
	}); err != nil {
		t.Fatalf("memory retrieval should ignore inactive dataset bindings: %v", err)
	}
	if err := checkCanvasRetrievalDependencies(ctx, db, map[string]any{
		"retrieval_from": "dataset",
		"dataset_ids":    []string{dataset.ID},
		"memory_ids":     []string{ownerOnlyMemory.ID},
	}); err != nil {
		t.Fatalf("dataset retrieval should ignore inactive memory bindings: %v", err)
	}
	for _, memoryID := range []string{callerOnlyMemory.ID, ownerOnlyMemory.ID} {
		if err := checkCanvasRetrievalDependencies(ctx, db, map[string]any{"memory_ids": []string{memoryID}}); !errors.Is(err, permission.ErrPermissionDenied) {
			t.Fatalf("check non-intersection memory %q error = %v, want permission denied", memoryID, err)
		}
	}

	missingCanvasState := runtime.NewCanvasState("run-2", "session-2")
	missingCanvasState.Sys["user_id"] = "caller"
	if err := checkCanvasRetrievalDependencies(runtime.WithState(t.Context(), missingCanvasState), db, nil); !errors.Is(err, permission.ErrInvalidPermission) {
		t.Fatalf("check missing canvas identity error = %v, want invalid permission", err)
	}
}
