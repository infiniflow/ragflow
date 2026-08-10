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
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/dao"
	"ragflow/internal/engine"
	redis2 "ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	"ragflow/internal/ingestion/knowledge_compile"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	taskpkg "ragflow/internal/ingestion/task"
	servicepkg "ragflow/internal/service"
	documentpkg "ragflow/internal/service/document"
	"ragflow/internal/utility"

	"github.com/cenkalti/backoff/v5"
)

const defaultHeartbeatInterval = 10 * time.Second

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
	currentTasks  map[string]struct{} // set of task IDs currently claimed by a worker
	tasksMu       sync.RWMutex
	activeWorkers atomic.Int32 // number of worker goroutines currently in workerLoop

	// Shutdown channel - receive on this to trigger graceful shutdown
	ShutdownCh chan struct{}

	// Worker pool
	taskChan   chan *taskpkg.TaskContext
	workerWg   sync.WaitGroup
	startOnce  sync.Once
	workerOnce sync.Once // guards startWorkerPool; must NOT be startOnce (Start wraps start() in startOnce, and start() calls startWorkerPool -> re-entry deadlock)
	stopOnce   sync.Once // guards close(ShutdownCh) against double-close on repeated Stop

	ingestionTaskSvc *servicepkg.IngestionTaskService
	docState         *docStateUpdater
	// memorySvc runs async memory-extraction tasks (TaskKindMemory) that share
	// the worker pool with ingestion tasks. nil disables memory extraction
	// (e.g. tests that don't exercise it).
	memorySvc *servicepkg.MemoryMessageService

	// knowledgeCompile is the dataset-level post-processing consumer (§11,
	// Option E) owned by this ingestor. It is driven by kcConcurrency owned
	// worker goroutines, each running the Consumer's Run loop (poll MySQL
	// scheduling rows + NATS notify → claim closed batch → merge → ack). The
	// MySQL knowledge_compile_docs table — not the broker — is the scheduling
	// system of record and the source of same-KB serialization, so different
	// datasets compile in parallel while the same dataset is serialized by its
	// claim row. The workers are started/joined within the ingestor (via
	// compileWg), so they share its lifecycle and goroutine set instead of
	// running as a separate service goroutine.
	knowledgeCompile *knowledge_compile.Consumer
	kcLLMID          string
	kcEmbedding      string
	kcConcurrency    int32 // number of parallel dataset-level compile workers

	compileWg sync.WaitGroup

	// runDocumentTask dispatches to the migrated task handler path.
	// Tests may override this to verify branch routing without invoking
	// the full downstream stack.
	runDocumentTask func(ctx context.Context, ingestionTask *entity.IngestionTask) error

	// cancelCheck is polled periodically (every 3s) during task execution.
	// When it returns true the task's context is cancelled, which causes the
	// pipeline to stop at the next ctx.Err() check. Defaults to a Redis
	// cancel-flag lookup that mirrors Python's has_canceled(). Tests may
	// override this to simulate cancel without Redis.
	cancelCheck func(ctx context.Context, taskID string) bool
}

func NewIngestor(name string, maxConcurrency int32, supportedTypes []string) *Ingestor {
	if maxConcurrency <= 0 {
		maxConcurrency = int32(runtime.NumCPU())
	}
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
		currentTasks:      make(map[string]struct{}),
		taskChan:          make(chan *taskpkg.TaskContext, maxConcurrency*2),
		ShutdownCh:        make(chan struct{}, 1),
		ingestionTaskSvc:  servicepkg.NewIngestionTaskService(),
		docState:          newDocStateUpdater(),
		heartbeatInterval: defaultHeartbeatInterval,
	}
	ingestor.runDocumentTask = ingestor.defaultRunDocumentTask
	ingestor.cancelCheck = ingestor.defaultCancelCheck
	ingestor.kcConcurrency = maxConcurrency // parallel dataset-level compile workers default to the task width
	return ingestor
}

func (e *Ingestor) ID() string {
	return e.id
}

// consumeErrorBackoff paces the consume loop when GetMessages returns an
// error, so a persistent MQ failure does not pin a CPU. The backoff is
// cancellable so a shutdown during backoff returns promptly.
const consumeErrorBackoff = 1 * time.Second

func (e *Ingestor) Start() error {
	common.Info(fmt.Sprintf("Ingestor %s initialized", e.id))
	var startErr error
	e.startOnce.Do(func() {
		startErr = e.start()
	})
	return startErr
}

