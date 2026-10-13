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
