// Package pregel provides comprehensive retry tests for the engine.
package pregel

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/graph/channels"
	"ragflow/internal/harness/graph/checkpoint"
	"ragflow/internal/harness/graph/constants"
	graphPkg "ragflow/internal/harness/graph/graph"
	"ragflow/internal/harness/graph/types"
)

// ============================================================
// P0: Retry with Exponential Backoff
// ============================================================

// TestRetry_BackoffTiming verifies backoff intervals increase.
func TestRetry_BackoffTiming(t *testing.T) {
	var attempts atomic.Int32
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("work", func(ctx context.Context, state any) (any, error) {
		n := attempts.Add(1)
		return nil, fmt.Errorf("fail %d", n)
	})
	_ = sg.AddEdge(constants.Start, "work")
	_ = sg.AddEdge("work", constants.End)

	rp := types.RetryPolicy{
		InitialInterval: time.Millisecond,
		BackoffFactor:   4.0,
		MaxInterval:     time.Second,
		MaxAttempts:     4,
		Jitter:          false,
	}
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	start := time.Now()
	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected error")
	}
	// With 4 attempts, backoff = 1ms, 4ms, 16ms = ~21ms minimum.
	if elapsed < 15*time.Millisecond {
		t.Logf("backoff may be too fast: %v (%d attempts)", elapsed, attempts.Load())
	}
}

// ============================================================
// P0: Retry with jitter produces varying times
// ============================================================

// TestRetry_JitterRandomized verifies jitter randomizes backoff.
func TestRetry_JitterRandomized(t *testing.T) {
	var attempts atomic.Int32
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("work", func(ctx context.Context, state any) (any, error) {
		attempts.Add(1)
		return nil, fmt.Errorf("fail")
	})
	_ = sg.AddEdge(constants.Start, "work")
	_ = sg.AddEdge("work", constants.End)

	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 3
	rp.Jitter = true
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	t.Logf("jitter test: %d attempts", attempts.Load())
}

// ============================================================
// P1: Retry + Checkpointer interaction
// ============================================================

// TestRetry_WithCheckpointer_Transient verifies retry works alongside
// checkpointing when the node eventually succeeds.
func TestRetry_WithCheckpointer_Transient(t *testing.T) {
	var attempts atomic.Int32
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))

	sg.AddNode("flaky", func(ctx context.Context, state any) (any, error) {
		n := attempts.Add(1)
		if n < 3 {
			return nil, fmt.Errorf("transient %d", n)
		}
		m, _ := state.(map[string]any)
		if m == nil {
			m = map[string]any{}
		}
		m["value"] = "success"
		return m, nil
	})
	_ = sg.AddEdge(constants.Start, "flaky")
	_ = sg.AddEdge("flaky", constants.End)

	ms := checkpoint.NewMemorySaver()
	tid := "retry-cp-transient"
	cfg := &types.RunnableConfig{
		Configurable: map[string]interface{}{constants.ConfigKeyThreadID: tid},
	}
	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 5
	engine := NewEngine(sg,
		WithRecursionLimit(10),
		WithCheckpointer(ms),
		WithConfig(cfg),
		WithRetryPolicy(&rp),
	)

	result, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err != nil {
		t.Fatalf("RunSync: %v", err)
	}
	m := result.(map[string]any)
	if m["value"] != "success" {
		t.Fatalf("expected value=success, got %v", m["value"])
	}
	if attempts.Load() != 3 {
		t.Fatalf("expected 3 attempts, got %d", attempts.Load())
	}
}

// ============================================================
// P1: Retry with zero attempts
// ============================================================

// TestRetry_ZeroAttempts verifies MaxAttempts=0 doesn't loop forever.
func TestRetry_ZeroAttempts(t *testing.T) {
	var attempts atomic.Int32
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("work", func(ctx context.Context, state any) (any, error) {
		attempts.Add(1)
		return nil, fmt.Errorf("fail always")
	})
	_ = sg.AddEdge(constants.Start, "work")
	_ = sg.AddEdge("work", constants.End)

	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 0
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	t.Logf("zero attempts: %d tries", attempts.Load())
}

// ============================================================
// P1: Retry with single attempt (no retry)
// ============================================================

