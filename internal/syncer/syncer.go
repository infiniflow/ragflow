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

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	"ragflow/internal/service"
	documentservice "ragflow/internal/service/document"
	syncerconnector "ragflow/internal/syncer/connector"
	"ragflow/internal/utility"

	"go.uber.org/zap"
)

// Syncer owns NATS/DB scheduling, task workers, and the shared batch job executor.
type Syncer struct {
	id          string
	config      Config
	queue       chan TaskEnvelope
	scheduler   *Scheduler
	worker      *TaskWorker
	executor    *SyncJobExecutor
	cancel      context.CancelFunc
	workerGroup sync.WaitGroup
	stopOnce    sync.Once
	ShutdownCh  chan struct{}
}

// NewSyncer creates a server-compatible syncer with default dependencies.
func NewSyncer(taskWorkerCount int) *Syncer {
	// init the config
	config := DefaultConfig()
	config.TaskWorkerCount = taskWorkerCount

	taskDAO := dao.NewSyncTaskDAO(nil)
	registry := syncerconnector.NewRegistry()

	syncerconnector.RegisterBuiltIns(registry)

	documentService := documentservice.NewDocumentService()
	pruneService := service.NewSyncPruneService(documentService, nil)

	return New(config, taskDAO, registry, documentService, pruneService)
}

// New creates a datasource syncer from explicit dependencies.
func New(config Config, taskDAO *dao.SyncTaskDAO, registry ConnectorRegistry, sink service.DocumentSink, pruneService *service.SyncPruneService) *Syncer {
	config = config.Normalize()
	queue := make(chan TaskEnvelope, config.TaskQueueSize)
	locker := NewConnectorLock()

	executor := NewSyncJobExecutor(SyncJobExecutorConfig{
		WorkerCount:  config.JobWorkerCount,
		JobQueueSize: config.JobQueueSize,
	})
	taskService := service.NewSyncTaskService(taskDAO)
	idResolver := service.NewDocumentIDResolver(service.NewGormDocumentStore())
	checkpoints := SyncCheckpointStore(newMemorySyncCheckpointStore())
	messageQueue := engine.GetMessageQueueEngine()
	if store, ok := messageQueue.(SyncCheckpointStore); ok {
		checkpoints = store
	}

	coordinator := NewTaskCoordinator(SyncRunnerConfig{
		ItemRetryCount:        config.ItemRetryCount,
		ItemRetryBaseDelay:    config.ItemRetryBaseDelay,
		MaxAnchorRestartCount: config.MaxAnchorRestartCount,
	}, taskDAO, taskService, registry, sink, pruneService, idResolver, executor, checkpoints)

	scheduler := NewScheduler(queue, taskDAO)
	if broker, ok := messageQueue.(SyncTaskBroker); ok {
		scheduler = NewNATSScheduler(queue, taskDAO, broker)
	}

	return &Syncer{
		id:         utility.GenerateUUID(),
		config:     config,
		queue:      queue,
		scheduler:  scheduler,
		worker:     NewTaskWorker(queue, taskDAO, taskService, coordinator, locker).WithScheduler(scheduler),
		executor:   executor,
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

// StartContext launches the scheduler and task workers.
func (s *Syncer) StartContext(ctx context.Context) error {
	if s == nil {
		return errors.New("syncer is nil")
	}

	runCtx, cancel := context.WithCancel(ctx)
	s.cancel = cancel
	s.workerGroup.Add(2)

	// run scheduler
	go func() {
		defer s.workerGroup.Done()
		if err := s.scheduler.Run(runCtx); err != nil && !errors.Is(err, context.Canceled) {
			common.Error("syncer scheduler stopped", err)
		}
	}()

	// run worker poll
	go func() {
		defer s.workerGroup.Done()
		s.worker.Run(runCtx, s.config.TaskWorkerCount)
	}()
	return nil
}

// Stop cancels the scheduler and waits for workers to exit.
func (s *Syncer) Stop() {
	if s == nil {
		return
	}
	s.stopOnce.Do(func() {
		if s.cancel != nil {
			s.cancel()
		}
		s.workerGroup.Wait()
		s.executor.Close()
		close(s.ShutdownCh)
	})
}

// logSyncTaskDuration get job run time
func logSyncTaskDuration(taskContext dao.SyncTaskContext, startedAt time.Time) {
	if taskContext.Task.TaskType != dao.TaskTypeSync {
		return
	}
	common.Info(
		"sync task duration",
		zap.String("task_id", taskContext.Task.ID),
		zap.String("connector_id", taskContext.Connector.ID),
		zap.String("kb_id", taskContext.Knowledgebase.ID),
		zap.String("source", taskContext.Connector.Source),
		zap.Duration("elapsed", time.Since(startedAt)),
	)
}
