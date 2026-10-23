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
	if err := checkFileAccess(ctx, userID, file, operation); err != nil {
		return err
	}
	if file.Type != FileTypeFolder {
		return nil
	}

	children, err := s.fileDAO.ListByParentID(ctx, dao.DB, file.ID)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := s.checkFileTreeAccess(ctx, userID, child, operation); err != nil {
			return err
		}
	}
	return nil
}
