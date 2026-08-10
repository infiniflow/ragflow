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

package syncer

import (
	"context"
	"fmt"
	"ragflow/internal/service"
	"sync"
	"time"
)

// TaskWorker consumes claimed task envelopes and runs coordinators.
type TaskWorker struct {
	queue       <-chan TaskEnvelope
	taskService *service.SyncTaskService
	coordinator *TaskCoordinator
	locker      ConnectorLocker
}

// NewTaskWorker creates a bounded task worker pool.
func NewTaskWorker(queue <-chan TaskEnvelope, taskService *service.SyncTaskService, coordinator *TaskCoordinator, locker ConnectorLocker) *TaskWorker {
	return &TaskWorker{queue: queue, taskService: taskService, coordinator: coordinator, locker: locker}
}

// Run starts worker goroutines and blocks until context cancellation.
func (w *TaskWorker) Run(ctx context.Context, concurrency int) {
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w.loop(ctx)
		}()
	}
	<-ctx.Done()
	wg.Wait()
}

// loop handles queue messages until cancellation.
func (w *TaskWorker) loop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case envelope := <-w.queue:
			w.handle(ctx, envelope)
		}
	}
}

// handle loads a claimed task and executes it under the connector lock.
func (w *TaskWorker) handle(ctx context.Context, envelope TaskEnvelope) {
	taskContext, err := w.taskService.GetContext(ctx, envelope.TaskID)
	if err != nil {
		if ctx.Err() != nil {
			_ = w.taskService.RescheduleClaimed(context.WithoutCancel(ctx), envelope.TaskID)
			return
		}
		_ = w.taskService.Fail(ctx, envelope.TaskID, "", err)
		return
	}
	if !w.locker.TryLock(taskContext.Connector.ID) {
		_ = w.taskService.RescheduleClaimed(ctx, taskContext.Task.ID)
		return
	}
	defer w.locker.Unlock(taskContext.Connector.ID)

	startedAt := time.Now()
	if err = w.coordinator.Execute(ctx, taskContext); err != nil {
		logTemporarySyncTaskDuration(taskContext, startedAt)
		if ctx.Err() != nil {
			_ = w.taskService.RescheduleClaimed(context.WithoutCancel(ctx), taskContext.Task.ID)
			return
		}
		_ = w.taskService.Fail(ctx, taskContext.Task.ID, taskContext.Connector.ID, fmt.Errorf("sync task failed: %w", err))
		return
	}
	logTemporarySyncTaskDuration(taskContext, startedAt)
}
