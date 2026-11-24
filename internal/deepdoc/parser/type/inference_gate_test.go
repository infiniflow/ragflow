package doctype

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Tests for the process-wide inference gate living in the dependency-free
// doctype package. The gate bounds inference concurrency across every backend
// (in-process sessions and remote inference clients alike). These tests are
// model-free: the gate is exercised on its own.

func TestInferenceGateUnregisteredAdmitsEverything(t *testing.T) {
	restore := swapProcessInferenceLimiter(nil)
	defer restore()

	if got := InferenceLimit(); got != 0 {
		t.Fatalf("unregistered process reports limit %d, want 0 (unlimited)", got)
	}
	if err := AcquireInference(context.Background()); err != nil {
		t.Fatalf("acquire with no limit registered: %v", err)
	}
	ReleaseInference()
}

// Adversarial: with capacity 1 held, a second acquire MUST block. A no-op gate
// (the pre-fix behaviour for remote clients) lets it through immediately and
// fails here — this is the falsifiable assertion the whole change rests on.
func TestInferenceGateQueuesWhenExhausted(t *testing.T) {
	restore := swapProcessInferenceLimiter(newInferenceLimiter(1))
	defer restore()
	ctx := context.Background()

	if err := AcquireInference(ctx); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	second := make(chan error, 1)
	go func() { second <- AcquireInference(ctx) }()

	select {
	case err := <-second:
		ReleaseInference()
		t.Fatalf("second acquire returned (%v) while the only slot was held", err)
	case <-time.After(200 * time.Millisecond):
		// blocked as expected
	}
	ReleaseInference()

	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second acquire after release: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("second acquire did not proceed after the slot was released")
	}
	ReleaseInference()
}

// Adversarial (error path): a blocked acquire must abort on ctx cancellation
// instead of deadlocking, so a cancelled page/task never leaks a slot.
func TestInferenceGateAcquireHonorsContextCancellation(t *testing.T) {
	restore := swapProcessInferenceLimiter(newInferenceLimiter(1))
	defer restore()

	if err := AcquireInference(context.Background()); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer ReleaseInference()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- AcquireInference(ctx) }()
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled acquire error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled acquire did not return")
	}
}

// Adversarial (boundary): non-positive capacity means unlimited, not a
// zero-slot gate that deadlocks every acquire.
func TestInferenceGateNonPositiveCapacityIsUnlimited(t *testing.T) {
	restore := swapProcessInferenceLimiter(newInferenceLimiter(0))
	defer restore()

	if err := AcquireInference(context.Background()); err != nil {
		t.Fatalf("acquire with non-positive capacity: %v", err)
	}
	ReleaseInference()
}

// Adversarial (once semantics): the process capacity is fixed at first set;
// a later SetInferenceLimit must not change an already-registered gate.
func TestInferenceGateCapacityFixedAfterFirstSet(t *testing.T) {
	// Do not swap here: exercise the real sync.Once path.
	SetInferenceLimit(3)
	first := InferenceLimit()
	SetInferenceLimit(9)
	second := InferenceLimit()
	if first != 3 || second != 3 {
		t.Fatalf("capacity changed across SetInferenceLimit calls: first=%d second=%d, want 3/3", first, second)
	}
}
