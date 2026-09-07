package utility

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPoolSubmitAndStats(t *testing.T) {
	pool := NewWorkerPool[int, int](2, 4, func(_ context.Context, in int) (int, error) {
		return in * 2, nil
	})
	defer pool.StopWait()

	f1, err := pool.Submit(t.Context(), 2)
	if err != nil {
		t.Fatalf("Submit(2): %v", err)
	}
	f2, err := pool.Submit(t.Context(), 3)
	if err != nil {
		t.Fatalf("Submit(3): %v", err)
	}

	r1, err := f1.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait(2): %v", err)
	}
	r2, err := f2.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait(3): %v", err)
	}
	if r1.Value != 4 || r2.Value != 6 {
		t.Fatalf("unexpected results: %+v %+v", r1, r2)
	}

	stats := pool.Stats()
	if stats.DesiredWorkers != 2 {
		t.Fatalf("DesiredWorkers = %d, want 2", stats.DesiredWorkers)
	}
	if stats.SubmittedTotal != 2 || stats.CompletedTotal != 2 {
		t.Fatalf("stats totals = %+v, want submitted=2 completed=2", stats)
	}
	if stats.FailedTotal != 0 || stats.PendingTotal != 0 {
		t.Fatalf("stats failure/pending = %+v, want 0", stats)
	}
}

func TestWorkerPoolSubmitToCanceledTaskReturnsContextError(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var ranSecond atomic.Uint64

	pool := NewWorkerPool[int, int](1, 2, func(ctx context.Context, in int) (int, error) {
		if in == 1 {
			close(started)
			<-release
			return in, nil
		}
		ranSecond.Add(1)
		return in, ctx.Err()
	})
	defer pool.StopWait()

	firstCh := make(chan WorkerPoolResult[int, int], 1)
	if err := pool.SubmitTo(t.Context(), 1, firstCh); err != nil {
		t.Fatalf("SubmitTo(first): %v", err)
	}
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	secondCh := make(chan WorkerPoolResult[int, int], 1)
	if err := pool.SubmitTo(ctx, 2, secondCh); err != nil {
		t.Fatalf("SubmitTo(second): %v", err)
	}
	cancel()
	close(release)

	select {
	case <-firstCh:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for first result")
	}

	select {
	case res := <-secondCh:
		if res.Err == nil {
			t.Fatal("second result error = nil, want context cancellation")
		}
		if ranSecond.Load() != 0 {
			t.Fatalf("second handler ran %d times, want 0", ranSecond.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second result")
	}
}

func TestWorkerPoolResize(t *testing.T) {
	pool := NewWorkerPool[int, int](1, 2, func(_ context.Context, in int) (int, error) {
		return in, nil
	})
	defer pool.StopWait()

	pool.Resize(3)
	stats := pool.Stats()
	if stats.DesiredWorkers != 3 {
		t.Fatalf("DesiredWorkers = %d, want 3", stats.DesiredWorkers)
	}
}

// TestStopWaitConcurrentSubmitDoesNotPanic hammers Submit from several
// goroutines while StopWait runs, repeated many times. Pre-fix, a submit that
// passed the stopped-state check before StopWait closed workChan panicked with
// "send on closed channel", and the drain was blind to submits whose send was
// still blocked on a full queue. Post-fix every submit either lands on a live
// channel (and is fully processed) or returns ErrWorkerPoolStopped, and
// StopWait never returns with a submitted-but-unfinished task.
func TestStopWaitConcurrentSubmitDoesNotPanic(t *testing.T) {
	for iter := 0; iter < 200; iter++ {
		pool := NewWorkerPool[int, int](4, 8, func(_ context.Context, in int) (int, error) {
			return in + 1, nil
		})
		var wg sync.WaitGroup
		stop := make(chan struct{})
		for g := 0; g < 8; g++ {
			wg.Add(1)
			go func(seed int) {
				defer wg.Done()
				for i := 0; i < 200; i++ {
					select {
					case <-stop:
						return
					default:
					}
					if _, err := pool.Submit(t.Context(), seed+i); err != nil {
						if err != ErrWorkerPoolStopped {
							t.Errorf("unexpected submit error: %v", err)
						}
						return
					}
				}
			}(g * 1000)
		}
		pool.StopWait()
		close(stop)
		wg.Wait()

		st := pool.Stats()
		if st.SubmittedTotal != st.CompletedTotal {
			t.Fatalf("iter %d: StopWait returned with %d submitted but %d completed",
				iter, st.SubmittedTotal, st.CompletedTotal)
		}
	}
}

// TestStopWaitWaitsForInFlightTask verifies the StopWait drain blocks until a
// task currently running in a worker has completed, so its result is delivered
// before StopWait returns.
func TestStopWaitWaitsForInFlightTask(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})

	pool := NewWorkerPool[int, int](1, 2, func(_ context.Context, in int) (int, error) {
		close(started)
		<-release
		return in, nil
	})
	// release is closed inline once the worker is guaranteed blocked in the
	// handler; the deferred close covers the early-return (failing) path so a
	// worker is never left blocked forever. sync.Once keeps the two paths from
	// double-closing the channel.
	var closeReleaseOnce sync.Once
	closeRelease := func() { closeReleaseOnce.Do(func() { close(release) }) }
	defer closeRelease()

	f, err := pool.Submit(t.Context(), 42)
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	<-started

	go func() {
		pool.StopWait()
		close(done)
	}()

	// StopWait must not return while the in-flight task is still running.
	select {
	case <-done:
		t.Fatal("StopWait returned before the in-flight task completed")
	case <-time.After(100 * time.Millisecond):
	}

	closeRelease()
	res, err := f.Wait(context.Background())
	if err != nil {
		t.Fatalf("Wait: %v", err)
	}
	if res.Value != 42 {
		t.Fatalf("result = %d, want 42", res.Value)
	}
	<-done
}

