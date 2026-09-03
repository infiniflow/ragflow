package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

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

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	// Memory extractor must be enabled for the memory branch to enqueue.
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(nil))

	payload, err := json.Marshal(map[string]any{
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

	ingestor := newUnitIngestor("test", 1, []string{"pdf"}) // memorySvc nil by default
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

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	// Real memory service (non-nil memories) so HandleSaveToMemoryTask gets past
	// the nil-guard and reaches the progress==-1 "already failed" branch.
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(service.NewMemoryService()))

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: taskID, TaskType: common.TaskTypeMemory}}
	taskCtx := taskpkg.NewMemoryTaskContextForScheduling(context.Background(), taskID, map[string]any{
		"memory_id": "mem-3", "source_id": 7,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	}, handle)

	// Prove the failure precondition: the seeded progress=-1 row must make
	// HandleSaveToMemoryTask return the "already failed" error. Otherwise the
	// Ack below would only reflect the success path and prove nothing about
	// the Ack-on-failure contract.
	if err := ingestor.memorySvc.HandleSaveToMemoryTask(context.Background(), taskID, taskCtx.MemoryPayload); err == nil {
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

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(service.NewMemoryService()))

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-task-x", TaskType: common.TaskTypeMemory}}
	taskCtx := taskpkg.NewMemoryTaskContextForScheduling(context.Background(), "mem-task-x", map[string]any{
		"memory_id": "mem-x", "source_id": 1,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	}, handle)

	// Precondition: with no tasks table, HandleSaveToMemoryTask must fail with a
	// transient error that is NOT wrapped in ErrMemoryTaskTerminal.
	err := ingestor.memorySvc.HandleSaveToMemoryTask(context.Background(), "mem-task-x", taskCtx.MemoryPayload)
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

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
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

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	// No task seeded in DB — StartRunning returns ErrTaskNotFound.
	handle := newFakeHandle("no-such-task", common.TaskTypeIngestionTask)

	ingestor.processMessage(handle)
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("not-found: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestProcessMessage_CreatedTaskStartsRunning verifies that a worker can claim
// a task after NATS accepts its message but before the API records SCHEDULED.
func TestProcessMessage_CreatedTaskStartsRunning(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.CREATED).Error; err != nil {
		t.Fatalf("reset task to CREATED: %v", err)
	}

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	handle := newFakeHandle(taskID, common.TaskTypeIngestionTask)

	ingestor.processMessage(handle)
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("CREATED task: expected 0 Ack/0 Nack (deferred), got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if len(ingestor.taskChan) != 1 {
		t.Fatalf("expected 1 task enqueued, got %d", len(ingestor.taskChan))
	}

	var task entity.IngestionTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.RUNNING {
		t.Fatalf("status = %q, want %q", task.Status, common.RUNNING)
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

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
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

// TestProcessMessage_ClaimFailsRenewsLease: a RUNNING task whose claim fails
// is already owned by another worker. The duplicate must not Ack the broker
// message before that owner reaches a durable outcome; it only renews the
// delivery lease and stays out of the worker queue.
func TestProcessMessage_ClaimFailsRenewsLease(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	// Pre-claim the task so processMessage sees a claim conflict.
	ingestor.claimTask(taskID)

	handle := newFakeHandle(taskID, common.TaskTypeIngestionTask)

	ingestor.processMessage(handle)
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("claim-fail: expected 0 Ack/0 Nack while owner runs, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if handle.inProgress.Load() != 1 {
		t.Fatalf("claim-fail: expected duplicate lease renewal, got InProgress=%d", handle.inProgress.Load())
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

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
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

// TestProcessMessage_ChannelFullBlocksUntilSlot: backpressure must NOT drop
// the task. When the task channel is at capacity, processMessage blocks on the
// send (consuming no slot, no Nack) until a worker frees one; the message is
// then enqueued and settled by the worker. Dropping on backpressure would
// permanently lose the task once the broker's MaxDeliver is exceeded.
func TestProcessMessage_ChannelFullBlocksUntilSlot(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	// maxConcurrency=2 → channel cap=4. Fill it completely.
	ingestor := newUnitIngestor("test", 2, []string{"pdf"})
	for i := 0; i < cap(ingestor.taskChan); i++ {
		ingestor.taskChan <- taskpkg.NewTaskContextForScheduling(nil, &entity.IngestionTask{ID: "filler"})
	}

	handle := newFakeHandle(taskID, common.TaskTypeIngestionTask)

	// processMessage must BLOCK on the full channel: no ack, no nack, no
	// immediate enqueue.
	done := make(chan struct{})
	go func() {
		ingestor.processMessage(handle)
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("processMessage returned while channel was full; it must block under backpressure")
	case <-time.After(100 * time.Millisecond):
	}
	if handle.nacks.Load() != 0 || handle.acks.Load() != 0 {
		t.Fatalf("expected 0 Ack/0 Nack while blocked, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if len(ingestor.taskChan) != cap(ingestor.taskChan) {
		t.Fatalf("expected channel still full, got %d/%d", len(ingestor.taskChan), cap(ingestor.taskChan))
	}

	// Free a slot: the blocked processMessage must now enqueue the task.
	<-ingestor.taskChan
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("processMessage did not enqueue after a slot freed")
	}
	if handle.nacks.Load() != 0 || handle.acks.Load() != 0 {
		t.Fatalf("expected 0 Ack/0 Nack (settlement deferred to worker), got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	// The real task must now be in the channel.
	found := false
	for i := 0; i < cap(ingestor.taskChan); i++ {
		tc := <-ingestor.taskChan
		if tc.IngestionTask != nil && tc.IngestionTask.ID == taskID {
			found = true
		}
	}
	if !found {
		t.Fatal("real task was not enqueued after a slot freed")
	}
}

// TestProcessMessage_ShutdownRaceMarksStopped: shutdown winning the race
// against the taskChan send must not leave the task in non-terminal RUNNING
// with no in-flight worker. StartRunning has already flipped the task to
// RUNNING, so the ctx.Done branch finalizes it as STOPPED (user-retryable)
// with a detached timeout, and leaves the message unsettled — the broker
// redelivers it and the terminal status ack-skips.
func TestProcessMessage_ShutdownRaceMarksStopped(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))
	// SeedTestData creates the task RUNNING; reset to CREATED so the race
	// below is the real one: processMessage's StartRunning flips it RUNNING
	// and then blocks on the full channel.
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.CREATED).Error; err != nil {
		t.Fatalf("reset task to CREATED: %v", err)
	}

	ingestor := newUnitIngestor("test", 1, []string{"pdf"}) // channel cap = 2
	for i := 0; i < cap(ingestor.taskChan); i++ {
		ingestor.taskChan <- taskpkg.NewTaskContextForScheduling(nil, &entity.IngestionTask{ID: "filler"})
	}

	handle := newFakeHandle(taskID, common.TaskTypeIngestionTask)
	done := make(chan struct{})
	go func() {
		ingestor.processMessage(handle)
		close(done)
	}()

	// Wait until StartRunning flipped the task RUNNING and processMessage
	// is blocked on the full channel.
	deadline := time.Now().Add(2 * time.Second)
	for {
		var task entity.IngestionTask
		if err := db.Where("id = ?", taskID).First(&task).Error; err == nil && task.Status == common.RUNNING {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("task never reached RUNNING; processMessage did not reach the blocking send")
		}
		time.Sleep(2 * time.Millisecond)
	}

	// Shutdown wins the race against the channel send.
	ingestor.cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("processMessage did not return after shutdown")
	}

	var task entity.IngestionTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("task status = %q, want %q (shutdown race must not strand a RUNNING row)", task.Status, common.STOPPED)
	}
	// Message unsettled: on redelivery the STOPPED status ack-skips it.
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 0 Ack/0 Nack (unsettled for redelivery), got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
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

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	handle := newFakeHandle(taskID, common.TaskTypeIngestionTask)

	ingestor.processMessage(handle)

	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("start-running error: expected 1 Nack/0 Ack (redeliver), got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if len(ingestor.taskChan) != 0 {
		t.Fatal("expected no task enqueued on activation failure")
	}
}

// TestProcessMessage_MemoryTaskHeartbeatsWhileQueued verifies that a claimed
// memory message renews its broker lease before a worker starts it. Without
// this, a task waiting in taskChan can exceed AckWait and consume unnecessary
// broker deliveries before a worker starts it.
func TestProcessMessage_MemoryTaskHeartbeatsWhileQueued(t *testing.T) {
	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(nil))
	ingestor.heartbeatInterval = 5 * time.Millisecond

	payload, err := json.Marshal(map[string]any{
		"memory_id": "mem-1", "source_id": 42,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-queued-hb-1", TaskType: common.TaskTypeMemory, Payload: payload}}

	ingestor.processMessage(handle)

	deadline := time.Now().Add(2 * time.Second)
	for handle.inProgress.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if handle.inProgress.Load() == 0 {
		t.Fatal("expected InProgress heartbeat while memory task was queued without a worker")
	}

	// Drain and stop the lease so the heartbeat goroutine does not leak.
	taskCtx := <-ingestor.taskChan
	taskCtx.StopLease()
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("queued memory task must not be settled by admission, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestProcessMessage_MemoryTaskRedeliveryRenewsLease verifies the claim guard on
// the memory dispatch path: when a memory task is redelivered by the broker
// while the first copy is still queued/in-flight, the duplicate must renew its
// lease without Acking it or enqueuing a second worker. A different task id
// must still be accepted (claim is per-task-id).
func TestProcessMessage_MemoryTaskRedeliveryRenewsLease(t *testing.T) {
	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(nil))

	payload, err := json.Marshal(map[string]any{
		"memory_id": "mem-1", "source_id": 42,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	// First delivery: claim succeeds, task is enqueued, message unsettled.
	first := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-redeliver-1", TaskType: common.TaskTypeMemory, Payload: payload}}
	ingestor.processMessage(first)
	if first.acks.Load() != 0 || first.nacks.Load() != 0 {
		t.Fatalf("first delivery: expected 0 Ack/0 Nack (settled by worker), got acks=%d nacks=%d", first.acks.Load(), first.nacks.Load())
	}
	if len(ingestor.taskChan) != 1 {
		t.Fatalf("first delivery: expected 1 memory task enqueued, got %d", len(ingestor.taskChan))
	}

	// Redelivery while the first copy is still queued: claim fails, the
	// duplicate must renew its lease and NOT be enqueued.
	dup := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-redeliver-1", TaskType: common.TaskTypeMemory, Payload: payload}}
	ingestor.processMessage(dup)
	if dup.acks.Load() != 0 || dup.nacks.Load() != 0 {
		t.Fatalf("redelivered copy: expected 0 Ack/0 Nack while owner runs, got acks=%d nacks=%d", dup.acks.Load(), dup.nacks.Load())
	}
	if dup.inProgress.Load() != 1 {
		t.Fatalf("redelivered copy: expected 1 lease renewal, got InProgress=%d", dup.inProgress.Load())
	}
	if len(ingestor.taskChan) != 1 {
		t.Fatalf("redelivered copy must not be enqueued, got %d tasks in channel", len(ingestor.taskChan))
	}

	// A different memory task must still be accepted (claim is per-task-id).
	otherPayload, err := json.Marshal(map[string]any{
		"memory_id": "mem-2", "source_id": 43,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	})
	if err != nil {
		t.Fatalf("marshal other payload: %v", err)
	}
	other := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-redeliver-2", TaskType: common.TaskTypeMemory, Payload: otherPayload}}
	ingestor.processMessage(other)
	if len(ingestor.taskChan) != 2 {
		t.Fatalf("different memory task should be enqueued, got %d tasks in channel", len(ingestor.taskChan))
	}

	// Drain both and stop leases so heartbeat goroutines do not leak.
	firstCtx := <-ingestor.taskChan
	firstCtx.StopLease()
	secondCtx := <-ingestor.taskChan
	secondCtx.StopLease()
}

// TestProcessMessage_MemoryTaskEmptyIDAcks verifies that a memory task with an
// empty envelope task id is Acked and skipped before claiming — an empty claim
// key would otherwise strand a no-op entry in currentTasks.
func TestProcessMessage_MemoryTaskEmptyIDAcks(t *testing.T) {
	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(nil))

	payload, err := json.Marshal(map[string]any{
		"memory_id": "mem-1", "source_id": 42,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "", TaskType: common.TaskTypeMemory, Payload: payload}}

	ingestor.processMessage(handle)
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("empty-id memory task: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if len(ingestor.taskChan) != 0 {
		t.Fatalf("empty-id memory task must not be enqueued, got %d tasks", len(ingestor.taskChan))
	}
	if _, claimed := ingestor.currentTasks[""]; claimed {
		t.Fatal("empty-id memory task must not claim the empty key")
	}
}

// TestExecuteMemoryTask_ReleasesClaim verifies that executeMemoryTask releases
// the claim taken by processMessage once the worker finishes, so a later
// redelivery (e.g. after restart) can re-claim and re-run the task instead of
// being permanently ack-skipped.
func TestExecuteMemoryTask_ReleasesClaim(t *testing.T) {
	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(nil))
	// Stub the runner so no DB / LLM is touched.
	ingestor.runMemoryTask = func(_ context.Context, _ string, _ map[string]any) error {
		return nil
	}

	if !ingestor.claimTask("mem-release-1") {
		t.Fatal("expected claim to succeed")
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-release-1", TaskType: common.TaskTypeMemory}}
	taskCtx := taskpkg.NewMemoryTaskContextForScheduling(context.Background(), "mem-release-1", map[string]any{
		"memory_id": "mem-r", "source_id": 1,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	}, handle)

	ingestor.executeMemoryTask(context.Background(), taskCtx)

	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack/0 Nack on success, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
	if _, stillClaimed := ingestor.currentTasks["mem-release-1"]; stillClaimed {
		t.Fatal("expected claim released after executeMemoryTask finished")
	}
	if !ingestor.claimTask("mem-release-1") {
		t.Fatal("expected re-claim to succeed after release (future redelivery can re-run)")
	}
	ingestor.releaseTask("mem-release-1")
}

// TestExecuteMemoryTask_HeartbeatsInProgressDuringLongTask drives the real
// admission path (processMessage claim + lease) then executes the task: a
// long-running memory task (LLM extraction can take 10-65s) must renew the
// broker lease via InProgress, and settlement must stop the heartbeat before
// Acking (no Ack while an InProgress is in flight).
func TestExecuteMemoryTask_HeartbeatsInProgressDuringLongTask(t *testing.T) {
	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(nil))
	ingestor.heartbeatInterval = 5 * time.Millisecond

	started := make(chan struct{})
	proceed := make(chan struct{})
	ingestor.runMemoryTask = func(_ context.Context, _ string, _ map[string]any) error {
		close(started) // admission heartbeat is running by now
		<-proceed      // simulate a long LLM extraction
		return nil
	}

	payload, err := json.Marshal(map[string]any{
		"memory_id": "mem-h", "source_id": 1,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: "mem-hb-1", TaskType: common.TaskTypeMemory, Payload: payload}}

	// Admit through processMessage so the claim + lease are real, then drain
	// and execute like a worker would. executeMemoryTask's deferred settlement
	// (stop lease → Ack → release claim) runs before the goroutine exits, so
	// waiting on done before reading currentTasks is race-free.
	ingestor.processMessage(handle)
	taskCtx := <-ingestor.taskChan
	done := make(chan struct{})
	go func() {
		defer close(done)
		ingestor.executeMemoryTask(context.Background(), taskCtx)
	}()
	<-started

	// Poll for heartbeats with a generous deadline so the test is resilient
	// to slow CI schedulers.
	heartbeatDeadline := time.Now().Add(2 * time.Second)
	for handle.inProgress.Load() == 0 && time.Now().Before(heartbeatDeadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if handle.inProgress.Load() == 0 {
		t.Fatal("expected InProgress heartbeats while runMemoryTask was blocked, got 0")
	}

	close(proceed) // release the long task — only after confirming heartbeats

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("executeMemoryTask did not return after the long task was released")
	}
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack/0 Nack on completion after heartbeat, got acks=%d nacks=%d inProgress=%d",
			handle.acks.Load(), handle.nacks.Load(), handle.inProgress.Load())
	}
	if handle.wasSettledWithInProgress() {
		t.Fatal("Ack ran while an InProgress was still in flight — heartbeat must stop before settlement")
	}
	if _, stillClaimed := ingestor.currentTasks["mem-hb-1"]; stillClaimed {
		t.Fatal("expected claim released after executeMemoryTask finished")
	}
}

// TestExecuteMemoryTaskAlreadyCompletedAcks verifies the idempotent
// short-circuit for a task row already extracted to completion (progress=1.0):
// a redelivery after restart (or a duplicate copy) must NOT re-run the LLM
// extraction — which would insert duplicate memory entries — but must Ack the
// message. The production runner short-circuits on the progress=1.0 row.
func TestExecuteMemoryTaskAlreadyCompletedAcks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	taskID := "mem-completed-1"
	if err := dao.DB.Create(&entity.Task{ID: taskID, DocID: "doc-mem-c", Progress: 1.0}).Error; err != nil {
		t.Fatalf("insert already-completed task: %v", err)
	}

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	// Real memory service so the production runner path is exercised.
	ingestor.SetMemoryMessageService(service.NewMemoryMessageService(service.NewMemoryService()))

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: taskID, TaskType: common.TaskTypeMemory}}
	taskCtx := taskpkg.NewMemoryTaskContextForScheduling(context.Background(), taskID, map[string]any{
		"memory_id": "mem-c", "source_id": 7,
		"message_dict": map[string]any{"user_id": "u", "agent_id": "a", "session_id": "s"},
	}, handle)

	ingestor.executeMemoryTask(context.Background(), taskCtx)
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("already-completed memory task: expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}
