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

// TestExponentialBackoff locks the shared capped exponential backoff primitive
// used by the retry loops (tool_invoke, jsonchat, http_helper). The contract:
//   - result is base * 2^(attempt-1), 1-indexed, so attempt 1 == base;
//   - result never exceeds max, even at very large attempt counts (this is the
//     property that fixes the tool_invoke unbounded-backoff bug);
//   - attempt < 1 is clamped to 1;
//   - when base already exceeds max, the result is max;
//   - max == 0 (or base == 0) yields 0 rather than a negative/garbage value.
func TestExponentialBackoff(t *testing.T) {
	const base = 100 * time.Millisecond
	const max = time.Minute

	cases := []struct {
		name    string
		base    time.Duration
		max     time.Duration
		attempt int
		want    time.Duration
	}{
		{"attempt1", base, max, 1, 100 * time.Millisecond},
		{"attempt2", base, max, 2, 200 * time.Millisecond},
		{"attempt3", base, max, 3, 400 * time.Millisecond},
		{"attempt4", base, max, 4, 800 * time.Millisecond},
		{"attempt10", base, max, 10, 100 * time.Millisecond * 512}, // 51.2s, still under cap
		{"cappedJustAboveMax", base, max, 11, max},                 // 102.4s -> 1min
		{"largeAttemptStaysCapped", base, max, 100, max},           // never grows past cap
		{"attemptZeroClampsTo1", base, max, 0, base},
		{"attemptNegativeClampsTo1", base, max, -5, base},
		{"baseGreaterThanMax", 2 * time.Minute, max, 1, max},
		{"maxZero", base, 0, 5, 0},
		{"baseZero", 0, max, 3, 0},
		// Negative base must behave like a zero base (no delay), not saturate
		// to max. The shared primitive otherwise conflated an overflowing
		// multiply with a negative base and returned the cap, which broke the
		// RetryWithBackoff "initialDelay <= 0 => no delay" contract.
		{"negativeBase", -1 * time.Second, max, 2, 0},
		{"negativeBaseAttempt1", -1 * time.Second, max, 1, 0},
		{"negativeBaseLargeMax", -1 * time.Second, 2 * time.Minute, 5, 0},
		// Overflow guards: a base near the int64 ceiling must clamp to max
		// instead of wrapping to a negative duration. The doubled value can
		// overflow time.Duration (int64 nanoseconds) before the >max check
		// would ever fire, so the primitive must saturate on overflow.
		{"hugeBaseOverflow", 1 << 62, time.Minute, 2, time.Minute},
		{"hugeBaseHugeMax", 1 << 61, 1 << 62, 3, 1 << 62},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ExponentialBackoff(c.base, c.max, c.attempt)
			if got != c.want {
				t.Errorf("ExponentialBackoff(%v, %v, %d) = %v, want %v", c.base, c.max, c.attempt, got, c.want)
			}
			if got > c.max {
				t.Errorf("ExponentialBackoff returned %v > max %v (uncapped backoff bug)", got, c.max)
			}
		})
	}
}

// TestRetryWithBackoff_NegativeDelayNoDelay guards the documented contract
// that a non-positive initialDelay yields no delay between retries. A negative
// DelayAfterError from the agent DSL must keep retrying immediately, never
// stall a full minute per retry (the regression introduced when RetryWithBackoff
// was routed through the shared ExponentialBackoff primitive without
// normalizing the base).
func TestRetryWithBackoff_NegativeDelayNoDelay(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	var calls atomic.Int32
	start := time.Now()
	err := RetryWithBackoff(ctx, 3, -1*time.Second, func() error {
		calls.Add(1)
		return errors.New("always fail")
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if calls.Load() != 4 { // 1 initial attempt + 3 retries
		t.Errorf("calls = %d, want 4", calls.Load())
	}
	// With a negative initialDelay there should be essentially no delay; a
	// regression made every retry after the first sleep a full minute, which
	// this guards against.
	if elapsed > 1*time.Second {
		t.Errorf("negative initialDelay caused %v of delay, want near-zero (no-delay contract)", elapsed)
	}
}
