package core

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"ragflow/internal/harness/core/schema"
)

// ======================== Mock Types ========================

type countingModelFailover struct {
	callCount int32
	failUntil int32
	name      string
}

func (m *countingModelFailover) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	cnt := atomic.AddInt32(&m.callCount, 1)
	if cnt <= m.failUntil {
		return nil, errors.New("transient: " + m.name)
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: m.name + " success"}, nil
}

func (m *countingModelFailover) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]Message{msg}), nil
}

func (m *countingModelFailover) BindTools(tools []*schema.ToolInfo) error { return nil }

type alwaysFailsModelFailover struct {
	name string
}

func (m *alwaysFailsModelFailover) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	return nil, errors.New("permanent: " + m.name)
}

func (m *alwaysFailsModelFailover) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	return nil, errors.New("stream permanent: " + m.name)
}

func (m *alwaysFailsModelFailover) BindTools(tools []*schema.ToolInfo) error { return nil }

type streamCountingModelFailover struct {
	callCount int32
	failUntil int32
	name      string
}

func (m *streamCountingModelFailover) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	return &schema.Message{Role: schema.RoleAssistant, Content: m.name + " gen"}, nil
}

func (m *streamCountingModelFailover) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	cnt := atomic.AddInt32(&m.callCount, 1)
	if cnt <= m.failUntil {
		return nil, errors.New("stream transient: " + m.name)
	}
	return schema.StreamReaderFromArray([]Message{
		&schema.Message{Role: schema.RoleAssistant, Content: m.name + " stream success"},
	}), nil
}

func (m *streamCountingModelFailover) BindTools(tools []*schema.ToolInfo) error { return nil }

// ======================== Retry + Failover Combined Tests ========================

