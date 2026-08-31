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
	"ragflow/internal/dao"
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"
	"time"
)

const connectorLockSafetyMargin = 5 * time.Second

// ConnectorRegistry opens registered connectors by source.
type ConnectorRegistry interface {
	// Open creates a connector for a task context.
	Open(ctx context.Context, taskContext dao.SyncTaskContext) (syncerconnector.Connector, error)
}

// TaskCoordinator owns one task execution window.
type TaskCoordinator struct {
	syncRunnerConfig SyncRunnerConfig
	taskDAO          *dao.SyncTaskDAO
	taskService      *service.SyncTaskService
	registry         ConnectorRegistry
	sink             service.DocumentSink
	pruneService     *service.SyncPruneService
	idResolver       *service.DocumentIDResolver
	executor         *SyncJobExecutor
	checkpoints      SyncCheckpointStore
}

// TaskOutcome describes post-run scheduling work for a completed task.
type TaskOutcome struct {
	NextTaskID string
}

// NewTaskCoordinator creates a coordinator for one claimed task at a time.
func NewTaskCoordinator(syncRunnerConfig SyncRunnerConfig, taskDAO *dao.SyncTaskDAO, taskService *service.SyncTaskService, registry ConnectorRegistry, sink service.DocumentSink, pruneService *service.SyncPruneService, idResolver *service.DocumentIDResolver, executor *SyncJobExecutor, checkpoints SyncCheckpointStore) *TaskCoordinator {
	if executor == nil {
		panic("task coordinator executor must not be nil")
	}
	if checkpoints == nil {
		checkpoints = newMemorySyncCheckpointStore()
	}

	return &TaskCoordinator{syncRunnerConfig: syncRunnerConfig, taskDAO: taskDAO, taskService: taskService, registry: registry, sink: sink, pruneService: pruneService, idResolver: idResolver, executor: executor, checkpoints: checkpoints}
}

// Execute dispatches a sync_logs task by task type.
func (c *TaskCoordinator) Execute(ctx context.Context, taskContext dao.SyncTaskContext, lease ConnectorLockLease) (TaskOutcome, error) {
	runCtx, cancel := context.WithDeadline(ctx, taskExecutionDeadline(time.Now(), taskContext, lease))
	defer cancel()
	ctx = runCtx

	connector, err := c.registry.Open(ctx, taskContext)
	if err != nil {
		return TaskOutcome{}, err
	}
	if err = connector.Validate(ctx); err != nil {
		return TaskOutcome{}, err
	}

	switch taskContext.Task.TaskType {
	case dao.TaskTypeSync:
		queue, err := c.executor.RegisterTask(ctx, taskContext.Task.ID)
		if err != nil {
			return TaskOutcome{}, err
		}
		defer queue.Close()

		runner := NewSyncRunner(c.syncRunnerConfig, c.taskDAO, c.taskService, c.sink, c.idResolver, queue, c.checkpoints)
		nextTaskID, err := runner.Run(ctx, taskContext, connector)
		return TaskOutcome{NextTaskID: nextTaskID}, err
	case dao.TaskTypePrune:
		runner := NewPruneRunner(c.taskDAO, c.taskService, c.pruneService)
		nextTaskID, err := runner.Run(ctx, taskContext, connector)
		return TaskOutcome{NextTaskID: nextTaskID}, err
	default:
		return TaskOutcome{}, fmt.Errorf("unsupported sync task type %q", taskContext.Task.TaskType)
	}
}

func taskExecutionDeadline(now time.Time, taskContext dao.SyncTaskContext, lease ConnectorLockLease) time.Time {
	timeout := connectorLockTTL
	if seconds := taskContext.Connector.TimeoutSecs; seconds > 0 && seconds <= int64(connectorLockTTL/time.Second) {
		timeout = time.Duration(seconds) * time.Second
	}

	deadline := now.Add(timeout)
	if lease.ExpiresAt.IsZero() {
		if timeout > connectorLockTTL {
			return now.Add(connectorLockTTL)
		}
		return deadline
	}

	lockDeadline := lease.ExpiresAt.Add(-connectorLockSafetyMargin)
	if lockDeadline.Before(now) {
		return now
	}
	if lockDeadline.Before(deadline) {
		return lockDeadline
	}
	return deadline
}
