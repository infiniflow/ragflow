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

	// Create a task log with invalid checkpoint (current_step is a string instead of number)
	err := db.Create(&entity.IngestionTaskLog{
		TaskID: taskID,
		Checkpoint: entity.JSONMap{
			"current_step": "not-a-number", // intentionally wrong type
			"total_step":   5,
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

	// Verify runDocumentTask was not called (we returned early due to checkpoint parse failure)
	if runDocumentTaskCalled {
		t.Fatal("expected runDocumentTask to not be called due to checkpoint parse failure")
	}

	// Verify task status was set to FAILED
	finalTask, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load final ingestion task: %v", err)
	}
	if finalTask.Status != common.FAILED {
		t.Fatalf("final status = %s, want %s", finalTask.Status, common.FAILED)
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

func TestExecuteTask_DataflowRoutesToTaskHandler(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	var routedToDataflow bool
	var gotTaskID string
	var gotProgress []float64
	var gotMsgs []string
	ingestor.runDocumentTask = func(ctx context.Context, ingestionTask *entity.IngestionTask) error {
		routedToDataflow = true
		gotTaskID = ingestionTask.ID
		wrapped := func(prog float64, msg string) {
			gotProgress = append(gotProgress, prog*100)
			gotMsgs = append(gotMsgs, msg)
		}
		wrapped(0.82, "mock dataflow start")
		wrapped(1.0, "mock dataflow done")
		return nil
	}

	taskCtx := taskpkg.NewTaskContextForScheduling(
		context.Background(),
		&entity.IngestionTask{ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING},
	)

	ingestor.executeTask(taskCtx)

	if !routedToDataflow {
		t.Fatal("expected executeTask to route dataflow task to runDocumentTask")
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
	if len(gotMsgs) != 2 || gotMsgs[1] != "mock dataflow done" {
		t.Fatalf("gotMsgs = %v, want final message %q", gotMsgs, "mock dataflow done")
	}
}
