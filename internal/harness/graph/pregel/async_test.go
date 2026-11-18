package pregel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"ragflow/internal/harness/graph/types"
)

func TestAsyncExecutor_ExecuteWithRetryHonorsWorkerPool(t *testing.T) {
	executor := NewAsyncExecutor(1)
	resultCh := executor.ExecuteWithRetry(
		context.Background(),
		"retry-task",
		func(context.Context) (any, error) {
			return len(executor.workerPool), nil
		},
		&RetryConfig{Policy: &types.RetryPolicy{MaxAttempts: 1}},
	)

	result, ok := <-resultCh
	if !ok || result == nil {
		t.Fatal("ExecuteWithRetry() returned no result")
	}
	if result.Err != nil {
		t.Fatalf("ExecuteWithRetry() error = %v", result.Err)
	}
	if availableSlots, ok := result.Output.(int); !ok || availableSlots != 0 {
		t.Fatalf("available worker slots during execution = %v, want 0", result.Output)
	}
}

func TestAsyncExecutor_CancelQueuedRetryTask(t *testing.T) {
	executor := NewAsyncExecutor(1)
	firstStarted := make(chan struct{})
	firstResultCh := executor.Execute(context.Background(), "blocking-task", func(ctx context.Context) (any, error) {
		close(firstStarted)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	<-firstStarted

	var retryCalls atomic.Int32
	retryResultCh := executor.ExecuteWithRetry(
		context.Background(),
		"queued-retry-task",
		func(context.Context) (any, error) {
			retryCalls.Add(1)
			return nil, nil
		},
		&RetryConfig{Policy: &types.RetryPolicy{MaxAttempts: 1}},
	)
	if active := executor.GetActiveTaskCount(); active != 2 {
		t.Fatalf("active tasks before cancellation = %d, want 2", active)
	}

	executor.Cancel()
	<-firstResultCh
	retryResult, ok := <-retryResultCh
	if !ok || retryResult == nil {
		t.Fatal("queued retry task returned no result")
	}
	if !errors.Is(retryResult.Err, context.Canceled) {
		t.Fatalf("queued retry task error = %v, want context.Canceled", retryResult.Err)
	}
	if calls := retryCalls.Load(); calls != 0 {
		t.Fatalf("queued retry task calls after cancellation = %d, want 0", calls)
	}
}
