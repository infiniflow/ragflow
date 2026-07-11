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
	"errors"
	"fmt"
	"ragflow/internal/utility"
	"sync"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	"ragflow/internal/entity"
	taskpkg "ragflow/internal/ingestion/task"
	servicepkg "ragflow/internal/service"

	"github.com/cenkalti/backoff/v5"
)

type Ingestor struct {
	id     string
	name   string
	ctx    context.Context
	cancel context.CancelFunc

	// Configuration
	maxConcurrency    int32
	supportedDocTypes []string
	version           string
	heartbeatInterval time.Duration

	// Runtime state
	currentTasks map[string]*taskpkg.TaskContext
	tasksMu      sync.RWMutex

	// Shutdown channel - receive on this to trigger graceful shutdown
	ShutdownCh chan struct{}

	// Worker pool
	taskChan  chan *taskpkg.TaskContext
	workerWg  sync.WaitGroup
	startOnce sync.Once

	ingestionTaskSvc *servicepkg.IngestionTaskService
	docState         *docStateUpdater

	// runDocumentTask dispatches to the migrated task handler path.
	// Tests may override this to verify branch routing without invoking
	// the full downstream stack.
	runDocumentTask func(ctx context.Context, ingestionTask *entity.IngestionTask) error
}

func NewIngestor(name string, maxConcurrency int32, supportedTypes []string) *Ingestor {
	ctx, cancel := context.WithCancel(context.Background())
	id := utility.GenerateUUID()
	ingestor := &Ingestor{
		id:                id,
		name:              name,
		ctx:               ctx,
		cancel:            cancel,
		maxConcurrency:    maxConcurrency,
		supportedDocTypes: supportedTypes,
		version:           "1.0.0",
		currentTasks:      make(map[string]*taskpkg.TaskContext),
		taskChan:          make(chan *taskpkg.TaskContext, maxConcurrency*2),
		ShutdownCh:        make(chan struct{}, 1),
		ingestionTaskSvc:  servicepkg.NewIngestionTaskService(),
		docState:          &docStateUpdater{},
		heartbeatInterval: 10 * time.Second,
	}
	ingestor.runDocumentTask = ingestor.defaultRunDocumentTask
	return ingestor
}

func (e *Ingestor) ID() string {
	return e.id
}

func (e *Ingestor) Start() error {
	common.Info(fmt.Sprintf("Ingestor %s initialized", e.id))
	msgQueueEngine := engine.GetMessageQueueEngine()
	err := msgQueueEngine.InitConsumer("tasks.RAGFLOW")
	if err != nil {
		return err
	}

	// Ensure worker pool is started on first task
	go e.startWorkerPool()

	for {
		var taskHandles []common.TaskHandle
		taskHandles, err = msgQueueEngine.GetMessages(4)
		if err != nil {
			common.Error("error consuming message", err)
			continue
		}
		for _, taskHandle := range taskHandles {
			if err := e.processMessage(taskHandle); err != nil {
				return err
			}
		}
	}
}

// processMessage handles a single incoming MQ message: filter by type,
// activate the task (state transition), guard against duplicate execution
// (claim), and enqueue to the worker pool (or backpressure-reject).
func (e *Ingestor) processMessage(handle common.TaskHandle) error {
	taskMessage := handle.GetMessage()
	common.Info(fmt.Sprintf("Received task id: %s, type: %s", taskMessage.TaskID, taskMessage.TaskType))

	if taskMessage.TaskType != common.TaskTypeIngestionTask {
		common.Info(fmt.Sprintf("task %s is not an ingestion task", taskMessage.TaskID))
		if err := handle.Ack(); err != nil {
			common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), err)
			return err
		}
		return nil
	}

	task, err := e.ingestionTaskSvc.StartRunning(taskMessage.TaskID)
	if err != nil {
		if errors.Is(err, common.ErrTaskNotFound) {
			common.Warn(fmt.Sprintf("task %s not found, skipping", taskMessage.TaskID))
			if ackErr := handle.Ack(); ackErr != nil {
				common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), ackErr)
				return ackErr
			}
			return nil
		}
		common.Error(fmt.Sprintf("error setting task %s to running", taskMessage.TaskID), err)
		return err
	}
	if task == nil {
		common.Info(fmt.Sprintf("task %s is already removed", taskMessage.TaskID))
		if ackErr := handle.Ack(); ackErr != nil {
			return ackErr
		}
		return nil
	}

	switch task.Status {
	case common.COMPLETED, common.STOPPED, common.FAILED:
		common.Info(fmt.Sprintf("task %s is already %s", taskMessage.TaskID, task.Status))
		if ackErr := handle.Ack(); ackErr != nil {
			common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), ackErr)
			return ackErr
		}
		return nil
	case common.STOPPING, common.CREATED:
		return fmt.Errorf("task %s is in unexpected status %s", taskMessage.TaskID, task.Status)
	case common.RUNNING:
		// Guard against MQ redelivery: if another worker in this
		// process is already processing this task, ack the redelivered
		// message and skip instead of scheduling it again.
		if !e.claimTask(task.ID) {
			common.Warn(fmt.Sprintf("task %s redelivered while worker still processing, ack skip (task_id=%s doc_id=%s kb_id=%s)",
				taskMessage.TaskID, task.ID, task.DocumentID, task.DatasetID))
			if ackErr := handle.Ack(); ackErr != nil {
				common.Error(fmt.Sprintf("error ack redelivered task %s", taskMessage.TaskID), ackErr)
				return ackErr
			}
			return nil
		}
	}

	// Construct TaskContext and carry the MQ handle so the worker can
	// Ack/Nack when the task reaches a terminal status.
	taskCtx := taskpkg.NewTaskContextForScheduling(e.ctx, task)
	taskCtx.Handle = handle

	// Push to task channel; if full, reject the task (backpressure).
	select {
	case e.taskChan <- taskCtx:
		common.Info(fmt.Sprintf("Task %s queued (channel: %d/%d)", task.ID, len(e.taskChan), cap(e.taskChan)))
	default:
		common.Info(fmt.Sprintf("No available slot for task %s, failed", task.ID))
		e.releaseTask(task.ID)
		if nackErr := handle.Nack(); nackErr != nil {
			common.Error(fmt.Sprintf("error nack task %s", taskMessage.TaskID), nackErr)
			return nackErr
		}
	}
	return nil
}