// start runs the full startup sequence. It is invoked at most once (guarded by
// startOnce in Start) so repeated Start calls cannot launch duplicate worker
// pools, compile consumers, or consume loops, and the first initialization
// error is retained and returned to every later caller.
func (e *Ingestor) start() error {
	msgQueueEngine := engine.GetMessageQueueEngine()
	if err := msgQueueEngine.InitConsumer(common.TaskSubject); err != nil {
		return err
	}

	// Start the task worker pool and the dataset-level compile consumer as
	// owned goroutines joined by Stop via workerWg/compileWg. Start follows
	// the standard lifecycle contract: it returns immediately after kicking
	// these off rather than blocking on the consume loop itself.
	e.startWorkerPool()
	e.startDatasetKnowledgeCompile()

	// Run the main tasks.RAGFLOW consume loop off the caller's goroutine so
	// Start returns promptly; it is joined by Stop via workerWg.
	e.workerWg.Add(1)
	go e.consumeLoop()
	return nil
}

// consumeLoop is the main tasks.RAGFLOW consume loop. It runs until e.ctx is
// cancelled (graceful shutdown); per-message failures are settled by
// processMessage and never terminate the consumer. Dataset-level compile work
// is owned by the kcConcurrency compileLoop workers (each running the
// Consumer's Run loop), so this loop stays focused on tasks.RAGFLOW and is
// never held up by compile work.
func (e *Ingestor) consumeLoop() {
	defer e.workerWg.Done()
	msgQueueEngine := engine.GetMessageQueueEngine()
	for {
		// Graceful shutdown is the only condition under which the consume
		// loop exits. Per-message processing failures never terminate the
		// consumer: processMessage settles (ack/nack) each message itself.
		if err := e.ctx.Err(); err != nil {
			return
		}
		taskHandles, err := msgQueueEngine.GetMessages(4)
		if err != nil {
			common.Error("error consuming message", err)
			select {
			case <-time.After(consumeErrorBackoff):
			case <-e.ctx.Done():
				return
			}
			continue
		}
		if len(taskHandles) > 0 {
			for _, taskHandle := range taskHandles {
				e.processMessage(taskHandle)
			}
			continue
		}
		// tasks.RAGFLOW idle: dataset-level compile work is owned by the
		// kcConcurrency compileLoop workers (started in
		// startDatasetKnowledgeCompile), so the task loop stays focused on
		// tasks.RAGFLOW and is never held up by compile work.
	}
}

// SetMemoryMessageService installs the memory-extraction service used by
// TaskKindMemory tasks that share the worker pool. Call it before Start; a nil
// value disables memory extraction (received memory tasks are ack-skipped).
func (e *Ingestor) SetMemoryMessageService(memorySvc *servicepkg.MemoryMessageService) {
	e.memorySvc = memorySvc
}

// SetKnowledgeCompileModelConfig supplies the default LLM/embedding model ids
// used by the dataset-level compile consumer's deduper. Call it before Start.
func (e *Ingestor) SetKnowledgeCompileModelConfig(llmID, embedding string) {
	e.kcLLMID = llmID
	e.kcEmbedding = embedding
}

// SetKnowledgeCompileConcurrency sets the number of parallel dataset-level
// compile workers. Multiple datasets compile concurrently (each worker runs its
// own Run loop that claims a KB's closed batch and merges it); the per-KB claim
// row serializes the same dataset, preserving the §11 O(unique pairs)
// merged-dedup invariant. Call before Start. A value <= 0 falls back to
// runtime.NumCPU() at start time.
func (e *Ingestor) SetKnowledgeCompileConcurrency(n int32) {
	e.kcConcurrency = n
}

// startDatasetKnowledgeCompile provisions the dataset-level compile scheduling
// store (MySQL knowledge_compile_docs + NATS notify subject), builds the
// Consumer, and starts the owned compile-worker pool. The consumer is driven by
// the ingestor's own goroutine set — kcConcurrency compileLoop workers, each
// running the Consumer's Run loop (see §11.7 of knowledge_compile_design.md) —
// so its lifecycle shares the ingestor's. Best-effort: any provisioning failure
// is logged and the pipeline continues to write available_int=0 compiled
// chunks; they just won't be merged until a scheduler is available.
func (e *Ingestor) startDatasetKnowledgeCompile() {
	mq := engine.GetMessageQueueEngine()
	if mq == nil {
		common.Warn("message queue not initialized; dataset-level compile consumer disabled")
		return
	}
	knowledge_compile.SetModelConfig(e.kcLLMID, e.kcEmbedding)
	if err := knowledge_compile.Provision(e.ctx, mq, dao.DB); err != nil {
		common.Warn(fmt.Sprintf("dataset-level compile consumer unavailable; compiled chunks will not be merged: %v", err))
		return
	}
	e.knowledgeCompile = knowledge_compile.NewConsumer(knowledge_compile.DefaultClaimer())
	n := e.kcConcurrency
	if n <= 0 {
		n = int32(runtime.NumCPU())
	}
	// kcConcurrency owned workers, each running its own Run loop. Concurrency is
	// bounded by the goroutine count itself (no extra semaphore). Different
	// datasets compile in parallel while the same dataset is serialized by its
	// MySQL claim row — preserving the §11 invariant that merged dedup stays
	// O(unique pairs) instead of O(N).
	for i := int32(0); i < n; i++ {
		e.compileWg.Add(1)
		go e.compileLoop(i)
	}
}

