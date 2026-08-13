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
	"ragflow/internal/common"
	"ragflow/internal/entity"
	"ragflow/internal/service"
	"sync"
	"time"

	"go.uber.org/zap"
)

// TaskEnvelope is the only payload sent through the task queue.
type TaskEnvelope struct {
	TaskID        string
	Handle        common.TaskHandle
	stopHeartbeat func()
}

// SyncTaskBroker publishes and subscribes to syncer task wake-up messages.
type SyncTaskBroker interface {
	InitSyncerStream() error
	InitSyncerConsumer() error
	PublishSyncerTask(taskID string) error
	PublishSyncerTaskWakeup(taskID string) error
	SubscribeSyncerTasks(ctx context.Context, handler func(common.TaskHandle)) error
}

// Scheduler discovers due work and enqueues task IDs for workers.
type Scheduler struct {
	queue       chan<- TaskEnvelope
	taskService *service.SyncTaskService
	broker      SyncTaskBroker
	timerMu     sync.Mutex
	timers      map[string]*time.Timer
}

// NewScheduler creates a global scheduler for datasource sync tasks.
func NewScheduler(queue chan<- TaskEnvelope, taskService *service.SyncTaskService) *Scheduler {
	return &Scheduler{queue: queue, taskService: taskService, timers: map[string]*time.Timer{}}
}

// NewNATSScheduler creates a JetStream-driven scheduler with DB reconciliation.
func NewNATSScheduler(queue chan<- TaskEnvelope, taskService *service.SyncTaskService, broker SyncTaskBroker) *Scheduler {
	return &Scheduler{
		queue:       queue,
		taskService: taskService,
		broker:      broker,
		timers:      map[string]*time.Timer{},
	}
}

// Run starts the NATS listener.
func (s *Scheduler) Run(ctx context.Context) error {
	if s.broker != nil {
		return s.runNATS(ctx)
	}
	return errors.New("syncer scheduler requires a NATS broker")
}

func (s *Scheduler) runNATS(ctx context.Context) error {
	if err := s.broker.InitSyncerStream(); err != nil {
		return err
	}
	if err := s.broker.InitSyncerConsumer(); err != nil {
		return err
	}
	if err := s.broker.SubscribeSyncerTasks(ctx, func(handle common.TaskHandle) {
		if err := s.enqueueHandle(ctx, handle); err != nil && ctx.Err() == nil {
			common.Error("syncer scheduler enqueue failed", err)
		}
	}); err != nil {
		return err
	}

	// scan DB for first time run
	if err := s.publishStartupTasks(ctx); err != nil && ctx.Err() == nil {
		common.Error("syncer scheduler startup publish failed", err)
	}

	<-ctx.Done()
	s.stopTimers()
	return ctx.Err()
}

func (s *Scheduler) enqueueHandle(ctx context.Context, handle common.TaskHandle) error {
	message := handle.GetMessage()
	if message.TaskID == "" {
		_ = handle.Ack()
		return nil
	}

	stopHeartbeat := startHandleHeartbeat(ctx, handle)
	select {
	case <-ctx.Done():
		stopHeartbeat()
		_ = handle.Nack()
		return ctx.Err()
	case s.queue <- TaskEnvelope{TaskID: message.TaskID, Handle: handle, stopHeartbeat: stopHeartbeat}:
		return nil
	}
}

func (s *Scheduler) queueAvailable() int {
	return cap(s.queue) - len(s.queue)
}

// publishStartupTasks scans DB once for startup reconciliation.
func (s *Scheduler) publishStartupTasks(ctx context.Context) error {
	if err := s.taskService.RecoverRunning(ctx); err != nil {
		return err
	}

	tasks, err := s.taskService.ListScheduledTasks(ctx)
	if err != nil {
		return err
	}

	for _, task := range tasks {
		if err = s.ScheduleTask(ctx, task); err != nil {
			return err
		}
	}
	return nil
}

// ScheduleTask publishes a due scheduled task or arms a one-shot timer.
func (s *Scheduler) ScheduleTask(ctx context.Context, task service.ScheduledSyncTask) error {
	delay, schedule := s.taskDelay(task, time.Now())
	if !schedule {
		return nil
	}
	return s.ScheduleTaskAfter(ctx, task.ID, delay)
}

// ScheduleTaskAfter publishes a task after delay.
func (s *Scheduler) ScheduleTaskAfter(ctx context.Context, taskID string, delay time.Duration) error {
	if s == nil || s.broker == nil || taskID == "" {
		return nil
	}
	if delay <= 0 {
		return s.publishTask(ctx, taskID)
	}
	s.timerMu.Lock()
	if existing := s.timers[taskID]; existing != nil {
		existing.Stop()
	}
	timer := time.AfterFunc(delay, func() {
		if err := s.publishTaskWakeup(ctx, taskID); err != nil && ctx.Err() == nil {
			common.Warn("syncer scheduler timer publish failed", zap.String("task_id", taskID), zap.Error(err))
			_ = s.ScheduleTaskAfter(ctx, taskID, 3*time.Second)
			return
		}
		s.timerMu.Lock()
		delete(s.timers, taskID)
		s.timerMu.Unlock()
	})
	s.timers[taskID] = timer
	s.timerMu.Unlock()
	return nil
}

func (s *Scheduler) publishTaskWakeup(ctx context.Context, taskID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.broker.PublishSyncerTaskWakeup(taskID); err != nil {
		common.Warn("syncer task wakeup publish failed", zap.String("task_id", taskID), zap.Error(err))
		return err
	}
	return nil
}

func (s *Scheduler) publishTask(ctx context.Context, taskID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.broker.PublishSyncerTask(taskID); err != nil {
		common.Warn("syncer task publish failed", zap.String("task_id", taskID), zap.Error(err))
		return err
	}
	return nil
}

func (s *Scheduler) stopTimers() {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	for taskID, timer := range s.timers {
		timer.Stop()
		delete(s.timers, taskID)
	}
}

// taskDelay reports the delay before publication and whether the task must be scheduled at all.
func (s *Scheduler) taskDelay(task service.ScheduledSyncTask, now time.Time) (time.Duration, bool) {
	freq := int64(0)
	switch task.TaskType {
	case service.TaskTypeSync:
		freq = task.ConnectorRefreshFreq
	case service.TaskTypePrune:
		if !syncerConfigBool(task.ConnectorConfig, "sync_deleted_files") {
			return 0, false
		}
		freq = task.ConnectorPruneFreq
	}
	if freq <= 0 || task.UpdateDate == nil {
		return 0, true
	}
	return task.UpdateDate.Add(time.Duration(freq) * time.Minute).Sub(now), true
}

func syncerConfigBool(config entity.JSONMap, key string) bool {
	value, ok := config[key]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed == "1" || typed == "true" || typed == "TRUE"
	default:
		return false
	}
}
