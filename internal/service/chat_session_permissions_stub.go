//go:build !ee

package service

import (
	"context"

	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func chatSessionListOwner(_ context.Context, _, requestedOwnerID string, _ chatSessionPermissionChecker) (string, error) {
	return requestedOwnerID, nil
}

func chatSessionOwnerFilter(ctx context.Context, chatID string, crossUserOperation permission.Operation, requestedOwnerID string, _ chatSessionPermissionChecker) (string, error) {
	return requestedOwnerID, nil
}

func authorizeChatSessionOwner(ctx context.Context, _ string, session *entity.ChatSession, operation permission.Operation, checker chatSessionPermissionChecker) error {
	if session == nil {
		return permission.ErrResourceNotFound
	}
	return checker.CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindChatSession, ID: session.ID}, operation)
}

func authorizeChatSessionConversation(context.Context, string, *entity.ChatSession, chatSessionPermissionChecker) error {
	return nil
}

func authorizeChatSessionFeedback(ctx context.Context, _ string, session *entity.ChatSession, checker chatSessionPermissionChecker) error {
	if session == nil {
		return permission.ErrResourceNotFound
	}
	return checker.CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindChatSession, ID: session.ID}, permission.OperationUpdate)
}
