package document

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/dao"
	"ragflow/internal/service"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func (s *DocumentService) ListIngestionTasks(ctx context.Context, userID string, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error) {
	return s.ingestionTaskSvc.ListByUser(ctx, userID, datasetID, page, pageSize)
}

func (s *DocumentService) IngestDocuments(ctx context.Context, datasetID, userID string, docIDs []string) ([]*service.ParseDocumentResponse, error) {
	responses, err := s.ingestionTaskSvc.CreateForDocuments(ctx, datasetID, userID, docIDs)
	if err != nil {
		return nil, err
	}
	common.Info(fmt.Sprintf("parse documents, dataset: %s, documents: %v", datasetID, docIDs))
	return responses, nil
}

func (s *DocumentService) StopIngestionTasks(ctx context.Context, tasks []string, userID string) ([]*entity.IngestionTask, error) {
	return s.ingestionTaskSvc.RequestStopMany(ctx, tasks, &userID)
}

func (s *DocumentService) RemoveIngestionTasks(ctx context.Context, tasks []string, userID string) ([]map[string]string, error) {
	return s.ingestionTaskSvc.RemoveMany(ctx, tasks, &userID)
}

func (s *DocumentService) Ingest(ctx context.Context, userID string, req *IngestDocumentRequest) (common.ErrorCode, error) {
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != IngestActionStart && action != IngestActionCancel {
		return common.CodeArgumentError, fmt.Errorf("invalid action: %q (must be %q or %q)", req.Action, IngestActionStart, IngestActionCancel)
	}

	docs, err := s.documentDAO.GetByIDs(ctx, dao.DB, req.DocIDs)
	if err != nil {
		return common.CodeExceptionError, fmt.Errorf("fail to get documents: %w", err)
	}

	docsByID := make(map[string]*entity.Document, len(docs))
	for _, doc := range docs {
		if doc != nil {
			docsByID[doc.ID] = doc
		}
	}

	// First pass: validate every document exists and is accessible before
	// mutating any state, so a single invalid doc rejects the whole request.
	type validatedDoc struct {
		doc *entity.Document
		kb  *entity.Knowledgebase
	}
	validated := make([]validatedDoc, 0, len(req.DocIDs))
	validatedIDs := make([]string, 0, len(req.DocIDs))
	for _, docID := range req.DocIDs {
		doc := docsByID[docID]
		if doc == nil {
			return common.CodeDataError, fmt.Errorf("document not found")
		}
		kb, err := s.kbDAO.GetByID(ctx, dao.DB, doc.KbID)
		if err != nil {
			return common.CodeDataError, fmt.Errorf("dataset not found")
		}
		if !s.kbDAO.Accessible(ctx, dao.DB, kb.ID, userID) {
			return common.CodeAuthenticationError, fmt.Errorf("no authorization")
		}
		validated = append(validated, validatedDoc{doc, kb})
		validatedIDs = append(validatedIDs, docID)
	}

	// Batch pre-check for reparse with delete: use the validated doc IDs
	// so we don't silently skip non-existent or unauthorized documents.
	if action == IngestActionStart && req.Delete {
		if err = s.AssertIngestionTasksTerminal(ctx, validatedIDs); err != nil {
			return common.CodeDataError, err
		}
	}

	for _, vd := range validated {
		doc := vd.doc
		kb := vd.kb

		if action == IngestActionStart {
			if err = s.StartParseDocuments(ctx, doc, kb, userID, StartParseOptions{
				ApplyKB:         req.ApplyKB,
				RerunWithDelete: req.Delete,
			}); err != nil {
				common.Error(fmt.Sprintf("go side, doc %s, start parse", doc.ID), err)
				return common.CodeExceptionError, err
			}
			continue
		}

		if action == IngestActionCancel {
			if err = s.CancelDocParse(ctx, doc); err != nil {
				common.Error(fmt.Sprintf("go side, start to process %s, run is cancel", doc.ID), err)
				if errors.Is(err, errParseNotRunning) {
					// Mirror the Python /documents/ingest endpoint's message.
					return common.CodeDataError, errors.New("Cannot cancel a task that is not in RUNNING status")
				}
				return common.CodeDataError, err
			}
			continue
		}
	}

	return common.CodeSuccess, nil
}
