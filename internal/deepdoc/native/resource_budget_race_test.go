//go:build cgo

package native

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	doctype "ragflow/internal/deepdoc/parser/type"
)

// sampleResource is a controllable ResourceBudget for driving the janitor.
type sampleResource struct {
	mu sync.Mutex
	v  int
}

func (r *sampleResource) Name() string { return "sample" }
func (r *sampleResource) TotalMB() int { return 10000 }
func (r *sampleResource) UsedMB() int  { r.mu.Lock(); defer r.mu.Unlock(); return r.v }
func (r *sampleResource) set(v int)    { r.mu.Lock(); r.v = v; r.mu.Unlock() }

// TestIdleAdmissionReservesAtomically reproduces the TOCTOU in idle
// admission: a check-only shouldPoolIdle() lets concurrent callers all observe
// a below-cap count before any of them reserves. Correct admission must
// check-and-reserve atomically, so at most totalCap callers may be admitted.
//
// Adversarial fixture: many concurrent check calls against a small cap with no
// external serialization. Against a check-only implementation every caller
// passes -> RED; against an atomic check-and-reserve at most cap pass -> GREEN.
func TestIdleAdmissionReservesAtomically(t *testing.T) {
	ConfigureIdleBudget(idleBudgetConfig{enabled: true, totalCap: 3})
	defer ConfigureIdleBudget(idleBudgetConfig{})

	const goroutines = 64
	var admitted atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			if shouldPoolIdle() { // must reserve the slot atomically
				admitted.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := admitted.Load(); got > 3 {
		t.Fatalf("admitted %d idle sessions with totalCap=3 (check-then-increment TOCTOU)", got)
	}
}

// TestJanitorClearsBackpressureOnFailedSample reproduces the stuck-backpressure
// bug: when UsedMB() returns 0 (read failure), the janitor `continue`s and
// never lifts backpressure previously engaged by a high sample. acquire()
// then blocks every deadline-less caller indefinitely.
func TestJanitorClearsBackpressureOnFailedSample(t *testing.T) {
	doctype.SetInferenceLimit(4)
	ConfigureIdleBudget(idleBudgetConfig{enabled: true, highWaterMB: 8000, lowWaterMB: 5000})
	defer ConfigureIdleBudget(idleBudgetConfig{})
	defer doctype.SetBackpressure(false)

	r := &sampleResource{}
	r.set(9000) // high water -> engage backpressure

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go resourceJanitor(ctx, r, 10*time.Millisecond, 8000)

	waitUntil(t, 2*time.Second, "backpressure to engage", func() bool { return doctype.Backpressure() })

	r.set(0) // failed sample

	waitUntil(t, 2*time.Second, "backpressure to clear after failed sample", func() bool {
		return !doctype.Backpressure()
	})
}

// TestJanitorPrunesIdleBetweenWatermarks reproduces the missing mid-band TTL
// prune: with used between low and high water, neither branch runs, so idle
// sessions older than idleTTL are never pruned.
func TestJanitorPrunesIdleBetweenWatermarks(t *testing.T) {
	ConfigureIdleBudget(idleBudgetConfig{
		enabled: true, highWaterMB: 8000, lowWaterMB: 5000, idleTTL: 10 * time.Millisecond,
	})
	defer ConfigureIdleBudget(idleBudgetConfig{})

	var pruned atomic.Int64
	registerIdlePruner(func(maxAge time.Duration, maxCount int) int {
		pruned.Add(1)
		return 0
	})

	r := &sampleResource{}
	r.set(7000) // between low and high water

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go resourceJanitor(ctx, r, 10*time.Millisecond, 8000)

	waitUntil(t, 2*time.Second, "idle prune between watermarks", func() bool {
		return pruned.Load() > 0
	})
}

func waitUntil(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

// TestEvictedKeyReleaseDoesNotDecrementIdleCounter verifies the admission/
// eviction asymmetry: shouldPoolIdle() reserves under one lock and only when
// it actually returns true (and the session is retained). Releasing a session
// whose key-pool was already evicted (live=false) must NOT decrement the
// global idle counter — shouldPoolIdle() was never called for it.
func TestEvictedKeyReleaseDoesNotDecrementIdleCounter(t *testing.T) {
	ConfigureIdleBudget(idleBudgetConfig{enabled: true, totalCap: 100})
	defer ConfigureIdleBudget(idleBudgetConfig{})
	// Reset the global counter for a deterministic assertion.
	idleBudget.mu.Lock()
	idleBudget.totalIdle = 0
	idleBudget.mu.Unlock()

	newFn := func() (*fakeSession, error) { return &fakeSession{}, nil }
	p := newSessionPool[string, *fakeSession](1 /*maxKeys*/, 0 /*maxFree*/)

	// Get+release key "a" -> retained idle (totalIdle=1). Keep its release fn.
	_, relA, err := p.Get(context.Background(), "a", newFn)
	if err != nil {
		t.Fatal(err)
	}
	relA()

	_, totalAfterA, _, _ := IdleBudgetStatus()
	if totalAfterA != 1 {
		t.Fatalf("after first release totalIdle=%d, want 1", totalAfterA)
	}

	// Get key "b" evicts key "a" (maxKeys=1): a's key-pool goes live=false and
	// its retained session is destroyed with a matching noteIdleRemoved.
	_, relB, err := p.Get(context.Background(), "b", newFn)
	if err != nil {
		t.Fatal(err)
	}
	// Release b so it becomes an idle session (totalIdle=1). This is the
	// counter that a stale release must NOT decrement.
	relB()
	_, totalBeforeStale, _, _ := IdleBudgetStatus()
	if totalBeforeStale != 1 {
		t.Fatalf("before stale release totalIdle=%d, want 1", totalBeforeStale)
	}

	// Now release the stale "a" session: a's key-pool is dead, so
	// shouldPoolIdle() is never called; totalIdle must stay 1.
	relA()

	_, totalAfterStale, _, _ := IdleBudgetStatus()
	if totalAfterStale != 1 {
		t.Fatalf("after stale release totalIdle=%d, want 1 (counter wrongly decremented by stale release)", totalAfterStale)
	}
}
