package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type recordingTaskPublisher struct {
	subject      string
	messages     []common.TaskMessage
	err          error
	beforeReturn func(taskID string)
}

type failFirstTaskPublisher struct {
	messages []common.TaskMessage
}

type duplicatePublishRecorder struct {
	mu              sync.Mutex
	messages        []common.TaskMessage
	firstPublished  chan struct{}
	secondPublished chan struct{}
	releaseFirst    chan struct{}
	releaseSecond   chan struct{}
}

func (p *failFirstTaskPublisher) PublishTaskMessage(_ string, msg common.TaskMessage) error {
	p.messages = append(p.messages, msg)
	if len(p.messages) == 1 {
		return errors.New("publish failed")
	}
	return nil
}

func (p *duplicatePublishRecorder) PublishTaskMessage(_ string, msg common.TaskMessage) error {
	p.mu.Lock()
	p.messages = append(p.messages, msg)
	publishNumber := len(p.messages)
	p.mu.Unlock()

	switch publishNumber {
	case 1:
		close(p.firstPublished)
		<-p.releaseFirst
	case 2:
		close(p.secondPublished)
		<-p.releaseSecond
	}
	return nil
}

func (p *recordingTaskPublisher) PublishTaskMessage(subject string, msg common.TaskMessage) error {
	p.subject = subject
	p.messages = append(p.messages, msg)
	if p.beforeReturn != nil {
		p.beforeReturn(msg.TaskID)
	}
	return p.err
}

func TestIngestionTaskServiceCreateForDocumentsPublishesTaskMessages(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	ctx := t.Context()
	resp, err := svc.CreateForDocuments(ctx, "kb-1", "user-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	if publisher.subject != "tasks.RAGFLOW" {
		t.Fatalf("subject = %q, want %q", publisher.subject, "tasks.RAGFLOW")
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("expected 1 published message, got %d", len(publisher.messages))
	}
	msg := publisher.messages[0]
	if msg.TaskType != common.TaskTypeIngestionTask {
		t.Fatalf("task type = %q, want %q", msg.TaskType, common.TaskTypeIngestionTask)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, msg.TaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.DocumentID != "doc-1" || task.DatasetID != "kb-1" || task.UserID != "user-1" {
		t.Fatalf("unexpected task: %+v", task)
	}
}

func TestIngestionTaskServiceCreateForDocumentsRejectsMissingRunMetadata(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestDoc(t, "doc-1", "missing-kb", 0, 0)

	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	responses, err := svc.CreateForDocuments(t.Context(), "missing-kb", "user-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments returns per-document failures: %v", err)
	}
	if len(responses) != 1 || !strings.Contains(responses[0].Result, "ensure run identity") {
		t.Fatalf("unexpected responses: %+v", responses)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages = %d, want 0", len(publisher.messages))
	}
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(t.Context(), db, "doc-1")
	if err != nil {
		t.Fatalf("load queued task: %v", err)
	}
	if task != nil {
		t.Fatalf("task = %+v, want no task after identity setup fails", task)
	}
	if got := countPipelineLogs(t, db, "doc-1"); got != 0 {
		t.Fatalf("pipeline log rows = %d, want 0 when metadata lookup fails", got)
	}
}

func TestIngestionTaskServiceCreateForDocumentsRejectsForeignCleanupClaim(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	if _, err := dao.NewDocumentCleanupClaimDAO().Acquire(t.Context(), db, "doc-1", "api-a", time.Now().Unix(), 120, 45); err != nil {
		t.Fatalf("acquire cleanup claim: %v", err)
	}

	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	responses, err := svc.CreateForDocuments(t.Context(), "kb-1", "user-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments returns per-document failures: %v", err)
	}
	if len(responses) != 1 || !strings.Contains(responses[0].Result, "cleanup claim") {
		t.Fatalf("unexpected responses: %+v", responses)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages = %d, want 0", len(publisher.messages))
	}
}

func TestIngestionTaskServiceCreateForDocumentsRejectsCleanupClaimDuringTakeoverGrace(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	now := time.Now().Unix()
	if _, err := dao.NewDocumentCleanupClaimDAO().Acquire(t.Context(), db, "doc-1", "cleanup-a", now-20, 10, 45); err != nil {
		t.Fatalf("acquire expired cleanup claim: %v", err)
	}

	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	responses, err := svc.CreateForDocuments(t.Context(), "kb-1", "user-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments returns per-document failures: %v", err)
	}
	if len(responses) != 1 || !strings.Contains(responses[0].Result, "cleanup claim") {
		t.Fatalf("unexpected responses: %+v", responses)
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("published messages = %d, want 0 during takeover grace", len(publisher.messages))
	}
}

func TestIngestionTaskServiceMarksTaskScheduledOnlyAfterPublish(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	statusDuringPublish := ""
	publisher := &recordingTaskPublisher{
		beforeReturn: func(taskID string) {
			task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, taskID)
			if err != nil {
				t.Fatalf("load task during publish: %v", err)
			}
			statusDuringPublish = task.Status
		},
	}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	responses, err := svc.CreateForDocuments(t.Context(), "kb-1", "user-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}
	if statusDuringPublish != common.CREATED {
		t.Fatalf("status during publish = %q, want %q", statusDuringPublish, common.CREATED)
	}
	if len(responses) != 1 || !strings.HasPrefix(responses[0].Result, "task_id: ") {
		t.Fatalf("unexpected parse response: %+v", responses)
	}

	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(t.Context(), db, "doc-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.SCHEDULED {
		t.Fatalf("status after publish = %q, want %q", task.Status, common.SCHEDULED)
	}
}

func TestIngestionTaskServiceAcceptsConsumerWinningPublishRace(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	publisher := &recordingTaskPublisher{
		beforeReturn: func(taskID string) {
			if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
				Update("status", common.RUNNING).Error; err != nil {
				t.Fatalf("simulate consumer start: %v", err)
			}
		},
	}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	responses, err := svc.CreateForDocuments(t.Context(), "kb-1", "user-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}
	if len(responses) != 1 || !strings.HasPrefix(responses[0].Result, "task_id: ") {
		t.Fatalf("unexpected parse response: %+v", responses)
	}

	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(t.Context(), db, "doc-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("status after consumer race = %q, want %q", task.Status, common.RUNNING)
	}
}

func TestIngestionTaskServiceStartRunningTransitionsScheduledTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.SCHEDULED)

	task, err := NewIngestionTaskService().TransitionTaskToRunning(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("StartRunning failed: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("status = %q, want %q", task.Status, common.RUNNING)
	}
}

func TestIngestionTaskServiceStartRunningFromCreatedTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)

	task, err := NewIngestionTaskService().StartRunning(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("StartRunning failed: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("status = %q, want %q", task.Status, common.RUNNING)
	}
}

func TestIngestionTaskServiceTransitionFromRejectsConflict(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.STOPPED)

	svc := NewIngestionTaskService()
	ctx := t.Context()
	_, err := svc.transitionFrom(ctx, "task-1", []string{common.CREATED, common.SCHEDULED}, common.RUNNING)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	var conflictErr *TaskStatusConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected TaskStatusConflictError, got %T (%v)", err, err)
	}
	if conflictErr.ExpectedFrom != "CREATED/SCHEDULED" {
		t.Fatalf("expected %q, got %q", "CREATED/SCHEDULED", conflictErr.ExpectedFrom)
	}
	if conflictErr.ActualCurrent != common.STOPPED {
		t.Fatalf("actual current = %q, want %q", conflictErr.ActualCurrent, common.STOPPED)
	}
}

