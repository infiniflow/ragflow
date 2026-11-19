package syncer

import (
	"context"
	"ragflow/internal/service"
	"reflect"
	"testing"
	"time"
)

// TestSyncJobExecutorIdleDoesNotPoll is the RED guard for the busy-poll fix:
// an idle executor must not spin on a 1ms timer (each time.After tick allocates
// three objects, ~300 per 100ms). An event-driven wakeup allocates only setup.
func TestSyncJobExecutorIdleDoesNotPoll(t *testing.T) {
	const idle = 100 * time.Millisecond
	allocs := testing.AllocsPerRun(12, func() {
		e := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 1})
		time.Sleep(idle)
		e.Close()
	})
	if allocs > 150 {
		t.Fatalf("idle executor allocations = %.1f, want <= 150 (1ms polling loop detected)", allocs)
	}
}

// TestSyncJobExecutorHasSharedBufferedWakeup asserts the wakeup wiring by shape:
// a buffered channel shared between executor and every task queue. Reflection is
// used so the assertion is a real failure (not a compile error) on the old code.
func TestSyncJobExecutorHasSharedBufferedWakeup(t *testing.T) {
	e := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 1})
	defer e.Close()
	q, err := e.RegisterTask(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("register task: %v", err)
	}

	execWakeup := reflect.ValueOf(e).Elem().FieldByName("wakeup")
	queueWakeup := reflect.ValueOf(q).Elem().FieldByName("wakeup")
	if !execWakeup.IsValid() {
		t.Fatal("SyncJobExecutor has no wakeup field")
	}
	if !queueWakeup.IsValid() {
		t.Fatal("SyncJobQueue has no wakeup field")
	}
	if execWakeup.Kind() != reflect.Chan || queueWakeup.Kind() != reflect.Chan {
		t.Fatalf("wakeup fields must be channels, got %v / %v", execWakeup.Kind(), queueWakeup.Kind())
	}
	if execWakeup.Cap() < 1 {
		t.Fatalf("executor wakeup cap = %d, want >= 1", execWakeup.Cap())
	}
	if execWakeup.Pointer() != queueWakeup.Pointer() {
		t.Fatal("task queue wakeup is not the executor wakeup channel")
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

	results := make([]<-chan syncJobResult, 0, jobs)
	for i := 0; i < jobs; i++ {
		result, err := queue.Submit(t.Context(), func(context.Context) (service.SyncStats, error) {
			return service.SyncStats{Added: 1}, nil
		})
		if err != nil {
			t.Fatalf("submit %d: %v", i, err)
		}
		results = append(results, result)
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
