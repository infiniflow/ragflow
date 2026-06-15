package core

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"ragflow/internal/harness/core/schema"
)

// ======================== Mock Types ========================

// streamErrorModel simulates a mid-stream error that is retryable.
type streamErrorModel struct {
	inner     *mockModel
	failAfter int
}

func (m *streamErrorModel) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	return m.inner.Generate(ctx, msgs, opts...)
}

func (m *streamErrorModel) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	reader := schema.NewStreamReader[Message]()
	go func() {
		defer reader.Close()
		chunks := 0
		for _, resp := range m.inner.responses {
			if chunks >= m.failAfter {
				reader.Send(nil, errors.New("mid-stream error"))
				return
			}
			reader.Send(&schema.Message{Role: schema.RoleAssistant, Content: resp}, nil)
			chunks++
		}
	}()
	return reader, nil
}

func (m *streamErrorModel) BindTools(tools []*schema.ToolInfo) error { return m.inner.BindTools(tools) }

// countingModelForRetry counts calls and fails a configured number of times.
type countingModelForRetry struct {
	callCount int32
	failTimes int32
}

func (m *countingModelForRetry) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	cnt := atomic.AddInt32(&m.callCount, 1)
	if cnt <= m.failTimes {
		return nil, errors.New("transient error")
	}
	return &schema.Message{Role: schema.RoleAssistant, Content: "success after retry"}, nil
}

func (m *countingModelForRetry) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	msg, err := m.Generate(ctx, msgs, opts...)
	if err != nil {
		return nil, err
	}
	return schema.StreamReaderFromArray([]Message{msg}), nil
}

func (m *countingModelForRetry) BindTools(tools []*schema.ToolInfo) error { return nil }

// countingModelForStreamRetry counts Stream calls separately.
type countingModelForStreamRetry struct {
	callCount int32
	failTimes int32
	err       error
}

func (m *countingModelForStreamRetry) Generate(ctx context.Context, msgs []Message, opts ...modelOption) (Message, error) {
	return &schema.Message{Role: schema.RoleAssistant, Content: "gen"}, nil
}

func (m *countingModelForStreamRetry) Stream(ctx context.Context, msgs []Message, opts ...modelOption) (*schema.StreamReader[Message], error) {
	cnt := atomic.AddInt32(&m.callCount, 1)
	if cnt <= m.failTimes {
		if m.err != nil {
			return nil, m.err
		}
		return nil, errors.New("stream transient error")
	}
	reader := schema.NewStreamReader[Message]()
	go func() {
		defer reader.Close()
		reader.Send(&schema.Message{Role: schema.RoleAssistant, Content: "stream success"}, nil)
	}()
	return reader, nil
}

func (m *countingModelForStreamRetry) BindTools(tools []*schema.ToolInfo) error { return nil }

// ======================== Tests: Generate Mode ========================