func TestIngestionTaskServiceListByUserFiltersDataset(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	insertTestIngestionTask(t, "task-2", "user-1", "doc-2", "kb-2")
	insertTestIngestionTask(t, "task-3", "user-2", "doc-3", "kb-1")

	svc := NewIngestionTaskService()
	datasetID := "kb-1"
	ctx := t.Context()
	tasks, err := svc.ListByUser(ctx, "user-1", &datasetID, 0, 0)
	if err != nil {
		t.Fatalf("ListByUser failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0].ID != "task-1" {
		t.Fatalf("task ID = %q, want %q", tasks[0].ID, "task-1")
	}
}

func TestIngestionTaskServiceRequestStopManyStopsOwnedTasks(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	ctx := t.Context()
	userID := "user-1"
	svc := NewIngestionTaskService()
	tasks, err := svc.RequestStopMany(ctx, []string{"task-1"}, &userID)
	if err != nil {
		t.Fatalf("RequestStopMany failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task response, got %d", len(tasks))
	}
	if tasks[0].Status != common.STOPPED {
		t.Fatalf("status = %q, want %q", tasks[0].Status, common.STOPPED)
	}
}

func TestIngestionTaskServiceRequestStopManyRejectsOtherUsersTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	ctx := t.Context()
	userID := "user-2"
	svc := NewIngestionTaskService()
	if _, err := svc.RequestStopMany(ctx, []string{"task-1"}, &userID); err == nil {
		t.Fatal("expected RequestStopMany to reject non-owner")
	}
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.CREATED {
		t.Fatalf("status = %q, want %q", task.Status, common.CREATED)
	}
}

func TestIngestionTaskServiceRequestStopManyAllowsAdmin(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	ctx := t.Context()
	svc := NewIngestionTaskService()
	tasks, err := svc.RequestStopMany(ctx, []string{"task-1"}, nil)
	if err != nil {
		t.Fatalf("RequestStopMany admin failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task response, got %d", len(tasks))
	}
	if tasks[0].Status != common.STOPPED {
		t.Fatalf("status = %q, want %q", tasks[0].Status, common.STOPPED)
	}
}

func TestIngestionTaskServiceRemoveManyRemovesOwnedTasks(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	ctx := t.Context()
	userID := "user-1"
	svc := NewIngestionTaskService()
	result, err := svc.RemoveMany(ctx, []string{"task-1"}, &userID)
	if err != nil {
		t.Fatalf("RemoveMany failed: %v", err)
	}
	if len(result) != 1 || result[0]["remove"] != "success" {
		t.Fatalf("unexpected remove result: %+v", result)
	}
	if _, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1"); err == nil {
		t.Fatal("task should be removed")
	}
}

func TestIngestionTaskServiceListAllForAdminIncludesRunAndUserEmail(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	status := "1"
	if err := dao.DB.Create(&entity.User{
		ID:              "user-1",
		Email:           "user-1@test.com",
		Nickname:        "user-1",
		IsAuthenticated: "1",
		IsActive:        "1",
		IsAnonymous:     "0",
		Status:          &status,
	}).Error; err != nil {
		t.Fatalf("insert user: %v", err)
	}
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	runCount := 3
	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              "run-1",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		TaskType:        string(entity.PipelineTaskTypeParse),
		OperationStatus: string(entity.TaskStatusRunning),
		RunCount:        &runCount,
	}).Error; err != nil {
		t.Fatalf("insert pipeline operation log: %v", err)
	}
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("pipeline_log_id", "run-1").Error; err != nil {
		t.Fatalf("bind pipeline operation log: %v", err)
	}
	if err := dao.DB.Exec(`INSERT INTO ingestion_task_log
		(task_id, pipeline_log_id, checkpoint, event_type, component, phase, message)
		VALUES (?, ?, '{}', ?, ?, ?, ?)`, "task-1", "run-1", dao.EventTypeLifecycle, "Parser", 1, "Parser Done").Error; err != nil {
		t.Fatalf("insert task log: %v", err)
	}

	ctx := t.Context()
	svc := NewIngestionTaskService()
	tasks, err := svc.ListAllForAdmin(ctx)
	if err != nil {
		t.Fatalf("ListAllForAdmin failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0]["user"] != "user-1@test.com" {
		t.Fatalf("user = %v, want user-1@test.com", tasks[0]["user"])
	}
	if tasks[0]["run_count"] != 3 {
		t.Fatalf("run_count = %v, want 3", tasks[0]["run_count"])
	}
	if tasks[0]["component_total"] != 0 {
		t.Fatalf("component_total = %v, want 0", tasks[0]["component_total"])
	}
	if tasks[0]["component_done"] != 0 {
		t.Fatalf("component_done = %v, want 0", tasks[0]["component_done"])
	}
}

func TestIngestionTaskServiceStartRunningTransitionsCreatedTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	svc := NewIngestionTaskService()
	ctx := t.Context()
	task, err := svc.TransitionTaskToRunning(ctx, "task-1")
	if err != nil {
		t.Fatalf("StartRunning failed: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("status = %q, want %q", task.Status, common.RUNNING)
	}
}

// TestPrepareValidatedRunResetsDocumentProgress verifies validation precedes
// document initialization and leaves the legacy progress_msg untouched.
func TestPrepareValidatedRunResetsDocumentProgress(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 100, 10)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	// Seed the document as a partially-processed state that the start transition must reset.
	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").
		Updates(map[string]interface{}{
			"progress":     float64(0.5),
			"progress_msg": "partial",
		}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}

	svc := NewIngestionTaskService()
	ctx := t.Context()
	task, err := svc.TransitionTaskToRunning(ctx, "task-1")
	if err != nil {
		t.Fatalf("TransitionTaskToRunning failed: %v", err)
	}
	svc.PrepareValidatedRun(ctx, task)

	var doc entity.Document
	if err := db.Where("id = ?", "doc-1").First(&doc).Error; err != nil {
		t.Fatalf("reload document: %v", err)
	}
	if doc.Progress != 0 {
		t.Fatalf("progress = %f, want 0", doc.Progress)
	}
	if doc.ChunkNum != 0 {
		t.Fatalf("chunk_num = %d, want 0", doc.ChunkNum)
	}
	if doc.TokenNum != 0 {
		t.Fatalf("token_num = %d, want 0", doc.TokenNum)
	}
	if doc.ProgressMsg == nil || *doc.ProgressMsg != "partial" {
		t.Fatalf("progress_msg = %v, want the legacy value unchanged", doc.ProgressMsg)
	}
	if doc.ProcessBeginAt == nil || doc.ProcessBeginAt.IsZero() {
		t.Fatal("process_begin_at not set")
	}
}

// TestStartRunningLeavesTerminalDocumentUntouched locks in the no-resurrection
// invariant: a task already in a terminal status is returned as-is by
// StartRunning, and its document's finished run status/counters are not reset.
func TestStartRunningLeavesTerminalDocumentUntouched(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 100, 10)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	if err := db.Model(&entity.Document{}).Where("id = ?", "doc-1").
		Updates(map[string]interface{}{
			"progress":     float64(1.0),
			"progress_msg": "done",
		}).Error; err != nil {
		t.Fatalf("seed document: %v", err)
	}
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("status", common.COMPLETED).Error; err != nil {
		t.Fatalf("set COMPLETED: %v", err)
	}

	svc := NewIngestionTaskService()
	ctx := t.Context()
	task, err := svc.TransitionTaskToRunning(ctx, "task-1")
	if err != nil {
		t.Fatalf("StartRunning failed: %v", err)
	}
	if task.Status != common.COMPLETED {
		t.Fatalf("status = %q, want %q (terminal must be preserved)", task.Status, common.COMPLETED)
	}

	var doc entity.Document
	if err := db.Where("id = ?", "doc-1").First(&doc).Error; err != nil {
		t.Fatalf("reload document: %v", err)
	}
	if doc.Progress != 1.0 {
		t.Fatalf("progress = %v, want 1.0", doc.Progress)
	}
	if doc.ChunkNum != 10 || doc.TokenNum != 100 {
		t.Fatalf("counters changed: chunk_num=%d token_num=%d, want 10/100", doc.ChunkNum, doc.TokenNum)
	}
}

