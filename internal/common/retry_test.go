package common

import (
	"context"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRetryWithBackoff_SuccessOnFirstAttempt(t *testing.T) {
	var calls atomic.Int32
	err := RetryWithBackoff(t.Context(), 3, time.Millisecond, func() error {
		calls.Add(1)
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1", calls.Load())
	}
}

func TestRetryWithBackoff_SuccessOnThirdAttempt(t *testing.T) {
	var calls atomic.Int32
	err := RetryWithBackoff(t.Context(), 3, time.Millisecond, func() error {
		if calls.Add(1) < 3 {
			return errors.New("transient")
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3", calls.Load())
	}
}

func TestRetryWithBackoff_AllRetriesExhausted(t *testing.T) {
	sentinel := errors.New("persistent failure")
	err := RetryWithBackoff(t.Context(), 3, time.Millisecond, func() error {
		return sentinel
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, sentinel) {
		t.Errorf("error should wrap sentinel, got %v", err)
	}
	if !strings.Contains(err.Error(), "after 3 retries") {
		t.Errorf("error should mention retry count: %v", err)
	}
}

func TestRetryWithBackoff_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	var calls atomic.Int32
	errChan := make(chan error, 1)
	go func() {
		errChan <- RetryWithBackoff(ctx, 3, 10*time.Second, func() error {
			calls.Add(1)
			cancel() // cancel before retry sleep
			return errors.New("fail")
		})
	}()
	err := <-errChan
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1 (no retry after cancel)", calls.Load())
	}
}

func TestRetryWithBackoff_ContextDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	var calls atomic.Int32
	err := RetryWithBackoff(ctx, 3, time.Second, func() error {
		calls.Add(1)
		return errors.New("fail")
	})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
	// Should have made at least 1 call; ctx deadline stops further retries
	if calls.Load() < 1 {
		t.Errorf("expected at least 1 call, got %d", calls.Load())
	}
}

func TestRetryWithBackoff_ZeroMaxRetries(t *testing.T) {
	var calls atomic.Int32
	err := RetryWithBackoff(t.Context(), 0, 0, func() error {
		calls.Add(1)
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1 (single attempt)", calls.Load())
	}
}

func TestRetryWithBackoff_NegativeMaxRetries(t *testing.T) {
	var calls atomic.Int32
	err := RetryWithBackoff(t.Context(), -1, 0, func() error {
		calls.Add(1)
		return errors.New("fail")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1 (negative → single attempt)", calls.Load())
	}
}

// TestRetryWithBackoff_NonRetryableStops verifies that when a
// shouldRetry predicate returns false, RetryWithBackoff aborts
// immediately instead of burning the remaining retries/backoff.
func TestRetryWithBackoff_NonRetryableStops(t *testing.T) {
	var calls atomic.Int32
	sentinel := errors.New("auth failure")
	err := RetryWithBackoff(t.Context(), 3, 10*time.Second, func() error {
		calls.Add(1)
		return sentinel
	}, func(err error) bool {
		return !errors.Is(err, sentinel)
	})
	if !errors.Is(err, sentinel) {
		t.Errorf("expected sentinel, got %v", err)
	}
	if calls.Load() != 1 {
		t.Errorf("calls = %d, want 1 (non-retryable → single attempt)", calls.Load())
	}
}

// TestRetryWithBackoff_NilPredicateRetriesAll confirms that omitting
// the predicate preserves the original blind-retry behavior.
func TestRetryWithBackoff_NilPredicateRetriesAll(t *testing.T) {
	var calls atomic.Int32
	err := RetryWithBackoff(t.Context(), 2, time.Millisecond, func() error {
		calls.Add(1)
		return errors.New("transient")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls.Load() != 3 {
		t.Errorf("calls = %d, want 3 (all retries attempted)", calls.Load())
	}
}
