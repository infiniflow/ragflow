package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// TestNewRetryToolMiddleware_ZeroBackoffKeepsBackingOff is the adversarial
// regression test for the backoff-base bug: when ToolRetryConfig.Backoff is
// unset (0) -- the documented default usage in the harness README -- the local
// var is normalized to 100ms for the first sleep, but the post-sleep recompute
// passed the RAW rc.Backoff (0) to common.ExponentialBackoff, collapsing every
// later retry delay to 0. This test asserts the delay sequence keeps doubling
// from the normalized 100ms base (100ms, 200ms, 400ms, ...) with no zero gap.
func TestNewRetryToolMiddleware_ZeroBackoffKeepsBackingOff(t *testing.T) {
	var gaps []time.Duration
	orig := toolRetrySleep
	toolRetrySleep = func(_ context.Context, d time.Duration) { gaps = append(gaps, d) }
	defer func() { toolRetrySleep = orig }()

	const maxAttempts = 5
	mw := NewRetryToolMiddleware(&ToolRetryConfig{
		MaxAttempts: maxAttempts,
		Backoff:     0,
		IsRetryable: func(err error) bool { return true },
	})
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		return nil, errors.New("always fails")
	}
	chained := ToolWrapperChain(fn, mw)
	if _, err := chained(t.Context(), &ToolInvocationContext{}); err == nil {
		t.Fatal("expected exhausted error")
	}

	if len(gaps) != maxAttempts {
		t.Fatalf("gaps = %d, want %d", len(gaps), maxAttempts)
	}
	want := []time.Duration{
		100 * time.Millisecond,
		200 * time.Millisecond,
		400 * time.Millisecond,
		800 * time.Millisecond,
		1600 * time.Millisecond,
	}
	for i := range want {
		if gaps[i] != want[i] {
			t.Errorf("gap[%d] = %v, want %v (zero-base backoff collapse)", i, gaps[i], want[i])
		}
	}
}

// TestNewRetryToolMiddleware_FirstSleepCapped is the adversarial regression test
// for the first-sleep-cap bug: the first retry sleep used the raw configured
// Backoff without passing it through the 1-minute cap, so a Backoff above 1
// minute produced an uncapped first gap even though later gaps were capped.
func TestNewRetryToolMiddleware_FirstSleepCapped(t *testing.T) {
	var gaps []time.Duration
	orig := toolRetrySleep
	toolRetrySleep = func(_ context.Context, d time.Duration) { gaps = append(gaps, d) }
	defer func() { toolRetrySleep = orig }()

	const maxAttempts = 3
	mw := NewRetryToolMiddleware(&ToolRetryConfig{
		MaxAttempts: maxAttempts,
		Backoff:     2 * time.Minute,
		IsRetryable: func(err error) bool { return true },
	})
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		return nil, errors.New("always fails")
	}
	chained := ToolWrapperChain(fn, mw)
	if _, err := chained(t.Context(), &ToolInvocationContext{}); err == nil {
		t.Fatal("expected exhausted error")
	}

	if len(gaps) != maxAttempts {
		t.Fatalf("gaps = %d, want %d", len(gaps), maxAttempts)
	}
	for i, g := range gaps {
		if g > time.Minute {
			t.Errorf("gap[%d] = %v > 1min (first sleep not capped)", i, g)
		}
	}
	if gaps[0] != time.Minute {
		t.Errorf("first gap = %v, want 1min (cap not applied to first retry)", gaps[0])
	}
}
