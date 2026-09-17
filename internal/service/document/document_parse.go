package document

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/service"
	"strconv"
	"strings"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/knowledge_compile"
	ingestionpipeline "ragflow/internal/ingestion/pipeline"
	"ragflow/internal/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type documentParseLock struct {
	mu   sync.Mutex
	refs int
}

var documentParseLocks = struct {
	sync.Mutex
	locks map[string]*documentParseLock
}{locks: make(map[string]*documentParseLock)}

const (
	cleanupClaimLeaseSeconds  int64 = 120
	cleanupTakeoverGraceSecs  int64 = 45
	cleanupClaimRenewInterval       = 30 * time.Second
	cleanupBatchTimeout             = 30 * time.Second
)

func lockDocumentParse(docID string) func() {
	documentParseLocks.Lock()
	lock := documentParseLocks.locks[docID]
	if lock == nil {
		lock = &documentParseLock{}
		documentParseLocks.locks[docID] = lock
	}
	lock.refs++
	documentParseLocks.Unlock()

	lock.mu.Lock()
	return func() {
		lock.mu.Unlock()
		documentParseLocks.Lock()
		lock.refs--
		if lock.refs == 0 {
			delete(documentParseLocks.locks, docID)
		}
		documentParseLocks.Unlock()
	}
}

func (s *DocumentService) acquireCleanupClaim(ctx context.Context, documentID, owner string) (*entity.DocumentCleanupClaim, error) {
	if s.cleanupClaimDAO == nil {
		return nil, errors.New("document cleanup claim DAO is nil")
	}
	var claim *entity.DocumentCleanupClaim
	err := dao.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := s.documentDAO.GetByIDForUpdate(ctx, tx, documentID); err != nil {
			return err
		}
		now, err := dao.CurrentUnixTime(ctx, tx)
		if err != nil {
			return err
		}
		claim, err = s.cleanupClaimDAO.Acquire(ctx, tx, documentID, owner, now, cleanupClaimLeaseSeconds, cleanupTakeoverGraceSecs)
		return err
	})
	return claim, err
}

func (s *DocumentService) renewCleanupClaim(ctx context.Context, documentID, token string) error {
	now, err := dao.CurrentUnixTime(ctx, dao.DB)
	if err != nil {
		return err
	}
	return s.cleanupClaimDAO.Renew(ctx, dao.DB, documentID, token, now, cleanupClaimLeaseSeconds)
}

// startCleanupClaimHeartbeat keeps a cleanup claim alive while the workflow
// performs database work and external I/O between its fenced batch
// boundaries. Each destructive batch still renews and validates independently;
// the heartbeat only prevents a healthy long-running owner from expiring in
// the gaps between those batches.
func (s *DocumentService) startCleanupClaimHeartbeat(ctx context.Context, documentID, token string) func() {
	if token == "" || s.cleanupClaimDAO == nil {
		return func() {}
	}
	heartbeatCtx, stop := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(cleanupClaimRenewInterval)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				if err := s.renewCleanupClaim(heartbeatCtx, documentID, token); err != nil {
					common.Warn(fmt.Sprintf("renew cleanup claim for document %s: %v", documentID, err))
				}
			}
		}
	}()
	return func() {
		stop()
		<-done
	}
}

func (s *DocumentService) cleanupClaimToken(ctx context.Context, documentID string) string {
	token, ok := service.DocumentCleanupClaimFromContext(ctx, documentID)
	if !ok || s.cleanupClaimDAO == nil {
		return ""
	}
	return token
}

// purgeTaskStateForCleanup removes resumable checkpoint, tracker, and chunk
// cache state before a task row is deleted. The bounded context makes Redis
// outages and hung clients observable to the caller instead of allowing a
// destructive task/document delete to proceed with stale resume state.
func (s *DocumentService) purgeTaskStateForCleanup(ctx context.Context, taskID string) error {
	purgeTaskState := s.purgeTaskState
	if purgeTaskState == nil {
		purgeTaskState = ingestionpipeline.PurgeTaskState
	}
	batchCtx, cancel := context.WithTimeout(ctx, cleanupBatchTimeout)
	defer cancel()
	return purgeTaskState(batchCtx, taskID)
}

