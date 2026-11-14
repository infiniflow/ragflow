package service

import (
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

func TestRecordTerminalPipelineLogPreservesRunIdentity(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	status := string(entity.StatusValid)
	if err := db.Create(&entity.Knowledgebase{
		ID: "kb-1", TenantID: "tenant-1", Name: "kb-1", EmbdID: "embd-1",
		CreatedBy: "user-1", Permission: string(entity.TenantPermissionMe), Status: &status,
	}).Error; err != nil {
		t.Fatalf("seed knowledgebase: %v", err)
	}

	docName := "doc.txt"
	newPipelineID := "new-canvas"
	newVersionID := "new-ver"
	if err := db.Create(&entity.Document{
		ID: "doc-1", KbID: "kb-1", ParserID: "naive", ParserConfig: entity.JSONMap{},
		Name: &docName, Suffix: ".txt", Type: "txt", Status: &status,
		PipelineID: &newPipelineID, PipelineVersionID: &newVersionID,
	}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	runCount := 1
	oldPipelineID := "old-canvas"
	oldVersionID := "old-ver"
	if err := db.Create(&entity.PipelineOperationLog{
		ID: "run-1", DocumentID: "doc-1", TenantID: "tenant-1", KbID: "kb-1",
		ParserID: "naive", DocumentName: docName, DocumentSuffix: ".txt",
		DocumentType: "txt", SourceFrom: "local", TaskType: string(entity.PipelineTaskTypeParse),
		OperationStatus: string(entity.TaskStatusRunning), RunCount: &runCount,
		PipelineID: &oldPipelineID, PipelineVersionID: &oldVersionID,
	}).Error; err != nil {
		t.Fatalf("seed pipeline log: %v", err)
	}

	pipelineLogID := "run-1"
	task := &entity.IngestionTask{
		ID: "task-1", DocumentID: "doc-1", DatasetID: "kb-1", PipelineLogID: &pipelineLogID,
	}
	ingestor := newUnitIngestor("test", 1, nil)
	ingestor.recordTerminalPipelineLog(t.Context(), task, string(entity.TaskStatusFail), "boom")

	var run entity.PipelineOperationLog
	if err := db.First(&run, "id = ?", "run-1").Error; err != nil {
		t.Fatalf("load pipeline log: %v", err)
	}
	if run.OperationStatus != string(entity.TaskStatusFail) {
		t.Fatalf("OperationStatus = %q, want FAIL", run.OperationStatus)
	}
	if run.PipelineID == nil || *run.PipelineID != oldPipelineID {
		t.Fatalf("PipelineID = %v, want the run's %q", run.PipelineID, oldPipelineID)
	}
	if run.PipelineVersionID == nil || *run.PipelineVersionID != oldVersionID {
		t.Fatalf("PipelineVersionID = %v, want the run's %q", run.PipelineVersionID, oldVersionID)
	}

	var terminal entity.IngestionTaskLog
	if err := db.Where("pipeline_log_id = ? AND event_type = ?", "run-1", dao.EventTypeTerminal).
		First(&terminal).Error; err != nil {
		t.Fatalf("terminal event missing: %v", err)
	}
}