// TestStartRunningFinalizesStoppingTask locks in the redelivery path: a
// STOPPING task (cancelled after being nacked, before any worker ran it) is
// moved to STOPPED by StartRunning, so the task reaches a terminal state and
// a later re-parse can transition it back to CREATED.
func TestStartRunningFinalizesStoppingTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("status", common.STOPPING).Error; err != nil {
		t.Fatalf("set STOPPING: %v", err)
	}

	svc := NewIngestionTaskService()
	ctx := t.Context()
	task, err := svc.TransitionTaskToRunning(ctx, "task-1")
	if err != nil {
		t.Fatalf("StartRunning failed: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("status = %q, want %q", task.Status, common.STOPPED)
	}
}

func TestIngestionTaskServiceRequestStopTransitionsCreatedTaskToStopped(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	ctx := t.Context()

	svc := NewIngestionTaskService()
	task, err := svc.RequestStop(ctx, "task-1")
	if err != nil {
		t.Fatalf("RequestStop failed: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("status = %q, want %q", task.Status, common.STOPPED)
	}
}

func TestIngestionTaskServiceRequestStopTransitionsScheduledTaskToStopped(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("status", common.SCHEDULED).Error; err != nil {
		t.Fatalf("set SCHEDULED: %v", err)
	}
	ctx := t.Context()

	svc := NewIngestionTaskService()
	task, err := svc.RequestStop(ctx, "task-1")
	if err != nil {
		t.Fatalf("RequestStop failed: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("status = %q, want %q", task.Status, common.STOPPED)
	}
}

func TestIngestionTaskServiceMarkCompletedRejectsNonRunningTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	ctx := t.Context()

	svc := NewIngestionTaskService()
	if err := svc.MarkCompleted(ctx, "task-1"); err == nil {
		t.Fatal("expected MarkCompleted to reject non-running task")
	}
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.CREATED {
		t.Fatalf("status = %q, want %q", task.Status, common.CREATED)
	}
}

func TestIngestionTaskServiceMarkCompletedUpdatesTaskStatus(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("status", common.RUNNING).Error; err != nil {
		t.Fatalf("set running status: %v", err)
	}
	ctx := t.Context()

	svc := NewIngestionTaskService()
	if err := svc.MarkCompleted(ctx, "task-1"); err != nil {
		t.Fatalf("MarkCompleted failed: %v", err)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.COMPLETED {
		t.Fatalf("status = %q, want %q", task.Status, common.COMPLETED)
	}
}

func TestIngestionTaskServiceMarkFailedUpdatesTaskStatus(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("status", common.RUNNING).Error; err != nil {
		t.Fatalf("set running status: %v", err)
	}
	ctx := t.Context()

	svc := NewIngestionTaskService()
	if err := svc.MarkFailed(ctx, "task-1"); err != nil {
		t.Fatalf("MarkFailed failed: %v", err)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("status = %q, want %q", task.Status, common.FAILED)
	}
}

func TestIngestionTaskServiceNewTaskStatusConflictErrorLoadsActualStatus(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("status", common.STOPPING).Error; err != nil {
		t.Fatalf("set stopping status: %v", err)
	}
	ctx := t.Context()

	svc := NewIngestionTaskService()
	err := svc.newTaskStatusConflictError(ctx, "task-1", common.CREATED, common.RUNNING)
	var conflictErr *TaskStatusConflictError
	if !errors.As(err, &conflictErr) {
		t.Fatalf("expected TaskStatusConflictError, got %T", err)
	}
	if conflictErr.TaskID != "task-1" || conflictErr.ExpectedFrom != common.CREATED || conflictErr.AttemptedTo != common.RUNNING || conflictErr.ActualCurrent != common.STOPPING {
		t.Fatalf("unexpected conflict error: %+v", conflictErr)
	}
}

func TestIngestionTaskServiceMarkCompletedReturnsTaskIDInTransitionError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	ctx := t.Context()

	svc := NewIngestionTaskService()
	err := svc.MarkCompleted(ctx, "task-1")
	var transitionErr *InvalidTaskTransitionError
	if !errors.As(err, &transitionErr) {
		t.Fatalf("expected InvalidTaskTransitionError, got %T", err)
	}
	if transitionErr.TaskID != "task-1" || transitionErr.From != common.CREATED || transitionErr.To != common.COMPLETED {
		t.Fatalf("unexpected transition error: %+v", transitionErr)
	}
}

func TestIngestionTaskServiceCreateAndEnqueueRetriesTerminalTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestDoc(t, "doc-2", "kb-1", 0, 0)
	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	testCases := []struct {
		name   string
		status string
		docID  string
	}{
		{name: "failed", status: common.FAILED, docID: "doc-1"},
		{name: "stopped", status: common.STOPPED, docID: "doc-2"},
	}

	ctx := t.Context()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			publisher.subject = ""
			publisher.messages = nil
			if err := dao.DB.Where("id = ?", "task-1").Delete(&entity.IngestionTask{}).Error; err != nil {
				t.Fatalf("clear task: %v", err)
			}
			insertTestIngestionTask(t, "task-1", "user-1", tc.docID, "kb-1")
			if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("status", tc.status).Error; err != nil {
				t.Fatalf("set terminal status: %v", err)
			}

			task, err := svc.CreateAndEnqueue(ctx, &entity.IngestionTask{
				DocumentID: tc.docID,
				UserID:     "user-1",
				DatasetID:  "kb-1",
				Status:     common.CREATED,
			})
			if err != nil {
				t.Fatalf("CreateAndEnqueue failed: %v", err)
			}
			if task.ID != "task-1" {
				t.Fatalf("task ID = %q, want task-1", task.ID)
			}
			if task.Status != common.SCHEDULED {
				t.Fatalf("status = %q, want %q", task.Status, common.SCHEDULED)
			}
			if len(publisher.messages) != 1 || publisher.messages[0].TaskID != "task-1" {
				t.Fatalf("unexpected published messages: %+v", publisher.messages)
			}
			reloaded, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
			if err != nil {
				t.Fatalf("reload task: %v", err)
			}
			if reloaded.Status != common.SCHEDULED {
				t.Fatalf("reloaded status = %q, want %q", reloaded.Status, common.SCHEDULED)
			}
		})
	}
}

func TestIngestionTaskServiceCreateAndEnqueueSchedulesExistingCreatedTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "stale-task", "user-1", "doc-1", "kb-1", common.CREATED)

	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	task, err := svc.CreateAndEnqueue(t.Context(), &entity.IngestionTask{
		DocumentID: "doc-1",
		UserID:     "user-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	})
	if err != nil {
		t.Fatalf("CreateAndEnqueue failed: %v", err)
	}
	if task.ID != "stale-task" {
		t.Fatalf("task ID = %q, want stale-task", task.ID)
	}
	if task.Status != common.SCHEDULED {
		t.Fatalf("status = %q, want %q", task.Status, common.SCHEDULED)
	}
	if len(publisher.messages) != 1 || publisher.messages[0].TaskID != task.ID {
		t.Fatalf("unexpected published messages: %+v", publisher.messages)
	}

	reloaded, err := dao.NewIngestionTaskDAO().GetByDocumentID(t.Context(), db, "doc-1")
	if err != nil {
		t.Fatalf("load task after scheduling created task: %v", err)
	}
	if reloaded == nil || reloaded.ID != "stale-task" || reloaded.Status != common.SCHEDULED {
		t.Fatalf("expected created task to be scheduled in place, task=%+v", reloaded)
	}
}

func TestIngestionTaskServiceConcurrentCreatedTaskPublicationsConverge(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)

	publisher := &duplicatePublishRecorder{
		firstPublished:  make(chan struct{}),
		secondPublished: make(chan struct{}),
		releaseFirst:    make(chan struct{}),
		releaseSecond:   make(chan struct{}),
	}
	firstService := NewIngestionTaskService()
	firstService.taskPublisher = publisher
	secondService := NewIngestionTaskService()
	secondService.taskPublisher = publisher

	type result struct {
		task *entity.IngestionTask
		err  error
	}
	results := make(chan result, 2)
	enqueue := func(svc *IngestionTaskService) {
		task, err := svc.CreateAndEnqueue(t.Context(), &entity.IngestionTask{
			DocumentID: "doc-1",
			UserID:     "user-1",
			DatasetID:  "kb-1",
			Status:     common.CREATED,
		})
		results <- result{task: task, err: err}
	}

	go enqueue(firstService)
	<-publisher.firstPublished
	go enqueue(secondService)
	<-publisher.secondPublished

	close(publisher.releaseFirst)
	first := <-results
	if first.err != nil {
		t.Fatalf("first CreateAndEnqueue: %v", first.err)
	}
	close(publisher.releaseSecond)
	second := <-results
	if second.err != nil {
		t.Fatalf("second CreateAndEnqueue: %v", second.err)
	}
	if first.task.ID != "task-1" || second.task.ID != "task-1" {
		t.Fatalf("concurrent requests returned task IDs %q and %q, want task-1", first.task.ID, second.task.ID)
	}

	publisher.mu.Lock()
	messages := append([]common.TaskMessage(nil), publisher.messages...)
	publisher.mu.Unlock()
	if len(messages) != 2 || messages[0].TaskID != "task-1" || messages[1].TaskID != "task-1" {
		t.Fatalf("published messages = %+v, want two messages for task-1", messages)
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, "task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.SCHEDULED {
		t.Fatalf("task status after concurrent publication = %q, want %q", task.Status, common.SCHEDULED)
	}
}