// beginCleanupBatch renews the request's fencing claim immediately before an
// external storage operation. A missing token means this helper is being used
// by an internal path that does not hold a cleanup claim.
func (s *DocumentService) beginCleanupBatch(ctx context.Context, documentID, token string) error {
	if token == "" {
		return nil
	}
	return s.renewCleanupClaim(ctx, documentID, token)
}

// finishCleanupBatch verifies that the same fencing token still owns an
// unexpired claim after an external operation. A takeover during the call
// therefore stops the cleanup before another batch can mutate storage.
func (s *DocumentService) finishCleanupBatch(ctx context.Context, documentID, token string) error {
	if token == "" {
		return nil
	}
	now, err := dao.CurrentUnixTime(ctx, dao.DB)
	if err != nil {
		return err
	}
	if !s.cleanupClaimDAO.Validate(ctx, dao.DB, documentID, token, now) {
		return dao.ErrDocumentCleanupClaimLost
	}
	return nil
}

// StartParseDocuments starts parsing a document via the DSL ingestion
// pipeline. It optionally clears prior results (RerunWithDelete), applies
// KB config (ApplyKB), validates storage, and enqueues an ingestion task.
// Extracted from Ingest so other entry points (e.g. ChunkService.Parse)
// can reuse the same start-parse flow.
func (s *DocumentService) StartParseDocuments(ctx context.Context, doc *entity.Document, kb *entity.Knowledgebase, userID string, opts StartParseOptions) error {
	// Validate storage first so we don't clear prior results and then fail
	// because the document can't be read, leaving the document with neither
	// old nor new parse results.
	if _, _, err := s.GetDocumentStorageAddress(ctx, doc); err != nil {
		return err
	}
	unlock := lockDocumentParse(doc.ID)
	defer unlock()
	runContext := ctx

	if opts.RerunWithDelete {
		claim, err := s.acquireCleanupClaim(ctx, doc.ID, fmt.Sprintf("document-service:%s", userID))
		if err != nil {
			return fmt.Errorf("acquire cleanup claim for document %s: %w", doc.ID, err)
		}
		stopHeartbeat := s.startCleanupClaimHeartbeat(ctx, doc.ID, claim.Token)
		defer stopHeartbeat()
		defer func() {
			if _, err := s.cleanupClaimDAO.Release(context.WithoutCancel(ctx), dao.DB, doc.ID, claim.Token); err != nil {
				common.Warn(fmt.Sprintf("release cleanup claim for document %s: %v", doc.ID, err))
			}
		}()
		runContext = service.WithDocumentCleanupClaim(ctx, doc.ID, claim.Token)
		if err := s.clearDocumentParseResults(runContext, doc, kb.TenantID); err != nil {
			return err
		}
		if err := s.renewCleanupClaim(runContext, doc.ID, claim.Token); err != nil {
			return fmt.Errorf("renew cleanup claim for document %s: %w", doc.ID, err)
		}
	}

	responses, err := s.IngestDocuments(runContext, doc.KbID, userID, []string{doc.ID})
	if err != nil {
		return err
	}
	if len(responses) == 0 {
		return fmt.Errorf("failed to enqueue document %s: empty ingestion response", doc.ID)
	}
	if !strings.HasPrefix(responses[0].Result, "task_id:") {
		return fmt.Errorf("failed to enqueue document %s: %s", doc.ID, responses[0].Result)
	}
	return nil
}

// AssertIngestionTasksTerminal verifies none of the documents has an
// in-flight (RUNNING/STOPPING) ingestion task. Used as a batch pre-check
// before re-parsing so a single non-terminal doc rejects the whole request
// up front instead of partially cleaning some docs then failing.
func (s *DocumentService) AssertIngestionTasksTerminal(ctx context.Context, docIDs []string) error {
	for _, docID := range docIDs {
		task, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, docID)
		if err != nil {
			return fmt.Errorf("check ingestion task for %s: %w", docID, err)
		}
		if task == nil {
			continue
		}
		if common.IsRunningOrStopping(task.Status) {
			return fmt.Errorf("document %s ingestion task is %s; stop it and wait for a terminal state before re-parsing", docID, task.Status)
		}
	}
	return nil
}