// compileLoop is one of the owned dataset-level compile workers. It runs the
// Consumer's Run loop (poll scheduling rows + NATS notify → claim closed batch
// → merge → ack). kcConcurrency such goroutines run in parallel, so different
// datasets compile concurrently while the same dataset is serialized by its
// claim row. All are joined in Stop via compileWg.
func (e *Ingestor) compileLoop(id int32) {
	defer e.compileWg.Done()
	if e.knowledgeCompile != nil {
		e.knowledgeCompile.Run(e.ctx)
	}
}

// processMessage handles a single incoming MQ message: filter by type,
// activate the task (state transition), guard against duplicate execution
// (claim), and enqueue to the worker pool (or backpressure-reject). It
// settles (ack/nack) every message itself and never returns an error: a
// single message can never terminate the consume loop. Only ctx cancellation
// (graceful shutdown) stops the consumer - see Start.
func (e *Ingestor) processMessage(handle common.TaskHandle) {
	taskMessage := handle.GetMessage()
	common.Info(fmt.Sprintf("Received task id: %s, type: %s", taskMessage.TaskID, taskMessage.TaskType))

	// Deferred claim release: if this function claims a task but the task
	// is not successfully enqueued to the worker pool (e.g. backpressure,
	// or a future error path added between claim and enqueue), the defer
	// cleans up so the task can be reclaimed on MQ redelivery. When the
	// task IS enqueued, claimedTaskID is cleared and executeTask's own
	// defer takes ownership of the release.
	var claimedTaskID string
	defer func() {
		if claimedTaskID != "" {
			e.releaseTask(claimedTaskID)
		}
	}()

	// Memory-extraction tasks share the tasks.RAGFLOW consumer and the worker
	// pool with ingestion tasks. They do NOT use the ingestion state machine
	// (no ingestion_task row): the message body is dispatched straight to a
	// worker via TaskContext.Kind==TaskKindMemory, which runs the memory
	// extractor and acks/nacks on its own.
	if taskMessage.TaskType == common.TaskTypeMemory {
		if e.memorySvc == nil {
			common.Warn(fmt.Sprintf("memory task %s received but memory extractor is disabled, ack", taskMessage.TaskID))
			if err := handle.Ack(); err != nil {
				common.Error(fmt.Sprintf("error ack memory task %s", taskMessage.TaskID), err)
			}
			return
		}
		var payload map[string]any
		if len(taskMessage.Payload) == 0 || json.Unmarshal(taskMessage.Payload, &payload) != nil {
			common.Warn(fmt.Sprintf("memory task %s has no parseable payload, ack", taskMessage.TaskID))
			if err := handle.Ack(); err != nil {
				common.Error(fmt.Sprintf("error ack memory task %s", taskMessage.TaskID), err)
			}
			return
		}
		taskCtx := taskpkg.NewMemoryTaskContextForScheduling(e.ctx, payload, handle)
		select {
		case e.taskChan <- taskCtx:
			common.Info(fmt.Sprintf("Memory task %s queued (channel: %d/%d)", taskMessage.TaskID, len(e.taskChan), cap(e.taskChan)))
		default:
			common.Info(fmt.Sprintf("No available slot for memory task %s, nack", taskMessage.TaskID))
			if nackErr := handle.Nack(); nackErr != nil {
				common.Error(fmt.Sprintf("error nack memory task %s", taskMessage.TaskID), nackErr)
			}
		}
		return
	}

	if taskMessage.TaskType != common.TaskTypeIngestionTask {
		common.Info(fmt.Sprintf("task %s is not an ingestion task", taskMessage.TaskID))
		if err := handle.Ack(); err != nil {
			common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), err)
		}
		return
	}

	task, err := e.ingestionTaskSvc.StartRunning(e.ctx, taskMessage.TaskID)
	if err != nil {
		if errors.Is(err, common.ErrTaskNotFound) {
			common.Warn(fmt.Sprintf("task %s not found, skipping", taskMessage.TaskID))
			if ackErr := handle.Ack(); ackErr != nil {
				common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), ackErr)
			}
			return
		}
		// Recoverable activation failure (e.g. a DB blip): nack for
		// redelivery instead of dropping the message or killing the
		// consumer.
		common.Error(fmt.Sprintf("error setting task %s to running", taskMessage.TaskID), err)
		if nackErr := handle.Nack(); nackErr != nil {
			common.Error(fmt.Sprintf("error nack task %s", taskMessage.TaskID), nackErr)
		}
		return
	}
	if task == nil {
		common.Info(fmt.Sprintf("task %s is already removed", taskMessage.TaskID))
		if ackErr := handle.Ack(); ackErr != nil {
			common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), ackErr)
		}
		return
	}

	switch task.Status {
	case common.COMPLETED, common.STOPPED, common.FAILED:
		common.Info(fmt.Sprintf("task %s is already %s", taskMessage.TaskID, task.Status))
		if ackErr := handle.Ack(); ackErr != nil {
			common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), ackErr)
		}
		return
	case common.RUNNING:
		// Guard against MQ redelivery: if another worker in this
		// process is already processing this task, ack the redelivered
		// message and skip instead of scheduling it again.
		if !e.claimTask(task.ID) {
			common.Warn(fmt.Sprintf("task %s redelivered while worker still processing, ack skip (task_id=%s doc_id=%s kb_id=%s)",
				taskMessage.TaskID, task.ID, task.DocumentID, task.DatasetID))
			if ackErr := handle.Ack(); ackErr != nil {
				common.Error(fmt.Sprintf("error ack redelivered task %s", taskMessage.TaskID), ackErr)
			}
			return
		}
		claimedTaskID = task.ID
	default:
		// Unreachable given StartRunning normalizes every status to
		// RUNNING/COMPLETED/STOPPED/FAILED, but defensive: ack-skip an
		// unknown status instead of enqueuing it for execution.
		common.Warn(fmt.Sprintf("task %s in unexpected status %s, ack-skip", taskMessage.TaskID, task.Status))
		if ackErr := handle.Ack(); ackErr != nil {
			common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), ackErr)
		}
		return
	}

	// Construct TaskContext and carry the MQ handle so the worker can
	// Ack/Nack when the task reaches a terminal status.
	taskCtx := taskpkg.NewTaskContextForScheduling(e.ctx, task)
	taskCtx.Handle = handle

	// Push to task channel; if full, reject the task (backpressure).
	select {
	case e.taskChan <- taskCtx:
		claimedTaskID = "" // executeTask owns the release now
		common.Info(fmt.Sprintf("Task %s queued (channel: %d/%d)", task.ID, len(e.taskChan), cap(e.taskChan)))
	default:
		common.Info(fmt.Sprintf("No available slot for task %s, failed", task.ID))
		// claimedTaskID is still set; defer will call releaseTask.
		if nackErr := handle.Nack(); nackErr != nil {
			common.Error(fmt.Sprintf("error nack task %s", taskMessage.TaskID), nackErr)
		}
	}
}

