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

package permission

import (
	"context"
	"strings"

	"gorm.io/gorm"
	"ragflow/internal/entity"
)

// CheckPlatformModelAccess checks the operation tenant before use of a platform model.
func CheckPlatformModelAccess(ctx context.Context, db *gorm.DB, tenantID string) error {
	tenantID = strings.TrimSpace(tenantID)
	if db == nil || tenantID == "" || strings.EqualFold(tenantID, entity.PlatformTenantID) {
		return ErrPermissionDenied
	}
	if existing := TenantIDFromContext(ctx); existing != "" && existing != tenantID {
		return ErrIdentityMismatch
	}
	var marker entity.SystemSettings
	if err := db.WithContext(ctx).Take(&marker, "name = ? AND source = ? AND value = ?", entity.PlatformProviderLockSetting, "system", "ready").Error; err != nil {
		return ErrPermissionDenied
	}
	var tenant entity.Tenant
	if err := db.WithContext(ctx).Take(&tenant, "id = ? AND status = ?", tenantID, "1").Error; err != nil {
		return err
	}
	if UserIDFromContext(ctx) == "" {
		if TrustedModelTenant(ctx) != tenantID {
			return ErrPermissionDenied
		}
		return nil
	}
	return NewDatabaseChecker(db).CheckTenant(ctx, tenantID, TenantMember)
}
