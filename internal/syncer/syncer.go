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
	"sync"
	"time"

	"ragflow/internal/dao"
	"ragflow/internal/service"
	documentservice "ragflow/internal/service/document"
	syncerconnector "ragflow/internal/syncer/connector"
	"ragflow/internal/utility"
)

// Syncer owns the scheduler, task queue, and bounded worker pool.
type Syncer struct {
	id          string
	config      Config
	queue       chan TaskEnvelope
	scheduler   *Scheduler
	worker      *TaskWorker
	cancel      context.CancelFunc
	workerGroup sync.WaitGroup
	ShutdownCh  chan struct{}
}

// NewSyncer creates a server-compatible syncer with default dependencies.
func NewSyncer(maxConcurrency int, pollInterval time.Duration) *Syncer {
	config := DefaultConfig()
	config.TaskConcurrency = maxConcurrency
	config.PollInterval = pollInterval
	taskDAO := dao.NewSyncTaskDAO(nil)
	registry := syncerconnector.NewRegistry()
	pruneService := service.NewSyncPruneService(dao.NewSyncPruneSnapshotDAO(taskDAO.DB()), documentservice.NewDocumentService(), nil)
	return New(config, taskDAO, registry, nil, pruneService)
}

// New creates a datasource syncer from explicit dependencies.
func New(config Config, taskDAO *dao.SyncTaskDAO, registry ConnectorRegistry, sink service.DocumentSink, pruneService *service.SyncPruneService) *Syncer {
	config = config.Normalize()
	queue := make(chan TaskEnvelope, config.TaskQueueSize)
	locker := NewConnectorLock()

	globalItems := make(chan struct{}, config.GlobalItemConcurrency)
	taskService := service.NewSyncTaskService(taskDAO)
	idResolver := service.NewDocumentIDResolver(service.NewGormDocumentStore())

	coordinator := NewTaskCoordinator(TaskCoordinatorConfig{
		PerTaskItemConcurrency: config.PerTaskItemConcurrency,
		ItemRetryCount:         config.ItemRetryCount,
		ItemRetryBaseDelay:     config.ItemRetryBaseDelay,
	}, taskService, registry, sink, pruneService, idResolver, globalItems)

	return &Syncer{
		id:         utility.GenerateUUID(),
		config:     config,
		queue:      queue,
		scheduler:  NewScheduler(config.PollInterval, queue, taskService),
		worker:     NewTaskWorker(queue, taskService, coordinator, locker),
		ShutdownCh: make(chan struct{}),
	}
}

// ID returns this syncer process ID.
func (s *Syncer) ID() string {
	if s == nil {
		return ""
	}
	return s.id
}

// Start launches the scheduler and task workers with a background context.
func (s *Syncer) Start() error {
	return s.StartContext(context.Background())
}

// StartContext launches the scheduler and task workers.
func (s *Syncer) StartContext(ctx context.Context) error {
	if s == nil {
		return errors.New("syncer is nil")
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.workerGroup.Add(2)

	go func() {
		defer s.workerGroup.Done()
		_ = s.scheduler.Run(runCtx)
	}()
	go func() {
		defer s.workerGroup.Done()
		s.worker.Run(runCtx, s.config.TaskConcurrency)
	}()
	return nil
}

// Stop cancels the scheduler and waits for workers to exit.
func (s *Syncer) Stop() {
	if s == nil || s.cancel == nil {
		return
	}
	s.cancel()
	s.workerGroup.Wait()
	close(s.ShutdownCh)
}
