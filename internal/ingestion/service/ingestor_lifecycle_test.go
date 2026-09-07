//
//  Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
//  Licensed under the Apache License, Version 2.0 (the "License");
//  you may not use this file except in compliance with the License.
//  You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
//  Unless required by applicable law or agreed to in writing, software
//  distributed under the License is distributed on an "AS IS" BASIS,
//  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//  See the License for the specific language governing permissions and
//  limitations under the License.
//

package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
	servicepkg "ragflow/internal/service"
)

type startupTaskPublisher struct {
	messages []common.TaskMessage
}

func (p *startupTaskPublisher) PublishTaskMessage(_ string, msg common.TaskMessage) error {
	p.messages = append(p.messages, msg)
	return nil
}

// TestStartWorkerPool_StartOnceIdempotent verifies that calling startWorkerPool
// twice only starts maxConcurrency workers (sync.Once gate). It observes the
// active worker count directly: a broken sync.Once would double the worker
// pool and activeWorkers would exceed concurrency after the second call.
func TestStartWorkerPool_StartOnceIdempotent(t *testing.T) {
	const concurrency int32 = 3
	ingestor := newUnitIngestor("test-idempotent", concurrency, nil)

	ingestor.startWorkerPool()
	// Wait for all workers to enter their loop (they block on the select
	// since ctx is not cancelled and no tasks are queued).
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if ingestor.activeWorkers.Load() == concurrency {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := ingestor.activeWorkers.Load(); got != concurrency {
		t.Fatalf("activeWorkers after first startWorkerPool = %d, want %d", got, concurrency)
	}

	// Calling again must not start additional workers (sync.Once gate).
	ingestor.startWorkerPool()
	// Allow any erroneously-started workers to register, then re-check.
	time.Sleep(50 * time.Millisecond)
	if got := ingestor.activeWorkers.Load(); got != concurrency {
		t.Fatalf("activeWorkers after second startWorkerPool = %d, want %d (sync.Once not idempotent)", got, concurrency)
	}

	ingestor.dispatchCancel()
	ingestor.workerWg.Wait()
	if got := ingestor.activeWorkers.Load(); got != 0 {
		t.Fatalf("activeWorkers after worker shutdown = %d, want 0", got)
	}
}

// TestStop_GracefulShutdown verifies that Stop cancels the context and waits
// for all worker goroutines to exit without hanging.
func TestStop_GracefulShutdown(t *testing.T) {
	const concurrency int32 = 2
	ingestor := newUnitIngestor("test-shutdown", concurrency, nil)

	// Start workers; they will block on the task channel since nothing is pushed.
	ingestor.startWorkerPool()

	done := make(chan struct{})
	go func() {
		ingestor.Stop(context.Background())
		close(done)
	}()

	select {
	case <-done:
		// workers exited cleanly
	case <-time.After(5 * time.Second):
		t.Fatal("Stop() timed out waiting for workers to exit")
	}
}

// TestStop_ClosesShutdownCh verifies that Stop closes ShutdownCh so the
// cmd-side select on <-ingestor.ShutdownCh unblocks and the orchestrator
// knows shutdown completed. Mirrors syncer.go which closes its ShutdownCh in
// Stop. Without this, the admin graceful-shutdown path is dead (cmd blocks
// forever on the receive).
func TestStop_ClosesShutdownCh(t *testing.T) {
	ingestor := newUnitIngestor("test-shutdown-ch", 1, nil)
	ingestor.Stop(context.Background())
	select {
	case <-ingestor.ShutdownCh:
		// closed - pass
	default:
		t.Fatal("ShutdownCh should be closed after Stop returns")
	}
}

// TestStop_TimesOutWhenWorkerStuck verifies the B1 fix: when a worker is
// blocked in a stage that does not honor ctx cancellation (e.g. a native
// CGO parse), Stop returns once its deadline expires instead of hanging on
// workerWg.Wait() forever. The in-flight task is left for broker redelivery.
func TestStop_TimesOutWhenWorkerStuck(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	const concurrency int32 = 1
	ingestor := newUnitIngestor("test-stuck", concurrency, []string{"pdf"})
	ingestor.startWorkerPool()

	// runDocumentTask blocks on release and ignores ctx, simulating a
	// non-cancellable native parse. started signals the worker is inside it.
	release := make(chan struct{})
	started := make(chan struct{})
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		close(started)
		<-release
		return nil
	}

	// Seed the task RUNNING so runTask's MarkCompleted path is valid.
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.RUNNING).Error; err != nil {
		t.Fatalf("set task RUNNING: %v", err)
	}

	slot := <-ingestor.idleSlots
	slot.inbox <- &fakeTaskHandle{msg: common.TaskMessage{TaskID: taskID, TaskType: common.TaskTypeIngestionTask}}

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not enter runDocumentTask")
	}

	// Stop with a short deadline must return instead of hanging.
	stopDone := make(chan struct{})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		defer cancel()
		ingestor.Stop(ctx)
		close(stopDone)
	}()

	select {
	case <-stopDone:
		// Stop returned within the deadline - the fix works.
	case <-time.After(3 * time.Second):
		t.Fatal("Stop() hung instead of returning on deadline")
	}

	// Release the stuck worker so it finishes and the test goroutine stays clean.
	close(release)
	ingestor.workerWg.Wait()
}

