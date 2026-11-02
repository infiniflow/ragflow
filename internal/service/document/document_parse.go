package document

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/service"
	"sort"
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
	cleanupBatchTimeout                    = 30 * time.Second
	taskTerminalMessageSuperseded          = "Task superseded by a new parse request."
	taskTerminalMessageDocumentDeleted     = "Task cancelled by document deletion."
	taskTerminalMessageDocumentRemoved     = "Task cancelled by document removal."
	taskTerminalMessageParseResultsDeleted = "Task cancelled by parse result deletion."
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

// cleanupDocumentTask settles and conditionally deletes the document's task
// before purging its run-scoped cache state. If the task starts running between
// settlement and deletion, the conditional delete fails and no cache state is
// removed for the new run.
func (s *DocumentService) cleanupDocumentTask(ctx context.Context, docID, terminalMessage string) error {
	task, err := s.ingestionTaskSvc.PrepareDocumentTaskForCleanup(ctx, docID, terminalMessage)
	if err != nil {
		return fmt.Errorf("prepare ingestion task for document %s cleanup: %w", docID, err)
	}
	if err := s.deletePreparedDocumentTask(ctx, docID, task); err != nil {
		return err
	}
	if task == nil {
		return nil
	}

	purgeTaskState := s.purgeTaskState
	if purgeTaskState == nil {
		purgeTaskState = ingestionpipeline.PurgeTaskState
	}
	// The task row is gone, so a request cancellation must not skip cleanup that
	// is now retryable only through the 24-hour TTL.
	batchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupBatchTimeout)
	defer cancel()
	if err := purgeTaskState(batchCtx, task.ID); err != nil {
		return fmt.Errorf("purge task state for document %s task %s: %w", docID, task.ID, err)
	}
	return nil
}

