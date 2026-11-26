package file

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
)

func currentFileTenantID(ctx context.Context) string {
	tenantID := permission.TenantIDFromContext(ctx)
	if tenantID == "" {
		tenantID = permission.UserIDFromContext(ctx)
	}
	return tenantID
}

// FileOwnerUserID resolves ownership for current and legacy file rows.
func FileOwnerUserID(file *entity.File, userID, tenantID string) string {
	if file == nil {
		return ""
	}
	if file.UserID != "" {
		return file.UserID
	}
	if file.CreatedBy != "" {
		return file.CreatedBy
	}
	// Older OSS file rows were created before user_id was populated. In OSS,
	// the tenant ID is the user ID, so an otherwise unowned legacy row remains
	// attributable to that same user only.
	if file.TenantID == tenantID && userID == tenantID {
		return userID
	}
	return ""
}

func checkFileAccess(ctx context.Context, file *entity.File, operation permission.Operation) error {
	if file == nil {
		return permission.ErrResourceNotFound
	}
	userID := permission.UserIDFromContext(ctx)
	tenantID := currentFileTenantID(ctx)
	if userID == "" {
		return permission.ErrUnauthenticated
	}
	if file.TenantID != tenantID {
		return permission.ErrResourceNotFound
	}
	action := permission.ActionRead
	if operation != permission.OperationRead {
		action = permission.ActionWrite
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckFeature(ctx, "file", action); err != nil {
		return err
	}
	if file.ParentID == file.ID || (file.SourceType == string(entity.FileSourceKnowledgebase) && file.UserID == "") {
		if operation == permission.OperationRead || operation == permission.OperationCreate {
			return nil
		}
		return permission.ErrPermissionDenied
	}
	if FileOwnerUserID(file, userID, tenantID) != userID {
		return permission.ErrPermissionDenied
	}
	return nil
}

func (s *FileService) checkFileTreeAccess(ctx context.Context, file *entity.File, operation permission.Operation) error {
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

	for i := range entries {
		if err := checkFileAccess(ctx, &entries[i], operation); err != nil {
			return err
		}
	}
	return nil
}
