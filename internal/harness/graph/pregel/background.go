// Package pregel provides background execution support for Pregel.
package pregel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// BackgroundTask represents a background task.
type BackgroundTask struct {
	ID       string
	Name     string
	Func     func(context.Context) (interface{}, error)
	Context  context.Context
	Result   chan *BackgroundTaskResult
	Cancel   context.CancelFunc
	Priority int
	Created  time.Time
}

// BackgroundTaskResult represents the result of a background task.
type BackgroundTaskResult struct {
	TaskID   string
	Name     string
	Output   interface{}
	Err      error
	Duration time.Duration
}

// BackgroundExecutor executes tasks in a background worker pool.
type BackgroundExecutor struct {
	maxWorkers   int
	taskQueue    chan *BackgroundTask
	results      chan *BackgroundTaskResult
	workers      []*backgroundWorker
	mu           sync.RWMutex
	running      bool
	stopCh       chan struct{}
	wg           sync.WaitGroup
	activeTasks  map[string]*BackgroundTask
	shutdownTimeout time.Duration
}

// backgroundWorker represents a worker goroutine.
type backgroundWorker struct {
	id      int
	executor *BackgroundExecutor
	stopCh  chan struct{}
}

// NewBackgroundExecutor creates a new background executor.
func NewBackgroundExecutor(maxWorkers int, queueSize int) *BackgroundExecutor {
	if maxWorkers <= 0 {
		maxWorkers = 10
	}
	if queueSize <= 0 {
		queueSize = 100
	}

	return &BackgroundExecutor{
		maxWorkers:      maxWorkers,
		taskQueue:       make(chan *BackgroundTask, queueSize),
		results:         make(chan *BackgroundTaskResult, queueSize),
		workers:         make([]*backgroundWorker, 0, maxWorkers),
		stopCh:          make(chan struct{}),
		activeTasks:     make(map[string]*BackgroundTask),
		shutdownTimeout: 30 * time.Second,
	}
}

// Start starts the background executor.
func (e *BackgroundExecutor) Start(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.running {
		return
	}

	e.running = true
	e.stopCh = make(chan struct{})

	// Start workers
	for i := 0; i < e.maxWorkers; i++ {
		worker := &backgroundWorker{
			id:      i,
			executor: e,
			stopCh:  make(chan struct{}),
		}
		e.workers = append(e.workers, worker)
		e.wg.Add(1)
		go worker.run()
	}
}

// Stop stops the background executor gracefully.
func (e *BackgroundExecutor) Stop() {
	e.mu.Lock()
	if !e.running {
		e.mu.Unlock()
		return
	}
	e.running = false
	close(e.stopCh)
	e.mu.Unlock()

	// Wait for workers to finish with timeout
	done := make(chan struct{})
	go func() {
		e.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// All workers stopped
	case <-time.After(e.shutdownTimeout):
		// Timeout - force stop
	}

	close(e.taskQueue)
	close(e.results)
}

// Submit submits a task for background execution.
func (e *BackgroundExecutor) Submit(ctx context.Context, name string, fn func(context.Context) (interface{}, error), priority int) (*BackgroundTask, error) {
	e.mu.RLock()
	if !e.running {
		e.mu.RUnlock()
		return nil, fmt.Errorf("executor not running")
	}
	e.mu.RUnlock()

	taskCtx, cancel := context.WithCancel(ctx)
	task := &BackgroundTask{
		ID:       uuid.New().String(),
		Name:     name,
		Func:     fn,
		Context:  taskCtx,
		Result:   make(chan *BackgroundTaskResult, 1),
		Cancel:   cancel,
		Priority: priority,
		Created:  time.Now(),
	}

	e.mu.Lock()
	e.activeTasks[task.ID] = task
	e.mu.Unlock()

	select {
	case e.taskQueue <- task:
		return task, nil
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	case <-time.After(5 * time.Second):
		cancel()
		return nil, fmt.Errorf("task queue full")
	}
}

// GetResult gets the result channel for receiving task results.
func (e *BackgroundExecutor) GetResult() <-chan *BackgroundTaskResult {
	return e.results
}

