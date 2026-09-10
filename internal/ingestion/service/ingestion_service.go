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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ragflow/internal/agent/canvas"
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
	"go.uber.org/zap"
)

// defaultHeartbeatInterval paces the InProgress() working pulse that keeps an
// in-flight message's ack deadline renewed while a worker parses. It MUST stay
// comfortably below the consumer's ack deadline, which the server normalizes
// to BackOff[0] = 5s (see NatsEngine.InitConsumer): a pulse slower than the
// deadline lets the broker redeliver mid-run. A duplicate delivery renews its
// lease but remains unsettled until the owning worker reaches a durable
// outcome. If the worker or process stops, the broker can redeliver the
// unsettled message after the ack deadline.
const defaultHeartbeatInterval = 2 * time.Second

type Ingestor struct {
	id     string
	name   string
	ctx    context.Context // execution context for handles already owned by workers
	cancel context.CancelFunc

	dispatchCtx    context.Context // worker queue registration and Pull lifecycle
	dispatchCancel context.CancelFunc

	// Configuration
	maxConcurrency    int32
	supportedDocTypes []string
	version           string
	heartbeatInterval time.Duration

	// Runtime state
	currentTasks  map[string]struct{} // set of task IDs currently claimed by a worker
	tasksMu       sync.RWMutex
	activeWorkers atomic.Int32 // number of worker goroutines currently in workerLoop
	activeLeases  map[*Heartbeat]*activeLease
	leasesMu      sync.Mutex
	stopLeases    atomic.Bool

	// Shutdown channel - receive on this to trigger graceful shutdown
	ShutdownCh chan struct{}

	// Worker pool
	workerQueue  chan *worker
	dispatcherWg sync.WaitGroup
	workerWg     sync.WaitGroup
	startOnce    sync.Once
	startErr     error
	workerOnce   sync.Once // guards startWorkerPool; must NOT be startOnce (Start wraps start() in startOnce, and start() calls startWorkerPool -> re-entry deadlock)
	stopOnce     sync.Once // guards close(ShutdownCh) against double-close on repeated Stop

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

	// runMemoryTask dispatches one async memory-extraction task. Tests may
	// override this to inject a panicking/failing runner without a live DB or
	// real MemoryMessageService. Defaults to defaultRunMemoryTask, which calls
	// memorySvc.HandleSaveToMemoryTask with the envelope task id and a unique
	// database lease owner.
	runMemoryTask func(ctx context.Context, taskID, leaseOwner string) (servicepkg.MemoryTaskDisposition, error)

	// cancelCheck is polled periodically (every 3s) during task execution.
	// When it returns true the task's context is cancelled, which causes the
	// pipeline to stop at the next ctx.Err() check. Defaults to a Redis
	// cancel-flag lookup that mirrors Python's has_canceled(). Tests may
	// override this to simulate cancel without Redis.
	cancelCheck func(ctx context.Context, taskID string) bool

	checkpointExists func(ctx context.Context, taskID string) (bool, error)
}

type worker struct {
	id    int32
	inbox chan common.TaskHandle
}

type activeLease struct {
	abandoned atomic.Bool
}

func NewIngestor(name string, maxConcurrency int32, supportedTypes []string) *Ingestor {
	if maxConcurrency <= 0 {
		maxConcurrency = int32(runtime.NumCPU())
	}
	ctx, cancel := context.WithCancel(context.Background())
	dispatchCtx, dispatchCancel := context.WithCancel(context.Background())
	id := utility.GenerateUUID()
	ingestor := &Ingestor{
		id:                id,
		name:              name,
		ctx:               ctx,
		cancel:            cancel,
		dispatchCtx:       dispatchCtx,
		dispatchCancel:    dispatchCancel,
		maxConcurrency:    maxConcurrency,
		supportedDocTypes: supportedTypes,
		version:           "1.0.0",
		currentTasks:      make(map[string]struct{}),
		activeLeases:      make(map[*Heartbeat]*activeLease),
		workerQueue:       make(chan *worker, maxConcurrency),
		ShutdownCh:        make(chan struct{}, 1),
		ingestionTaskSvc:  servicepkg.NewIngestionTaskService(),
		docState:          newDocStateUpdater(),
		heartbeatInterval: defaultHeartbeatInterval,
	}
	ingestor.runDocumentTask = ingestor.defaultRunDocumentTask
	ingestor.runMemoryTask = ingestor.defaultRunMemoryTask
	ingestor.cancelCheck = ingestor.defaultCancelCheck
	ingestor.checkpointExists = canvas.RedisCheckpointExists
	ingestor.kcConcurrency = maxConcurrency // parallel dataset-level compile workers default to the task width
	return ingestor
}

