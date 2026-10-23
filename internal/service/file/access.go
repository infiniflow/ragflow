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
	return permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.Subject{UserID: userID}, permission.ResourceRef{
		Kind: kind,
		ID:   file.ID,
	}, operation)
}

func fileRequestSubject(ctx context.Context, identity string) (string, string, error) {
	subject, ok := permission.SubjectFromContext(ctx)
	if !ok {
		return identity, identity, nil
	}
	if subject.UserID == "" || subject.UserID != identity {
		return "", "", permission.ErrInvalidSubject
	}
	tenantID := subject.TenantID
	if tenantID == "" {
		tenantID = identity
	}
	return subject.UserID, tenantID, nil
}

func tenantFileContext(ctx context.Context, userID, tenantID string) (context.Context, error) {
	if subject, ok := permission.SubjectFromContext(ctx); ok {
		if subject.UserID != "" && subject.UserID != userID || subject.TenantID != "" && subject.TenantID != tenantID {
			return nil, permission.ErrInvalidSubject
		}
	}
	return permission.WithSubject(ctx, permission.Subject{UserID: userID, TenantID: tenantID}), nil
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
		accessible, err := checker.FilterResources(ctx, permission.Subject{UserID: userID}, refs, operation)
		if err != nil {
			return err
		}
		if len(accessible) != len(refs) {
			return permission.ErrPermissionDenied
		}
	}
	return nil
}
