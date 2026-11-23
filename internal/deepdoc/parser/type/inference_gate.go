package doctype

import (
	"context"
	"sync"
	"sync/atomic"
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
	slots        chan struct{}
	backpressure atomic.Bool
	bpMu         sync.Mutex
	bpChanged    chan struct{}
}

func newInferenceLimiter(capacity int) *inferenceLimiter {
	if capacity < 1 {
		return nil
	}
	return &inferenceLimiter{
		slots:     make(chan struct{}, capacity),
		bpChanged: make(chan struct{}),
	}
}

// acquire takes a slot, waiting until one frees up or ctx is done.
func (l *inferenceLimiter) acquire(ctx context.Context) error {
	if l == nil {
		return nil
	}
	// Reject an already-cancelled context before any select: with a free slot
	// and a done ctx both cases are ready and Go may pick the slot send.
	if err := ctx.Err(); err != nil {
		return err
	}
	for {
		// Fast path: an atomic read of the flag needs no lock. Only when
		// pressure is actually on do we take bpMu to grab a consistent
		// (flag, channel) snapshot — the channel can be swapped by
		// SetBackpressure, so reading flag and channel in separate steps would
		// risk a lost wakeup. This keeps the common (no-pressure) acquire
		// allocation- and contention-free.
		if l.backpressure.Load() {
			pressured, ch := l.pressureSnapshot()
			if pressured {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ch:
					continue
				}
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case l.slots <- struct{}{}:
		}
		// Recheck pressure after taking the slot: a caller that passed the
		// check above may have queued for a slot while the janitor engaged
		// backpressure. Do not start inference under pressure — give the slot
		// back and wait for pressure to lift. An atomic read suffices here:
		// a false positive just retries harmlessly, and the locked snapshot
		// above handles the real wait.
		if l.backpressure.Load() {
			<-l.slots
			if err := ctx.Err(); err != nil {
				return err
			}
			continue
		}
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

// SetBackpressure enables/disables the device-pressure gate. When enabled,
// acquire() blocks new calls until it is lifted (in-flight calls finish and
// free memory). Driven by the resource janitor. It is a soft, runtime-
// adjustable complement to the fixed K capacity.
func (l *inferenceLimiter) SetBackpressure(on bool) {
	if l == nil {
		return
	}
	l.bpMu.Lock()
	defer l.bpMu.Unlock()
	if l.backpressure.Load() == on {
		return
	}
	l.backpressure.Store(on)
	close(l.bpChanged)
	l.bpChanged = make(chan struct{})
}

func (l *inferenceLimiter) Backpressure() bool {
	if l == nil {
		return false
	}
	return l.backpressure.Load()
}

// pressureSnapshot returns the pressure flag and the notification channel
// under one lock, so a concurrent SetBackpressure(false) cannot slip between
// the flag read and the channel fetch and strand the caller on a stale channel.
func (l *inferenceLimiter) pressureSnapshot() (bool, <-chan struct{}) {
	l.bpMu.Lock()
	defer l.bpMu.Unlock()
	return l.backpressure.Load(), l.bpChanged
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

// SetBackpressure enables/disables the device-pressure gate at process scope.
func SetBackpressure(on bool) {
	processLimiter.SetBackpressure(on)
}

// Backpressure reports whether the device-pressure gate is engaged.
func Backpressure() bool {
	return processLimiter.Backpressure()
}

// swapProcessInferenceLimiter installs l for the duration of a test and returns
// a restore func. Unregistered production code writes the gate exactly once at
// startup; tests may swap it as long as they restore the previous value.
func swapProcessInferenceLimiter(l *inferenceLimiter) func() {
	old := processLimiter
	processLimiter = l
	return func() { processLimiter = old }
}

// ── Local (in-process) session gate ──────────────────────────────────────
//
// This is the INNER budget. While the outer gate above bounds every inference
// call at K (so remote HTTP clients can use the full budget), this gate bounds
// only in-process ONNX Runs at min(K, available cores), so local inference can
// never oversubscribe the machine. Every in-process Run passes through the
// native session pool, which acquires this gate; remote clients do not.
//
// localLimiterInst is written once at startup by SetLocalInferenceLimit and
// read afterwards. Tests swap it via swapLocalLimiter and restore it.
var localLimiterInst *inferenceLimiter

// LocalInferenceCapacity derives the in-process Run budget from the single
// user-facing concurrency K and the machine's CPU cores: min(K, cores), with
// K<=0 meaning "all cores" and cores<=0 flooring to 1. The result is the
// capacity of the local gate and never exceeds the core count, which is what
// keeps in-memory inference from oversubscribing even when K is large (e.g.
// K=32 on a 16-core box yields a local capacity of 16).
func LocalInferenceCapacity(k, cores int) int {
	if cores < 1 {
		return 1
	}
	if k <= 0 || k > cores {
		return cores
	}
	return k
}

// SetLocalInferenceLimit registers the inner in-process gate capacity. It is
// called once at startup with LocalInferenceCapacity(K, cores). Registration
// is guarded by the limiter being nil rather than sync.Once: production writes
// it once, while tests using swapLocalLimiter restore it to nil and can drive
// the setter again under `-count=N`.
func SetLocalInferenceLimit(n int) {
	if localLimiterInst == nil {
		localLimiterInst = newInferenceLimiter(n)
	}
}

// AcquireLocal takes one in-process inference slot, blocking until one is free
// or ctx is done. No-op when no local budget was registered.
func AcquireLocal(ctx context.Context) error {
	return localLimiterInst.acquire(ctx)
}

// ReleaseLocal returns an in-process inference slot taken by AcquireLocal.
func ReleaseLocal() {
	localLimiterInst.release()
}

func swapLocalLimiter(l *inferenceLimiter) func() {
	old := localLimiterInst
	localLimiterInst = l
	return func() { localLimiterInst = old }
}