func (e *Ingestor) startWorkerPool() {
	e.startOnce.Do(func() {
		for i := int32(0); i < e.maxConcurrency; i++ {
			e.workerWg.Add(1)
			go e.workerLoop(i)
		}
		common.Info(fmt.Sprintf("Worker pool started with %d workers", e.maxConcurrency))
	})
}

func (e *Ingestor) workerLoop(id int32) {
	defer e.workerWg.Done()
	common.Info(fmt.Sprintf("Worker %d started", id))
	for {
		select {
		case <-e.ctx.Done():
			return
		case taskCtx := <-e.taskChan:
			common.Info("task context:" + taskCtx.IngestionTask.ID)
			e.executeTask(taskCtx)
		}
	}
}

func (e *Ingestor) executeTask(taskCtx *taskpkg.TaskContext) {
	task := taskCtx.IngestionTask
	common.Info(fmt.Sprintf("Starting task %s", task.ID))

	// Release the claim when executeTask returns so that a future
	// redelivery (after restart) can re-claim the task.
	defer e.releaseTask(task.ID)

	e.settleMessage(taskCtx, func(ctx context.Context) bool {
		return e.runTask(ctx, task)
	})
}

// markFailed persists FAILED status for the task and reports whether the
// terminal status was durably written, so the caller can decide Ack vs Nack.
func (e *Ingestor) markFailed(taskID string) bool {
	if uErr := e.ingestionTaskSvc.MarkFailed(taskID); uErr != nil {
		common.Error(fmt.Sprintf("Failed to set task %s to FAILED", taskID), uErr)
		return false
	}
	return true
}

// runTask executes the task's business logic — checkpoint advance, document
// pipeline, and completion — behind the heartbeat. It returns whether the
// task reached a durably-persisted terminal status. The ctx check runs
// before AdvanceStepCheckpoint (problem 4) so a cancelled task does not
// bump the checkpoint and inflate the step counter on redelivery.
func (e *Ingestor) runTask(ctx context.Context, task *entity.IngestionTask) bool {
	select {
	case <-ctx.Done():
		common.Info(fmt.Sprintf("Task %s cancelled", task.ID))
		return false
	default:
	}

	if err := e.ingestionTaskSvc.AdvanceStepCheckpoint(task.ID); err != nil {
		common.Error(fmt.Sprintf("Failed to advance step checkpoint for task %s", task.ID), err)
		return e.markFailed(task.ID)
	}

	if err := e.runDocumentTask(ctx, task); err != nil {
		common.Error(fmt.Sprintf("Task %s failed", task.ID), err)
		return e.markFailed(task.ID)
	}

	_, err := backoff.Retry(ctx, func() (struct{}, error) {
		return struct{}{}, e.ingestionTaskSvc.MarkCompleted(task.ID)
	}, backoff.WithMaxTries(3))
	if err != nil {
		common.Error(fmt.Sprintf("Task %s update status failed", task.ID), err)
		return false
	}

	common.Info(fmt.Sprintf("Task %s completed", task.ID))
	return true
}

