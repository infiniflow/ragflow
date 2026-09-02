package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

// TestExecuteTask_WatchdogDeadlineFailsStuckTask pins the "document stuck in
// RUNNING at 0% forever" fix: a pipeline that never returns on its own is
// aborted by the per-task watchdog deadline, the task settles as FAILED (not
// STOPPED, not RUNNING) so the broker message is Acked and the worker slot is
// freed, and the document row is moved to run=FAIL with a timeout marker.
func TestExecuteTask_WatchdogDeadlineFailsStuckTask(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.SetTaskTimeout(50 * time.Millisecond)
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		// Simulate a stuck pipeline that only wakes when the watchdog
		// aborts the task context, then surfaces the context error the
		// way pipeline components do (often wrapped as Canceled).
		<-ctx.Done()
		return context.Canceled
	}

	taskCtx := taskpkg.NewTaskContextForScheduling(
		t.Context(),
		&entity.IngestionTask{ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING},
	)

	done := make(chan struct{})
	go func() {
		defer close(done)
		ingestor.executeTask(t.Context(), taskCtx)
	}()
	select {
	case <-done:
	case <-time.After(15 * time.Second):
		t.Fatal("executeTask did not return after the watchdog deadline fired")
	}

	finalTask, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if finalTask.Status != common.FAILED {
		t.Fatalf("task status = %s, want FAILED (a watchdog abort must not leave the task RUNNING or mark it STOPPED)", finalTask.Status)
	}

	var doc entity.Document
	if err := db.Where("id = ?", docID).First(&doc).Error; err != nil {
		t.Fatalf("load document: %v", err)
	}
	if doc.Run == nil || *doc.Run != string(entity.TaskStatusFail) {
		t.Fatalf("document run = %v, want FAIL after watchdog timeout", doc.Run)
	}
	msg := ""
	if doc.ProgressMsg != nil {
		msg = *doc.ProgressMsg
	}
	if !strings.Contains(msg, "timed out") {
		t.Fatalf("progress_msg should carry the timeout marker, got %q", msg)
	}
}

// TestRunTask_WatchdogDeadlineBeforePipelineStart covers the deadline firing
// before the pipeline even starts: runTask must record a FAILED timeout, not
// a user-initiated STOPPED cancel.
func TestRunTask_WatchdogDeadlineBeforePipelineStart(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	var runDocCalled bool
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		runDocCalled = true
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	terminal := ingestor.runTask(ctx, &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably recorded timeout failure)")
	}
	if runDocCalled {
		t.Fatal("expected runDocumentTask to be skipped on an already-expired deadline")
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("task status = %s, want FAILED", task.Status)
	}
}

// TestRunTask_WatchdogDisabledKeepsCancelSemantics ensures that with the
// watchdog disabled (taskTimeout == 0) a user-cancelled task still settles as
// STOPPED exactly as before.
func TestRunTask_WatchdogDisabledKeepsCancelSemantics(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	if ingestor.taskTimeout != 0 {
		t.Fatalf("expected watchdog disabled by default in unit ingestor, got %v", ingestor.taskTimeout)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	terminal := ingestor.runTask(ctx, &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})

	if !terminal {
		t.Fatal("expected true (terminal: durably recorded cancel)")
	}

	task, err := dao.NewIngestionTaskDAO().GetByID(t.Context(), db, taskID)
	if err != nil {
		t.Fatalf("load task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("task status = %s, want STOPPED for a user cancel with the watchdog disabled", task.Status)
	}
}