func TestIngestionTaskServiceScheduleCreatedTasksKeepsTaskCreatedOnPublishFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)

	svc := NewIngestionTaskService()
	svc.taskPublisher = &recordingTaskPublisher{err: errors.New("publish failed")}
	if err := svc.ScheduleCreatedTasks(t.Context()); err == nil {
		t.Fatal("expected ScheduleCreatedTasks to return publish error")
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, "task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("status after failed recovery publish = %q, want %q", task.Status, common.FAILED)
	}
}

func TestIngestionTaskServiceScheduleCreatedTasksContinuesAfterPublishFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestDoc(t, "doc-2", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)
	insertTestIngestionTaskWithStatus(t, "task-2", "user-1", "doc-2", "kb-1", common.CREATED)
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("create_time", 1).Error; err != nil {
		t.Fatalf("set first task create time: %v", err)
	}
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "task-2").Update("create_time", 2).Error; err != nil {
		t.Fatalf("set second task create time: %v", err)
	}

	publisher := &failFirstTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher
	if err := svc.ScheduleCreatedTasks(t.Context()); err == nil {
		t.Fatal("expected ScheduleCreatedTasks to return publish error")
	}
	if len(publisher.messages) != 2 {
		t.Fatalf("published messages = %d, want 2", len(publisher.messages))
	}

	first, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, "task-1")
	if err != nil {
		t.Fatalf("reload first task: %v", err)
	}
	if first.Status != common.FAILED {
		t.Fatalf("first task status = %q, want %q", first.Status, common.FAILED)
	}
	second, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, "task-2")
	if err != nil {
		t.Fatalf("reload second task: %v", err)
	}
	if second.Status != common.SCHEDULED {
		t.Fatalf("second task status = %q, want %q", second.Status, common.SCHEDULED)
	}
}

func TestIngestionTaskServiceCreateAndEnqueueRejectsActiveExistingTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.SCHEDULED)
	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	ctx := t.Context()
	_, err := svc.CreateAndEnqueue(ctx, &entity.IngestionTask{DocumentID: "doc-1", UserID: "user-1", DatasetID: "kb-1", Status: common.CREATED})
	if err == nil {
		t.Fatal("expected CreateAndEnqueue to reject existing created task")
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("expected no published messages, got %+v", publisher.messages)
	}
}

func TestIngestionTaskServiceCreateAndEnqueueSettlesNewRunOnPublishFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	publisher := &recordingTaskPublisher{err: errors.New("publish failed")}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	ctx := t.Context()
	_, err := svc.CreateAndEnqueue(ctx, &entity.IngestionTask{
		DocumentID: "doc-1",
		UserID:     "user-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	})
	if err == nil || err.Error() != "publish failed" {
		t.Fatalf("expected publish failure, got %v", err)
	}
	task, getErr := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-1")
	if getErr != nil {
		t.Fatalf("reload task by document id: %v", getErr)
	}
	if task == nil || task.Status != common.FAILED {
		t.Fatalf("task = %+v, want a durably failed task", task)
	}
	var run entity.PipelineOperationLog
	if err := db.Where("id = ?", *task.PipelineLogID).First(&run).Error; err != nil {
		t.Fatalf("load numbered pipeline log: %v", err)
	}
	if run.RunCount == nil || *run.RunCount != 1 || run.OperationStatus != string(entity.TaskStatusFail) {
		t.Fatalf("run = %+v, want numbered failed run", run)
	}
}

func TestIngestionTaskServiceCreateAndEnqueueSettlesRetriedRunOnPublishFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	// Seed the document and KB so buildOpenLogInput succeeds: without them no
	// open row is created and the rollback cleanup below is never exercised.
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("status", common.FAILED).Error; err != nil {
		t.Fatalf("set failed status: %v", err)
	}

	publisher := &recordingTaskPublisher{err: errors.New("publish failed")}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	ctx := t.Context()
	_, err := svc.CreateAndEnqueue(ctx, &entity.IngestionTask{
		DocumentID: "doc-1",
		UserID:     "user-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	})
	if err == nil || err.Error() != "publish failed" {
		t.Fatalf("expected publish failure, got %v", err)
	}
	reloaded, getErr := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if getErr != nil {
		t.Fatalf("reload task: %v", getErr)
	}
	if reloaded.Status != common.FAILED {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.FAILED)
	}
	if reloaded.PipelineLogID == nil {
		t.Fatal("failed retry lost its run identity")
	}
	var run entity.PipelineOperationLog
	if err := db.Where("id = ?", *reloaded.PipelineLogID).First(&run).Error; err != nil {
		t.Fatalf("load failed retry run: %v", err)
	}
	if run.RunCount == nil || *run.RunCount != 1 || run.OperationStatus != string(entity.TaskStatusFail) {
		t.Fatalf("run = %+v, want numbered failed retry", run)
	}
}

func TestIngestionTaskServiceRemoveSettlesNumberedQueuedRun(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)

	runCount := 1
	if err := db.Create(&entity.PipelineOperationLog{
		ID:              "numbered-open-log",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		TaskType:        string(entity.PipelineTaskTypeParse),
		OperationStatus: string(entity.TaskStatusUnstart),
		RunCount:        &runCount,
	}).Error; err != nil {
		t.Fatalf("seed numbered pipeline log: %v", err)
	}
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("pipeline_log_id", "numbered-open-log").Error; err != nil {
		t.Fatalf("bind numbered pipeline log: %v", err)
	}

	if _, err := NewIngestionTaskService().Remove(t.Context(), "task-1", sptr("user-1")); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	var run entity.PipelineOperationLog
	if err := db.First(&run, "id = ?", "numbered-open-log").Error; err != nil {
		t.Fatalf("numbered run was deleted: %v", err)
	}
	if run.OperationStatus != string(entity.TaskStatusCancel) {
		t.Fatalf("numbered run status = %q, want %q", run.OperationStatus, entity.TaskStatusCancel)
	}
	var terminal entity.IngestionTaskLog
	if err := db.Where("pipeline_log_id = ? AND event_type = ?", "numbered-open-log", dao.EventTypeTerminal).
		First(&terminal).Error; err != nil {
		t.Fatalf("terminal event missing: %v", err)
	}
	if terminal.Message != "Task superseded by a new parse request." {
		t.Fatalf("terminal message = %q, want supersede reason", terminal.Message)
	}
}

