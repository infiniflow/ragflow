package common

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

type failChat struct{ calls *atomic.Int32 }

func (c *failChat) Chat(_ context.Context, _ ChatRequest) (*ChatResponse, error) {
	c.calls.Add(1)
	// "connection reset by peer" carries no HTTP status, so
	// common.IsTransientError treats it as transient and GenJSON retries.
	return nil, errors.New("connection reset by peer")
}

// TestGenJSON_RetriesTransientErrorThenFails is a regression guard for the
// GenJSON retry loop after its delay computation was switched to the shared
// common.ExponentialBackoff primitive. The retry count must be unchanged:
// maxRetries=2 yields 1 initial + 2 retries = 3 calls before exhaustion.
func TestGenJSON_RetriesTransientErrorThenFails(t *testing.T) {
	var calls atomic.Int32
	chat := &failChat{calls: &calls}
	ctx := WithRetryDelay(context.Background(), time.Millisecond)
	if _, err := GenJSON(ctx, chat, ChatRequest{}, 2); err == nil {
		t.Fatal("expected error after exhausting retries")
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("expected 3 calls (1 + 2 retries), got %d", got)
	}
}