func (s *DocumentService) clearDocumentParseResults(ctx context.Context, doc *entity.Document, tenantID string) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}
	claimToken := s.cleanupClaimToken(ctx, doc.ID)
	if err := s.beginCleanupBatch(ctx, doc.ID, claimToken); err != nil {
		return fmt.Errorf("begin cleanup for document %s: %w", doc.ID, err)
	}

	// Refuse to clear a non-terminal ingestion task. An in-flight worker
	// (RUNNING) or one mid-stop (STOPPING) would keep writing chunks and
	// corrupt the new run's results. The caller must stop the task first
	// and wait for a terminal state (COMPLETED/STOPPED/FAILED), CREATED, or
	// SCHEDULED.
	task, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, doc.ID)
	if err != nil {
		return fmt.Errorf("get ingestion task for document %s: %w", doc.ID, err)
	}
	taskExisted := task != nil
	if task != nil {
		if task.Status == common.RUNNING || task.Status == common.STOPPING {
			return fmt.Errorf("document %s ingestion task is %s; stop it and wait for a terminal state before re-parsing", doc.ID, task.Status)
		}
		if task.Status == common.CREATED || task.Status == common.SCHEDULED {
			if err := s.ingestionTaskSvc.SupersedeUnstartedTask(ctx, task.ID); err != nil {
				return fmt.Errorf("supersede queued ingestion task for document %s: %w", doc.ID, err)
			}
			task.Status = common.STOPPED
		}
		taskExisted = true
	}
	if task != nil {
		if err := s.beginCleanupBatch(ctx, doc.ID, claimToken); err != nil {
			return fmt.Errorf("begin task state cleanup for document %s: %w", doc.ID, err)
		}
		if err := s.purgeTaskStateForCleanup(ctx, task.ID); err != nil {
			return fmt.Errorf("purge task state for document %s: %w", doc.ID, err)
		}
		if err := s.finishCleanupBatch(ctx, doc.ID, claimToken); err != nil {
			return fmt.Errorf("cleanup claim for document %s was lost: %w", doc.ID, err)
		}
	}

	// Delete terminal, CREATED, and SCHEDULED ingestion tasks atomically, leaving
	// RUNNING/STOPPING tasks untouched so the check-then-delete window
	// between GetByDocumentID and the delete cannot delete a task that just
	// transitioned to RUNNING. In that case, do not clear its parse results.
	deleted, err := s.ingestionTaskDAO.DeleteIfTerminal(ctx, dao.DB, doc.ID)
	if err != nil {
		return err
	}
	if taskExisted && deleted == 0 {
		return fmt.Errorf("document %s ingestion task started running; stop it before re-parsing", doc.ID)
	}
	if err := s.clearDocumentAndKBCountersForRerun(doc.ID, doc.KbID); err != nil {
		return err
	}

	if s.docEngine == nil {
		return nil
	}
	if err := s.finishCleanupBatch(ctx, doc.ID, claimToken); err != nil {
		return fmt.Errorf("cleanup claim for document %s was lost: %w", doc.ID, err)
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	exists, err := s.docEngine.ChunkStoreExists(ctx, indexName, doc.KbID)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	_, taskTypes, err := s.documentKnowledgeCompileTypes(ctx, tenantID, doc.KbID, doc.ID)
	if err != nil {
		return fmt.Errorf("resolve generated products for document %s: %w", doc.ID, err)
	}
	if err := s.deleteDocumentGeneratedChunks(ctx, tenantID, doc.KbID, doc.ID); err != nil {
		return fmt.Errorf("delete generated products for document %s: %w", doc.ID, err)
	}
	if err := s.deleteDocumentChunkImages(ctx, indexName, doc.KbID, doc.ID); err != nil {
		return fmt.Errorf("delete chunk images for document %s: %w", doc.ID, err)
	}
	if err = s.deleteSourceChunks(ctx, tenantID, doc.KbID, doc.ID); err != nil {
		return err
	}
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if err := knowledge_compile.PublishDeleted(publishCtx, tenantID, doc.KbID, doc.ID, taskTypes); err != nil {
		return fmt.Errorf("publish document cleanup for %s: %w", doc.ID, err)
	}
	return nil
}