// CancelTask cancels a specific task.
func (e *BackgroundExecutor) CancelTask(taskID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if task, ok := e.activeTasks[taskID]; ok {
		task.Cancel()
		return true
	}
	return false
}

// GetActiveTasks returns the list of active task IDs.
func (e *BackgroundExecutor) GetActiveTasks() []string {
	e.mu.RLock()
	defer e.mu.RUnlock()

	tasks := make([]string, 0, len(e.activeTasks))
	for id := range e.activeTasks {
		tasks = append(tasks, id)
	}
	return tasks
}

// run is the worker goroutine.
func (w *backgroundWorker) run() {
	defer w.executor.wg.Done()

	for {
		select {
		case <-w.executor.stopCh:
			return
		case task, ok := <-w.executor.taskQueue:
			if !ok {
				return
			}
			w.executeTask(task)
		}
	}
}

// executeTask executes a single task.
func (w *backgroundWorker) executeTask(task *BackgroundTask) {
	startTime := time.Now()

	// Execute the task function
	output, err := task.Func(task.Context)

	result := &BackgroundTaskResult{
		TaskID:   task.ID,
		Name:     task.Name,
		Output:   output,
		Err:      err,
		Duration: time.Since(startTime),
	}

	// Remove from active tasks
	w.executor.mu.Lock()
	delete(w.executor.activeTasks, task.ID)
	w.executor.mu.Unlock()

	// Send result
	select {
	case task.Result <- result:
	case <-time.After(5 * time.Second):
		// Task result channel blocked
	}

	// Also send to global results channel
	select {
	case w.executor.results <- result:
	case <-time.After(5 * time.Second):
		// Results channel blocked
	}
}

// TaskPriority constants for common priorities.
const (
	PriorityLow      = 0
	PriorityNormal   = 5
	PriorityHigh     = 10
	PriorityCritical = 20
)

// PriorityQueue implements a priority queue for tasks.
type PriorityQueue struct {
	tasks []*BackgroundTask
	mu    sync.RWMutex
}

// NewPriorityQueue creates a new priority queue.
func NewPriorityQueue() *PriorityQueue {
	return &PriorityQueue{
		tasks: make([]*BackgroundTask, 0),
	}
}

// Push adds a task to the queue.
func (pq *PriorityQueue) Push(task *BackgroundTask) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	pq.tasks = append(pq.tasks, task)
	pq.heapifyUp(len(pq.tasks) - 1)
}

// Pop removes and returns the highest priority task.
func (pq *PriorityQueue) Pop() (*BackgroundTask, bool) {
	pq.mu.Lock()
	defer pq.mu.Unlock()

	if len(pq.tasks) == 0 {
		return nil, false
	}

	task := pq.tasks[0]
	pq.tasks[0] = pq.tasks[len(pq.tasks)-1]
	pq.tasks = pq.tasks[:len(pq.tasks)-1]

	if len(pq.tasks) > 0 {
		pq.heapifyDown(0)
	}

	return task, true
}

// Len returns the number of tasks in the queue.
func (pq *PriorityQueue) Len() int {
	pq.mu.RLock()
	defer pq.mu.RUnlock()
	return len(pq.tasks)
}

// heapifyUp maintains heap property when adding.
func (pq *PriorityQueue) heapifyUp(index int) {
	for index > 0 {
		parent := (index - 1) / 2
		if pq.tasks[parent].Priority >= pq.tasks[index].Priority {
			break
		}
		pq.tasks[parent], pq.tasks[index] = pq.tasks[index], pq.tasks[parent]
		index = parent
	}
}

// heapifyDown maintains heap property when removing.
func (pq *PriorityQueue) heapifyDown(index int) {
	n := len(pq.tasks)
	for {
		largest := index
		left := 2*index + 1
		right := 2*index + 2

		if left < n && pq.tasks[left].Priority > pq.tasks[largest].Priority {
			largest = left
		}
		if right < n && pq.tasks[right].Priority > pq.tasks[largest].Priority {
			largest = right
		}
		if largest == index {
			break
		}
		pq.tasks[index], pq.tasks[largest] = pq.tasks[largest], pq.tasks[index]
		index = largest
	}
}
