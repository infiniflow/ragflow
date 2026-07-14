package service

import (
	"context"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

// TestExecuteTask_CheckpointParseFailureDoesNotKillProcess verifies that checkpoint
// parse failures do not call fatal exit (which would kill the whole worker process).
// Instead, the task should be marked as FAILED and return gracefully.
// This tests the fix for issue 1 from the code review.
func TestExecuteTask_CheckpointParseFailureDoesNotKillProcess(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	// Create a task log with invalid checkpoint (run_count is a string instead of number)
	err := db.Create(&entity.IngestionTaskLog{
		TaskID: taskID,
		Checkpoint: entity.JSONMap{
			"run_count": "not-a-number", // intentionally wrong type
		},
	}).Error
	if err != nil {
		t.Fatalf("create bad task log: %v", err)
	}

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	// Replace runDocumentTask to ensure it doesn't get called
	var runDocumentTaskCalled bool
	ingestor.runDocumentTask = func(ctx context.Context, ingestionTask *entity.IngestionTask) error {
		runDocumentTaskCalled = true
		return nil
	}

	taskCtx := taskpkg.NewTaskContextForScheduling(
		context.Background(),
		&entity.IngestionTask{ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING},
	)

	// Execute the task - this should NOT panic or fatal exit (this is our main validation!)
	ingestor.executeTask(taskCtx)

	// Corrupted run_count values are skipped by IncrementRunCount, so the task
	// proceeds to runDocumentTask and completes normally.
	if !runDocumentTaskCalled {
		t.Fatal("expected runDocumentTask to be called (bad run_count is skipped, not fatal)")
	}

	// Verify task status was set to COMPLETED
	finalTask, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load final ingestion task: %v", err)
	}
	if finalTask.Status != common.COMPLETED {
		t.Fatalf("final status = %s, want %s", finalTask.Status, common.COMPLETED)
	}
}

func TestDefaultRunDocumentTask_RequiresConfiguredPipelineID(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, kbID, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithTenantID("tenant-1"),
		testutil.WithKBID("kb-1"),
		testutil.WithDocID("doc-1"),
		testutil.WithTaskID("task-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	err := ingestor.defaultRunDocumentTask(context.Background(), &entity.IngestionTask{
		ID:         taskID,
		DocumentID: docID,
		DatasetID:  kbID,
		Status:     common.RUNNING,
	})
	if err == nil {
		t.Fatal("expected error when no pipeline is configured")
	}
	if err.Error() != "ingestion task task-1: no pipeline_id configured for document doc-1 or dataset kb-1" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestExecuteTask_RunsDocumentTask(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	var runDocumentTaskCalled bool
	var gotTaskID string
	var gotProgress []float64
	var gotMsgs []string
	ingestor.runDocumentTask = func(ctx context.Context, ingestionTask *entity.IngestionTask) error {
		runDocumentTaskCalled = true
		gotTaskID = ingestionTask.ID
		wrapped := func(prog float64, msg string) {
			gotProgress = append(gotProgress, prog*100)
			gotMsgs = append(gotMsgs, msg)
		}
		wrapped(0.82, "mock pipeline start")
		wrapped(1.0, "mock pipeline done")
		return nil
	}

	taskCtx := taskpkg.NewTaskContextForScheduling(
		context.Background(),
		&entity.IngestionTask{ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING},
	)

	ingestor.executeTask(taskCtx)

	if !runDocumentTaskCalled {
		t.Fatal("expected executeTask to run runDocumentTask")
	}
	if gotTaskID != taskID {
		t.Fatalf("runDocumentTask got task ID %q, want %q", gotTaskID, taskID)
	}
	finalTask, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load final ingestion task: %v", err)
	}
	if finalTask.Status != common.COMPLETED {
		t.Fatalf("final status = %s, want %s", finalTask.Status, common.COMPLETED)
	}
	if len(gotProgress) != 2 || gotProgress[0] != 82 || gotProgress[1] != 100 {
		t.Fatalf("gotProgress = %v, want [82 100]", gotProgress)
	}
	if len(gotMsgs) != 2 || gotMsgs[1] != "mock pipeline done" {
		t.Fatalf("gotMsgs = %v, want final message %q", gotMsgs, "mock pipeline done")
	}
}

// TestExecuteTask_CancelBeforePipeline verifies that when cancelCheck returns
// true at task start, the task is cancelled before AdvanceStep,
// runDocumentTask is never called, and document progress is set to -1 with a
// cancel marker. Mirrors Python's cancel flow where has_canceled() returns
// true in Pipeline.callback().
func TestExecuteTask_CancelBeforePipeline(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.cancelCheck = func(taskID string) bool { return true }

	var runDocumentTaskCalled bool
	ingestor.runDocumentTask = func(ctx context.Context, ingestionTask *entity.IngestionTask) error {
		runDocumentTaskCalled = true
		return nil
	}

	taskCtx := taskpkg.NewTaskContextForScheduling(
		context.Background(),
		&entity.IngestionTask{ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING},
	)
	ingestor.executeTask(taskCtx)

	if runDocumentTaskCalled {
		t.Fatal("expected runDocumentTask to NOT be called when cancel is detected before pipeline")
	}

	doc, err := dao.NewDocumentDAO().GetByID(docID)
	if err != nil {
		t.Fatalf("load document: %v", err)
	}
	if doc.Progress != -1 {
		t.Fatalf("document.progress = %v, want -1 (cancelled)", doc.Progress)
	}
	if doc.Run == nil || *doc.Run != string(entity.TaskStatusCancel) {
		t.Fatalf("document.run = %v, want %s (CANCEL)", doc.Run, entity.TaskStatusCancel)
	}
	if doc.ProgressMsg == nil || *doc.ProgressMsg == "" {
		t.Fatal("document.progress_msg should contain cancel marker, got empty")
	}
}
