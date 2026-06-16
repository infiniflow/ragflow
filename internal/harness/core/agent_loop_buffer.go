package core

import "sync"

// turnBuffer is a thread-safe blocking buffer used internally by AgentLoop.
type turnBuffer[T any] struct {
	buf      []T
	mu       sync.Mutex
	notEmpty *sync.Cond
	closed   bool
	woken    bool
}

func newTurnBuffer[T any]() *turnBuffer[T] {
	tb := &turnBuffer[T]{}
	tb.notEmpty = sync.NewCond(&tb.mu)
	return tb
}

// TrySend enqueues a value. Returns false if the buffer is closed — no panic.
func (tb *turnBuffer[T]) TrySend(value T) bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.closed {
		return false
	}

	tb.buf = append(tb.buf, value)
	tb.notEmpty.Signal()
	return true
}

func (tb *turnBuffer[T]) Receive() (T, bool) {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	for len(tb.buf) == 0 && !tb.closed && !tb.woken {
		tb.notEmpty.Wait()
	}

	tb.woken = false

	if len(tb.buf) == 0 {
		var zero T
		return zero, false
	}

	val := tb.buf[0]
	tb.buf = tb.buf[1:]
	return val, true
}

func (tb *turnBuffer[T]) Close() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if !tb.closed {
		tb.closed = true
		tb.notEmpty.Broadcast()
	}
}

func (tb *turnBuffer[T]) IsClosed() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.closed
}

func (tb *turnBuffer[T]) TakeAll() []T {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if len(tb.buf) == 0 {
		return nil
	}

	values := tb.buf
	tb.buf = nil
	return values
}

func (tb *turnBuffer[T]) PushFront(values []T) {
	if len(values) == 0 {
		return
	}

	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.buf = append(append([]T{}, values...), tb.buf...)
	tb.notEmpty.Signal()
}

func (tb *turnBuffer[T]) Wakeup() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.woken = true
	tb.notEmpty.Broadcast()
}

func (tb *turnBuffer[T]) ClearWakeup() {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	tb.woken = false
}
