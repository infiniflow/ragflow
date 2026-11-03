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

	"ragflow/internal/permission"
)

// Identity identifies a user in a tenant. TenantID may be empty when an
// edition derives the effective tenant from the resource or user.
type Identity struct {
	UserID   string
	TenantID string
}

// ResolveIdentity returns the authenticated user and their active tenant.
// Explicit Identity values remain available to background work without a
// request context.
func ResolveIdentity(ctx context.Context) (Identity, error) {
	userID := strings.TrimSpace(permission.UserIDFromContext(ctx))
	if userID == "" {
		return Identity{}, permission.ErrUnauthenticated
	}

	tenantID, err := requireCurrentTenantID(ctx, userID)
	if err != nil {
		return Identity{}, err
	}

	return Identity{UserID: userID, TenantID: tenantID}, nil
}
