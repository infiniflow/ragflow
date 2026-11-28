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

package tool

import (
	"context"
	"fmt"
	"testing"

	"ragflow/internal/agent/runtime"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/permission/testsupport"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupCanvasBatchDB(t *testing.T, count int) (*gorm.DB, []string) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.Knowledgebase{}, &entity.UserCanvas{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate tables: %v", err)
	}
	status := string(entity.StatusValid)
	tenant := "owner"
	if err := db.Create(&entity.UserCanvas{
		ID: "canvas-1", UserID: "owner", TenantID: &tenant,
		Permission: string(entity.TenantPermissionMe),
	}).Error; err != nil {
		t.Fatalf("create canvas: %v", err)
	}
	ids := make([]string, 0, count)
	for i := 0; i < count; i++ {
		id := fmt.Sprintf("canvas-kb-%02d", i)
		ids = append(ids, id)
		if err := db.Create(&entity.Knowledgebase{
			ID: id, TenantID: tenant, Name: id,
			Permission: string(entity.TenantPermissionMe), CreatedBy: "owner", Status: &status,
		}).Error; err != nil {
			t.Fatalf("create dataset %s: %v", id, err)
		}
	}
	if err := db.Create(&entity.UserTenant{
		ID: "owner-membership", UserID: "owner", TenantID: tenant,
		Role: string(permission.RoleOwner), InvitedBy: "owner", Status: &status,
	}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	if err := testsupport.PrepareEnterprisePermissionFixture(db, tenant, "owner"); err != nil {
		t.Fatalf("prepare permission fixture: %v", err)
	}
	return db, ids
}

func canvasBatchContext(t *testing.T) context.Context {
	t.Helper()
	state := runtime.NewCanvasState("run-1", "session-1")
	state.Sys["canvas_id"] = "canvas-1"
	state.Sys["user_id"] = "owner"
	state.Sys["tenant_id"] = "owner"
	return permission.WithIdentity(runtime.WithState(t.Context(), state), "owner", "owner")
}

func TestCheckCanvasDependenciesBatchesResourceLoads(t *testing.T) {
	db, ids := setupCanvasBatchDB(t, 5)
	capture := &retrievalBatchSQLCapture{}
	captureDB := db.Session(&gorm.Session{Logger: capture})

	if err := CheckCanvasDependencies(canvasBatchContext(t), captureDB, permission.ResourceKindDataset, ids); err != nil {
		t.Fatalf("CheckCanvasDependencies: %v", err)
	}
	if n, limit := capture.knowledgebaseSelects(), 2*testsupport.DatasetPermissionQueryCount(len(ids)); n > limit {
		t.Fatalf("knowledgebase SELECTs = %d for %d dependencies, want <= %d; got SQL: %v", n, len(ids), limit, capture.sqls)
	}
}

// Adversarial: a dependency the caller cannot use is still rejected.
func TestCheckCanvasDependenciesRejectsUnauthorizedDependency(t *testing.T) {
	db, ids := setupCanvasBatchDB(t, 2)
	status := string(entity.StatusValid)
	foreign := "canvas-foreign-kb"
	if err := db.Create(&entity.Knowledgebase{
		ID: foreign, TenantID: "foreign", Name: foreign,
		Permission: string(entity.TenantPermissionMe), CreatedBy: "foreign", Status: &status,
	}).Error; err != nil {
		t.Fatal(err)
	}

	err := CheckCanvasDependencies(canvasBatchContext(t), db, permission.ResourceKindDataset, []string{ids[0], foreign, ids[1]})
	if err == nil {
		t.Fatal("expected permission error for foreign dependency")
	}
}
