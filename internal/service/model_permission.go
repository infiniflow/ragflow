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

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
)

func checkModelPermission(ctx context.Context, userID string, kind permission.ResourceKind, id string, operation permission.Operation) error {
	return permission.NewDatabaseChecker(dao.DB).CheckResource(
		ctx,
		permission.Subject{UserID: userID},
		permission.ResourceRef{Kind: kind, ID: id},
		operation,
	)
}

func accessibleModelResourceIDs(ctx context.Context, userID string, kind permission.ResourceKind, parent permission.ResourceRef, operation permission.Operation, entry *permission.ResourceRef, entryOperation permission.Operation) ([]string, error) {
	query := permission.ScopeQuery{Kind: kind, Parent: parent, Operation: operation}
	if entry != nil {
		query.Entry = entry
		query.EntryOp = entryOperation
	}
	scope, err := permission.NewDatabaseChecker(dao.DB).Scope(ctx, permission.Subject{UserID: userID}, query)
	if err != nil {
		return nil, err
	}
	if scope.Mode != permission.ScopeIDs {
		return []string{}, nil
	}
	return scope.ResourceIDs, nil
}
func normalizeModelPermissionError(err error) (common.ErrorCode, error) {
	return permissionresponse.NormalizeHidden(err)
}
