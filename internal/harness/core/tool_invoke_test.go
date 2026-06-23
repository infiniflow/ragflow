package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// ======================== ToolInvocationContext ========================

func TestToolInvocationContext_Basic(t *testing.T) {
	ictx := &ToolInvocationContext{
		Name:    "test_tool",
		CallID:  "call_123",
		Timeout: 5 * time.Second,
	}
	if ictx.Name != "test_tool" {
		t.Errorf("expected 'test_tool', got %s", ictx.Name)
	}
	if ictx.Timeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", ictx.Timeout)
	}
}

// ======================== ToolWrapperChain ========================

func TestToolWrapperChain_NoMiddleware(t *testing.T) {
	var called bool
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		called = true
		return &schema.ToolResult{Content: "ok"}, nil
	}

	chained := ToolWrapperChain(fn)
	result, err := chained(context.Background(), &ToolInvocationContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "ok" {
		t.Errorf("expected 'ok', got %s", result.Content)
	}
	if !called {
		t.Error("expected fn to be called")
	}
}

func TestToolWrapperChain_MiddlewareOrder(t *testing.T) {
	var order []string

	mw1 := func(next InvokeTool) InvokeTool {
		return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
			order = append(order, "mw1_before")
			result, err := next(ctx, ictx)
			order = append(order, "mw1_after")
			return result, err
		}
	}

	mw2 := func(next InvokeTool) InvokeTool {
		return func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
			order = append(order, "mw2_before")
			result, err := next(ctx, ictx)
			order = append(order, "mw2_after")
			return result, err
		}
	}

	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		order = append(order, "core")
		return &schema.ToolResult{Content: "done"}, nil
	}

	chained := ToolWrapperChain(fn, mw1, mw2)
	_, err := chained(context.Background(), &ToolInvocationContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := []string{"mw1_before", "mw2_before", "core", "mw2_after", "mw1_after"}
	if len(order) != len(expected) {
		t.Fatalf("expected order %v, got %v", expected, order)
	}
	for i := range expected {
		if order[i] != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], order[i])
		}
	}
}

// ======================== Timeout Middleware ========================

func TestNewTimeoutToolMiddleware_NoTimeout(t *testing.T) {
	mw := NewTimeoutToolMiddleware(0)
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		return &schema.ToolResult{Content: "fast"}, nil
	}

	chained := ToolWrapperChain(fn, mw)
	result, err := chained(context.Background(), &ToolInvocationContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "fast" {
		t.Errorf("expected 'fast', got %s", result.Content)
	}
}

func TestNewTimeoutToolMiddleware_ToolExceedsTimeout(t *testing.T) {
	mw := NewTimeoutToolMiddleware(10 * time.Millisecond)
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
		return &schema.ToolResult{Content: "slow"}, nil
	}

	chained := ToolWrapperChain(fn, mw)
	_, err := chained(context.Background(), &ToolInvocationContext{})
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestNewTimeoutToolMiddleware_PerInvocationTimeout(t *testing.T) {
	mw := NewTimeoutToolMiddleware(5 * time.Second) // generous default
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		time.Sleep(1 * time.Millisecond)
		return &schema.ToolResult{Content: "ok"}, nil
	}

	// Per-invocation timeout (shorter) should be used
	ictx := &ToolInvocationContext{Timeout: 100 * time.Millisecond}
	chained := ToolWrapperChain(fn, mw)
	result, err := chained(context.Background(), ictx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "ok" {
		t.Errorf("expected 'ok', got %s", result.Content)
	}
}

// ======================== Retry Middleware ========================

func TestNewRetryToolMiddleware_SuccessFirstTry(t *testing.T) {
	var callCount int32
	mw := NewRetryToolMiddleware(&ToolRetryConfig{MaxAttempts: 3, IsRetryable: func(err error) bool { return true }})
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		atomic.AddInt32(&callCount, 1)
		return &schema.ToolResult{Content: "ok"}, nil
	}

	chained := ToolWrapperChain(fn, mw)
	_, err := chained(context.Background(), &ToolInvocationContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c := atomic.LoadInt32(&callCount); c != 1 {
		t.Errorf("expected 1 call, got %d", c)
	}
}