func (e *Ingestor) ID() string {
	return e.id
}

// consumeErrorBackoff paces failed Pull requests so a persistent MQ failure
// does not pin a CPU. The backoff is cancellable and does not block another
// idle worker from making its own Pull request.
const consumeErrorBackoff = 1 * time.Second

const taskPullRequestTimeout = 1 * time.Second

func (e *Ingestor) Start() error {
	common.Info(fmt.Sprintf("Ingestor %s initialized", e.id))
	e.startOnce.Do(func() {
		e.startErr = e.start()
	})
	return e.startErr
}

// start runs the full startup sequence. It is invoked at most once (guarded by
// startOnce in Start) so repeated Start calls cannot launch duplicate worker
// pools, compile consumers, or consume loops, and the first initialization
// error is retained and returned to every later caller.
func (e *Ingestor) start() error {
	msgQueueEngine := engine.GetMessageQueueEngine()
	if msgQueueEngine == nil {
		return fmt.Errorf("message queue engine not initialized; run engine.InitMessageQueue first")
	}
	if err := msgQueueEngine.InitConsumer(common.TaskSubject); err != nil {
		return err
	}
	if dao.DB != nil {
		if err := e.ingestionTaskSvc.ScheduleCreatedTasks(e.ctx); err != nil {
			common.Warn(fmt.Sprintf("schedule CREATED ingestion tasks at startup: %v", err))
		}
	}

	// Start the task worker pool and the dataset-level compile consumer as
	// owned goroutines joined by Stop via workerWg/compileWg. Start follows
	// the standard lifecycle contract: it returns immediately after kicking
	// these off rather than blocking on the consume loop itself.
	e.startWorkerPool()
	e.startDatasetKnowledgeCompile()

	// Run the main tasks.RAGFLOW dispatcher off the caller's goroutine so Start
	// returns promptly. Stop joins it before cancelling task execution.
	e.dispatcherWg.Add(1)
	go e.consumeLoop()

	return nil
}

// consumeLoop is the worker dispatcher for tasks.RAGFLOW. It waits for an
// available worker before synchronously pulling one task, preventing prefetch
// and limiting this ingestor to one outstanding broker request.
func (e *Ingestor) consumeLoop() {
	defer e.dispatcherWg.Done()
	msgQueueEngine := engine.GetMessageQueueEngine()
	for {
		var w *worker
		select {
		case <-e.dispatchCtx.Done():
			return
		case w = <-e.workerQueue:
		}
		if e.dispatchCtx.Err() != nil {
			e.returnWorker(w)
			return
		}

		pullStart := time.Now()
		pullCtx, cancel := context.WithTimeout(e.dispatchCtx, taskPullRequestTimeout)
		handle, err := msgQueueEngine.PullMessage(pullCtx)
		cancel()
		if err != nil {
			e.returnWorker(w)
			e.logPullError(err)
			if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
				e.waitAfterPullError()
			}
			continue
		}
		if handle == nil {
			e.returnWorker(w)
			continue
		}

		recvTime := time.Now()
		latency := time.Since(pullStart)
		common.Debug(fmt.Sprintf("Pull delivered handle in %v", latency),
			zap.Duration("pull_first_handle_latency", latency),
		)
		select {
		case w.inbox <- handle:
			handoffDuration := time.Since(recvTime)
			common.Debug(fmt.Sprintf("Handed off handle to worker %d in %v", w.id, handoffDuration),
				zap.Int32("worker_id", w.id),
				zap.Duration("handoff_duration", handoffDuration),
			)
		case <-e.dispatchCtx.Done():
			if err := handle.Nack(); err != nil {
				common.Error("nack task after shutdown interrupted handoff", err)
			}
			return
		}
	}
}

func (e *Ingestor) logPullError(err error) {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return
	}
	common.Error("error pulling task message", err)
}

func (e *Ingestor) waitAfterPullError() {
	select {
	case <-time.After(consumeErrorBackoff):
	case <-e.dispatchCtx.Done():
	}
}

