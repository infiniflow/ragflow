package document

import (
	"errors"
	"strings"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

func insertTestPipelineLog(t *testing.T, id, docID, kbID, tenantID string, dsl entity.JSONMap) {
	t.Helper()
	opLog := &entity.PipelineOperationLog{
		ID:              id,
		DocumentID:      docID,
		TenantID:        tenantID,
		KbID:            kbID,
		ParserID:        "pipeline",
		DocumentName:    "hlm.docx",
		DocumentSuffix:  ".docx",
		DocumentType:    "docx",
		SourceFrom:      "local",
		TaskType:        "parse",
		OperationStatus: "done",
		DSL:             dsl,
	}
	if err := dao.DB.Create(opLog).Error; err != nil {
		t.Fatalf("insert test pipeline log: %v", err)
	}
}

func rerunTestService(t *testing.T) (*DocumentService, *recordingTaskPublisher) {
	t.Helper()
	svc := testDocumentService(t)
	publisher := &recordingTaskPublisher{}
	svc.ingestionTaskSvc.SetTaskPublisher(publisher)
	return svc, publisher
}

func TestRerunDocument_UnknownLogID(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	svc, _ := rerunTestService(t)
	err := svc.RerunDocument(t.Context(), "tenant-1", "missing-log", entity.JSONMap{"components": struct{}{}}, "c1")
	if err != ErrRerunDocumentNotFound {
		t.Fatalf("err = %v, want ErrRerunDocumentNotFound", err)
	}
}

func TestRerunDocument_InaccessibleDocument(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 9, 4)
	insertTestPipelineLog(t, "log-1", "doc-1", "kb-1", "tenant-1", nil)

	// No user_tenant row joins "stranger" to tenant-1 and the kb is owned
	// by tenant-1, so Accessible must deny.
	svc, _ := rerunTestService(t)
	err := svc.RerunDocument(t.Context(), "stranger", "log-1", entity.JSONMap{"components": struct{}{}}, "c1")
	if err != ErrRerunDocumentNotFound {
		t.Fatalf("err = %v, want ErrRerunDocumentNotFound", err)
	}
}

func TestRerunDocument_DocumentProcessing(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 0, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 9, 4)
	if err := dao.DB.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("progress", 0.5).Error; err != nil {
		t.Fatalf("set progress: %v", err)
	}
	insertTestPipelineLog(t, "log-1", "doc-1", "kb-1", "tenant-1", nil)

	svc, _ := rerunTestService(t)
	err := svc.RerunDocument(t.Context(), "tenant-1", "log-1", entity.JSONMap{"components": struct{}{}}, "c1")
	if err == nil || !strings.Contains(err.Error(), "is processing") {
		t.Fatalf("err = %v, want 'is processing' message", err)
	}
	// The handler classifies via errors.As, so pin the typed contract.
	var processingErr *RerunDocumentProcessingError
	if !errors.As(err, &processingErr) {
		t.Fatalf("err = %T, want RerunDocumentProcessingError", err)
	}
}

func TestRerunDocument_RerunsAndPersistsDSL(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 9, 4)
	insertTestDoc(t, "doc-1", "kb-1", 9, 4)
	// GetDocumentStorageAddress requires a non-empty document location.
	if err := dao.DB.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("location", "loc-1").Error; err != nil {
		t.Fatalf("set location: %v", err)
	}
	insertTestPipelineLog(t, "log-1", "doc-1", "kb-1", "tenant-1", entity.JSONMap{"components": map[string]interface{}{}})

	svc, publisher := rerunTestService(t)
	dsl := map[string]interface{}{
		"components": map[string]interface{}{"c1": map[string]interface{}{"obj": map[string]interface{}{}}},
	}
	if err := svc.RerunDocument(t.Context(), "tenant-1", "log-1", dsl, "c1"); err != nil {
		t.Fatalf("RerunDocument: %v", err)
	}

	// The caller's DSL map is not mutated in place.
	if _, ok := dsl["path"]; ok {
		t.Fatalf("caller dsl was mutated in place: %v", dsl)
	}

	// The edited DSL is persisted on the log with the rerun entry point.
	updated, err := svc.pipelineLogDAO.GetByID(t.Context(), db, "log-1")
	if err != nil {
		t.Fatalf("reload log: %v", err)
	}
	path, _ := updated.DSL["path"].([]interface{})
	if len(path) != 1 || path[0] != "c1" {
		t.Fatalf("log dsl path = %v, want [c1]", updated.DSL["path"])
	}
	if _, ok := updated.DSL["components"]; !ok {
		t.Fatalf("log dsl lost its components: %v", updated.DSL)
	}

	// A fresh ingestion task is enqueued for the document.
	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(publisher.messages))
	}
	task, err := svc.ingestionTaskDAO.GetByID(t.Context(), db, publisher.messages[0].TaskID)
	if err != nil {
		t.Fatalf("load ingestion task: %v", err)
	}
	// CreateAndEnqueue marks the task SCHEDULED once its queue message has
	// been published (the publisher here records instead of failing), so the
	// persisted row is already past CREATED by the time we reload it.
	if task.DocumentID != "doc-1" || task.DatasetID != "kb-1" || task.Status != common.SCHEDULED {
		t.Fatalf("ingestion task = %+v", task)
	}

	// Prior counters are cleared for the rerun.
	doc, err := svc.documentDAO.GetByID(t.Context(), db, "doc-1")
	if err != nil {
		t.Fatalf("reload doc: %v", err)
	}
	if doc.TokenNum != 0 || doc.ChunkNum != 0 {
		t.Fatalf("doc counters after rerun = token %d chunk %d, want zeros", doc.TokenNum, doc.ChunkNum)
	}
}

func TestRerunDocument_EmptyDSLPersistsEntryPath(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)

	insertTestKB(t, "kb-1", "tenant-1", 1, 9, 4)
	insertTestDoc(t, "doc-1", "kb-1", 9, 4)
	// GetDocumentStorageAddress requires a non-empty document location.
	if err := dao.DB.Model(&entity.Document{}).Where("id = ?", "doc-1").Update("location", "loc-1").Error; err != nil {
		t.Fatalf("set location: %v", err)
	}
	insertTestPipelineLog(t, "log-1", "doc-1", "kb-1", "tenant-1", entity.JSONMap{"components": map[string]interface{}{}})

	svc, publisher := rerunTestService(t)
	if err := svc.RerunDocument(t.Context(), "tenant-1", "log-1", entity.JSONMap{}, "c1"); err != nil {
		t.Fatalf("RerunDocument: %v", err)
	}

	// An empty but non-nil dsl is still persisted, with the rerun entry
	// point recorded on the log row.
	updated, err := svc.pipelineLogDAO.GetByID(t.Context(), db, "log-1")
	if err != nil {
		t.Fatalf("reload log: %v", err)
	}
	path, _ := updated.DSL["path"].([]interface{})
	if len(path) != 1 || path[0] != "c1" {
		t.Fatalf("log dsl path = %v, want [c1]", updated.DSL["path"])
	}

	// The rerun itself is still enqueued.
	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(publisher.messages))
	}
}