// TestStopDeadlineStopsStuckWorkerHeartbeat prevents a task that ignores
// execution cancellation from renewing its broker lease after graceful
// shutdown has timed out. Once Stop returns at its deadline, the unfinished
// handle must be left for broker redelivery instead of being kept alive by a
// leaked heartbeat.
func TestStopDeadlineStopsStuckWorkerHeartbeat(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, _, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test-stuck-heartbeat", 1, []string{"pdf"})
	ingestor.heartbeatInterval = 10 * time.Millisecond
	ingestor.startWorkerPool()

	release := make(chan struct{})
	started := make(chan struct{})
	ingestor.runDocumentTask = func(context.Context, *entity.IngestionTask) error {
		close(started)
		<-release
		return nil
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{TaskID: taskID, TaskType: common.TaskTypeIngestionTask}}
	slot := <-ingestor.idleSlots
	slot.inbox <- handle
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not enter runDocumentTask")
	}

	deadline := time.Now().Add(time.Second)
	for handle.inProgress.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if handle.inProgress.Load() == 0 {
		t.Fatal("heartbeat did not renew the running handle")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	ingestor.Stop(stopCtx)
	cancel()

	pulsesAtStop := handle.inProgress.Load()
	time.Sleep(50 * time.Millisecond)
	if got := handle.inProgress.Load(); got != pulsesAtStop {
		t.Fatalf("heartbeat renewals after Stop deadline = %d, want none", got-pulsesAtStop)
	}

	close(release)
	ingestor.workerWg.Wait()
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("timed-out handle settlement = %d Ack / %d Nack, want none", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestStopDeadlineLeavesStuckMemoryHandleUnsettled applies the same broker
// redelivery rule to memory extraction: a non-cooperative memory runner must
// not settle its old handle after the ingestor's Stop deadline passes.
func TestStopDeadlineLeavesStuckMemoryHandleUnsettled(t *testing.T) {
	ingestor := newUnitIngestor("test-stuck-memory", 1, nil)
	ingestor.memorySvc = &servicepkg.MemoryMessageService{}
	ingestor.startWorkerPool()

	release := make(chan struct{})
	started := make(chan struct{})
	ingestor.runMemoryTask = func(context.Context, string, map[string]any) error {
		close(started)
		<-release
		return nil
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   "memory-stop-timeout",
		TaskType: common.TaskTypeMemory,
		Payload:  []byte(`{}`),
	}}
	slot := <-ingestor.idleSlots
	slot.inbox <- handle
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not enter memory runner")
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	ingestor.Stop(stopCtx)
	cancel()

	close(release)
	ingestor.workerWg.Wait()
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("timed-out memory handle settlement = %d Ack / %d Nack, want none", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestPollCancel_ExitsWhenDoneClosed verifies that closing the done channel
// causes pollCancel to return even when cancelCheck is blocked (e.g. on a
// long DB query). Without BP3, the initial cancelCheck call runs
// synchronously and pollCancel cannot observe done until it returns.
func TestPollCancel_ExitsWhenDoneClosed(t *testing.T) {
	ingestor := newUnitIngestor("test", 1, []string{"pdf"})

	// Block cancelCheck until released — simulate a stuck DB call.
	blocking := make(chan struct{})
	released := make(chan struct{})
	ingestor.cancelCheck = func(ctx context.Context, taskID string) bool {
		close(blocking)
		<-released
		return false
	}

	done := make(chan struct{})
	exited := make(chan struct{})
	go func() {
		ingestor.pollCancel("task-1", func() {}, done)
		close(exited)
	}()

	// Wait for cancelCheck to enter the blocking call.
	<-blocking

	// Close done — pollCancel must exit even though cancelCheck is stuck.
	close(done)

	select {
	case <-exited:
		// pollCancel returned — BP3 fix works.
	case <-time.After(2 * time.Second):
		t.Fatal("pollCancel did not exit when done closed (stuck in blocking cancelCheck)")
	}

	close(released) // cleanup
}

// TestStartNilEngine verifies that Start returns an error instead of panicking
// when the message queue engine has not been initialized.
func TestStartNilEngine(t *testing.T) {
	previousEngine := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(nil)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousEngine) })

	ingestor := newUnitIngestor("test-nil-engine", 1, []string{"pdf"})
	defer ingestor.Stop(context.Background())

	err := ingestor.Start()
	if err == nil || !strings.Contains(err.Error(), "not initialized") {
		t.Fatalf("Start() with nil engine: err = %v, want 'not initialized'", err)
	}
	if got := ingestor.activeWorkers.Load(); got != 0 {
		t.Fatalf("activeWorkers after failed Start = %d, want 0", got)
	}
}

