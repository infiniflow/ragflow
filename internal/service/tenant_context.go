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
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

func requireCurrentTenantID(ctx context.Context) (string, error) {
	userID := strings.TrimSpace(permission.UserIDFromContext(ctx))
	if userID == "" {
		return "", permission.ErrUnauthenticated
	}
	if tenantID := strings.TrimSpace(permission.TenantIDFromContext(ctx)); tenantID != "" {
		if err := permission.NewDatabaseChecker(dao.DB).CheckTenant(ctx, tenantID, permission.TenantMember); err != nil {
			return "", err
		}
		return tenantID, nil
	}
	tenantID, handled, err := resolveDefaultTenantIDEE(ctx)
	if err != nil {
		return "", err
	}
	if handled {
		return tenantID, nil
	}
	return strings.TrimSpace(userID), nil
}
