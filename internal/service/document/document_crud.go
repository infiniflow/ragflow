package document

import (
	"context"
	"fmt"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"gorm.io/gorm"
)

// Accessible reports whether docID belongs to a knowledge base
// reachable by userID. Used by agent endpoints (e.g. RerunAgent,
// PR #15145) to gate destructive / run-again actions on a document
// the caller has access to. Returns false on any lookup failure or
// empty inputs so callers can treat a denial as a 404-equivalent
// and avoid leaking whether the document exists at all.
func (s *DocumentService) Accessible(docID, userID string) bool {
	if docID == "" || userID == "" {
		return false
	}
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil || doc == nil {
		return false
	}
	return s.kbDAO.Accessible(doc.KbID, userID)
}

func (s *DocumentService) GetDocumentStorageAddress(doc *entity.Document) (string, string, error) {
	if doc == nil {
		return "", "", fmt.Errorf("document is nil")
	}

	file2DocumentDAO := dao.NewFile2DocumentDAO()
	fileDAO := dao.NewFileDAO()

	mappings, err := file2DocumentDAO.GetByDocumentID(doc.ID)
	if err != nil {
		return "", "", err
	}

	if len(mappings) > 0 && mappings[0].FileID != nil {
		file, err := fileDAO.GetByID(*mappings[0].FileID)
		if err != nil {
			return "", "", err
		}

		if file.SourceType == "" || entity.FileSource(file.SourceType) == entity.FileSourceLocal {
			if file.Location == nil || *file.Location == "" {
				return "", "", fmt.Errorf("file location is empty")
			}
			return file.ParentID, *file.Location, nil
		}
	}

	if doc.Location == nil || *doc.Location == "" {
		return "", "", fmt.Errorf("document location is empty")
	}
	return doc.KbID, *doc.Location, nil
}

func (s *DocumentService) DownloadDocument(datasetID, docID string) (*DownloadDocumentResp, error) {
	if docID == "" {
		return nil, fmt.Errorf("Specify document_id please.")
	}
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil || doc.KbID != datasetID {
		return nil, fmt.Errorf("Document not found!")
	}
	bucket, name, err := s.GetDocumentStorageAddress(doc)
	if err != nil {
		return nil, err
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	data, err := storageImpl.Get(bucket, name)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("This file is empty.")
	}

	fileName := ""
	if doc.Name != nil {
		fileName = *doc.Name
	}

	return &DownloadDocumentResp{
		Data:        data,
		FileName:    fileName,
		ContentType: "application/octet-stream",
	}, nil
}

// GetDocumentByID get document by ID
func (s *DocumentService) GetDocumentByID(id string) (*DocumentResponse, error) {
	document, err := s.documentDAO.GetByID(id)
	if err != nil {
		return nil, err
	}

	return s.toResponse(document), nil
}

// UpdateDocument update document
func (s *DocumentService) UpdateDocument(id string, req *UpdateDocumentRequest) error {
	document, err := s.documentDAO.GetByID(id)
	if err != nil {
		return err
	}

	if req.Name != nil {
		document.Name = req.Name
	}
	if req.Run != nil {
		document.Run = req.Run
	}
	if req.TokenNum != nil {
		document.TokenNum = *req.TokenNum
	}
	if req.ChunkNum != nil {
		document.ChunkNum = *req.ChunkNum
	}
	if req.Progress != nil {
		document.Progress = *req.Progress
	}
	if req.ProgressMsg != nil {
		document.ProgressMsg = req.ProgressMsg
	}

	return s.documentDAO.Update(document)
}

// IncrementChunkNum atomically increments chunk/token counters on the document and its knowledge base in a transaction
func (s *DocumentService) IncrementChunkNum(docID, kbID string, chunkNum, tokenNum int, duration float64) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		// Update document
		if err := tx.Model(&entity.Document{}).
			Where("id = ? AND kb_id = ?", docID, kbID).
			Updates(map[string]interface{}{
				"chunk_num":        gorm.Expr("chunk_num + ?", int64(chunkNum)),
				"token_num":        gorm.Expr("token_num + ?", int64(tokenNum)),
				"process_duration": gorm.Expr("process_duration + ?", duration),
			}).Error; err != nil {
			return err
		}

		// Update knowledgebase
		if err := tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", kbID).
			Updates(map[string]interface{}{
				"chunk_num": gorm.Expr("chunk_num + ?", int64(chunkNum)),
				"token_num": gorm.Expr("token_num + ?", int64(tokenNum)),
			}).Error; err != nil {
			return err
		}

		return nil
	})
}

// UpdateRunProgress mirrors a pipeline run's live progress into the document
// row so the document-list endpoint (which reads document.progress/run/
// progress_msg) reflects in-flight Go pipeline progress. Best-effort by
// design; callers log and continue on error.
func (s *DocumentService) UpdateRunProgress(docID string, progress float64, run, progressMsg string) error {
	return s.documentDAO.UpdateByID(docID, map[string]interface{}{
		"progress":     progress,
		"run":          run,
		"progress_msg": progressMsg,
	})
}

