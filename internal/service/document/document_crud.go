package document

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/knowledge_compile"
	"ragflow/internal/permission"
	permissionresponse "ragflow/internal/permission/response"
	"ragflow/internal/service"
	"ragflow/internal/service/billing"
	"ragflow/internal/service/datasetversion"
	"ragflow/internal/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (s *DocumentService) GetDocumentStorageAddress(ctx context.Context, doc *entity.Document) (string, string, error) {
	if doc == nil {
		return "", "", fmt.Errorf("document is nil")
	}

	file2DocumentDAO := dao.NewFile2DocumentDAO()
	fileDAO := dao.NewFileDAO()

	mappings, err := file2DocumentDAO.GetByDocumentID(ctx, dao.DB, doc.ID)
	if err != nil {
		return "", "", err
	}

	if len(mappings) > 0 && mappings[0].FileID != nil {
		file, err := fileDAO.GetByID(ctx, dao.DB, *mappings[0].FileID)
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

func (s *DocumentService) DownloadDocument(ctx context.Context, datasetID, docID string) (*DownloadDocumentResp, error) {
	if docID == "" {
		return nil, fmt.Errorf("specify document_id please")
	}
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return nil, ErrDocumentNotFound
		}
		return nil, fmt.Errorf("failed to get document %s: %w", docID, err)
	}
	if doc == nil || doc.KbID != datasetID {
		return nil, ErrDocumentNotFound
	}
	bucket, name, err := s.GetDocumentStorageAddress(ctx, doc)
	if err != nil {
		return nil, err
	}

	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil, fmt.Errorf("storage not initialized")
	}

	data, err := storageImpl.Get(ctx, bucket, name)
	if err != nil {
		return nil, err
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
func (s *DocumentService) GetDocumentByID(ctx context.Context, id string) (*DocumentResponse, error) {
	document, err := s.documentDAO.GetByID(ctx, dao.DB, id)
	if err != nil {
		return nil, err
	}

	return s.toResponse(ctx, document)
}

// UpdateDocument update document
func (s *DocumentService) UpdateDocument(ctx context.Context, id string, req *UpdateDocumentRequest) (common.ErrorCode, error) {
	if req == nil {
		return common.CodeDataError, errors.New("invalid request payload")
	}

	document, err := s.documentDAO.GetByID(ctx, dao.DB, id)
	if err != nil {
		if dao.IsNotFoundErr(err) {
			return common.CodeDataError, errors.New("document not found")
		}
		return common.CodeServerError, err
	}

	if err = validateImmutableDocumentFields(document, immutableDocumentFields{
		chunkNum:            req.ChunkNum,
		chunkNumRequestName: "chunk_num",
		tokenNum:            req.TokenNum,
		tokenNumRequestName: "token_num",
		progress:            req.Progress,
		progressMsg:         req.ProgressMsg,
	}); err != nil {
		return common.CodeDataError, err
	}

	if req.Name == nil {
		return common.CodeSuccess, nil
	}
	if code, err := s.validateDocumentName(ctx, document, *req.Name); err != nil {
		return code, err
	}
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, document.KbID)
	if err != nil {
		return common.CodeServerError, err
	}
	if err = s.updateDocumentNameOnly(ctx, document, kb, *req.Name); err != nil {
		return common.CodeServerError, err
	}
	return common.CodeSuccess, nil
}

// ApplyDocCounts records a pipeline run's chunk/token/duration counts on the
// document and rolls the change into its knowledge base aggregate. It is
// idempotent per document: the document row holds this document's own counts, so
// re-applying the same run - e.g. an at-least-once task redelivery that re-runs
// the pipeline - sets the same document values and adjusts the knowledge base by
// a zero delta. It also carries a changed count (a re-parse producing a
// different result) correctly, since only the delta reaches the aggregate. The
// document row is locked so the read-modify-write of its current counts is
// atomic against a concurrent apply, and the aggregate is clamped at zero.
func (s *DocumentService) ApplyDocCounts(ctx context.Context, docID, kbID string, chunkNum, tokenNum int, duration float64) error {
	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var doc entity.Document
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND kb_id = ?", docID, kbID).
			First(&doc).Error; err != nil {
			return err
		}

		chunkDelta := int64(chunkNum) - doc.ChunkNum
		tokenDelta := int64(tokenNum) - doc.TokenNum

		// Set the document to this run's absolute counts.
		if err := tx.WithContext(ctx).Model(&entity.Document{}).
			Where("id = ? AND kb_id = ?", docID, kbID).
			Updates(map[string]interface{}{
				"chunk_num":        int64(chunkNum),
				"token_num":        int64(tokenNum),
				"process_duration": duration,
			}).Error; err != nil {
			return err
		}

		// Roll only the delta into the knowledge base aggregate; a re-applied run
		// contributes zero. Clamp at zero so a stale aggregate cannot go negative.
		if chunkDelta == 0 && tokenDelta == 0 {
			return nil
		}
		return tx.WithContext(ctx).Model(&entity.Knowledgebase{}).
			Where("id = ?", kbID).
			Updates(map[string]interface{}{
				"chunk_num": gorm.Expr("CASE WHEN chunk_num + ? >= 0 THEN chunk_num + ? ELSE 0 END", chunkDelta, chunkDelta),
				"token_num": gorm.Expr("CASE WHEN token_num + ? >= 0 THEN token_num + ? ELSE 0 END", tokenDelta, tokenDelta),
			}).Error
	})
}