func (e *Ingestor) returnWorker(w *worker) {
	select {
	case e.workerQueue <- w:
	case <-e.dispatchCtx.Done():
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

// handleAndExecute owns one handle from its hand-off to final settlement. It
// runs only in the worker that received the handle through its private inbox.
func (e *Ingestor) handleAndExecute(handle common.TaskHandle) {
	startTime := time.Now()
	taskMessage := handle.GetMessage()
	common.Info(fmt.Sprintf("Received task id: %s, type: %s", taskMessage.TaskID, taskMessage.TaskType),
		zap.String("task_id", taskMessage.TaskID),
		zap.String("task_type", taskMessage.TaskType),
	)
	defer func() {
		settlementDuration := time.Since(startTime)
		common.Debug(fmt.Sprintf("Task %s settlement finished in %v", taskMessage.TaskID, settlementDuration),
			zap.String("task_id", taskMessage.TaskID),
			zap.Duration("settlement_duration", settlementDuration),
		)
	}()

	hb := NewHeartbeat(taskMessage.TaskID, handle, e.heartbeatInterval).
		WithContext(context.Background())
	hb.Start()
	e.registerLease(hb)
	defer e.unregisterLease(hb)

	// Memory-extraction tasks share the tasks.RAGFLOW consumer and the worker
	// pool with ingestion tasks. They do NOT use the ingestion state machine.
	if taskMessage.TaskType == common.TaskTypeMemory {
		if e.memorySvc == nil {
			common.Warn(fmt.Sprintf("memory task %s received but memory extractor is disabled, ack", taskMessage.TaskID))
			e.ackHandle(hb, handle, taskMessage.TaskID)
			return
		}
		if taskMessage.TaskID == "" {
			common.Warn("memory task with empty task id received, ack")
			e.ackHandle(hb, handle, taskMessage.TaskID)
			return
		}
		taskCtx := taskpkg.NewMemoryTaskContextForScheduling(e.ctx, taskMessage.TaskID, handle)
		e.executeMemoryTaskWithHeartbeat(e.ctx, taskCtx, hb)
		return
	}

	if taskMessage.TaskType != common.TaskTypeIngestionTask {
		common.Info(fmt.Sprintf("task %s is not an ingestion task", taskMessage.TaskID))
		e.ackHandle(hb, handle, taskMessage.TaskID)
		return
	}

	task, err := e.ingestionTaskSvc.StartRunning(e.ctx, taskMessage.TaskID)
	if err != nil {
		if errors.Is(err, common.ErrTaskNotFound) {
			common.Warn(fmt.Sprintf("task %s not found, skipping", taskMessage.TaskID))
			e.ackHandle(hb, handle, taskMessage.TaskID)
			return
		}
		common.Error(fmt.Sprintf("error setting task %s to running", taskMessage.TaskID), err)
		e.nackHandle(hb, handle, taskMessage.TaskID)
		return
	}
	if task == nil {
		common.Info(fmt.Sprintf("task %s is already removed", taskMessage.TaskID))
		e.ackHandle(hb, handle, taskMessage.TaskID)
		return
	}

	switch task.Status {
	case common.COMPLETED, common.STOPPED, common.FAILED:
		common.Info(fmt.Sprintf("task %s is already %s", taskMessage.TaskID, task.Status))
		e.ackHandle(hb, handle, taskMessage.TaskID)
		return
	case common.RUNNING:
		if !e.claimTask(task.ID) {
			common.Warn(fmt.Sprintf("task %s redelivered while worker still processing, renew lease (task_id=%s doc_id=%s kb_id=%s)",
				taskMessage.TaskID, task.ID, task.DocumentID, task.DatasetID))
			e.renewDuplicateHandle(hb, handle, taskMessage.TaskID)
			return
		}
	default:
		common.Warn(fmt.Sprintf("task %s in unexpected status %s, ack-skip", taskMessage.TaskID, task.Status))
		e.ackHandle(hb, handle, taskMessage.TaskID)
		return
	}

	taskCtx := taskpkg.NewTaskContextForScheduling(e.ctx, task)
	taskCtx.Handle = handle
	e.executeTaskWithHeartbeat(e.ctx, taskCtx, hb)
}

func (e *Ingestor) ackHandle(hb *Heartbeat, handle common.TaskHandle, taskID string) {
	hb.Stop()
	if e.leaseAbandoned(hb) {
		return
	}
	if err := handle.Ack(); err != nil {
		common.Error(fmt.Sprintf("ack task %s", taskID), err)
	}
}

func (e *Ingestor) nackHandle(hb *Heartbeat, handle common.TaskHandle, taskID string) {
	hb.Stop()
	if e.leaseAbandoned(hb) {
		return
	}
	if err := handle.Nack(); err != nil {
		common.Error(fmt.Sprintf("nack task %s", taskID), err)
	}
}

func (e *Ingestor) renewDuplicateHandle(hb *Heartbeat, handle common.TaskHandle, taskID string) {
	hb.Stop()
	if e.leaseAbandoned(hb) {
		return
	}
	if err := handle.InProgress(); err != nil {
		common.Error(fmt.Sprintf("renew redelivered task %s", taskID), err)
	}
}

func (e *Ingestor) registerLease(hb *Heartbeat) {
	lease := &activeLease{}
	e.leasesMu.Lock()
	stop := e.stopLeases.Load()
	if stop {
		lease.abandoned.Store(true)
	}
	e.activeLeases[hb] = lease
	e.leasesMu.Unlock()
	if stop {
		hb.Stop()
	}
}

func (e *Ingestor) unregisterLease(hb *Heartbeat) {
	e.leasesMu.Lock()
	delete(e.activeLeases, hb)
	e.leasesMu.Unlock()
}

func (e *Ingestor) leaseAbandoned(hb *Heartbeat) bool {
	e.leasesMu.Lock()
	lease := e.activeLeases[hb]
	e.leasesMu.Unlock()
	return lease != nil && lease.abandoned.Load()
}

func (e *Ingestor) stopActiveLeases() {
	e.leasesMu.Lock()
	e.stopLeases.Store(true)
	leases := make([]*Heartbeat, 0, len(e.activeLeases))
	for heartbeat, lease := range e.activeLeases {
		lease.abandoned.Store(true)
		leases = append(leases, heartbeat)
	}
	e.leasesMu.Unlock()

	for _, lease := range leases {
		lease.Stop()
	}
}

func (e *Ingestor) startWorkerPool() {
	e.workerOnce.Do(func() {
		for i := int32(0); i < e.maxConcurrency; i++ {
			e.workerWg.Add(1)
			w := &worker{id: i, inbox: make(chan common.TaskHandle)}
			go e.workerLoop(w)
		}
		common.Info(fmt.Sprintf("Worker pool started with %d workers", e.maxConcurrency))
	})
}

func (e *Ingestor) workerLoop(w *worker) {
	defer e.workerWg.Done()
	defer e.activeWorkers.Add(-1)
	e.activeWorkers.Add(1)
	common.Info(fmt.Sprintf("Worker %d started", w.id))
	for {
		select {
		case e.workerQueue <- w:
		case <-e.dispatchCtx.Done():
			return
		}

		select {
		case handle := <-w.inbox:
			e.handleAndExecute(handle)
		case <-e.dispatchCtx.Done():
			return
		}
	}
}

func (e *Ingestor) executeMemoryTask(ctx context.Context, taskCtx *taskpkg.TaskContext) {
	hb := NewHeartbeat(taskCtx.ID(), taskCtx.Handle, e.heartbeatInterval).WithContext(ctx)
	hb.Start()
	e.executeMemoryTaskWithHeartbeat(ctx, taskCtx, hb)
}

// executeMemoryTaskWithHeartbeat settles a memory handle with the heartbeat
// that the receiving worker started before admission.
func (e *Ingestor) executeMemoryTaskWithHeartbeat(ctx context.Context, taskCtx *taskpkg.TaskContext, hb *Heartbeat) {
	taskID := taskCtx.ID()

	settleAck := false
	defer func() {
		hb.Stop()
		if r := recover(); r != nil {
			common.Error(fmt.Sprintf("memory task %s panicked: %v", taskID, r), fmt.Errorf("%v", r))
		}
		if taskCtx.Handle == nil || e.leaseAbandoned(hb) {
			return
		}
		if settleAck {
			if err := taskCtx.Handle.Ack(); err != nil {
				common.Error(fmt.Sprintf("ack memory task %s", taskID), err)
			}
		}
	}()

	common.Info(fmt.Sprintf("Starting memory task %s", taskID))
	if taskCtx.Handle == nil {
		common.Warn("memory task handle is nil, skip")
		return
	}
	if e.memorySvc == nil {
		common.Warn(fmt.Sprintf("memory task %s: memory extractor disabled, ack", taskID))
		settleAck = true
		return
	}
	leaseOwner := fmt.Sprintf("%s:%s", e.id, utility.GenerateUUID())
	disposition, err := e.runMemoryTask(ctx, taskID, leaseOwner)
	if err != nil {
		common.Error(fmt.Sprintf("memory task %s execution failed", taskID), err)
	}
	if disposition == servicepkg.MemoryTaskAcknowledge {
		settleAck = true
		common.Info(fmt.Sprintf("Memory task %s delivery acknowledged", taskID))
		return
	}
	common.Warn(fmt.Sprintf("memory task %s delivery left unsettled for durable recovery", taskID))
}

// defaultRunMemoryTask is the production memory-task runner. It is held behind
// the runMemoryTask field so tests can substitute a panicking/failing runner
// without a live DB or real MemoryMessageService.
func (e *Ingestor) defaultRunMemoryTask(ctx context.Context, taskID, leaseOwner string) (servicepkg.MemoryTaskDisposition, error) {
	return e.memorySvc.HandleSaveToMemoryTask(ctx, taskID, leaseOwner)
}

func (e *Ingestor) executeTask(ctx context.Context, taskCtx *taskpkg.TaskContext) {
	hb := NewHeartbeat(taskCtx.ID(), taskCtx.Handle, e.heartbeatInterval).WithContext(taskCtx.Ctx)
	hb.Start()
	e.executeTaskWithHeartbeat(ctx, taskCtx, hb)
}

// executeTaskWithHeartbeat executes one document task using the lease already
// started by the worker that owns its Handle.
func (e *Ingestor) executeTaskWithHeartbeat(ctx context.Context, taskCtx *taskpkg.TaskContext, hb *Heartbeat) {
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

	e.settleMessage(ctx, taskCtx, hb, func(ctx context.Context) bool {
		return e.runTask(ctx, task)
	})
}

// markStopped transitions the task to STOPPED (terminal). It first calls
// RequestStop to handle RUNNING → STOPPING, then MarkStopped for the final
// STOPPING → STOPPED transition. Finally it cleans up the Redis cancel flag
// so that a future retry of the same task does not immediately re-cancel.
// The caller's context is detached before touching the DB: markStopped runs
// on failure/cancel/shutdown paths whose context is typically already
// cancelled, and a contaminated context would make the terminal write fail
// exactly when it matters most.
func (e *Ingestor) markStopped(ctx context.Context, taskID string) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
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
// The caller's context is detached before touching the DB (see markStopped):
// failure paths usually carry an already-cancelled context, and losing the
// FAILED write would turn a settled task into a redelivery loop.
func (e *Ingestor) markFailed(ctx context.Context, taskID string) bool {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
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
		if e.ctx.Err() != nil || e.dispatchCtx.Err() != nil {
			common.Info(fmt.Sprintf("Task %s aborted due to ingestor shutdown, leaving for redelivery", task.ID))
			return false
		}
		common.Info(fmt.Sprintf("Task %s cancelled", task.ID))
		e.markCancelProgress(task)
		stopped := e.markStopped(context.Background(), task.ID)
		if stopped {
			e.recordTerminalPipelineLog(context.Background(), task, string(entity.TaskStatusCancel))
		}
		return stopped
	default:
	}

	// The three DB/Redis lookups below must survive a context cancelled
	// between the pre-check and the call (cancel poll races the pipeline
	// start): detach the caller's ctx with a short timeout so a cancelled
	// run fails through markFailed with a real error, not a context error.
	dbCtx, dbCancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer dbCancel()
	if err := e.ingestionTaskSvc.IncrementRunCount(dbCtx, task.ID); err != nil {
		common.Error(fmt.Sprintf("Failed to increment run count for task %s", task.ID), err)
		ok := e.markFailed(ctx, task.ID)
		if ok {
			e.recordTerminalPipelineLog(ctx, task, string(entity.TaskStatusFail))
		}
		return ok
	}
	checkpointExists := e.checkpointExists
	if checkpointExists == nil {
		checkpointExists = canvas.RedisCheckpointExists
	}
	resumeCheckpoint, checkpointErr := checkpointExists(dbCtx, task.ID)
	if checkpointErr != nil {
		common.Error(fmt.Sprintf("Failed to check checkpoint for task %s", task.ID), checkpointErr)
		ok := e.markFailed(ctx, task.ID)
		if ok {
			e.recordTerminalPipelineLog(ctx, task, string(entity.TaskStatusFail))
		}
		return ok
	}
	if !resumeCheckpoint {
		if err := e.ingestionTaskSvc.ClearComponentProgress(dbCtx, task.ID); err != nil {
			common.Error(fmt.Sprintf("Failed to clear previous component progress for task %s", task.ID), err)
			ok := e.markFailed(ctx, task.ID)
			if ok {
				e.recordTerminalPipelineLog(ctx, task, string(entity.TaskStatusFail))
			}
			return ok
		}
	} else {
		common.Info(fmt.Sprintf("Preserving component progress for checkpoint resume of task %s", task.ID))
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
			if e.ctx.Err() != nil || e.dispatchCtx.Err() != nil {
				common.Info(fmt.Sprintf("Task %s pipeline interrupted by ingestor shutdown, leaving for redelivery", task.ID))
				return false
			}
			common.Info(fmt.Sprintf("Task %s cancelled during pipeline", task.ID))
			e.markCancelProgress(task)
			stopped := e.markStopped(ctx, task.ID)
			if stopped {
				e.recordTerminalPipelineLog(ctx, task, string(entity.TaskStatusCancel))
			}
			return stopped
		}
		if errors.Is(err, context.DeadlineExceeded) {
			common.Info(fmt.Sprintf("Task %s timed out during pipeline", task.ID))
			e.markTimeoutProgress(task)
			ok := e.markFailed(ctx, task.ID)
			if ok {
				e.recordTerminalPipelineLog(ctx, task, string(entity.TaskStatusFail))
			}
			return ok
		}
		common.Error(fmt.Sprintf("Task %s failed", task.ID), err)
		ok := e.markFailed(ctx, task.ID)
		if ok {
			e.recordTerminalPipelineLog(ctx, task, string(entity.TaskStatusFail))
		}
		return ok
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

// settleMessage runs body under an injected heartbeat lease, then settles the MQ message.
// The heartbeat is stopped (and waited on) before ack/nack — see Heartbeat.Stop.
// A panic in body is recovered: the task is marked FAILED and the message is
// Nacked for redelivery, so a single task's panic never crashes the worker.
// Settlement queries the DB for the task's actual status: a terminal state
// (COMPLETED/STOPPED/FAILED) means Ack; anything else means Nack. The body's
// return value is advisory only — DB truth is authoritative (BP1).
func (e *Ingestor) settleMessage(ctx context.Context, taskCtx *taskpkg.TaskContext, hb *Heartbeat, body func(context.Context) bool) (terminal bool) {
	defer func() {
		hb.Stop() // stop heartbeat (and wait) before ack/nack
		if r := recover(); r != nil {
			// Recover the panic so the worker process survives. Mark the
			// task FAILED so a redelivery does not re-run a poison message
			// (handleAndExecute Ack-skips an already-FAILED task); Nack for
			// redelivery. The broker's redelivery limit handles deterministic
			// poison messages.
			common.Error(fmt.Sprintf("task %s panicked: %v", taskCtx.IngestionTask.ID, r), fmt.Errorf("%v", r))
			e.markFailed(ctx, taskCtx.IngestionTask.ID)
			terminal = false
		}
		if e.leaseAbandoned(hb) {
			return
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
// The claim is released by releaseTask when the worker finishes, so a future
// redelivery (after restart) can re-claim the task. Broker lease renewal is a
// separate concern handled by the worker-owned Heartbeat until settlement.
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

func (e *Ingestor) recordTerminalPipelineLog(ctx context.Context, ingestionTask *entity.IngestionTask, status string) {
	if ingestionTask == nil || status == "" {
		return
	}
	ctx = context.WithoutCancel(ctx)
	if err := taskpkg.RecordPipelineLog(ctx, dao.DB, taskpkg.PipelineLogInput{
		KbID:       ingestionTask.DatasetID,
		DocumentID: ingestionTask.DocumentID,
		Status:     status,
	}); err != nil {
		common.Warn(fmt.Sprintf("record terminal pipeline log for task %s document %s: %v", ingestionTask.ID, ingestionTask.DocumentID, err))
	}
}

// Stop first stops worker registration and the dispatcher's Pull, then cancels
// execution. The caller's deadline bounds the wait for non-cooperative tasks.
func (e *Ingestor) Stop(ctx context.Context) {
	common.Info(fmt.Sprintf("Stopping ingestor %s", e.id))
	e.dispatchCancel()

	waitDone := make(chan struct{})
	go func() {
		e.dispatcherWg.Wait()
		e.cancel()
		e.workerWg.Wait()
		e.compileWg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
		common.Info("All tasks completed")
	case <-ctx.Done():
		e.stopActiveLeases()
		e.cancel()
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
