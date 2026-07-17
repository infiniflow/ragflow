package common

import (
	"sync"
)

// Pool bounds concurrent execution of fns with a weighted semaphore and
// collects the first error. It is the Go equivalent of Python's asyncio.Semaphore
// worker pool, and is dependency-free (no x/sync).
type Pool struct {
	sem chan struct{}
	wg  sync.WaitGroup
	mu  sync.Mutex
	err error
}

// NewPool constructs a Pool with at most n concurrent workers (min 1).
func NewPool(n int) *Pool {
	if n <= 0 {
		n = 1
	}
	return &Pool{sem: make(chan struct{}, n)}
}

// Go schedules fn to run; it blocks a worker slot and records the first error.
func (p *Pool) Go(fn func() error) {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		p.sem <- struct{}{}
		defer func() { <-p.sem }()
		if err := fn(); err != nil {
			p.mu.Lock()
			if p.err == nil {
				p.err = err
			}
			p.mu.Unlock()
		}
	}()
}

// Wait blocks until all scheduled fns finish and returns the first error, if any.
func (p *Pool) Wait() error {
	p.wg.Wait()
	return p.err
}
