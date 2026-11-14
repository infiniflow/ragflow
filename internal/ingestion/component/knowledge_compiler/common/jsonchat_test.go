package common

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"ragflow/internal/agent/runtime"
)

// failChatInvoker returns err on every Chat call and counts invocations.
type failChatInvoker struct {
	mu    sync.Mutex
	calls int
	err   error
}

// Chat records an invocation and returns the configured error.
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

// TestGenJSON_RetryProgressReportsBackoffDelay verifies that retry failure
// progress messages report the calculated exponential backoff delay rather than
// repeating the static base delay.
func TestGenJSON_RetryProgressReportsBackoffDelay(t *testing.T) {
	inv := &failChatInvoker{err: errors.New("connection timeout")}
	baseDelay := 10 * time.Millisecond
	ctx := WithRetryDelay(t.Context(), baseDelay)

	var messages []string
	var mu sync.Mutex
	ctx = runtime.WithProgressMessageCallback(ctx, func(component, msg string) {
		mu.Lock()
		messages = append(messages, msg)
		mu.Unlock()
	})

	// With retryMax=4 (5 attempts total: 1 initial + 4 retries),
	// attempt 1 (first failure) reports retrying in 10ms.
	// attempt 4 (3 repeated failures) reports retrying in 80ms (10ms * 2^3).
	if _, err := GenJSON(ctx, inv, ChatRequest{}, 4); err == nil {
		t.Fatal("expected error")
	}

	mu.Lock()
	defer mu.Unlock()

	if len(messages) < 2 {
		t.Fatalf("expected at least 2 progress messages, got %d: %v", len(messages), messages)
	}

	// First failure message should report base delay (10ms).
	if !strings.Contains(messages[0], "retrying in 10ms") {
		t.Errorf("first message %q does not contain 'retrying in 10ms'", messages[0])
	}

	// The 4th attempt (coalesced 3 repeated failures) should report 80ms (10ms * 8), NOT 10ms.
	var foundFourthAttempt bool
	for _, m := range messages {
		if strings.Contains(m, "3 repeated failures") {
			foundFourthAttempt = true
			if !strings.Contains(m, "retrying in 80ms") {
				t.Errorf("repeated failure message %q does not report backoff delay (wanted 'retrying in 80ms')", m)
			}
			if strings.Contains(m, "retrying in 10ms") {
				t.Errorf("repeated failure message %q erroneously reports static base delay 10ms", m)
			}
		}
	}
	if !foundFourthAttempt {
		t.Errorf("did not find coalesced repeated failure progress message in %v", messages)
	}
}