// DeleteDocument delete document — delegates to full cleanup logic.
func (s *DocumentService) DeleteDocument(id string) error {
	return s.deleteDocumentFull(id)
}

// DeleteDocuments deletes multiple documents under a dataset.
//
//	ids: specific document IDs; deleteAll: delete all docs in the dataset.
//	Returns the number of successfully deleted documents.
func (s *DocumentService) DeleteDocuments(ids []string, deleteAll bool, datasetID, userID string) (int, error) {
	// 1. Check dataset is accessible by the user
	if !s.kbDAO.Accessible(datasetID, userID) {
		return 0, fmt.Errorf("You don't own the dataset %s.", datasetID)
	}

	// 2. Resolve document IDs
	if deleteAll {
		if err := dao.DB.Model(&entity.Document{}).
			Where("kb_id = ?", datasetID).
			Pluck("id", &ids).Error; err != nil {
			return 0, fmt.Errorf("failed to query documents: %w", err)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}

	// 3. Deduplicate (before validation so dup count doesn't matter)
	ids = common.Deduplicate(ids)

	// 4. Validate IDs belong to this dataset (only for explicit ids; deleteAll is already scoped)
	if !deleteAll {
		if _, err := s.validateDocsInDataset(ids, datasetID); err != nil {
			return 0, err
		}
	}

	// 5. Delete each document (non-critical failures are tolerated per doc)
	deleted := 0
	for _, docID := range ids {
		if err := s.deleteDocumentFull(docID); err != nil {
			common.Warn(fmt.Sprintf("DeleteDocuments: failed to delete %s: %v", docID, err))
			continue
		}
		deleted++
	}

	return deleted, nil
}

// deleteDocumentFull performs full document cleanup. Non-critical failures
// are tolerated (logged and continue). Critical failures (e.g. document or
// KB not found) return an error immediately.
func (s *DocumentService) deleteDocumentFull(docID string) error {
	doc, kb, err := s.resolveDocAndKB(docID)
	if err != nil {
		return err
	}

	// Delete tasks from DB
	ingestionTask, err := s.ingestionTaskDAO.GetByDocumentID(docID)
	if err != nil {
		common.Error(fmt.Sprintf("failed to get ingestion task by doc:%s", doc.ID), err)
		return err
	}
	if ingestionTask != nil {
		taskInfo, err := s.ingestionTaskSvc.Remove(ingestionTask.ID, &ingestionTask.UserID)
		if err != nil {
			return err
		}
		// FIXME: need to add logic to delete files in taskInfo
		common.Warn(fmt.Sprintf("need to delete files from taskInfo: %v", taskInfo))
	}

	s.deleteDocEngineData(docID, kb.TenantID, doc.KbID)
	if err := s.deleteDocRecordWithCounters(doc, kb.ID); err != nil {
		return err
	}
	s.cleanupFileReferences(docID)

	return nil
}

// RemoveDocumentKeepFile removes a document's chunks/metadata and the document
// row, decrementing the KB counters (doc_num/chunk_num/token_num), WITHOUT
// deleting the underlying file record, its storage blob, or its file2document
// mappings. Mirrors Python DocumentService.remove_document — the caller is
// responsible for cleaning up the file2document mappings separately.
func (s *DocumentService) RemoveDocumentKeepFile(docID string) error {
	doc, kb, err := s.resolveDocAndKB(docID)
	if err != nil {
		return err
	}
	if _, delErr := s.taskDAO.DeleteByDocIDs([]string{docID}); delErr != nil {
		common.Logger.Warn(fmt.Sprintf("RemoveDocumentKeepFile: failed to delete tasks for %s: %v", docID, delErr))
	}
	s.deleteDocEngineData(docID, kb.TenantID, doc.KbID)
	return s.deleteDocRecordWithCounters(doc, kb.ID)
}

// InsertDocument creates a document row and increments the owning KB's doc_num
// counter in a single transaction. Mirrors Python DocumentService.insert, which
// updates dataset/document counters on insert. The document's ID and timestamps
// are populated by the caller / model hooks before insertion.
func (s *DocumentService) InsertDocument(doc *entity.Document) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(doc).Error; err != nil {
			return fmt.Errorf("failed to create document: %w", err)
		}
		// Guard the counter bump with RowsAffected: documents.kb_id has no DB-level
		// FK, so Create can succeed against a non-existent KB and the Update would
		// then report a nil error with 0 rows touched, silently desyncing doc_num.
		// Roll the whole transaction back in that case (mirrors the counter checks
		// in deleteDocRecordWithCounters).
		result := tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", doc.KbID).
			Update("doc_num", gorm.Expr("doc_num + 1"))
		if result.Error != nil {
			return fmt.Errorf("failed to increment doc_num for KB %s: %w", doc.KbID, result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("knowledgebase %s not found", doc.KbID)
		}
		return nil
	})
}