// TestStartRetainsStartupFailure prevents a second Start call from
// reporting success after initialization failed on the first attempt.
func TestStartRetainsStartupFailure(t *testing.T) {
	previousEngine := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(nil)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousEngine) })

	ingestor := newUnitIngestor("test-startup-failure", 1, nil)
	t.Cleanup(func() { ingestor.Stop(context.Background()) })

	if err := ingestor.Start(); err == nil {
		t.Fatal("first Start unexpectedly succeeded with nil engine")
	}
	if err := ingestor.Start(); err == nil {
		t.Fatal("second Start hid the prior startup failure")
	}
	if got := ingestor.activeWorkers.Load(); got != 0 {
		t.Fatalf("active workers after failed Start = %d, want 0", got)
	}
}

// TestExecuteTask_MarkFailedAfterCtxCancelAcks verifies that a generic task
// failure is persisted and acknowledged even after the task context is
// cancelled by the pipeline.
func TestExecuteTask_MarkFailedAfterCtxCancelAcks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()
	handle := &fakeTaskHandle{}
	taskCtx := taskpkg.NewTaskContextForScheduling(parentCtx, &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})
	taskCtx.Handle = handle
	ingestor.runDocumentTask = func(_ context.Context, _ *entity.IngestionTask) error {
		parentCancel()
		return errors.New("boom")
	}

	ingestor.executeTask(context.Background(), taskCtx)

	var task entity.IngestionTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("task status = %q, want %q", task.Status, common.FAILED)
	}
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestStart_FullPathReturnsAndStartsWorkers is a regression test for the
// sync.Once re-entrancy deadlock. Before the fix, Start() wrapped the whole
// startup (start()) in e.startOnce.Do, but start() also called startWorkerPool()
// which nested the SAME startOnce. sync.Once.Do blocks forever when re-entered
// from inside its own callback, so Start() hung after InitConsumer succeeded:
// no worker pool, no consumeLoop, and ingestion tasks were never consumed.
//
// The test drives the real Start() path (start -> startWorkerPool -> consumeLoop)
// against an embedded NATS server and asserts Start() returns within a deadline
// and that workers are actually up.
func TestStart_FullPathReturnsAndStartsWorkers(t *testing.T) {
	// SetMessageQueueEngine mutates process-global state; restore the previous
	// engine so later tests don't inherit a closed embedded NATS server.
	previousEngine := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(testutil.SetupNatsEngine(t))
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousEngine) })

	const concurrency int32 = 2
	ing := newUnitIngestor("test-start-fullpath", concurrency, nil)
	t.Cleanup(func() { ing.Stop(context.Background()) })

	done := make(chan error, 1)
	go func() {
		done <- ing.Start()
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start() returned error: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Start() did not return within 10s; sync.Once re-entrancy deadlock likely")
	}

	// Start() launches workers asynchronously and returns immediately; poll
	// briefly so we don't observe zero before a worker enters workerLoop.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && ing.activeWorkers.Load() <= 0 {
		time.Sleep(time.Millisecond)
	}
	if got := ing.activeWorkers.Load(); got <= 0 {
		t.Fatalf("expected activeWorkers > 0 after Start(), got %d", got)
	}
}

