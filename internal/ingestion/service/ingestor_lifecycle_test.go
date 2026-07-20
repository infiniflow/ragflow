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
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"
)

// TestStartWorkerPool_StartOnceIdempotent verifies that calling startWorkerPool
// twice only starts maxConcurrency workers (sync.Once gate). It observes the
// active worker count directly: a broken sync.Once would double the worker
// pool and activeWorkers would exceed concurrency after the second call.
func TestStartWorkerPool_StartOnceIdempotent(t *testing.T) {
	const concurrency int32 = 3
	ingestor := NewIngestor("test-idempotent", concurrency, nil)

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

	ingestor.cancel()
	ingestor.workerWg.Wait()
}

// TestStop_GracefulShutdown verifies that Stop cancels the context and waits
// for all worker goroutines to exit without hanging.
func TestStop_GracefulShutdown(t *testing.T) {
	const concurrency int32 = 2
	ingestor := NewIngestor("test-shutdown", concurrency, nil)

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
	ingestor := NewIngestor("test-shutdown-ch", 1, nil)
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
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	const concurrency int32 = 1
	ingestor := NewIngestor("test-stuck", concurrency, []string{"pdf"})
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

	taskCtx := taskpkg.NewTaskContextForScheduling(ingestor.ctx, &entity.IngestionTask{
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
	})
	ingestor.taskChan <- taskCtx

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

// TestPollCancel_ExitsWhenDoneClosed verifies that closing the done channel
// causes pollCancel to return even when cancelCheck is blocked (e.g. on a
// long DB query). Without BP3, the initial cancelCheck call runs
// synchronously and pollCancel cannot observe done until it returns.
func TestPollCancel_ExitsWhenDoneClosed(t *testing.T) {
	ingestor := NewIngestor("test", 1, []string{"pdf"})

	// Block cancelCheck until released — simulate a stuck DB call.
	blocking := make(chan struct{})
	released := make(chan struct{})
	ingestor.cancelCheck = func(taskID string) bool {
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
