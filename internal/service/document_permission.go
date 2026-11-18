package service

import (
	"context"

	"ragflow/internal/dao"
	"ragflow/internal/permission"
)

// CheckDocumentAccess requires access to both the dataset and the document.
func CheckDocumentAccess(ctx context.Context, documentID, datasetID string, operation permission.Operation) error {
	checker := permission.NewDatabaseChecker(dao.DB)
	if err := checker.CheckResource(ctx, permission.ResourceRef{
		Kind: permission.ResourceKindDataset,
		ID:   datasetID,
	}, operation); err != nil {
		return err
	}
	return checker.CheckResource(ctx, permission.ResourceRef{
		Kind: permission.ResourceKindDocument,
		ID:   documentID,
	}, operation)
}

// ListReadableDocumentIDs returns the requested documents visible to the
// current user in the given datasets. A nil candidate list means all readable
// documents; a non-nil empty list matches no documents.
func ListReadableDocumentIDs(ctx context.Context, datasetIDs, candidateIDs []string) ([]string, error) {
	return permission.ListReadableDocumentIDs(ctx, dao.DB, datasetIDs, candidateIDs)
}
