package service

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// CheckDatasetAccess checks a dataset operation using the active edition's permission policy.
func CheckDatasetAccess(ctx context.Context, subject permission.Subject, datasetID string, operation permission.Operation) error {
	return permission.NewDatabaseChecker(dao.DB).CheckResource(
		ctx,
		subject,
		permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID},
		operation,
	)
}