func (s *DocumentService) deletePreparedDocumentTask(ctx context.Context, docID string, task *entity.IngestionTask) error {
	if task == nil {
		return nil
	}
	deleted, err := s.ingestionTaskDAO.DeleteIfTerminal(ctx, dao.DB, task.ID)
	if err != nil {
		return fmt.Errorf("delete ingestion task %s for document %s: %w", task.ID, docID, err)
	}
	if deleted != 1 {
		return fmt.Errorf("document %s ingestion task %s started running; stop it before cleanup", docID, task.ID)
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

	if opts.ApplyKB {
		if err := s.applyKnowledgebaseParserConfig(ctx, doc, kb); err != nil {
			return err
		}
	}

	if opts.RerunWithDelete {
		if err := s.clearDocumentParseResults(ctx, doc, kb.TenantID, taskTerminalMessageSuperseded); err != nil {
			return err
		}
	}

	responses, err := s.IngestDocuments(ctx, doc.KbID, userID, []string{doc.ID})
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

// applyKnowledgebaseParserConfig merges the dataset-scoped parser_config into
// the document's existing component-scoped config and persists the merged
// result before parsing when ApplyKB is requested.
//
// The dataset may contribute a top-level llm_id (used only to fill empty
// Extractor / Compiler node llm_ids) and a top-level modular metadata object
// (parser_config["metadata"] = {enabled, metadata, built_in_metadata}). Those
// are scoped onto the document's existing Extractor nodes via
// service.ApplyComponentScopedParserConfig, which also strips legacy flat
// metadata fields. No top-level llm_id is written, and when the dataset sets
// no metadata no top-level metadata key is left behind; the document's own
// component nodes and unrelated keys are preserved. Nested maps/slices are
// deep-copied so a failed persist cannot mutate the caller's document or the
// source dataset. On a persistence failure the document's in-memory config is
// left untouched.
func (s *DocumentService) applyKnowledgebaseParserConfig(ctx context.Context, doc *entity.Document, kb *entity.Knowledgebase) error {
	if doc == nil || kb == nil {
		return fmt.Errorf("apply dataset parser config: document and dataset are required")
	}

	merged := deepCopyJSONMap(doc.ParserConfig)
	if merged == nil {
		merged = entity.JSONMap{}
	}

	// The dataset's llm_id is used only as the helper's fill value for empty
	// Extractor/Compiler node llm_ids. It is resolved from the same persisted
	// source the dataset writer uses (explicit top-level value, else the
	// tenant-resolved value already scoped into the dataset's Extractor nodes).
	// No top-level llm_id flat key is written (the pipeline ignores it), and
	// any legacy top-level llm_id the document already carries is dropped
	// rather than re-written into the result.
	llmID := resolveDatasetLLMID(kb.ParserConfig)
	delete(merged, "llm_id")

	// Carry the dataset's modular metadata object when present. Resolution
	// follows the dataset reader contract: the top-level modular object
	// first, else the first (sorted) Extractor node carrying modular metadata
	// — the stored form after component scoping. When absent everywhere the
	// dataset expresses "no metadata config": reset each existing Extractor
	// node's metadata to the disabled default directly, mirroring the Python
	// apply_kb defaults, without creating a top-level key the helper would
	// otherwise have to clean up.
	meta, metaOK, fromTopLevel := resolveDatasetModularMetadata(kb.ParserConfig)
	if metaOK {
		merged["metadata"] = deepCopyValue(meta)
	} else {
		for _, cpnID := range sortedExtractorKeys(merged) {
			params, ok := nodeParams(merged[cpnID])
			if !ok {
				continue
			}
			params["metadata"] = map[string]any{
				"enabled":           false,
				"metadata":          []any{},
				"built_in_metadata": []any{},
			}
		}
	}

	merged = service.ApplyComponentScopedParserConfig(merged, llmID)
	if !fromTopLevel {
		// The dataset itself carries no top-level modular object: never leave
		// the transport key behind, mirroring the stored dataset shape.
		delete(merged, "metadata")
	}

	if err := s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{
		"parser_config": entity.JSONMap(merged),
	}); err != nil {
		return fmt.Errorf("apply dataset parser config for document %s: %w", doc.ID, err)
	}
	doc.ParserConfig = merged
	return nil
}

// sortedExtractorKeys returns the component IDs in parserConfig that name
// Extractor nodes, in sorted order so multi-Extractor configs resolve
// deterministically instead of depending on Go map iteration order.
func sortedExtractorKeys(parserConfig entity.JSONMap) []string {
	var keys []string
	for k := range parserConfig {
		lower := strings.ToLower(k)
		if strings.HasPrefix(lower, "extractor:") || strings.HasPrefix(lower, "extractor_") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// nodeParams returns a writable view of a component node's params, accepting
// both the decoded (map[string]any) and entity (entity.JSONMap) shapes.
// The returned map shares storage with the input, so writes are visible.
func nodeParams(raw any) (map[string]any, bool) {
	switch params := raw.(type) {
	case map[string]any:
		return params, true
	case entity.JSONMap:
		return map[string]any(params), true
	default:
		return nil, false
	}
}

// nodeModularMetadata returns the node's modular metadata object
// ({enabled, metadata, built_in_metadata}) in either stored map shape.
func nodeModularMetadata(params map[string]any) (map[string]any, bool) {
	if meta, ok := params["metadata"].(map[string]any); ok {
		return meta, true
	}
	if meta, ok := params["metadata"].(entity.JSONMap); ok {
		return map[string]any(meta), true
	}
	return nil, false
}

// resolveDatasetModularMetadata returns the dataset's effective modular
// metadata config, following the dataset reader contract: the top-level
// modular object first, else the first (sorted) Extractor node carrying
// modular metadata — the stored form after component scoping. It reports
// whether any was found and whether it came from the top level. The returned
// map is a live view of the dataset config; callers must copy before mutating.
func resolveDatasetModularMetadata(parserConfig entity.JSONMap) (map[string]any, bool, bool) {
	if mm, ok := parserConfig["metadata"].(map[string]any); ok {
		return mm, true, true
	}
	if mm, ok := parserConfig["metadata"].(entity.JSONMap); ok {
		return map[string]any(mm), true, true
	}
	for _, cpnID := range sortedExtractorKeys(parserConfig) {
		params, ok := nodeParams(parserConfig[cpnID])
		if !ok {
			continue
		}
		if meta, ok := nodeModularMetadata(params); ok {
			return meta, true, false
		}
	}
	return nil, false, false
}

// resolveDatasetLLMID returns the dataset's effective LLM id: an explicit
// non-empty top-level llm_id first, else the first (sorted) Extractor node's
// non-empty llm_id (populated from the tenant default at dataset write time).
// Empty means the dataset carries no llm_id and empty node llm_ids stay empty.
func resolveDatasetLLMID(parserConfig entity.JSONMap) string {
	if s, ok := parserConfig["llm_id"].(string); ok && strings.TrimSpace(s) != "" {
		return s
	}
	for _, cpnID := range sortedExtractorKeys(parserConfig) {
		params, ok := nodeParams(parserConfig[cpnID])
		if !ok {
			continue
		}
		if s, ok := params["llm_id"].(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return ""
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
		if task != nil && common.IsRunningOrStopping(task.Status) {
			return fmt.Errorf("document %s ingestion task %s is %s; stop it and wait for a terminal state before re-parsing", docID, task.ID, task.Status)
		}
	}
	return nil
}

func (s *DocumentService) clearDocumentParseResults(ctx context.Context, doc *entity.Document, tenantID, terminalMessage string) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}

	// Settle and conditionally delete the task before touching cache state or
	// durable results. A task that starts running after preparation blocks the
	// delete and leaves both the document and its results intact.
	if err := s.cleanupDocumentTask(ctx, doc.ID, terminalMessage); err != nil {
		return err
	}
	if err := s.clearDocumentAndKBCountersForRerun(doc.ID, doc.KbID); err != nil {
		return err
	}

	if s.docEngine == nil {
		return nil
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	exists, err := s.docEngine.ChunkStoreExists(ctx, indexName, doc.KbID)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	variants, taskTypes, err := s.documentKnowledgeCompileTypes(ctx, tenantID, doc.KbID, doc.ID)
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
	if len(variants) == 0 {
		common.Warn(fmt.Sprintf("skip document cleanup event for %s: existing knowledge products have no routing metadata", doc.ID))
		return nil
	}
	publishCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if err := knowledge_compile.PublishDeleted(publishCtx, tenantID, doc.KbID, doc.ID, variants, taskTypes); err != nil {
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
			if errors.Is(cancelErr, ErrParseNotRunning) {
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

// ErrParseNotRunning is returned by CancelDocParse when the document has no
// in-flight ingestion task and is not already stopped. Callers map it to their
// endpoint-specific message (the Python /documents/ingest and /documents/stop messages differ).
var ErrParseNotRunning = errors.New("parse task is not in running status")

// CancelDocParse stops the document's active ingestion task. It returns
// ErrParseNotRunning when the task is absent or already completed/failed.
func (s *DocumentService) CancelDocParse(ctx context.Context, doc *entity.Document) error {
	task, err := s.ingestionTaskDAO.GetByDocumentID(ctx, dao.DB, doc.ID)
	if err != nil {
		return fmt.Errorf("failed to get ingestion task for %s: %w", doc.ID, err)
	}
	if task == nil {
		return ErrParseNotRunning
	}
	switch {
	case common.IsActiveTaskStatus(task.Status):
		if _, err := s.ingestionTaskSvc.RequestStop(ctx, task.ID); err != nil {
			return fmt.Errorf("failed to stop ingestion task %s: %w", task.ID, err)
		}
		return nil
	case task.Status == common.STOPPED:
		return nil
	case task.Status == common.COMPLETED || task.Status == common.FAILED:
		return ErrParseNotRunning
	default:
		return fmt.Errorf("ingestion task %s has unrecognized status %q", task.ID, task.Status)
	}
}

// deleteDocumentChunkImages removes source chunk image objects in bounded
// search batches. Repeating the operation is safe because deleting an absent
// object is a no-op.
func (s *DocumentService) deleteDocumentChunkImages(ctx context.Context, indexName, datasetID, documentID string) error {
	if s.docEngine == nil {
		return nil
	}
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return nil
	}

	const pageSize = 1000
	for offset := 0; ; offset += pageSize {
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

func (s *DocumentService) updateDocumentStatusOnly(ctx context.Context, doc *entity.Document, kb *entity.Knowledgebase, status int) error {
	statusStr := strconv.Itoa(status)
	if doc.Status != nil && *doc.Status == statusStr {
		return nil
	}

	if err := s.documentDAO.UpdateByID(ctx, dao.DB, doc.ID, map[string]interface{}{"status": statusStr}); err != nil {
		return errors.New("database error (Document update)")
	}

	if s.docEngine != nil {
		if err := s.updateDocumentChunkAvailability(ctx, kb.TenantID, doc.KbID, doc.ID, status); err != nil {
			return err
		}
	}
	// A status transition changes document availability, not its parsed
	// content. The status event handles dataset-level retraction/re-enable;
	// scheduling a delayed Wiki recompilation here creates duplicate events
	// and a visible 20-second oscillation.
	s.publishKnowledgeCompileStatusChange(ctx, kb.TenantID, doc.KbID, doc.ID, status)
	return nil
}
