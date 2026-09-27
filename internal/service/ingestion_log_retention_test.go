package service

import (
	"fmt"
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

func TestRecordTerminalFoldsTerminalRunAfterWritingEvent(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertFoldPipelineLog(t, db, "run-1", entity.TaskStatusDone, 1)
	for i := 0; i < 8; i++ {
		insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", "detail")
	}

	svc := NewIngestionTaskService()
	if err := svc.SetIngestionLogSettings(IngestionLogSettings{
		MaxRowsPerRun:      5,
		MaxRowsPerDocument: 20,
		MaxMessageChars:    4_000,
		MaxMessageBytes:    16_384,
	}); err != nil {
		t.Fatalf("SetIngestionLogSettings failed: %v", err)
	}
	if err := svc.RecordTerminal(t.Context(), "run-1", "task-1", "Task completed."); err != nil {
		t.Fatalf("RecordTerminal failed: %v", err)
	}
	if got := countFoldEvents(t, db, "run-1"); got != 5 {
		t.Fatalf("event count = %d, want folded cap 5", got)
	}
	events := listFoldEvents(t, db, "run-1")
	if !containsFoldMessage(events, "omitted 5 process messages") {
		t.Fatalf("fold summary missing five omitted events: %+v", events)
	}
	var terminals int
	for _, event := range events {
		if event.EventType == dao.EventTypeTerminal && event.Message == "Task completed." {
			terminals++
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal events = %d, want 1", terminals)
	}
}

func TestRecordTerminalDropsOldTerminalRunsOverDocumentCap(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertFoldPipelineLog(t, db, "run-1", entity.TaskStatusDone, 1)
	insertFoldPipelineLog(t, db, "run-2", entity.TaskStatusDone, 2)
	for i := 0; i < 5; i++ {
		insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", "old detail")
		insertFoldEvent(t, db, "run-2", "task-1", dao.EventTypeMessage, 0, "", "new detail")
	}

	svc := NewIngestionTaskService()
	if err := svc.SetIngestionLogSettings(IngestionLogSettings{
		MaxRowsPerRun:      5,
		MaxRowsPerDocument: 5,
		MaxMessageChars:    4_000,
		MaxMessageBytes:    16_384,
	}); err != nil {
		t.Fatalf("SetIngestionLogSettings failed: %v", err)
	}
	if err := svc.RecordTerminal(t.Context(), "run-2", "task-1", "Task completed."); err != nil {
		t.Fatalf("RecordTerminal failed: %v", err)
	}
	if got := countFoldEvents(t, db, "run-1"); got != 0 {
		t.Fatalf("old run event count = %d, want entire run removed", got)
	}
	if got := countFoldEvents(t, db, "run-2"); got != 5 {
		t.Fatalf("latest run event count = %d, want 5", got)
	}
	var oldRun entity.PipelineOperationLog
	if err := db.First(&oldRun, "id = ?", "run-1").Error; err != nil {
		t.Fatalf("old pipeline log was deleted: %v", err)
	}
}

func TestTrimIngestionDocumentNeverDeletesRunningRun(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertFoldPipelineLog(t, db, "run-1", entity.TaskStatusDone, 1)
	insertFoldPipelineLog(t, db, "run-2", entity.TaskStatusRunning, 2)
	for i := 0; i < 4; i++ {
		insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", "old detail")
		insertFoldEvent(t, db, "run-2", "task-1", dao.EventTypeMessage, 0, "", "running detail")
	}

	result, err := NewIngestionTaskService().trimIngestionDocument(t.Context(), "doc-1", "run-2", 4)
	if err != nil {
		t.Fatalf("trimIngestionDocument failed: %v", err)
	}
	if result.DeletedRuns != 1 || result.DeletedEvents != 4 {
		t.Fatalf("trim result = %+v, want one old run/four events deleted", result)
	}
	if got := countFoldEvents(t, db, "run-1"); got != 0 {
		t.Fatalf("old terminal run events = %d, want 0", got)
	}
	if got := countFoldEvents(t, db, "run-2"); got != 4 {
		t.Fatalf("running run event count = %d, want untouched", got)
	}
}

func TestFoldIngestionRunScalesBudgetBeyondDefaultCap(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertFoldPipelineLog(t, db, "run-1", entity.TaskStatusDone, 1)

	for i := 0; i < 12; i++ {
		insertFoldEvent(t, db, "run-1", "task-1", dao.EventTypeMessage, 0, "", fmt.Sprintf("msg-%d", i))
	}

	result, err := NewIngestionTaskService().foldIngestionRun(t.Context(), "run-1", 10)
	if err != nil {
		t.Fatalf("foldIngestionRun failed: %v", err)
	}
	if result.Before != 12 || result.After != 10 || result.SummaryID == 0 {
		t.Fatalf("result = %+v, want 12 -> 10 with summary", result)
	}
	if got := countFoldEvents(t, db, "run-1"); got != 10 {
		t.Fatalf("event count = %d, want 10", got)
	}
	events := listFoldEvents(t, db, "run-1")
	var hasHead, hasTailLast bool
	for _, ev := range events {
		if ev.Message == "msg-0" {
			hasHead = true
		}
		if ev.Message == "msg-11" {
			hasTailLast = true
		}
	}
	if !hasHead || !hasTailLast {
		t.Fatalf("events missing expected head or tail: %+v", events)
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
