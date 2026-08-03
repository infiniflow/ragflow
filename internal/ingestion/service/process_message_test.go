package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
	"ragflow/internal/service"
)

func newFakeHandle(taskID, taskType string) *fakeTaskHandle {
	return &fakeTaskHandle{msg: common.TaskMessage{TaskID: taskID, TaskType: taskType}}
}

// TestProcessMessage_MemoryTaskDispatches verifies that a task_type="memory"
// message (with a NATS payload) is dispatched to the shared worker pool as a
// TaskKindMemory context rather than being acked-skipped as a non-ingestion
// task. It must NOT touch the ingestion state machine (no StartRunning).
func TestProcessMessage_MemoryTaskDispatches(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	// Memory extractor must be enabled for the memory branch to enqueue.
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(nil))

	payload, err := json.Marshal(map[string]any{
		"id":        "mem-task-1",
		"task_type": "memory",
		"memory_id": "mem-1",
		"source_id": 42,
		"message_dict": map[string]any{
			"user_id":        "u1",
			"agent_id":       "a1",
			"session_id":     "s1",
			"user_input":     "hi",
			"agent_response": "hello",
		},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-task-1", TaskType: common.TaskTypeMemory, Payload: payload}}

	ingestor.processMessage(handle)
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("memory dispatch: expected 0 Ack/0 Nack (settled by worker), got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if len(ingestor.taskChan) != 1 {
		t.Fatalf("expected 1 memory task enqueued, got %d", len(ingestor.taskChan))
	}
	taskCtx := <-ingestor.taskChan
	if taskCtx.Kind != taskpkg.TaskKindMemory {
		t.Fatalf("task kind = %v, want TaskKindMemory", taskCtx.Kind)
	}
	if taskCtx.IngestionTask != nil {
		t.Fatalf("memory task must not carry an IngestionTask (no ingestion state machine), got %+v", taskCtx.IngestionTask)
	}
	if taskCtx.MemoryPayload == nil || taskCtx.MemoryPayload["memory_id"] != "mem-1" {
		t.Fatalf("memory payload not carried correctly: %+v", taskCtx.MemoryPayload)
	}
}

// TestProcessMessage_MemoryTaskDisabledAcks verifies that when the memory
// extractor is not installed (nil), a memory task is acked and skipped so it
// does not loop forever on the worker pool.
func TestProcessMessage_MemoryTaskDisabledAcks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	ingestor := NewIngestor("test", 1, []string{"pdf"}) // memorySvc nil by default
	handle := newFakeHandle("mem-task-2", common.TaskTypeMemory)

	ingestor.processMessage(handle)
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("memory disabled: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if len(ingestor.taskChan) != 0 {
		t.Fatal("expected no memory task enqueued when extractor disabled")
	}
}

// TestExecuteMemoryTaskAlreadyFailedAcks verifies the Ack-on-failure contract in
// the case that matters: the task row is already persisted with progress=-1 (the
// durable "failed" marker written by HandleSaveToMemoryTask). In that state
// HandleSaveToMemoryTask returns "already failed", and executeMemoryTask must
// Ack — a Nack would redeliver the message into an infinite retry loop against
// an already-failed row. Using a real taskDAO + MemoryService (not a nil-guard)
// so the test exercises the "task already failed" branch, not the nil-guard.
func TestExecuteMemoryTaskAlreadyFailedAcks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	// Seed a task row already marked failed (progress=-1), mirroring what
	// HandleSaveToMemoryTask persists before returning an error.
	taskID := "mem-task-3"
	if err := dao.DB.Create(&entity.Task{ID: taskID, DocID: "doc-mem-3", Progress: -1}).Error; err != nil {
		t.Fatalf("insert already-failed task: %v", err)
	}

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	// Real memory service (non-nil memories) so HandleSaveToMemoryTask gets past
	// the nil-guard and reaches the progress==-1 "already failed" branch.
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(service.NewMemoryService()))

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: taskID, TaskType: common.TaskTypeMemory}}
	taskCtx := taskpkg.NewMemoryTaskContextForScheduling(context.Background(), map[string]any{
		"id": taskID, "task_type": "memory", "memory_id": "mem-3", "source_id": 7,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	}, handle)

	// Prove the failure precondition: the seeded progress=-1 row must make
	// HandleSaveToMemoryTask return the "already failed" error. Otherwise the
	// Ack below would only reflect the success path and prove nothing about
	// the Ack-on-failure contract.
	if err := ingestor.memorySvc.HandleSaveToMemoryTask(context.Background(), taskCtx.MemoryPayload); err == nil {
		t.Fatal("expected HandleSaveToMemoryTask to fail on an already-failed (progress=-1) task, got nil")
	}

	ingestor.executeMemoryTask(context.Background(), taskCtx)
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("already-failed memory task: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestExecuteMemoryTaskTransientFailureNacks verifies that a transient (non-terminal)
// failure from HandleSaveToMemoryTask — e.g. a task-load DB error before any durable
// progress=-1 marker is written — is Nacked so the message is redelivered, instead of
// being Acked and permanently dropped. The tasks table is intentionally NOT migrated
// so taskDAO.GetByID fails with a "no such table" error rather than gorm.ErrRecordNotFound.
func TestExecuteMemoryTaskTransientFailureNacks(t *testing.T) {
	// Migrate only a table unrelated to tasks so GetByID returns a transient
	// "no such table: tasks" error (not gorm.ErrRecordNotFound).
	db := testutil.SetupTestDB(t, &entity.IngestionTask{})
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	ingestor := NewIngestor("test", 1, []string{"pdf"})
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(service.NewMemoryService()))

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-task-x", TaskType: common.TaskTypeMemory}}
	taskCtx := taskpkg.NewMemoryTaskContextForScheduling(context.Background(), map[string]any{
		"id": "mem-task-x", "task_type": "memory", "memory_id": "mem-x", "source_id": 1,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	}, handle)

	// Precondition: with no tasks table, HandleSaveToMemoryTask must fail with a
	// transient error that is NOT wrapped in ErrMemoryTaskTerminal.
	err := ingestor.memorySvc.HandleSaveToMemoryTask(context.Background(), taskCtx.MemoryPayload)
	if err == nil {
		t.Fatal("expected HandleSaveToMemoryTask to fail with missing tasks table, got nil")
	}
	if errors.Is(err, service.ErrMemoryTaskTerminal) {
		t.Fatalf("expected a transient error, got terminal: %v", err)
	}

	ingestor.executeMemoryTask(context.Background(), taskCtx)
	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("transient memory task failure: expected 0 Ack/1 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
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
