package service

import (
	"context"
	"errors"

	"ragflow/internal/dao"
)

// FirstChatChannelAPIKey follows the key selection used by webpage embeds.
func FirstChatChannelAPIKey(ctx context.Context, tenantID string) (string, error) {
	keys, err := dao.NewAPITokenDAO().GetByTenantID(ctx, dao.DB, tenantID)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", errors.New("Please create API Key first.")
	}
	return keys[0].Token, nil
}
