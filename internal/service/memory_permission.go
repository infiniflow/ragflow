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

package service

import (
	"context"
	"strings"

	"ragflow/internal/permission"
)

func (s *MemoryService) checkMemoryPermission(ctx context.Context, userID, memoryID string, operation permission.Operation) error {
	return permission.NewDatabaseChecker(s.db).CheckResource(
		ctx,
		permission.Subject{UserID: userID},
		permission.ResourceRef{Kind: permission.ResourceKindMemory, ID: memoryID},
		operation,
	)
}

func (s *MemoryService) accessibleMemoryIDs(ctx context.Context, userID string, operation permission.Operation) ([]string, error) {
	scope, err := permission.NewDatabaseChecker(s.db).Scope(ctx, permission.Subject{UserID: userID}, permission.ScopeQuery{
		Kind:      permission.ResourceKindMemory,
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

func (s *MemoryService) accessibleMemoryIDsForCanvas(ctx context.Context, userID, canvasID string) ([]string, error) {
	canvasID = strings.TrimSpace(canvasID)
	if canvasID == "" {
		return s.accessibleMemoryIDs(ctx, userID, permission.OperationRead)
	}
	scope, err := permission.NewDatabaseChecker(s.db).Scope(ctx, permission.Subject{UserID: userID}, permission.ScopeQuery{
		Kind:      permission.ResourceKindMemory,
		Operation: permission.OperationUse,
		Entry:     &permission.ResourceRef{Kind: permission.ResourceKindCanvas, ID: canvasID},
		EntryOp:   permission.OperationRun,
	})
	if err != nil {
		return nil, err
	}
	if scope.Mode != permission.ScopeIDs {
		return []string{}, nil
	}
	return scope.ResourceIDs, nil
}

func (s *MemoryService) filterMemoryIDs(ctx context.Context, userID string, memoryIDs []string, operation permission.Operation) ([]string, error) {
	refs := make([]permission.ResourceRef, 0, len(memoryIDs))
	for _, memoryID := range memoryIDs {
		refs = append(refs, permission.ResourceRef{Kind: permission.ResourceKindMemory, ID: memoryID})
	}
	accessible, err := permission.NewDatabaseChecker(s.db).FilterResources(
		ctx,
		permission.Subject{UserID: userID},
		refs,
		operation,
	)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(accessible))
	for _, ref := range accessible {
		ids = append(ids, ref.ID)
	}
	return ids, nil
}