func TestIngestionTaskServiceUpdateComponentTotalPersistsDenominator(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	ctx := t.Context()

	svc := NewIngestionTaskService()
	if err := svc.UpdateComponentTotal(ctx, "task-1", 4); err != nil {
		t.Fatalf("UpdateComponentTotal failed: %v", err)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.ComponentTotal != 4 {
		t.Fatalf("component_total = %d, want 4", task.ComponentTotal)
	}
}

func TestIngestionTaskServiceRecordLifecyclePersistsRunScopedLifecycleEvent(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	svc := NewIngestionTaskService()
	if err := svc.RecordLifecycle(t.Context(), "run-1", "task-1", "Parser", 1, "Parser Done"); err != nil {
		t.Fatalf("RecordLifecycle failed: %v", err)
	}
	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByPipelineLogID(t.Context(), db, "run-1")
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("event count = %d, want 1", len(logs))
	}
	event := logs[0]
	if event.PipelineLogID == nil || *event.PipelineLogID != "run-1" {
		t.Fatalf("pipeline_log_id = %v, want run-1", event.PipelineLogID)
	}
	if event.EventType != dao.EventTypeLifecycle || event.Component != "Parser" || event.Phase != 1 {
		t.Fatalf("event = %+v, want lifecycle Parser/1", event)
	}
}

func TestIngestionTaskServiceAggregateTaskProgressByRunClassifiesByPhase(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	ctx := t.Context()

	svc := NewIngestionTaskService()
	if err := svc.RecordLifecycle(ctx, "run-1", "task-1", "Parser", 1, "Parser Done"); err != nil {
		t.Fatalf("record Parser: %v", err)
	}
	if err := svc.RecordLifecycle(ctx, "run-1", "task-1", "Chunker", 0, "Chunker Started"); err != nil {
		t.Fatalf("record Chunker: %v", err)
	}
	if err := svc.RecordLifecycle(ctx, "run-2", "task-1", "Parser", 0, "other run"); err != nil {
		t.Fatalf("record other run: %v", err)
	}
	agg, err := svc.AggregateTaskProgressByPipelineLogID(ctx, "run-1", 2)
	if err != nil {
		t.Fatalf("AggregateTaskProgressByPipelineLogID failed: %v", err)
	}
	if agg.Done != 1 || agg.Running != 1 || agg.Failed != 0 {
		t.Fatalf("aggregate = %+v, want Done=1 Running=1 Failed=0", agg)
	}
	if agg.Percent != 50 {
		t.Fatalf("percent = %v, want 50", agg.Percent)
	}
}

func TestIngestionTaskServiceRecordMessageClearsLifecycleFields(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	if err := NewIngestionTaskService().RecordMessage(t.Context(), "run-1", "task-1", "detail"); err != nil {
		t.Fatalf("RecordMessage failed: %v", err)
	}

	var event entity.IngestionTaskLog
	if err := db.Order("id DESC").First(&event).Error; err != nil {
		t.Fatalf("load event: %v", err)
	}
	if event.EventType != dao.EventTypeMessage || event.Component != "" || event.Phase != 0 || event.Message != "detail" {
		t.Fatalf("event = %+v, want message with empty component and phase", event)
	}
}

func TestIngestionTaskServiceRecordEventRejectsInvalidIdentityAndPhase(t *testing.T) {
	svc := NewIngestionTaskService()
	cases := []struct {
		name string
		call func() error
	}{
		{name: "missing pipeline log id", call: func() error {
			return svc.RecordMessage(t.Context(), "", "task-1", "detail")
		}},
		{name: "missing task id", call: func() error {
			return svc.RecordMessage(t.Context(), "run-1", "", "detail")
		}},
		{name: "missing lifecycle component", call: func() error {
			return svc.RecordLifecycle(t.Context(), "run-1", "task-1", "", 0, "started")
		}},
		{name: "invalid lifecycle phase", call: func() error {
			return svc.RecordLifecycle(t.Context(), "run-1", "task-1", "Parser", 3, "bad")
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.call(); err == nil {
				t.Fatal("expected event validation error")
			}
		})
	}
}

func TestIngestionTaskServiceEventMessageIsBoundedByRunesAndBytes(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	message := strings.Repeat("界", 10_000)
	if err := NewIngestionTaskService().RecordMessage(t.Context(), "run-1", "task-1", message); err != nil {
		t.Fatalf("RecordMessage failed: %v", err)
	}

	var event entity.IngestionTaskLog
	if err := db.Order("id DESC").First(&event).Error; err != nil {
		t.Fatalf("load event: %v", err)
	}
	if got := len([]rune(event.Message)); got > 4_000 {
		t.Fatalf("message rune length = %d, want <= 4000", got)
	}
	if got := len([]byte(event.Message)); got > 16_384 {
		t.Fatalf("message byte length = %d, want <= 16384", got)
	}
	if !strings.Contains(event.Message, "… [truncated,") || !strings.HasSuffix(event.Message, " chars dropped]") {
		t.Fatalf("message lacks truncation marker: %q", event.Message[len(event.Message)-64:])
	}
	if !utf8.ValidString(event.Message) {
		t.Fatal("message is not valid UTF-8")
	}
}

func TestIngestionTaskServiceUsesConfiguredEventMessageLimits(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	svc := NewIngestionTaskService()
	if err := svc.SetIngestionLogSettings(IngestionLogSettings{
		MaxRowsPerRun:      5,
		MaxRowsPerDocument: 20,
		MaxMessageChars:    8,
		MaxMessageBytes:    16,
	}); err != nil {
		t.Fatalf("SetIngestionLogSettings failed: %v", err)
	}
	if err := svc.RecordMessage(t.Context(), "run-1", "task-1", "abcdefghijk"); err != nil {
		t.Fatalf("RecordMessage failed: %v", err)
	}

	var event entity.IngestionTaskLog
	if err := db.Order("id DESC").First(&event).Error; err != nil {
		t.Fatalf("load event: %v", err)
	}
	if got := len([]rune(event.Message)); got > 8 || len([]byte(event.Message)) > 16 {
		t.Fatalf("message limits = %d runes/%d bytes, want <= 8/16: %q", got, len([]byte(event.Message)), event.Message)
	}
}

func TestIngestionTaskServiceAdvanceOpenLogLeavesLegacyMessageUntouched(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	legacy := "legacy value"
	if err := db.Create(&entity.PipelineOperationLog{
		ID:              "run-1",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		TaskType:        string(entity.PipelineTaskTypeParse),
		OperationStatus: string(entity.TaskStatusUnstart),
		ProgressMsg:     &legacy,
	}).Error; err != nil {
		t.Fatalf("insert pipeline operation log: %v", err)
	}
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("pipeline_log_id", "run-1").Error; err != nil {
		t.Fatalf("bind pipeline operation log: %v", err)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}

	NewIngestionTaskService().advanceOpenLog(t.Context(), task, logFromUnstart, string(entity.TaskStatusSchedule))
	var run entity.PipelineOperationLog
	if err := db.First(&run, "id = ?", "run-1").Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	if run.ProgressMsg == nil || *run.ProgressMsg != legacy {
		t.Fatalf("progress_msg = %v, want unchanged legacy value %q", run.ProgressMsg, legacy)
	}
}

func TestIngestionTaskServiceMarkStoppedTransitionsStoppingTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	// Override to STOPPING.
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("status", common.STOPPING).Error; err != nil {
		t.Fatalf("set task STOPPING: %v", err)
	}
	ctx := t.Context()

	svc := NewIngestionTaskService()
	if err := svc.MarkStopped(ctx, "task-1"); err != nil {
		t.Fatalf("MarkStopped failed: %v", err)
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("task status = %s, want STOPPED", task.Status)
	}
}

func TestIngestionTaskServiceMarkStoppedIdempotentOnAlreadyStopped(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("status", common.STOPPED).Error; err != nil {
		t.Fatalf("set task STOPPED: %v", err)
	}

	svc := NewIngestionTaskService()
	ctx := t.Context()
	if err := svc.MarkStopped(ctx, "task-1"); err != nil {
		t.Fatalf("MarkStopped on already STOPPED task should be idempotent, got: %v", err)
	}
}

