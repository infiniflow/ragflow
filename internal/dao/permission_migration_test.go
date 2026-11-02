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
		VALUES ('membership-1', 'user-1', 'tenant-1', 'normal', 'owner-1', '1'),
		('membership-owner', 'tenant-1', 'tenant-1', 'owner', '', '1')`).Error; err != nil {
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
	if err := setDatabaseMigrationVersion(ctx, db, "v1.0.0-rc3.dev2"); err != nil {
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
	if err := db.Exec("UPDATE tenant_model_provider SET user_id = '' WHERE id = ?", "provider-1").Error; err != nil {
		t.Fatalf("clear model provider owner: %v", err)
	}
	if err := db.Exec("UPDATE api_token SET user_id = '' WHERE tenant_id = ? AND token = ?", "tenant-1", "token-1").Error; err != nil {
		t.Fatalf("clear API token owner: %v", err)
	}
	if err := db.Exec("UPDATE user_canvas SET tenant_id = '' WHERE id = ?", "canvas-1").Error; err != nil {
		t.Fatalf("clear canvas tenant: %v", err)
	}
	if err := migratePermissions(ctx, db); err != nil {
		t.Fatalf("repeat permission migration: %v", err)
	}
	if err := db.Take(&provider, "id = ?", "provider-1").Error; err != nil {
		t.Fatalf("read model provider after repeated migration: %v", err)
	}
	if provider.UserID != "" {
		t.Fatalf("repeated migration backfilled model provider user_id = %q, want it unchanged", provider.UserID)
	}
	if err := db.Take(&token, "tenant_id = ? AND token = ?", "tenant-1", "token-1").Error; err != nil {
		t.Fatalf("read API token after repeated migration: %v", err)
	}
	if token.UserID == nil || *token.UserID != "" {
		t.Fatalf("repeated migration backfilled API token user_id = %v, want it unchanged", token.UserID)
	}
	if err := db.Take(&canvas, "id = ?", "canvas-1").Error; err != nil {
		t.Fatalf("read canvas after repeated migration: %v", err)
	}
	if canvas.TenantID == nil || *canvas.TenantID != "" {
		t.Fatalf("repeated migration backfilled canvas tenant_id = %v, want it unchanged", canvas.TenantID)
	}
}

func TestMigrateUserTenantDuplicates(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserTenant{}); err != nil {
		t.Fatalf("create user_tenant table: %v", err)
	}
	if err := db.Migrator().DropIndex(&entity.UserTenant{}, "idx_user_tenant_user_tenant"); err != nil {
		t.Fatalf("drop user_tenant unique index: %v", err)
	}
	for _, statement := range []string{
		`CREATE TABLE group_info (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL)`,
		`CREATE TABLE group_member (id TEXT PRIMARY KEY, member_id TEXT NOT NULL)`,
		`CREATE TABLE department_member (id TEXT PRIMARY KEY, member_id TEXT NOT NULL)`,
		`CREATE TABLE permission (id TEXT PRIMARY KEY, member_id TEXT)`,
		`INSERT INTO user_tenant (id, user_id, tenant_id, role, invited_by, status) VALUES
			('member-normal', 'user-1', 'tenant-1', 'normal', 'user-1', '1'),
			('member-owner', 'user-1', 'tenant-1', 'owner', 'user-1', '1')`,
		`INSERT INTO group_info (id, owner_id) VALUES ('group-1', 'member-normal')`,
		`INSERT INTO group_member (id, member_id) VALUES ('group-member-1', 'member-normal')`,
		`INSERT INTO department_member (id, member_id) VALUES ('department-member-1', 'member-normal')`,
		`INSERT INTO permission (id, member_id) VALUES ('permission-1', 'member-normal')`,
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("execute test setup statement: %v", err)
		}
	}

	if err := migrateUserTenantDuplicates(context.Background(), db); err != nil {
		t.Fatalf("deduplicate user_tenant memberships: %v", err)
	}
	var memberships []entity.UserTenant
	if err := db.Find(&memberships).Error; err != nil {
		t.Fatalf("read user_tenant memberships: %v", err)
	}
	if len(memberships) != 1 || memberships[0].ID != "member-owner" {
		t.Fatalf("kept user_tenant memberships = %+v, want only owner membership", memberships)
	}
	for _, reference := range []struct {
		table string
		id    string
	}{
		{table: "group_info", id: "group-1"},
		{table: "group_member", id: "group-member-1"},
		{table: "department_member", id: "department-member-1"},
		{table: "permission", id: "permission-1"},
	} {
		var row struct {
			OwnerID  string `gorm:"column:owner_id"`
			MemberID string `gorm:"column:member_id"`
		}
		if err := db.Table(reference.table).Where("id = ?", reference.id).Take(&row).Error; err != nil {
			t.Fatalf("read %s reference: %v", reference.table, err)
		}
		memberID := row.MemberID
		if memberID == "" {
			memberID = row.OwnerID
		}
		if memberID != "member-owner" {
			t.Fatalf("%s membership reference = %q, want member-owner", reference.table, memberID)
		}
	}
	if err := db.AutoMigrate(&entity.UserTenant{}); err != nil {
		t.Fatalf("auto-migrate unique user_tenant index: %v", err)
	}
	if err := db.Exec("INSERT INTO user_tenant (id, user_id, tenant_id, role) VALUES (?, ?, ?, ?)", "member-duplicate", "user-1", "tenant-1", "normal").Error; err == nil {
		t.Fatal("duplicate user_tenant membership was accepted after AutoMigrate")
	}
}

func TestMigrateLegacyTenantInvitationStatuses(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	if err := db.AutoMigrate(&entity.UserTenant{}); err != nil {
		t.Fatalf("auto-migrate user_tenant: %v", err)
	}
	joined := entity.TenantMembershipStatusJoined
	if err := db.Create(&entity.UserTenant{
		ID: "invitation-1", UserID: "user-1", TenantID: "tenant-1", Role: "invite", Status: &joined,
	}).Error; err != nil {
		t.Fatalf("create legacy invitation: %v", err)
	}
	if err := db.Create(&entity.UserTenant{
		ID: "member-1", UserID: "user-2", TenantID: "tenant-1", Role: "normal", Status: &joined,
	}).Error; err != nil {
		t.Fatalf("create joined member: %v", err)
	}
	if err := migrateLegacyTenantInvitationStatuses(t.Context(), db); err != nil {
		t.Fatalf("migrate legacy invitations: %v", err)
	}
	var invitation, member entity.UserTenant
	if err := db.Where("id = ?", "invitation-1").Take(&invitation).Error; err != nil {
		t.Fatalf("read migrated invitation: %v", err)
	}
	if invitation.Role != "" || invitation.Status == nil || *invitation.Status != entity.TenantMembershipStatusInvited {
		status := "<nil>"
		if invitation.Status != nil {
			status = *invitation.Status
		}
		t.Fatalf("legacy invitation = role %q status %q, want empty role and invited status", invitation.Role, status)
	}
	if err := db.Where("id = ?", "member-1").Take(&member).Error; err != nil {
		t.Fatalf("read joined member: %v", err)
	}
	if member.Role != "normal" || member.Status == nil || *member.Status != entity.TenantMembershipStatusJoined {
		t.Fatalf("joined member changed: role %q status %v", member.Role, member.Status)
	}
}
