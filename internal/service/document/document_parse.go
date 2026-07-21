package document

import (
	"context"
	"errors"
	"fmt"
	"ragflow/internal/service"
	"strconv"
	"strings"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	enginetypes "ragflow/internal/engine/types"
	"ragflow/internal/entity"
	"ragflow/internal/storage"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// StartParseDocuments starts parsing a document via the DSL ingestion
// pipeline. It optionally clears prior results (RerunWithDelete), applies
// KB config (ApplyKB), validates storage, and enqueues an ingestion task.
// The document run status is NOT set here; service.IngestionTaskService.StartRunning
// sets it to RUNNING when the worker picks up the task and transitions it from
// CREATED. Extracted from Ingest so other entry points (e.g. ChunkService.Parse)
// can reuse the same start-parse flow.
func (s *DocumentService) StartParseDocuments(doc *entity.Document, kb *entity.Knowledgebase, userID string, opts StartParseOptions) error {
	// Validate storage first so we don't clear prior results and then fail
	// because the document can't be read, leaving the document with neither
	// old nor new parse results.
	if _, _, err := s.GetDocumentStorageAddress(doc); err != nil {
		return err
	}

	if opts.RerunWithDelete {
		if err := s.clearDocumentParseResults(doc, kb.TenantID); err != nil {
			return err
		}
	}

	if _, err := s.IngestDocuments(doc.KbID, userID, []string{doc.ID}); err != nil {
		return err
	}
	return nil
}

// AssertIngestionTasksTerminal verifies none of the documents has an
// in-flight (RUNNING/STOPPING) ingestion task. Used as a batch pre-check
// before re-parsing so a single non-terminal doc rejects the whole request
// up front instead of partially cleaning some docs then failing.
func (s *DocumentService) AssertIngestionTasksTerminal(docIDs []string) error {
	for _, docID := range docIDs {
		task, err := s.ingestionTaskDAO.GetByDocumentID(docID)
		if err != nil {
			return fmt.Errorf("check ingestion task for %s: %w", docID, err)
		}
		if task == nil {
			continue
		}
		if task.Status == common.RUNNING || task.Status == common.STOPPING {
			return fmt.Errorf("document %s ingestion task is %s; stop it and wait for a terminal state before re-parsing", docID, task.Status)
		}
	}
	return nil
}

func (s *DocumentService) clearDocumentParseResults(doc *entity.Document, tenantID string) error {
	if doc == nil {
		return fmt.Errorf("document is nil")
	}

	// Refuse to clear a non-terminal ingestion task. An in-flight worker
	// (RUNNING) or one mid-stop (STOPPING) would keep writing chunks and
	// corrupt the new run's results. The caller must stop the task first
	// and wait for a terminal state (COMPLETED/STOPPED/FAILED) or CREATED.
	if task, _ := s.ingestionTaskDAO.GetByDocumentID(doc.ID); task != nil {
		if task.Status == common.RUNNING || task.Status == common.STOPPING {
			return fmt.Errorf("document %s ingestion task is %s; stop it and wait for a terminal state before re-parsing", doc.ID, task.Status)
		}
	}

	// Delete terminal and CREATED ingestion tasks atomically, leaving
	// RUNNING/STOPPING tasks untouched so the check-then-delete window
	// between GetByDocumentID and the delete above cannot delete a task
	// that just transitioned to RUNNING.
	if _, err := s.ingestionTaskDAO.DeleteIfTerminal(doc.ID); err != nil {
		return err
	}

	if err := s.clearDocumentAndKBCountersForRerun(doc.ID, doc.KbID); err != nil {
		return err
	}

	if s.docEngine == nil {
		return nil
	}

	indexName := fmt.Sprintf("ragflow_%s", tenantID)
	exists, err := s.docEngine.ChunkStoreExists(context.Background(), indexName, doc.KbID)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	if _, err := s.docEngine.DeleteChunks(context.Background(), map[string]interface{}{"doc_id": doc.ID}, indexName, doc.KbID); err != nil {
		return err
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

func (s *DocumentService) countDoneDocuments(datasetID string) (int64, error) {
	var count int64
	err := dao.GetDB().Model(&entity.Document{}).
		Where("kb_id = ? AND run = ?", datasetID, string(entity.TaskStatusDone)).
		Count(&count).Error
	return count, err
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

func (s *DocumentService) ParseDocuments(datasetID, userID string, docIDs []string) ([]*service.ParseDocumentResponse, error) {
	// create document parse id
	// save to task table
	// send to message queue

	// deduplicate the document id
	uniqueDocIDs := common.Deduplicate(docIDs)
	if uniqueDocIDs == nil || len(uniqueDocIDs) == 0 {
		return nil, fmt.Errorf("no documents to parse")
	}

	var responses []*service.ParseDocumentResponse

	// query database, if the document ids are valid
	for _, docID := range uniqueDocIDs {
		doc, err := s.documentDAO.GetByID(docID)
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

		// create task for each document
		//task := &entity.IngestionTask{
		//	ID:         utility.GenerateToken(),
		//	DocumentID: docID,
		//	UserID:     userID,
		//}

		// save the task to database
		//err = s.ingestionTaskDAO.Create(task)
		//if err != nil {
		//	errorMessage := err.Error()
		//	responses = append(responses, &service.ParseDocumentResponse{
		//		DocumentID: docID,
		//		Result:     &errorMessage,
		//	})
		//	continue
		//}

		// Send task to message queue

	}

	common.Info(fmt.Sprintf("parse documents, dataset: %s, documents: %v", datasetID, docIDs))
	return responses, nil
}

// StopParseDocuments stops parsing for the given documents in a dataset.
// It sets Redis cancel signals for associated tasks and updates doc.run to CANCEL.
// Returns a map with success_count and optionally errors.
func (s *DocumentService) StopParseDocuments(datasetID string, docIDs []string) (map[string]interface{}, error) {
	deduped := common.Deduplicate(docIDs)
	if len(deduped) == 0 {
		return nil, fmt.Errorf("no document IDs provided")
	}

	docs, err := s.validateDocsInDataset(deduped, datasetID)
	if err != nil {
		return nil, err
	}

	var errors []string
	successCount := 0
	for _, doc := range docs {
		if cancelErr := s.CancelDocParse(doc); cancelErr != nil {
			errors = append(errors, cancelErr.Error())
			continue
		}
		successCount++
	}

	result := map[string]interface{}{"success_count": successCount}
	if len(errors) > 0 {
		result["errors"] = errors
	}
	return result, nil
}

// validateDocsInDataset deduplicates IDs, fetches the documents, and ensures
// every document exists and belongs to the given dataset. Returns the resolved
// documents.
func (s *DocumentService) validateDocsInDataset(docIDs []string, datasetID string) ([]*entity.Document, error) {
	docs, err := s.documentDAO.GetByIDs(docIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch documents: %w", err)
	}
	if len(docs) != len(docIDs) {
		return nil, fmt.Errorf("some document IDs not found in dataset %s", datasetID)
	}
	var invalid []string
	for _, d := range docs {
		if d.KbID != datasetID {
			invalid = append(invalid, d.ID)
		}
	}
	if len(invalid) > 0 {
		return nil, fmt.Errorf("These documents do not belong to dataset %s: %v", datasetID, invalid)
	}
	return docs, nil
}

// CancelDocParse stops the ingestion task for the document by calling
// RequestStop (STOPPING), then marks the document run status as CANCEL.
func (s *DocumentService) CancelDocParse(doc *entity.Document) error {
	task, err := s.ingestionTaskDAO.GetByDocumentID(doc.ID)
	if err != nil {
		return fmt.Errorf("failed to get ingestion task for %s: %v", doc.ID, err)
	}
	if task == nil {
		return fmt.Errorf("no ingestion task found for document %s", doc.ID)
	}

	if _, err := s.ingestionTaskSvc.RequestStop(task.ID); err != nil {
		return fmt.Errorf("failed to stop ingestion task %s: %v", task.ID, err)
	}

	if upErr := s.documentDAO.UpdateByID(doc.ID, map[string]interface{}{"run": string(entity.TaskStatusCancel)}); upErr != nil {
		return fmt.Errorf("failed to update document %s: %v", doc.ID, upErr)
	}
	return nil
}

func (s *DocumentService) resetDocumentForReparse(doc *entity.Document, tenantID string, parserID *string, pipelineID *string) error {
	progressMsg := ""
	run := string(entity.TaskStatusUnstart)
	updates := map[string]interface{}{
		"progress":     0,
		"progress_msg": progressMsg,
		"run":          run,
	}
	if parserID != nil {
		updates["parser_id"] = *parserID
	}
	if pipelineID != nil {
		updates["pipeline_id"] = *pipelineID
	}

	if err := s.documentDAO.UpdateByID(doc.ID, updates); err != nil {
		return errors.New("Document not found!")
	}

	if doc.TokenNum > 0 {
		decremented, err := s.decrementDocumentAndKBCountersForReparse(doc)
		if err != nil {
			return errors.New("Document not found!")
		}
		if !decremented {
			return nil
		}
		if s.docEngine != nil {
			indexName := fmt.Sprintf("ragflow_%s", tenantID)
			s.deleteChunkImages(doc, indexName)
			if _, err := s.docEngine.DeleteChunks(context.Background(), map[string]interface{}{"doc_id": doc.ID}, indexName, doc.KbID); err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *DocumentService) deleteChunkImages(doc *entity.Document, indexName string) {
	if s.docEngine == nil {
		return
	}
	storageImpl := storage.GetStorageFactory().GetStorage()
	if storageImpl == nil {
		return
	}

	const pageSize = 1000
	for offset := 0; ; offset += pageSize {
		result, err := s.docEngine.Search(context.Background(), &enginetypes.SearchRequest{
			IndexNames:   []string{indexName},
			KbIDs:        []string{doc.KbID},
			Offset:       offset,
			Limit:        pageSize,
			SelectFields: []string{"id", "img_id"},
			Filter:       map[string]interface{}{"doc_id": doc.ID},
			MatchExprs:   nil,
			OrderBy:      nil,
			RankFeature:  nil,
		})
		if err != nil || result == nil || len(result.Chunks) == 0 {
			return
		}
		for _, chunk := range result.Chunks {
			imageKey, ok := chunkImageStorageKey(doc.KbID, chunk)
			if !ok {
				continue
			}
			if storageImpl.ObjExist(doc.KbID, imageKey) {
				_ = storageImpl.Remove(doc.KbID, imageKey)
			}
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

func (s *DocumentService) updateDocumentStatusOnly(doc *entity.Document, kb *entity.Knowledgebase, status int) error {
	statusStr := strconv.Itoa(status)
	if doc.Status != nil && *doc.Status == statusStr {
		return nil
	}

	if err := s.documentDAO.UpdateByID(doc.ID, map[string]interface{}{"status": statusStr}); err != nil {
		return errors.New("Database error (Document update)!")
	}

	if s.docEngine == nil {
		return nil
	}

	indexName := fmt.Sprintf("ragflow_%s", kb.TenantID)
	return s.docEngine.UpdateChunks(
		context.Background(),
		map[string]interface{}{"doc_id": doc.ID},
		map[string]interface{}{"available_int": status},
		indexName,
		doc.KbID,
	)
}
