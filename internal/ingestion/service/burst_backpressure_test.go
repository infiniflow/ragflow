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
//
//  This test reproduces the "20+ files selected, only some parse, the rest
//  get stuck forever" report against the Go ingestor (canvas / pipeline path).
//
//  Root cause it guards against: processMessage applies backpressure by
//  NACKing the message when taskChan is full. The NATS consumer is configured
//  with MaxDeliver: 16 and NO dead-letter subject (internal/engine/nats/nats.go),
//  so a message NACKed 16 times is permanently dropped. By that point
//  StartRunning has already flipped the task (and its document) to RUNNING in
//  the DB, and the ingestor has no scan-and-re-enqueue path on completion — so
//  the dropped document is stuck in RUNNING forever.
//
//  The test drives the REAL processMessage + REAL worker pool, but substitutes
//  a deterministic "broker" that mirrors NATS' contract: bounded redelivery
//  (MaxDeliver) with no dead-letter. A slow worker models a saturated pipeline
//  (e.g. LLM calls in a canvas). The invariant under test is that NO task is
//  ever lost: all N delivered tasks eventually reach COMPLETED.
//
//  Under the buggy (NACK-on-backpressure) code, the burst overflow is dropped
//  after MaxDeliver and the test FAILS. After the fix (blocking send so the
//  consume loop applies backpressure instead of dropping), every task is
//  processed and the test PASSES.

package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	"ragflow/internal/ingestion/testutil"

	"gorm.io/gorm"
)

const (
	burstTaskCount  = 20
	burstMaxDeliver = 16 // mirrors NATS MaxDeliver in internal/engine/nats/nats.go
	burstWorkerMs   = 50 // slow worker models a saturated pipeline
)

// burstMsg models one in-flight broker message: the handle processMessage
// settles, plus how many times it has been delivered (1 = initial delivery).
type burstMsg struct {
	handle     *fakeTaskHandle
	deliveries int
}

