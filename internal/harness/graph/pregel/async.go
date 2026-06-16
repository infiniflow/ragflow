// Package pregel provides async execution pipeline for Pregel.
package pregel

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"ragflow/internal/harness/graph/types"
)

// AsyncExecutor provides async execution of nodes with concurrency control.
type AsyncExecutor struct {
	maxConcurrency int
	workerPool     chan struct{}
	results        chan *asyncTaskResult
	mu             sync.Mutex
	activeTasks    map[string]*asyncTask
}

// asyncTask represents an asynchronous task.
type asyncTask struct {
	ID       string
	Name     string
	Func     func(context.Context) (interface{}, error)
	Context  context.Context
	Cancel   context.CancelFunc
	Priority int
}

// asyncTaskResult represents the result of an async task.
type asyncTaskResult struct {
	TaskID   string
	Name     string
	Output   interface{}
	Err      error
	Duration time.Duration
}

// NewAsyncExecutor creates a new async executor.
func NewAsyncExecutor(maxConcurrency int) *AsyncExecutor {
	if maxConcurrency <= 0 {
		maxConcurrency = 10 // Default concurrency
	}
	
	exec := &AsyncExecutor{
		maxConcurrency: maxConcurrency,
		workerPool:     make(chan struct{}, maxConcurrency),
		results:        make(chan *asyncTaskResult, 100),
		activeTasks:    make(map[string]*asyncTask),
	}
	
	// Pre-fill worker pool
	for i := 0; i < maxConcurrency; i++ {
		exec.workerPool <- struct{}{}
	}
	
	return exec
}

// Execute executes a single task asynchronously.
func (e *AsyncExecutor) Execute(ctx context.Context, name string, fn func(context.Context) (interface{}, error)) <-chan *asyncTaskResult {
	resultCh := make(chan *asyncTaskResult, 1)
	
	// Create cancellable context so Cancel() can stop running tasks.
	taskCtx, cancel := context.WithCancel(ctx)
	task := &asyncTask{
		ID:      uuid.New().String(),
		Name:    name,
		Func:    fn,
		Context: taskCtx,
		Cancel:  cancel,
	}
	
	e.mu.Lock()
	e.activeTasks[task.ID] = task
	e.mu.Unlock()
	
	go func() {
		defer close(resultCh)
		
		startTime := time.Now()
		
		// Acquire worker slot
		select {
		case <-e.workerPool:
			defer func() { e.workerPool <- struct{}{} }()
		case <-ctx.Done():
			resultCh <- &asyncTaskResult{
				TaskID: task.ID,
				Name:   task.Name,
				Err:    ctx.Err(),
			}
			return
		}
		
		// Execute task
		output, err := task.Func(task.Context)
		
		result := &asyncTaskResult{
			TaskID:   task.ID,
			Name:     task.Name,
			Output:   output,
			Err:      err,
			Duration: time.Since(startTime),
		}
		
		e.mu.Lock()
		delete(e.activeTasks, task.ID)
		e.mu.Unlock()
		
		resultCh <- result
	}()
	
	return resultCh
}

// ExecuteBatch executes multiple tasks concurrently with controlled concurrency.
func (e *AsyncExecutor) ExecuteBatch(ctx context.Context, tasks []asyncTask) <-chan *asyncTaskResult {
	resultCh := make(chan *asyncTaskResult, len(tasks))
	
	for i := range tasks {
		tasks[i].ID = uuid.New().String()
		e.mu.Lock()
		e.activeTasks[tasks[i].ID] = &tasks[i]
		e.mu.Unlock()
	}
	
	var wg sync.WaitGroup
	for i := range tasks {
		wg.Add(1)
		go func(task *asyncTask) {
			defer wg.Done()
			
			startTime := time.Now()
			
			// Acquire worker slot
			select {
			case <-e.workerPool:
				defer func() { e.workerPool <- struct{}{} }()
			case <-ctx.Done():
				resultCh <- &asyncTaskResult{
					TaskID: task.ID,
					Name:   task.Name,
					Err:    ctx.Err(),
				}
				return
			}
			
			// Execute task
			output, err := task.Func(task.Context)
			
			resultCh <- &asyncTaskResult{
				TaskID:   task.ID,
				Name:     task.Name,
				Output:   output,
				Err:      err,
				Duration: time.Since(startTime),
			}
			
			e.mu.Lock()
			delete(e.activeTasks, task.ID)
			e.mu.Unlock()
		}(&tasks[i])
	}
	
	go func() {
		wg.Wait()
		close(resultCh)
	}()
	
	return resultCh
}

