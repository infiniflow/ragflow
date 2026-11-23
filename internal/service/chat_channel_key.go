package service

import (
	"context"
	"errors"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

var errChatChannelNoAPIKey = errors.New("Please create API Key first.")

// FirstChatChannelAPIKey checks for a tenant key when configuring a channel.
func FirstChatChannelAPIKey(ctx context.Context, tenantID string) (string, error) {
	keys, err := dao.NewAPITokenDAO().GetByTenantID(ctx, dao.DB, tenantID)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", errChatChannelNoAPIKey
	}
	return keys[0].Token, nil
}

// chatChannelExecutionKey selects the first key whose REST identity can use the
// channel and run its saved target. The REST API still checks both permissions.
func chatChannelExecutionKey(ctx context.Context, channel *entity.ChatChannel) (string, error) {
	keys, err := dao.NewAPITokenDAO().GetByTenantID(ctx, dao.DB, channel.TenantID)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", errChatChannelNoAPIKey
	}
	users := NewUserService()
	checker := permission.NewDatabaseChecker(dao.DB)
	channelRef := permission.ResourceRef{Kind: permission.ResourceKindChatChannel, ID: channel.ID}
	for _, key := range keys {
		identity, _, err := users.resolveAPITokenIdentity(ctx, key)
		if err != nil || (identity.User.IsSuperuser != nil && *identity.User.IsSuperuser) {
			continue
		}
		keyCtx := permission.WithIdentity(ctx, identity.User.ID, identity.TenantID)
		err = checker.CheckResource(keyCtx, channelRef, permission.OperationUse)
		if err == nil {
			err = validateChannelBinding(keyCtx, channel.ChatID, channel.AgentID)
		}
		if err == nil && channel.ChatID != nil {
			err = checker.CheckResource(keyCtx, permission.ResourceRef{Kind: permission.ResourceKindChat, ID: *channel.ChatID}, permission.OperationRun)
		}
		if err == nil {
			return key.Token, nil
		}
		if !errors.Is(err, permission.ErrPermissionDenied) && !errors.Is(err, permission.ErrResourceNotFound) {
			return "", err
		}
	}
	return "", errors.New("no tenant API key can run the bound chat or agent")
}
