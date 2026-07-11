package service

import (
	"errors"
	"testing"

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

	resp, err := svc.CreateForDocuments("kb-1", "user-1", []string{"doc-1"})
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
	task, err := dao.NewIngestionTaskDAO().GetByID(msg.TaskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.DocumentID != "doc-1" || task.DatasetID != "kb-1" || task.UserID != "user-1" {
		t.Fatalf("unexpected task: %+v", task)
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
	tasks, err := svc.ListByUser("user-1", &datasetID, 0, 0)
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

	userID := "user-1"
	svc := NewIngestionTaskService()
	tasks, err := svc.RequestStopMany([]string{"task-1"}, &userID)
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

	userID := "user-2"
	svc := NewIngestionTaskService()
	if _, err := svc.RequestStopMany([]string{"task-1"}, &userID); err == nil {
		t.Fatal("expected RequestStopMany to reject non-owner")
	}
	task, err := dao.NewIngestionTaskDAO().GetByID("task-1")
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

	svc := NewIngestionTaskService()
	tasks, err := svc.RequestStopMany([]string{"task-1"}, nil)
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

	userID := "user-1"
	svc := NewIngestionTaskService()
	result, err := svc.RemoveMany([]string{"task-1"}, &userID)
	if err != nil {
		t.Fatalf("RemoveMany failed: %v", err)
	}
	if len(result) != 1 || result[0]["remove"] != "success" {
		t.Fatalf("unexpected remove result: %+v", result)
	}
	if _, err := dao.NewIngestionTaskDAO().GetByID("task-1"); err == nil {
		t.Fatal("task should be removed")
	}
}

func TestIngestionTaskServiceListAllForAdminIncludesStepAndUserEmail(t *testing.T) {
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
		Checkpoint: entity.JSONMap{"current_step": 3},
	}).Error; err != nil {
		t.Fatalf("insert task log: %v", err)
	}

	svc := NewIngestionTaskService()
	tasks, err := svc.ListAllForAdmin()
	if err != nil {
		t.Fatalf("ListAllForAdmin failed: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task, got %d", len(tasks))
	}
	if tasks[0]["user"] != "user-1@test.com" {
		t.Fatalf("user = %v, want user-1@test.com", tasks[0]["user"])
	}
	if tasks[0]["step"] != 3 {
		t.Fatalf("step = %v, want 3", tasks[0]["step"])
	}
}

func TestIngestionTaskServiceStartRunningTransitionsCreatedTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	svc := NewIngestionTaskService()
	task, err := svc.StartRunning("task-1")
	if err != nil {
		t.Fatalf("StartRunning failed: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("status = %q, want %q", task.Status, common.RUNNING)
	}
}