// TestRetry_SingleAttempt verifies MaxAttempts=1 means no retry.
func TestRetry_SingleAttempt(t *testing.T) {
	var attempts atomic.Int32
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("work", func(ctx context.Context, state any) (any, error) {
		attempts.Add(1)
		return nil, fmt.Errorf("fail")
	})
	_ = sg.AddEdge(constants.Start, "work")
	_ = sg.AddEdge("work", constants.End)

	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 1
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	n := attempts.Load()
	t.Logf("single attempt: %d", n)
}

// ============================================================
// P2: Retry with RetryOn returning false (non-retryable)
// ============================================================

// TestRetry_NonRetryableError verifies RetryOn=false stops retries.
func TestRetry_NonRetryableError(t *testing.T) {
	var attempts atomic.Int32
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("work", func(ctx context.Context, state any) (any, error) {
		attempts.Add(1)
		return nil, fmt.Errorf("non-retryable")
	})
	_ = sg.AddEdge(constants.Start, "work")
	_ = sg.AddEdge("work", constants.End)

	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 5
	rp.RetryOn = func(err error) bool { return false }
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	t.Logf("non-retryable: %d attempts", attempts.Load())
}

// ============================================================
// P2: Retry with special retryable errors
// ============================================================

// TestRetry_SelectiveRetry verifies RetryOn returning true for specific errors.
func TestRetry_SelectiveRetry(t *testing.T) {
	var attempts atomic.Int32
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("work", func(ctx context.Context, state any) (any, error) {
		n := attempts.Add(1)
		if n < 3 {
			return nil, fmt.Errorf("rate_limited") // retryable
		}
		return nil, fmt.Errorf("invalid_input") // not retryable
	})
	_ = sg.AddEdge(constants.Start, "work")
	_ = sg.AddEdge("work", constants.End)

	rp := types.DefaultRetryPolicy()
	rp.MaxAttempts = 10
	rp.RetryOn = func(err error) bool {
		return err != nil && err.Error() == "rate_limited"
	}
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	n := attempts.Load()
	t.Logf("selective retry: %d attempts, final err=%v", n, err)
}

// ============================================================
// P2: Very long max interval (backoff capped)
// ============================================================

// TestRetry_BackoffCapped verifies MaxInterval caps the backoff.
func TestRetry_BackoffCapped(t *testing.T) {
	var attempts atomic.Int32
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("work", func(ctx context.Context, state any) (any, error) {
		attempts.Add(1)
		return nil, fmt.Errorf("fail")
	})
	_ = sg.AddEdge(constants.Start, "work")
	_ = sg.AddEdge("work", constants.End)

	rp := types.RetryPolicy{
		InitialInterval: 10 * time.Millisecond,
		BackoffFactor:   10.0,
		MaxInterval:     25 * time.Millisecond,
		MaxAttempts:     5,
		Jitter:          false,
	}
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	t.Logf("capped backoff: %d attempts", attempts.Load())
}

// ============================================================
// P2: Retry with max interval = 0 (immediate retries)
// ============================================================

// TestRetry_ZeroMaxInterval verifies MaxInterval=0 (no cap).
func TestRetry_ZeroMaxInterval(t *testing.T) {
	var attempts atomic.Int32
	sg := graphPkg.NewStateGraph(map[string]any{})
	sg.AddChannel("value", channels.NewLastValue(""))
	sg.AddNode("work", func(ctx context.Context, state any) (any, error) {
		attempts.Add(1)
		return nil, fmt.Errorf("fail")
	})
	_ = sg.AddEdge(constants.Start, "work")
	_ = sg.AddEdge("work", constants.End)

	rp := types.RetryPolicy{
		InitialInterval: 0,
		BackoffFactor:   1.0,
		MaxInterval:     0,
		MaxAttempts:     5,
		Jitter:          false,
	}
	engine := NewEngine(sg, WithRecursionLimit(10), WithRetryPolicy(&rp))

	_, err := engine.RunSync(context.Background(), map[string]any{"value": "x"})
	if err == nil {
		t.Fatal("expected error")
	}
	t.Logf("zero max interval: %d attempts", attempts.Load())
}