// UpdateRunState mirrors live progress into the document row when
// the existing progress log cannot be read. It intentionally leaves the log
// untouched so a later event can retry seeding and append it safely.
func (s *DocumentService) UpdateRunState(ctx context.Context, docID string, progress float64) error {
	updates := map[string]interface{}{
		"progress": progress,
	}
	if doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID); err != nil {
		return err
	} else if doc != nil && doc.ProcessBeginAt != nil {
		duration := time.Since(*doc.ProcessBeginAt).Seconds()
		if duration < 0 {
			duration = 0
		}
		updates["process_duration"] = duration
	}
	return s.documentDAO.UpdateByID(ctx, dao.DB, docID, updates)
}

// DeleteDocument delete document — delegates to full cleanup logic.
func (s *DocumentService) DeleteDocument(ctx context.Context, id string) error {
	// The dataset is resolved before the delete: once the document row is gone
	// there is nothing left to read the owning dataset from, and the version
	// would have nowhere to attach. Its name is captured here for the same
	// reason.
	doc, kb, err := s.resolveDocAndKB(ctx, id)
	if err != nil {
		return err
	}
	change := documentVersionChange(entity.DatasetVersionChangeActionDelete, doc)
	if err = s.deleteDocumentFull(ctx, id); err != nil {
		return err
	}
	// The history is part of the delete: a version that cannot be stored fails
	// the request, so a removal is never silent to a restore.
	if _, err = datasetversion.RecordContentVersionChangesOwnTx(ctx, kb.ID, entity.DatasetVersionKindDelete,
		[]entity.DatasetVersionChange{change}); err != nil {
		return err
	}
	return nil
}

// DeleteDocuments deletes multiple documents under a dataset.
//
//	ids: specific document IDs; deleteAll: delete all docs in the dataset.
//	Returns the number of successfully deleted documents.
func (s *DocumentService) DeleteDocuments(ctx context.Context, ids []string, deleteAll bool, datasetID string) (int, error) {
	return s.deleteDocuments(ctx, ids, deleteAll, datasetID, true)
}

// DeleteDocumentsForRestore removes documents as part of a content restore
// rather than because the user asked to. The restore records the whole
// transition in its own version, so a delete version here would put a
// "documents deleted" entry in the history that no one requested, describing an
// intermediate state that never existed outside the restore. The documents are
// still removed, so the checkpoint the restore wrote can bring them back.
func (s *DocumentService) DeleteDocumentsForRestore(ctx context.Context, ids []string, datasetID string) (int, error) {
	return s.deleteDocuments(ctx, ids, false, datasetID, false)
}

