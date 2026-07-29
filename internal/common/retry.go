// Package common — generic retry utility with exponential backoff.
//
// RetryWithBackoff calls fn up to maxRetries+1 times (one initial
// attempt + maxRetries retries), sleeping delay*2^attempt between
// failures (capped at 1 minute). The sleep honours ctx cancellation.
//
// maxRetries <= 0 yields a single attempt (no retries).
// initialDelay <= 0 results in no delay between retries.
//
// Usage:
//
//	err := RetryWithBackoff(ctx, 3, 2*time.Second, func() error {
//	    return someFailableOperation()
//	})

package common

import (
	"context"
	"fmt"
	"time"
)

const (
	// DefaultRetryMax is the default number of retries (3).
	DefaultRetryMax = 3
	// DefaultRetryDelay is the initial backoff delay (2s).
	DefaultRetryDelay = 2 * time.Second
)

// RetryWithBackoff retries fn on error with exponential backoff.
// Returns nil on the first successful attempt. Returns the last
// error wrapped with the retry count when all attempts fail.
//
// An optional shouldRetry predicate lets callers abort on
// non-transient errors: when supplied and it returns false for a
// given error, RetryWithBackoff stops immediately and returns that
// error without further backoff. A nil predicate (or a nil function
// in the slice) retries on every error, preserving the original
// behavior.
func RetryWithBackoff(ctx context.Context, maxRetries int, initialDelay time.Duration, fn func() error, shouldRetry ...func(error) bool) error {
	if maxRetries <= 0 {
		return fn()
	}
	canRetry := func(err error) bool {
		if len(shouldRetry) == 0 || shouldRetry[0] == nil {
			return true
		}
		return shouldRetry[0](err)
	}
	delay := initialDelay
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}
		lastErr = err
		if !canRetry(err) {
			return err
		}
		if attempt == maxRetries {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
		if delay > time.Minute {
			delay = time.Minute
		}
	}
	return fmt.Errorf("failed after %d retries: %w", maxRetries, lastErr)
}
