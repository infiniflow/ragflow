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
	"ragflow/internal/dao"
	"ragflow/internal/service"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TaskWorker consumes claimed task envelopes and runs coordinators.
type TaskWorker struct {
	queue       <-chan TaskEnvelope
	taskDAO     *dao.SyncTaskDAO
	taskService *service.SyncTaskService
	coordinator *TaskCoordinator
	locker      ConnectorLocker
	scheduler   *Scheduler
}

// NewTaskWorker creates a bounded task worker pool.
func NewTaskWorker(queue <-chan TaskEnvelope, taskDAO *dao.SyncTaskDAO, taskService *service.SyncTaskService, coordinator *TaskCoordinator, locker ConnectorLocker) *TaskWorker {
	return &TaskWorker{queue: queue, taskDAO: taskDAO, taskService: taskService, coordinator: coordinator, locker: locker}
}

// WithScheduler attaches the event scheduler used for one-shot task publishing.
func (w *TaskWorker) WithScheduler(scheduler *Scheduler) *TaskWorker {
	w.scheduler = scheduler
	return w
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
			w.scheduleRetryIfTaskScheduled(ctx, envelope.TaskID, 3*time.Second)
			return
		}
	}

	// get the whole context by task_id from nats
	taskContext, err := w.taskDAO.GetTaskContext(ctx, envelope.TaskID)
	if err != nil {
		if ctx.Err() != nil { // exiting
			if err = w.rescheduleClaimed(context.WithoutCancel(ctx), envelope.TaskID); err != nil {
				common.Warn("syncer task reschedule failed after context cancellation", zap.String("task_id", envelope.TaskID), zap.Error(err))
			}
			nackEnvelope(envelope)
			return
		}
		if failErr := w.taskDAO.FailTask(ctx, envelope.TaskID, "", syncTaskErrorMessage(err), 1); failErr != nil { // getContext failed
			if err = w.rescheduleClaimed(context.WithoutCancel(ctx), envelope.TaskID); err != nil {
				common.Warn("syncer task reschedule failed after context load failure", zap.String("task_id", envelope.TaskID), zap.Error(err))
			}
			nackEnvelope(envelope)
			return
		}
		ackEnvelope(envelope)
		return
	}

	// lock the connector and the KB
	lease, locked := w.locker.TryLock(taskContext.Connector.ID, taskContext.Knowledgebase.ID)
	if !locked {
		if err = w.rescheduleClaimed(context.WithoutCancel(ctx), taskContext.Task.ID); err != nil {
			common.Warn("syncer task reschedule failed after lock contention", zap.String("task_id", taskContext.Task.ID), zap.Error(err))
			nackEnvelope(envelope)
			return
		}
		w.scheduleRetry(ctx, taskContext.Task.ID, 3*time.Second)
		ackEnvelope(envelope)
		return
	}
	defer w.locker.Unlock(taskContext.Connector.ID, taskContext.Knowledgebase.ID)

	startedAt := time.Now()
	outcome, err := w.coordinator.Execute(ctx, taskContext, lease)
	if err != nil { // execute the task(sync/ prune)
		logSyncTaskDuration(taskContext, startedAt) // test sync task run time
		if errors.Is(err, errSyncTaskCanceled) {    // task is canceled by the user
			ackEnvelope(envelope)
			return
		}
		if ctx.Err() != nil { // the task is canceled by system, this need to rerun
			if err = w.rescheduleClaimed(context.WithoutCancel(ctx), taskContext.Task.ID); err != nil {
				common.Warn("syncer task reschedule failed after execution cancellation", zap.String("task_id", taskContext.Task.ID), zap.Error(err))
				nackEnvelope(envelope)
				return
			}
			w.scheduleRetry(context.WithoutCancel(ctx), taskContext.Task.ID, 3*time.Second)
			ackEnvelope(envelope)
			return
		}
		if isTransientSyncError(err) {
			attempts, failed, transientErr := w.taskDAO.HandleTransientFailure(ctx, taskContext.Task.ID, taskContext.Connector.ID, syncTaskErrorMessage(err), maxTransientTaskRetries)
			if transientErr != nil {
				if err = w.rescheduleClaimed(context.WithoutCancel(ctx), taskContext.Task.ID); err != nil {
					common.Warn("syncer task reschedule failed after transient failure handling error", zap.String("task_id", taskContext.Task.ID), zap.Error(err))
				}
				nackEnvelope(envelope)
				return
			}
			logTransientSyncRetry(taskContext, attempts, failed, err)
			if !failed {
				w.scheduleRetry(ctx, taskContext.Task.ID, transientRetryDelay(attempts))
			}
			ackEnvelope(envelope)
			return
		}
		if failErr := w.taskDAO.FailTask(ctx, taskContext.Task.ID, taskContext.Connector.ID, syncTaskErrorMessage(fmt.Errorf("sync task failed: %w", err)), 1); failErr != nil {
			if err = w.rescheduleClaimed(context.WithoutCancel(ctx), taskContext.Task.ID); err != nil {
				common.Warn("syncer task reschedule failed after terminal failure handling error", zap.String("task_id", taskContext.Task.ID), zap.Error(err))
			}
			nackEnvelope(envelope)
			return
		}
		ackEnvelope(envelope)
		return
	}
	logSyncTaskDuration(taskContext, startedAt)
	w.scheduleNext(ctx, outcome.NextTaskID)
	ackEnvelope(envelope)
}

