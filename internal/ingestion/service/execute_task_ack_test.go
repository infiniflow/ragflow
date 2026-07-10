package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

// fakeTaskHandle records Ack/Nack/InProgress calls so tests can assert the worker
// acknowledges the MQ message at the right time without a live broker.
type fakeTaskHandle struct {
	msg        common.TaskMessage
	acks       int
	nacks      int
	inProgress int
}

func (f *fakeTaskHandle) GetMessage() common.TaskMessage { return f.msg }
func (f *fakeTaskHandle) Ack() error                     { f.acks++; return nil }
func (f *fakeTaskHandle) Nack() error                    { f.nacks++; return nil }
func (f *fakeTaskHandle) InProgress() error              { f.inProgress++; return nil }

func newAckTaskCtx(ctx context.Context, taskID, docID string, handle *fakeTaskHandle) *taskpkg.TaskContext {
	taskCtx := taskpkg.NewTaskContextForScheduling(
		ctx,
		&entity.IngestionTask{ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING},
	)
	taskCtx.Handle = handle
	return taskCtx
}

// TestExecuteTask_AcksMessageOnCompletion: a successfully completed task must
// Ack its MQ message so consumer-group queues do not redeliver (and double-
// execute) it. Regression test for the missing ack on the success path.
func TestExecuteTask_AcksMessageOnCompletion(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return nil
	}

	handle := &fakeTaskHandle{}
	ingestor.executeTask(newAckTaskCtx(context.Background(), taskID, docID, handle))

	if handle.acks != 1 || handle.nacks != 0 {
		t.Fatalf("expected 1 Ack / 0 Nack on completion, got acks=%d nacks=%d", handle.acks, handle.nacks)
	}
	finalTask, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load final task: %v", err)
	}
	if finalTask.Status != common.COMPLETED {
		t.Fatalf("final status = %s, want %s", finalTask.Status, common.COMPLETED)
	}
}

// TestExecuteTask_AcksMessageOnFailure: a task whose runDocumentTask fails is
// marked FAILED (a terminal status) and must still be Acked - redelivery would
// only re-find it FAILED and skip.
func TestExecuteTask_AcksMessageOnFailure(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		return errors.New("boom")
	}

	handle := &fakeTaskHandle{}
	ingestor.executeTask(newAckTaskCtx(context.Background(), taskID, docID, handle))

	if handle.acks != 1 || handle.nacks != 0 {
		t.Fatalf("expected 1 Ack / 0 Nack on failure, got acks=%d nacks=%d", handle.acks, handle.nacks)
	}
	finalTask, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load final task: %v", err)
	}
	if finalTask.Status != common.FAILED {
		t.Fatalf("final status = %s, want %s", finalTask.Status, common.FAILED)
	}
}

// TestExecuteTask_NacksMessageOnContextCancel: a task cancelled mid-flight
// (e.g. shutdown) is left non-terminal and must be Nacked so it is redelivered
// and resumed after restart, rather than silently dropped.
func TestExecuteTask_NacksMessageOnContextCancel(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	var runCalled bool
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		runCalled = true
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	handle := &fakeTaskHandle{}
	ingestor.executeTask(newAckTaskCtx(ctx, taskID, docID, handle))

	if runCalled {
		t.Fatal("expected runDocumentTask to be skipped on cancelled ctx")
	}
	if handle.nacks != 1 || handle.acks != 0 {
		t.Fatalf("expected 1 Nack / 0 Ack on cancel, got acks=%d nacks=%d", handle.acks, handle.nacks)
	}
}

// TestExecuteTask_HeartbeatsInProgressDuringLongTask: a long-running task
// must call InProgress periodically while processing so the broker does not
// redeliver the unacked message mid-task (AckWait timer stays fresh).
// Regression test for in-flight double execution of slow tasks.
func TestExecuteTask_HeartbeatsInProgressDuringLongTask(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.heartbeatInterval = 5 * time.Millisecond

	started := make(chan struct{})
	proceed := make(chan struct{})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		close(started) // heartbeat goroutine is running by now
		<-proceed      // simulate a long task
		return nil
	}

	handle := &fakeTaskHandle{}
	go ingestor.executeTask(newAckTaskCtx(context.Background(), taskID, docID, handle))

	<-started
	time.Sleep(30 * time.Millisecond) // let the ticker fire a few times
	close(proceed)                    // release the long task

	if handle.inProgress == 0 {
		t.Fatal("expected InProgress heartbeats while runDocumentTask was blocked, got 0")
	}

	// Poll for Ack completion (executeTask must finish MarkCompleted + Ack).
	deadline := time.Now().Add(2 * time.Second)
	for handle.acks == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if handle.acks != 1 {
		t.Fatalf("expected 1 Ack on completion after heartbeat, got acks=%d nacks=%d inProgress=%d",
			handle.acks, handle.nacks, handle.inProgress)
	}
}
