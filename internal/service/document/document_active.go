package document

import (
	"context"

	"ragflow/internal/dao"
)

// HasActiveIngestionTasks reports whether the dataset has ingestion tasks
// in a non-terminal state (CREATED/SCHEDULED/RUNNING/STOPPING).
// It powers the list response's has_active_tasks field so pagination-
// scoped polling does not stall when active docs are off the current page.
func (s *DocumentService) HasActiveIngestionTasks(ctx context.Context, datasetID string) (bool, error) {
	count, err := s.ingestionTaskDAO.CountActiveByDatasetID(ctx, dao.DB, datasetID)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}
