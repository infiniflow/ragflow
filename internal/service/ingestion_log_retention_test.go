package service

import (
	"strings"
	"testing"

	"ragflow/internal/dao"
	"ragflow/internal/entity"

	"gorm.io/gorm"
)

func TestFoldIngestionRunLeavesRunningRunUntouched(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertFoldPipelineLog(t, db, "run-1", entity.TaskStatusRunning, 1)
	for i := 0; i < 6; i++ {
		insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", "detail")
	}

	result, err := NewIngestionTaskService().foldIngestionRun(t.Context(), "run-1", 4)
	if err != nil {
		t.Fatalf("foldIngestionRun failed: %v", err)
	}
	if !result.Skipped || result.SkipReason != "run_not_terminal" {
		t.Fatalf("result = %+v, want running run skipped", result)
	}
	if got := countFoldEvents(t, db, "run-1"); got != 6 {
		t.Fatalf("event count = %d, want 6", got)
	}
}

func TestFoldIngestionRunProtectsLatestLifecycleAndTerminal(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertFoldPipelineLog(t, db, "run-1", entity.TaskStatusDone, 1)
	insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeLifecycle, 0, "Parser", "parser started")
	insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", "head detail")
	insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeLifecycle, 1, "Parser", "parser done")
	insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeLifecycle, 0, "Chunker", "chunker started")
	insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", "middle detail 1")
	insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", "middle detail 2")
	insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeTerminal, 0, "", "Task completed.")
	insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", "tail detail")

	result, err := NewIngestionTaskService().foldIngestionRun(t.Context(), "run-1", 5)
	if err != nil {
		t.Fatalf("foldIngestionRun failed: %v", err)
	}
	if result.Before != 8 || result.After != 5 || result.Protected != 3 || result.DeletedRows != 3 || result.SummaryID == 0 {
		t.Fatalf("result = %+v, want 8 -> 5 with 3 protected/deleted rows and summary", result)
	}
	if got := countFoldEvents(t, db, "run-1"); got != 5 {
		t.Fatalf("event count = %d, want 5", got)
	}

	events := listFoldEvents(t, db, "run-1")
	var parserDone, chunkerStarted, terminal, summary, tail bool
	for _, event := range events {
		switch {
		case event.EventType == dao.EventTypeLifecycle && event.Component == "Parser" && event.Phase == 1:
			parserDone = true
		case event.EventType == dao.EventTypeLifecycle && event.Component == "Chunker" && event.Phase == 0:
			chunkerStarted = true
		case event.EventType == dao.EventTypeTerminal:
			terminal = true
		case event.EventType == dao.EventTypeSystem:
			summary = event.Component == "" && event.Phase == 0 && event.Message != ""
		case event.EventType == dao.EventTypeMessage && event.Message == "tail detail":
			tail = true
		}
	}
	if !parserDone || !chunkerStarted || !terminal || !summary || !tail {
		t.Fatalf("folded events lost protected/latest rows: %+v", events)
	}
	if !containsFoldMessage(events, "omitted 4 process messages") {
		t.Fatalf("fold summary does not report four omitted events: %+v", events)
	}

	progress, err := dao.NewIngestionTaskLogDAO().AggregateProgressByPipelineLogID(t.Context(), db, "run-1", 2)
	if err != nil {
		t.Fatalf("aggregate folded progress: %v", err)
	}
	if progress.Done != 1 || progress.Running != 1 || progress.Failed != 0 {
		t.Fatalf("progress after fold = %+v, want parser done/chunker running", progress)
	}
}

func containsFoldMessage(events []*entity.IngestionTaskLog, fragment string) bool {
	for _, event := range events {
		if event.EventType == dao.EventTypeSystem && strings.Contains(event.Message, fragment) {
			return true
		}
	}
	return false
}