// ExecuteWithRetry executes a task with retry logic.
func (e *AsyncExecutor) ExecuteWithRetry(ctx context.Context, name string, fn func(context.Context) (interface{}, error), retryConfig *RetryConfig) <-chan *asyncTaskResult {
	resultCh := make(chan *asyncTaskResult, 1)
	
	taskCtx, cancel := context.WithCancel(ctx)
	task := &asyncTask{
		ID:      uuid.New().String(),
		Name:    name,
		Context: taskCtx,
		Cancel:  cancel,
	}
	
	e.mu.Lock()
	e.activeTasks[task.ID] = task
	e.mu.Unlock()
	
	go func() {
		defer close(resultCh)
		
		executor := NewRetryExecutor(retryConfig.Policy)
		
		startTime := time.Now()
		output, err := executor.Execute(ctx, name, fn)
		
		result := &asyncTaskResult{
			TaskID:   task.ID,
			Name:     name,
			Output:   output,
			Err:      err,
			Duration: time.Since(startTime),
		}
		
		e.mu.Lock()
		delete(e.activeTasks, task.ID)
		e.mu.Unlock()
		
		resultCh <- result
	}()
	
	return resultCh
}

// Cancel cancels all active tasks by invoking their cancel functions.
func (e *AsyncExecutor) Cancel() {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	for id, task := range e.activeTasks {
		if task.Cancel != nil {
			task.Cancel()
		}
		delete(e.activeTasks, id)
	}
}

// GetActiveTaskCount returns the number of currently active tasks.
func (e *AsyncExecutor) GetActiveTaskCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.activeTasks)
}

// Wait waits for all active tasks to complete.
func (e *AsyncExecutor) Wait(ctx context.Context) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if e.GetActiveTaskCount() == 0 {
				return nil
			}
		}
	}
}

// AsyncPipeline provides an async execution pipeline for the Pregel loop.
type AsyncPipeline struct {
	executor *AsyncExecutor
	retryer  *RetryExecutor
	
	// Stream channels
	events   chan interface{}
	errors   chan error
	
	// Control
	mu       sync.RWMutex
	cancel   context.CancelFunc
	running  bool
}

// NewAsyncPipeline creates a new async pipeline.
func NewAsyncPipeline(maxConcurrency int, retryPolicy *types.RetryPolicy) *AsyncPipeline {
	return &AsyncPipeline{
		executor: NewAsyncExecutor(maxConcurrency),
		retryer:  NewRetryExecutor(retryPolicy),
		events:   make(chan interface{}, 100),
		errors:   make(chan error, 10),
		running:  false,
	}
}

// Start starts the async pipeline.
func (p *AsyncPipeline) Start(ctx context.Context) context.Context {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if p.running {
		return ctx
	}
	
	ctx, p.cancel = context.WithCancel(ctx)
	p.running = true
	
	return ctx
}

// Stop stops the async pipeline.
func (p *AsyncPipeline) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	
	if !p.running {
		return
	}
	
	if p.cancel != nil {
		p.cancel()
	}
	
	p.executor.Cancel()
	close(p.events)
	close(p.errors)
	p.running = false
}

