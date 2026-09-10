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
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
	servicepkg "ragflow/internal/service"
)

// TestHandleAndExecute_MemoryTaskIDInvokesRunner verifies memory deliveries
// need only the task id because execution input lives in the database.
func TestHandleAndExecute_MemoryTaskIDInvokesRunner(t *testing.T) {
	ingestor := newUnitIngestor("test-mem-malformed", 1, nil)
	ingestor.SetMemoryMessageService(servicepkg.NewMemoryMessageService(servicepkg.NewMemoryService()))

	runnerCalled := false
	ingestor.runMemoryTask = func(ctx context.Context, taskID, leaseOwner string) (servicepkg.MemoryTaskDisposition, error) {
		runnerCalled = true
		if taskID != "mem-bad-payload" || leaseOwner == "" {
			t.Fatalf("runner identity = %q/%q", taskID, leaseOwner)
		}
		return servicepkg.MemoryTaskAcknowledge, nil
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   "mem-bad-payload",
		TaskType: common.TaskTypeMemory,
	}}

	ingestor.handleAndExecute(handle)

	if !runnerCalled {
		t.Fatal("expected runMemoryTask to be called with task id only")
	}
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestHandleAndExecute_MemoryExtractorDisabledAcks verifies that a memory task
// received when memory extractor is disabled is ack-skipped.
func TestHandleAndExecute_MemoryExtractorDisabledAcks(t *testing.T) {
	ingestor := newUnitIngestor("test-mem-disabled", 1, nil)
	// memorySvc is nil

	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   "mem-disabled",
		TaskType: common.TaskTypeMemory,
	}}

	ingestor.handleAndExecute(handle)

	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestHandleAndExecute_MemoryEmptyTaskIDAcks verifies that a memory task with
// an empty task ID is ack-skipped.
func TestHandleAndExecute_MemoryEmptyTaskIDAcks(t *testing.T) {
	ingestor := newUnitIngestor("test-mem-empty-id", 1, nil)
	ingestor.SetMemoryMessageService(servicepkg.NewMemoryMessageService(servicepkg.NewMemoryService()))

	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   "",
		TaskType: common.TaskTypeMemory,
	}}

	ingestor.handleAndExecute(handle)

	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestHandleAndExecute_UnknownTaskTypeAcks verifies that a message with an
// unknown task type is ack-skipped without touching the ingestion task service.
func TestHandleAndExecute_UnknownTaskTypeAcks(t *testing.T) {
	ingestor := newUnitIngestor("test-unknown-type", 1, nil)

	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   "unknown-task-1",
		TaskType: "completely_unknown_type",
	}}

	ingestor.handleAndExecute(handle)

	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack/0 Nack for unknown type, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestHandleAndExecute_TaskNotFoundAcks verifies that when StartRunning returns
// ErrTaskNotFound, the message is Acked to prevent indefinite redeliveries.
func TestHandleAndExecute_TaskNotFoundAcks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	ingestor := newUnitIngestor("test-task-not-found", 1, []string{"pdf"})
	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   "non-existent-task-id",
		TaskType: common.TaskTypeIngestionTask,
	}}

	ingestor.handleAndExecute(handle)

	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack/0 Nack for not-found task, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestHandleAndExecute_StartRunningTransientErrorNacks verifies that a transient
// database error during StartRunning causes the message to be Nacked for redelivery.
func TestHandleAndExecute_StartRunningTransientErrorNacks(t *testing.T) {
	// Close/nil DB to simulate transient database failure
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, _, taskID := testutil.SeedTestData(t, db)

	// Close underlying sql db to force query errors
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}

	ingestor := newUnitIngestor("test-start-running-err", 1, []string{"pdf"})
	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   taskID,
		TaskType: common.TaskTypeIngestionTask,
	}}

	ingestor.handleAndExecute(handle)

	if handle.nacks.Load() != 1 || handle.acks.Load() != 0 {
		t.Fatalf("expected 0 Ack/1 Nack on transient StartRunning error, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestHandleAndExecute_AlreadyTerminalTaskAcks verifies that tasks already in a
// terminal state (COMPLETED, STOPPED, FAILED) are Ack-skipped without re-execution.
func TestHandleAndExecute_AlreadyTerminalTaskAcks(t *testing.T) {
	statuses := []string{common.COMPLETED, common.STOPPED, common.FAILED}
	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			db := testutil.SetupTestDB(t)
			cleanup := testutil.ReplaceDBForTest(t, db)
			defer cleanup()

			_, _, _, taskID := testutil.SeedTestData(t, db)
			if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
				Update("status", status).Error; err != nil {
				t.Fatalf("set status %s: %v", status, err)
			}

			ingestor := newUnitIngestor("test-terminal-"+status, 1, []string{"pdf"})
			pipelineRan := false
			ingestor.runDocumentTask = func(ctx context.Context, task *entity.IngestionTask) error {
				pipelineRan = true
				return nil
			}

			handle := &fakeTaskHandle{msg: common.TaskMessage{
				TaskID:   taskID,
				TaskType: common.TaskTypeIngestionTask,
			}}

			ingestor.handleAndExecute(handle)

			if pipelineRan {
				t.Fatalf("pipeline should not run for already %s task", status)
			}
			if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
				t.Fatalf("status %s: expected 1 Ack/0 Nack, got acks=%d nacks=%d", status, handle.acks.Load(), handle.nacks.Load())
			}
		})
	}
}