func (s *DocumentService) deleteDocuments(ctx context.Context, ids []string, deleteAll bool, datasetID string, recordVersion bool) (int, error) {
	// Deleting the entire dataset requires dataset-level access; deleting
	// explicit documents also accepts direct document grants.
	if deleteAll {
		if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}, permission.OperationDeleteContent); err != nil {
			_, permissionErr := permissionresponse.Normalize(err)
			return 0, permissionErr
		}
		if err := dao.DB.Model(&entity.Document{}).
			Where("kb_id = ?", datasetID).
			Pluck("id", &ids).Error; err != nil {
			return 0, fmt.Errorf("failed to query documents: %w", err)
		}
	}
	if len(ids) == 0 {
		return 0, nil
	}
	if err := permission.NewDatabaseChecker(dao.DB).CheckResource(ctx, permission.ResourceRef{Kind: permission.ResourceKindDataset, ID: datasetID}, permission.OperationDeleteContent); err != nil &&
		!errors.Is(err, permission.ErrPermissionDenied) && !errors.Is(err, permission.ErrMembershipNotFound) {
		_, permissionErr := permissionresponse.Normalize(err)
		return 0, permissionErr
	}

	// 3. Deduplicate (before validation so dup count doesn't matter)
	ids = common.Deduplicate(ids)

	// Validate IDs and permissions before mutating any document.
	if !deleteAll {
		documents, err := s.validateDocsInDataset(ctx, ids, datasetID)
		if err != nil {
			return 0, err
		}
		for _, doc := range documents {
			if err := service.CheckDocumentAccess(ctx, doc.ID, datasetID, permission.OperationDeleteContent); err != nil {
				_, permissionErr := permissionresponse.Normalize(err)
				return 0, permissionErr
			}
		}
	}

	// 5. Delete each document (non-critical failures are tolerated per doc)
	// The names are read before the rows go: the version's change set shows
	// which documents were removed, and a removed row has no name left to read.
	names := documentNamesByID(ctx, ids)
	deleted := 0
	changes := make([]entity.DatasetVersionChange, 0, len(ids))
	for _, docID := range ids {
		if err := s.deleteDocumentFull(ctx, docID); err != nil {
			common.Warn(fmt.Sprintf("DeleteDocuments: failed to delete %s: %v", docID, err))
			continue
		}
		deleted++
		changes = append(changes, entity.DatasetVersionChange{
			Entity: entity.DatasetVersionChangeEntityDocument,
			Action: entity.DatasetVersionChangeActionDelete,
			DocID:  docID,
			Name:   names[docID],
		})
	}

	// One version per request: the caller asked for one deletion, and the
	// history records that the dataset's content changed; the change set names
	// the documents it took with it. A restore-driven removal is skipped: the
	// restore's own version already describes where the dataset ended up.
	if recordVersion && deleted > 0 {
		if _, err := datasetversion.RecordContentVersionChangesOwnTx(ctx, datasetID, entity.DatasetVersionKindDelete, changes); err != nil {
			return deleted, err
		}
	}

	return deleted, nil
}

// deleteDocumentFull performs full document cleanup. Non-critical failures
// are tolerated (logged and continue). Critical failures (e.g. document or
// KB not found) return an error immediately.
func (s *DocumentService) deleteDocumentFull(ctx context.Context, docID string) error {
	doc, kb, err := s.resolveDocAndKB(ctx, docID)
	if err != nil {
		return err
	}
	unlock := lockDocumentParse(docID)
	defer unlock()

	// Keep the row and its files restorable before anything irreversible runs.
	// A failure aborts the delete: the content history promises this document
	// can be put back, and it cannot be if its row was never copied.
	if err := recordDocumentRemovalEE(ctx, doc, kb); err != nil {
		return err
	}

	// Settle and delete the task before purging its cache state. A conditional
	// delete failure leaves the document and the newly active run intact.
	if err := s.cleanupDocumentTask(ctx, docID, taskTerminalMessageDocumentDeleted); err != nil {
		return err
	}

	// Release an uncommitted parse hold before the document disappears.
	if billingService := billing.Get(); billingService != nil && billingService.Enabled() {
		if releaseErr := billingService.ReleaseDocumentHold(ctx, docID); releaseErr != nil {
			common.Warn(fmt.Sprintf("failed to release billing hold for doc %s: %v", docID, releaseErr))
		}
	}

	variants, taskTypes, typeErr := s.documentKnowledgeCompileTypes(ctx, kb.TenantID, kb.ID, docID)
	if typeErr != nil {
		return fmt.Errorf("resolve knowledge compile types for document %s: %w", docID, typeErr)
	}
	if err := s.deleteDocEngineData(ctx, docID, kb.TenantID, doc.KbID, variants, taskTypes); err != nil {
		return err
	}
	fileIDs, err := s.deleteDocRecordWithCounters(ctx, doc, kb.ID, true)
	if err != nil {
		return err
	}

	fileCleanupCtx := context.WithoutCancel(ctx)
	// Preview artifacts are independent of the source-file cleanup. Remove them
	// before the fallible file cleanup so a file cleanup failure cannot orphan
	// them after the document row is gone.
	cleanupPreviewArtifacts(fileCleanupCtx, doc)

	// The mappings went with the document row, so what is left is best-effort:
	// removing the file rows and blobs no mapping references any more. A failure
	// here leaves an unreferenced file behind, which is recoverable; reporting an
	// error would not be, because the document is already gone and the retry
	// cannot reach it. EE keeps the blob for restore via keepFileBlobForDocumentRestore.
	s.cleanupDocumentFiles(fileCleanupCtx, doc.KbID, fileIDs)

	return nil
}

