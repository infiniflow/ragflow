package utility

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
)

const (
	workerPoolStateRunning uint32 = 0
	workerPoolStateStopped uint32 = 1
)

var (
	// ErrWorkerPoolStopped is returned when a task is submitted to a stopped pool.
	ErrWorkerPoolStopped = errors.New("workerpool: already stopped")
)

// WorkerPoolHandler processes one task input and returns its typed result.
type WorkerPoolHandler[T any, R any] func(context.Context, T) (R, error)

// WorkerPoolResult carries the input, output, and execution error for one task.
type WorkerPoolResult[T any, R any] struct {
	Input T
	Value R
	Err   error
}

// WorkerPoolFuture wraps the asynchronous result for one submitted task.
type WorkerPoolFuture[T any, R any] struct {
	ch <-chan WorkerPoolResult[T, R]
}

// Wait blocks until the task completes or ctx is cancelled.
func (f WorkerPoolFuture[T, R]) Wait(ctx context.Context) (WorkerPoolResult[T, R], error) {
	select {
	case <-ctx.Done():
		var zero WorkerPoolResult[T, R]
		return zero, ctx.Err()
	case res := <-f.ch:
		return res, nil
	}
}

// WorkerPoolStats exposes a snapshot of pool activity.
type WorkerPoolStats struct {
	DesiredWorkers int
	LiveWorkers    int
	ActiveWorkers  int
	QueueDepth     int
	SubmittedTotal uint64
	CompletedTotal uint64
	FailedTotal    uint64
	PendingTotal   uint64
}

type workerPoolJob[T any, R any] struct {
	ctx   context.Context
	input T
	out   chan<- WorkerPoolResult[T, R]
}

// WorkerPool is a reusable, process-local worker pool for homogeneous tasks.
// Workers are long-lived and can be resized at runtime.
type WorkerPool[T any, R any] struct {
	handler  WorkerPoolHandler[T, R]
	workChan chan workerPoolJob[T, R]

	state uint32

	desiredWorkers int64
	liveWorkers    int64
	activeWorkers  int64
	submittedTotal uint64
	completedTotal uint64
	failedTotal    uint64

	taskDone sync.Cond
	mu       sync.Mutex
	submitN  uint64
	doneN    uint64

	workerWg sync.WaitGroup
}

// NewWorkerPool creates a worker pool with fixed queue capacity and starts workers immediately.
func NewWorkerPool[T any, R any](workers, queueSize int, handler WorkerPoolHandler[T, R]) *WorkerPool[T, R] {
	if workers <= 0 {
		panic("workerpool: workers must be greater than zero")
	}
	if queueSize <= 0 {
		panic("workerpool: queueSize must be greater than zero")
	}
	if handler == nil {
		panic("workerpool: handler must not be nil")
	}

	p := &WorkerPool[T, R]{
		handler:        handler,
		workChan:       make(chan workerPoolJob[T, R], queueSize),
		desiredWorkers: int64(workers),
	}
	p.taskDone.L = &p.mu
	p.start(workers)
	return p
}

func (p *WorkerPool[T, R]) start(workers int) {
	for range workers {
		p.workerWg.Add(1)
		go p.worker()
	}
}

func (p *WorkerPool[T, R]) worker() {
	atomic.AddInt64(&p.liveWorkers, 1)
	defer func() {
		atomic.AddInt64(&p.liveWorkers, -1)
		p.workerWg.Done()
	}()

	for j := range p.workChan {
		res := WorkerPoolResult[T, R]{Input: j.input}
		if err := j.ctx.Err(); err != nil {
			res.Err = err
		} else {
			atomic.AddInt64(&p.activeWorkers, 1)
			func() {
				defer func() {
					if r := recover(); r != nil {
						res.Err = fmt.Errorf("workerpool: handler panic: %v", r)
					}
					atomic.AddInt64(&p.activeWorkers, -1)
				}()
				value, err := p.handler(j.ctx, j.input)
				res.Value = value
				res.Err = err
			}()
		}

		if res.Err != nil {
			atomic.AddUint64(&p.failedTotal, 1)
		}
		atomic.AddUint64(&p.completedTotal, 1)
		if j.out != nil {
			j.out <- res
		}
		p.markDone()

		if atomic.LoadInt64(&p.liveWorkers) > atomic.LoadInt64(&p.desiredWorkers) {
			return
		}
	}
}

// Resize adjusts the target worker count. When shrinking, extra workers retire
// after completing their current task.
func (p *WorkerPool[T, R]) Resize(workers int) {
	if workers <= 0 {
		panic("workerpool: workers must be greater than zero")
	}

	current := int(atomic.LoadInt64(&p.desiredWorkers))
	atomic.StoreInt64(&p.desiredWorkers, int64(workers))
	if workers > current {
		p.start(workers - current)
	}
}

// Submit enqueues one task and returns a future for its result.
func (p *WorkerPool[T, R]) Submit(ctx context.Context, input T) (WorkerPoolFuture[T, R], error) {
	resultCh := make(chan WorkerPoolResult[T, R], 1)
	if err := p.SubmitTo(ctx, input, resultCh); err != nil {
		return WorkerPoolFuture[T, R]{}, err
	}
	return WorkerPoolFuture[T, R]{ch: resultCh}, nil
}

// SubmitTo enqueues one task and routes its result into out.
func (p *WorkerPool[T, R]) SubmitTo(ctx context.Context, input T, out chan<- WorkerPoolResult[T, R]) error {
	if atomic.LoadUint32(&p.state) == workerPoolStateStopped {
		return ErrWorkerPoolStopped
	}

	j := workerPoolJob[T, R]{ctx: ctx, input: input, out: out}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case p.workChan <- j:
		atomic.AddUint64(&p.submittedTotal, 1)
		p.mu.Lock()
		p.submitN++
		p.mu.Unlock()
		return nil
	}
}

func (p *WorkerPool[T, R]) markDone() {
	p.mu.Lock()
	p.doneN++
	if p.submitN == p.doneN {
		p.taskDone.Broadcast()
	}
	p.mu.Unlock()
}

// StopWait stops accepting new tasks, waits for queued/running tasks to finish,
// then shuts down the worker pool.
func (p *WorkerPool[T, R]) StopWait() {
	if !atomic.CompareAndSwapUint32(&p.state, workerPoolStateRunning, workerPoolStateStopped) {
		return
	}

	p.mu.Lock()
	for p.submitN != p.doneN {
		p.taskDone.Wait()
	}
	p.mu.Unlock()

	close(p.workChan)
	p.workerWg.Wait()
}

// Stats returns a point-in-time view of pool usage counters.
func (p *WorkerPool[T, R]) Stats() WorkerPoolStats {
	submitted := atomic.LoadUint64(&p.submittedTotal)
	completed := atomic.LoadUint64(&p.completedTotal)

	return WorkerPoolStats{
		DesiredWorkers: int(atomic.LoadInt64(&p.desiredWorkers)),
		LiveWorkers:    int(atomic.LoadInt64(&p.liveWorkers)),
		ActiveWorkers:  int(atomic.LoadInt64(&p.activeWorkers)),
		QueueDepth:     len(p.workChan),
		SubmittedTotal: submitted,
		CompletedTotal: completed,
		FailedTotal:    atomic.LoadUint64(&p.failedTotal),
		PendingTotal:   submitted - completed,
	}
}
