package doctype

import (
	"context"
	"sync"
)

// inference_gate.go — the process-wide DeepDoc inference budget.
//
// This gate lives in the dependency-free doctype package so that EVERY
// inference backend acquires the same budget: the in-process ONNX session pool
// and remote inference clients alike. Keeping it here (rather than inside the
// cgo-only native package) is what makes the budget cover client-side HTTP
// inference, which otherwise bypassed it entirely.
//
// Each unit of work is one inference call (a single ONNX Run in-process, or one
// /predict/* request for a remote client). A nil limiter admits everything, so
// a process that never registers a budget (a one-shot CLI or a test) needs no
// separate code path.

type inferenceLimiter struct {
	slots chan struct{}
}

func newInferenceLimiter(capacity int) *inferenceLimiter {
	if capacity < 1 {
		return nil
	}
	return &inferenceLimiter{slots: make(chan struct{}, capacity)}
}

// acquire takes a slot, waiting until one frees up or ctx is done.
func (l *inferenceLimiter) acquire(ctx context.Context) error {
	if l == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case l.slots <- struct{}{}:
		return nil
	}
}

// release returns a slot taken by acquire. No-op for an unlimited gate.
func (l *inferenceLimiter) release() {
	if l == nil {
		return
	}
	<-l.slots
}

var (
	inferenceLimitOnce sync.Once
	processLimiter     *inferenceLimiter
)

// SetInferenceLimit caps how many DeepDoc inference calls this process may run
// at once. It must be called once at startup by the process owner; later calls
// are ignored so the capacity can never change under calls already holding
// slots. n < 1 leaves the gate open (unlimited).
func SetInferenceLimit(n int) {
	inferenceLimitOnce.Do(func() {
		processLimiter = newInferenceLimiter(n)
	})
}

// InferenceLimit returns the registered capacity, or 0 when unlimited/no
// budget was registered.
func InferenceLimit() int {
	if processLimiter == nil {
		return 0
	}
	return cap(processLimiter.slots)
}

// AcquireInference takes one process-wide inference slot, blocking until one is
// free or ctx is done. It is a no-op when no budget was registered. Every
// inference backend (in-process sessions and remote clients) must call this
// before starting an inference call and pair it with ReleaseInference.
func AcquireInference(ctx context.Context) error {
	return processLimiter.acquire(ctx)
}

// ReleaseInference returns a slot taken by AcquireInference. No-op when no
// budget was registered.
func ReleaseInference() {
	processLimiter.release()
}

// swapProcessInferenceLimiter installs l for the duration of a test and returns
// a restore func. Unregistered production code writes the gate exactly once at
// startup; tests may swap it as long as they restore the previous value.
func swapProcessInferenceLimiter(l *inferenceLimiter) func() {
	old := processLimiter
	processLimiter = l
	return func() { processLimiter = old }
}
