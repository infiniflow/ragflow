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
//

package dao

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"ragflow/internal/entity"
)

// TestMigratePermissions verifies legacy permission fields are migrated once.
func TestMigratePermissions(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.Exec(`CREATE TABLE user_tenant (
		id TEXT PRIMARY KEY, user_id TEXT NOT NULL, tenant_id TEXT NOT NULL,
		role TEXT NOT NULL, invited_by TEXT NOT NULL, status varchar(1)
	)`).Error; err != nil {
		t.Fatalf("create existing membership table: %v", err)
	}
	if err := db.Exec(`INSERT INTO user_tenant (id, user_id, tenant_id, role, invited_by, status)
		VALUES ('membership-1', 'user-1', 'tenant-1', 'normal', 'owner-1', '1')`).Error; err != nil {
		t.Fatalf("create existing membership: %v", err)
	}
	if err := db.Exec(`CREATE TABLE api_token (tenant_id varchar(32) NOT NULL, token varchar(255) NOT NULL,
		PRIMARY KEY (tenant_id, token))`).Error; err != nil {
		t.Fatalf("create existing API token table: %v", err)
	}
	if err := db.Exec(`INSERT INTO api_token (tenant_id, token) VALUES ('tenant-1', 'token-1')`).Error; err != nil {
		t.Fatalf("create existing API token: %v", err)
	}
	if err := db.Exec(`CREATE TABLE user_canvas (id varchar(32) PRIMARY KEY, user_id varchar(255) NOT NULL)`).Error; err != nil {
		t.Fatalf("create existing canvas table: %v", err)
	}
	if err := db.Exec(`INSERT INTO user_canvas (id, user_id) VALUES ('canvas-1', 'tenant-1')`).Error; err != nil {
		t.Fatalf("create existing canvas: %v", err)
	}
	if err := db.Exec(`CREATE TABLE tenant_model_provider (
		id varchar(32) PRIMARY KEY, tenant_id varchar(32) NOT NULL, provider_name varchar(128) NOT NULL
	)`).Error; err != nil {
		t.Fatalf("create legacy model provider table: %v", err)
	}
	if err := db.Exec(`INSERT INTO tenant_model_provider (id, tenant_id, provider_name)
		VALUES ('provider-1', 'tenant-1', 'openai')`).Error; err != nil {
		t.Fatalf("create legacy model provider: %v", err)
	}
	if err := db.AutoMigrate(&entity.SystemSettings{}); err != nil {
		t.Fatalf("create system settings: %v", err)
	}
	if err := db.AutoMigrate(
		&entity.UserTenant{},
		&entity.APIToken{},
		&entity.UserCanvas{},
		&entity.TenantModelProvider{},
	); err != nil {
		t.Fatalf("auto-migrate shared permission migration entities: %v", err)
	}
	if err := db.AutoMigrate(permissionDataModelsEE()...); err != nil {
		t.Fatalf("auto-migrate enterprise permission entities: %v", err)
	}
	ctx := context.Background()
	if err := setDatabaseMigrationVersion(ctx, db, tenantModelMaxTokensTargetVersion); err != nil {
		t.Fatalf("set previous migration version: %v", err)
	}
	if err := migratePermissions(ctx, db); err != nil {
		t.Fatalf("migrate permissions: %v", err)
	}
	columns, err := db.Migrator().ColumnTypes(&entity.UserTenant{})
	if err != nil {
		t.Fatalf("inspect user_tenant columns: %v", err)
	}
	for _, column := range columns {
		if column.Name() == "invited_by" {
			if nullable, known := column.Nullable(); !known || !nullable {
				t.Fatalf("user_tenant.invited_by nullable = %v, known = %v", nullable, known)
			}
		}
	}
	for _, model := range permissionDataModelsEE() {
		if !db.Migrator().HasTable(model) {
			t.Fatalf("permission migration did not create %T", model)
		}
	}
	var provider entity.TenantModelProvider
	if err := db.Take(&provider, "id = ?", "provider-1").Error; err != nil {
		t.Fatalf("read migrated model provider: %v", err)
	}
	if provider.UserID != "tenant-1" {
		t.Fatalf("model provider user_id = %q, want tenant-1", provider.UserID)
	}
	var token entity.APIToken
	if err := db.Take(&token, "tenant_id = ? AND token = ?", "tenant-1", "token-1").Error; err != nil {
		t.Fatalf("read migrated API token: %v", err)
	}
	if token.UserID == nil || *token.UserID != "tenant-1" {
		t.Fatalf("API token user_id = %v, want tenant-1", token.UserID)
	}
	var canvas entity.UserCanvas
	if err := db.Take(&canvas, "id = ?", "canvas-1").Error; err != nil {
		t.Fatalf("read migrated canvas: %v", err)
	}
	if canvas.TenantID == nil || *canvas.TenantID != "tenant-1" {
		t.Fatalf("canvas tenant_id = %v, want tenant-1", canvas.TenantID)
	}
	if err := db.Exec("INSERT INTO api_token (tenant_id, user_id, token) VALUES (?, ?, ?)", "tenant-1", "tenant-1", "token-1").Error; err == nil {
		t.Fatal("duplicate (tenant_id, token) was accepted")
	}
	if err := db.Exec("INSERT INTO api_token (tenant_id, user_id, token) VALUES (?, ?, ?)", "tenant-2", "user-2", "token-1").Error; err != nil {
		t.Fatalf("same token in another tenant was rejected: %v", err)
	}
	columns, err = db.Migrator().ColumnTypes(&entity.APIToken{})
	if err != nil {
		t.Fatalf("inspect API token columns: %v", err)
	}
	for _, column := range columns {
		if column.Name() == "user_id" {
			if nullable, known := column.Nullable(); !known || !nullable {
				t.Fatalf("api_token.user_id nullable = %v, known = %v", nullable, known)
			}
		}
	}
	version, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		t.Fatalf("get migration version: %v", err)
	}
	if version != permissionMigrationTargetVersion {
		t.Fatalf("migration version = %q, want %q", version, permissionMigrationTargetVersion)
	}
	if err := db.Exec("UPDATE user_tenant SET invited_by = NULL WHERE id = ?", "membership-1").Error; err != nil {
		t.Fatalf("clear invited_by: %v", err)
	}
	if err := db.Exec("UPDATE tenant_model_provider SET user_id = ? WHERE id = ?", "owner-user", "provider-1").Error; err != nil {
		t.Fatalf("set model provider owner: %v", err)
	}
	if err := migratePermissions(ctx, db); err != nil {
		t.Fatalf("repeat permission migration: %v", err)
	}
	if err := db.Take(&provider, "id = ?", "provider-1").Error; err != nil {
		t.Fatalf("read model provider after repeated migration: %v", err)
	}
	if provider.UserID != "owner-user" {
		t.Fatalf("repeated migration overwrote model provider user_id = %q, want owner-user", provider.UserID)
	}
}
