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

// TestRunTask_ContextCancelledBeforeCheckpoint: a cancelled context makes
// runTask return true (terminal: durably recorded cancel) immediately, without
// bumping the checkpoint or calling runDocumentTask. The task status is
// transitioned to STOPPED so it does not stay RUNNING forever.
func TestRunTask_ContextCancelledBeforeCheckpoint(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	var runDocCalled bool
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		runDocCalled = true
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	terminal := ingestor.runTask(ctx, &entity.IngestionTask{
		ID: taskID, DocumentID: "doc-1", DatasetID: "kb-1",
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably recorded cancel) on cancelled ctx")
	}
	if runDocCalled {
		t.Fatal("expected runDocumentTask to be skipped on cancelled ctx")
	}
	// Checkpoint must not have been bumped — no log row should exist.
	logs, err := dao.NewIngestionTaskLogDAO().ListLogsByTaskID(taskID)
	if err != nil {
		t.Fatalf("list logs: %v", err)
	}
	if len(logs) != 0 {
		t.Fatalf("expected 0 checkpoint rows (ctx cancelled before checkpoint), got %d", len(logs))
	}
	// Task must be STOPPED, not left in RUNNING.
	task, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("task status = %s, want STOPPED", task.Status)
	}
}

// TestRunTask_CheckpointFailureMarksFailed: a corrupted run_count value is
// skipped by IncrementRunCount (it scans past unparseable rows). The task
// proceeds normally and completes.
func TestRunTask_CorruptedRunCountSkipped(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	// Seed a bad checkpoint: run_count is a string, not a number.
	if err := db.Create(&entity.IngestionTaskLog{
		TaskID: taskID,
		Checkpoint: entity.JSONMap{
			"run_count": "not-a-number",
		},
	}).Error; err != nil {
		t.Fatalf("insert bad log: %v", err)
	}

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	var runDocCalled bool
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		runDocCalled = true
		return nil
	}

	terminal := ingestor.runTask(context.Background(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: completed despite corrupted run_count)")
	}
	if !runDocCalled {
		t.Fatal("expected runDocumentTask to be called (bad run_count is skipped, not fatal)")
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.COMPLETED {
		t.Fatalf("task status = %s, want COMPLETED", task.Status)
	}
}

// TestRunTask_RunDocumentTaskFailureMarksFailed: when runDocumentTask errors,
// runTask marks the task FAILED and returns durably-written.
func TestRunTask_RunDocumentTaskFailureMarksFailed(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return errors.New("boom")
	}

	terminal := ingestor.runTask(context.Background(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably marked FAILED)")
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("task status = %s, want FAILED", task.Status)
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

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return context.Canceled
	}

	terminal := ingestor.runTask(context.Background(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.STOPPING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably marked STOPPED)")
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(taskID)
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

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return context.DeadlineExceeded
	}

	terminal := ingestor.runTask(context.Background(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably marked FAILED)")
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("task status = %s, want FAILED (DeadlineExceeded is a failure, not a cancel)", task.Status)
	}
}

// TestRunTask_MarkCompletedFailure: when runDocumentTask succeeds but
// MarkCompleted fails (status conflict), runTask returns false (non-terminal)
// so the message is Nacked for retry.
func TestRunTask_MarkCompletedFailure(t *testing.T) {
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

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return nil
	}

	terminal := ingestor.runTask(context.Background(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if terminal {
		t.Fatal("expected false (non-terminal) on MarkCompleted failure")
	}

	// Task must still be COMPLETED (MarkCompleted failed to transition it).
	task, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.COMPLETED {
		t.Fatalf("task status = %s, want COMPLETED (unchanged)", task.Status)
	}
}

// TestRunTask_SuccessfulCompletion: when everything succeeds, runTask returns
// true (terminal) and the task is COMPLETED.
func TestRunTask_SuccessfulCompletion(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return nil
	}

	terminal := ingestor.runTask(context.Background(), &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably completed)")
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.COMPLETED {
		t.Fatalf("task status = %s, want COMPLETED", task.Status)
	}
}