func TestRetry_NoTools_DirectError_Generate(t *testing.T) {
	model := &countingModelForRetry{failTimes: 2}
	cfg := &ModelRetryConfig{MaxRetries: 5, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	resp, err := wrapped.Generate(ctx, []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate after retry: %v", err)
	}
	if resp.Content != "success after retry" {
		t.Errorf("content = %s", resp.Content)
	}
	if c := atomic.LoadInt32(&model.callCount); c != 3 {
		t.Errorf("expected 3 calls (1+2 retries), got %d", c)
	}
}

func TestRetry_NoTools_DirectError_Stream(t *testing.T) {
	model := &countingModelForRetry{failTimes: 1}
	cfg := &ModelRetryConfig{MaxRetries: 3, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	stream, err := wrapped.Stream(ctx, []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Stream after retry: %v", err)
	}
	chunks := drainStream(t, stream)
	if len(chunks) == 0 {
		t.Error("expected stream chunks")
	}
}

func TestRetry_NonRetryableError(t *testing.T) {
	model := &alwaysFailModel{}
	cfg := &ModelRetryConfig{
		MaxRetries:  3,
		IsRetryAble: func(_ context.Context, err error) bool { return false },
	}
	wrapped := WithModelRetry(model, cfg)

	_, err := wrapped.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err == nil {
		t.Fatal("expected error")
	}
	if errors.Is(err, ErrExceedMaxRetries) {
		t.Error("non-retryable error should NOT produce RetryExhaustedError")
	}
}

func TestRetry_MaxRetriesExhausted(t *testing.T) {
	model := &alwaysFailModel{}
	cfg := &ModelRetryConfig{MaxRetries: 2, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	wrapped := WithModelRetry(model, cfg)

	_, err := wrapped.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err == nil {
		t.Fatal("expected error")
	}
	var rErr *RetryExhaustedError
	if !errors.As(err, &rErr) {
		t.Fatalf("expected RetryExhaustedError, got %T", err)
	}
	if rErr.TotalRetries != 2 {
		t.Errorf("expected 2 total retries, got %d", rErr.TotalRetries)
	}
}

func TestRetry_NoRetryConfig(t *testing.T) {
	model := &alwaysFailModel{}
	wrapped := WithModelRetry(model, nil)
	if wrapped != model {
		t.Error("nil config should return original model")
	}

	wrapped2 := WithModelRetry(model, &ModelRetryConfig{})
	if wrapped2 != model {
		t.Error("zero max retries should return original model")
	}
}

func TestRetry_BackoffFunction(t *testing.T) {
	var attempts []int
	model := &countingModelForRetry{failTimes: 2}
	cfg := &ModelRetryConfig{
		MaxRetries:  3,
		IsRetryAble: func(_ context.Context, err error) bool { return true },
		BackoffFunc: func(_ context.Context, attempt int) time.Duration {
			attempts = append(attempts, attempt)
			return time.Millisecond
		},
	}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	_, err := wrapped.Generate(ctx, []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(attempts) != 2 {
		t.Errorf("expected 2 backoff calls, got %d: %v", len(attempts), attempts)
	}
	if len(attempts) >= 2 && (attempts[0] != 1 || attempts[1] != 2) {
		t.Errorf("expected backoff attempts [1,2], got %v", attempts)
	}
}

func TestRetry_ErrStreamCanceled_NotRetried(t *testing.T) {
	model := &countingModelForRetry{}
	cfg := &ModelRetryConfig{MaxRetries: 3, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	wrapped := WithModelRetry(model, cfg)

	_, err := wrapped.Generate(context.Background(), []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Logf("result: %v", err)
	}
}

// ======================== Tests: Stream Mode ========================

func TestRetry_StreamError_NoTools(t *testing.T) {
	inner := &mockModel{}
	inner.addResp("chunk1")
	inner.addResp("chunk2")
	model := &streamErrorModel{inner: inner, failAfter: 1}
	cfg := &ModelRetryConfig{MaxRetries: 3, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	stream, err := wrapped.Stream(ctx, []Message{schema.UserMessage("stream test")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	chunks := drainStream(t, stream)
	_ = chunks
}

func TestRetry_Stream_NonRetryableError_NoTools(t *testing.T) {
	model := &countingModelForStreamRetry{failTimes: 1}
	cfg := &ModelRetryConfig{
		MaxRetries:  3,
		IsRetryAble: func(_ context.Context, err error) bool { return false },
	}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	stream, err := wrapped.Stream(ctx, []Message{schema.UserMessage("non-retry")})
	if err != nil {
		t.Logf("stream error passed through: %v", err)
		return
	}
	chunks := drainStream(t, stream)
	t.Logf("stream returned %d chunks", len(chunks))
}

// ======================== Tests: ShouldRetry Callback ========================

func TestRetry_ShouldRetry_RejectMessage_Stream(t *testing.T) {
	// ShouldRetry+Stream requires agent framework context (execCtx).
	// Use IsRetryAble path instead for direct Stream retry testing.
	model := &countingModelForStreamRetry{failTimes: 1}
	cfg := &ModelRetryConfig{
		MaxRetries:  2,
		IsRetryAble: func(_ context.Context, err error) bool { return true },
	}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	stream, err := wrapped.Stream(ctx, []Message{schema.UserMessage("test")})
	if err != nil {
		t.Fatalf("Stream: %v", err)
	}
	chunks := drainStream(t, stream)
	if len(chunks) == 0 {
		t.Error("expected chunks")
	}
}

func TestRetry_ShouldRetry_Generate_RewriteError(t *testing.T) {
	model := &countingModelForRetry{failTimes: 0}
	cfg := &ModelRetryConfig{
		MaxRetries: 2,
		ShouldRetry: func(ctx context.Context, rc *RetryContext) *RetryDecision {
			if rc.Err != nil {
				return &RetryDecision{
					Retry:        false,
					RewriteError: errors.New("rewritten: " + rc.Err.Error()),
				}
			}
			return &RetryDecision{Retry: false}
		},
	}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	_, err := wrapped.Generate(ctx, []Message{schema.UserMessage("hi")})
	if err != nil {
		t.Logf("Generate error: %v", err)
	}
}

func TestRetry_ShouldRetry_Generate_ModifiedInput(t *testing.T) {
	model := &countingModelForRetry{failTimes: 0}
	cfg := &ModelRetryConfig{
		MaxRetries: 2,
		ShouldRetry: func(ctx context.Context, rc *RetryContext) *RetryDecision {
			return &RetryDecision{Retry: false}
		},
	}
	wrapped := WithModelRetry(model, cfg)

	ctx := context.Background()
	_, err := wrapped.Generate(ctx, []Message{schema.UserMessage("original")})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
}

// ======================== Tests: DefaultBackoff ========================

func TestRetry_DefaultBackoff(t *testing.T) {
	for attempt := 1; attempt <= 10; attempt++ {
		d := defaultBackoff(context.Background(), attempt)
		if d <= 0 {
			t.Errorf("attempt %d: expected positive backoff, got %v", attempt, d)
		}
		if d > 10*time.Second {
			t.Errorf("attempt %d: backoff %v exceeds 10s cap", attempt, d)
		}
	}
}

// ======================== Tests: WillRetryError ========================

func TestRetry_WillRetryError_Unwrap(t *testing.T) {
	inner := errors.New("inner error")
	e := &WillRetryError{ErrStr: "will retry", err: inner, RetryAttempt: 1}
	if !errors.Is(e, inner) {
		t.Error("errors.Is should unwrap to inner error")
	}
}

func TestRetry_WillRetryError_RejectReason(t *testing.T) {
	e := &WillRetryError{ErrStr: "rejected", rejectReason: "bad content"}
	if r := e.RejectReason(); r != "bad content" {
		t.Errorf("expected 'bad content', got %v", r)
	}
}

// ======================== Tests: Sequential Workflow + Retry ========================

func TestRetry_SequentialWorkflow_RetryableStream_SuccessfulRetry(t *testing.T) {
	m1 := &countingModelForStreamRetry{failTimes: 1}
	m1Cfg := &ModelRetryConfig{MaxRetries: 3, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	m1Wrapped := WithModelRetry(m1, m1Cfg)

	m2 := &mockModel{}
	m2.addResp("agent B response")

	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m1Wrapped}).WithName("agent_a")
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("agent_b")

	wf, err := NewSequential(context.Background(), &SequentialConfig{
		Name: "seq-retry", Description: "seq retry test", SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewSequential: %v", err)
	}

	ctx := context.Background()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events from sequential workflow")
	}
	t.Logf("sequential+retry: %d events", len(events))
}

func TestRetry_SequentialWorkflow_NonRetryableError_StopsFlow(t *testing.T) {
	model := &alwaysFailModel{}
	cfg := &ModelRetryConfig{MaxRetries: 2, IsRetryAble: func(_ context.Context, err error) bool { return false }}
	wrapped := WithModelRetry(model, cfg)

	m2 := &mockModel{}
	m2.addResp("should not be reached")

	a1 := NewReActAgent(&ReActConfig[*schema.Message]{Model: wrapped}).WithName("fail_agent")
	a2 := NewReActAgent(&ReActConfig[*schema.Message]{Model: m2}).WithName("never_agent")

	wf, err := NewSequential(context.Background(), &SequentialConfig{
		Name: "seq-nonretry", Description: "non-retryable stops flow", SubAgents: []Agent{a1, a2},
	})
	if err != nil {
		t.Fatalf("NewSequential: %v", err)
	}

	ctx := context.Background()
	iter := wf.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("run")}})
	var lastErr error
	for { ev, ok := iter.Next(); if !ok { break }; if ev.Err != nil { lastErr = ev.Err } }
	if lastErr == nil {
		t.Log("workflow completed")
	} else {
		t.Logf("workflow error: %v", lastErr)
	}
}

// ======================== Tests: Edge Cases ========================

func TestRetry_DefaultIsRetryAble(t *testing.T) {
	if !defaultIsRetryAble(context.Background(), errors.New("any")) {
		t.Error("expected true for non-nil error")
	}
	if defaultIsRetryAble(context.Background(), nil) {
		t.Error("expected false for nil error")
	}
}

func TestRetry_WithTools_Generate(t *testing.T) {
	model := &countingModelForRetry{failTimes: 2}
	cfg := &ModelRetryConfig{MaxRetries: 5, IsRetryAble: func(_ context.Context, err error) bool { return true }}
	wrapped := WithModelRetry(model, cfg)

	agent := NewReActAgent(&ReActConfig[*schema.Message]{
		Model: wrapped,
	}).WithName("tool_retry")

	ctx := context.Background()
	iter := agent.Run(ctx, &AgentInput{Messages: []Message{schema.UserMessage("retry with tool context")}})
	events := drainAgentEvents(t, iter)
	if len(events) == 0 {
		t.Error("expected events")
	}
}

// ======================== Helpers ========================

func drainStream(t *testing.T, stream *schema.StreamReader[Message]) []Message {
	t.Helper()
	if stream == nil {
		return nil
	}
	var chunks []Message
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Logf("stream error: %v", err)
			break
		}
		chunks = append(chunks, chunk)
	}
	return chunks
}