func TestNewRetryToolMiddleware_RetriesOnFailure(t *testing.T) {
	var callCount int32
	mw := NewRetryToolMiddleware(&ToolRetryConfig{MaxAttempts: 3, IsRetryable: func(err error) bool { return true }})
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		c := atomic.AddInt32(&callCount, 1)
		if c < 3 {
			return nil, errors.New("transient failure")
		}
		return &schema.ToolResult{Content: "ok after retry"}, nil
	}

	chained := ToolWrapperChain(fn, mw)
	result, err := chained(context.Background(), &ToolInvocationContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "ok after retry" {
		t.Errorf("expected 'ok after retry', got %s", result.Content)
	}
	if c := atomic.LoadInt32(&callCount); c != 3 {
		t.Errorf("expected 3 calls, got %d", c)
	}
}

func TestNewRetryToolMiddleware_Exhausted(t *testing.T) {
	var callCount int32
	mw := NewRetryToolMiddleware(&ToolRetryConfig{MaxAttempts: 2, Backoff: time.Millisecond, IsRetryable: func(err error) bool { return true }})
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		atomic.AddInt32(&callCount, 1)
		return nil, errors.New("permanent failure")
	}

	chained := ToolWrapperChain(fn, mw)
	_, err := chained(context.Background(), &ToolInvocationContext{})
	if err == nil {
		t.Fatal("expected retry exhausted error")
	}
	if c := atomic.LoadInt32(&callCount); c != 3 {
		t.Errorf("expected 3 calls (1 initial + 2 retries), got %d", c)
	}
}

func TestNewRetryToolMiddleware_NonRetryableError(t *testing.T) {
	var callCount int32
	mw := NewRetryToolMiddleware(&ToolRetryConfig{MaxAttempts: 3, IsRetryable: func(err error) bool { return false }})
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		atomic.AddInt32(&callCount, 1)
		return nil, errors.New("non-retryable")
	}

	chained := ToolWrapperChain(fn, mw)
	_, err := chained(context.Background(), &ToolInvocationContext{})
	if err == nil {
		t.Fatal("expected error")
	}
	if c := atomic.LoadInt32(&callCount); c != 1 {
		t.Errorf("expected only 1 call, got %d", c)
	}
}

// ======================== Fallback Middleware ========================

func TestNewFallbackToolMiddleware_PrimarySucceeds(t *testing.T) {
	var primaryCalled, fallbackCalled bool
	fb := func(ctx context.Context, args *schema.ToolArgument) (*schema.ToolResult, error) {
		fallbackCalled = true
		return &schema.ToolResult{Content: "fallback"}, nil
	}

	mw := NewFallbackToolMiddleware(fb)
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		primaryCalled = true
		return &schema.ToolResult{Content: "primary"}, nil
	}

	chained := ToolWrapperChain(fn, mw)
	result, err := chained(context.Background(), &ToolInvocationContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "primary" {
		t.Errorf("expected 'primary', got %s", result.Content)
	}
	if !primaryCalled {
		t.Error("expected primary to be called")
	}
	if fallbackCalled {
		t.Error("expected fallback NOT to be called")
	}
}

func TestNewFallbackToolMiddleware_FallbackOnFailure(t *testing.T) {
	var fallbackCalled bool
	fb := func(ctx context.Context, args *schema.ToolArgument) (*schema.ToolResult, error) {
		fallbackCalled = true
		return &schema.ToolResult{Content: "fallback result"}, nil
	}

	mw := NewFallbackToolMiddleware(fb)
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		return nil, errors.New("primary failed")
	}

	chained := ToolWrapperChain(fn, mw)
	result, err := chained(context.Background(), &ToolInvocationContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "fallback result" {
		t.Errorf("expected 'fallback result', got %s", result.Content)
	}
	if !fallbackCalled {
		t.Error("expected fallback to be called")
	}
}

func TestNewFallbackToolMiddleware_NoFallbackConfigured(t *testing.T) {
	mw := NewFallbackToolMiddleware(nil) // no fallback
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		return nil, errors.New("primary failed")
	}

	chained := ToolWrapperChain(fn, mw)
	_, err := chained(context.Background(), &ToolInvocationContext{})
	if err == nil {
		t.Fatal("expected error when primary fails with no fallback")
	}
}

// ======================== Combined Middleware ========================

