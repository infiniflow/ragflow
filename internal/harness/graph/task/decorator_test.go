package task

import (
	"context"
	"errors"
	"testing"
	"time"

	"ragflow/internal/harness/graph/types"
)

func TestTaskDecorator(t *testing.T) {
	ctx := context.Background()

	// Test basic task decorator
	fn := func(ctx context.Context, input interface{}) (interface{}, error) {
		return input.(int) + 1, nil
	}

	decorated := Task(fn, WithName("test-task"))
	result, err := decorated(ctx, 5)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 6 {
		t.Errorf("expected 6, got %v", result)
	}
}

func TestTaskDecoratorWithRetry(t *testing.T) {
	ctx := context.Background()

	callCount := 0
	fn := func(ctx context.Context, input interface{}) (interface{}, error) {
		callCount++
		if callCount < 3 {
			return nil, errors.New("temporary error")
		}
		return "success", nil
	}

	policy := &types.RetryPolicy{
		MaxAttempts:     3,
		InitialInterval: 1 * time.Millisecond,
		BackoffFactor:   1.0,
		MaxInterval:     10 * time.Millisecond,
		Jitter:          false,
		RetryOn:         func(err error) bool { return true },
	}

	decorated := Task(fn, WithRetryPolicy(policy))
	result, err := decorated(ctx, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %v", result)
	}
	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}

func TestTaskDecoratorWithRetryExhausted(t *testing.T) {
	ctx := context.Background()

	callCount := 0
	fn := func(ctx context.Context, input interface{}) (interface{}, error) {
		callCount++
		return nil, errors.New("persistent error")
	}

	policy := &types.RetryPolicy{
		MaxAttempts:     2,
		InitialInterval: 1 * time.Millisecond,
		BackoffFactor:   1.0,
		MaxInterval:     10 * time.Millisecond,
		Jitter:          false,
		RetryOn:         func(err error) bool { return true },
	}

	decorated := Task(fn, WithRetryPolicy(policy))
	_, err := decorated(ctx, nil)
	if err == nil {
		t.Error("expected error after retry exhausted")
	}
	if callCount != 2 {
		t.Errorf("expected 2 calls, got %d", callCount)
	}
}

func TestTaskDecoratorWithNonRetryableError(t *testing.T) {
	ctx := context.Background()

	callCount := 0
	fn := func(ctx context.Context, input interface{}) (interface{}, error) {
		callCount++
		return nil, errors.New("non-retryable error")
	}

	policy := &types.RetryPolicy{
		MaxAttempts:     3,
		InitialInterval: 1 * time.Millisecond,
		BackoffFactor:   1.0,
		MaxInterval:     10 * time.Millisecond,
		Jitter:          false,
		RetryOn:         func(err error) bool { return false }, // Never retry
	}

	decorated := Task(fn, WithRetryPolicy(policy))
	_, err := decorated(ctx, nil)
	if err == nil {
		t.Error("expected error")
	}
	if callCount != 1 {
		t.Errorf("expected 1 call (no retries), got %d", callCount)
	}
}

func TestNamed(t *testing.T) {
	ctx := context.Background()

	fn := func(ctx context.Context, input interface{}) (interface{}, error) {
		return input, nil
	}

	decorated := Named("my-task", fn)
	result, err := decorated(ctx, "test")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "test" {
		t.Errorf("expected 'test', got %v", result)
	}
}

func TestRetryable(t *testing.T) {
	ctx := context.Background()

	callCount := 0
	fn := func(ctx context.Context, input interface{}) (interface{}, error) {
		callCount++
		if callCount < 2 {
			return nil, errors.New("temporary error")
		}
		return "success", nil
	}

	decorated := Retryable(fn, 3, 1.0)
	result, err := decorated(ctx, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "success" {
		t.Errorf("expected 'success', got %v", result)
	}
}

func TestWithTimeout(t *testing.T) {
	ctx := context.Background()

	fn := func(ctx context.Context, input interface{}) (interface{}, error) {
		select {
		case <-time.After(500 * time.Millisecond):
			return "done", nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	timeoutFn := WithTimeout(fn, 50*time.Millisecond)
	_, err := timeoutFn(ctx, nil)
	if err == nil {
		t.Error("expected timeout error")
	}
	if err != context.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded, got %v", err)
	}
}

func TestWithTimeoutSuccess(t *testing.T) {
	ctx := context.Background()

	fn := func(ctx context.Context, input interface{}) (interface{}, error) {
		return "done", nil
	}

	timeoutFn := WithTimeout(fn, 100*time.Millisecond)
	result, err := timeoutFn(ctx, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "done" {
		t.Errorf("expected 'done', got %v", result)
	}
}

func TestCompose(t *testing.T) {
	ctx := context.Background()

	fn := func(ctx context.Context, input interface{}) (interface{}, error) {
		return input.(int) + 1, nil
	}

	// Create decorators that double and triple the input
	double := func(f types.NodeFunc) types.NodeFunc {
		return func(ctx context.Context, input interface{}) (interface{}, error) {
			result, err := f(ctx, input)
			if err != nil {
				return nil, err
			}
			return result.(int) * 2, nil
		}
	}

	addTen := func(f types.NodeFunc) types.NodeFunc {
		return func(ctx context.Context, input interface{}) (interface{}, error) {
			result, err := f(ctx, input)
			if err != nil {
				return nil, err
			}
			return result.(int) + 10, nil
		}
	}

	// Compose applies decorators from last to first: double wraps fn, then addTen wraps double.
	// Execution order: base (0+1=1) → double (×2=2) → addTen (+10=12)
	composed := Compose(addTen, double)(fn)
	// (0 + 1) * 2 + 10 = 12
	result, err := composed(ctx, 0)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != 12 {
		t.Errorf("expected 12, got %v", result)
	}
}

func TestTaskContext(t *testing.T) {
	tc := &TaskContext{
		Name:    "test",
		ID:      "123",
		Input:   "input",
		Output:  "output",
		Attempt: 1,
		Start:   time.Now(),
		End:     time.Now().Add(10 * time.Millisecond),
	}

	if tc.Duration() < 10*time.Millisecond {
		t.Error("expected duration >= 10ms")
	}
}

func TestEntrypoint(t *testing.T) {
	ctx := context.Background()

	fn := func(ctx context.Context, input interface{}) (interface{}, error) {
		return "initialized", nil
	}

	entry := NewEntrypoint("start", fn, map[string]interface{}{"key": "value"})
	if entry.Name() != "start" {
		t.Errorf("expected 'start', got %s", entry.Name())
	}

	result, err := entry.Execute(ctx, nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "initialized" {
		t.Errorf("expected 'initialized', got %v", result)
	}
}