// resolveDocAndKB loads the document and its knowledgebase, returning both or
// an error.
func (s *DocumentService) resolveDocAndKB(docID string) (*entity.Document, *entity.Knowledgebase, error) {
	doc, err := s.documentDAO.GetByID(docID)
	if err != nil {
		return nil, nil, fmt.Errorf("document not found: %w", err)
	}
	kb, err := s.kbDAO.GetByID(doc.KbID)
	if err != nil {
		return nil, nil, fmt.Errorf("knowledgebase not found: %w", err)
	}
	return doc, kb, nil
}

// deleteDocEngineData removes chunks and metadata from the document engine.
// No-op when the engine is nil.
func (s *DocumentService) deleteDocEngineData(docID, tenantID, kbID string) {
	if s.docEngine == nil {
		return
	}
	ctx := context.Background()
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	if _, delErr := s.docEngine.DeleteChunks(ctx, map[string]interface{}{"doc_id": docID}, indexName, kbID); delErr != nil {
		common.Logger.Warn(fmt.Sprintf("deleteDocEngineData: failed to delete chunks for %s: %v", docID, delErr))
	}
	if s.metadataSvc != nil {
		_ = s.DeleteDocumentAllMetadata(docID) // logs internally
	}
}

// deleteDocRecordWithCounters hard-deletes the document row and decrements the
// KB counters in a single transaction. Counters are only decremented when a
// document row was actually removed (RowsAffected > 0), guarding against
// double-decrement on retries or concurrent deletes.
func (s *DocumentService) deleteDocRecordWithCounters(doc *entity.Document, kbID string) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Where("id = ?", doc.ID).Delete(&entity.Document{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete document %s: %w", doc.ID, result.Error)
		}
		if result.RowsAffected == 0 {
			return nil // already deleted by a concurrent request — skip counters
		}

		result = tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", kbID).
			Updates(map[string]interface{}{
				"doc_num":   gorm.Expr("doc_num - 1"),
				"chunk_num": gorm.Expr("chunk_num - ?", doc.ChunkNum),
				"token_num": gorm.Expr("token_num - ?", doc.TokenNum),
			})
		if result.Error != nil {
			return fmt.Errorf("failed to decrement counters for KB %s: %w", kbID, result.Error)
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("knowledgebase %s not found", kbID)
		}
		return nil
	})
}

func (s *DocumentService) rollbackAddFileFromKBError(doc *entity.Document, kbID string, err error) error {
	if cleanupErr := s.deleteDocRecordWithCounters(doc, kbID); cleanupErr != nil {
		return fmt.Errorf("%w; rollback cleanup failed: %w", err, cleanupErr)
	}
	return err
}

// cleanupFileReferences deletes file2document mappings for docID, and for each
// referenced file, only hard-deletes the file record and its storage blob when
// no other document still references the same file_id.
func (s *DocumentService) cleanupFileReferences(docID string) {
	mappings, mapErr := s.file2DocumentDAO.GetByDocumentID(docID)
	if mapErr != nil {
		common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: failed to get f2d mappings for %s: %v", docID, mapErr))
	}
	if len(mappings) == 0 {
		return
	}

	// Collect unique file_ids
	seen := make(map[string]bool)
	var fileIDs []string
	for _, m := range mappings {
		if m.FileID == nil || seen[*m.FileID] {
			continue
		}
		seen[*m.FileID] = true
		fileIDs = append(fileIDs, *m.FileID)
	}

	// Delete all file2document rows for this document
	if delErr := s.file2DocumentDAO.DeleteByDocumentID(docID); delErr != nil {
		common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: failed to delete f2d for %s: %v", docID, delErr))
	}

	// For each file, only delete the record and blob when no other doc references it
	for _, fileID := range fileIDs {
		remaining, remErr := s.file2DocumentDAO.GetByFileID(fileID)
		if remErr != nil {
			common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: failed to check remaining f2d for %s: %v", fileID, remErr))
			continue
		}
		if len(remaining) > 0 {
			continue
		}

		fileDAO := dao.NewFileDAO()
		file, fErr := fileDAO.GetByID(fileID)
		if fErr != nil || file == nil {
			common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: file not found %s: %v", fileID, fErr))
			continue
		}
		if _, delErr := fileDAO.DeleteByIDs([]string{fileID}); delErr != nil {
			common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: failed to delete file %s: %v", fileID, delErr))
			continue // keep the blob so the live file row still has its object
		}
		if file.Location != nil && *file.Location != "" {
			storageImpl := storage.GetStorageFactory().GetStorage()
			if storageImpl != nil {
				if rmErr := storageImpl.Remove(file.ParentID, *file.Location); rmErr != nil {
					common.Logger.Warn(fmt.Sprintf("cleanupFileReferences: failed to remove blob %s/%s: %v", file.ParentID, *file.Location, rmErr))
				}
			}
		}
	}
}
