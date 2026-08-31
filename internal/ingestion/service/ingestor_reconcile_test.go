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
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"

	"gorm.io/gorm"
)

// enqueueRecorder captures re-enqueue publishes so the reconciliation scan
// can be asserted without a live broker.
type enqueueRecorder struct {
	mu      sync.Mutex
	taskIDs []string
}

func (p *enqueueRecorder) PublishTaskMessage(_ string, msg common.TaskMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.taskIDs = append(p.taskIDs, msg.TaskID)
	return nil
}

func (p *enqueueRecorder) published() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.taskIDs...)
}

// seedAgedTask inserts an ingestion task with explicit create/update
// timestamps aged back by the given duration (startup reconciliation judges
// staleness by these columns).
func seedAgedTask(t *testing.T, db *gorm.DB, id, docID, status string, age time.Duration) {
	t.Helper()
	ts := time.Now().Add(-age).UnixMilli()
	if err := db.Create(&entity.IngestionTask{
		ID:         id,
		UserID:     "u1",
		DocumentID: docID,
		DatasetID:  "kb-1",
		Status:     status,
		BaseModel:  entity.BaseModel{CreateTime: &ts, UpdateTime: &ts},
	}).Error; err != nil {
		t.Fatalf("seed task %s: %v", id, err)
	}
}

// TestReconcileTasks converges every old CREATED row to SCHEDULED, recovers
// stale unleased RUNNING rows left by old writers, and publishes each
// resulting dispatch intent. A fresh unleased RUNNING row is left alone
// because it may belong to a live worker that predates lease initialization.
func TestReconcileTasks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	seedAgedTask(t, db, "task-run-stale", "doc-run-stale", common.RUNNING, 16*time.Minute)
	seedAgedTask(t, db, "task-run-fresh", "doc-run-fresh", common.RUNNING, 1*time.Minute)
	seedAgedTask(t, db, "task-created-stale", "doc-created-stale", common.CREATED, 6*time.Minute)
	seedAgedTask(t, db, "task-created-fresh", "doc-created-fresh", common.CREATED, 1*time.Minute)
	seedAgedTask(t, db, "task-completed-old", "doc-completed-old", common.COMPLETED, 1*time.Hour)

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	recorder := &enqueueRecorder{}
	ingestor.ingestionTaskSvc.SetTaskPublisher(recorder)

	ingestor.reconcileTasks(t.Context())

	statusOf := func(id string) string {
		t.Helper()
		var task entity.IngestionTask
		if err := db.Where("id = ?", id).First(&task).Error; err != nil {
			t.Fatalf("reload task %s: %v", id, err)
		}
		return task.Status
	}

	for _, id := range []string{"task-run-stale", "task-created-stale", "task-created-fresh"} {
		if got := statusOf(id); got != common.SCHEDULED {
			t.Fatalf("task %s status = %q, want SCHEDULED", id, got)
		}
	}
	if got := statusOf("task-run-fresh"); got != common.RUNNING {
		t.Fatalf("fresh unleased task status = %q, want RUNNING", got)
	}
	if got := statusOf("task-completed-old"); got != common.COMPLETED {
		t.Fatalf("completed task status = %q, want COMPLETED", got)
	}

	// The reconciliation pass publishes every runnable task and never revives
	// terminal rows.
	published := recorder.published()
	if len(published) != 3 {
		t.Fatalf("re-enqueued tasks = %v, want three runnable tasks", published)
	}
}

func TestDispatchScheduledTasksSkipsRecentlyDispatchedTasks(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	now := time.Now().Truncate(time.Millisecond)
	for _, task := range []*entity.IngestionTask{
		{ID: "task-fresh", UserID: "u1", DocumentID: "doc-fresh", DatasetID: "kb-1", Status: common.SCHEDULED, LastDispatchedAt: now.Add(-time.Second).UnixMilli()},
		{ID: "task-never-dispatched", UserID: "u1", DocumentID: "doc-never-dispatched", DatasetID: "kb-1", Status: common.SCHEDULED},
	} {
		if err := db.Create(task).Error; err != nil {
			t.Fatalf("create task %s: %v", task.ID, err)
		}
	}

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	recorder := &enqueueRecorder{}
	ingestor.ingestionTaskSvc.SetTaskPublisher(recorder)
	ingestor.dispatchScheduledTasks(t.Context(), now)

	if got := recorder.published(); len(got) != 1 || got[0] != "task-never-dispatched" {
		t.Fatalf("dispatched tasks = %v, want [task-never-dispatched]", got)
	}
}