func (e *Ingestor) startWorkerPool() {
	e.workerOnce.Do(func() {
		for i := int32(0); i < e.maxConcurrency; i++ {
			e.workerWg.Add(1)
			go e.workerLoop(i)
		}
		common.Info(fmt.Sprintf("Worker pool started with %d workers", e.maxConcurrency))
	})
}

func (e *Ingestor) workerLoop(id int32) {
	defer e.workerWg.Done()
	defer e.activeWorkers.Add(-1)
	e.activeWorkers.Add(1)
	common.Info(fmt.Sprintf("Worker %d started", id))
	for {
		select {
		case <-e.ctx.Done():
			return
		case taskCtx := <-e.taskChan:
			if taskCtx.Kind == taskpkg.TaskKindMemory {
				e.executeMemoryTask(e.ctx, taskCtx)
				continue
			}
			common.Info("task context:" + taskCtx.IngestionTask.ID)
			e.executeTask(e.ctx, taskCtx)
		}
	}
}

// executeMemoryTask runs one async memory-extraction task (TaskKindMemory) on
// a worker of the shared pool. Unlike ingestion tasks, memory tasks have no
// ingestion_task row / state machine: HandleSaveToMemoryTask persists the
// extracted messages and settles task progress on the way out.
//
// Settlement is error-category aware:
//   - Terminal failure (task row absent, already-failed, or progress=-1 already
//     persisted) is Acked so an already-consumed message is never redelivered
//     into an infinite nack loop.
//   - Transient failure (a task-load DB error before any durable marker, or an
//     LLM/network failure that did not reach progress=-1) is Nacked so the
//     message is redelivered and retried instead of being silently dropped.
func (e *Ingestor) executeMemoryTask(ctx context.Context, taskCtx *taskpkg.TaskContext) {
	taskID, _ := taskCtx.MemoryPayload["id"].(string)
	if taskID == "" {
		taskID, _ = taskCtx.MemoryPayload["task_id"].(string)
	}
	common.Info(fmt.Sprintf("Starting memory task %s", taskID))
	if taskCtx.Handle == nil {
		common.Warn("memory task handle is nil, skip")
		return
	}
	if e.memorySvc == nil {
		common.Warn(fmt.Sprintf("memory task %s: memory extractor disabled, ack", taskID))
		if err := taskCtx.Handle.Ack(); err != nil {
			common.Error(fmt.Sprintf("ack memory task %s", taskID), err)
		}
		return
	}
	if err := e.memorySvc.HandleSaveToMemoryTask(ctx, taskCtx.MemoryPayload); err != nil {
		// HandleSaveToMemoryTask wraps terminal outcomes in ErrMemoryTaskTerminal
		// (durable progress=-1 written, or no row to retry). Everything else is
		// transient and must be redelivered rather than dropped.
		if errors.Is(err, servicepkg.ErrMemoryTaskTerminal) {
			common.Error(fmt.Sprintf("memory task %s failed terminally, ack", taskID), err)
			if ackErr := taskCtx.Handle.Ack(); ackErr != nil {
				common.Error(fmt.Sprintf("ack failed memory task %s", taskID), ackErr)
			}
			return
		}
		common.Error(fmt.Sprintf("memory task %s failed transiently, nack for redelivery", taskID), err)
		if nackErr := taskCtx.Handle.Nack(); nackErr != nil {
			common.Error(fmt.Sprintf("nack memory task %s", taskID), nackErr)
		}
		return
	}
	common.Info(fmt.Sprintf("Memory task %s completed", taskID))
	if err := taskCtx.Handle.Ack(); err != nil {
		common.Error(fmt.Sprintf("ack memory task %s", taskID), err)
	}
}

