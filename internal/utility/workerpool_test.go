package utility

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestWorkerPoolSubmitAndStats(t *testing.T) {
	pool := NewWorkerPool[int, int](2, 4, func(_ context.Context, in int) (int, error) {
		return in * 2, nil
	})
	defer pool.StopWait()

	f1, err := pool.Submit(context.Background(), 2)
	if err != nil {
		t.Fatalf("Submit(2): %v", err)
	}
	f2, err := pool.Submit(context.Background(), 3)
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
	if err := pool.SubmitTo(context.Background(), 1, firstCh); err != nil {
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
