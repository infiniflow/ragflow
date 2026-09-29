package service

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

// TestRunTask_ContextCancelledBeforePipeline makes a cancelled context settle
// the task as STOPPED without entering the pipeline.
func TestRunTask_ContextCancelledBeforeCheckpoint(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))
	runID := "run-" + taskID

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	var runDocCalled bool
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		runDocCalled = true
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	terminal := ingestor.runTask(ctx, &entity.IngestionTask{
		ID: taskID, DocumentID: "doc-1", DatasetID: "kb-1", PipelineLogID: &runID,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably recorded cancel) on cancelled ctx")
	}
	if runDocCalled {
		t.Fatal("expected runDocumentTask to be skipped on cancelled ctx")
	}
	testCtx := t.Context()
	// Cancellation is terminal for the bound run even when it happens before
	// pipeline execution begins.
	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByPipelineLogID(testCtx, db, "run-"+taskID)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 1 || logs[0].EventType != dao.EventTypeTerminal || logs[0].Message != "Task stopped by user." {
		t.Fatalf("terminal events = %+v, want one stopped event", logs)
	}
	// Task must be STOPPED, not left in RUNNING.
	task, err := dao.NewIngestionTaskDAO().GetByID(testCtx, db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("task status = %s, want STOPPED", task.Status)
	}
}

// TestRunTask_RunDocumentTaskFailureMarksFailed: when runDocumentTask errors,
// runTask marks the task FAILED and returns durably-written.
func TestRunTask_RunDocumentTaskFailureMarksFailed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return errors.New("boom")
	}

	runID := "run-" + taskID
	terminal := ingestor.runTask(t.Context(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
		PipelineLogID: &runID,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably marked FAILED)")
	}

	ctx := t.Context()
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("task status = %s, want FAILED", task.Status)
	}
	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByPipelineLogID(ctx, db, runID)
	if err != nil {
		t.Fatalf("list terminal events: %v", err)
	}
	if len(logs) != 1 || logs[0].EventType != dao.EventTypeTerminal || logs[0].Message == "" {
		t.Fatalf("terminal events = %+v, want one terminal event", logs)
	}
}

// TestRunTask_PipelineCancelledMarksStopped: when runDocumentTask returns
// context.Canceled (the pipeline detected a cancel signal), runTask treats
// it as a cancel, not a failure, and transitions the task to STOPPED.
func TestRunTask_PipelineCancelledMarksStopped(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	// Simulate RequestStop was already called: task in STOPPING.
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.STOPPING).Error; err != nil {
		t.Fatalf("set task STOPPING: %v", err)
	}

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return context.Canceled
	}

	terminal := ingestor.runTask(t.Context(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.STOPPING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably marked STOPPED)")
	}

	ctx := t.Context()
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("task status = %s, want STOPPED", task.Status)
	}
}

// TestRunTask_ComponentTimeoutMarksFailed: when runDocumentTask returns
// context.DeadlineExceeded (component Invoke hit its per-class timeout),
// runTask marks the task FAILED, not STOPPED. A component timeout is a
// system resource exhaustion, not a user-initiated cancellation.
func TestRunTask_ComponentTimeoutMarksFailed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return context.DeadlineExceeded
	}

	terminal := ingestor.runTask(t.Context(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably marked FAILED)")
	}

	ctx := t.Context()
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("task status = %s, want FAILED (DeadlineExceeded is a failure, not a cancel)", task.Status)
	}
}

// TestRunTask_AlreadyCompletedAcksNotRedelivers: when the pipeline succeeds but
// the task is already COMPLETED (e.g. another worker won a redelivery race),
// MarkCompleted's transition fails terminally. runTask must treat this as
// terminal and Ack - the work is done, redelivering would just ack-skip - and
// must NOT retry the deterministically-invalid transition.
func TestRunTask_AlreadyCompletedAcksNotRedelivers(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	// Pre-set the task COMPLETED so the RUNNING→COMPLETED transition inside
	// MarkCompleted fails.
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.COMPLETED).Error; err != nil {
		t.Fatalf("set task COMPLETED: %v", err)
	}

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return nil
	}

	terminal := ingestor.runTask(t.Context(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: task already COMPLETED, Ack instead of redeliver)")
	}

	// Task must still be COMPLETED (MarkCompleted failed to transition it).
	ctx := t.Context()
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.COMPLETED {
		t.Fatalf("task status = %s, want COMPLETED (unchanged)", task.Status)
	}
}

// TestRunTask_PipelineSucceedsConcurrentStopSettlesStopped: the pipeline
// finishes successfully, but a concurrent user stop (RequestStop) moved the
// task RUNNING->STOPPING just before MarkCompleted. The RUNNING->COMPLETED
// transition is now terminally invalid; runTask must settle the task to
// STOPPED and Ack (the pipeline already indexed the chunks) instead of
// retrying the invalid transition and Nacking for redelivery.
func TestRunTask_PipelineSucceedsConcurrentStopSettlesStopped(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, task *entity.IngestionTask) error {
		// Simulate the user pressing Stop mid-pipeline: RUNNING->STOPPING.
		if _, err := ingestor.ingestionTaskSvc.RequestStop(ctx, task.ID); err != nil {
			t.Fatalf("RequestStop: %v", err)
		}
		return nil // pipeline still finishes successfully
	}

	terminal := ingestor.runTask(t.Context(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: settled to STOPPED, Ack)")
	}

	ctx := t.Context()
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("task status = %s, want STOPPED (settled from concurrent STOPPING)", task.Status)
	}
}

// TestRunTask_SuccessfulCompletion: when everything succeeds, runTask returns
// true (terminal) and the task is COMPLETED.
func TestRunTask_SuccessfulCompletion(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return nil
	}

	terminal := ingestor.runTask(t.Context(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably completed)")
	}

	ctx := t.Context()
	task, err := dao.NewIngestionTaskDAO().GetByID(ctx, db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.COMPLETED {
		t.Fatalf("task status = %s, want COMPLETED", task.Status)
	}
}