func (e *Ingestor) executeTask(ctx context.Context, taskCtx *taskpkg.TaskContext) {
	task := taskCtx.IngestionTask
	common.Info(fmt.Sprintf("Starting task %s", task.ID))

	// Release the claim when executeTask returns so that a future
	// redelivery (after restart) can re-claim the task.
	defer e.releaseTask(task.ID)

	// Derive a per-task cancelable context so that an external cancel
	// signal (Redis cancel flag, mirrored from Python's {task_id}-cancel)
	// can stop only this task without affecting the whole Ingestor.
	perTaskCtx, perTaskCancel := context.WithCancel(taskCtx.Ctx)
	taskCtx.Ctx = perTaskCtx
	cancelDone := make(chan struct{})
	pollExited := make(chan struct{})
	go func() {
		defer close(pollExited)
		e.pollCancel(task.ID, perTaskCancel, cancelDone)
	}()
	defer func() {
		close(cancelDone)
		<-pollExited
		perTaskCancel()
	}()

	// Synchronous check: if already cancelled (e.g. flag set between MQ
	// delivery and worker claim), stop before the pipeline even starts.
	if e.cancelCheck(ctx, task.ID) {
		common.Info(fmt.Sprintf("Task %s cancel flag detected before pipeline start, cancelling", task.ID))
		perTaskCancel()
	}

	e.settleMessage(ctx, taskCtx, func(ctx context.Context) bool {
		return e.runTask(ctx, task)
	})
}

// markStopped transitions the task to STOPPED (terminal). It first calls
// RequestStop to handle RUNNING → STOPPING, then MarkStopped for the final
// STOPPING → STOPPED transition. Finally it cleans up the Redis cancel flag
// so that a future retry of the same task does not immediately re-cancel.
func (e *Ingestor) markStopped(ctx context.Context, taskID string) bool {
	if _, err := e.ingestionTaskSvc.RequestStop(ctx, taskID); err != nil {
		common.Error(fmt.Sprintf("markStopped: RequestStop task %s: %v", taskID, err), err)
		return false
	}
	if err := e.ingestionTaskSvc.MarkStopped(ctx, taskID); err != nil {
		common.Error(fmt.Sprintf("markStopped: MarkStopped task %s: %v", taskID, err), err)
		return false
	}
	if rc := redis2.Get(); rc != nil {
		utility.BestEffort(fmt.Sprintf("clear cancel flag for %s", taskID), func() error {
			rc.Delete(ctx, fmt.Sprintf("%s-cancel", taskID))
			return nil // Delete returns bool; the bool does not distinguish "not found" from "error"
		})
	}
	return true
}

// markFailed persists FAILED status for the task and reports whether the
// terminal status was durably written, so the caller can decide Ack vs Nack.
func (e *Ingestor) markFailed(ctx context.Context, taskID string) bool {
	if uErr := e.ingestionTaskSvc.MarkFailed(ctx, taskID); uErr != nil {
		common.Error(fmt.Sprintf("Failed to set task %s to FAILED", taskID), uErr)
		return false
	}
	return true
}