// settleMessage runs body under a heartbeat, then settles the MQ message. The
// heartbeat is stopped (and waited on) before ack/nack — see startHeartbeat.
// terminal is derived from body's return value; on panic terminal defaults to
// false (non-terminal → Nack) so the broker redelivers after restart.
func (e *Ingestor) settleMessage(taskCtx *taskpkg.TaskContext, body func(context.Context) bool) (terminal bool) {
	stop := e.startHeartbeat(taskCtx)
	defer func() {
		stop() // stop heartbeat (and wait) before ack/nack
		e.ackOrNack(taskCtx, terminal)
	}()
	terminal = body(taskCtx.Ctx)
	return
}

// ackOrNack settles the MQ message according to the terminal flag: Ack if the
// task reached a durably-persisted terminal status, Nack otherwise so the
// broker redelivers and resumes. A nil handle (standalone/test path) is a no-op.
func (e *Ingestor) ackOrNack(taskCtx *taskpkg.TaskContext, terminal bool) {
	if taskCtx.Handle == nil {
		return
	}
	if terminal {
		if err := taskCtx.Handle.Ack(); err != nil {
			common.Error(fmt.Sprintf("ack task %s", taskCtx.IngestionTask.ID), err)
		}
		return
	}
	if err := taskCtx.Handle.Nack(); err != nil {
		common.Error(fmt.Sprintf("nack task %s", taskCtx.IngestionTask.ID), err)
	}
}

// claimTask registers a worker claim on a task ID. Returns false if another
// worker has already claimed it (e.g. MQ redelivery), true on first claim.
// startHeartbeat launches a goroutine that calls Handle.InProgress every
// heartbeatInterval to keep the broker AckWait timer fresh during long tasks.
// It returns a stop function that signals the goroutine to exit and BLOCKS
// until it has, so the caller can ack/nack with no in-flight InProgress on the
// same message. Returns a no-op stop when there is no handle or no interval
// (standalone/test path).
func (e *Ingestor) startHeartbeat(taskCtx *taskpkg.TaskContext) func() {
	if taskCtx.Handle == nil || e.heartbeatInterval <= 0 {
		return func() {}
	}
	var wg sync.WaitGroup
	wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(e.heartbeatInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := taskCtx.Handle.InProgress(); err != nil {
					common.Error(fmt.Sprintf("heartbeat task %s", taskCtx.IngestionTask.ID), err)
				}
			case <-done:
				return
			case <-taskCtx.Ctx.Done():
				return
			}
		}
	}()
	return func() {
		close(done)
		wg.Wait()
	}
}

func (e *Ingestor) claimTask(taskID string) bool {
	e.tasksMu.Lock()
	defer e.tasksMu.Unlock()
	if _, ok := e.currentTasks[taskID]; ok {
		return false
	}
	e.currentTasks[taskID] = nil // placeholder; replaced after scheduling
	return true
}

// releaseTask removes the claim so a future redelivery (after process restart)
// can re-claim the task.
func (e *Ingestor) releaseTask(taskID string) {
	e.tasksMu.Lock()
	delete(e.currentTasks, taskID)
	e.tasksMu.Unlock()
}

func (e *Ingestor) defaultRunDocumentTask(ctx context.Context, ingestionTask *entity.IngestionTask) error {
	docTaskCtx, err := taskpkg.LoadFromIngestionTask(ingestionTask)
	if err != nil {
		return fmt.Errorf("load task context for %s: %w", ingestionTask.ID, err)
	}
	if docTaskCtx.PipelineID == "" {
		return fmt.Errorf("ingestion task %s: no pipeline_id configured for document %s or dataset %s", ingestionTask.ID, docTaskCtx.Doc.ID, docTaskCtx.KB.ID)
	}
	docTaskCtx.Ctx = ctx
	// The sink owns all document/ingestion_task_log/ingestion_task.component_total
	// writes for this run; inject it into the executor so the pipeline reports
	// progress to the service layer instead of touching the DAO directly.
	sink := newProgressSink(e.ingestionTaskSvc)
	result, err := taskpkg.NewTaskHandler(docTaskCtx).
		WithPipelineExecutorFactory(func(c *taskpkg.TaskContext, canvasID string) (*taskpkg.PipelineExecutor, error) {
			ex, err := taskpkg.NewPipelineExecutor(c, canvasID, 0)
			if err != nil {
				return nil, err
			}
			return ex.WithProgressSink(sink), nil
		}).
		Handle()
	if err != nil {
		return err
	}
	e.docState.apply(result)
	return nil
}

// Stop gracefully shuts down the ingestor
func (e *Ingestor) Stop() {
	common.Info(fmt.Sprintf("Stopping ingestor %s", e.id))
	e.cancel()

	// Wait for all workers to finish (they exit on ctx.Done())
	e.workerWg.Wait()
	common.Info("All tasks completed")
}
