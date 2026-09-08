//go:build integration

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
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	natsengine "ragflow/internal/engine/nats"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/testutil"

	"github.com/nats-io/nats-server/v2/server"
)

func setupRealNatsCluster(t *testing.T) (host string, port int) {
	t.Helper()
	opts := &server.Options{
		Port:      -1,
		JetStream: true,
		StoreDir:  t.TempDir(),
		NoLog:     true,
		NoSigs:    true,
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("create embedded NATS server: %v", err)
	}
	ns.Start()
	if !ns.ReadyForConnections(10 * time.Second) {
		ns.Shutdown()
		t.Fatal("embedded NATS server did not become ready within 10s")
	}
	t.Cleanup(func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	})
	addr := ns.Addr().(*net.TCPAddr)
	return "127.0.0.1", addr.Port
}

// TestIntegration_MultiInstanceSharedConsumerNoOverExecution verifies TaskRP.md §6.2:
// Multiple ingestor instances sharing the same JetStream durable consumer (RAGFLOW_CONSUMER)
// compete for tasks without duplicate or overlapping execution.
func TestIntegration_MultiInstanceSharedConsumerNoOverExecution(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	host, port := setupRealNatsCluster(t)

	// Engine 1 initializes stream and consumer
	engine1 := natsengine.NewNatsEngine(host, port)
	if err := engine1.Init(); err != nil {
		t.Fatalf("engine1.Init: %v", err)
	}
	if err := engine1.InitConsumer(common.TaskSubject); err != nil {
		t.Fatalf("engine1.InitConsumer: %v", err)
	}

	// Engine 2 connects to the same stream and consumer
	engine2 := natsengine.NewNatsEngine(host, port)
	if err := engine2.Init(); err != nil {
		t.Fatalf("engine2.Init: %v", err)
	}
	if err := engine2.InitConsumer(common.TaskSubject); err != nil {
		t.Fatalf("engine2.InitConsumer: %v", err)
	}

	const taskCount = 6
	taskIDs := seedBurstTasks(t, db, taskCount)
	for _, id := range taskIDs {
		if err := db.Model(&entity.IngestionTask{}).Where("id = ?", id).
			Update("status", common.SCHEDULED).Error; err != nil {
			t.Fatalf("schedule task %s: %v", id, err)
		}
	}

	var mu sync.Mutex
	executionCounts := make(map[string]int)
	var activeConcurrentRuns atomic.Int32
	var maxConcurrentObserved atomic.Int32

	runner := func(ctx context.Context, task *entity.IngestionTask) error {
		cur := activeConcurrentRuns.Add(1)
		defer activeConcurrentRuns.Add(-1)
		for {
			oldMax := maxConcurrentObserved.Load()
			if cur <= oldMax || maxConcurrentObserved.CompareAndSwap(oldMax, cur) {
				break
			}
		}

		mu.Lock()
		executionCounts[task.ID]++
		mu.Unlock()

		time.Sleep(30 * time.Millisecond)
		return nil
	}

	ingestor1 := newUnitIngestor("instance-1", 2, []string{"pdf"})
	ingestor1.runDocumentTask = runner

	ingestor2 := newUnitIngestor("instance-2", 2, []string{"pdf"})
	ingestor2.runDocumentTask = runner

	// Point global MQ engine to engine1 then start ingestor1, then engine2 and start ingestor2
	previousEngine := engine.GetMessageQueueEngine()
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousEngine) })

	engine.SetMessageQueueEngine(engine1)
	if err := ingestor1.Start(); err != nil {
		t.Fatalf("ingestor1.Start: %v", err)
	}
	t.Cleanup(func() { ingestor1.Stop(context.Background()) })

	engine.SetMessageQueueEngine(engine2)
	if err := ingestor2.Start(); err != nil {
		t.Fatalf("ingestor2.Start: %v", err)
	}
	t.Cleanup(func() { ingestor2.Stop(context.Background()) })

	// Publish tasks
	for _, id := range taskIDs {
		payload, err := json.Marshal(common.TaskMessage{
			TaskID:   id,
			TaskType: common.TaskTypeIngestionTask,
		})
		if err != nil {
			t.Fatalf("marshal task %s: %v", id, err)
		}
		if err := engine1.PublishTask(common.TaskSubject, payload); err != nil {
			t.Fatalf("publish task %s: %v", id, err)
		}
	}

	// Wait for all tasks to be completed in DB
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		var completedCount int64
		db.Model(&entity.IngestionTask{}).Where("id IN ? AND status = ?", taskIDs, common.COMPLETED).Count(&completedCount)
		if int(completedCount) == taskCount {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	for _, id := range taskIDs {
		if count := executionCounts[id]; count != 1 {
			t.Fatalf("task %s execution count = %d, want 1 (exactly-once without duplicate execution)", id, count)
		}
	}
	t.Logf("Multi-instance executed %d tasks cleanly. Max concurrency across instances = %d", taskCount, maxConcurrentObserved.Load())
}