func TestToolWrapperChain_TimeoutThenRetry(t *testing.T) {
	var callCount int32
	timeoutMw := NewTimeoutToolMiddleware(50 * time.Millisecond)
	retryMw := NewRetryToolMiddleware(&ToolRetryConfig{MaxAttempts: 2, Backoff: time.Millisecond, IsRetryable: func(err error) bool { return true }})

	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		c := atomic.AddInt32(&callCount, 1)
		time.Sleep(10 * time.Millisecond)
		if c <= 2 {
			return nil, errors.New("transient")
		}
		return &schema.ToolResult{Content: "success after retry"}, nil
	}

	chained := ToolWrapperChain(fn, timeoutMw, retryMw)
	result, err := chained(context.Background(), &ToolInvocationContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "success after retry" {
		t.Errorf("expected 'success after retry', got %s", result.Content)
	}
}

// ======================== ToolToInvokeFn / EnhancedToolToInvokeFn ========================

func TestToolToInvokeFn(t *testing.T) {
	tool := newTestTool("echo", "echo tool")
	invokeFn := ToolToInvokeFn(tool)

	ictx := &ToolInvocationContext{
		Name:      "echo",
		CallID:    "call_1",
		Arguments: &schema.ToolArgument{Arguments: `"hello"`},
	}

	result, err := invokeFn(context.Background(), ictx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content == "" {
		t.Error("expected non-empty content")
	}
}

func TestEnhancedToolToInvokeFn(t *testing.T) {
	et := newTestEnhancedTool("enhanced_tool", "enhanced")
	invokeFn := EnhancedToolToInvokeFn(et)

	ictx := &ToolInvocationContext{
		Name:   "enhanced_tool",
		CallID: "call_2",
		Arguments: &schema.ToolArgument{
			Name: "enhanced_tool", Arguments: `{}`, CallID: "call_2",
		},
	}

	result, err := invokeFn(context.Background(), ictx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ToolCallID != "call_2" {
		t.Errorf("expected call_2, got %s", result.ToolCallID)
	}
}

// ======================== Approval Middleware ========================

func TestAutoApprovalMiddleware(t *testing.T) {
	mw := AutoApprovalMiddleware()
	var called bool
	fn := func(ctx context.Context, ictx *ToolInvocationContext) (*schema.ToolResult, error) {
		called = true
		return &schema.ToolResult{Content: "approved"}, nil
	}

	chained := ToolWrapperChain(fn, mw)
	result, err := chained(context.Background(), &ToolInvocationContext{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "approved" {
		t.Errorf("expected 'approved', got %s", result.Content)
	}
	if !called {
		t.Error("expected fn to be called")
	}
}

// ======================== Test Helpers ========================

type simpleTestTool struct {
	name string
	desc string
}

func newTestTool(name, desc string) *simpleTestTool { return &simpleTestTool{name: name, desc: desc} }
func (t *simpleTestTool) Name() string              { return t.name }
func (t *simpleTestTool) Description() string       { return t.desc }
func (t *simpleTestTool) Invoke(ctx context.Context, args string, opts ...ToolOption) (string, error) {
	return "result: " + args, nil
}
func (t *simpleTestTool) Stream(ctx context.Context, args string, opts ...ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{"stream: " + args}), nil
}

type simpleEnhancedTestTool struct {
	name string
	desc string
}

func newTestEnhancedTool(name, desc string) *simpleEnhancedTestTool {
	return &simpleEnhancedTestTool{name: name, desc: desc}
}
func (t *simpleEnhancedTestTool) Name() string        { return t.name }
func (t *simpleEnhancedTestTool) Description() string { return t.desc }
func (t *simpleEnhancedTestTool) Invoke(ctx context.Context, args string, opts ...ToolOption) (string, error) {
	return "plain", nil
}
func (t *simpleEnhancedTestTool) Stream(ctx context.Context, args string, opts ...ToolOption) (*schema.StreamReader[string], error) {
	return schema.StreamReaderFromArray([]string{"plain stream"}), nil
}
func (t *simpleEnhancedTestTool) EnhancedInvoke(ctx context.Context, args *schema.ToolArgument, opts ...ToolOption) (*schema.ToolResult, error) {
	return &schema.ToolResult{
		Name: args.Name, Content: "enhanced: " + args.Arguments,
		ToolCallID: args.CallID,
	}, nil
}
func (t *simpleEnhancedTestTool) EnhancedStream(ctx context.Context, args *schema.ToolArgument, opts ...ToolOption) (*schema.StreamReader[*schema.ToolResult], error) {
	r := &schema.ToolResult{Name: args.Name, Content: "enhanced stream", ToolCallID: args.CallID}
	return schema.StreamReaderFromArray([]*schema.ToolResult{r}), nil
}
