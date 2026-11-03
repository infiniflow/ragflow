package file

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func checkFileAccess(ctx context.Context, userID string, file *entity.File, operation permission.Operation) error {
	if file == nil {
		return permission.ErrResourceNotFound
	}
	kind := permission.ResourceKindFile
	if file.Type == FileTypeFolder {
		kind = permission.ResourceKindFolder
	}
	return permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{
		Kind: kind,
		ID:   file.ID,
	}, operation)
}

func fileRequestIdentity(ctx context.Context, identity string) (string, string, error) {
	userID := permission.UserIDFromContext(ctx)
	tenantID := permission.TenantIDFromContext(ctx)
	if userID == "" && tenantID == "" {
		return identity, identity, nil
	}
	if userID == "" || userID != identity {
		return "", "", permission.ErrIdentityMismatch
	}
	if tenantID == "" {
		tenantID = identity
	}
	return userID, tenantID, nil
}

func tenantFileContext(ctx context.Context, userID, tenantID string) (context.Context, error) {
	contextUserID := permission.UserIDFromContext(ctx)
	contextTenantID := permission.TenantIDFromContext(ctx)
	if contextUserID != "" && contextUserID != userID || contextTenantID != "" && contextTenantID != tenantID {
		return nil, permission.ErrIdentityMismatch
	}
	return permission.WithIdentity(ctx, userID, tenantID), nil
}

func (s *FileService) checkFileTreeAccess(ctx context.Context, userID string, file *entity.File, operation permission.Operation) error {
	if file == nil {
		return permission.ErrResourceNotFound
	}
	entries := []entity.File{*file}
	if file.Type == FileTypeFolder {
		descendants, err := s.fileDAO.ListDescendants(ctx, dao.DB, file.ID)
		if err != nil {
			return err
		}
		entries = append(entries, descendants...)
	}

	const permissionBatchSize = 500
	checker := permission.NewDatabaseChecker(dao.DB)
	for start := 0; start < len(entries); start += permissionBatchSize {
		end := start + permissionBatchSize
		if end > len(entries) {
			end = len(entries)
		}
		refs := make([]permission.ResourceRef, 0, end-start)
		for _, entry := range entries[start:end] {
			kind := permission.ResourceKindFile
			if entry.Type == FileTypeFolder {
				kind = permission.ResourceKindFolder
			}
			refs = append(refs, permission.ResourceRef{Kind: kind, ID: entry.ID})
		}
		accessible, err := checker.FilterResources(ctx, refs, operation)
		if err != nil {
			return err
		}
		if len(accessible) != len(refs) {
			return permission.ErrPermissionDenied
		}
	}
	return nil
}