func TestFoldIngestionRunIsIdempotent(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertFoldPipelineLog(t, db, "run-1", entity.TaskStatusFail, 1)
	for i := 0; i < 8; i++ {
		insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", "detail")
	}

	first, err := NewIngestionTaskService().foldIngestionRun(t.Context(), "run-1", 5)
	if err != nil {
		t.Fatalf("first fold failed: %v", err)
	}
	second, err := NewIngestionTaskService().foldIngestionRun(t.Context(), "run-1", 5)
	if err != nil {
		t.Fatalf("second fold failed: %v", err)
	}
	if first.SummaryID == 0 || second.SummaryID != first.SummaryID {
		t.Fatalf("summary ids = %d then %d, want the same row", first.SummaryID, second.SummaryID)
	}
	if second.DeletedRows != 0 || second.Before != second.After {
		t.Fatalf("second result = %+v, want idempotent no-op", second)
	}
	var summaries int64
	if err := db.Model(&entity.IngestionTaskLog{}).
		Where("pipeline_log_id = ? AND event_type = ?", "run-1", dao.EventTypeSystem).
		Count(&summaries).Error; err != nil {
		t.Fatalf("count summary rows: %v", err)
	}
	if summaries != 1 {
		t.Fatalf("summary rows = %d, want 1", summaries)
	}
}

func TestFoldIngestionRunSkipsWhenProtectedRowsReachLimit(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertFoldPipelineLog(t, db, "run-1", entity.TaskStatusCancel, 1)
	for _, component := range []string{"Parser", "Chunker", "Writer"} {
		insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeLifecycle, 0, component, "started")
	}
	insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeTerminal, 0, "", "Task stopped by user.")
	insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", "extra")

	result, err := NewIngestionTaskService().foldIngestionRun(t.Context(), "run-1", 4)
	if err != nil {
		t.Fatalf("foldIngestionRun failed: %v", err)
	}
	if !result.Skipped || result.SkipReason != "protected_rows_reach_limit" {
		t.Fatalf("result = %+v, want protected-row skip", result)
	}
	if got := countFoldEvents(t, db, "run-1"); got != 5 {
		t.Fatalf("event count = %d, want 5", got)
	}
}

func insertFoldPipelineLog(t *testing.T, db *gorm.DB, id string, status entity.TaskStatus, runCount int) {
	t.Helper()
	if err := db.Create(&entity.PipelineOperationLog{
		ID:              id,
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		DocumentName:    "doc.txt",
		DocumentSuffix:  ".txt",
		DocumentType:    "text",
		SourceFrom:      "local",
		TaskType:        string(entity.PipelineTaskTypeParse),
		OperationStatus: string(status),
		RunCount:        &runCount,
	}).Error; err != nil {
		t.Fatalf("insert pipeline log: %v", err)
	}
}

func insertFoldEvent(t *testing.T, db *gorm.DB, runID, taskID string, eventType, phase int, component, message string) int {
	t.Helper()
	result := db.Model(&entity.IngestionTaskLog{}).Create(map[string]interface{}{
		"task_id":         taskID,
		"pipeline_log_id": runID,
		"checkpoint":      entity.JSONMap{},
		"event_type":      eventType,
		"component":       component,
		"phase":           phase,
		"message":         message,
	})
	if result.Error != nil {
		t.Fatalf("insert event: %v", result.Error)
	}
	var event entity.IngestionTaskLog
	if err := db.Order("id DESC").First(&event).Error; err != nil {
		t.Fatalf("load inserted event: %v", err)
	}
	return event.ID
}

func listFoldEvents(t *testing.T, db *gorm.DB, runID string) []*entity.IngestionTaskLog {
	t.Helper()
	events, err := dao.NewIngestionTaskLogDAO().ListLogsByPipelineLogID(t.Context(), db, runID)
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	return events
}

func countFoldEvents(t *testing.T, db *gorm.DB, runID string) int {
	t.Helper()
	var count int64
	if err := db.Model(&entity.IngestionTaskLog{}).Where("pipeline_log_id = ?", runID).Count(&count).Error; err != nil {
		t.Fatalf("count events: %v", err)
	}
	return int(count)
}
