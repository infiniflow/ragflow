package service

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

// fakeTaskHandle records Ack/Nack/InProgress calls so tests can assert the worker
// acknowledges the MQ message at the right time without a live broker. The
// counters are atomic because executeTask drives them from multiple goroutines
// (the heartbeat goroutine calls InProgress; the defer calls Ack/Nack) while the
// test goroutine reads them - plain ints would be a data race under -race.
type fakeTaskHandle struct {
	msg        common.TaskMessage
	acks       atomic.Int64
	nacks      atomic.Int64
	inProgress atomic.Int64
}

func (f *fakeTaskHandle) GetMessage() common.TaskMessage { return f.msg }
func (f *fakeTaskHandle) Ack() error                     { f.acks.Add(1); return nil }
func (f *fakeTaskHandle) Nack() error                    { f.nacks.Add(1); return nil }
func (f *fakeTaskHandle) InProgress() error              { f.inProgress.Add(1); return nil }

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

	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack / 0 Nack on completion, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
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

	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack / 0 Nack on failure, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	finalTask, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load final task: %v", err)
	}
	if finalTask.Status != common.FAILED {
		t.Fatalf("final status = %s, want %s", finalTask.Status, common.FAILED)
	}
}

// TestExecuteTask_AcksMessageOnContextCancel: a task with a cancelled context
// (e.g. Redis cancel flag or Ingestor shutdown) is now terminal — the cancel
// is durably recorded (progress=-1, run=CANCEL) and the message is Acked to
// prevent indefinite redeliveries of an already-cancelled task.
func TestExecuteTask_AcksMessageOnContextCancel(t *testing.T) {
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
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack / 0 Nack on cancel, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
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

	// Poll for heartbeats with a generous deadline so the test is resilient
	// to slow CI schedulers. The ticker fires every heartbeatInterval (5ms);
	// the first tick may be delayed if the goroutine is not scheduled promptly.
	heartbeatDeadline := time.Now().Add(2 * time.Second)
	for handle.inProgress.Load() == 0 && time.Now().Before(heartbeatDeadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if handle.inProgress.Load() == 0 {
		t.Fatal("expected InProgress heartbeats while runDocumentTask was blocked, got 0")
	}

	close(proceed) // release the long task — only after confirming heartbeats

	// Poll for Ack completion (executeTask must finish MarkCompleted + Ack).
	deadline := time.Now().Add(2 * time.Second)
	for handle.acks.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if handle.acks.Load() != 1 {
		t.Fatalf("expected 1 Ack on completion after heartbeat, got acks=%d nacks=%d inProgress=%d",
			handle.acks.Load(), handle.nacks.Load(), handle.inProgress.Load())
	}
}

// TestClaimTask_FirstTrueThenFalse: claiming a task for the first time must
// succeed; a second claim while the first worker is still processing must
// fail. This is the local guard that catches MQ redeliveries.
func TestClaimTask_FirstTrueThenFalse(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})

	if !ingestor.claimTask("task-1") {
		t.Fatal("first claim should succeed")
	}

	// Same task claimed again — must fail (another worker is already on it).
	if ingestor.claimTask("task-1") {
		t.Fatal("second claim should fail: task already claimed by another worker")
	}

	// Different task — should succeed.
	if !ingestor.claimTask("task-2") {
		t.Fatal("different task should succeed")
	}
}

// TestClaimTask_AfterReleaseCanReclaim: after a worker finishes and releases
// the task, a fresh claim (e.g. on restart) must succeed again.
func TestClaimTask_AfterReleaseCanReclaim(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.claimTask("task-1")
	ingestor.releaseTask("task-1")

	if !ingestor.claimTask("task-1") {
		t.Fatal("after release should be able to reclaim (previous worker finished)")
	}
}

// TestExecuteTask_ReleasesTaskFromCurrentTasks: executeTask must call
// releaseTask in its defer so the task is removed from currentTasks and
// a subsequent claim succeeds.
func TestExecuteTask_ReleasesTaskFromCurrentTasks(t *testing.T) {
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
	ingestor.claimTask(taskID)

	handle := &fakeTaskHandle{}
	ingestor.executeTask(newAckTaskCtx(context.Background(), taskID, docID, handle))

	if _, stillActive := ingestor.currentTasks[taskID]; stillActive {
		t.Fatal("expected task released from currentTasks after executeTask finished")
	}
	// After release, a new claim must succeed.
	if !ingestor.claimTask(taskID) {
		t.Fatal("expected reclaim after executeTask to succeed")
	}
}

