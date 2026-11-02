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

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
)

type resourceChecker interface {
	CheckResource(context.Context, permission.Subject, permission.ResourceRef, permission.Operation) error
}

type resourceScopeChecker interface {
	resourceChecker
	Scope(context.Context, permission.Subject, permission.ScopeQuery) (permission.Scope, error)
}

func checkChatPermission(ctx context.Context, checker resourceChecker, userID, chatID string, operation permission.Operation) error {
	if checker == nil {
		checker = permission.NewDatabaseChecker(dao.DB)
	}
	return checker.CheckResource(
		ctx,
		permission.Subject{UserID: userID},
		permission.ResourceRef{Kind: permission.ResourceKindChat, ID: chatID},
		operation,
	)
}

func checkChatSessionPermission(ctx context.Context, checker resourceChecker, userID, sessionID string, operation permission.Operation) error {
	if checker == nil {
		checker = permission.NewDatabaseChecker(dao.DB)
	}
	return checker.CheckResource(
		ctx,
		permission.Subject{UserID: userID},
		permission.ResourceRef{Kind: permission.ResourceKindChatSession, ID: sessionID},
		operation,
	)
}

func accessibleChatIDs(ctx context.Context, checker resourceScopeChecker, userID string, operation permission.Operation) ([]string, error) {
	if checker == nil {
		checker = permission.NewDatabaseChecker(dao.DB)
	}
	scope, err := checker.Scope(ctx, permission.Subject{UserID: userID}, permission.ScopeQuery{
		Kind:      permission.ResourceKindChat,
		Operation: operation,
	})
	if err != nil {
		return nil, err
	}
	if scope.Mode == permission.ScopeNone {
		return []string{}, nil
	}
	return scope.ResourceIDs, nil
}

func accessibleChatSessionIDs(ctx context.Context, checker resourceScopeChecker, userID, chatID string, operation permission.Operation) ([]string, error) {
	if checker == nil {
		checker = permission.NewDatabaseChecker(dao.DB)
	}
	chatRef := permission.ResourceRef{Kind: permission.ResourceKindChat, ID: chatID}
	if err := checkChatPermission(ctx, checker, userID, chatID, permission.OperationRead); err != nil {
		return nil, err
	}
	scope, err := checker.Scope(ctx, permission.Subject{UserID: userID}, permission.ScopeQuery{
		Kind:      permission.ResourceKindChatSession,
		Parent:    chatRef,
		Operation: operation,
		Entry:     &chatRef,
		EntryOp:   permission.OperationRead,
	})
	if err != nil {
		return nil, err
	}
	if scope.Mode == permission.ScopeNone {
		return []string{}, nil
	}
	return scope.ResourceIDs, nil
}

func normalizeChatPermissionError(err error) (common.ErrorCode, error) {
	return permissionresponse.NormalizeHidden(err)
}
