package document

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/dao"
	"ragflow/internal/service"

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
	run := fmt.Sprint(req.Run)

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

	// Start parsing: filter out in-flight documents (RUNNING, SCHEDULED,
	// CREATED, STOPPING) so active parses continue undisturbed, and skip
	// already COMPLETED documents when delete is false to avoid duplicate
	// chunk errors. Only documents requiring a new parse run are started.
	if run == string(entity.TaskStatusRunning) {
		taskMap := make(map[string]*entity.IngestionTask, len(validatedIDs))
		if s.ingestionTaskDAO != nil && len(validatedIDs) > 0 {
			taskMap, err = s.ingestionTaskDAO.GetLatestByDocumentIDs(ctx, dao.DB, validatedIDs)
			if err != nil {
				return common.CodeExceptionError, fmt.Errorf("fail to get ingestion tasks: %w", err)
			}
		}

		toStart := make([]validatedDoc, 0, len(validated))
		for _, vd := range validated {
			task := taskMap[vd.doc.ID]
			if task != nil && common.IsActiveTaskStatus(task.Status) {
				common.Debug(fmt.Sprintf("skip document %s ingestion, active task status: %s", vd.doc.ID, task.Status))
				continue
			}
			if !req.Delete && task != nil && task.Status == common.COMPLETED {
				common.Debug(fmt.Sprintf("skip document %s ingestion, already completed and delete is false", vd.doc.ID))
				continue
			}
			toStart = append(toStart, vd)
		}

		if skipped := len(validated) - len(toStart); skipped > 0 {
			common.Info(fmt.Sprintf("batch ingest: requested %d, started %d, skipped %d", len(validated), len(toStart), skipped))
		}

		if len(toStart) == 0 {
			return common.CodeSuccess, nil
		}

		for _, vd := range toStart {
			doc := vd.doc
			kb := vd.kb
			if err = s.StartParseDocuments(ctx, doc, kb, userID, StartParseOptions{
				ApplyKB:         req.ApplyKB,
				RerunWithDelete: req.Delete,
			}); err != nil {
				common.Error(fmt.Sprintf("go side, doc %s, start parse", doc.ID), err)
				return common.CodeExceptionError, err
			}
		}
		return common.CodeSuccess, nil
	}

	for _, vd := range validated {
		doc := vd.doc
		kb := vd.kb

		// Cancel: RequestStop (STOPPING) and update doc state. Do NOT
		// delete the ingestion task or chunks here — deletion races with
		// the worker's async markStopped/settleToTerminal flow. Once the
		// worker detects STOPPING and transitions to STOPPED, the task
		// is terminal and can be safely cleaned up.
		if run == string(entity.TaskStatusCancel) {
			if err = s.CancelDocParse(ctx, doc); err != nil {
				common.Error(fmt.Sprintf("go side, start to process %s, run is cancel", doc.ID), err)
				if errors.Is(err, errParseNotRunning) {
					// Mirror the Python /documents/ingest endpoint's message.
					return common.CodeDataError, errors.New("Cannot cancel a task that is not in RUNNING status")
				}
				return common.CodeDataError, err
			}
			if err = s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{
				"progress": 0,
			}); err != nil {
				common.Error(fmt.Sprintf("go side, doc %s, UpdateByID failed", doc.ID), err)
				return common.CodeExceptionError, err
			}
			continue
		}

		// Delete-only: user asked to remove prior parse results without
		// starting a new parse. RUNNING already continued above.
		if err = s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{
			"progress": 0,
		}); err != nil {
			common.Error(fmt.Sprintf("go side, doc %s, UpdateByID failed", doc.ID), err)
			return common.CodeExceptionError, err
		}

		if req.Delete {
			// Capture the run's own log row before its task is removed. The
			// cleanup must drop only that row: a concurrent re-parse may
			// already own a newer one for the same document.
			var pipelineLogID string
			if task, taskErr := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, doc.ID); taskErr != nil {
				common.Error(fmt.Sprintf("go side, doc %s, load ingestion task for pipeline log cleanup failed", doc.ID), taskErr)
			} else if task != nil && task.PipelineLogID != nil {
				pipelineLogID = *task.PipelineLogID
			}
			if _, delErr := s.taskDAO.DeleteIngestionTasksByDocIDs(ctx, dao.DB, []string{doc.ID}); delErr != nil {
				if errors.Is(delErr, context.Canceled) || errors.Is(delErr, context.DeadlineExceeded) {
					return common.CodeExceptionError, fmt.Errorf("delete ingestion tasks: %w", delErr)
				}
				common.Error(fmt.Sprintf("go side, doc %s, DeleteIngestionTasksByDocIDs failed", doc.ID), delErr)
			} else if err := s.pipelineLogDAO.DeleteOpenLogByID(ctx, dao.DB, pipelineLogID); err != nil {
				common.Error(fmt.Sprintf("go side, doc %s, delete open pipeline log failed", doc.ID), err)
			}
			indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
			if s.docEngine != nil {
				var exists bool
				exists, err = s.docEngine.ChunkStoreExists(ctx, indexName, doc.KbID)
				if err != nil {
					common.Error(fmt.Sprintf("go side, doc %s, ChunkStoreExists failed", doc.ID), err)
					return common.CodeExceptionError, err
				}
				if exists {
					if _, err = s.docEngine.DeleteChunks(ctx, map[string]interface{}{"doc_id": doc.ID}, indexName, doc.KbID); err != nil {
						common.Error(fmt.Sprintf("go side, doc %s, DeleteChunks failed", doc.ID), err)
						return common.CodeExceptionError, err
					}
				}
			}
		}
	}

	return common.CodeSuccess, nil
}