// runTask executes the task's business logic — run-count advance, document
// pipeline, and completion — behind the heartbeat. It returns whether the
// task reached a durably-persisted terminal status.
func (e *Ingestor) runTask(ctx context.Context, task *entity.IngestionTask) bool {
	select {
	case <-ctx.Done():
		common.Info(fmt.Sprintf("Task %s cancelled", task.ID))
		e.markCancelProgress(task)
		return e.markStopped(context.Background(), task.ID)
	default:
	}

	if err := e.ingestionTaskSvc.IncrementRunCount(ctx, task.ID); err != nil {
		common.Error(fmt.Sprintf("Failed to increment run count for task %s", task.ID), err)
		return e.markFailed(ctx, task.ID)
	}

	// This is a new run (IncrementRunCount succeeded). Any Redis cancel flag
	// that exists now is stale — a leftover from a previous run whose
	// markStopped failed to delete it. The current run's cancel is signalled
	// by the DB status (STOPPING), which defaultCancelCheck falls back to
	// when the Redis flag is absent. Clearing a stale flag here is safe:
	// a genuine concurrent cancel sets the task to STOPPING in DB.
	if rc := redis2.Get(); rc != nil {
		key := fmt.Sprintf("%s-cancel", task.ID)
		utility.BestEffort(fmt.Sprintf("clear stale cancel flag for %s", task.ID), func() error {
			rc.Delete(ctx, key)
			return nil // Delete returns bool; false may mean "key not found" or "error"
		})
	}

	if err := e.runDocumentTask(ctx, task); err != nil {
		if errors.Is(err, context.Canceled) {
			common.Info(fmt.Sprintf("Task %s cancelled during pipeline", task.ID))
			e.markCancelProgress(task)
			return e.markStopped(ctx, task.ID)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			common.Info(fmt.Sprintf("Task %s timed out during pipeline", task.ID))
			e.markTimeoutProgress(task)
			return e.markFailed(ctx, task.ID)
		}
		common.Error(fmt.Sprintf("Task %s failed", task.ID), err)
		return e.markFailed(ctx, task.ID)
	}

	if err := e.completeTask(ctx, task.ID); err != nil {
		common.Error(fmt.Sprintf("Task %s update status failed", task.ID), err)
		return false
	}

	common.Info(fmt.Sprintf("Task %s completed", task.ID))
	return true
}

// completeTask persists the task's terminal status after a successful pipeline.
// MarkCompleted is retried with backoff for transient (DB) failures only. A
// terminal transition failure - the task is no longer RUNNING because a
// concurrent stop (or another worker) moved it - is NOT retried: the pipeline
// already did the work, so completeOrSettle settles the task to its actual
// terminal state and the caller Acks instead of redelivering.
func (e *Ingestor) completeTask(ctx context.Context, taskID string) error {
	_, err := backoff.Retry(ctx, func() (struct{}, error) {
		return struct{}{}, e.completeOrSettle(ctx, taskID)
	}, backoff.WithMaxTries(3))
	return err
}

// completeOrSettle marks the task COMPLETED, or - if the transition is
// terminally invalid because the task is no longer RUNNING - settles it to its
// actual terminal state. Returns nil once the task is in any terminal state;
// returns a non-terminal (transient) error only for retry-worthy DB failures.
func (e *Ingestor) completeOrSettle(ctx context.Context, taskID string) error {
	if err := e.ingestionTaskSvc.MarkCompleted(ctx, taskID); err != nil {
		if isTerminalTransitionError(err) {
			return e.settleToTerminal(ctx, taskID)
		}
		return err
	}
	return nil
}

// isTerminalTransitionError reports whether err is a state-machine transition
// failure - an invalid transition or a lost optimistic CAS - meaning the task's
// status moved on and MarkCompleted will never succeed as-is. Not retry-worthy;
// the caller settles by the task's current status.
func isTerminalTransitionError(err error) bool {
	var ite *servicepkg.InvalidTaskTransitionError
	var tce *servicepkg.TaskStatusConflictError
	return errors.As(err, &ite) || errors.As(err, &tce)
}

// settleToTerminal finalizes a task whose MarkCompleted failed because it was
// no longer RUNNING. STOPPING is moved to STOPPED via markStopped (which also
// clears the Redis cancel flag so a future retry does not immediately
// re-cancel); already-terminal states (COMPLETED/STOPPED/FAILED) need no
// action. An unexpected status returns an error so the caller nacks and
// redelivery settles it.
func (e *Ingestor) settleToTerminal(ctx context.Context, taskID string) error {
	task, err := e.ingestionTaskSvc.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	switch task.Status {
	case common.STOPPING:
		if !e.markStopped(ctx, taskID) {
			return fmt.Errorf("task %s: settle to STOPPED failed", taskID)
		}
		return nil
	case common.COMPLETED, common.STOPPED, common.FAILED:
		return nil
	default:
		return fmt.Errorf("task %s in unexpected status %s after transition failure", taskID, task.Status)
	}
}

