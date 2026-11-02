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

	"gorm.io/gorm"
	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

func requireCurrentTenantID(ctx context.Context, userID string) (string, error) {
	if strings.TrimSpace(userID) == "" {
		return "", permission.ErrUnauthenticated
	}
	if subject, ok := permission.SubjectFromContext(ctx); ok {
		if subject.UserID != "" && subject.UserID != userID {
			return "", permission.ErrInvalidSubject
		}
		if tenantID := strings.TrimSpace(subject.TenantID); tenantID != "" {
			if err := permission.NewDatabaseChecker(dao.DB).CheckTenant(ctx, permission.Subject{UserID: userID}, tenantID, permission.TenantMember); err != nil {
				return "", err
			}
			return tenantID, nil
		}
	}
	tenantID, handled, err := resolveDefaultTenantIDEE(ctx, userID)
	if err != nil {
		return "", err
	}
	if handled {
		return tenantID, nil
	}
	return strings.TrimSpace(userID), nil
}

func checkFeatureAction(ctx context.Context, userID string, feature permission.Feature, action permission.Action) error {
	return permission.NewDatabaseChecker(dao.DB).CheckFeature(ctx, permission.Subject{UserID: userID}, feature, action)
}

func grantCreatedResourceOwner(ctx context.Context, tx *gorm.DB, userID, tenantID string, kind permission.ResourceKind, resourceID string) error {
	return permission.NewDatabaseChecker(tx).GrantResourceOwner(ctx, permission.Subject{UserID: userID, TenantID: tenantID}, permission.ResourceRef{Kind: kind, ID: resourceID})
}