// RemoveDocumentKeepFile removes a document's chunks/metadata and the document
// row, decrementing the KB counters (doc_num/chunk_num/token_num), WITHOUT
// deleting the underlying file record, its storage blob, or its file2document
// mappings. Mirrors Python DocumentService.remove_document — the caller is
// responsible for cleaning up the file2document mappings separately.
//
// Chunk + metadata cleanup is delegated to deleteDocEngineData so the keep-
// file path matches deleteDocumentFull. The doc_deleted event is published
// from inside deleteDocEngineData; no separate PublishDeleted is needed here.
func (s *DocumentService) RemoveDocumentKeepFile(ctx context.Context, docID string) error {
	doc, kb, err := s.resolveDocAndKB(ctx, docID)
	if err != nil {
		return err
	}
	unlock := lockDocumentParse(docID)
	defer unlock()

	variants, taskTypes, typeErr := s.documentKnowledgeCompileTypes(ctx, kb.TenantID, kb.ID, docID)
	if typeErr != nil {
		return fmt.Errorf("resolve knowledge compile types for document %s: %w", docID, typeErr)
	}

	// Settle and delete the task before purging its cache state. Active workers
	// must be stopped before this path can run.
	if err := s.cleanupDocumentTask(ctx, docID, taskTerminalMessageDocumentRemoved); err != nil {
		return err
	}
	if _, delErr := s.taskDAO.DeleteByDocIDs(ctx, dao.DB, []string{docID}); delErr != nil {
		if errors.Is(delErr, context.Canceled) || errors.Is(delErr, context.DeadlineExceeded) {
			return fmt.Errorf("RemoveDocumentKeepFile: failed to delete tasks for %s: %w", docID, delErr)
		}
		common.Warn(fmt.Sprintf("RemoveDocumentKeepFile: failed to delete tasks for %s: %v", docID, delErr))
	}
	if err := s.deleteDocEngineData(ctx, docID, kb.TenantID, doc.KbID, variants, taskTypes); err != nil {
		return err
	}
	// The mappings stay: the caller is expected to remove them separately, and
	// this path keeps the file row and blob alive.
	if _, err := s.deleteDocRecordWithCounters(ctx, doc, kb.ID, false); err != nil {
		return err
	}
	if _, err := datasetversion.RecordContentVersionChangesOwnTx(ctx, kb.ID, entity.DatasetVersionKindDelete,
		[]entity.DatasetVersionChange{documentVersionChange(entity.DatasetVersionChangeActionDelete, doc)}); err != nil {
		return err
	}
	cleanupPreviewArtifacts(context.WithoutCancel(ctx), doc)
	return nil
}

// InsertDocument creates a document row and increments the owning KB's doc_num
// counter in a single transaction. Mirrors Python DocumentService.insert, which
// updates dataset/document counters on insert. The document's ID and timestamps
// are populated by the caller / model hooks before insertion.
func (s *DocumentService) InsertDocument(ctx context.Context, doc *entity.Document) error {
	if doc.UserID == "" {
		doc.UserID = doc.CreatedBy
	}
	return dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
		if err := grantDocumentOwnerEE(ctx, tx, doc); err != nil {
			return fmt.Errorf("grant document owner: %w", err)
		}
		// The settings the document is created with are its first recorded state:
		// without them a restore to a version that predates the next edit would
		// have nothing to put back. They are parked for the upload version this
		// flow records at the end - written here, inside the insert transaction,
		// they would have no number to carry.
		parkDocumentParserSettingStates(ctx, tx, []*entity.Document{doc})
		// The metadata is the other half of that first state, and it is empty: a
		// document is created without values, so a restore to the version that
		// added it has to put an empty map back instead of leaving whatever a
		// later edit wrote.
		parkDocumentMetadataState(ctx, []*entity.Document{doc})
		return nil
	})
}

