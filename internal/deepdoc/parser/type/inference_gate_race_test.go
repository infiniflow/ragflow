package doctype

import (
	"context"
	"runtime"
	"testing"
	"time"
)

// TestBackpressureReleaseWakesWaiter reproduces the lost-notification race:
// acquire reads the pressure flag without bpMu, then fetches the notification
// channel under bpMu. If SetBackpressure(false) lands between those steps, it
// closes the OLD channel and installs a fresh open one; the waiter then blocks
// on the new channel that nobody will close. Repeating the toggle with a short
// timeout makes the race falsifiable: a correct implementation always wakes.
func TestBackpressureReleaseWakesWaiter(t *testing.T) {
	const iterations = 300
	for i := 0; i < iterations; i++ {
		l := newInferenceLimiter(1)
		restore := swapProcessInferenceLimiter(l)
		l.SetBackpressure(true)

		ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
		done := make(chan error, 1)
		go func() { done <- AcquireInference(ctx) }()

		// Widen the window between the flag read and the channel fetch.
		runtime.Gosched()
		SetBackpressure(false)

		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("iteration %d: acquire returned error %v", i, err)
			}
			ReleaseInference()
		case <-ctx.Done():
			t.Fatalf("iteration %d: acquire stayed blocked after backpressure was lifted (lost notification)", i)
		}
		cancel()
		restore()
	}
}

// TestAcquireDoesNotProceedWhenBackpressureEngagedWhileQueued reproduces the
// missing post-slot recheck: a caller that passed the pressure check while a
// slot was occupied must NOT start inference when backpressure is engaged
// before the slot is released.
func TestAcquireDoesNotProceedWhenBackpressureEngagedWhileQueued(t *testing.T) {
	l := newInferenceLimiter(1)
	restore := swapProcessInferenceLimiter(l)
	defer restore()

	// Occupy the only slot.
	if err := AcquireInference(context.Background()); err != nil {
		t.Fatalf("initial acquire: %v", err)
	}

	queued := make(chan error, 1)
	qctx, qcancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer qcancel()
	go func() { queued <- AcquireInference(qctx) }()

	// Let the queued caller pass the (currently false) pressure check and park
	// waiting for the slot.
	time.Sleep(30 * time.Millisecond)
	SetBackpressure(true)
	ReleaseInference()

	// The queued caller must not proceed while pressure is engaged.
	select {
	case err := <-queued:
		if err == nil {
			ReleaseInference()
		}
		t.Fatalf("queued acquire proceeded while backpressure was engaged (err=%v)", err)
	case <-time.After(250 * time.Millisecond):
		// Expected: still waiting.
	}

	// Lifting pressure must let it through.
	SetBackpressure(false)
	select {
	case err := <-queued:
		if err != nil {
			t.Fatalf("after lifting pressure: %v", err)
		}
		ReleaseInference()
	case <-time.After(3 * time.Second):
		t.Fatal("queued acquire never released after backpressure lifted")
	}
}

// TestAcquireRejectsAlreadyCancelledContextWithFreeSlot is the adversarial
// counterpart to the existing cancellation test (which holds the only slot).
// Here a slot is FREE and the context is ALREADY cancelled, so both select
// cases are ready; without an explicit ctx.Err() check Go may pick the send
// case and wrongly admit a cancelled request. Repeating makes it falsifiable.
func TestAcquireRejectsAlreadyCancelledContextWithFreeSlot(t *testing.T) {
	l := newInferenceLimiter(1)
	restore := swapProcessInferenceLimiter(l)
	defer restore()

	for i := 0; i < 200; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		err := AcquireInference(ctx)
		if err == nil {
			ReleaseInference() // clean up the wrongly-granted slot
			t.Fatalf("iteration %d: acquire succeeded with an already-cancelled context", i)
		}
		cancel()
	}
}