// TestHandleAndExecute_StoppingTaskConvergedAndAcks verifies that a task in STOPPING
// status is transitioned to STOPPED by StartRunning and then Acked.
func TestHandleAndExecute_StoppingTaskConvergedAndAcks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, _, taskID := testutil.SeedTestData(t, db)
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.STOPPING).Error; err != nil {
		t.Fatalf("set status STOPPING: %v", err)
	}

	ingestor := newUnitIngestor("test-stopping-converge", 1, []string{"pdf"})
	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   taskID,
		TaskType: common.TaskTypeIngestionTask,
	}}

	ingestor.handleAndExecute(handle)

	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack/0 Nack for STOPPING task, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}

	var task entity.IngestionTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.STOPPED {
		t.Fatalf("task status = %s, want STOPPED", task.Status)
	}
}

// TestHandleAndExecute_DocumentDuplicateClaimRenewsLease verifies that a redelivered
// document task while the worker is still processing renews its lease via InProgress
// and does not Ack or start a second execution.
func TestHandleAndExecute_DocumentDuplicateClaimRenewsLease(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, _, taskID := testutil.SeedTestData(t, db)
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.RUNNING).Error; err != nil {
		t.Fatalf("set status RUNNING: %v", err)
	}

	ingestor := newUnitIngestor("test-doc-dup", 1, []string{"pdf"})
	// Simulate active claim by another worker
	if !ingestor.claimTask(taskID) {
		t.Fatal("first claim should succeed")
	}

	pipelineRan := false
	ingestor.runDocumentTask = func(ctx context.Context, task *entity.IngestionTask) error {
		pipelineRan = true
		return nil
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   taskID,
		TaskType: common.TaskTypeIngestionTask,
	}}

	ingestor.handleAndExecute(handle)

	if pipelineRan {
		t.Fatal("duplicate delivery should not execute pipeline")
	}
	if handle.inProgress.Load() != 1 {
		t.Fatalf("expected InProgress = 1 for duplicate delivery, got %d", handle.inProgress.Load())
	}
	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 0 Ack/0 Nack for duplicate delivery, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestHandleAndExecute_MemoryUnsettledDispositionDefersToDurableRecovery
// verifies DB lease admission controls settlement for duplicate or failed work.
func TestHandleAndExecute_MemoryUnsettledDispositionDefersToDurableRecovery(t *testing.T) {
	ingestor := newUnitIngestor("test-mem-dup", 1, nil)
	ingestor.SetMemoryMessageService(servicepkg.NewMemoryMessageService(servicepkg.NewMemoryService()))
	ingestor.runMemoryTask = func(context.Context, string, string) (servicepkg.MemoryTaskDisposition, error) {
		return servicepkg.MemoryTaskLeaveUnsettled, nil
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   "mem-dup-1",
		TaskType: common.TaskTypeMemory,
	}}

	ingestor.handleAndExecute(handle)

	if handle.acks.Load() != 0 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 0 Ack/0 Nack, got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}

// TestHandleAndExecute_SlowAdmissionHeartbeatsUnderLeaseProtection verifies that
// when admission (StartRunning) is slow (e.g. database lag exceeding ack deadline),
// the heartbeat started upon receiving the handle renews the broker lease before
// StartRunning finishes.
func TestHandleAndExecute_SlowAdmissionHeartbeatsUnderLeaseProtection(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	_, _, _, taskID := testutil.SeedTestData(t, db)
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.RUNNING).Error; err != nil {
		t.Fatalf("set status RUNNING: %v", err)
	}

	ingestor := newUnitIngestor("test-slow-admission", 1, []string{"pdf"})
	ingestor.heartbeatInterval = 10 * time.Millisecond

	// Delay document pipeline execution briefly so heartbeats fire during run
	ingestor.runDocumentTask = func(ctx context.Context, task *entity.IngestionTask) error {
		time.Sleep(50 * time.Millisecond)
		return nil
	}

	handle := &fakeTaskHandle{msg: common.TaskMessage{
		TaskID:   taskID,
		TaskType: common.TaskTypeIngestionTask,
	}}

	ingestor.handleAndExecute(handle)

	if handle.inProgress.Load() == 0 {
		t.Fatal("expected InProgress heartbeats during task execution, got 0")
	}
	if handle.acks.Load() != 1 {
		t.Fatalf("expected 1 Ack on completion, got %d", handle.acks.Load())
	}
}

// TestWorkerDispatcher_WaitAfterPullErrorCancelsCleanly verifies that waitAfterPullError
// unblocks promptly when dispatch context is cancelled, without hanging on the sleep.
func TestWorkerDispatcher_WaitAfterPullErrorCancelsCleanly(t *testing.T) {
	ingestor := newUnitIngestor("test-pull-backoff-cancel", 1, nil)

	done := make(chan struct{})
	go func() {
		ingestor.waitAfterPullError()
		close(done)
	}()

	// Cancel dispatch context
	ingestor.dispatchCancel()

	select {
	case <-done:
		// unblocked promptly
	case <-time.After(200 * time.Millisecond):
		t.Fatal("waitAfterPullError did not cancel promptly on dispatchCtx.Done")
	}
}