// UniqueDocumentName returns a non-colliding document name within the dataset,
// mirroring Python duplicate_name(DocumentService.query, name=..., kb_id=...).
// Returns an error when the existing-name lookup fails so callers never write
// blind and risk duplicated document names.
func (s *DocumentService) UniqueDocumentName(ctx context.Context, kbID, name string) (string, error) {
	return common.UniqueFileName(name, common.NameLimit255, func(candidate string) (bool, error) {
		return s.documentDAO.NameExistsInKB(ctx, dao.DB, kbID, candidate)
	})
}

// resolveDocAndKB loads the document and its knowledgebase, returning both or
// an error.
func (s *DocumentService) resolveDocAndKB(ctx context.Context, docID string) (*entity.Document, *entity.Knowledgebase, error) {
	doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
	if err != nil {
		return nil, nil, fmt.Errorf("document not found: %w", err)
	}
	kb, err := s.kbDAO.GetByID(ctx, dao.DB, doc.KbID)
	if err != nil {
		return nil, nil, fmt.Errorf("knowledgebase not found: %w", err)
	}
	return doc, kb, nil
}

// deleteDocEngineData removes chunks and metadata from the document engine.
// No-op when the engine is nil. Callers pass the resolved compile variants and
// task types so the file-delete hot path does not search for them twice.
func (s *DocumentService) deleteDocEngineData(ctx context.Context, docID, tenantID, kbID string, variants, taskTypes []string) error {
	if s.docEngine == nil {
		return nil
	}
	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	deleteCtx, cancel := context.WithTimeout(ctx, cleanupBatchTimeout)
	_, delErr := s.docEngine.DeleteChunks(deleteCtx, map[string]interface{}{"doc_id": docID}, indexName, kbID)
	if delErr != nil && deleteCtx.Err() == nil {
		// Retry once under a fresh deadline: reusing deleteCtx would start the
		// second attempt with whatever time the first attempt left over.
		retryCtx, retryCancel := context.WithTimeout(ctx, cleanupBatchTimeout)
		_, delErr = s.docEngine.DeleteChunks(retryCtx, map[string]interface{}{"doc_id": docID}, indexName, kbID)
		retryCancel()
	}
	cancel()
	if delErr != nil {
		common.Warn(fmt.Sprintf("deleteDocEngineData: failed to delete chunks for %s: %v", docID, delErr))
		return fmt.Errorf("delete chunks for document %s: %w", docID, delErr)
	}
	// The chunk history is deliberately kept: a content version recorded before
	// this delete can put the document back, and the chunks it restores are read
	// from that history. The removed-document cleanup drops both when the owner
	// deletes the removal row.
	if len(variants) == 0 {
		if s.metadataSvc != nil {
			if err := s.DeleteDocumentAllMetadata(ctx, docID); err != nil {
				return err
			}
		}
		return nil
	}
	// Notify the dataset-level post-processing consumer (§11) that this document's
	// source + per-doc compiled chunks are gone. The consumer removes the
	// dataset-scoped merged products and triggers incremental re-dedup. Best-
	// effort, non-fatal: the synchronous deletion above already removed the
	// source/per-doc chunks, so the consumer only owns merged-product cleanup.
	// Bound the publish with a timeout so a stalled scheduler (MySQL/NATS) can
	// never block the document delete, which already succeeded above.
	pubCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := knowledge_compile.PublishDeleted(pubCtx, tenantID, kbID, docID, variants, taskTypes); err != nil {
		common.Warn(fmt.Sprintf("deleteDocEngineData: publish doc_deleted for %s failed: %v", docID, err))
	}
	if s.metadataSvc != nil {
		if err := s.DeleteDocumentAllMetadata(ctx, docID); err != nil {
			return err
		}
	}
	return nil
}