func (s *DocumentService) clearDocumentAndKBCountersForRerun(docID, kbID string) error {
	return dao.DB.Transaction(func(tx *gorm.DB) error {
		var current entity.Document
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND kb_id = ?", docID, kbID).
			First(&current).Error; err != nil {
			return err
		}

		if current.TokenNum == 0 && current.ChunkNum == 0 && current.ProcessDuration == 0 {
			return nil
		}

		result := tx.Model(&entity.Document{}).
			Where("id = ? AND kb_id = ?", docID, kbID).
			Updates(map[string]interface{}{
				"token_num":        0,
				"chunk_num":        0,
				"process_duration": 0,
			})
		if result.Error != nil {
			return result.Error
		}
		if current.TokenNum == 0 && current.ChunkNum == 0 {
			return nil
		}

		result = tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", kbID).
			Updates(map[string]interface{}{
				"token_num": gorm.Expr("token_num - ?", current.TokenNum),
				"chunk_num": gorm.Expr("chunk_num - ?", current.ChunkNum),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return fmt.Errorf("knowledgebase not found")
		}
		return nil
	})
}

func (s *DocumentService) clearKBChunkNumWhenRerun(doc *entity.Document) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}
	return dao.GetDB().Model(&entity.Knowledgebase{}).Where("id = ?", doc.KbID).Updates(map[string]interface{}{
		"token_num": gorm.Expr("token_num - ?", doc.TokenNum),
		"chunk_num": gorm.Expr("chunk_num - ?", doc.ChunkNum),
	}).Error
}

func (s *DocumentService) ParseDocuments(ctx context.Context, datasetID, userID string, docIDs []string) ([]*service.ParseDocumentResponse, error) {
	// deduplicate the document id
	uniqueDocIDs := common.Deduplicate(docIDs)
	if uniqueDocIDs == nil || len(uniqueDocIDs) == 0 {
		return nil, fmt.Errorf("no documents to parse")
	}

	var responses []*service.ParseDocumentResponse

	// query database, if the document ids are valid
	for _, docID := range uniqueDocIDs {
		doc, err := s.documentDAO.GetByID(ctx, dao.DB, docID)
		if err != nil {
			errorMessage := err.Error()
			responses = append(responses, &service.ParseDocumentResponse{
				DocumentID: docID,
				Result:     errorMessage,
			})
			continue
		}
		if doc == nil {
			errorMessage := "no such document"
			responses = append(responses, &service.ParseDocumentResponse{
				DocumentID: docID,
				Result:     errorMessage,
			})
			continue
		}

		if doc.Status != nil && *doc.Status != "0" {
			errorMessage := fmt.Sprintf("document %s is already parsed", docID)
			responses = append(responses, &service.ParseDocumentResponse{
				DocumentID: docID,
				Result:     errorMessage,
			})
			continue
		}

	}

	common.Info(fmt.Sprintf("parse documents, dataset: %s, documents: %v", datasetID, docIDs))
	return responses, nil
}

// StopParseDocuments stops parsing for the given documents in a dataset.
// It requests stop for the associated ingestion tasks.
// Returns a map with success_count and optionally errors.
func (s *DocumentService) StopParseDocuments(ctx context.Context, datasetID string, docIDs []string) (map[string]interface{}, error) {
	deduped := common.Deduplicate(docIDs)
	if len(deduped) == 0 {
		return nil, fmt.Errorf("no document IDs provided")
	}

	docs, err := s.validateDocsInDataset(ctx, deduped, datasetID)
	if err != nil {
		// Mirror the Python parse/stop endpoint's "Documents not found" message.
		var notInDataset *documentsNotInDatasetError
		if errors.As(err, &notInDataset) {
			quoted := make([]string, len(notInDataset.ids))
			for i, id := range notInDataset.ids {
				quoted[i] = "'" + id + "'"
			}
			return nil, fmt.Errorf("Documents not found: [%s]", strings.Join(quoted, ", "))
		}
		return nil, err
	}

	var errs []string
	successCount := 0
	for _, doc := range docs {
		if cancelErr := s.CancelDocParse(ctx, doc); cancelErr != nil {
			if errors.Is(cancelErr, errParseNotRunning) {
				// Mirror the Python /documents/stop endpoint's message.
				errs = append(errs, "Can't stop parsing document that has not started or already completed")
			} else {
				errs = append(errs, cancelErr.Error())
			}
			continue
		}
		successCount++
	}

	result := map[string]interface{}{"success_count": successCount}
	if len(errs) > 0 {
		result["errors"] = errs
	}
	return result, nil
}

