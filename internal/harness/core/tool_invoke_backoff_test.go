package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// TestNewRetryToolMiddleware_BackoffBounded guards the unbounded-backoff
// regression: with a high retry budget the middleware must keep every retry
// delay at or below 1 minute. The legacy `backoff *= 2` had no ceiling and
// would grow without bound under a large MaxAttempts.
func TestNewRetryToolMiddleware_BackoffBounded(t *testing.T) {
	prev := toolRetrySleep
	defer func() { toolRetrySleep = prev }()

	var maxDelay atomic.Int64
	toolRetrySleep = func(d time.Duration) <-chan time.Time {
		for {
			cur := maxDelay.Load()
			if d <= time.Duration(cur) {
				break
			}
			if maxDelay.CompareAndSwap(cur, int64(d)) {
				break
			}
		}
		ch := make(chan time.Time)
		close(ch) // ready immediately so the select never blocks
		return ch
	}

	mw := NewRetryToolMiddleware(&ToolRetryConfig{
		MaxAttempts: 200,
		Backoff:     100 * time.Millisecond,
		IsRetryable: func(error) bool { return true },
	})
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		return nil, errors.New("always fails")
	}
	chained := ToolWrapperChain(fn, mw)
	if _, err := chained(t.Context(), &ToolInvocationContext{}); err == nil {
		t.Fatal("expected retry-exhausted error")
	}
	if d := time.Duration(maxDelay.Load()); d > time.Minute {
		t.Fatalf("retry delay %s exceeded 1-minute cap (unbounded backoff bug)", d)
	}
}
