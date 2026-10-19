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
	"fmt"

	"ragflow/internal/entity"

	"gorm.io/gorm"
)

const permissionMigrationTargetVersion = "v1.0.0-rc3.dev1"

// migratePermissions applies the permission schema migration and records its version.
func migratePermissions(ctx context.Context, db *gorm.DB) error {
	if err := migratePermissionDataEE(ctx, db); err != nil {
		return fmt.Errorf("migrate enterprise permission data: %w", err)
	}
	if err := migrateInvitedByNullable(ctx, db); err != nil {
		return err
	}
	if err := migrateTenantModelProviderUserID(ctx, db); err != nil {
		return err
	}
	if err := migrateAPITokenUserID(ctx, db); err != nil {
		return err
	}
	if err := migrateUserCanvasTenantID(ctx, db); err != nil {
		return err
	}
	version, err := GetDatabaseMigrationVersion(ctx, db)
	if err != nil {
		return fmt.Errorf("read database migration version: %w", err)
	}
	if shouldSkipMigration(version, permissionMigrationTargetVersion) {
		return nil
	}
	if err := setDatabaseMigrationVersion(ctx, db, permissionMigrationTargetVersion); err != nil {
		return fmt.Errorf("mark permission migration version: %w", err)
	}
	return nil
}

// migrateTenantModelProviderUserID backfills missing model provider owner IDs.
func migrateTenantModelProviderUserID(ctx context.Context, db *gorm.DB) error {
	scoped := db.WithContext(ctx)
	migrator := scoped.Migrator()
	if !migrator.HasTable(&entity.TenantModelProvider{}) {
		return nil
	}
	if !migrator.HasColumn(&entity.TenantModelProvider{}, "UserID") {
		return fmt.Errorf("tenant_model_provider.user_id is missing after GORM migration")
	}
	if err := scoped.Model(&entity.TenantModelProvider{}).
		Where("user_id IS NULL OR user_id = ''").
		UpdateColumn("user_id", gorm.Expr("tenant_id")).Error; err != nil {
		return fmt.Errorf("backfill tenant_model_provider.user_id from tenant_id: %w", err)
	}
	return nil
}

// migrateUserCanvasTenantID backfills tenant IDs for existing canvases.
func migrateUserCanvasTenantID(ctx context.Context, db *gorm.DB) error {
	migrator := db.WithContext(ctx).Migrator()
	if !migrator.HasTable(&entity.UserCanvas{}) {
		return nil
	}
	if !migrator.HasColumn(&entity.UserCanvas{}, "TenantID") {
		return fmt.Errorf("user_canvas.tenant_id is missing after GORM migration")
	}
	if err := db.WithContext(ctx).Exec("UPDATE user_canvas SET tenant_id = user_id WHERE tenant_id IS NULL OR tenant_id = ''").Error; err != nil {
		return fmt.Errorf("backfill user_canvas.tenant_id: %w", err)
	}
	return nil
}

// migrateAPITokenUserID backfills user IDs for existing API tokens.
func migrateAPITokenUserID(ctx context.Context, db *gorm.DB) error {
	migrator := db.WithContext(ctx).Migrator()
	if !migrator.HasTable(&entity.APIToken{}) {
		return nil
	}
	if !migrator.HasColumn(&entity.APIToken{}, "UserID") {
		return fmt.Errorf("api_token.user_id is missing after GORM migration")
	}
	if err := db.WithContext(ctx).Exec("UPDATE api_token SET user_id = tenant_id WHERE user_id IS NULL OR user_id = ''").Error; err != nil {
		return fmt.Errorf("backfill api_token.user_id: %w", err)
	}
	return nil
}

// migrateInvitedByNullable allows tenant membership requests without an inviter.
func migrateInvitedByNullable(ctx context.Context, db *gorm.DB) error {
	migrator := db.WithContext(ctx).Migrator()
	columns, err := migrator.ColumnTypes(&entity.UserTenant{})
	if err != nil {
		return fmt.Errorf("inspect user_tenant columns: %w", err)
	}
	for _, column := range columns {
		if column.Name() != "invited_by" {
			continue
		}
		nullable, known := column.Nullable()
		if !known {
			return fmt.Errorf("user_tenant.invited_by nullability is unknown")
		}
		if nullable {
			return nil
		}
		// AutoMigrate does not relax NOT NULL on an existing column for every
		// supported dialect, so use GORM's migrator for this one legacy constraint.
		if err := migrator.AlterColumn(&entity.UserTenant{}, "InvitedBy"); err != nil {
			return fmt.Errorf("make user_tenant.invited_by nullable: %w", err)
		}
		return nil
	}
	return fmt.Errorf("user_tenant.invited_by column is missing")
}