// TestProcessMessage_BurstNoTaskLossUnderBackpressure drives a 20-file burst
// through the real processMessage + worker pool with a simulated NATS broker
// (bounded redelivery, no dead-letter). It asserts that every task completes
// and none are dropped.
func TestProcessMessage_BurstNoTaskLossUnderBackpressure(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	taskIDs := seedBurstTasks(t, db, burstTaskCount)

	ingestor := newUnitIngestor("test", 1, []string{"pdf"})
	// Slow worker: model a saturated pipeline so the channel is full during
	// the burst, forcing the backpressure path.
	ingestor.runDocumentTask = func(ctx context.Context, _ *entity.IngestionTask) error {
		time.Sleep(burstWorkerMs * time.Millisecond)
		return nil
	}
	ingestor.startWorkerPool()
	defer ingestor.Stop(context.Background())

	// Simulated NATS broker: pending holds messages waiting to be delivered.
	var mu sync.Mutex
	var pending []*burstMsg
	dropped := 0

	seedBroker := func() {
		mu.Lock()
		defer mu.Unlock()
		for _, id := range taskIDs {
			pending = append(pending, &burstMsg{
				handle:     newFakeHandle(id, common.TaskTypeIngestionTask),
				deliveries: 1,
			})
		}
	}
	seedBroker()

	// Simulated consume loop: mirrors consumeLoop -> GetMessages(4) ->
	// processMessage, with NATS-style bounded redelivery on Nack.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-stop:
				return
			default:
			}
			mu.Lock()
			if len(pending) == 0 {
				mu.Unlock()
				time.Sleep(5 * time.Millisecond) // idle backoff
				continue
			}
			batchSize := 4
			if len(pending) < batchSize {
				batchSize = len(pending)
			}
			batch := pending[:batchSize]
			pending = pending[batchSize:]
			mu.Unlock()

			for _, bm := range batch {
				ingestor.processMessage(bm.handle)
				if bm.handle.nacks.Load() > 0 {
					mu.Lock()
					bm.deliveries++
					if bm.deliveries < burstMaxDeliver {
						// Redeliver (no delay — worst case for loss).
						pending = append(pending, bm)
					} else {
						// NATS drops after MaxDeliver with no dead-letter:
						// the task is permanently lost.
						dropped++
					}
					mu.Unlock()
				}
			}
		}
	}()

	// Wait until all tasks reach COMPLETED, or time out.
	deadline := time.Now().Add(30 * time.Second)
	completed := 0
	for time.Now().Before(deadline) {
		completed = countTasksWithStatus(t, db, taskIDs, common.COMPLETED)
		if completed == burstTaskCount {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	close(stop)
	wg.Wait()

	if dropped > 0 {
		t.Fatalf("BUG: %d/%d task(s) were permanently dropped (lost) under burst backpressure "+
			"with MaxDeliver=%d and no dead-letter; only %d/%d completed. "+
			"This is the 'files get stuck after the first few parse' defect.",
			dropped, burstTaskCount, burstMaxDeliver, completed, burstTaskCount)
	}
	if completed != burstTaskCount {
		t.Fatalf("expected all %d tasks to complete, got %d (stuck in RUNNING)",
			burstTaskCount, completed)
	}
}

// TestFetchBudget_NeverExceedsChannelCapacity: the consume loop must never
// pull more messages than it can enqueue immediately. The fetch budget is
// min(free channel slots, maxConcurrency) with a floor of 1 so a fully
// saturated channel still polls (and lets blocked sends drain) instead of
// stalling until AckWait expires on in-flight messages.
func TestFetchBudget_NeverExceedsChannelCapacity(t *testing.T) {
	ingestor := newUnitIngestor("test", 2, []string{"pdf"}) // channel cap = 4
	defer ingestor.Stop(context.Background())

	if got := ingestor.fetchBudget(); got != 2 {
		t.Fatalf("empty channel: fetchBudget = %d, want 2 (capped by maxConcurrency)", got)
	}

	for i := 0; i < cap(ingestor.taskChan)-1; i++ {
		ingestor.taskChan <- taskpkg.NewTaskContextForScheduling(t.Context(), &entity.IngestionTask{ID: "filler"})
	}
	if got := ingestor.fetchBudget(); got != 1 {
		t.Fatalf("3/4-full channel: fetchBudget = %d, want 1 (one free slot)", got)
	}

	ingestor.taskChan <- taskpkg.NewTaskContextForScheduling(t.Context(), &entity.IngestionTask{ID: "filler"})
	if got := ingestor.fetchBudget(); got != 1 {
		t.Fatalf("full channel: fetchBudget = %d, want 1 (floor: must never fetch 0)", got)
	}

	// A wide pool is bounded by the channel, not the worker count.
	wide := newUnitIngestor("test-wide", 8, []string{"pdf"}) // channel cap = 16
	defer wide.Stop(context.Background())
	if got := wide.fetchBudget(); got != 8 {
		t.Fatalf("wide empty channel: fetchBudget = %d, want 8 (capped by maxConcurrency)", got)
	}
}

// seedBurstTasks creates one tenant/kb plus N (document, ingestion_task) pairs
// in CREATED status, mirroring how CreateAndEnqueue publishes a message for a
// freshly-created task. Returns the ingestion task IDs.
func seedBurstTasks(t *testing.T, db *gorm.DB, n int) []string {
	t.Helper()
	const tenantID, kbID = "burst-tenant", "burst-kb"
	if err := db.Create(&entity.Tenant{ID: tenantID, LLMID: "gpt-4", Status: testutil.StrPtr("1")}).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	if err := db.Create(&entity.Knowledgebase{
		ID: kbID, TenantID: tenantID, EmbdID: "embd-1", Status: testutil.StrPtr("1"), ParserConfig: entity.JSONMap{},
	}).Error; err != nil {
		t.Fatalf("create kb: %v", err)
	}

	ids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		docID := fmt.Sprintf("burst-doc-%d", i)
		taskID := fmt.Sprintf("burst-task-%d", i)
		loc := "doc_store/" + docID
		if err := db.Create(&entity.Document{
			ID: docID, KbID: kbID, Name: &docID, ParserID: "naive",
			ParserConfig: entity.JSONMap{}, PipelineID: testutil.StrPtr("flow-1"),
			Status: testutil.StrPtr("1"), Type: "pdf", Location: &loc,
		}).Error; err != nil {
			t.Fatalf("create doc %s: %v", docID, err)
		}
		if err := db.Create(&entity.IngestionTask{
			ID: taskID, UserID: "u1", DocumentID: docID, DatasetID: kbID, Status: common.CREATED,
		}).Error; err != nil {
			t.Fatalf("create ingestion task %s: %v", taskID, err)
		}
		ids = append(ids, taskID)
	}
	return ids
}

// countTasksWithStatus returns how many of the given task IDs have the given status.
func countTasksWithStatus(t *testing.T, db *gorm.DB, ids []string, status string) int {
	t.Helper()
	var count int64
	if err := db.Model(&entity.IngestionTask{}).
		Where("id IN ? AND status = ?", ids, status).
		Count(&count).Error; err != nil {
		t.Fatalf("count tasks: %v", err)
	}
	return int(count)
}
