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

func checkConnectorPermission(ctx context.Context, userID, connectorID string, operation permission.Operation) error {
	return permission.NewDatabaseChecker(dao.DB).CheckResource(
		ctx,
		permission.Subject{UserID: userID},
		permission.ResourceRef{Kind: permission.ResourceKindConnector, ID: connectorID},
		operation,
	)
}

func accessibleConnectorIDs(ctx context.Context, userID string, operation permission.Operation) ([]string, error) {
	scope, err := permission.NewDatabaseChecker(dao.DB).Scope(ctx, permission.Subject{UserID: userID}, permission.ScopeQuery{
		Kind:      permission.ResourceKindConnector,
		Operation: operation,
	})
	if err != nil {
		return nil, err
	}
	if scope.Mode != permission.ScopeIDs {
		return []string{}, nil
	}
	return scope.ResourceIDs, nil
}

func checkSyncTaskPermission(ctx context.Context, userID, connectorID, taskID string, operation permission.Operation) error {
	checker := permission.NewDatabaseChecker(dao.DB)
	connectorRef := permission.ResourceRef{Kind: permission.ResourceKindConnector, ID: connectorID}
	scope, err := checker.Scope(ctx, permission.Subject{UserID: userID}, permission.ScopeQuery{
		Kind:      permission.ResourceKindSyncTask,
		Parent:    connectorRef,
		Operation: operation,
		Entry:     &connectorRef,
		EntryOp:   operation,
	})
	if err != nil {
		return err
	}
	for _, scopedTaskID := range scope.ResourceIDs {
		if scopedTaskID == taskID {
			return nil
		}
	}
	return permission.ErrResourceNotFound
}

func accessibleSyncTaskIDs(ctx context.Context, userID, connectorID string, operation permission.Operation) ([]string, error) {
	checker := permission.NewDatabaseChecker(dao.DB)
	subject := permission.Subject{UserID: userID}
	connectorRef := permission.ResourceRef{Kind: permission.ResourceKindConnector, ID: connectorID}
	if err := checker.CheckResource(ctx, subject, connectorRef, permission.OperationRead); err != nil {
		return nil, err
	}
	scope, err := checker.Scope(ctx, subject, permission.ScopeQuery{
		Kind:      permission.ResourceKindSyncTask,
		Parent:    connectorRef,
		Operation: operation,
		Entry:     &connectorRef,
		EntryOp:   permission.OperationRead,
	})
	if err != nil {
		return nil, err
	}
	if scope.Mode != permission.ScopeIDs {
		return []string{}, nil
	}
	return scope.ResourceIDs, nil
}

func accessibleSyncTaskIDsForConnectors(ctx context.Context, userID string, connectorIDs []string, operation permission.Operation) ([]string, error) {
	var taskIDs []string
	for _, connectorID := range connectorIDs {
		ids, err := accessibleSyncTaskIDs(ctx, userID, connectorID, operation)
		if err != nil {
			return nil, err
		}
		taskIDs = append(taskIDs, ids...)
	}
	return taskIDs, nil
}

func normalizeConnectorPermissionError(err error) (common.ErrorCode, error) {
	return permissionresponse.NormalizeHidden(err)
}