// deleteDocRecordWithCounters hard-deletes the document row and decrements the
// KB counters in a single transaction. Counters are only decremented when a
// document row was actually removed (RowsAffected > 0), guarding against
// double-decrement on retries or concurrent deletes.
//
// When dropMappings is set, the document's file2document rows are deleted in
// the same transaction and the referenced file ids are returned for the
// caller's best-effort cleanup. The mappings belong in this transaction because
// they are the only thing that keeps the file rows and their blobs reachable:
// deleting them after the document row has already committed turned a mapping
// failure into "the delete reports an error although the document is gone",
// and the retry cannot do anything about it because the row is gone.
func (s *DocumentService) deleteDocRecordWithCounters(ctx context.Context, doc *entity.Document, kbID string, dropMappings bool) ([]string, error) {
	var fileIDs []string
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		fileIDs = nil
		if dropMappings {
			mappings, mapErr := s.file2DocumentDAO.GetByDocumentID(ctx, tx, doc.ID)
			if mapErr != nil {
				return fmt.Errorf("load the file mappings of document %s: %w", doc.ID, mapErr)
			}
			fileIDs = uniqueFileIDs(mappings)
		}

		result := tx.Where("id = ?", doc.ID).Delete(&entity.Document{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete document %s: %w", doc.ID, result.Error)
		}

		if result.RowsAffected > 0 {
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
		}

		if dropMappings {
			// Idempotent, and still right when the row was already gone: the
			// mappings have to go either way, otherwise nothing references the
			// file rows and nothing can find them again.
			if delErr := s.file2DocumentDAO.DeleteByDocumentID(ctx, tx, doc.ID); delErr != nil {
				return fmt.Errorf("delete the file mappings of document %s: %w", doc.ID, delErr)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return fileIDs, nil
}

func (s *DocumentService) rollbackAddFileFromKBError(ctx context.Context, doc *entity.Document, kbID string, err error) error {
	// The rollback runs before the mapping is created, so there is nothing to
	// drop beyond the document row and its counters.
	if _, cleanupErr := s.deleteDocRecordWithCounters(ctx, doc, kbID, false); cleanupErr != nil {
		return fmt.Errorf("%w; rollback cleanup failed: %w", err, cleanupErr)
	}
	return err
}

// uniqueFileIDs lists the distinct file ids a document's mappings point at.
func uniqueFileIDs(mappings []*entity.File2Document) []string {
	seen := make(map[string]bool, len(mappings))
	fileIDs := make([]string, 0, len(mappings))
	for _, m := range mappings {
		if m == nil || m.FileID == nil || seen[*m.FileID] {
			continue
		}
		seen[*m.FileID] = true
		fileIDs = append(fileIDs, *m.FileID)
	}
	return fileIDs
}

// cleanupDocumentFiles drops the file rows and storage blobs of files no
// document references any more. It receives the ids read while the document's
// mappings were deleted inside the delete transaction, so the mappings are
// already gone and every step here is best-effort: a failure leaves an
// unreferenced file behind, which a later sweep can collect, instead of a
// document that reports a failed delete while being gone.
//
// A file is only hard-deleted when it is a knowledgebase-owned upload
// (source_type == knowledgebase); files linked from file management keep their
// row and blob, only the mapping is removed.
func (s *DocumentService) cleanupDocumentFiles(ctx context.Context, kbID string, fileIDs []string) {
	// For each file, only delete the record and blob when it is a
	// knowledgebase-owned upload and no other doc references it
	for _, fileID := range fileIDs {
		remaining, remErr := s.file2DocumentDAO.GetByFileID(ctx, dao.DB, fileID)
		if remErr != nil {
			common.Warn(fmt.Sprintf("Document file cleanup: failed to check remaining f2d for %s: %v", fileID, remErr))
			continue
		}
		if len(remaining) > 0 {
			continue
		}

		fileDAO := dao.NewFileDAO()
		file, fErr := fileDAO.GetByID(ctx, dao.DB, fileID)
		if fErr != nil || file == nil {
			common.Warn(fmt.Sprintf("Document file cleanup: file not found %s: %v", fileID, fErr))
			continue
		}
		if entity.FileSource(file.SourceType) != entity.FileSourceKnowledgebase {
			continue // linked from file management — unlink only, keep the file
		}
		if _, delErr := fileDAO.DeleteByIDs(ctx, dao.DB, []string{fileID}); delErr != nil {
			common.Warn(fmt.Sprintf("Document file cleanup: failed to delete file %s: %v", fileID, delErr))
			continue // keep the blob so the live file row still has its object
		}
		// An edition that keeps content history leaves the blob in storage: the
		// removal row records where it lives, and removing the removed document
		// deletes it.
		if file.Location != nil && *file.Location != "" && !keepFileBlobForDocumentRestore() {
			storageImpl := storage.GetStorageFactory().GetStorage()
			if storageImpl != nil {
				// Dataset uploads use the KB ID; ParentID is the file-manager folder.
				rmErr := storage.RemoveBlob(ctx, storageImpl, kbID, *file.Location)
				if rmErr != nil {
					common.Warn(fmt.Sprintf("Document file cleanup: failed to remove blob %s/%s: %v", kbID, *file.Location, rmErr))
				}
			}
		}
	}
}
