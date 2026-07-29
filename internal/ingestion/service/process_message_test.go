package service

import (
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

func newFakeHandle(taskID, taskType string) *fakeTaskHandle {
	return &fakeTaskHandle{msg: common.TaskMessage{TaskID: taskID, TaskType: taskType}}
}

// TestProcessMessage_NonIngestionTaskAcks: a non-ingestion task is acked and
// skipped without touching the task DB or enqueuing.
func TestProcessMessage_NonIngestionTaskAcks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	handle := newFakeHandle("task-1", "not-ingestion")

	ingestor.processMessage(handle)
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("non-ingestion: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if len(ingestor.taskChan) != 0 {
		t.Fatal("expected no task enqueued")
	}
}

// TestProcessMessage_TaskNotFoundAcks: when StartRunning returns
// ErrTaskNotFound the message is acked and skipped.
func TestProcessMessage_TaskNotFoundAcks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	// No task seeded in DB — StartRunning returns ErrTaskNotFound.
	handle := newFakeHandle("no-such-task", common.TaskTypeIngestionTask)

	ingestor.processMessage(handle)
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("not-found: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestProcessMessage_AlreadyCompletedAcks: a task already in a terminal state
// (COMPLETED) is acked and skipped — no enqueue, and the document is NOT
// resurrected to RUNNING. A redelivered terminal task must not reset a
// finished document's run status or counters.
func TestProcessMessage_AlreadyCompletedAcks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	// Mark the document as a finished parse: a non-RUNNING run status and
	// non-zero counters that StartRunning would clobber to "1"/0.
	finishedRun := "3"
	if err := db.Model(&entity.Document{}).Where("id = ?", docID).
		Updates(map[string]interface{}{
			"run":       finishedRun,
			"chunk_num": 42,
			"token_num": 99,
		}).Error; err != nil {
		t.Fatalf("set document finished sentinel: %v", err)
	}

	// Set the task COMPLETED so the status switch skips it.
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.COMPLETED).Error; err != nil {
		t.Fatalf("set COMPLETED: %v", err)
	}

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	handle := newFakeHandle(taskID, common.TaskTypeIngestionTask)

	ingestor.processMessage(handle)
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("completed: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if len(ingestor.taskChan) != 0 {
		t.Fatal("expected no task enqueued for completed task")
	}

	// The finished document must be untouched - no resurrection to RUNNING.
	var doc entity.Document
	if err := db.Where("id = ?", docID).First(&doc).Error; err != nil {
		t.Fatalf("reload document: %v", err)
	}
	if doc.Run == nil || *doc.Run != finishedRun {
		t.Fatalf("document.run = %v, want %q (must not be resurrected to RUNNING)", doc.Run, finishedRun)
	}
	if doc.ChunkNum != 42 || doc.TokenNum != 99 {
		t.Fatalf("document counters changed: chunk_num=%d token_num=%d, want 42/99", doc.ChunkNum, doc.TokenNum)
	}
}

// TestProcessMessage_ClaimFailsAcks: a RUNNING task whose claim fails
// (redelivery guard) is acked without enqueuing — another worker is already
// processing it.
func TestProcessMessage_ClaimFailsAcks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	// Pre-claim the task so processMessage sees a claim conflict.
	ingestor.claimTask(taskID)

	handle := newFakeHandle(taskID, common.TaskTypeIngestionTask)

	ingestor.processMessage(handle)
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("claim-fail: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if len(ingestor.taskChan) != 0 {
		t.Fatal("expected no task enqueued when claim fails")
	}
}

// TestProcessMessage_ClaimSucceedsEnqueues: a RUNNING task with a successful
// claim is enqueued to the worker pool and the message is NOT settled yet
// (ack/nack is deferred to the worker).
func TestProcessMessage_ClaimSucceedsEnqueues(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	handle := newFakeHandle(taskID, common.TaskTypeIngestionTask)

	ingestor.processMessage(handle)
	// Ack/Nack must not be called — settlement is deferred to the worker.
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("enqueued: expected 0 Ack/0 Nack (deferred), got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	// Task must be in the channel.
	if len(ingestor.taskChan) != 1 {
		t.Fatalf("expected 1 task enqueued, got %d", len(ingestor.taskChan))
	}
	// Drain and verify.
	taskCtx := <-ingestor.taskChan
	if taskCtx.IngestionTask.ID != taskID {
		t.Fatalf("enqueued task ID = %s, want %s", taskCtx.IngestionTask.ID, taskID)
	}
}

// TestProcessMessage_ChannelFullNacks: when the task channel is at capacity
// backpressure rejects the task with Nack, releases the claim, and returns nil
// so the message is redelivered and a future attempt can re-claim it.
func TestProcessMessage_ChannelFullNacks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	// maxConcurrency=2 → channel cap=4. Fill it completely.
	ingestor := NewIngestor("test", 2, []string{"pdf"})
	for i := 0; i < cap(ingestor.taskChan); i++ {
		ingestor.taskChan <- taskpkg.NewTaskContextForScheduling(nil, &entity.IngestionTask{ID: "filler"})
	}

	handle := newFakeHandle(taskID, common.TaskTypeIngestionTask)

	ingestor.processMessage(handle)
	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("channel-full: expected 1 Nack/0 Ack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}

	// Claim must be released so a future redelivery can re-claim it.
	if !ingestor.claimTask(taskID) {
		t.Fatal("claim was not released on channel-full — task would be stuck forever")
	}
	ingestor.releaseTask(taskID)

	// Drain the fillers.
	for i := 0; i < cap(ingestor.taskChan); i++ {
		<-ingestor.taskChan
	}
}

// TestProcessMessage_StartRunningErrorNacks: when StartRunning returns a
// non-ErrTaskNotFound error (e.g. a DB blip), processMessage nacks the
// message for redelivery instead of killing the consumer (B2 fix). The
// consume loop's resilience relies on this never being a fatal return.
func TestProcessMessage_StartRunningErrorNacks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	// Drop the tasks table so GetTask fails with a generic SQL error, not
	// ErrRecordNotFound (which would map to ErrTaskNotFound and ack-skip).
	if err := db.Migrator().DropTable(&entity.IngestionTask{}); err != nil {
		t.Fatalf("drop table: %v", err)
	}

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	handle := newFakeHandle(taskID, common.TaskTypeIngestionTask)

	ingestor.processMessage(handle)

	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("start-running error: expected 1 Nack/0 Ack (redeliver), got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if len(ingestor.taskChan) != 0 {
		t.Fatal("expected no task enqueued on activation failure")
	}
}