// TestIngestionTaskServiceRequestStopNeverReopensTerminalTask locks the invariant
// that a terminal task (COMPLETED/STOPPED/FAILED) can never be moved back to
// STOPPING. A terminal task whose message was already acked must not regress to
// the in-flight STOPPING state (which has no settled worker and would be stuck
// forever). RequestStop is a no-op for terminal states.
func TestIngestionTaskServiceRequestStopNeverReopensTerminalTask(t *testing.T) {
	for _, status := range []string{common.COMPLETED, common.STOPPED, common.FAILED} {
		t.Run(status, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", status)
			ctx := t.Context()

			svc := NewIngestionTaskService()
			task, err := svc.RequestStop(ctx, "task-1")
			if err != nil {
				t.Fatalf("RequestStop on %s task should be a no-op, got: %v", status, err)
			}
			if task.Status != status {
				t.Fatalf("RequestStop moved %s task to %q, must stay %s", status, task.Status, status)
			}
		})
	}
}

// TestValidateTransitionRejectsTerminalToStopping locks the state-machine
// invariant directly: STOPPING is only reachable from RUNNING, never from a
// terminal state (COMPLETED/STOPPED/FAILED) or from STOPPING itself.
func TestValidateTransitionRejectsTerminalToStopping(t *testing.T) {
	for _, from := range []string{common.COMPLETED, common.STOPPED, common.FAILED, common.STOPPING, common.CREATED} {
		if err := validateTransition(from, common.STOPPING); err == nil {
			t.Errorf("validateTransition(%s -> STOPPING) = nil, want error", from)
		}
	}
	if err := validateTransition(common.RUNNING, common.STOPPING); err != nil {
		t.Errorf("validateTransition(RUNNING -> STOPPING) = %v, want nil", err)
	}
}

func TestIngestionTaskServiceMarkFailedIdempotentOnAlreadyTerminal(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("status", common.COMPLETED).Error; err != nil {
		t.Fatalf("set task COMPLETED: %v", err)
	}

	svc := NewIngestionTaskService()
	ctx := t.Context()
	if err := svc.MarkFailed(ctx, "task-1"); err != nil {
		t.Fatalf("MarkFailed on already COMPLETED task should be idempotent, got: %v", err)
	}
}

func TestIngestionTaskServiceMarkCompletedIdempotentOnAlreadyTerminal(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("status", common.FAILED).Error; err != nil {
		t.Fatalf("set task FAILED: %v", err)
	}

	ctx := t.Context()
	svc := NewIngestionTaskService()
	if err := svc.MarkCompleted(ctx, "task-1"); err != nil {
		t.Fatalf("MarkCompleted on already FAILED task should be idempotent, got: %v", err)
	}
}

func loadOpenPipelineLog(t *testing.T, ctx context.Context, db *gorm.DB, documentID string) *entity.PipelineOperationLog {
	t.Helper()
	var open entity.PipelineOperationLog
	if err := db.WithContext(ctx).
		Where("document_id = ? AND operation_status IN ?", documentID, dao.OpenPipelineOperationStatuses()).
		Order("create_time DESC").
		Order("id DESC").
		First(&open).Error; err != nil {
		t.Fatalf("expected open pipeline log for document %s", documentID)
	}
	return &open
}

func countPipelineLogs(t *testing.T, db *gorm.DB, documentID string) int {
	t.Helper()
	var count int64
	if err := db.Model(&entity.PipelineOperationLog{}).Where("document_id = ?", documentID).Count(&count).Error; err != nil {
		t.Fatalf("count pipeline logs: %v", err)
	}
	return int(count)
}

func TestIngestionTaskServiceCreateForDocumentsOpensPreTerminalPipelineLog(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	svc := NewIngestionTaskService()
	svc.taskPublisher = &recordingTaskPublisher{}
	ctx := t.Context()
	if _, err := svc.CreateForDocuments(ctx, "kb-1", "user-1", []string{"doc-1"}); err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}

	open := loadOpenPipelineLog(t, ctx, db, "doc-1")
	if open.OperationStatus != string(entity.TaskStatusSchedule) {
		t.Fatalf("OperationStatus = %q, want %q (scheduled write wins over created)", open.OperationStatus, string(entity.TaskStatusSchedule))
	}
	if open.KbID != "kb-1" || open.TenantID != "tenant-1" {
		t.Fatalf("unexpected kb/tenant scope: %+v", open)
	}
	if open.RunCount == nil || *open.RunCount != 1 {
		t.Fatalf("run_count = %v, want 1", open.RunCount)
	}
	if open.ProgressMsg != nil {
		t.Fatalf("ProgressMsg = %v, want legacy field untouched", open.ProgressMsg)
	}
	events, err := dao.NewIngestionTaskLogDAO().ListLogsByPipelineLogID(ctx, db, open.ID)
	if err != nil {
		t.Fatalf("list run events: %v", err)
	}
	var queuedMessage *entity.IngestionTaskLog
	for _, event := range events {
		if event.EventType == dao.EventTypeMessage && event.Message == "Task is queued..." {
			queuedMessage = event
			break
		}
	}
	if queuedMessage == nil {
		t.Fatalf("expected queued message event for run %s, events: %+v", open.ID, events)
	}
	if countPipelineLogs(t, db, "doc-1") != 1 {
		t.Fatalf("expected exactly 1 pipeline log row for one run")
	}
	// The task must be bound to the row so the terminal writer knows which row
	// belongs to this run.
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.PipelineLogID == nil || *task.PipelineLogID != open.ID {
		t.Fatalf("task PipelineLogID = %v, want %q (row bound to the owning run)", task.PipelineLogID, open.ID)
	}
}

// loadTaskPipelineLogID returns the pipeline-operation-log id bound to a task.
func loadTaskPipelineLogID(t *testing.T, ctx context.Context, db *gorm.DB, taskID string) string {
	t.Helper()
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, taskID)
	if err != nil {
		t.Fatalf("load task %s: %v", taskID, err)
	}
	if task.PipelineLogID == nil {
		t.Fatalf("task %s has no bound pipeline log", taskID)
	}
	return *task.PipelineLogID
}

func TestIngestionTaskServiceStartRunningAdvancesPreTerminalPipelineLog(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	svc := NewIngestionTaskService()
	svc.taskPublisher = &recordingTaskPublisher{}
	ctx := t.Context()
	resp, err := svc.CreateForDocuments(ctx, "kb-1", "user-1", []string{"doc-1"})
	if err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}
	if len(resp) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resp))
	}
	open := loadOpenPipelineLog(t, ctx, db, "doc-1")
	if open.OperationStatus != string(entity.TaskStatusSchedule) {
		t.Fatalf("OperationStatus after schedule = %q, want %q", open.OperationStatus, string(entity.TaskStatusSchedule))
	}
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	task, err = svc.TransitionTaskToRunning(ctx, task.ID)
	if err != nil {
		t.Fatalf("TransitionTaskToRunning failed: %v", err)
	}
	svc.PrepareValidatedRun(ctx, task)
	open = loadOpenPipelineLog(t, ctx, db, "doc-1")
	if open.OperationStatus != string(entity.TaskStatusRunning) {
		t.Fatalf("OperationStatus after start = %q, want %q", open.OperationStatus, string(entity.TaskStatusRunning))
	}
	if countPipelineLogs(t, db, "doc-1") != 1 {
		t.Fatalf("expected running to reuse the queued row, not insert a second one")
	}
	if bound := loadTaskPipelineLogID(t, ctx, db, task.ID); bound != open.ID {
		t.Fatalf("task bound to %q, want the same running row %q", bound, open.ID)
	}
}

func TestIngestionTaskServiceRequestStopBeforeRunClosesPreTerminalPipelineLog(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	svc := NewIngestionTaskService()
	svc.taskPublisher = &recordingTaskPublisher{}
	ctx := t.Context()
	if _, err := svc.CreateForDocuments(ctx, "kb-1", "user-1", []string{"doc-1"}); err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}
	loadOpenPipelineLog(t, ctx, db, "doc-1")
	task, err := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	stopped, err := svc.RequestStop(ctx, task.ID)
	if err != nil {
		t.Fatalf("RequestStop failed: %v", err)
	}
	if stopped.Status != common.STOPPED {
		t.Fatalf("status = %q, want %q", stopped.Status, common.STOPPED)
	}
	var done entity.PipelineOperationLog
	if err := db.Where("document_id = ?", "doc-1").First(&done).Error; err != nil {
		t.Fatalf("load pipeline log: %v", err)
	}
	if done.OperationStatus != string(entity.TaskStatusCancel) {
		t.Fatalf("OperationStatus = %q, want %q (stop without worker must close the row)", done.OperationStatus, string(entity.TaskStatusCancel))
	}
	if countPipelineLogs(t, db, "doc-1") != 1 {
		t.Fatalf("expected stop to reuse the queued row, not insert a second one")
	}
}

