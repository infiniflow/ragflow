package document

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/dao"
	"ragflow/internal/service"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
)

func (s *DocumentService) ListIngestionTasks(ctx context.Context, datasetID *string, page, pageSize int) ([]*entity.IngestionTask, error) {
	return s.ingestionTaskSvc.List(ctx, datasetID, page, pageSize)
}

func (s *DocumentService) IngestDocuments(ctx context.Context, datasetID string, docIDs []string) ([]*service.ParseDocumentResponse, error) {
	if permission.UserIDFromContext(ctx) == "" {
		return nil, permission.ErrUnauthenticated
	}
	if _, err := s.kbDAO.GetByID(ctx, dao.DB, datasetID); err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, permission.ErrResourceNotFound
		}
		return nil, fmt.Errorf("failed to fetch dataset: %w", err)
	}
	for _, docID := range common.Deduplicate(docIDs) {
		doc, err := s.documentDAO.GetByDocumentIDAndDatasetID(ctx, dao.DB, docID, datasetID)
		if err != nil {
			if dao.IsNotFoundErr(err) {
				continue
			}
			return nil, fmt.Errorf("failed to fetch document: %w", err)
		}
		if doc == nil {
			continue
		}
		if err := service.CheckDocumentWrite(ctx, docID, datasetID, permission.OperationRun, ""); err != nil {
			_, permissionErr := permissionresponse.Normalize(err)
			return nil, permissionErr
		}
	}
	responses, err := s.ingestionTaskSvc.CreateForDocuments(ctx, datasetID, docIDs)
	if err != nil {
		return nil, err
	}
	common.Info(fmt.Sprintf("parse documents, dataset: %s, documents: %v", datasetID, docIDs))
	return responses, nil
}

func (s *DocumentService) StopIngestionTasks(ctx context.Context, tasks []string) ([]*entity.IngestionTask, error) {
	return s.stopIngestionTasks(ctx, tasks)
}

func (s *DocumentService) RemoveIngestionTasks(ctx context.Context, tasks []string) ([]map[string]string, error) {
	userID := permission.UserIDFromContext(ctx)
	return s.ingestionTaskSvc.RemoveMany(ctx, tasks, &userID)
}

func (s *DocumentService) Ingest(ctx context.Context, req *IngestDocumentRequest) (common.ErrorCode, error) {
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

	// First pass: validate every document exists before mutating any state, so
	// a single invalid doc rejects the whole request. Collect the distinct
	// datasets once: a batch often contains many documents from the same KB,
	// and authorizing/loading that KB per document is pure overhead.
	kbIDs := make([]string, 0, len(req.DocIDs))
	seenKB := make(map[string]struct{}, len(req.DocIDs))
	for _, docID := range req.DocIDs {
		doc := docsByID[docID]
		if doc == nil {
			return common.CodeDataError, fmt.Errorf("document not found")
		}
		if _, seen := seenKB[doc.KbID]; seen {
			continue
		}
		seenKB[doc.KbID] = struct{}{}
		kbIDs = append(kbIDs, doc.KbID)
	}
	// Authorize every distinct dataset with one resource load and fetch them
	// with one query. The per-document loop below still decides which error
	// wins, in document order: a missing dataset and a denied dataset are
	// reported exactly as before when the old code checked each document in
	// turn (GetByID first, then the dataset write check).
	checker := permission.NewDatabaseChecker(dao.DB)
	refs := make([]permission.ResourceRef, 0, len(kbIDs))
	for _, kbID := range kbIDs {
		refs = append(refs, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: kbID})
	}
	accessible, filterErr := checker.FilterResources(ctx, refs, permission.OperationRun)
	if filterErr != nil {
		code, permissionErr := permissionresponse.Normalize(filterErr)
		return code, permissionErr
	}
	allowed := make(map[permission.ResourceRef]bool, len(accessible))
	for _, ref := range accessible {
		allowed[ref] = true
	}
	loadedKBs, loadErr := service.LoadKBsInOrder(ctx, dao.DB, s.kbDAO, kbIDs)
	if loadErr != nil {
		return common.CodeDataError, fmt.Errorf("dataset not found")
	}
	kbByID := make(map[string]*entity.Knowledgebase, len(loadedKBs))
	for i, kb := range loadedKBs {
		if kb != nil {
			kbByID[kbIDs[i]] = kb
		}
	}

	type validatedDoc struct {
		doc *entity.Document
		kb  *entity.Knowledgebase
	}
	validated := make([]validatedDoc, 0, len(req.DocIDs))
	validatedIDs := make([]string, 0, len(req.DocIDs))
	for _, docID := range req.DocIDs {
		doc := docsByID[docID]
		kb := kbByID[doc.KbID]
		if kb == nil {
			return common.CodeDataError, fmt.Errorf("dataset not found")
		}
		ref := permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: doc.KbID}
		if !allowed[ref] {
			if err := checker.CheckResource(ctx, ref, permission.OperationRun); err != nil {
				code, permissionErr := permissionresponse.Normalize(err)
				return code, permissionErr
			}
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
			if err = s.StartParseDocuments(ctx, doc, kb, StartParseOptions{
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
				if errors.Is(err, ErrParseNotRunning) {
					// Mirror the Python /documents/ingest endpoint's message.
					return common.CodeDataError, errors.New("Cannot cancel a task that is not in RUNNING status")
				}
				return common.CodeDataError, err
			}
			if err = s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{
				"progress": 0,
				// The content this document holds is about to be replaced by the run,
				// so the marker saying it was replayed from a dataset version no
				// longer describes it.
				"restored_version": nil,
			}); err != nil {
				common.Error(fmt.Sprintf("go side, doc %s, UpdateByID failed", doc.ID), err)
				return common.CodeExceptionError, err
			}
			continue
		}

		if req.Delete {
			// Delete-only: user asked to remove prior parse results without
			// starting a new parse. Serialize the counter reset and cleanup with
			// StartParseDocuments so a concurrent parse cannot reuse the task
			// while its old cache state is being removed.
			unlock := lockDocumentParse(doc.ID)
			if err = s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{
				"progress": 0,
				// The content this document holds is about to be replaced by the run,
				// so the marker saying it was replayed from a dataset version no
				// longer describes it.
				"restored_version": nil,
			}); err != nil {
				unlock()
				common.Error(fmt.Sprintf("go side, doc %s, UpdateByID failed", doc.ID), err)
				return common.CodeExceptionError, err
			}
			if err := s.clearDocumentParseResults(ctx, doc, kb.TenantID, taskTerminalMessageParseResultsDeleted); err != nil {
				unlock()
				return common.CodeExceptionError, err
			}
			unlock()
			continue
		}

		if err = s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{
			"progress": 0,
			// The content this document holds is about to be replaced by the run,
			// so the marker saying it was replayed from a dataset version no longer
			// describes it.
			"restored_version": nil,
		}); err != nil {
			common.Error(fmt.Sprintf("go side, doc %s, UpdateByID failed", doc.ID), err)
			return common.CodeExceptionError, err
		}
	}

	return common.CodeSuccess, nil
}
