package service

import (
	"errors"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
)

type recordingTaskPublisher struct {
	subject  string
	messages []common.TaskMessage
	err      error
}

func (p *recordingTaskPublisher) PublishTaskMessage(subject string, msg common.TaskMessage) error {
	p.subject = subject
	p.messages = append(p.messages, msg)
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
	if task.Status != common.SCHEDULED {
		t.Fatalf("status = %q, want %q", task.Status, common.SCHEDULED)
	}
	doc, err := dao.NewDocumentDAO().GetByID(ctx, db, "doc-1")
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	if doc.Run == nil || *doc.Run != string(entity.TaskStatusSchedule) {
		t.Fatalf("document run = %v, want %q", doc.Run, entity.TaskStatusSchedule)
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
	if task.Status != common.SCHEDULED {
		t.Fatalf("status = %q, want %q", task.Status, common.SCHEDULED)
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
	if err := dao.DB.Create(&entity.IngestionTaskLog{
		TaskID:     "task-1",
		Checkpoint: entity.JSONMap{"run_count": 3},
	}).Error; err != nil {
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

func TestIngestionTaskServiceTryClaimStartsScheduledTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestKB(t, "kb-1", "tenant-1", 1, 0, 0)
	insertTestDoc(t, "doc-1", "kb-1", 100, 10)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.SCHEDULED)

	task, claimed, err := NewIngestionTaskService().TryClaim(t.Context(), "task-1", 15*time.Second)
	if err != nil {
		t.Fatalf("TryClaim: %v", err)
	}
	if !claimed {
		t.Fatal("scheduled task should be claimed")
	}
	if task.Status != common.RUNNING || task.ClaimToken == "" || task.ClaimExpiresAt <= time.Now().UnixMilli() {
		t.Fatalf("claimed task = %+v, want RUNNING with a live claim", task)
	}

	var doc entity.Document
	if err := db.Where("id = ?", "doc-1").First(&doc).Error; err != nil {
		t.Fatalf("reload document: %v", err)
	}
	if doc.Run == nil || *doc.Run != string(entity.TaskStatusRunning) {
		t.Fatalf("run = %v, want RUNNING(%q)", doc.Run, entity.TaskStatusRunning)
	}
	if doc.ChunkNum != 0 || doc.TokenNum != 0 || doc.Progress != 0 {
		t.Fatalf("document counters were not reset: %+v", doc)
	}
}

func TestIngestionTaskServiceFinalizeClaimCompletesCurrentOwner(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.RUNNING)
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Updates(map[string]interface{}{
		"claim_token":      "owner-a",
		"claim_expires_at": time.Now().Add(time.Minute).UnixMilli(),
	}).Error; err != nil {
		t.Fatalf("seed claim: %v", err)
	}

	finalized, err := NewIngestionTaskService().FinalizeClaim(t.Context(), "task-1", "owner-a", common.RUNNING, common.COMPLETED)
	if err != nil {
		t.Fatalf("FinalizeClaim: %v", err)
	}
	if !finalized {
		t.Fatal("current owner should finalize")
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, "task-1")
	if err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.COMPLETED || task.ClaimToken != "" || task.ClaimExpiresAt != 0 {
		t.Fatalf("final task = %+v, want completed task with cleared claim", task)
	}
}

func TestIngestionTaskServiceRequestStopTransitionsScheduledTaskToStopped(t *testing.T) {
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

func TestIngestionTaskServiceRequestStopTransitionsExplicitScheduledTaskToStopped(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTaskWithStatus(t, "task-1", "user-1", "doc-1", "kb-1", common.SCHEDULED)

	task, err := NewIngestionTaskService().RequestStop(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("RequestStop: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("status = %q, want %q", task.Status, common.STOPPED)
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

func TestIngestionTaskServiceCreateAndEnqueueRetriesTerminalTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	publisher := &recordingTaskPublisher{}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	testCases := []struct {
		name   string
		status string
	}{
		{name: "failed", status: common.FAILED},
		{name: "stopped", status: common.STOPPED},
	}

	ctx := t.Context()
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			publisher.subject = ""
			publisher.messages = nil
			if err := dao.DB.Where("id = ?", "task-1").Delete(&entity.IngestionTask{}).Error; err != nil {
				t.Fatalf("clear task: %v", err)
			}
			insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
			if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("status", tc.status).Error; err != nil {
				t.Fatalf("set terminal status: %v", err)
			}

			task, err := svc.CreateAndEnqueue(ctx, &entity.IngestionTask{
				DocumentID: "doc-1",
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

func TestIngestionTaskServiceCreateAndEnqueueRejectsActiveExistingTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
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

func TestIngestionTaskServiceCreateAndEnqueueKeepsNewTaskScheduledOnPublishFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	publisher := &recordingTaskPublisher{err: errors.New("publish failed")}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	ctx := t.Context()
	task, err := svc.CreateAndEnqueue(ctx, &entity.IngestionTask{
		DocumentID: "doc-1",
		UserID:     "user-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	})
	if err != nil {
		t.Fatalf("CreateAndEnqueue: %v", err)
	}
	task, getErr := dao.NewIngestionTaskDAO().GetByDocumentID(ctx, db, "doc-1")
	if getErr != nil {
		t.Fatalf("reload task by document id: %v", getErr)
	}
	if task == nil || task.Status != common.SCHEDULED {
		t.Fatalf("task = %+v, want retained SCHEDULED task", task)
	}
}

func TestIngestionTaskServiceCreateAndEnqueueKeepsRetriedTaskScheduledOnPublishFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("status", common.FAILED).Error; err != nil {
		t.Fatalf("set failed status: %v", err)
	}

	publisher := &recordingTaskPublisher{err: errors.New("publish failed")}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	ctx := t.Context()
	task, err := svc.CreateAndEnqueue(ctx, &entity.IngestionTask{
		DocumentID: "doc-1",
		UserID:     "user-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	})
	if err != nil {
		t.Fatalf("CreateAndEnqueue: %v", err)
	}
	if task.Status != common.SCHEDULED {
		t.Fatalf("task status = %q, want %q", task.Status, common.SCHEDULED)
	}
	reloaded, getErr := dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1")
	if getErr != nil {
		t.Fatalf("reload task: %v", getErr)
	}
	if reloaded.Status != common.SCHEDULED {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.SCHEDULED)
	}
}

func TestIngestionTaskServiceRemoveDeletesOwnedTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	ctx := t.Context()

	userID := "user-1"
	svc := NewIngestionTaskService()
	info, err := svc.Remove(ctx, "task-1", &userID)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if info == nil || info.TaskID != "task-1" {
		t.Fatalf("unexpected task info: %+v", info)
	}
	if _, err = dao.NewIngestionTaskDAO().GetByID(ctx, db, "task-1"); err == nil {
		t.Fatal("task should be removed")
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

func TestIngestionTaskServiceRecordComponentProgressAppendsRow(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	ctx := t.Context()

	svc := NewIngestionTaskService()
	if err := svc.RecordComponentProgress(ctx, "task-1", "Parser", 1, "Parser Done"); err != nil {
		t.Fatalf("RecordComponentProgress failed: %v", err)
	}
	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByTaskID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log row, got %d", len(logs))
	}
	row := logs[0]
	if row.Component != "Parser" || row.Phase != 1 || row.Message != "Parser Done" {
		t.Fatalf("unexpected log row: %+v", row)
	}
	if len(row.Checkpoint) != 0 {
		t.Fatalf("component progress row must have empty checkpoint, got %v", row.Checkpoint)
	}
}

func TestIngestionTaskServiceAggregateTaskProgressClassifiesByPhase(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	ctx := t.Context()

	svc := NewIngestionTaskService()
	if err := svc.RecordComponentProgress(ctx, "task-1", "Parser", 1, "Parser Done"); err != nil {
		t.Fatalf("record Parser: %v", err)
	}
	if err := svc.RecordComponentProgress(ctx, "task-1", "Chunker", 0, "Chunker Started"); err != nil {
		t.Fatalf("record Chunker: %v", err)
	}
	agg, err := svc.AggregateTaskProgress(ctx, "task-1", 2)
	if err != nil {
		t.Fatalf("AggregateTaskProgress failed: %v", err)
	}
	if agg.Done != 1 || agg.Running != 1 || agg.Failed != 0 {
		t.Fatalf("aggregate = %+v, want Done=1 Running=1 Failed=0", agg)
	}
	if agg.Percent != 50 {
		t.Fatalf("percent = %v, want 50", agg.Percent)
	}
}

func TestIngestionTaskServiceIncrementRunCountInitializesAndBumps(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	ctx := t.Context()

	svc := NewIngestionTaskService()
	if err := svc.IncrementRunCount(ctx, "task-1"); err != nil {
		t.Fatalf("IncrementRunCount (first call) failed: %v", err)
	}
	run, ok := svc.lastRunCount(ctx, "task-1")
	if !ok || run != 1 {
		t.Fatalf("run_count = %v (ok=%v), want 1", run, ok)
	}

	// Second call bumps the existing counter to 2.
	if err := svc.IncrementRunCount(ctx, "task-1"); err != nil {
		t.Fatalf("IncrementRunCount (second call) failed: %v", err)
	}
	run, _ = svc.lastRunCount(ctx, "task-1")
	if run != 2 {
		t.Fatalf("run_count after second bump = %v, want 2", run)
	}
}

func TestIngestionTaskServiceIncrementRunCountSkippedCorruptedRunCount(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Create(&entity.IngestionTaskLog{
		TaskID:     "task-1",
		Checkpoint: entity.JSONMap{"run_count": "not-a-number"},
	}).Error; err != nil {
		t.Fatalf("insert bad task log: %v", err)
	}
	ctx := t.Context()

	svc := NewIngestionTaskService()
	// Corrupted value is skipped; a fresh run_count=1 row is created.
	if err := svc.IncrementRunCount(ctx, "task-1"); err != nil {
		t.Fatalf("IncrementRunCount should skip corrupted value, got: %v", err)
	}
	run, ok := svc.lastRunCount(ctx, "task-1")
	if !ok || run != 1 {
		t.Fatalf("run_count = %v (ok=%v), want 1", run, ok)
	}
}

func TestIngestionTaskServiceIncrementRunCountRecoversFromComponentProgressLog(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	ctx := t.Context()

	// Simulate a previous run that created some component-progress logs
	// but died before recording a run_count row. The latest log has no run_count.
	svc := NewIngestionTaskService()
	if err := svc.RecordComponentProgress(ctx, "task-1", "Parser", 1, "Parser Done"); err != nil {
		t.Fatalf("record Parser: %v", err)
	}
	if err := svc.RecordComponentProgress(ctx, "task-1", "Chunker", 1, "Chunker Done"); err != nil {
		t.Fatalf("record Chunker: %v", err)
	}
	// Verify latest log has empty checkpoint (no run_count).
	latest, err := dao.NewIngestionTaskLogDAO().LatestLogByTaskID(ctx, db, "task-1")
	if err != nil {
		t.Fatalf("load latest: %v", err)
	}
	if len(latest.Checkpoint) != 0 {
		t.Fatalf("component-progress row should have empty checkpoint, got %v", latest.Checkpoint)
	}

	// IncrementRunCount should create a new row with run_count=1.
	if err = svc.IncrementRunCount(ctx, "task-1"); err != nil {
		t.Fatalf("IncrementRunCount failed: %v", err)
	}
	run, ok := svc.lastRunCount(ctx, "task-1")
	if !ok || run != 1 {
		t.Fatalf("run_count = %v (ok=%v), want 1", run, ok)
	}

	// AggregateProgress should still work (run_count row with component=""
	// has phase=0, which doesn't affect counts).
	agg, err := svc.AggregateTaskProgress(ctx, "task-1", 2)
	if err != nil {
		t.Fatalf("AggregateTaskProgress: %v", err)
	}
	if agg.Done != 2 {
		t.Fatalf("Done = %d, want 2 (run_count row didn't interfere)", agg.Done)
	}
}

func TestIngestionTaskServiceIncrementRunCountAccumulatesAcrossRetries(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	svc := NewIngestionTaskService()
	ctx := t.Context()

	// First attempt: IncrementRunCount creates run_count=1.
	if err := svc.IncrementRunCount(ctx, "task-1"); err != nil {
		t.Fatalf("first IncrementRunCount: %v", err)
	}
	// Simulate first run: some components progress, then failure.
	if err := svc.RecordComponentProgress(ctx, "task-1", "Parser", 1, "Parser Done"); err != nil {
		t.Fatalf("record Parser: %v", err)
	}

	// Second attempt (retry): should find previous run_count=1 and create row with run_count=2.
	if err := svc.IncrementRunCount(ctx, "task-1"); err != nil {
		t.Fatalf("second IncrementRunCount: %v", err)
	}
	// More progress, then failure.
	if err := svc.RecordComponentProgress(ctx, "task-1", "Chunker", 1, "Chunker Done"); err != nil {
		t.Fatalf("record Chunker: %v", err)
	}

	// Third attempt (retry): should find previous run_count=2 and create row with run_count=3.
	if err := svc.IncrementRunCount(ctx, "task-1"); err != nil {
		t.Fatalf("third IncrementRunCount: %v", err)
	}

	run, ok := svc.lastRunCount(ctx, "task-1")
	if !ok || run != 3 {
		t.Fatalf("run_count = %v (ok=%v), want 3", run, ok)
	}

	// ListAllForAdmin should still pick up the correct run_count.
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
	adminTasks, err := svc.ListAllForAdmin(ctx)
	if err != nil {
		t.Fatalf("ListAllForAdmin: %v", err)
	}
	if len(adminTasks) != 1 || adminTasks[0]["id"] != "task-1" {
		t.Fatalf("ListAllForAdmin = %+v, want single task task-1", adminTasks)
	}
	if adminTasks[0]["run_count"] != 3 {
		t.Fatalf("ListAllForAdmin run_count = %v, want 3", adminTasks[0]["run_count"])
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