func TestStartSchedulesCreatedTasks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, _, taskID := testutil.SeedTestData(t, db)
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.CREATED).Error; err != nil {
		t.Fatalf("set task CREATED: %v", err)
	}

	previousEngine := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(testutil.SetupNatsEngine(t))
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousEngine) })

	ingestor := newUnitIngestor("test-schedule-created", 1, nil)
	publisher := &startupTaskPublisher{}
	ingestor.ingestionTaskSvc.SetTaskPublisher(publisher)
	t.Cleanup(func() { ingestor.Stop(context.Background()) })

	if err := ingestor.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	if len(publisher.messages) != 1 {
		t.Fatalf("published messages = %d, want 1", len(publisher.messages))
	}
	if publisher.messages[0].TaskID != taskID {
		t.Fatalf("published task ID = %q, want %q", publisher.messages[0].TaskID, taskID)
	}

	var task entity.IngestionTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		t.Fatalf("load scheduled task: %v", err)
	}
	if task.Status != common.SCHEDULED {
		t.Fatalf("task status = %q, want %q", task.Status, common.SCHEDULED)
	}
}

// TestSlotDispatcherDoesNotActivateTaskUntilWorkerOwnsSlot prevents a busy
// worker from prefetching its next task. With the old buffered dispatcher,
// task-2 moves to RUNNING while task-1 still occupies the only worker.
func TestSlotDispatcherDoesNotActivateTaskUntilWorkerOwnsSlot(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	t.Cleanup(cleanup)

	taskIDs := seedBurstTasks(t, db, 2)
	for _, taskID := range taskIDs {
		if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
			Update("status", common.SCHEDULED).Error; err != nil {
			t.Fatalf("schedule task %s: %v", taskID, err)
		}
	}

	queue := testutil.SetupNatsEngine(t)
	previousEngine := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousEngine) })

	ingestor := newUnitIngestor("test-slot-dispatch", 1, []string{"pdf"})
	releaseFirst := make(chan struct{})
	firstStarted := make(chan struct{})
	ingestor.runDocumentTask = func(_ context.Context, task *entity.IngestionTask) error {
		if task.ID == taskIDs[0] {
			close(firstStarted)
			<-releaseFirst
		}
		return nil
	}
	t.Cleanup(func() {
		close(releaseFirst)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ingestor.Stop(ctx)
	})

	for _, taskID := range taskIDs {
		payload, err := json.Marshal(common.TaskMessage{
			TaskID:   taskID,
			TaskType: common.TaskTypeIngestionTask,
		})
		if err != nil {
			t.Fatalf("marshal task %s: %v", taskID, err)
		}
		if err := queue.PublishTask(common.TaskSubject, payload); err != nil {
			t.Fatalf("publish task %s: %v", taskID, err)
		}
	}

	if err := ingestor.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case <-firstStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first task did not start")
	}

	time.Sleep(150 * time.Millisecond)
	var second entity.IngestionTask
	if err := db.Where("id = ?", taskIDs[1]).First(&second).Error; err != nil {
		t.Fatalf("load second task: %v", err)
	}
	if second.Status != common.SCHEDULED {
		t.Fatalf("second task status = %q, want %q while the only worker is busy", second.Status, common.SCHEDULED)
	}
}
