//go:build !ee

package service

import (
	"context"

	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func chatSessionListOwner(_ context.Context, _, requestedOwnerID string) (string, error) {
	return requestedOwnerID, nil
}

func chatSessionOwnerFilter(ctx context.Context, chatID string, crossUserOperation permission.Operation, requestedOwnerID string) (string, error) {
	return requestedOwnerID, nil
}

func authorizeChatSessionOwner(context.Context, string, *entity.ChatSession, permission.Operation) error {
	return nil
}

func authorizeChatSessionConversation(context.Context, string, *entity.ChatSession) error {
	return nil
}

func authorizeChatSessionFeedback(context.Context, string, *entity.ChatSession) error {
	return nil
}
