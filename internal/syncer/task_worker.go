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
	"errors"
	"fmt"
	"ragflow/internal/common"
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
	// start heartbeat, communication with nats
	if envelope.Handle != nil && envelope.stopHeartbeat == nil {
		envelope.stopHeartbeat = startHandleHeartbeat(ctx, envelope.Handle)
	}
	defer stopEnvelopeHeartbeat(envelope)

	if envelope.Handle != nil {
		// claim a task(sync/ prune)
		claimed, err := w.taskService.Claim(ctx, envelope.TaskID)
		if err != nil {
			_ = envelope.Handle.Nack()
			return
		}
		if !claimed {
			_ = envelope.Handle.Ack() // this task has been claimed by other worker
			return
		}
	}

	// get the whole context by task_id from nats
	taskContext, err := w.taskService.GetContext(ctx, envelope.TaskID)
	if err != nil {
		if ctx.Err() != nil { // exiting
			_ = w.taskService.RescheduleClaimed(context.WithoutCancel(ctx), envelope.TaskID)
			nackEnvelope(envelope)
			return
		}
		if failErr := w.taskService.Fail(ctx, envelope.TaskID, "", err); failErr != nil { // getContext failed
			_ = w.taskService.RescheduleClaimed(context.WithoutCancel(ctx), envelope.TaskID)
			nackEnvelope(envelope)
			return
		}
		ackEnvelope(envelope)
		return
	}

	// lock the connector and the KB
	lease, locked := w.locker.TryLock(taskContext.Connector.ID, taskContext.Knowledgebase.ID)
	if !locked {
		_ = w.taskService.RescheduleClaimed(ctx, taskContext.Task.ID)
		ackEnvelope(envelope)
		return
	}
	defer w.locker.Unlock(taskContext.Connector.ID, taskContext.Knowledgebase.ID)

	startedAt := time.Now()
	if err = w.coordinator.Execute(ctx, taskContext, lease); err != nil { // execute the task(sync/ prune)
		logSyncTaskDuration(taskContext, startedAt)
		if errors.Is(err, errSyncTaskCanceled) { // task is canceled by the user
			ackEnvelope(envelope)
			return
		}
		if ctx.Err() != nil { // the task is canceled by system, this need to rerun
			_ = w.taskService.RescheduleClaimed(context.WithoutCancel(ctx), taskContext.Task.ID)
			nackEnvelope(envelope)
			return
		}
		if failErr := w.taskService.Fail(ctx, taskContext.Task.ID, taskContext.Connector.ID, fmt.Errorf("sync task failed: %w", err)); failErr != nil {
			_ = w.taskService.RescheduleClaimed(context.WithoutCancel(ctx), taskContext.Task.ID)
			nackEnvelope(envelope)
			return
		}
		ackEnvelope(envelope)
		return
	}
	logSyncTaskDuration(taskContext, startedAt) // Todo delete soon
	ackEnvelope(envelope)
}

func ackEnvelope(envelope TaskEnvelope) {
	if envelope.Handle != nil {
		_ = envelope.Handle.Ack()
	}
}

func nackEnvelope(envelope TaskEnvelope) {
	if envelope.Handle != nil {
		_ = envelope.Handle.Nack()
	}
}

func stopEnvelopeHeartbeat(envelope TaskEnvelope) {
	if envelope.stopHeartbeat != nil {
		envelope.stopHeartbeat()
	}
}

// startHandleHeartbeat start handle heartbeat
func startHandleHeartbeat(ctx context.Context, handle common.TaskHandle) func() {
	if handle == nil {
		return func() {}
	}

	done := make(chan struct{})
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				return
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = handle.InProgress()
			}
		}
	}()
	return func() {
		close(done)
		<-stopped
	}
}