// settleMessage runs body under a heartbeat, then settles the MQ message. The
// heartbeat is stopped (and waited on) before ack/nack — see startHeartbeat.
// A panic in body is recovered: the task is marked FAILED and the message is
// Nacked for redelivery, so a single task's panic never crashes the worker.
// Settlement queries the DB for the task's actual status: a terminal state
// (COMPLETED/STOPPED/FAILED) means Ack; anything else means Nack. The body's
// return value is advisory only — DB truth is authoritative (BP1).
func (e *Ingestor) settleMessage(ctx context.Context, taskCtx *taskpkg.TaskContext, body func(context.Context) bool) (terminal bool) {
	stop := e.startHeartbeat(taskCtx)
	defer func() {
		stop() // stop heartbeat (and wait) before ack/nack
		if r := recover(); r != nil {
			// Recover the panic so the worker process survives. Mark the
			// task FAILED so a redelivery does not re-run a poison message
			// (processMessage Ack-skips an already-FAILED task); Nack for
			// redelivery. The broker's redelivery limit handles deterministic
			// poison messages.
			common.Error(fmt.Sprintf("task %s panicked: %v", taskCtx.IngestionTask.ID, r), fmt.Errorf("%v", r))
			e.markFailed(ctx, taskCtx.IngestionTask.ID)
			terminal = false
		}
		// Settlement authority is the DB, not the in-memory bool (BP1).
		// Fall back to the in-memory bool only when the DB is unavailable.
		if dbTerminal, ok := e.safeGetTerminal(ctx, taskCtx.IngestionTask.ID); ok {
			terminal = dbTerminal
		}
		e.ackOrNack(taskCtx, terminal)
	}()
	terminal = body(taskCtx.Ctx)
	return
}

// safeGetTerminal queries the DB for the task's actual status and returns
// whether it is terminal (COMPLETED/STOPPED/FAILED). A recover guards
// against nil-DB panics in test environments — in that case (false, false)
// is returned so the caller falls back to the in-memory bool.
func (e *Ingestor) safeGetTerminal(ctx context.Context, taskID string) (terminal bool, ok bool) {
	defer func() { recover() }()
	task, err := e.ingestionTaskSvc.GetTask(ctx, taskID)
	if err != nil {
		return false, false
	}
	return task.Status == common.COMPLETED ||
		task.Status == common.STOPPED ||
		task.Status == common.FAILED, true
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

// defaultCancelCheck reads the Redis cancel flag that Python sets via
// REDIS_CONN.set(f"{task_id}-cancel", "x"). Falls back to checking the
// task status in DB when Redis is unavailable — a STOPPING status
// (set by RequestStop) is treated as a cancel signal.
func (e *Ingestor) defaultCancelCheck(ctx context.Context, taskID string) bool {
	rc := redis2.Get()
	if rc != nil {
		if ok, _ := rc.Exist(ctx, fmt.Sprintf("%s-cancel", taskID)); ok {
			return true
		}
	}
	task, err := e.ingestionTaskSvc.GetTask(ctx, taskID)
	if err != nil {
		return false
	}
	return task.Status == common.STOPPING
}

// pollCancel ticks every 3s to check the cancel flag. When cancelCheck
// returns true it cancels the per-task context, which causes the pipeline's
// next ctx.Err() check to abort and runTask to record progress=-1. The
// goroutine exits when done is closed (executeTask returns).
func (e *Ingestor) pollCancel(taskID string, cancel context.CancelFunc, done <-chan struct{}) {
	// checkOnce runs cancelCheck in a goroutine so the caller can select
	// between the result and the done signal. This prevents a blocked
	// cancelCheck (e.g. stuck DB call) from blocking pollCancel itself,
	// which would cause executeTask's defer to deadlock on <-pollExited.
	checkOnce := func() <-chan bool {
		result := make(chan bool, 1)
		go func() {
			defer func() { recover() }() // goroutine may outlive pollCancel; must not crash process
			result <- e.cancelCheck(e.ctx, taskID)
		}()
		return result
	}

	// Initial check (immediately, for the test path).
	select {
	case <-done:
		return
	case ok := <-checkOnce():
		if ok {
			common.Info(fmt.Sprintf("Task %s cancel flag detected during polling, cancelling pipeline", taskID))
			cancel()
			return
		}
	}

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			select {
			case <-done:
				return
			case ok := <-checkOnce():
				if ok {
					common.Info(fmt.Sprintf("Task %s cancel flag detected during polling, cancelling pipeline", taskID))
					cancel()
					return
				}
			}
		}
	}
}

// markCancelProgress writes the cancelled-progress markers to the document
// row. Mirrors Python's cancel_all_task_of: progress=-1, run=CANCEL, and an
// appended timestamped cancel message (progress_msg += cancelMsg).
func (e *Ingestor) markCancelProgress(task *entity.IngestionTask) {
	svc := documentpkg.NewDocumentService()
	doc, err := svc.GetDocumentByID(e.ctx, task.DocumentID)
	if err != nil {
		common.Error(fmt.Sprintf("markCancelProgress: load document %s: %v", task.DocumentID, err), err)
		return
	}
	cancelMsg := fmt.Sprintf("\n%s Task stopped by user.", time.Now().Format("15:04:05"))
	existingMsg := ""
	if doc.ProgressMsg != nil {
		existingMsg = *doc.ProgressMsg
	}
	_ = svc.UpdateRunProgress(e.ctx, task.DocumentID, -1.0, string(entity.TaskStatusCancel), existingMsg+cancelMsg)
}