// documentsNotInDatasetError carries the ids that are missing from (or do not
// belong to) a dataset so each endpoint can format its own contract message.
type documentsNotInDatasetError struct {
	datasetID string
	ids       []string
}

// Error mirrors the Python delete endpoint's message.
func (e *documentsNotInDatasetError) Error() string {
	return fmt.Sprintf("These documents do not belong to dataset %s or Document not found: %s", e.datasetID, strings.Join(e.ids, ", "))
}

// validateDocsInDataset deduplicates IDs, fetches the documents, and ensures
// every document exists and belongs to the given dataset. Returns the resolved
// documents.
func (s *DocumentService) validateDocsInDataset(ctx context.Context, docIDs []string, datasetID string) ([]*entity.Document, error) {
	docs, err := s.documentDAO.GetByIDs(ctx, dao.DB, docIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch documents: %w", err)
	}
	invalid := make([]string, 0)
	if len(docs) != len(docIDs) {
		found := make(map[string]struct{}, len(docs))
		for _, d := range docs {
			found[d.ID] = struct{}{}
		}
		for _, id := range docIDs {
			if _, ok := found[id]; !ok {
				invalid = append(invalid, id)
			}
		}
	} else {
		for _, d := range docs {
			if d.KbID != datasetID {
				invalid = append(invalid, d.ID)
			}
		}
	}
	if len(invalid) > 0 {
		return nil, &documentsNotInDatasetError{datasetID: datasetID, ids: invalid}
	}
	return docs, nil
}

// errParseNotRunning is returned by CancelDocParse when the document has no
// in-flight ingestion task and is not already stopped. Callers map it to their
// endpoint-specific message (the Python /documents/ingest and /documents/stop messages differ).
var errParseNotRunning = errors.New("parse task is not in running status")

// CancelDocParse stops the ingestion task for the document by calling
// RequestStop (STOPPING).
// It returns errParseNotRunning if the document has neither an in-flight
// ingestion task (CREATED/SCHEDULED/RUNNING/STOPPING) nor an already stopped
// task (STOPPED).
func (s *DocumentService) CancelDocParse(ctx context.Context, doc *entity.Document) error {
	task, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, doc.ID)
	if err != nil {
		return fmt.Errorf("failed to get ingestion task for %s: %w", doc.ID, err)
	}

	inFlight := task != nil && common.IsActiveTaskStatus(task.Status)
	isStopped := task != nil && task.Status == common.STOPPED
	if !inFlight && !isStopped {
		return errParseNotRunning
	}

	if inFlight {
		if _, err = s.ingestionTaskSvc.RequestStop(ctx, task.ID); err != nil {
			return fmt.Errorf("failed to stop ingestion task %s: %w", task.ID, err)
		}
	}

	return nil
}