func TestConcurrentIngestorsReserveScheduledDispatchOnce(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	if err := db.Create(&entity.IngestionTask{
		ID:               "task-concurrent",
		UserID:           "u1",
		DocumentID:       "doc-concurrent",
		DatasetID:        "kb-1",
		Status:           common.SCHEDULED,
		LastDispatchedAt: time.Now().Add(-dispatchGracePeriod - time.Second).UnixMilli(),
	}).Error; err != nil {
		t.Fatalf("create task: %v", err)
	}

	first := newUnitIngestor("first", 1, []string{"pdf"})
	second := newUnitIngestor("second", 1, []string{"pdf"})
	firstPublisher := &enqueueRecorder{}
	secondPublisher := &enqueueRecorder{}
	first.ingestionTaskSvc.SetTaskPublisher(firstPublisher)
	second.ingestionTaskSvc.SetTaskPublisher(secondPublisher)

	var wg sync.WaitGroup
	for _, ingestor := range []*Ingestor{first, second} {
		wg.Add(1)
		go func(ingestor *Ingestor) {
			defer wg.Done()
			if err := ingestor.ingestionTaskSvc.DispatchScheduledTask(t.Context(), "task-concurrent", time.Now()); err != nil {
				t.Errorf("DispatchScheduledTask: %v", err)
			}
		}(ingestor)
	}
	wg.Wait()

	if got := len(firstPublisher.published()) + len(secondPublisher.published()); got != 1 {
		t.Fatalf("concurrent dispatches = %d, want one reservation", got)
	}
}

// TestRecoverExpiredClaimsMarksPoisonDocument verifies that the recovery cap
// turns the next expired lease into a terminal task failure and surfaces the
// reason on its document instead of publishing another wake-up message.
func TestRecoverExpiredClaimsMarksPoisonDocument(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()
	_, _, docID, taskID := testutil.SeedTestData(t, db, testutil.WithPipelineID("flow-1"))

	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).Updates(map[string]interface{}{
		"claim_expires_at":       time.Now().Add(-time.Second).UnixMilli(),
		"lease_recovery_attempt": defaultLeaseRecoveryMax,
	}).Error; err != nil {
		t.Fatalf("expire task at recovery cap: %v", err)
	}
	if err := db.Model(&entity.Document{}).Where("id = ?", docID).
		Update("progress_msg", strings.Repeat("x", progressLogMaxChars+100)).Error; err != nil {
		t.Fatalf("seed long progress message: %v", err)
	}

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	recorder := &enqueueRecorder{}
	ingestor.ingestionTaskSvc.SetTaskPublisher(recorder)
	ingestor.recoverExpiredClaims(t.Context(), time.Now())

	var task entity.IngestionTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("task status = %q, want FAILED", task.Status)
	}
	if published := recorder.published(); len(published) != 0 {
		t.Fatalf("poison task was republished: %v", published)
	}

	var document entity.Document
	if err := db.Where("id = ?", docID).First(&document).Error; err != nil {
		t.Fatalf("reload document: %v", err)
	}
	if document.Run == nil || *document.Run != string(entity.TaskStatusFail) || document.Progress != -1 {
		t.Fatalf("document state = run:%v progress:%v, want FAIL/-1", document.Run, document.Progress)
	}
	if document.ProgressMsg == nil || !strings.Contains(*document.ProgressMsg, "Failed after repeated lease expiry") {
		t.Fatalf("document progress message = %v, want poison reason", document.ProgressMsg)
	}
	if len(*document.ProgressMsg) > progressLogMaxChars {
		t.Fatalf("poison progress message length = %d, want <= %d", len(*document.ProgressMsg), progressLogMaxChars)
	}
}

// TestStartNilEngine verifies that Start on an ingestor whose message queue
// engine was never initialized returns an error instead of panicking on the
// nil engine (mirror of the nats package uninitialized-engine contract).
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
	// The failure must happen before the worker pool is provisioned.
	if got := ingestor.activeWorkers.Load(); got != 0 {
		t.Fatalf("activeWorkers after failed Start = %d, want 0", got)
	}
}

// TestExecuteTask_MarkFailedAfterCtxCancelAcks verifies the ctx de-contamination
// of the failure path: the pipeline cancels the task context and then fails with
// a generic error. The FAILED write must still land (detached short timeout) and
// the message must be Acked (terminal), not Nack-requeued against a dead run.
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
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING, ClaimToken: testutil.TestClaimToken,
	})
	taskCtx.Handle = handle

	ingestor.runDocumentTask = func(_ context.Context, _ *entity.IngestionTask) error {
		parentCancel()            // cancel races the pipeline...
		return errors.New("boom") // ...and a generic failure lands afterwards
	}

	ingestor.executeTask(context.Background(), taskCtx)

	var task entity.IngestionTask
	if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
		t.Fatalf("reload task: %v", err)
	}
	if task.Status != common.FAILED {
		t.Fatalf("task status = %q, want %q (FAILED write must survive a cancelled ctx)", task.Status, common.FAILED)
	}
	if handle.acks.Load() != 1 || handle.nacks.Load() != 0 {
		t.Fatalf("expected 1 Ack/0 Nack (terminal), got acks=%d nacks=%d", handle.acks.Load(), handle.nacks.Load())
	}
}
