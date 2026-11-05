package dataset

import (
	"context"
	"errors"
	"strings"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

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

// GetFieldMap returns the field map for the given knowledge base IDs.
func (d *DatasetService) GetFieldMap(ctx context.Context, ids []string) (map[string]interface{}, error) {
	return d.kbDAO.GetFieldMap(ctx, dao.DB, ids)
}