// ExecuteNode executes a node in the pipeline.
func (p *AsyncPipeline) ExecuteNode(ctx context.Context, name string, fn func(context.Context) (interface{}, error), retryConfig *RetryConfig) <-chan *asyncTaskResult {
	if retryConfig != nil {
		return p.executor.ExecuteWithRetry(ctx, name, fn, retryConfig)
	}
	return p.executor.Execute(ctx, name, fn)
}

// Events returns the event channel.
func (p *AsyncPipeline) Events() <-chan interface{} {
	return p.events
}

// Errors returns the error channel.
func (p *AsyncPipeline) Errors() <-chan error {
	return p.errors
}

// EmitEvent emits an event to the pipeline.
func (p *AsyncPipeline) EmitEvent(event interface{}) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if p.running {
		select {
		case p.events <- event:
		default:
			// Channel full, drop event
		}
	}
}

// EmitError emits an error to the pipeline.
func (p *AsyncPipeline) EmitError(err error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	
	if p.running {
		select {
		case p.errors <- err:
		default:
			// Channel full, drop error
		}
	}
}

// IsRunning returns whether the pipeline is running.
func (p *AsyncPipeline) IsRunning() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.running
}

// ConcurrencyLimiter limits concurrency for specific nodes.
type ConcurrencyLimiter struct {
	limits map[string]chan struct{}
	mu     sync.RWMutex
}

// NewConcurrencyLimiter creates a new concurrency limiter.
func NewConcurrencyLimiter() *ConcurrencyLimiter {
	return &ConcurrencyLimiter{
		limits: make(map[string]chan struct{}),
	}
}

// SetLimit sets the concurrency limit for a node.
func (l *ConcurrencyLimiter) SetLimit(node string, limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	
	ch := make(chan struct{}, limit)
	for i := 0; i < limit; i++ {
		ch <- struct{}{}
	}
	l.limits[node] = ch
}

// Acquire acquires a slot for a node.
func (l *ConcurrencyLimiter) Acquire(ctx context.Context, node string) error {
	l.mu.RLock()
	ch, ok := l.limits[node]
	l.mu.RUnlock()
	
	if !ok {
		// No limit set
		return nil
	}
	
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Release releases a slot for a node.
func (l *ConcurrencyLimiter) Release(node string) {
	l.mu.RLock()
	ch, ok := l.limits[node]
	l.mu.RUnlock()
	
	if !ok {
		return
	}
	
	select {
	case ch <- struct{}{}:
	default:
		// Channel full, shouldn't happen
	}
}

// PriorityTask represents a prioritized task.
type PriorityTask struct {
	Func     func(context.Context) (interface{}, error)
	Priority int
}

// PriorityExecutor executes tasks with priority scheduling.
type PriorityExecutor struct {
	tasks chan PriorityTask
	mu    sync.Mutex
}

// NewPriorityExecutor creates a new priority executor.
func NewPriorityExecutor(bufferSize int) *PriorityExecutor {
	return &PriorityExecutor{
		tasks: make(chan PriorityTask, bufferSize),
	}
}

// Submit submits a task with priority.
func (e *PriorityExecutor) Submit(task PriorityTask) error {
	select {
	case e.tasks <- task:
		return nil
	default:
		return fmt.Errorf("task queue full")
	}
}

// Execute executes tasks in priority order.
func (e *PriorityExecutor) Execute(ctx context.Context, maxConcurrency int) <-chan interface{} {
	resultCh := make(chan interface{}, maxConcurrency)
	
	// Simple priority scheduling using multiple channels
	// In a real implementation, you'd use a priority queue
	go func() {
		defer close(resultCh)
		
		// This is a simplified implementation
		// A full implementation would use a heap-based priority queue
		for {
			select {
			case <-ctx.Done():
				return
			case task := <-e.tasks:
				output, err := task.Func(ctx)
				resultCh <- map[string]interface{}{
					"output":   output,
					"error":    err,
					"priority": task.Priority,
				}
			}
		}
	}()
	
	return resultCh
}