func (s *DocumentService) resetDocumentForReparse(ctx context.Context, doc *entity.Document, tenantID string, parserID *string, pipelineID *string) error {
	updates := map[string]interface{}{
		"progress": 0,
	}
	if parserID != nil {
		updates["parser_id"] = *parserID
	}
	if pipelineID != nil {
		updates["pipeline_id"] = *pipelineID
	}

	if err := s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, updates); err != nil {
		return errors.New("document not found")
	}

	if doc.TokenNum > 0 {
		decremented, err := s.decrementDocumentAndKBCountersForReparse(doc)
		if err != nil {
			return errors.New("document not found")
		}
		if !decremented {
			return nil
		}
		if s.docEngine != nil {
			indexName := fmt.Sprintf("ragflow_%s", tenantID)
			s.deleteChunkImages(ctx, doc, indexName)
			if err = s.deleteSourceChunks(ctx, tenantID, doc.KbID, doc.ID); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *DocumentService) deleteChunkImages(ctx context.Context, doc *entity.Document, indexName string) {
	if doc == nil {
		return
	}
	_ = s.deleteDocumentChunkImages(ctx, indexName, doc.KbID, doc.ID)
}

// deleteDocumentChunkImages removes source chunk image objects in bounded,
// claim-fenced search batches. Images are external storage state, so a
// takeover can fence the cleanup only at batch boundaries; repeating the
// operation is safe because deleting an absent object is a no-op.
func (s *DocumentService) deleteDocumentChunkImages(ctx context.Context, indexName, datasetID, documentID string) error {
	if s.docEngine == nil {
		return nil
	}
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil
	}

	const pageSize = 1000
	claimToken := s.cleanupClaimToken(ctx, documentID)
	for offset := 0; ; offset += pageSize {
		if err := s.beginCleanupBatch(ctx, documentID, claimToken); err != nil {
			return err
		}
		batchCtx, cancel := context.WithTimeout(ctx, cleanupBatchTimeout)
		result, err := s.docEngine.Search(batchCtx, &enginetypes.SearchRequest{
			IndexNames:   []string{indexName},
			KbIDs:        []string{datasetID},
			Offset:       offset,
			Limit:        pageSize,
			SelectFields: []string{"id", "img_id", "compile_kwd"},
			Filter:       map[string]interface{}{"doc_id": documentID},
		})
		if err != nil {
			cancel()
			return err
		}
		if result == nil || len(result.Chunks) == 0 {
			cancel()
			if err := s.finishCleanupBatch(ctx, documentID, claimToken); err != nil {
				return err
			}
			return nil
		}
		for _, chunk := range result.Chunks {
			if strings.TrimSpace(documentStoreString(chunk["compile_kwd"])) != "" {
				continue
			}
			imageKey, ok := chunkImageStorageKey(datasetID, chunk)
			if !ok || !storageImpl.ObjExist(batchCtx, datasetID, imageKey) {
				continue
			}
			if err := storageImpl.Remove(batchCtx, datasetID, imageKey); err != nil {
				cancel()
				return err
			}
		}
		cancel()
		if err := s.finishCleanupBatch(ctx, documentID, claimToken); err != nil {
			return err
		}
		if int64(offset+len(result.Chunks)) >= result.Total {
			return nil
		}
	}
}

func chunkImageStorageKey(defaultBucket string, chunk map[string]interface{}) (string, bool) {
	imgID := firstStringField(chunk, "img_id")
	if imgID != "" {
		prefix := defaultBucket + "-"
		if strings.HasPrefix(imgID, prefix) && len(imgID) > len(prefix) {
			return strings.TrimPrefix(imgID, prefix), true
		}
		return imgID, true
	}

	chunkID := firstStringField(chunk, "id", "_id")
	if chunkID == "" {
		return "", false
	}
	return chunkID, true
}

func firstStringField(m map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := m[key]; ok {
			if s, ok := value.(string); ok {
				return s
			}
		}
	}
	return ""
}

func (s *DocumentService) decrementDocumentAndKBCountersForReparse(doc *entity.Document) (bool, error) {
	decremented := false
	err := dao.DB.Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&entity.Document{}).
			Where("id = ? AND kb_id = ? AND token_num = ? AND chunk_num = ?", doc.ID, doc.KbID, doc.TokenNum, doc.ChunkNum).
			Updates(map[string]interface{}{
				"token_num":        gorm.Expr("token_num - ?", doc.TokenNum),
				"chunk_num":        gorm.Expr("chunk_num - ?", doc.ChunkNum),
				"process_duration": gorm.Expr("process_duration - ?", doc.ProcessDuration),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return nil
		}
		decremented = true

		return tx.Model(&entity.Knowledgebase{}).
			Where("id = ?", doc.KbID).
			Updates(map[string]interface{}{
				"token_num": gorm.Expr("token_num - ?", doc.TokenNum),
				"chunk_num": gorm.Expr("chunk_num - ?", doc.ChunkNum),
			}).Error
	})
	return decremented, err
}

func (s *DocumentService) updateDocumentStatusOnly(ctx context.Context, doc *entity.Document, kb *entity.Knowledgebase, status int) error {
	statusStr := strconv.Itoa(status)
	if doc.Status != nil && *doc.Status == statusStr {
		return nil
	}

	if err := s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{"status": statusStr}); err != nil {
		return errors.New("database error (Document update)")
	}

	if s.docEngine != nil {
		if err := s.updateSourceChunkAvailability(ctx, kb.TenantID, doc.KbID, doc.ID, status); err != nil {
			return err
		}
	}
	s.markDocumentWikiDirty(ctx, kb.TenantID, doc.KbID, doc.ID)
	s.publishKnowledgeCompileStatusChange(ctx, kb.TenantID, doc.KbID, doc.ID, status)
	return nil
}
