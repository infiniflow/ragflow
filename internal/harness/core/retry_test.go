package core

import (
	"context"
	"errors"
	"testing"

	"ragflow/internal/harness/core/schema"
)

type countingModel struct {
	calls int
	err   error
}

func (m *countingModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: "ok"}, nil
}
func (m *countingModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, _ := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]Message{msg}), nil
}
func (m *countingModel) BindTools(tools []*schema.ToolInfo) error { return nil }

func TestWithModelRetry_NilConfig(t *testing.T) {
	model := &countingModel{}
	wrapped := WithModelRetry(model, nil)
	if wrapped != model {
		t.Error("nil config should return original model")
	}
}

func TestWithModelRetry_ZeroMaxRetries(t *testing.T) {
	model := &countingModel{}
	cfg := &ModelRetryConfig{MaxRetries: 0}
	wrapped := WithModelRetry(model, cfg)
	if wrapped != model {
		t.Error("zero max retries should return original model")
	}
}

func TestWithModelRetry_SuccessFirstTry(t *testing.T) {
	model := &countingModel{}
	cfg := &ModelRetryConfig{MaxRetries: 3}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	msgs := []Message{schema.UserMessage("hi")}
	resp, err := wrapped.Generate(ctx, msgs)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %s", resp.Content)
	}
	if model.calls != 1 {
		t.Errorf("expected 1 call, got %d", model.calls)
	}
}

func TestWithModelRetry_RetriesOnFailure(t *testing.T) {
	callCount := 0
	model := &failingModel{failTimes: 2, callCount: &callCount}
	cfg := &ModelRetryConfig{MaxRetries: 5, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	msgs := []Message{schema.UserMessage("retry me")}
	_, err := wrapped.Generate(ctx, msgs)
	if err != nil {
		t.Fatalf("Generate after retries: %v", err)
	}
	if callCount < 2 {
		t.Errorf("expected >= 2 calls, got %d", callCount)
	}
}

func TestWithModelRetry_Exhausted(t *testing.T) {
	model := &alwaysFailModel{}
	cfg := &ModelRetryConfig{MaxRetries: 2, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	_, err := wrapped.Generate(ctx, []Message{schema.UserMessage("")})
	if err == nil {
		t.Error("expected error after exhausting retries")
	}
	var rErr *RetryExhaustedError
	if !errors.As(err, &rErr) {
		t.Errorf("expected RetryExhaustedError, got %T", err)
	}
}

func TestRetryExhaustedError_Unwrap(t *testing.T) {
	e := &RetryExhaustedError{LastErr: errors.New("boom"), TotalRetries: 3}
	if errors.Unwrap(e) != ErrExceedMaxRetries {
		t.Error("Unwrap should return ErrExceedMaxRetries")
	}
	if e.Error() == "" {
		t.Error("non-empty Error() expected")
	}
}

func TestWillRetryError(t *testing.T) {
	e := &WillRetryError{ErrStr: "retrying", RetryAttempt: 2}
	if e.Error() != "retrying" {
		t.Error("wrong message")
	}
	if e.RejectReason() != nil {
		t.Error("RejectReason should be nil by default")
	}
}

type failingModel struct {
	failTimes int
	callCount *int
}

func (m *failingModel) Generate(_ context.Context, _ []Message, _ ...modelOption) (Message, error) {
	*m.callCount++
	if *m.callCount <= m.failTimes {
		return nil, errors.New("transient failure")
	}
	return &schema.Message{Content: "success"}, nil
}
func (m *failingModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]Message{msg}), err
}
func (m *failingModel) BindTools(tools []*schema.ToolInfo) error { return nil }

type alwaysFailModel struct{}

func (m *alwaysFailModel) Generate(_ context.Context, _ []Message, _ ...modelOption) (Message, error) {
	return nil, errors.New("permanent failure")
}
func (m *alwaysFailModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	_, err := m.Generate(ctx, msgs, opts...)
	return schema.StreamReaderFromArray([]Message{}), err
}
func (m *alwaysFailModel) BindTools(tools []*schema.ToolInfo) error { return nil }
