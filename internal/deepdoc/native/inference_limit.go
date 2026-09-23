//go:build cgo

package native

// inference_limit.go — the process-wide DeepDoc inference budget.
//
// ONNX Runtime gives every session its own intra-op thread pool (the C API
// never switches a session onto a shared/global pool), so the threads a single
// DeepDoc inference Run occupies equal the per-session intra-op thread count
// (see intraOpThreadCount in inference_config.go, registered once at startup as
// max(1, N/K) from the CPU-core budget N and the concurrency K). The number of
// Runs in flight at once is bounded by the capacity registered here. Together
// the two levers set the total cores inference may occupy: intraOpThreadCount ×
// concurrency ≤ N.
//
// This capacity lives here — at the one boundary every inference call passes
// through — instead of at the call sites, so neither a new caller nor a new call
// site inside an existing caller can bypass the budget by forgetting to acquire
// a slot.
//
// The capacity is a process resource policy and is deliberately NOT decided
// here: the process owner registers it at startup (the server's backend wiring
// calls SetInferenceLimit with the parser's budget). A process that never
// registers one stays unlimited, which is what a one-shot CLI or a test process
// wants.

import (
	"context"
	"sync"
)

// inferenceLimiter caps how many inference calls may run at once. A nil
// *inferenceLimiter admits everything, so an unregistered process needs no
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

// release returns a slot taken by acquire.
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
// at once. It is meant to be called once, at startup, by the process owner:
// later calls are ignored, so the capacity can never change under calls that are
// already holding slots. n < 1 leaves the gate open (unlimited), which is also
// the state of a process that never calls this.
func SetInferenceLimit(n int) {
	inferenceLimitOnce.Do(func() {
		processLimiter = newInferenceLimiter(n)
	})
}

// InferenceLimit returns the registered capacity, or 0 when the process never
// registered one (unlimited). Startup wiring logs it so an unlimited process is
// visible rather than silently unbounded.
func InferenceLimit() int {
	if processLimiter == nil {
		return 0
	}
	return cap(processLimiter.slots)
}

// acquireInference takes one process-wide inference slot; releaseInference
// returns it. Both are no-ops when no limit was registered.
func acquireInference(ctx context.Context) error {
	return processLimiter.acquire(ctx)
}

func releaseInference() {
	processLimiter.release()
}
