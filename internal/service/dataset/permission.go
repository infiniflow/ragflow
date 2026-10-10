package dataset

import (
	"context"
	"errors"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	"ragflow/internal/service"
)

// CheckAccess authorizes an operation on a dataset under the active edition's policy.
func (d *DatasetService) CheckAccess(ctx context.Context, subject permission.Subject, datasetID string, operation permission.Operation) error {
	return service.CheckDatasetAccess(ctx, subject, datasetID, operation)
}

// GetKnowledgebaseByID resolves a dataset entity without applying permission
// checks. Upload needs the same existence-then-auth ordering as Python.
func (d *DatasetService) GetKnowledgebaseByID(ctx context.Context, datasetID string) (*entity.Knowledgebase, error) {
	datasetID = strings.TrimSpace(datasetID)
	if datasetID == "" {
		return nil, errors.New("Lack of \"Dataset ID\"")
	}
	normalizedID, err := normalizeDatasetID(datasetID)
	if err != nil {
		return nil, err
	}
	return d.kbDAO.GetByID(ctx, dao.DB, normalizedID)
}
