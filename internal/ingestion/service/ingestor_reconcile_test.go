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

// TestStartupReconciliation drives the startup scan: stale CREATED orphans
// are re-published for consumption, stale RUNNING orphans are intentionally
// left untouched (recovery relies on NATS redelivery, not startup FAILED;
// see ingestion_service.go reconcileStartupTasks), and fresh rows are left alone.
func TestStartupReconciliation(t *testing.T) {
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

	ingestor.reconcileStartupTasks()

	statusOf := func(id string) string {
		t.Helper()
		var task entity.IngestionTask
		if err := db.Where("id = ?", id).First(&task).Error; err != nil {
			t.Fatalf("reload task %s: %v", id, err)
		}
		return task.Status
	}

	// RUNNING orphans are intentionally left untouched (NATS redelivery path).
	if got := statusOf("task-run-stale"); got != common.RUNNING {
		t.Fatalf("stale RUNNING task status = %q, want %q (should be left for NATS redelivery)", got, common.RUNNING)
	}
	// Fresh RUNNING row belongs to a live worker — untouched.
	if got := statusOf("task-run-fresh"); got != common.RUNNING {
		t.Fatalf("fresh RUNNING task status = %q, want %q", got, common.RUNNING)
	}
	// CREATED orphans are only re-published; their status stays CREATED so
	// the normal consume path (StartRunning) owns the transition.
	if got := statusOf("task-created-stale"); got != common.CREATED {
		t.Fatalf("stale CREATED task status = %q, want %q", got, common.CREATED)
	}
	if got := statusOf("task-created-fresh"); got != common.CREATED {
		t.Fatalf("fresh CREATED task status = %q, want %q", got, common.CREATED)
	}

	// Exactly the stale CREATED task was re-enqueued — fresh CREATED rows
	// still have their original message in flight, and terminal rows are
	// never resurrected by the scan.
	published := recorder.published()
	if len(published) != 1 || published[0] != "task-created-stale" {
		t.Fatalf("re-enqueued tasks = %v, want [task-created-stale]", published)
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
		ID: taskID, DocumentID: docID, DatasetID: "kb-1", Status: common.RUNNING,
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
