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
	"fmt"
	"ragflow/internal/service"
	syncerconnector "ragflow/internal/syncer/connector"
	"sync"
	"time"
)

var errSyncJobExecutorClosed = errors.New("sync job executor is closed")

// SyncJobExecutorConfig controls the shared batch job executor.
type SyncJobExecutorConfig struct {
	WorkerCount      int // num of the workers
	JobQueueSize     int // jobs channel size
	PerTaskQueueSize int // sync_task's queue size
}

// normalize prevent the channel from malfunctioning when set to 0
func (c SyncJobExecutorConfig) normalize() SyncJobExecutorConfig {
	if c.WorkerCount <= 0 {
		c.WorkerCount = 1
	}
	if c.JobQueueSize <= 0 {
		c.JobQueueSize = c.WorkerCount
	}
	if c.PerTaskQueueSize <= 0 {
		c.PerTaskQueueSize = c.JobQueueSize
	}
	return c
}

// syncJobFunc the func that the batch job to execute
type syncJobFunc func(context.Context) (service.SyncStats, error)

// syncJobResult the stats
type syncJobResult struct {
	stats      service.SyncStats
	checkpoint *syncerconnector.SyncCheckpoint
	err        error
}

type syncJob struct {
	ctx        context.Context
	fn         syncJobFunc
	checkpoint *syncerconnector.SyncCheckpoint
	done       chan syncJobResult
}

// SyncJobQueue is one Coordinator-owned queue feeding the fair dispatcher.
type SyncJobQueue struct {
	taskID string
	jobs   chan *syncJob
	close  func(string)
	once   sync.Once
}

// Submit adds one BatchJob to this task's dispatcher queue.
func (q *SyncJobQueue) Submit(ctx context.Context, fn syncJobFunc) (<-chan syncJobResult, error) {
	return q.submit(ctx, fn, nil)
}

func (q *SyncJobQueue) submit(ctx context.Context, fn syncJobFunc, checkpoint *syncerconnector.SyncCheckpoint) (<-chan syncJobResult, error) {
	if q == nil {
		return nil, fmt.Errorf("sync job queue is nil")
	}

	done := make(chan syncJobResult, 1) // done channel
	job := &syncJob{ctx: ctx, fn: fn, checkpoint: checkpoint, done: done}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case q.jobs <- job:
		return done, nil
	}
}

// Close unregisters this task from the fair dispatcher.
func (q *SyncJobQueue) Close() {
	if q == nil {
		return
	}
	q.once.Do(func() {
		if q.close != nil {
			q.close(q.taskID)
		}
	})
}

type executorCommandKind int

const (
	executorRegister executorCommandKind = iota
	executorUnregister
)

type executorCommand struct {
	kind  executorCommandKind
	queue *SyncJobQueue
	task  string
	err   chan error
	done  chan struct{}
}

type syncTaskState struct {
	jobs <-chan *syncJob
}

// SyncJobExecutor fairly dispatches per-task BatchJobs into one shared worker channel.
type SyncJobExecutor struct {
	perTaskQueueSize int
	jobs             chan *syncJob
	commands         chan executorCommand
	stop             chan struct{}
	done             chan struct{}
	stopOnce         sync.Once
	workerGroup      sync.WaitGroup
}

// NewSyncJobExecutor creates a global BatchJob executor.
func NewSyncJobExecutor(config SyncJobExecutorConfig) *SyncJobExecutor {
	config = config.normalize()
	executor := &SyncJobExecutor{
		perTaskQueueSize: config.PerTaskQueueSize,
		jobs:             make(chan *syncJob, config.JobQueueSize),
		commands:         make(chan executorCommand),
		stop:             make(chan struct{}),
		done:             make(chan struct{}),
	}

	go executor.dispatch()
	for i := 0; i < config.WorkerCount; i++ {
		executor.workerGroup.Add(1)
		go executor.work()
	}
	return executor
}

// RegisterTask creates the bounded Coordinator queue for one running task.
func (e *SyncJobExecutor) RegisterTask(ctx context.Context, taskID string) (*SyncJobQueue, error) {
	if e == nil {
		return nil, fmt.Errorf("sync job executor is nil")
	}
	queue := &SyncJobQueue{taskID: taskID, jobs: make(chan *syncJob, e.perTaskQueueSize), close: e.unregisterTask}
	reply := make(chan error, 1)
	command := executorCommand{kind: executorRegister, queue: queue, err: reply}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-e.stop:
		return nil, errSyncJobExecutorClosed
	case e.commands <- command:
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-reply:
		if err != nil {
			return nil, err
		}
		return queue, nil
	}
}