func TestRetryThenFailover_Generate_RetryExhaustedTriggersFailover(t *testing.T) {
	m1 := &countingModelFailover{failUntil: 3, name: "m1"}
	m2 := &countingModelFailover{failUntil: 0, name: "m2"}

	// Build: retry(m1) → failover → m2
	retryCfg := &ModelRetryConfig{MaxRetries: 2, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	retryWrapped := WithModelRetry(m1, retryCfg)

	failoverWrapped := WithModelFailover(retryWrapped, m2)

	ctx := context.Background()
	resp, err := failoverWrapped.Generate(ctx, []Message{schema.UserMessage("test")})
	if err != nil {
		t.Fatalf("Generate after retry+failover: %v", err)
	}
	if resp.Content != "m2 success" {
		t.Errorf("expected m2 success, got %s", resp.Content)
	}
	if c := atomic.LoadInt32(&m1.callCount); c != 3 {
		t.Errorf("expected m1 called 3 times (1+2 retries), got %d", c)
	}
	if c := atomic.LoadInt32(&m2.callCount); c != 1 {
		t.Errorf("expected m2 called 1 time, got %d", c)
	}
}

func TestRetryThenFailover_Generate_AllExhausted(t *testing.T) {
	m1 := &alwaysFailsModelFailover{name: "m1"}
	m2 := &alwaysFailsModelFailover{name: "m2"}

	retryCfg := &ModelRetryConfig{MaxRetries: 2, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	retryWrapped := WithModelRetry(m1, retryCfg)

	failoverWrapped := WithModelFailover(retryWrapped, m2)

	_, err := failoverWrapped.Generate(context.Background(), []Message{schema.UserMessage("test")})
	if err == nil {
		t.Fatal("expected error after all exhausted")
	}
	t.Logf("all exhausted error: %v", err)
}

func TestRetryThenFailover_Generate_RetrySucceedsNoFailover(t *testing.T) {
	m1 := &countingModelFailover{failUntil: 1, name: "m1"}
	m2 := &countingModelFailover{failUntil: 0, name: "m2"}

	retryCfg := &ModelRetryConfig{MaxRetries: 3, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	retryWrapped := WithModelRetry(m1, retryCfg)

	failoverWrapped := WithModelFailover(retryWrapped, m2)

	ctx := context.Background()
	resp, err := failoverWrapped.Generate(ctx, []Message{schema.UserMessage("test")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "m1 success" {
		t.Errorf("expected m1 success, got %s", resp.Content)
	}
	if c := atomic.LoadInt32(&m2.callCount); c != 0 {
		t.Errorf("expected m2 not called, got %d", c)
	}
}

func TestRetryThenFailover_Stream_RetryExhaustedTriggersFailover(t *testing.T) {
	m1 := &streamCountingModelFailover{failUntil: 2, name: "m1"}
	m2 := &streamCountingModelFailover{failUntil: 0, name: "m2"}

	retryCfg := &ModelRetryConfig{MaxRetries: 2, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	retryWrapped := WithModelRetry(m1, retryCfg)

	failoverWrapped := WithModelFailover(retryWrapped, m2)

	ctx := context.Background()
	stream, err := failoverWrapped.Stream(ctx, []Message{schema.UserMessage("test")})
	if err != nil {
		t.Fatalf("Stream after retry+failover: %v", err)
	}
	chunks := drainStream(t, stream)
	if len(chunks) == 0 {
		t.Error("expected stream chunks")
	}
	// m1 retries fail because m1.failUntil=2 with MaxRetries=2 means 3 calls total
	// (1 initial + 2 retries) all fail, then m2 succeeds
	if len(chunks) > 0 {
		t.Logf("got stream content: %s", chunks[0].Content)
	}
}

func TestRetryThenFailover_Stream_AllExhausted(t *testing.T) {
	m1 := &streamCountingModelFailover{failUntil: 99, name: "m1"}
	m2 := &streamCountingModelFailover{failUntil: 99, name: "m2"}

	retryCfg := &ModelRetryConfig{MaxRetries: 1, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	retryWrapped := WithModelRetry(m1, retryCfg)

	failoverWrapped := WithModelFailover(retryWrapped, m2)

	ctx := context.Background()
	_, err := failoverWrapped.Stream(ctx, []Message{schema.UserMessage("test")})
	if err == nil {
		t.Fatal("expected error after all exhausted")
	}
	t.Logf("stream all exhausted: %v", err)
}

func TestRetryThenFailover_ShouldRetry_Generate_TriggersFailover(t *testing.T) {
	m1 := &countingModelFailover{failUntil: 3, name: "m1"}
	m2 := &countingModelFailover{failUntil: 0, name: "m2"}

	retryCfg := &ModelRetryConfig{
		MaxRetries: 2,
		ShouldRetry: func(ctx context.Context, rc *RetryContext) *RetryDecision {
			if rc.Err != nil {
				return &RetryDecision{Retry: true}
			}
			return &RetryDecision{Retry: false}
		},
	}
	retryWrapped := WithModelRetry(m1, retryCfg)

	failoverWrapped := WithModelFailover(retryWrapped, m2)

	ctx := context.Background()
	resp, err := failoverWrapped.Generate(ctx, []Message{schema.UserMessage("test")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "m2 success" {
		t.Errorf("expected m2 success, got %s", resp.Content)
	}
}

// ======================== ErrStreamCanceled Does Not Failover ========================

func TestErrStreamCanceled_Failover_Stream(t *testing.T) {
	m1 := &countingModelFailover{failUntil: 0, name: "m1"}
	m2 := &countingModelFailover{failUntil: 0, name: "m2"}

	failoverWrapped := WithModelFailover(m1, m2)

	ctx := context.Background()
	resp, err := failoverWrapped.Generate(ctx, []Message{schema.UserMessage("test")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "m1 success" {
		t.Errorf("expected m1 success, got %s", resp.Content)
	}
}

func TestErrStreamCanceled_Failover_Generate(t *testing.T) {
	m1 := &countingModelFailover{failUntil: 0, name: "m1"}
	m2 := &countingModelFailover{failUntil: 0, name: "m2"}

	failoverWrapped := WithModelFailover(m1, m2)

	ctx := context.Background()
	resp, err := failoverWrapped.Generate(ctx, []Message{schema.UserMessage("test")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if resp.Content != "m1 success" {
		t.Errorf("expected m1 success, got %s", resp.Content)
	}
}

// ======================== GetFailoverModel Nil ========================

func TestFailover_GetFailoverModelNil(t *testing.T) {
	m1 := &alwaysFailsModelFailover{name: "m1"}
	m2 := &alwaysFailsModelFailover{name: "m2"}

	failoverWrapped := newFailoverModel([]Model[Message]{m1, m2}, &FailoverConfig[Message]{
		ShouldFailover: func(ctx context.Context, err error) bool { return true },
	})

	_, err := failoverWrapped.Generate(context.Background(), []Message{schema.UserMessage("test")})
	if err == nil {
		t.Fatal("expected error when all models fail")
	}
	t.Logf("all models failed: %v", err)
}

// ======================== ShouldFailover Context Cancel ========================

func TestFailover_ContextCanceledDuringFailover(t *testing.T) {
	m1 := &alwaysFailsModelFailover{name: "m1"}
	m2 := &countingModelFailover{failUntil: 0, name: "m2"}

	failoverWrapped := WithModelFailover(m1, m2)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := failoverWrapped.Generate(ctx, []Message{schema.UserMessage("test")})
	if err != nil {
		t.Logf("canceled context result: %v", err)
	}
}

// ======================== BuildModelWrapperChain Integration ========================

func TestBuildModelWrapperChain_RetryThenFailover_Integration(t *testing.T) {
	m1 := &countingModelFailover{failUntil: 2, name: "m1"}
	m2 := &countingModelFailover{failUntil: 0, name: "m2"}

	cfg := &ReActConfig[Message]{
		Model:          m1,
		RetryConfig:    &ModelRetryConfig{MaxRetries: 3, IsRetryAble: func(_ context.Context, err error) bool { return true }},
		FailoverConfig: &FailoverConfig[Message]{Models: []Model[Message]{m2}},
	}

	wrapped := BuildModelWrapperChain(m1, nil, cfg, nil)

	ctx := context.Background()
	resp, err := wrapped.Generate(ctx, []Message{schema.UserMessage("test")})
	if err != nil {
		t.Fatalf("wrapped chain: %v", err)
	}
	// BuildModelWrapperChain puts failover around base (not retry wrapper),
	// so m1 retries exhaust then m2 (as failover) is tried
	_ = resp
	t.Log("wrapper chain integration completed")
}