// markTimeoutProgress writes the timeout-progress markers to the document
// row. Unlike cancellation (markCancelProgress), this records a TIMEOUT
// failure rather than a user-initiated stop.
func (e *Ingestor) markTimeoutProgress(task *entity.IngestionTask) {
	svc := documentpkg.NewDocumentService()
	doc, err := svc.GetDocumentByID(e.ctx, task.DocumentID)
	if err != nil {
		common.Error(fmt.Sprintf("markTimeoutProgress: load document %s: %v", task.DocumentID, err), err)
		return
	}
	timeoutMsg := fmt.Sprintf("\n%s Task timed out.", time.Now().Format("15:04:05"))
	existingMsg := ""
	if doc.ProgressMsg != nil {
		existingMsg = *doc.ProgressMsg
	}
	_ = svc.UpdateRunProgress(e.ctx, task.DocumentID, -1.0, string(entity.TaskStatusFail), existingMsg+timeoutMsg)
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
	e.currentTasks[taskID] = struct{}{}
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
	docTaskCtx, err := taskpkg.LoadFromIngestionTask(ctx, ingestionTask)
	if err != nil {
		return fmt.Errorf("load task context for %s: %w", ingestionTask.ID, err)
	}

	pipelineID := strings.TrimSpace(docTaskCtx.PipelineID)
	parserID := strings.TrimSpace(docTaskCtx.Doc.ParserID)
	isBuiltin := pipelineID == ""

	if pipelineID == "" {
		if parserID == "" {
			return fmt.Errorf("ingestion task %s: no pipeline_id or parser_id configured for document %s", ingestionTask.ID, docTaskCtx.Doc.ID)
		}
		pipelineID = parserID // builtin: parser_id acts as the logical pipeline identifier
	}

	docTaskCtx.Ctx = ctx
	// The sink owns all document/ingestion_task_log/ingestion_task.component_total
	// writes for this run; inject it into the executor so the pipeline reports
	// progress to the service layer instead of touching the DAO directly.
	executor, err := taskpkg.NewPipelineExecutor(docTaskCtx, pipelineID, 0)
	if err != nil {
		return err
	}
	if isBuiltin {
		// Builtin path: load DSL from the embedded registry, skipping canvas DB lookup.
		executor.WithLoadDSLFunc(func(ctx context.Context, _ string) (string, string, error) {
			common.Info(fmt.Sprintf("load built in DSL for: %s", parserID))
			dsl, lerr := pipelinepkg.LoadBuiltinDSL(parserID)
			if lerr != nil {
				return "", "", lerr
			}
			return dsl, parserID, nil
		})
	}
	result, err := executor.WithRequireResume().WithProgressSink(newProgressSink(ctx, e.ingestionTaskSvc)).Execute(docTaskCtx.Ctx)
	if err != nil {
		return err
	}
	e.docState.apply(ctx, result)
	return nil
}

// Stop gracefully shuts down the ingestor. It cancels the root context so
// idle workers exit immediately and in-flight pipelines abort at their next
// ctx.Err() check, then waits for workers to return. The wait is bounded by
// ctx: a stage that does not honor cancellation (e.g. a native CGO parse)
// would otherwise block workerWg.Wait() indefinitely; when ctx expires Stop
// returns and leaves the broker to redeliver any in-flight messages
// (at-least-once). Callers must pass a deadline-bearing context.
func (e *Ingestor) Stop(ctx context.Context) {
	common.Info(fmt.Sprintf("Stopping ingestor %s", e.id))
	e.cancel()

	waitDone := make(chan struct{})
	go func() {
		e.workerWg.Wait()
		e.compileWg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		common.Info("All tasks completed")
	case <-ctx.Done():
		e.tasksMu.RLock()
		ids := make([]string, 0, len(e.currentTasks))
		for id := range e.currentTasks {
			ids = append(ids, id)
		}
		e.tasksMu.RUnlock()
		common.Warn(fmt.Sprintf("Stop timed out with %d task(s) still in-flight (will be redelivered by broker): %v", len(ids), ids))
	}

	// Signal shutdown completion so the cmd-side select on <-ShutdownCh
	// unblocks (the admin graceful-shutdown path). Guarded by stopOnce: a
	// repeated Stop must not double-close the channel.
	e.stopOnce.Do(func() { close(e.ShutdownCh) })
}
