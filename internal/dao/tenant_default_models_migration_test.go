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

package dao

import (
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

func TestMigrateTenantDefaultModelsOnlyMovesValuesToOwner(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&entity.SystemSettings{}, &entity.UserTenant{}); err != nil {
		t.Fatalf("migrate destination tables: %v", err)
	}

	columns := append([]string{"id"}, tenantDefaultModelColumns...)
	definitions := "id TEXT PRIMARY KEY"
	for _, column := range tenantDefaultModelColumns {
		definitions += ", " + column + " TEXT"
	}
	if err := db.Exec("CREATE TABLE tenant (" + definitions + ")").Error; err != nil {
		t.Fatalf("create legacy tenant table: %v", err)
	}

	for _, tenantID := range []string{"tenant-1", "tenant-2"} {
		placeholders := "?"
		args := []any{tenantID}
		for _, column := range tenantDefaultModelColumns {
			placeholders += ", ?"
			args = append(args, tenantID+"-"+column)
		}
		query := "INSERT INTO tenant (" + strings.Join(columns, ", ") + ") VALUES (" + placeholders + ")"
		if err := db.Exec(query, args...).Error; err != nil {
			t.Fatalf("seed legacy tenant %q: %v", tenantID, err)
		}
	}

	joined := entity.TenantMembershipStatusJoined
	memberships := []entity.UserTenant{
		{ID: "owner-role", UserID: "owner-user", TenantID: "tenant-1", Role: "owner", Status: &joined},
		{ID: "member-1", UserID: "member-user", TenantID: "tenant-1", Role: "member-role-id", Status: &joined},
		{ID: "owner-id-match", UserID: "tenant-2", TenantID: "tenant-2", Role: "owner-role-id", Status: &joined},
		{ID: "member-2", UserID: "member-user-2", TenantID: "tenant-2", Role: "member-role-id", Status: &joined},
	}
	if err := db.Create(&memberships).Error; err != nil {
		t.Fatalf("seed memberships: %v", err)
	}
	if err := setDatabaseMigrationVersion(t.Context(), db, "v1.0.0-rc3.dev4"); err != nil {
		t.Fatalf("set previous migration version: %v", err)
	}

	if err := migrateTenantDefaultModels(t.Context(), db); err != nil {
		t.Fatalf("migrate tenant default models: %v", err)
	}

	for _, membership := range []struct {
		id       string
		tenantID string
		migrated bool
	}{
		{id: "owner-role", tenantID: "tenant-1", migrated: true},
		{id: "member-1", tenantID: "tenant-1"},
		{id: "owner-id-match", tenantID: "tenant-2", migrated: true},
		{id: "member-2", tenantID: "tenant-2"},
	} {
		var got entity.UserTenant
		if err := db.Where("id = ?", membership.id).Take(&got).Error; err != nil {
			t.Fatalf("load membership %q: %v", membership.id, err)
		}
		wantLLMID := ""
		if membership.migrated {
			wantLLMID = membership.tenantID + "-llm_id"
		}
		if got.LLMID != wantLLMID {
			t.Errorf("membership %q llm_id = %q, want %q", membership.id, got.LLMID, wantLLMID)
		}
		if membership.migrated {
			wantTenantLLMID := membership.tenantID + "-tenant_llm_id"
			if got.TenantLLMID == nil || *got.TenantLLMID != wantTenantLLMID {
				t.Errorf("membership %q tenant_llm_id = %v, want %q", membership.id, got.TenantLLMID, wantTenantLLMID)
			}
		} else if got.TenantLLMID != nil && *got.TenantLLMID != "" {
			t.Errorf("non-owner membership %q tenant_llm_id = %q, want empty", membership.id, *got.TenantLLMID)
		}
	}

	for _, column := range tenantDefaultModelColumns {
		exists, err := tenantDefaultModelColumnExists(db.Migrator(), column)
		if err != nil {
			t.Fatalf("inspect legacy tenant.%s: %v", column, err)
		}
		if exists {
			t.Errorf("legacy tenant.%s column still exists", column)
		}
	}

	version, err := GetDatabaseMigrationVersion(t.Context(), db)
	if err != nil {
		t.Fatalf("read migration version: %v", err)
	}
	if version != tenantDefaultModelsTargetVersion {
		t.Fatalf("migration version = %q, want %q", version, tenantDefaultModelsTargetVersion)
	}
	if err := migrateTenantDefaultModels(t.Context(), db); err != nil {
		t.Fatalf("repeat migration: %v", err)
	}
}
