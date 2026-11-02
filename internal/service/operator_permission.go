package service

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// ResolveOperatorPermission returns the caller's resource permission mask.
// Feature-role limits are intentionally excluded from this value.
func ResolveOperatorPermission(ctx context.Context, userID string, resource permission.ResourceRef) (int32, error) {
	access, err := permission.NewDatabaseChecker(dao.DB).ResolveAccess(
		ctx,
		permission.Subject{UserID: userID},
		resource,
	)
	if err != nil {
		return 0, err
	}
	return access.PermissionMask, nil
}
