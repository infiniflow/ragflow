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
	syncerconnector "ragflow/internal/syncer/connector"
	"time"
)

const connectorLockSafetyMargin = 5 * time.Second

// ConnectorRegistry opens registered connectors by source.
type ConnectorRegistry interface {
	// Open creates a connector for a task context.
	Open(ctx context.Context, taskContext any) (syncerconnector.Connector, error)
}

// TaskCoordinatorConfig controls per-task document processing.
type TaskCoordinatorConfig struct {
	ItemRetryCount     int
	ItemRetryBaseDelay time.Duration
}

// TaskCoordinator owns one task execution window.
type TaskCoordinator struct {
	config       TaskCoordinatorConfig
	taskService  *service.SyncTaskService
	registry     ConnectorRegistry
	sink         service.DocumentSink
	pruneService *service.SyncPruneService
	idResolver   *service.DocumentIDResolver
	executor     *SyncJobExecutor
}

// NewTaskCoordinator creates a coordinator for one claimed task at a time.
func NewTaskCoordinator(config TaskCoordinatorConfig, taskService *service.SyncTaskService, registry ConnectorRegistry, sink service.DocumentSink, pruneService *service.SyncPruneService, idResolver *service.DocumentIDResolver, executor *SyncJobExecutor) *TaskCoordinator {
	if config.ItemRetryCount <= 0 {
		config.ItemRetryCount = 1
	}

	if config.ItemRetryBaseDelay <= 0 {
		config.ItemRetryBaseDelay = time.Second
	}
	if executor == nil {
		panic("task coordinator executor must not be nil")
	}

	return &TaskCoordinator{config: config, taskService: taskService, registry: registry, sink: sink, pruneService: pruneService, idResolver: idResolver, executor: executor}
}

// Execute dispatches a sync_logs task by task type.
func (c *TaskCoordinator) Execute(ctx context.Context, taskContext service.SyncTaskContext, lease ConnectorLockLease) error {
	runCtx, cancel := context.WithDeadline(ctx, taskExecutionDeadline(time.Now(), taskContext, lease))
	defer cancel()
	ctx = runCtx

	connector, err := c.registry.Open(ctx, taskContext)
	if err != nil {
		return err
	}
	if err = connector.Validate(ctx); err != nil {
		return err
	}

	switch taskContext.Task.TaskType {
	case service.TaskTypeSync:
		queue, err := c.executor.RegisterTask(ctx, taskContext.Task.ID)
		if err != nil {
			return err
		}
		defer queue.Close()

		runner := NewSyncRunner(c.config, c.taskService, c.sink, c.idResolver, queue)
		return runner.Run(ctx, taskContext, connector)
	case service.TaskTypePrune:
		runner := NewPruneRunner(c.taskService, c.pruneService)
		return runner.Run(ctx, taskContext, connector)
	default:
		return fmt.Errorf("unsupported sync task type %q", taskContext.Task.TaskType)
	}
}

func taskExecutionDeadline(now time.Time, taskContext service.SyncTaskContext, lease ConnectorLockLease) time.Time {
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