// TestIntegration_SlowTaskHeartbeatPreventsPrematureRedelivery verifies TaskRP.md §6.2:
// Under the NATS consumer's BackOff schedule (first retry window = 5s), a task whose execution
// takes longer than 5s is NOT redelivered mid-flight because the worker's heartbeat calls InProgress.
func TestIntegration_SlowTaskHeartbeatPreventsPrematureRedelivery(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	host, port := setupRealNatsCluster(t)
	mq := natsengine.NewNatsEngine(host, port)
	if err := mq.Init(); err != nil {
		t.Fatalf("mq.Init: %v", err)
	}
	if err := mq.InitConsumer(common.TaskSubject); err != nil {
		t.Fatalf("mq.InitConsumer: %v", err)
	}

	_, _, _, taskID := testutil.SeedTestData(t, db)
	if err := db.Model(&entity.IngestionTask{}).Where("id = ?", taskID).
		Update("status", common.SCHEDULED).Error; err != nil {
		t.Fatalf("schedule task %s: %v", taskID, err)
	}

	ingestor := newUnitIngestor("slow-task-ingestor", 1, []string{"pdf"})
	ingestor.heartbeatInterval = 1 * time.Second // comfortably below BackOff[0] = 5s

	taskExecutionStarted := make(chan struct{})
	var executions atomic.Int32

	ingestor.runDocumentTask = func(ctx context.Context, task *entity.IngestionTask) error {
		executions.Add(1)
		close(taskExecutionStarted)
		// Simulate a slow task that runs for 6 seconds (> 5s BackOff[0])
		select {
		case <-time.After(6 * time.Second):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	previousEngine := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(mq)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousEngine) })

	if err := ingestor.Start(); err != nil {
		t.Fatalf("ingestor.Start: %v", err)
	}
	t.Cleanup(func() { ingestor.Stop(context.Background()) })

	payload, err := json.Marshal(common.TaskMessage{
		TaskID:   taskID,
		TaskType: common.TaskTypeIngestionTask,
	})
	if err != nil {
		t.Fatalf("marshal task: %v", err)
	}
	if err := mq.PublishTask(common.TaskSubject, payload); err != nil {
		t.Fatalf("publish task: %v", err)
	}

	select {
	case <-taskExecutionStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("task execution did not start")
	}

	// While task is still running at t = 5.5s (after BackOff[0] elapsed):
	// A manual pull on the same consumer must find NO messages available,
	// because the heartbeat extended the lease.
	time.Sleep(5500 * time.Millisecond)

	handles, err := mq.PullMessagesForAdmin(1)
	if err != nil {
		t.Fatalf("PullMessagesForAdmin: %v", err)
	}
	if len(handles) != 0 {
		t.Fatalf("expected 0 redelivered messages while task is running with heartbeat, got %d", len(handles))
	}

	// Wait for task completion
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		var task entity.IngestionTask
		if err := db.Where("id = ?", taskID).First(&task).Error; err == nil && task.Status == common.COMPLETED {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if executions.Load() != 1 {
		t.Fatalf("task executions = %d, want 1 (heartbeat should prevent second delivery)", executions.Load())
	}
}

