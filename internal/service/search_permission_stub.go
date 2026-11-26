//go:build !ee

package service

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

func checkSearchUpdatePermission(ctx context.Context, searchID string) error {
	return permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{
		Kind: permission.ResourceKindSearchApp,
		ID:   searchID,
	}, permission.OperationUpdate)
}