// TestSubmitAfterStopWaitReturnsStopped verifies the post-stop contract:
// submits on a stopped pool fail fast with ErrWorkerPoolStopped.
func TestSubmitAfterStopWaitReturnsStopped(t *testing.T) {
	pool := NewWorkerPool[int, int](1, 2, func(_ context.Context, in int) (int, error) {
		return in, nil
	})
	pool.StopWait()
	if _, err := pool.Submit(t.Context(), 1); err != ErrWorkerPoolStopped {
		t.Fatalf("Submit after StopWait = %v, want ErrWorkerPoolStopped", err)
	}
}

// TestStopWaitIdempotent verifies a second StopWait is a no-op: the state is
// already stopped and the channel already closed, so it must not double-close
// (panicking) or double-wait.
func TestStopWaitIdempotent(t *testing.T) {
	pool := NewWorkerPool[int, int](1, 2, func(_ context.Context, in int) (int, error) {
		return in, nil
	})
	pool.StopWait()
	pool.StopWait()
}

// TestResizeAfterStopWaitIsNoop verifies Resize on a stopped pool does not
// panic (workerWg.Add racing workerWg.Wait is a WaitGroup misuse) and does not
// revive workers.
func TestResizeAfterStopWaitIsNoop(t *testing.T) {
	pool := NewWorkerPool[int, int](2, 4, func(_ context.Context, in int) (int, error) {
		return in, nil
	})
	pool.StopWait()
	pool.Resize(8)
	if got := pool.Stats().LiveWorkers; got != 0 {
		t.Fatalf("Resize after StopWait revived workers: live=%d, want 0", got)
	}
}

// TestStopWaitWithBlockedSubmitNoDeadlock verifies that a submit blocked on a
// full workChan (all workers busy and the queue full) does not deadlock
// StopWait. Previously SubmitTo held mu across the blocking channel send, so a
// worker finishing its current job blocked in markDone on the same mu and could
// never receive the next queued job: the queue never drained, the blocked
// sender never made progress, and StopWait hung forever.
func TestStopWaitWithBlockedSubmitNoDeadlock(t *testing.T) {
	started := make(chan struct{})
	released := make(chan struct{})
	pool := NewWorkerPool[int, int](1, 1, func(_ context.Context, in int) (int, error) {
		if in == 1 {
			close(started)
		}
		<-released
		return in, nil
	})

	ctx := t.Context()
	if _, err := pool.Submit(ctx, 1); err != nil {
		t.Fatalf("first Submit: %v", err)
	}
	<-started // the worker is now inside the handler, blocked on released

	if _, err := pool.Submit(ctx, 2); err != nil {
		t.Fatalf("second Submit: %v", err)
	}
	// Queue capacity is 1 and now holds task 2, so the next send must block.

	blocked := make(chan error, 1)
	go func() {
		_, err := pool.Submit(ctx, 3)
		blocked <- err
	}()
	time.Sleep(50 * time.Millisecond) // let the third submit reach the channel send

	stopDone := make(chan struct{})
	go func() {
		pool.StopWait()
		close(stopDone)
	}()
	close(released) // let the worker drain the queue so the blocked send can proceed

	select {
	case <-stopDone:
		// No deadlock.
	case <-time.After(5 * time.Second):
		t.Fatal("StopWait deadlocked with a submit blocked on a full queue")
	}

	select {
	case err := <-blocked:
		if err != nil {
			t.Fatalf("blocked submit returned: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("blocked submit never completed after StopWait")
	}

	st := pool.Stats()
	if st.SubmittedTotal != 3 || st.CompletedTotal != 3 {
		t.Fatalf("expected 3/3 tasks completed, got submitted=%d completed=%d",
			st.SubmittedTotal, st.CompletedTotal)
	}
}