func (w *TaskWorker) scheduleNext(ctx context.Context, taskID string) {
	if w.scheduler == nil || taskID == "" {
		return
	}
	task, err := w.taskDAO.GetScheduledTask(ctx, taskID)
	if err != nil {
		common.Warn("syncer schedule next task lookup failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if err = w.scheduler.ScheduleTask(ctx, task); err != nil {
		common.Warn("syncer schedule next task failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

// scheduleRetry schedule retry when web error occurred
func (w *TaskWorker) scheduleRetry(ctx context.Context, taskID string, delay time.Duration) {
	if w.scheduler == nil || taskID == "" {
		return
	}
	if err := w.scheduler.ScheduleTaskAfter(ctx, taskID, delay); err != nil {
		common.Warn("syncer retry task publish failed", zap.String("task_id", taskID), zap.Duration("delay", delay), zap.Error(err))
	}
}

func (w *TaskWorker) scheduleRetryIfTaskScheduled(ctx context.Context, taskID string, delay time.Duration) {
	if w.scheduler == nil || taskID == "" {
		return
	}
	taskContext, err := w.taskDAO.GetTaskContext(ctx, taskID)
	if err != nil {
		common.Warn("syncer retry task lookup failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if taskContext.Task.Status != dao.SyncStatusSchedule {
		return
	}
	w.scheduleRetry(ctx, taskID, delay)
}

func (w *TaskWorker) rescheduleClaimed(ctx context.Context, taskID string) error {
	if w == nil || w.taskDAO == nil || taskID == "" {
		return nil
	}
	return w.taskDAO.RescheduleClaimed(ctx, taskID)
}

func syncTaskErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// transientRetryDelay return retry delay
func transientRetryDelay(attempts int64) time.Duration {
	if attempts < 1 {
		attempts = 1
	}

	shift := attempts - 1
	if shift > 5 {
		shift = 5
	}
	return time.Duration(1<<shift) * 30 * time.Second
}

func logTransientSyncRetry(taskContext dao.SyncTaskContext, attempts int64, failed bool, err error) {
	message := "sync task transient retry scheduled"
	if failed {
		message = "sync task failed after transient retries"
	}
	common.Warn(
		message,
		zap.String("task_id", taskContext.Task.ID),
		zap.String("connector_id", taskContext.Connector.ID),
		zap.String("kb_id", taskContext.Knowledgebase.ID),
		zap.String("source", taskContext.Connector.Source),
		zap.Int64("attempts", attempts),
		zap.Int64("max_retries", maxTransientTaskRetries),
		zap.Error(err),
	)
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
