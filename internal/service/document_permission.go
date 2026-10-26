package service

import (
	"context"
	"errors"

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// CheckDocumentAccess keeps dataset grants effective for every document while
// allowing a direct document grant to expose that document independently.
func CheckDocumentAccess(ctx context.Context, subject permission.Subject, documentID, datasetID string, operation permission.Operation) error {
	checker := permission.NewDatabaseChecker(dao.DB)
	datasetErr := checker.CheckResource(ctx, subject, permission.ResourceRef{
		Kind: permission.ResourceKindDataset,
		ID:   datasetID,
	}, operation)
	if datasetErr == nil {
		return nil
	}
	if !errors.Is(datasetErr, permission.ErrPermissionDenied) && !errors.Is(datasetErr, permission.ErrMembershipNotFound) {
		return datasetErr
	}
	return checker.CheckResource(ctx, subject, permission.ResourceRef{
		Kind: permission.ResourceKindDocument,
		ID:   documentID,
	}, operation)
}
