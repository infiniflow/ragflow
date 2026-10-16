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

func migratePermissions(ctx context.Context, db *gorm.DB) error {
	if err := migrateInvitedByNullable(ctx, db); err != nil {
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

func migrateUserCanvasTenantID(ctx context.Context, db *gorm.DB) error {
	migrator := db.WithContext(ctx).Migrator()
	if !migrator.HasTable(&entity.UserCanvas{}) {
		return nil
	}
	if !migrator.HasColumn(&entity.UserCanvas{}, "TenantID") {
		if err := migrator.AddColumn(&entity.UserCanvas{}, "TenantID"); err != nil {
			return fmt.Errorf("add user_canvas.tenant_id: %w", err)
		}
	}
	if err := db.WithContext(ctx).Exec("UPDATE user_canvas SET tenant_id = user_id WHERE tenant_id IS NULL OR tenant_id = ''").Error; err != nil {
		return fmt.Errorf("backfill user_canvas.tenant_id: %w", err)
	}
	return nil
}

func migrateAPITokenUserID(ctx context.Context, db *gorm.DB) error {
	migrator := db.WithContext(ctx).Migrator()
	if !migrator.HasTable(&entity.APIToken{}) {
		return nil
	}
	if !migrator.HasColumn(&entity.APIToken{}, "UserID") {
		if err := migrator.AddColumn(&entity.APIToken{}, "UserID"); err != nil {
			return fmt.Errorf("add api_token.user_id: %w", err)
		}
	}
	if err := db.WithContext(ctx).Exec("UPDATE api_token SET user_id = tenant_id WHERE user_id IS NULL OR user_id = ''").Error; err != nil {
		return fmt.Errorf("backfill api_token.user_id: %w", err)
	}
	return nil
}

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
		if err := migrator.AlterColumn(&entity.UserTenant{}, "InvitedBy"); err != nil {
			return fmt.Errorf("make user_tenant.invited_by nullable: %w", err)
		}
		return nil
	}
	return fmt.Errorf("user_tenant.invited_by column is missing")
}