// TestSettleMessage_AckOnTerminal: body returns true -> Ack, no Nack.
func TestSettleMessage_AckOnTerminal(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	handle := &fakeTaskHandle{}
	taskCtx := newAckTaskCtx(context.Background(), "task-1", "doc-1", handle)

	ingestor.settleMessage(taskCtx, func(ctx context.Context) bool { return true })

	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("body=true: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestSettleMessage_NackOnNonTerminal: body returns false -> Nack, no Ack.
func TestSettleMessage_NackOnNonTerminal(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	handle := &fakeTaskHandle{}
	taskCtx := newAckTaskCtx(context.Background(), "task-1", "doc-1", handle)

	ingestor.settleMessage(taskCtx, func(ctx context.Context) bool { return false })

	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("body=false: expected 1 Nack/0 Ack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestSettleMessage_RecoversPanicAndAcksWhenTaskTerminal: if body panics,
// settleMessage must recover it (a single task's panic must not crash the
// worker process) and mark the task FAILED. With BP1 (DB truth), the DB
// showing FAILED makes the message terminal → Ack, avoiding an unnecessary
// redelivery. The panic must NOT propagate out of settleMessage.
func TestSettleMessage_RecoversPanicAndAcksWhenTaskTerminal(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	handle := &fakeTaskHandle{}
	taskCtx := newAckTaskCtx(context.Background(), taskID, docID, handle)

	panicked := false
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicked = true
			}
		}()
		ingestor.settleMessage(taskCtx, func(ctx context.Context) bool {
			panic("boom")
		})
	}()

	if panicked {
		t.Fatal("expected settleMessage to recover the panic, but it propagated out")
	}
	// BP1: markFailed succeeded → DB shows FAILED (terminal) → Ack.
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("body=panic: expected 1 Ack/0 Nack (markFailed→FAILED→DB terminal), got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	finalTask, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load final task: %v", err)
	}
	if finalTask.Status != common.FAILED {
		t.Fatalf("final status = %s, want FAILED (panic -> markFailed)", finalTask.Status)
	}
}

// TestAckOrNack_AckOnTerminal: terminal=true -> Ack called, Nack not called.
func TestAckOrNack_AckOnTerminal(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	handle := &fakeTaskHandle{}
	taskCtx := newAckTaskCtx(context.Background(), "task-1", "doc-1", handle)

	ingestor.ackOrNack(taskCtx, true)

	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("terminal=true: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestAckOrNack_NackOnNonTerminal: terminal=false -> Nack called, Ack not called.
func TestAckOrNack_NackOnNonTerminal(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	handle := &fakeTaskHandle{}
	taskCtx := newAckTaskCtx(context.Background(), "task-1", "doc-1", handle)

	ingestor.ackOrNack(taskCtx, false)

	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("terminal=false: expected 1 Nack/0 Ack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestAckOrNack_NoOpWhenNoHandle: nil handle -> no ack/nack, no panic.
func TestAckOrNack_NoOpWhenNoHandle(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})
	taskCtx := taskpkg.NewTaskContextForScheduling(
		context.Background(),
		&entity.IngestionTask{ID: "task-1", DocumentID: "doc-1", DatasetID: "kb-1", Status: common.RUNNING},
	)
	// taskCtx.Handle is nil
	// Must not panic
	ingestor.ackOrNack(taskCtx, true)
	ingestor.ackOrNack(taskCtx, false)
}

// TestSettleMessage_DBTruthOverridesBodyReturn: even when the body returns
// false (non-terminal), if the DB shows the task in a terminal state the
// message must still be Acked. This is the DB truth that BP1 establishes:
// settlement is authoritative (DB state), the in-memory bool is advisory.
// The classic case: a panic was recovered and markFailed succeeded, so the
// task is FAILED even though the body returned false / panicked.
func TestSettleMessage_DBTruthOverridesBodyReturn(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db,
		testutil.WithPipelineID("flow-1"),
		testutil.WithTenantID("tenant-1"),
	)

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	handle := &fakeTaskHandle{}
	taskCtx := newAckTaskCtx(context.Background(), taskID, docID, handle)

	// body returns false AND marks the task FAILED — simulating a panic
	// recovery where markFailed succeeded: the task is terminal (FAILED)
	// but the body signals non-terminal.
	body := func(ctx context.Context) bool {
		ingestor.markFailed(taskID)
		return false
	}

	ingestor.settleMessage(taskCtx, body)

	// DB shows FAILED → terminal → Ack, overriding body's false.
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack / 0 Nack (DB FAILED overrides body=false), got acks=%d nacks=%d",
			handle.acks.Load(), handle.nacks.Load())
	}
	finalTask, err := dao.NewIngestionTaskDAO().GetByID(taskID)
	if err != nil {
		t.Fatalf("load final task: %v", err)
	}
	if finalTask.Status != common.FAILED {
		t.Fatalf("final status = %s, want FAILED (body did markFailed)", finalTask.Status)
	}
}
