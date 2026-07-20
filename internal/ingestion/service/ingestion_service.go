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
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ragflow/internal/common"
	"ragflow/internal/engine"
	redis2 "ragflow/internal/engine/redis"
	"ragflow/internal/entity"
	pipelinepkg "ragflow/internal/ingestion/pipeline"
	taskpkg "ragflow/internal/ingestion/task"
	servicepkg "ragflow/internal/service"
	documentpkg "ragflow/internal/service/document"

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
	taskChan  chan *taskpkg.TaskContext
	workerWg  sync.WaitGroup
	startOnce sync.Once
	stopOnce  sync.Once // guards close(ShutdownCh) against double-close on repeated Stop

	ingestionTaskSvc *servicepkg.IngestionTaskService
	docState         *docStateUpdater

	// runDocumentTask dispatches to the migrated task handler path.
	// Tests may override this to verify branch routing without invoking
	// the full downstream stack.
	runDocumentTask func(ctx context.Context, ingestionTask *entity.IngestionTask) error

	// cancelCheck is polled periodically (every 3s) during task execution.
	// When it returns true the task's context is cancelled, which causes the
	// pipeline to stop at the next ctx.Err() check. Defaults to a Redis
	// cancel-flag lookup that mirrors Python's has_canceled(). Tests may
	// override this to simulate cancel without Redis.
	cancelCheck func(taskID string) bool
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
	msgQueueEngine := engine.GetMessageQueueEngine()
	err := msgQueueEngine.InitConsumer("tasks.RAGFLOW")
	if err != nil {
		return err
	}

	// Ensure worker pool is started on first task
	go e.startWorkerPool()

	for {
		// Graceful shutdown is the only condition under which the consume
		// loop exits. Per-message processing failures never terminate the
		// consumer: processMessage settles (ack/nack) each message itself.
		if err := e.ctx.Err(); err != nil {
			return nil
		}
		taskHandles, err := msgQueueEngine.GetMessages(4)
		if err != nil {
			common.Error("error consuming message", err)
			select {
			case <-time.After(consumeErrorBackoff):
			case <-e.ctx.Done():
				return nil
			}
			continue
		}
		for _, taskHandle := range taskHandles {
			e.processMessage(taskHandle)
		}
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

	if taskMessage.TaskType != common.TaskTypeIngestionTask {
		common.Info(fmt.Sprintf("task %s is not an ingestion task", taskMessage.TaskID))
		if err := handle.Ack(); err != nil {
			common.Error(fmt.Sprintf("error ack task %s", taskMessage.TaskID), err)
		}
		return
	}

	task, err := e.ingestionTaskSvc.StartRunning(taskMessage.TaskID)
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
	defer e.activeWorkers.Add(-1)
	e.activeWorkers.Add(1)
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
	if e.cancelCheck(task.ID) {
		common.Info(fmt.Sprintf("Task %s cancel flag detected before pipeline start, cancelling", task.ID))
		perTaskCancel()
	}

	e.settleMessage(taskCtx, func(ctx context.Context) bool {
		return e.runTask(ctx, task)
	})
}

// markStopped transitions the task to STOPPED (terminal). It first calls
// RequestStop to handle RUNNING → STOPPING, then MarkStopped for the final
// STOPPING → STOPPED transition. Finally it cleans up the Redis cancel flag
// so that a future retry of the same task does not immediately re-cancel.
func (e *Ingestor) markStopped(taskID string) bool {
	if _, err := e.ingestionTaskSvc.RequestStop(taskID); err != nil {
		common.Error(fmt.Sprintf("markStopped: RequestStop task %s: %v", taskID, err), err)
		return false
	}
	if err := e.ingestionTaskSvc.MarkStopped(taskID); err != nil {
		common.Error(fmt.Sprintf("markStopped: MarkStopped task %s: %v", taskID, err), err)
		return false
	}
	if rc := redis2.Get(); rc != nil {
		utility.BestEffort(fmt.Sprintf("clear cancel flag for %s", taskID), func() error {
			rc.Delete(fmt.Sprintf("%s-cancel", taskID))
			return nil // Delete returns bool; the bool does not distinguish "not found" from "error"
		})
	}
	return true
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

// runTask executes the task's business logic — run-count advance, document
// pipeline, and completion — behind the heartbeat. It returns whether the
// task reached a durably-persisted terminal status.
func (e *Ingestor) runTask(ctx context.Context, task *entity.IngestionTask) bool {
	select {
	case <-ctx.Done():
		common.Info(fmt.Sprintf("Task %s cancelled", task.ID))
		e.markCancelProgress(task)
		return e.markStopped(task.ID)
	default:
	}

	if err := e.ingestionTaskSvc.IncrementRunCount(task.ID); err != nil {
		common.Error(fmt.Sprintf("Failed to increment run count for task %s", task.ID), err)
		return e.markFailed(task.ID)
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
			rc.Delete(key)
			return nil // Delete returns bool; false may mean "key not found" or "error"
		})
	}

	if err := e.runDocumentTask(ctx, task); err != nil {
		if errors.Is(err, context.Canceled) {
			common.Info(fmt.Sprintf("Task %s cancelled during pipeline", task.ID))
			e.markCancelProgress(task)
			return e.markStopped(task.ID)
		}
		if errors.Is(err, context.DeadlineExceeded) {
			common.Info(fmt.Sprintf("Task %s timed out during pipeline", task.ID))
			e.markTimeoutProgress(task)
			return e.markFailed(task.ID)
		}
		common.Error(fmt.Sprintf("Task %s failed", task.ID), err)
		return e.markFailed(task.ID)
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
		return struct{}{}, e.completeOrSettle(taskID)
	}, backoff.WithMaxTries(3))
	return err
}

