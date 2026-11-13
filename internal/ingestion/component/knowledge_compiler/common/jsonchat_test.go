package common

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// failChatInvoker returns err on every Chat call and counts invocations.
type failChatInvoker struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *failChatInvoker) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	f.mu.Lock()
	f.calls++
	f.mu.Unlock()
	return nil, f.err
}

// TestGenJSON_RetriesTransientError verifies the retry loop still retries on a
// transient error and that WithRetryDelay controls the (now ExponentialBackoff)
// base delay. With retryMax=3 it must issue 1 initial + 3 retries = 4 calls.
func TestGenJSON_RetriesTransientError(t *testing.T) {
	inv := &failChatInvoker{err: errors.New("connection timeout")}
	ctx := WithRetryDelay(t.Context(), time.Millisecond)
	if _, err := GenJSON(ctx, inv, ChatRequest{}, 3); err == nil {
		t.Fatal("expected error")
	}
	if inv.calls != 4 {
		t.Errorf("calls = %d, want 4 (1 initial + 3 retries)", inv.calls)
	}
}

// TestGenJSON_NoRetryOnPermanentError verifies a permanent (non-transient)
// error escapes the loop immediately with a single call.
func TestGenJSON_NoRetryOnPermanentError(t *testing.T) {
	inv := &failChatInvoker{err: errors.New("API request failed with status 400: bad request")}
	if _, err := GenJSON(t.Context(), inv, ChatRequest{}); err == nil {
		t.Fatal("expected error")
	}
	if inv.calls != 1 {
		t.Errorf("calls = %d, want 1 (no retry on permanent error)", inv.calls)
	}
}
