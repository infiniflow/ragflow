package syncer

import (
	"context"
	"ragflow/internal/service"
	"sync"
	"testing"
	"time"
)

// TestSyncJobExecutorIdleDoesNotPoll is the RED guard for the busy-poll fix:
// an idle executor must not spin on a 1ms timer (each time.After tick allocates
// three objects, ~300 per 100ms). An event-driven wakeup allocates only setup.
func TestSyncJobExecutorIdleDoesNotPoll(t *testing.T) {
	const idle = 100 * time.Millisecond
	// Baseline measured on this host (Go 1.27): the old 1ms time.After loop
	// allocated ~291 objects per 100ms idle; the event-driven dispatcher
	// allocates only executor setup (~10). maxIdleAllocs leaves headroom for
	// runtime variation while still catching a reintroduced poll loop.
	// BenchmarkSyncJobExecutorIdle tracks allocs/op as the long-term guard.
	const maxIdleAllocs = 150
	allocs := testing.AllocsPerRun(12, func() {
		e := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 1})
		time.Sleep(idle)
		e.Close()
	})
	if allocs > maxIdleAllocs {
		t.Fatalf("idle executor allocations = %.1f, want <= %d (1ms polling loop detected)", allocs, maxIdleAllocs)
	}
}

// TestSyncJobSubmitSignalsWakeupAfterClose verifies submit publishes a wakeup
// token after enqueuing. The dispatcher is stopped first so the token is stable
// and observable.
func TestSyncJobSubmitSignalsWakeupAfterClose(t *testing.T) {
	e := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 1})
	q, err := e.RegisterTask(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("register task: %v", err)
	}
	e.Close()

	if _, err := q.Submit(t.Context(), func(context.Context) (service.SyncStats, error) {
		return service.SyncStats{Added: 1}, nil
	}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	select {
	case <-q.jobs:
	case <-time.After(time.Second):
		t.Fatal("submitted job was not enqueued")
	}
	select {
	case <-q.wakeup:
	default:
		t.Fatal("submit did not signal wakeup after enqueue")
	}
}

// TestSyncJobQueueSubmitWithNilWakeupDoesNotBlock guards the direct-construction
// path (no dispatcher, nil wakeup channel): submit must still enqueue and return.
func TestSyncJobQueueSubmitWithNilWakeupDoesNotBlock(t *testing.T) {
	q := &SyncJobQueue{taskID: "task-1", jobs: make(chan *syncJob, 1)}
	result, err := q.Submit(t.Context(), func(context.Context) (service.SyncStats, error) {
		return service.SyncStats{Added: 1}, nil
	})
	if err != nil {
		t.Fatalf("submit with nil wakeup: %v", err)
	}
	select {
	case job := <-q.jobs:
		stats, err := job.fn(t.Context())
		if err != nil || stats.Added != 1 {
			t.Fatalf("job result = %+v, %v", stats, err)
		}
		job.done <- syncJobResult{stats: stats}
		if item := <-result; item.stats.Added != 1 {
			t.Fatalf("result = %+v", item)
		}
	case <-time.After(time.Second):
		t.Fatal("job was not enqueued")
	}
}

// TestSyncJobExecutorWakeupBurstDispatchesAll verifies a burst of concurrent
// submits collapses into wakeup tokens without losing a single job.
func TestSyncJobExecutorWakeupBurstDispatchesAll(t *testing.T) {
	const jobs = 64
	executor := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 4, JobQueueSize: 8})
	defer executor.Close()
	queue, err := executor.RegisterTask(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("register task: %v", err)
	}
	defer queue.Close()

	// Release all submitters through a shared barrier so their wakeup signals
	// genuinely race; each writes to its own slice slot to avoid an unsafe
	// concurrent append. This exercises wakeup-token coalescing under concurrent
	// producers, which a sequential submit loop cannot.
	results := make([]<-chan syncJobResult, jobs)
	submitErrs := make([]error, jobs)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < jobs; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			result, err := queue.Submit(t.Context(), func(context.Context) (service.SyncStats, error) {
				return service.SyncStats{Added: 1}, nil
			})
			results[i] = result
			submitErrs[i] = err
		}(i)
	}
	close(start)
	wg.Wait()
	for i, err := range submitErrs {
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
	}
	for i, result := range results {
		select {
		case item := <-result:
			if item.err != nil || item.stats.Added != 1 {
				t.Fatalf("result %d = %+v", i, item)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("result %d timed out", i)
		}
	}
}

// BenchmarkSyncJobExecutorIdle tracks the allocation profile of a parked
// dispatcher so a future reintroduction of timer polling is caught.
func BenchmarkSyncJobExecutorIdle(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		e := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 1})
		time.Sleep(10 * time.Millisecond)
		e.Close()
	}
}
