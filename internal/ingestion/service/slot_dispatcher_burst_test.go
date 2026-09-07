//
// Copyright 2026 The InfiniFlow Authors. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package service

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"
)

// TestSlotDispatcherBurstCompletesEveryTask protects against the former
// backpressure loss: a burst must stay in JetStream until a concrete worker
// slot is ready, rather than being Nacked repeatedly until MaxDeliver drops it.
func TestSlotDispatcherBurstCompletesEveryTask(t *testing.T) {
	const (
		taskCount   = 20
		concurrency = 1
	)

	db := testutil.SetupTestDB(t)
	cleanupDB := testutil.ReplaceDBForTest(t, db)
	defer cleanupDB()
	taskIDs := seedBurstTasks(t, db, taskCount)

	queue := testutil.SetupNatsEngine(t)
	previousQueue := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(queue)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousQueue) })

	ingestor := newUnitIngestor("test-slot-burst", concurrency, []string{"pdf"})
	firstTaskStarted := make(chan struct{})
	releaseFirstTask := make(chan struct{})
	var firstTask sync.Once
	var release sync.Once
	releaseFirst := func() {
		release.Do(func() { close(releaseFirstTask) })
	}
	t.Cleanup(releaseFirst)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		ingestor.Stop(ctx)
	})
	ingestor.runDocumentTask = func(context.Context, *entity.IngestionTask) error {
		firstTask.Do(func() {
			close(firstTaskStarted)
			<-releaseFirstTask
		})
		return nil
	}

	for _, taskID := range taskIDs {
		if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
			Update("status", common.SCHEDULED).Error; err != nil {
			t.Fatalf("schedule task %s: %v", taskID, err)
		}
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
	case <-firstTaskStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("first task did not start")
	}

	// The sole worker is busy, so the remaining 19 tasks must remain broker
	// pending. A buffered dispatcher used to Nack this burst until MaxDeliver.
	releaseFirst()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var completed int64
		if err := db.Model(&entity.IngestionTask{}).
			Where("id IN ? AND status = ?", taskIDs, common.COMPLETED).
			Count(&completed).Error; err != nil {
			t.Fatalf("count completed burst tasks: %v", err)
		}
		if completed == taskCount {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	incomplete := make([]string, 0, taskCount)
	for _, taskID := range taskIDs {
		var task entity.IngestionTask
		if err := db.Where("id = ?", taskID).First(&task).Error; err != nil {
			t.Fatalf("load burst task %s: %v", taskID, err)
		}
		if task.Status != common.COMPLETED {
			incomplete = append(incomplete, fmt.Sprintf("%s=%s", taskID, task.Status))
		}
	}
	t.Fatalf("burst left %d/%d task(s) incomplete: %v", len(incomplete), taskCount, incomplete)
}
