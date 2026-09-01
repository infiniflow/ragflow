package syncer

import (
	"context"
	"errors"
	"ragflow/internal/service"
	"testing"
	"time"
)

// TestSyncJobExecutorUsesIdleWorkers verifies one task can use the full shared pool.
func TestSyncJobExecutorUsesIdleWorkers(t *testing.T) {
	executor := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 3})
	defer executor.Close()

	queue, err := executor.RegisterTask(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("register task: %v", err)
	}
	defer queue.Close()

	started := make(chan struct{}, 3)
	release := make(chan struct{})
	results := make([]<-chan syncJobResult, 0, 3)
	for i := 0; i < 3; i++ {
		result, err := queue.Submit(t.Context(), func(ctx context.Context) (service.SyncStats, error) {
			started <- struct{}{}
			<-release
			return service.SyncStats{Added: 1}, nil
		})
		if err != nil {
			t.Fatalf("submit: %v", err)
		}
		results = append(results, result)
	}

	for i := 0; i < 3; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatalf("started workers = %d, want 3", i)
		}
	}
	close(release)
	for _, result := range results {
		if item := <-result; item.err != nil || item.stats.Added != 1 {
			t.Fatalf("job result = %+v", item)
		}
	}
}

// TestSyncJobExecutorDispatchesRoundRobin verifies a waiting task is not hidden behind one large task.
func TestSyncJobExecutorDispatchesRoundRobin(t *testing.T) {
	executor := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 1})
	defer executor.Close()

	task1, err := executor.RegisterTask(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("register task-1: %v", err)
	}
	defer task1.Close()
	task2, err := executor.RegisterTask(t.Context(), "task-2")
	if err != nil {
		t.Fatalf("register task-2: %v", err)
	}
	defer task2.Close()

	started := make(chan string, 3)
	releaseFirst := make(chan struct{})
	firstResult, err := task1.Submit(t.Context(), func(ctx context.Context) (service.SyncStats, error) {
		started <- "task-1-a"
		<-releaseFirst
		return service.SyncStats{Added: 1}, nil
	})
	if err != nil {
		t.Fatalf("submit first task-1: %v", err)
	}
	if got := waitStarted(t, started); got != "task-1-a" {
		t.Fatalf("first started = %s", got)
	}

	task2Result, err := task2.Submit(t.Context(), func(ctx context.Context) (service.SyncStats, error) {
		started <- "task-2"
		return service.SyncStats{Updated: 1}, nil
	})
	if err != nil {
		t.Fatalf("submit task-2: %v", err)
	}
	task1Result, err := task1.Submit(t.Context(), func(ctx context.Context) (service.SyncStats, error) {
		started <- "task-1-b"
		return service.SyncStats{Skipped: 1}, nil
	})
	if err != nil {
		t.Fatalf("submit second task-1: %v", err)
	}

	close(releaseFirst)
	if item := <-firstResult; item.err != nil {
		t.Fatalf("first task-1 result: %v", item.err)
	}
	if got := waitStarted(t, started); got != "task-2" {
		t.Fatalf("second started = %s, want task-2", got)
	}
	if item := <-task2Result; item.err != nil || item.stats.Updated != 1 {
		t.Fatalf("task-2 result = %+v", item)
	}
	if got := waitStarted(t, started); got != "task-1-b" {
		t.Fatalf("third started = %s, want task-1-b", got)
	}
	if item := <-task1Result; item.err != nil || item.stats.Skipped != 1 {
		t.Fatalf("second task-1 result = %+v", item)
	}
}

// TestSyncJobExecutorCloseSettlesQueuedJobs verifies shutdown replies to jobs still in task queues.
func TestSyncJobExecutorCloseSettlesQueuedJobs(t *testing.T) {
	executor := NewSyncJobExecutor(SyncJobExecutorConfig{WorkerCount: 1, JobQueueSize: 1, PerTaskQueueSize: 4})
	queue, err := executor.RegisterTask(t.Context(), "task-1")
	if err != nil {
		t.Fatalf("register task: %v", err)
	}

	started := make(chan struct{})
	release := make(chan struct{})
	results := make([]<-chan syncJobResult, 0, 4)
	first, err := queue.Submit(t.Context(), func(ctx context.Context) (service.SyncStats, error) {
		close(started)
		<-release
		return service.SyncStats{Added: 1}, nil
	})
	if err != nil {
		t.Fatalf("submit first: %v", err)
	}
	results = append(results, first)
	<-started

	for i := 0; i < 3; i++ {
		result, err := queue.Submit(t.Context(), func(ctx context.Context) (service.SyncStats, error) {
			return service.SyncStats{Updated: 1}, nil
		})
		if err != nil {
			t.Fatalf("submit queued %d: %v", i, err)
		}
		results = append(results, result)
	}

	closed := make(chan struct{})
	go func() {
		executor.Close()
		close(closed)
	}()
	close(release)

	closedErrs := 0
	for _, result := range results {
		select {
		case item := <-result:
			if errors.Is(item.err, errSyncJobExecutorClosed) {
				closedErrs++
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for shutdown result")
		}
	}
	if closedErrs == 0 {
		t.Fatalf("queued jobs were not settled with closed error")
	}
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatalf("executor did not close")
	}
}

func waitStarted(t *testing.T, started <-chan string) string {
	t.Helper()
	select {
	case task := <-started:
		return task
	case <-time.After(time.Second):
		t.Fatalf("timed out waiting for job start")
		return ""
	}
}
