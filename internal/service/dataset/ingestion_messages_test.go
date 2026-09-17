package dataset

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/service"

	"gorm.io/gorm"
)

func TestListIngestionMessagesUsesRunScopedKeysets(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline operation log: %v", err)
	}
	insertCompilationOwnerKB(t, "kb-1", "user-1")
	insertMessageRun(t, "run-1", "kb-1", "doc-1", entity.TaskStatusRunning, 2)
	for i := 0; i < 5; i++ {
		insertMessageEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, "detail")
	}

	response, code, err := NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "run-1", 2, nil, nil)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListIngestionMessages = (%+v, %v, %v), want success", response, code, err)
	}
	if response.RunCount != 2 || response.OldestID != 4 || response.NewestID != 5 || !response.HasMoreBefore || response.HasMoreAfter || response.Terminal {
		t.Fatalf("first response = %+v, want newest page for active run", response)
	}
	assertMessageEventIDs(t, response.Items, 4, 5)

	afterID := 2
	response, code, err = NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "run-1", 2, &afterID, nil)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("after page = (%+v, %v, %v), want success", response, code, err)
	}
	assertMessageEventIDs(t, response.Items, 3, 4)
	if !response.HasMoreBefore || !response.HasMoreAfter {
		t.Fatalf("after response flags = before:%t after:%t, want true/true", response.HasMoreBefore, response.HasMoreAfter)
	}

	beforeID := 4
	if _, code, err := NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "run-1", 2, &afterID, &beforeID); err == nil || code != common.CodeArgumentError {
		t.Fatalf("mutually exclusive cursors = code:%v err:%v, want argument error", code, err)
	}
}

func TestListIngestionMessagesTreatsLegacyRunAsTerminalAndEmpty(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	if err := db.AutoMigrate(&entity.PipelineOperationLog{}); err != nil {
		t.Fatalf("migrate pipeline operation log: %v", err)
	}
	insertCompilationOwnerKB(t, "kb-1", "user-1")
	insertMessageRun(t, "legacy-run", "kb-1", "doc-1", entity.TaskStatusRunning, 0)
	insertMessageEvent(t, db, "legacy-run", "task-1", dao.EventTypeMessage, "old detail")

	response, code, err := NewDatasetService().ListIngestionMessages(t.Context(), "kb-1", "user-1", "legacy-run", 200, nil, nil)
	if err != nil || code != common.CodeSuccess {
		t.Fatalf("ListIngestionMessages = (%+v, %v, %v), want success", response, code, err)
	}
	if response.RunCount != 0 || !response.Terminal || len(response.Items) != 0 || response.OldestID != 0 || response.NewestID != 0 {
		t.Fatalf("legacy response = %+v, want terminal empty history", response)
	}
}

func insertMessageRun(t *testing.T, id, kbID, documentID string, status entity.TaskStatus, runCount int) {
	t.Helper()
	runCountPtr := &runCount
	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              id,
		DocumentID:      documentID,
		TenantID:        "user-1",
		KbID:            kbID,
		ParserID:        "naive",
		DocumentName:    "doc.txt",
		DocumentSuffix:  ".txt",
		DocumentType:    "text",
		SourceFrom:      "local",
		TaskType:        string(entity.PipelineTaskTypeParse),
		OperationStatus: string(status),
		RunCount:        runCountPtr,
	}).Error; err != nil {
		t.Fatalf("insert run: %v", err)
	}
}

func insertMessageEvent(t *testing.T, db *gorm.DB, runID, taskID string, eventType int, message string) {
	t.Helper()
	if err := db.Model(&entity.IngestionTaskLog{}).Create(map[string]interface{}{
		"task_id":         taskID,
		"pipeline_log_id": runID,
		"checkpoint":      entity.JSONMap{},
		"event_type":      eventType,
		"component":       "",
		"phase":           0,
		"message":         message,
	}).Error; err != nil {
		t.Fatalf("insert event: %v", err)
	}
}

func assertMessageEventIDs(t *testing.T, items []service.IngestionEventItem, want ...int) {
	t.Helper()
	if len(items) != len(want) {
		t.Fatalf("item count = %d, want %d (%v)", len(items), len(want), want)
	}
	for i, item := range items {
		if item.ID != want[i] {
			t.Fatalf("item %d ID = %d, want %d", i, item.ID, want[i])
		}
	}
}