// completeOrSettle marks the task COMPLETED, or - if the transition is
// terminally invalid because the task is no longer RUNNING - settles it to its
// actual terminal state. Returns nil once the task is in any terminal state;
// returns a non-terminal (transient) error only for retry-worthy DB failures.
func (e *Ingestor) completeOrSettle(taskID string) error {
	if err := e.ingestionTaskSvc.MarkCompleted(taskID); err != nil {
		if isTerminalTransitionError(err) {
			return e.settleToTerminal(taskID)
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
func (e *Ingestor) settleToTerminal(taskID string) error {
	task, err := e.ingestionTaskSvc.GetTask(taskID)
	if err != nil {
		return err
	}
	switch task.Status {
	case common.STOPPING:
		if !e.markStopped(taskID) {
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
func (e *Ingestor) settleMessage(taskCtx *taskpkg.TaskContext, body func(context.Context) bool) (terminal bool) {
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
			e.markFailed(taskCtx.IngestionTask.ID)
			terminal = false
		}
		// Settlement authority is the DB, not the in-memory bool (BP1).
		// Fall back to the in-memory bool only when the DB is unavailable.
		if dbTerminal, ok := e.safeGetTerminal(taskCtx.IngestionTask.ID); ok {
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
func (e *Ingestor) safeGetTerminal(taskID string) (terminal bool, ok bool) {
	defer func() { recover() }()
	task, err := e.ingestionTaskSvc.GetTask(taskID)
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
func (e *Ingestor) defaultCancelCheck(taskID string) bool {
	rc := redis2.Get()
	if rc != nil {
		if ok, _ := rc.Exist(fmt.Sprintf("%s-cancel", taskID)); ok {
			return true
		}
	}
	task, err := e.ingestionTaskSvc.GetTask(taskID)
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
			result <- e.cancelCheck(taskID)
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
	doc, err := svc.GetDocumentByID(task.DocumentID)
	if err != nil {
		common.Error(fmt.Sprintf("markCancelProgress: load document %s: %v", task.DocumentID, err), err)
		return
	}
	cancelMsg := fmt.Sprintf("\n%s Task stopped by user.", time.Now().Format("15:04:05"))
	existingMsg := ""
	if doc.ProgressMsg != nil {
		existingMsg = *doc.ProgressMsg
	}
	_ = svc.UpdateRunProgress(task.DocumentID, -1.0, string(entity.TaskStatusCancel), existingMsg+cancelMsg)
}

// markTimeoutProgress writes the timeout-progress markers to the document
// row. Unlike cancellation (markCancelProgress), this records a TIMEOUT
// failure rather than a user-initiated stop.
func (e *Ingestor) markTimeoutProgress(task *entity.IngestionTask) {
	svc := documentpkg.NewDocumentService()
	doc, err := svc.GetDocumentByID(task.DocumentID)
	if err != nil {
		common.Error(fmt.Sprintf("markTimeoutProgress: load document %s: %v", task.DocumentID, err), err)
		return
	}
	timeoutMsg := fmt.Sprintf("\n%s Task timed out.", time.Now().Format("15:04:05"))
	existingMsg := ""
	if doc.ProgressMsg != nil {
		existingMsg = *doc.ProgressMsg
	}
	_ = svc.UpdateRunProgress(task.DocumentID, -1.0, string(entity.TaskStatusFail), existingMsg+timeoutMsg)
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
	docTaskCtx, err := taskpkg.LoadFromIngestionTask(ingestionTask)
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
	result, err := executor.WithRequireResume().WithProgressSink(newProgressSink(e.ingestionTaskSvc)).Execute(docTaskCtx.Ctx)
	if err != nil {
		return err
	}
	e.docState.apply(result)
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
