package document

import (
	"context"
	"fmt"
	"ragflow/internal/service"

	"ragflow/internal/common"
	"ragflow/internal/entity"
)

func (s *DocumentService) ListIngestionTasks(userID string, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error) {
	return s.ingestionTaskSvc.ListByUser(userID, datasetID, page, pageSize)
}

func (s *DocumentService) IngestDocuments(datasetID, userID string, docIDs []string) ([]*service.ParseDocumentResponse, error) {
	responses, err := s.ingestionTaskSvc.CreateForDocuments(datasetID, userID, docIDs)
	if err != nil {
		return nil, err
	}
	common.Info(fmt.Sprintf("parse documents, dataset: %s, documents: %v", datasetID, docIDs))
	return responses, nil
}

func (s *DocumentService) StopIngestionTasks(tasks []string, userID string) ([]*entity.IngestionTask, error) {
	return s.ingestionTaskSvc.RequestStopMany(tasks, &userID)
}

func (s *DocumentService) RemoveIngestionTasks(tasks []string, userID string) ([]map[string]string, error) {
	return s.ingestionTaskSvc.RemoveMany(tasks, &userID)
}

func (s *DocumentService) Ingest(userID string, req *IngestDocumentRequest) (common.ErrorCode, error) {
	run := fmt.Sprint(req.Run)

	docs, err := s.documentDAO.GetByIDs(req.DocIDs)
	if err != nil {
		return common.CodeExceptionError, fmt.Errorf("fail to get documents: %s", err.Error())
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
		kb, err := s.kbDAO.GetByID(doc.KbID)
		if err != nil {
			return common.CodeDataError, fmt.Errorf("dataset not found")
		}
		if !s.kbDAO.Accessible(kb.ID, userID) {
			return common.CodeAuthenticationError, fmt.Errorf("no authorization")
		}
		validated = append(validated, validatedDoc{doc, kb})
		validatedIDs = append(validatedIDs, docID)
	}

	// Batch pre-check for re-parse with delete: use the validated doc IDs
	// so we don't silently skip non-existent or unauthorized documents.
	if run == string(entity.TaskStatusRunning) && req.Delete {
		if err := s.AssertIngestionTasksTerminal(validatedIDs); err != nil {
			return common.CodeDataError, err
		}
	}

	for _, vd := range validated {
		doc := vd.doc
		kb := vd.kb

		// Start parsing: delegates to the shared start-parse flow. The
		// document run status is set by service.IngestionTaskService.StartRunning
		// when the task transitions from CREATED, not here.
		if run == string(entity.TaskStatusRunning) {
			if err := s.StartParseDocuments(doc, kb, userID, StartParseOptions{
				ApplyKB:         req.ApplyKB,
				RerunWithDelete: req.Delete,
			}); err != nil {
				common.Error(fmt.Sprintf("go side, doc %s, start parse", doc.ID), err)
				return common.CodeExceptionError, err
			}
			continue
		}

		// Cancel: RequestStop (STOPPING) and update doc state. Do NOT
		// delete the ingestion task or chunks here — deletion races with
		// the worker's async markStopped/settleToTerminal flow. Once the
		// worker detects STOPPING and transitions to STOPPED, the task
		// is terminal and can be safely cleaned up.
		if run == string(entity.TaskStatusCancel) {
			if err := s.CancelDocParse(doc); err != nil {
				common.Error(fmt.Sprintf("go side, start to process %s, run is cancel", doc.ID), err)
				return common.CodeDataError, err
			}
			if err := s.documentDAO.UpdateByID(doc.ID, map[string]interface{}{
				"run":      string(entity.TaskStatusCancel),
				"progress": 0,
			}); err != nil {
				common.Error(fmt.Sprintf("go side, doc %s, UpdateByID failed", doc.ID), err)
				return common.CodeExceptionError, err
			}
			continue
		}

		// Delete-only: user asked to remove prior parse results without
		// starting a new parse. RUNNING already continued above.
		if err := s.documentDAO.UpdateByID(doc.ID, map[string]interface{}{
			"run":      run,
			"progress": 0,
		}); err != nil {
			common.Error(fmt.Sprintf("go side, doc %s, UpdateByID failed", doc.ID), err)
			return common.CodeExceptionError, err
		}

		if req.Delete {
			_, _ = s.taskDAO.DeleteIngestionTasksByDocIDs([]string{doc.ID})
			indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
			if s.docEngine != nil {
				exists, err := s.docEngine.ChunkStoreExists(context.Background(), indexName, doc.KbID)
				if err != nil {
					common.Error(fmt.Sprintf("go side, doc %s, ChunkStoreExists failed", doc.ID), err)
					return common.CodeExceptionError, err
				}
				if exists {
					if _, err := s.docEngine.DeleteChunks(context.Background(), map[string]interface{}{"doc_id": doc.ID}, indexName, doc.KbID); err != nil {
						common.Error(fmt.Sprintf("go side, doc %s, DeleteChunks failed", doc.ID), err)
						return common.CodeExceptionError, err
					}
				}
			}
		}
	}

	return common.CodeSuccess, nil
}
