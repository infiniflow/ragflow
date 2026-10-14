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
	"errors"
	"testing"

	"ragflow/internal/agent/component"
	"ragflow/internal/agent/runtime"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func TestMemorySaverAuthorizesWithAuthenticatedCanvasCaller(t *testing.T) {
	setupMemoryMessageTestDB(t)
	if err := dao.DB.AutoMigrate(&entity.UserCanvas{}); err != nil {
		t.Fatalf("migrate canvas table: %v", err)
	}
	status := string(entity.StatusValid)
	canvas := entity.UserCanvas{
		ID:         "canvas-1",
		UserID:     "canvas-owner",
		Permission: string(entity.TenantPermissionTeam),
	}
	if err := dao.DB.Create(&canvas).Error; err != nil {
		t.Fatalf("seed canvas: %v", err)
	}
	for _, membership := range []*entity.UserTenant{
		{ID: "canvas-owner-membership", UserID: "canvas-owner", TenantID: "canvas-owner", Role: string(permission.RoleOwner), Status: &status},
		{ID: "caller-canvas-membership", UserID: "caller", TenantID: "canvas-owner", Role: string(permission.RoleNormal), Status: &status},
		{ID: "memory-owner-membership", UserID: "canvas-owner", TenantID: "memory-tenant", Role: string(permission.RoleNormal), Status: &status},
		{ID: "memory-tenant-owner-membership", UserID: "memory-tenant", TenantID: "memory-tenant", Role: string(permission.RoleOwner), Status: &status},
	} {
		if err := dao.DB.Create(membership).Error; err != nil {
			t.Fatalf("seed membership: %v", err)
		}
	}
	memory := entity.Memory{
		ID:               "memory-1",
		Name:             "Shared memory",
		TenantID:         "memory-tenant",
		StorageType:      "table",
		EmbdID:           "embedding",
		LLMID:            "chat",
		Permissions:      string(entity.TenantPermissionTeam),
		ForgettingPolicy: string(ForgettingPolicyFIFO),
	}
	if err := dao.DB.Create(&memory).Error; err != nil {
		t.Fatalf("seed memory: %v", err)
	}

	state := runtime.NewCanvasState("run-1", "session-1")
	state.Sys["canvas_id"] = canvas.ID
	state.Sys["user_id"] = "caller"
	ctx := runtime.WithState(t.Context(), state)

	// req.UserID is the memory-message partition key and must not be trusted as
	// the permission subject. The canvas owner can access memory-1, but caller
	// cannot, so using req.UserID here would incorrectly authorize this write.
	err := NewMemorySaverAdapter(NewMemoryService()).Save(ctx, component.MemorySaveRequest{
		MemoryIDs: []string{memory.ID},
		UserID:    "canvas-owner",
		AgentID:   canvas.ID,
	})
	if !errors.Is(err, permission.ErrPermissionDenied) {
		t.Fatalf("Save error = %v, want permission denied for authenticated caller", err)
	}
}