// TestIntegration_ShutdownRedeliveryRecovery verifies TaskRP.md §6.2 item 4:
// After an ingestor shutdown times out under full load (SIGTERM simulation):
//   - Already finished tasks are settled (Acked) and never redelivered;
//   - In-flight uncompleted tasks have their leases abandoned (stopActiveLeases),
//     and are redelivered by the broker to a successor ingestor and completed.
func TestIntegration_ShutdownRedeliveryRecovery(t *testing.T) {
	db := testutil.SetupTestDB(t)
	cleanup := testutil.ReplaceDBForTest(t, db)
	defer cleanup()

	host, port := setupRealNatsCluster(t)
	mq := natsengine.NewNatsEngine(host, port)
	if err := mq.Init(); err != nil {
		t.Fatalf("mq.Init: %v", err)
	}
	if err := mq.InitConsumer(common.TaskSubject); err != nil {
		t.Fatalf("mq.InitConsumer: %v", err)
	}

	previousEngine := engine.GetMessageQueueEngine()
	engine.SetMessageQueueEngine(mq)
	t.Cleanup(func() { engine.SetMessageQueueEngine(previousEngine) })

	// Seed two tasks: taskA (fast) and taskB (slow, will be in-flight during SIGTERM)
	taskIDs := seedBurstTasks(t, db, 2)
	taskIDA := taskIDs[0]
	taskIDB := taskIDs[1]

	for _, id := range []string{taskIDA, taskIDB} {
		if err := db.Model(&entity.IngestionTask{}).Where("id = ?", id).
			Update("status", common.SCHEDULED).Error; err != nil {
			t.Fatalf("schedule task %s: %v", id, err)
		}
	}

	ingestor1 := newUnitIngestor("shutdown-ingestor-1", 2, []string{"pdf"})
	ingestor1.heartbeatInterval = 500 * time.Millisecond

	taskBStarted := make(chan struct{})
	var (
		taskAExecutions atomic.Int32
		taskBExecutions atomic.Int32
	)

	ingestor1.runDocumentTask = func(ctx context.Context, task *entity.IngestionTask) error {
		if task.ID == taskIDA {
			taskAExecutions.Add(1)
			return nil
		}
		if task.ID == taskIDB {
			taskBExecutions.Add(1)
			close(taskBStarted)
			// Simulate long-running task interrupted by shutdown
			select {
			case <-time.After(30 * time.Second):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	}

	if err := ingestor1.Start(); err != nil {
		t.Fatalf("ingestor1.Start: %v", err)
	}

	// Publish taskA and taskB
	for _, id := range []string{taskIDA, taskIDB} {
		payload, err := json.Marshal(common.TaskMessage{
			TaskID:   id,
			TaskType: common.TaskTypeIngestionTask,
		})
		if err != nil {
			t.Fatalf("marshal %s: %v", id, err)
		}
		if err := mq.PublishTask(common.TaskSubject, payload); err != nil {
			t.Fatalf("publish %s: %v", id, err)
		}
	}

	// Wait for taskB to start execution and taskA to complete
	<-taskBStarted
	deadlineA := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadlineA) {
		var taskA entity.IngestionTask
		if err := db.Where("id = ?", taskIDA).First(&taskA).Error; err == nil && taskA.Status == common.COMPLETED {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Simulate SIGTERM with 200ms graceful shutdown timeout (which will time out for taskB)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer stopCancel()
	ingestor1.Stop(stopCtx)

	// Ingestor1 timed out and cancelled taskB, calling stopActiveLeases().
	// Now start ingestor2 to recover un-acked tasks from the shared consumer.
	ingestor2 := newUnitIngestor("recovery-ingestor-2", 2, []string{"pdf"})
	ingestor2.heartbeatInterval = 500 * time.Millisecond

	var (
		taskAExec2 atomic.Int32
		taskBExec2 atomic.Int32
	)
	ingestor2.runDocumentTask = func(ctx context.Context, task *entity.IngestionTask) error {
		if task.ID == taskIDA {
			taskAExec2.Add(1)
			return nil
		}
		if task.ID == taskIDB {
			taskBExec2.Add(1)
			return nil
		}
		return nil
	}

	if err := ingestor2.Start(); err != nil {
		t.Fatalf("ingestor2.Start: %v", err)
	}
	defer ingestor2.Stop(context.Background())

	// Wait for taskB to be redelivered and completed by ingestor2
	deadlineB := time.Now().Add(12 * time.Second)
	taskBCompleted := false
	for time.Now().Before(deadlineB) {
		var taskB entity.IngestionTask
		if err := db.Where("id = ?", taskIDB).First(&taskB).Error; err == nil && taskB.Status == common.COMPLETED {
			taskBCompleted = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !taskBCompleted {
		t.Fatalf("taskB was not redelivered and completed after ingestor1 shutdown")
	}

	// Assertions:
	// 1. taskA was settled by ingestor1 and NEVER executed by ingestor2 (no duplicate delivery)
	if taskAExec2.Load() != 0 {
		t.Fatalf("taskA was redelivered to ingestor2 (%d times), but it was already completed and Acked", taskAExec2.Load())
	}
	// 2. taskB was executed once on ingestor2 to completion
	if taskBExec2.Load() != 1 {
		t.Fatalf("taskB executions on ingestor2 = %d, want 1", taskBExec2.Load())
	}
}
