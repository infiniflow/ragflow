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
	"ragflow/internal/service"
	"time"
)

// TaskEnvelope is the only payload sent through the task queue.
type TaskEnvelope struct {
	TaskID        string
	Handle        common.TaskHandle
	stopHeartbeat func()
}

// SyncTaskBroker publishes and pulls syncer task wake-up messages.
type SyncTaskBroker interface {
	InitSyncerStream() error
	InitSyncerConsumer() error
	PublishSyncerTask(taskID string) error
	FetchSyncerTasks(batchSize int) ([]common.TaskHandle, error)
}

// Scheduler discovers due work and enqueues task IDs for workers.
type Scheduler struct {
	pollInterval time.Duration
	queue        chan<- TaskEnvelope
	taskService  *service.SyncTaskService
	broker       SyncTaskBroker
}

// NewScheduler creates a global scheduler for datasource sync tasks.
func NewScheduler(pollInterval time.Duration, queue chan<- TaskEnvelope, taskService *service.SyncTaskService) *Scheduler {
	return &Scheduler{pollInterval: pollInterval, queue: queue, taskService: taskService}
}

// NewNATSScheduler creates a JetStream-driven scheduler with DB reconciliation.
func NewNATSScheduler(pollInterval time.Duration, queue chan<- TaskEnvelope, taskService *service.SyncTaskService, broker SyncTaskBroker) *Scheduler {
	return &Scheduler{
		pollInterval: pollInterval,
		queue:        queue,
		taskService:  taskService,
		broker:       broker,
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

	// scan DB for first time run
	if err := s.publishStartupTasks(ctx); err != nil && ctx.Err() == nil {
		common.Error("syncer scheduler startup publish failed", err)
	}

	ticker := time.NewTicker(s.pollInterval)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C: // Run `publishDueTasks` periodically
			if err := s.publishDueTasks(ctx); err != nil && ctx.Err() == nil {
				common.Error("syncer scheduler due publish failed", err)
			}
			continue
		default:
		}

		// check `scheduler`'s task queue's slot
		available := s.queueAvailable()
		if available <= 0 {
			if err := waitNATSFetchCapacity(ctx); err != nil {
				return err
			}
			continue
		}

		// pull `available` tasks from nats
		handles, err := s.broker.FetchSyncerTasks(available)
		if err != nil {
			common.Error("syncer scheduler fetch failed", err)
			if waitErr := waitNATSFetchCapacity(ctx); waitErr != nil {
				return waitErr
			}
		} else if len(handles) == 0 {
			if waitErr := waitNATSFetchCapacity(ctx); waitErr != nil {
				return waitErr
			}
		} else if err = s.enqueueHandles(ctx, handles); err != nil { // put tasks to task queue
			return err
		}
	}
}

func waitNATSFetchCapacity(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

// enqueueHandles put task to `scheduler`'s task queue
func (s *Scheduler) enqueueHandles(ctx context.Context, handles []common.TaskHandle) error {
	for index, handle := range handles {
		message := handle.GetMessage()
		if message.TaskID == "" {
			_ = handle.Ack()
			continue
		}

		stopHeartbeat := startHandleHeartbeat(ctx, handle)
		select {
		case <-ctx.Done():
			stopHeartbeat()
			for _, pending := range handles[index:] {
				_ = pending.Nack()
			}
			return ctx.Err()
		case s.queue <- TaskEnvelope{TaskID: message.TaskID, Handle: handle, stopHeartbeat: stopHeartbeat}:
		}
	}
	return nil
}

func (s *Scheduler) queueAvailable() int {
	return cap(s.queue) - len(s.queue)
}

// publishStartupTasks scan DB for first time run
func (s *Scheduler) publishStartupTasks(ctx context.Context) error {
	// TODO restores running tasks during syncer startup.
	if err := s.taskService.RecoverRunning(ctx); err != nil {
		return err
	}

	tasks, err := s.taskService.ListStartupTasks(ctx)
	if err != nil {
		return err
	}

	// publish task to nats
	var publishErr error
	for _, task := range tasks {
		if err = s.broker.PublishSyncerTask(task.ID); err != nil {
			publishErr = errors.Join(publishErr, err)
		}
	}
	return publishErr
}

// publishDueTasks publishes due scheduled DB tasks to nats
func (s *Scheduler) publishDueTasks(ctx context.Context) error {
	now := time.Now()
	tasks, err := s.taskService.ListDueTasks(ctx, now)
	if err != nil {
		return err
	}

	var publishErr error
	for _, task := range tasks {
		if err = s.broker.PublishSyncerTask(task.ID); err != nil {
			publishErr = errors.Join(publishErr, err)
		}
	}
	return publishErr
}