func TestIngestionTaskServiceRetryAfterTerminalOpensFreshPipelineLog(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("status", common.FAILED).Error; err != nil {
		t.Fatalf("set failed status: %v", err)
	}

	svc := NewIngestionTaskService()
	svc.taskPublisher = &recordingTaskPublisher{}
	ctx := t.Context()
	finished := &entity.PipelineOperationLog{
		ID:              "finished-log",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		TaskType:        "Parse",
		OperationStatus: string(entity.TaskStatusFail),
	}
	if err := dao.DB.Create(finished).Error; err != nil {
		t.Fatalf("seed finished log: %v", err)
	}
	if _, err := svc.CreateAndEnqueue(ctx, &entity.IngestionTask{
		DocumentID: "doc-1",
		UserID:     "user-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	}); err != nil {
		t.Fatalf("CreateAndEnqueue retry failed: %v", err)
	}
	open := loadOpenPipelineLog(t, ctx, db, "doc-1")
	if open.ID == "finished-log" {
		t.Fatalf("retry reused the finished row; want a fresh queued row")
	}
	if countPipelineLogs(t, db, "doc-1") != 2 {
		t.Fatalf("expected 2 pipeline log rows (finished + fresh queued)")
	}
}

func TestIngestionTaskServiceRetryAllocatesNextRunCount(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	svc := NewIngestionTaskService()
	svc.taskPublisher = &recordingTaskPublisher{}
	first, err := svc.CreateAndEnqueue(t.Context(), &entity.IngestionTask{
		DocumentID: "doc-1", UserID: "user-1", DatasetID: "kb-1", Status: common.CREATED,
	})
	if err != nil {
		t.Fatalf("create first run: %v", err)
	}
	if first.PipelineLogID == nil {
		t.Fatal("first run has no pipeline log id")
	}
	svc.settlePublishFailure(t.Context(), first)

	second, err := svc.CreateAndEnqueue(t.Context(), &entity.IngestionTask{
		DocumentID: "doc-1", UserID: "user-1", DatasetID: "kb-1", Status: common.CREATED,
	})
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if second.PipelineLogID == nil || *second.PipelineLogID == *first.PipelineLogID {
		t.Fatalf("second run id = %v, want a new run", second.PipelineLogID)
	}
	var secondLog entity.PipelineOperationLog
	if err := db.First(&secondLog, "id = ?", *second.PipelineLogID).Error; err != nil {
		t.Fatalf("load second run: %v", err)
	}
	if secondLog.RunCount == nil || *secondLog.RunCount != 2 {
		t.Fatalf("second run_count = %v, want 2", secondLog.RunCount)
	}
}

func TestIngestionTaskServiceReloadAndValidateRunIdentityRejectsInvalidBindings(t *testing.T) {
	validRunCount := 1
	invalidRunCount := 0
	testCases := []struct {
		name          string
		pipelineLogID *string
		createRun     bool
		runDocumentID string
		runDatasetID  string
		runCount      *int
		wantReason    string
	}{
		{name: "missing pipeline log ID", wantReason: "missing_pipeline_log_id"},
		{name: "pipeline log not found", pipelineLogID: sptr("missing-run"), wantReason: "pipeline_log_not_found"},
		{name: "pipeline log document mismatch", pipelineLogID: sptr("run-1"), createRun: true, runDocumentID: "other-doc", runDatasetID: "kb-1", runCount: &validRunCount, wantReason: "pipeline_log_document_mismatch"},
		{name: "pipeline log dataset mismatch", pipelineLogID: sptr("run-1"), createRun: true, runDocumentID: "doc-1", runDatasetID: "other-kb", runCount: &validRunCount, wantReason: "pipeline_log_dataset_mismatch"},
		{name: "missing run count", pipelineLogID: sptr("run-1"), createRun: true, runDocumentID: "doc-1", runDatasetID: "kb-1", wantReason: "invalid_run_count"},
		{name: "non-positive run count", pipelineLogID: sptr("run-1"), createRun: true, runDocumentID: "doc-1", runDatasetID: "kb-1", runCount: &invalidRunCount, wantReason: "invalid_run_count"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			db := setupServiceTestDB(t)
			pushServiceDB(t, db)
			insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
			insertTestDoc(t, "doc-1", "kb-1", 0, 0)
			insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

			if testCase.createRun {
				if err := db.Create(&entity.PipelineOperationLog{
					ID:              *testCase.pipelineLogID,
					DocumentID:      testCase.runDocumentID,
					KbID:            testCase.runDatasetID,
					TaskType:        string(entity.PipelineTaskTypeParse),
					OperationStatus: string(entity.TaskStatusRunning),
					RunCount:        testCase.runCount,
				}).Error; err != nil {
					t.Fatalf("create pipeline operation log: %v", err)
				}
			}
			if testCase.pipelineLogID != nil {
				if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("pipeline_log_id", *testCase.pipelineLogID).Error; err != nil {
					t.Fatalf("bind pipeline operation log: %v", err)
				}
			}

			_, err := NewIngestionTaskService().ReloadAndValidateRunIdentity(t.Context(), "task-1")
			var identityErr *InvalidRunIdentityError
			if !errors.As(err, &identityErr) || identityErr.Reason != testCase.wantReason {
				t.Fatalf("error = %v, want invalid run identity reason %q", err, testCase.wantReason)
			}
		})
	}
}

// TestIngestionTaskServiceOpensPreTerminalLogBeforePublish locks the ordering that
// closes the orphan window: the run's row must exist before its message is
// published, so a worker that claims and finishes the task immediately still
// finds a row to terminalize.
func TestIngestionTaskServiceOpensPreTerminalLogBeforePublish(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	var logIDAtPublish string
	publisher := &recordingTaskPublisher{
		beforeReturn: func(taskID string) {
			task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, taskID)
			if err != nil {
				t.Fatalf("load task at publish: %v", err)
			}
			if task.PipelineLogID == nil {
				t.Fatal("task was published before its open log row was bound")
			}
			logIDAtPublish = *task.PipelineLogID
		},
	}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	if _, err := svc.CreateForDocuments(t.Context(), "kb-1", "user-1", []string{"doc-1"}); err != nil {
		t.Fatalf("CreateForDocuments failed: %v", err)
	}
	if logIDAtPublish == "" {
		t.Fatal("publish did not observe a bound open log row")
	}
	open := loadOpenPipelineLog(t, t.Context(), db, "doc-1")
	if open.ID != logIDAtPublish {
		t.Fatalf("open row = %q, want the row bound before publish %q", open.ID, logIDAtPublish)
	}
}

// TestIngestionTaskServiceStartRunningClosingStoppingTaskClosesPreTerminalLog locks
// the workerless STOPPING finalize: MQ redelivery of a STOPPING task stops it
// without ever running a terminal pipeline-log writer, so StartRunning must
// close the row itself.
func TestIngestionTaskServiceStartRunningClosingStoppingTaskClosesPreTerminalLog(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.STOPPING)

	runningMsg := "Task is running..."
	if err := dao.DB.Create(&entity.PipelineOperationLog{
		ID:              "running-log",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		TaskType:        "Parse",
		OperationStatus: string(entity.TaskStatusRunning),
		ProgressMsg:     &runningMsg,
	}).Error; err != nil {
		t.Fatalf("seed running log: %v", err)
	}
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("pipeline_log_id", "running-log").Error; err != nil {
		t.Fatalf("bind running log: %v", err)
	}

	svc := NewIngestionTaskService()
	task, err := svc.TransitionTaskToRunning(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("StartRunning failed: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("status = %q, want %q", task.Status, common.STOPPED)
	}
	var done entity.PipelineOperationLog
	if err := db.First(&done, "id = ?", "running-log").Error; err != nil {
		t.Fatalf("load running log: %v", err)
	}
	if done.OperationStatus != string(entity.TaskStatusCancel) {
		t.Fatalf("OperationStatus = %q, want %q (workerless stop must close the row)", done.OperationStatus, string(entity.TaskStatusCancel))
	}
}