// Close stops the dispatcher and waits for fixed workers to exit.
func (e *SyncJobExecutor) Close() {
	if e == nil {
		return
	}
	e.stopOnce.Do(func() {
		close(e.stop)
		<-e.done
		e.workerGroup.Wait()
	})
}

func (e *SyncJobExecutor) unregisterTask(taskID string) {
	done := make(chan struct{})
	command := executorCommand{kind: executorUnregister, task: taskID, done: done}
	select {
	case <-e.stop:
		return
	case e.commands <- command:
		<-done
	}
}

func (e *SyncJobExecutor) dispatch() {
	defer close(e.done)
	tasks := map[string]*syncTaskState{}
	order := []string{}
	cursor := 0
	var pending *syncJob

	for {
		if pending == nil {
			pending = popReadyJob(tasks, order, &cursor)
		}
		if pending == nil {
			select {
			case <-e.stop:
				settleQueuedJobs(nil, tasks)
				close(e.jobs)
				return
			case command := <-e.commands:
				order = applyExecutorCommand(command, tasks, order, &cursor)
			case <-time.After(time.Millisecond):
			}
			continue
		}

		select {
		case <-e.stop:
			settleQueuedJobs(pending, tasks)
			close(e.jobs)
			return
		case command := <-e.commands:
			order = applyExecutorCommand(command, tasks, order, &cursor)
		case e.jobs <- pending:
			pending = nil
		}
	}
}

func settleQueuedJobs(pending *syncJob, tasks map[string]*syncTaskState) {
	if pending != nil {
		pending.done <- syncJobResult{err: errSyncJobExecutorClosed}
	}
	for _, task := range tasks {
		if task == nil {
			continue
		}
		settleTaskJobs(task)
	}
}

func settleTaskJobs(task *syncTaskState) {
	for {
		select {
		case job := <-task.jobs:
			job.done <- syncJobResult{err: errSyncJobExecutorClosed}
		default:
			return
		}
	}
}

func applyExecutorCommand(command executorCommand, tasks map[string]*syncTaskState, order []string, cursor *int) []string {
	switch command.kind {
	case executorRegister:
		err := registerExecutorTask(command.queue, tasks, &order)
		command.err <- err
	case executorUnregister:
		order = unregisterExecutorTask(command.task, tasks, order, cursor)
		close(command.done)
	}
	return order
}

func registerExecutorTask(queue *SyncJobQueue, tasks map[string]*syncTaskState, order *[]string) error {
	if queue == nil || queue.taskID == "" {
		return fmt.Errorf("sync job task id is required")
	}
	if tasks[queue.taskID] != nil {
		return fmt.Errorf("sync job task %q is already registered", queue.taskID)
	}
	tasks[queue.taskID] = &syncTaskState{jobs: queue.jobs}
	*order = append(*order, queue.taskID)
	return nil
}

func unregisterExecutorTask(taskID string, tasks map[string]*syncTaskState, order []string, cursor *int) []string {
	if tasks[taskID] == nil {
		return order
	}
	delete(tasks, taskID)
	for i, existing := range order {
		if existing != taskID {
			continue
		}
		order = append(order[:i], order[i+1:]...)
		if len(order) == 0 {
			*cursor = 0
		} else if *cursor >= len(order) {
			*cursor = 0
		}
		return order
	}
	return order
}

// popReadyJob pop the job fairly
func popReadyJob(tasks map[string]*syncTaskState, order []string, cursor *int) *syncJob {
	if len(order) == 0 {
		return nil
	}

	for i := 0; i < len(order); i++ {
		index := (*cursor + i) % len(order)
		task := tasks[order[index]]
		if task == nil {
			continue
		}
		select {
		case job := <-task.jobs:
			*cursor = (index + 1) % len(order)
			return job
		default:
		}
	}
	return nil
}

// work execute the job
func (e *SyncJobExecutor) work() {
	defer e.workerGroup.Done()

	for job := range e.jobs { //
		if err := job.ctx.Err(); err != nil {
			job.done <- syncJobResult{err: err}
			continue
		}
		stats, err := job.fn(job.ctx) // run the job
		job.done <- syncJobResult{stats: stats, checkpoint: job.checkpoint, err: err}
	}
}
