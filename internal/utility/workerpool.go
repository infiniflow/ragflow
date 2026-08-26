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

	// state is guarded by mu: the only transition is Running→Stopped (in
	// StopWait); SubmitTo and Resize read it to reject new work once the pool
	// is stopped. It is never accessed atomically — every read/write happens
	// under mu so the check and the channel send/close stay mutually exclusive.
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

	// activeSend counts submits that have passed the stopped-state check and
	// are about to (or currently) send on workChan. It is guarded by mu.
	// StopWait waits for it to reach zero before closing workChan, so a send
	// can never race the close, while SubmitTo is free to release mu before a
	// potentially blocking send (holding mu across a full-queue send would
	// deadlock: a worker blocked in markDone on the same mu can never receive
	// the next job to drain the queue).
	activeSend uint64

	senderDone sync.Cond
	workerWg   sync.WaitGroup
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
	p.senderDone.L = &p.mu
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
//
// Resize is a no-op on a stopped pool: it must never spawn workers after
// StopWait has marked the pool stopped, because workerWg.Add racing
// StopWait's workerWg.Wait is a WaitGroup misuse.
func (p *WorkerPool[T, R]) Resize(workers int) {
	if workers <= 0 {
		panic("workerpool: workers must be greater than zero")
	}

	// Guard the read-modify-write of desiredWorkers and the workerWg.Add in
	// start() with mu, the same lock StopWait holds while marking the pool
	// stopped. The lock serializes Resize against StopWait: once the pool is
	// stopped no Resize can add workers, so workerWg.Add never races Wait.
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.state == workerPoolStateStopped {
		return
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
//
// The stopped-state check and the activeSend increment are atomic with
// respect to StopWait's close(workChan): both run while holding mu, and
// StopWait marks the pool stopped and waits for activeSend to reach zero
// before closing the channel. A submit that has passed the check therefore
// either sends before the close, or (if its context is already done) returns
// without sending — never a "send on closed channel" panic. submitN is
// incremented before the send, so StopWait's drain counts a submit whose send
// is still blocked on a full queue and never returns while that task is in
// flight. mu is released before the send itself: a send that blocks on a full
// queue must not hold mu, or a worker finishing its current job would deadlock
// in markDone before it can receive the next job and drain the queue.
func (p *WorkerPool[T, R]) SubmitTo(ctx context.Context, input T, out chan<- WorkerPoolResult[T, R]) error {
	j := workerPoolJob[T, R]{ctx: ctx, input: input, out: out}

	p.mu.Lock()
	if p.state == workerPoolStateStopped {
		p.mu.Unlock()
		return ErrWorkerPoolStopped
	}
	p.submitN++
	p.activeSend++
	p.mu.Unlock()

	select {
	case <-ctx.Done():
		p.mu.Lock()
		p.submitN--
		p.activeSend--
		if p.activeSend == 0 {
			p.senderDone.Broadcast()
		}
		p.mu.Unlock()
		return ctx.Err()
	case p.workChan <- j:
		p.mu.Lock()
		p.activeSend--
		if p.activeSend == 0 {
			p.senderDone.Broadcast()
		}
		p.mu.Unlock()
		atomic.AddUint64(&p.submittedTotal, 1)
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
//
// Safe to call concurrently with Submit, SubmitTo and Resize. The pool is
// marked stopped and all counters are drained under mu. StopWait first waits
// for activeSend to reach zero — no submit that has already passed the
// stopped-state check is still sending — before closing workChan, so the close
// can never race a send. Because SubmitTo releases mu before a potentially
// blocking send, workers are free to drain the queue and unblock those senders
// instead of deadlocking on markDone. No Resize can spawn workers after the
// pool stops.
func (p *WorkerPool[T, R]) StopWait() {
	p.mu.Lock()
	if p.state == workerPoolStateStopped {
		p.mu.Unlock()
		return
	}
	p.state = workerPoolStateStopped
	for p.activeSend != 0 {
		p.senderDone.Wait()
	}
	for p.submitN != p.doneN {
		p.taskDone.Wait()
	}
	close(p.workChan)
	p.mu.Unlock()
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
