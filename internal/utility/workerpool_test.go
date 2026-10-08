package utility

import (
	"context"
	"errors"
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

// TestWorkerPool_ResizeShrinkThenGrowDoesNotOverspawn covers the
// "shrink then grow while idle" bug: workers only retire after completing a
// job, so an idle pool keeps its old live count. Resize computes the spawn
// count from the *previous* desired (now low) instead of the live count, and
// grows back to a value still below live — spawning (target − desired) extra
// goroutines on top of the still-idle original. With the fix the spawn count
// is computed from live, so growing to a value ≤ live starts zero workers
// while growing to > live starts only the gap.
//
// The test also verifies queued work continues to run after the sequence, so
// the fix does not regress throughput.
func TestWorkerPool_ResizeShrinkThenGrowDoesNotOverspawn(t *testing.T) {
	for iter := 0; iter < 10; iter++ {
		pool := NewWorkerPool[int, int](4, 32, func(_ context.Context, in int) (int, error) {
			return in, nil
		})

		// Wait for the 4 initial workers to register in liveWorkers. The
		// worker goroutine increments liveWorkers before parking on the
		// workChan receive, but NewWorkerPool does not synchronize on that
		// increment, so reading liveWorkers immediately after construction
		// can race with worker startup.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) && pool.Stats().LiveWorkers < 4 {
			time.Sleep(5 * time.Millisecond)
		}

		// No work submitted between the two Resize calls, so the 4 initial
		// workers stay idle and never trigger self-retirement.
		pool.Resize(1)
		pool.Resize(3)
		// Pre-fix: live=4 (idle) + 2 (new) = 6.
		// Post-fix: live=4, growing to 3 ≤ 4 spawns zero extra workers.
		if got := pool.Stats().LiveWorkers; got != 4 {
			pool.StopWait()
			t.Fatalf("iter %d: live=%d, want 4 (shrink-then-grow overspawned)", iter, got)
		}

		ctx := t.Context()
		for i := 0; i < 8; i++ {
			f, err := pool.Submit(ctx, i)
			if err != nil {
				pool.StopWait()
				t.Fatalf("iter %d Submit(%d): %v", iter, i, err)
			}
			res, err := f.Wait(ctx)
			if err != nil {
				pool.StopWait()
				t.Fatalf("iter %d Wait(%d): %v", iter, i, err)
			}
			if res.Value != i {
				pool.StopWait()
				t.Fatalf("iter %d result[%d] = %d, want %d", iter, i, res.Value, i)
			}
		}
		pool.StopWait()
	}
}

// TestWorkerPool_ShrinkDoesNotRetireBelowTarget covers the retirement race:
// N workers all finish a job at the same instant, all observe live > desired
// concurrently, and (without serialization) all return — taking live below
// desired. With the retirement decision serialized under the same mu Resize
// holds, the first worker to retire decrements live, the next worker sees the
// decremented count, and retirements stop exactly at desired.
//
// 16 workers gives the race a wide enough window to manifest reliably under
// -race (the OS schedules enough of them into the retirement check
// concurrently that the un-decremented live value is observed by all of them).
func TestWorkerPool_ShrinkDoesNotRetireBelowTarget(t *testing.T) {
	const nWorkers = 16
	const nRetained = 2
	for iter := 0; iter < 50; iter++ {
		release := make(chan struct{})
		started := make(chan struct{}, nWorkers)
		pool := NewWorkerPool[int, int](nWorkers, 64, func(_ context.Context, in int) (int, error) {
			started <- struct{}{}
			<-release
			return in, nil
		})

		var futures [nWorkers]WorkerPoolFuture[int, int]
		for i := 0; i < nWorkers; i++ {
			f, err := pool.Submit(t.Context(), i)
			if err != nil {
				close(release)
				pool.StopWait()
				t.Fatalf("iter %d Submit(%d): %v", iter, i, err)
			}
			futures[i] = f
		}
		for i := 0; i < nWorkers; i++ {
			<-started
		}
		// All nWorkers workers are blocked in the handler. Shrink to
		// nRetained: pre-fix the desired=nRetained is now below the
		// nWorkers still-live workers; once we release them they can all
		// decide to retire before any of them decrements liveWorkers,
		// leaving live < nRetained. Post-fix retirements serialize under mu
		// and stop at exactly nRetained.
		pool.Resize(nRetained)
		close(release)
		for i := 0; i < nWorkers; i++ {
			res, err := futures[i].Wait(context.Background())
			if err != nil {
				pool.StopWait()
				t.Fatalf("iter %d Wait(%d): %v", iter, i, err)
			}
			if res.Value != i {
				pool.StopWait()
				t.Fatalf("iter %d result[%d] = %d, want %d", iter, i, res.Value, i)
			}
		}
		// Give any in-flight retirement goroutine a chance to land its
		// decrement. Stats() reads liveWorkers atomically, but the race we're
		// pinning is the decision order under mu, not the visibility of the
		// final write.
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if got := pool.Stats().LiveWorkers; got == nRetained {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		if got := pool.Stats().LiveWorkers; got != nRetained {
			pool.StopWait()
			t.Fatalf("iter %d: live=%d, want %d (retirement raced below target)", iter, got, nRetained)
		}
		pool.StopWait()
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
						if !errors.Is(err, ErrWorkerPoolStopped) {
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
	if _, err := pool.Submit(t.Context(), 1); !errors.Is(err, ErrWorkerPoolStopped) {
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