func TestIngestionTaskServiceRequestStopTransitionsCreatedTaskToStopped(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	svc := NewIngestionTaskService()
	task, err := svc.RequestStop("task-1")
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

	svc := NewIngestionTaskService()
	if err := svc.MarkCompleted("task-1"); err == nil {
		t.Fatal("expected MarkCompleted to reject non-running task")
	}
	task, err := dao.NewIngestionTaskDAO().GetByID("task-1")
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

	svc := NewIngestionTaskService()
	if err := svc.MarkCompleted("task-1"); err != nil {
		t.Fatalf("MarkCompleted failed: %v", err)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID("task-1")
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

	svc := NewIngestionTaskService()
	if err := svc.MarkFailed("task-1"); err != nil {
		t.Fatalf("MarkFailed failed: %v", err)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID("task-1")
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

	svc := NewIngestionTaskService()
	err := svc.newTaskStatusConflictError("task-1", common.CREATED, common.RUNNING)
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

	svc := NewIngestionTaskService()
	err := svc.MarkCompleted("task-1")
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

			task, err := svc.CreateAndEnqueue(&entity.IngestionTask{
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
			if task.Status != common.CREATED {
				t.Fatalf("status = %q, want %q", task.Status, common.CREATED)
			}
			if len(publisher.messages) != 1 || publisher.messages[0].TaskID != "task-1" {
				t.Fatalf("unexpected published messages: %+v", publisher.messages)
			}
			reloaded, err := dao.NewIngestionTaskDAO().GetByID("task-1")
			if err != nil {
				t.Fatalf("reload task: %v", err)
			}
			if reloaded.Status != common.CREATED {
				t.Fatalf("reloaded status = %q, want %q", reloaded.Status, common.CREATED)
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

	_, err := svc.CreateAndEnqueue(&entity.IngestionTask{DocumentID: "doc-1", UserID: "user-1", DatasetID: "kb-1", Status: common.CREATED})
	if err == nil {
		t.Fatal("expected CreateAndEnqueue to reject existing created task")
	}
	if len(publisher.messages) != 0 {
		t.Fatalf("expected no published messages, got %+v", publisher.messages)
	}
}

func TestIngestionTaskServiceCreateAndEnqueueRollsBackNewTaskOnPublishFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	publisher := &recordingTaskPublisher{err: errors.New("publish failed")}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	_, err := svc.CreateAndEnqueue(&entity.IngestionTask{
		DocumentID: "doc-1",
		UserID:     "user-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	})
	if err == nil || err.Error() != "publish failed" {
		t.Fatalf("expected publish failure, got %v", err)
	}
	task, getErr := dao.NewIngestionTaskDAO().GetByDocumentID("doc-1")
	if getErr != nil {
		t.Fatalf("reload task by document id: %v", getErr)
	}
	if task != nil {
		t.Fatalf("expected created task to be deleted after publish failure, got %+v", task)
	}
}

func TestIngestionTaskServiceCreateAndEnqueueRollsBackRetriedTaskOnPublishFailure(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Model(&entity.IngestionTask{}).Where("id = ?", "task-1").Update("status", common.FAILED).Error; err != nil {
		t.Fatalf("set failed status: %v", err)
	}

	publisher := &recordingTaskPublisher{err: errors.New("publish failed")}
	svc := NewIngestionTaskService()
	svc.taskPublisher = publisher

	_, err := svc.CreateAndEnqueue(&entity.IngestionTask{
		DocumentID: "doc-1",
		UserID:     "user-1",
		DatasetID:  "kb-1",
		Status:     common.CREATED,
	})
	if err == nil || err.Error() != "publish failed" {
		t.Fatalf("expected publish failure, got %v", err)
	}
	reloaded, getErr := dao.NewIngestionTaskDAO().GetByID("task-1")
	if getErr != nil {
		t.Fatalf("reload task: %v", getErr)
	}
	if reloaded.Status != common.FAILED {
		t.Fatalf("status = %q, want %q", reloaded.Status, common.FAILED)
	}
}

func TestIngestionTaskServiceRemoveDeletesOwnedTask(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	userID := "user-1"
	svc := NewIngestionTaskService()
	info, err := svc.Remove("task-1", &userID)
	if err != nil {
		t.Fatalf("Remove failed: %v", err)
	}
	if info == nil || info.TaskID != "task-1" {
		t.Fatalf("unexpected task info: %+v", info)
	}
	if _, err := dao.NewIngestionTaskDAO().GetByID("task-1"); err == nil {
		t.Fatal("task should be removed")
	}
}

func TestIngestionTaskServiceUpdateComponentTotalPersistsDenominator(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	svc := NewIngestionTaskService()
	if err := svc.UpdateComponentTotal("task-1", 4); err != nil {
		t.Fatalf("UpdateComponentTotal failed: %v", err)
	}
	task, err := dao.NewIngestionTaskDAO().GetByID("task-1")
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

	svc := NewIngestionTaskService()
	if err := svc.RecordComponentProgress("task-1", "Parser", 0, 1, "Parser Done"); err != nil {
		t.Fatalf("RecordComponentProgress failed: %v", err)
	}
	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByTaskID("task-1")
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("expected 1 log row, got %d", len(logs))
	}
	row := logs[0]
	if row.Component != "Parser" || row.ComponentIndex != 0 || row.Phase != 1 || row.Message != "Parser Done" {
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

	svc := NewIngestionTaskService()
	if err := svc.RecordComponentProgress("task-1", "Parser", 0, 1, "Parser Done"); err != nil {
		t.Fatalf("record Parser: %v", err)
	}
	if err := svc.RecordComponentProgress("task-1", "Chunker", 1, 0, "Chunker Started"); err != nil {
		t.Fatalf("record Chunker: %v", err)
	}
	agg, err := svc.AggregateTaskProgress("task-1", 2)
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

func TestIngestionTaskServiceAdvanceStepCheckpointInitializesAndBumps(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")

	svc := NewIngestionTaskService()
	if err := svc.AdvanceStepCheckpoint("task-1"); err != nil {
		t.Fatalf("AdvanceStepCheckpoint (first call) failed: %v", err)
	}
	latest, err := dao.NewIngestionTaskLogDAO().LatestLogByTaskID("task-1")
	if err != nil {
		t.Fatalf("load latest log: %v", err)
	}
	step, ok := common.GetInt(latest.Checkpoint[checkpointKeyCurrentStep])
	if !ok || step != 2 {
		t.Fatalf("current_step = %v (ok=%v), want 2", latest.Checkpoint[checkpointKeyCurrentStep], ok)
	}
	total, ok := common.GetInt(latest.Checkpoint[checkpointKeyTotalStep])
	if !ok || total != 5 {
		t.Fatalf("total_step = %v (ok=%v), want 5", latest.Checkpoint[checkpointKeyTotalStep], ok)
	}

	// Second call bumps the existing row to 3.
	if err := svc.AdvanceStepCheckpoint("task-1"); err != nil {
		t.Fatalf("AdvanceStepCheckpoint (second call) failed: %v", err)
	}
	latest, err = dao.NewIngestionTaskLogDAO().LatestLogByTaskID("task-1")
	if err != nil {
		t.Fatalf("reload latest log: %v", err)
	}
	step, _ = common.GetInt(latest.Checkpoint[checkpointKeyCurrentStep])
	if step != 3 {
		t.Fatalf("current_step after second bump = %v, want 3", step)
	}
}

func TestIngestionTaskServiceAdvanceStepCheckpointParseFailureReturnsError(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestIngestionTask(t, "task-1", "user-1", "doc-1", "kb-1")
	if err := dao.DB.Create(&entity.IngestionTaskLog{
		TaskID:     "task-1",
		Checkpoint: entity.JSONMap{"current_step": "not-a-number", "total_step": 5},
	}).Error; err != nil {
		t.Fatalf("insert bad task log: %v", err)
	}

	svc := NewIngestionTaskService()
	if err := svc.AdvanceStepCheckpoint("task-1"); err == nil {
		t.Fatal("expected error when current_step cannot be parsed, got nil")
	}
}

func TestDocumentServiceUpdateRunProgressMirrorsFields(t *testing.T) {
	db := setupServiceTestDB(t)
	pushServiceDB(t, db)
	insertTestDoc(t, "doc-1", "kb-1", 0, 0)

	svc := testDocumentService(t)
	if err := svc.UpdateRunProgress("doc-1", 0.5, "1", "halfway"); err != nil {
		t.Fatalf("UpdateRunProgress failed: %v", err)
	}
	doc, err := dao.NewDocumentDAO().GetByID("doc-1")
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	if doc.Progress != 0.5 {
		t.Fatalf("progress = %v, want 0.5", doc.Progress)
	}
	if doc.Run == nil || *doc.Run != "1" {
		t.Fatalf("run = %v, want 1", doc.Run)
	}
	if doc.ProgressMsg == nil || *doc.ProgressMsg != "halfway" {
		t.Fatalf("progress_msg = %v, want halfway", doc.ProgressMsg)
	}
}