// TestIngestionTaskServiceKeepsBoundRowOverUnrelatedOpenRow locks the ownership
// rule: a run reuses the row it is bound to, never an unrelated open row that
// happens to exist for the same document.
func TestIngestionTaskServiceKeepsBoundRowOverUnrelatedOpenRow(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)

	queuedMsg := "Task is queued..."
	runCount := 1
	seed := func(id, status string, runCount *int) {
		t.Helper()
		if err := dao.DB.Create(&entity.PipelineOperationLog{
			ID:              id,
			DocumentID:      "doc-1",
			RunCount:        runCount,
			TenantID:        "tenant-1",
			KbID:            "kb-1",
			ParserID:        "naive",
			TaskType:        "Parse",
			OperationStatus: status,
			ProgressMsg:     &queuedMsg,
		}).Error; err != nil {
			t.Fatalf("seed %s: %v", id, err)
		}
	}
	seed("bound-log", string(entity.TaskStatusUnstart), &runCount)
	seed("unrelated-log", string(entity.TaskStatusSchedule), nil)
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").
		Update("pipeline_log_id", "bound-log").Error; err != nil {
		t.Fatalf("bind: %v", err)
	}

	svc := NewIngestionTaskService()
	svc.taskPublisher = &recordingTaskPublisher{}
	if _, err := svc.CreateAndEnqueue(t.Context(), &entity.IngestionTask{
		DocumentID: "doc-1",
		UserID:     "user-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	}); err != nil {
		t.Fatalf("CreateAndEnqueue failed: %v", err)
	}

	if got := countPipelineLogs(t, db, "doc-1"); got != 2 {
		t.Fatalf("pipeline log rows = %d, want 2 (bound row advanced, no third row created)", got)
	}
	if bound := loadTaskPipelineLogID(t, t.Context(), db, "task-1"); bound != "bound-log" {
		t.Fatalf("task bound to %q, want its own row bound-log", bound)
	}
	var bound entity.PipelineOperationLog
	if err := db.First(&bound, "id = ?", "bound-log").Error; err != nil {
		t.Fatalf("load bound log: %v", err)
	}
	if bound.OperationStatus != string(entity.TaskStatusSchedule) {
		t.Fatalf("bound row status = %q, want %q", bound.OperationStatus, string(entity.TaskStatusSchedule))
	}
}

func TestIngestionTaskServiceStaleSnapshotKeepsEstablishedOpenLogBinding(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)

	taskDAO := dao.NewIngestionTaskDAO()
	firstSnapshot, err := taskDAO.GetByID(t.Context(), db, "task-1")
	if err != nil {
		t.Fatalf("load first task snapshot: %v", err)
	}
	staleSnapshot, err := taskDAO.GetByID(t.Context(), db, "task-1")
	if err != nil {
		t.Fatalf("load stale task snapshot: %v", err)
	}

	svc := NewIngestionTaskService()
	if err := svc.ensureRunIdentity(t.Context(), firstSnapshot, nil); err != nil {
		t.Fatalf("establish first identity: %v", err)
	}
	firstLogID := loadTaskPipelineLogID(t, t.Context(), db, "task-1")
	if err := svc.ensureRunIdentity(t.Context(), staleSnapshot, nil); err != nil {
		t.Fatalf("reuse established identity: %v", err)
	}

	if got := loadTaskPipelineLogID(t, t.Context(), db, "task-1"); got != firstLogID {
		t.Fatalf("stale snapshot replaced binding %q with %q", firstLogID, got)
	}
	if got := countPipelineLogs(t, db, "doc-1"); got != 1 {
		t.Fatalf("pipeline log rows = %d, want the original single row", got)
	}
	var firstLog entity.PipelineOperationLog
	if err := db.First(&firstLog, "id = ?", firstLogID).Error; err != nil {
		t.Fatalf("established open log was removed: %v", err)
	}
}

// TestIngestionTaskServicePreTerminalLogAdvanceIsMonotonic locks the monotonic
// transition contract: a late queued write must not regress a row a concurrent
// worker already moved to running.
func TestIngestionTaskServicePreTerminalLogAdvanceIsMonotonic(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.CREATED)

	openLog := &entity.PipelineOperationLog{
		ID:              "log-1",
		DocumentID:      "doc-1",
		TenantID:        "tenant-1",
		KbID:            "kb-1",
		ParserID:        "naive",
		TaskType:        "Parse",
		OperationStatus: string(entity.TaskStatusUnstart),
	}
	if err := dao.DB.Create(openLog).Error; err != nil {
		t.Fatalf("seed open log: %v", err)
	}
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("pipeline_log_id", "log-1").Error; err != nil {
		t.Fatalf("bind task: %v", err)
	}

	svc := NewIngestionTaskService()
	ctx := t.Context()
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("load task: %v", err)
	}

	svc.advanceOpenLog(ctx, task, logFromUnstart, string(entity.TaskStatusSchedule))
	if got := loadOpenPipelineLog(t, ctx, db, "doc-1").OperationStatus; got != string(entity.TaskStatusSchedule) {
		t.Fatalf("after schedule = %q, want %q", got, string(entity.TaskStatusSchedule))
	}
	svc.advanceOpenLog(ctx, task, logFromUnstartOrScheduled, string(entity.TaskStatusRunning))
	if got := loadOpenPipelineLog(t, ctx, db, "doc-1").OperationStatus; got != string(entity.TaskStatusRunning) {
		t.Fatalf("after start = %q, want %q", got, string(entity.TaskStatusRunning))
	}
	// The enqueue-time scheduled write lands late; it must not regress the row.
	svc.advanceOpenLog(ctx, task, logFromUnstart, string(entity.TaskStatusSchedule))
	open := loadOpenPipelineLog(t, ctx, db, "doc-1")
	if open.OperationStatus != string(entity.TaskStatusRunning) {
		t.Fatalf("late scheduled write regressed the row to %q, want %q", open.OperationStatus, string(entity.TaskStatusRunning))
	}
	if open.ID != "log-1" || countPipelineLogs(t, db, "doc-1") != 1 {
		t.Fatalf("expected the same single row, got id=%q count=%d", open.ID, countPipelineLogs(t, db, "doc-1"))
	}
}

// TestIngestionTaskServiceAdvanceRejectsMissingIdentity ensures a status
// advance never creates a run identity. Only publish-time EnsureRunIdentity
// may bind a task to a pipeline-operation-log row.
func TestIngestionTaskServiceAdvanceRejectsMissingIdentity(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-terminal", "kb-1", 0, 0)
	insertTestDoc(t, "doc-live", "kb-1", 0, 0)
	insertTestIngestionTaskWithStatus(t, "task-terminal", "user-1", "doc-terminal", "kb-1", common.STOPPED)
	insertTestIngestionTaskWithStatus(t, "task-live", "user-1", "doc-live", "kb-1", common.CREATED)

	svc := NewIngestionTaskService()
	ctx := t.Context()

	terminal, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-terminal")
	if err != nil {
		t.Fatalf("load terminal task: %v", err)
	}
	svc.advanceOpenLog(ctx, terminal, logFromUnstart, string(entity.TaskStatusSchedule))
	if got := countPipelineLogs(t, db, "doc-terminal"); got != 0 {
		t.Fatalf("terminal task resurrected %d queued row(s); want none", got)
	}

	live, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-live")
	if err != nil {
		t.Fatalf("load live task: %v", err)
	}
	svc.advanceOpenLog(ctx, live, logFromUnstart, string(entity.TaskStatusSchedule))
	if got := countPipelineLogs(t, db, "doc-live"); got != 0 {
		t.Fatalf("missing identity created %d pipeline log rows, want 0", got)
	}
}
