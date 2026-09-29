//go:build cgo

package native

import (
	"context"
	"errors"
	"testing"
	"time"
)

// Unit tests for the process-wide inference gate (inference_limit.go). They are
// model-free: the gate is exercised on its own, and the pool wiring is checked
// with a local pool whose constructor never touches ONNX Runtime.

// swapProcessLimiter installs l as the process gate for the duration of a test.
// The package-level value is normally written once at startup (SetInferenceLimit)
// and only read afterwards, so a test may swap it as long as it restores the
// previous value before the next test runs.
func swapProcessLimiter(l *inferenceLimiter) func() {
	old := processLimiter
	processLimiter = l
	return func() { processLimiter = old }
}

func TestInferenceLimitUnregisteredAdmitsEverything(t *testing.T) {
	if got := InferenceLimit(); got != 0 {
		t.Fatalf("unregistered process reports limit %d, want 0 (unlimited)", got)
	}
	if err := acquireInference(context.Background()); err != nil {
		t.Fatalf("acquire with no limit registered: %v", err)
	}
	releaseInference()
}

func TestInferenceLimiterQueuesWhenExhausted(t *testing.T) {
	l := newInferenceLimiter(1)
	ctx := context.Background()

	if err := l.acquire(ctx); err != nil {
		t.Fatalf("first acquire: %v", err)
	}

	second := make(chan error, 1)
	go func() { second <- l.acquire(ctx) }()

	select {
	case err := <-second:
		t.Fatalf("second acquire returned (%v) while the only slot was held", err)
	case <-time.After(200 * time.Millisecond):
	}

	l.release()
	select {
	case err := <-second:
		if err != nil {
			t.Fatalf("second acquire after release: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("second acquire did not proceed after the slot was released")
	}
	l.release()
}

func TestInferenceLimiterHonorsContextCancellation(t *testing.T) {
	l := newInferenceLimiter(1)
	if err := l.acquire(context.Background()); err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer l.release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := l.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("acquire on a cancelled context returned %v, want context.Canceled", err)
	}
}

// TestSessionPoolGetTakesInferenceSlot pins the wiring: a pool hands out no
// session while the process budget is exhausted, and proceeds once a slot frees
// up. The local pool's constructor fails on purpose — it must not even be
// reached before the slot is available.
func TestSessionPoolGetTakesInferenceSlot(t *testing.T) {
	restore := swapProcessLimiter(newInferenceLimiter(1))
	defer restore()

	if err := acquireInference(context.Background()); err != nil {
		t.Fatalf("hold slot: %v", err)
	}

	pool := newSessionPool[sessKey, *session](0, 0)
	constructed := make(chan struct{})
	errs := make(chan error, 1)
	go func() {
		_, _, err := pool.Get(context.Background(), sessKey{modelPath: "test"}, func() (*session, error) {
			close(constructed)
			return nil, errors.New("constructor reached")
		})
		errs <- err
	}()

	select {
	case <-constructed:
		t.Fatal("pool reached the session constructor while the inference budget was exhausted")
	case <-time.After(200 * time.Millisecond):
	}

	releaseInference()
	select {
	case err := <-errs:
		if err == nil || err.Error() != "constructor reached" {
			t.Fatalf("Get returned %v, want the constructor error", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Get did not proceed after the inference slot was released")
	}
}
