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

package service

import (
	"context"
	"errors"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

func requireOwnedTenantID(ctx context.Context, userID string) (string, error) {
	if strings.TrimSpace(userID) == "" {
		return "", permission.ErrUnauthenticated
	}
	tenantDAO := dao.NewUserTenantDAO()
	tenants, err := tenantDAO.GetByUserIDAndRole(ctx, dao.DB, userID, string(permission.RoleOwner))
	if err != nil {
		return "", err
	}
	checker := permission.NewDatabaseChecker(dao.DB)
	for _, tenant := range tenants {
		if tenant == nil || strings.TrimSpace(tenant.TenantID) == "" {
			continue
		}
		if err := checker.CheckTenant(ctx, permission.Subject{UserID: userID}, tenant.TenantID, permission.TenantOwner); err == nil {
			return tenant.TenantID, nil
		} else if !errors.Is(err, permission.ErrMembershipNotFound) {
			return "", err
		}
	}
	return "", permission.ErrMembershipNotFound
}
